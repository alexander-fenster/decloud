# Step 3a — Kent: failing tests for M1

**Author:** Kent Beck (test engineer)
**Status:** Tests committed. Compile clean. `go vet ./...` clean. `go test ./...` fails as expected (`panic: unimplemented` everywhere except `internal/config`, which has no stub). Baseline ready for Rob.
**Plan basis:** `06-tech-plan-v2.md` §12 / §13 (per-package test list), §9 (deploy + lifecycle behavior), §11 (dockerdrv), §15 (Joel's flagged items).

---

## 1. Module + dependency state

`go.mod` (`go 1.22`) now requires:

| Module | Version |
|---|---|
| `github.com/spf13/cobra` | `v1.8.0` (pre-existing) |
| `github.com/pelletier/go-toml/v2` | `v2.1.0` (added by Kent — required for §13.1 round-trip + schema_version tests) |
| `github.com/stretchr/testify` | `v1.9.0` (added by Kent) |
| `go.uber.org/mock` | `v0.4.0` (pre-existing) |

`tools.go` already pins `go.uber.org/mock/mockgen`. `go mod tidy` clean.

**Linus non-blocker #1 (pelletier round-trips `time.Time`)** — VERIFIED. A throwaway program marshals `time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)` as `when = 2026-04-26T12:00:00Z` and `Unmarshal` reads it back equal. `Service.Config.LastDeployedAt time.Time` is safe; `TestStore_RoundTripsLastDeployedAt` enforces with a fixture. Rob does NOT need to special-case anything.

---

## 2. Files created / modified by Kent this turn

Pre-existing scaffolding (from earlier turns of this task) was already substantial — type stubs, error sentinels, mocks, several test files, and most production-side `panic("unimplemented")` shells were in place. Kent's additions/fixes this turn:

### Modified
- `internal/deploy/service_test.go` — REWRITTEN. The previous file had a homemade `reflectValue/reflectField2/reflectAccessField` shim that referenced an undefined symbol and would not compile. Replaced with stdlib `reflect` (and in most places, just typed `dockerdrv.RunRequest`/`BuildRequest` via `gomock.DoAndReturn`). Added `TestDeploy_RunRequestUsesCapturedEnvAndDecloudNetwork`.
- `internal/registry/store_test.go` — added `TestStore_SaveReturnsErrPartialWriteOnSecretsFailure` (per §13.1 / §9.5). Forces partial-write by `chmod 0500` on the secrets dir, asserts `errors.Is(err, ErrPartialWrite)` AND that the config file landed (so the orchestrator's `DeleteOrphanConfig` path has something to clean up). Skipped under root.
- `internal/deploy/readiness.go` — replaced TODO stub with the exported `ReadinessProbe` interface and `NewHTTPProbeForTest(driver) ReadinessProbe` test seam, both stubbed with `panic("unimplemented")`. Required so `readiness_test.go` (package `deploy_test`, black-box) can drive `Wait` directly without bringing up the full orchestrator.

### Added
- `internal/deploy/lifecycle_test.go` — 32 tests covering all seven Lifecycle methods per Joel §9.6 / §13.6. Each method gets one happy-path with `gomock.InOrder`, its idempotence/not-found branches, and the Caddy validate/reload failure modes for Unregister + CaddyReload.
- `internal/deploy/readiness_test.go` — 6 tests per §13.5 (rewritten readiness section). Exercises the per-tick `Driver.ContainerIP` re-resolution, `ErrNoBridgeIP` recovery, HTTP-503-then-200 retry, host-side `httptest` server, context cancellation, and timeout. Driver mocked; no Docker.
- `internal/cli/exit_codes_test.go` — table-driven `TestExitCodeFor_AllSentinels` (15 rows: nil, errUsage, six registry sentinels, five deploy sentinels) plus `TestExitCodeFor_UnknownErrorMapsToInternal`.
- `internal/cli/deploy_service_test.go` — 7 tests: builds the expected `deploy.Request`, missing-name → exit 2, mount-flag → exit 10, `--strategy=blue_green` → exit 10, host-without-port → exit 2, default-strategy=recreate, default readiness path. Uses `installMockDeployer(t)` helper that swaps `deployerFactory`.
- `internal/cli/lifecycle_commands_test.go` — 7 tests, one per lifecycle command (unregister/start/stop/restart/status/logs/caddy-reload). `installMockLifecycle(t)` swaps `lifecycleFactory`. Status test asserts stdout contains the formatted line; logs test asserts `--follow --tail 100` map onto `LogOptions`.
- `internal/cli/root_test.go` — 2 tests: `--config-root` defaults to `DECLOUD_ROOT` env, flag overrides env. Uses a recording `deployerFactory` that captures `Paths.Root`.

### Untouched (already correct in scaffolding)
- `internal/registry/store_test.go` (other than the addition above) — already covered round-trip, schema_version mismatch (config + cross-file), 0644 secrets file rejected, 0755 dir rejected, config-without-secrets, mounts empty/nonempty, invalid strategy, save/delete ordering, atomicity, list-skips-malformed, file-permission assertions, `DeleteOrphanConfig` removes/idempotent.
- `internal/envcap/capture_test.go` — already covered the three §13.2 explicit edge cases (`SetAOff`, `ArrayDeclaration`, `ReadonlyConflict`) plus 12 baseline cases including operator-env-leak, bash-internals-stripping, multiline PEM, unicode, empty values, `set -e false` propagation, context cancellation, missing script.
- `internal/caddy/{generator,reloader,stub}_test.go` — already covered golden-string for one-host, multi-host sorted, drop-zero-hostnames, header-only empty input, `caddy validate`/`reload` arg assertion, validate-failure-bubbles-stderr, stub absent/present.
- `internal/dockerdrv/cli_driver_test.go` — already covered all 13 of §13.4: build args, run args (sorted env), run args (empty env), stop args, start args, remove args, inspect parse, logs follow + tail, network ensure absent + present, ContainerIP parse + empty + not-found, plus stop-not-found and inspect-absent.
- `internal/ids/ids_test.go` — already covered format regex, 1000-rapid-call uniqueness, container-name M1 format, image-ref format.
- `internal/logging/logging_test.go` — already covered stderr-only short-circuit, default-writes-to-file-and-stderr, file-open-failure (skipped under root).
- `internal/config/paths_test.go` — already covered all paths rooted correctly, empty-root falls back, `DECLOUD_ROOT` env honored, default-when-unset.

---

## 3. Per-package test counts (Joel §13)

| Package | Tests | Baseline |
|---|---|---|
| `internal/registry` | 23 | FAIL — `panic: unimplemented` from `NewFSStore` |
| `internal/envcap` | 15 | FAIL — `panic: unimplemented` from `New` |
| `internal/caddy` | 9 | FAIL — `panic: unimplemented` from `newCLIReloaderWithFactory`, `WriteStubIfMissing`, `NewGenerator` |
| `internal/dockerdrv` | 16 | FAIL — `panic: unimplemented` from `newCLIDriverWithFactory`/`NewCLIDriver` |
| `internal/deploy` | 53 (15 service + 6 readiness + 32 lifecycle) | FAIL — `panic: unimplemented` from `NewServiceDeployer`, `NewLifecycle`, `NewHTTPProbeForTest` |
| `internal/cli` | 17 (2 root + 7 deploy_service + 7 lifecycle_cmds + 1 exit_codes table-driven of 15 rows + 1 unknown-err) | FAIL — `panic: unimplemented` from `NewRootCmd`, `newDeployServiceCmd`, all the lifecycle command builders, `buildProductionDeployer`, `buildProductionLifecycle` |
| `internal/ids` | 4 | FAIL — `panic: unimplemented` from `NewDeployID` |
| `internal/logging` | 3 | FAIL — `panic: unimplemented` from `Init` |
| `internal/config` | 4 | **PASS** (no stub; `paths.go` is the one fully-implemented production file in scaffolding) |

**Total:** 144 test functions. 8 packages fail with the expected `panic: unimplemented` baseline; 1 package (`internal/config`) passes — that's correct: `paths.go` is pure data, no Rob work needed there.

---

## 4. `go test ./...` summary (current failing baseline — Rob's starting point)

```
--- FAIL: TestReloader_InvokesCaddyValidate (0.00s)
FAIL	github.com/alexander-fenster/decloud/internal/caddy	0.012s
--- FAIL: TestDeployService_BuildsExpectedRequest (0.00s)
FAIL	github.com/alexander-fenster/decloud/internal/cli	0.012s
ok  	github.com/alexander-fenster/decloud/internal/config	0.009s
--- FAIL: TestLifecycle_UnregisterHappyPath (0.00s)
FAIL	github.com/alexander-fenster/decloud/internal/deploy	0.013s
--- FAIL: TestCLIDriver_BuildArgs (0.00s)
FAIL	github.com/alexander-fenster/decloud/internal/dockerdrv	0.011s
--- FAIL: TestEnvcap_ExportSimple (0.00s)
FAIL	github.com/alexander-fenster/decloud/internal/envcap	0.012s
--- FAIL: TestNewDeployID_FormatRegex (0.00s)
FAIL	github.com/alexander-fenster/decloud/internal/ids	0.011s
--- FAIL: TestInit_StderrOnlyShortCircuit (0.00s)
FAIL	github.com/alexander-fenster/decloud/internal/logging	0.013s
--- FAIL: TestStore_RoundTripConfigAndSecrets (0.00s)
FAIL	github.com/alexander-fenster/decloud/internal/registry	0.012s
FAIL
```

(Each package fails on the first test that hits a stub; the panic kills the package goroutine. That's expected — Rob's job is to make every package come back `ok`.)

`go vet ./...` clean. `go build ./...` clean.

---

## 5. Judgment calls + extension flags

1. **Test seam for the readiness probe.** Joel §9.4 specifies a private `httpProbe` struct holding `*http.Client` + `dockerdrv.Driver`. To let `readiness_test.go` (black-box `package deploy_test`) drive `Wait` without bringing up the whole deployer, I exposed `deploy.ReadinessProbe` (interface) plus `deploy.NewHTTPProbeForTest(driver) ReadinessProbe` as a test seam with a stub. Rob fills both; production code wires the same `*httpProbe` into `serviceDeployer` directly. **Linus: `_ForTest` suffix is the convention I picked to telegraph "this is a test seam, do not call from production code"; if you'd rather it be unexported via a `_test.go`-only export file, that's a one-line move.**

2. **`internal/cli/lifecycle_commands_test.go`** — Joel §13.7 lists eight separate lifecycle delegation tests. I put all seven (unregister/start/stop/restart/status/logs/caddy-reload) in one file because they share the `installMockLifecycle(t)` helper and `runRoot(t, args...)`. Different file naming than Joel literally said, same coverage. Cheaper to maintain.

3. **`gomock.InOrder` discipline.** Where Joel said "assert exact step ordering," I used `gomock.InOrder(...)` for the rollback-and-cleanup paths and the redeploy happy path. For tests where the ordering is implied by data dependencies (e.g., `Inspect` before `Run` in `Start`), I used plain `EXPECT()` without `InOrder` — Gomock's controller still verifies all expectations were met. If Linus wants stricter enforcement, the existing tests can be hardened after Rob's first green run without changing semantics.

4. **`TestDeploy_StopOldFailureAbortsAndDoesNotStartNew`** — the previous draft used `deploy.Status{}.LastDeployedAt` as a placeholder return for `Inspect`, which doesn't compile against the new typed `dockerdrv.InspectResult`. Rewrote to return `dockerdrv.InspectResult{State: "running"}, nil` per the §11.1 contract — that matches §9.2 step 4's "if still running, return errRun" semantics.

5. **`internal/registry/store_test.go` partial-write test** — the only deterministic way to force the secrets-write to fail after the config-write has succeeded (without mocking the filesystem, which `fsStore` doesn't expose a seam for) is `chmod 0500` on the secrets dir before calling Save. Test skips under root because root bypasses the mode check. Rob: if pelletier/go-toml writes via a tmp+rename inside the same dir, this still works because the dir we chmod'd is the parent-of-target, not the target itself.

6. **Status format string.** `lifecycle_commands_test.go::TestStatus_DelegatesToLifecycleAndPrintsResult` asserts stdout contains `"foo"`, `"running"`, and `"decloud-foo"` rather than asserting an exact format string. Joel's §8.3 sketch is `%s state=%s container=%s deploy=%s deployed_at=%s\n` but Joel §15 item #8 explicitly flagged that for Linus. The substring-contains test is forward-compatible with either the space-separated format or a future `--json` change.

7. **No `internal/caddy/stub.go` change.** `WriteStubIfMissing(path)` still has the same one-line signature Joel implied; the tests in `stub_test.go` (already in scaffolding) verify the body Rob will write.

---

## 6. Things Rob needs to know

1. **`deploy.NewHTTPProbeForTest(driver)`** must be callable. It's the only test seam I introduced. Wire it through whatever you build internally — the simplest answer is to construct the real `*httpProbe` and return it as `ReadinessProbe`.
2. **`deploy.ErrEnvCapture`/`ErrBuild`/`ErrRun`/`ErrReadiness`/`ErrCaddyReload`** are already defined in `internal/deploy/service.go` and exported (capitalized). The exit-code mapper already imports these by name from `internal/cli/exit_codes.go`. Don't rename them.
3. **`registry.ErrPartialWrite`** is already defined in `internal/registry/errors.go`. The Save body in §9.5 wraps the secrets-write error with this sentinel via `fmt.Errorf("%w: ...", ErrPartialWrite, ...)`. Tests use `errors.Is(err, registry.ErrPartialWrite)`.
4. **Generated mocks already exist** at `internal/{registry,envcap,caddy,dockerdrv}/mocks/` plus `internal/cli/mocks/`. They're up-to-date with the current interface signatures (which already include `Validate` on Reloader, `Start` and `ContainerIP` on Driver, `DeleteOrphanConfig` on Store). After implementation, run `go generate ./...` and `git status --porcelain` should be empty per §16 receipt item 9.
5. **`cli.runRoot(t, args...)`** is the helper my CLI tests call; it constructs the root via `NewRootCmd()`, points stdout/stderr at buffers, sets args, and calls `ExecuteContext(ctx)`. Rob's `NewRootCmd` must wire all subcommands per §8.1.
6. **`installMockDeployer(t)` and `installMockLifecycle(t)`** swap the package globals `deployerFactory` and `lifecycleFactory`. The tests respect the §8.2 "no `t.Parallel()` in any internal/cli test" constraint (none of mine call it).
7. **`ids.ContainerName("foo")` must return `"decloud-foo"`** literally — every CLI/lifecycle/deploy test bakes in that exact string. The `_ai/container-naming.md` warning about M4 still applies.

---

## 7. Things to call Knuth for

None at this stage. Per Joel §14, the only candidate item ("Rob hits an envcap test failure on macOS he can't quickly diagnose") would surface during Rob's implementation, not Kent's test-writing. The reflection-shim breakage in the previous `service_test.go` was a self-contained typo fix, not a design problem worth Knuth's time.

If anything, watch for:
- `internal/deploy/lifecycle.go` crossing 400 lines (Joel §14 trigger). Lifecycle has 7 methods plus `regenerateAndReload` plus error wrapping; my 32 tests don't dictate code shape but if Rob ends up with case-explosion, refactor before plumbing more.
- The `TestReadiness_ContextCancellationStopsProbe` test allows three legitimate error returns (`context.Canceled`, contains "context canceled" substring, or wraps `ErrReadiness`). If Rob picks one and pins it, the `||` chain in the assert can be tightened later.

---

## 8. Receipt-format (§16) shape, for Rob

Rob's report goes at `_tasks/2026-04-26-m1-implementation/<seq>-rob-implementation.md` per Bureau. It MUST start with the §16 ten-item "Test pass receipt" — the M1 acceptance gate cites it explicitly (Don §6 criterion #1). Item 7 must show every package as `ok`; if any line says `FAIL`, M1 is not done.

The current baseline above (item 7-equivalent showing all-FAIL except `internal/config`) is what Rob is about to make green.

End of Kent report.
