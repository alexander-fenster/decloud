# Linus's plan review of Joel's v2.2 delta — APPROVE

## Verdict: APPROVE

Five of the six items are exact restatements of findings I or Kevlin filed; Joel quoted the verbatim replacement text for each, cited the live (post-v2.1) line numbers, and explicitly flagged that the v2.1-era line numbers in §3.4/§3.5 are stale. That is the correct hygiene for a delta plan. Item 1 (the one strategic finding) is mechanical and correctly scoped. The implementation order is sound. Cross-doc reference in §13.8 verifies cleanly against the actual file. Ship to Kent and Rob.

What follows answers each of Don's six explicit verification questions.

---

## Per-question verdict

### Q1 — Item 1 (§13.1): test name and subtests cover both Stop and Remove cancellation paths? Wrap point correct?

**Test coverage**: PASS. `TestDeploy_RedeployStopRemovePreviousContainerCancelledReturnsErrInterrupted` with two subtests (`stop-cancelled`, `remove-cancelled`) is the correct shape for the §3.4.5 control flow. Joel's two subtests cover both load-bearing return paths:

- `stop-cancelled`: requires `Inspect` to return `State: "running"` so the inner branch fires and the cancellation pre-check at line 191 is exercised. Joel correctly noted this requires `withoutInspectAbsentDefault()` (the default AnyTimes Inspect mock would interfere).
- `remove-cancelled`: requires `Stop → nil` (the not-found-or-still-running branch falls through) so control reaches the Remove site at line 196. The cancellation pre-check at the new Remove site fires.

Joel's call to write a sibling test rather than extending Test 9 (`TestDeploy_DefensiveOrphanInspectCancelledReturnsErrInterrupted`) is correct. The §3.5 harness setup (`withoutInspectAbsentDefault()` + `Store.Load → ErrNotFound`) is materially different from the §3.4.5 setup (`Store.Load → newPrev()`, with the AnyTimes Inspect default still appropriate for non-orphan-path calls). Mixing them in one table forces conditional setup logic. Two clean tests > one branchy table.

Joel also correctly flagged that the existing `TestDeploy_StopOldFailureAbortsAndDoesNotStartNew` (`service_test.go:297-312`) does NOT need editing — it uses `errors.New("stop timed out")`, which does not satisfy `isCancellation`, so the existing `ErrRun` assertion still holds after the fix. I verified this claim by skimming the test body. Correct.

**Wrap point**: PASS. The §13.1 production diff places the `isCancellation` pre-check INSIDE the existing `if !errors.Is(err, dockerdrv.ErrContainerNotFound)` block at the Stop site, and INSIDE the existing `if err != nil && !errors.Is(...)` filter at the Remove site. Joel's reasoning at lines 1525-1527 of the tech plan is correct: an `ErrContainerNotFound` from `Stop` is a desired post-state and should fall through to `Remove`; cancellation is a different shape and only matters on the path that would otherwise wrap as `ErrRun`. Layering cancellation on top of the not-found filter (rather than ahead of it) is the right structural call.

Joel also correctly identified that the Stop site at line 191 is "defensive" because cancellation almost always sends control through the inner Inspect's `ierr != nil` skip → fall-through to Remove. The Remove site at line 196 is the load-bearing fix; the Stop site is symmetry. Both get fixed for parity with §3.5. Correct call.

**One micro-nit (NOT BLOCKING)**: the wrap form `return fmt.Errorf("%w: %w", ErrInterrupted, err)` matches §3.5's wrap form exactly. Symmetry preserved. No re-litigation needed.

### Q2 — Item 5 (§13.5): `isCancellation(err)` placement, wrapped-error handling, six call sites accounted for?

**Placement**: PASS. Joel placed the helper "immediately after the existing `newCleanupContext` helper". Verified by reading `service.go:23-42` — the sentinels, `cleanupTimeout` const, and `newCleanupContext` helper cluster together at the top of the file. Adding `isCancellation` adjacent to `newCleanupContext` co-locates all the cancellation-discipline helpers. This is the right scope (package-private, same file, no exported surface widening, no new imports beyond what `service.go` already imports — `errors` and `context` are both already pulled in).

