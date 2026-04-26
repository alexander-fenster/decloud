package caddy

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordedCmd struct {
	Name string
	Args []string
}

func recordingFactory(records *[]recordedCmd) cmdFactory {
	return func(ctx context.Context, name string, args ...string) *exec.Cmd {
		*records = append(*records, recordedCmd{Name: name, Args: args})
		return exec.CommandContext(ctx, "true")
	}
}

func failingFactory(stderr string) cmdFactory {
	return func(ctx context.Context, name string, args ...string) *exec.Cmd {
		c := exec.CommandContext(ctx, "/bin/sh", "-c", "echo "+strings.ReplaceAll(stderr, " ", "_")+" 1>&2; exit 1")
		return c
	}
}

func TestReloader_InvokesCaddyValidate(t *testing.T) {
	var records []recordedCmd
	r := newCLIReloaderWithFactory(recordingFactory(&records))

	require.NoError(t, r.Validate(context.Background(), "/etc/caddy/Caddyfile"))
	require.Len(t, records, 1)
	assert.Equal(t, "caddy", records[0].Name)
	assert.Equal(t, []string{"validate", "--config", "/etc/caddy/Caddyfile"}, records[0].Args)
}

func TestReloader_InvokesCaddyReload(t *testing.T) {
	var records []recordedCmd
	r := newCLIReloaderWithFactory(recordingFactory(&records))

	require.NoError(t, r.Reload(context.Background(), "/etc/caddy/Caddyfile"))
	require.Len(t, records, 1)
	assert.Equal(t, "caddy", records[0].Name)
	assert.Equal(t, []string{"reload", "--config", "/etc/caddy/Caddyfile"}, records[0].Args)
}

func TestReloader_ValidateFailureReturnsError(t *testing.T) {
	r := newCLIReloaderWithFactory(failingFactory("bad_caddyfile_syntax"))
	err := r.Validate(context.Background(), "/etc/caddy/Caddyfile")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bad_caddyfile_syntax")
}
