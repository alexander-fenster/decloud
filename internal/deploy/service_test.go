package deploy_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

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

// stubGenerate is the standard happy-path mock action for Generator.Generate.
// It writes a non-empty placeholder so the orchestrator's tmp+rename step has
// a real file to operate on. Tests that need to assert the generator inputs
// can wrap this with DoAndReturn.
func stubGenerate(path string, _ []*registry.Service) error {
	return os.WriteFile(path, []byte("# generated for test\n"), 0o644)
}

// passThroughProbe is a Probe that calls Driver.ContainerIP once and returns
// success on a non-error result. The deploy_test fixtures mock ContainerIP
// directly so the orchestrator's probe step exercises the Driver-based seam
// without making a real HTTP request.
type passThroughProbe struct{ driver dockerdrv.Driver }

func (p *passThroughProbe) Wait(ctx context.Context, name string, _ registry.ReadinessSpec, _ int) error {
	deadline := time.Now().Add(2 * time.Second)
	for {
		ip, err := p.driver.ContainerIP(ctx, name)
		if err == nil && ip != "" {
			return nil
		}
		if time.Now().After(deadline) {
			if err != nil {
				return err
			}
			return errors.New("readiness: no IP")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

type deployerHarness struct {
	deployer  deploy.ServiceDeployer
	store     *registrymocks.MockStore
	capturer  *envcapmocks.MockCapturer
	driver    *dockermocks.MockDriver
	generator *caddymocks.MockGenerator
	reloader  *caddymocks.MockReloader
	paths     config.Paths
	stdout    *bytes.Buffer
	stderr    *bytes.Buffer
}

func newDeployerHarness(t *testing.T) *deployerHarness {
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

	d, err := deploy.NewServiceDeployer(deploy.Dependencies{
		Paths:     paths,
		Store:     store,
		Capturer:  capturer,
		Driver:    driver,
		Generator: gen,
		Reloader:  rel,
		Stdout:    stdout,
		Stderr:    stderr,
		Probe:     &passThroughProbe{driver: driver},
	})
	require.NoError(t, err)
	return &deployerHarness{
		deployer:  d,
		store:     store,
		capturer:  capturer,
		driver:    driver,
		generator: gen,
		reloader:  rel,
		paths:     paths,
		stdout:    stdout,
		stderr:    stderr,
	}
}

func newRequest() deploy.Request {
	return deploy.Request{
		Name:             "foo",
		SourceDir:        "/srv/foo",
		Dockerfile:       "Dockerfile",
		Hosts:            []string{"foo.example.com"},
		Port:             8080,
		EnvFile:          "/srv/foo/env.sh",
		ReadinessPath:    "/healthz",
		ReadinessTimeout: 60 * time.Second,
		Strategy:         "recreate",
	}
}

func newPrev() *registry.Service {
	return &registry.Service{
		Config: registry.ServiceConfig{
			SchemaVersion: 1,
			Name:          "foo",
			Build:         registry.BuildSpec{Dockerfile: "Dockerfile", ImageRef: "decloud-foo:prev123"},
			Run:           registry.RunSpec{Network: "decloud", Port: 8080, Restart: "unless-stopped"},
			Strategy:      "recreate",
		},
		Secrets: registry.ServiceSecrets{SchemaVersion: 1, Name: "foo", Env: map[string]string{"X": "1"}},
	}
}

func TestDeploy_HappyPathFirstDeploy(t *testing.T) {
	h := newDeployerHarness(t)
	ctx := context.Background()

	gomock.InOrder(
		h.driver.EXPECT().NetworkEnsure(gomock.Any(), "decloud").Return(nil),
		h.capturer.EXPECT().Capture(gomock.Any(), gomock.Any()).Return(map[string]string{"X": "1"}, nil),
		h.store.EXPECT().Load(gomock.Any(), "foo").Return(nil, registry.ErrNotFound),
		h.driver.EXPECT().Build(gomock.Any(), gomock.Any()).Return("img-id", nil),
		h.driver.EXPECT().Run(gomock.Any(), gomock.Any()).Return("cid", nil),
		h.driver.EXPECT().ContainerIP(gomock.Any(), gomock.Any()).Return("172.18.0.5", nil),
		h.store.EXPECT().Save(gomock.Any(), gomock.Any()).Return(nil),
		h.store.EXPECT().List(gomock.Any()).Return(nil, nil),
		h.generator.EXPECT().Generate(gomock.Any(), gomock.Any()).DoAndReturn(stubGenerate),
		h.reloader.EXPECT().Validate(gomock.Any(), gomock.Any()).Return(nil),
		h.reloader.EXPECT().Reload(gomock.Any(), gomock.Any()).Return(nil),
	)

	require.NoError(t, h.deployer.Deploy(ctx, newRequest()))
}

func TestDeploy_HappyPathRedeploy(t *testing.T) {
	h := newDeployerHarness(t)
	ctx := context.Background()
	prev := newPrev()

	gomock.InOrder(
		h.driver.EXPECT().NetworkEnsure(gomock.Any(), "decloud").Return(nil),
		h.capturer.EXPECT().Capture(gomock.Any(), gomock.Any()).Return(map[string]string{"X": "2"}, nil),
		h.store.EXPECT().Load(gomock.Any(), "foo").Return(prev, nil),
		h.driver.EXPECT().Build(gomock.Any(), gomock.Any()).Return("img-id", nil),
		h.driver.EXPECT().Stop(gomock.Any(), "decloud-foo", gomock.Any()).Return(nil),
		h.driver.EXPECT().Remove(gomock.Any(), "decloud-foo").Return(nil),
		h.driver.EXPECT().Run(gomock.Any(), gomock.Any()).Return("cid", nil),
		h.driver.EXPECT().ContainerIP(gomock.Any(), gomock.Any()).Return("172.18.0.5", nil),
		h.store.EXPECT().Save(gomock.Any(), gomock.Any()).Return(nil),
		h.store.EXPECT().List(gomock.Any()).Return(nil, nil),
		h.generator.EXPECT().Generate(gomock.Any(), gomock.Any()).DoAndReturn(stubGenerate),
		h.reloader.EXPECT().Validate(gomock.Any(), gomock.Any()).Return(nil),
		h.reloader.EXPECT().Reload(gomock.Any(), gomock.Any()).Return(nil),
	)

	require.NoError(t, h.deployer.Deploy(ctx, newRequest()))
}

func TestDeploy_LoadPreviousErrSecretsMissingTreatedAsFirstDeploy(t *testing.T) {
	h := newDeployerHarness(t)

	h.driver.EXPECT().NetworkEnsure(gomock.Any(), "decloud").Return(nil)
	h.capturer.EXPECT().Capture(gomock.Any(), gomock.Any()).Return(map[string]string{"X": "1"}, nil)
	h.store.EXPECT().Load(gomock.Any(), "foo").Return(nil, registry.ErrSecretsMissing)
	h.driver.EXPECT().Build(gomock.Any(), gomock.Any()).Return("img", nil)
	h.driver.EXPECT().Run(gomock.Any(), gomock.Any()).Return("cid", nil)
	h.driver.EXPECT().ContainerIP(gomock.Any(), gomock.Any()).Return("172.18.0.5", nil)
	h.store.EXPECT().Save(gomock.Any(), gomock.Any()).Return(nil)
	h.store.EXPECT().List(gomock.Any()).Return(nil, nil)
	h.generator.EXPECT().Generate(gomock.Any(), gomock.Any()).DoAndReturn(stubGenerate)
	h.reloader.EXPECT().Validate(gomock.Any(), gomock.Any()).Return(nil)
	h.reloader.EXPECT().Reload(gomock.Any(), gomock.Any()).Return(nil)

	require.NoError(t, h.deployer.Deploy(context.Background(), newRequest()))
}

func TestDeploy_EnvCaptureFailureAbortsBeforeAnythingChanges(t *testing.T) {
	h := newDeployerHarness(t)
	bashErr := errors.New("bash exited 1: stderr=\"set -e: false\"")

	h.driver.EXPECT().NetworkEnsure(gomock.Any(), "decloud").Return(nil)
	h.capturer.EXPECT().Capture(gomock.Any(), gomock.Any()).Return(nil, bashErr)

	err := h.deployer.Deploy(context.Background(), newRequest())
	require.Error(t, err)
	assert.True(t, errors.Is(err, deploy.ErrEnvCapture))
}

func TestDeploy_BuildFailureAbortsBeforeStoppingOld(t *testing.T) {
	h := newDeployerHarness(t)

	h.driver.EXPECT().NetworkEnsure(gomock.Any(), "decloud").Return(nil)
	h.capturer.EXPECT().Capture(gomock.Any(), gomock.Any()).Return(map[string]string{"X": "1"}, nil)
	h.store.EXPECT().Load(gomock.Any(), "foo").Return(newPrev(), nil)
	h.driver.EXPECT().Build(gomock.Any(), gomock.Any()).Return("", errors.New("docker build failed"))

	err := h.deployer.Deploy(context.Background(), newRequest())
	require.Error(t, err)
	assert.True(t, errors.Is(err, deploy.ErrBuild))
}

func TestDeploy_StopOldFailureAbortsAndDoesNotStartNew(t *testing.T) {
	h := newDeployerHarness(t)
	prev := newPrev()

	h.driver.EXPECT().NetworkEnsure(gomock.Any(), "decloud").Return(nil)
	h.capturer.EXPECT().Capture(gomock.Any(), gomock.Any()).Return(map[string]string{"X": "1"}, nil)
	h.store.EXPECT().Load(gomock.Any(), "foo").Return(prev, nil)
	h.driver.EXPECT().Build(gomock.Any(), gomock.Any()).Return("img", nil)
	h.driver.EXPECT().Stop(gomock.Any(), "decloud-foo", gomock.Any()).Return(errors.New("stop timed out"))
	h.driver.EXPECT().Inspect(gomock.Any(), "decloud-foo").Return(
		dockerdrv.InspectResult{State: "running"}, nil).AnyTimes()

	err := h.deployer.Deploy(context.Background(), newRequest())
	require.Error(t, err)
	assert.True(t, errors.Is(err, deploy.ErrRun))
}

func TestDeploy_RunNewFailureRollsBackToOld(t *testing.T) {
	h := newDeployerHarness(t)
	prev := newPrev()
	var rolledBackImage string

	h.driver.EXPECT().NetworkEnsure(gomock.Any(), "decloud").Return(nil)
	h.capturer.EXPECT().Capture(gomock.Any(), gomock.Any()).Return(map[string]string{"X": "1"}, nil)
	h.store.EXPECT().Load(gomock.Any(), "foo").Return(prev, nil)
	h.driver.EXPECT().Build(gomock.Any(), gomock.Any()).Return("img", nil)
	h.driver.EXPECT().Stop(gomock.Any(), "decloud-foo", gomock.Any()).Return(nil)
	h.driver.EXPECT().Remove(gomock.Any(), "decloud-foo").Return(nil)
	gomock.InOrder(
		h.driver.EXPECT().Run(gomock.Any(), gomock.Any()).Return("", errors.New("docker run failed")),
		h.driver.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, req dockerdrv.RunRequest) (string, error) {
				rolledBackImage = req.Image
				return "rb-cid", nil
			}),
	)

	err := h.deployer.Deploy(context.Background(), newRequest())
	require.Error(t, err)
	assert.True(t, errors.Is(err, deploy.ErrRun))
	assert.Equal(t, prev.Config.Build.ImageRef, rolledBackImage,
		"rollback must re-run with the previous image")
}

func TestDeploy_ReadinessFailureRollsBackToOld(t *testing.T) {
	h := newDeployerHarness(t)
	prev := newPrev()

	h.driver.EXPECT().NetworkEnsure(gomock.Any(), "decloud").Return(nil)
	h.capturer.EXPECT().Capture(gomock.Any(), gomock.Any()).Return(map[string]string{"X": "1"}, nil)
	h.store.EXPECT().Load(gomock.Any(), "foo").Return(prev, nil)
	h.driver.EXPECT().Build(gomock.Any(), gomock.Any()).Return("img", nil)
	h.driver.EXPECT().Stop(gomock.Any(), "decloud-foo", gomock.Any()).Return(nil)
	h.driver.EXPECT().Remove(gomock.Any(), "decloud-foo").Return(nil)
	h.driver.EXPECT().Run(gomock.Any(), gomock.Any()).Return("cid", nil)
	h.driver.EXPECT().ContainerIP(gomock.Any(), gomock.Any()).Return("", errors.New("no IP")).AnyTimes()
	h.driver.EXPECT().Stop(gomock.Any(), "decloud-foo", gomock.Any()).Return(nil)
	h.driver.EXPECT().Remove(gomock.Any(), "decloud-foo").Return(nil)
	h.driver.EXPECT().Run(gomock.Any(), gomock.Any()).Return("rb-cid", nil)

	req := newRequest()
	req.ReadinessTimeout = 200 * time.Millisecond
	err := h.deployer.Deploy(context.Background(), req)
	require.Error(t, err)
	assert.True(t, errors.Is(err, deploy.ErrReadiness))
}

func TestDeploy_SaveErrPartialWriteRollsBackAndDeletesOrphanConfig(t *testing.T) {
	h := newDeployerHarness(t)

	h.driver.EXPECT().NetworkEnsure(gomock.Any(), "decloud").Return(nil)
	h.capturer.EXPECT().Capture(gomock.Any(), gomock.Any()).Return(map[string]string{"X": "1"}, nil)
	h.store.EXPECT().Load(gomock.Any(), "foo").Return(nil, registry.ErrNotFound)
	h.driver.EXPECT().Build(gomock.Any(), gomock.Any()).Return("img", nil)
	h.driver.EXPECT().Run(gomock.Any(), gomock.Any()).Return("cid", nil)
	h.driver.EXPECT().ContainerIP(gomock.Any(), gomock.Any()).Return("172.18.0.5", nil)
	gomock.InOrder(
		h.store.EXPECT().Save(gomock.Any(), gomock.Any()).Return(registry.ErrPartialWrite),
		h.store.EXPECT().DeleteOrphanConfig(gomock.Any(), "foo").Return(nil),
		h.driver.EXPECT().Stop(gomock.Any(), "decloud-foo", gomock.Any()).Return(nil),
		h.driver.EXPECT().Remove(gomock.Any(), "decloud-foo").Return(nil),
	)

	err := h.deployer.Deploy(context.Background(), newRequest())
	require.Error(t, err)
}

func TestDeploy_SaveFailsBeforePartialWriteSkipsDeleteOrphanConfig(t *testing.T) {
	h := newDeployerHarness(t)

	h.driver.EXPECT().NetworkEnsure(gomock.Any(), "decloud").Return(nil)
	h.capturer.EXPECT().Capture(gomock.Any(), gomock.Any()).Return(map[string]string{"X": "1"}, nil)
	h.store.EXPECT().Load(gomock.Any(), "foo").Return(nil, registry.ErrNotFound)
	h.driver.EXPECT().Build(gomock.Any(), gomock.Any()).Return("img", nil)
	h.driver.EXPECT().Run(gomock.Any(), gomock.Any()).Return("cid", nil)
	h.driver.EXPECT().ContainerIP(gomock.Any(), gomock.Any()).Return("172.18.0.5", nil)
	h.store.EXPECT().Save(gomock.Any(), gomock.Any()).Return(errors.New("validation failed"))
	h.driver.EXPECT().Stop(gomock.Any(), "decloud-foo", gomock.Any()).Return(nil)
	h.driver.EXPECT().Remove(gomock.Any(), "decloud-foo").Return(nil)

	err := h.deployer.Deploy(context.Background(), newRequest())
	require.Error(t, err)
}

func TestDeploy_CaddyValidateFailureLeavesOldFileAndKeepsNewContainer(t *testing.T) {
	h := newDeployerHarness(t)

	h.driver.EXPECT().NetworkEnsure(gomock.Any(), "decloud").Return(nil)
	h.capturer.EXPECT().Capture(gomock.Any(), gomock.Any()).Return(map[string]string{"X": "1"}, nil)
	h.store.EXPECT().Load(gomock.Any(), "foo").Return(nil, registry.ErrNotFound)
	h.driver.EXPECT().Build(gomock.Any(), gomock.Any()).Return("img", nil)
	h.driver.EXPECT().Run(gomock.Any(), gomock.Any()).Return("cid", nil)
	h.driver.EXPECT().ContainerIP(gomock.Any(), gomock.Any()).Return("172.18.0.5", nil)
	h.store.EXPECT().Save(gomock.Any(), gomock.Any()).Return(nil)
	h.store.EXPECT().List(gomock.Any()).Return(nil, nil)
	h.generator.EXPECT().Generate(gomock.Any(), gomock.Any()).DoAndReturn(stubGenerate)
	h.reloader.EXPECT().Validate(gomock.Any(), gomock.Any()).Return(errors.New("syntax error at line 5"))

	err := h.deployer.Deploy(context.Background(), newRequest())
	require.Error(t, err)
	assert.True(t, errors.Is(err, deploy.ErrCaddyReload))
}

func TestDeploy_CaddyReloadFailureDoesNotRollBackContainer(t *testing.T) {
	h := newDeployerHarness(t)

	h.driver.EXPECT().NetworkEnsure(gomock.Any(), "decloud").Return(nil)
	h.capturer.EXPECT().Capture(gomock.Any(), gomock.Any()).Return(map[string]string{"X": "1"}, nil)
	h.store.EXPECT().Load(gomock.Any(), "foo").Return(nil, registry.ErrNotFound)
	h.driver.EXPECT().Build(gomock.Any(), gomock.Any()).Return("img", nil)
	h.driver.EXPECT().Run(gomock.Any(), gomock.Any()).Return("cid", nil)
	h.driver.EXPECT().ContainerIP(gomock.Any(), gomock.Any()).Return("172.18.0.5", nil)
	h.store.EXPECT().Save(gomock.Any(), gomock.Any()).Return(nil)
	h.store.EXPECT().List(gomock.Any()).Return(nil, nil)
	h.generator.EXPECT().Generate(gomock.Any(), gomock.Any()).DoAndReturn(stubGenerate)
	h.reloader.EXPECT().Validate(gomock.Any(), gomock.Any()).Return(nil)
	h.reloader.EXPECT().Reload(gomock.Any(), gomock.Any()).Return(errors.New("port already bound"))

	err := h.deployer.Deploy(context.Background(), newRequest())
	require.Error(t, err)
	assert.True(t, errors.Is(err, deploy.ErrCaddyReload))
}

func TestDeploy_CaddyValidateFailureMentionsCaddyUpRecovery(t *testing.T) {
	h := newDeployerHarness(t)

	h.driver.EXPECT().NetworkEnsure(gomock.Any(), "decloud").Return(nil)
	h.capturer.EXPECT().Capture(gomock.Any(), gomock.Any()).Return(map[string]string{"X": "1"}, nil)
	h.store.EXPECT().Load(gomock.Any(), "foo").Return(nil, registry.ErrNotFound)
	h.driver.EXPECT().Build(gomock.Any(), gomock.Any()).Return("img", nil)
	h.driver.EXPECT().Run(gomock.Any(), gomock.Any()).Return("cid", nil)
	h.driver.EXPECT().ContainerIP(gomock.Any(), gomock.Any()).Return("172.18.0.5", nil)
	h.store.EXPECT().Save(gomock.Any(), gomock.Any()).Return(nil)
	h.store.EXPECT().List(gomock.Any()).Return(nil, nil)
	h.generator.EXPECT().Generate(gomock.Any(), gomock.Any()).DoAndReturn(stubGenerate)
	h.reloader.EXPECT().Validate(gomock.Any(), gomock.Any()).
		Return(errors.New(`container "decloud-caddy" is not running; run 'decloud caddy up' first`))

	err := h.deployer.Deploy(context.Background(), newRequest())
	require.Error(t, err)
	assert.True(t, errors.Is(err, deploy.ErrCaddyReload))
	assert.Contains(t, err.Error(), "decloud caddy up")
	assert.Contains(t, err.Error(), "registered")
	assert.Contains(t, err.Error(), "Caddy is not routing")
}

func TestDeploy_CaddyReloadFailureMentionsCaddyUpRecovery(t *testing.T) {
	h := newDeployerHarness(t)

	h.driver.EXPECT().NetworkEnsure(gomock.Any(), "decloud").Return(nil)
	h.capturer.EXPECT().Capture(gomock.Any(), gomock.Any()).Return(map[string]string{"X": "1"}, nil)
	h.store.EXPECT().Load(gomock.Any(), "foo").Return(nil, registry.ErrNotFound)
	h.driver.EXPECT().Build(gomock.Any(), gomock.Any()).Return("img", nil)
	h.driver.EXPECT().Run(gomock.Any(), gomock.Any()).Return("cid", nil)
	h.driver.EXPECT().ContainerIP(gomock.Any(), gomock.Any()).Return("172.18.0.5", nil)
	h.store.EXPECT().Save(gomock.Any(), gomock.Any()).Return(nil)
	h.store.EXPECT().List(gomock.Any()).Return(nil, nil)
	h.generator.EXPECT().Generate(gomock.Any(), gomock.Any()).DoAndReturn(stubGenerate)
	h.reloader.EXPECT().Validate(gomock.Any(), gomock.Any()).Return(nil)
	innerErr := errors.New(`container "decloud-caddy" is not running; run 'decloud caddy up' first`)
	h.reloader.EXPECT().Reload(gomock.Any(), gomock.Any()).Return(innerErr)

	err := h.deployer.Deploy(context.Background(), newRequest())
	require.Error(t, err)
	assert.True(t, errors.Is(err, deploy.ErrCaddyReload))
	assert.True(t, errors.Is(err, innerErr))
	assert.Contains(t, err.Error(), "decloud caddy up")
	assert.Contains(t, err.Error(), "registered")
	assert.Contains(t, err.Error(), "Caddy is not routing")
}

func TestDeploy_DeployIDIsStableThroughoutOneDeploy(t *testing.T) {
	h := newDeployerHarness(t)
	var buildImage, runImage string

	h.driver.EXPECT().NetworkEnsure(gomock.Any(), "decloud").Return(nil)
	h.capturer.EXPECT().Capture(gomock.Any(), gomock.Any()).Return(map[string]string{"X": "1"}, nil)
	h.store.EXPECT().Load(gomock.Any(), "foo").Return(nil, registry.ErrNotFound)
	h.driver.EXPECT().Build(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, req dockerdrv.BuildRequest) (string, error) {
			buildImage = req.ImageRef
			return "img", nil
		})
	h.driver.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, req dockerdrv.RunRequest) (string, error) {
			runImage = req.Image
			return "cid", nil
		})
	h.driver.EXPECT().ContainerIP(gomock.Any(), gomock.Any()).Return("172.18.0.5", nil)
	h.store.EXPECT().Save(gomock.Any(), gomock.Any()).Return(nil)
	h.store.EXPECT().List(gomock.Any()).Return(nil, nil)
	h.generator.EXPECT().Generate(gomock.Any(), gomock.Any()).DoAndReturn(stubGenerate)
	h.reloader.EXPECT().Validate(gomock.Any(), gomock.Any()).Return(nil)
	h.reloader.EXPECT().Reload(gomock.Any(), gomock.Any()).Return(nil)

	require.NoError(t, h.deployer.Deploy(context.Background(), newRequest()))
	require.NotEmpty(t, buildImage)
	require.NotEmpty(t, runImage)
	assert.Equal(t, buildImage, runImage,
		"the same deploy must use the same image:tag for build and run")
}