**Wrapped-error handling**: PASS. The helper uses `errors.Is`, which traverses the `Unwrap` chain. So a wrapped `context.Canceled` (e.g. via `fmt.Errorf("driver Stop: %w", context.Canceled)`) correctly satisfies `isCancellation`. This is the same semantics as the inline idiom it replaces — pure refactor, no behavior change. No tests need editing for this reason. Correct.

**Six call sites accounted for**: PASS. I re-grepped against the live `service.go`:

```
internal/deploy/service.go:201:                if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
internal/deploy/service.go:215:                    if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
internal/deploy/service.go:221:                    if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
internal/deploy/service.go:244:        if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
internal/deploy/service.go:261:        cancelled := errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
internal/deploy/service.go:341:        if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
```

Six exactly. Joel's enumeration is correct. The call-site swaps replace `if errors.Is(...) || errors.Is(...) {` with `if isCancellation(err) {` at five sites, and `cancelled := errors.Is(...) || errors.Is(...)` with `cancelled := isCancellation(err)` at line 261. The line-261 hoist (already in v2.1) is preserved and reads cleaner under the new helper.

**Item 5 → Item 1 ordering**: SOUND (see Q4 below).

### Q3 — Item 6 (§13.6): exact replacement message strings? Consistent with `lifecycle.go:26-29`?

**Replacement text**: PASS. Joel quoted the exact replacement at §13.6, post-state code block:

```go
logger.Warn("cleanup failed; manual removal may be required",
    "container", containerName, "error", <stopErr|rmErr>)
```

This is the verbatim form from Don's lockdown §"Item 6". No hand-waving. No "or similar" weasel words. All four sites get the same fixed message string (`"cleanup failed; manual removal may be required"`); the only variance is the per-site error variable name (`stopErr` vs `rmErr`).

**Consistency with `lifecycle.go:26-29` exemplar**: PASS. I verified the exemplar by reading the file. `lifecycle.go:26-29` reads:

```go
logger.Warn("stop failed during unregister", "error", err)
...
logger.Warn("remove failed during unregister", "error", err)
```

Both use a fixed message string with the action and the context (during unregister), and `error` as a structured field. No variable interpolation into the message. Joel's new form `"cleanup failed; manual removal may be required"` matches this convention exactly:
- Fixed message ✓
- Action + intent ("cleanup failed; manual removal may be required") in the message ✓
- Variable bits (`container`, `error`) in structured fields ✓

The shape is consistent. Grep-stable across all four sites. Operators querying the audit log for this row do `message="cleanup failed; manual removal may be required"` and get all four kinds of cleanup-failure events; they then filter by `step` or `container` field. That is exactly the slog value proposition.

**Recovery hint placement**: Joel correctly noted that the recovery action (`docker rm -f decloud-<name>`) does NOT need to live in every per-event log message because it's already in `_docs/usage.md` §8 (the user-facing recovery doc). The error chain RETURNED to the caller still contains `'docker rm -f decloud-foo'` (verified — see line 217 and 223 of service.go where the orphan-cleanup error returns retain the recovery hint). The slog message is for operator filtering; the error-chain recovery hint is for the user-facing terminal output. Two different audiences, two different surfaces. Correct separation.

### Q4 — §13.7 implementation order: Item 5 (helper) before Item 1 (new sites). Sound?

**SOUND.** The dependency is real:

1. Item 5 introduces `isCancellation`. After Item 5, the existing six sites read `if isCancellation(err) {` and the code compiles + tests pass. Pure refactor, zero behavior change. `go test ./...` MUST be green at this checkpoint.
2. Item 1 adds two NEW cancellation pre-checks at `service.go:185-197`. With the helper already in place, the new pre-checks naturally use `isCancellation(err)` rather than the inline idiom — eight total call sites ALL using the helper, no inconsistency. Without Item 5 first, Rob would either: (a) write the new sites with the inline idiom and refactor them in Item 5 (extra churn), or (b) write Item 5 inline-ahead-of-time and intermix the refactor with the new logic (mixed-concern commit).

