# Don's final lockdown — v2.1

## Decision

Linus APPROVED v2 (`06-linus-review-v2.md`) and flagged ONE non-blocking item: §3.5 wraps `context.Canceled` from Inspect/Stop/Remove as `ErrRun` instead of `ErrInterrupted`. Asymmetric with §3.4.3.

**Picked: Option A — fix in scope.**

## Reasoning

The whole point of this task is getting cancellation semantics right. Shipping with a known cancellation-mis-wrap inside the same task that's specifically about cancellation discipline would be cowardice. "Narrow window" is a valid observation; it's not a valid excuse.

Symmetry matters more than the cheap path here. §3.4.1, §3.4.2, §3.4.3 all detect `context.Canceled`/`DeadlineExceeded` and re-wrap as `ErrInterrupted`. §3.5 runs on the same request `ctx`. Same predicate, same wrap. Three sites, six lines, plus one Kent test row. The fix is mechanical and the contract becomes uniform across the entire `Deploy` orchestrator.

The "ship as-is, fix in follow-up" alternative would mean a future-Don revisits the same code path to add the same six lines, after a future-user reports exit 40 when they pressed ctrl+c during orphan cleanup. That's two trips to the same file for one fix. I'd rather do it RIGHT now while the code is fresh.

The maintenance test: would I want to debug "why does ctrl+c sometimes give exit 40 and sometimes exit 130 depending on what stage we're in?" at 2 AM? No. Make it consistent.

## Plan updates

- `02-plan.md` §3.3 — added v2.1 cancellation row to the failure-mode coverage list and a v2.1 revision note pointing here. Revision history bumped to v2.1.
- `03-tech-plan.md` §3.5 — code block now has three `errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)` checks before each `ErrRun` wrap; matrix gains a cancellation row; revision note added.
- `03-tech-plan.md` §5.10.1 — new test case `TestDeploy_DefensiveOrphanInspectCancelledReturnsErrInterrupted` covers the inspect-error site; stop/remove sites are mechanically identical. Kent's call whether to table-drive all three or trust the inspect anchor; recommend table-driven.
- Revision history bumped to v2.1 in both plans.

## Status

Planning is locked. Kent starts.

— Don
