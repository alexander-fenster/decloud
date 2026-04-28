# 013 — Joel's closeout

## Verdict

**DONE.** Concur with Don (012). Linus's PLAN re-entry can re-run for final approval; Ward + Andy proceed to finalization.

## 1. Did execution match my plan?

**Yes — byte-exact across §B.1–§B.10 (v1) plus v2 §B (supersedes B.1.5) and v2 §C (supersedes B.1.6).** Spot-checked the actual disk state against my prescribed `old_string`/`new_string` pairs:

- v1 §B.1.1–§B.1.4: applied verbatim at `m1-scope.md:8,13,15,16`.
- v2 §B (replaces v1 §B.1.5): applied verbatim at `m1-scope.md:18` — Viper-introduction milestone moved to M3 plus the Option-C-trap warning naming the rejected `/etc/decloud/config.toml` parsing path with the cross-reference to `002-don-plan.md` §"Justification". Resolved.
- v2 §C (replaces v1 §B.1.6): applied verbatim at `m1-scope.md:32–36` — canonical roadmap rewritten, C2 "M7 is provisional / do NOT repeat M3a/M3b mistake" paragraph in place, Linus-approval pointer carries the appended resequence sentence. Resolved.
- §B.2.1, §B.2.2, §B.3.1, §B.4.1, §B.4.2, §B.5.1, §B.5.2, §B.7.1, §B.7.2, §B.8.1, §B.9.1, §B.10.1: all applied byte-exact per Raymond's table in `009` §1, cross-checked via Kevlin's hallucination sweep (`010` §1) and Linus's spot-verification (`011` "What I checked").
- §B.6 (`container-naming.md`): zero edits as prescribed.
- §B.11.1, §B.11.2, §B.11.3 (Rob's three Go strings): applied byte-exact per `008` §"The three edits", verified by Kevlin (`010` §4) and Linus (`011` §1).
- §C.1, §C.2, §C.3 (Kent's three new test assertions): applied with one improvement on Kent's part — the constructor symbol is `newDeployServiceCmd`, not my placeholder `newDeployServiceCommand`. Kent caught and corrected the planning hallucination before commit (`007` §C.3 final paragraph). That's exactly the pre-execution discipline §C.3 itself recommended ("Rob: if `newDeployServiceCommand` takes a different constructor signature in the existing tests, follow that pattern" — Kent did the analogous audit for Kent-side code).

The §D execution order (Kent → Rob → Raymond, Rob single-atomic-commit, TDD red bar bounded) was honored exactly. No surface diverged from any other surface at any point operators would have observed.

**Net: 24 prescribed surgical changes + 1 in-scope Raymond fix-while-fresh = 25 changes shipped.** Every prescribed change applied; the +1 is a same-class survivor I missed in audit (§3 below).

## 2. Do my prior overrides of Don still look right post-execution?

Two overrides to re-check:

### Override #1 — no separate `_ai/decisions/milestone-resequence.md` (v1 §A.4)

**Holds.** The pointer surface that actually shipped (`m1-scope.md:36` appended sentence + `MEMORY.md:57` task-pointer bullet + `m1-scope.md:18` C1 trap warning + `m1-scope.md:34` C2 provisional-bundle warning) is sufficient. Linus re-checked post-execution at `011` §4 and concurred. Future-Don reading `MEMORY.md` lands in the right index section in two clicks. A separate decision file would duplicate the pointer and force a third hop. The C2 warning is the actual forward-looking guardrail and lives where future-Don will be when planning M7 — exactly the right placement.

### Override #2 — terse rendering for `m1-test-strategy.md` footnote (v1 §A.5)

**Holds.** The shipped text at `m1-test-strategy.md:7,49` reads "the next milestone's first feedback signal" / "the next milestone's first priority" — milestone-numbering-agnostic. The substantive claim ("we ship unit-tests-only; the manual smoke-test bridges to real-system reality") is preserved without coupling to today's M2/M3 labels. This makes the file robust to *future* resequences, which is exactly the property I argued for. Don leaned this way too; Linus approved in `006`. Post-execution, no reader stumbles, no claim is weakened.

Both overrides hold without reservation.

## 3. The audit miss — `caddy-runs-in-container.md:52`

**Failure mode in one sentence:** I grepped for the source token "M2 introduces Viper" and got hits at lines 15 and 58, but missed line 52's "until M2's config file lands" because it phrased the same architectural event in different surface words — variant phrasings of the same event survive grep but not end-to-end read.

**Yes, this is a discipline lesson worth Ward capturing.** It's already named in `011` §6 and `012` §3 as the audit-by-read refinement. My role here is to confirm: this was my miss, the lesson is real, and the fix is methodological — when planning a label-rename across N files, the audit pass must include at least one end-to-end read per touched file, not only a grep-by-source-token sweep. Raymond caught it because his execution methodology *does* read end-to-end (his "Sweep 4: bare M2/M3 token survey" in `009` §2 walks every line containing the token). The lesson: planners should adopt the same end-to-end discipline at audit time so the planning pass doesn't push survivors into Raymond's lap.

Append to `_ai/review-discipline/fix-now-while-fresh.md` or a sibling file under `_ai/review-discipline/`. Cite this task as the precedent.

## 4. Verdict

**DONE.** Concur with Don's `012` verdict. All four user requirements (M2/M3 swap, `--mount` next, client to M7, secrets to M7) shipped coherently across nine docs and three Go strings. All five operator-visible surfaces (`--help`, CLI runtime, loader runtime, `usage.md`, roadmap line) say M2 with no contradictions. All three Linus plan-review conditions (C1/C2/C3) discharged on disk. Kevlin's four nits and Linus's three follow-ups are correctly classified non-blocking; the two real learnings (audit-by-read; semantic-token-substring carve-out for `cli-flag-surface-coherence.md:29-31`) belong to Ward's finalization, not to this PLAN re-entry.

Linus's PLAN re-entry can re-run for the final third-vote approval. Ward + Andy proceed.

## Files relevant to this closeout (absolute paths)

- `/Users/fenster/dev/decloud/_tasks/2026-04-28-milestone-resequence/` (full task directory, 001–012)
- `/Users/fenster/dev/decloud/_ai/decisions/m1-scope.md` (canonical roadmap; v2 §B at line 18, v2 §C at lines 32–36)
- `/Users/fenster/dev/decloud/_ai/decisions/caddy-runs-in-container.md` (line 52 — the audit miss Raymond caught)
- `/Users/fenster/dev/decloud/_ai/review-discipline/fix-now-while-fresh.md` (Ward's audit-by-read target per §3)
- `/Users/fenster/dev/decloud/_ai/cli-flag-surface-coherence.md` (Ward's semantic-token carve-out target per Kevlin §6.3 / Linus FU#1)
- `/Users/fenster/dev/decloud/internal/cli/deploy_service.go` (Rob's edits at lines 61, 72)
- `/Users/fenster/dev/decloud/internal/registry/store.go` (Rob's edit at line 69)
