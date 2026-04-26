package caddy

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

//go:generate mockgen -source=reloader.go -destination=mocks/mock_reloader.go -package=mocks

// Reloader wraps the local `caddy` binary's validate + reload subcommands.
type Reloader interface {
	Validate(ctx context.Context, configPath string) error
	Reload(ctx context.Context, configPath string) error
}

type cmdFactory func(ctx context.Context, name string, args ...string) *exec.Cmd

type cliReloader struct {
	cmd cmdFactory
}

// NewCLIReloader returns the production reloader that shells out to caddy.
func NewCLIReloader() Reloader {
	return &cliReloader{cmd: exec.CommandContext}
}

func newCLIReloaderWithFactory(f cmdFactory) Reloader {
	return &cliReloader{cmd: f}
}

func (r *cliReloader) Validate(ctx context.Context, configPath string) error {
	return r.runCaddy(ctx, "validate", configPath)
}

func (r *cliReloader) Reload(ctx context.Context, configPath string) error {
	return r.runCaddy(ctx, "reload", configPath)
}

func (r *cliReloader) runCaddy(ctx context.Context, sub, configPath string) error {
	var stderr bytes.Buffer
	cmd := r.cmd(ctx, "caddy", sub, "--config", configPath)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("caddy %s: %w; stderr=%q", sub, err, stderr.String())
	}
	return nil
}