Joel's "Item 5 before Item 1" ordering avoids both. After Item 1, all eight call sites uniformly use `isCancellation`. After Items 4 and 6, `service.go` is the new shape Don's lockdown specified. Then Items 2 and 3 are doc-only edits Raymond can run in parallel with Rob's last step.

Don's iteration-2 acceptance criterion 2 ("§3.4.5 checks `isCancellation(err)` before each `ErrRun` wrap; cancellation re-wraps as `ErrInterrupted`. Two new sites.") falls out naturally from this ordering. Correct.

### Q5 — §13.8 cross-doc reference: `_docs/usage.md:237` references the log string?

**VERIFIED.** I grepped `_docs/usage.md` for `removed orphan container` and `removing orphan container`:

```
_docs/usage.md:237: ... The audit log records `removed orphan container from prior interrupted deploy` at warn level. ...
```

One hit, exact string match against the pre-state log message at `service.go:212`. Joel's claim is correct — after Item 4 changes the log message from `"removed"` to `"removing"`, the doc string at `_docs/usage.md:237` will diverge from the production string. Operators grepping the log for the documented phrase would miss the row.

Joel correctly flagged this as a downstream consistency check that falls out of Item 4, NOT as a seventh in-scope item. He routed it to Raymond's iteration-2 doc work. That's the right framing — it's a one-character edit (`removed` → `removing`) in `_docs/usage.md:237`, mechanical, cannot conflict with anything else, but worth calling out so Raymond doesn't ship Item 4 + Item 2 + Item 3 and forget the §13.8 sync.

**Recommendation for Don**: confirm with Raymond's iteration-2 task brief that §13.8 is on the list. If Raymond's brief doesn't already include it, add it. Two-minute fix.

---

## What I checked but did NOT find a problem with

