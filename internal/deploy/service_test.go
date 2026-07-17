package deploy_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
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

// cancellingProbe is a Probe that blocks until ctx is cancelled and then
// returns the raw context error — exactly the contract httpProbe.Wait now
// follows after the v2 readiness change. Used by the §5.1 / §5.6 / §5.10.1
// cancellation tests; injected via newDeployerHarnessWithProbe.
type cancellingProbe struct{}

func (cancellingProbe) Wait(ctx context.Context, _ string, _ registry.ReadinessSpec, _ int) error {
	<-ctx.Done()
	return ctx.Err()
}

// notCancelledCtxMatcher asserts that the captured argument is a context
// whose Err() is nil at the moment the mock was invoked. Used to verify
// cleanup paths receive a fresh, non-cancelled context.
type notCancelledCtxMatcher struct{}

func (notCancelledCtxMatcher) Matches(x any) bool {
	ctx, ok := x.(context.Context)
	if !ok {
		return false
	}
	return ctx.Err() == nil
}

func (notCancelledCtxMatcher) String() string {
	return "is a context with Err() == nil at call time"
}

func notCancelledCtx() gomock.Matcher { return notCancelledCtxMatcher{} }

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

// harnessOption configures newDeployerHarnessWithProbe.
type harnessOption func(*harnessConfig)

type harnessConfig struct {
	skipInspectAbsentDefault bool
}

// withoutInspectAbsentDefault opts the harness out of installing the default
// Inspect → absent AnyTimes expectation. Required by tests that need a
// non-absent Inspect response on the request path (gomock matches
// expectations in FIFO insertion order, so the harness default would
// otherwise win against a test-supplied expectation).
func withoutInspectAbsentDefault() harnessOption {
	return func(c *harnessConfig) { c.skipInspectAbsentDefault = true }
}

func newDeployerHarness(t *testing.T, opts ...harnessOption) *deployerHarness {
	t.Helper()
	return newDeployerHarnessWithProbe(t, nil, opts...)
}

