# Tech Plan: M1 Execution — v2

**Author:** Joel Spolsky (implementation planner)
**Status:** Standalone revision of `03-tech-plan.md` after Linus's `04-linus-review.md` REQUESTED REVISIONS and Don's revised plan in `05-plan-v2.md`. This file replaces `03-tech-plan.md` for execution; `03-tech-plan.md` remains as history.
**Source of truth:** Don's `05-plan-v2.md` is the binding execution plan. This document is the implementation-level expansion.
**Audience:** Kent (writing tests next), Rob (implementation), Raymond (docs), Linus (reviewing this plan).

---

## 0. Changes from v1 (so Linus can diff fast)

Ten substantive changes; everything else from v1 stands.

**Blocking fixes (Linus's holes + Don's directives):**

1. **NEW §9.6 — Lifecycle method specifications.** Seven subsections (Unregister, Stop, Start, Restart, Status, Logs, CaddyReload) with full Go signatures, step ordering, error sentinels, exit-code mapping, and named test cases. v1 declared the interface but did not specify behavior; that gap is closed.
2. **REWRITTEN §9.4 — Readiness probe.** v1 had three options and committed to none. v2 picks Don's choice (Linus's Option D): host-side HTTP probe using `Driver.ContainerIP(ctx, name)`-derived bridge IP. The contradictory three-option text is gone. `Driver.OneShotProbe` is REMOVED from the interface.
3. **UPDATED §9.2 step 8b — Caddy pre-validation.** New `Reloader.Validate(ctx, configPath)` is invoked on the tmp file BEFORE atomic-rename. If validate fails, the OLD Caddyfile on disk is untouched; the deploy aborts with `errCaddyReload`. Same change applied in §9.6.1 (Unregister) and §9.6.7 (CaddyReload).
4. **NEW §16 — Handoff receipt format (Don §3.4).** Rob's step-3b implementation report MUST include a "Test pass receipt" section with the ten items Don enumerated. M1 acceptance does not pass without it. CI deferred.

**Non-blocking fixes:**

5. **RENAMED §9.5 — `Store.RollbackPartialCreate` → `Store.DeleteOrphanConfig`.** Same body, clearer name. All call sites updated. Test names updated.
6. **NEW §11.1 driver methods — `Driver.ContainerIP` and `Driver.Start`.** ContainerIP added per §9.4 rewrite. Start added per §9.6.4 (Restart) so we have a stop+start primitive distinct from `docker restart`.
7. **EXPANDED §13.2 — `internal/envcap` test names.** Three explicit edge-case test names enumerated by name (`TestEnvcap_SetAOff_VariablesDropped`, `TestEnvcap_ArrayDeclaration_OnlyFirstElementCaptured`, `TestEnvcap_ReadonlyConflict_FailsWithSetE`) instead of "Kent copies the table from prior plan §3.5."
8. **NEW comment in §8.2 — `deployerFactory` parallel-safety.** Package-global pattern stays; Joel adds the `// do NOT call t.Parallel() in any internal/cli test` comment block above `deployerFactory` and `lifecycleFactory`.
9. **NEW §2 file — `internal/deploy/lifecycle.go`.** Lifecycle methods cluster in their own file, separate from `service.go`. Both files share `*serviceDeployer` (one struct, two interfaces — confirmed below).
10. **NEW deferral row §3 — `state/deploys/<name>/<deploy-id>/source.tar.gz`.** Explicit "no M1 code populates this tree" record so future Don knows it wasn't an oversight.

**Mockgen layout asymmetry** (`internal/cli/mocks/mock_deployer.go`) — UNCHANGED from v1 §5.1. Acceptable per Linus answer #4. v2 explicitly documents the asymmetry rationale in §5.1 plus a header comment in the generated file.

**§10 (mounts rejection)** — UNCHANGED from v1. Verified still consistent with everything in v2; the `len(cfg.Run.Mounts) > 0` check on `Load` is the single point of enforcement, with the CLI flag mirroring it.

---

## 1. Inherited from prior task — canonical references (unchanged from v1)

The bones of M1 were settled in the prior planning task and approved by Linus. Citations (one line each) so Kent and Rob know where the canonical text lives:

- **Spec / contract** — `_tasks/2026-04-26-readme-implementation-planning/06-tech-plan-v2.md` §1.
- **Module path + repo tree shape** — prior `06-tech-plan-v2.md` §2.1, §2.2 (this plan §2 supersedes with the final tree).
- **`cmd/decloud/main.go` skeleton** — prior `06-tech-plan-v2.md` §2.3 (this plan §7 supersedes).
- **Portable env-capture mechanism** — prior `06-tech-plan-v2.md` §3 plus `_ai/envcap-portable-bash.md`.
- **Type definitions** for `ServiceConfig`, `ServiceSecrets`, `Service`, `SourceSpec`, `BuildSpec`, `RunSpec`, `Mount`, `Route`, `ReadinessSpec`, `ServiceState` — prior `06-tech-plan-v2.md` §4.2.
- **`Store` interface signature, save/load/delete ordering** — prior `06-tech-plan-v2.md` §4.3, §4.5, §4.6, §4.7. v2 ADDS `DeleteOrphanConfig` (§9.5).
- **Permission enforcement on `Load`** — prior `06-tech-plan-v2.md` §4.4.
- **Atomic `writeAtomic` helper** — prior `06-tech-plan-v2.md` §4.5.
- **`pelletier/go-toml/v2` strict-mode setup** — prior `06-tech-plan-v2.md` §4.9.
- **`schema_version = 1` policy** — prior `06-tech-plan-v2.md` §5 plus `_ai/decisions/schema-versioning.md`.
- **CLI flag table for `decloud deploy service`** — prior `06-tech-plan-v2.md` §6.2.
- **Exit code constants** — prior `06-tech-plan-v2.md` §6.4 (this plan §8.4 reproduces with the lifecycle additions).
- **Deploy step sequence (recreate)** — prior `06-tech-plan-v2.md` §6.6 (this plan §9.2 expands with rollback table; §9.2 step 8b updated for pre-validation).
- **Caddyfile generator template + `WriteStubIfMissing` body** — prior `06-tech-plan-v2.md` §7.1, §7.2. v2 ADDS `Reloader.Validate` (§9.2 step 8b).
- **`internal/config/paths.go`** — prior `06-tech-plan-v2.md` §9.1.
- **`internal/cli/root.go` Cobra wiring sketch** — prior `06-tech-plan-v2.md` §9.2 (this plan §8.1 commits to the final form).
- **`internal/logging/logging.go` slog setup** — prior `06-tech-plan-v2.md` §9.3 (this plan §6 adds the file-mode + `MkdirAll` perm note).
- **Container naming policy** — `_ai/container-naming.md`.
- **Recoverable "config without secrets" contract** — prior `06-tech-plan-v2.md` §4.5–§4.7 plus `07-linus-review-v2.md` Issue 1 resolution.
- **Operational deliverables list** — prior `06-tech-plan-v2.md` §10 (this plan §3 supersedes with LICENSE/CI deferral confirmed and the `state/deploys/` tarball deferral added).

If you want to know "why a thing is shaped that way," the citations above are the answer.

---

## 2. Final repo tree (file-by-file checklist)

Same as v1 except: `internal/deploy/lifecycle.go` and `internal/deploy/lifecycle_test.go` are NEW; `internal/caddy/reloader.go` gains a `Validate` method (no new file).

```
declouding/
  go.mod                                          # Rob, step 1: go mod init
  go.sum                                          # generated
  tools.go                                        # Rob, step 1: pins mockgen
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
        doc.go                                    # Rob, §5.1 — asymmetry-rationale comment
    config/
      paths.go                                    # Rob (cite prior §9.1)
      paths_test.go                               # Kent
    logging/
      logging.go                                  # Rob, §6
      logging_test.go                             # Kent
    registry/
      types.go                                    # Rob (cite prior §4.2)
      errors.go                                   # Rob, §10 + §9.5
      store.go                                    # Rob (cite prior §4.3-§4.9 plus §9.5 DeleteOrphanConfig)
      store_test.go                               # Kent
      mocks/
        mock_store.go                             # generated, §5
    envcap/
      capture.go                                  # Rob (cite prior §3.4)
      capture_test.go                             # Kent
      mocks/
        mock_capturer.go                          # generated, §5
    caddy/
      generator.go                                # Rob (cite prior §7.1)
      generator_test.go                           # Kent
      reloader.go                                 # Rob (cite prior §7.1 + §9.2 step 8b Validate)
      reloader_test.go                            # Kent
      stub.go                                     # Rob (cite prior §7.2)
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
      lifecycle.go                                # Rob, §9.6  (NEW)
      lifecycle_test.go                           # Kent       (NEW)
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
      installation.md                             # Raymond (per 05-plan-v2 §2.2.1)
      usage.md                                    # Raymond (per 05-plan-v2 §2.2.2)
  _ai/
    MEMORY.md                                     # exists, Raymond updates the index
    container-naming.md                           # exists
    envcap-portable-bash.md                       # exists
    decisions/
      m1-scope.md                                 # exists
      secrets-split.md                            # exists
      schema-versioning.md                        # exists
      m1-test-strategy.md                         # Raymond, NEW (per 05-plan-v2 §3 / §5)
  _tasks/
    2026-04-26-m1-implementation/                 # this task
```

Notable absences (intentional):
- No `LICENSE` — deferred per §3.
- No `.github/workflows/test.yml` — deferred per §3 + §16 (receipt format compensates).
- No `state/deploys/<name>/...` populated by code — directory created by installation steps, no M1 code writes to it. Per §3.
- No separate `internal/cli/exit_codes.go` and mapper file — both live in the same file.
- No `internal/deploy/mocks/` — `internal/deploy.ServiceDeployer` and `internal/deploy.Lifecycle` mocks live at `internal/cli/mocks/` (closer to the only consumer). See §5.1.

---

## 3. Operational deliverables — final decisions (updated from v1 §3)

| Deliverable | M1? | Reasoning |
|---|---|---|
| `go.mod` with `go 1.22` | YES | Required to compile. See §4.1. |
| `tools.go` (mockgen pin) | YES | Necessary so `go generate ./...` produces deterministic mocks. See §5. |
| `LICENSE` (Apache-2.0) | NO — DEFER | Maintainer call. The one-sentence note in `_docs/operator/installation.md` step 7 (per Don §2.2.1) covers the operator. Don §5 ratifies. |
| `.github/workflows/test.yml` | NO — DEFER | Per Don §3.4 / §5. The receipt format Rob attaches (this plan §16) is the M1 consolation prize. |
| `slog`-based structured logging | YES | Per Don §3 + prior tech plan §9.3. `DECLOUD_LOG_TO_STDERR_ONLY=1` test escape hatch is mandatory. |
| `_docs/` operator + architecture + CLI | YES | Per Don §2.2 and prior tech plan §10. Raymond owns. |
| `_ai/decisions/m1-test-strategy.md` (new) | YES | Per Don §3 / §5. Captures the "unit tests only for M1" directive. Raymond owns. |
| `state/deploys/<name>/<deploy-id>/source.tar.gz` | NO — DEFER | Per Don §4.2. Directory created by installation step 4; no M1 code writes to it. M6 backups will sweep an empty tree harmlessly. Re-evaluate when M6 is planned. |

