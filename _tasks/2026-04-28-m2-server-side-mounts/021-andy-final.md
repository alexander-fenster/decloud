# 021 — Andy's review of agent definitions (M2 closeout)

FINALIZATION step 4.2. Reviewed task files 001–020 plus
`integration-test-run-log-handoff.md`, the full branch commit log
(`94a2c0d` → `f045df2`, 21 commits), and the live agent definitions
under `~/.claude/agents/`.

CLAUDE.md sets a "VERY HIGH BAR" for agent-definition updates. Default
is **LEAVE**. An update is justified only when (a) there is a genuine
behavioural gap, (b) the gap will recur on future tasks (not a one-off
environmental constraint), and (c) Ward's library entries and the
instance feedback memories cannot already cover it.

Decision: **LEAVE ALL AGENT DEFINITIONS UNCHANGED.** Reasoning per
agent below.

---

## 1. User corrections during this task

The user's literal request was `/do implement M2 features` — terse,
no behavioural pushback during the task itself. There was ONE
mid-task instruction surfaced via the host environment / earlier
feedback memory:

> "There is no docker on this laptop and there won't be. Don't try
> running any docker commands here."

This is an **environmental fact about THIS specific dev box**, not a
behavioural pattern about an agent. It is already captured as an
instance-specific feedback memory for this assistant. None of Don,
Joel, Kent, Rob, Raymond, Kevlin, Linus, Ward, or Andy needs to know
"the maintainer's Mac has no Docker" baked into their definition —
the next dev box may have Docker, the next contributor's box almost
certainly will.

