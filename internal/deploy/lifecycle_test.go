package deploy_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	caddymocks "github.com/alexander-fenster/decloud/internal/caddy/mocks"
	"github.com/alexander-fenster/decloud/internal/config"
	"github.com/alexander-fenster/decloud/internal/deploy"
	"github.com/alexander-fenster/decloud/internal/dockerdrv"
	dockermocks "github.com/alexander-fenster/decloud/internal/dockerdrv/mocks"
	envcapmocks "github.com/alexander-fenster/decloud/internal/envcap/mocks"
	"github.com/alexander-fenster/decloud/internal/registry"
	registrymocks "github.com/alexander-fenster/decloud/internal/registry/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

type lifecycleHarness struct {
	lc        deploy.Lifecycle
	store     *registrymocks.MockStore
	driver    *dockermocks.MockDriver
	generator *caddymocks.MockGenerator
	reloader  *caddymocks.MockReloader
	stdout    *bytes.Buffer
	stderr    *bytes.Buffer
}

func newLifecycleHarness(t *testing.T) *lifecycleHarness {
	t.Helper()
	ctrl := gomock.NewController(t)
	root := t.TempDir()
	paths := config.NewPaths(root)
	store := registrymocks.NewMockStore(ctrl)
	capturer := envcapmocks.NewMockCapturer(ctrl)
	driver := dockermocks.NewMockDriver(ctrl)
	gen := caddymocks.NewMockGenerator(ctrl)
	rel := caddymocks.NewMockReloader(ctrl)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	lc, err := deploy.NewLifecycle(deploy.Dependencies{
		Paths:     paths,
		Store:     store,
		Capturer:  capturer,
		Driver:    driver,
		Generator: gen,
		Reloader:  rel,
		Stdout:    stdout,
		Stderr:    stderr,
	})
	require.NoError(t, err)
	return &lifecycleHarness{
		lc:        lc,
		store:     store,
		driver:    driver,
		generator: gen,
		reloader:  rel,
		stdout:    stdout,
		stderr:    stderr,
	}
}

func newRegisteredService() *registry.Service {
	return &registry.Service{
		Config: registry.ServiceConfig{
			SchemaVersion: 1,
			Name:          "foo",
			Build:         registry.BuildSpec{ImageRef: "decloud-foo:20260426-120000-abc123"},
			Run:           registry.RunSpec{Network: "decloud", Port: 8080, Restart: "unless-stopped"},
			Strategy:      "recreate",
		},
		Secrets: registry.ServiceSecrets{SchemaVersion: 1, Name: "foo", Env: map[string]string{"X": "1"}},
	}
}

// ----------------------------------------------------------------------------
// Unregister (Joel §9.6.1)
// ----------------------------------------------------------------------------

func TestLifecycle_UnregisterHappyPath(t *testing.T) {
	h := newLifecycleHarness(t)
	prev := newRegisteredService()

	gomock.InOrder(
		h.store.EXPECT().Load(gomock.Any(), "foo").Return(prev, nil),
		h.driver.EXPECT().Stop(gomock.Any(), "decloud-foo", gomock.Any()).Return(nil),
		h.driver.EXPECT().Remove(gomock.Any(), "decloud-foo").Return(nil),
		h.store.EXPECT().Delete(gomock.Any(), "foo").Return(nil),
		h.store.EXPECT().List(gomock.Any()).Return(nil, nil),
		h.generator.EXPECT().Generate(gomock.Any(), gomock.Any()).DoAndReturn(stubGenerate),
		h.reloader.EXPECT().Validate(gomock.Any(), gomock.Any()).Return(nil),
		h.reloader.EXPECT().Reload(gomock.Any(), gomock.Any()).Return(nil),
	)

	require.NoError(t, h.lc.Unregister(context.Background(), "foo"))
}

func TestLifecycle_UnregisterServiceNotFoundReturnsErrNotFound(t *testing.T) {
	h := newLifecycleHarness(t)
	h.store.EXPECT().Load(gomock.Any(), "foo").Return(nil, registry.ErrNotFound)

	err := h.lc.Unregister(context.Background(), "foo")
	require.Error(t, err)
	assert.True(t, errors.Is(err, registry.ErrNotFound))
}

