package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexander-fenster/decloud/internal/cli/mocks"
	"github.com/alexander-fenster/decloud/internal/config"
	"github.com/alexander-fenster/decloud/internal/deploy"
	"github.com/alexander-fenster/decloud/internal/envcap"
	"github.com/alexander-fenster/decloud/internal/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// installMockDeployer overrides deployerFactory for the duration of t and
// returns the mock so the test can set up expectations.
func installMockDeployer(t *testing.T) *mocks.MockServiceDeployer {
	t.Helper()
	ctrl := gomock.NewController(t)
	mock := mocks.NewMockServiceDeployer(ctrl)
	prev := deployerFactory
	deployerFactory = func(_ config.Paths) (deploy.ServiceDeployer, error) { return mock, nil }
	t.Cleanup(func() { deployerFactory = prev })
	return mock
}

func runRoot(t *testing.T, args ...string) (stdout, stderr *bytes.Buffer, err error) {
	t.Helper()
	stdout = &bytes.Buffer{}
	stderr = &bytes.Buffer{}
	cmd := NewRootCmd()
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs(args)
	err = cmd.ExecuteContext(context.Background())
	return stdout, stderr, err
}

func TestDeployService_BuildsExpectedRequest(t *testing.T) {
	mock := installMockDeployer(t)
	var got deploy.Request
	mock.EXPECT().Deploy(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req deploy.Request) error {
			got = req
			return nil
		})

	_, _, err := runRoot(t,
		"deploy", "service",
		"--name", "foo",
		"--host", "foo.example.com",
		"--port", "8080",
		"/srv/foo",
	)
	require.NoError(t, err)
	assert.Equal(t, "foo", got.Name)
	assert.Equal(t, []string{"foo.example.com"}, got.Hosts)
	assert.Equal(t, 8080, got.Port)
	assert.Equal(t, "recreate", got.Strategy)
	assert.Equal(t, "/healthz", got.ReadinessPath)
	assert.True(t, strings.HasSuffix(got.SourceDir, "/srv/foo") || got.SourceDir == "/srv/foo",
		"source dir must be resolved (got %q)", got.SourceDir)
	assert.Equal(t, "", got.EnvFile,
		"no env.sh present at /srv/foo and no --env-file flag: EnvFile must be empty")
}

func TestDeployService_MissingNameReturnsExitUsageError(t *testing.T) {
	installMockDeployer(t)
	_, _, err := runRoot(t, "deploy", "service", "/srv/foo")
	require.Error(t, err)
	assert.Equal(t, ExitUsageError, ExitCodeFor(err))
}

func TestDeployService_MountFlagAcceptsValidMounts(t *testing.T) {
	mock := installMockDeployer(t)
	var got deploy.Request
	mock.EXPECT().Deploy(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req deploy.Request) error {
			got = req
			return nil
		})

	_, _, err := runRoot(t,
		"deploy", "service",
		"--name", "foo",
		"--port", "8080",
		"--mount", "/h1:/c1",
		"--mount", "/h2:/c2:ro",
		"--mount", "vol1:/c3:ro",
		"/srv/foo",
	)
	require.NoError(t, err)
	require.Len(t, got.Mounts, 3)
	assert.Equal(t, registry.Mount{HostPath: "/h1", ContainerPath: "/c1", ReadOnly: false}, got.Mounts[0])
	assert.Equal(t, registry.Mount{HostPath: "/h2", ContainerPath: "/c2", ReadOnly: true}, got.Mounts[1])
	assert.Equal(t, registry.Mount{HostPath: "vol1", ContainerPath: "/c3", ReadOnly: true}, got.Mounts[2])
	assert.True(t, got.Mounts[2].IsNamed(), "vol1 must be classified as a named volume")
}

