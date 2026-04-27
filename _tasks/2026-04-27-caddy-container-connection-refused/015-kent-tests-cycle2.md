# 015 — Kent's Cycle-2 Test Report

Author: Kent Beck (test engineer)
Date: 2026-04-27
Status: Cycle-2 EXECUTION 3.1 complete. Test added, fails as expected. Awaiting Rob's implementation in Phase 2.

## Reading log

1. `_tasks/.../013-joel-tech-plan-cycle2.md` — §1.5 specifies the test shape; §1.3 specifies the literal the test locks.
2. `_tasks/.../014-linus-review-cycle2.md` — APPROVED, with two verification-gate items (helper smoke test on Phase 5; verify `TestManager_UpRunFailsWithoutRollback` sentinel does not contain either substring).
3. `internal/caddy/manager.go` — current `Up` body, line 94-96, the wrap point Rob will edit.
4. `internal/caddy/manager_test.go` — the harness conventions (`newManagerHarness`, `absentInspect`, `gomock.InOrder`, `h.manager` accessor).

The plan's §1.5 example used `h.mgr` but the actual harness exposes `h.manager` — I matched the existing file's accessor, no other deviations.

## What I added

**File:** `internal/caddy/manager_test.go`

**Test:** `TestManager_UpPortsBoundActionableError`

Two table-driven sub-tests:
- `kernel bind` — drives `RunWithOptions` to return an error containing `Error starting userland proxy: listen tcp4 0.0.0.0:80: bind: address already in use`.
- `docker allocator` — drives `RunWithOptions` to return an error containing `Bind for 0.0.0.0:80 failed: port is already allocated`.

The `runErr` is shaped via `fmt.Errorf("docker run: exit status 125; stderr=%q", tc.stderrSub)` — exactly the wrap shape from `internal/dockerdrv/cli_driver.go:239`, so the substring match in `Manager.Up` will see the canonical chain.

Each sub-test runs the standard fresh-install precondition chain (`NetworkEnsure → Inspect(absent) → ImagePull → RunWithOptions(err)`) and asserts:

1. `errors.Is(err, caddy.ErrCaddyUp)` — sentinel preserved.
2. `Contains(err.Error(), "ports 80/443 already in use")` — actionable symptom text.
3. `Contains(err.Error(), "systemctl disable --now caddy && systemctl mask caddy")` — recovery command #1.
4. `Contains(err.Error(), "apt-get remove -y caddy")` — recovery command #2.
5. `NotContains(err.Error(), ": docker run: docker run:")` — locks the branch choice. The cycle-1 generic-wrap path produces this doubled prefix (manager wrap + driver wrap); the actionable branch must not contain it.

## One import added

Added `"fmt"` to `internal/caddy/manager_test.go` imports — needed to shape the wrapped error literal in the test.

## What I did NOT add

- No `Long` help-text test (per Joel §1.6 / task instructions — Cobra renders `Long` independently of `Short`).
- No driver-layer test changes (per Joel §1.2 — detection lives in the manager, not the driver).
- No new helpers — the existing `newManagerHarness`, `absentInspect`, `expectedCaddyRunOptions` cover the setup. A one-shot per-sub-test `runErr := fmt.Errorf(...)` is below rule-of-three for extraction; inline is the right call.

## Verification — go build

```
$ go build ./...
(no output, exit 0)
```

## Verification — failing test output