func TestDeploy_CapturedEnvNotLoggedAsKeysOrValues(t *testing.T) {
	h := newDeployerHarness(t)
	const secret = "TOPSECRET_VALUE_DO_NOT_LOG"

	h.driver.EXPECT().NetworkEnsure(gomock.Any(), "decloud").Return(nil)
	h.capturer.EXPECT().Capture(gomock.Any(), gomock.Any()).Return(
		map[string]string{"DATABASE_URL": secret}, nil)
	h.store.EXPECT().Load(gomock.Any(), "foo").Return(nil, registry.ErrNotFound)
	h.driver.EXPECT().Build(gomock.Any(), gomock.Any()).Return("img", nil)
	h.driver.EXPECT().Run(gomock.Any(), gomock.Any()).Return("cid", nil)
	h.driver.EXPECT().ContainerIP(gomock.Any(), gomock.Any()).Return("172.18.0.5", nil)
	h.store.EXPECT().Save(gomock.Any(), gomock.Any()).Return(nil)
	h.store.EXPECT().List(gomock.Any()).Return(nil, nil)
	h.generator.EXPECT().Generate(gomock.Any(), gomock.Any()).DoAndReturn(stubGenerate)
	h.reloader.EXPECT().Validate(gomock.Any(), gomock.Any()).Return(nil)
	h.reloader.EXPECT().Reload(gomock.Any(), gomock.Any()).Return(nil)

	require.NoError(t, h.deployer.Deploy(context.Background(), newRequest()))
	assert.NotContains(t, h.stdout.String(), secret, "captured env value must not be logged")
	assert.NotContains(t, h.stderr.String(), secret, "captured env value must not be logged")
	assert.NotContains(t, h.stdout.String(), "DATABASE_URL")
	assert.NotContains(t, h.stderr.String(), "DATABASE_URL")
}

