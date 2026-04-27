package cli

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/alexander-fenster/decloud/internal/cli/mocks"
	"github.com/alexander-fenster/decloud/internal/config"
	"github.com/alexander-fenster/decloud/internal/deploy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// captureConfigRoot installs a deployerFactory that records the Paths.Root it
// was called with, then runs the given args against a fresh root command.
func captureConfigRoot(t *testing.T, args ...string) string {
	t.Helper()
	ctrl := gomock.NewController(t)
	mock := mocks.NewMockServiceDeployer(ctrl)
	mock.EXPECT().Deploy(gomock.Any(), gomock.Any()).Return(nil)

	var seenRoot string
	prev := deployerFactory
	deployerFactory = func(p config.Paths) (deploy.ServiceDeployer, error) {
		seenRoot = p.Root
		return mock, nil
	}
	t.Cleanup(func() { deployerFactory = prev })

	cmd := NewRootCmd()
	cmd.SetArgs(args)
	require.NoError(t, cmd.ExecuteContext(context.Background()))
	return seenRoot
}

func TestRoot_ConfigRootDefaultsToDecloudRootEnv(t *testing.T) {
	t.Setenv("DECLOUD_ROOT", "/tmp/from-env")
	root := captureConfigRoot(t,
		"deploy", "service", "--name", "foo", "--port", "8080", "/srv/foo",
	)
	assert.Equal(t, "/tmp/from-env", root)
}

func TestRoot_ConfigRootFlagOverridesEnv(t *testing.T) {
	t.Setenv("DECLOUD_ROOT", "/tmp/from-env")
	root := captureConfigRoot(t,
		"--config-root", "/tmp/from-flag",
		"deploy", "service", "--name", "foo", "--port", "8080", "/srv/foo",
	)
	assert.Equal(t, "/tmp/from-flag", root)
}

func TestRoot_HelpDoesNotRequireFilesystem(t *testing.T) {
	t.Setenv("DECLOUD_ROOT", "/nonexistent/path/that/cannot/be/created/by/test")
	t.Setenv("DECLOUD_LOG_TO_STDERR_ONLY", "")

	cmd := NewRootCmd()
	cmd.SetArgs([]string{"--help"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	require.NoError(t, cmd.ExecuteContext(context.Background()))
}

// TestRoot_ConfigRootFlagControlsLogPlacement relies on Cobra's
// flag-default-from-env mechanism in root.go (the --config-root flag default
// is config.RootFromEnv()), so passing --config-root overrides DECLOUD_ROOT
// for both registry paths and log placement.
func TestRoot_ConfigRootFlagControlsLogPlacement(t *testing.T) {
	envRoot := t.TempDir()
	flagRoot := t.TempDir()
	t.Setenv("DECLOUD_LOG_TO_STDERR_ONLY", "")
	t.Setenv("DECLOUD_ROOT", envRoot)

	captureConfigRoot(t,
		"--config-root", flagRoot,
		"deploy", "service",
		"--name", "foo",
		"--port", "8080",
		t.TempDir(),
	)

	flagLog := filepath.Join(flagRoot, "logs", "decloud.log")
	_, err := os.Stat(flagLog)
	require.NoError(t, err, "log must be at flagRoot/logs/decloud.log")

	envLog := filepath.Join(envRoot, "logs", "decloud.log")
	_, err = os.Stat(envLog)
	assert.True(t, os.IsNotExist(err),
		"log must NOT be at envRoot/logs/decloud.log")
}
