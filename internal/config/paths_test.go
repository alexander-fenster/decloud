package config_test

import (
	"path/filepath"
	"testing"

	"github.com/alexander-fenster/decloud/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPaths_AllPathsRootedCorrectly(t *testing.T) {
	root := "/srv/decloud-test"
	paths := config.NewPaths(root)

	require.Equal(t, root, paths.Root)
	assert.Equal(t, filepath.Join(root, "config"), paths.ConfigDir)
	assert.Equal(t, filepath.Join(root, "config", "services"), paths.ServicesDir)
	assert.Equal(t, filepath.Join(root, "config", "jobs"), paths.JobsDir)
	assert.Equal(t, filepath.Join(root, "config", "caddy"), paths.CaddyDir)
	assert.Equal(t, filepath.Join(root, "config", "caddy", "Caddyfile"), paths.CaddyfilePath)
	assert.Equal(t, filepath.Join(root, "secrets"), paths.SecretsDir)
	assert.Equal(t, filepath.Join(root, "state"), paths.StateDir)
	assert.Equal(t, filepath.Join(root, "state", "deploys"), paths.DeploysDir)
	assert.Equal(t, filepath.Join(root, "logs"), paths.LogsDir)
	assert.Equal(t, filepath.Join(root, "logs", "decloud.log"), paths.LogFile)
}

func TestNewPaths_EmptyRootFallsBackToDefault(t *testing.T) {
	paths := config.NewPaths("")
	assert.Equal(t, config.DefaultRoot, paths.Root)
	assert.Equal(t, filepath.Join(config.DefaultRoot, "config", "caddy", "Caddyfile"), paths.CaddyfilePath)
}

func TestRootFromEnv_HonorsDecloudRoot(t *testing.T) {
	t.Setenv("DECLOUD_ROOT", "/tmp/declouding-test-x")
	assert.Equal(t, "/tmp/declouding-test-x", config.RootFromEnv())
}

func TestRootFromEnv_DefaultsToDecloudingPath(t *testing.T) {
	t.Setenv("DECLOUD_ROOT", "")
	assert.Equal(t, config.DefaultRoot, config.RootFromEnv())
}
