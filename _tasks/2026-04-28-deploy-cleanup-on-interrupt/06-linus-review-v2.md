# Linus's plan review v2 — deploy cleanup on interrupt

## Verdict: APPROVE (with one minor follow-up flagged for awareness, not blocking)

Don conceded all six items I raised in v1, picked the right options on each, and revised both `02-plan.md` and `03-tech-plan.md` in place with proper revision-history headers and a v2 summary table. The backlog entries (items 7 and 8) are well-written. Kent can start writing tests.

---

## Issue-by-issue check

### Issue 1 (label-gated orphan cleanup) — RESOLVED, well

`InspectResult.Labels map[string]string` is the right shape. New optional field on an existing struct, no breaking change to call sites. Existing `dockerdrv.InspectResult{State: "absent"}` literals continue to compile (Labels zero-values to nil). The migration of `cliDriver.Inspect` from whitespace `--format` to JSON `--format` is correct — labels can contain arbitrary characters and the old `strings.Fields` parser would have been a footgun. The `{{json .Config.Labels}}` template function is standard Docker template syntax.

The label-mismatch tests (§5.10) cover both failure modes (missing label, mismatched value) and assert the recovery hint. The driver-level tests (§5.11) cover argv shape and JSON parsing including the nil-labels case. Adequate.

The error wrap shapes for the §3.5 branch follow `_ai/error-wrap-discipline.md` correctly:
- Refusal path: `fmt.Errorf("%w: ... label decloud.service=%q does not match %q; ...", ErrRun, ..., labelVal, reqName, ...)` — `%w` on the sentinel, `%q` on string values, no inner error chain (because the refusal isn't caused by another error). Correct.
- Cleanup-failure path: `fmt.Errorf("%w: cleaning up orphan container %s; please run 'docker rm -f %s' and retry: %w", ErrRun, ..., err)` — `%w: <ctx>: %w` shape. Correct.
- Inspect-error path: `fmt.Errorf("%w: inspect orphan check %s: %w", ErrRun, ..., err)` — `%w: <ctx>: %w` shape. Correct.

**Don was right to push back on his original v1 plan softening this to "no registry entry → destroy."** The label gate converts silent destruction to a labelled contract.

### Issue 2 (ExitCodeFor over-broad) — RESOLVED

`ExitCodeFor` now matches only `deploy.ErrInterrupted`. The bare `context.Canceled` / `context.DeadlineExceeded` cases are dropped from §3.6 and from the §5.8 test table. The optional negative-test cases (`context.Canceled` → `ExitInternal`, `context.DeadlineExceeded` → `ExitInternal`) are a nice belt-and-suspenders that locks the new contract against future "helpful" regressions. Joel marked these optional; I'd say take them — they're cheap insurance.

Joel's note that the `context` import in `exit_codes.go` may drop is correct; verify during implementation.

### Issue 3 (probe wrap shape + log fork) — RESOLVED, cleanly

`readiness.go:69-72` is now untouched — `return ctx.Err()` raw. The orchestrator-side audit log fork at §3.4.1 is right:

```go
if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
    logger.Info("deploy cancelled during readiness wait", "step", "readiness")
} else {
    logger.Error("readiness failed", "step", "readiness", "error", err)
}
```

**On Don's question of whether this leaks policy from CLI to deploy package:** No. The deploy orchestrator owns its own logging. CLI policy is exit-code mapping in `internal/cli/exit_codes.go`. Deciding "log Info vs Error based on the cause" is the orchestrator's business — it has the context (deploy_id, service name, current step) that the CLI layer doesn't. This is correctly placed.

The duplicated cancellation predicate (computed once for the log fork, once for the return wrap) is a minor stylistic question. Not blocking; Rob can hoist if it offends him.

### Issue 4 (harness AnyTimes default) — RESOLVED, with a verifiable assumption

Don claims gomock matches LIFO (most recently added expectation first). Per the uber-go/mock godoc and `WithOverridableExpectations`, this is correct: more recent specific expectations take precedence over an earlier `AnyTimes()` default. Tests with explicit `Inspect(gomock.Any(), "decloud-foo").Return(running, ...)` registered AFTER `newDeployerHarness(t)` will match before the harness's `Inspect(Any, Any).Return(absent).AnyTimes()` default.

**Concern that I considered and dismissed:** does an over-permissive `AnyTimes()` default mask the §3.5 contract? Answer: only the dedicated §3.5 contract tests (§5.3, §5.4, §5.5, §5.10) verify "Inspect IS called." If a future regression makes the implementation skip §3.5 entirely, those tests catch it. The other 11 tests don't owe verification of §3.5; they verify their own behaviors. Concentration of contract verification in dedicated tests is correct.

**One sanity-check Kent should run early:** the existing `TestDeploy_StopOldFailureAbortsAndDoesNotStartNew` already has its own `Inspect(...).Return(running, nil).AnyTimes()` for the hasPrev stop-failure inspect path (a separate use of `Inspect` at `service.go:175`). With the harness adding ANOTHER `Inspect(Any, Any).Return(absent).AnyTimes()`, gomock's LIFO precedence should let the test's own (more specific by service name) expectation win. If for some reason it doesn't (gomock's matcher semantics are subtler than the docs suggest), Kent will see test failures and we'll debug. **Not a blocker; flagging for Kent's awareness.**

