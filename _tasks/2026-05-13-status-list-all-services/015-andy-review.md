# Andy's review of agent definitions — `status-list-all-services`

Task slug: `status-list-all-services`. Branch: `task/status-list-all-services`.
Reports read: `01-user-request.md`, `002-don-plan.md`, `03-tech-plan.md`,
`04-linus-review.md`, `005-kent-tests.md`, `006-rob-implementation.md`,
`007-raymond-docs.md`, `009-linus-review-exec.md`, `010-kevlin-review.md`
(including Lap-2 verification footer), `011-don-round2.md`,
`012-raymond-padding-fix.md`, `013-don-final.md`, `014-joel-final.md`.
Git history reviewed: `git log main..HEAD --oneline` (14 commits, clean
linear workflow order).

## TL;DR

**No agent definition changes warranted this task.**

## What I checked

CLAUDE.md is explicit: agent definitions should not be updated lightly,
and the trigger is "user asked for any corrections." I went looking for
that signal at three levels:

### 1. Did the USER (human) intervene?

No. The user's only interaction was the initial `/do` request quoted in
`01-user-request.md`:

> for "decloud status" command, can we have a no parameters variant
> that would list all the registered services with their statuses?

After that, the task ran 14 commits end-to-end without a single
follow-up message, course-correction, complaint, or expression of
dissatisfaction. No "wait, that's not what I meant," no "you missed
X," no "do it differently."

The user shipped a clean prompt and the agent team delivered. There
is no user correction to align agent instructions to.

### 2. Did any agent do something its description forbids?

No. Spot-checked each agent's behaviour against what I know of their
description:

- **Don** (plan): traced actual file/line refs (`internal/cli/status.go:11-30`,
  `internal/deploy/lifecycle.go:91-118`, `internal/registry/store.go:175-204`)
  before proposing. Pattern-discovery preamble was honoured (he called
  out that "list every registered service" was already implemented as
  `registry.Store.List`, then deliberately routed around it for the new
  surface). No invented APIs.
- **Joel** (tech plan): expanded with concrete signatures and pushed
  back on Don's over-engineered nine-state error enum — collapsed to
  one state with detail on stderr. This is exactly the "real
  disagreement when you have one" behaviour the workflow wants.
- **Linus** (plan review + exec review): read actual code, verified
  file/line refs, listed concrete approvals and concrete non-blocking
  observations. Did not rubber-stamp.
- **Kent** (tests): wrote tests that compile to "missing-implementation"
  failures (per CLAUDE.md test-first discipline), added the exact
  tests Joel called out plus the Linus-suggested full-line equality
  tightening on the existing single-service test (`04-linus-review.md`
  Risk A). Not change-detector tests.
- **Rob** (impl): zero deviations from Joel's plan. Ran `go generate`
  cleanly, verified the three expected mock diffs, ran `gofmt -l`
  empty, ran the `%w: %v` grep clean. Went and verified each tech-plan
  §1.x line.
- **Raymond** (docs): cross-referenced every doc claim to a code line.
  Kevlin caught one drift (hand-typed tabwriter padding in two example
  blocks), Raymond fixed it byte-exact and verified by running the
  exact rows through real `text/tabwriter`. The doc-as-source-of-truth
  bar held.
- **Kevlin** (low-level review): hallucination-checked the five-value
  state enum at five code sites in `lifecycle.go`. Caught the
  tabwriter-rendering drift in the docs that Raymond had flagged as a
  known risk. Lap-2 re-verification was byte-precise.

No agent did something its description says not to do. The workflow
caught its own drift (the docs padding nit) via the planned review
discipline — that is the system working, not a system gap.

### 3. Did any agent need extra context not in its description?

No. The one round of iteration (Don round-2 → Raymond padding fix →
Kevlin Lap-2 re-verification) was triggered by Kevlin's careful low-
level review, which is *exactly* what his role is for. Don escalated
Kevlin's nit from "non-blocking" to "iterate" because he applies a
strict "docs must not lie about what the binary prints" bar. That is
Don's job at PLAN-after-EXECUTION, not an instruction gap.

The workflow defined in CLAUDE.md held up cleanly:

1. PLAN (Don → Joel → Linus) produced a tight, verified plan.
2. EXECUTION (Kent → Rob → Raymond → Kevlin + Linus parallel review)
   shipped the plan with one real defect caught.
3. PLAN round 2 (Don assessed, deemed ITERATE on docs only).
4. EXECUTION lap 2 (Raymond's padding fix, Kevlin's Lap-2 verify).
5. PLAN final (Don DONE, Joel DONE; Linus's earlier APPROVED stands
   since lap-2 was doc-only).

That is the workflow doing its job. No gap surfaced.

## What I considered but rejected as triggers

For completeness, I weighed three candidate "should we update an
instruction?" items:

1. **Should Raymond's description say "never hand-type tabular doc
   examples; always render through real tabwriter"?** The lesson is
   real but it is task-shaped, not agent-shaped — it is exactly the
   sort of operational learning Ward's STEP 4.1 captures into `_ai/`
   for the next time docs claim to mirror a stdlib rendering. Adding
   it to Raymond's description would risk over-fitting to one
   stdlib library. Better as a `_ai/` doc with a general rule
   ("when docs claim to mirror programmatic output, render it
   programmatically").

2. **Should Don's description push him to call Kevlin-style nits
   "iterate" by default?** No. Don's strict ship-bar (no docs that
   lie about actual stdout) is a judgment call that worked exactly
   right here. Codifying it into "always iterate on nits" would
   over-correct: many nits genuinely are non-blocking, and Don's
   role is to discriminate. The discrimination is the value.

3. **Should the workflow definition in CLAUDE.md call out a
   "single-agent re-review lap" pattern (Don's "Kevlin only, not
   Linus, for a one-file doc fix") as canonical?** The CLAUDE.md
   workflow already permits this implicitly ("iterate to come up
   with a new plan"), and Don exercised judgment correctly. Adding
   prescriptive guidance about which reviewer to re-run for a given
   nit class would be premature standardisation.

These are all candidate `_ai/` learnings for Ward, not agent
description changes.

## Recommendation for knowledge-librarian

**RECOMMENDATION for Ward / knowledge-librarian:** Consider adding a
learning to `_ai/` along the lines of "When docs include example
output that purports to mirror a programmatic rendering (e.g.
`text/tabwriter`, `json.MarshalIndent`, `pretty.Sprint`), render
the example by running the actual library against the actual values;
do not hand-type the spacing. Verify with a throwaway Go file or by
capturing real test output as a fixture." This is the generalisable
operational learning from Raymond's §5.3 acknowledged-risk → Kevlin's
N1 catch → Don's round-2 ITERATE → Raymond's lap-2 byte-exact fix
chain. Suggested file: a new `_ai/doc-example-rendering.md` or an
addendum to whichever existing `_ai/` doc covers docs/contract
coherence (likely `_ai/cli-flag-surface-coherence.md`'s neighbour).

## Final verdict

**No agent definition changes warranted.**

The user did not correct anything. No agent violated its description.
The workflow as defined caught its own drift via planned discipline,
not by exception. The one operational learning (render examples
programmatically, never hand-type) is a `_ai/` knowledge-base entry,
not an agent-instruction patch.

Proceed to squash-merge.

— Andy
