# Kevlin's low-level review — deploy cleanup on interrupt

## Verdict: APPROVE (with two trivially fixable doc/log nits)

The implementation matches the v2.1 spec line-for-line. Build/vet/gofmt clean, full test suite green, the `%w: %v` grep finds nothing. Tests assert observable contracts (cleanup ctx is non-cancelled at call time, error chains satisfy `errors.Is`, refusal messages contain user-actionable substrings) — none read as change-detector tests. The cancellation symmetry §3.5 promised in Don's lockdown is in place at all three sites. The documentation update is mostly faithful; one cited line range in `_ai/exit-code-sentinel-not-context-err.md` slipped past Raymond's self-audit and one audit-log string in `service.go` is past-tense before the work is done. Both fixable in <30 seconds; neither blocks the merge.

The codebase reads better after this change than before it. The probe-failure block is the test case: ten lines of `_ = d.deps.Driver.Stop(ctx, ...)` ignored-error swallowing become an explicit cleanup window with a fork between cancellation-info and failure-error logging. That is the rare refactor that makes the file shorter to *read* even when it is longer to *measure*.

---

## Sanity check (numbers, not opinion)

```
go build ./...                              -> ok
go vet ./...                                -> ok
gofmt -l internal/ cmd/                     -> empty
grep -rn '%w: %v' internal/ cmd/            -> empty
go test ./...                               -> all pass (deploy 12.06s)
```

All pre-existing tests pass; all 12 new/updated tests pass. The `TestDeploy_DefensiveOrphanInspectCancelledReturnsErrInterrupted` table-driven test runs three subtests (inspect, stop, remove cancellation) — all green, confirming the v2.1 cancellation-symmetry lockdown landed everywhere it was supposed to.

---

## Findings — actionable

### 1. (nit, doc) `_ai/exit-code-sentinel-not-context-err.md:69` cites the wrong line range

The "All four in `internal/cli/exit_codes_test.go:38-41`" claim is off by two lines: the four interrupted/bare-ctx rows actually live at `exit_codes_test.go:40-43`. Lines 38-39 are the `caddy-down` rows. Raymond's self-audit at `010-raymond-docs.md:61` claims the rows are at `exit_codes_test.go:38-43` — close, but the focused `_ai` doc rounded too aggressively.

**Fix (doc-only typo):** change `:38-41` to `:40-43` in `_ai/exit-code-sentinel-not-context-err.md:69`. Single line edit.

This is the one symbol Raymond cited that did not survive verification. Every other line range / symbol / test name / file path I checked grepped clean — the audit was thorough.

### 2. (nit, log) `service.go:212` audit log is past-tense before the work has happened

```go
logger.Warn("removed orphan container from prior interrupted deploy",
    "container", containerName, "state", inspect.State)
if err := d.deps.Driver.Stop(ctx, containerName, 10*time.Second); err != nil ...
if err := d.deps.Driver.Remove(ctx, containerName); err != nil ...
```

The log says "removed" before `Stop` and `Remove` have run. If either fails (or is cancelled) the operator's log shows "removed orphan container ..." followed by the failure — which is mildly misleading during incident review.

**Fix options (pick one):**
- Move the log line to *after* the successful `Remove` returns (cleanest; the Warn fires only on the success path, mirroring `lifecycle.go:25-30`).
- Reword to present tense: `"removing orphan container from prior interrupted deploy"`. Smallest diff.

I lean **option B** because it's a one-word edit and the log already lives at the head of the orphan branch where its position-in-flow tells the operator "this is where decloud is starting to act." Option A is structurally cleaner but pushes the Stop/Remove error-handling further away from the log line that announces the intent.

Either is fine. Not a blocker — the log is present-of-action-about-to-happen rather than past-of-action-just-done, which is a venial sin in a sea of cardinal sins this task could have committed.

### 3. (style, optional) `errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)` repeats six times in `service.go`

Sites: 201, 215, 221, 244, 261 (hoisted to `cancelled`), 341. Each spelling is identical. A two-line helper:

```go
func isCancellation(err error) bool {
    return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}
```

…would let every site read `if isCancellation(err) { return fmt.Errorf("%w: %w", ErrInterrupted, err) }`. Six invocations of an idiom is the threshold where the idiom deserves a name. The hoist Rob already did at `261` for the audit-log fork is the same instinct on a smaller scale.

