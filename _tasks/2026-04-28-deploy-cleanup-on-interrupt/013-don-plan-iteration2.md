# Don's iteration-2 lockdown — fix-in-scope, all five

## Decision summary

| Item | Source | Severity | Decision | Site count |
|---|---|---|---|---|
| 1. §3.4.5 redeploy stop+remove cancellation asymmetry | Linus, `12-linus-impl-review.md` Issue 1 | Strategic | **FIX IN SCOPE** | 2 sites, ~6 lines |
| 2. `_docs/usage.md:240` second-ctrl+c falsehood | Linus, `12-linus-impl-review.md` Issue 2 | Doc inaccuracy | **FIX IN SCOPE** | 1 sentence |
| 3. `_ai/exit-code-sentinel-not-context-err.md:69` line-range typo | Kevlin, `011-kevlin-review.md` nit 1 | Doc typo | **FIX IN SCOPE** | 1 character pair |
| 4. `service.go:212` past-tense "removed" before action | Kevlin, `011-kevlin-review.md` nit 2 | Log correctness | **FIX IN SCOPE** | 1 word |
| 5. `isCancellation(err)` helper hoist | Kevlin, `011-kevlin-review.md` style 3 + Linus Issue 4 | Style | **FIX IN SCOPE** | 1 helper, 6 call-sites updated |
| 6. slog message-vs-field duplication | Kevlin, `011-kevlin-review.md` style 4 + Linus Issue 5 | Style/convention | **FIX IN SCOPE** | 4 sites |

**Verdict: one more iteration. EXECUTION pass with Joel/Kent/Rob/Kevlin/Linus.**

## Reasoning, item by item

### Item 1 — §3.4.5 cancellation asymmetry (Linus Issue 1)

**Verified**: Read `service.go:185-197`. The `hasPrev` redeploy stop+remove branch returns `fmt.Errorf("%w: stop previous container: %w", ErrRun, err)` and `fmt.Errorf("%w: remove previous container: %w", ErrRun, err)` without checking for `context.Canceled` or `context.DeadlineExceeded`. The §3.5 `!hasPrev` branch immediately below it (lines 198-227) DOES check at three sites (201, 215, 221).

**This is the same bug shape Linus and I fixed in the v2.1 lockdown.** Same predicate, same wrap, sibling code paths, asymmetric contracts. A user pressing ctrl+c during a fresh-deploy orphan check sees exit 130; the same user pressing ctrl+c during a redeploy old-container stop sees exit 40. Same key combo, same intent, two exit codes.

**The lockdown precedent is binding on me.** From `007-don-final-lockdown.md`:

> "The whole point of this task is getting cancellation semantics right. Shipping with a known cancellation-mis-wrap inside the same task that's specifically about cancellation discipline would be cowardice. 'Narrow window' is a valid observation; it's not a valid excuse."

That standard applies verbatim here. If I let this one slip into a backlog item now, I am cherry-picking when the lockdown applies, and my own words a week ago condemn me. The maintenance-test argument is unchanged: I do not want future-Don debugging "why does ctrl+c sometimes give exit 40 and sometimes exit 130 depending on which stage we're in?" at 2 AM.

**Linus's mea culpa is honest and worth recording**: he flagged this as "his fault to miss in v2." That is what good high-level review looks like — circle back when you find a gap. Doesn't excuse us from fixing it; it commits us to fixing it.

**Linus's recommendation**: Option A — fix in scope, ~6 lines, two sites, one new test row mirroring `TestDeploy_DefensiveOrphanInspectCancelledReturnsErrInterrupted` against the §3.4.5 sites. I take Option A.

**Cost**: ~30 minutes total. Two cancellation pre-checks (six lines), one table-driven test row (or two subtests if Kent prefers parity with §3.5's three-subtest shape — Kent's call). Joel updates the tech plan to v2.2 with a one-paragraph delta on §3.4.5.

