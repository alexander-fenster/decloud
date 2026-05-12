package caddy_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alexander-fenster/decloud/internal/caddy"
	"github.com/alexander-fenster/decloud/internal/config"
	"github.com/alexander-fenster/decloud/internal/dockerdrv"
	dockermocks "github.com/alexander-fenster/decloud/internal/dockerdrv/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

type managerHarness struct {
	manager caddy.Manager
	driver  *dockermocks.MockDriver
	paths   config.Paths
	stdout  *bytes.Buffer
}

func newManagerHarness(t *testing.T) *managerHarness {
	t.Helper()
	ctrl := gomock.NewController(t)
	driver := dockermocks.NewMockDriver(ctrl)
	paths := config.NewPaths(t.TempDir())
	stdout := &bytes.Buffer{}
	mgr := caddy.NewCLIManager(caddy.ManagerConfig{
		Driver: driver,
		Paths:  paths,
		Stdout: stdout,
	})
	return &managerHarness{manager: mgr, driver: driver, paths: paths, stdout: stdout}
}

func absentInspect() dockerdrv.InspectResult {
	return dockerdrv.InspectResult{State: "absent"}
}

func runningInspect() dockerdrv.InspectResult {
	return dockerdrv.InspectResult{ContainerID: "cid", State: "running"}
}

func exitedInspect() dockerdrv.InspectResult {
	return dockerdrv.InspectResult{ContainerID: "cid", State: "exited"}
}

func expectedCaddyRunOptions(paths config.Paths) dockerdrv.RunOptions {
	return dockerdrv.RunOptions{
		Name:    caddy.ContainerName,
		Service: "caddy",
		Image:   caddy.DefaultImage,
		Network: caddy.NetworkName,
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
			{Source: paths.CaddyDir, Target: "/etc/caddy", ReadOnly: true, IsNamed: false},
			{Source: "decloud_caddy_data", Target: "/data", ReadOnly: false, IsNamed: true},
			{Source: "decloud_caddy_config", Target: "/config", ReadOnly: false, IsNamed: true},
		},
	}
}

func TestManager_UpFreshInstall(t *testing.T) {
	h := newManagerHarness(t)
	wantOpts := expectedCaddyRunOptions(h.paths)

	gomock.InOrder(
		h.driver.EXPECT().NetworkEnsure(gomock.Any(), caddy.NetworkName).Return(nil),
		h.driver.EXPECT().Inspect(gomock.Any(), caddy.ContainerName).Return(absentInspect(), nil),
		h.driver.EXPECT().ImagePull(gomock.Any(), caddy.DefaultImage).Return(nil),
		h.driver.EXPECT().RunWithOptions(gomock.Any(), wantOpts).Return("cid", nil),
	)

	require.NoError(t, h.manager.Up(context.Background()))
	assert.Contains(t, h.stdout.String(), "caddy:2")
	assert.Contains(t, h.stdout.String(), "decloud-caddy")
}

func TestManager_UpAlreadyRunning(t *testing.T) {
	h := newManagerHarness(t)

	gomock.InOrder(
		h.driver.EXPECT().NetworkEnsure(gomock.Any(), caddy.NetworkName).Return(nil),
		h.driver.EXPECT().Inspect(gomock.Any(), caddy.ContainerName).Return(runningInspect(), nil),
	)

	require.NoError(t, h.manager.Up(context.Background()))
	assert.Contains(t, h.stdout.String(), "caddy already running")
}

func TestManager_UpAfterPriorStop(t *testing.T) {
	h := newManagerHarness(t)

	gomock.InOrder(
		h.driver.EXPECT().NetworkEnsure(gomock.Any(), caddy.NetworkName).Return(nil),
		h.driver.EXPECT().Inspect(gomock.Any(), caddy.ContainerName).Return(exitedInspect(), nil),
		h.driver.EXPECT().Start(gomock.Any(), caddy.ContainerName).Return(nil),
	)

	require.NoError(t, h.manager.Up(context.Background()))
	assert.Contains(t, h.stdout.String(), "caddy started")
}

func TestManager_UpUnexpectedStateWraps(t *testing.T) {
	h := newManagerHarness(t)

	h.driver.EXPECT().NetworkEnsure(gomock.Any(), caddy.NetworkName).Return(nil)
	h.driver.EXPECT().Inspect(gomock.Any(), caddy.ContainerName).
		Return(dockerdrv.InspectResult{State: "paused"}, nil)

	err := h.manager.Up(context.Background())
	require.Error(t, err)
	assert.True(t, errors.Is(err, caddy.ErrCaddyUp))
	assert.Contains(t, err.Error(), "paused")
}

