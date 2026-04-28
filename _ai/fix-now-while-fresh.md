# Fix-now-while-fresh: when a related defect surfaces during a task, fix it in scope

When an unrelated-but-related defect is uncovered during review of a task, Don's repeated rule is: **fix it now, while the code, the context, and the reviewers are fresh.** Don't punt to a backlog item if (a) the fix is mechanical, (b) it touches a file you're already in, and (c) backlogging it costs more bookkeeping than the fix itself.

## The rationale, in Don's own words

From `_tasks/2026-04-28-deploy-cleanup-on-interrupt/007-don-final-lockdown.md` (v2.1 lockdown, on §3.5 cancellation discrimination):

> "The whole point of this task is getting cancellation semantics right. Shipping with a known cancellation-mis-wrap inside the same task that's specifically about cancellation discipline would be cowardice. 'Narrow window' is a valid observation; it's not a valid excuse."

From `013-don-plan-iteration2.md` (iter2 lockdown, applying the same rule to himself):

> "If I lock down v2.1 with 'do it now while the code is fresh' and then punt iteration-2's six items because 'we already shipped iteration 1,' I am the inconsistency the lockdown was supposed to prevent."

> "**The '5 minutes per fix' floor.** Items 2, 3, and 4 are <5 minutes each. Backlogging them is more work than fixing them — drafting the backlog entries, finding the items again later, re-reading the context, deciding the fix shape. Inexcusable."

## The decision rule

For a defect surfaced mid-task, fix in scope when ALL of:

1. **Mechanical fix** — the shape is already established by sibling code (e.g. "mirror §3.5 at §3.4.5"). No new architectural decisions needed.
2. **Same file or adjacent files** — touching `service.go` once for five items beats touching it once now and four times later.
3. **5-minute floor** — the per-item fix cost is below the per-item bookkeeping cost of a backlog entry (find again, re-read context, decide shape).
4. **The task's stated theme covers it** — the deploy-cleanup task was about cancellation discipline; an asymmetric cancellation contract found mid-task IS the task.

Defer when ANY of:

1. The fix requires new architectural decisions (e.g. real second-signal exit-fast behaviour in `cmd/decloud/main.go` — Linus Issue 2 Option B; deferred as backlog item 9).
2. The fix touches a different package with its own test surface and review reviewers.
3. The defect was already known before this task and explicitly out-of-scope on the original plan.

## Cost-amortisation argument

The deploy-cleanup task ran two execution iterations. iter1 (v2.1) shipped the headline fix plus §3.5 cancellation symmetry. iter2 (v2.2) added six more items totalling ~2.5 hours of agent work. Don's calculation in `013-don-plan-iteration2.md`: Joel's v2 budget was 12 hours; iter2 is +2.5; we have budget. Two extra iterations of EXECUTION + PLAN are cheap relative to a future-pager-incident. The bound is honesty about budget, not aversion to one more iteration.

## When NOT to apply

The "fix-now-while-fresh" rule has a discipline shadow: scope creep. Anchor against this with:

- An **explicit deviations list** in each implementation report (Rob's `009-rob-impl.md` §"Deviations" cites two cosmetic notes and zero substantive changes — that's the standard).
- Any architectural change goes back through the PLAN step (Don/Joel/Linus) before EXECUTION continues.
- Backlog items still exist for items where the rule explicitly fails the decision rule above. Backlog item 9 (real second-signal exit) was correctly punted out of v2.2 even when Issue 2's doc fix was taken in scope.

## Originator

Pattern observed across:
- `_tasks/2026-04-28-deploy-cleanup-on-interrupt/007-don-final-lockdown.md` (v2.1, §3.5 lockdown).
- `_tasks/2026-04-28-deploy-cleanup-on-interrupt/013-don-plan-iteration2.md` (v2.2, six-item lockdown — applying the rule to Don's own consistency).
- `_tasks/2026-04-28-deploy-cleanup-on-interrupt/021-don-final-signoff.md` (final verdict — cumulative diff coherent, not patchwork).
