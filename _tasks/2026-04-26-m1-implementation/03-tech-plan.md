# Tech Plan: M1 Execution

**Author:** Joel Spolsky (implementation planner)
**Status:** Execution-tailored. Standalone document. Read the "Inherited from prior task" section for canonical type and behavior references; everything else here is new or revised for the execution round.
**Inputs:** `01-user-request.md`, `02-plan.md` in this task; the approved planning artifacts from the prior task (cited below).
**Audience:** Kent (writing tests next), Rob (implementation), Raymond (docs), Linus (reviewing this plan).

---

## 0. Inherited from prior task — canonical references

The bones of M1 were settled in the prior planning task and approved by Linus. Citations (one line each) so Kent and Rob know where the canonical text lives and don't waste time re-deriving:

- **Spec / contract** — `06-tech-plan-v2.md` §1.
- **Module path + repo tree shape** — `06-tech-plan-v2.md` §2.1, §2.2 (this plan §2 supersedes with the final tree).
- **`cmd/decloud/main.go` skeleton** — `06-tech-plan-v2.md` §2.3 (this plan §7 supersedes with the final form including the `--config-root` propagation fix).
- **Portable env-capture mechanism (verified on macOS bash 3.2)** — `06-tech-plan-v2.md` §3 plus `_ai/envcap-portable-bash.md`.
- **Type definitions for `ServiceConfig`, `ServiceSecrets`, `Service`, `SourceSpec`, `BuildSpec`, `RunSpec`, `Mount`, `Route`, `ReadinessSpec`, `ServiceState`** — `06-tech-plan-v2.md` §4.2. Unchanged.
- **`Store` interface signature and `Load`/`Save`/`Delete` ordering rules** — `06-tech-plan-v2.md` §4.3, §4.5, §4.6, §4.7. Unchanged.
- **Permission enforcement on `Load`** — `06-tech-plan-v2.md` §4.4. Unchanged.
- **Atomic `writeAtomic` helper** — `06-tech-plan-v2.md` §4.5. Unchanged.
- **`pelletier/go-toml/v2` strict-mode setup** — `06-tech-plan-v2.md` §4.9. Unchanged.
- **`schema_version = 1` policy** — `06-tech-plan-v2.md` §5 plus `_ai/decisions/schema-versioning.md`.
- **CLI flag table for `decloud deploy service`** — `06-tech-plan-v2.md` §6.2. Unchanged.
- **Exit code constants** — `06-tech-plan-v2.md` §6.4. Unchanged.
- **Deploy step sequence (recreate)** — `06-tech-plan-v2.md` §6.6. This plan §9 expands it with the per-step rollback table.
- **Caddyfile generator template + `WriteStubIfMissing` body** — `06-tech-plan-v2.md` §7.1, §7.2. Unchanged.
- **`internal/config/paths.go` (`Paths`, `NewPaths`, `RootFromEnv`)** — `06-tech-plan-v2.md` §9.1. Unchanged.
- **`internal/cli/root.go` Cobra wiring sketch** — `06-tech-plan-v2.md` §9.2. This plan §8 commits to the final form.
- **`internal/logging/logging.go` slog setup** — `06-tech-plan-v2.md` §9.3. Unchanged in spirit; this plan §6 adds the file-mode + `MkdirAll` perm fix.
- **Container naming policy (one helper, M1 vs M4)** — `_ai/container-naming.md`.
- **Recoverable "config without secrets" contract** — `06-tech-plan-v2.md` §4.5–§4.7 plus Linus's verification in `07-linus-review-v2.md` (Issue 1 resolution).
- **Operational deliverables list** — `06-tech-plan-v2.md` §10 (this plan §3 supersedes with the LICENSE/CI deferral confirmed).

If you want to know "why a thing is shaped that way," the citations above are the answer. This document only re-derives where the execution round changes something.

---

## 1. Execution-round deltas at a glance

A. **Unit tests only.** `06-tech-plan-v2.md` §12.2's integration tests are out of scope. No `-tags integration` plumbing in M1. `internal/envcap` runs against real `/bin/bash` but that's a unit test (no Docker, no Caddy, no network).

B. **`go install` is the install path.** No supervisor process exists in M1. The deploy binary is invoked one-shot per operator action. Raymond's installation doc (`_docs/operator/installation.md`) tells the operator to install Docker, install Caddy with its own systemd unit pointing at `/opt/declouding/config/caddy/Caddyfile`, `mkdir -p` the `/opt/declouding/` tree (with `chmod 0700 secrets/`), `docker network create decloud`, then `go install github.com/alexander-fenster/decloud/cmd/decloud@latest`. The implementation Rob writes must match what Raymond will document — pinned constants for the network name, the Caddyfile path, the secrets-dir mode. See §4.

C. **LICENSE and CI workflow are DEFERRED out of M1.** I accept Don's call. Defense in §3.

