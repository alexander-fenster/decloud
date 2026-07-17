# 011 — Kent: Mutation Resolution (Linus §4 vs Kevlin §2.2)

> Status: EXECUTION step 4 follow-up. Settles the direct factual contradiction between
> `009-linus-code-review.md` §4 and `010-kevlin-review.md` §2.2 about test 14, by re-running the
> mutation rather than adjudicating prose.
>
> **Verdict: LINUS IS RIGHT. Kevlin's "Not a hole" is wrong.** The gap was real. It is now closed with
> Linus's one row, proven to fail on the mutant and pass on shipped code. Production code untouched.

## 1. The contradiction, resolved

| Reviewer | Claim | Actual |
| --- | --- | --- |
| Linus §4 | mutant `if hasPrev && !req.DisableCompression` leaves `internal/deploy` **GREEN** | ✅ **Confirmed.** Full suite green, uncached. |
| Kevlin §2.2 | "all three terms of the warn condition are independently pinned… **Not a hole**" | ❌ **False.** The middle term was unpinned. |

I applied Linus's exact mutant to `service.go:320`:

```go
// shipped:  if hasPrev && prev.Config.DisableCompression && !req.DisableCompression {
// mutant:   if hasPrev && !req.DisableCompression {
```

**Result — the whole repo stays green:**

```
### FULL PACKAGE, -count=1, no cache ###
ok  	github.com/alexander-fenster/decloud/internal/deploy	12.089s

### FULL SUITE, -count=1 ###
ok  	internal/caddy    ok  internal/cli     ok  internal/config
ok  	internal/deploy   ok  internal/dockerdrv ...  (all 9 packages ok)
```

All three of my rows passed the mutant. Linus's walk of them is exactly right: `reset_without_flag`
still warns; `flag_passed_again` has `req=true` so `!req` is false; `first_deploy` has
`hasPrev=false`. **My table only tested states where the middle term cannot change the answer.**

**Note on method:** my first run reported `ok (cached)`. A cached result on mutated source is not
evidence, so I ran `go clean -testcache` and re-ran with `-count=1`. The result held. Anyone
re-checking this should do the same — it is the obvious way to get a false GREEN or a false CAUGHT.

## 2. Where Kevlin went wrong — he mutated two terms and reported three

His §2.2 table is not wrong in any individual row. I re-ran all of them and **both of his term
mutations reproduce exactly as he described**:

| Mutation | Kevlin's report | My re-run |
| --- | --- | --- |
| Drop `hasPrev` | panics (nil deref) | ✅ **PANIC (caught)** |
| Drop `!req.DisableCompression` | `flag_passed_again` fails | ✅ **FAIL (caught)** |
| Remove the warn entirely | `reset_without_flag` fails | ✅ (consistent) |
| **Drop `prev.Config.DisableCompression`** | **absent from his table** | ❌ **GREEN — survived** |

The error is a **counting slip, not a measurement error**: his third mutation was *remove the warn
statement entirely*, which is a whole-statement deletion, not a term deletion. So he mutated **two of
three terms plus the whole statement**, saw three green-to-red results, and wrote the conclusion as
"all three terms are independently pinned." Three mutations is not three terms. The one term he
skipped is the one that was unpinned.

This is worth recording precisely because his review was otherwise the most rigorous on the task — he
caught a real false claim about upstream `init()` that three of us had read past. **Mutation testing
proves what you mutate and nothing else. The conclusion must be scoped to the mutants actually run,
and the coverage claim ("all N terms") has to be checked against the operator list, not the row
count.**

## 3. The fix — Linus's row, added verbatim

`internal/deploy/service_test.go`, one row in the existing table:

```go
{"ordinary_redeploy_never_disabled", false, true, false, false},
```

*(prevDisabled=false, hasPrev=true, requestedDisabled=false, wantWarning=false — an ordinary redeploy
of a service that never disabled compression: the single most common operation in the product.)*

