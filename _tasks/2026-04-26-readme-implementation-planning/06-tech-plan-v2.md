# Tech Plan v2: M1 — Server-side `decloud deploy service` end-to-end

**Author:** Joel Spolsky (implementation planner)
**Status:** Revised draft for Linus re-review. Supersedes `03-tech-plan.md`.
**Builds on:** `05-plan-v2.md` (Don Melton).
**Scope:** Implementation specification for M1 only. M2-M7 referenced only where M1 must accommodate them.

---

## 0. Diff vs `03-tech-plan.md` (read this first)

For reviewers tracking what changed since v1. Everything not in this list is unchanged in spirit (acceptance criteria, exit-code taxonomy, partial-failure rollback structure, package boundaries, the docker driver interface, the readiness model).

- **Two-file persistence (Don §3, Linus Issue 1).** `internal/registry/types.go` splits the old `ServiceSpec` into `ServiceConfig` (non-secret, mode 0644 in `config/services/`) and `ServiceSecrets` (mode 0600 in `secrets/<name>/`). Loader merges both into an in-memory `Service` struct; saver writes both atomically; deleter deletes secrets first. Permission enforcement is now a load-time check, not just a write-time discipline. New "config-without-secrets" recoverable state is documented and tested.
- **Portable env capture (Don §4.1, Linus Issue 2).** Replaced `env -0` (GNU only) with `compgen -e` + `printf '%s=%s\0'` (bash builtins). Verified working on macOS bash 3.2 (see §3 for the verification command). The hermetic `env -i ... bash --noprofile --norc -c '...'` wrapper is unchanged. Baseline-diff strategy is unchanged.
- **`schema_version` stays at 1 (Don §5).** v1 was internally contradictory; v2 commits to "M1 writes 1, M3 writes 1, only bumps when semantics break old loaders." `pelletier/go-toml/v2`'s `DisallowUnknownFields()` is the forward-compat backstop.
- **Loader rejects non-empty `mounts` (Don §7.1, Linus smaller-issues).** The hand-edit loophole closes: the CLI flag and the loader give the same "M3 only" error.
- **Stub Caddyfile on first deploy (Don §7.1, Linus smaller-issues).** If `config/caddy/Caddyfile` does not exist, the deployer writes a minimal valid file before invoking `caddy reload`, so the operator's pre-installed Caddy systemd unit doesn't crash on a missing config.
- **Viper deferred to M2 (Don §8).** `internal/cli/` uses plain Cobra plus `os.Getenv("DECLOUD_ROOT")` for `--config-root`. `internal/config/` exists and holds path constants but does not import Viper. M2 retrofits.
- **Cache sentinel dropped (Don §6).** No `cache/docker-network-created`. Just call `docker network inspect ... || docker network create ...` every deploy.
- **M1 operational deliverables added (Don §10).** `go.mod` with `go 1.22`, LICENSE, `.github/workflows/test.yml`, `_docs/` and `_ai/` targets, `slog`-based structured logging to stderr + `/opt/declouding/logs/decloud.log`. All called out in §11 below.
- **M3 subdivision noted (Don §9).** M3a (server-side mounts/secret-files/env hardening) and M3b (client binary). Not an M1 deliverable; mentioned only so the abstractions in M1 stay shaped right.
- **M1→M4 container-rename migration noted (Don §9).** Explicit M4 deliverable, not a comment in code. M1 uses `decloud-<name>`; M4 will recreate them as `decloud-<name>-<deploy-id>`.

Everything else from `03-tech-plan.md` stands.

---

## 1. Spec (unchanged from v1, restated for self-containedness)

> An operator who is SSH'd into a host that already has Docker, Caddy, and the `decloud` server binary installed — and who has a directory containing a `Dockerfile` and an `env.sh` — can run **one** `decloud deploy service` command, and at the end of that command (a) the container is running on a shared Docker network, (b) `/opt/declouding/config/services/<name>.toml` exists and is parseable by the same binary, (c) `/opt/declouding/secrets/<name>/env.toml` exists with mode 0600 in a 0700 directory and contains the captured env, (d) `/opt/declouding/config/caddy/Caddyfile` has been generated (or stubbed on first deploy) and `caddy reload` has succeeded, (e) `curl https://<host>/` reaches the container with a real Let's Encrypt cert (DNS permitting), and (f) the same binary's `status`, `logs`, `start`, `stop`, `restart`, and `unregister` subcommands operate on that service. Strategy is `recreate` only. Exit code is 0 on success, non-zero with a specific exit code per failure class on failure (see §6.4).

That is the contract. Everything below serves it.

---

## 2. Module + repository layout

### 2.1 Module path

```
github.com/alexander-fenster/decloud
```

### 2.2 Directory tree

```
declouding/
  go.mod                              # go 1.22 directive
  go.sum
  LICENSE                             # Apache-2.0 (maintainer's call; default Apache-2.0)
  .github/
    workflows/
      test.yml                        # go test ./... on push/PR; Linux runner; integration tests gated to nightly
  cmd/
    decloud/
      main.go                         # tiny: builds root cobra cmd, calls Execute, sets up slog
  internal/
    cli/                              # cobra command wiring; one file per subcommand
      root.go                         # NewRootCmd(), global flags, --config-root via os.Getenv
      deploy_service.go               # `decloud deploy service`
      unregister.go
      start.go
      stop.go
      restart.go
      status.go
      logs.go
      caddy_reload.go
      exit_codes.go                   # ExitCodeFor(err) error -> int mapping
    config/                           # process-level paths; NO Viper in M1 (deferred to M2)
      paths.go                        # Paths struct: ConfigDir, SecretsDir, StateDir, LogsDir, CaddyfilePath; built from a single root
    logging/                          # slog setup: stderr + /opt/declouding/logs/decloud.log
      logging.go
    registry/                         # service registration TOML I/O + state (TWO-FILE)
      types.go                        # ServiceConfig, ServiceSecrets, Service (merged), schema_version
      store.go                        # Store interface; fsStore implementation; atomic two-file writes
      store_test.go                   # round-trip; permission rejection; orphan recovery
    envcap/                           # env.sh capture (PORTABLE — no env -0)
      capture.go                      # Capturer interface; bashCapturer with compgen -e + printf '\0'
      capture_test.go                 # runs against real /bin/bash on macOS and Linux
    caddy/                            # Caddyfile generation + reload + first-deploy stub
      generator.go                    # Generator interface; text/template impl; deterministic ordering
      generator_test.go
      reloader.go                     # Reloader interface; caddyCLIReloader (caddy reload --config <path>)
      stub.go                         # WriteStubIfMissing(path) for first deploy
    dockerdrv/                        # docker CLI driver
      driver.go                       # Driver interface
      cli_driver.go                   # exec.Command("docker", ...) implementation
      cli_driver_test.go              # argument-construction tests
    deploy/                           # the orchestration: build -> stop-old -> run-new -> readiness -> save -> caddy reload
      service.go                      # ServiceDeployer; takes Store, Capturer, Driver, Generator, Reloader as ctor args
      service_test.go                 # gomock-driven happy + every failure branch
      readiness.go                    # HTTP probe + HEALTHCHECK polling
      readiness_test.go
    ids/                              # deterministic deploy IDs and container names
      ids.go
  _docs/                              # API/operator docs (Raymond)
    cli/
      decloud-deploy-service.md
    architecture/
      m1-recreate-strategy.md
      secrets-layout.md
    operator/
      manual-install.md
  _ai/                                # AI-facing decision records (Raymond)
    decisions/
      m1-scope.md
      secrets-split.md
      schema-versioning.md
  _tasks/                             # planning artefacts (this file lives here)
```