### Issues 5 & 6 (backlog entries) — RESOLVED

Items 7 and 8 in `_ai/m1x-backlog.md` are well-written. Each has Where/Why deferred/Fix shape/Originator with cross-references to this task. Future-Don can find both the originating analysis and the proposed fix shape. Good librarianship.

---

## Did v2 introduce any new problems?

**One minor consistency hole, NOT a blocker:**

§3.5 (defensive orphan cleanup) runs on the request `ctx`, not the cleanup ctx — correct, this is forward progress. But: if the user presses ctrl+c during this branch, `Inspect` returns `context.Canceled`, and §3.5 wraps as:

```go
return fmt.Errorf("%w: inspect orphan check %s: %w", ErrRun, containerName, err)
```

This surfaces as `ErrRun`. Then `ExitCodeFor` matches `ErrRun` → exit 40 (`ExitRunFail`), NOT `ErrInterrupted` → 130. The user pressed ctrl+c during the orphan check, but they see "run failure" semantics.

Two observations:
1. **This existed in v1 too** (v1's defensive cleanup branch had the same shape, just without label gating). It's not a v2 regression.
2. **The fix is symmetric to §3.4.3:** check `errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)` and wrap as `ErrInterrupted` instead of `ErrRun`. One conditional, three lines.

I'm not blocking on this. It's a tiny consistency hole; the user's headline complaint (orphan leaks) is fixed regardless. **DON: consider whether to add cancellation-detection to §3.5's three return-error sites for symmetry with §3.4.3.** If you say "ship it as-is, fix in a follow-up," I accept. If you say "fix now, three lines in three sites," I also accept. I won't make this a blocker because it's narrow (only fires if the user ctrl+c's during the ~50ms orphan inspect call) and the existing v1 had the same hole.

**Nothing else smells worse than v1.** The changes are:
- Driver interface widening: clean.
- JSON format for Inspect: more robust than the whitespace shape.
- Audit log fork: correctly placed in the orchestrator.
- Harness default: real win (eliminates 11 mechanical edits).
- ExitCodeFor tightening: right call.
- Probe wrap: correct (no change to readiness.go).
- Backlog entries: well-written.

---

## What Joel and Rob need to verify during implementation

1. **gomock LIFO precedence in practice.** The harness `AnyTimes()` default plus per-test specific expectations should "just work" — but Kent should run the existing test suite at step 3 of the v2 implementation order (per Joel's P.P.S.) BEFORE introducing any new behavior. If pre-existing tests fail, the LIFO assumption is wrong and the harness needs `gomock.Not("decloud-foo")` or similar specificity. Joel's gut and the godoc both say it works. Verify empirically.

2. **JSON `--format` output shape from real docker.** Joel sketched `'{"id":{{json .Id}},"state":{{json .State.Status}},"labels":{{json .Config.Labels}}}'`. Rob should confirm with a one-shot `docker inspect <some-container> --format='...'` against a real container before locking the format string into `cli_driver_test.go` argv expectations.

3. **The `context` import in `exit_codes.go`** may or may not drop after §3.6 simplification. Verify.

4. **The `strings` import in `readiness_test.go`** becomes unused after §5.7. `goimports` removes; otherwise `go test` fails.

These are implementation-time verifications, not plan defects.

---

## Signoff

The plan correctly addresses all six items from `04-linus-review.md`. The architecture is sound. The test plan covers the new contract (label gating, audit log fork, exit-code precision) without padding. The backlog entries record the deferred items. Implementation order is sensible (driver-level changes first, then harness, then orchestrator changes, then new tests).

**Don, Joel: ship it.**

Kent: write the tests at §5.1 (with §5.2 folded in), §5.3, §5.4, §5.5, §5.6, §5.7 (update), §5.8 (with the optional negative cases — take them), §5.10 (both variants), §5.11 (driver-level). Use the harness `AnyTimes()` default per §5.9 / §4.4. The `notCancelledCtxMatcher`, `cancellingProbe`, and `newDeployerHarnessWithProbe` are all reasonable additions; don't bikeshed the names.

Rob: implement in Joel's P.P.S. v2 order. Run `go test ./...` after each step. Watch the gomock precedence behavior at step 3 — if any pre-existing test breaks, stop and re-evaluate before continuing.

Raymond: when docs phase comes, the `_docs/usage.md` paragraph needs to mention the label-gated refusal behavior and the recovery hint. Don't bury it.

— Linus

---

## Optional (not blocking): the §3.5 cancellation-during-orphan-inspect consistency hole

If Don wants to close this in the same task, the three return-error sites in §3.5 each get a check before the wrap:

```go
if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
    return fmt.Errorf("%w: %w", ErrInterrupted, err)
}
return fmt.Errorf("%w: inspect orphan check %s: %w", ErrRun, containerName, err)
```

Three sites, six lines. If Don says "follow-up," add a third backlog entry. If Don says "fix now," it's mechanical.

Either is acceptable. This is a heads-up, not a blocker.
