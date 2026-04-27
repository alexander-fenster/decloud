# Iter2 Step 3d — Kevlin re-review of fix-up code + docs

**Author:** Kevlin Henney (low-level reviewer)
**Status:** **PASS.** Both prior Blockers resolved with the right fix shape, no shortcuts. Praise for the architectural decisions Joel made on the way (especially the `Capture("")` defensive contract and the `%w: %w` audit). Two non-blocking suggestions for future M1.x. No regressions.
**Build verified:** `go vet ./...` clean, `gofmt -l .` empty, `go test ./...` all 9 packages green (172 tests). Live-binary test of `decloud --help` against `DECLOUD_ROOT=/nonexistent/path/abc` exits 0 with full subcommand list, no stderr noise about FS access. Subcommand-level help (`decloud deploy --help`, `decloud deploy service --help`) also exits 0.

---

## Verification of prior findings

### B1 — `decloud --help` on a fresh box → **FIXED**

Verified end-to-end:

1. **`cmd/decloud/main.go`** (5 lines of body): no `logging.Init()` call, no `logging` import. Just `signal.NotifyContext` → `cli.NewRootCmd().ExecuteContext(ctx)` → `os.Exit(cli.ExitCodeFor(err))`. Tells its story in five lines; perfect.
2. **`internal/cli/root.go:22-24`**: `PersistentPreRunE = func(cmd, args) error { return logging.Init() }`. Cobra's `--help` and `help` short-circuit *before* `PersistentPreRunE` fires — confirmed by the live-binary test above and by the `TestRoot_HelpDoesNotRequireFilesystem` test in `root_test.go:55-65`.
3. **`internal/logging/logging.go:21-43`**: the EACCES/ENOENT fallback is in place. `MkdirAll` failure → one stderr warning + `setStderrOnly()` + `return nil`. `OpenFile` failure → same. No more exit-70 on a fresh box.

