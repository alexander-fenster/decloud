# Tech Plan: M1 — Server-side `decloud deploy service` end-to-end

**Author:** Joel Spolsky (implementation planner)
**Status:** Draft for Linus review
**Builds on:** `02-plan.md` (Don Melton)
**Scope:** Implementation specification for M1 only. M2-M7 referenced only where M1 must accommodate them.

---

## 0. Spec-first sanity check

Don's M1 acceptance criteria, restated in functional terms so we cannot drift:

> An operator who is SSH'd into a host that already has Docker, Caddy, and the `decloud` server binary installed — and who has a directory containing a `Dockerfile` and an `env.sh` — can run **one** `decloud deploy service` command, and at the end of that command (a) the container is running on a shared Docker network, (b) `/opt/decloud/config/services/<name>.toml` exists and is parseable by the same binary, (c) `/opt/decloud/config/caddy/Caddyfile` has been regenerated and `caddy reload` has succeeded, (d) `curl https://<host>/` reaches the container with a real Let's Encrypt cert (DNS permitting), and (e) the same binary's `status`, `logs`, `start`, `stop`, `restart`, and `unregister` subcommands operate on that service. Strategy is `recreate` only — no blue/green this milestone. Exit code is 0 on success, non-zero with a specific exit code per failure class on failure (see §5.4).

That is the contract. Everything below serves it.

---

## 1. Module + repository layout

### 1.1 Module path

```
github.com/alexander-fenster/decloud
```

This matches Don's likely intent and the README's example install line (`go install github.com/alexander-fenster/decloud/client@latest`). It also leaves room for the future client subpackage at `./client`.

### 1.2 Directory tree

```
decloud/
  go.mod
  go.sum
  cmd/
    decloud/
      main.go                       # tiny: builds root cobra cmd, calls Execute
  internal/
    cli/                            # cobra command wiring; one file per subcommand
      root.go                       # NewRootCmd(), global flags
      deploy_service.go             # `decloud deploy service`
      unregister.go
      start.go
      stop.go
      restart.go
      status.go
      logs.go
      caddy_reload.go
    config/                         # process-level config (paths, log level)
      paths.go                      # default /opt/decloud/* paths, overridable
      config.go                     # Viper-backed loader for /etc/decloud/config.toml
    registry/                       # service registration TOML I/O + state
      types.go                      # ServiceSpec, ServiceState, schema_version
      store.go                      # Load/Save/List/Delete; atomic writes
      store_test.go
    envcap/                         # env.sh capture
      capture.go                    # Capture(ctx, path) -> map[string]string, error
      capture_test.go
    caddy/                          # Caddyfile generation + reload
      generator.go                  # Generator interface + textTemplateGenerator
      generator_test.go
      reloader.go                   # Reloader interface + caddyCLIReloader
    dockerdrv/                      # docker CLI driver (named to avoid stdlib clash)
      driver.go                     # Driver interface
      cli_driver.go                 # exec.Command("docker", ...) implementation
      cli_driver_test.go
    deploy/                         # the orchestration: build -> run -> reload
      service.go                    # ServiceDeployer.Deploy(ctx, req) -> result
      service_test.go               # uses mocks for all four sub-interfaces
      readiness.go                  # HTTP probe + HEALTHCHECK polling
      readiness_test.go
    ids/                            # deterministic IDs (deploy ID, container name)
      ids.go
  _docs/                            # API docs (Raymond)
  _ai/                              # AI docs (Raymond)
  _tasks/                           # planning artefacts
```

**Why these splits, not Don's "opening bid" of `internal/{registry,caddy,docker,deploy}` only:**

- `internal/cli/` — Cobra wiring is plumbing. Keeping it out of `cmd/decloud/main.go` makes `main.go` 10 lines and lets us test command construction. Don's plan didn't address where commands live; this is the obvious place.
- `internal/envcap/` — env.sh capture is non-trivial enough (see §3) and reused by `decloud deploy service` and any future redeploy/reload paths. Lives on its own so it can be tested in isolation against a real bash.
- `internal/dockerdrv/` (not `internal/docker/`) — `docker` is a common identifier and we may import `github.com/docker/docker` SDK material in M4+. Avoid the name clash now.
- `internal/ids/` — deterministic generation of deploy IDs (e.g. `20260426-153012-ab12cd`) and container names (`decloud-<service>-<deploy-id>`). Tiny but central, and the deterministic format will matter when M4 needs to find "the old container" to kill.
- `internal/config/` — separate from `internal/registry/` because process-level config (where is `/opt/decloud/`?) is not service registration data.

The package boundary that matters most for M1 is **`internal/deploy/` depends only on the four interfaces** (`registry.Store`, `envcap.Capturer`, `caddy.Generator` + `caddy.Reloader`, `dockerdrv.Driver`). That is what makes it testable with Gomock-generated mocks.

### 1.3 `cmd/decloud/main.go`

```go
package main

import (
    "context"
    "fmt"
    "os"
    "os/signal"
    "syscall"

    "github.com/alexander-fenster/decloud/internal/cli"
)

func main() {
    ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
    defer stop()

    if err := cli.NewRootCmd().ExecuteContext(ctx); err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(cli.ExitCodeFor(err))
    }
}
```