### 2.3 `cmd/decloud/main.go`

```go
package main

import (
    "context"
    "fmt"
    "os"
    "os/signal"
    "syscall"

    "github.com/alexander-fenster/decloud/internal/cli"
    "github.com/alexander-fenster/decloud/internal/logging"
)

func main() {
    if err := logging.Init(); err != nil {
        fmt.Fprintln(os.Stderr, "logging init failed:", err)
        os.Exit(cli.ExitInternal)
    }
    ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
    defer stop()
    if err := cli.NewRootCmd().ExecuteContext(ctx); err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(cli.ExitCodeFor(err))
    }
}
```

`logging.Init()` opens `/opt/declouding/logs/decloud.log` for append (creating parents if missing), sets `slog.SetDefault` to a `slog.NewJSONHandler` that fans out to both stderr and the file via a tiny `io.MultiWriter`. Failure to open the log file is fatal in production; in tests `Init()` honors a `DECLOUD_LOG_TO_STDERR_ONLY=1` short-circuit.

---

## 3. The macOS-portable env capture (the headline change)

### 3.1 Verification command (run this BEFORE writing code)

Any engineer can validate the mechanism on their box with one shell invocation. **On macOS (BSD env, bash 3.2 at `/bin/bash`) and on Linux (GNU env, bash 5+), this command must produce the same result: NUL-separated `KEY=VALUE` records covering the variables set by `env.sh`.**

```bash
cat > /tmp/test_env.sh <<'EOF'
export DATABASE_URL="postgres://user:p@ssw0rd@host/db"
export MULTILINE_PEM="-----BEGIN-----
line2
line3
-----END-----"
PLAIN_NO_EXPORT=hello_world
export UNICODE_VAL="héllo wörld"
EOF

/usr/bin/env -i \
    PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin \
    HOME=/tmp \
    /bin/bash --noprofile --norc -c '
set -a
source "$1"
while IFS= read -r name; do
    printf "%s=%s\0" "$name" "${!name}"
done < <(compgen -e)
' _ /tmp/test_env.sh | tr '\0' '\n'
```

I ran this on this Mac (macOS, `GNU bash, version 3.2.57(1)-release (arm64-apple-darwin25)`, `/usr/bin/env` is BSD env). It produces:

```
DATABASE_URL=postgres://user:p@ssw0rd@host/db
HOME=/tmp
MULTILINE_PEM=-----BEGIN-----
line2
line3
-----END-----
PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
PLAIN_NO_EXPORT=hello_world
PWD=/Users/fenster/dev/declouding
SHLVL=1
UNICODE_VAL=héllo wörld
```

That covers every edge case: multiline values survive (NUL is the separator, newlines are payload bytes), unicode passes through byte-transparent, `PLAIN_NO_EXPORT` is captured by `set -a` despite no `export` keyword, and bash internals (`PWD`, `SHLVL`, `HOME`, `PATH`) appear and will be filtered out by the baseline-diff in Go.

I also confirmed BSD `env -0` does NOT produce NUL-separated output on macOS — it interprets `-0` as a `name=value` argument and silently does nothing, which is the classic "looks fine until you parse it" failure mode. The `compgen -e` + `printf '\0'` mechanism avoids the GNU dependency entirely.

### 3.2 Why each piece of the bash snippet is required

- `/usr/bin/env -i` — wipes the inherited environment of the Go process. Without this, the operator's SSH session env (`SSH_AUTH_SOCK`, `LANG`, `TERM`, `LS_COLORS`, ...) leaks into the captured env. Different operators get different captures for the same `env.sh`. Nightmare to debug.
- `PATH=...` and `HOME=/tmp` re-introduced — bash and any external commands `env.sh` invokes need a PATH; bash needs `HOME` for tilde expansion. We pass a hardcoded sane PATH; we pass `/tmp` as HOME because we don't want the operator's dotfiles pulled in via `~/...` paths.
- `/bin/bash --noprofile --norc` — skip `~/.bash_profile`, `~/.bashrc`, `/etc/bash.bashrc`. Without this, the operator's `eval "$(direnv hook bash)"` runs at deploy time and your env is suddenly direnv-controlled. Or `source ~/.aws/credentials` exfiltrates AWS creds into the container's env. Real, common, dangerous.
- `set -a` — auto-export every assignment. Operators write `FOO=bar` without `export` all the time. Without `set -a` those are silently lost.
- `compgen -e` — bash builtin that lists exported variables. Bash 2+, present on macOS bash 3.2, present on Linux bash 5+. No external command, so no GNU/BSD divergence.
- `printf '%s=%s\0' "$name" "${!name}"` — `printf` is a bash builtin (also a GNU coreutils binary, but the builtin takes precedence inside bash); `\0` is interpreted by the builtin's format string; `${!name}` is bash indirect expansion ("the value of the variable named by `$name`"), bash 2+. NUL is the only byte that cannot legally appear in an env var value, so it's the only safe separator.
- `_ /tmp/test_env.sh` — `bash -c '...' _ /tmp/test_env.sh` makes `$0=_` and `$1=/tmp/test_env.sh`. The script in the `-c` argument references `"$1"`. Cleaner than embedding the path in the script body.

### 3.3 Baseline-diff strategy

The capture above includes `PATH`, `HOME`, `PWD`, `SHLVL`, plus on bash 5+ also `BASH`, `BASH_VERSION`, `_`, `BASH_EXECUTION_STRING`, etc. We run the same `env -i ... bash --noprofile --norc -c '<print env>'` command **without** sourcing the script, capture that as a baseline map, and subtract from the full capture.

Specifically: for each `(k, v)` in the full capture, drop it if the baseline contains `k` with the *same value* `v`. Keep it otherwise. This means:
- A variable the script newly defines: kept (not in baseline). Correct.
- A variable the script changes from the baseline value: kept (baseline has `k` with different `v`). Correct.
- A variable identical in baseline and capture (e.g. unmodified `PATH`): dropped. Correct — script didn't touch it.
- A variable the script `unset`s: not in capture, so not in output. Correct — operator's intent.