Joel explicitly pre-approved this hoist in `03-tech-plan.md` §3.4.1 ("If Linus pushes back on duplication, hoist to `cancelled := ...`") and Rob took the local hoist there but not the function-level one. Linus didn't push back in `06-linus-review-v2.md` §"Issue 3 (probe wrap shape + log fork)" — he flagged it as "minor stylistic question. Not blocking; Rob can hoist if it offends him."

It mildly offends me, and Kevlin Henney would call it "comments asking for a name." But it does not offend me enough to block the merge. **Recommend a follow-up; not a precondition.** The behaviour is identical; the reader's eye learns the pattern after the second occurrence anyway.

### 4. (nit, structured-logging) `slog.Warn` message string interpolates `containerName` AND passes it as a structured field

```go
logger.Warn("cleanup failed; please remove "+containerName+" manually",
    "container", containerName, "error", stopErr)
```

The container name appears twice: once inside the human-readable message and once as the `container=...` structured key. Operators querying the log for `container=decloud-foo` would find the row regardless of message text; conversely operators reading the message line see the name without needing to consult the kv pairs. So the duplication is a small cost (extra string concatenation, slightly noisier log line) for a small benefit (both readers see the name).

The dominant pattern in `service.go`'s slog calls is **fixed message string + structured fields** (e.g. `"network ensure failed", "step", "network", "error", err` at line 145). This new pattern is the only place that breaks it. Either match the convention — `"cleanup failed; manual removal may be required"` with the name in the structured field — or accept the inconsistency.

**Strongly recommend matching the convention.** Two reasons: (a) `slog`'s value proposition is that the message string is grep-stable across deploys, with variable bits in structured fields; concatenating the name into the message defeats grep-stability. (b) `lifecycle.go:26-29` (cited as the exemplar by `03-tech-plan.md` §4.1) already follows the convention — match it.

This is a four-site change in `service.go` (lines 270, 274, 331, 335) and the existing test `TestDeploy_DefensiveOrphanCleanupFailureWrapsErrRun` only asserts on the *returned* error (line 459 `assert.Contains(t, err.Error(), "docker rm -f decloud-foo")`), not on the slog Warn message — so no test changes are needed.

Not blocking; the field is already there for any operator who wants it. But it's the kind of small inconsistency that compounds.

---

## Findings — explicit non-issues (verified, listed for the record)