That is it. All real wiring lives in `internal/cli/root.go`.

---

## 2. Library choices

### 2.1 CLI — Cobra

Mandated by CLAUDE.md. Use `github.com/spf13/cobra` latest (v1.8+). `RunE`-style commands so errors propagate to `main` for exit-code mapping.

### 2.2 Process config — Viper

Mandated by CLAUDE.md for "YAML configuration." We need very little process-level config in M1 (just an override for the `/opt/decloud/` root, useful for tests), but wire it now so M2+ does not retrofit. Viper reads `/etc/decloud/config.toml` if present, env vars `DECLOUD_*`, and CLI flags, in that precedence. **Viper is for process config only — service registrations are read directly via the TOML lib (see §2.3) because Viper's merging semantics are wrong for "many independent files."**

### 2.3 TOML — `github.com/pelletier/go-toml/v2`

**Pick: pelletier/go-toml v2.** Reasoning vs `BurntSushi/toml`:

| Concern | pelletier v2 | BurntSushi |
|---|---|---|
| TOML 1.0.0 spec compliance | Yes | Yes |
| Performance (encode/decode) | Faster (~3-5x in published benchmarks) | Slower |
| Strict mode (unknown fields error out) | `decoder.DisallowUnknownFields()` — first-class | Available via `MetaData.Undecoded()` post-hoc |
| Maintenance velocity (2025-2026) | Active | Slower |
| API ergonomics | Closer to `encoding/json` | Idiosyncratic |

The **strict-mode** point is what tips it. We need `DisallowUnknownFields()` on by default for the registry loader so that a stale binary reading a newer-schema file fails loudly (combined with `schema_version`, this is belt-and-braces). pelletier makes that one line.

Viper itself uses pelletier under the hood as of v1.18, so we are not adding a transitive dep — we are using directly what is already pulled in.

### 2.4 Testing — Testify + Gomock (uber-go/mock)

Mandated by CLAUDE.md. Gomock for the four interfaces in §3.4 (`registry.Store`, `envcap.Capturer`, `caddy.Generator`, `caddy.Reloader`, `dockerdrv.Driver`). Generate via `//go:generate mockgen` directives at the top of each interface file; commit the generated mocks under `internal/<pkg>/mocks/` so CI does not need `mockgen` installed to run tests.

### 2.5 Invoking `docker` and `caddy` — `os/exec` only for M1

Per Don. Both have stable CLIs; the docker SDK is heavyweight (`github.com/docker/docker` pulls in ~hundreds of packages including containerd, swarm, etc.) and the M1 surface — `docker build`, `docker run`, `docker stop`, `docker rm`, `docker ps`, `docker inspect`, `docker logs` — is comfortably handled by `exec.CommandContext`. Pipe stdout/stderr through; capture exit code; map to typed errors in `dockerdrv/cli_driver.go`. Caddy is just `caddy reload --config <path>` and (later, M4) HTTP calls to `http://localhost:2019/`.

**Future-proofing note:** the `dockerdrv.Driver` interface (§3.4) is shaped so an SDK-backed implementation can drop in later without touching `internal/deploy/`. We do not implement it now. That is a clean abstraction boundary, not speculative generality — we are committing to the interface, not to the second implementation.

### 2.6 Caddy admin API — note for M4 only

Caddy exposes `http://localhost:2019/` by default. M4 will PATCH `/config/apps/http/servers/.../routes/N/handle/0/upstreams` to swap upstreams atomically. We do **not** open this surface in M1; we only run `caddy reload --config <path>`. Document the M4 plan in `internal/caddy/reloader.go` as a comment so the next person doesn't reinvent it.

---

## 3. Type definitions

### 3.1 The TOML registry schema (`internal/registry/types.go`)