**For Raymond, the no-license sentence in `_docs/operator/installation.md` step 7 is (verbatim, from Don §2.2.1.7):**

> Note: this repository does not yet declare a license. If you intend to redistribute the binary or use it in a context that requires explicit license grant, ask the maintainer before doing so.

Plain English, no FUD, no legalese.

---

## 4. Module init + dependency pinning

### 4.1 Module init

```bash
cd /Users/fenster/dev/declouding
go mod init github.com/alexander-fenster/decloud
```

`go.mod` declares `go 1.22`. Required for `log/slog` (1.21+), `slices` (1.21+), and `range int` (1.22).

### 4.2 Direct dependencies (pin major.minor; let patch float)

Use `go get` to add each at the specified minimum version. `go.mod`'s `require` block then floats the patch via `go mod tidy`.

| Module | Minimum version | Why |
|---|---|---|
| `github.com/spf13/cobra` | `v1.8.0` | CLI per CLAUDE.md item 3. v1.8 has the validators we use. |
| `github.com/pelletier/go-toml/v2` | `v2.1.0` | TOML per CLAUDE.md preference. v2.1+ has `Decoder.DisallowUnknownFields()`. |
| `github.com/stretchr/testify` | `v1.9.0` | Tests per CLAUDE.md item 4. v1.9 stabilized `require.EventuallyWithT`. |
| `go.uber.org/mock` | `v0.4.0` | Mocks per CLAUDE.md item 4. v0.4 compiles cleanly under Go 1.22 and supports `-typed`. |

**No Viper.** Per Don plan-v2 §8, deferred to M2.

### 4.3 `tools.go` — pinning mockgen

```go
//go:build tools
// +build tools

package tools

import (
    _ "go.uber.org/mock/mockgen"
)
```

Rob runs `go install go.uber.org/mock/mockgen@v0.4.0` once locally. The `tools.go` import keeps the dep in `go.mod` so re-running `go generate ./...` produces the same mock sources on any machine. Excluded from normal builds via `//go:build tools`.

---

## 5. Mockgen — invocations and layout

### 5.1 Layout decision (UNCHANGED from v1; Linus accepted)

**`<pkg>/mocks/mock_<iface>.go`.** Each package owns its mocks. Reasons unchanged:
- Test files and the mocks they consume live in the same import-tree branch.
- A single top-level `internal/mocks/` would force unnecessary "mocks know about everyone" coupling.
- Per-package `mocks/` subdirs keep `go generate` invocations local.

**One documented exception:** `internal/deploy.ServiceDeployer` and `internal/deploy.Lifecycle` are consumed only by `internal/cli`. v1 picked Option (b): the mock lives at `internal/cli/mocks/mock_deployer.go` (and `mock_lifecycle.go`), co-located with the only consumer. Linus answer #4 accepted this with the requirement to **document the deviation in the generated file's header**.

**The asymmetry-rationale comment** lives in `internal/cli/mocks/doc.go` (NEW file Rob writes by hand — `go generate` overwrites the generated mock files but `doc.go` is human-authored):

```go
// Package mocks contains generated mocks for interfaces consumed (but not
// defined) by internal/cli.
//
// LAYOUT NOTE: By project convention, mocks live next to the interface they
// mock — for example, internal/registry/mocks/mock_store.go mocks
// internal/registry.Store. The mocks in this package are an EXCEPTION: they
// mock interfaces defined in internal/deploy (ServiceDeployer, Lifecycle)
// because internal/cli is the sole consumer of those interfaces. Co-locating
// the mock with the only consumer beats co-locating with the interface when
// the interface has exactly one consumer outside its own package.
//
// If a second consumer of internal/deploy.ServiceDeployer or
// internal/deploy.Lifecycle ever appears (e.g., a future "decloud bootstrap"
// command in another package), MOVE these mocks to internal/deploy/mocks/.
package mocks
```

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

In `internal/cli/deploy_service.go` (cross-package):
```go
//go:generate mockgen -destination=mocks/mock_deployer.go -package=mocks github.com/alexander-fenster/decloud/internal/deploy ServiceDeployer
//go:generate mockgen -destination=mocks/mock_lifecycle.go -package=mocks github.com/alexander-fenster/decloud/internal/deploy Lifecycle
```

### 5.3 Running mockgen

```bash
cd /Users/fenster/dev/declouding
go install go.uber.org/mock/mockgen@v0.4.0
go generate ./...
go test ./...
```

Rob commits the generated mocks. `go generate` should be idempotent — the §16 receipt verifies `git status --porcelain` is empty after `go generate ./...`.

### 5.4 Why `-source=` instead of reflect mode

`-source=` preserves doc comments and produces deterministic output. Reflect mode requires the package to compile before generation, which creates a chicken-and-egg with `internal/cli/mocks/mock_deployer.go` (CLI imports deploy; mid-implementation, deploy may not compile). Source mode parses syntactically.

The cross-package case (`mock_deployer.go`, `mock_lifecycle.go`) uses reflect mode by necessity — there is no `-source` because the interface lives in another package. Step ordering in §9: `internal/deploy/service.go` and `internal/deploy/lifecycle.go` must compile before Rob runs `go generate` for those two mocks.

---

## 6. Slog setup

Specified in prior `06-tech-plan-v2.md` §9.3. v2 unchanged. Two notes for Rob to avoid second-guessing:

- `os.MkdirAll(paths.LogsDir, 0o755)` is correct. `/opt/declouding/logs/` is not in the secrets tree, so 0755 dir + 0644 log file is right.
- JSON to stderr + log file via `io.MultiWriter`. Test escape hatch via `DECLOUD_LOG_TO_STDERR_ONLY=1`. Logger is `slog.NewJSONHandler` at `slog.LevelInfo`.

Fields the deploy orchestration logs at minimum:
- `deploy_id` (string, every log line in a deploy carries it)
- `service` (string, the `--name` value)
- `step` (string: `envcap`, `build`, `stop_old`, `run_new`, `readiness`, `save_registry`, `caddy_validate`, `caddy_reload`)
- `duration_ms` (int, set on step-completion log lines)
- `error` (string, only set on error lines)

The Lifecycle methods (§9.6) log with the same `service` field plus a `lifecycle_op` field (`unregister`, `start`, `stop`, `restart`, `status`, `logs`, `caddy_reload`).

**Rob: do NOT log captured env values.** The `envcap` step's success log says `vars_captured=N`, not the keys, not the values. `TestDeploy_CapturedEnvNotLoggedAsKeysOrValues` enforces.

---

## 7. `cmd/decloud/main.go`

Final form. Same as v1.

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

`logging.Init()` reads `DECLOUD_ROOT` (or honors `DECLOUD_LOG_TO_STDERR_ONLY=1`). Failure exits via `cli.ExitInternal` (70).

---

## 8. `internal/cli/` — Cobra wiring (no Viper)

### 8.1 `root.go`

Same as v1.

```go
package cli

import (
    "github.com/alexander-fenster/decloud/internal/config"
    "github.com/spf13/cobra"
)

// rootContext holds the resolved config root after persistent-flag parsing.
type rootContext struct {
    ConfigRoot string
}

func NewRootCmd() *cobra.Command {
    rc := &rootContext{}
    root := &cobra.Command{
        Use:           "decloud",
        Short:         "Declouding: a personal-scale platform-as-a-service",
        SilenceUsage:  true,
        SilenceErrors: true,
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

`SilenceErrors: true` is load-bearing — Cobra would otherwise mangle our slog output.

### 8.2 `deploy_service.go`

Same as v1 except the new comment block above `deployerFactory` (per §4.4 of Don's v2 / Linus Hole #5).

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

// deployerFactory is a package-global test seam. Tests reassign it during
// setup and restore in teardown. Do NOT call t.Parallel() in any internal/cli
// test — concurrent reassignment is unsafe. If parallel CLI tests are ever
// needed, refactor to functional options on NewRootCmd. Same applies to
// lifecycleFactory in internal/cli/unregister.go (and the other lifecycle
// command files).
var deployerFactory = buildProductionDeployer

type deployServiceFlags struct {
    Name             string
    Hosts            []string
    Port             int
    EnvFile          string
    Mounts           []string
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
        EnvFile:          f.EnvFile,
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

### 8.3 Lifecycle subcommands

`unregister`, `start`, `stop`, `restart`, `status`, `logs`, `caddy reload` all follow the same shape: parse flags, call the matching `Lifecycle.<Action>(ctx, ...)`. The interface lives in `internal/deploy/service.go` (interface definition; implementation in `internal/deploy/lifecycle.go`).

`lifecycleFactory` mirrors `deployerFactory` — package-global test seam with the same parallel-safety comment.

```go
// In internal/cli/unregister.go (and shared by start/stop/restart/status/logs/caddy_reload):

// lifecycleFactory is a package-global test seam. Same constraints as
// deployerFactory in internal/cli/deploy_service.go: do NOT call t.Parallel()
// in any internal/cli test — concurrent reassignment is unsafe.
var lifecycleFactory = buildProductionLifecycle