// newDeployerHarnessWithProbe constructs the harness with a caller-supplied
// readiness probe. Passing nil falls back to passThroughProbe (production-
// shape happy-path probe). Tests that need cancellation-driven probe
// behaviour pass cancellingProbe{}.
func newDeployerHarnessWithProbe(t *testing.T, probe deploy.ReadinessProbe, opts ...harnessOption) *deployerHarness {
	t.Helper()
	cfg := harnessConfig{}
	for _, opt := range opts {
		opt(&cfg)
	}
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

	if probe == nil {
		probe = &passThroughProbe{driver: driver}
	}

	// Default expectation: any first-deploy test that exercises the §3.5
	// defensive-orphan branch sees an "absent" container. Tests that care
	// about a non-absent inspect override this entirely with the
	// withoutInspectAbsentDefault() option (gomock matches FIFO; see
	// 06-linus-review-v2.md and 08-kent-tests.md for the empirical note).
	if !cfg.skipInspectAbsentDefault {
		driver.EXPECT().
			Inspect(gomock.Any(), gomock.Any()).
			Return(dockerdrv.InspectResult{State: "absent"}, nil).
			AnyTimes()
	}

	d, err := deploy.NewServiceDeployer(deploy.Dependencies{
		Paths:     paths,
		Store:     store,
		Capturer:  capturer,
		Driver:    driver,
		Generator: gen,
		Reloader:  rel,
		Stdout:    stdout,
		Stderr:    stderr,
		Probe:     probe,
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
	h := newDeployerHarness(t, withoutInspectAbsentDefault())
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

func TestDeploy_ProbeCancellationCleansUpWithFreshContext(t *testing.T) {
	h := newDeployerHarnessWithProbe(t, cancellingProbe{})
	ctx, cancel := context.WithCancel(context.Background())

	h.driver.EXPECT().NetworkEnsure(gomock.Any(), "decloud").Return(nil)
	h.capturer.EXPECT().Capture(gomock.Any(), gomock.Any()).Return(map[string]string{"X": "1"}, nil)
	h.store.EXPECT().Load(gomock.Any(), "foo").Return(nil, registry.ErrNotFound)
	h.driver.EXPECT().Build(gomock.Any(), gomock.Any()).Return("img", nil)
	h.driver.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ dockerdrv.RunRequest) (string, error) {
			cancel()
			return "cid", nil
		})
	h.driver.EXPECT().Stop(notCancelledCtx(), "decloud-foo", gomock.Any()).Return(nil)
	h.driver.EXPECT().Remove(notCancelledCtx(), "decloud-foo").Return(nil)

	err := h.deployer.Deploy(ctx, newRequest())
	require.Error(t, err)
	assert.True(t, errors.Is(err, deploy.ErrInterrupted),
		"probe cancellation must surface as ErrInterrupted; got %v", err)
	assert.True(t, errors.Is(err, context.Canceled),
		"context.Canceled must traverse the error chain; got %v", err)
	assert.False(t, errors.Is(err, deploy.ErrReadiness),
		"cancellation must NOT be wrapped as ErrReadiness; got %v", err)
}

func TestDeploy_DefensiveOrphanCleanupOnFreshDeployWhenContainerExists(t *testing.T) {
	h := newDeployerHarness(t, withoutInspectAbsentDefault())

	gomock.InOrder(
		h.driver.EXPECT().NetworkEnsure(gomock.Any(), "decloud").Return(nil),
		h.capturer.EXPECT().Capture(gomock.Any(), gomock.Any()).Return(map[string]string{"X": "1"}, nil),
		h.store.EXPECT().Load(gomock.Any(), "foo").Return(nil, registry.ErrNotFound),
		h.driver.EXPECT().Build(gomock.Any(), gomock.Any()).Return("img", nil),
		h.driver.EXPECT().Inspect(gomock.Any(), "decloud-foo").
			Return(dockerdrv.InspectResult{
				ContainerID: "orphan-id",
				State:       "running",
				Labels:      map[string]string{"decloud.service": "foo"},
			}, nil),
		h.driver.EXPECT().Stop(gomock.Any(), "decloud-foo", gomock.Any()).Return(nil),
		h.driver.EXPECT().Remove(gomock.Any(), "decloud-foo").Return(nil),
		h.driver.EXPECT().Run(gomock.Any(), gomock.Any()).Return("new-cid", nil),
		h.driver.EXPECT().ContainerIP(gomock.Any(), gomock.Any()).Return("172.18.0.5", nil),
		h.store.EXPECT().Save(gomock.Any(), gomock.Any()).Return(nil),
		h.store.EXPECT().List(gomock.Any()).Return(nil, nil),
		h.generator.EXPECT().Generate(gomock.Any(), gomock.Any()).DoAndReturn(stubGenerate),
		h.reloader.EXPECT().Validate(gomock.Any(), gomock.Any()).Return(nil),
		h.reloader.EXPECT().Reload(gomock.Any(), gomock.Any()).Return(nil),
	)

	require.NoError(t, h.deployer.Deploy(context.Background(), newRequest()))
}

func TestDeploy_DefensiveOrphanCleanupSkippedWhenContainerAbsent(t *testing.T) {
	h := newDeployerHarness(t, withoutInspectAbsentDefault())

	gomock.InOrder(
		h.driver.EXPECT().NetworkEnsure(gomock.Any(), "decloud").Return(nil),
		h.capturer.EXPECT().Capture(gomock.Any(), gomock.Any()).Return(map[string]string{"X": "1"}, nil),
		h.store.EXPECT().Load(gomock.Any(), "foo").Return(nil, registry.ErrNotFound),
		h.driver.EXPECT().Build(gomock.Any(), gomock.Any()).Return("img", nil),
		h.driver.EXPECT().Inspect(gomock.Any(), "decloud-foo").
			Return(dockerdrv.InspectResult{State: "absent"}, nil),
		h.driver.EXPECT().Run(gomock.Any(), gomock.Any()).Return("cid", nil),
		h.driver.EXPECT().ContainerIP(gomock.Any(), gomock.Any()).Return("172.18.0.5", nil),
		h.store.EXPECT().Save(gomock.Any(), gomock.Any()).Return(nil),
		h.store.EXPECT().List(gomock.Any()).Return(nil, nil),
		h.generator.EXPECT().Generate(gomock.Any(), gomock.Any()).DoAndReturn(stubGenerate),
		h.reloader.EXPECT().Validate(gomock.Any(), gomock.Any()).Return(nil),
		h.reloader.EXPECT().Reload(gomock.Any(), gomock.Any()).Return(nil),
	)
	h.driver.EXPECT().Stop(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
	h.driver.EXPECT().Remove(gomock.Any(), gomock.Any()).Times(0)

	require.NoError(t, h.deployer.Deploy(context.Background(), newRequest()))
}

func TestDeploy_DefensiveOrphanCleanupFailureWrapsErrRun(t *testing.T) {
	h := newDeployerHarness(t, withoutInspectAbsentDefault())

	h.driver.EXPECT().NetworkEnsure(gomock.Any(), "decloud").Return(nil)
	h.capturer.EXPECT().Capture(gomock.Any(), gomock.Any()).Return(map[string]string{"X": "1"}, nil)
	h.store.EXPECT().Load(gomock.Any(), "foo").Return(nil, registry.ErrNotFound)
	h.driver.EXPECT().Build(gomock.Any(), gomock.Any()).Return("img", nil)
	h.driver.EXPECT().Inspect(gomock.Any(), "decloud-foo").
		Return(dockerdrv.InspectResult{
			State:  "running",
			Labels: map[string]string{"decloud.service": "foo"},
		}, nil)
	stopErr := errors.New("daemon hung")
	h.driver.EXPECT().Stop(gomock.Any(), "decloud-foo", gomock.Any()).Return(stopErr)

	err := h.deployer.Deploy(context.Background(), newRequest())
	require.Error(t, err)
	assert.True(t, errors.Is(err, deploy.ErrRun),
		"orphan-cleanup failure must surface as ErrRun; got %v", err)
	assert.True(t, errors.Is(err, stopErr),
		"inner stop error must traverse the chain; got %v", err)
	assert.Contains(t, err.Error(), "docker rm -f decloud-foo",
		"recovery hint must mention 'docker rm -f decloud-foo'; got %q", err.Error())
}

func TestDeploy_RestoreOldContainerUsesFreshContextOnRedeployCancellation(t *testing.T) {
	h := newDeployerHarnessWithProbe(t, cancellingProbe{})
	prev := newPrev()
	ctx, cancel := context.WithCancel(context.Background())

	h.driver.EXPECT().NetworkEnsure(gomock.Any(), "decloud").Return(nil)
	h.capturer.EXPECT().Capture(gomock.Any(), gomock.Any()).Return(map[string]string{"X": "1"}, nil)
	h.store.EXPECT().Load(gomock.Any(), "foo").Return(prev, nil)
	h.driver.EXPECT().Build(gomock.Any(), gomock.Any()).Return("img", nil)
	h.driver.EXPECT().Stop(gomock.Any(), "decloud-foo", gomock.Any()).Return(nil)
	h.driver.EXPECT().Remove(gomock.Any(), "decloud-foo").Return(nil)
	h.driver.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ dockerdrv.RunRequest) (string, error) {
			cancel()
			return "new-cid", nil
		})
	h.driver.EXPECT().Stop(notCancelledCtx(), "decloud-foo", gomock.Any()).Return(nil)
	h.driver.EXPECT().Remove(notCancelledCtx(), "decloud-foo").Return(nil)
	h.driver.EXPECT().Run(notCancelledCtx(), gomock.Any()).DoAndReturn(
		func(_ context.Context, req dockerdrv.RunRequest) (string, error) {
			assert.Equal(t, prev.Config.Build.ImageRef, req.Image,
				"rollback restores the previous image")
			return "rb-cid", nil
		})

	err := h.deployer.Deploy(ctx, newRequest())
	require.Error(t, err)
	assert.True(t, errors.Is(err, deploy.ErrInterrupted),
		"redeploy cancellation must surface as ErrInterrupted; got %v", err)
}

