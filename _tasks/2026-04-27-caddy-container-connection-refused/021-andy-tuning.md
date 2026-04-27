# 021 — Andy: agent-tuning analysis (FINALIZATION 4.2)

Author: Andy Grove (HR / agent-architecture agent)
Date: 2026-04-26
Status: FINALIZATION step 4.2. **Verdict: NO agent definition changes recommended.**

## TL;DR

The user posted the bug at `001-user-request.md`, the team ran the standard
two-cycle PLAN/EXECUTION/PLAN workflow, and the user has not pushed back at
any point — they have not yet exercised the fix on their host. The
very-high-bar criterion ("user asked for any corrections during this task")
is **not cleared**. Default answer applies: no changes.

I still walked the run looking for *silent* misbehaviour that would justify
a tuning regardless of explicit user complaint. Two candidates surfaced;
both resolve to "no change."

---

## Candidate 1 — Rob silently deferred Joel §1.5 substring detection

### What happened

Joel v2 §1.5 specced a port-conflict substring-detection branch in
`Manager.Up` (match `address already in use` / `port is already
allocated`, emit a Caddy-specific recovery message). Rob's cycle-1
implementation (`009-rob-implementation.md`) did NOT implement that
branch — `Manager.Up` wraps the docker stderr verbatim instead. Rob did
not flag the deferral in his report's "Pushback on Kent's work" or
"Implementation choices worth flagging" sections.

The gap was caught in cycle 1 by Kevlin (`011-kevlin-review.md` Nit 1)
and elevated by Don's final review (`012-don-final-review.md` §3) to a
must-fix for cycle 2. Rob then implemented the branch correctly in cycle
2 (`016-rob-implementation-cycle2.md`).

### Should rob.md change?

**No.** Reasons:

