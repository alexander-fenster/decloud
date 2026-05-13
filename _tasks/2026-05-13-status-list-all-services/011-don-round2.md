# Don's round-2 assessment — `decloud status` lists every registered service

Task slug: `status-list-all-services`. Branch: `task/status-list-all-services`.
Predecessor reports read: `002-don-plan.md`, `03-tech-plan.md`,
`04-linus-review.md`, `005-kent-tests.md`, `006-rob-implementation.md`,
`007-raymond-docs.md`, `009-linus-review-exec.md`, `010-kevlin-review.md`.
Branch shape: nine commits, `git diff main...HEAD --stat` shows 21 files,
+2870/-20. Code/test/mock files all touched per the plan; no drift.

## TL;DR

**VERDICT: ITERATE.** One more EXECUTION lap. Raymond fixes two example
blocks in `_docs/usage.md`. Then re-review (Kevlin only — single-file
doc nit, no need to drag Linus back in) and we're done.

Single most important reason: Kevlin caught two **operator-visible docs
hallucinations** — the example blocks claim a column layout that
`tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)` does not produce. I verified
his claim byte-for-byte by running the exact rows through stdlib
tabwriter. He's right. Shipping it would publish docs that lie about
what `decloud status` actually prints. That is not "non-blocking" in
my book — that is exactly the embarrassment-driven-development bar I
hold the team to. Five-minute fix; do it.

## What I verified myself (not assumed)

### 1. Kevlin's tabwriter claim — VERIFIED, byte-precise

I refused to take this on faith. I ran the exact strings from
`_docs/usage.md` §4.1 and §5 through `text/tabwriter` with the same
config the code uses (`NewWriter(out, 0, 0, 2, ' ', 0)`). Output:

**§4.1 actual:**

```
NAME        STATE    CONTAINER    DEPLOY                  DEPLOYED_AT
bar         stopped  decloud-bar  20260426-102001-aa11bb  2026-04-26T10:20:01Z
broken-svc  error    -            -                       -
foo         running  decloud-foo  20260426-093214-7f3a9c  2026-04-26T09:32:14Z
```

**§5 actual:**

```
NAME       STATE    CONTAINER          DEPLOY                  DEPLOYED_AT
myservice  running  decloud-myservice  20260426-093214-7f3a9c  2026-04-26T09:32:14Z
other      stopped  decloud-other      20260425-110000-def456  2026-04-25T11:00:00Z
```

What the docs currently show: CONTAINER column padded to ~22 chars
(§4.1) and ~20 chars (§5). Reality: tabwriter padding is the configured
`padding=2`, so the column is exactly 2 spaces after the widest cell.
Kevlin's replacement blocks are byte-correct.

### 2. The branch shape

`git log main..HEAD --oneline` is exactly the nine commits I'd expect:
user request, Don's plan, Joel's tech plan, Linus's plan review, Kent's
tests, Rob's impl, Raymond's docs, Linus's exec review, Kevlin's exec
review. No surprises. No stray "fix" commits. Clean.

### 3. Linus's review

Read in full. Approved. Both his non-blocking notes from
`04-linus-review.md` (`ErrorDetail` as `string`, single-service test
tightened to full-line equality) were addressed. No new concerns from
him. I have no reason to drag him into a re-review of a two-block doc
fix.

## Why this is not "non-blocking"

Kevlin classified the nit as non-blocking. He is right that it doesn't
affect operator behaviour and the docs are internally readable. **I
disagree on the ship-or-fix line, and here is why:**

1. **Docs that lie about literal stdout are user-facing wrongness.**
   An example block under "Status format" with monospaced shell output
   carries an implicit contract: "this is what you will see." When the
   real output doesn't match, the operator either thinks their binary
   is broken or learns to stop trusting our docs. Both outcomes are
   worse than the five-minute fix.