- The §13 delta does NOT scope-creep into §3.4.5's cleanup-context discipline. The §3.4.5 branch uses the request `ctx` (forward progress, correctly) and the v2.2 fix is purely about cancellation discrimination at the existing return paths. No `cleanupCtx` redirection. Correct — that branch hasn't *failed* yet at the Stop/Remove site, so there's nothing to clean up under a non-cancellable budget; the next operations (`Run`, then probe, then potentially the §3.4.1 cleanup) handle their own cleanup-context discipline. Joel did NOT confuse "cancellation-aware error wrap" with "cleanup-context discipline." This is a precision win.
- The new test does NOT exercise `cleanupCtx` (because §3.4.5 doesn't use one). Joel's test shape correctly asserts on the returned error chain only. No `notCancelledCtxMatcher` involvement, no contortion to fit a pattern that doesn't apply. Good.
- The §13.5 helper's docstring is honest: it names both the predicate (`reports whether err is a context cancellation or deadline`) and the policy reason (`so the orchestrator can wrap as ErrInterrupted (exit 130) rather than the step-specific ErrRun/ErrReadiness sentinels`). A future maintainer reading the helper understands both *what* it does and *why* it exists. That is the bar.
- Item 6's "container appears twice" finding is a real logging consistency issue, not a stylistic preference. Joel correctly cited `lifecycle.go:25-30` as the existing convention exemplar. The replacement form matches the convention exactly. Four sites, one mechanical edit per site, zero ambiguity.
- The §13.10 acceptance criteria delta correctly carries forward v2.1's acceptance criteria (clean go test/vet/gofmt + zero `%w: %v`) and adds the v2.2-specific criteria. Don and I can verify "iteration-2 done" against this list mechanically.
- §13.9 (out-of-scope) correctly captures Linus's Issue 2 Option B (real second-signal exit-fast behavior) as backlog item 9 — Don's lockdown already noted this. Kevlin's nested-indent observation on the `service.go:198-227` block correctly stays out of scope (Kevlin himself decided against demanding the refactor; the test surface is locked, the cost-benefit is wrong for now).
- Joel's "no test churn" claims at §13.5 and §13.6: correct. The `isCancellation` refactor doesn't change observable behavior (tests assert on the returned error chain, not on which exact predicate spelled out the cancellation check). The slog message change doesn't affect any test (no test asserts on the `"cleanup failed; please remove"` substring; verified by Kevlin in his finding #4 and re-verified by the test grep against the existing service_test.go).

---

## What concerns me strategically (and why they're not blockers)

### Concern: the §13.1 fix reproduces §3.5's pattern under a different `ctx` discipline

§3.5 uses the request `ctx` for forward-progress operations and only switches to `cleanupCtx` when something fails. §13.1 (§3.4.5) is the same shape: forward-progress on the request `ctx`, with the new cancellation pre-check before the `ErrRun` wrap.

What I want to verify is that the §13.1 fix does NOT silently introduce an asymmetry of its own. Specifically: if cancellation fires during the §3.4.5 Stop and we now wrap as `ErrInterrupted`, the orchestrator returns immediately. That returns control to `Deploy`'s caller without running ANY post-Stop cleanup — but post-Stop cleanup at this site doesn't exist (no `cleanupCtx` block follows; the next thing the orchestrator would do is `Run` the new container, which obviously shouldn't proceed on cancellation). This is correct behavior — return early on cancellation, leave the user-facing surface clean.

The asymmetry I worried about: does §13.1's fix orphan a partially-stopped previous container? Answer: no. If `Stop` returned `context.Canceled`, the previous container is in some partial state — but the next deploy's §3.5 defensive orphan check will catch it (the previous container carries the `decloud.service=<name>` label, so the §3.5 cleanup applies). Recovery on next deploy. Same SIGKILL/power-loss recovery shape that the headline fix relies on. Defended.

This is a strategic concern only in the sense that I want it on the record: the §13.1 fix is correct because the §3.5 recovery exists. Joel didn't spell this out in §13.1 (he could have, in three sentences). Not blocking — the reasoning is implicit in the v2.1 lockdown's "next deploy recovers" framing — but worth recording here for future-Linus's benefit.

### Concern: zero test churn claim at §13.5 is technically sleight-of-hand

Joel claims "Test churn (existing tests): NONE" for Item 5. Strictly true for the *six existing call sites* — the `isCancellation` helper is observably equivalent. But Item 5 is a prerequisite for Item 1, and Item 1 adds new behavior with a new test. So the COMBINED v2.2 churn is "1 new test for §13.1, zero edits to existing tests." Joel said this honestly elsewhere (§13.7 step 2 + §13.10 criterion 3), but a reader skimming §13.5 in isolation could mis-read "no test churn" as "no new tests for v2.2 at all."

Trivially fixable doc nit; not blocking. If Joel does a v2.2.1 doc revision, change §13.5's "Test churn" line to "Test churn (existing tests): NONE. (See §13.1 for the new test that fires after both Item 5 and Item 1 land.)" — but I'm not asking for a revision over this.

### Concern: the §13.4 Item 4 update will produce a transiently broken doc until §13.8 syncs

If Rob lands Item 4 (production log change to `"removing"`) before Raymond lands the §13.8 doc sync, the doc says `"removed"` but the audit log emits `"removing"`. Operators reading the doc and grepping the log get an empty result. Window of vulnerability is however long iteration-2's Rob → Raymond hand-off takes.

Mitigation: Joel's §13.7 implementation order has Raymond running steps 5 and 6 (`_docs/usage.md` and `_ai/exit-code-sentinel-not-context-err.md`) AFTER Rob's steps 1–4. If §13.8's doc sync at `_docs/usage.md:237` is added to Raymond's iteration-2 brief (per my recommendation in Q5), it lands in the same step as Item 2's `:240` rewrite — both on the same file, Raymond's iteration-2 commit covers both. The transient-broken-doc window collapses to zero (or to the inter-commit gap if Raymond commits between Items 2 and 13.8, which is operator discretion).

Strategic concern: minor. Don should ensure Raymond's brief covers §13.8 explicitly. Not a plan defect.

---

## Strategic non-issues (verified, not concerns)

- Cross-doc verification of §13.8 succeeded on the first try. Joel's grep claim at `_docs/usage.md:237` is exact.
- The `_ai/exit-code-sentinel-not-context-err.md:69` line range (Item 3) is verified — `exit_codes_test.go:40-43` has the four rows (`interrupted`, `interrupted-wrapped`, `context-canceled-bare`, `context-deadline-bare`), as Joel and Kevlin both claimed. Trivial doc fix.
- Item 4's one-word change (`removed` → `removing`) is mechanical. No tests reference the substring.
- Item 6's four-site mechanical edit will not break tests (Kevlin verified this in his finding #4; Joel re-verified in §13.6).
- The implementation order at §13.7 maintains `go test ./...` green at every checkpoint (steps 1, 2, 3, 4 all leave the codebase in a buildable, testable state because each is either a pure refactor with equivalent semantics, or a mechanical edit, or a new behavior with its own test). Don's iteration-2 acceptance criterion 8 is achievable mechanically.
- Joel did NOT scope-creep into §3.5 (already shipped clean), §3.4.1 audit-log fork (already shipped clean), `cleanupCtx` plumbing (already shipped clean), `restoreOldContainer` parameter rename (already shipped clean), or any of the v2.1 cleanup-context discipline. v2.2 is a delta on the surface polish of v2.1, not a structural revisit. Right framing.
- The §13.11 summary table is a useful at-a-glance view: 1 item per row, source citation, file:line, edit shape, owner, test churn. Future-Don can read this in 30 seconds.

---

## Praise (rare)

Joel's §13 delta is the right way to do iteration planning:
- DELTA, not rewrite. v2.1's §1–§12 stay authoritative for everything not explicitly overridden.
- LIVE line numbers, not stale ones. Joel grepped the post-v2.1 file and explicitly flagged that the v2.1 plan's line numbers in §3.4/§3.5 are no longer authoritative.
- VERBATIM replacement text from Don's lockdown for each item, with no editorial modifications. Don set the policy; Joel transcribed it. Right boundary.
- IMPLEMENTATION ORDER spelled out with the `Item 5 before Item 1` dependency called out explicitly. Rob can follow the order mechanically.
- ACCEPTANCE CRITERIA delta reads as a checklist — Don and I can verify "done" mechanically without re-reading the whole plan.
- §13.8 cross-doc note is a downstream consistency check Joel did not have to flag (it falls outside the six items) but DID flag because it's the right thing to do.
- §13.9 out-of-scope section explicitly captures what's NOT being fixed (Linus's Option B, Kevlin's indent observation). Future-Don who comes back to this task in six months can see what was intentionally deferred vs. what was missed.

This is the second time in this task Joel has taken Don's lockdown text and produced a delta that is implementable from end to end. The first time (v2.1, after Don's §007 lockdown) shipped clean — Kevlin and I both APPROVED. The v2.2 delta will likely ship the same way.

---

## Recommendation

**APPROVE.** No revisions requested. Route to Kent for the new test and Rob for the production edits per §13.7's implementation order. Raymond runs steps 5–6 in parallel; ensure his brief covers §13.8's downstream sync of `_docs/usage.md:237` (one-character edit alongside the §13.2 sentence rewrite — same file, same commit).

After iteration-2 EXECUTION, Kevlin and I re-review in parallel. If both APPROVE with no new findings (the expected outcome — every item is mechanical), iteration-3 PLAN is a one-paragraph "all green, ship it" lockdown from Don.

— Linus
