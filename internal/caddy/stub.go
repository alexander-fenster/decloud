package caddy

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

const stubBody = `# decloud Caddyfile stub — no services registered yet.
:80 {
    respond "no services registered yet" 404
}
`

// WriteStubIfMissing writes the first-deploy Caddyfile stub at path iff path
// does not yet exist.
func WriteStubIfMissing(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(stubBody), 0o644)
}