```go
package registry

// CurrentSchemaVersion is the schema version this binary writes.
// Loader rejects files with SchemaVersion != CurrentSchemaVersion in M1.
// M3 will add a migration path.
const CurrentSchemaVersion = 1

// ServiceSpec is the on-disk registration for one service.
// Persisted at /opt/decloud/config/services/<Name>.toml.
type ServiceSpec struct {
    SchemaVersion int    `toml:"schema_version"`
    Name          string `toml:"name"`

    Source SourceSpec `toml:"source"`
    Build  BuildSpec  `toml:"build"`
    Run    RunSpec    `toml:"run"`
    Routes []Route    `toml:"routes"`

    Strategy string `toml:"strategy"` // "recreate" only in M1; "blue_green" in M4

    Readiness ReadinessSpec `toml:"readiness"`

    Env map[string]string `toml:"env"` // captured from env.sh; stored verbatim

    State ServiceState `toml:"state"`
}

type SourceSpec struct {
    // Absolute path to the directory the operator deployed from.
    // Used for redeploys (M3+) and for the operator to find their source.
    Dir string `toml:"dir"`
}

type BuildSpec struct {
    Dockerfile string `toml:"dockerfile"` // relative to Source.Dir; default "Dockerfile"
    ImageRef   string `toml:"image_ref"`  // e.g. "decloud-foo:20260426-153012-ab12cd"
}

type RunSpec struct {
    Network string   `toml:"network"`  // default "decloud"
    Port    int      `toml:"port"`     // container's listen port
    Restart string   `toml:"restart"`  // default "unless-stopped"
    Mounts  []Mount  `toml:"mounts"`   // EMPTY in M1; schema present so M3 doesn't bump version
}

type Mount struct {
    HostPath      string `toml:"host_path"`
    ContainerPath string `toml:"container_path"`
    ReadOnly      bool   `toml:"read_only"`
}

type Route struct {
    Hostname string `toml:"hostname"` // FQDN, e.g. "foo.example.com"
}

type ReadinessSpec struct {
    Kind         string `toml:"kind"`          // "http" or "healthcheck"
    HTTPPath     string `toml:"http_path"`     // e.g. "/healthz"; required if kind=="http"
    TimeoutSecs  int    `toml:"timeout_secs"`  // default 60
    IntervalSecs int    `toml:"interval_secs"` // default 1
}

// ServiceState is the runtime-tracked half. Co-located in the same TOML
// for M1 simplicity (one file per service, atomic write covers both).
// If write contention emerges later, split into <name>.toml + <name>.state.toml.
type ServiceState struct {
    LastDeployID    string `toml:"last_deploy_id"`     // matches BuildSpec.ImageRef tag
    BuiltImageID    string `toml:"built_image_id"`     // sha256:... from `docker inspect`
    ContainerID     string `toml:"container_id"`       // long ID from `docker run`
    ContainerName   string `toml:"container_name"`     // "decloud-<name>-<deploy-id>"
    LastDeployedAt  string `toml:"last_deployed_at"`   // RFC3339 UTC
    LastDeployedBy  string `toml:"last_deployed_by"`   // os.Getenv("USER") on host at deploy time
}
```

**Why mounts and env are in the schema even though M1 doesn't fully exercise mounts:** Don's plan calls for `schema_version` to stay at 1 through M1 and bump at M3. Reserving the field shape now means M3 doesn't bump the version — it just starts populating empty fields. That preserves "M1-era files load in M3 binary" for free.

**Strategy validation:** in M1 the loader accepts `"recreate"` and rejects everything else with a clear error. M4 adds `"blue_green"`. Setting the default in code (not in the file) — if the file has empty `strategy`, treat as `"recreate"` and rewrite on next save.

### 3.2 The Caddyfile generator interface (`internal/caddy/generator.go`)

```go
package caddy

import "io"

// Generator renders a Caddyfile from a set of registered services.
type Generator interface {
    // Generate writes a complete Caddyfile to w covering all routes
    // declared by the input specs. Implementation must be deterministic:
    // the same input yields byte-identical output (sorted hostnames, etc.)
    // so that diffs are meaningful and tests are stable.
    Generate(w io.Writer, services []GeneratorInput) error
}

// GeneratorInput is the slim view of a ServiceSpec the generator needs.
// Decoupled from registry.ServiceSpec so caddy/ doesn't depend on registry/.
type GeneratorInput struct {
    ServiceName   string  // used to derive container DNS name
    ContainerName string  // exact name to put in `reverse_proxy`
    Port          int
    Hostnames     []string
}

// Reloader applies a Caddyfile to a running Caddy.
type Reloader interface {
    // Reload tells Caddy to load the file at path. M1 implementation:
    // exec `caddy reload --config <path> --adapter caddyfile`.
    // Returns nil iff Caddy accepted the new config.
    Reload(ctx context.Context, path string) error
}
```

**Generation strategy:** `text/template` with a tiny template. Caddyfile is line-oriented and our shape is trivial:

```caddyfile
# Generated by decloud. Do not edit.
{{range .}}
{{range .Hostnames}}{{.}} {{end}}{
    reverse_proxy {{.ContainerName}}:{{.Port}}
}
{{end}}
```

Programmatic AST-style builders are overkill for a config we control end-to-end. The template + a sort of inputs by service name gives us deterministic output.

### 3.3 The Docker driver interface (`internal/dockerdrv/driver.go`)