func TestLifecycle_UnregisterContinuesIfContainerAlreadyGone(t *testing.T) {
	h := newLifecycleHarness(t)
	prev := newRegisteredService()

	gomock.InOrder(
		h.store.EXPECT().Load(gomock.Any(), "foo").Return(prev, nil),
		h.driver.EXPECT().Stop(gomock.Any(), "decloud-foo", gomock.Any()).Return(dockerdrv.ErrContainerNotFound),
		h.driver.EXPECT().Remove(gomock.Any(), "decloud-foo").Return(dockerdrv.ErrContainerNotFound),
		h.store.EXPECT().Delete(gomock.Any(), "foo").Return(nil),
		h.store.EXPECT().List(gomock.Any()).Return(nil, nil),
		h.generator.EXPECT().Generate(gomock.Any(), gomock.Any()).DoAndReturn(stubGenerate),
		h.reloader.EXPECT().Validate(gomock.Any(), gomock.Any()).Return(nil),
		h.reloader.EXPECT().Reload(gomock.Any(), gomock.Any()).Return(nil),
	)

	require.NoError(t, h.lc.Unregister(context.Background(), "foo"))
}

func TestLifecycle_UnregisterConfigOnlyOrphanProceeds(t *testing.T) {
	h := newLifecycleHarness(t)

	h.store.EXPECT().Load(gomock.Any(), "foo").Return(nil, registry.ErrSecretsMissing)
	h.driver.EXPECT().Stop(gomock.Any(), "decloud-foo", gomock.Any()).Return(dockerdrv.ErrContainerNotFound)
	h.driver.EXPECT().Remove(gomock.Any(), "decloud-foo").Return(dockerdrv.ErrContainerNotFound)
	h.store.EXPECT().Delete(gomock.Any(), "foo").Return(nil)
	h.store.EXPECT().List(gomock.Any()).Return(nil, nil)
	h.generator.EXPECT().Generate(gomock.Any(), gomock.Any()).DoAndReturn(stubGenerate)
	h.reloader.EXPECT().Validate(gomock.Any(), gomock.Any()).Return(nil)
	h.reloader.EXPECT().Reload(gomock.Any(), gomock.Any()).Return(nil)

	require.NoError(t, h.lc.Unregister(context.Background(), "foo"))
}

func TestLifecycle_UnregisterCaddyValidateFailureReturnsErrCaddyReload(t *testing.T) {
	h := newLifecycleHarness(t)
	prev := newRegisteredService()

	h.store.EXPECT().Load(gomock.Any(), "foo").Return(prev, nil)
	h.driver.EXPECT().Stop(gomock.Any(), "decloud-foo", gomock.Any()).Return(nil)
	h.driver.EXPECT().Remove(gomock.Any(), "decloud-foo").Return(nil)
	h.store.EXPECT().Delete(gomock.Any(), "foo").Return(nil)
	h.store.EXPECT().List(gomock.Any()).Return(nil, nil)
	h.generator.EXPECT().Generate(gomock.Any(), gomock.Any()).DoAndReturn(stubGenerate)
	h.reloader.EXPECT().Validate(gomock.Any(), gomock.Any()).Return(errors.New("validate failed"))

	err := h.lc.Unregister(context.Background(), "foo")
	require.Error(t, err)
	assert.True(t, errors.Is(err, deploy.ErrCaddyReload))
}

func TestLifecycle_UnregisterCaddyReloadFailureReturnsErrCaddyReload(t *testing.T) {
	h := newLifecycleHarness(t)
	prev := newRegisteredService()

	h.store.EXPECT().Load(gomock.Any(), "foo").Return(prev, nil)
	h.driver.EXPECT().Stop(gomock.Any(), "decloud-foo", gomock.Any()).Return(nil)
	h.driver.EXPECT().Remove(gomock.Any(), "decloud-foo").Return(nil)
	h.store.EXPECT().Delete(gomock.Any(), "foo").Return(nil)
	h.store.EXPECT().List(gomock.Any()).Return(nil, nil)
	h.generator.EXPECT().Generate(gomock.Any(), gomock.Any()).DoAndReturn(stubGenerate)
	h.reloader.EXPECT().Validate(gomock.Any(), gomock.Any()).Return(nil)
	h.reloader.EXPECT().Reload(gomock.Any(), gomock.Any()).Return(errors.New("reload failed"))

	err := h.lc.Unregister(context.Background(), "foo")
	require.Error(t, err)
	assert.True(t, errors.Is(err, deploy.ErrCaddyReload))
}