D. **Don's two questions, answered up front:**
   - **Mockgen layout:** `<pkg>/mocks/mock_<iface>.go`. Confirmed Don's preference. Each package owns its mocks. Generated via `go:generate` directives that live next to the interface. Pinned version `go.uber.org/mock v0.4.0`. Detail in §5.
   - **Strict-mode loader on empty `mounts`:** `mounts = []` is accepted (the field's presence with empty value is the same as absence — `len(slice) == 0` either way). Non-empty `mounts` is rejected with the exact message in §10.1. The check is `if len(cfg.Run.Mounts) > 0`. Empty array passes.

---

## 2. Final repo tree (file-by-file checklist)

Kent: this is your test-file checklist. Rob: this is your implementation-file checklist. Every `_test.go` file listed gets created in step 3a (Kent); every `.go` (non-test) file listed gets created in step 3b (Rob), with the exception of `cmd/decloud/main.go` and `go.mod` which are bootstrap.

```
declouding/
  go.mod                                          # Rob, step 1: go mod init
  go.sum                                          # generated
  tools.go                                        # Rob, step 1: pins mockgen for `go install` from go.mod
  CLAUDE.md                                       # exists
  README.md                                       # exists
  cmd/
    decloud/
      main.go                                     # Rob, §7
  internal/
    cli/
      root.go                                     # Rob, §8.1
      root_test.go                                # Kent
      deploy_service.go                           # Rob, §8.2
      deploy_service_test.go                      # Kent
      unregister.go                               # Rob, §8.3
      unregister_test.go                          # Kent
      start.go                                    # Rob, §8.3
      start_test.go                               # Kent
      stop.go                                     # Rob, §8.3
      stop_test.go                                # Kent
      restart.go                                  # Rob, §8.3
      restart_test.go                             # Kent
      status.go                                   # Rob, §8.3
      status_test.go                              # Kent
      logs.go                                     # Rob, §8.3
      logs_test.go                                # Kent
      caddy_reload.go                             # Rob, §8.3
      caddy_reload_test.go                        # Kent
      exit_codes.go                               # Rob, §8.4
      exit_codes_test.go                          # Kent
      mocks/
        mock_deployer.go                          # generated, §5
        mock_lifecycle.go                         # generated, §5
    config/
      paths.go                                    # Rob (cite §9.1 of prior plan)
      paths_test.go                               # Kent
    logging/
      logging.go                                  # Rob, §6
      logging_test.go                             # Kent
    registry/
      types.go                                    # Rob (cite §4.2 of prior plan)
      errors.go                                   # Rob, §10
      store.go                                    # Rob (cite §4.3-§4.9 of prior plan)
      store_test.go                               # Kent
      mocks/
        mock_store.go                             # generated, §5
    envcap/
      capture.go                                  # Rob (cite §3.4 of prior plan)
      capture_test.go                             # Kent
      mocks/
        mock_capturer.go                          # generated, §5
    caddy/
      generator.go                                # Rob (cite §7.1 of prior plan)
      generator_test.go                           # Kent
      reloader.go                                 # Rob (cite §7.1 of prior plan)
      reloader_test.go                            # Kent
      stub.go                                     # Rob (cite §7.2 of prior plan)
      stub_test.go                                # Kent
      mocks/
        mock_generator.go                         # generated, §5
        mock_reloader.go                          # generated, §5
    dockerdrv/
      driver.go                                   # Rob, §11
      cli_driver.go                               # Rob, §11
      cli_driver_test.go                          # Kent
      mocks/
        mock_driver.go                            # generated, §5
    deploy/
      service.go                                  # Rob, §9
      service_test.go                             # Kent
      readiness.go                                # Rob, §9.4
      readiness_test.go                           # Kent
    ids/
      ids.go                                      # Rob, §12
      ids_test.go                                 # Kent
  _docs/
    cli/
      decloud-deploy-service.md                   # Raymond
    architecture/
      m1-recreate-strategy.md                     # Raymond
      secrets-layout.md                           # Raymond
    operator/
      installation.md                             # Raymond (per 02-plan §2.2.1)
      usage.md                                    # Raymond (per 02-plan §2.2.2)
  _ai/
    MEMORY.md                                     # exists, Raymond updates the index
    container-naming.md                           # exists
    envcap-portable-bash.md                       # exists
    decisions/
      m1-scope.md                                 # exists
      secrets-split.md                            # exists
      schema-versioning.md                        # exists
      m1-test-strategy.md                         # Raymond, NEW (per 02-plan §3)
  _tasks/
    2026-04-26-m1-implementation/                 # this task
```

Notable absences (intentional):
- No `LICENSE` — deferred per §3.
- No `.github/workflows/test.yml` — deferred per §3.
- No `internal/cli/exit_codes.go` separate from `internal/cli/` — the constants live in `exit_codes.go`, the `ExitCodeFor(err) int` mapper lives in the same file. One file, kept together, easier to keep in sync.
- No `internal/deploy/mocks/` — `internal/deploy.ServiceDeployer` is consumed by `internal/cli`, so its mock lives at `internal/cli/mocks/mock_deployer.go` (closer to the test that uses it). See §5.

---

## 3. Operational deliverables — final decisions

Confirming Don's calls in `02-plan.md` §3 with my reasoning so Linus has a target.

| Deliverable | M1? | Reasoning |
|---|---|---|
| `go.mod` with `go 1.22` | YES | Required to compile. See §4.1. |
| `tools.go` (mockgen pin) | YES | Necessary so `go generate ./...` produces deterministic mocks across machines. See §5. |
| `LICENSE` (Apache-2.0) | NO — DEFER | Don is right. The user did not ask for a license; the README does not reference one; this is a maintainer call (legal, irreversible). Adding Apache-2.0 by default is a decision I should not make. Adding a LICENSE in a follow-up commit is one line of work, no implementation depends on it. **Confirmed deferred.** |
| `.github/workflows/test.yml` | NO — DEFER | Don is right. The user did not ask for CI. We have not confirmed a public GitHub repo. Module path `github.com/alexander-fenster/decloud` *implies* a GitHub repo but the maintainer has not said "this is hosted there and CI runs there." Adding a workflow file bakes in GitHub-specific assumptions for zero current benefit. The M1 acceptance gate is `go test ./...` on the maintainer's macOS box (per Don §6.1). Adding CI later is another small task. **Confirmed deferred.** |
| `slog`-based structured logging | YES | Per Don §3 + prior tech plan §9.3. The `DECLOUD_LOG_TO_STDERR_ONLY=1` test escape hatch is mandatory so unit tests don't write to `/opt/declouding/logs/`. See §6. |
| `_docs/` operator + architecture + CLI | YES | Per 02-plan §2.2 and prior tech plan §10. Raymond owns. |
| `_ai/decisions/m1-test-strategy.md` (new) | YES | Per Don §3. Captures the "unit tests only for M1" directive so future Don doesn't think it was an oversight. Raymond owns. |

If Linus wants to push back on the LICENSE deferral, the substantive argument is: the module path `github.com/alexander-fenster/decloud` DOES imply public distribution, and `go install` (the install path per delta B) implies users will pull this code. Pulling code without a license is legally ambiguous (default copyright applies). I considered this; my call is still defer because **the user has not greenlit a license choice and I will not pick one for them.** A one-line commit by the maintainer fixes this whenever they want.

---

## 4. Module init + dependency pinning

### 4.1 Module init

```bash
cd /Users/fenster/dev/declouding
go mod init github.com/alexander-fenster/decloud
```

`go.mod` declares `go 1.22`. The Go 1.22 minimum is non-negotiable because we use `log/slog` (1.21+), the `slices` package (1.21+), and we want `range int` for any loop counters Rob writes (1.22).

### 4.2 Direct dependencies (pin major.minor; let patch float)

Use `go get` to add each at the specified minimum version. `go.mod`'s `require` block then floats the patch via `go mod tidy`.

| Module | Minimum version | Why |
|---|---|---|
| `github.com/spf13/cobra` | `v1.8.0` | CLI per CLAUDE.md item 3. v1.8 has the `cobra.MatchAll` and `cobra.OnlyValidArgs` validators we use in `deploy_service.go`. |
| `github.com/pelletier/go-toml/v2` | `v2.1.0` | TOML per CLAUDE.md preference. v2.1+ has `Decoder.DisallowUnknownFields()` returning `*toml.StrictMissingError` for the strict-mode catch (prior tech plan §4.9). |
| `github.com/stretchr/testify` | `v1.9.0` | Tests per CLAUDE.md item 4. v1.9 stabilized `require.EventuallyWithT` which `internal/deploy/readiness_test.go` uses. |
| `go.uber.org/mock` | `v0.4.0` | Mocks per CLAUDE.md item 4. v0.4 is the first release that compiles cleanly under Go 1.22's `range int` and supports the `-typed` flag we want. |

**No Viper.** Per Don plan-v2 §8, deferred to M2.

### 4.3 `tools.go` — pinning mockgen via `go.mod`

Standard Go-tool-pinning idiom so `go install` of mockgen pulls the exact version `go.mod` records. Lives at the repo root.

```go
//go:build tools
// +build tools

package tools

import (
    _ "go.uber.org/mock/mockgen"
)
```

Rob runs `go install go.uber.org/mock/mockgen@v0.4.0` once locally. The `tools.go` import keeps the dep in `go.mod` (else `go mod tidy` would prune it). Re-running `go generate ./...` produces the same mock sources on any machine with `mockgen` on PATH.

`tools.go` is excluded from normal builds via the `//go:build tools` tag. Standard pattern.

---

## 5. Mockgen — invocations and layout

### 5.1 Layout decision (Don's question, answered)

**`<pkg>/mocks/mock_<iface>.go`.** Each package owns its mocks. Reasons:
- Test files (`internal/<pkg>/<x>_test.go`) and the mocks they consume live in the same import-tree branch. Easy to find when reading test code.
- A single top-level `internal/mocks/` would force every package to export its interfaces just to be mockable across the package boundary, which is fine for `Store`/`Capturer`/`Driver`/`Generator`/`Reloader` (they're already exported) but creates an unnecessary "mocks know about everyone" dependency.
- Per-package `mocks/` subdirs keep `go generate` invocations local: each interface declares its own `go:generate` directive next to itself.

**One exception:** `internal/deploy.ServiceDeployer` is consumed by `internal/cli` tests (CLI tests need to assert "the right `deploy.Request` was constructed and `Deploy()` was called"). Two options:
- (a) Mock lives at `internal/deploy/mocks/mock_deployer.go`; CLI tests import that mock.
- (b) Mock lives at `internal/cli/mocks/mock_deployer.go`; the mock is generated against the deploy package interface but lives next to the test that uses it.

I pick **(b)** because CLI tests are the only consumer of that mock; co-locating it with its only consumer beats "mocks live with the interface" when the interface has exactly one consumer outside its own package. Documented in `internal/cli/mocks/mock_deployer.go`'s generated `go:generate` comment so nobody is surprised.

### 5.2 Exact `go:generate` directives

Each one lives at the top of the file that defines the interface (or in `internal/cli/deploy_service.go` for the cross-package case).

In `internal/registry/store.go`:
```go
//go:generate mockgen -source=store.go -destination=mocks/mock_store.go -package=mocks
```

In `internal/envcap/capture.go`:
```go
//go:generate mockgen -source=capture.go -destination=mocks/mock_capturer.go -package=mocks
```

In `internal/caddy/generator.go`:
```go
//go:generate mockgen -source=generator.go -destination=mocks/mock_generator.go -package=mocks
```

In `internal/caddy/reloader.go`:
```go
//go:generate mockgen -source=reloader.go -destination=mocks/mock_reloader.go -package=mocks
```

In `internal/dockerdrv/driver.go`:
```go
//go:generate mockgen -source=driver.go -destination=mocks/mock_driver.go -package=mocks
```

In `internal/cli/deploy_service.go` (the cross-package case):
```go
//go:generate mockgen -destination=mocks/mock_deployer.go -package=mocks github.com/alexander-fenster/decloud/internal/deploy ServiceDeployer
//go:generate mockgen -destination=mocks/mock_lifecycle.go -package=mocks github.com/alexander-fenster/decloud/internal/deploy Lifecycle
```

(`Lifecycle` is the interface for `start`/`stop`/`restart`/`unregister` actions — see §8.3.)

### 5.3 Running mockgen

```bash
cd /Users/fenster/dev/declouding
go install go.uber.org/mock/mockgen@v0.4.0    # one-time per machine; tools.go pins the version
go generate ./...                              # regenerates every mock
go test ./...                                  # mocks must compile
```

Rob commits the generated mocks (they're checked-in source). `go generate` should be idempotent; CI (when added in M2) verifies "no diff after `go generate`."

### 5.4 Why `-source=` instead of reflect mode

`-source=` (source mode) preserves doc comments on the interface in the generated mock and produces deterministic output. Reflect mode requires the package to compile before generation, which creates a chicken-and-egg with `internal/cli/mocks/mock_deployer.go` (CLI imports deploy; if deploy doesn't compile yet because Rob is mid-implementation, reflect-mode generation fails). Source mode parses the file syntactically.

The cross-package case (`mock_deployer.go`) uses reflect mode by necessity — there is no `-source` because the interface lives in another package. Rob runs `go generate` once `internal/deploy/service.go` compiles; that's a step-ordering note for §9.

---

## 6. Slog setup

Already specified in `06-tech-plan-v2.md` §9.3. One small fix Rob must apply:

The prior plan's `os.MkdirAll(paths.LogsDir, 0o755)` is correct, BUT the `os.OpenFile` for `paths.LogFile` opens with mode `0o644`. That's fine for the log file itself, but the `LogsDir` MUST NOT be `0o755` if it's inside `/opt/declouding/` and the operator wants logs to be readable only by root. Rob checks the README's "Persistence layout" — `/opt/declouding/logs/` is not in the secrets tree, so `0o755` directory + `0o644` file is correct. **No change.** Documenting here so Rob doesn't second-guess.

JSON to stderr + log file via `io.MultiWriter`. Test escape hatch via `DECLOUD_LOG_TO_STDERR_ONLY=1`. Logger is `slog.NewJSONHandler` at `slog.LevelInfo`. Fields the deploy orchestration logs at minimum:
- `deploy_id` (string, every log line in a deploy carries it)
- `service` (string, the `--name` value)
- `step` (string: `envcap`, `build`, `stop_old`, `run_new`, `readiness`, `save_registry`, `caddy_reload`)
- `duration_ms` (int, set on step-completion log lines)
- `error` (string, only set on error lines)

Rob: do NOT log captured env values. The `envcap` step's success log says `vars_captured=N`, not the keys, not the values. This is the only place where the `slog` defaults need explicit care. Test asserts that capture-success log does not contain any of the captured var names.

---

## 7. `cmd/decloud/main.go`

Final form. Slight delta from `06-tech-plan-v2.md` §2.3: pass `--config-root` to logging.Init so tests can override the log destination via the same flag operators use.

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

That's it. `logging.Init()` reads `DECLOUD_ROOT` to find the log path (or honors `DECLOUD_LOG_TO_STDERR_ONLY=1` for tests). All other behavior delegates to `cli.NewRootCmd()`.

`logging.Init()` failure exits via `cli.ExitInternal` (70) per Don's accepted call in 02-plan §7.

---

## 8. `internal/cli/` — Cobra wiring (no Viper)

### 8.1 `root.go`

```go
package cli

import (
    "github.com/alexander-fenster/decloud/internal/config"
    "github.com/spf13/cobra"
)

// rootContext holds the resolved config root after persistent-flag parsing.
// Subcommand factories receive a pointer so they read it post-Cobra-parse.
type rootContext struct {
    ConfigRoot string
}

func NewRootCmd() *cobra.Command {
    rc := &rootContext{}
    root := &cobra.Command{
        Use:           "decloud",
        Short:         "Declouding: a personal-scale platform-as-a-service",
        SilenceUsage:  true,
        SilenceErrors: true, // we print errors ourselves in main.go for exit-code mapping
    }
    root.PersistentFlags().StringVar(&rc.ConfigRoot, "config-root", config.RootFromEnv(),
        "root directory for /opt/declouding-style layout (env: DECLOUD_ROOT)")

    deploy := &cobra.Command{Use: "deploy", Short: "Deploy a workload"}
    deploy.AddCommand(newDeployServiceCmd(rc))
    root.AddCommand(deploy)

    root.AddCommand(newUnregisterCmd(rc))
    root.AddCommand(newStartCmd(rc))
    root.AddCommand(newStopCmd(rc))
    root.AddCommand(newRestartCmd(rc))
    root.AddCommand(newStatusCmd(rc))
    root.AddCommand(newLogsCmd(rc))

    caddy := &cobra.Command{Use: "caddy", Short: "Caddy management"}
    caddy.AddCommand(newCaddyReloadCmd(rc))
    root.AddCommand(caddy)

    return root
}
```

`SilenceErrors: true` is load-bearing — Cobra would otherwise print errors to stderr itself, mangling our slog output. We print in `main.go` via `fmt.Fprintln(os.Stderr, err)` and exit with the mapped code.

### 8.2 `deploy_service.go`

The `newDeployServiceCmd` factory builds the Cobra command, parses flags into a `deploy.Request`, constructs a `ServiceDeployer` with concrete dependencies (overridden in tests via DI), invokes `Deploy(ctx, req)`, and returns the error for `ExitCodeFor` mapping.

```go
package cli

import (
    "context"
    "errors"
    "fmt"
    "path/filepath"
    "time"

    "github.com/alexander-fenster/decloud/internal/caddy"
    "github.com/alexander-fenster/decloud/internal/config"
    "github.com/alexander-fenster/decloud/internal/deploy"
    "github.com/alexander-fenster/decloud/internal/dockerdrv"
    "github.com/alexander-fenster/decloud/internal/envcap"
    "github.com/alexander-fenster/decloud/internal/registry"
    "github.com/spf13/cobra"
)

// deployerFactory is overridden in tests to inject a mock ServiceDeployer.
// Production builds use buildProductionDeployer.
var deployerFactory = buildProductionDeployer

type deployServiceFlags struct {
    Name             string
    Hosts            []string
    Port             int
    EnvFile          string
    Mounts           []string // M1: any non-empty value -> error
    ReadinessPath    string
    ReadinessTimeout time.Duration
    Strategy         string
    Dockerfile       string
}

func newDeployServiceCmd(rc *rootContext) *cobra.Command {
    var f deployServiceFlags
    cmd := &cobra.Command{
        Use:   "service [flags] <source-dir>",
        Short: "Deploy or redeploy a service",
        Args:  cobra.ExactArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            return runDeployService(cmd.Context(), rc, &f, args[0])
        },
    }
    cmd.Flags().StringVar(&f.Name, "name", "", "service name (required, [a-z][a-z0-9-]{0,38})")
    cmd.Flags().StringSliceVar(&f.Hosts, "host", nil, "public hostname(s); repeatable")
    cmd.Flags().IntVar(&f.Port, "port", 0, "container listen port (required if --host set)")
    cmd.Flags().StringVar(&f.EnvFile, "env-file", "", "path to env.sh (default: <source-dir>/env.sh if present)")
    cmd.Flags().StringSliceVar(&f.Mounts, "mount", nil, "M1: rejected with ExitConfigError (M3 only)")
    cmd.Flags().StringVar(&f.ReadinessPath, "readiness-path", "/healthz", "HTTP readiness path")
    cmd.Flags().DurationVar(&f.ReadinessTimeout, "readiness-timeout", 60*time.Second, "total readiness wait")
    cmd.Flags().StringVar(&f.Strategy, "strategy", "recreate", "deploy strategy (M1: recreate only)")
    cmd.Flags().StringVar(&f.Dockerfile, "dockerfile", "Dockerfile", "Dockerfile path relative to <source-dir>")
    _ = cmd.MarkFlagRequired("name")
    return cmd
}

func runDeployService(ctx context.Context, rc *rootContext, f *deployServiceFlags, sourceDir string) error {
    if len(f.Mounts) > 0 {
        return fmt.Errorf("--mount is not supported until M3: %w", registry.ErrMountsNotSupported)
    }
    if f.Strategy != "recreate" {
        return fmt.Errorf("--strategy=%q: only \"recreate\" is supported in M1: %w", f.Strategy, registry.ErrInvalidStrategy)
    }
    if len(f.Hosts) > 0 && f.Port == 0 {
        return fmt.Errorf("--port is required when --host is set: %w", errUsage)
    }
    abs, err := filepath.Abs(sourceDir)
    if err != nil {
        return fmt.Errorf("resolving source-dir: %w", err)
    }
    paths := config.NewPaths(rc.ConfigRoot)
    req := deploy.Request{
        Name:             f.Name,
        SourceDir:        abs,
        Dockerfile:       f.Dockerfile,
        Hosts:            f.Hosts,
        Port:             f.Port,
        EnvFile:          f.EnvFile, // empty -> deployer defaults to <SourceDir>/env.sh
        ReadinessPath:    f.ReadinessPath,
        ReadinessTimeout: f.ReadinessTimeout,
        Strategy:         f.Strategy,
    }
    d, err := deployerFactory(paths)
    if err != nil {
        return fmt.Errorf("building deployer: %w", err)
    }
    return d.Deploy(ctx, req)
}

func buildProductionDeployer(paths config.Paths) (deploy.ServiceDeployer, error) {
    return deploy.NewServiceDeployer(deploy.Dependencies{
        Paths:     paths,
        Store:     registry.NewFSStore(paths),
        Capturer:  envcap.New(),
        Driver:    dockerdrv.NewCLIDriver(),
        Generator: caddy.NewGenerator(),
        Reloader:  caddy.NewCLIReloader(),
    })
}

var errUsage = errors.New("usage error")
```

The `deployerFactory` indirection is the test seam. CLI tests set `deployerFactory = func(_ config.Paths) (deploy.ServiceDeployer, error) { return mockDeployer, nil }` in their `t.Setenv`-style setup, then assert on the mock's recorded `Deploy(ctx, req)` call.

### 8.3 Lifecycle subcommands

`unregister`, `start`, `stop`, `restart`, `status`, `logs`, `caddy reload` all follow the same shape: parse flags into a `deploy.LifecycleRequest`, call `lifecycle.<Action>(ctx, req)`. The interface lives in `internal/deploy/service.go`:

```go
type Lifecycle interface {
    Unregister(ctx context.Context, name string) error
    Start(ctx context.Context, name string) error
    Stop(ctx context.Context, name string) error
    Restart(ctx context.Context, name string) error
    Status(ctx context.Context, name string) (Status, error)
    Logs(ctx context.Context, name string, opts LogOptions) error
    CaddyReload(ctx context.Context) error
}
```

Same `lifecycleFactory` indirection as `deployerFactory`. Each subcommand file is ~30 lines.

### 8.4 `exit_codes.go`

Constants from `06-tech-plan-v2.md` §6.4 plus the mapper:

```go
package cli

import (
    "errors"

    "github.com/alexander-fenster/decloud/internal/registry"
)

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

func ExitCodeFor(err error) int {
    switch {
    case err == nil:
        return ExitOK
    case errors.Is(err, errUsage):
        return ExitUsageError
    case errors.Is(err, registry.ErrMountsNotSupported),
        errors.Is(err, registry.ErrInvalidStrategy),
        errors.Is(err, registry.ErrSchemaMismatch),
        errors.Is(err, registry.ErrUnknownField),
        errors.Is(err, registry.ErrPermissionMode),
        errors.Is(err, registry.ErrSecretsMissing),
        errors.Is(err, registry.ErrNotFound):
        return ExitConfigError
    case errors.Is(err, errEnvCapture):
        return ExitEnvCaptureFail
    case errors.Is(err, errBuild):
        return ExitBuildFail
    case errors.Is(err, errRun):
        return ExitRunFail
    case errors.Is(err, errReadiness):
        return ExitReadinessFail
    case errors.Is(err, errCaddyReload):
        return ExitCaddyReloadFail
    default:
        return ExitInternal
    }
}
```

The `errEnvCapture`, `errBuild`, `errRun`, `errReadiness`, `errCaddyReload` sentinels live in `internal/deploy/service.go` and the deploy orchestrator wraps each step's error with the relevant sentinel via `fmt.Errorf("%w: ...", errBuild)`. That keeps the exit-code mapping decoupled from the package the error originated in.

---

## 9. The deploy orchestrator (`internal/deploy/service.go`)

This is the load-bearing piece. Don's plan §2.1 specifically calls out the step-7 mid-write rollback as a Knuth-risk area; this section gives Rob the exact sequence and per-step rollback so he doesn't have to invent it.

### 9.1 Public API

```go
package deploy

import (
    "context"
    "time"

    "github.com/alexander-fenster/decloud/internal/caddy"
    "github.com/alexander-fenster/decloud/internal/config"
    "github.com/alexander-fenster/decloud/internal/dockerdrv"
    "github.com/alexander-fenster/decloud/internal/envcap"
    "github.com/alexander-fenster/decloud/internal/registry"
)

type Request struct {
    Name             string
    SourceDir        string
    Dockerfile       string
    Hosts            []string
    Port             int
    EnvFile          string
    ReadinessPath    string
    ReadinessTimeout time.Duration
    Strategy         string
}

type Status struct {
    Name           string
    ContainerID    string
    ContainerName  string
    State          string // "running" | "stopped" | "absent"
    LastDeployID   string
    LastDeployedAt time.Time
}

type LogOptions struct {
    Follow bool
    Tail   int
}

type Dependencies struct {
    Paths     config.Paths
    Store     registry.Store
    Capturer  envcap.Capturer
    Driver    dockerdrv.Driver
    Generator caddy.Generator
    Reloader  caddy.Reloader
}

type ServiceDeployer interface {
    Deploy(ctx context.Context, req Request) error
}

type Lifecycle interface {
    Unregister(ctx context.Context, name string) error
    Start(ctx context.Context, name string) error
    Stop(ctx context.Context, name string) error
    Restart(ctx context.Context, name string) error
    Status(ctx context.Context, name string) (Status, error)
    Logs(ctx context.Context, name string, opts LogOptions) error
    CaddyReload(ctx context.Context) error
}

func NewServiceDeployer(deps Dependencies) (ServiceDeployer, error) { /* validates deps; returns *serviceDeployer */ }
func NewLifecycle(deps Dependencies) (Lifecycle, error)             { /* same */ }
```

`*serviceDeployer` implements both `ServiceDeployer` and `Lifecycle` — they share state (the same `Dependencies`). We expose them as separate interfaces so CLI tests can mock independently.

### 9.2 Step sequence with rollback (the recreate strategy)

The eight steps from `06-tech-plan-v2.md` §6.6 with explicit rollback at each step. This is the definitive M1 orchestration.

| # | Step | What rolls back if THIS step fails |
|---|---|---|
| 1 | Capture env via `Capturer.Capture(ctx, envFilePath)` | Nothing changed yet. Return `errEnvCapture`. |
| 2 | Resolve previous registration: `prev, err := Store.Load(ctx, name)`. `ErrNotFound` and `ErrSecretsMissing` are both treated as "no usable previous" — first deploy or recoverable mid-create. Other errors abort. | If `Load` fails with anything other than `ErrNotFound`/`ErrSecretsMissing`, return that error wrapped (typically `ErrPermissionMode` → `ExitConfigError`). |
| 3 | Build image: `Driver.Build(ctx, BuildRequest{ImageRef: "decloud-<name>:<deploy-id>", SourceDir, Dockerfile, Stdout, Stderr})`. Streams `docker build` output to stdout per `06-tech-plan-v2.md` §6.5. | Nothing on disk yet. Return `errBuild`. |
| 4 | Stop old container if `prev != nil`: `Driver.Stop(ctx, ContainerName(prev.Name))` with 10s grace. | **Downtime starts here.** If stop fails, log the error, attempt `Driver.Inspect` to check state. If still running, return `errRun` and don't proceed (we'd have two containers fighting for the network alias). If stop succeeded but earlier steps put us in an odd state, see step-5/6 rollback. |
| 5 | Remove old container: `Driver.Remove(ctx, ContainerName(prev.Name))`. Ignore "no such container" (idempotent). | If Remove fails with a real error, abort: return `errRun`. The new container can't start because the name is taken. |
| 6 | Run new container: `Driver.Run(ctx, RunRequest{Name: ContainerName(req.Name), Image: <built>, Network: "decloud", Env: capturedEnv, Restart: "unless-stopped", Port: req.Port})`. | If Run fails, attempt `restoreOldContainer(prev)` — see §9.3. Return `errRun`. |
| 7a | Wait readiness via `readiness.Wait(ctx, ReadinessSpec{Kind: "http", HTTPPath: req.ReadinessPath, TimeoutSecs: int(req.ReadinessTimeout.Seconds())})`. | If readiness fails, `Driver.Stop` + `Driver.Remove` the new container, then `restoreOldContainer(prev)`, return `errReadiness`. |
| 7b | Save registry: `Store.Save(ctx, &Service{Config, Secrets})`. **This is the two-file write.** Config (mode 0644) writes first, then secrets (mode 0600). | If Save returns `ErrPartialWrite` (new sentinel; see §9.5) — config wrote, secrets didn't — `Store.RollbackPartialCreate(ctx, name)` deletes the just-written config to avoid orphan, then `Driver.Stop` + `Driver.Remove` the new container, then `restoreOldContainer(prev)`. Return `errInternal` wrapping `ErrPartialWrite`. If Save fails before any write happens, just kill the new container and restore old. |
| 8a | `caddy.WriteStubIfMissing(paths.CaddyfilePath)`. | If stub write fails, log, continue — Caddy might still be alive on its previous config. |
| 8b | Generate new Caddyfile from `Store.List(ctx)` → `caddy.Generator.Generate(file, inputs)` → atomic write via `writeAtomic`. | If generate or write fails, log warning, exit `errCaddyReload`. New container is up; routing is degraded. We do NOT roll back the deploy because the registration is good and the container is healthy; operator fixes Caddy by hand. |
| 8c | `caddy.Reloader.Reload(ctx, paths.CaddyfilePath)`. | If reload fails, same as 8b: log, exit `errCaddyReload`, don't roll back. |

The rollback semantics are **forward-only at step 8.** After the registry save (7b) succeeds, the deploy is "committed" — only Caddy reload can fail without rolling back the new container. Steps 1–7 are rollback-on-failure.

### 9.3 `restoreOldContainer` — the "rollback" primitive

Called from step 6, 7a, 7b failure paths. Best-effort restoration of the previous container.

```go
func (d *serviceDeployer) restoreOldContainer(ctx context.Context, prev *registry.Service) {
    if prev == nil {
        return // first deploy; nothing to restore
    }
    runReq := dockerdrv.RunRequest{
        Name:    ids.ContainerName(prev.Config.Name),
        Image:   prev.Config.Build.ImageRef, // the previous image, still in local docker cache
        Network: "decloud",
        Env:     prev.Secrets.Env,
        Restart: prev.Config.Run.Restart,
        Port:    prev.Config.Run.Port,
    }
    if err := d.deps.Driver.Run(ctx, runReq); err != nil {
        slog.Error("rollback: failed to restart previous container",
            "service", prev.Config.Name, "error", err)
        return
    }
    slog.Info("rollback: previous container restored", "service", prev.Config.Name)
}
```

Key correctness points:
- The previous image (`prev.Config.Build.ImageRef`) MUST still be in the local Docker cache. M6 image-GC pruning will eventually remove it, but during a deploy the operator's previous image is at most a few minutes old — well inside the GC window. If image is gone, `Driver.Run` fails and we log; manual recovery required.
- We do NOT re-run readiness on the rollback. The previous container was previously healthy; if it can't start now, that's a real problem the operator must see, not a "deploy succeeded" lie.
- The new (failed) container is removed BEFORE this is called by the orchestrator — the network alias is free.

### 9.4 `internal/deploy/readiness.go`

```go
package deploy

import (
    "context"
    "errors"
    "fmt"
    "net/http"
    "time"

    "github.com/alexander-fenster/decloud/internal/registry"
)

type readinessProbe interface {
    Wait(ctx context.Context, containerName string, spec registry.ReadinessSpec, port int) error
}

type httpProbe struct {
    client *http.Client
}

func newHTTPProbe() *httpProbe {
    return &httpProbe{client: &http.Client{Timeout: 2 * time.Second}}
}

func (p *httpProbe) Wait(ctx context.Context, containerName string, spec registry.ReadinessSpec, port int) error {
    if spec.IntervalSecs <= 0 {
        spec.IntervalSecs = 2
    }
    if spec.TimeoutSecs <= 0 {
        spec.TimeoutSecs = 60
    }
    deadline := time.Now().Add(time.Duration(spec.TimeoutSecs) * time.Second)
    url := fmt.Sprintf("http://%s:%d%s", containerName, port, spec.HTTPPath)
    ticker := time.NewTicker(time.Duration(spec.IntervalSecs) * time.Second)
    defer ticker.Stop()
    for {
        if err := p.probe(ctx, url); err == nil {
            return nil
        }
        if time.Now().After(deadline) {
            return fmt.Errorf("readiness probe timed out after %ds: %w", spec.TimeoutSecs, errReadiness)
        }
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-ticker.C:
        }
    }
}

func (p *httpProbe) probe(ctx context.Context, url string) error {
    req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
    resp, err := p.client.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    if resp.StatusCode >= 200 && resp.StatusCode < 400 {
        return nil
    }
    return errors.New("non-2xx/3xx status")
}
```

Probes the container by Docker DNS name on the shared `decloud` network. The decloud binary itself is NOT on that network in M1 (it's running on the host); `containerName` resolves via Docker's embedded DNS only when probing from inside the network. **Correction:** since the decloud process runs on the host, it can't resolve `<containerName>` over Docker DNS. For M1 readiness, we instead probe `127.0.0.1:<host-port>` — but M1 doesn't publish container ports to the host because Caddy reaches the container via Docker DNS on the shared network. **Resolution:** probe via `docker exec <containerName> wget -q -O- http://localhost:<port><path>` OR run the readiness check from a one-shot container on the same network. The cleanest M1 solution is the one-shot:

```
docker run --rm --network=decloud curlimages/curl:8.5.0 -fsS http://<containerName>:<port><path>
```

Rob: implement readiness via this one-shot pattern through the existing `Driver` interface (add `Driver.OneShotProbe(ctx, network, target string) error` — see §11). This keeps readiness on the same Docker plane as Caddy reaches the container.

**This is one of the items I'm flagging for Knuth review** — the one-shot-probe approach pulls a curl image at deploy time, which has cold-start latency. Acceptable for M1; M4 may swap to admin-API-driven readiness. Don's plan §5 already named readiness as a Knuth-risk area; this design choice is the specific shape Rob should validate before fully implementing.

### 9.5 `Store.Save` partial-write detection

Per `06-tech-plan-v2.md` §4.5–§4.6, the two-file Save is config-first then secrets. The orchestrator needs to know if Save partially wrote (config landed, secrets failed) vs failed before any write.

Add a sentinel:

```go
// in internal/registry/errors.go
var ErrPartialWrite = errors.New("registry: partial write (config wrote, secrets failed)")
```

And in `fsStore.Save`:

```go
func (s *fsStore) Save(ctx context.Context, svc *Service) error {
    cfgPath := filepath.Join(s.paths.ServicesDir, svc.Config.Name+".toml")
    secPath := filepath.Join(s.paths.SecretsDir, svc.Config.Name, "env.toml")
    // Pre-validate before any write
    if err := s.validateForSave(svc); err != nil {
        return err
    }
    // Ensure directories exist with correct modes
    if err := os.MkdirAll(s.paths.ServicesDir, 0o755); err != nil {
        return err
    }
    if err := os.MkdirAll(filepath.Dir(secPath), 0o700); err != nil {
        return err
    }
    cfgBytes, err := marshalTOML(svc.Config)
    if err != nil {
        return err
    }
    secBytes, err := marshalTOML(svc.Secrets)
    if err != nil {
        return err
    }
    if err := writeAtomic(cfgPath, 0o644, cfgBytes); err != nil {
        return fmt.Errorf("registry: writing config: %w", err)
    }
    if err := writeAtomic(secPath, 0o600, secBytes); err != nil {
        return fmt.Errorf("%w: writing secrets at %s: %v", ErrPartialWrite, secPath, err)
    }
    return nil
}
```

And `RollbackPartialCreate`:

```go
func (s *fsStore) RollbackPartialCreate(ctx context.Context, name string) error {
    cfgPath := filepath.Join(s.paths.ServicesDir, name+".toml")
    if err := os.Remove(cfgPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
        return err
    }
    return nil
}
```

Add `RollbackPartialCreate` to the `Store` interface:

```go
type Store interface {
    Load(ctx context.Context, name string) (*Service, error)
    Save(ctx context.Context, svc *Service) error
    RollbackPartialCreate(ctx context.Context, name string) error // new in this plan
    List(ctx context.Context) ([]*Service, error)
    Delete(ctx context.Context, name string) error
}
```

This changes the `Store` interface from the prior tech plan — flagging for Linus. The change is minimal (one method added) and necessary because Don explicitly required step-7 mid-write rollback to delete the orphan config (`02-plan.md` §2.1 second bullet). Without `RollbackPartialCreate`, the orchestrator would need to know fsStore's path layout, which breaks the abstraction.

---

## 10. Loader rejection of `mounts` — exact mechanics

### 10.1 The check

Lives in `internal/registry/store.go`'s `Load`, after `DisallowUnknownFields()` decode succeeds:

```go
if len(cfg.Run.Mounts) > 0 {
    return nil, fmt.Errorf("%w: service %q declares %d mount(s) in %s; mounts are not supported until M3",
        ErrMountsNotSupported, cfg.Name, len(cfg.Run.Mounts), cfgPath)
}
```

`ErrMountsNotSupported` is defined in `internal/registry/errors.go`:

```go
var ErrMountsNotSupported = errors.New("registry: mounts not supported in M1")
```

Exit code mapping in `internal/cli/exit_codes.go`: `errors.Is(err, registry.ErrMountsNotSupported)` → `ExitConfigError` (10).

The full operator-visible error string (after `fmt.Errorf` chain unwrap) is:

```
Error: registry: mounts not supported in M1: service "foo" declares 2 mount(s) in /opt/declouding/config/services/foo.toml; mounts are not supported until M3
```

### 10.2 Empty array IS accepted

Don asked the explicit question. Confirmed: `mounts = []` in the TOML is accepted. The `len(cfg.Run.Mounts) > 0` check evaluates to false on both "field absent" and "field present, empty array." The strict-mode loader does not distinguish them at the TOML level. M3 will start writing real entries here without any schema bump.

Test case in `internal/registry/store_test.go`:
- `TestStore_LoadAcceptsEmptyMountsArray` — config file containing `[run]\nmounts = []\n` loads cleanly.
- `TestStore_LoadRejectsNonEmptyMounts` — config file containing `[[run.mounts]]\nhost_path = "/x"\ncontainer_path = "/y"\n` returns `ErrMountsNotSupported`.

### 10.3 The CLI flag does the equivalent rejection

`internal/cli/deploy_service.go` `runDeployService`:

```go
if len(f.Mounts) > 0 {
    return fmt.Errorf("--mount is not supported until M3: %w", registry.ErrMountsNotSupported)
}
```

Same wrapped sentinel → same exit code → operator scripts can pattern-match on the exit code.

---

## 11. `internal/dockerdrv/` — driver shape and arg-construction tests

Don's plan §5 specifically called out the dockerdrv argument-construction discipline. Here's the shape Rob implements; this is also one of the "call Knuth if friction" areas.

### 11.1 The `Driver` interface

```go
package dockerdrv

import (
    "context"
    "io"
)

type BuildRequest struct {
    ImageRef   string
    SourceDir  string
    Dockerfile string
    Stdout     io.Writer
    Stderr     io.Writer
}

type RunRequest struct {
    Name    string
    Image   string
    Network string
    Env     map[string]string
    Restart string
    Port    int
}

type InspectResult struct {
    ContainerID string
    State       string // "running" | "exited" | "absent"
}

type Driver interface {
    Build(ctx context.Context, req BuildRequest) (imageID string, err error)
    Run(ctx context.Context, req RunRequest) (containerID string, err error)
    Stop(ctx context.Context, containerName string, gracePeriod time.Duration) error
    Remove(ctx context.Context, containerName string) error
    Inspect(ctx context.Context, containerName string) (InspectResult, error)
    Logs(ctx context.Context, containerName string, opts LogsOptions) error
    NetworkEnsure(ctx context.Context, networkName string) error
    OneShotProbe(ctx context.Context, networkName, targetURL string) error
}

type LogsOptions struct {
    Follow bool
    Tail   int
    Stdout io.Writer
    Stderr io.Writer
}
```

### 11.2 The injectable command factory

```go
package dockerdrv

import (
    "context"
    "os/exec"
)

type cmdFactory func(ctx context.Context, name string, args ...string) *exec.Cmd

type cliDriver struct {
    cmd cmdFactory
}

func NewCLIDriver() Driver {
    return &cliDriver{cmd: exec.CommandContext}
}

// for tests:
func newCLIDriverWithFactory(f cmdFactory) Driver {
    return &cliDriver{cmd: f}
}
```

Tests construct `cliDriver` with a recording factory:

```go
type recordedCmd struct {
    Name string
    Args []string
}

func recordingFactory(records *[]recordedCmd) cmdFactory {
    return func(ctx context.Context, name string, args ...string) *exec.Cmd {
        *records = append(*records, recordedCmd{Name: name, Args: args})
        return exec.CommandContext(ctx, "true") // a no-op that exits 0
    }
}
```

Then assert that, for `Run(req)` with `req.Name="decloud-foo"`, `req.Image="decloud-foo:abc123"`, `req.Network="decloud"`, `req.Env={"DATABASE_URL": "postgres://..."}`, `req.Restart="unless-stopped"`, `req.Port=8080`, the recorded args equal:

```go
[]string{
    "run", "-d",
    "--name", "decloud-foo",
    "--network", "decloud",
    "--restart", "unless-stopped",
    "--env", "DATABASE_URL=postgres://...",
    "--label", "decloud.service=foo",
    "decloud-foo:abc123",
}
```

(Port is NOT exposed via `-p` because Caddy reaches the container over the shared network, not via a host port. Document in `_docs/architecture/m1-recreate-strategy.md`.)

For each Driver method, the test file includes a comment with the hand-typed equivalent `docker` command at the top, so a reviewer can spot-check args without booting Docker. Per Don plan §5 first bullet.

### 11.3 Why this shape is the right answer for Knuth-review

The injectable factory pattern is well-known but Rob may want a sanity check on:
- **Map ordering for `--env`.** Go map iteration is unordered. The test must canonicalize. Either sort keys in the implementation (so output is deterministic) or have the test compare as a set. **My call: sort in implementation.** Keeps output deterministic regardless of where called from. Rob: sort `Env` keys lexically before constructing args.
- **Whether to use `--env-file` instead of multiple `--env`.** Multiple `--env` is simpler and avoids leaving an env file on disk that contains secrets. M1 chooses multiple `--env`. Document.
- **Stdout/stderr handling in `Build`.** The build needs to stream `docker build` output live to the operator's terminal. Rob: pass `req.Stdout`/`req.Stderr` to `cmd.Stdout`/`cmd.Stderr` in `Build` only; for other commands, capture to in-process buffers for error-message inclusion.

If any of these feels off, call Knuth before implementing.

---

## 12. `internal/ids/` — deploy IDs and container names

```go
package ids

import (
    "crypto/rand"
    "encoding/hex"
    "fmt"
    "time"
)

// NewDeployID returns a deploy identifier matching ^[0-9]{8}-[0-9]{6}-[a-f0-9]{6}$.
// Format: YYYYMMDD-HHMMSS-<6-hex-random>.
func NewDeployID() string {
    now := time.Now().UTC()
    var b [3]byte
    _, _ = rand.Read(b[:])
    return fmt.Sprintf("%s-%s-%s", now.Format("20060102"), now.Format("150405"), hex.EncodeToString(b[:]))
}

// ContainerName returns the M1 container name for a service. M4 will recreate
// containers under "<name>-<deployID>" — see _ai/container-naming.md. Route ALL
// container-name construction through this function so M4 changes one body,
// not every call site.
func ContainerName(serviceName string) string {
    return "decloud-" + serviceName
}

// ImageRef returns the image tag for a deploy.
func ImageRef(serviceName, deployID string) string {
    return "decloud-" + serviceName + ":" + deployID
}
```

Tests: format-stability regex match, uniqueness across rapid calls (1000 IDs in a `map[string]struct{}` — no collision).

---

## 13. Test plan — package-by-package checklist for Kent

This is the complete coverage gate for M1. Each bullet is one test case Kent writes (one `Test_...` function). Where the prior plan §12.1 already enumerated, I cite and add only the deltas.

### 13.1 `internal/registry`

Per `06-tech-plan-v2.md` §12.1 row 1, plus deltas:
- `TestStore_RoundTripConfigAndSecrets` — write then read; equal in memory.
- `TestStore_LoadRejectsUnknownConfigField` — extra field in config TOML → `ErrUnknownField`.
- `TestStore_LoadRejectsUnknownSecretsField` — same for secrets.
- `TestStore_LoadRejectsConfigSchemaMismatch` — `schema_version = 2` → `ErrSchemaMismatch`.
- `TestStore_LoadRejectsCrossFileSchemaMismatch` — config v1, secrets v2 → `ErrSchemaMismatch`.
- `TestStore_LoadRejectsSecretsFileMode0644` — file 0644 → `ErrPermissionMode`.
- `TestStore_LoadRejectsSecretsDirMode0755` — dir 0755 → `ErrPermissionMode`.
- `TestStore_LoadConfigWithoutSecretsReturnsErrSecretsMissing` — the recoverable state.
- `TestStore_LoadAcceptsEmptyMountsArray` — `mounts = []` loads cleanly. **(NEW per §10.2)**
- `TestStore_LoadRejectsNonEmptyMounts` — `[[run.mounts]]` populated → `ErrMountsNotSupported`.
- `TestStore_LoadRejectsInvalidStrategy` — `strategy = "blue_green"` → `ErrInvalidStrategy`.
- `TestStore_SaveOrderConfigBeforeSecrets` — using a fakeFS that records open-for-write order, assert config opened first.
- `TestStore_DeleteOrderSecretsBeforeConfig` — same, for delete.
- `TestStore_SaveAtomicityConfigWriteFails` — inject fs error on config write; assert nothing on disk after.
- `TestStore_SaveReturnsErrPartialWriteOnSecretsFailure` — config wrote, secrets failed → `errors.Is(err, ErrPartialWrite)`. **(NEW per §9.5)**
- `TestStore_RollbackPartialCreateRemovesConfig` — after a `ErrPartialWrite`, calling `RollbackPartialCreate` removes the config file. **(NEW per §9.5)**
- `TestStore_RollbackPartialCreateIsIdempotent` — calling it twice is fine. **(NEW)**
- `TestStore_ListSkipsMalformedFiles` — one bad file in the dir; `List` returns the good ones, logs the bad one.
- `TestStore_SaveSetsCorrectFilePermissions` — even when umask is 0022, secrets file ends up 0600.

### 13.2 `internal/envcap`

Per `06-tech-plan-v2.md` §3.5 (full table — Kent copies the table into test names). Runs against real `/bin/bash`. No build-tag skip. Uses `t.TempDir()` to write the test scripts.

### 13.3 `internal/caddy`

Per `06-tech-plan-v2.md` §12.1 row 3, plus deltas:
- `TestGenerator_OneServiceOneHost` — golden-string equality on a specific input.
- `TestGenerator_MultiServiceMultiHost` — sorted output, deterministic.
- `TestGenerator_DropsZeroHostnameServices` — services with `Hostnames=nil` produce no output stanza.
- `TestGenerator_EmptyInputProducesHeaderOnly` — empty `[]GeneratorInput` → just the `# Generated by decloud. Do not edit.\n` header.
- `TestStub_WritesByteExactStubWhenAbsent` — golden-string equality on the §7.2 stub.
- `TestStub_NoOpWhenFileExists` — pre-existing file with arbitrary content is untouched.
- `TestReloader_InvokesCaddyReload` — using a recording cmdFactory, assert `caddy reload --config <path>` is the recorded command.

### 13.4 `internal/dockerdrv`

All argument-construction. Per §11.2 of this plan. One test per method:
- `TestCLIDriver_BuildArgs`
- `TestCLIDriver_RunArgsWithEnvSorted` — assert env vars appear in lexical order.
- `TestCLIDriver_RunArgsWithEmptyEnv` — no `--env` flags when env is empty.
- `TestCLIDriver_StopArgs` — `docker stop -t 10 <name>`.
- `TestCLIDriver_RemoveArgs` — `docker rm <name>`.
- `TestCLIDriver_InspectArgsAndParse` — assert args; inject a fake JSON response and assert `InspectResult` parsing.
- `TestCLIDriver_LogsArgsFollow` and `TestCLIDriver_LogsArgsTailN`.
- `TestCLIDriver_NetworkEnsureWhenAbsent` — `docker network inspect` exits non-zero, then `docker network create` is called.
- `TestCLIDriver_NetworkEnsureWhenPresent` — `docker network inspect` exits zero, no create call.
- `TestCLIDriver_OneShotProbeArgs` — `docker run --rm --network <net> curlimages/curl:8.5.0 -fsS <url>`.

Each test file's top comment includes the hand-typed equivalent `docker` invocation per Don §5.

### 13.5 `internal/deploy`

Gomock-driven. One test per failure branch, one happy path. Each uses `gomock.InOrder(...)` to assert the exact step ordering — not just call counts.

- `TestDeploy_HappyPathFirstDeploy` — no prev; envcap → build → run → readiness → save → caddy reload, in order.
- `TestDeploy_HappyPathRedeploy` — prev exists; envcap → build → stop old → remove old → run new → readiness → save → caddy reload.
- `TestDeploy_LoadPreviousErrSecretsMissingTreatedAsFirstDeploy` — prev exists as config-only; treat as first deploy (re-capture, write both files).
- `TestDeploy_EnvCaptureFailureAbortsBeforeAnythingChanges`.
- `TestDeploy_BuildFailureAbortsBeforeStoppingOld`.
- `TestDeploy_StopOldFailureAbortsAndDoesNotStartNew`.
- `TestDeploy_RunNewFailureRollsBackToOld` — assert `restoreOldContainer` called.
- `TestDeploy_ReadinessFailureRollsBackToOld` — assert new container removed, old restored.
- `TestDeploy_SaveFailsBeforePartialWriteRollsBack` — Store.Save returns a non-`ErrPartialWrite` error; assert new container removed, old restored, NO `RollbackPartialCreate` called.
- `TestDeploy_SaveErrPartialWriteRollsBackAndDeletesOrphanConfig` — Store.Save returns `ErrPartialWrite`; assert `RollbackPartialCreate` called THEN container rollback.
- `TestDeploy_CaddyReloadFailureDoesNotRollBackContainer` — new container stays up; exit `errCaddyReload`.
- `TestDeploy_CaddyStubWrittenWhenCaddyfileMissing` — file absent → `WriteStubIfMissing` called before reload.
- `TestDeploy_DeployIDIsStableThroughoutOneDeploy` — single deploy logs the same `deploy_id` on every step.
- `TestDeploy_CapturedEnvNotLoggedAsKeysOrValues` — assert log records contain `vars_captured=N` but no captured key names.
- `TestReadiness_HTTPSuccessReturnsNil`.
- `TestReadiness_HTTPTimeoutReturnsErrReadiness`.
- `TestReadiness_HTTPRetriesOnTransientFailure`.
- `TestReadiness_ContextCancellationStopsProbe`.

### 13.6 `internal/cli`

- `TestRoot_ConfigRootDefaultsToDecloudRootEnv` — `t.Setenv("DECLOUD_ROOT", "/tmp/x")`; root command's `--config-root` default is `/tmp/x`.
- `TestRoot_ConfigRootFlagOverridesEnv`.
- `TestDeployService_BuildsExpectedRequest` — mocked deployer; assert `Deploy` called with the right `deploy.Request`.
- `TestDeployService_MissingNameReturnsExitUsageError`.
- `TestDeployService_MountFlagReturnsErrMountsNotSupported` — `--mount /a:/b` → `ExitConfigError` (10).
- `TestDeployService_StrategyBlueGreenReturnsErrInvalidStrategy` — `--strategy=blue_green` → `ExitConfigError` (10).
- `TestDeployService_HostWithoutPortReturnsExitUsageError`.
- `TestDeployService_DefaultEnvFileResolution` — when `--env-file` empty and `<source-dir>/env.sh` exists, deployer receives that path.
- `TestExitCodeFor_AllSentinels` — table-driven; one row per sentinel; assert mapped int.
- `TestExitCodeFor_UnknownErrorMapsToInternal`.
- `TestUnregister_DelegatesToLifecycle` and analogous tests for `start`/`stop`/`restart`/`status`/`logs`/`caddy reload`.

### 13.7 `internal/ids`

- `TestNewDeployID_FormatRegex` — matches `^[0-9]{8}-[0-9]{6}-[a-f0-9]{6}$`.
- `TestNewDeployID_UniqueAcrossRapidCalls` — 1000 calls, no collision.
- `TestContainerName_M1Format` — `ContainerName("foo") == "decloud-foo"`.
- `TestImageRef_Format` — `ImageRef("foo", "20260426-120000-abc123") == "decloud-foo:20260426-120000-abc123"`.

### 13.8 `internal/logging`

- `TestInit_StderrOnlyShortCircuit` — `t.Setenv("DECLOUD_LOG_TO_STDERR_ONLY", "1")`; `Init` does not touch disk.
- `TestInit_DefaultWritesToFileAndStderr` — `t.Setenv("DECLOUD_ROOT", t.TempDir())`; `Init`; emit a log line; assert it appears in both `<root>/logs/decloud.log` and a captured stderr buffer.
- `TestInit_FileOpenFailureReturnsError` — set DECLOUD_ROOT to a path the test process cannot write to; assert error.

### 13.9 `internal/config`

- `TestNewPaths_AllPathsRootedCorrectly` — table of expected paths.
- `TestRootFromEnv_HonorsDecloudRoot`.
- `TestRootFromEnv_DefaultsToDecloudingPath`.

### 13.10 What we explicitly do NOT test

- No test that Cobra's help text contains specific phrases (change-detector).
- No test that the slog JSON keys have a specific stable order (Go's `slog` does not guarantee).
- No snapshot tests of `_docs/` markdown — Raymond's docs are reviewed by Kevlin for content, not snapshotted.
- No integration tests against real Docker, real Caddy, or `caddy validate`. Per user constraint.

---

## 14. Where Knuth gets called (revised list)

Don's plan §5 named two; here's the consolidated list with my additions:

1. **`internal/envcap/capture.go`** — bash 3.2 portability is verified in spirit (per `_ai/envcap-portable-bash.md`), but if Rob hits any test failure on macOS that he can't quickly diagnose, call Knuth. The mechanism is fragile-looking even when correct.
2. **`internal/deploy/service.go` step-7 mid-write rollback path** — the distinction between "Save returned `ErrPartialWrite` (call `RollbackPartialCreate` first, then container rollback)" vs "Save returned anything else (just container rollback)" is subtle. If the test setup boilerplate exceeds ~200 lines or the test is hard to read, call Knuth before refactoring.
3. **`internal/dockerdrv` cmdFactory shape** — if Rob cannot get the recording-factory pattern to give clean tests (e.g., needing to mock `*exec.Cmd` itself for stdin/stdout/stderr behavior), call Knuth. Per Don §5 first bullet.
4. **`internal/deploy/readiness.go` probe transport** — the "decloud process can't resolve Docker DNS from the host" issue (§9.4) has a workable solution (one-shot probe container) but if Rob finds that approach has friction (image-pull delay, docker daemon overhead, flakiness), call Knuth before either implementing or switching to a different design.

---

## 15. Things I'm flagging for Linus

Linus: please attack these specifically.

1. **The `RollbackPartialCreate` addition to the `Store` interface (§9.5).** Necessary for the orchestrator to clean up an orphan config file after a partial write without knowing fsStore's path layout. Adds one method. Acceptable encapsulation tradeoff?

2. **The readiness one-shot-container approach (§9.4).** The host-side decloud process can't resolve container DNS on the shared Docker network, so we run readiness via `docker run --rm --network=decloud curlimages/curl:8.5.0 ...`. This pulls a curl image at first-use. Acceptable for M1 or do you want me to design something else?

3. **Container port not published to host (§11.2 last paragraph).** M1 reaches the container only via Caddy on the shared `decloud` network; no `-p` flag. This is correct per the README's "Caddy is the only ingress" but worth confirming you agree before we ship.

4. **The mockgen layout (§5.1) and the one exception for `mock_deployer.go` living in `internal/cli/mocks/`.** Pragmatic-but-asymmetric.

5. **LICENSE and CI deferred (§3).** I'm carrying Don's call. Defended in the table reasoning.

Everything else is mechanical execution of the prior tech plan with the deltas listed up front.

---

## 16. Handoff

When this plan is approved (Linus reviews next):

- **Step 3a (Kent):** writes failing unit tests per §13. Stubs implementation files with `panic("unimplemented")` bodies so tests compile-and-fail. Commits and reports.
- **Step 3b (Rob):** implements per §2, §6–§12. Runs `go generate ./...` for mocks. Reports.
- **Step 3c (Raymond):** writes operator + architecture + CLI docs per `02-plan.md` §2.2 plus `_ai/decisions/m1-test-strategy.md`. Reports.
- **Steps 3d/3e (Kevlin/Linus parallel):** review.
- **Step 2-redux (Don/Joel/Linus):** PLAN re-check; iterate or done.

End of plan.