The one residual limitation Linus already accepted: if a script *intends* to set a variable to a value that happens to equal the baseline, the assignment is dropped. Vanishingly rare; the most common case (`export PATH="$PATH"`) is a no-op anyway. Documented in operator-facing docs (`_docs/cli/decloud-deploy-service.md`) and in the godoc on `Capturer.Capture`.

### 3.4 The `internal/envcap/capture.go` implementation

```go
package envcap

import (
    "bytes"
    "context"
    "fmt"
    "os/exec"
    "regexp"
)

var validVarName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Capturer extracts the environment variables that a bash script sets when sourced.
type Capturer interface {
    // Capture sources scriptPath in a hermetic bash subshell and returns the
    // resulting environment minus everything the empty-script baseline already
    // contained. See package doc for the long tail of edge cases.
    Capture(ctx context.Context, scriptPath string) (map[string]string, error)
}

// Hardcoded values used to build a hermetic bash environment. Both macOS and
// Linux have /usr/bin/env and /bin/bash; the PATH is the standard FHS PATH plus
// /usr/local/{sbin,bin}; HOME is /tmp because we never want operator dotfiles
// involved (we pass --noprofile --norc anyway, but defense in depth).
const (
    bashPath = "/bin/bash"
    envPath  = "/usr/bin/env"
    seedPATH = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
    seedHOME = "/tmp"
)

// captureScript is what runs inside the hermetic bash. With scriptPath empty
// (the baseline), we skip the source line and just emit the env. With a script,
// we set -a so unexported assignments are picked up, source it, and emit.
//
// compgen -e enumerates exported variable names (bash builtin, no GNU dep).
// printf '%s=%s\0' uses bash's printf builtin to emit NUL-separated records.
// ${!name} is bash indirect expansion; bash 2+, works on macOS bash 3.2.
const captureScript = `
if [ -n "$1" ]; then
    set -a
    source "$1"
fi
while IFS= read -r __decloud_name; do
    printf '%s=%s\0' "$__decloud_name" "${!__decloud_name}"
done < <(compgen -e)
`

type bashCapturer struct{}

func New() Capturer { return &bashCapturer{} }

func (b *bashCapturer) Capture(ctx context.Context, scriptPath string) (map[string]string, error) {
    baseline, err := b.run(ctx, "")
    if err != nil {
        return nil, fmt.Errorf("envcap: baseline bash failed: %w", err)
    }
    full, err := b.run(ctx, scriptPath)
    if err != nil {
        return nil, fmt.Errorf("envcap: sourcing %s failed: %w", scriptPath, err)
    }
    out := make(map[string]string, len(full))
    for k, v := range full {
        if bv, ok := baseline[k]; ok && bv == v {
            continue
        }
        if !validVarName.MatchString(k) {
            continue // bash internals like BASH_FUNC_foo%% get filtered here
        }
        out[k] = v
    }
    return out, nil
}

func (b *bashCapturer) run(ctx context.Context, scriptPath string) (map[string]string, error) {
    args := []string{
        "-i",
        "PATH=" + seedPATH,
        "HOME=" + seedHOME,
        bashPath, "--noprofile", "--norc", "-c", captureScript, "_", scriptPath,
    }
    cmd := exec.CommandContext(ctx, envPath, args...)
    var stdout, stderr bytes.Buffer
    cmd.Stdout = &stdout
    cmd.Stderr = &stderr
    if err := cmd.Run(); err != nil {
        return nil, fmt.Errorf("%w: stderr=%q", err, stderr.String())
    }
    return parseNULEnv(stdout.Bytes()), nil
}

func parseNULEnv(b []byte) map[string]string {
    out := make(map[string]string)
    for _, rec := range bytes.Split(b, []byte{0}) {
        if len(rec) == 0 {
            continue
        }
        eq := bytes.IndexByte(rec, '=')
        if eq <= 0 {
            continue
        }
        out[string(rec[:eq])] = string(rec[eq+1:])
    }
    return out
}
```

Key change vs v1: the embedded bash uses `compgen -e` + `printf '%s=%s\0'`. No `env -0`. Same hermetic wrapper. Same baseline-diff. The `_ scriptPath` trailing args set `$1`; when `scriptPath == ""`, the `if [ -n "$1" ]` guard skips sourcing — same script for baseline and full runs.

### 3.5 Test plan for envcap

`internal/envcap/capture_test.go` runs against real `/bin/bash`. No build-tag skip. Required cases:

| Case | Asserts |
|---|---|
| `export FOO=bar` | `map["FOO"] == "bar"` |
| `FOO=bar` (no export) | `map["FOO"] == "bar"` (set -a captures) |
| Multiline value (PEM-style) | newlines preserved byte-exact |
| Unicode value (`héllo wörld`) | UTF-8 round-trip |
| Value containing `=` (`KEY=a=b=c`) | first `=` is the delimiter; rest is value |
| Empty value (`export EMPTY=`) | `map["EMPTY"] == ""` |
| `unset PATH` inside script | no PATH in output (it's in baseline; full has it absent; not in output map) |
| Script defines a function (`foo() { :; }`) | function name not in output (filtered by `validVarName`) |
| `set -e; false` mid-script | `Capture` returns error wrapping stderr |
| `ctx.Done()` before script completes | `Capture` returns `context.DeadlineExceeded` (or canceled) |
| Script that doesn't exist | `Capture` returns error mentioning path |

CI matrix: Linux + macOS both run `go test ./internal/envcap/...` on every PR. **No build tag skipping** — the whole point of the v2 change is that the test runs on both.

---

## 4. Two-file persistence layer

### 4.1 The split

Per service `<name>`:

- `/opt/declouding/config/services/<name>.toml` — root:root, mode **0644**, world-readable. Holds operational metadata (build, run, routes, readiness, state). Safe to inspect, safe to git-mirror in the future.
- `/opt/declouding/secrets/<name>/env.toml` — root:root, mode **0600**, in a directory at mode **0700**. Holds the captured env from `env.sh`. Never world-readable.

Both files declare the same `schema_version`; the loader rejects mismatches.

### 4.2 The types

`internal/registry/types.go`:

```go
package registry

const CurrentSchemaVersion = 1

// ServiceConfig is the on-disk non-secret half. Persisted at
// /opt/declouding/config/services/<Name>.toml, mode 0644.
type ServiceConfig struct {
    SchemaVersion int    `toml:"schema_version"`
    Name          string `toml:"name"`

    Source    SourceSpec    `toml:"source"`
    Build     BuildSpec     `toml:"build"`
    Run       RunSpec       `toml:"run"`
    Routes    []Route       `toml:"routes"`
    Strategy  string        `toml:"strategy"`  // "recreate" only in M1
    Readiness ReadinessSpec `toml:"readiness"`

    State ServiceState `toml:"state"`
}

// ServiceSecrets is the on-disk secret half. Persisted at
// /opt/declouding/secrets/<Name>/env.toml, mode 0600 in a 0700 dir.
type ServiceSecrets struct {
    SchemaVersion int               `toml:"schema_version"`
    Name          string            `toml:"name"` // sanity-check match with config
    Env           map[string]string `toml:"env"`
}

// Service is the merged in-memory view. Never persisted directly; always
// split into ServiceConfig + ServiceSecrets at write time.
type Service struct {
    Config  ServiceConfig
    Secrets ServiceSecrets
}

type SourceSpec struct {
    Dir string `toml:"dir"` // absolute path the operator deployed from
}

type BuildSpec struct {
    Dockerfile string `toml:"dockerfile"`
    ImageRef   string `toml:"image_ref"`
}

type RunSpec struct {
    Network string  `toml:"network"`
    Port    int     `toml:"port"`
    Restart string  `toml:"restart"`
    Mounts  []Mount `toml:"mounts"` // RESERVED for M3; loader rejects non-empty in M1
}

type Mount struct {
    HostPath      string `toml:"host_path"`
    ContainerPath string `toml:"container_path"`
    ReadOnly      bool   `toml:"read_only"`
}

type Route struct {
    Hostname string `toml:"hostname"`
}

type ReadinessSpec struct {
    Kind         string `toml:"kind"`          // "http" | "healthcheck"
    HTTPPath     string `toml:"http_path"`
    TimeoutSecs  int    `toml:"timeout_secs"`
    IntervalSecs int    `toml:"interval_secs"`
}

type ServiceState struct {
    LastDeployID   string `toml:"last_deploy_id"`
    BuiltImageID   string `toml:"built_image_id"`
    ContainerID    string `toml:"container_id"`
    ContainerName  string `toml:"container_name"`
    LastDeployedAt string `toml:"last_deployed_at"`
    LastDeployedBy string `toml:"last_deployed_by"`
}
```

`Env` lives **only** in `ServiceSecrets`. `ServiceConfig` has no `env` field; the strict-mode loader will reject any config TOML containing one.

### 4.3 The Store interface

`internal/registry/store.go`:

```go
package registry

import "context"

// Store is the persistence layer for service registrations. fsStore is the
// only implementation in M1; mocks are generated for tests.
type Store interface {
    // Load reads both files for the named service and returns the merged Service.
    // Errors:
    //   - ErrNotFound if the config file is absent.
    //   - ErrSecretsMissing if config exists but secrets do not.
    //   - ErrPermissionMode if secrets file is not 0600 or its dir is not 0700.
    //   - ErrSchemaMismatch if schema_version is unsupported or differs between files.
    //   - ErrUnknownField if either file contains a field not in the struct.
    //   - ErrMountsNotSupported if config.Run.Mounts is non-empty.
    //   - ErrInvalidStrategy if config.Strategy is not "recreate" (M1).
    Load(ctx context.Context, name string) (*Service, error)

    // Save writes both files atomically (each via tmp+rename). On create the
    // ORDER is: config first, then secrets. See §4.5 for the justification.
    Save(ctx context.Context, svc *Service) error

    // List returns all loadable services. Files that fail to load are skipped
    // with a warning logged (so one bad file doesn't kill the inventory).
    List(ctx context.Context) ([]*Service, error)

    // Delete removes both files. ORDER: secrets first, then config. See §4.5.
    Delete(ctx context.Context, name string) error
}
```

### 4.4 Permission enforcement on Load

```go
// Inside fsStore.Load, after locating secretsPath:
info, err := os.Stat(secretsPath)
if err != nil {
    if errors.Is(err, fs.ErrNotExist) {
        return nil, ErrSecretsMissing
    }
    return nil, fmt.Errorf("registry: stat secrets: %w", err)
}
if mode := info.Mode().Perm(); mode != 0o600 {
    return nil, fmt.Errorf("%w: %s has mode %#o, expected 0600", ErrPermissionMode, secretsPath, mode)
}
dirInfo, err := os.Stat(filepath.Dir(secretsPath))
if err != nil {
    return nil, fmt.Errorf("registry: stat secrets dir: %w", err)
}
if mode := dirInfo.Mode().Perm(); mode != 0o700 {
    return nil, fmt.Errorf("%w: %s has mode %#o, expected 0700", ErrPermissionMode, filepath.Dir(secretsPath), mode)
}
```

`Load` does NOT silently fix permissions. If the secrets file is mode 0644, we fail loudly — silently fixing hides whatever process broke them, which is exactly the audit signal we want surfaced.

### 4.5 Atomic write via tmp + rename, per file

Each file:

```go
func writeAtomic(path string, mode fs.FileMode, data []byte) error {
    dir := filepath.Dir(path)
    tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
    if err != nil {
        return err
    }
    tmpPath := tmp.Name()
    defer os.Remove(tmpPath) // no-op if rename succeeded
    if _, err := tmp.Write(data); err != nil {
        tmp.Close()
        return err
    }
    if err := tmp.Chmod(mode); err != nil {
        tmp.Close()
        return err
    }
    if err := tmp.Sync(); err != nil {
        tmp.Close()
        return err
    }
    if err := tmp.Close(); err != nil {
        return err
    }
    return os.Rename(tmpPath, path)
}
```

`os.CreateTemp` in the same directory guarantees the rename is on the same filesystem (POSIX-atomic). `Chmod` after creation defends against umask interfering with the file mode we want (the secrets file MUST end up 0600 regardless of umask). The `defer os.Remove` cleans up the tmp on any error path.

**Two-file write is NOT cross-atomic.** A crash between the two renames produces an inconsistent state. The choices are:

- **On create:** Save writes config first, then secrets. If the crash happens between, we have config-without-secrets. The next `Load` returns `ErrSecretsMissing`, the deployer surfaces "service registration is incomplete; re-run `decloud deploy service` to recover" with `ExitConfigError`. The container is not yet running (Save runs after deploy succeeds in §6 step 7), so there is no orphaned runtime state — just a bad file pair that the operator fixes by retrying.
- **On update:** same order. Secrets-without-config would be much worse: a stray secrets file with no way to find it from the registration, and the loader's `List` skips it. Config-without-secrets is recoverable; secrets-without-config is an orphan.
- **On delete:** secrets first, then config. If we crash between, we have config-without-secrets, which the loader detects on next read. If we deleted config first, a crash would leave secrets-without-config — orphaned secret data with no registration pointing at it. Don explicitly called this rule in plan-v2 §3.3 and his reasoning is correct: prefer recoverable inconsistency over orphan-secret inconsistency.

**Recoverable state contract (Don's terminology):** "config without secrets" is a recoverable state. The operator runs `decloud deploy service ...` again; the deployer detects `ErrSecretsMissing` from `Store.Load`, treats it as if the service did not exist (or more precisely: re-captures env, re-writes both files), and proceeds. We document this in `_docs/architecture/secrets-layout.md`. The unit test `TestStore_LoadConfigWithoutSecretsReturnsErrSecretsMissing` asserts the exact behavior.

### 4.6 Save order on create — justified

Per §4.5: **config first, then secrets.** Reasoning:

- If config write succeeds and secrets write fails (or the process crashes between), we have a config file pointing at a service whose secrets we lost. The loader returns `ErrSecretsMissing`. The deployer treats this as "registration incomplete," asks the operator to re-run, and on the re-run captures `env.sh` afresh — secrets are recovered from source.
- If we wrote secrets first and config failed, we have an orphan secrets file. `List` skips it (no matching config). The operator runs `decloud unregister foo` — fails because no config exists. The orphan secret sits there until the operator manually deletes it. Worse outcome for the same crash window.

The deployer's step ordering (§6) writes the registry only after the new container is healthy, so a crash during `Save` does not strand a running container relative to a missing registration — the container is up, the registration is incomplete, the operator re-runs and we converge.

### 4.7 Delete order — secrets first, justified

Per Don §3.3 of plan-v2: **secrets first, then config.** Same reasoning inverted: a crash mid-delete leaves config-without-secrets (recoverable, loader surfaces it cleanly) rather than secrets-without-config (orphan).

The full sequence:

1. `os.Remove(secretsPath)` — secrets file gone.
2. `os.Remove(secretsDir)` — best-effort `rmdir`; ignore "directory not empty" because M3 will leave secret files.
3. `os.Remove(configPath)` — config file gone.

If step 1 fails with `fs.ErrNotExist`, that's fine — proceed. If step 1 fails with permission denied, abort and surface — we should not silently delete config while leaving secrets behind.

### 4.8 Loader rejects non-empty `mounts` (the hand-edit loophole)

In `Load`, after unmarshalling the config TOML:

```go
if len(cfg.Run.Mounts) > 0 {
    return nil, fmt.Errorf("%w: service %q declares %d mount(s); mounts are not supported until M3",
        ErrMountsNotSupported, cfg.Name, len(cfg.Run.Mounts))
}
```

`ErrMountsNotSupported` maps to exit code `ExitConfigError` (10) in `cli.ExitCodeFor`. The error message is verbatim what the operator sees on stderr. Same exit code as the CLI's `--mount` rejection so scripts can pattern-match.

### 4.9 TOML library configuration

```go
import "github.com/pelletier/go-toml/v2"

dec := toml.NewDecoder(file)
dec.DisallowUnknownFields()
if err := dec.Decode(&cfg); err != nil {
    var derr *toml.StrictMissingError
    if errors.As(err, &derr) {
        return nil, fmt.Errorf("%w: %s", ErrUnknownField, derr.String())
    }
    return nil, fmt.Errorf("registry: parse %s: %w", path, err)
}
```

`DisallowUnknownFields()` is the §5 forward-compat backstop. An old binary reading a future-schema file fails loudly with a structured error pointing at the offending field.

---

## 5. `schema_version` policy (Don's call: stays at 1)

### 5.1 The rule

- M1 writes `schema_version = 1`.
- M3 writes `schema_version = 1`. M3 only populates fields M1 reserved (`Mounts`, future secret-file declarations under `mounts`); the schema *shape* is unchanged.
- The version bumps to 2 only when a field's *meaning* changes in a way an older loader can't handle — for example, if M5 wanted to repurpose the `routes` field to mean something different than M1's hostname list. We do not pre-emptively bump.

### 5.2 What the loader does on mismatch

```go
if cfg.SchemaVersion != CurrentSchemaVersion {
    return nil, fmt.Errorf("%w: file declares version %d, this binary supports %d; upgrade or downgrade decloud to match",
        ErrSchemaMismatch, cfg.SchemaVersion, CurrentSchemaVersion)
}
if secrets.SchemaVersion != cfg.SchemaVersion {
    return nil, fmt.Errorf("%w: config has version %d but secrets has version %d",
        ErrSchemaMismatch, cfg.SchemaVersion, secrets.SchemaVersion)
}
```

`ErrSchemaMismatch` -> `ExitConfigError` (10). Operator sees a clear actionable message.

### 5.3 Strict mode is the forward-compat backstop

`DisallowUnknownFields()` (per §4.9) catches the case where a future binary writes a new field that the M1 binary doesn't know about. M1 binary refuses to load with a structured "unknown field `<name>` in config TOML" error. The operator either upgrades `decloud` (if they're on an old binary trying to read a new file) or rolls back (if they downgraded and forgot). Either way they get a clear signal.

### 5.4 If the schema is genuinely wrong in M1

Per Don §5.4: if Kent or Rob discovers during M1 implementation that the schema cannot work and needs a v2, **stop and bring it back to plan**. We do not silently introduce migration code mid-milestone. A schema bump in M1 means the design was wrong; we re-plan.

---

## 6. The `decloud deploy service` command surface for M1

### 6.1 Synopsis

```
decloud deploy service [flags] <source-dir>
```

### 6.2 Flags

| Flag | Type | Required | Default | Notes |
|---|---|---|---|---|
| `--name` | string | yes | — | `[a-z][a-z0-9-]{0,38}`. Used as TOML filename, container-name, image-tag prefix. |
| `--host` | stringSlice | no | none | Public hostname(s) for Caddy. Repeatable. Zero hosts => no Caddy route. |
| `--port` | int | yes if `--host` set | 0 | Container's listen port. |
| `--env-file` | string | no | `<source-dir>/env.sh` if exists | Path to env.sh. Missing default = empty env (no error). |
| `--mount` | stringSlice | no | none | M1: rejected with "mounts not supported until M3". Reserves the flag. |
| `--readiness-path` | string | no | `/healthz` if `--port` set | HTTP path. |
| `--readiness-timeout` | duration | no | `60s` | Total wait. |
| `--strategy` | string | no | `recreate` | Only `recreate` accepted in M1. |
| `--dockerfile` | string | no | `Dockerfile` | Relative to `<source-dir>`. |
| `--config-root` | string | no | `os.Getenv("DECLOUD_ROOT")` else `/opt/declouding` | **No Viper.** Plain Cobra `StringVar` with the default computed from env. |

### 6.3 Positional arg

Exactly one: source dir (absolute or relative). Resolved to absolute at start. Stored in `Source.Dir`.

### 6.4 Exit codes

```go
package cli

const (
    ExitOK              = 0
    ExitUsageError      = 2
    ExitConfigError     = 10
    ExitEnvCaptureFail  = 20
    ExitBuildFail       = 30
    ExitRunFail         = 40
    ExitReadinessFail   = 50
    ExitCaddyReloadFail = 60
    ExitInternal        = 70
)
```

`ExitCodeFor(err error) int` lives in `internal/cli/exit_codes.go`, switches on typed errors via `errors.Is` / `errors.As`. `ErrMountsNotSupported`, `ErrSchemaMismatch`, `ErrPermissionMode`, `ErrSecretsMissing`, `ErrInvalidStrategy`, `ErrUnknownField` all map to `ExitConfigError`.

### 6.5 Stdout/stderr

- `docker build` log: stdout, live-streamed.
- Progress lines (`==> sourcing env.sh`, `==> building image ...`, `==> waiting for readiness`, `==> reloading caddy`, `==> deploy succeeded in 47s`): stderr.
- Errors: stderr.
- Structured `slog` JSON: stderr AND `/opt/declouding/logs/decloud.log` (per §11).

### 6.6 Behavior on partial failure (recreate strategy)

Sequence for redeploy of an existing service:

1. Capture env (`envcap.Capture`). Fail = nothing changed.
2. `docker build` new image with new tag. Fail = nothing changed.
3. Stop the old container (SIGTERM, 10s grace, SIGKILL). **Downtime starts here.**
4. `docker rm` old container.
5. `docker run` new container.
6. Wait for readiness probe.
7. **`Store.Save`** — write config first, secrets second. Atomic per file; not cross-atomic.
8. Regenerate Caddyfile from all current registrations; if the file does not exist (first deploy ever), seed with the stub from §7.2; `caddy reload`.

Failure handling per step is unchanged from v1's §4.6 except for:

- **Step 7 has a new failure mode (between the two file renames).** If config wrote successfully and secrets failed: stop the new container, remove it, attempt to restart the previous container from the in-memory previous `BuildSpec.ImageRef`, exit `ExitInternal` with "registry write failed mid-write; deploy rolled back; re-run to retry." We also delete the config file we just wrote so we don't leave a config-without-secrets orphan from a failed deploy attempt (vs a successful initial create where we actively want config-without-secrets to be a recoverable signal).
- **Step 8 stub creation:** if the Caddyfile didn't exist before this deploy, the deployer writes the stub (per §7.2) before regenerating. This is a one-time op; subsequent deploys see a real file.

**Container naming in M1:** still `decloud-<name>` (no deploy-id suffix). M4's tech plan owns the rename migration as an explicit deliverable (per Don plan-v2 §9).

---

## 7. Caddyfile generator + first-deploy stub

### 7.1 Generator (unchanged shape from v1)

`internal/caddy/generator.go`:

```go
package caddy

import (
    "context"
    "io"
)

type Generator interface {
    Generate(w io.Writer, services []GeneratorInput) error
}

type GeneratorInput struct {
    ServiceName   string
    ContainerName string
    Port          int
    Hostnames     []string
}

type Reloader interface {
    Reload(ctx context.Context, path string) error
}
```

Template (deterministic; sort services by name, sort hostnames within a service, drop services with zero hostnames):

```caddyfile
# Generated by decloud. Do not edit.
{{range .}}
{{range .Hostnames}}{{.}} {{end}}{
    reverse_proxy {{.ContainerName}}:{{.Port}}
}
{{end}}
```

### 7.2 The first-deploy stub

Before `caddy reload`, the deployer calls `caddy.WriteStubIfMissing(caddyfilePath)`. If the file already exists, no-op. If not, write this:

```caddyfile
# Generated by decloud on first deploy.
# This stub exists so `caddy run --config <this file>` does not fail before
# any services are registered. Once services are deployed it is overwritten
# with real reverse_proxy directives by the same deployer.

:80 {
    respond "decloud: no services registered yet" 404
}
```

Bytes-exact (modulo trailing newline). Linus's review specifically asked for an explicit no-op stanza vs an empty file because Caddy's behavior on an empty config has historically been "accept but warn" and we don't want a confusing first run. The `:80` listener with a 404 responder is unambiguously valid Caddyfile syntax, accepted by all Caddy v2 versions, and gives the operator a clear "the system is alive but I haven't deployed anything yet" signal if they `curl http://host/`.

### 7.3 Atomicity of write-then-reload

The Caddyfile write goes through the same `writeAtomic` helper as the registry files (`<path>.tmp-XXXX` -> `os.Rename`). After the rename succeeds, we invoke `caddy reload --config <path>` via `exec.CommandContext`. Sequence:

1. Compute new Caddyfile bytes.
2. `writeAtomic(caddyfilePath, 0o644, newBytes)`.
3. `caddy reload --config <caddyfilePath>`.

If step 3 fails (Caddy syntax-rejected the file, or Caddy isn't running), the file is already on disk — Caddy will pick it up on next start. Per §6.6 step 8 failure handling, a reload failure logs a warning and exits `ExitCaddyReloadFail` but does not roll back the deploy (the new container is healthy and Docker DNS is already routing to it).

### 7.4 Empty-state test

Per Linus's smaller-issues list: an explicit unit test verifies that `Generator.Generate` with an empty input slice produces a Caddyfile that Caddy accepts. We can validate by spawning `caddy validate --config <file>` in the integration test. The stub from §7.2 is what gets used when the registry has no services, so this test exercises the same code path as a fresh install.

---

## 8. Loader rejection rule for `mounts`

Already specified in §4.8. Restated for the deliverables checklist Linus asked for:

```
Error: registry: service "<name>" declares N mount(s); mounts are not supported until M3
Exit code: 10 (ExitConfigError)
```

The same wording is used by the CLI's `--mount` rejection in `internal/cli/deploy_service.go`:

```
Error: --mount is not supported until M3
Exit code: 10 (ExitConfigError)
```

Identical exit code so operator scripts can detect "mount-related failure" with a single condition.

---

## 9. Updated package layout (Viper deferred)

### 9.1 `internal/config/` exists but does not pull Viper

`internal/config/paths.go`:

```go
package config

import (
    "os"
    "path/filepath"
)

const DefaultRoot = "/opt/declouding"

type Paths struct {
    Root          string
    ConfigDir     string
    ServicesDir   string
    JobsDir       string
    CaddyDir      string
    CaddyfilePath string
    SecretsDir    string
    StateDir      string
    DeploysDir    string
    LogsDir       string
    LogFile       string
}

func NewPaths(root string) Paths {
    if root == "" {
        root = DefaultRoot
    }
    return Paths{
        Root:          root,
        ConfigDir:     filepath.Join(root, "config"),
        ServicesDir:   filepath.Join(root, "config", "services"),
        JobsDir:       filepath.Join(root, "config", "jobs"),
        CaddyDir:      filepath.Join(root, "config", "caddy"),
        CaddyfilePath: filepath.Join(root, "config", "caddy", "Caddyfile"),
        SecretsDir:    filepath.Join(root, "secrets"),
        StateDir:      filepath.Join(root, "state"),
        DeploysDir:    filepath.Join(root, "state", "deploys"),
        LogsDir:       filepath.Join(root, "logs"),
        LogFile:       filepath.Join(root, "logs", "decloud.log"),
    }
}

// RootFromEnv resolves the config root using DECLOUD_ROOT then the default.
// CLI's --config-root flag overrides this if explicitly set.
func RootFromEnv() string {
    if v := os.Getenv("DECLOUD_ROOT"); v != "" {
        return v
    }
    return DefaultRoot
}
```

No Viper. Three lines for the env-var default. M2 will introduce Viper here when `/etc/decloud/config.toml` becomes a real thing for the bootstrap to write.

### 9.2 `internal/cli/` builds Cobra commands directly

`internal/cli/root.go`:

```go
func NewRootCmd() *cobra.Command {
    var configRoot string
    root := &cobra.Command{
        Use:   "decloud",
        Short: "Declouding: a personal-scale platform-as-a-service",
        SilenceUsage: true,
    }
    root.PersistentFlags().StringVar(&configRoot, "config-root", config.RootFromEnv(),
        "root directory for /opt/declouding-style layout (env: DECLOUD_ROOT)")
    deploy := &cobra.Command{Use: "deploy", Short: "Deploy a workload"}
    deploy.AddCommand(newDeployServiceCmd(&configRoot))
    root.AddCommand(deploy)
    root.AddCommand(newUnregisterCmd(&configRoot))
    root.AddCommand(newStartCmd(&configRoot))
    root.AddCommand(newStopCmd(&configRoot))
    root.AddCommand(newRestartCmd(&configRoot))
    root.AddCommand(newStatusCmd(&configRoot))
    root.AddCommand(newLogsCmd(&configRoot))
    root.AddCommand(newCaddyReloadCmd(&configRoot))
    return root
}
```

No `viper.BindPFlag`. No `viper.AutomaticEnv`. Just Cobra's `StringVar` with `config.RootFromEnv()` as the default.

### 9.3 Slog initialization

Lives in `internal/logging/logging.go`. Called once from `cmd/decloud/main.go` before any other code runs:

```go
package logging

import (
    "io"
    "log/slog"
    "os"
    "path/filepath"

    "github.com/alexander-fenster/decloud/internal/config"
)

func Init() error {
    if os.Getenv("DECLOUD_LOG_TO_STDERR_ONLY") == "1" {
        slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))
        return nil
    }
    paths := config.NewPaths(config.RootFromEnv())
    if err := os.MkdirAll(paths.LogsDir, 0o755); err != nil {
        return err
    }
    f, err := os.OpenFile(paths.LogFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
    if err != nil {
        return err
    }
    h := slog.NewJSONHandler(io.MultiWriter(os.Stderr, f), &slog.HandlerOptions{Level: slog.LevelInfo})
    slog.SetDefault(slog.New(h))
    return nil
}
```

JSON format. Tests set `DECLOUD_LOG_TO_STDERR_ONLY=1` so they don't write to the real `/opt/declouding/`. M2's bootstrap installs a logrotate config for the file.

---

## 10. M1 operational deliverables

Per Don plan-v2 §10. These are not "technically interesting" — they are easy to forget, so list them explicitly so Rob doesn't ship without them.

| Deliverable | Owner | Notes |
|---|---|---|
| `go.mod` with `go 1.22` | Rob | Module path `github.com/alexander-fenster/decloud`. Direct deps: `github.com/spf13/cobra`, `github.com/pelletier/go-toml/v2`, `github.com/stretchr/testify`, `go.uber.org/mock`. |
| `LICENSE` | Maintainer | Apache-2.0 default; maintainer overrides if they want MIT. |
| `.github/workflows/test.yml` | Rob | `go test ./...` on push/PR. Linux runner only for M1. macOS runner explicitly added in M2 once Docker-on-Mac story is decided. Integration tests gated to nightly via `-tags integration`. |
| `_docs/cli/decloud-deploy-service.md` | Raymond | Operator-facing reference for the M1 command. |
| `_docs/architecture/m1-recreate-strategy.md` | Raymond | Why M1 is recreate-only; how to know when blue/green lands (M4). |
| `_docs/architecture/secrets-layout.md` | Raymond | The §4 split, why, how to inspect (with `sudo cat`). |
| `_docs/operator/manual-install.md` | Raymond | Manual Docker + Caddy + decloud install steps; the Caddy systemd unit pointing at `/opt/declouding/config/caddy/Caddyfile`; the directory tree to pre-create. |
| `_ai/decisions/m1-scope.md` | Raymond | Summarizes Don's plan-v2. |
| `_ai/decisions/secrets-split.md` | Raymond | Captures §4 + the rejected alternatives. |
| `_ai/decisions/schema-versioning.md` | Raymond | Captures §5. |
| Structured logging via `log/slog` | Rob | Per §9.3. JSON to stderr + `/opt/declouding/logs/decloud.log`. Logrotate is M2. |

---

## 11. M3a / M3b subdivision (restated at the tech level)

Don made M3 a known-fat milestone and committed to subdividing when M3's tech plan is written. From M1's perspective, the only thing that matters is that the M1 abstractions stay shaped right for both halves. Confirming:

- **M3a (server-side: env hardening, mounts, secret files):**
  - The `Mounts` field shape in `RunSpec` is already correct for M3a (just remove the loader's "non-empty rejection" guard and start populating).
  - The `dockerdrv.RunRequest.Mounts` slice is already wired through to the driver (currently always empty); M3a flips one switch.
  - Secret-file declarations: a new `SecretFiles []SecretFileSpec` field added to `ServiceSecrets` (or to `ServiceConfig` if the *paths* are the secret-class data — TBD in M3a's plan). Either way, the two-file split is what makes this clean: secret file *contents* live under `secrets/<name>/files/`, the mount declarations either live in config (paths are operational) or in secrets (paths might leak directory structure). M3a decides; M1 doesn't care.
- **M3b (client binary):**
  - The M1 server-side `decloud deploy service <source-dir>` accepts a positional dir arg. M3b adds `--stdin` which extracts a tar bundle to a tmpdir on the server and runs the M1 logic against that path. The `<source-dir>/env.sh` default discovery applies post-extraction, so the M1 flag shape is forward-compatible.
  - The client binary lives at `./client/` (not `internal/client/` — it's a separate distributable). M1 reserves no path for it; M3b creates the tree.

No M1 deliverable changes because of M3 subdivision. The note exists so the M3 planner knows the abstractions are pre-shaped.

---

## 12. Test plan structure

### 12.1 Unit tests (Testify; mocks via Gomock)

| Package | What's tested | Mocks |
|---|---|---|
| `internal/registry` | TOML round-trip per file; strict mode rejects unknown fields; schema_version mismatch errors; cross-file version mismatch errors; permission mode 0644 on secrets is rejected; secrets dir 0755 is rejected; `ErrSecretsMissing` when config exists but secrets don't (the recoverable state); `ErrMountsNotSupported` on non-empty Mounts; atomic write via tmp+rename (inject fs error mid-write, assert no torn file); Save order on create (config first, then secrets — observed via injected fs hook); Delete order (secrets first); List skips malformed files with a logged warning. | None — uses real `t.TempDir()`. |
| `internal/envcap` | All cases from §3.5. **Runs on macOS and Linux without skip.** Requires real `/bin/bash`. | None. |
| `internal/caddy` | Generator deterministic output (sort by service name, sort hostnames within a service, drop zero-hostname services); `WriteStubIfMissing` writes the §7.2 stub byte-exact; `WriteStubIfMissing` is no-op when file exists; reverse-proxy line correct for one-host one-port; multi-host emits hostnames space-separated. | None. |
| `internal/dockerdrv` | Argument-construction tests: Build, Run, Stop, Remove, Inspect, Logs produce the expected `exec.Cmd` args. We expose a hook for the `exec.Command` factory and assert. | None — no actual docker call. |
| `internal/deploy` | Full orchestration: happy path; env capture fails (nothing written); build fails (nothing written); old-container stop fails (abort, exit RunFail); new-container run fails (rollback to old image); readiness fails (rollback); registry Save config-succeeds-secrets-fails (rollback container); caddy reload fails (warn, exit CaddyReloadFail, don't rollback). One test per failure branch. | **Yes — Gomock for Store, Capturer, Driver, Generator, Reloader.** |
| `internal/cli` | Cobra wiring: `deploy service` with various flag combos produces the expected `deploy.Request`; `--mount` rejected with `ExitConfigError`; `--strategy=blue_green` rejected with `ExitConfigError`; missing required flags give `ExitUsageError`; `--config-root` honors `DECLOUD_ROOT`. | Yes — mock the deployer. |
| `internal/ids` | Format stability: `NewDeployID()` matches `^[0-9]{8}-[0-9]{6}-[a-f0-9]{6}$`; uniqueness across rapid calls. | None. |
| `internal/logging` | `Init` with `DECLOUD_LOG_TO_STDERR_ONLY=1` does not touch disk; `Init` with default writes to both stderr and the log file. | None. |

### 12.2 Integration tests (build tag `integration`)

| Test | Verifies |
|---|---|
| `internal/dockerdrv/integration_test.go` | Build + run + inspect + logs + stop + remove against real docker. |
| `internal/caddy/integration_test.go` | Generate a Caddyfile, spawn `caddy run --config <file>` in background, HTTP-GET a route, kill caddy. Also: `caddy validate --config <stub-file>` accepts the §7.2 stub. |
| `internal/deploy/integration_test.go` | Full M1 deploy: build a tiny `nginx:alpine`-based service, deploy, curl, unregister. Slowest test; the only one proving M1 works end-to-end. |

CI matrix: PR gate runs unit tests on Linux. Nightly runs unit + integration on Linux. macOS runner introduced in M2. **Unit envcap tests run on both Linux and macOS in CI from day one** (this is the v2 portability fix; we explicitly want to catch regressions to portability before they merge).

### 12.3 Not tested

Per CLAUDE.md item 4 (no change-detector tests):

- No snapshot tests of full Caddyfile output beyond a couple of canonical cases.
- No snapshot tests of TOML output. Round-trip equality is tested instead (stronger).
- No tests of Cobra command descriptions, help text strings, etc.

---

## 13. Risks and gotchas (the v2 list)

### 13.1 Two-file write is not cross-atomic

§4.5 already discusses. Mitigation: the chosen ordering produces "config without secrets" on a crash, which the loader detects and the operator recovers from by re-running the deploy. Tested.

### 13.2 Permission drift on secrets

If something on the host (a misguided cleanup script, an operator running `chmod -R` carelessly) changes `/opt/declouding/secrets/<name>/env.toml` to 0644, the loader refuses to load with `ErrPermissionMode`. The deployer surfaces "secrets file has wrong permissions; refusing to load; fix with `chmod 600 <path> && chmod 700 <dir>` and re-run." We do not silently fix because that hides the audit signal.

### 13.3 Bash version drift

We depend on bash 3.2 features (`compgen -e`, `${!name}`, `set -a`). bash 3.2 ships on macOS by default; bash 5+ on Linux. Both work. If macOS ever drops `/bin/bash` (Apple has been rumored to want to migrate to `/bin/zsh` exclusively), we need to revisit. Document as a known dependency in `_docs/operator/manual-install.md`.

### 13.4 `compgen -e` in restricted bash

If someone replaced `/bin/bash` with `rbash` or some restricted shell that disables `compgen`, capture fails. Vanishingly unlikely on the production host (root operator; we control the install). Document and move on.

### 13.5 Caddy not running when reload is invoked

If the operator's pre-installed Caddy systemd unit isn't running, `caddy reload --config <path>` fails. The deployer surfaces "caddy reload failed; ensure caddy is running; check /opt/declouding/config/caddy/Caddyfile." Exit `ExitCaddyReloadFail`. The new container is up and Docker DNS routes work; only Caddy ingress is degraded. Operator fixes their systemd.

### 13.6 `docker network inspect` not idempotent

Per Linus's review, we dropped the sentinel and just call `docker network inspect decloud >/dev/null 2>&1 || docker network create decloud` every deploy. `docker network create` of an existing network returns non-zero with "network already exists" — but the `||` short-circuit means we only run create if inspect failed. If inspect failed for a non-existence reason (docker daemon down), create will also fail and we exit `ExitRunFail` with the create's stderr. Acceptable.

### 13.7 SIGINT during deploy

Same as v1. The signal-aware `ctx` in `main.go` propagates cancellation. Step 7's two-file write is the only new concern: if SIGINT lands between writing config and writing secrets, we leave config-without-secrets. Per §4.5, that's a recoverable state. Operator re-runs.

---

## 14. Handoff

When Linus approves: Kent writes tests in the package layout from §2.2 against the type signatures in §4 and the behavior in §6 + §12. Rob then implements against Kent's tests and the same specs. The two trickiest pieces remain `internal/envcap/capture.go` (now portable per §3) and `internal/deploy/service.go` (orchestration with the new step-7 two-file failure mode per §6.6). If anyone gets stuck on either, call Knuth.

End of plan.