func TestDeployService_MountFlagInvalidReturnsExitUsageError(t *testing.T) {
	cases := []struct {
		name     string
		mountArg []string
	}{
		{"single_component", []string{"--mount", "/foo"}},
		{"unknown_mode", []string{"--mount", "/h:/c:zz"}},
		{"empty_target", []string{"--mount", "/h:"}},
		{"relative_target", []string{"--mount", "/h:relative"}},
		{"duplicate_target", []string{"--mount", "/a:/x", "--mount", "/b:/x"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			installMockDeployer(t)
			args := append([]string{
				"deploy", "service",
				"--name", "foo",
				"--port", "8080",
			}, tc.mountArg...)
			args = append(args, "/srv/foo")

			_, _, err := runRoot(t, args...)
			require.Error(t, err)
			assert.True(t, errors.Is(err, errUsage),
				"CLI parse failure must wrap errUsage so exit code is 2; got %v", err)
			assert.Equal(t, ExitUsageError, ExitCodeFor(err))
			assert.Contains(t, err.Error(), "--mount",
				"error must name the offending flag for operator-debug context")
			if tc.name == "duplicate_target" {
				assert.False(t, errors.Is(err, registry.ErrInvalidMount),
					"CLI dup-target must NOT chain ErrInvalidMount; see addendum Issue 1")
			}
		})
	}
}

func TestDeployService_MountFlagPathWithCommaWorks(t *testing.T) {
	mock := installMockDeployer(t)
	var got deploy.Request
	mock.EXPECT().Deploy(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req deploy.Request) error {
			got = req
			return nil
		})

	_, _, err := runRoot(t,
		"deploy", "service",
		"--name", "foo",
		"--port", "8080",
		"--mount", "/path/with,comma:/data",
		"/srv/foo",
	)
	require.NoError(t, err)
	require.Len(t, got.Mounts, 1)
	assert.Equal(t, "/path/with,comma", got.Mounts[0].HostPath,
		"comma in host path must NOT split the flag value (StringArrayVar, not StringSliceVar)")
}

func TestDeployService_StrategyBlueGreenReturnsErrInvalidStrategy(t *testing.T) {
	installMockDeployer(t)
	_, _, err := runRoot(t,
		"deploy", "service",
		"--name", "foo",
		"--strategy", "blue_green",
		"/srv/foo",
	)
	require.Error(t, err)
	assert.True(t, errors.Is(err, registry.ErrInvalidStrategy))
	assert.Equal(t, ExitConfigError, ExitCodeFor(err))
}

func TestDeployService_HostWithoutPortReturnsExitUsageError(t *testing.T) {
	installMockDeployer(t)
	_, _, err := runRoot(t,
		"deploy", "service",
		"--name", "foo",
		"--host", "foo.example.com",
		"/srv/foo",
	)
	require.Error(t, err)
	assert.Equal(t, ExitUsageError, ExitCodeFor(err))
}

func TestDeployService_DefaultStrategyIsRecreate(t *testing.T) {
	mock := installMockDeployer(t)
	var got deploy.Request
	mock.EXPECT().Deploy(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req deploy.Request) error {
			got = req
			return nil
		})

	_, _, err := runRoot(t, "deploy", "service",
		"--name", "foo",
		"--port", "8080",
		"/srv/foo",
	)
	require.NoError(t, err)
	assert.Equal(t, "recreate", got.Strategy)
}

func TestDeployService_AutoDiscoversEnvSh(t *testing.T) {
	mock := installMockDeployer(t)
	var got deploy.Request
	mock.EXPECT().Deploy(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req deploy.Request) error {
			got = req
			return nil
		})

	sourceDir := t.TempDir()
	envSh := filepath.Join(sourceDir, "env.sh")
	require.NoError(t, os.WriteFile(envSh, []byte("export X=1\n"), 0o644))

	_, _, err := runRoot(t, "deploy", "service",
		"--name", "foo",
		"--port", "8080",
		sourceDir,
	)
	require.NoError(t, err)
	assert.Equal(t, envSh, got.EnvFile)
}

func TestDeployService_NoEnvShIsValid(t *testing.T) {
	mock := installMockDeployer(t)
	var got deploy.Request
	mock.EXPECT().Deploy(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req deploy.Request) error {
			got = req
			return nil
		})

	sourceDir := t.TempDir()

	_, _, err := runRoot(t, "deploy", "service",
		"--name", "foo",
		"--port", "8080",
		sourceDir,
	)
	require.NoError(t, err)
	assert.Equal(t, "", got.EnvFile)
}