### Item 2 — `_docs/usage.md:240` second-ctrl+c falsehood (Linus Issue 2)

**Verified**: Read `_docs/usage.md:240`. The doc says "A second ctrl+c (impatient double-tap) bypasses graceful cleanup and may leave the container behind." Linus is correct — `signal.NotifyContext`'s handler stays registered until `stop()` runs in the deferred path. Second SIGINT is absorbed by the package handler, not propagated to the OS default handler. The user has to wait for the 30s `cleanupTimeout` or `kill -9`. The doc is technically wrong.

**Decision**: Linus's Option A — one-sentence rewrite. The intent the doc tries to communicate is correct; the mechanism it claims is wrong. Raymond fixes in 30 seconds. The "real" fix (Linus's Option B — wire actual second-signal exit-fast behavior) is a backlog item, not in scope.

**Doc text** (Raymond, this is the replacement for line 240):

> "A second ctrl+c during cleanup does not interrupt cleanup; the Go signal handler absorbs it. To force exit before the 30-second cleanup window completes, send SIGKILL (`kill -9 <pid>`); the orphan recovery in path (1) above still applies on the next deploy."

**Cost**: 30 seconds.

### Item 3 — `_ai/exit-code-sentinel-not-context-err.md:69` line-range typo (Kevlin nit 1)

**Verified**: Read `internal/cli/exit_codes_test.go:38-43`. The four rows (`interrupted`, `interrupted-wrapped`, `context-canceled-bare`, `context-deadline-bare`) are at lines 40-43, not 38-41. Lines 38-39 are the `caddy-down` rows.

**Decision**: Trivial doc fix. Raymond changes `:38-41` to `:40-43` in the `_ai` doc. Sixty-second fix. Inexcusable to backlog.

**Cost**: 60 seconds.

### Item 4 — `service.go:212` past-tense audit log (Kevlin nit 2)

**Verified**: Read `service.go:212-225`. The `logger.Warn("removed orphan container from prior interrupted deploy", ...)` fires BEFORE `Stop` and `Remove` execute. If either fails (or is cancelled), the log shows "removed" followed by an error — misleading during incident review.

**Decision**: Kevlin's Option B — present-tense rewrite. One-word change: `"removed"` → `"removing"`. I prefer this over Kevlin's Option A (move the log line to after success) because:

1. The log at the head of the orphan branch tells the operator "this is where decloud is starting to act on the orphan" — a useful flow signal even if the action subsequently fails.
2. The current pre-condition placement keeps the announcement line and the action it announces visually adjacent. Moving the log down separates them by error-handling code.
3. One-word edit is the smallest possible diff. No tests reference the message string.