```go
package dockerdrv

import (
    "context"
    "io"
    "time"
)

// Driver is the abstraction over the docker daemon used by the deployer.
// M1 has exactly one implementation: cliDriver, which shells out to `docker`.
// Shaped so a future SDK-backed implementation can replace it without
// touching internal/deploy/.
type Driver interface {
    // Build runs `docker build -t imageRef -f dockerfile contextDir`.
    // stdout/stderr are streamed to the writers (so the operator sees
    // the build log live) and also captured for error reporting.
    Build(ctx context.Context, req BuildRequest, stdout, stderr io.Writer) (BuildResult, error)

    // NetworkEnsure creates the named bridge network if missing. Idempotent.
    NetworkEnsure(ctx context.Context, name string) error

    // Run starts a detached container. Returns the container ID once the
    // daemon has accepted it (NOT once the app inside is ready —
    // readiness is the deployer's job, not the driver's).
    Run(ctx context.Context, req RunRequest) (RunResult, error)

    // Stop sends SIGTERM, waits up to gracePeriod, then SIGKILL.
    Stop(ctx context.Context, containerID string, gracePeriod time.Duration) error

    // Remove removes a stopped container. Returns nil if already gone.
    Remove(ctx context.Context, containerID string) error

    // Inspect returns enough state for `decloud status`.
    Inspect(ctx context.Context, containerID string) (InspectResult, error)

    // Logs returns the container's logs. Tail==0 means "all".
    Logs(ctx context.Context, containerID string, tail int, follow bool, w io.Writer) error
}

type BuildRequest struct {
    ContextDir string            // host path passed as docker's positional arg
    Dockerfile string            // relative to ContextDir; "" == "Dockerfile"
    ImageTag   string            // e.g. "decloud-foo:20260426-153012-ab12cd"
    BuildArgs  map[string]string // M1: empty
}

type BuildResult struct {
    ImageID string // sha256:... from `docker inspect --format '{{.Id}}' <tag>`
}

type RunRequest struct {
    ImageRef      string
    ContainerName string
    Network       string
    Env           map[string]string // injected via repeated -e flags (NOT --env-file; see §3.5)
    Mounts        []Mount           // M1: empty
    RestartPolicy string            // "unless-stopped"
    Labels        map[string]string // includes decloud.service=<name>
}

type Mount struct {
    HostPath      string
    ContainerPath string
    ReadOnly      bool
}

type RunResult struct {
    ContainerID string
}

type InspectResult struct {
    ContainerID string
    State       string // "running", "exited", "created", ...
    StartedAt   time.Time
    ExitCode    int
}
```

### 3.4 Other interfaces (for Gomock)

- `registry.Store`: `Load(name) (ServiceSpec, error)`; `Save(ServiceSpec) error`; `List() ([]ServiceSpec, error)`; `Delete(name) error`. File-backed; uses pelletier/go-toml v2; atomic via `os.WriteFile` to `<name>.toml.tmp` then `os.Rename`.
- `envcap.Capturer`: `Capture(ctx, scriptPath string) (map[string]string, error)`. See §3.5.
- `caddy.Generator` and `caddy.Reloader`: as in §3.2.
- `dockerdrv.Driver`: as in §3.3.

The deployer in `internal/deploy/service.go` takes all four as constructor arguments. **In M1 unit tests, all four are mocked.** The `internal/deploy/service_test.go` exhaustively exercises the orchestration logic without touching disk, docker, caddy, or bash.

### 3.5 `envcap.Capture` mechanism — the critical detail

This is the section Don explicitly asked me to nail. Here is the chosen implementation and the reasoning behind every flag.

**Goal:** Run `env.sh` in a subshell and return a `map[string]string` containing **only** the variables that `env.sh` itself sets — not the variables inherited from the Go process environment, and not the variables bash itself sets internally (`BASH`, `BASH_VERSION`, `PWD`, `SHLVL`, etc.).

