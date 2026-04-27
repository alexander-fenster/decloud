package caddy

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/alexander-fenster/decloud/internal/dockerdrv"
)

//go:generate mockgen -source=reloader.go -destination=mocks/mock_reloader.go -package=mocks

// Reloader wraps Caddy's validate + reload subcommands inside the
// decloud-caddy container.
//
// IMPORTANT CONTRACT: configPath passed to Validate/Reload MUST be a host
// path inside the bind-mounted Caddyfile directory (Paths.CaddyDir). Paths
// outside that directory return an error with no exec attempt.
type Reloader interface {
	// Validate runs `caddy validate` against configPath translated to the
	// container's view of the bind mount. configPath must be inside
	// hostCaddyDir; otherwise an error is returned without invoking Caddy.
	Validate(ctx context.Context, configPath string) error

	// Reload runs `caddy reload` against configPath translated to the
	// container's view of the bind mount. Same path constraint as Validate.
	Reload(ctx context.Context, configPath string) error
}

type cliReloader struct {
	driver       dockerdrv.Driver
	hostCaddyDir string
}

// NewCLIReloader returns the production reloader. driver is the same
// dockerdrv.Driver the deploy uses; hostCaddyDir is the host-side directory
// bind-mounted into the decloud-caddy container at /etc/caddy.
func NewCLIReloader(driver dockerdrv.Driver, hostCaddyDir string) Reloader {
	return &cliReloader{driver: driver, hostCaddyDir: hostCaddyDir}
}

func (r *cliReloader) Validate(ctx context.Context, configPath string) error {
	return r.execCaddy(ctx, "validate", configPath)
}

func (r *cliReloader) Reload(ctx context.Context, configPath string) error {
	return r.execCaddy(ctx, "reload", configPath)
}

func (r *cliReloader) execCaddy(ctx context.Context, sub, hostPath string) error {
	ctrPath, err := r.translatePath(hostPath)
	if err != nil {
		return err
	}
	stderr := &bytes.Buffer{}
	err = r.driver.Exec(ctx, dockerdrv.ExecOptions{
		Container: ContainerName,
		Cmd:       []string{"caddy", sub, "--config", ctrPath},
		Stderr:    stderr,
	})
	if err == nil {
		return nil
	}
	if errors.Is(err, dockerdrv.ErrContainerNotFound) || isNotRunningStderr(stderr.String()) {
		return fmt.Errorf("caddy %s: container %q is not running; run 'decloud caddy up' first",
			sub, ContainerName)
	}
	return fmt.Errorf("caddy %s: %w; stderr=%q", sub, err, stderr.String())
}

func (r *cliReloader) translatePath(hostPath string) (string, error) {
	cleanHost := filepath.Clean(hostPath)
	cleanRoot := filepath.Clean(r.hostCaddyDir)
	rel, err := filepath.Rel(cleanRoot, cleanHost)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("caddy reloader: path %q is outside the bind-mount %q",
			hostPath, r.hostCaddyDir)
	}
	return path.Join("/etc/caddy", filepath.ToSlash(rel)), nil
}

// isNotRunningStderr matches docker exec stderr when the container exists
// but is not in the running state. ErrContainerNotFound covers the absent
// case; this covers exited/created/restarting.
func isNotRunningStderr(s string) bool {
	return strings.Contains(strings.ToLower(s), "is not running")
}