**Cost**: 60 seconds. Rob makes the edit; Kent verifies no test asserts on `"removed orphan container"` substring (it doesn't — the test asserts on the returned error, not on the log line).

### Item 5 — `isCancellation(err)` helper hoist (Kevlin style 3 / Linus Issue 4)

**Verified**: 6 occurrences of `errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)` in `service.go` (lines 201, 215, 221, 244, 261, 341). Joel pre-approved the hoist in `03-tech-plan.md` §3.4.1. Rob took the local hoist at line 261 (`cancelled := ...`) but not the function-level helper.

**Decision**: Hoist to package-private helper.

**The Kevlin Henney threshold**: "Six invocations of an idiom is the threshold where the idiom deserves a name." I agree, and it costs nothing to do it now while the code is fresh. The helper is two lines:

```go
func isCancellation(err error) bool {
    return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}
```

Six call sites become `if isCancellation(err) { ... }`. The hoisted local at line 261 (`cancelled := isCancellation(err)`) reads cleaner. Iteration-2 EXECUTION absorbs the diff cleanly because we're already touching `service.go` for Item 1.

**No test changes needed** — the helper is observably equivalent to the idiom it replaces; the existing tests assert on `errors.Is(err, deploy.ErrInterrupted)` and `errors.Is(err, context.Canceled)`, which still hold.

**Cost**: 5 minutes. One file edit.

### Item 6 — slog message-vs-field duplication (Kevlin style 4 / Linus Issue 5)

**Verified**: 4 sites at `service.go:270, 274, 331, 335`. Each uses `logger.Warn("cleanup failed; please remove "+containerName+" manually", "container", containerName, "error", stopErr)`. The container name appears twice — concatenated into the message AND as a structured field. Breaks slog grep-stability. Inconsistent with the rest of `service.go` (`"network ensure failed", "step", "network", "error", err` at line 145) and with `lifecycle.go:26-29` (cited as the exemplar in `03-tech-plan.md` §4.1).

**Decision**: Match the convention.

**Replacement** (Rob, this is the new shape for all four sites):

```go
logger.Warn("cleanup failed; manual removal may be required",
    "container", containerName, "error", stopErr)
```

(Or `"...required"` → `"...required (run docker rm -f decloud-<name>)"` if Rob wants to keep the recovery hint in the message. I lean fixed string + field — operators can grep `container=decloud-foo` to find their row, and the recovery action is documented in `_docs/usage.md` §8.)

**Why fix in scope**: Two reasons. (a) `slog`'s value proposition is grep-stable messages with variable bits in fields. Concatenating the name into the message defeats grep-stability across the entire `cleanup failed; please remove X manually` message corpus, since X varies per service. (b) The pattern is easier to fix while we're already touching the file (Items 1, 4, 5) than as a separate "polish" pass later. Touching the file once is cheaper than touching it twice.

**No test changes needed** — `TestDeploy_DefensiveOrphanCleanupFailureWrapsErrRun` asserts on the returned error (`docker rm -f decloud-foo` substring), not on the slog message. Verified by Kevlin in `011-kevlin-review.md` finding #4.

**Cost**: 5 minutes. Four mechanical edits in one file.

## Total cost estimate

- Joel tech plan v2.2 delta: 30 minutes (six items, mostly mechanical, references existing patterns).
- Linus plan-review of v2.2: 15 minutes (mechanical confirmation; nothing strategic).
- Kent test additions: 30 minutes (one new table-driven test for §3.4.5 cancellation, mirroring Test 9; six existing tests already pass and stay passing).
- Rob implementation: 30 minutes (one helper, six call-site swaps, two cancellation pre-checks, one log-line word swap, four slog-message rewrites).
- Kevlin re-review: 20 minutes (mechanical re-check of the same files).
- Linus re-review: 15 minutes (strategic re-check; nothing new should appear).

**Total**: ~2.5 hours of agent work to land all six items. Well under the "is it worth one more iteration" threshold.

## What goes to backlog

**Nothing from this iteration.** All six items fix in scope.

The two backlog items already documented in `02-plan.md` §12.7 stand:

- Item 7 — Apply cleanup-context pattern to `internal/caddy/manager.go`.
- Item 8 — `restoreOldContainer` failures should surface in the error chain.

Linus's Issue 2 Option B (real second-signal exit-fast behavior in `cmd/decloud/main.go`) becomes **new backlog item 9**, drafted below for Raymond:

**Item 9 — Wire impatient-second-ctrl+c to exit-fast in `cmd/decloud/main.go`**

Where: `cmd/decloud/main.go:14-15` (`signal.NotifyContext` setup).

Why deferred: scoped tight per `_tasks/2026-04-28-deploy-cleanup-on-interrupt/`. The cleanup-context fix from that task makes the 30s cleanup window observable and actionable; an impatient operator may want to bail out faster. Today they have to `kill -9`. Documented in `_docs/usage.md` §8 as the recommended bail path.

Fix shape: replace the `signal.NotifyContext` setup with a manual `signal.Notify` channel that, on the SECOND signal, cancels the context AND calls `os.Exit(130)` directly. New tests cover (a) one-signal flow still does graceful cleanup, (b) two-signal flow exits fast and skips cleanup. Updates `_docs/usage.md` §8 to reflect the new behavior.

Originator: Linus, `12-linus-impl-review.md` Issue 2.

**Raymond, please append item 9 when you update the docs in iteration 2.**

## Iteration plan

**Step 1 — Joel**: write `03-tech-plan.md` v2.2 delta. Six items above. Do NOT rewrite the whole tech plan; append a v2.2 section at the end with one paragraph per item, including the verbatim code/text replacements I sketched in this lockdown. Keep §1-§6 of the v2.1 tech plan unchanged; the implementation already shipped against them and the iteration-2 changes are deltas, not a rewrite.

**Step 2 — Linus**: plan-review the v2.2 delta. APPROVE before iteration unless something surprises you (it should not — five of six items are your or Kevlin's findings restated, and Item 1 is your finding verbatim).

**Step 3 — back to EXECUTION**: alexander.fenster orchestrates Kent → Rob → Raymond → Kevlin/Linus parallel.

**Step 4 — back to PLAN**: Don/Joel/Linus iteration-3 sign-off. If iteration 2 is clean (it should be — every item is mechanical), iteration-3 is a one-paragraph "all green, ship it" lockdown.

**Step 5 — FINALIZATION**: Ward + Andy.

## Acceptance criteria for iteration 2

The iteration-2 EXECUTION pass is done when:

1. `service.go:185-197` checks `isCancellation(err)` before each `ErrRun` wrap; cancellation re-wraps as `ErrInterrupted` (parity with §3.5).
2. New test (or extension of Test 9) covers cancellation at the two §3.4.5 sites; subtests pass.
3. `service.go` defines `func isCancellation(err error) bool`; six existing call sites use it; line 261 reads `cancelled := isCancellation(err)`.
4. `service.go:212` log message is `"removing orphan container from prior interrupted deploy"` (present tense).
5. Four slog.Warn sites (`service.go:270, 274, 331, 335`) use fixed message `"cleanup failed; manual removal may be required"` with `containerName` only as a structured field.
6. `_docs/usage.md:240` second-ctrl+c sentence rewritten per the text in Item 2 above.
7. `_ai/exit-code-sentinel-not-context-err.md:69` line-range corrected from `:38-41` to `:40-43`.
8. `_ai/m1x-backlog.md` gains Item 9 per the text in §"What goes to backlog" above.
9. `go test ./...` passes; `go vet ./...` clean; `gofmt -l internal/ cmd/` empty; `grep -rn '%w: %v' internal/ cmd/` empty.
10. Kevlin re-review: APPROVE with no new findings.
11. Linus re-review: APPROVE with no new findings.

## Why not punt to follow-up

I considered it. Here's why I'm not.

**The asymmetry argument cuts both ways.** Linus called Item 1 "the kind of thing we should NOT punt on — symmetry was the whole point of the v2.1 lockdown." That's true. It's also true that the same standard applies to my own decision-making across iterations. If I lock down v2.1 with "do it now while the code is fresh" and then punt iteration-2's six items because "we already shipped iteration 1," I am the inconsistency the lockdown was supposed to prevent.

**Fresh-code economics.** Five of six items touch `service.go`. Touching `service.go` once for all five is cheaper than touching it once now and again next month. The marginal cost of the helper hoist (Item 5) and the slog convention fix (Item 6) is near-zero given we're already in the file for Items 1 and 4.

**The "5 minutes per fix" floor.** Items 2, 3, and 4 are <5 minutes each. Backlogging them is more work than fixing them — drafting the backlog entries, finding the items again later, re-reading the context, deciding the fix shape. Inexcusable.

**The "one more iteration is fine" threshold.** Joel's tech plan v2 budgeted 12 hours of agent work for the full task; iteration 2 adds ~2.5 hours. We have budget.

## Status

Iteration-2 plan: **locked**. Ready for Joel to write the v2.2 delta.

— Don