```
$ go test ./internal/caddy/... -run TestManager_UpPortsBound -count=1 -v
=== RUN   TestManager_UpPortsBoundActionableError
=== RUN   TestManager_UpPortsBoundActionableError/kernel_bind
    manager_test.go:202: 
        	Error Trace:	/Users/fenster/dev/decloud/internal/caddy/manager_test.go:202
        	Error:      	"caddy: up failed: docker run: docker run: exit status 125; stderr=\"Error starting userland proxy: listen tcp4 0.0.0.0:80: bind: address already in use\"" does not contain "ports 80/443 already in use"
        	Test:       	TestManager_UpPortsBoundActionableError/kernel_bind
    manager_test.go:203: 
        	Error Trace:	/Users/fenster/dev/decloud/internal/caddy/manager_test.go:203
        	Error:      	"caddy: up failed: docker run: docker run: exit status 125; stderr=\"Error starting userland proxy: listen tcp4 0.0.0.0:80: bind: address already in use\"" does not contain "systemctl disable --now caddy && systemctl mask caddy"
        	Test:       	TestManager_UpPortsBoundActionableError/kernel_bind
    manager_test.go:204: 
        	Error Trace:	/Users/fenster/dev/decloud/internal/caddy/manager_test.go:204
        	Error:      	"caddy: up failed: docker run: docker run: exit status 125; stderr=\"Error starting userland proxy: listen tcp4 0.0.0.0:80: bind: address already in use\"" does not contain "apt-get remove -y caddy"
        	Test:       	TestManager_UpPortsBoundActionableError/kernel_bind
    manager_test.go:205: 
        	Error Trace:	/Users/fenster/dev/decloud/internal/caddy/manager_test.go:205
        	Error:      	"caddy: up failed: docker run: docker run: exit status 125; stderr=\"Error starting userland proxy: listen tcp4 0.0.0.0:80: bind: address already in use\"" should not contain ": docker run: docker run:"
        	Test:       	TestManager_UpPortsBoundActionableError/kernel_bind
=== RUN   TestManager_UpPortsBoundActionableError/docker_allocator
    manager_test.go:202: 
        	Error Trace:	/Users/fenster/dev/decloud/internal/caddy/manager_test.go:202
        	Error:      	"caddy: up failed: docker run: docker run: exit status 125; stderr=\"Bind for 0.0.0.0:80 failed: port is already allocated\"" does not contain "ports 80/443 already in use"
        	Test:       	TestManager_UpPortsBoundActionableError/docker_allocator
    manager_test.go:203: 
        	Error Trace:	/Users/fenster/dev/decloud/internal/caddy/manager_test.go:203
        	Error:      	"caddy: up failed: docker run: docker run: exit status 125; stderr=\"Bind for 0.0.0.0:80 failed: port is already allocated\"" does not contain "systemctl disable --now caddy && systemctl mask caddy"
        	Test:       	TestManager_UpPortsBoundActionableError/docker_allocator
    manager_test.go:204: 
        	Error Trace:	/Users/fenster/dev/decloud/internal/caddy/manager_test.go:204
        	Error:      	"caddy: up failed: docker run: docker run: exit status 125; stderr=\"Bind for 0.0.0.0:80 failed: port is already allocated\"" does not contain "apt-get remove -y caddy"
        	Test:       	TestManager_UpPortsBoundActionableError/docker_allocator
    manager_test.go:205: 
        	Error Trace:	/Users/fenster/dev/decloud/internal/caddy/manager_test.go:205
        	Error:      	"caddy: up failed: docker run: docker run: exit status 125; stderr=\"Bind for 0.0.0.0:80 failed: port is already allocated\"" should not contain ": docker run: docker run:"
        	Test:       	TestManager_UpPortsBoundActionableError/docker_allocator
--- FAIL: TestManager_UpPortsBoundActionableError (0.00s)
    --- FAIL: TestManager_UpPortsBoundActionableError/kernel_bind (0.00s)
    --- FAIL: TestManager_UpPortsBoundActionableError/docker_allocator (0.00s)
FAIL
FAIL	github.com/alexander-fenster/decloud/internal/caddy	0.008s
?   	github.com/alexander-fenster/decloud/internal/caddy/mocks	[no test files]
FAIL
```

Both sub-tests fail with the expected mismatch. The actual rendered error (`caddy: up failed: docker run: docker run: exit status 125; stderr="..."`) is the cycle-1 generic-wrap path — confirming that today's `Manager.Up` does NOT take an actionable branch on either substring. After Rob's Phase 2 implementation, all four positive assertions and the negative branch-choice assertion will flip to PASS.

## Verification — siblings still green