**Edge cases to handle:**
1. Variable values containing newlines (e.g. multi-line PEM files). NUL-separated parsing is non-negotiable.
2. Variable values containing arbitrary bytes (UTF-8 text, but bash is byte-transparent — we should be too).
3. Variable *names* with weird characters — bash actually allows only `[A-Za-z_][A-Za-z0-9_]*`, so we filter on that.
4. Variables that bash itself sets (must be excluded so we capture only the user's intent).
5. `env.sh` failing — non-zero exit must propagate as a typed error with stderr captured.
6. Infinite loop in `env.sh` — context with timeout (default 30s, configurable later).
7. `env.sh` doing `unset PATH`, `cd /tmp`, etc. — bash exits cleanly, we don't care; our process is unaffected because it's a subprocess.

**The implementation:**

```go
package envcap

import (
    "bytes"
    "context"
    "fmt"
    "os/exec"
    "regexp"
    "strings"
)

// Capture sources scriptPath in a clean bash subshell and returns the
// resulting environment.
//
// Mechanism:
//   env -i PATH=$PATH HOME=$HOME bash --noprofile --norc -c '
//     set -a
//     source "$1"
//     env -0
//   ' _ <scriptPath>
//
// `env -i` wipes the inherited environment except for the explicitly
// re-introduced variables (PATH and HOME — needed because env.sh almost
// certainly invokes external commands and bash needs HOME for tilde
// expansion).  `--noprofile --norc` keeps user dotfiles out.  `set -a`
// auto-exports every assignment so `FOO=bar` (no `export`) is captured
// just like `export FOO=bar`.  `env -0` prints NUL-separated KEY=VALUE
// records so newlines in values survive.
//
// We then DIFF the captured env against a baseline run of bash with the
// same env -i / --noprofile / --norc but without sourcing the script.
// Anything in the baseline (BASH, PWD, SHLVL, _, PATH, HOME, ...) is
// dropped from the result.  What remains is exactly what the script set.

var validVarName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

type Capturer interface {
    Capture(ctx context.Context, scriptPath string) (map[string]string, error)
}

type bashCapturer struct {
    BashPath string // default "/bin/bash"
}

func New() Capturer { return &bashCapturer{BashPath: "/bin/bash"} }

func (b *bashCapturer) Capture(ctx context.Context, scriptPath string) (map[string]string, error) {
    baseline, err := b.runBash(ctx, "")
    if err != nil {
        return nil, fmt.Errorf("baseline bash failed: %w", err)
    }
    full, err := b.runBash(ctx, scriptPath)
    if err != nil {
        return nil, fmt.Errorf("sourcing %s failed: %w", scriptPath, err)
    }
    out := make(map[string]string, len(full))
    for k, v := range full {
        if bv, ok := baseline[k]; ok && bv == v {
            continue // unchanged from baseline; not the script's doing
        }
        if !validVarName.MatchString(k) {
            continue // bash internal like "BASH_FUNC_foo%%"
        }
        out[k] = v
    }
    return out, nil
}

func (b *bashCapturer) runBash(ctx context.Context, scriptPath string) (map[string]string, error) {
    script := `env -0`
    if scriptPath != "" {
        script = `set -a; source "$1"; env -0`
    }
    args := []string{"--noprofile", "--norc", "-c", script}
    if scriptPath != "" {
        args = append(args, "_", scriptPath) // $0=_, $1=scriptPath
    }
    // env -i wipes; we re-add the bare minimum bash needs to function.
    cmd := exec.CommandContext(ctx, "/usr/bin/env", append([]string{
        "-i",
        "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
        "HOME=/tmp",
        b.BashPath,
    }, args...)...)
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
    _ = strings.TrimSpace // silence unused if linter complains
    return out
}
```

**Verification of Don's `bash -c 'set -a; source env.sh; env -0'` suggestion:** Don's suggestion is **directionally correct but incomplete**. Without `env -i` we capture the inherited environment of the Go process, polluting the result with whatever the operator's SSH session has set (`SSH_AUTH_SOCK`, `LANG`, `TERM`, `LS_COLORS`, ...). Without `--noprofile --norc` we accidentally execute the operator's `~/.bashrc`, which is both a security smell and a reproducibility hazard. The baseline-diff trick is necessary because even `env -i bash --noprofile --norc -c 'env -0'` produces ~7 variables that bash itself adds (`BASH`, `SHLVL`, `_`, `PWD`, `BASH_EXECUTION_STRING`, plus the few we explicitly added: `PATH`, `HOME`).

**Edge cases explicitly handled:**
- **Newlines in values:** survive via NUL separation.
- **Unicode/UTF-8:** byte-transparent through `string([]byte)`. We do not normalize; if the operator wrote raw bytes, raw bytes go to the container.
- **`env.sh` exits non-zero:** `cmd.Run()` returns error; we wrap with stderr.
- **`env.sh` runs forever:** caller's `context.Context` deadline (default 30s in deployer).
- **`env.sh` modifies `$PATH`:** captured, persisted, injected. That's the user's intent.
- **`env.sh` does `unset HOME`:** baseline still has HOME, full does not — diff produces no entry for HOME (we only emit keys present in `full`). Acceptable: if the operator unset it, they meant to.
- **`env.sh` defines a function:** bash exports it with name like `BASH_FUNC_foo%%` — filtered out by `validVarName`.

**One known limitation:** if the operator's `env.sh` re-sets a variable to the same value it already has in the baseline (e.g. `export PATH=$PATH:/opt/foo` happens to equal the baseline PATH), we miss it. This is a vanishingly rare case and the operator workaround is trivial (set a different value). Document this in the godoc.

---

## 4. The exact `decloud deploy service` command surface for M1

### 4.1 Synopsis

```
decloud deploy service [flags] <source-dir>
```

### 4.2 Flags

| Flag | Type | Required | Default | Notes |
|---|---|---|---|---|
| `--name` | string | yes | — | Service name. Validated `[a-z][a-z0-9-]{0,38}`. Used as TOML filename, container-name prefix, image-tag prefix. |
| `--host` | stringSlice | no (zero or more) | none | Public hostname(s) to route through Caddy. Repeatable: `--host a.example.com --host b.example.com`. Zero hosts = service runs but Caddy routes nothing to it (still useful for internal-only or jobs-precursor work). |
| `--port` | int | yes if any `--host` | 0 | Container's internal listen port. Required when `--host` is given. |
| `--env-file` | string | no | `<source-dir>/env.sh` if exists, else none | Path to env.sh. If the default path doesn't exist and no flag is given, env is empty (no error). |
| `--mount` | stringSlice | no | none | M1: declared in schema but **rejected with clear error if non-empty** ("mounts not supported until M3"). Reserves the flag name. |
| `--readiness-path` | string | no | `/healthz` if `--port` set | HTTP path for readiness probe. Implies `kind=http`. |
| `--readiness-timeout` | duration | no | `60s` | Total time to wait for readiness before failing. |
| `--strategy` | string | no | `recreate` | M1 accepts only `recreate`. `blue_green` produces "not supported until M4" error. |
| `--dockerfile` | string | no | `Dockerfile` | Relative to `<source-dir>`. |
| `--config-root` | string | no | `/opt/decloud` | Override for tests; honors `DECLOUD_ROOT` env var. |

### 4.3 Positional argument

Exactly one: the absolute or relative path to the source directory containing the Dockerfile. `.` is fine. The deployer resolves to absolute path at start and stores in `SourceSpec.Dir`.

### 4.4 Exit codes

```go
package cli

const (
    ExitOK             = 0
    ExitUsageError     = 2  // bad flags, missing args
    ExitConfigError    = 10 // /opt/decloud not writable, etc.
    ExitEnvCaptureFail = 20 // env.sh failed to source
    ExitBuildFail      = 30 // docker build returned non-zero
    ExitRunFail        = 40 // docker run returned non-zero
    ExitReadinessFail  = 50 // probe never returned 200 within timeout
    ExitCaddyReloadFail = 60 // caddy reload returned non-zero
    ExitInternal       = 70 // anything else (file I/O, parse errors, etc.)
)
```

`ExitCodeFor(err error) int` in `internal/cli/root.go` switches on typed errors from the various subsystems.

### 4.5 Stdout / stderr behavior

- Build output (docker build log) streams to **stdout** live. This is what the operator most wants to see.
- High-level progress lines ("==> sourcing env.sh", "==> building image decloud-foo:...", "==> waiting for readiness", "==> reloading caddy", "==> deploy succeeded in 47s") go to **stderr** so they don't pollute stdout if anyone ever pipes the tool.
- Error messages go to stderr. No JSON output in M1 (defer until a real consumer exists).

### 4.6 Behavior on partial failure (the recreate strategy)

This is M1's main correctness hazard. The sequence for a redeploy of an existing service:

1. Capture env from `env.sh`. **Fail here = leave everything untouched.** Old container still running.
2. `docker build` new image with new tag. **Fail here = leave everything untouched.** Old container still running, old image still tagged.
3. Stop the old container (SIGTERM, 10s grace, SIGKILL). **From this moment until step 6 succeeds, the service has downtime.** This is acceptable per `strategy=recreate`.
4. `docker rm` the old container.
5. `docker run` the new container.
6. Wait for readiness probe.
7. Write `<name>.toml` with new `BuildSpec` and `ServiceState` (atomic).
8. Regenerate Caddyfile from all current registrations and `caddy reload`.

**If step 5 fails:** old container is gone. Try to restart it from the previous image (we have the old `BuildSpec.ImageRef` in memory from step 1). If that also fails, exit with `ExitRunFail` and leave a clear message: "deploy failed; previous container could not be restarted; service is DOWN; use `decloud deploy service` to retry." This is the worst case in M1 and we accept it as the cost of the recreate strategy. M4 fixes it via blue/green.

**If step 6 fails:** new container is running but unhealthy. Stop and remove it. Try to restart the previous one. Same fallback as above.

**If step 7 fails (TOML write):** new container is running and healthy but unrecorded. Stop it, remove it, restart old. Exit `ExitInternal` with "registry write failed; deploy rolled back." This should be impossible in practice (we tested writability at step 0) but defensive code is mandatory here.

**If step 8 fails (caddy reload):** new container is running, healthy, and recorded. Caddy still routes to... wait, in `recreate` Caddy was already pointing at the container name, which we've reused (we use the same container name in M1's recreate strategy — see §6). So if Caddy was already healthy, it's still routing correctly via Docker DNS to the new container. The reload was attempting to apply *other* changes (e.g. a new hostname). Log warning, exit `ExitCaddyReloadFail`, but the service is up. Operator can investigate.

