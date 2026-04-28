# Cancellation symmetry: audit the WHOLE code path, not just the reported site

When fixing a `context.Canceled` mis-wrap at one call site, scan EVERY sibling site that shares the same request ctx for the same anti-pattern. The bug class generalises across adjacent forward-progress branches; the user's report names only the site they happened to hit.

## How this bit us

v2.1 fixed cancellation discrimination at the §3.5 `!hasPrev` orphan-check branch (Inspect, Stop, Remove → `isCancellation(err) → ErrInterrupted`, not `ErrRun`). The sibling §3.4.5 `hasPrev` redeploy stop+remove branch at `internal/deploy/service.go:185-197` had the IDENTICAL bug shape — same request ctx, same `ErrRun` wrap, no cancellation pre-check. Result: ctrl+c during a fresh deploy returned exit 130, ctrl+c during a redeploy returned exit 40. Same key combo, two exit codes.

Linus missed it during v2 plan-review and v2.1 lockdown. He caught it only on iter1 implementation re-review with fresh eyes (`12-linus-impl-review.md` Issue 1, with explicit mea culpa). v2.2 closed the gap with two new `if isCancellation(err) { return %w: %w ErrInterrupted, err }` pre-checks at `service.go:199-201` and `:207-209`.

## The audit recipe

When a `context.Canceled` mis-wrap is reported at site X:

1. `grep -n 'errors.Is(err, context.Canceled)' <package>/*.go` — count current sites.
2. For each branch that takes the request ctx and wraps an error as a step-specific sentinel (`ErrRun`, `ErrReadiness`, etc.), check whether it has the cancellation pre-check.
3. Asymmetric contracts between adjacent sibling branches (`if hasPrev` vs `else`, save-path vs run-path) are the smell — same trigger, different exit code, no semantic justification.
4. Fix all sibling sites in the same task. The marginal cost of N pre-checks is near-zero; the cost of "future-me debugs why exit 40 sometimes, 130 other times" is a 2 AM pager incident.

## When to apply

Any orchestrator that converts cancellation into a sentinel sees this risk:

- Multiple call sites take the request ctx (forward-progress branches).
- Each site wraps errors with a step-specific sentinel.
- Cancellation must be discriminated BEFORE the step-specific wrap.

The `isCancellation(err)` helper at `service.go:48-50` (six → eight call sites) makes the audit grep-trivial. If a future site wraps `ctx`-derived errors without calling `isCancellation`, that's the new asymmetry.

## Locked in by

- `TestDeploy_DefensiveOrphanInspectCancelledReturnsErrInterrupted` (three subtests, §3.5 sites).
- `TestDeploy_RedeployStopRemovePreviousContainerCancelledReturnsErrInterrupted` (two subtests, §3.4.5 sites).
- Both use the same matcher shape; a new site without a matching test will stand out.

## Originator

`_tasks/2026-04-28-deploy-cleanup-on-interrupt/12-linus-impl-review.md` Issue 1; resolved in `013-don-plan-iteration2.md` and `03-tech-plan.md` §13.1.