func buildProductionLifecycle(paths config.Paths) (deploy.Lifecycle, error) {
    return deploy.NewLifecycle(deploy.Dependencies{
        Paths:     paths,
        Store:     registry.NewFSStore(paths),
        Capturer:  envcap.New(),
        Driver:    dockerdrv.NewCLIDriver(),
        Generator: caddy.NewGenerator(),
        Reloader:  caddy.NewCLIReloader(),
    })
}
```

Each subcommand file is ~30 lines. Sketch for `unregister.go`:

```go
func newUnregisterCmd(rc *rootContext) *cobra.Command {
    return &cobra.Command{
        Use:   "unregister <name>",
        Short: "Remove a registered service (stop, remove, delete config+secrets, regenerate Caddyfile)",
        Args:  cobra.ExactArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            paths := config.NewPaths(rc.ConfigRoot)
            lc, err := lifecycleFactory(paths)
            if err != nil {
                return fmt.Errorf("building lifecycle: %w", err)
            }
            return lc.Unregister(cmd.Context(), args[0])
        },
    }
}
```

`start.go`, `stop.go`, `restart.go` follow the identical shape with `lc.Start/Stop/Restart(ctx, args[0])`.

`status.go`:
```go
func newStatusCmd(rc *rootContext) *cobra.Command {
    return &cobra.Command{
        Use:   "status <name>",
        Short: "Show runtime + registry status of a service",
        Args:  cobra.ExactArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            paths := config.NewPaths(rc.ConfigRoot)
            lc, err := lifecycleFactory(paths)
            if err != nil {
                return err
            }
            st, err := lc.Status(cmd.Context(), args[0])
            if err != nil {
                return err
            }
            // Single-line human-readable format. --json deferred to M1.5.
            fmt.Fprintf(cmd.OutOrStdout(), "%s state=%s container=%s deploy=%s deployed_at=%s\n",
                st.Name, st.State, st.ContainerName, st.LastDeployID, st.LastDeployedAt.Format(time.RFC3339))
            return nil
        },
    }
}
```

`logs.go`:
```go
func newLogsCmd(rc *rootContext) *cobra.Command {
    var follow bool
    var tail int
    cmd := &cobra.Command{
        Use:   "logs <name>",
        Short: "Stream logs for a service container",
        Args:  cobra.ExactArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            paths := config.NewPaths(rc.ConfigRoot)
            lc, err := lifecycleFactory(paths)
            if err != nil {
                return err
            }
            return lc.Logs(cmd.Context(), args[0], deploy.LogOptions{Follow: follow, Tail: tail})
        },
    }
    cmd.Flags().BoolVarP(&follow, "follow", "f", false, "follow log output")
    cmd.Flags().IntVar(&tail, "tail", 0, "number of trailing lines to show (0 = all)")
    return cmd
}
```

`caddy_reload.go`:
```go
func newCaddyReloadCmd(rc *rootContext) *cobra.Command {
    return &cobra.Command{
        Use:   "reload",
        Short: "Regenerate Caddyfile from registry, validate, and reload Caddy",
        Args:  cobra.NoArgs,
        RunE: func(cmd *cobra.Command, args []string) error {
            paths := config.NewPaths(rc.ConfigRoot)
            lc, err := lifecycleFactory(paths)
            if err != nil {
                return err
            }
            return lc.CaddyReload(cmd.Context())
        },
    }
}
```

### 8.4 `exit_codes.go`

Constants from prior `06-tech-plan-v2.md` §6.4 plus the mapper. Updated to map the lifecycle-specific sentinels Don's §3.1 introduces.

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

The `errEnvCapture`, `errBuild`, `errRun`, `errReadiness`, `errCaddyReload` sentinels live in `internal/deploy/service.go` (Deploy uses all five) and are reused by `internal/deploy/lifecycle.go` (Lifecycle's `Unregister` and `CaddyReload` wrap with `errCaddyReload`; `Start` wraps `Driver.Run` failure with `errRun`). The mapping is by sentinel, so the same exit codes apply across deploy and lifecycle paths.

---

## 9. The deploy orchestrator + lifecycle (`internal/deploy/`)

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
    State          string // "running" | "stopped" | "absent" | "config-only"
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
    Stdout    io.Writer  // defaults to os.Stdout in NewServiceDeployer/NewLifecycle if zero
    Stderr    io.Writer  // defaults to os.Stderr
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
func NewLifecycle(deps Dependencies) (Lifecycle, error)             { /* same backing struct */ }
```