The general discipline that *was* learned (compile-clean ≠ run-clean,
plus opt-in tests don't gate merge) is not the no-Docker fact — it's
the orthogonal "claim a PASS only after a real run" rule that Ward
already saved as `_ai/compile-clean-not-run-clean.md`.

**Conclusion**: no agent-definition update warranted by the
no-Docker correction.

## 2. Per-agent assessment

For each agent: did they perform within their definition? If not,
was the gap behavioural-recurrent (definition update) or
context-specific (library/memory already covers it)?

### Don — LEAVE

Created plan, killed phantom env-file-hardening scope (002 §1a),
made the integration-test-fix call at impl-review (012 §1), tied
the run-log gate vote on the closeout reversal (018 §1). All within
his definition (tech-lead role: scope discipline, plan-stage
decisions, closeout votes). The phantom-scope-kill discipline he
exhibited is now a Ward library entry (`_ai/phantom-scope-kill.md`)
— future-Don picks it up via the library, not his definition.

### Joel — LEAVE

Locked decisions with rationale (003), pushed back on Don's leans
on Decisions 1 and 4 (decision-2 dual-sentinel, decision-4
mock-regen β over α — both held up under impl review per
011 §1, 011 §4), wrote addendum-v1 (005) and addendum-v2 (013).
Decision quality high. The β-decision pattern (small blast
radius preferred when both options are correct) is implicit in
existing `m1-test-strategy.md`; Ward considered promoting it (020
candidate d) and correctly declined as doctrine inflation on a
single data point.

### Linus — LEAVE

Caught the integration-test alpine-no-Cmd bug at impl-review by
code-read (011 §5) — a meaningful save that would have shipped
otherwise. Reversed his own run-log gate position (017 §2) when
the cost/benefit changed. Both moves are within his
high-level-reviewer charter. The reversal demonstrates the
"don't double down on yesterday's gate when today's evidence
changes" pattern, but that's not a behavioural rule Linus needs
in his definition — it's part of how good reviewers operate, and
the M2 evidence is now in Ward's `compile-clean-not-run-clean.md`
for future-Linus to read in context.

### Kent — LEAVE (the closest call)

The genuine gap: Kent shipped the RED suite cleanly (007), wrote
the integration test to spec, and reported `go build -tags
integration ./...` clean. He **did not** flag explicitly in his
report that compile-pass is not run-pass and that the integration
test had not been executed end-to-end. Linus and Don both noted
this ("nobody actually ran ... before signing off" — 011 §5,
012 §1).

**Why this stays as LEAVE despite the obvious "instill it in Kent"
temptation:**

1. **Ward already addressed it.** `_ai/compile-clean-not-run-clean.md`
   is a first-class library entry tied directly to this exact arc.
   It is loaded by every agent that reads `_ai/MEMORY.md` (which
   Kent's definition mandates: step 3 "Check `_ai/*.md` for
   additional project-specific guidelines"). The discipline is
   instilled at the library layer, where it benefits every agent
   who writes or runs an integration test (not just Kent — Rob
   reported the same compile-clean signal, and even Linus
   *attempted* to run it locally and was blocked by the lack of
   Docker).

2. **Kent's definition isn't Kent-specific on this axis.** "When
   you write a build-tagged test, explicitly state your
   verification gate" is not a TDD-specific rule; it applies
   equally to Rob ("when you ship a build-tagged test, run it or
   say you didn't"), to Linus ("when you review a build-tagged
   test, demand the run-log or accept the carve-out"), and to
   Don/Joel at closeout. Putting it only in Kent's definition
   would mis-shelve a cross-cutting rule.

3. **The pattern needs the carve-out from rule 2** (opt-in tests
   don't gate merge). A definition update saying "always require a
   run-log for build-tagged tests" would over-correct and miss the
   Don/Linus reversal at v2 closeout. The library entry captures
   both rules together; the definition update would capture only
   the first half and would have produced exactly the gate-stall
   we just escaped.

4. **The cost of leaving is bounded.** Future-Kent reads the
   library when his task touches an integration test (per his
   definition step 3); the entry is two pages and immediately
   relevant. The cost of updating is permanent verbiage in a
   already-long definition for a rule that's properly
   cross-cutting.

**Conclusion on Kent**: LEAVE. The compile-clean-not-run-clean
discipline is correctly housed at the library layer, where it
applies to every agent that touches integration tests, with the
opt-in-doesn't-gate-merge carve-out included.

### Rob — LEAVE

Shipped GREEN cleanly (008), atomic five-surface flip in one
commit (verified by Linus 011 §2), applied v2 fixes mechanically
(015), surfaced no decision drift. Within his definition. Same
compile-clean-not-run-clean lesson applies (Rob also reported only
compile-clean), and the same library-layer fix covers it.

### Raymond — LEAVE

Bare-token sweep, hallucination-checked by Kevlin and Linus
(010 §A, 011 §7), edited eight files. Within his definition. The
historical-narration reframe of `cli-flag-surface-coherence.md:42`
(rather than retrofitting a new live example) was the correct
call and aligns with his "narrate-as-historical, don't
rewrite-to-new-prose" disposition that Linus has reinforced
across multiple tasks.

### Kevlin — LEAVE

Low-level review with three optional fixes (010), all bundled
into v2 (015). Caught the prose-staleness in usage.md:3 ("M1 CLI"
framing) which is exactly the bare-token discipline Raymond's
sweep should have caught — but Kevlin caught it on review, which
is what his role is for. Within his definition.

### Ward — LEAVE

Extracted four learnings from eight candidates (020), rejected
four with reasons, modified `_ai/MEMORY.md` to index. The
selectivity ratio (4 kept / 8 considered) demonstrates the "not
every observation is library-worthy" discipline his definition
already enforces. Within his definition.

## 3. Decision matrix

| Agent | Decision | Reason |
|---|---|---|
| Don | LEAVE | Within definition; phantom-scope-kill captured by Ward |
| Joel | LEAVE | Within definition; β-pattern correctly not promoted |
| Linus | LEAVE | Within definition; reversal pattern is reviewer maturity |
| Kent | LEAVE | Compile-clean discipline lives at library layer (cross-cutting) |
| Rob | LEAVE | Within definition; same library coverage as Kent |
| Raymond | LEAVE | Within definition; doctrine-preserving instinct intact |
| Kevlin | LEAVE | Within definition; caught what Raymond's sweep missed |
| Ward | LEAVE | Within definition; selectivity ratio appropriate |
| Andy (self) | LEAVE | Within definition; high-bar discipline applied here |

## 4. The Kent compile-clean question (explicit answer)

**Question**: Ward saved `_ai/compile-clean-not-run-clean.md` as a
library entry. Library entries are loaded by every agent reading
`_ai/MEMORY.md`. Does that obviate the need for a Kent-definition
update?

**Answer**: Yes, fully. Three reasons:

1. **Cross-cutting rule, not Kent-specific.** Rob, Linus, Don,
   and Joel all need this rule in different shapes (writer,
   reviewer, decider, planner). Library entry serves them all.
2. **The two-rule structure (rule 1 + carve-out) doesn't fit a
   definition.** A definition update would naturally capture only
   rule 1 ("always require a run-log") and miss rule 2 ("opt-in
   tests don't gate merge"). Without rule 2 we'd have stalled v2
   closeout. The library entry holds both rules together with the
   exact carve-out wording.
3. **Kent's definition already mandates `_ai/*.md` reading**
   (step 3, line 56). The library is the right shelf; the
   definition is the right pointer to the shelf. Don't duplicate.

## 5. The no-Docker constraint (explicit answer)

**Question**: Does any agent definition need to know about
no-Docker as a fact about this specific dev environment?

**Answer**: No. Three reasons:

1. **It's environmental, not behavioural.** A future contributor's
   box may have Docker. Encoding "no Docker on dev box" in an
   agent definition would mislead a Linux-host contributor.
2. **It's already saved correctly** as an instance-specific
   feedback memory for this assistant (per the earlier
   "Mac-no-Docker" memory write).
3. **The general lesson it surfaces is captured elsewhere.** The
   "what to do when an agent can't satisfy a verification gate
   because of environment constraints" pattern is exactly the
   hand-off-note shape used in
   `_tasks/2026-04-28-m2-server-side-mounts/integration-test-run-log-handoff.md`,
   which Ward considered promoting to library (020 candidate i)
   and correctly declined as a one-off pending recurrence.

## 6. Summary

Zero agent definitions updated. The disciplines learned from M2
are correctly housed:

- **`_ai/compile-clean-not-run-clean.md`** (NEW) covers the
  compile-vs-run discipline plus the opt-in-doesn't-gate-merge
  carve-out — applies to all agents that touch integration tests.
- **`_ai/error-wrap-discipline.md`** (EXTENDED) covers the
  one-sentinel-per-chain rule that Linus caught at plan stage.
- **`_ai/cobra-stringarray-not-stringslice.md`** (NEW) covers the
  Cobra flag-type gotcha Joel caught at planning §8.9.
- **`_ai/phantom-scope-kill.md`** (NEW) covers Don's milestone-
  resequence-residue discipline.
- **No-Docker dev-box fact** lives correctly as an
  instance-specific feedback memory, not in any agent definition.

The high bar held. No agent under-performed in a way that needs a
definition change; every observed behaviour is either within the
agent's role, already captured by Ward's library pass, or a
context-specific environmental fact handled by the right
mechanism.

## 7. Files reviewed

Task files (001–020 + handoff):
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/001-user-request.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/002-don-plan.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/003-joel-tech-plan.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/004-linus-plan-review.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/005-joel-tech-plan-addendum.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/006-linus-plan-review-v2.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/007-kent-tests.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/008-rob-impl.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/009-raymond-docs.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/010-kevlin-review.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/011-linus-impl-review.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/012-don-closeout.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/013-joel-tech-plan-addendum-v2.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/014-linus-addendum-v2-review.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/015-rob-impl-v2.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/016-kevlin-review-v2.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/017-linus-review-v2.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/018-don-closeout-vote.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/019-joel-closeout-vote.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/020-ward-knowledge.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/integration-test-run-log-handoff.md`

Agent definitions (read to confirm fit):
- `/Users/fenster/.claude/agents/kent.md` (closest-call agent)

Library entries Ward produced (read to confirm coverage):
- `/Users/fenster/dev/decloud/_ai/compile-clean-not-run-clean.md`
