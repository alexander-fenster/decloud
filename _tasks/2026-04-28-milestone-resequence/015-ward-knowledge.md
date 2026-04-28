# 015 — Ward's knowledge capture

## Verdict

**DONE.** Two doctrine refinements landed where the closeout votes asked. No new files created — both edits extend existing pages so future readers find them through the entry points they already know.

## Files touched

### `/Users/fenster/dev/decloud/_ai/cli-flag-surface-coherence.md`

Renamed the existing §"Why we don't test surface 3" to "Why we don't test surface 3 (default)" and appended a new section §"Carve-out: semantic-token contract assertions". The carve-out:

- Distinguishes **change-detector** (assertion on prose phrasing — banned) from **semantic-token contract** (assertion on a token whose value participates in a multi-surface contract — allowed).
- Concrete examples on both sides: `assert.Contains(t, help, "container listen port (required)")` is banned; `assert.Contains(t, help, "M2")` is allowed.
- Cites `TestDeployService_MountFlagHelpReferencesM2` as the live example.
- Originator pointer: `010-kevlin-review.md §6.3`, `011-linus-impl-review.md §5.3`, `012-don-closeout.md §3`.

The doctrine line at the old §29-31 (CLAUDE.md change-detector ban) is preserved verbatim — the carve-out is additive. A future reader hitting `cli-flag-surface-coherence.md` reads the default-no-test rule first, then the carve-out, and lands on the right answer for either case.

### `/Users/fenster/dev/decloud/_ai/fix-now-while-fresh.md`

Inserted a new §"Refinement: audit-by-read, not just audit-by-grep" before the §"Originator" section, and extended Originator's bullet list to include the milestone-resequence task. The refinement:

- States the rule: for label-rename / version-bump / milestone-resequence sweeps, a final end-to-end read of each touched file's surrounding context is required; grep alone is insufficient.
- Walks the live example: Joel grepped for `"M2 introduces Viper"` (hit lines 15 and 58 of `caddy-runs-in-container.md`); line 52 said `"until M2's config file lands"` — same event, different surface words. Raymond caught it on end-to-end read.
- Quotes Joel's one-sentence failure mode verbatim from `013` §3.
- Generalises the lesson: when the rename target has *meaning* (a milestone, a version, a feature flag), the source token can be paraphrased anywhere in prose. Grep finds verbatim; only a sequential read finds paraphrase.
- Notes the existing decision rule still gates the survivor's fix-while-fresh treatment (Raymond's line-52 met all four conditions). The rule is now load-tested across two distinct surfacing patterns: Joel's pre-existing-bug-surfaced-by-being-on-theme (`install.md:121`) and Raymond's same-architectural-event survivor (`caddy-runs-in-container.md:52`).

## Considered and rejected

Three candidate entries didn't clear the librarian bar:

1. **Standalone memory line for the milestone resequence itself.** `MEMORY.md:57` already has the task-pointer bullet under "Source-of-truth task artefacts" (Raymond's edit B.7.2). Line 9's "M1/M2/M7 all write version 1" reflects the resequence in the schema-versioning summary. Line 7's "full M1→M7 milestone sequence" is range-based and unaffected by ordering. The index is coherent post-resequence; adding a third pointer would duplicate without clarifying. Joel argued no separate decision file in `005` §A.4; that call held in closeout (`011` §4, `013` §2.1) and the same logic applies to MEMORY.md.

2. **"User constraint drives ordering" as a pattern entry.** The user said "I need `--mount` next" and that single constraint forced the M2/M3 swap. But this is project-management obvious — CLAUDE.md already establishes the user-driven workflow, and any future re-prioritisation will read the milestone-resequence task directly via the MEMORY.md pointer. Documenting "listen to users" as doctrine is memory bloat.

3. **`m1-scope.md:8` minor rhetorical bump (Linus FU#3).** Linus correctly classified this as not worth a re-edit on its own — the M2/M3 cut/cut/cut/cut pattern reads slightly less crisply post-resequence but the substance is preserved. Capturing it as a doctrine line would teach future agents nothing they wouldn't already learn from doing the same kind of label-flip carefully. Rejected.

4. **Decision-rule contingency-clause-fired-correctly observation.** Joel's v2 §B and §C did supersede v1 §B.1.5 and §B.1.6 cleanly when the user-driven re-plan happened mid-task, and the docs-only-task-became-code-touching contingency Joel called out also fired correctly (`009-raymond-docs.md` ended up paired with `008-rob-impl.md` as planned). But "plans should have superseding mechanisms" isn't a tactical lesson — it's a workflow attribute that's already implicit in CLAUDE.md's PLAN→EXECUTION→PLAN re-entry loop. Memory bloat.

## Net change

Two pages touched, both extended (not rewritten). No new files. Existing entry points (`MEMORY.md`'s "Implementation patterns" line for `cli-flag-surface-coherence.md`, "Review discipline" line for `fix-now-while-fresh.md`) keep working — the new sections are reachable from where readers already look.

The two real lessons from this task — the semantic-token carve-out and the audit-by-read refinement — are now captured at the canonical pages, with originator pointers back to the task directory for full rationale.

## Files relevant to this report (absolute paths)

- `/Users/fenster/dev/decloud/_ai/cli-flag-surface-coherence.md` (carve-out added)
- `/Users/fenster/dev/decloud/_ai/fix-now-while-fresh.md` (audit-by-read refinement added)
- `/Users/fenster/dev/decloud/_ai/MEMORY.md` (verified coherent; no edit needed)
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-milestone-resequence/011-linus-impl-review.md` §5.3, §6, §7
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-milestone-resequence/012-don-closeout.md` §3, §5
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-milestone-resequence/013-joel-closeout.md` §3
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-milestone-resequence/014-linus-final-vote.md`