func TestDeploy_DefensiveOrphanRefusesContainerWithoutDecloudLabel(t *testing.T) {
	h := newDeployerHarness(t, withoutInspectAbsentDefault())

	h.driver.EXPECT().NetworkEnsure(gomock.Any(), "decloud").Return(nil)
	h.capturer.EXPECT().Capture(gomock.Any(), gomock.Any()).Return(map[string]string{"X": "1"}, nil)
	h.store.EXPECT().Load(gomock.Any(), "foo").Return(nil, registry.ErrNotFound)
	h.driver.EXPECT().Build(gomock.Any(), gomock.Any()).Return("img", nil)
	h.driver.EXPECT().Inspect(gomock.Any(), "decloud-foo").
		Return(dockerdrv.InspectResult{
			State:  "running",
			Labels: map[string]string{"some.other.label": "value"},
		}, nil)
	h.driver.EXPECT().Stop(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
	h.driver.EXPECT().Remove(gomock.Any(), gomock.Any()).Times(0)
	h.driver.EXPECT().Run(gomock.Any(), gomock.Any()).Times(0)

	err := h.deployer.Deploy(context.Background(), newRequest())
	require.Error(t, err)
	assert.True(t, errors.Is(err, deploy.ErrRun),
		"missing decloud label must surface as ErrRun; got %v", err)
	assert.Contains(t, err.Error(), "was not created by decloud",
		"refusal message must mention provenance; got %q", err.Error())
	assert.Contains(t, err.Error(), "docker rm -f decloud-foo",
		"refusal message must include manual recovery hint; got %q", err.Error())
}

func TestDeploy_DefensiveOrphanRefusesContainerWithMismatchedLabel(t *testing.T) {
	h := newDeployerHarness(t, withoutInspectAbsentDefault())

	h.driver.EXPECT().NetworkEnsure(gomock.Any(), "decloud").Return(nil)
	h.capturer.EXPECT().Capture(gomock.Any(), gomock.Any()).Return(map[string]string{"X": "1"}, nil)
	h.store.EXPECT().Load(gomock.Any(), "foo").Return(nil, registry.ErrNotFound)
	h.driver.EXPECT().Build(gomock.Any(), gomock.Any()).Return("img", nil)
	h.driver.EXPECT().Inspect(gomock.Any(), "decloud-foo").
		Return(dockerdrv.InspectResult{
			State:  "running",
			Labels: map[string]string{"decloud.service": "bar"},
		}, nil)
	h.driver.EXPECT().Stop(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
	h.driver.EXPECT().Remove(gomock.Any(), gomock.Any()).Times(0)
	h.driver.EXPECT().Run(gomock.Any(), gomock.Any()).Times(0)

	err := h.deployer.Deploy(context.Background(), newRequest())
	require.Error(t, err)
	assert.True(t, errors.Is(err, deploy.ErrRun),
		"mismatched decloud.service label must surface as ErrRun; got %v", err)
	assert.Contains(t, err.Error(), `decloud.service="bar"`,
		"refusal message must surface the offending label value; got %q", err.Error())
	assert.Contains(t, err.Error(), "does not match",
		"refusal message must mention the mismatch; got %q", err.Error())
}

func TestDeploy_DefensiveOrphanInspectCancelledReturnsErrInterrupted(t *testing.T) {
	cases := []struct {
		name     string
		register func(h *deployerHarness)
	}{
		{
			name: "inspect-cancelled",
			register: func(h *deployerHarness) {
				h.driver.EXPECT().Inspect(gomock.Any(), "decloud-foo").
					Return(dockerdrv.InspectResult{}, context.Canceled)
			},
		},
		{
			name: "stop-cancelled",
			register: func(h *deployerHarness) {
				h.driver.EXPECT().Inspect(gomock.Any(), "decloud-foo").
					Return(dockerdrv.InspectResult{
						State:  "running",
						Labels: map[string]string{"decloud.service": "foo"},
					}, nil)
				h.driver.EXPECT().Stop(gomock.Any(), "decloud-foo", gomock.Any()).
					Return(context.Canceled)
			},
		},
		{
			name: "remove-cancelled",
			register: func(h *deployerHarness) {
				h.driver.EXPECT().Inspect(gomock.Any(), "decloud-foo").
					Return(dockerdrv.InspectResult{
						State:  "running",
						Labels: map[string]string{"decloud.service": "foo"},
					}, nil)
				h.driver.EXPECT().Stop(gomock.Any(), "decloud-foo", gomock.Any()).Return(nil)
				h.driver.EXPECT().Remove(gomock.Any(), "decloud-foo").
					Return(context.Canceled)
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newDeployerHarness(t, withoutInspectAbsentDefault())

			h.driver.EXPECT().NetworkEnsure(gomock.Any(), "decloud").Return(nil)
			h.capturer.EXPECT().Capture(gomock.Any(), gomock.Any()).Return(map[string]string{"X": "1"}, nil)
			h.store.EXPECT().Load(gomock.Any(), "foo").Return(nil, registry.ErrNotFound)
			h.driver.EXPECT().Build(gomock.Any(), gomock.Any()).Return("img", nil)
			tc.register(h)

			err := h.deployer.Deploy(context.Background(), newRequest())
			require.Error(t, err)
			assert.True(t, errors.Is(err, deploy.ErrInterrupted),
				"cancellation during orphan check must surface as ErrInterrupted; got %v", err)
			assert.False(t, errors.Is(err, deploy.ErrRun),
				"cancellation must NOT be wrapped as ErrRun; got %v", err)
		})
	}
}

func TestDeploy_RedeployStopRemovePreviousContainerCancelledReturnsErrInterrupted(t *testing.T) {
	cases := []struct {
		name     string
		register func(h *deployerHarness)
	}{
		{
			name: "stop-cancelled",
			register: func(h *deployerHarness) {
				h.driver.EXPECT().Stop(gomock.Any(), "decloud-foo", gomock.Any()).
					Return(context.Canceled)
				h.driver.EXPECT().Inspect(gomock.Any(), "decloud-foo").
					Return(dockerdrv.InspectResult{State: "running"}, nil)
			},
		},
		{
			name: "remove-cancelled",
			register: func(h *deployerHarness) {
				h.driver.EXPECT().Stop(gomock.Any(), "decloud-foo", gomock.Any()).Return(nil)
				h.driver.EXPECT().Remove(gomock.Any(), "decloud-foo").
					Return(context.Canceled)
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newDeployerHarness(t, withoutInspectAbsentDefault())
			prev := newPrev()

			h.driver.EXPECT().NetworkEnsure(gomock.Any(), "decloud").Return(nil)
			h.capturer.EXPECT().Capture(gomock.Any(), gomock.Any()).Return(map[string]string{"X": "1"}, nil)
			h.store.EXPECT().Load(gomock.Any(), "foo").Return(prev, nil)
			h.driver.EXPECT().Build(gomock.Any(), gomock.Any()).Return("img", nil)
			tc.register(h)

			err := h.deployer.Deploy(context.Background(), newRequest())
			require.Error(t, err)
			assert.True(t, errors.Is(err, deploy.ErrInterrupted),
				"cancellation during redeploy stop/remove must surface as ErrInterrupted; got %v", err)
			assert.False(t, errors.Is(err, deploy.ErrRun),
				"cancellation must NOT be wrapped as ErrRun; got %v", err)
		})
	}
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
	assert.Equal(t, "foo", seen.Service,
		"RunRequest.Service must equal req.Name so the driver derives tag=decloud/foo")
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

func newRequestWithMounts(mounts []registry.Mount) deploy.Request {
	r := newRequest()
	r.Mounts = mounts
	return r
}

func expectedVolumes(mounts []registry.Mount) []dockerdrv.VolumeMount {
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

func TestDeploy_DeployWithMountsPassesVolumesToDriver(t *testing.T) {
	h := newDeployerHarness(t)
	mounts := []registry.Mount{
		{HostPath: "/host", ContainerPath: "/data", ReadOnly: true},
		{HostPath: "vol", ContainerPath: "/var", ReadOnly: false},
	}
	var seenVolumes []dockerdrv.VolumeMount

	h.driver.EXPECT().NetworkEnsure(gomock.Any(), "decloud").Return(nil)
	h.capturer.EXPECT().Capture(gomock.Any(), gomock.Any()).Return(map[string]string{"X": "1"}, nil)
	h.store.EXPECT().Load(gomock.Any(), "foo").Return(nil, registry.ErrNotFound)
	h.driver.EXPECT().Build(gomock.Any(), gomock.Any()).Return("img", nil)
	h.driver.EXPECT().Run(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req dockerdrv.RunRequest) (string, error) {
			seenVolumes = req.Volumes
			return "cid", nil
		})
	h.driver.EXPECT().ContainerIP(gomock.Any(), gomock.Any()).Return("172.18.0.5", nil)
	h.store.EXPECT().Save(gomock.Any(), gomock.Any()).Return(nil)
	h.store.EXPECT().List(gomock.Any()).Return(nil, nil)
	h.generator.EXPECT().Generate(gomock.Any(), gomock.Any()).DoAndReturn(stubGenerate)
	h.reloader.EXPECT().Validate(gomock.Any(), gomock.Any()).Return(nil)
	h.reloader.EXPECT().Reload(gomock.Any(), gomock.Any()).Return(nil)

	require.NoError(t, h.deployer.Deploy(context.Background(), newRequestWithMounts(mounts)))
	assert.Equal(t, expectedVolumes(mounts), seenVolumes,
		"deploy must thread req.Mounts → driver.Run.Volumes with IsNamed derived from HostPath")
	require.Len(t, seenVolumes, 2)
	assert.False(t, seenVolumes[0].IsNamed, "/host must classify as bind, not named")
	assert.True(t, seenVolumes[1].IsNamed, "vol must classify as named volume")
}

func TestDeploy_DeployWithMountsSavesMountsToRegistry(t *testing.T) {
	h := newDeployerHarness(t)
	mounts := []registry.Mount{
		{HostPath: "/host", ContainerPath: "/data", ReadOnly: true},
		{HostPath: "vol", ContainerPath: "/var", ReadOnly: false},
	}
	var savedMounts []registry.Mount

	h.driver.EXPECT().NetworkEnsure(gomock.Any(), "decloud").Return(nil)
	h.capturer.EXPECT().Capture(gomock.Any(), gomock.Any()).Return(map[string]string{"X": "1"}, nil)
	h.store.EXPECT().Load(gomock.Any(), "foo").Return(nil, registry.ErrNotFound)
	h.driver.EXPECT().Build(gomock.Any(), gomock.Any()).Return("img", nil)
	h.driver.EXPECT().Run(gomock.Any(), gomock.Any()).Return("cid", nil)
	h.driver.EXPECT().ContainerIP(gomock.Any(), gomock.Any()).Return("172.18.0.5", nil)
	h.store.EXPECT().Save(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, svc *registry.Service) error {
			savedMounts = svc.Config.Run.Mounts
			return nil
		})
	h.store.EXPECT().List(gomock.Any()).Return(nil, nil)
	h.generator.EXPECT().Generate(gomock.Any(), gomock.Any()).DoAndReturn(stubGenerate)
	h.reloader.EXPECT().Validate(gomock.Any(), gomock.Any()).Return(nil)
	h.reloader.EXPECT().Reload(gomock.Any(), gomock.Any()).Return(nil)

	require.NoError(t, h.deployer.Deploy(context.Background(), newRequestWithMounts(mounts)))
	assert.Equal(t, mounts, savedMounts,
		"registry.Save must persist req.Mounts so subsequent Load round-trips them")
}

func TestDeploy_RestoreOldContainerPassesVolumesToDriver(t *testing.T) {
	h := newDeployerHarness(t)
	prev := newPrev()
	prev.Config.Run.Mounts = []registry.Mount{
		{HostPath: "/old-host", ContainerPath: "/data", ReadOnly: false},
		{HostPath: "oldvol", ContainerPath: "/var/lib", ReadOnly: true},
	}
	var rollbackVolumes []dockerdrv.VolumeMount
	var rollbackSvc string

	h.driver.EXPECT().NetworkEnsure(gomock.Any(), "decloud").Return(nil)
	h.capturer.EXPECT().Capture(gomock.Any(), gomock.Any()).Return(map[string]string{"X": "1"}, nil)
	h.store.EXPECT().Load(gomock.Any(), "foo").Return(prev, nil)
	h.driver.EXPECT().Build(gomock.Any(), gomock.Any()).Return("img", nil)
	h.driver.EXPECT().Stop(gomock.Any(), "decloud-foo", gomock.Any()).Return(nil)
	h.driver.EXPECT().Remove(gomock.Any(), "decloud-foo").Return(nil)
	gomock.InOrder(
		h.driver.EXPECT().Run(gomock.Any(), gomock.Any()).Return("", errors.New("docker run failed")),
		h.driver.EXPECT().Run(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, req dockerdrv.RunRequest) (string, error) {
				rollbackVolumes = req.Volumes
				rollbackSvc = req.Service
				return "rb-cid", nil
			}),
	)

	err := h.deployer.Deploy(context.Background(), newRequest())
	require.Error(t, err)
	assert.True(t, errors.Is(err, deploy.ErrRun))
	assert.Equal(t, expectedVolumes(prev.Config.Run.Mounts), rollbackVolumes,
		"rollback must re-apply prev.Config.Run.Mounts so the recreate strategy preserves volumes")
	assert.Equal(t, "foo", rollbackSvc,
		"rollback RunRequest.Service must equal prev.Config.Name so the restored container shares the journald tag")
}

// expectHappyPathDeploy installs the standard happy-path expectations around a
// caller-supplied Load result. Passing a nil prev makes it a first deploy
// (ErrNotFound, no stop/remove of a previous container).
func expectHappyPathDeploy(h *deployerHarness, prev *registry.Service) {
	h.driver.EXPECT().NetworkEnsure(gomock.Any(), "decloud").Return(nil)
	h.capturer.EXPECT().Capture(gomock.Any(), gomock.Any()).Return(map[string]string{"X": "1"}, nil)
	if prev == nil {
		h.store.EXPECT().Load(gomock.Any(), "foo").Return(nil, registry.ErrNotFound)
	} else {
		h.store.EXPECT().Load(gomock.Any(), "foo").Return(prev, nil)
		h.driver.EXPECT().Stop(gomock.Any(), "decloud-foo", gomock.Any()).Return(nil)
		h.driver.EXPECT().Remove(gomock.Any(), "decloud-foo").Return(nil)
	}
	h.driver.EXPECT().Build(gomock.Any(), gomock.Any()).Return("img-id", nil)
	h.driver.EXPECT().Run(gomock.Any(), gomock.Any()).Return("cid", nil)
	h.driver.EXPECT().ContainerIP(gomock.Any(), gomock.Any()).Return("172.18.0.5", nil)
	h.store.EXPECT().List(gomock.Any()).Return(nil, nil)
	h.generator.EXPECT().Generate(gomock.Any(), gomock.Any()).DoAndReturn(stubGenerate)
	h.reloader.EXPECT().Validate(gomock.Any(), gomock.Any()).Return(nil)
	h.reloader.EXPECT().Reload(gomock.Any(), gomock.Any()).Return(nil)
}

// captureDeployLogs redirects the default slog logger — which Deploy derives
// its logger from — into a buffer for the duration of t. slog.SetDefault is
// process-global, so no test using this may call t.Parallel().
func captureDeployLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

func TestDeploy_PersistsDisableCompression(t *testing.T) {
	h := newDeployerHarness(t)
	var saved *registry.Service

	expectHappyPathDeploy(h, nil)
	h.store.EXPECT().Save(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, svc *registry.Service) error {
			saved = svc
			return nil
		})

	req := newRequest()
	req.DisableCompression = true
	require.NoError(t, h.deployer.Deploy(context.Background(), req))

	require.NotNil(t, saved)
	assert.True(t, saved.Config.DisableCompression,
		"Request.DisableCompression must reach the persisted config; the generator reads it from there")
}

