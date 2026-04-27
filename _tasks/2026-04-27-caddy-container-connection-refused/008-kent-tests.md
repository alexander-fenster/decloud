# 008 — Kent: failing tests for the containerised-Caddy plan

Author: Kent Beck (test-engineer agent)
Date: 2026-04-27
Status: EXECUTION step 3.1 complete. Tests committed to disk; build green; tests fail in expected ways pending Rob's implementation.

## Scope

Tests cover all four phases of Joel v2 §7:

- **Phase 1** — `internal/dockerdrv` driver primitives (`ImagePull`, `Exec`, `RunWithOptions`, `formatPortMap`).
- **Phase 2** — `internal/caddy/manager.go` lifecycle (`Up`, `Down`, `IsRunning`).
- **Phase 3** — `internal/caddy/reloader.go` rewire onto `Driver.Exec`, including path-translation contract.
- **Phase 4** — `internal/cli/caddy_up.go`, `caddy_down.go` Cobra wiring and the `caddyManagerFactory` test seam.
- **Phase 5** — exit-code mapping for `caddy.ErrCaddyUp`/`ErrCaddyDown` and the deploy-failure recovery wrap text.

No production code beyond what's needed to compile (all new method bodies `panic("not implemented")`). The `cmdFactory` test seam in `internal/caddy/reloader.go` is deleted (the rewritten file no longer references it).

## Files modified to make tests compile

These changes are scaffolding only. Rob will replace the panicking bodies in Phase 1-4 of the implementation order.

- `internal/dockerdrv/driver.go` — extended `Driver` interface with three methods (`ImagePull`, `RunWithOptions`, `Exec`); added `PortMap`, `VolumeMount`, `RunOptions`, `ExecOptions` types.
- `internal/dockerdrv/cli_driver.go` — added stub method bodies for the three new methods plus `formatPortMap`/`formatVolume` helpers, all panicking. Existing methods unchanged.
- `internal/dockerdrv/mocks/mock_driver.go` — regenerated via `go generate ./internal/dockerdrv/...`.
- `internal/caddy/manager.go` — NEW. `Manager` interface, `ManagerConfig`, constants (`ContainerName`, `NetworkName`, `DefaultImage`), sentinels (`ErrCaddyUp`, `ErrCaddyDown`), `cliManager` with panicking method bodies.
- `internal/caddy/mocks/mock_manager.go` — generated from `manager.go` via `go generate`.
- `internal/caddy/reloader.go` — REWRITTEN. New constructor `NewCLIReloader(driver, hostCaddyDir)`. `cmdFactory`, `newCLIReloaderWithFactory` deleted. Method bodies panic.
- `internal/cli/deploy_service.go` — `buildProductionDeployer`/`buildProductionLifecycle` updated to call `caddy.NewCLIReloader(driver, paths.CaddyDir)`. New `caddyManagerFactory` test seam and `buildProductionCaddyManager` helper.
- `internal/cli/caddy_up.go`, `caddy_down.go` — NEW Cobra commands wired through `caddyManagerFactory`.
- `internal/cli/root.go` — registered the new `caddy up` and `caddy down` subcommands as siblings of `caddy reload`.

## Test files created or rewritten

### `internal/dockerdrv/cli_driver_test.go` (additions)

Existing argv-shape tests unchanged. New tests added:

| Test | What it asserts |
|---|---|
| `TestCLIDriver_ImagePullArgs` | `docker pull caddy:2`. |
| `TestCLIDriver_ImagePullPropagatesStderrOnFailure` | Pull failure returns an error containing the docker stderr. |
| `TestCLIDriver_ExecArgsBasic` | `docker exec decloud-caddy caddy validate --config /etc/caddy/Caddyfile.tmp`. |
| `TestCLIDriver_ExecPropagatesContainerNotFound` | Stderr `No such container` → `ErrContainerNotFound`. |
| `TestCLIDriver_ExecPropagatesGenericError` | Other exec failures wrap (not `ErrContainerNotFound`) and surface stderr. |
| `TestCLIDriver_RunWithOptionsCaddyShape` | Full canonical Caddy `docker run` argv with all six dual-stack `-p` flags and three `-v` flags, in declared order. |
| `TestCLIDriver_RunWithOptionsDualStackPorts` | Two `PortMap` entries with `0.0.0.0` and `[::]` produce `-p 0.0.0.0:1234:5678/tcp -p [::]:1234:5678/tcp` (Linus revision #4). |
| `TestCLIDriver_RunWithOptionsBindReadOnly` | `VolumeMount{ReadOnly:true}` → `:ro` suffix. |
| `TestCLIDriver_RunWithOptionsNamedVolumeNotReadOnly` | Named volume → `vol:/dst` (no `:ro`). |
| `TestCLIDriver_RunWithOptionsLabelsSorted` | Labels `{b:2, a:1}` emit sorted `a=1`, then `b=2`. |
| `TestCLIDriver_RunWithOptionsPortsDeclaredOrder` | Ports emit in declared order, NOT sorted. |
| `TestCLIDriver_RunWithOptionsPortDefaultProto` | `Proto:""` defaults to `tcp`. |
| `TestCLIDriver_RunWithOptionsEmptyHostBind` | Empty `HostBind` collapses to `<host>:<container>/<proto>` (contract-clean fallback). |
| `TestFormatPortMap_DoesNotAutoBracketIPv6` | `formatPortMap` splices `[::]` literally; no auto-bracket (Joel §9.9). |
| `TestFormatPortMap_EmptyHostBindOmitsBindSegment` | Direct unit test for the empty-`HostBind` collapse. |

Helpers added: `caddyRunOptionsFixture()`, `portFlagsFromArgs()`, `volumeFlagsFromArgs()`, `labelFlagsFromArgs()`, `flagValuesByName()`. They're declared once at the bottom of the file and reused across the new tests.

### `internal/caddy/manager_test.go` (NEW)

Black-box tests (`package caddy_test`) backed by `MockDriver` and a `managerHarness` helper. `gomock.InOrder` per `_ai/gomock-inorder-sequencing.md`.

| Test | What it asserts |
|---|---|
| `TestManager_UpFreshInstall` | `NetworkEnsure(decloud)` → `Inspect(decloud-caddy)` (absent) → `ImagePull(caddy:2)` → `RunWithOptions` with the canonical six-port dual-stack shape. Stdout mentions `caddy:2` and `decloud-caddy`. |
| `TestManager_UpAlreadyRunning` | `Inspect` returns `running`; no `ImagePull`/`Run`/`Start`. Stdout: `caddy already running`. |
| `TestManager_UpAfterPriorStop` | `Inspect` returns `exited` → `Start`; no `ImagePull`/`Run`. Stdout: `caddy started`. |
| `TestManager_UpUnexpectedStateWraps` | `paused` state → `ErrCaddyUp` wrap; message names the unexpected state. |
| `TestManager_UpNetworkEnsureFails` | Inner sentinel travels through `errors.Is(err, ErrCaddyUp)` AND `errors.Is(err, innerErr)` (`%w: %w` discipline). |
| `TestManager_UpImagePullFails` | Same wrap shape on the pull leg. |
| `TestManager_UpRunFailsWithoutRollback` | `RunWithOptions` errors → `ErrCaddyUp`-wrapped, AND `Stop`/`Remove` are explicitly `Times(0)` (locks Linus revision #2). |
| `TestManager_UpStubWriteFailsWrappedAsCaddyUp` | Pre-create CaddyDir as a regular file → stub write fails → wrap = `ErrCaddyUp`. |
| `TestManager_UpStubCaddyfileWritten` | After a successful `Up` against an already-running container, the Caddyfile stub exists on disk. |
| `TestManager_UpStubWriteIdempotent` | A pre-existing `/Caddyfile` is left untouched on `Up` (operator content survives). |
| `TestManager_DownHappyPath` | `Stop(decloud-caddy, 10s)` → `Remove(decloud-caddy)`. Stdout mentions "caddy down". |
| `TestManager_DownContainerAbsent` | `Stop` and `Remove` both return `ErrContainerNotFound` → `Down` returns nil (idempotent). |
| `TestManager_DownStopFailsHard` | Real `Stop` failure → `ErrCaddyDown`-wrapped; `Remove` not called. |
| `TestManager_IsRunningTrue` / `False*` | Three sub-cases: running → true; exited → false; absent → false. |

### `internal/caddy/reloader_test.go` (REWRITTEN)

Existing tests deleted: `TestReloader_InvokesCaddyValidate`, `TestReloader_InvokesCaddyReload`, `TestReloader_ValidateFailureReturnsError`, plus the `recordingFactory`/`failingFactory` helpers — all coupled to the now-deleted `cmdFactory`. Joel v2 §6.2 explicitly calls them out as obsolete.

New tests:

| Test | What it asserts |
|---|---|
| `TestReloader_ValidateCallsDockerExec` | `Driver.Exec` invoked with `Container=decloud-caddy`, `Cmd=[caddy, validate, --config, /etc/caddy/Caddyfile.tmp]`. |
| `TestReloader_ReloadCallsDockerExec` | Same shape for `caddy reload --config /etc/caddy/Caddyfile`. |
| `TestReloader_PathTranslationCanonicalForm` | `/opt/decloud/config/caddy/Caddyfile.tmp` → `/etc/caddy/Caddyfile.tmp` (Linus §7 positive). |
| `TestReloader_PathTranslationOutsideBindMount` | `/tmp/foo` → error containing "outside the bind-mount"; `Driver.Exec` `Times(0)`. |
| `TestReloader_PathTranslationParentEscape` | `.../caddy/../../etc/passwd` → same error; `Driver.Exec` `Times(0)`. |
| `TestReloader_ContainerNotRunningSurfacesActionableError` | `Driver.Exec` returns `ErrContainerNotFound` → error mentions `container "decloud-caddy" is not running` AND `decloud caddy up`. |
| `TestReloader_ContainerExitedSurfacesActionableError` | Generic exec error + stderr signature `is not running` → same actionable message. |
| `TestReloader_ValidateExitNonzeroPreservesStderr` | Inner exec error survives `errors.Is`; stderr text surfaced (locks `%w` discipline). |
| `TestReloader_StderrIsCapturedEvenWithoutCallerWriter` | `ExecOptions.Stderr` is non-nil even if the test doesn't pass one — the reloader must always capture stderr internally for the not-running detection. |

Helper: `captureExec(t, h, ret, stderrText)` records the `ExecOptions` and writes a configurable stderr blob; lets each test just set up the mock once.

### `internal/cli/caddy_up_test.go`, `caddy_down_test.go` (NEW)

`caddyManagerFactory` test seam, mirroring `installMockDeployer` in `deploy_service_test.go`. NO `t.Parallel()`.

| Test | What it asserts |
|---|---|
| `TestCaddyUp_DelegatesToManager` | `decloud caddy up` calls `Manager.Up`. |
| `TestCaddyUp_ManagerErrorReturnsExitRunFail` | Wrapped `ErrCaddyUp` maps to exit 40. |
| `TestCaddyUp_NoFlags` | `--image foo` is rejected with a usage error. |
| `TestCaddyUp_PassesContextThrough` | Context propagates from Cobra into `Manager.Up`. |
| `TestCaddyDown_DelegatesToManager` | Mirror. |
| `TestCaddyDown_ManagerErrorReturnsExitRunFail` | Wrapped `ErrCaddyDown` maps to exit 40. |
| `TestCaddyDown_NoFlags` | `--remove-volumes` rejected with usage error. |

### `internal/cli/exit_codes_test.go` (extended)

Four new sub-cases added to `TestExitCodeFor_AllSentinels` covering `caddy.ErrCaddyUp` and `caddy.ErrCaddyDown` (both bare and wrapped) — both must map to `ExitRunFail` (40).

### `internal/deploy/service_test.go` (extended)

Two new tests for the deploy-failure recovery wrap text (Linus revision #5 / Joel v2 §4.9):

| Test | What it asserts |
|---|---|
| `TestDeploy_CaddyValidateFailureMentionsCaddyUpRecovery` | Final error contains "decloud caddy up", "registered", "Caddy is not routing", AND `errors.Is(err, ErrCaddyReload)` still holds. |
| `TestDeploy_CaddyReloadFailureMentionsCaddyUpRecovery` | Same shape on the reload leg. Inner reloader sentinel survives the wrap (`errors.Is(err, innerErr)` true). |

The existing `TestDeploy_CaddyValidateFailureLeavesOldFileAndKeepsNewContainer` and `TestDeploy_CaddyReloadFailureDoesNotRollBackContainer` are unchanged and still pass — they only assert on `errors.Is(err, ErrCaddyReload)`, which the wrap preserves.

## Expected go test output

`go build ./...` succeeds. `gofmt -l internal/` is empty. `go vet ./...` is empty.

`go test ./...` shows the following expected failures (each fails because production code panics with "not implemented" or because the recovery text/exit-code mapping isn't wired yet — exactly what Rob will fix in Phases 1-5).

### `internal/dockerdrv` — 15 failing tests, all on `panic: not implemented`

```
TestCLIDriver_ImagePullArgs                              FAIL (panic: not implemented in cli_driver.go ImagePull)
TestCLIDriver_ImagePullPropagatesStderrOnFailure         FAIL (same)
TestCLIDriver_ExecArgsBasic                              FAIL (panic in Exec)
TestCLIDriver_ExecPropagatesContainerNotFound            FAIL (same)
TestCLIDriver_ExecPropagatesGenericError                 FAIL (same)
TestCLIDriver_RunWithOptionsCaddyShape                   FAIL (panic in RunWithOptions)
TestCLIDriver_RunWithOptionsDualStackPorts               FAIL (same)
TestCLIDriver_RunWithOptionsBindReadOnly                 FAIL (same)
TestCLIDriver_RunWithOptionsNamedVolumeNotReadOnly       FAIL (same)
TestCLIDriver_RunWithOptionsLabelsSorted                 FAIL (same)
TestCLIDriver_RunWithOptionsPortsDeclaredOrder           FAIL (same)
TestCLIDriver_RunWithOptionsPortDefaultProto             FAIL (same)
TestCLIDriver_RunWithOptionsEmptyHostBind                FAIL (same)
TestFormatPortMap_DoesNotAutoBracketIPv6                 FAIL (panic in formatPortMap)
TestFormatPortMap_EmptyHostBindOmitsBindSegment          FAIL (same)
```

All existing `TestCLIDriver_*` argv tests still pass (verified: 16 pass).

### `internal/caddy` — 25 failing tests

All 16 `TestManager_*` tests panic with "not implemented" inside `cliManager.Up` / `Down` / `IsRunning`.
All 9 `TestReloader_*` tests panic with "not implemented" inside `cliReloader.Validate` / `Reload`.

Existing `TestStub*`, `TestNormalize*`, `TestGenerator*` tests still pass.

### `internal/cli` — 6 failing tests

```
TestCaddyUp_ManagerErrorReturnsExitRunFail               FAIL (expected 40, got 70: ErrCaddyUp not yet in ExitCodeFor)
TestCaddyDown_ManagerErrorReturnsExitRunFail             FAIL (same: ErrCaddyDown)
TestExitCodeFor_AllSentinels/caddy-up                    FAIL (40 vs 70)
TestExitCodeFor_AllSentinels/caddy-up-wrapped            FAIL (40 vs 70)
TestExitCodeFor_AllSentinels/caddy-down                  FAIL (40 vs 70)
TestExitCodeFor_AllSentinels/caddy-down-wrapped          FAIL (40 vs 70)
```

`TestCaddyUp_DelegatesToManager`, `TestCaddyDown_DelegatesToManager`, `TestCaddyUp_NoFlags`, `TestCaddyDown_NoFlags`, `TestCaddyUp_PassesContextThrough` pass (the test seam works; the contract that fails is the exit-code mapping). All other CLI tests unchanged and passing.

### `internal/deploy` — 2 failing tests

```
TestDeploy_CaddyValidateFailureMentionsCaddyUpRecovery   FAIL (expected text "registered" / "Caddy is not routing" missing)
TestDeploy_CaddyReloadFailureMentionsCaddyUpRecovery     FAIL (same)
```

All 30+ existing deploy tests still pass.

## Implementation map for Rob

These are the exact slots the failing tests pin. Joel v2 has the algorithms; this is the test → source-line crosswalk.

1. **`internal/dockerdrv/cli_driver.go::ImagePull`** — Joel v2 §4.7. Emit `pull <ref>`. Return wrapped error with stderr on non-zero. Tests: `TestCLIDriver_ImagePullArgs`, `TestCLIDriver_ImagePullPropagatesStderrOnFailure`.

2. **`internal/dockerdrv/cli_driver.go::Exec`** — Joel v2 §4.6. Emit `exec <container> <cmd...>`. Map stderr `No such container` → `ErrContainerNotFound`. Stdout/Stderr writers from opts (use `io.MultiWriter` to also keep an internal copy when `opts.Stderr` is non-nil — for the not-found detection on top of forwarding to caller). Tests: `TestCLIDriver_Exec*`.

3. **`internal/dockerdrv/cli_driver.go::RunWithOptions`** — Joel v2 §4.8. Emit `run -d --name --network --restart`, then `--env` (sorted), `--label` (sorted), `-p` (declared order), `-v` (declared order), then image. Tests: all `TestCLIDriver_RunWithOptions*`.

4. **`internal/dockerdrv/cli_driver.go::formatPortMap`** — Joel v2 §4.8. `<HostBind>:<HostPort>:<ContainerPort>/<Proto>` with HostBind splatted literally (no auto-brackets). Empty HostBind → `<HostPort>:<ContainerPort>/<Proto>`. Empty Proto → `tcp`. Tests: `TestFormatPortMap_*`.

5. **`internal/caddy/manager.go::Up`** — Joel v2 §4.2 algorithm. NetworkEnsure → WriteStubIfMissing → Inspect → switch on State (running/exited/absent/other). Tests: all `TestManager_Up*`.

6. **`internal/caddy/manager.go::Down`** — Joel v2 §4.3. Stop with 10s grace → Remove. Both tolerant of `ErrContainerNotFound`. Tests: all `TestManager_Down*`.

7. **`internal/caddy/manager.go::IsRunning`** — Joel v2 §4.4. Inspect → return `State == "running"`. Tests: all `TestManager_IsRunning*`.

8. **`internal/caddy/reloader.go::Validate`/`Reload`/`execCaddy`** — Joel v2 §4.5. Path-translate via `translatePath`; `Driver.Exec` with `[]string{"caddy", sub, "--config", ctrPath}`; map `ErrContainerNotFound` and stderr `is not running` to the actionable message. Tests: all `TestReloader_*`.

9. **`internal/caddy/reloader.go::translatePath`** — Joel v2 §4.5. `filepath.Rel` of host path against `hostCaddyDir`; reject if `rel == ".."` or starts with `"../"`; return `path.Join("/etc/caddy", rel)`. Tests: `TestReloader_PathTranslation*`.

10. **`internal/cli/exit_codes.go::ExitCodeFor`** — add two cases mapping `caddy.ErrCaddyUp` and `caddy.ErrCaddyDown` to `ExitRunFail`. No new constants. Tests: `TestExitCodeFor_AllSentinels/caddy-up*`, `TestCaddyUp_ManagerErrorReturnsExitRunFail`, mirrors for `down`.

11. **`internal/deploy/service.go:314-322`** — Joel v2 §4.9. On both validate and reload failure legs, append `; service is registered and running but Caddy is not routing traffic; run 'decloud caddy up' (and then 'decloud caddy reload' if needed) to restore routing` to the wrap text. Keep the `%w: %w` shape. Tests: `TestDeploy_Caddy*FailureMentionsCaddyUpRecovery`.

## Items I noticed during writing

These are FYI for Rob/Don/Joel; none change the plan.

1. **Joel v2 §4.6's `Driver.Exec` impl wires `opts.Stderr` via `io.MultiWriter` only when non-nil; otherwise it captures into a local `bytes.Buffer`.** I added `TestReloader_StderrIsCapturedEvenWithoutCallerWriter` to lock the contract from the *reloader* side: the reloader always supplies a non-nil `Stderr` (so the not-running stderr detection works regardless of what the driver does internally). This is a slightly stronger contract than Joel spelled out — the reloader can't rely on the driver's internal capture because the reloader needs to scan the stderr text for "is not running" itself. Implementation note for Rob: in `cliReloader.execCaddy`, always allocate a `bytes.Buffer` and pass it as `opts.Stderr`.

2. **Joel v2 §3.3 says "Volumes are declared-order".** My `TestCLIDriver_RunWithOptionsCaddyShape` locks the order `bind → data → config`, which matches Joel §3.2. If Rob ever reorders these in the manager, the dockerdrv test stays happy (it just tests argv shape from any RunOptions); the manager test (`TestManager_UpFreshInstall`) is the one that pins the manager-side order. That's the right split.

3. **`TestManager_UpStubWriteFailsWrappedAsCaddyUp` writes a regular file at the CaddyDir path so `MkdirAll` inside `WriteStubIfMissing` fails.** Joel v2 §6.1 said "pre-create Paths.CaddyfilePath's parent as a regular file"; the parent of `Paths.CaddyfilePath` is `Paths.CaddyDir`, so writing a regular file *at* `Paths.CaddyDir` is the cleanest way to produce the failure. Reading Joel literally one could imagine creating `Paths.CaddyfilePath` itself as a directory — that wouldn't fail `MkdirAll` though. My read is correct; flagging in case Rob reads Joel and mine differently.

4. **`TestCaddyUp_NoFlags` and `TestCaddyDown_NoFlags` both PASS already**, before Rob writes any code, because Cobra rejects unknown flags by default. The contract is locked correctly; just noting the test isn't doing meaningful failure-driven work in the TDD sense. It's a regression guard for "someone adds `--image` later without thinking."

5. **`go generate ./internal/dockerdrv/...` and `go generate ./internal/caddy/...` succeeded cleanly** with the new interface methods and the new `Manager` interface. Rob does not need to re-run mockgen unless he adds or changes interface methods.

6. **The existing `TestCLIDriver_RunArgsWithEnvSorted` keeps the legacy `--label decloud.service=foo` shape** for `Driver.Run` (the service deploy path). The new `RunWithOptions` does NOT auto-add a label; manager passes `decloud.managed=caddy` through `Labels` explicitly. Joel v2 §3.2/§4.8 line up with this; I just want to flag for Rob that the two paths have different label conventions — `Run` auto-derives, `RunWithOptions` is explicit.

7. **No `cmdFactory` reference survives in `internal/caddy/reloader.go`.** `grep -rn cmdFactory internal/caddy/` returns zero hits. The `cmdFactory` in `internal/dockerdrv/cli_driver.go` stays — it's the test seam for the dockerdrv argv-shape tests (16 existing tests rely on it) and is unrelated to the reloader rewire. Joel v2 §2.2 / §4.5 / acceptance criterion #21 only require deleting the reloader's `cmdFactory`; that's done.

— Kent
