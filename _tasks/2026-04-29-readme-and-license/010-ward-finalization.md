# Ward — FINALIZATION step: `_ai/` learnings extracted

**Author:** Ward Cunningham (knowledge librarian)
**Step:** FINALIZATION #1 — preserve learnings from this task in `_ai/` for future reference.
**Branch:** `task/readme-and-license`
**Status:** four `_ai/` updates committed; ready for Andy's pass and squash-merge.

---

## TL;DR

Three new files created, one existing file extended. All four meet the `_ai/` bar (non-obvious, durable, code-or-task referenced, would save a future contributor a round-trip).

| Action | File | Why it earned its place |
|---|---|---|
| New | `_ai/cross-ref-content-not-line-number.md` | Bare `README.md:NNN` cites silently rotted across four `_ai/decisions/` files. Captures the four-rule discipline plus the detection grep recipe. |
| New | `_ai/readme-mid-build-honesty.md` | Dated Project Status section before Quick start is the load-bearing pattern; the apologetic-tone failure modes ("we're sorry," "still missing") are non-obvious. |
| New | `_ai/review-bar-by-surface.md` | Don overrode two reviewers' "non-blocking" cosmetic flags on a front-page surface; the upgrade rule and the bikeshedding-avoidance bound are both reusable. |
| Extended | `_ai/fix-now-while-fresh.md` | Added a refinement: debris created by the current task is automatically on-theme (condition #4) and so eligible for in-scope cleanup. |

I evaluated every candidate from the FINALIZATION prompt plus my own scan and rejected a few. Details in §3.

---

## 1. What I did

### 1.1 Read the chain

In order: `01-user-request.md` → `02-plan.md` → `03-tech-plan.md` → `04-linus-review.md` → `005-rob-implementation.md` → `006-raymond-docs.md` → `007-kevlin-review.md` → `007-linus-execution-review.md` → `008-don-final.md` → `009-linus-final.md` → `009-joel-final.md`. Read `_ai/fix-now-while-fresh.md`, `_ai/doc-grep-discipline.md`, the `_ai/decisions/` index, and the `_ai/m1x-backlog.md` opener for context.

### 1.2 Cross-checked the existing `_ai/` library

Before creating new files I scanned the existing `_ai/*.md` for overlap with each candidate learning. Specifically grepped for `citation`, `cross-ref`, `stale`, `rewrite`, `README`, `bar`, `cosmet`, `front` — none of the candidate learnings were already covered. The closest existing doc was `_ai/doc-grep-discipline.md`, which covers a related-but-distinct discipline (error-string fabrication in operator-facing docs); the new `cross-ref-content-not-line-number.md` is a different shape (cross-reference rot in non-error prose) and earns its own home.

### 1.3 Made the additions

Four edits committed in one commit:

1. **`_ai/cross-ref-content-not-line-number.md`** (new). The four-rule discipline + the detection `grep -rn -E 'README\.md:[0-9]+' _ai/ _docs/` recipe + the `_tasks/`-is-immutable carve-out.
2. **`_ai/readme-mid-build-honesty.md`** (new). Section ordering (Project Status before Quick start), tone discipline (no apology, dated, uniform tags), the design-doc-vs-README distinction, the "what to cut at this maturity" list (no badges/FAQ/code-of-conduct/comparison-matrix).
3. **`_ai/review-bar-by-surface.md`** (new). Don's override of Kevlin and Linus on the README:13 nit. The three when-to-apply conditions, the does-NOT-apply list, and the single-pass-and-bounded anti-bikeshedding bound.
4. **`_ai/fix-now-while-fresh.md`** (extended). Added a "Refinement: debris created by the current task is on-theme by definition" section between the existing "Refinement: audit-by-read" and "Originator" sections, citing the README rewrite + Raymond's `_ai/decisions/` sweep + Linus's `007-linus-execution-review.md` §4 ratification.

---

## 2. Decisions on the FINALIZATION prompt's candidates

The user pre-listed five candidates. My disposition:

### 2.1 Stale citation pattern → **kept** (`cross-ref-content-not-line-number.md`)

The prompt was right: this is non-obvious and durable. The file documents the four-rule discipline plus a concrete grep recipe.

### 2.2 `fix-now-while-fresh` extension → **kept** (in-place edit)

The existing `fix-now-while-fresh.md` already covered the four-condition decision rule. The new live example sharpens condition #4 ("the task's stated theme covers it") with a stronger framing: when *your* task created the staleness, condition #4 is automatically satisfied. That's a useful tightening without duplicating the rest of the doc.

### 2.3 Project Status pattern → **kept** (`readme-mid-build-honesty.md`)

The pattern is reusable and the failure modes (apologetic tone, undated framing, design-doc-as-README) are concrete. Worth a standalone file because it'll be the doc someone greps when starting the next milestone-adjacent README pass.

### 2.4 Linus's process note about Raymond's report under-disclosure → **deferred to Andy**

Don explicitly routed this in `008-don-final.md` §6.2 L1 with the note "Goes to Ward (knowledge librarian) for `_ai/` capture; possibly Andy for an agent-instruction tightening on Raymond." Linus's framing in `007-linus-execution-review.md` §9 was a "process learning, not an artifact change."

I considered creating `_ai/report-honesty-when-diff-exceeds-summary.md` but **rejected** for two reasons:

1. **It's a one-instance event.** A `_ai/` doc on agent-report honesty after one occurrence is premature pattern-extraction. If Raymond (or any agent) does it again, that's pattern; one occurrence is incident.
2. **The lesson is naturally captured already.** `007-linus-execution-review.md` §9 contains Linus's coaching note in plain prose, and the workflow trail under `_tasks/2026-04-29-readme-and-license/` is itself a queryable record. Andy is the right owner for "should we tighten Raymond's agent description" — it's an agent-instruction concern, not a knowledge-library concern.

Empty extraction is better than padding. If Andy decides this rises to an instruction tightening, that's fine; if Andy decides the high-bar-for-instruction-changes rule keeps it out, that's also fine. Either way, `_ai/` is not the home.

### 2.5 "Anything else you spot"

Two more candidates I considered:

**(a) Don's override of two reviewers' "non-blocking" cosmetic flag → kept (`review-bar-by-surface.md`).** This was genuinely surprising — Kevlin and Linus are senior reviewers; Don explicitly went over their heads on the cosmetic. The pattern is reusable for any future front-page-touch task and the anti-bikeshedding bound is what makes it discipline rather than tic. Worth its own file.

**(b) The `_docs/install.md` §8 "stale-the-moment-LICENSE-lands" pre-flag in Don's plan §2.4 → rejected.** This is a specific application of `fix-now-while-fresh.md` rather than a new pattern, and the existing doc's condition #4 already covers it. Adding a separate file would be redundant. The mechanism (Don pre-flagged the staleness in the PLAN step so Joel/Rob couldn't silently miss it) is a planning-step discipline that the chain itself documents adequately.

---

## 3. What I considered but did NOT add

### 3.1 No file on Joel's verbatim-shell-block discipline

Linus praised this in `007-linus-execution-review.md` §8.1 ("Rob's discipline in pasting verbatim shell blocks"). It's a real reusable lesson — the planning value evaporates if the implementer re-derives the bytes. But it's already implicit in the workflow's plan/execution split, and I couldn't find a non-obvious sharpening that wasn't just "follow the workflow." Skipped.

### 3.2 No file on Rob's grep self-test

Linus also praised `Rob's grep self-test (his §2.7)` — running `grep -nE 'blue/green|restic|backup|client binary|bootstrap|\bjob\b|\bgc\b' README.md` against his own output before declaring done. This is a useful pattern but it's specific to no-feature-leak grep tests, which is already an acceptance criterion in the plan. Adding `_ai/grep-self-test-before-declaring-done.md` would border on padding. The existing `_ai/doc-grep-discipline.md` covers the grep-as-doc-test discipline well enough.

### 3.3 No update to `_ai/m1x-backlog.md`

This task generated no M1.x backlog items. All work landed cleanly in scope.

### 3.4 No `MEMORY.md` update

The user-level `~/.claude/projects/.../memory/MEMORY.md` is for cross-conversation feedback (e.g., "no Docker on this Mac"). Nothing about this task generates a new persistent user-level memory item.

---

## 4. Files committed in this step

```
_ai/cross-ref-content-not-line-number.md     (new, ~50 lines)
_ai/readme-mid-build-honesty.md              (new, ~45 lines)
_ai/review-bar-by-surface.md                 (new, ~55 lines)
_ai/fix-now-while-fresh.md                   (extended +12 lines)
_tasks/2026-04-29-readme-and-license/010-ward-finalization.md  (this file)
```

No code touched. No `_docs/` touched. No production artifacts changed.

---

## 5. Hand-off

- **Andy (HR/agent manager):** Linus's process note on Raymond's report under-disclosure (§4.2 of `007-linus-execution-review.md`, §3 of `008-don-final.md`) is a candidate for an agent-instruction tightening on Raymond. Don explicitly noted the "high bar — agent instructions don't update lightly" caveat. Your call. I deliberately did NOT capture this in `_ai/` because one occurrence is incident, not pattern.
- **Squash-merge step (whoever runs it):** branch is `task/readme-and-license`. Recommended squash-commit message shape: `docs: rewrite README and add MIT LICENSE` with body covering (a) README rewrite + Project Status + Roadmap, (b) MIT LICENSE addition, (c) `_docs/install.md` §8 fix, (d) `_ai/decisions/` cleanup sweep, (e) `_ai/` learnings preservation. Conventional-commit prefix `docs:` because no production code changed.

The library is denser by three real lessons and one sharpening of an existing one. The README rewrite chain itself is the live example backing all four. Future-Ward, future-Don, and future-Raymond all benefit.

— Ward