**Container naming in M1 recreate:** use `decloud-<name>` (no deploy ID suffix) so Caddy's `reverse_proxy decloud-<name>:<port>` keeps working across redeploys without touching Caddy. M4 will switch to `decloud-<name>-<deploy-id>` so blue and green can coexist; the Caddyfile (or admin API) flips between them. This is a small but real divergence between M1 and M4 — flag it in code.

---

## 5. File layout on disk under `/opt/decloud/`

Refining the README's provisional layout to something concrete enough to implement:

```
/opt/decloud/
  config/
    services/
      <name>.toml                # one per service, owner root, mode 0644
    jobs/                         # M5 — exists empty in M1 so backups are simple
    caddy/
      Caddyfile                   # generated by decloud; owner root, mode 0644
                                  # `caddy reload --config /opt/decloud/config/caddy/Caddyfile`
  secrets/
    <name>/                       # M3 — exists empty in M1
                                  # owner root, mode 0700; files inside 0600
  state/
    deploys/
      <name>/
        <deploy-id>/
          source.tar.gz           # the source bundle that built this deploy
                                  # M1: the source dir, tarred, for future redeploys
                                  # one per deploy, kept until pruned (M6)
  logs/
    decloud.log                   # the decloud binary's own structured log
                                  # rotate via logrotate (M2 bootstrap config)
  cache/
    docker-network-created        # sentinel file so we don't NetworkEnsure every deploy
```

**What lives here in M1 specifically:**
- `config/services/<name>.toml` — yes, populated.
- `config/caddy/Caddyfile` — yes, generated.
- `state/deploys/<name>/<deploy-id>/source.tar.gz` — yes; tar of source dir at deploy time (handy for "what did we actually build?" forensics).
- Everything else: directories created with correct permissions, contents empty.

**Permissions:** `/opt/decloud/secrets/` is mode 0700 from day one even though we don't write into it in M1. Setting permissions at directory create time is free; doing it retroactively when M3 lands is a security regression waiting to happen.

**Ownership:** all files owned by root in production. In tests we override `--config-root` to a tmpdir.

---

## 6. Test plan structure

### 6.1 Unit tests (Testify; mocks via Gomock where needed)