**Proven both directions, exactly as Linus did:**

- **New row + mutant → FAIL** ✅, quoting my own assert message back:

  ```
  --- FAIL: TestDeploy_WarnsWhenCompressionReEnabledOnRedeploy/ordinary_redeploy_never_disabled
      Error:    "...\"level\":\"WARN\",\"msg\":\"compression re-enabled: previous deploy set
                 disable_compression; pass --no-compression to keep it off\"..."
                 should not contain "--no-compression"
      Messages: a warning nobody can act on is trained-to-ignore within a week
  ```

  The captured log is the harm, visible: a `WARN` on a service that never touched the flag.

- **New row + shipped code → PASS** ✅ (all 4 rows pass; full suite green, `gofmt` and `go vet` clean).

**All three terms are now genuinely, independently pinned** — re-verified by running every condition
mutation against the fixed table:

```
drop hasPrev (Kevlin #1)                       -> PANIC (caught)
drop !req.DisableCompression (Kevlin #3)       -> FAIL (caught): /flag_passed_again
drop prev.Config.DisableCompression (MIDDLE)   -> FAIL (caught): /ordinary_redeploy_never_disabled
```

## 4. My own error, stated plainly

Linus's sharpest point is not that a row was missing — it is *which* row:

> "Both of them named this property as the one that mattered most, and neither of them tested it."

He is right and it is the part worth keeping. Joel wrote *"a warning that fires when it shouldn't is
worse than one that never fires."* I believed it enough to **type it into an assert message** — and
then wrote a table whose negative rows only covered the two cases nobody would plausibly break
(`flag_passed_again`, `first_deploy`), while the mutation a real reviewer would actually propose
("why is this condition three terms? simplify it") walked straight through.

The generalizable lesson, and it is not "add more rows": **I tabled the cases the implementation
suggested, not the cases the property required.** I enumerated around the code's shape — the guard
terms — instead of asking "what states must NOT warn?" and enumerating those. The most common state in
the entire product (ordinary redeploy, never disabled) was not in my table at all, because it isn't
interesting from the condition's point of view. It is the *only* interesting one from the operator's.

A negative row that names a property in its failure message and never exercises the state that
property protects is decoration. Mine was, for one deploy.

## 5. Tree state

- **Production code: untouched.** `git diff internal/deploy/service.go` is empty; the mutant is
  reverted and I re-verified the shipped three-term condition is byte-identical to `68bf792`.
- **I staged only `internal/deploy/service_test.go` and this report.** Raymond's `_ai/` docs fixes and
  Rob's `internal/cli/deploy_service.go` comment nit were in the working tree in progress while I
  worked; I deliberately left both unstaged and untouched.
- No docs touched, no `internal/cli` touched, no other test weakened or modified.

## 5.1 Process note — staging is not a lock

Briefly, because it cost real time and will recur: while I worked, a concurrent all-inclusive
`git add` swept my *staged* files into another agent's docs commit, and my own `git commit` reported
*"nothing to commit, working tree clean."* It was resolved at the source — that commit was amended to
drop my files, restoring them to my staging area — so **the final history is correct and this is a
non-event in the log.** I did not rewrite any shared history to chase it.

The lesson is worth one line for Ward: **with parallel agents on one branch, staging is not a lock.**
`git add` + a later `git commit` is a race. Commit with an explicit pathspec, and verify what actually
landed (`git log -S` for your own change) rather than trusting that a green `git commit` committed
*your* work.

## 6. Status

- **Linus §4 Option A: DONE.** One row, proven both directions.
- **Kevlin §2.2 "Not a hole": corrected.** It was a hole; his two term mutations were sound, his
  coverage claim was not.
- **Kevlin §7 item 1** (the false `init()`-only-from-`Write` claim) is Raymond's and is unaffected by
  this.
- Report discipline unchanged: **byte-asserted; pending operator `caddy validate`.** Nothing here
  claims validation — this was a Go-level mutation test, no Caddy involved.