func TestDeploy_RunRequestUsesCapturedEnvAndDecloudNetwork(t *testing.T) {
	h := newDeployerHarness(t)
	captured := map[string]string{"DATABASE_URL": "postgres://x"}
	var seen dockerdrv.RunRequest

	h.driver.EXPECT().NetworkEnsure(gomock.Any(), "decloud").Return(nil)
	h.capturer.EXPECT().Capture(gomock.Any(), gomock.Any()).Return(captured, nil)
	h.store.EXPECT().Load(gomock.Any(), "foo").Return(nil, registry.ErrNotFound)
	h.driver.EXPECT().Build(gomock.Any(), gomock.Any()).Return("img", nil)
	h.driver.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, req dockerdrv.RunRequest) (string, error) {
			seen = req
			return "cid", nil
		})
	h.driver.EXPECT().ContainerIP(gomock.Any(), gomock.Any()).Return("172.18.0.5", nil)
	h.store.EXPECT().Save(gomock.Any(), gomock.Any()).Return(nil)
	h.store.EXPECT().List(gomock.Any()).Return(nil, nil)
	h.generator.EXPECT().Generate(gomock.Any(), gomock.Any()).DoAndReturn(stubGenerate)
	h.reloader.EXPECT().Validate(gomock.Any(), gomock.Any()).Return(nil)
	h.reloader.EXPECT().Reload(gomock.Any(), gomock.Any()).Return(nil)

	require.NoError(t, h.deployer.Deploy(context.Background(), newRequest()))
	assert.Equal(t, "decloud", seen.Network)
	assert.Equal(t, "decloud-foo", seen.Name)
	assert.True(t, reflect.DeepEqual(captured, seen.Env), "RunRequest.Env must equal captured env")
}