| Package | What's tested | Mocks needed? |
|---|---|---|
| `internal/registry` | TOML round-trip; strict mode rejects unknown fields; schema_version mismatch errors; atomic write (rename) survives crash mid-write (simulated via injected fs error); List with malformed file in dir doesn't kill the loader. | None — uses real tmpdir. |
| `internal/envcap` | Real bash subprocess; cases: simple `export FOO=bar`; multiline value; unicode value; `unset` of inherited; `env.sh` with `set -e; false` exits nonzero; context cancellation. | None — needs real bash. Skip on Windows builders via build tag (we only target Linux+macOS for dev). |
| `internal/caddy` | `Generator.Generate` produces deterministic output (sort hostnames, sort services); reverse-proxy line for one service one host; multi-host service emits hostnames space-separated; zero-host service is omitted entirely. | None — pure function. |
| `internal/dockerdrv` | Argument-construction tests: `Build`, `Run`, etc. produce the expected `exec.Cmd` args. We do **not** actually run docker in unit tests. | None — we expose a hook for the `exec.Command` factory and assert on args. |
| `internal/deploy` | The full orchestration: happy path; env capture fails; build fails; run fails; readiness fails (rollback); caddy reload fails (no rollback, warn). One test per failure branch in §4.6. | **Yes — Gomock for all four interfaces.** This is where Gomock pays for itself. |
| `internal/cli` | Cobra wiring: `deploy service` with various flag combos produces the expected `deploy.Request`; bad flags produce `ExitUsageError`. | Yes — mock the deployer. |
| `internal/ids` | Format stability: `NewDeployID()` matches regex `^[0-9]{8}-[0-9]{6}-[a-f0-9]{6}$`; uniqueness across rapid calls (use crypto/rand for the suffix). | None. |

### 6.2 Integration tests (build tag `integration`, gated)

`go test -tags integration ./...` runs these; default `go test ./...` skips. Requires `docker` and `caddy` binaries on PATH; CI provides them.

| Test | What it verifies |
|---|---|
| `internal/dockerdrv/integration_test.go` | Build a tiny inline Dockerfile; run it; inspect; logs; stop; remove. Asserts on the actual side effects. |
| `internal/caddy/integration_test.go` | Generate a Caddyfile; spawn `caddy run --config <file>` in background; HTTP-GET a route; kill caddy. |
| `internal/deploy/integration_test.go` | The full M1 deploy against real docker + real caddy on a real network. Builds a `nginx:alpine`-based service, deploys it, curls the resulting endpoint, then `unregister`s. Slowest test but the only one that proves M1 works end-to-end. |

CI matrix: Linux runner with docker installed runs integration tests; macOS runner runs unit tests only (docker on macOS is a VM and slow). PR gate is unit tests; nightly is unit + integration.

### 6.3 What we do NOT test

Per CLAUDE.md item 4: no change-detector tests. Specifically:
- We don't snapshot-test the Caddyfile output beyond a couple of canonical cases. The deterministic-output unit test plus the integration test that proves caddy accepts it is sufficient.
- We don't snapshot-test the TOML output. We test round-trip equality, which is stronger.
- We don't test that Cobra commands have specific descriptions, that flag descriptions match a fixture, etc.

---

## 7. Risks Don flagged + my resolution

### 7.1 (a) `env.sh` capture mechanism — RESOLVED in §3.5

Don's suggestion needs three additions: `env -i` for hermetic input, `--noprofile --norc` to skip dotfiles, and a baseline-diff to filter out bash internals. Result is the implementation in §3.5.

### 7.2 (b) Package layout — RESOLVED in §1.2

Refined Don's `internal/{registry,caddy,docker,deploy}` to seven packages: added `internal/cli/`, `internal/config/`, `internal/envcap/`, `internal/ids/`, renamed `internal/docker/` to `internal/dockerdrv/`. Justification per package in §1.2.

### 7.3 Caddy admin API vs file (Don's M1 risk)

M1 sidesteps cleanly: only file + `caddy reload`. Document the M4 plan as a comment in `internal/caddy/reloader.go`. **In M4, the file remains source of truth and is regenerated on every change; the admin API is used additionally only for the hot-path upstream swap during a blue/green deploy. After the swap, the Caddyfile is regenerated to match (so a Caddy restart converges).** That dual-write rule is what keeps the file as the unambiguous source of truth.

### 7.4 Atomicity of registration writes — RESOLVED in §3.4

`registry.Store.Save` writes to `<name>.toml.tmp` in the same directory, then `os.Rename`. POSIX-atomic on the same filesystem. **Hidden hazard:** on macOS in dev, `/tmp` may be a different filesystem than the test tmpdir; tests must use `t.TempDir()` (which is on the same fs as the workspace) and not hardcode `/tmp`.

### 7.5 Schema versioning — RESOLVED in §3.1

`SchemaVersion` field at top of every file. Loader rejects mismatches with explicit "this binary writes v1, file is v2 — upgrade decloud" message. M3 will introduce a migration path; for M1 the rule is strict.

### 7.6 New risk surfaced by this plan: docker build context size