func TestManager_UpNetworkEnsureFails(t *testing.T) {
	h := newManagerHarness(t)
	innerErr := errors.New("docker daemon unreachable")

	h.driver.EXPECT().NetworkEnsure(gomock.Any(), caddy.NetworkName).Return(innerErr)

	err := h.manager.Up(context.Background())
	require.Error(t, err)
	assert.True(t, errors.Is(err, caddy.ErrCaddyUp))
	assert.True(t, errors.Is(err, innerErr))
}

func TestManager_UpImagePullFails(t *testing.T) {
	h := newManagerHarness(t)
	innerErr := errors.New("network unreachable")

	gomock.InOrder(
		h.driver.EXPECT().NetworkEnsure(gomock.Any(), caddy.NetworkName).Return(nil),
		h.driver.EXPECT().Inspect(gomock.Any(), caddy.ContainerName).Return(absentInspect(), nil),
		h.driver.EXPECT().ImagePull(gomock.Any(), caddy.DefaultImage).Return(innerErr),
	)

	err := h.manager.Up(context.Background())
	require.Error(t, err)
	assert.True(t, errors.Is(err, caddy.ErrCaddyUp))
	assert.True(t, errors.Is(err, innerErr))
}

func TestManager_UpRunFailsWithoutRollback(t *testing.T) {
	h := newManagerHarness(t)
	innerErr := errors.New("port allocation failed")

	gomock.InOrder(
		h.driver.EXPECT().NetworkEnsure(gomock.Any(), caddy.NetworkName).Return(nil),
		h.driver.EXPECT().Inspect(gomock.Any(), caddy.ContainerName).Return(absentInspect(), nil),
		h.driver.EXPECT().ImagePull(gomock.Any(), caddy.DefaultImage).Return(nil),
		h.driver.EXPECT().RunWithOptions(gomock.Any(), gomock.Any()).Return("", innerErr),
	)
	h.driver.EXPECT().Stop(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
	h.driver.EXPECT().Remove(gomock.Any(), gomock.Any()).Times(0)

	err := h.manager.Up(context.Background())
	require.Error(t, err)
	assert.True(t, errors.Is(err, caddy.ErrCaddyUp))
	assert.True(t, errors.Is(err, innerErr))
}

func TestManager_UpPortsBoundActionableError(t *testing.T) {
	cases := []struct {
		name      string
		stderrSub string
	}{
		{"kernel bind", "Error starting userland proxy: listen tcp4 0.0.0.0:80: bind: address already in use"},
		{"docker allocator", "Bind for 0.0.0.0:80 failed: port is already allocated"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newManagerHarness(t)
			runErr := fmt.Errorf("docker run: exit status 125; stderr=%q", tc.stderrSub)

			gomock.InOrder(
				h.driver.EXPECT().NetworkEnsure(gomock.Any(), caddy.NetworkName).Return(nil),
				h.driver.EXPECT().Inspect(gomock.Any(), caddy.ContainerName).Return(absentInspect(), nil),
				h.driver.EXPECT().ImagePull(gomock.Any(), caddy.DefaultImage).Return(nil),
				h.driver.EXPECT().RunWithOptions(gomock.Any(), gomock.Any()).Return("", runErr),
			)

			err := h.manager.Up(context.Background())
			require.Error(t, err)
			assert.True(t, errors.Is(err, caddy.ErrCaddyUp))
			assert.Contains(t, err.Error(), "ports 80/443 already in use")
			assert.Contains(t, err.Error(), "systemctl disable --now caddy && systemctl mask caddy")
			assert.Contains(t, err.Error(), "apt-get remove -y caddy")
			assert.NotContains(t, err.Error(), ": docker run: docker run:")
		})
	}
}

func TestManager_UpStubWriteFailsWrappedAsCaddyUp(t *testing.T) {
	h := newManagerHarness(t)
	require.NoError(t, os.MkdirAll(filepath.Dir(h.paths.CaddyDir), 0o755))
	require.NoError(t, os.WriteFile(h.paths.CaddyDir, []byte("not a directory"), 0o644))

	h.driver.EXPECT().NetworkEnsure(gomock.Any(), caddy.NetworkName).Return(nil)

	err := h.manager.Up(context.Background())
	require.Error(t, err)
	assert.True(t, errors.Is(err, caddy.ErrCaddyUp))
}

