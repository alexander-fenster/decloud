# 006 — Linus re-review of Joel's v2 addendum

## Verdict

**APPROVED FOR EXECUTION — Kent/Rob/Raymond proceed.**

Joel addressed all three conditions. Two of his placement choices are better than what I half-suggested; one (C3) is the right discipline for this codebase given there's no CI gate to lean on. Details below in case anyone re-litigates later.

---

## C1 — placement & prose

I floated two homes ("on §B.1.6 new_string OR in 'Explicit M1 cuts'"). Joel picked Explicit M1 cuts, riding the existing line-18 Viper bullet. **That's the better call** and I should have specified it the first time.

Reasoning Joel got right:
- The roadmap one-liner (§B.1.6) is already long; bolting an Option-C-trap warning onto it dilutes the roadmap signal at the place future-Don skims for ordering.
- The Viper-introduction claim already lives at line 18. C1 is a continuation of that exact thought ("where Viper lands AND why M2 must not pre-empt it"). Same bullet, same eye-jump, no new section noise.
- A future contributor planning M2 reads "Explicit M1 cuts → Viper bullet" before they touch any code. This is exactly where the warning needs to fire.

Prose check on the verbatim new_string at v2 §B: it adds three sentences after the existing Viper sentence. They name the Option-C trap by name, point at Don's plan §"Justification" by path, and tell the reader specifically what NOT to do (`/etc/decloud/config.toml` parsing for default-mount-options). That's the right level of specificity — not vague hand-waving, not over-prescription. Approved.

**One nit, not a blocker:** the cross-reference points at `_tasks/2026-04-28-milestone-resequence/002-don-plan.md` §"Justification". That's correct today, but the canonical roadmap pointer the reader has *already* followed by being on `m1-scope.md` is to the task directory itself, which contains the don-plan. The deeper pointer is fine. Leave it.

## C2 — placement & prose

Joel placed C2 on the canonical roadmap line (the M7-defined line) rather than in Explicit M1 cuts. **Adequate, and arguably better** — anyone planning M7 lands on the canonical roadmap line first because that's the one place every milestone is named in order. Putting "M7 is provisional" anywhere else means future-Don sees the bundle without the disclaimer.

Joel's prose explicitly invokes "do NOT repeat the M3a/M3b mistake" by name. That's exactly the failure mode I cited and exactly the language a future-Don will pattern-match on. Better than my suggested wording. Approved.

**One nit, not a blocker:** the new paragraph sits between the canonical roadmap one-liner and the "Don't reopen this sequencing" sentence. That's structurally a little awkward (it's three paragraphs where there used to be two), but the alternative (bury the warning at the bottom or collapse it into the Linus-approval-pointer paragraph) loses signal. Joel's structure is the right tradeoff.

## C3 — mechanism

Joel's mechanism: Rob commits §B.11.1, §B.11.2, §B.11.3 as a single atomic commit. Between Kent's commit and Rob's, `go test ./...` is RED by design. If Rob is materially delayed (>1 working day), Kent's commit gets reverted from main and re-applied later as part of the TDD pair.

**This is the right discipline for this codebase.** Three reasons:

1. **No CI gate exists.** Joel verified by reading `_docs/install.md` §3 that this repo's test workflow is operator-runs-`go test`-locally. There's no merge-blocking automation that would catch a red main between commits. So "main goes red between Kent and Rob" is a developer-local signal, not a broken-build broadcast. The cost of TDD-red-between-paired-commits is bounded.

2. **The single-commit constraint on Rob's side is the actual safety property.** What I cared about in C3 was "the binary never disagrees with itself mid-task" — i.e., no operator ever sees a `--help` text saying "M2 only" while the runtime rejection still says "until M3". Joel's atomic-commit constraint guarantees that. The Kent-then-Rob ordering is upstream of that property and orthogonal to it.

3. **The >1-day revert escape hatch is correct.** TDD-paired commits where the test commit lands first is a standard pattern; the failure mode is "test commit lands, source fix gets blocked indefinitely, main sits red." Joel's mitigation (revert if Rob is materially delayed) is the right discipline. The alternative ("squash Kent's tests into Rob's commit so main never goes red") loses the test-author/implementer role separation the workflow is built on.