If the operator runs `decloud deploy service .` from a large directory (especially with `node_modules/` or `.git/`), `docker build` ships the whole context to the daemon. M1 honors the operator's `.dockerignore` — it's docker's own behavior, not ours. **No `.dockerignore`?** We do not create one for them, but we log a warning if the resolved context size exceeds 100MB. (Cheap to compute via `filepath.WalkDir`; optional for M1, fits in a follow-up.)

### 7.7 New risk: SIGINT during deploy

Operator hits Ctrl-C mid-build. The signal-aware `context.Context` in `main.go` propagates cancellation to `cmd.Process.Signal(os.Interrupt)` for the docker subprocess. We must **not** corrupt the registry — only write the TOML in step 7 (per §4.6), which is an atomic rename. Pre-step-7 cancellation leaves the registry untouched. Step-3-to-step-6 cancellation can leave a stopped old container with no replacement; we accept this as the recreate-strategy cost and exit with a clear message.

---

## 8. Questions for Linus

Stated explicitly so Linus can't miss them in the prose:

**Q1 (load-bearing):** Is the `recreate`-only strategy in M1 a defensible interim, or does it tempt us into shortcuts that prevent a clean M4 transition? My read: the abstractions in §3.3 (`Driver`) and §3.2 (`Generator` / `Reloader`) are shaped right for blue/green in M4 — only the orchestration in `internal/deploy/service.go` and the container-naming convention (§4.6 last paragraph) change. M4 is genuinely additive, not a rewrite. Confirm or push back.

**Q2 (load-bearing):** Is "no client binary in M1, operator SSHes in" a defensible cut, or does it bias the server-side CLI shape away from what the eventual client/server split needs? My read: the server-side `decloud deploy service` is *exactly* what the client will invoke over SSH (probably as `tar c <dir> | ssh host decloud deploy service --stdin --name foo ...`). The M1 surface is a strict subset of the M3 surface — we add a `--stdin` flag in M3 that reads the source bundle from stdin instead of a directory argument. No rework. Confirm or push back.

**Q3 (subsidiary):** Is wiring Viper in M1 (despite needing essentially no process-level config) worth it, or is it a YAGNI violation? My read: it's two files (~50 lines) and hooks `--config-root` to `DECLOUD_ROOT` env var, which tests need anyway. Keeps M2 from doing surgery on the cli wiring. Confirm or push back.

**Q4 (subsidiary):** Should the source bundle (`state/deploys/<name>/<deploy-id>/source.tar.gz`) be written in M1 at all, or defer to M6 when backups exist? My read: write it. It's <50 lines of code, gives us "what built this?" forensics for free, and M6's backup just sweeps `/opt/decloud/` so it gets backed up automatically once M6 lands. Skipping it now means retrofitting deploy IDs and source preservation later. Confirm or push back.

---

## 9. What is explicitly OUT of scope for M1

Restating Don's cuts in implementation-precise terms so Kent and Rob have a checklist of "do not write this":

- **No `client/` directory, no client binary, no SSH transport code.** The whole `client/` package does not exist in M1. Operator SSHes in and runs `decloud deploy service` directly. (Adds in M3.)
- **No `decloud deploy job`, no `internal/jobs/` package, no systemd timer/service unit generation, no `.timer`/`.service` template files.** (M5.)
- **No `decloud backup` subcommand, no restic invocation, no `internal/backup/` package, no nightly timer for backups.** (M6.)
- **No `decloud bootstrap` subcommand, no shell script in `scripts/bootstrap.sh`, no apt invocations, no host systemd unit installation.** Operator installs Docker, Caddy, and the `decloud` binary by hand for M1. (M2.)
- **No volume/mount support.** The `--mount` flag is reserved (rejected with "M3 only" if used); the `Mounts` field is in the TOML schema (so M3 doesn't bump version) but never populated; `dockerdrv.RunRequest.Mounts` is wired through but always empty.
- **No in-house supervisor / no Decloud host systemd unit.** Container restart-on-crash is `--restart=unless-stopped` on the docker run line. (Maybe M7, maybe never.)
- **No blue/green deploy, no Caddy admin API calls, no concurrent old+new containers, no per-service deploy lock.** Strategy is `recreate` only; the `--strategy=blue_green` flag value is rejected with a clear "M4 only" message. (M4.)
- **No image pruning, no `decloud gc`, no weekly timer.** (M6.)
- **No log aggregation beyond `decloud logs <name>` which shells out to `docker logs <container>`.** (Probably never.)
- **No web UI, no HTTP management API, no daemon listening on any port (Caddy's :2019 admin endpoint is an M4 thing and remains bound to localhost).** (Non-goals per README.)
- **No multi-host, no clustering, no scheduling, no scale-anything.** (Non-goals per README.)

If during M1 implementation Rob or Kent finds themselves about to write code that touches any item above, **stop and bring it back to plan.** The whole point of the M1 cut is that none of the above is on the critical path.

---

## 10. Handoff

When Linus approves: Kent writes tests in the package layout from §1.2 against the type signatures in §3 and the behavior in §4 + §6. Rob then implements against Kent's tests and the same specs. If either gets stuck on §3.5 (env capture) or §4.6 (rollback semantics), call Knuth — those are the two most subtle pieces of M1.

End of plan.