// ----------------------------------------------------------------------------
// Stop (Joel §9.6.2)
// ----------------------------------------------------------------------------

func TestLifecycle_StopHappyPath(t *testing.T) {
	h := newLifecycleHarness(t)
	h.driver.EXPECT().Stop(gomock.Any(), "decloud-foo", gomock.Any()).Return(nil)

	require.NoError(t, h.lc.Stop(context.Background(), "foo"))
}

func TestLifecycle_StopAlreadyStoppedIsIdempotent(t *testing.T) {
	h := newLifecycleHarness(t)
	h.driver.EXPECT().Stop(gomock.Any(), "decloud-foo", gomock.Any()).Return(nil)

	require.NoError(t, h.lc.Stop(context.Background(), "foo"))
}

func TestLifecycle_StopServiceNotFoundReturnsErrNotFound(t *testing.T) {
	h := newLifecycleHarness(t)
	h.driver.EXPECT().Stop(gomock.Any(), "decloud-foo", gomock.Any()).Return(dockerdrv.ErrContainerNotFound)

	err := h.lc.Stop(context.Background(), "foo")
	require.Error(t, err)
	assert.True(t, errors.Is(err, registry.ErrNotFound))
}

func TestLifecycle_StopUnexpectedDriverErrorReturnsErrRun(t *testing.T) {
	h := newLifecycleHarness(t)
	h.driver.EXPECT().Stop(gomock.Any(), "decloud-foo", gomock.Any()).Return(errors.New("connection reset"))

	err := h.lc.Stop(context.Background(), "foo")
	require.Error(t, err)
	assert.True(t, errors.Is(err, deploy.ErrRun))
}

// ----------------------------------------------------------------------------
// Start (Joel §9.6.3)
// ----------------------------------------------------------------------------

func TestLifecycle_StartFromExited(t *testing.T) {
	h := newLifecycleHarness(t)
	prev := newRegisteredService()

	gomock.InOrder(
		h.store.EXPECT().Load(gomock.Any(), "foo").Return(prev, nil),
		h.driver.EXPECT().Inspect(gomock.Any(), "decloud-foo").
			Return(dockerdrv.InspectResult{ContainerID: "cid", State: "exited"}, nil),
		h.driver.EXPECT().Start(gomock.Any(), "decloud-foo").Return(nil),
	)

	require.NoError(t, h.lc.Start(context.Background(), "foo"))
}

func TestLifecycle_StartFromAbsentReRunsContainer(t *testing.T) {
	h := newLifecycleHarness(t)
	prev := newRegisteredService()
	var seen dockerdrv.RunRequest

	gomock.InOrder(
		h.store.EXPECT().Load(gomock.Any(), "foo").Return(prev, nil),
		h.driver.EXPECT().Inspect(gomock.Any(), "decloud-foo").
			Return(dockerdrv.InspectResult{State: "absent"}, nil),
		h.driver.EXPECT().Run(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, req dockerdrv.RunRequest) (string, error) {
				seen = req
				return "cid", nil
			}),
	)

	require.NoError(t, h.lc.Start(context.Background(), "foo"))
	assert.Equal(t, "decloud-foo", seen.Name)
	assert.Equal(t, prev.Config.Build.ImageRef, seen.Image)
	assert.Equal(t, "decloud", seen.Network)
	assert.Equal(t, prev.Secrets.Env, seen.Env)
	assert.Equal(t, "foo", seen.Service,
		"absent-state Start must populate RunRequest.Service so the re-run container gets tag=decloud/foo")
}

