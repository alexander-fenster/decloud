# Phantom-scope kill: trace handed-down prose to runtime referents BEFORE expanding scope

When a milestone or task description includes a phrase like "X hardening" or "Y improvements" without naming a specific code path, test, or backlog item, **trace it to runtime/test/file referents before letting the planner expand the scope.** If no referent exists, kill the phrase explicitly in writing and strip it from the carrying doc.

This is the inverse of fix-now-while-fresh: that rule says fix real defects in scope; this rule says don't invent fake defects from vague prose.

## How it bit M2 (and was killed cleanly)

`_ai/decisions/m1-scope.md:32` (pre-M2) read: "M2 server-side mounts + env-file hardening (`--mount` flag, loader populates `Mounts`)". The parenthetical names ONLY mounts work. "Env-file hardening" was loose prose surviving from the M3a-bundle resequence — when M3a still bundled mounts + secret-files + env-capture work into one milestone, "env-file hardening" had a referent (the unimplemented secret-files-on-disk substructure under `secrets/<name>/files/`). After the resequence pulled secret-files out to M7, the phrase remained as boilerplate carrying no concrete work.

Don's trace at `_tasks/2026-04-28-m2-server-side-mounts/002-don-plan.md §1a`:

- **Trace A — read `internal/envcap/capture.go`.** Mechanism is portable bash 3.2+, hermetic. No known bug. Tests pass on macOS bash 3.2 and Linux bash 5+.
- **Trace B — read `_ai/m1x-backlog.md` items 1-8.** Item 4 is the only env-adjacent backlog entry: a comment-only clarification on `Capture("")`. Three lines. Not "hardening" by any normal use of the word.
- **Trace C — read `_tasks/2026-04-28-milestone-resequence/`.** Don/Joel/Linus signed off on the M2 scope as `--mount` + the prose phrase "env-file hardening" without ever defining what the latter contained.
- **Trace D — `grep -rn "TODO\|FIXME\|XXX\|harden" internal/envcap/`.** Returns nothing actionable.

**Verdict: phantom.** Don killed it explicitly: "I am calling it dead, in writing, here. If a real env-capture issue surfaces during M2 implementation (Kent's tests, Rob's bench, Linus's review), it gets logged as a new m1x-backlog item or its own future task — NOT folded into M2 retroactively."

Linus ratified at `004-linus-plan-review.md` Issue 9 ("Phantom kill (env-file hardening) is justified... There is no concrete env-capture work hiding under the 'hardening' label.").

## The rule

For any milestone/task description that contains a phrase without a named referent:

1. **Grep the codebase** for TODO/FIXME/XXX comments on the named subsystem.
2. **Read the implementation file end-to-end** — does it have unfinished branches, defensive `panic("unimplemented")` calls, or known-broken paths?
3. **Read the existing backlog** for adjacent items that the phrase might be mislabelling.
4. **Read the upstream task that introduced the phrase** — was a referent named there that has since been split off to a different milestone?
5. **If all four traces come up empty: KILL the phrase in writing.** Don't expand scope to "do whatever the phrase might mean." Strip it from the carrying doc as part of the same task's docs sweep (fix-while-fresh on stale prose).

The kill must be explicit — in the plan file, name the phrase, name the traces, name the verdict. Future-author reading the plan after the milestone ships needs to see "this phrase was traced and killed" so they don't resurrect it from the original carrying doc.

## When NOT to kill

If trace 1-4 surface a real referent, the phrase is legitimate scope and the task should expand to cover it (or split it to its own task). The kill only applies when **all four traces come up empty** AND the phrase post-dates a milestone resequence/rename that plausibly orphaned it.

A phrase with even one weak referent (e.g., a single TODO comment) is not a phantom — it's a real backlog item. Promote it to `_ai/m1x-backlog.md` with a fix shape. Don't fold it silently into the current milestone.

## Why "fix-while-fresh on stale prose" applies

After killing the phantom, the doc that carried it (the m1-scope.md line, the MEMORY.md summary, etc.) needs to lose the phrase. That's a same-file diff during the docs sweep — exactly the shape `_ai/fix-now-while-fresh.md` covers. Don's plan §1a explicitly chained: "Action for Raymond at the doc-update step: strip 'env-file hardening' from `_ai/decisions/m1-scope.md:32` and the corresponding `MEMORY.md:7` line as part of the 'M2 has shipped, update tense' sweep."

## Originator

Don §1a of `_tasks/2026-04-28-m2-server-side-mounts/002-don-plan.md`; Linus ratified at `004-linus-plan-review.md` Issue 9.
