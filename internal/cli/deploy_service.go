package cli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/alexander-fenster/decloud/internal/caddy"
	"github.com/alexander-fenster/decloud/internal/config"
	"github.com/alexander-fenster/decloud/internal/deploy"
	"github.com/alexander-fenster/decloud/internal/dockerdrv"
	"github.com/alexander-fenster/decloud/internal/envcap"
	"github.com/alexander-fenster/decloud/internal/registry"
	"github.com/spf13/cobra"
)

// deployerFactory is a package-global test seam. Tests reassign during setup
// and restore in teardown. Do NOT call t.Parallel() in any internal/cli test —
// concurrent reassignment is unsafe. If parallel CLI tests are ever needed,
// refactor to functional options on NewRootCmd. Same constraint applies to
// lifecycleFactory below.
var deployerFactory = buildProductionDeployer

// lifecycleFactory mirrors deployerFactory; same parallel-safety constraint.
var lifecycleFactory = buildProductionLifecycle

// caddyManagerFactory mirrors deployerFactory/lifecycleFactory; same
// parallel-safety constraint. Tests reassign during setup, restore in teardown.
var caddyManagerFactory = buildProductionCaddyManager

type deployServiceFlags struct {
	Name             string
	Hosts            []string
	Port             int
	EnvFile          string
	Mounts           []string
	ReadinessPath    string
	ReadinessTimeout time.Duration
	Strategy         string
	Dockerfile       string
}

func newDeployServiceCmd(rc *rootContext) *cobra.Command {
	var f deployServiceFlags
	cmd := &cobra.Command{
		Use:   "service [flags] <source-dir>",
		Short: "Deploy or redeploy a service",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDeployService(cmd.Context(), rc, &f, args[0])
		},
	}
	cmd.Flags().StringVar(&f.Name, "name", "", "service name (required, [a-z][a-z0-9-]{0,38})")
	cmd.Flags().StringSliceVar(&f.Hosts, "host", nil, "public hostname(s); repeatable")
	cmd.Flags().IntVar(&f.Port, "port", 0, "container listen port (required)")
	cmd.Flags().StringVar(&f.EnvFile, "env-file", "", "path to env.sh (default: <source-dir>/env.sh if present)")
	cmd.Flags().StringSliceVar(&f.Mounts, "mount", nil, "M1: rejected with ExitConfigError (M3 only)")
	cmd.Flags().StringVar(&f.ReadinessPath, "readiness-path", "/healthz", "HTTP readiness path")
	cmd.Flags().DurationVar(&f.ReadinessTimeout, "readiness-timeout", 60*time.Second, "total readiness wait")
	cmd.Flags().StringVar(&f.Strategy, "strategy", "recreate", "deploy strategy (M1: recreate only)")
	cmd.Flags().StringVar(&f.Dockerfile, "dockerfile", "Dockerfile", "Dockerfile path relative to <source-dir>")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

func runDeployService(ctx context.Context, rc *rootContext, f *deployServiceFlags, sourceDir string) error {
	if len(f.Mounts) > 0 {
		return fmt.Errorf("--mount is not supported until M3: %w", registry.ErrMountsNotSupported)
	}
	if f.Strategy != "recreate" {
		return fmt.Errorf("--strategy=%q: only \"recreate\" is supported in M1: %w", f.Strategy, registry.ErrInvalidStrategy)
	}
	if f.Port == 0 {
		return fmt.Errorf("--port is required: %w", errUsage)
	}
	abs, err := filepath.Abs(sourceDir)
	if err != nil {
		return fmt.Errorf("resolving source-dir: %w", err)
	}
	dockerfile := f.Dockerfile
	if dockerfile == "" {
		dockerfile = "Dockerfile"
	}
	if !filepath.IsAbs(dockerfile) {
		dockerfile = filepath.Join(abs, dockerfile)
	}
	envFile, err := resolveEnvFile(f.EnvFile, abs)
	if err != nil {
		return err
	}
	paths := config.NewPaths(rc.ConfigRoot)
	req := deploy.Request{
		Name:             f.Name,
		SourceDir:        abs,
		Dockerfile:       dockerfile,
		Hosts:            f.Hosts,
		Port:             f.Port,
		EnvFile:          envFile,
		ReadinessPath:    f.ReadinessPath,
		ReadinessTimeout: f.ReadinessTimeout,
		Strategy:         f.Strategy,
	}
	d, err := deployerFactory(paths)
	if err != nil {
		return fmt.Errorf("building deployer: %w", err)
	}
	return d.Deploy(ctx, req)
}

func resolveEnvFile(flagValue, sourceDir string) (string, error) {
	if flagValue != "" {
		if _, err := os.Stat(flagValue); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return "", fmt.Errorf("%w: %s", envcap.ErrEnvScriptMissing, flagValue)
			}
			return "", fmt.Errorf("%w: %s: %w", envcap.ErrEnvScriptUnreadable, flagValue, err)
		}
		return flagValue, nil
	}
	candidate := filepath.Join(sourceDir, "env.sh")
	if _, err := os.Stat(candidate); err == nil {
		return candidate, nil
	}
	return "", nil
}

func buildProductionDeployer(paths config.Paths) (deploy.ServiceDeployer, error) {
	driver := dockerdrv.NewCLIDriver()
	return deploy.NewServiceDeployer(deploy.Dependencies{
		Paths:     paths,
		Store:     registry.NewFSStore(paths),
		Capturer:  envcap.New(),
		Driver:    driver,
		Generator: caddy.NewGenerator(),
		Reloader:  caddy.NewCLIReloader(driver, paths.CaddyDir),
	})
}

func buildProductionLifecycle(paths config.Paths) (deploy.Lifecycle, error) {
	driver := dockerdrv.NewCLIDriver()
	return deploy.NewLifecycle(deploy.Dependencies{
		Paths:     paths,
		Store:     registry.NewFSStore(paths),
		Capturer:  envcap.New(),
		Driver:    driver,
		Generator: caddy.NewGenerator(),
		Reloader:  caddy.NewCLIReloader(driver, paths.CaddyDir),
	})
}

func buildProductionCaddyManager(paths config.Paths) (caddy.Manager, error) {
	return caddy.NewCLIManager(caddy.ManagerConfig{
		Driver: dockerdrv.NewCLIDriver(),
		Paths:  paths,
	}), nil
}
