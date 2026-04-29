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

## Refinement: audit-by-read, not just audit-by-grep

For label-rename / version-bump / milestone-resequence sweeps across N files, **a final end-to-end read of each touched file's surrounding context is required.** Grep alone is insufficient: variant phrasings of the same architectural event survive grep but not read.

Live example from `_tasks/2026-04-28-milestone-resequence/`: Joel's tech-plan audit grepped for the source token `"M2 introduces Viper"` and got hits at `caddy-runs-in-container.md:15` and `:58`. Three lines below an enumerated hit, line 52 said `"until M2's config file lands"` — same architectural event (introduction of `/etc/decloud/config.toml` via Viper), different surface words. Raymond caught it during execution because his sweep methodology *does* read end-to-end (`009-raymond-docs.md` §2 "bare M2/M3 token survey"). Without the catch, the file would have contradicted itself across three lines (`:15` says M3, `:52` says M2, `:58` says M3) and a future-Don re-read would have whiplashed.

Joel's one-sentence failure mode in `013-joel-closeout.md` §3: *"variant phrasings of the same event survive grep but not end-to-end read."*

Lesson: when the rename target has *meaning* (a milestone, a version, a feature flag), the source token can be paraphrased anywhere in prose. Grep finds the verbatim form; only a sequential read finds the paraphrase.

The same fix-while-fresh decision rule still applies to the survivor — Raymond's line-52 fix met all four conditions (mechanical, same file, <5 minutes, on-theme) and was a direct parallel to Joel's own pre-existing-bug fix at `install.md:121`. The rule has now load-tested across two distinct surfacing patterns: Joel's pre-existing bug surfaced by being on-theme, and Raymond's same-architectural-event survivor of an enumerated grep-audit.

## Refinement: debris created by the current task is on-theme by definition

A subtle extension of condition #4 (the task's stated theme covers it): when a doc rewrite, rename, or refactor invalidates references in *other* files, those references are debris created BY this task. Fixing them in scope is hygiene, not creep — even if the touched files (`_ai/decisions/`, `_docs/`, etc.) are off the original-deliverable list.

Live example from `_tasks/2026-04-29-readme-and-license/`: Rob's README rewrite shrank the file from 278 to 154 lines. Three `_ai/decisions/` files (`no-magic-zero-modes.md`, `secrets-split.md`, `m1-scope.md`) made present-tense citations to README content the rewrite removed, including a bare `README.md:215` line-number cite that no longer resolves at all. Raymond extended the same in-scope-fix discipline to those files in the same task. Linus's framing in `007-linus-execution-review.md` §4: *"the sweep cleaned up debris created by Rob's commit. ... This is the same logic that justified bundling the `_docs/install.md` §8 fix into Rob's commit. If we cared so much about scope-policing that we couldn't fix the doc debris we created, we'd be doing a worse job."*

The decision rule still applies — the sweep was bounded to citation-tense fixes plus M2-shipped accuracy fixes, all mechanical, all in `_ai/decisions/` files that were already stale before the rewrite or freshly stale because of it. None of the decision rationales were touched.

Lesson: when grading a candidate fix against the four conditions, ask "did *this task's* changes cause this staleness?" If yes, condition #4 (on-theme) is automatically satisfied — your task is responsible for the debris field around itself.

## Originator

Pattern observed across:
- `_tasks/2026-04-28-deploy-cleanup-on-interrupt/007-don-final-lockdown.md` (v2.1, §3.5 lockdown).
- `_tasks/2026-04-28-deploy-cleanup-on-interrupt/013-don-plan-iteration2.md` (v2.2, six-item lockdown — applying the rule to Don's own consistency).
- `_tasks/2026-04-28-deploy-cleanup-on-interrupt/021-don-final-signoff.md` (final verdict — cumulative diff coherent, not patchwork).
- `_tasks/2026-04-28-milestone-resequence/{009-raymond-docs.md, 011-linus-impl-review.md §6, 012-don-closeout.md §3, 013-joel-closeout.md §3}` (audit-by-read refinement).
- `_tasks/2026-04-29-readme-and-license/{006-raymond-docs.md, 007-linus-execution-review.md §4}` (debris-from-current-task refinement).