func TestLifecycle_StartAbsentBranchPassesVolumesToDriver(t *testing.T) {
	h := newLifecycleHarness(t)
	prev := newRegisteredService()
	prev.Config.Run.Mounts = []registry.Mount{
		{HostPath: "/host", ContainerPath: "/data", ReadOnly: true},
		{HostPath: "vol", ContainerPath: "/var", ReadOnly: false},
	}
	var seen dockerdrv.RunRequest

	gomock.InOrder(
		h.store.EXPECT().Load(gomock.Any(), "foo").Return(prev, nil),
		h.driver.EXPECT().Inspect(gomock.Any(), "decloud-foo").
			Return(dockerdrv.InspectResult{State: "absent"}, nil),
		h.driver.EXPECT().Run(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, req dockerdrv.RunRequest) (string, error) {
				seen = req
				return "cid", nil
			}),
	)

	require.NoError(t, h.lc.Start(context.Background(), "foo"))
	require.Len(t, seen.Volumes, 2)
	assert.Equal(t, dockerdrv.VolumeMount{Source: "/host", Target: "/data", ReadOnly: true, IsNamed: false}, seen.Volumes[0])
	assert.Equal(t, dockerdrv.VolumeMount{Source: "vol", Target: "/var", ReadOnly: false, IsNamed: true}, seen.Volumes[1])
	assert.Equal(t, "foo", seen.Service)
}

func TestLifecycle_StartFromRunningIsNoOp(t *testing.T) {
	h := newLifecycleHarness(t)
	prev := newRegisteredService()

	h.store.EXPECT().Load(gomock.Any(), "foo").Return(prev, nil)
	h.driver.EXPECT().Inspect(gomock.Any(), "decloud-foo").
		Return(dockerdrv.InspectResult{ContainerID: "cid", State: "running"}, nil)

	require.NoError(t, h.lc.Start(context.Background(), "foo"))
}

func TestLifecycle_StartServiceNotFoundReturnsErrNotFound(t *testing.T) {
	h := newLifecycleHarness(t)
	h.store.EXPECT().Load(gomock.Any(), "foo").Return(nil, registry.ErrNotFound)

	err := h.lc.Start(context.Background(), "foo")
	require.Error(t, err)
	assert.True(t, errors.Is(err, registry.ErrNotFound))
}

func TestLifecycle_StartSecretsMissingReturnsErrSecretsMissing(t *testing.T) {
	h := newLifecycleHarness(t)
	h.store.EXPECT().Load(gomock.Any(), "foo").Return(nil, registry.ErrSecretsMissing)

	err := h.lc.Start(context.Background(), "foo")
	require.Error(t, err)
	assert.True(t, errors.Is(err, registry.ErrSecretsMissing))
}

func TestLifecycle_StartImageMissingReturnsErrRun(t *testing.T) {
	h := newLifecycleHarness(t)
	prev := newRegisteredService()

	h.store.EXPECT().Load(gomock.Any(), "foo").Return(prev, nil)
	h.driver.EXPECT().Inspect(gomock.Any(), "decloud-foo").
		Return(dockerdrv.InspectResult{State: "absent"}, nil)
	h.driver.EXPECT().Run(gomock.Any(), gomock.Any()).
		Return("", errors.New("Unable to find image 'decloud-foo:prev123' locally"))

	err := h.lc.Start(context.Background(), "foo")
	require.Error(t, err)
	assert.True(t, errors.Is(err, deploy.ErrRun))
}

// ----------------------------------------------------------------------------
// Restart (Joel §9.6.4)
// ----------------------------------------------------------------------------

func TestLifecycle_RestartHappyPath(t *testing.T) {
	h := newLifecycleHarness(t)
	prev := newRegisteredService()

	gomock.InOrder(
		h.driver.EXPECT().Stop(gomock.Any(), "decloud-foo", gomock.Any()).Return(nil),
		h.store.EXPECT().Load(gomock.Any(), "foo").Return(prev, nil),
		h.driver.EXPECT().Inspect(gomock.Any(), "decloud-foo").
			Return(dockerdrv.InspectResult{State: "exited"}, nil),
		h.driver.EXPECT().Start(gomock.Any(), "decloud-foo").Return(nil),
	)

	require.NoError(t, h.lc.Restart(context.Background(), "foo"))
}

func TestLifecycle_RestartFromAbsentReRunsContainer(t *testing.T) {
	h := newLifecycleHarness(t)
	prev := newRegisteredService()

	gomock.InOrder(
		h.driver.EXPECT().Stop(gomock.Any(), "decloud-foo", gomock.Any()).
			Return(dockerdrv.ErrContainerNotFound),
		h.store.EXPECT().Load(gomock.Any(), "foo").Return(prev, nil),
		h.driver.EXPECT().Inspect(gomock.Any(), "decloud-foo").
			Return(dockerdrv.InspectResult{State: "absent"}, nil),
		h.driver.EXPECT().Run(gomock.Any(), gomock.Any()).Return("cid", nil),
	)

	require.NoError(t, h.lc.Restart(context.Background(), "foo"))
}

