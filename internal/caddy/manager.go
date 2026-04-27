package caddy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/alexander-fenster/decloud/internal/config"
	"github.com/alexander-fenster/decloud/internal/dockerdrv"
)

//go:generate mockgen -source=manager.go -destination=mocks/mock_manager.go -package=mocks

const (
	ContainerName = "decloud-caddy"
	NetworkName   = "decloud"
	DefaultImage  = "caddy:2"
)

var (
	ErrCaddyUp   = errors.New("caddy: up failed")
	ErrCaddyDown = errors.New("caddy: down failed")
)

// Manager manages the lifecycle of the decloud-caddy container.
type Manager interface {
	// Up ensures the decloud network exists, the Caddyfile stub is on disk,
	// and the decloud-caddy container is running. Idempotent. Does NOT
	// probe Caddy's admin API; returns when docker run/start returns.
	Up(ctx context.Context) error

	// Down stops and removes the decloud-caddy container. Named volumes
	// (decloud_caddy_data, decloud_caddy_config) are NOT removed. Idempotent.
	Down(ctx context.Context) error

	// IsRunning reports whether the decloud-caddy container is in the
	// "running" state. Returns false (no error) when the container is absent
	// or in any non-running state.
	IsRunning(ctx context.Context) (bool, error)
}

// ManagerConfig wires a Manager to its dependencies.
type ManagerConfig struct {
	Driver dockerdrv.Driver
	Paths  config.Paths
	Stdout io.Writer
}

type cliManager struct {
	cfg ManagerConfig
}

// NewCLIManager returns the production manager.
func NewCLIManager(cfg ManagerConfig) Manager {
	if cfg.Stdout == nil {
		cfg.Stdout = os.Stdout
	}
	return &cliManager{cfg: cfg}
}

func (m *cliManager) Up(ctx context.Context) error {
	if err := m.cfg.Driver.NetworkEnsure(ctx, NetworkName); err != nil {
		return fmt.Errorf("%w: network ensure: %w", ErrCaddyUp, err)
	}
	if err := WriteStubIfMissing(m.cfg.Paths.CaddyfilePath); err != nil {
		return fmt.Errorf("%w: stub write: %w", ErrCaddyUp, err)
	}
	inspect, err := m.cfg.Driver.Inspect(ctx, ContainerName)
	if err != nil {
		return fmt.Errorf("%w: inspect: %w", ErrCaddyUp, err)
	}
	switch inspect.State {
	case "running":
		fmt.Fprintln(m.cfg.Stdout, "caddy already running")
		return nil
	case "exited":
		if err := m.cfg.Driver.Start(ctx, ContainerName); err != nil {
			return fmt.Errorf("%w: start: %w", ErrCaddyUp, err)
		}
		fmt.Fprintln(m.cfg.Stdout, "caddy started")
		return nil
	case "absent":
		// fall through to pull + run
	default:
		return fmt.Errorf("%w: unexpected container state %q", ErrCaddyUp, inspect.State)
	}
	if err := m.cfg.Driver.ImagePull(ctx, DefaultImage); err != nil {
		return fmt.Errorf("%w: image pull: %w", ErrCaddyUp, err)
	}
	fmt.Fprintf(m.cfg.Stdout, "caddy up: pulled %s\n", DefaultImage)
	if _, err := m.cfg.Driver.RunWithOptions(ctx, m.runOpts()); err != nil {
		if isPortsBoundErr(err) {
			return fmt.Errorf("%w: ports 80/443 already in use; if you ran the M1.0 install, run 'systemctl disable --now caddy && systemctl mask caddy' or 'apt-get remove -y caddy' to make the change persistent", ErrCaddyUp)
		}
		return fmt.Errorf("%w: docker run: %w", ErrCaddyUp, err)
	}
	fmt.Fprintf(m.cfg.Stdout, "caddy up: container %s running on network %s\n", ContainerName, NetworkName)
	return nil
}

func (m *cliManager) Down(ctx context.Context) error {
	if err := m.cfg.Driver.Stop(ctx, ContainerName, 10*time.Second); err != nil && !errors.Is(err, dockerdrv.ErrContainerNotFound) {
		return fmt.Errorf("%w: stop: %w", ErrCaddyDown, err)
	}
	if err := m.cfg.Driver.Remove(ctx, ContainerName); err != nil && !errors.Is(err, dockerdrv.ErrContainerNotFound) {
		return fmt.Errorf("%w: remove: %w", ErrCaddyDown, err)
	}
	fmt.Fprintln(m.cfg.Stdout, "caddy down: container removed (volumes retained)")
	return nil
}

func (m *cliManager) IsRunning(ctx context.Context) (bool, error) {
	inspect, err := m.cfg.Driver.Inspect(ctx, ContainerName)
	if err != nil {
		return false, err
	}
	return inspect.State == "running", nil
}

func (m *cliManager) runOpts() dockerdrv.RunOptions {
	return dockerdrv.RunOptions{
		Name:    ContainerName,
		Image:   DefaultImage,
		Network: NetworkName,
		Restart: "unless-stopped",
		Labels:  map[string]string{"decloud.managed": "caddy"},
		Ports: []dockerdrv.PortMap{
			{HostBind: "0.0.0.0", HostPort: 80, ContainerPort: 80, Proto: "tcp"},
			{HostBind: "[::]", HostPort: 80, ContainerPort: 80, Proto: "tcp"},
			{HostBind: "0.0.0.0", HostPort: 443, ContainerPort: 443, Proto: "tcp"},
			{HostBind: "[::]", HostPort: 443, ContainerPort: 443, Proto: "tcp"},
			{HostBind: "0.0.0.0", HostPort: 443, ContainerPort: 443, Proto: "udp"},
			{HostBind: "[::]", HostPort: 443, ContainerPort: 443, Proto: "udp"},
		},
		Volumes: []dockerdrv.VolumeMount{
			{Source: m.cfg.Paths.CaddyDir, Target: "/etc/caddy", ReadOnly: true, IsNamed: false},
			{Source: "decloud_caddy_data", Target: "/data", ReadOnly: false, IsNamed: true},
			{Source: "decloud_caddy_config", Target: "/config", ReadOnly: false, IsNamed: true},
		},
	}
}

// isPortsBoundErr matches the two stderr substrings docker emits when a
// publish-target port is already bound on the host: kernel bind(2) failure
// ("address already in use") and docker allocator failure ("port is already
// allocated"). Brittle to docker error-text wording; locked by
// TestManager_UpPortsBoundActionableError so a docker upgrade that reworded
// the message would fail loudly.
func isPortsBoundErr(err error) bool {
	s := err.Error()
	return strings.Contains(s, "address already in use") ||
		strings.Contains(s, "port is already allocated")
}