func TestDeployService_ExplicitEnvFileMissingReturnsExitConfigError(t *testing.T) {
	installMockDeployer(t)

	_, _, err := runRoot(t,
		"deploy", "service",
		"--name", "foo",
		"--port", "8080",
		"--env-file", "/no/such/file",
		t.TempDir(),
	)
	require.Error(t, err)
	assert.True(t, errors.Is(err, envcap.ErrEnvScriptMissing))
	assert.Equal(t, ExitConfigError, ExitCodeFor(err))
}

func TestDeployService_DefaultDockerfileIsJoinedWithSourceDir(t *testing.T) {
	mock := installMockDeployer(t)
	var got deploy.Request
	mock.EXPECT().Deploy(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req deploy.Request) error {
			got = req
			return nil
		})

	sourceDir := t.TempDir()
	_, _, err := runRoot(t, "deploy", "service",
		"--name", "foo",
		"--port", "8080",
		sourceDir,
	)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(sourceDir, "Dockerfile"), got.Dockerfile)
}

func TestDeployService_RelativeDockerfileIsJoinedWithSourceDir(t *testing.T) {
	mock := installMockDeployer(t)
	var got deploy.Request
	mock.EXPECT().Deploy(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req deploy.Request) error {
			got = req
			return nil
		})

	sourceDir := t.TempDir()
	_, _, err := runRoot(t, "deploy", "service",
		"--name", "foo",
		"--port", "8080",
		"--dockerfile", "docker/prod.Dockerfile",
		sourceDir,
	)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(sourceDir, "docker", "prod.Dockerfile"), got.Dockerfile)
}

func TestDeployService_AbsoluteDockerfileIsPreserved(t *testing.T) {
	mock := installMockDeployer(t)
	var got deploy.Request
	mock.EXPECT().Deploy(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req deploy.Request) error {
			got = req
			return nil
		})

	abs := "/etc/shared/X.Dockerfile"
	_, _, err := runRoot(t, "deploy", "service",
		"--name", "foo",
		"--port", "8080",
		"--dockerfile", abs,
		t.TempDir(),
	)
	require.NoError(t, err)
	assert.Equal(t, abs, got.Dockerfile)
}

func TestDeployService_RelativeSourceDirAndRelativeDockerfileBothResolved(t *testing.T) {
	mock := installMockDeployer(t)
	var got deploy.Request
	mock.EXPECT().Deploy(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req deploy.Request) error {
			got = req
			return nil
		})

	parent, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	sub := filepath.Join(parent, "svc")
	require.NoError(t, os.MkdirAll(sub, 0o755))

	origCwd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(parent))
	t.Cleanup(func() { _ = os.Chdir(origCwd) })

	_, _, err = runRoot(t, "deploy", "service",
		"--name", "foo",
		"--port", "8080",
		"./svc",
	)
	require.NoError(t, err)
	assert.True(t, filepath.IsAbs(got.Dockerfile),
		"Dockerfile must be absolute, got %q", got.Dockerfile)
	assert.Equal(t, filepath.Join(sub, "Dockerfile"), got.Dockerfile)
}

func TestDeployService_NoPortReturnsExitUsageError(t *testing.T) {
	installMockDeployer(t)
	_, _, err := runRoot(t,
		"deploy", "service",
		"--name", "foo",
		t.TempDir(),
	)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUsage))
	assert.Equal(t, ExitUsageError, ExitCodeFor(err))
}

func TestDeployService_PortZeroExplicitReturnsExitUsageError(t *testing.T) {
	installMockDeployer(t)
	_, _, err := runRoot(t,
		"deploy", "service",
		"--name", "foo",
		"--port", "0",
		t.TempDir(),
	)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUsage))
	assert.Equal(t, ExitUsageError, ExitCodeFor(err))
}
