# Cross-references in `_ai/` and `_docs/`: cite content, not line numbers

When a `_ai/decisions/` doc, a `_docs/*.md` page, or any prose-side reference cites a sibling Markdown file, prefer **content-based phrasing** over **bare `file.md:NNN` line numbers**. Line-number cites silently rot the next time the cited file is rewritten or reflowed; content-based phrasing degrades to a `grep`-able anchor that survives.

## The failure mode

`_tasks/2026-04-29-readme-and-license/006-raymond-docs.md` §4.1: pre-rewrite, `_ai/decisions/no-magic-zero-modes.md:25` carried `(cross-referenced README.md:215 and _ai/decisions/m1-scope.md:32)`. Rob's README rewrite shrank the file from 278 lines to 154 — line 215 ceased to exist. The cite did not throw an error; it just silently pointed past the end of the file. The next reader greppping for "where is the `--port=0` contract documented" would land at a dead anchor, scroll up looking for context that wasn't there, and re-derive what the cite used to say.

The same shape hit three more files in the same sweep:
- `_ai/decisions/secrets-split.md:3` — present-tense cite of "the README's 'Handling secrets' section" that the rewrite removed.
- `_ai/decisions/m1-scope.md:13, 14, 17` — three present-tense paraphrases of pre-rewrite README content.

All four were mechanical to fix once detected. The detection itself was the cost: a manual `grep -rn 'README\.md:[0-9]+' _ai/` plus an end-to-end read of every file with a present-tense `README` mention. Days of staleness can compound silently before someone bothers.

## Discipline

When writing or reviewing a cite from one Markdown file to another:

1. **Prefer content-based phrasing.** "the pre-rewrite README's CLI-surface section" survives any number of edits to that section because a future reader can `grep -i "CLI surface"` and find the moved content. `README.md:215` survives nothing.

2. **If you must cite a line number, cite a stable one.** Anchors that move when neighboring sections grow are unstable; section-heading lines (`## Foo`) and constant declarations (`const Bar = "..."`) are stable. Code-side cites like `internal/registry/types.go:5` are usually fine because Go file structure is stable; prose-side cites like `README.md:215` are usually not.

3. **Tense-mark a citation when it describes a historical state.** "The pre-rewrite README's X section *required* Y" reads as a historical record; "The README's X section *requires* Y" reads as a present-tense factual claim that becomes a lie the moment X is removed.

4. **Forward-pointer to the new home when the old citation rots.** If a contract that *was* documented in `README.md` now lives in `_docs/usage.md` §2, the fixed cite should say so: "the user-facing X contract now lives in `_docs/usage.md` §2." Saves the next reader a grep.

## Detection recipe

When rewriting any frequently-cited Markdown doc (README, `_docs/*.md`, an architecture overview), close the loop:

```sh
# Find bare line-number cites against the rewritten file:
grep -rn -E '<file>\.md:[0-9]+' _ai/ _docs/

# Find present-tense paraphrases of cut content:
grep -rn -E 'the (README|<file>)' _ai/ _docs/
```

The first form catches bare line cites; the second catches paraphrased content claims that survived a `grep -F` against the verbatim source token but no longer match the rewritten doc. Both forms are necessary — Raymond's `006-raymond-docs.md` §4 caught one of each shape on the same task.

## Why `_tasks/` records are exempt

Historical task records under `_tasks/` are immutable by convention; they describe state-of-the-world at a specific moment and rewriting them would falsify the workflow trail. Broken `README.md:215` cites in `_tasks/2026-04-26-fix-deploy-service-review-findings/04-linus-review.md` are correctly left as-is — they were true at the time. Only `_ai/` (which is meant to be live, current reference material) and `_docs/` (which is user-facing) get the cleanup.

This asymmetry is enforced by Raymond's restraint in `006-raymond-docs.md` §4.4 and ratified by Linus in `007-linus-execution-review.md` §4.3.

## Originator

`_tasks/2026-04-29-readme-and-license/006-raymond-docs.md` §4 (the four-file sweep), `007-linus-execution-review.md` §4 (sweep-was-the-right-call ratification), `008-don-final.md` §6.2 L2 (Don explicitly routed the pattern to Ward).
