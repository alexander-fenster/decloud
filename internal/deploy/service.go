package deploy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/alexander-fenster/decloud/internal/caddy"
	"github.com/alexander-fenster/decloud/internal/config"
	"github.com/alexander-fenster/decloud/internal/dockerdrv"
	"github.com/alexander-fenster/decloud/internal/envcap"
	"github.com/alexander-fenster/decloud/internal/ids"
	"github.com/alexander-fenster/decloud/internal/registry"
)

//go:generate mockgen -destination=../cli/mocks/mock_deployer.go -package=mocks github.com/alexander-fenster/decloud/internal/deploy ServiceDeployer
//go:generate mockgen -destination=../cli/mocks/mock_lifecycle.go -package=mocks github.com/alexander-fenster/decloud/internal/deploy Lifecycle

var (
	ErrEnvCapture  = errors.New("deploy: env capture failed")
	ErrBuild       = errors.New("deploy: docker build failed")
	ErrRun         = errors.New("deploy: docker run failed")
	ErrReadiness   = errors.New("deploy: readiness probe failed")
	ErrCaddyReload = errors.New("deploy: caddy reload failed")
	ErrInterrupted = errors.New("deploy: cancelled by user")
)

// cleanupTimeout bounds post-failure cleanup work: 10s docker stop grace,
// plus buffer for docker rm and a rollback docker run.
const cleanupTimeout = 30 * time.Second

// newCleanupContext returns a context derived from context.Background() with
// cleanupTimeout. It is intentionally NOT derived from the caller's ctx so
// that user cancellation does not abort cleanup. Callers MUST invoke the
// returned cancel func via defer.
func newCleanupContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), cleanupTimeout)
}

