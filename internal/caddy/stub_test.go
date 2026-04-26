package caddy_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alexander-fenster/decloud/internal/caddy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStub_WritesValidCaddyfileWhenAbsent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Caddyfile")
	require.NoError(t, caddy.WriteStubIfMissing(path))

	body, err := os.ReadFile(path)
	require.NoError(t, err)
	str := string(body)
	assert.Contains(t, str, ":80")
	assert.Contains(t, str, "respond")
	assert.Contains(t, str, "404")
	assert.Contains(t, str, "no services registered yet")
}

func TestStub_NoOpWhenFileExists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Caddyfile")
	original := []byte("# operator-edited\n:80 { respond \"hello\" 200 }\n")
	require.NoError(t, os.WriteFile(path, original, 0o644))

	require.NoError(t, caddy.WriteStubIfMissing(path))

	body, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, string(original), string(body))
}
