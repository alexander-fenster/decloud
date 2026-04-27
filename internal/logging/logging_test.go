package logging_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alexander-fenster/decloud/internal/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInit_StderrOnlyShortCircuit(t *testing.T) {
	t.Setenv("DECLOUD_LOG_TO_STDERR_ONLY", "1")
	require.NoError(t, logging.Init("/path/that/cannot/exist/for/test"))
}

func TestInit_DefaultWritesToFileAndStderr(t *testing.T) {
	root := t.TempDir()
	t.Setenv("DECLOUD_LOG_TO_STDERR_ONLY", "")

	require.NoError(t, logging.Init(root))

	logPath := filepath.Join(root, "logs", "decloud.log")
	info, err := os.Stat(logPath)
	require.NoError(t, err)
	assert.False(t, info.IsDir())
}

func TestInit_PermissionDeniedFallsBackToStderr(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root bypasses the read-only directory check")
	}
	root := filepath.Join(t.TempDir(), "wedged")
	require.NoError(t, os.MkdirAll(root, 0o755))
	require.NoError(t, os.Chmod(root, 0o500))
	t.Cleanup(func() { _ = os.Chmod(root, 0o755) })

	t.Setenv("DECLOUD_LOG_TO_STDERR_ONLY", "")

	require.NoError(t, logging.Init(root))
}

func TestInit_LogFileOpenFailureFallsBackToStderr(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root bypasses the read-only directory check")
	}
	root := t.TempDir()
	logsDir := filepath.Join(root, "logs")
	require.NoError(t, os.MkdirAll(logsDir, 0o755))
	require.NoError(t, os.Chmod(logsDir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(logsDir, 0o755) })

	t.Setenv("DECLOUD_LOG_TO_STDERR_ONLY", "")

	require.NoError(t, logging.Init(root))
}

func TestInit_UsesPassedRootNotEnv(t *testing.T) {
	t.Setenv("DECLOUD_LOG_TO_STDERR_ONLY", "")
	t.Setenv("DECLOUD_ROOT", "/path/that/must/not/be/written/to")

	root := t.TempDir()
	require.NoError(t, logging.Init(root))

	logPath := filepath.Join(root, "logs", "decloud.log")
	_, err := os.Stat(logPath)
	require.NoError(t, err, "log must be written under the passed root, not env")

	_, err = os.Stat("/path/that/must/not/be/written/to")
	assert.True(t, os.IsNotExist(err),
		"DECLOUD_ROOT must not be consulted by Init when a root is passed")
}

func TestInit_EmptyStringRootFallsBackToDefault(t *testing.T) {
	t.Setenv("DECLOUD_LOG_TO_STDERR_ONLY", "1")
	require.NoError(t, logging.Init(""))
}
