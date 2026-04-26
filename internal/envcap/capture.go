package envcap

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"strings"
)

// ErrEnvScriptMissing means the explicitly-requested env script does not
// exist. CLI maps to ExitConfigError (10).
var ErrEnvScriptMissing = errors.New("envcap: env script not found")

// ErrEnvScriptUnreadable means the env script exists but the OS denied
// stat or open, or the path resolves to a non-regular file. CLI maps to
// ExitConfigError (10).
var ErrEnvScriptUnreadable = errors.New("envcap: env script unreadable")

//go:generate mockgen -source=capture.go -destination=mocks/mock_capturer.go -package=mocks

// Capturer extracts the environment variables a bash script sets when sourced.
type Capturer interface {
	Capture(ctx context.Context, scriptPath string) (map[string]string, error)
}

// New returns the production Capturer backed by /bin/bash.
func New() Capturer {
	return &bashCapturer{}
}

type bashCapturer struct{}

const captureScript = `if [ -n "$1" ]; then
    set -a
    source "$1"
fi
while IFS= read -r __name; do
    printf "%s=%s\0" "$__name" "${!__name}"
done < <(compgen -e)
`

func (b *bashCapturer) Capture(ctx context.Context, scriptPath string) (map[string]string, error) {
	if scriptPath == "" {
		return nil, nil
	}
	info, err := os.Stat(scriptPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s", ErrEnvScriptMissing, scriptPath)
		}
		return nil, fmt.Errorf("%w: %s: %w", ErrEnvScriptUnreadable, scriptPath, err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("%w: %s is a directory", ErrEnvScriptUnreadable, scriptPath)
	}
	baseline, err := runHermeticBash(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("env capture baseline: %w", err)
	}
	full, err := runHermeticBash(ctx, scriptPath)
	if err != nil {
		return nil, fmt.Errorf("env capture: %w", err)
	}
	return diffEnv(baseline, full), nil
}

func runHermeticBash(ctx context.Context, scriptPath string) (map[string]string, error) {
	args := []string{
		"-i",
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"HOME=/tmp",
		"/bin/bash", "--noprofile", "--norc", "-c", captureScript, "_", scriptPath,
	}
	cmd := exec.CommandContext(ctx, "/usr/bin/env", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("bash exited: %w; stderr=%q", err, stderr.String())
	}
	return parseNullDelimited(stdout.Bytes()), nil
}

func parseNullDelimited(data []byte) map[string]string {
	out := make(map[string]string)
	for _, rec := range bytes.Split(data, []byte{0}) {
		if len(rec) == 0 {
			continue
		}
		eq := bytes.IndexByte(rec, '=')
		if eq < 0 {
			continue
		}
		key := string(rec[:eq])
		val := string(rec[eq+1:])
		out[key] = val
	}
	return out
}

func diffEnv(baseline, full map[string]string) map[string]string {
	out := make(map[string]string)
	for k, v := range full {
		if isBashInternal(k) {
			continue
		}
		if bv, ok := baseline[k]; ok && bv == v {
			continue
		}
		out[k] = v
	}
	return out
}

func isBashInternal(k string) bool {
	if strings.HasPrefix(k, "BASH_FUNC_") {
		return true
	}
	switch k {
	case "_", "PWD", "OLDPWD", "SHLVL", "BASH", "BASH_VERSION", "BASH_VERSINFO",
		"PATH", "HOME", "IFS", "PS1", "PS2", "PS4", "UID", "EUID", "PPID",
		"HOSTNAME", "HOSTTYPE", "MACHTYPE", "OSTYPE", "TERM", "LANG",
		"SHELL", "LOGNAME", "USER":
		return true
	}
	return false
}