1. **The system caught it.** Kevlin's hallucination-check found the
   downstream symptom (Raymond's doc fab — see Candidate 2). Don's
   final-review caught the upstream cause (the spec-vs-impl gap). The
   PLAN-after-EXECUTION loop did exactly what CLAUDE.md requires it to
   do.
2. **Joel's spec marked the branch as "best-effort."** §1.5 row 1 ends
   with "Do NOT pre-flight `lsof` — adds a host dependency for one error
   case." The branch was discretionary-feeling at the spec layer. Rob
   skipping it without a flag is a judgement-call miss, not a process
   failure.
3. **rob.md already has the correct guidance.** Lines 116-121 ("Feedback
   for Test Engineers") cover the upstream-flag case for tests; the file
   does not have a parallel "flag spec items you skipped to your report"
   bullet, but adding one would be a band-aid for one missed item across
   two cycles. No pattern. The PLAN loop is the right place to catch
   this — that's why the loop exists.
4. **The fix is on the LOOP discipline, not on the agent.** If we
   wanted to harden against this, the right place is to ensure Don's
   final review explicitly diff-checks every Joel-spec'd item against
   the implementation. That happens today (`012-don-final-review.md`
   §2 has the criterion table) and it worked. Making Rob individually
   responsible for self-flagging deferrals adds noise to his reports
   for very small benefit.

### Verdict

No change to `rob.md`.

---

## Candidate 2 — Raymond fabricated example error renderings

### What happened

Raymond's cycle-1 docs (`010-raymond-docs.md`) wrote two operator-facing
error-text examples in `_docs/install.md` §7 troubleshooting that the
implementation does NOT emit:

- Line 173: `caddy up: ports 80/443 already in use` — this string was
  Joel's §1.5 *spec*, not what the code at the time produced. The code
  emitted the wrapped `caddy: up failed: docker run: ...; stderr=...`
  chain because Rob had skipped the substring branch (Candidate 1).
- Line 189: `caddy up: docker run: listen tcp [::]:80: ...` — same
  class of mismatch (missing `caddy: up failed:` prefix; not what the
  wrap chain produces).

Kevlin caught both as doc-hallucination nits (`011-kevlin-review.md` §
"Two minor doc accuracy nits"). Don promoted them to must-fix
(`012-don-final-review.md` §3). Cycle 2 fixed both: the ports case got
the implementation matched to the doc (Candidate 1's fix); the IPv6 case
got the doc reworded to match the actual wrap chain
(`017-raymond-docs-cycle2.md` Edits 1 and 2).

### Should raymond.md change?

**No.** Reasons:

1. **raymond.md already has loud, prominent guidance against this exact
   class of error.** Lines 32-46 carry a `🚨 CRITICAL: NEVER
   HALLUCINATE FIELD NAMES! 🚨` block with a verification checklist
   ("ALWAYS check actual struct definitions and JSON tags before writing
   examples"). The guidance generalises straightforwardly from "field
   names" to "any operator-facing text claimed in docs" — it's the same
   instruction.
2. **The miss looks like a special case of plan-trust, not a
   guidance gap.** Raymond was reading Joel's plan as the source of
   truth for the operator-facing text. Joel's plan had specced the
   message; the implementation skipped the spec; Raymond
   doc-as-spec'd. The right fix is *cross-referencing the actual
   emitted text against the actual code* — which is precisely what
   raymond.md §3 already says.
3. **Kevlin's review is the last-line defence and worked.** CLAUDE.md
   process step 3.4 mandates Kevlin doing the hallucination check.
   It caught both. The two-tier guard is doing its job.
4. **Raymond is the Bubblehouse Next.js docs agent.** The file's
   examples (lines 4-5, 8, 91, 105, 141-146) are heavily flavoured for
   the Bubblehouse loyalty platform / msgpack / fireerr / `_docs/src`
   Next.js setup. Decloud uses plain markdown in `_docs/*.md` and the
   guidance has been working anyway. A cross-project hardening would
   be a larger project-instructions edit, not a tuning of this run's
   feedback. Out of scope.

### Verdict

No change to `raymond.md`.

---

## Other candidates considered, all dismissed

- **Kent (test author):** Tests landed correctly in both cycles. No
  change-detector tests, helpers reused with intent-naming, contract
  assertions over implementation assertions. No misbehaviour.
- **Don (planner):** Final review at `012` is the model citizen of this
  run — it catches both candidate-issues, makes a clear cycle-2 call,
  enumerates 5 surgical items. No tuning needed.
- **Joel (tech planner):** v2 plan is high-fidelity; cycle-2 plan
  responsively scoped. The §11.3 readiness-loop landmine and §11.7
  image-pin trap were both flagged-and-resolved cleanly. Open question
  count is reasonable (8 in v1, 1 in cycle-2). No tuning needed.
- **Linus (reviewer):** Two clean reviews
  (`011-linus-impl-review.md`, `018-linus-impl-review-cycle2.md`).
  Approval thresholds appropriate.
- **Kevlin (low-level reviewer):** Caught the doc fabs. Job done.
- **Ward (knowledge librarian):** Not invoked yet (FINALIZATION step
  4.1 follows me).

---

## Knowledge-base recommendations

I am NOT recommending agent definition diffs. I AM flagging one
operational pattern Ward may want to capture for future runs:

> RECOMMENDATION for knowledge-librarian: in `_ai/MEMORY.md` or a new
> `_ai/process-loop-effectiveness.md`, add a one-line entry noting that
> the cycle-1→Don's-final-review→cycle-2 loop on this task caught BOTH
> a Rob spec-deferral and a Raymond doc-fab in cycle 1, and resolved
> both surgically in cycle 2 with one round of corrections. This is
> evidence that the standard PLAN-after-EXECUTION loop in CLAUDE.md is
> doing what it was designed to do, and that the bar for tightening
> individual agent definitions to catch these classes of issue is
> correspondingly higher than it might naively seem.

This is a Ward call, not an Andy call. I include it because it gave me
the empirical basis to refuse both candidate-tunings above with
confidence.

---

## Final verdict

**No agent definition changes.** The very-high-bar criterion is not
cleared (no user pushback) and no silent-misbehaviour case rises to
"definition update." The two issues that did surface in cycle 1 were
caught and fixed by the existing process. That is the correct outcome.

Filed for future-Andy: when revisiting this question for a similar
run, look first at whether the PLAN-after-EXECUTION loop caught the
issue. If it did, the answer is almost always "no tuning."

— Andy