**Cleaner pattern I considered and rejected:** Kent could write the tests against the *new* wording but skip them with `t.Skip("locked by Rob's commit at <task ref>")` until Rob lands. That avoids red-main entirely. I rejected it because (a) it adds a skip-then-unskip round-trip Rob has to remember to do, (b) `t.Skip` calls are exactly the kind of thing that get forgotten and silently disable tests forever, and (c) Joel's revert-if-delayed mitigation is cheaper than introducing a skip-and-unskip dance for a hypothetical >1-day delay that this task is unlikely to hit anyway. Joel's pattern is correct.

Approved.

## Anything Joel let slip

Read v2 end to end. The verbatim substitutions in §B and §C anchor against bytes I confirm exist in `_ai/decisions/m1-scope.md` (Joel re-verified during the v2 pass; I trust his re-verification given his §A.1 grep methodology in v1).

Specific prose I scrutinized:

- **v2 §B new_string:** "do not 'helpfully' add `/etc/decloud/config.toml` parsing in M2 for default-mount-options or similar" — the scare-quotes on "helpfully" carry the right tone for a guard-rail comment future-Don needs to internalize. Good.
- **v2 §C new_string:** "Bundling client binary + secret files + operational polish there is bin-packing convenience, not a commitment to ship them as one milestone — do NOT repeat the M3a/M3b mistake by treating 'everything in M7' as a single deliverable." Names the failure mode, names the prior mistake, gives the reader the framing to act differently. Good.
- **v2 §D Rob constraint:** "as a single atomic commit, never half-landed" plus the cross-surface coherence justification (`_ai/cli-flag-surface-coherence.md`). Cites the existing pattern doc as the rationale rather than inventing a new one. Good.
- **v2 §D red-bar mitigation:** the >1-day revert rule is stated, not just implied. Good.

One thing Joel could have but didn't, and I'm not blocking on it: **Kent's report should explicitly include the line "RED — expected; will go green when Rob lands §B.11"** so anyone reading the task directory between commits doesn't think the build is broken. Joel mentions this in §D paragraph "Mitigation" but it's a directive to Kent, not a constraint Kent can validate his report against. Trust Kent to honor it; if he doesn't, Donald-Knuth gets called and we find out. Not worth blocking on.

## What this approval covers

Everything in v1 §H plus:

- C1 addressed at `_ai/decisions/m1-scope.md` line 18 (Explicit M1 cuts → Viper bullet).
- C2 addressed at `_ai/decisions/m1-scope.md` line 32-34 (Milestone sequence section, adjacent to the canonical roadmap line where M7 is defined).
- C3 addressed in v2 §D — Rob commits §B.11 atomically; Kent-then-Rob is sequential with explicit revert-if-delayed escape hatch.
- Total change count holds at 24 distinct surgical changes (Joel's count math is correct: C1 and C2 are substitutions of v1 edits, not new ones, since the edit unit is the Edit call not the sentence).
- All other v1 approvals from `004-linus-plan-review.md` §G unchanged.

## What happens next

1. Kent writes the three test additions per v1 §C. Tests fail against current source — that's the TDD signal, not a bug.
2. Rob lands `internal/cli/deploy_service.go` (two edits) + `internal/registry/store.go` (one edit) **as a single atomic commit**. `go test ./...` flips green. Reports completion.
3. Raymond executes the doc edits per v1 §D.3 sub-order, applying v2 §B in place of v1 §B.1.5 and v2 §C in place of v1 §B.1.6. All other v1 edits stand.
4. Kevlin + Linus review in parallel. Kevlin handles doc-hallucination check; I handle architectural review of the actual diff.
5. PLAN re-entry per CLAUDE.md.

If anything material slips during execution, Donald-Knuth gets called and we re-plan. Otherwise: ship the resequence.

## Files referenced

- `/Users/fenster/dev/decloud/_tasks/2026-04-28-milestone-resequence/001-user-request.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-milestone-resequence/002-don-plan.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-milestone-resequence/003-joel-tech-plan.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-milestone-resequence/004-linus-plan-review.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-milestone-resequence/005-joel-tech-plan-v2.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-milestone-resequence/006-linus-rereview.md` (this file)
- `/Users/fenster/dev/decloud/_ai/decisions/m1-scope.md` (anchors verified by Joel)
- `/Users/fenster/dev/decloud/_ai/cli-flag-surface-coherence.md` (cited by C3 rationale)