```
$ go test ./internal/caddy/... -count=1 -v | grep -E "^--- (PASS|FAIL)"
--- PASS: TestGenerator_OneServiceOneHost (0.00s)
--- PASS: TestGenerator_MultiServiceMultiHostSorted (0.00s)
--- PASS: TestGenerator_DropsZeroHostnameServices (0.00s)
--- PASS: TestGenerator_EmptyInputProducesHeaderOnly (0.00s)
--- PASS: TestManager_UpFreshInstall (0.00s)
--- PASS: TestManager_UpAlreadyRunning (0.00s)
--- PASS: TestManager_UpAfterPriorStop (0.00s)
--- PASS: TestManager_UpUnexpectedStateWraps (0.00s)
--- PASS: TestManager_UpNetworkEnsureFails (0.00s)
--- PASS: TestManager_UpImagePullFails (0.00s)
--- PASS: TestManager_UpRunFailsWithoutRollback (0.00s)
--- FAIL: TestManager_UpPortsBoundActionableError (0.00s)
--- PASS: TestManager_UpStubWriteFailsWrappedAsCaddyUp (0.00s)
--- PASS: TestManager_UpStubCaddyfileWritten (0.00s)
--- PASS: TestManager_UpStubWriteIdempotent (0.00s)
--- PASS: TestManager_DownHappyPath (0.00s)
--- PASS: TestManager_DownContainerAbsent (0.00s)
--- PASS: TestManager_DownStopFailsHard (0.00s)
--- PASS: TestManager_IsRunningTrue (0.00s)
--- PASS: TestManager_IsRunningFalseWhenExited (0.00s)
--- PASS: TestManager_IsRunningFalseWhenAbsent (0.00s)
--- PASS: TestReloader_ValidateCallsDockerExec (0.00s)
--- PASS: TestReloader_ReloadCallsDockerExec (0.00s)
--- PASS: TestReloader_PathTranslationCanonicalForm (0.00s)
--- PASS: TestReloader_PathTranslationOutsideBindMount (0.00s)
--- PASS: TestReloader_PathTranslationParentEscape (0.00s)
--- PASS: TestReloader_ContainerNotRunningSurfacesActionableError (0.00s)
--- PASS: TestReloader_ContainerExitedSurfacesActionableError (0.00s)
--- PASS: TestReloader_ValidateExitNonzeroPreservesStderr (0.00s)
--- PASS: TestReloader_StderrIsCapturedEvenWithoutCallerWriter (0.00s)
--- PASS: TestStub_WritesValidCaddyfileWhenAbsent (0.00s)
--- PASS: TestStub_NoOpWhenFileExists (0.00s)
```

Only `TestManager_UpPortsBoundActionableError` fails. Every other test in the `caddy` package passes — including `TestManager_UpRunFailsWithoutRollback`, the test Linus asked me to verify still locks the cycle-1 generic-wrap path for C2-3.

## Linus's verification-gate item — addressed

Linus §7 note 2: verify `TestManager_UpRunFailsWithoutRollback` sentinel error contains neither substring.

The test's `innerErr := errors.New("port allocation failed")`. Substring check:
- `"port allocation failed"` does NOT contain `"address already in use"`. ✓
- `"port allocation failed"` does NOT contain `"port is already allocated"`. ✓ (note the word ordering — `"port allocation"` vs `"port is already allocated"` — they share the word "port" but the canonical Docker phrase is not a substring of the test's sentinel).

So C2-3's negative coverage stands: that test will continue to assert the generic-wrap path (`%w: docker run: %w`) fires for non-port-conflict failures after Rob lands the substring branch.

## What Rob needs to do

Per Joel §1.3:

1. Replace lines 94-96 of `internal/caddy/manager.go` with a substring-detecting branch that returns:
   ```
   caddy: up failed: ports 80/443 already in use; if you ran the M1.0 install, run 'systemctl disable --now caddy && systemctl mask caddy' or 'apt-get remove -y caddy' to make the change persistent
   ```
2. Add `isPortsBoundErr` helper near the bottom of `manager.go` (after `runOpts`), matching `address already in use` OR `port is already allocated` via `strings.Contains` on `err.Error()`.
3. Add `"strings"` to the imports.

After that, my new test passes — both sub-tests, all five assertions each — and `TestManager_UpRunFailsWithoutRollback` continues to pass (its sentinel misses both substrings).

## Files touched

- `internal/caddy/manager_test.go` — added `"fmt"` import; added `TestManager_UpPortsBoundActionableError` after `TestManager_UpRunFailsWithoutRollback` and before `TestManager_UpStubWriteFailsWrappedAsCaddyUp`.

`gofmt -l internal/caddy/` empty after the edit.

— Kent