func TestDeploy_NoEnvScript_SkipsCapturerEntirely(t *testing.T) {
	h := newDeployerHarness(t)

	gomock.InOrder(
		h.driver.EXPECT().NetworkEnsure(gomock.Any(), "decloud").Return(nil),
		h.store.EXPECT().Load(gomock.Any(), "foo").Return(nil, registry.ErrNotFound),
		h.driver.EXPECT().Build(gomock.Any(), gomock.Any()).Return("img-id", nil),
		h.driver.EXPECT().Run(gomock.Any(), gomock.Any()).Return("cid", nil),
		h.driver.EXPECT().ContainerIP(gomock.Any(), gomock.Any()).Return("172.18.0.5", nil),
		h.store.EXPECT().Save(gomock.Any(), gomock.Any()).Return(nil),
		h.store.EXPECT().List(gomock.Any()).Return(nil, nil),
		h.generator.EXPECT().Generate(gomock.Any(), gomock.Any()).DoAndReturn(stubGenerate),
		h.reloader.EXPECT().Validate(gomock.Any(), gomock.Any()).Return(nil),
		h.reloader.EXPECT().Reload(gomock.Any(), gomock.Any()).Return(nil),
	)
	h.capturer.EXPECT().Capture(gomock.Any(), gomock.Any()).Times(0)

	req := newRequest()
	req.EnvFile = ""
	require.NoError(t, h.deployer.Deploy(context.Background(), req))
}