I traced the code with `DECLOUD_ROOT=/nonexistent/path/abc`:
- `decloud --help` → cobra short-circuits, no Init call, exit 0. Confirmed at the binary level.
- `decloud deploy --help` → same. Confirmed.
- `decloud deploy service --help` → same. Confirmed.
- `decloud deploy service --name x /tmp` (a real subcommand) → `PersistentPreRunE` fires → `logging.Init()` → `MkdirAll("/nonexistent/path/abc/logs")` returns `mkdir /nonexistent: permission denied` → fallback engages → one stderr warning → `Init` returns nil → deploy proceeds (and then fails further on for unrelated reasons because the source dir is empty, but that's not exit 70).

The fix is exactly what I asked for in B1 option 1+option 2 combined. Joel/Rob picked both; that's the right call because the env-var test escape hatch (`DECLOUD_LOG_TO_STDERR_ONLY=1`) and the operator-fresh-box fallback are different concerns and now have different implementations.

**Praise for the warning text:** `decloud: log dir unavailable, using stderr only: <err>` is exactly the right message — it names what's happening, what we did about it, and the underlying cause. Two notable subtleties Rob's report flagged:
- The warning leaks to test stderr on every CLI test. Cosmetic only — no test asserts stderr-cleanliness. The strategy doc (`m1-test-strategy.md` §5) names the env-var workaround. Acceptable.
- Init now returns nil on every M1 path; signature stays `error` so future I/O failures can surface. Defensible — no need to drop the return type just because nothing currently uses it.

### B2 — `--env-file` auto-discovery + optional → **FIXED**

Three layers verified:

1. **`internal/cli/deploy_service.go:103-118`**: `resolveEnvFile(flagValue, sourceDir)` implements the precedence exactly as Joel specified:
   - explicit flag → stat; if missing, return `envcap.ErrEnvScriptMissing`; if other stat error, return `envcap.ErrEnvScriptUnreadable`; else return the path
   - empty flag → stat `<sourceDir>/env.sh`; if present, use it; else return `""`
   - No env-var fallback, no fancy precedence. Deterministic and one-screen-readable.

2. **`internal/envcap/capture.go:46-49`**: `Capture(ctx, "")` returns `(nil, nil)` defensively. Comment in capture.go is missing — Rob shipped Joel's snippet without the explanatory "production code MUST NOT rely on this" comment. **Not a blocker** because the orchestrator has the `if envFile != ""` guard one level up, and `grep -rn 'Capturer\.Capture' internal/ cmd/` confirms exactly one production caller (`service.go:140`) — which is the orchestrator. The defensive return is reachable only by future callers that ignore the contract; an inline comment would help the next reader. Captured as Suggestion S-NEW-1 below.

3. **`internal/deploy/service.go:137-149`**: orchestrator skips `Capture` when `envFile == ""` and logs `env capture skipped: no env script`. The log message is honest and useful for operator debugging — this is the kind of "the code tells the operator what it did" detail that matters.

4. **`internal/cli/exit_codes.go:44-45`**: `envcap.ErrEnvScriptMissing` and `envcap.ErrEnvScriptUnreadable` map to `ExitConfigError` (10). Verified by `TestDeployService_ExplicitEnvFileMissingReturnsExitConfigError` which passes `--env-file=/no/such/file` and asserts `errors.Is(err, envcap.ErrEnvScriptMissing) && ExitCodeFor(err) == ExitConfigError`.

Quick-start example in `_docs/usage.md` §1 will now actually work end-to-end (modulo the docker/caddy install steps the user already does).

### S1 — Seven near-duplicate lifecycle command files → **NOT TOUCHED (correct)**

All seven files (`unregister.go`, `start.go`, `stop.go`, `restart.go`, `status.go`, `logs.go`, `caddy_reload.go`) are unchanged from iter1. Don deferred this to M1.x. Rob did not half-do it. Good — touching half of them and leaving the other half would have been worse than touching none.

### S3 — `else after if-return` in readiness.go → **FIXED**

`internal/deploy/readiness.go:47-67` now uses a `switch` with three branches:
- `ipErr != nil` → `lastErr = ipErr`
- `ip == ""` → `lastErr = dockerdrv.ErrNoBridgeIP` (the silent branch is now loud)
- default → probe; success returns nil; failure assigns `lastErr`

The inner `else`-after-return that I called out as "the same shape and gets the same treatment" is now the explicit `if/else` inside the default case. That's still technically `else`-after-not-quite-return (the `return nil` is in the `else`), but it's two lines and unambiguous. Acceptable.

The new `TestReadiness_EmptyIPNoErrorReportsErrNoBridgeIP` (`readiness_test.go:129-141`) verifies the silent branch is now loud: `Driver.ContainerIP` returns `("", nil)`, the probe returns `errors.Is(err, deploy.ErrReadiness) && errors.Is(err, dockerdrv.ErrNoBridgeIP)`. The `%w: %w` wrap in `readiness.go:64` is what makes the `dockerdrv.ErrNoBridgeIP` traverse — Item 6 and Item 5 dovetail correctly.

### S4 — `%w: %v` → `%w: %w` across 21 sites → **FIXED**

Spot-checked 5 sites:
- `internal/deploy/service.go:133` (NetworkEnsure) — `%w: ensuring decloud network: %w` ✓
- `internal/deploy/service.go:168` (Build) — `%w: %w` ✓
- `internal/deploy/lifecycle.go:43` (Stop) — `%w: stop %s: %w` ✓
- `internal/registry/store.go:140` (mkdir secrets) — `%w: mkdir secrets dir %s: %w` ✓
- `internal/deploy/readiness.go:64` (readiness wrap) — `%w: %w` ✓

`grep -rn '%w: %v' /Users/fenster/dev/decloud/internal/ /Users/fenster/dev/decloud/cmd/` returns zero matches. All 21 sites are converted.

The optional `TestDeploy_BuildErrorPreservesInnerSentinel` proves the chain works: a synthetic `errors.New("synthetic build err")` wrapped as `fmt.Errorf("docker build: %w", sentinel)` survives the orchestrator's `fmt.Errorf("%w: %w", ErrBuild, err)` wrap and is recoverable via `errors.Is(err, sentinel)`. Good — this is the test that would catch a regression to `%v` in the future.

### S6 — `assert.True(t, errors.Is(...))` → `assert.ErrorIs(...)` → **NOT TOUCHED (correct)**

Don deferred. I confirmed via `grep -rn 'assert\.True(t, errors\.Is\|require\.True(t, errors\.Is'` that the verbose form is still in use (≈30+ sites across `lifecycle_test.go`, `service_test.go`, `readiness_test.go`, etc.). No partial conversion. Good — half-done refactors are worse than not-done refactors.

### S7 — `NewHTTPProbeForTest` rename → **FIXED**

`internal/deploy/readiness.go:19-22` exports `NewHTTPProbe`. The lowercase `newHTTPProbe` private constructor stays as the production path. All six call sites in `readiness_test.go` are now `deploy.NewHTTPProbe(driver)`:
- line 60, 69, 91, 102, 122, 141 — confirmed via grep.
- No `NewHTTPProbeForTest` reference anywhere in `internal/` or `cmd/`.

The doc comment on `NewHTTPProbe` no longer apologizes ("do not call from production"); it just says "constructs the production HTTP readiness probe". The signal-to-noise on this symbol went up.

---

## New checks for iter2

### Dead code introduced? — **NO**

Item 5 added `ErrNoBridgeIP` *use* (the dockerdrv error already existed; the readiness probe now references it). Verified the surface end-to-end:
- `dockerdrv/driver.go:20` — sentinel definition (pre-existed).
- `dockerdrv/cli_driver.go:183` — `ContainerIP` returns it when `docker inspect` returns empty IP (pre-existed).
- `deploy/readiness.go:53` — readiness probe records it as `lastErr` when driver returns `("", nil)` (NEW in iter2).
- `deploy/readiness.go:64` — wraps as `%w: %w` so it traverses through `ErrReadiness`.
- `deploy/readiness_test.go:140` — test asserts `errors.Is(err, dockerdrv.ErrNoBridgeIP)` traverses.

`ErrNoBridgeIP` is now properly surfaced through the readiness path. Operators will see clearer error messages on degenerate driver responses.

### Footgun in `Capture("")` returning `(nil, nil)`? — **NO (with one caveat)**

Production has exactly one caller: `service.go:140`, gated by `if envFile != ""` on line 139. So `Capture("")` is unreachable in production. The defensive return is for future callers that bypass the orchestrator's guard.

**Caveat (S-NEW-1):** the `Capture("")` empty-path branch in `capture.go:46-49` has no inline comment explaining why it's there. The full discussion is in Joel's plan §architectural decision #3 and Rob's report §subtle-behavior #2, but neither is visible to a future reader of `capture.go`. Three lines of comment would earn their keep:

```go
// Capture("") returns (nil, nil) defensively. The production orchestrator
// (deploy/service.go) skips this call when EnvFile is empty; this branch
// exists only so a future bypass doesn't crash with a stat-empty-path error.
// Production code MUST NOT rely on this — pass an empty path means "no
// env capture" at the call site, not here.
if scriptPath == "" {
    return nil, nil
}
```

Not a blocker (the contract is correct; the comment would just make the contract self-documenting). Captured as a non-blocking suggestion for future Don.

### `PersistentPreRunE` and `--help` — **VERIFIED CLEAN**

Three help paths tested against the live binary with `DECLOUD_ROOT=/nonexistent/path/abc`:
1. `decloud --help` → exits 0, no `Init` call (cobra short-circuits before PersistentPreRunE).
2. `decloud deploy --help` → exits 0, same path.
3. `decloud deploy service --help` → exits 0, same path.

I also checked the `help` subcommand:
4. `decloud help deploy` → exits 0.

All four cases short-circuit before `PersistentPreRunE`. Cobra's behavior here is documented: `cobra.Command.Help` and the implicit `help` subcommand bypass PreRun hooks. The `TestRoot_HelpDoesNotRequireFilesystem` test (`root_test.go:55-65`) locks this in for `--help`; the other three are not explicitly tested but rely on the same cobra mechanism. Acceptable.

### Raymond's docs — **ACCURATE**

Spot-checked four claims:

1. **Exit-code table** (`usage.md:88-99`):
   - Row 10 now includes "explicit `--env-file=<path>` pointing at a missing or unreadable file" — verified against `cli/exit_codes.go:44-45` and `cli/deploy_service.go:103-118`.
   - Row 10 also says "`decloud stop`, `start`, `restart`, or `logs` against a container that is not registered" — verified against `lifecycle.go:40-42` (Stop) and `:127-128` (Logs); Start/Restart route through `Store.Load` returning `ErrNotFound`.
   - Row 40 now says "`docker network create` (the deployer ensures the `decloud` network on every deploy)" — verified against `service.go:131-134` wrapping `NetworkEnsure` failures as `ErrRun`.
   - Row 40 explicitly excludes `docker stop`-on-missing-container, redirecting to row 10 — accurate.

2. **§1 quick-start `env.sh` optional language** (`usage.md:9-11`): "If you do not pass `--env-file`, Decloud looks for `<source-dir>/env.sh` and uses it if it exists; if it does not, the container runs with no captured environment. Passing `--env-file=<path>` to a missing file is a hard error (exit 10) — auto-discovery is silent, but explicit asks must succeed." — verified against `resolveEnvFile()` in `deploy_service.go:103-118`. The wording is precisely the contract.

3. **§2 step 0** (`usage.md:76`): "Ensure the `decloud` Docker network exists. Missing networks are created on the fly; failures here surface as exit 40." — verified.

4. **install.md §3 `ExecReload`** (line 43): no `--force` — verified, matches `internal/caddy/reloader.go:38-48` which does NOT pass `--force`.

No hallucinations in the iter2 doc deltas. Field names in the example status output (`name`, `state`, `container`, `deploy`, `deployed_at`) match `internal/cli/status.go:25-26` byte-for-byte from iter1; iter2 didn't touch those.

### `m1-test-strategy.md` accuracy — **ACCURATE**

Read end-to-end (`_ai/decisions/m1-test-strategy.md`, 53 lines). Section-by-section verification:

- **§1 user directive**: quotes "I will test it on a real system after M1 is done" — matches the task brief at `01-user-request.md`.
- **§2 per-package test seam**:
  - `dockerdrv` "argument-construction tests via injectable `exec.Command` factory" — verified at `internal/dockerdrv/cli_driver.go` (the `cmdFactory` field) and `cli_driver_test.go` (the recording factory).
  - `caddy` "golden-string tests" + "`Reloader.Validate`/`Reloader.Reload` exercised via recording `cmdFactory`" — verified at `internal/caddy/`.
  - `deploy` "every step of the orchestrator sequence (now including step 0 `NetworkEnsure` per item 3)" — accurate; my reading of `service_test.go` confirms step 0 is exercised.
  - `envcap` "runs against the real `/bin/bash` on the maintainer's box" — verified, no `-tags` gate; macOS bash 3.2 is the floor.
- **§3 receipt** matches Rob's `017-rob-fixup-impl.md` §10-item handoff receipt exactly (Go version, host, bash, docker, caddy, test output, vet, gofmt, generate, files modified).
- **§4 expected breakage classes** is honest (docker version skew, bash version skew, caddy semantic differences, network-namespace assumptions). This is exactly what M2 should look at first when the user reports something.
- **§5 `DECLOUD_LOG_TO_STDERR_ONLY=1` tip** is accurate per `logging.go:22-25`.

The doc is precisely the kind of decision-record we want: it tells future-Don *why* M1 has no integration tests, names every test seam, and predicts the failure modes that the unit-tests-only strategy will miss. Not aspirational. Reflects actual test behavior.

---

## New issues introduced in iter2

**None of blocker severity.**

Two non-blocking suggestions (M1.x candidates):

### S-NEW-1. `Capture("")` defensive branch needs three lines of comment

`internal/envcap/capture.go:46-49` returns `(nil, nil)` for empty path. The contract is defensible (Joel architectural decision #3) but invisible from this file. Add a comment explaining "production callers must guard at the call site; this branch is here for safety, not as a documented public API." See "Footgun" check above for the suggested wording.

### S-NEW-2. `decloud: log dir unavailable, using stderr only: ...` warning leaks to test stderr

Confirmed in Rob's §subtle-behavior #1. Cosmetic — no test asserts stderr-cleanliness, no test fails because of the message. The strategy doc §5 names the env-var workaround. If we ever decide test stderr should be clean, the fix is one line in `logging.go` to suppress the warning when `os.Getenv("DECLOUD_TEST_QUIET")` or similar is set; but that's M1.x bikeshed territory.

---

## Praise

- **`Capture("")` defensive return + orchestrator-side guard.** This is the right architectural call. The Capturer's contract stays simple (a real path means real work, no path means no work); the orchestrator owns the decision of whether to call. Joel's architectural decision #3 captures this and Rob implemented it cleanly. Two-layer defense without coupling.
- **`%w: %w` audit was thorough.** All 21 sites Joel enumerated are converted. The optional `TestDeploy_BuildErrorPreservesInnerSentinel` test proves the change is real, not cargo-cult. Future callers can use `errors.Is` to peek inside the error chain — a free improvement that costs five characters per call site.
- **`PersistentPreRunE` placement.** The test (`TestRoot_HelpDoesNotRequireFilesystem`) and the live-binary verification both confirm the cobra-help-short-circuit. This is exactly the deferred-init pattern the cobra docs recommend; Rob picked the right tool.
- **`switch` rewrite of the readiness loop.** The three-branch switch (ipErr / empty IP / probe) reads top-to-bottom like the contract it implements. The previous if/else-if/else chain hid the empty-IP case. Now that case has a name (`dockerdrv.ErrNoBridgeIP`) and a test.
- **Decision record.** `_ai/decisions/m1-test-strategy.md` is precisely the artifact future-Don will need. Names every test seam, predicts every failure class, doesn't pretend integration tests exist. Raymond's prose is dense without being terse — five sections, every claim cross-referenced to code or task brief.
- **Restraint on deferred items.** Rob did NOT touch S1 (lifecycle command duplication) or S6 (assert.ErrorIs migration). Half-done refactors are worse than not-done refactors. The deferred items remain on the M1.x list with their full original scope.

---

## Test verification

Live run on this box: `go test ./...` → all 9 packages PASS, 172 tests, 0 failures, 0 skips except the root-EUID guard tests in `logging_test.go` (which correctly skip when running as root, n/a here). `go vet ./...` clean. `gofmt -l .` empty.

Targeted re-runs of the 5 most important new tests:
- `TestReadiness_EmptyIPNoErrorReportsErrNoBridgeIP` — PASS
- `TestDeploy_NetworkEnsureCalledFirst` — PASS (gomock.InOrder confirms NetworkEnsure precedes Capture/Build/Run)
- `TestDeploy_NetworkEnsureFailureReturnsErrRun` — PASS (errors.Is(err, ErrRun) traverses)
- `TestDeploy_NoEnvScript_SkipsCapturerEntirely` — PASS (Capturer.EXPECT().Capture(...).Times(0) is satisfied)
- `TestDeploy_BuildErrorPreservesInnerSentinel` — PASS (proves %w:%w fix is real)

Live binary test:
- `decloud --help` with `DECLOUD_ROOT=/nonexistent/path/abc` → exit 0, full subcommand list, no FS access, no stderr noise.

---

## Verdict

**PASS.** Both Blockers from `011-kevlin-review.md` are fully resolved with the right fix shape. Suggestions S3, S4, S7 are fully resolved. Suggestions S1 and S6 are correctly deferred to M1.x. No regressions. Two new non-blocking suggestions (S-NEW-1 comment on `Capture("")`, S-NEW-2 stderr warning quiet-mode for tests) are M1.x candidates, not M1 blockers.

Ship M1. Hand it to the user.

End of Kevlin re-review.