func TestDeploy_WarnsWhenCompressionReEnabledOnRedeploy(t *testing.T) {
	cases := []struct {
		name              string
		prevDisabled      bool
		hasPrev           bool
		requestedDisabled bool
		wantWarning       bool
	}{
		{"reset_without_flag", true, true, false, true},
		{"flag_passed_again", true, true, true, false},
		{"first_deploy_has_no_previous_config", false, false, false, false},
		{"ordinary_redeploy_never_disabled", false, true, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newDeployerHarness(t)
			logs := captureDeployLogs(t)

			var prev *registry.Service
			if tc.hasPrev {
				prev = newPrev()
				prev.Config.DisableCompression = tc.prevDisabled
			}
			expectHappyPathDeploy(h, prev)
			h.store.EXPECT().Save(gomock.Any(), gomock.Any()).Return(nil)

			req := newRequest()
			req.DisableCompression = tc.requestedDisabled
			require.NoError(t, h.deployer.Deploy(context.Background(), req))

			if tc.wantWarning {
				assert.Contains(t, logs.String(), "--no-compression",
					"the warning must name the flag that keeps compression off")
				assert.Contains(t, logs.String(), "disable_compression",
					"the warning must name the key the operator sees in the TOML")
				return
			}
			assert.NotContains(t, logs.String(), "--no-compression",
				"a warning nobody can act on is trained-to-ignore within a week")
		})
	}
}