func TestDeploy_NetworkEnsureCalledFirst(t *testing.T) {
	h := newDeployerHarness(t)

	gomock.InOrder(
		h.driver.EXPECT().NetworkEnsure(gomock.Any(), "decloud").Return(nil),
		h.capturer.EXPECT().Capture(gomock.Any(), gomock.Any()).Return(map[string]string{"X": "1"}, nil),
		h.store.EXPECT().Load(gomock.Any(), "foo").Return(nil, registry.ErrNotFound),
		h.driver.EXPECT().Build(gomock.Any(), gomock.Any()).Return("img-id", nil),
		h.driver.EXPECT().Run(gomock.Any(), gomock.Any()).Return("cid", nil),
		h.driver.EXPECT().ContainerIP(gomock.Any(), gomock.Any()).Return("172.18.0.5", nil),
		h.store.EXPECT().Save(gomock.Any(), gomock.Any()).Return(nil),
		h.store.EXPECT().List(gomock.Any()).Return(nil, nil),
		h.generator.EXPECT().Generate(gomock.Any(), gomock.Any()).DoAndReturn(stubGenerate),
		h.reloader.EXPECT().Validate(gomock.Any(), gomock.Any()).Return(nil),
		h.reloader.EXPECT().Reload(gomock.Any(), gomock.Any()).Return(nil),
	)

	require.NoError(t, h.deployer.Deploy(context.Background(), newRequest()))
}