- **`_ai/error-wrap-discipline.md`:** every new `fmt.Errorf` in `service.go` and `cli_driver.go` uses `%w` or `%w: %w`. No `%w: %v`. Grep confirms zero. ✓
- **`Inspect` JSON parsing:** verified end-to-end against four JSON shapes (`null`, `{}`, value-with-spaces-and-quotes, missing labels key). All parse correctly with `Labels` either nil or non-nil-empty as appropriate. The label-gate code at `service.go:207` reads `inspect.Labels["decloud.service"]` which yields `""` for nil maps, missing keys, and empty maps — all of which then fail the `labelVal != req.Name` check and trigger the refusal path. The contract holds for every shape docker can plausibly emit. ✓
- **`cleanupCtx` defer scoping:** Go defers fire on function return, not block exit. Each cleanup block creates one `cleanupCtx`; the `if err != nil` path always returns immediately after the cleanup block. So at most one defer registers per `Deploy` invocation. No leak. ✓
- **`restoreOldContainer` parameter rename to `cleanupCtx`:** the rename is the entire point of the change — at every call site (`service.go:242, 278, 339`) the caller passes a `newCleanupContext()`-derived value. A future maintainer who tries to pass `ctx` (the request ctx) sees an immediate type-name mismatch at the call site. Self-documenting. ✓
- **Audit-log fork on cancellation vs failure (`service.go:262-266`):** correctly uses `slog.Info` for cancellation and `slog.Error` for real readiness failure. Matches the per-step convention (`logger.Info("network ensured", ...)`, `logger.Error("readiness failed", ...)`). The `cancelled` boolean is hoisted once and reused at line 280 — Joel's pre-approved hoist option exercised. ✓
- **`ExitCodeFor` ordering:** `errors.Is(err, deploy.ErrInterrupted)` is correctly placed BEFORE the deploy.ErrRun/ErrReadiness/etc cases. Cancellation that wraps `ErrInterrupted` plus an inner `ErrReadiness` (theoretical; doesn't happen today but defended against) would otherwise route to `ExitReadinessFail` instead of `ExitInterrupted`. Plan §3.6 called this out explicitly; implementation honours it. ✓
- **`InspectResult.Labels` field addition is non-breaking:** every existing `InspectResult{State: "absent"}` literal in tests (e.g. `internal/deploy/lifecycle_test.go`, the harness default at `service_test.go:153`) zero-values `Labels` to nil and continues to compile. No mock regeneration needed. ✓
- **gomock FIFO surprise:** Kent caught this empirically and Raymond documented it in `_ai/gomock-fifo-matching.md`. The opt-out option `withoutInspectAbsentDefault()` is used at the seven sites that need it (267, 391, 419, 443, 499, 525, 590). The remaining tests genuinely don't need to assert anything about `Inspect` and the AnyTimes default absorbs the call. ✓
- **The `%q` formatting for the mismatched label and req.Name (`service.go:209-210`):** correctly emits `decloud.service="bar"` and `"foo"` — quoted, so the user can see the exact label value the host had vs. what we wanted. The test `TestDeploy_DefensiveOrphanRefusesContainerWithMismatchedLabel` asserts the literal `decloud.service="bar"` substring. ✓
- **The `cleanup-context-discipline.md` line-range citations** (`service.go:32-42`, `:260-287`, `:198-227`, `:185-197`, `:209`, `:240`, `:323`): all verified by grep against the current file. ✓
- **`_docs/usage.md` §8 error-message rendering:** the doc shows `decloud.service="..."` and `'docker rm -f decloud-<name>'` placeholders that match the actual `%q`-quoted format at `service.go:209-210`. The user-facing "container <name> exists but was not created by decloud" prose matches. The exit-code table addition at line 106 (`130 / ExitInterrupted`) reads correctly. The `ExitRunFail` clause at line 102 forward-references §8 for the label-mismatch case. ✓
- **Test name citations in `_ai/cleanup-context-discipline.md` and `_ai/label-gated-orphan-recovery.md`:** every cited test (`TestDeploy_ProbeCancellationCleansUpWithFreshContext` at 364, `TestDeploy_RestoreOldContainerUsesFreshContextOnRedeployCancellation` at 467, `TestDeploy_DefensiveOrphan*` at 390/418/442/498/524/550, `TestCLIDriver_InspectParsesDecloudServiceLabel` at 268, `TestCLIDriver_InspectReturnsNilLabelsWhenContainerHasNone` at 280) exists and lives where the doc claims (give or take one line for inclusion of the comment header). ✓
- **`_ai/MEMORY.md` index entries** (lines 22-25): four new patterns added in the existing "Implementation patterns (reusable)" section, each one-liner-with-cross-reference. Matches the file's style. ✓
- **No new unused imports.** `exit_codes.go` did NOT need to add `context` (the bare-ctx case dropped per Linus Issue 2; only the test file imports `context` for the negative-case rows). `cli_driver.go` correctly added `encoding/json`. `service.go` already imported everything needed. ✓
- **No commented-out tests, no `t.Skip`, no debug `slog.Debug` left over, no TODOs added in this task.** ✓

---

## Pattern adherence (the `_ai/` directory check)

I checked the implementation against the relevant `_ai/` patterns the codebase already locked in:

| Pattern | Adhered? | Notes |
|---|---|---|
| `error-wrap-discipline.md` | ✓ | `%w: %w` everywhere; grep confirms zero `%w: %v` |
| `gomock-inorder-sequencing.md` | ✓ | `gomock.InOrder` used in `TestDeploy_DefensiveOrphanCleanupOnFreshDeployWhenContainerExists` (390-414) and `TestDeploy_DefensiveOrphanCleanupSkippedWhenContainerAbsent` (418-440). Contract tests, not implementation tests. |
| `optional-input-two-layer.md` | n/a | No new optional inputs in this task. |
| `cobra-init-pattern.md` | n/a | No CLI init paths touched. |
| `explicit-inputs-not-globals.md` | ✓ | `cleanupTimeout` is a package const (an explicit input); `newCleanupContext` is a package-private helper. No new globals, no new env-var reads. |
| `cleanup-context-discipline.md` (this task added it) | ✓ self-consistent | The pattern this task introduces is faithfully followed by the same task that introduced it — recursion of intent. |
| `label-gated-orphan-recovery.md` (this task added it) | ✓ self-consistent | Same. |
| `gomock-fifo-matching.md` (this task added it) | ✓ self-consistent | The opt-out `withoutInspectAbsentDefault` is applied at every site that needs it. |
| `exit-code-sentinel-not-context-err.md` (this task added it) | ✓ self-consistent | `ExitCodeFor` matches only `deploy.ErrInterrupted`; negative-test rows lock the contract. |

Four new pattern files. Each is genuinely reusable, each cross-references the others, each has a "When to apply" section, each has a "Locked in by" test list. This is the right level of patterning for a codebase that has just learned a new shape of bug — codify it once, point future-maintainers at it.

---

## Code quality / story-telling assessment

**The probe-failure block (`service.go:260-287`) is the centerpiece. Read it standalone:**

```go
if err := d.probe.Wait(ctx, containerName, spec, req.Port); err != nil {
    cancelled := errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
    if cancelled {
        logger.Info("deploy cancelled during readiness wait", "step", "readiness")
    } else {
        logger.Error("readiness failed", "step", "readiness", "error", err)
    }
    cleanupCtx, cleanupCancel := newCleanupContext()
    defer cleanupCancel()
    if stopErr := d.deps.Driver.Stop(cleanupCtx, containerName, 10*time.Second); ... {
        logger.Warn("cleanup failed; please remove "+containerName+" manually", ...)
    }
    if rmErr := d.deps.Driver.Remove(cleanupCtx, containerName); ... {
        logger.Warn(...)
    }
    if hasPrev {
        d.restoreOldContainer(cleanupCtx, prev)
    }
    if cancelled {
        return fmt.Errorf("%w: %w", ErrInterrupted, err)
    }
    if errors.Is(err, ErrReadiness) {
        return err
    }
    return fmt.Errorf("%w: %w", ErrReadiness, err)
}
```

The story arc reads: **observe** (probe error) -> **classify** (cancellation or failure) -> **audit** (Info or Error) -> **clean up under a non-cancellable budget** -> **rollback if applicable** -> **return the right sentinel.** Six lines of comment would tell that story; the code structure tells it without. That is the bar Henney would set.

**Counterpoint: the defensive orphan branch (`service.go:198-227`) is denser than I'd like.** It nests `if !hasPrev { if err != nil { if cancellation { ... } } if state != absent { if labelVal != reqName { ... } if stopErr ... if rmErr ... } }` four levels deep. The behaviour is correct; the indentation tax is real. A small refactor — extract `defensiveOrphanCleanup(ctx, containerName, reqName) error` returning the right sentinel — would let the body of `Deploy` read at a single level of indentation. But that's a structural change, and the test surface is well-locked, so a follow-up is fine.

I considered demanding the extraction. Decided against because: (a) the duplication isn't lethal, (b) the existing tests are tight contracts on the orchestrator's behaviour, and (c) extracting now means re-writing `010-raymond-docs.md`'s line citations to point at a new function. The cost-benefit favours leaving it for a follow-up. Item for `m1x-backlog.md` if Don agrees it's worth tracking.

---

## What I checked but did NOT find a problem with

- Boundary conditions on `cleanupTimeout = 30 * time.Second`: enough for `docker stop -t 10s` (10s grace) + `docker rm` (~1s) + rollback `docker run` (~1s) with ~18s spare for daemon contention. Sized right.
- The `cancellingProbe` and `notCancelledCtxMatcher` test helpers: small, single-purpose, well-commented. Reused appropriately.
- Mock regeneration: not needed (Labels is a field on a struct, not a new method).
- The `passThroughProbe` vs `cancellingProbe` swap via `newDeployerHarnessWithProbe`: clean two-helper split, no test rewrites needed for the existing happy-path tests.
- The four `cleanup failed; please remove ... manually` slog.Warn sites all have `error=<stop|rm err>` structured fields; an operator can `grep ' container=decloud-foo '` and find the exact failure.

---

## Recommendation

**APPROVE.** The two trivially-fixable items (doc line-range typo, audit-log past-tense) are not worth bouncing the task. Items 3 and 4 are stylistic preferences worth a five-line follow-up commit; not blockers.

If Don wants belt-and-suspenders, fix items 1 and 2 inline before he signs off, and file items 3 and 4 against `m1x-backlog.md` as a single "polish pass on cleanup-context call sites" entry. Either path is acceptable. The cleanup-on-interrupt fix itself is solid.

— Kevlin
