package caddy_test

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/alexander-fenster/decloud/internal/caddy"
	"github.com/alexander-fenster/decloud/internal/dockerdrv"
	dockermocks "github.com/alexander-fenster/decloud/internal/dockerdrv/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

const hostCaddyDir = "/opt/decloud/config/caddy"

type reloaderHarness struct {
	reloader caddy.Reloader
	driver   *dockermocks.MockDriver
}

func newReloaderHarness(t *testing.T) *reloaderHarness {
	t.Helper()
	ctrl := gomock.NewController(t)
	driver := dockermocks.NewMockDriver(ctrl)
	return &reloaderHarness{
		reloader: caddy.NewCLIReloader(driver, hostCaddyDir),
		driver:   driver,
	}
}

type capturedExec struct {
	opts dockerdrv.ExecOptions
}

func captureExec(_ *testing.T, h *reloaderHarness, ret error, stderrText string) *capturedExec {
	c := &capturedExec{}
	h.driver.EXPECT().Exec(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, opts dockerdrv.ExecOptions) error {
			c.opts = opts
			if stderrText != "" && opts.Stderr != nil {
				_, _ = io.WriteString(opts.Stderr, stderrText)
			}
			return ret
		})
	return c
}

func TestReloader_ValidateCallsDockerExec(t *testing.T) {
	h := newReloaderHarness(t)
	c := captureExec(t, h, nil, "")

	require.NoError(t, h.reloader.Validate(context.Background(),
		hostCaddyDir+"/Caddyfile.tmp"))
	assert.Equal(t, caddy.ContainerName, c.opts.Container)
	assert.Equal(t, []string{
		"caddy", "validate", "--config", "/etc/caddy/Caddyfile.tmp",
	}, c.opts.Cmd)
}

func TestReloader_ReloadCallsDockerExec(t *testing.T) {
	h := newReloaderHarness(t)
	c := captureExec(t, h, nil, "")

	require.NoError(t, h.reloader.Reload(context.Background(),
		hostCaddyDir+"/Caddyfile"))
	assert.Equal(t, caddy.ContainerName, c.opts.Container)
	assert.Equal(t, []string{
		"caddy", "reload", "--config", "/etc/caddy/Caddyfile",
	}, c.opts.Cmd)
}

func TestReloader_PathTranslationCanonicalForm(t *testing.T) {
	h := newReloaderHarness(t)
	c := captureExec(t, h, nil, "")

	require.NoError(t, h.reloader.Validate(context.Background(),
		hostCaddyDir+"/Caddyfile.tmp"))
	assert.Equal(t, "/etc/caddy/Caddyfile.tmp", c.opts.Cmd[3])
}

func TestReloader_PathTranslationOutsideBindMount(t *testing.T) {
	h := newReloaderHarness(t)
	h.driver.EXPECT().Exec(gomock.Any(), gomock.Any()).Times(0)

	err := h.reloader.Validate(context.Background(), "/tmp/foo")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "outside the bind-mount")
}

func TestReloader_PathTranslationParentEscape(t *testing.T) {
	h := newReloaderHarness(t)
	h.driver.EXPECT().Exec(gomock.Any(), gomock.Any()).Times(0)

	err := h.reloader.Validate(context.Background(),
		hostCaddyDir+"/../../etc/passwd")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "outside the bind-mount")
}

func TestReloader_ContainerNotRunningSurfacesActionableError(t *testing.T) {
	h := newReloaderHarness(t)
	captureExec(t, h, dockerdrv.ErrContainerNotFound, "")

	err := h.reloader.Validate(context.Background(), hostCaddyDir+"/Caddyfile")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `container "decloud-caddy" is not running`)
	assert.Contains(t, err.Error(), "decloud caddy up")
}

func TestReloader_ContainerExitedSurfacesActionableError(t *testing.T) {
	h := newReloaderHarness(t)
	captureExec(t, h, errors.New("exit status 1"),
		"Error response from daemon: Container decloud-caddy is not running")

	err := h.reloader.Validate(context.Background(), hostCaddyDir+"/Caddyfile")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `container "decloud-caddy" is not running`)
	assert.Contains(t, err.Error(), "decloud caddy up")
}

func TestReloader_ValidateExitNonzeroPreservesStderr(t *testing.T) {
	h := newReloaderHarness(t)
	innerErr := errors.New("exit status 1")
	captureExec(t, h, innerErr, "bad caddyfile syntax at line 5")

	err := h.reloader.Validate(context.Background(), hostCaddyDir+"/Caddyfile")
	require.Error(t, err)
	assert.True(t, errors.Is(err, innerErr))
	assert.Contains(t, err.Error(), "bad caddyfile syntax")
}

func TestReloader_StderrIsCapturedEvenWithoutCallerWriter(t *testing.T) {
	h := newReloaderHarness(t)
	h.driver.EXPECT().Exec(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, opts dockerdrv.ExecOptions) error {
			require.NotNil(t, opts.Stderr,
				"reloader must always supply a Stderr buffer for not-running detection")
			_, _ = opts.Stderr.Write([]byte("dummy stderr"))
			return errors.New("exit 1")
		})

	err := h.reloader.Validate(context.Background(), hostCaddyDir+"/Caddyfile")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dummy stderr")
}