func TestDeploy_NetworkEnsureFailureReturnsErrRun(t *testing.T) {
	h := newDeployerHarness(t)

	h.driver.EXPECT().NetworkEnsure(gomock.Any(), "decloud").
		Return(errors.New("docker network create failed"))
	h.capturer.EXPECT().Capture(gomock.Any(), gomock.Any()).Times(0)
	h.driver.EXPECT().Build(gomock.Any(), gomock.Any()).Times(0)
	h.driver.EXPECT().Run(gomock.Any(), gomock.Any()).Times(0)

	err := h.deployer.Deploy(context.Background(), newRequest())
	require.Error(t, err)
	assert.True(t, errors.Is(err, deploy.ErrRun))
}

func TestDeploy_BuildErrorPreservesInnerSentinel(t *testing.T) {
	h := newDeployerHarness(t)
	sentinel := errors.New("synthetic build err")

	h.driver.EXPECT().NetworkEnsure(gomock.Any(), "decloud").Return(nil)
	h.capturer.EXPECT().Capture(gomock.Any(), gomock.Any()).Return(map[string]string{}, nil)
	h.store.EXPECT().Load(gomock.Any(), "foo").Return(nil, registry.ErrNotFound)
	h.driver.EXPECT().Build(gomock.Any(), gomock.Any()).
		Return("", fmt.Errorf("docker build: %w", sentinel))

	err := h.deployer.Deploy(context.Background(), newRequest())
	require.Error(t, err)
	assert.True(t, errors.Is(err, deploy.ErrBuild))
	assert.True(t, errors.Is(err, sentinel),
		"inner sentinel must traverse the chain after %%w:%%w fix")
}
