package deploy

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/alexander-fenster/decloud/internal/dockerdrv"
	"github.com/alexander-fenster/decloud/internal/ids"
	"github.com/alexander-fenster/decloud/internal/registry"
)

func (d *serviceDeployer) Unregister(ctx context.Context, name string) error {
	logger := slog.With("service", name, "lifecycle_op", "unregister")
	if _, err := d.deps.Store.Load(ctx, name); err != nil {
		if errors.Is(err, registry.ErrSecretsMissing) {
			logger.Info("config-only orphan; proceeding with unregister")
		} else {
			return fmt.Errorf("loading service: %w", err)
		}
	}
	containerName := ids.ContainerName(name)
	if err := d.deps.Driver.Stop(ctx, containerName, 10*time.Second); err != nil && !errors.Is(err, dockerdrv.ErrContainerNotFound) {
		logger.Warn("stop failed during unregister", "error", err)
	}
	if err := d.deps.Driver.Remove(ctx, containerName); err != nil && !errors.Is(err, dockerdrv.ErrContainerNotFound) {
		logger.Warn("remove failed during unregister", "error", err)
	}
	if err := d.deps.Store.Delete(ctx, name); err != nil {
		return fmt.Errorf("deleting registry entry: %w", err)
	}
	return d.regenerateAndReload(ctx)
}

func (d *serviceDeployer) Stop(ctx context.Context, name string) error {
	containerName := ids.ContainerName(name)
	if err := d.deps.Driver.Stop(ctx, containerName, 10*time.Second); err != nil {
		if errors.Is(err, dockerdrv.ErrContainerNotFound) {
			return fmt.Errorf("stopping container: %w", registry.ErrNotFound)
		}
		return fmt.Errorf("%w: stop %s: %w", ErrRun, containerName, err)
	}
	return nil
}

func (d *serviceDeployer) Start(ctx context.Context, name string) error {
	prev, err := d.deps.Store.Load(ctx, name)
	if err != nil {
		return fmt.Errorf("loading service: %w", err)
	}
	containerName := ids.ContainerName(name)
	inspect, err := d.deps.Driver.Inspect(ctx, containerName)
	if err != nil {
		return fmt.Errorf("%w: inspect %s: %w", ErrRun, containerName, err)
	}
	switch inspect.State {
	case "running":
		return nil
	case "exited":
		if err := d.deps.Driver.Start(ctx, containerName); err != nil {
			return fmt.Errorf("%w: start %s: %w", ErrRun, containerName, err)
		}
		return nil
	default:
		runReq := dockerdrv.RunRequest{
			Name:    containerName,
			Image:   prev.Config.Build.ImageRef,
			Network: "decloud",
			Env:     prev.Secrets.Env,
			Restart: prev.Config.Run.Restart,
			Port:    prev.Config.Run.Port,
		}
		if _, err := d.deps.Driver.Run(ctx, runReq); err != nil {
			return fmt.Errorf("%w: run %s: %w", ErrRun, containerName, err)
		}
		return nil
	}
}

func (d *serviceDeployer) Restart(ctx context.Context, name string) error {
	if err := d.Stop(ctx, name); err != nil && !errors.Is(err, registry.ErrNotFound) {
		return err
	}
	return d.Start(ctx, name)
}

func (d *serviceDeployer) Status(ctx context.Context, name string) (Status, error) {
	prev, err := d.deps.Store.Load(ctx, name)
	if err != nil {
		if errors.Is(err, registry.ErrSecretsMissing) {
			return Status{Name: name, State: "config-only"}, nil
		}
		return Status{}, fmt.Errorf("loading service: %w", err)
	}
	containerName := ids.ContainerName(name)
	inspect, err := d.deps.Driver.Inspect(ctx, containerName)
	if err != nil {
		return Status{}, fmt.Errorf("%w: inspect %s: %w", ErrRun, containerName, err)
	}
	state := inspect.State
	switch state {
	case "exited":
		state = "stopped"
	case "running", "absent":
	}
	return Status{
		Name:           prev.Config.Name,
		ContainerID:    inspect.ContainerID,
		ContainerName:  containerName,
		State:          state,
		LastDeployID:   prev.Config.State.LastDeployID,
		LastDeployedAt: prev.Config.LastDeployedAt,
	}, nil
}

func (d *serviceDeployer) Logs(ctx context.Context, name string, opts LogOptions) error {
	containerName := ids.ContainerName(name)
	err := d.deps.Driver.Logs(ctx, containerName, dockerdrv.LogsOptions{
		Follow: opts.Follow,
		Tail:   opts.Tail,
		Stdout: d.deps.Stdout,
		Stderr: d.deps.Stderr,
	})
	if err != nil {
		if errors.Is(err, dockerdrv.ErrContainerNotFound) {
			return fmt.Errorf("logs: %w", registry.ErrNotFound)
		}
		return fmt.Errorf("%w: logs %s: %w", ErrRun, containerName, err)
	}
	return nil
}

func (d *serviceDeployer) CaddyReload(ctx context.Context) error {
	return d.regenerateAndReload(ctx)
}