2. **Raymond himself flagged this risk** in his §5.3
   ("...the spacing in the example is hand-typed to approximate
   tabwriter's two-space padding. If Kevlin wants byte-precise
   alignment, the right fix is a follow-up..."). The hand-typed
   approximation drifted exactly the way he warned it might. Kevlin
   now has the byte-precise blocks. We have zero excuse to leave
   wrong docs in the repo when correct ones are sitting in the review
   report.

3. **The cost is two paste operations.** Five minutes from Raymond
   + a one-file re-skim from Kevlin. The cost of NOT fixing is "the
   next maintainer who runs `decloud status` and diffs against our
   docs files a bug." Embarrassment-driven development says fix it
   now.

4. **Kevlin's verdict was APPROVED with one nit; my verdict on round
   2 is what gates DONE.** Per CLAUDE.md "Don, Joel and Linus must
   ALL agree the task is fully done." I'm Don. I don't agree yet.

## What must happen in the next EXECUTION lap

### Required change — Raymond

**File: `_docs/usage.md`**

**Block 1 — replace lines 227-230 verbatim** with:

```text
NAME        STATE    CONTAINER    DEPLOY                  DEPLOYED_AT
bar         stopped  decloud-bar  20260426-102001-aa11bb  2026-04-26T10:20:01Z
broken-svc  error    -            -                       -
foo         running  decloud-foo  20260426-093214-7f3a9c  2026-04-26T09:32:14Z
```

**Block 2 — replace lines 285-287 verbatim** with:

```text
NAME       STATE    CONTAINER          DEPLOY                  DEPLOYED_AT
myservice  running  decloud-myservice  20260426-093214-7f3a9c  2026-04-26T09:32:14Z
other      stopped  decloud-other      20260425-110000-def456  2026-04-25T11:00:00Z
```

No other edits to `_docs/usage.md`. No `README.md` change. No code
change. No test change. The byte-precise blocks above were verified
by me with the actual stdlib `text/tabwriter` against the same
config the code uses.

### Verification step — Raymond

After editing, Raymond MUST verify the blocks by running the actual
rows through a one-off Go file with `tabwriter.NewWriter(out, 0, 0,
2, ' ', 0)` (or by piping `decloud status` output from a real test
fixture). The "hand-typed approximation" approach is what got us
here; we replace it with proof.

### Re-review — Kevlin only

This is a two-block doc fix. Kevlin already specified the exact
replacement bytes. The re-review is "did Raymond paste what Kevlin
wrote." That's a 30-second sanity check, not a full second pass.
Linus does NOT need to re-review — his APPROVAL stands on the code
and execution; this doesn't touch either.

### After Kevlin signs off

Don/Joel/Linus declare done. Proceed to STEP 4: Ward (learnings),
Andy (agent instruction review), squash-merge to `main`.

## Other gaps I looked for and did NOT find

I went looking for things to be unhappy about. Specifically:

1. **Linus's "Observation A" (double `loading service:` prefix on
   stderr).** Linus called this out as a polish-not-a-bug. I agree.
   Not a round-2 issue. If it bothers an operator, one
   `strings.TrimPrefix` in `StatusAll` solves it later. Skip.

2. **Raymond's §3.3 unrelated `--config-root` doc concern.** Out of
   scope per the task brief. Flagged in his report for a future
   task. Not a round-2 issue. Skip.

3. **The five-value state enum hallucination check.** Kevlin
   verified all five values at five code sites in `lifecycle.go`.
   The docs list exactly five values. Surface coherence holds.

4. **Mock regen scope.** Three expected diffs, one empty
   (`mock_deployer.go`). Joel's safety check passed. No drift.

5. **Test coverage.** Kent's eight `StatusAll` tests + seven
   `ListNames` tests + six new CLI tests cover every branch in
   §3 of Joel's tech plan. The architectural keystone
   (`List` silent-skip vs `ListNames` no-skip) has two named
   regression locks. Kent's
   `TestStore_ListAndListNamesAgreeWhenAllServicesLoadCleanly`
   pins the refactor invariant. That is the right test discipline.

6. **gofmt / `%w` discipline.** Both Kevlin and Rob verified.
   Clean.

7. **The single-service path bit-for-bit identical.** Linus
   verified at code-line precision. Kent's test tightened to
   full-line equality locks it. Right.

Nothing else gates DONE. The doc nit is the only thing.

## Final verdict

**VERDICT: ITERATE.**

Next lap: Raymond replaces the two example blocks in `_docs/usage.md`
with the byte-precise blocks above, then Kevlin re-skims that single
file diff. Then we're DONE.

Don't ship docs that lie about what the binary prints. Five minutes.
Do it.

— Don
