//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alexander-fenster/decloud/internal/dockerdrv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	integrationEnvVar    = "DECLOUD_INTEGRATION"
	mountTestContainer   = "decloud-mounttest"
	mountTestImage       = "nginx:alpine"
	mountTestMarkerBytes = "hello-mount-m2"
)

func skipUnlessIntegrationEnabled(t *testing.T) {
	t.Helper()
	if os.Getenv(integrationEnvVar) != "1" {
		t.Skipf("set %s=1 to run real-Docker integration tests", integrationEnvVar)
	}
}

func removeContainerIdempotent(t *testing.T, name string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_ = exec.CommandContext(ctx, "docker", "rm", "-f", name).Run()
}

func ensureNetwork(t *testing.T, driver dockerdrv.Driver) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	require.NoError(t, driver.NetworkEnsure(ctx, "decloud"))
}

func writeMarkerFile(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "marker.txt"), []byte(mountTestMarkerBytes), 0o644))
}

func TestIntegration_MountBindRoundTrip(t *testing.T) {
	skipUnlessIntegrationEnabled(t)

	hostDir := t.TempDir()
	writeMarkerFile(t, hostDir)
	driver := dockerdrv.NewCLIDriver()
	ensureNetwork(t, driver)
	t.Cleanup(func() { removeContainerIdempotent(t, mountTestContainer) })
	removeContainerIdempotent(t, mountTestContainer)

	pullCtx, pullCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer pullCancel()
	require.NoError(t, driver.ImagePull(pullCtx, mountTestImage))

	runCtx, runCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer runCancel()
	_, err := driver.Run(runCtx, dockerdrv.RunRequest{
		Name:    mountTestContainer,
		Image:   mountTestImage,
		Network: "decloud",
		Restart: "no",
		Volumes: []dockerdrv.VolumeMount{
			{Source: hostDir, Target: "/data", ReadOnly: true, IsNamed: false},
		},
	})
	require.NoError(t, err, "real docker run with -v must accept the bind-mount argv shape")

	execCtx, execCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer execCancel()
	var stdout bytes.Buffer
	require.NoError(t, driver.Exec(execCtx, dockerdrv.ExecOptions{
		Container: mountTestContainer,
		Cmd:       []string{"cat", "/data/marker.txt"},
		Stdout:    &stdout,
	}))
	assert.Equal(t, mountTestMarkerBytes, strings.TrimSpace(stdout.String()),
		"file written on host must be visible inside the container at the bind target")
}