func TestManager_UpStubCaddyfileWritten(t *testing.T) {
	h := newManagerHarness(t)

	gomock.InOrder(
		h.driver.EXPECT().NetworkEnsure(gomock.Any(), caddy.NetworkName).Return(nil),
		h.driver.EXPECT().Inspect(gomock.Any(), caddy.ContainerName).Return(runningInspect(), nil),
	)

	require.NoError(t, h.manager.Up(context.Background()))
	_, err := os.Stat(h.paths.CaddyfilePath)
	assert.NoError(t, err, "Caddyfile stub must be written on Up")
}

func TestManager_UpStubWriteIdempotent(t *testing.T) {
	h := newManagerHarness(t)
	require.NoError(t, os.MkdirAll(h.paths.CaddyDir, 0o755))
	preExisting := []byte("# operator's existing Caddyfile\n")
	require.NoError(t, os.WriteFile(h.paths.CaddyfilePath, preExisting, 0o644))

	gomock.InOrder(
		h.driver.EXPECT().NetworkEnsure(gomock.Any(), caddy.NetworkName).Return(nil),
		h.driver.EXPECT().Inspect(gomock.Any(), caddy.ContainerName).Return(runningInspect(), nil),
	)

	require.NoError(t, h.manager.Up(context.Background()))
	got, err := os.ReadFile(h.paths.CaddyfilePath)
	require.NoError(t, err)
	assert.Equal(t, preExisting, got)
}

func TestManager_DownHappyPath(t *testing.T) {
	h := newManagerHarness(t)

	gomock.InOrder(
		h.driver.EXPECT().Stop(gomock.Any(), caddy.ContainerName, 10*time.Second).Return(nil),
		h.driver.EXPECT().Remove(gomock.Any(), caddy.ContainerName).Return(nil),
	)

	require.NoError(t, h.manager.Down(context.Background()))
	assert.Contains(t, h.stdout.String(), "caddy down")
}

func TestManager_DownContainerAbsent(t *testing.T) {
	h := newManagerHarness(t)

	gomock.InOrder(
		h.driver.EXPECT().Stop(gomock.Any(), caddy.ContainerName, 10*time.Second).
			Return(dockerdrv.ErrContainerNotFound),
		h.driver.EXPECT().Remove(gomock.Any(), caddy.ContainerName).
			Return(dockerdrv.ErrContainerNotFound),
	)

	require.NoError(t, h.manager.Down(context.Background()))
}

func TestManager_DownStopFailsHard(t *testing.T) {
	h := newManagerHarness(t)
	innerErr := errors.New("docker daemon down")

	h.driver.EXPECT().Stop(gomock.Any(), caddy.ContainerName, 10*time.Second).Return(innerErr)
	h.driver.EXPECT().Remove(gomock.Any(), gomock.Any()).Times(0)

	err := h.manager.Down(context.Background())
	require.Error(t, err)
	assert.True(t, errors.Is(err, caddy.ErrCaddyDown))
	assert.True(t, errors.Is(err, innerErr))
}

func TestManager_IsRunningTrue(t *testing.T) {
	h := newManagerHarness(t)
	h.driver.EXPECT().Inspect(gomock.Any(), caddy.ContainerName).Return(runningInspect(), nil)

	got, err := h.manager.IsRunning(context.Background())
	require.NoError(t, err)
	assert.True(t, got)
}

func TestManager_IsRunningFalseWhenExited(t *testing.T) {
	h := newManagerHarness(t)
	h.driver.EXPECT().Inspect(gomock.Any(), caddy.ContainerName).Return(exitedInspect(), nil)

	got, err := h.manager.IsRunning(context.Background())
	require.NoError(t, err)
	assert.False(t, got)
}

func TestManager_IsRunningFalseWhenAbsent(t *testing.T) {
	h := newManagerHarness(t)
	h.driver.EXPECT().Inspect(gomock.Any(), caddy.ContainerName).Return(absentInspect(), nil)

	got, err := h.manager.IsRunning(context.Background())
	require.NoError(t, err)
	assert.False(t, got)
}