func TestLifecycle_RestartStopFailureAbortsBeforeStart(t *testing.T) {
	h := newLifecycleHarness(t)
	h.driver.EXPECT().Stop(gomock.Any(), "decloud-foo", gomock.Any()).
		Return(errors.New("connection reset"))

	err := h.lc.Restart(context.Background(), "foo")
	require.Error(t, err)
	assert.True(t, errors.Is(err, deploy.ErrRun))
}

// ----------------------------------------------------------------------------
// Status (Joel §9.6.5)
// ----------------------------------------------------------------------------

func TestLifecycle_StatusRunning(t *testing.T) {
	h := newLifecycleHarness(t)
	prev := newRegisteredService()

	h.store.EXPECT().Load(gomock.Any(), "foo").Return(prev, nil)
	h.driver.EXPECT().Inspect(gomock.Any(), "decloud-foo").
		Return(dockerdrv.InspectResult{ContainerID: "cid", State: "running"}, nil)

	st, err := h.lc.Status(context.Background(), "foo")
	require.NoError(t, err)
	assert.Equal(t, "foo", st.Name)
	assert.Equal(t, "running", st.State)
	assert.Equal(t, "decloud-foo", st.ContainerName)
	assert.Equal(t, "cid", st.ContainerID)
}

func TestLifecycle_StatusStopped(t *testing.T) {
	h := newLifecycleHarness(t)
	prev := newRegisteredService()

	h.store.EXPECT().Load(gomock.Any(), "foo").Return(prev, nil)
	h.driver.EXPECT().Inspect(gomock.Any(), "decloud-foo").
		Return(dockerdrv.InspectResult{ContainerID: "cid", State: "exited"}, nil)

	st, err := h.lc.Status(context.Background(), "foo")
	require.NoError(t, err)
	assert.Equal(t, "stopped", st.State)
}

func TestLifecycle_StatusAbsentContainer(t *testing.T) {
	h := newLifecycleHarness(t)
	prev := newRegisteredService()

	h.store.EXPECT().Load(gomock.Any(), "foo").Return(prev, nil)
	h.driver.EXPECT().Inspect(gomock.Any(), "decloud-foo").
		Return(dockerdrv.InspectResult{State: "absent"}, nil)

	st, err := h.lc.Status(context.Background(), "foo")
	require.NoError(t, err)
	assert.Equal(t, "absent", st.State)
	assert.Empty(t, st.ContainerID)
}

func TestLifecycle_StatusConfigOnlyOrphan(t *testing.T) {
	h := newLifecycleHarness(t)
	h.store.EXPECT().Load(gomock.Any(), "foo").Return(nil, registry.ErrSecretsMissing)

	st, err := h.lc.Status(context.Background(), "foo")
	require.NoError(t, err)
	assert.Equal(t, "config-only", st.State)
	assert.Equal(t, "foo", st.Name)
}

func TestLifecycle_StatusServiceNotFoundReturnsErrNotFound(t *testing.T) {
	h := newLifecycleHarness(t)
	h.store.EXPECT().Load(gomock.Any(), "foo").Return(nil, registry.ErrNotFound)

	_, err := h.lc.Status(context.Background(), "foo")
	require.Error(t, err)
	assert.True(t, errors.Is(err, registry.ErrNotFound))
}

// ----------------------------------------------------------------------------
// Logs (Joel §9.6.6)
// ----------------------------------------------------------------------------

func TestLifecycle_LogsTailN(t *testing.T) {
	h := newLifecycleHarness(t)
	var seen dockerdrv.LogsOptions

	h.driver.EXPECT().Logs(gomock.Any(), "decloud-foo", gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, opts dockerdrv.LogsOptions) error {
			seen = opts
			return nil
		})

	require.NoError(t, h.lc.Logs(context.Background(), "foo", deploy.LogOptions{Tail: 50}))
	assert.Equal(t, 50, seen.Tail)
	assert.False(t, seen.Follow)
}