// isCancellation reports whether err is a context cancellation or deadline.
// Used by Deploy and its helpers to discriminate user-cancelled paths from
// genuine failures so the orchestrator can wrap as ErrInterrupted (exit 130)
// rather than the step-specific ErrRun/ErrReadiness sentinels.
func isCancellation(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

type Request struct {
	Name             string
	SourceDir        string
	Dockerfile       string
	Hosts            []string
	Port             int
	EnvFile          string
	Mounts           []registry.Mount
	ReadinessPath    string
	ReadinessTimeout time.Duration
	Strategy         string
}

type Status struct {
	Name           string
	ContainerID    string
	ContainerName  string
	State          string
	LastDeployID   string
	LastDeployedAt time.Time
	// ErrorDetail carries the wrapped error message when StatusAll could
	// not produce a real row for this service (State == "error"). Empty
	// for the single-service Status() path and for non-error multi-row
	// entries. NOT rendered in stdout; the CLI prints it to stderr.
	ErrorDetail string
}

type LogOptions struct {
	Follow bool
	Tail   int
}

type Dependencies struct {
	Paths     config.Paths
	Store     registry.Store
	Capturer  envcap.Capturer
	Driver    dockerdrv.Driver
	Generator caddy.Generator
	Reloader  caddy.Reloader
	Stdout    io.Writer
	Stderr    io.Writer
	// Probe overrides the readiness probe. Production leaves this nil so the
	// constructor wires the default HTTP probe (newHTTPProbe). Tests inject a
	// fake to avoid a real HTTP round-trip against the mocked Driver.ContainerIP.
	Probe ReadinessProbe
}

type ServiceDeployer interface {
	Deploy(ctx context.Context, req Request) error
}

type Lifecycle interface {
	Unregister(ctx context.Context, name string) error
	Start(ctx context.Context, name string) error
	Stop(ctx context.Context, name string) error
	Restart(ctx context.Context, name string) error
	Status(ctx context.Context, name string) (Status, error)
	StatusAll(ctx context.Context) ([]Status, error)
	Logs(ctx context.Context, name string, opts LogOptions) error
	CaddyReload(ctx context.Context) error
}

type serviceDeployer struct {
	deps  Dependencies
	probe ReadinessProbe
}

func newServiceDeployer(deps Dependencies) (*serviceDeployer, error) {
	if deps.Store == nil {
		return nil, errors.New("deploy: nil Store")
	}
	if deps.Driver == nil {
		return nil, errors.New("deploy: nil Driver")
	}
	if deps.Generator == nil {
		return nil, errors.New("deploy: nil Generator")
	}
	if deps.Reloader == nil {
		return nil, errors.New("deploy: nil Reloader")
	}
	if deps.Stdout == nil {
		deps.Stdout = os.Stdout
	}
	if deps.Stderr == nil {
		deps.Stderr = os.Stderr
	}
	probe := deps.Probe
	if probe == nil {
		probe = newHTTPProbe(deps.Driver)
	}
	return &serviceDeployer{deps: deps, probe: probe}, nil
}

// NewServiceDeployer returns the production deployer.
func NewServiceDeployer(deps Dependencies) (ServiceDeployer, error) {
	return newServiceDeployer(deps)
}

// NewLifecycle returns the production lifecycle controller.
func NewLifecycle(deps Dependencies) (Lifecycle, error) {
	return newServiceDeployer(deps)
}

func (d *serviceDeployer) Deploy(ctx context.Context, req Request) error {
	deployID := ids.NewDeployID()
	logger := slog.With("deploy_id", deployID, "service", req.Name)

	if err := d.deps.Driver.NetworkEnsure(ctx, "decloud"); err != nil {
		logger.Error("network ensure failed", "step", "network", "error", err)
		return fmt.Errorf("%w: ensuring decloud network: %w", ErrRun, err)
	}
	logger.Info("network ensured", "step", "network", "network", "decloud")

	envFile := req.EnvFile
	var captured map[string]string
	if envFile != "" {
		c, err := d.deps.Capturer.Capture(ctx, envFile)
		if err != nil {
			logger.Error("env capture failed", "step", "envcap", "error", err)
			return fmt.Errorf("%w: %w", ErrEnvCapture, err)
		}
		captured = c
		logger.Info("env captured", "step", "envcap", "vars_captured", len(captured))
	} else {
		logger.Info("env capture skipped: no env script", "step", "envcap")
	}

	prev, loadErr := d.deps.Store.Load(ctx, req.Name)
	hasPrev := loadErr == nil
	if loadErr != nil && !errors.Is(loadErr, registry.ErrNotFound) && !errors.Is(loadErr, registry.ErrSecretsMissing) {
		return fmt.Errorf("loading previous registration: %w", loadErr)
	}

	imageRef := ids.ImageRef(req.Name, deployID)
	containerName := ids.ContainerName(req.Name)

	if _, err := d.deps.Driver.Build(ctx, dockerdrv.BuildRequest{
		ImageRef:   imageRef,
		SourceDir:  req.SourceDir,
		Dockerfile: req.Dockerfile,
		Stdout:     d.deps.Stdout,
		Stderr:     d.deps.Stderr,
	}); err != nil {
		logger.Error("build failed", "step", "build", "error", err)
		return fmt.Errorf("%w: %w", ErrBuild, err)
	}
	logger.Info("build succeeded", "step", "build", "image", imageRef)

	if hasPrev {
		if err := d.deps.Driver.Stop(ctx, containerName, 10*time.Second); err != nil {
			if !errors.Is(err, dockerdrv.ErrContainerNotFound) {
				inspect, ierr := d.deps.Driver.Inspect(ctx, containerName)
				if ierr == nil && inspect.State == "running" {
					logger.Error("stop old container failed and still running", "step", "stop_old", "error", err)
					if isCancellation(err) {
						return fmt.Errorf("%w: %w", ErrInterrupted, err)
					}
					return fmt.Errorf("%w: stop previous container: %w", ErrRun, err)
				}
			}
		}
		if err := d.deps.Driver.Remove(ctx, containerName); err != nil && !errors.Is(err, dockerdrv.ErrContainerNotFound) {
			if isCancellation(err) {
				return fmt.Errorf("%w: %w", ErrInterrupted, err)
			}
			return fmt.Errorf("%w: remove previous container: %w", ErrRun, err)
		}
	} else {
		inspect, err := d.deps.Driver.Inspect(ctx, containerName)
		if err != nil {
			if isCancellation(err) {
				return fmt.Errorf("%w: %w", ErrInterrupted, err)
			}
			return fmt.Errorf("%w: inspect orphan check %s: %w", ErrRun, containerName, err)
		}
		if inspect.State != "absent" {
			labelVal := inspect.Labels["decloud.service"]
			if labelVal != req.Name {
				return fmt.Errorf("%w: container %s exists but was not created by decloud (label decloud.service=%q does not match %q); refusing to remove. Run 'docker rm -f %s' manually if you want to claim this name, or pick a different service name",
					ErrRun, containerName, labelVal, req.Name, containerName)
			}
			logger.Warn("removing orphan container from prior interrupted deploy",
				"container", containerName, "state", inspect.State)
			if err := d.deps.Driver.Stop(ctx, containerName, 10*time.Second); err != nil && !errors.Is(err, dockerdrv.ErrContainerNotFound) {
				if isCancellation(err) {
					return fmt.Errorf("%w: %w", ErrInterrupted, err)
				}
				return fmt.Errorf("%w: cleaning up orphan container %s; please run 'docker rm -f %s' and retry: %w", ErrRun, containerName, containerName, err)
			}
			if err := d.deps.Driver.Remove(ctx, containerName); err != nil && !errors.Is(err, dockerdrv.ErrContainerNotFound) {
				if isCancellation(err) {
					return fmt.Errorf("%w: %w", ErrInterrupted, err)
				}
				return fmt.Errorf("%w: cleaning up orphan container %s; please run 'docker rm -f %s' and retry: %w", ErrRun, containerName, containerName, err)
			}
		}
	}

	runReq := dockerdrv.RunRequest{
		Name:    containerName,
		Service: req.Name,
		Image:   imageRef,
		Network: "decloud",
		Env:     captured,
		Restart: "unless-stopped",
		Port:    req.Port,
		Volumes: toVolumeMounts(req.Mounts),
	}
	if _, err := d.deps.Driver.Run(ctx, runReq); err != nil {
		logger.Error("run new container failed", "step", "run_new", "error", err)
		if hasPrev {
			cleanupCtx, cleanupCancel := newCleanupContext()
			defer cleanupCancel()
			d.restoreOldContainer(cleanupCtx, prev)
		}
		if isCancellation(err) {
			return fmt.Errorf("%w: %w", ErrInterrupted, err)
		}
		return fmt.Errorf("%w: %w", ErrRun, err)
	}
	logger.Info("new container running", "step", "run_new", "container", containerName)

	spec := registry.ReadinessSpec{
		Kind:         "http",
		HTTPPath:     req.ReadinessPath,
		TimeoutSecs:  int(req.ReadinessTimeout.Seconds()),
		IntervalSecs: 1,
	}
	if spec.HTTPPath == "" {
		spec.HTTPPath = "/healthz"
	}
	if err := d.probe.Wait(ctx, containerName, spec, req.Port); err != nil {
		cancelled := isCancellation(err)
		if cancelled {
			logger.Info("deploy cancelled during readiness wait", "step", "readiness")
		} else {
			logger.Error("readiness failed", "step", "readiness", "error", err)
		}
		cleanupCtx, cleanupCancel := newCleanupContext()
		defer cleanupCancel()
		if stopErr := d.deps.Driver.Stop(cleanupCtx, containerName, 10*time.Second); stopErr != nil && !errors.Is(stopErr, dockerdrv.ErrContainerNotFound) {
			logger.Warn("cleanup failed; manual removal may be required",
				"container", containerName, "error", stopErr)
		}
		if rmErr := d.deps.Driver.Remove(cleanupCtx, containerName); rmErr != nil && !errors.Is(rmErr, dockerdrv.ErrContainerNotFound) {
			logger.Warn("cleanup failed; manual removal may be required",
				"container", containerName, "error", rmErr)
		}
		if hasPrev {
			d.restoreOldContainer(cleanupCtx, prev)
		}
		if cancelled {
			return fmt.Errorf("%w: %w", ErrInterrupted, err)
		}
		if errors.Is(err, ErrReadiness) {
			return err
		}
		return fmt.Errorf("%w: %w", ErrReadiness, err)
	}
	logger.Info("readiness passed", "step", "readiness")

	routes := make([]registry.Route, 0, len(req.Hosts))
	for _, h := range req.Hosts {
		routes = append(routes, registry.Route{Hostname: h})
	}
	svc := &registry.Service{
		Config: registry.ServiceConfig{
			SchemaVersion: registry.CurrentSchemaVersion,
			Name:          req.Name,
			Source:        registry.SourceSpec{Dir: req.SourceDir},
			Build:         registry.BuildSpec{Dockerfile: req.Dockerfile, ImageRef: imageRef},
			Run: registry.RunSpec{
				Network: "decloud",
				Port:    req.Port,
				Restart: "unless-stopped",
				Mounts:  req.Mounts,
			},
			Routes:    routes,
			Strategy:  req.Strategy,
			Readiness: spec,
			State: registry.ServiceState{
				LastDeployID:  deployID,
				ContainerName: containerName,
			},
			LastDeployedAt: time.Now().UTC(),
		},
		Secrets: registry.ServiceSecrets{
			SchemaVersion: registry.CurrentSchemaVersion,
			Name:          req.Name,
			Env:           captured,
		},
	}
	if err := d.deps.Store.Save(ctx, svc); err != nil {
		logger.Error("registry save failed", "step", "save_registry", "error", err)
		cleanupCtx, cleanupCancel := newCleanupContext()
		defer cleanupCancel()
		if errors.Is(err, registry.ErrPartialWrite) {
			if rbErr := d.deps.Store.DeleteOrphanConfig(cleanupCtx, req.Name); rbErr != nil {
				logger.Error("rollback: delete orphan config failed", "error", rbErr)
			}
		}
		if stopErr := d.deps.Driver.Stop(cleanupCtx, containerName, 10*time.Second); stopErr != nil && !errors.Is(stopErr, dockerdrv.ErrContainerNotFound) {
			logger.Warn("cleanup failed; manual removal may be required",
				"container", containerName, "error", stopErr)
		}
		if rmErr := d.deps.Driver.Remove(cleanupCtx, containerName); rmErr != nil && !errors.Is(rmErr, dockerdrv.ErrContainerNotFound) {
			logger.Warn("cleanup failed; manual removal may be required",
				"container", containerName, "error", rmErr)
		}
		if hasPrev {
			d.restoreOldContainer(cleanupCtx, prev)
		}
		if isCancellation(err) {
			return fmt.Errorf("%w: %w", ErrInterrupted, err)
		}
		return fmt.Errorf("registry save: %w", err)
	}
	logger.Info("registry saved", "step", "save_registry")

	if err := d.regenerateAndReload(ctx); err != nil {
		logger.Error("caddy step failed", "error", err)
		return err
	}
	logger.Info("deploy complete")
	return nil
}

func (d *serviceDeployer) restoreOldContainer(cleanupCtx context.Context, prev *registry.Service) {
	if prev == nil {
		return
	}
	runReq := dockerdrv.RunRequest{
		Name:    ids.ContainerName(prev.Config.Name),
		Service: prev.Config.Name,
		Image:   prev.Config.Build.ImageRef,
		Network: "decloud",
		Env:     prev.Secrets.Env,
		Restart: prev.Config.Run.Restart,
		Port:    prev.Config.Run.Port,
		Volumes: toVolumeMounts(prev.Config.Run.Mounts),
	}
	if _, err := d.deps.Driver.Run(cleanupCtx, runReq); err != nil {
		slog.Error("rollback: failed to restart previous container",
			"service", prev.Config.Name, "error", err)
		return
	}
	slog.Info("rollback: previous container restored", "service", prev.Config.Name)
}

func (d *serviceDeployer) regenerateAndReload(ctx context.Context) error {
	services, err := d.deps.Store.List(ctx)
	if err != nil {
		return fmt.Errorf("listing services: %w", err)
	}
	if err := caddy.WriteStubIfMissing(d.deps.Paths.CaddyfilePath); err != nil {
		slog.Warn("caddy stub write failed", "error", err)
	}
	tmpPath := d.deps.Paths.CaddyfilePath + ".tmp"
	if err := d.deps.Generator.Generate(tmpPath, services); err != nil {
		return fmt.Errorf("%w: generating caddyfile: %w", ErrCaddyReload, err)
	}
	if err := d.deps.Reloader.Validate(ctx, tmpPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("%w: caddy validate failed: %w; service is registered and running but Caddy is not routing traffic; run 'decloud caddy up' (and then 'decloud caddy reload' if needed) to restore routing", ErrCaddyReload, err)
	}
	if err := os.Rename(tmpPath, d.deps.Paths.CaddyfilePath); err != nil {
		return fmt.Errorf("%w: rename caddyfile: %w", ErrCaddyReload, err)
	}
	if err := d.deps.Reloader.Reload(ctx, d.deps.Paths.CaddyfilePath); err != nil {
		return fmt.Errorf("%w: caddy reload failed: %w; service is registered and running but Caddy is not routing traffic; run 'decloud caddy up' (and then 'decloud caddy reload' if needed) to restore routing", ErrCaddyReload, err)
	}
	return nil
}

// toVolumeMounts converts persisted registry mounts into the driver's
// VolumeMount shape. The IsNamed flag is derived from the source string
// using the convention documented on registry.Mount.IsNamed: bind sources
// start with "/", named-volume sources do not.
func toVolumeMounts(mounts []registry.Mount) []dockerdrv.VolumeMount {
	if len(mounts) == 0 {
		return nil
	}
	out := make([]dockerdrv.VolumeMount, 0, len(mounts))
	for _, m := range mounts {
		out = append(out, dockerdrv.VolumeMount{
			Source:   m.HostPath,
			Target:   m.ContainerPath,
			ReadOnly: m.ReadOnly,
			IsNamed:  m.IsNamed(),
		})
	}
	return out
}