**Decision (Don's open question §3.1):** ONE struct, two interfaces. `*serviceDeployer` implements BOTH `ServiceDeployer` and `Lifecycle`. Rationale:
- The dependencies are identical (same `Paths`/`Store`/`Capturer`/`Driver`/`Generator`/`Reloader`).
- Several lifecycle methods reuse helpers from `Deploy` (notably `regenerateAndReload` for Unregister/CaddyReload).
- Two structs would be pure duplication; a private `regenerator` substruct would be over-engineered for M1.

**File split:** `service.go` holds the `Deploy` method, `restoreOldContainer`, the private `regenerateAndReload` helper, and the sentinel error vars. `lifecycle.go` holds the seven Lifecycle methods. Both files are in `package deploy`; `*serviceDeployer` is defined in `service.go` and methods are added across both files.

### 9.2 Deploy step sequence with rollback (the recreate strategy)

Same as v1 EXCEPT step 8b adds Caddy pre-validation. The eight steps from prior `06-tech-plan-v2.md` §6.6 with explicit per-step rollback:

| # | Step | What rolls back if THIS step fails |
|---|---|---|
| 1 | Capture env via `Capturer.Capture(ctx, envFilePath)` | Nothing changed yet. Return `errEnvCapture`. |
| 2 | Resolve previous registration: `prev, err := Store.Load(ctx, name)`. `ErrNotFound` and `ErrSecretsMissing` are both treated as "no usable previous." Other errors abort. | If Load fails with anything else, return that error wrapped. |
| 3 | Build image: `Driver.Build(ctx, BuildRequest{ImageRef: ids.ImageRef(req.Name, deployID), SourceDir, Dockerfile, Stdout, Stderr})`. | Nothing on disk yet. Return `errBuild`. |
| 4 | Stop old container if `prev != nil`: `Driver.Stop(ctx, ContainerName(prev.Name), 10*time.Second)`. | **Downtime starts here.** If stop fails, log; attempt `Driver.Inspect`; if still running, return `errRun` and don't proceed. |
| 5 | Remove old container: `Driver.Remove(ctx, ContainerName(prev.Name))`. Ignore "no such container." | If Remove fails with a real error, return `errRun`. |
| 6 | Run new container: `Driver.Run(ctx, RunRequest{Name: ContainerName(req.Name), Image: <built>, Network: "decloud", Env: capturedEnv, Restart: "unless-stopped", Port: req.Port})`. | If Run fails, attempt `restoreOldContainer(prev)`. Return `errRun`. |
| 7a | Wait readiness via `readiness.Wait(ctx, ContainerName(req.Name), spec, req.Port)`. | If readiness fails, `Driver.Stop` + `Driver.Remove` the new container, then `restoreOldContainer(prev)`, return `errReadiness`. |
| 7b | Save registry: `Store.Save(ctx, &Service{Config, Secrets})`. Config (mode 0644) writes first, then secrets (mode 0600). | If Save returns `ErrPartialWrite` — config wrote, secrets didn't — `Store.DeleteOrphanConfig(ctx, name)` deletes the just-written config, then `Driver.Stop` + `Driver.Remove` the new container, then `restoreOldContainer(prev)`. Return `errInternal` wrapping `ErrPartialWrite`. If Save fails before any write happens, just kill the new container and restore old. |
| 8a | `caddy.WriteStubIfMissing(paths.CaddyfilePath)`. | If stub write fails, log, continue. |
| 8b | Generate new Caddyfile from `Store.List(ctx)` to a tmp file via `caddy.Generator.Generate(tmpPath, inputs)`. **Then `Reloader.Validate(ctx, tmpPath)` — wraps `caddy validate --config <tmp>`.** If Validate fails, log error including stderr from `caddy validate`, return `errCaddyReload`. **The OLD Caddyfile on disk is untouched.** Then atomic-rename tmp → real Caddyfile path. | Pre-validation guarantees the on-disk file is reload-able. If atomic-rename itself fails, return `errCaddyReload`; old file still on disk. |
| 8c | `caddy.Reloader.Reload(ctx, paths.CaddyfilePath)`. | If reload fails (rare; usually a runtime issue like port-already-bound), log warning, return `errCaddyReload`. The new Caddyfile IS on disk and reflects the new state; Caddy is still serving the old config in memory. Recovery procedure documented in `_docs/operator/usage.md` §6 per Don §2.2.2. |

**Forward-only at step 8.** After registry save (7b) succeeds, only Caddy reload can fail without rolling back the new container. Steps 1–7 are rollback-on-failure.

**Why pre-validation closes Linus Hole #2:** v1's failure mode — atomic-write a syntactically-broken Caddyfile, then reload fails, then on next host reboot Caddy refuses to start — is now impossible for syntax errors. The validate step catches generator bugs before the rename. Runtime errors (port-already-bound, certificate provisioning, etc.) still possible at reload time, but those leave a syntactically-VALID file on disk; the operator's recovery path (read Caddy's error log, fix the runtime issue, re-run `decloud caddy reload`) works against a valid file.

### 9.3 `restoreOldContainer` — the rollback primitive

UNCHANGED from v1.

```go
func (d *serviceDeployer) restoreOldContainer(ctx context.Context, prev *registry.Service) {
    if prev == nil {
        return
    }
    runReq := dockerdrv.RunRequest{
        Name:    ids.ContainerName(prev.Config.Name),
        Image:   prev.Config.Build.ImageRef,
        Network: "decloud",
        Env:     prev.Secrets.Env,
        Restart: prev.Config.Run.Restart,
        Port:    prev.Config.Run.Port,
    }
    if _, err := d.deps.Driver.Run(ctx, runReq); err != nil {
        slog.Error("rollback: failed to restart previous container",
            "service", prev.Config.Name, "error", err)
        return
    }
    slog.Info("rollback: previous container restored", "service", prev.Config.Name)
}
```

Same correctness points as v1: previous image must still be in local Docker cache (M6 GC window is wide); we do NOT re-run readiness on the rollback (previous container was previously healthy); the new failed container is removed BEFORE this call.

### 9.4 `internal/deploy/readiness.go` — REWRITTEN

The v1 §9.4 contradictory three-options text is GONE. Replaced with the single answer.

Probes the new container by HTTP GET from the host process. Container IP discovered via `Driver.ContainerIP(ctx, name) (string, error)` (NEW driver method, see §11.1).

```go
package deploy

import (
    "context"
    "errors"
    "fmt"
    "net/http"
    "time"

    "github.com/alexander-fenster/decloud/internal/dockerdrv"
    "github.com/alexander-fenster/decloud/internal/registry"
)

type readinessProbe interface {
    Wait(ctx context.Context, containerName string, spec registry.ReadinessSpec, port int) error
}

type httpProbe struct {
    client *http.Client
    driver dockerdrv.Driver // injected for ContainerIP lookup
}

func newHTTPProbe(driver dockerdrv.Driver) *httpProbe {
    return &httpProbe{
        client: &http.Client{Timeout: 2 * time.Second},
        driver: driver,
    }
}

func (p *httpProbe) Wait(ctx context.Context, containerName string, spec registry.ReadinessSpec, port int) error {
    if spec.IntervalSecs <= 0 {
        spec.IntervalSecs = 2
    }
    if spec.TimeoutSecs <= 0 {
        spec.TimeoutSecs = 60
    }
    deadline := time.Now().Add(time.Duration(spec.TimeoutSecs) * time.Second)
    ticker := time.NewTicker(time.Duration(spec.IntervalSecs) * time.Second)
    defer ticker.Stop()
    for {
        // Re-resolve IP per-tick: if container restarts mid-probe, Docker reassigns IP.
        // Also handles the race between docker run returning and the container being
        // attached to the network (ContainerIP may briefly return ErrNoBridgeIP).
        ip, ipErr := p.driver.ContainerIP(ctx, containerName)
        if ipErr == nil && ip != "" {
            url := fmt.Sprintf("http://%s:%d%s", ip, port, spec.HTTPPath)
            if err := p.probe(ctx, url); err == nil {
                return nil
            }
        }
        if time.Now().After(deadline) {
            if ipErr != nil {
                return fmt.Errorf("readiness: %v: %w", ipErr, errReadiness)
            }
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

**`Driver.ContainerIP(ctx, name)` implementation** (in `internal/dockerdrv/cli_driver.go`): wraps
```
docker inspect <name> --format '{{ .NetworkSettings.Networks.decloud.IPAddress }}'
```
Returns the trimmed stdout. Empty string → return `ErrNoBridgeIP` (NEW sentinel in `internal/dockerdrv/driver.go`).

**Why per-tick re-resolution rather than caching:** the IP CAN change if the container restarts (Docker reassigns); for a one-deploy probe within seconds this is unlikely, but re-inspecting per-tick is one syscall per second and removes the cache-invalidation question entirely.

**Why this works on the maintainer's macOS Docker Desktop dev box:** the default `bridge` network driver makes container IPs reachable from the host's network namespace on Linux. Docker Desktop on macOS supports this via its VM bridge — the bridge network's gateway IP is reachable from the host. The `decloud` network MUST use the default bridge driver (no `--driver` flag), which `_docs/operator/installation.md` step 5 mandates per Don §2.2.1. If a future operator creates the network with `--driver=macvlan` or some other driver where host-to-container IP is unreachable, readiness will fail; the install doc's explicit constraint plus the `decloud network ensure` self-heal (which doesn't pass `--driver`) makes the wrong-driver case unreachable through normal use.

**The `OneShotProbe` driver method from v1 §11.1 is REMOVED.** No `curlimages/curl`. No `docker run --rm`. Joel deletes the method from the `Driver` interface and the corresponding test (`TestCLIDriver_OneShotProbeArgs` is GONE).

### 9.5 `Store.Save` partial-write detection + `DeleteOrphanConfig` (RENAMED)

Per prior `06-tech-plan-v2.md` §4.5–§4.6, the two-file Save is config-first then secrets. The orchestrator needs to know if Save partially wrote (config landed, secrets failed) vs failed before any write.

Add a sentinel:

```go
// in internal/registry/errors.go
var ErrPartialWrite = errors.New("registry: partial write (config wrote, secrets failed)")
```

`fsStore.Save` body:

```go
func (s *fsStore) Save(ctx context.Context, svc *Service) error {
    cfgPath := filepath.Join(s.paths.ServicesDir, svc.Config.Name+".toml")
    secPath := filepath.Join(s.paths.SecretsDir, svc.Config.Name, "env.toml")
    if err := s.validateForSave(svc); err != nil {
        return err
    }
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

**RENAMED `DeleteOrphanConfig`** (was `RollbackPartialCreate` in v1; per Linus answer #1 / Don §4.1):

```go
func (s *fsStore) DeleteOrphanConfig(ctx context.Context, name string) error {
    cfgPath := filepath.Join(s.paths.ServicesDir, name+".toml")
    if err := os.Remove(cfgPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
        return err
    }
    return nil
}
```

Updated `Store` interface:

```go
type Store interface {
    Load(ctx context.Context, name string) (*Service, error)
    Save(ctx context.Context, svc *Service) error
    DeleteOrphanConfig(ctx context.Context, name string) error // RENAMED from RollbackPartialCreate
    List(ctx context.Context) ([]*Service, error)
    Delete(ctx context.Context, name string) error
}
```

Caller (in `service.go` step 7b error branch):
```go
if errors.Is(err, registry.ErrPartialWrite) {
    if rbErr := d.deps.Store.DeleteOrphanConfig(ctx, req.Name); rbErr != nil {
        slog.Error("rollback: failed to delete orphan config", "service", req.Name, "error", rbErr)
    }
    // ... then container rollback ...
}
```

Test names updated:
- `TestStore_DeleteOrphanConfigRemovesConfig` (was `TestStore_RollbackPartialCreateRemovesConfig`)
- `TestStore_DeleteOrphanConfigIsIdempotent` (was `TestStore_RollbackPartialCreateIsIdempotent`)

### 9.6 Lifecycle method specifications (NEW per Linus Hole #1 / Don §3.1)

Lives in `internal/deploy/lifecycle.go`. All seven methods are receiver methods on `*serviceDeployer` (the same struct that implements `Deploy`). They share `d.deps`.

A common private helper used by `Unregister` and `CaddyReload` (and reused by Deploy step 8 logic):

```go
// regenerateAndReload regenerates the Caddyfile from Store.List, validates the
// tmp file via caddy validate, atomically renames into place, and reloads
// Caddy. On any failure, returns an error wrapped with errCaddyReload. The
// previous Caddyfile on disk is preserved on validate failure (because the
// rename happens after validate succeeds).
func (d *serviceDeployer) regenerateAndReload(ctx context.Context) error {
    services, err := d.deps.Store.List(ctx)
    if err != nil {
        return fmt.Errorf("listing services: %w", err)
    }
    if err := caddy.WriteStubIfMissing(d.deps.Paths.CaddyfilePath); err != nil {
        slog.Warn("caddy stub write failed", "error", err)
    }
    tmpPath := d.deps.Paths.CaddyfilePath + ".tmp"
    if err := d.deps.Generator.Generate(tmpPath, services); err != nil {
        return fmt.Errorf("generating caddyfile: %w: %v", errCaddyReload, err)
    }
    if err := d.deps.Reloader.Validate(ctx, tmpPath); err != nil {
        _ = os.Remove(tmpPath)
        return fmt.Errorf("caddy validate failed: %w: %v", errCaddyReload, err)
    }
    if err := os.Rename(tmpPath, d.deps.Paths.CaddyfilePath); err != nil {
        return fmt.Errorf("rename caddyfile: %w: %v", errCaddyReload, err)
    }
    if err := d.deps.Reloader.Reload(ctx, d.deps.Paths.CaddyfilePath); err != nil {
        return fmt.Errorf("caddy reload failed: %w: %v", errCaddyReload, err)
    }
    return nil
}
```

Used by `Unregister` (step 5–8), `CaddyReload`, and the Deploy orchestrator's step 8a–8c block (Joel: refactor the inline Deploy step-8 to call this helper for consistency).

#### §9.6.1 `Unregister(ctx, name string) error`

**Purpose:** full removal of a registered service. Stops the container, removes it, deletes both registry files (secrets-first per prior `06-tech-plan-v2.md` §4.7), regenerates the Caddyfile, validates, reloads.

**Signature:**
```go
func (d *serviceDeployer) Unregister(ctx context.Context, name string) error
```

**Step sequence:**

1. `prev, err := d.deps.Store.Load(ctx, name)`.
   - `errors.Is(err, registry.ErrNotFound)` → return wrapped (operator sees `ExitConfigError`).
   - `errors.Is(err, registry.ErrSecretsMissing)` → treat as "config-only orphan"; proceed with steps 2–4 anyway, skipping anything that needs secrets (there's nothing in this path that does).
   - other errors → return wrapped (typically `ErrPermissionMode` → `ExitConfigError`).
2. `_ = d.deps.Driver.Stop(ctx, ids.ContainerName(name), 10*time.Second)`. Idempotent — ignore "no such container" specifically (operator may have removed the container manually). The driver returns `ErrContainerNotFound` for that case; we swallow it.
3. `_ = d.deps.Driver.Remove(ctx, ids.ContainerName(name))`. Same idempotence on `ErrContainerNotFound`.
4. `if err := d.deps.Store.Delete(ctx, name); err != nil { return wrapped }`. Per prior `06-tech-plan-v2.md` §4.7, `Delete` removes secrets FIRST, then config. Forward-only after this — if `Delete` fails partway (secrets removed but config write-protected), the registry is in a partial state but the container is already gone, so operator runs `decloud unregister` again until clean.
5. `return d.regenerateAndReload(ctx)`. The just-removed service is now absent from `Store.List(ctx)`; the regenerated Caddyfile drops its stanza.

**Rollback semantics:** forward-only after step 4. Steps 2–3 are idempotent so partial failure there means re-run; step 4 commits the registry change before the Caddy change, mirroring the deploy orchestrator's "registry-then-Caddy" ordering. If step 5's validate or reload fails, the registry is already correct; the operator follows `_docs/operator/usage.md` §6 (Don §2.2.2) to recover Caddy.

**Error sentinels and exit codes:**
- `registry.ErrNotFound` (from step 1) → `ExitConfigError` (10)
- `registry.ErrPermissionMode` etc. (from step 1) → `ExitConfigError` (10)
- step 4 returns Store errors wrapped → `ExitConfigError` typically; if it's an unexpected error, `ExitInternal`
- step 5 wraps with `errCaddyReload` → `ExitCaddyReloadFail` (60)

**Operator-visible output:** stdout silent on success (Unix convention); stderr gets the slog JSON output that always goes there. Logger emits `lifecycle_op=unregister`, `service=<name>`, `step=stop|remove|delete_registry|caddy_validate|caddy_reload`, `duration_ms=N`.

**Tests Kent writes (in `internal/deploy/lifecycle_test.go`):**
- `TestLifecycle_UnregisterHappyPath` — Store.Load returns prev; Driver.Stop, Driver.Remove, Store.Delete, Generator.Generate, Reloader.Validate, Reloader.Reload all succeed in order (`gomock.InOrder`).
- `TestLifecycle_UnregisterServiceNotFoundReturnsErrNotFound` — Store.Load returns `ErrNotFound`; assert error wraps it; Driver.Stop NOT called.
- `TestLifecycle_UnregisterContinuesIfContainerAlreadyGone` — Store.Load returns prev; Driver.Stop returns `ErrContainerNotFound`; Driver.Remove returns `ErrContainerNotFound`; Store.Delete still called; succeeds.
- `TestLifecycle_UnregisterConfigOnlyOrphanProceeds` — Store.Load returns `ErrSecretsMissing`; assert Driver.Stop + Driver.Remove + Store.Delete + regenerate are still called.
- `TestLifecycle_UnregisterCaddyValidateFailureReturnsErrCaddyReload` — registry already deleted; Reloader.Validate returns error; assert returned error wraps `errCaddyReload`; assert Reloader.Reload NOT called.
- `TestLifecycle_UnregisterCaddyReloadFailureReturnsErrCaddyReload` — Reloader.Validate succeeds; Reloader.Reload fails; assert wrapped error.

#### §9.6.2 `Stop(ctx, name string) error`

**Purpose:** graceful container halt without unregistering.

**Signature:**
```go
func (d *serviceDeployer) Stop(ctx context.Context, name string) error
```

**Step sequence:**

1. `if err := d.deps.Driver.Stop(ctx, ids.ContainerName(name), 10*time.Second); err != nil { ... }`.
   - `errors.Is(err, dockerdrv.ErrContainerNotFound)` → return `registry.ErrNotFound` wrapped (operator's container is already gone).
   - other errors → return wrapped with `errRun`.
2. **No registry mutation.** Per Don §3.1.2: container state is queried live via `docker inspect`, not persisted. `decloud status foo` after `decloud stop foo` reads docker, sees "exited," reports "stopped."
3. **No Caddy reload.** A stopped container is unreachable on the network; Caddy returns 502 for routes pointing at it. That's correct user-visible behavior.

**Error sentinels and exit codes:**
- `registry.ErrNotFound` → `ExitConfigError` (10)
- `errRun` → `ExitRunFail` (40) for unexpected docker errors

**Tests:**
- `TestLifecycle_StopHappyPath` — Driver.Stop called with `(name, 10s)`, returns nil.
- `TestLifecycle_StopAlreadyStoppedIsIdempotent` — Driver.Stop returns nil for already-stopped container (docker stop is idempotent on already-stopped).
- `TestLifecycle_StopServiceNotFoundReturnsErrNotFound` — Driver.Stop returns `ErrContainerNotFound`; assert returned error wraps `registry.ErrNotFound`.
- `TestLifecycle_StopUnexpectedDriverErrorReturnsErrRun` — Driver.Stop returns some other error; assert wraps `errRun`.

#### §9.6.3 `Start(ctx, name string) error`

**Purpose:** restart a stopped container, or re-run from registry if the container was removed.

**Signature:**
```go
func (d *serviceDeployer) Start(ctx context.Context, name string) error
```

**Step sequence:**

1. `prev, err := d.deps.Store.Load(ctx, name)`.
   - `ErrNotFound` → return wrapped (`ExitConfigError`).
   - `ErrSecretsMissing` → return wrapped (`ExitConfigError`); operator should `decloud unregister` then redeploy.
   - other Load errors → return wrapped.
2. `inspect, err := d.deps.Driver.Inspect(ctx, ids.ContainerName(name))`.
   - `inspect.State == "running"` → no-op, return nil. Idempotent.
   - `inspect.State == "exited"` → `d.deps.Driver.Start(ctx, ids.ContainerName(name))` (NEW driver method per §11.1). Container resumes with previous env baked in (Docker preserves env on stopped containers).
   - `inspect.State == "absent"` → re-`docker run` from `prev.Config.Build.ImageRef` with `prev.Secrets.Env`:
     ```go
     _, err := d.deps.Driver.Run(ctx, dockerdrv.RunRequest{
         Name:    ids.ContainerName(name),
         Image:   prev.Config.Build.ImageRef,
         Network: "decloud",
         Env:     prev.Secrets.Env,
         Restart: prev.Config.Run.Restart,
         Port:    prev.Config.Run.Port,
     })
     ```
     If image is no longer in local cache (M6 GC removed it), `Driver.Run` fails with the docker error; return wrapped `errRun`. **Start does NOT rebuild from source** — that's `decloud deploy service`'s job.
3. **No Caddy reload.** Caddy already routes to this container's name on the shared network.

**Error sentinels and exit codes:**
- `registry.ErrNotFound`, `registry.ErrSecretsMissing` → `ExitConfigError` (10)
- `errRun` (Driver.Start, Driver.Run failures) → `ExitRunFail` (40)

**Tests:**
- `TestLifecycle_StartFromExited` — Inspect returns exited; assert Driver.Start called with `ContainerName(name)`.
- `TestLifecycle_StartFromAbsentReRunsContainer` — Inspect returns absent; assert Driver.Run called with the RunRequest derived from prev.Config and prev.Secrets.
- `TestLifecycle_StartFromRunningIsNoOp` — Inspect returns running; assert no Driver.Start, no Driver.Run.
- `TestLifecycle_StartServiceNotFoundReturnsErrNotFound` — Store.Load returns ErrNotFound; assert wrapped.
- `TestLifecycle_StartSecretsMissingReturnsErrSecretsMissing` — Store.Load returns ErrSecretsMissing; assert wrapped.
- `TestLifecycle_StartImageMissingReturnsErrRun` — Driver.Run returns "no such image" error; assert wraps `errRun`.

#### §9.6.4 `Restart(ctx, name string) error`

**Purpose:** stop-then-start, preserving the container.

**Signature:**
```go
func (d *serviceDeployer) Restart(ctx context.Context, name string) error
```

**Step sequence:**

1. `if err := d.Stop(ctx, name); err != nil && !errors.Is(err, registry.ErrNotFound) { return err }`. Tolerate "no such container" — that's fine, container is absent, Start will re-run.
2. `return d.Start(ctx, name)`.

**Why stop-then-start, not `docker restart`:** lets us reuse the existing methods and gives the operator the same 10s grace period as a deploy. `docker restart` is a single command but its grace handling is implicit; ours is explicit. Per Don §3.1.4.

**NOT recreate.** Operator who wants a fresh container should re-run `decloud deploy service`.

**Error sentinels and exit codes:** inherited from Stop and Start.

**Tests:**
- `TestLifecycle_RestartHappyPath` — `gomock.InOrder` on Driver.Stop then Driver.Inspect (returns exited) then Driver.Start.
- `TestLifecycle_RestartFromAbsentReturnsErrNotFound` — Stop returns ErrContainerNotFound (tolerated as ErrNotFound), then Start's Inspect returns absent and Driver.Run fails (no image); assert errRun returned. (Or, more usefully: `TestLifecycle_RestartFromAbsentReRunsContainer` — Stop tolerated, Start re-runs from registry; assert Driver.Run called.)
- `TestLifecycle_RestartStopFailureAbortsBeforeStart` — Driver.Stop returns a real (non-ErrContainerNotFound) error; assert Start NOT called; assert returned error wraps Stop's.

#### §9.6.5 `Status(ctx, name string) (Status, error)`

**Purpose:** runtime + registry view.

**Signature:**
```go
func (d *serviceDeployer) Status(ctx context.Context, name string) (Status, error)
```

**Step sequence:**

1. `prev, err := d.deps.Store.Load(ctx, name)`.
   - `ErrNotFound` → return `Status{}, wrapped`.
   - `ErrSecretsMissing` → return `Status{Name: name, State: "config-only", LastDeployedAt: time.Time{}}, nil` (operator sees the orphan and can `decloud unregister` to clean up).
   - other errors → propagate wrapped.
2. `inspect, err := d.deps.Driver.Inspect(ctx, ids.ContainerName(name))`.
   - `inspect.State == "absent"` → `Status{State: "absent"}` (with the registry fields populated below).
   - `inspect.State == "exited"` → `Status{State: "stopped"}`.
   - `inspect.State == "running"` → `Status{State: "running"}`.
3. Populate Status fields:
   - `Name`: `prev.Config.Name`
   - `ContainerID`: `inspect.ContainerID` (empty if absent)
   - `ContainerName`: `ids.ContainerName(name)`
   - `State`: per step 2
   - `LastDeployID`: parse the tag from `prev.Config.Build.ImageRef` (last colon-delimited component)
   - `LastDeployedAt`: from `prev.Config.LastDeployedAt`. **VERIFICATION NEEDED:** prior `06-tech-plan-v2.md` §4.2 — if `LastDeployedAt time.Time` is not yet a field on `ServiceConfig`, ADD it to the struct. The Save path (Deploy step 7b) sets it to `time.Now().UTC()` before marshaling. Joel: confirmed this is a small ServiceConfig extension; flag for Linus as "extension to prior tech-plan §4.2 type definition." If Linus rejects the extension, fall back to `LastDeployedAt: time.Time{}` (zero-value — operator sees no useful "deployed at" but Status still works).

**Output to stdout:** see `internal/cli/status.go` in §8.3 — single-line human-readable format. M1 ships only that; `--json` is M1.5 if anyone asks.

**Error sentinels and exit codes:**
- `registry.ErrNotFound` → `ExitConfigError` (10)
- Inspect errors → `errRun` (`ExitRunFail`) — though `Inspect` on an absent container returns `State: "absent"` rather than an error per the §11.1 contract, so this is rare.

**Tests:**
- `TestLifecycle_StatusRunning` — Load + Inspect (running) → Status with State="running".
- `TestLifecycle_StatusStopped` — Load + Inspect (exited) → State="stopped".
- `TestLifecycle_StatusAbsentContainer` — Load + Inspect (absent) → State="absent".
- `TestLifecycle_StatusConfigOnlyOrphan` — Load returns ErrSecretsMissing → State="config-only", err nil.
- `TestLifecycle_StatusServiceNotFoundReturnsErrNotFound` — Load returns ErrNotFound; assert wrapped error.

#### §9.6.6 `Logs(ctx, name string, opts LogOptions) error`

**Purpose:** stream container logs.

**Signature:**
```go
func (d *serviceDeployer) Logs(ctx context.Context, name string, opts LogOptions) error
```

**Step sequence:**

1. `return d.deps.Driver.Logs(ctx, ids.ContainerName(name), dockerdrv.LogsOptions{Follow: opts.Follow, Tail: opts.Tail, Stdout: d.deps.Stdout, Stderr: d.deps.Stderr})`. Pass-through to `docker logs <name> [-f] [--tail N]`. Stdout/Stderr come from the Dependencies (defaulting to os.Stdout/os.Stderr in production; tests inject buffers).
2. No registry interaction. If the container doesn't exist, `docker logs` exits non-zero with its own error message; the driver returns `dockerdrv.ErrContainerNotFound` and the deployer wraps as `registry.ErrNotFound`.

**Error sentinels and exit codes:**
- `registry.ErrNotFound` → `ExitConfigError` (10)
- other docker errors → `errRun` → `ExitRunFail`

**Tests:**
- `TestLifecycle_LogsTailN` — opts.Tail=50; assert Driver.Logs called with `LogsOptions{Tail: 50}`.
- `TestLifecycle_LogsFollow` — opts.Follow=true; assert Driver.Logs called with `LogsOptions{Follow: true}`.
- `TestLifecycle_LogsServiceNotFoundReturnsErrNotFound` — Driver.Logs returns ErrContainerNotFound; assert wrapped as `registry.ErrNotFound`.

#### §9.6.7 `CaddyReload(ctx) error`

**Purpose:** regenerate-from-registry then reload. The operator's "I edited something out-of-band" escape hatch.

**Signature:**
```go
func (d *serviceDeployer) CaddyReload(ctx context.Context) error
```

**Step sequence:**

1. `return d.regenerateAndReload(ctx)` — uses the shared helper defined above. List → stub → generate → validate → rename → reload, all per §9.2 step 8 semantics.

**Error sentinels and exit codes:**
- `errCaddyReload` → `ExitCaddyReloadFail` (60)

**Tests:**
- `TestLifecycle_CaddyReloadHappyPath` — Store.List + Generator.Generate + Reloader.Validate + Reloader.Reload all succeed in order.
- `TestLifecycle_CaddyReloadValidateFailureLeavesOldFileIntact` — Reloader.Validate fails; assert tmp file removed (or assert os.Rename NOT called); assert Reloader.Reload NOT called; assert returned error wraps `errCaddyReload`.
- `TestLifecycle_CaddyReloadReloadFailureNewFileOnDisk` — Validate succeeds; rename succeeds; Reload fails; assert returned error wraps `errCaddyReload`. (No undo of the rename — the new file IS on disk per §9.2 step 8c semantics.)
- `TestLifecycle_CaddyReloadEmptyRegistryWritesStubOnly` — Store.List returns empty slice; assert Generator.Generate is still called (with empty input → header-only output); assert reload still called.
- `TestLifecycle_CaddyReloadStoreListFailurePropagates` — Store.List returns error; assert wrapped error returned.

---

## 10. Loader rejection of `mounts` — exact mechanics (UNCHANGED from v1)

VERIFIED still consistent with v2. No changes here. Reproduced for completeness so Kent and Rob don't have to bounce between documents.

### 10.1 The check

In `internal/registry/store.go`'s `Load`, after `DisallowUnknownFields()` decode succeeds:

```go
if len(cfg.Run.Mounts) > 0 {
    return nil, fmt.Errorf("%w: service %q declares %d mount(s) in %s; mounts are not supported until M3",
        ErrMountsNotSupported, cfg.Name, len(cfg.Run.Mounts), cfgPath)
}
```

`ErrMountsNotSupported` in `internal/registry/errors.go`:

```go
var ErrMountsNotSupported = errors.New("registry: mounts not supported in M1")
```

Exit code mapping in `internal/cli/exit_codes.go`: `errors.Is(err, registry.ErrMountsNotSupported)` → `ExitConfigError` (10).

Operator-visible error string:
```
Error: registry: mounts not supported in M1: service "foo" declares 2 mount(s) in /opt/declouding/config/services/foo.toml; mounts are not supported until M3
```

### 10.2 Empty array IS accepted

`mounts = []` in the TOML is accepted. `len(cfg.Run.Mounts) > 0` evaluates to false on both "field absent" and "field present, empty array." M3 starts writing real entries here without any schema bump.

### 10.3 The CLI flag mirrors the rejection

`internal/cli/deploy_service.go` `runDeployService`:

```go
if len(f.Mounts) > 0 {
    return fmt.Errorf("--mount is not supported until M3: %w", registry.ErrMountsNotSupported)
}
```

Same wrapped sentinel → same exit code.

### 10.4 Tests (in `internal/registry/store_test.go` plus `internal/cli/deploy_service_test.go`)
- `TestStore_LoadAcceptsEmptyMountsArray`
- `TestStore_LoadRejectsNonEmptyMounts`
- `TestDeployService_MountFlagReturnsErrMountsNotSupported`

---

## 11. `internal/dockerdrv/` — driver shape and arg-construction tests

Don's plan §5 specifically called out the dockerdrv argument-construction discipline. v2 ADDS `ContainerIP` and `Start`; REMOVES `OneShotProbe`.

### 11.1 The `Driver` interface (UPDATED)

```go
package dockerdrv

import (
    "context"
    "errors"
    "io"
    "time"
)

// ErrContainerNotFound is returned by Stop, Remove, Start, Logs, ContainerIP
// when the named container does not exist. Lifecycle methods (§9.6) treat
// this as idempotent for some operations (Stop, Remove during Unregister)
// and as registry.ErrNotFound for others (Logs, Stop standalone).
var ErrContainerNotFound = errors.New("dockerdrv: container not found")

// ErrNoBridgeIP is returned by ContainerIP when docker inspect returns an
// empty IP string. Typically a transient race between docker run completing
// and the container being attached to the network; the readiness probe
// handles this by re-resolving on the next tick.
var ErrNoBridgeIP = errors.New("dockerdrv: container has no bridge network IP")

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

type LogsOptions struct {
    Follow bool
    Tail   int
    Stdout io.Writer
    Stderr io.Writer
}

type Driver interface {
    Build(ctx context.Context, req BuildRequest) (imageID string, err error)
    Run(ctx context.Context, req RunRequest) (containerID string, err error)
    Stop(ctx context.Context, containerName string, gracePeriod time.Duration) error
    Start(ctx context.Context, containerName string) error                  // NEW per §9.6.3
    Remove(ctx context.Context, containerName string) error
    Inspect(ctx context.Context, containerName string) (InspectResult, error)
    Logs(ctx context.Context, containerName string, opts LogsOptions) error
    NetworkEnsure(ctx context.Context, networkName string) error
    ContainerIP(ctx context.Context, containerName string) (string, error)  // NEW per §9.4
    // OneShotProbe REMOVED per §9.4.
}
```

### 11.2 The injectable command factory (UNCHANGED from v1)

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
        return exec.CommandContext(ctx, "true") // no-op exit 0
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

(Port is NOT exposed via `-p` because Caddy reaches the container over the shared network. Per Don §2.2.2 step 6, the operator-facing doc covers the `docker exec` path for direct probing.)

Each test file's top comment includes the hand-typed equivalent `docker` command per Don §5 (e.g., for ContainerIP: `# docker inspect <name> --format '{{ .NetworkSettings.Networks.decloud.IPAddress }}'`).

### 11.3 Why this shape is the right answer for Knuth-review (UNCHANGED from v1)

The injectable factory pattern is well-known but Rob may want a sanity check on:
- **Map ordering for `--env`.** Sort env keys lexically before constructing args. Rob: sort in implementation, not in tests.
- **Whether to use `--env-file` instead of multiple `--env`.** Multiple `--env` is simpler and avoids leaving an env file on disk that contains secrets. M1 chooses multiple `--env`. Document.
- **Stdout/stderr handling in `Build`.** Pass `req.Stdout`/`req.Stderr` to `cmd.Stdout`/`cmd.Stderr` in `Build` only; for other commands, capture to in-process buffers for error-message inclusion.

If any of these feels off, call Knuth before implementing.

### 11.4 New driver method specifics

**`Driver.Start(ctx, containerName) error`** (per §9.6.3):
- Wraps `docker start <containerName>`.
- On `docker start` exit 1 with stderr containing "No such container", return `ErrContainerNotFound`.
- Otherwise wrap the error with the captured stderr.

**`Driver.ContainerIP(ctx, containerName) (string, error)`** (per §9.4):
- Wraps `docker inspect <containerName> --format '{{ .NetworkSettings.Networks.decloud.IPAddress }}'`.
- On `docker inspect` exit 1 with stderr containing "No such" / "Error: No such object", return `ErrContainerNotFound`.
- On exit 0 with empty trimmed stdout, return `ErrNoBridgeIP`.
- On exit 0 with non-empty stdout, return `strings.TrimSpace(stdout), nil`.

---

## 12. `internal/ids/` — deploy IDs and container names (UNCHANGED from v1)

```go
package ids

import (
    "crypto/rand"
    "encoding/hex"
    "fmt"
    "time"
)

func NewDeployID() string {
    now := time.Now().UTC()
    var b [3]byte
    _, _ = rand.Read(b[:])
    return fmt.Sprintf("%s-%s-%s", now.Format("20060102"), now.Format("150405"), hex.EncodeToString(b[:]))
}

func ContainerName(serviceName string) string {
    return "decloud-" + serviceName
}

func ImageRef(serviceName, deployID string) string {
    return "decloud-" + serviceName + ":" + deployID
}
```

Tests: format-stability regex match, uniqueness across rapid calls (1000 IDs, no collision).

---

## 13. Test plan — package-by-package checklist for Kent

### 13.1 `internal/registry`

Per prior `06-tech-plan-v2.md` §12.1 row 1, plus deltas:
- `TestStore_RoundTripConfigAndSecrets`
- `TestStore_LoadRejectsUnknownConfigField`
- `TestStore_LoadRejectsUnknownSecretsField`
- `TestStore_LoadRejectsConfigSchemaMismatch` — `schema_version = 2` → `ErrSchemaMismatch`.
- `TestStore_LoadRejectsCrossFileSchemaMismatch` — config v1, secrets v2 → `ErrSchemaMismatch`.
- `TestStore_LoadRejectsSecretsFileMode0644`
- `TestStore_LoadRejectsSecretsDirMode0755`
- `TestStore_LoadConfigWithoutSecretsReturnsErrSecretsMissing`
- `TestStore_LoadAcceptsEmptyMountsArray` (per §10.2)
- `TestStore_LoadRejectsNonEmptyMounts`
- `TestStore_LoadRejectsInvalidStrategy`
- `TestStore_SaveOrderConfigBeforeSecrets`
- `TestStore_DeleteOrderSecretsBeforeConfig`
- `TestStore_SaveAtomicityConfigWriteFails`
- `TestStore_SaveReturnsErrPartialWriteOnSecretsFailure` (per §9.5)
- `TestStore_DeleteOrphanConfigRemovesConfig` (RENAMED per §9.5)
- `TestStore_DeleteOrphanConfigIsIdempotent` (RENAMED per §9.5)
- `TestStore_ListSkipsMalformedFiles`
- `TestStore_SaveSetsCorrectFilePermissions`

### 13.2 `internal/envcap`

Runs against real `/bin/bash`. No build-tag skip. Uses `t.TempDir()` to write the test scripts.

Per prior `06-tech-plan-v2.md` §3.5, Kent enumerates each row in the portability table as a `Test_...` function. **In addition to (and not instead of) those, the following three explicit edge-case tests MUST be present** (per Don §4.3 / Linus Hole #4):

- `TestEnvcap_SetAOff_VariablesDropped` — script does `export FOO=before; set +a; BAR=after`. Capture should include `FOO` but NOT `BAR`. Lock in the failure mode with a test, not just documentation.
- `TestEnvcap_ArrayDeclaration_OnlyFirstElementCaptured` — script does `MY_ARR=(a b c); export MY_ARR`. Assert capture behavior: bash 3.2 exposes `MY_ARR` as the first element via `compgen -e` — assert `result["MY_ARR"] == "a"` (or whatever the actual behavior is on the maintainer's box; test discovers and locks in).
- `TestEnvcap_ReadonlyConflict_FailsWithSetE` — script does `readonly FOO=bar; FOO=baz`. With `set -e` in the captor's bash invocation, the second assignment fails; the captor exits non-zero. Assert returned error wraps `errEnvCapture`.

Kent verifies that none of the three names duplicate names from §3.5; if there's overlap, keep the §3.5 name (that table is canonical) and mark the duplicate as covered.

### 13.3 `internal/caddy`

Per prior `06-tech-plan-v2.md` §12.1 row 3, plus deltas:
- `TestGenerator_OneServiceOneHost` — golden-string equality.
- `TestGenerator_MultiServiceMultiHost` — sorted output, deterministic.
- `TestGenerator_DropsZeroHostnameServices`
- `TestGenerator_EmptyInputProducesHeaderOnly`
- `TestStub_WritesByteExactStubWhenAbsent`
- `TestStub_NoOpWhenFileExists`
- `TestReloader_InvokesCaddyReload` — recording cmdFactory; assert `caddy reload --config <path>`.
- `TestReloader_InvokesCaddyValidate` — recording cmdFactory; assert `caddy validate --config <path>`. **(NEW per §9.2 step 8b)**
- `TestReloader_ValidateFailureReturnsError` — caddy validate exits 1 with stderr; assert returned error wraps the stderr text. **(NEW)**

### 13.4 `internal/dockerdrv`

All argument-construction. Per §11.2 of this plan. One test per method:
- `TestCLIDriver_BuildArgs`
- `TestCLIDriver_RunArgsWithEnvSorted` — env vars in lexical order.
- `TestCLIDriver_RunArgsWithEmptyEnv` — no `--env` flags when env is empty.
- `TestCLIDriver_StopArgs` — `docker stop -t 10 <name>`.
- `TestCLIDriver_StartArgs` — `docker start <name>`. **(NEW per §11.4)**
- `TestCLIDriver_RemoveArgs` — `docker rm <name>`.
- `TestCLIDriver_InspectArgsAndParse`
- `TestCLIDriver_LogsArgsFollow`, `TestCLIDriver_LogsArgsTailN`
- `TestCLIDriver_NetworkEnsureWhenAbsent`
- `TestCLIDriver_NetworkEnsureWhenPresent`
- `TestCLIDriver_ContainerIPArgsAndParse` — assert recorded args (`docker inspect <name> --format '{{ .NetworkSettings.Networks.decloud.IPAddress }}'`); inject a fake stdout `"172.18.0.5\n"` and assert returned `"172.18.0.5"`. **(NEW per §9.4 / §11.4)**
- `TestCLIDriver_ContainerIPEmptyReturnsErrNoBridgeIP` — fake stdout empty; assert `errors.Is(err, ErrNoBridgeIP)`. **(NEW)**
- `TestCLIDriver_ContainerIPNotFoundReturnsErrContainerNotFound` — fake stderr "No such container"; assert `errors.Is(err, ErrContainerNotFound)`. **(NEW)**

`TestCLIDriver_OneShotProbeArgs` from v1 is REMOVED.

Each test file's top comment includes the hand-typed equivalent `docker` invocation per Don §5.

### 13.5 `internal/deploy` (Deploy + Readiness)

Gomock-driven. One test per failure branch, one happy path. Each uses `gomock.InOrder(...)` to assert exact step ordering.

Deploy:
- `TestDeploy_HappyPathFirstDeploy` — no prev; envcap → build → run → readiness → save → caddy validate → caddy rename → caddy reload.
- `TestDeploy_HappyPathRedeploy` — prev exists; envcap → build → stop old → remove old → run new → readiness → save → caddy validate → caddy rename → caddy reload.
- `TestDeploy_LoadPreviousErrSecretsMissingTreatedAsFirstDeploy` — prev exists as config-only; treat as first deploy.
- `TestDeploy_EnvCaptureFailureAbortsBeforeAnythingChanges`
- `TestDeploy_BuildFailureAbortsBeforeStoppingOld`
- `TestDeploy_StopOldFailureAbortsAndDoesNotStartNew`
- `TestDeploy_RunNewFailureRollsBackToOld` — assert `restoreOldContainer` invoked.
- `TestDeploy_ReadinessFailureRollsBackToOld` — assert new container removed, old restored.
- `TestDeploy_SaveFailsBeforePartialWriteRollsBack` — Save returns non-`ErrPartialWrite`; assert new container removed, old restored, NO `DeleteOrphanConfig` called.
- `TestDeploy_SaveErrPartialWriteRollsBackAndDeletesOrphanConfig` — Save returns `ErrPartialWrite`; assert `DeleteOrphanConfig` THEN container rollback (`gomock.InOrder`).
- `TestDeploy_CaddyValidateFailureLeavesOldFileAndKeepsNewContainer` — Validate fails; assert os.Rename NOT called; assert Reload NOT called; assert new container NOT rolled back; return `errCaddyReload`. **(NEW per §9.2 step 8b)**
- `TestDeploy_CaddyReloadFailureDoesNotRollBackContainer` — Validate succeeds; Reload fails; new container stays up.
- `TestDeploy_CaddyStubWrittenWhenCaddyfileMissing`
- `TestDeploy_DeployIDIsStableThroughoutOneDeploy`
- `TestDeploy_CapturedEnvNotLoggedAsKeysOrValues`

Readiness (REWRITTEN per §9.4):
- `TestReadiness_HTTPSuccessReturnsNil` — mock Driver.ContainerIP returns "172.18.0.5"; HTTP server returns 200; probe succeeds.
- `TestReadiness_ContainerIPLookupFailureReturnsErrReadiness` — Driver.ContainerIP returns error every tick; assert probe times out and returns `errReadiness` wrapping the inspect error.
- `TestReadiness_ContainerIPInitiallyEmptyThenReady` — Driver.ContainerIP returns ErrNoBridgeIP on first tick, IP on second; HTTP succeeds; probe returns nil.
- `TestReadiness_HTTPTimeoutReturnsErrReadiness` — Driver returns IP; HTTP server never responds; probe times out.
- `TestReadiness_HTTPRetriesOnTransientFailure` — Driver returns IP; HTTP server returns 503 then 200; probe succeeds on retry.
- `TestReadiness_ContextCancellationStopsProbe` — Driver returns IP; ctx canceled; probe returns ctx.Err().

### 13.6 `internal/deploy` Lifecycle (NEW per §9.6)

In `internal/deploy/lifecycle_test.go`. All Gomock-driven.

Unregister:
- `TestLifecycle_UnregisterHappyPath`
- `TestLifecycle_UnregisterServiceNotFoundReturnsErrNotFound`
- `TestLifecycle_UnregisterContinuesIfContainerAlreadyGone`
- `TestLifecycle_UnregisterConfigOnlyOrphanProceeds`
- `TestLifecycle_UnregisterCaddyValidateFailureReturnsErrCaddyReload`
- `TestLifecycle_UnregisterCaddyReloadFailureReturnsErrCaddyReload`

Stop:
- `TestLifecycle_StopHappyPath`
- `TestLifecycle_StopAlreadyStoppedIsIdempotent`
- `TestLifecycle_StopServiceNotFoundReturnsErrNotFound`
- `TestLifecycle_StopUnexpectedDriverErrorReturnsErrRun`

Start:
- `TestLifecycle_StartFromExited`
- `TestLifecycle_StartFromAbsentReRunsContainer`
- `TestLifecycle_StartFromRunningIsNoOp`
- `TestLifecycle_StartServiceNotFoundReturnsErrNotFound`
- `TestLifecycle_StartSecretsMissingReturnsErrSecretsMissing`
- `TestLifecycle_StartImageMissingReturnsErrRun`

Restart:
- `TestLifecycle_RestartHappyPath`
- `TestLifecycle_RestartFromAbsentReRunsContainer`
- `TestLifecycle_RestartStopFailureAbortsBeforeStart`

Status:
- `TestLifecycle_StatusRunning`
- `TestLifecycle_StatusStopped`
- `TestLifecycle_StatusAbsentContainer`
- `TestLifecycle_StatusConfigOnlyOrphan`
- `TestLifecycle_StatusServiceNotFoundReturnsErrNotFound`

Logs:
- `TestLifecycle_LogsTailN`
- `TestLifecycle_LogsFollow`
- `TestLifecycle_LogsServiceNotFoundReturnsErrNotFound`

CaddyReload:
- `TestLifecycle_CaddyReloadHappyPath`
- `TestLifecycle_CaddyReloadValidateFailureLeavesOldFileIntact`
- `TestLifecycle_CaddyReloadReloadFailureNewFileOnDisk`
- `TestLifecycle_CaddyReloadEmptyRegistryWritesStubOnly`
- `TestLifecycle_CaddyReloadStoreListFailurePropagates`

### 13.7 `internal/cli`

- `TestRoot_ConfigRootDefaultsToDecloudRootEnv` — `t.Setenv("DECLOUD_ROOT", "/tmp/x")`; default is `/tmp/x`.
- `TestRoot_ConfigRootFlagOverridesEnv`
- `TestDeployService_BuildsExpectedRequest` — mocked deployer; assert `Deploy` called with the right `deploy.Request`.
- `TestDeployService_MissingNameReturnsExitUsageError`
- `TestDeployService_MountFlagReturnsErrMountsNotSupported` — `--mount /a:/b` → `ExitConfigError` (10).
- `TestDeployService_StrategyBlueGreenReturnsErrInvalidStrategy` — `--strategy=blue_green` → `ExitConfigError` (10).
- `TestDeployService_HostWithoutPortReturnsExitUsageError`
- `TestDeployService_DefaultEnvFileResolution`
- `TestExitCodeFor_AllSentinels` — table-driven; one row per sentinel; assert mapped int.
- `TestExitCodeFor_UnknownErrorMapsToInternal`
- `TestUnregister_DelegatesToLifecycle` — CLI unregister command calls `Lifecycle.Unregister(ctx, name)`.
- `TestStart_DelegatesToLifecycle`
- `TestStop_DelegatesToLifecycle`
- `TestRestart_DelegatesToLifecycle`
- `TestStatus_DelegatesToLifecycleAndPrintsResult` — assert stdout contains the formatted status line.
- `TestLogs_DelegatesToLifecycleWithFlags` — assert `LogOptions` populated from flags.
- `TestCaddyReload_DelegatesToLifecycle`

**No `t.Parallel()` in any `internal/cli` test** — see comment in §8.2 about `deployerFactory` / `lifecycleFactory` parallel-safety.

### 13.8 `internal/ids`

- `TestNewDeployID_FormatRegex` — `^[0-9]{8}-[0-9]{6}-[a-f0-9]{6}$`.
- `TestNewDeployID_UniqueAcrossRapidCalls`
- `TestContainerName_M1Format` — `ContainerName("foo") == "decloud-foo"`.
- `TestImageRef_Format`

### 13.9 `internal/logging`

- `TestInit_StderrOnlyShortCircuit`
- `TestInit_DefaultWritesToFileAndStderr`
- `TestInit_FileOpenFailureReturnsError`

### 13.10 `internal/config`

- `TestNewPaths_AllPathsRootedCorrectly`
- `TestRootFromEnv_HonorsDecloudRoot`
- `TestRootFromEnv_DefaultsToDecloudingPath`

### 13.11 What we explicitly do NOT test

- No test that Cobra's help text contains specific phrases (change-detector).
- No test that the slog JSON keys have a specific stable order (Go's `slog` does not guarantee).
- No snapshot tests of `_docs/` markdown — Raymond's docs are reviewed by Kevlin for content.
- No integration tests against real Docker, real Caddy, or `caddy validate`. Per user constraint.

---

## 14. Where Knuth gets called (revised list)

1. **`internal/envcap/capture.go`** — bash 3.2 portability is verified in spirit, but if Rob hits any test failure on macOS he can't quickly diagnose, call Knuth.
2. **`internal/deploy/service.go` step-7 mid-write rollback path** — the distinction between "Save returned `ErrPartialWrite` (call `DeleteOrphanConfig` first, then container rollback)" vs "Save returned anything else (just container rollback)" is subtle. If the test setup boilerplate exceeds ~200 lines, call Knuth before refactoring.
3. **`internal/dockerdrv` cmdFactory shape** — if Rob cannot get the recording-factory pattern to give clean tests (especially for `Build` which streams stdout/stderr), call Knuth.
4. **`internal/dockerdrv/cli_driver.go` ContainerIP parser (NEW per §9.4)** — if Docker Desktop on macOS reports a different network shape than Docker on Linux, the parser may need conditional handling. The Format string `{{ .NetworkSettings.Networks.decloud.IPAddress }}` is supported on both platforms per Docker docs. If Rob hits any platform divergence, call Knuth before forking the parser.
5. **`internal/deploy/lifecycle.go` (NEW per §9.6)** — none of the seven methods is individually tricky, but the cluster sharing `*serviceDeployer` state may grow unwieldy. If `lifecycle.go` crosses 400 lines or test setup boilerplate becomes painful, call Knuth before refactoring blindly.
6. **`internal/caddy/reloader.go Validate` (NEW per §9.2 step 8b)** — three lines of code wrapping `caddy validate --config <path>`. Should not need Knuth.

---

## 15. Things I'm flagging for Linus

1. **`Status.LastDeployedAt` requires extending `ServiceConfig`** (§9.6.5). If the field doesn't exist in prior `06-tech-plan-v2.md` §4.2 (Joel's quick check: it isn't called out; the field is `Build.ImageRef` for the deploy ID, but no timestamp field). v2 ADDS `LastDeployedAt time.Time` to `ServiceConfig`, populated in Deploy step 7b. This is a backward-compatible TOML schema addition (existing files without the field just get zero-value). Schema version stays at 1. Acceptable extension?

2. **`Reloader.Validate` extension to the prior plan's `Reloader` interface** (§9.2 step 8b). The prior `06-tech-plan-v2.md` §7.1 only defined `Reload`. v2 adds `Validate(ctx, configPath) error`. Necessary to close the Caddy-broken-on-disk failure mode. Acceptable extension?

3. **`Driver.Start` and `Driver.ContainerIP` additions; `Driver.OneShotProbe` removal** (§11.1). Net change: +2 methods, -1 method. All driven by §9.4 and §9.6.3 decisions you've already approved in the plan-v2. Recording for completeness so the diff is on the table.

4. **`*serviceDeployer` is one struct implementing two interfaces** (§9.1). Don §3.1 left this to me; my call is one struct (shared deps, methods clustered across `service.go` and `lifecycle.go`). Linus: object if you disagree.

5. **`internal/deploy/lifecycle.go` is a separate file from `service.go`** (§2). Don §9 left this to me; my call is separate file for readability (seven methods + the regenerateAndReload helper would push `service.go` past 500 lines if combined). Linus: object if you disagree.

6. **The `regenerateAndReload` private helper** (§9.6 preamble) is shared by Deploy step 8, Unregister step 5, and CaddyReload. Three call sites for the same five-step Caddy regeneration. DRY-positive. Linus: object if you disagree.

7. **Mockgen layout asymmetry** (§5.1) — UNCHANGED from v1, you accepted in answer #4. Documenting in `internal/cli/mocks/doc.go` per your suggestion. Recording for completeness.

8. **`Status` single-line stdout format** (§8.3 status.go) — I picked a space-separated key=value format. If you'd prefer JSON-by-default (over the `--json` flag I deferred), say so now; it's a five-line change.

Everything else is mechanical execution of v1 plus the deltas listed in §0.

---

## 16. Handoff — receipt format Rob attaches (NEW per Don §3.4)

When this plan is approved (Linus reviews next):

- **Step 3a (Kent):** writes failing unit tests per §13. Stubs implementation files with `panic("unimplemented")` bodies so tests compile-and-fail. Commits and reports.
- **Step 3b (Rob):** implements per §2, §6–§12. Runs `go generate ./...` for mocks. **Attaches the receipt below to his report.**
- **Step 3c (Raymond):** writes operator + architecture + CLI docs per Don §2.2 plus `_ai/decisions/m1-test-strategy.md`. Reports.
- **Steps 3d/3e (Kevlin/Linus parallel):** review.
- **Step 2-redux (Don/Joel/Linus):** PLAN re-check; iterate or done.

### 16.1 The receipt format

Rob's report file (Bureau gives him the path; typically `_tasks/2026-04-26-m1-implementation/<seq>-rob-implementation.md`) MUST include a section titled exactly **"Test pass receipt"** containing ALL ten items below. **M1 acceptance gate (Don §6 DONE criterion #1) does not pass without this receipt.**

The receipt sits at the top of the report so reviewers don't have to scroll. Rob captures everything in one shell session to avoid drift between item generation.

```
## Test pass receipt

1. Command run:
   cd /Users/fenster/dev/declouding && go test ./... -v -count=1 2>&1

2. Go version:
   $ go version
   <output>

3. Host (uname -a + sw_vers if macOS):
   $ uname -a
   <output>
   $ sw_vers   # macOS only
   <output>

4. Bash version:
   $ bash --version | head -1
   <output>

5. Docker version:
   $ docker version --format '{{.Server.Version}}'
   <output>

6. Caddy version:
   $ caddy version
   <output>

7. Test summary (extracted from the full output below — top-level so reviewers
   see at a glance whether everything passed):
   ok      github.com/alexander-fenster/decloud/internal/registry  0.123s
   ok      github.com/alexander-fenster/decloud/internal/envcap    1.456s
   ok      github.com/alexander-fenster/decloud/internal/caddy     0.089s
   ok      github.com/alexander-fenster/decloud/internal/dockerdrv 0.234s
   ok      github.com/alexander-fenster/decloud/internal/deploy    0.567s
   ok      github.com/alexander-fenster/decloud/internal/cli       0.345s
   ok      github.com/alexander-fenster/decloud/internal/ids       0.012s
   ok      github.com/alexander-fenster/decloud/internal/logging   0.023s
   ok      github.com/alexander-fenster/decloud/internal/config    0.011s

8. Vet result (must be empty / no findings):
   $ go vet ./...
   <output>

9. Go generate is idempotent (must be empty porcelain):
   $ go generate ./...
   $ git status --porcelain
   <output>

10. Full verbose test output (verbatim, no editing):
    <stdout+stderr of `go test ./... -v -count=1 2>&1` — long is fine,
     wrap in <details><summary>...</summary> markdown if the report gets unwieldy>
```

If item 7 has any line not starting with `ok`, M1 is NOT done — Rob iterates per CLAUDE.md before producing a re-attempt receipt.

If item 9's porcelain is non-empty, Rob commits the regenerated mocks (or fixes whatever generator drift caused the diff) and re-runs from scratch.

If item 8 has any output, Rob fixes the vet finding and re-runs.

This receipt is the M1 acceptance gate Don §6 criterion #1 cites. It also serves as the frozen test-pass evidence for any post-handoff regression triage (the user says "the binary doesn't work on Linux"; first question: "what does Rob's receipt show on macOS?"). When CI is added in M2 (per Don §3.4), this receipt becomes a one-line `make test-receipt` for manual local runs and the CI badge takes over the gating role.

---

## 17. What this plan explicitly does NOT change from v1

- §2 file tree (except the two new files in §9.6 / lifecycle).
- §3 operational deliverables table (except the new `state/deploys/` deferral row).
- §4 module init / dependency pinning.
- §5 mockgen layout (except adding the `internal/cli/mocks/doc.go` rationale comment).
- §6 slog setup (except the `caddy_validate` step name in the field list).
- §7 `cmd/decloud/main.go` final form.
- §8.1 `root.go` Cobra wiring.
- §8.2 `deploy_service.go` (except the parallel-safety comment block above `deployerFactory`).
- §8.4 `exit_codes.go` constants and mapper.
- §9.2 deploy step sequence (except step 8b validate insertion).
- §9.3 `restoreOldContainer` body.
- §9.5 partial-write detection mechanism (only the method NAME changes — same body).
- §10 mounts rejection (verified consistent).
- §11.2 cmdFactory shape and recording-factory pattern.
- §11.3 Knuth-review preemptive answers.
- §12 `internal/ids/` body.

Everything in v1 not listed in §0 stands. Diff against v1 is contained to the changes enumerated.

---

## 18. Final word

Two real holes in v1 (Lifecycle behavior, Caddy reload recovery), one decision was three-options-and-no-answer (readiness), one was deferred-without-consolation-prize (CI). v2 closes all four:
- Lifecycle behavior: §9.6 specifies all seven methods with signatures, sequences, sentinels, exit codes, test names. No spec gap remains.
- Caddy reload safety: §9.2 step 8b pre-validates via `caddy validate` BEFORE atomic-rename; the OLD Caddyfile on disk is preserved on validate failure. Same wiring in §9.6.1 (Unregister) and §9.6.7 (CaddyReload) via shared `regenerateAndReload` helper.
- Readiness: §9.4 rewritten around `Driver.ContainerIP` and a host-side `httpProbe`. No `OneShotProbe`. No `curlimages/curl`. No supply-chain risk.
- CI deferral: §16 defines the receipt format Rob attaches. M1 acceptance gate cites the receipt explicitly.

Plus six non-blocking cleanups: `DeleteOrphanConfig` rename, `state/deploys/` deferral row, three explicit envcap test names, `deployerFactory` parallel-safety comment, `Driver.Start` addition, mockgen-asymmetry rationale comment.

The rest of v1 stands. Kent: write tests against this. Rob: implement against Kent's tests. Raymond: write docs against Don's §2.2. Don't gold-plate. Don't reopen settled questions. Ship M1.

End of plan v2.