func TestLifecycle_LogsFollow(t *testing.T) {
	h := newLifecycleHarness(t)
	var seen dockerdrv.LogsOptions

	h.driver.EXPECT().Logs(gomock.Any(), "decloud-foo", gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, opts dockerdrv.LogsOptions) error {
			seen = opts
			return nil
		})

	require.NoError(t, h.lc.Logs(context.Background(), "foo", deploy.LogOptions{Follow: true}))
	assert.True(t, seen.Follow)
}

func TestLifecycle_LogsServiceNotFoundReturnsErrNotFound(t *testing.T) {
	h := newLifecycleHarness(t)
	h.driver.EXPECT().Logs(gomock.Any(), "decloud-foo", gomock.Any()).
		Return(dockerdrv.ErrContainerNotFound)

	err := h.lc.Logs(context.Background(), "foo", deploy.LogOptions{})
	require.Error(t, err)
	assert.True(t, errors.Is(err, registry.ErrNotFound))
}

// ----------------------------------------------------------------------------
// CaddyReload (Joel §9.6.7)
// ----------------------------------------------------------------------------

func TestLifecycle_CaddyReloadHappyPath(t *testing.T) {
	h := newLifecycleHarness(t)

	gomock.InOrder(
		h.store.EXPECT().List(gomock.Any()).Return(nil, nil),
		h.generator.EXPECT().Generate(gomock.Any(), gomock.Any()).DoAndReturn(stubGenerate),
		h.reloader.EXPECT().Validate(gomock.Any(), gomock.Any()).Return(nil),
		h.reloader.EXPECT().Reload(gomock.Any(), gomock.Any()).Return(nil),
	)

	require.NoError(t, h.lc.CaddyReload(context.Background()))
}

func TestLifecycle_CaddyReloadValidateFailureLeavesOldFileIntact(t *testing.T) {
	h := newLifecycleHarness(t)

	h.store.EXPECT().List(gomock.Any()).Return(nil, nil)
	h.generator.EXPECT().Generate(gomock.Any(), gomock.Any()).DoAndReturn(stubGenerate)
	h.reloader.EXPECT().Validate(gomock.Any(), gomock.Any()).Return(errors.New("syntax error"))

	err := h.lc.CaddyReload(context.Background())
	require.Error(t, err)
	assert.True(t, errors.Is(err, deploy.ErrCaddyReload))
}

func TestLifecycle_CaddyReloadReloadFailureNewFileOnDisk(t *testing.T) {
	h := newLifecycleHarness(t)

	h.store.EXPECT().List(gomock.Any()).Return(nil, nil)
	h.generator.EXPECT().Generate(gomock.Any(), gomock.Any()).DoAndReturn(stubGenerate)
	h.reloader.EXPECT().Validate(gomock.Any(), gomock.Any()).Return(nil)
	h.reloader.EXPECT().Reload(gomock.Any(), gomock.Any()).Return(errors.New("port already bound"))

	err := h.lc.CaddyReload(context.Background())
	require.Error(t, err)
	assert.True(t, errors.Is(err, deploy.ErrCaddyReload))
}

func TestLifecycle_CaddyReloadEmptyRegistryWritesStubOnly(t *testing.T) {
	h := newLifecycleHarness(t)

	h.store.EXPECT().List(gomock.Any()).Return([]*registry.Service{}, nil)
	h.generator.EXPECT().Generate(gomock.Any(), gomock.Any()).DoAndReturn(stubGenerate)
	h.reloader.EXPECT().Validate(gomock.Any(), gomock.Any()).Return(nil)
	h.reloader.EXPECT().Reload(gomock.Any(), gomock.Any()).Return(nil)

	require.NoError(t, h.lc.CaddyReload(context.Background()))
}

func TestLifecycle_CaddyReloadStoreListFailurePropagates(t *testing.T) {
	h := newLifecycleHarness(t)
	h.store.EXPECT().List(gomock.Any()).Return(nil, errors.New("permission denied"))

	err := h.lc.CaddyReload(context.Background())
	require.Error(t, err)
	assert.True(t,
		strings.Contains(err.Error(), "permission denied"),
		"expected wrapped Store.List error; got %v", err)
}
