# 009 — Linus's Code Review: HTTP compression in the generated Caddyfile

> Status: EXECUTION step 4, high-level architectural review of `0b7371a` (Kent), `68bf792` (Rob),
> `f83568b` (Raymond). Kevlin reviews low-level in parallel. Reviewed `git diff main...HEAD`.

## VERDICT: **APPROVED.** One test gap worth closing (§4). Nothing else.

The implementation is the plan, exactly. Three files, 8 production lines outside the CLI, five
non-goals absent, no test weakened, no mock churned, no lifecycle plumbing. I verified the tripwire
myself with `git diff -w` — Rob's count is honest and his attribution of the raw 15 to gofmt
re-aligning existing struct fields is correct.

I found **one thing worth fixing**, and it's a test gap, not a defect: **test 14 does not lock the
warning's condition.** I proved it by mutation (§4). The shipped code is right; the suite would not
notice if it stopped being right, in the one way it's plausibly likely to.

Everything else below is me answering the three questions I was asked, and I verified all three
myself rather than adjudicating between reports.

---

## 1. Raymond vs. the plan text — **Raymond is RIGHT. Ship his wording.**

**He's right, the plan is wrong, and the proof is already in Kent's test suite.**

The condition reads `prev.Config.DisableCompression`. `prev.Config` is whatever `fsStore.Load`
decoded out of the TOML. It is **provenance-blind** — there is no field, no flag, nothing anywhere in
`ServiceConfig` that records whether a value came from a `--no-compression` flag or a text editor. So
a hand-set `disable_compression = true` satisfies the condition identically and **warns on reset**.
"Silently wiped" is false.

**The shipped tests already prove this, which nobody noticed.** Kent's test 14 sets

```go
prev.Config.DisableCompression = tc.prevDisabled
```

**directly on the struct — it never routes through a flag.** That is byte-for-byte the state a
hand-edited TOML produces after `Load` decodes it. So `reset_without_flag` **is** the hand-edit case,
already green, already shipped. Raymond's doc claim is backed by an existing passing test.

**How the plan got it wrong:** "silently" was written in rev 1, when the reset *was* silent. Don's
§7.1 Option C ruling falsified it. Neither Don nor Joel updated the trap wording, and **I reviewed
both revisions and didn't catch it either** — I checked that Option C was specified correctly and
never checked what it invalidated elsewhere in the same document. Three of us read past a plan that
contradicted itself.

**Raymond's call is correct and the reasoning is the important part.** He was handed explicit
instructions from two senior agents, checked them against the code, found them false, and shipped the
truth instead of the instruction. Copying "silently" into `_docs/` would have put a fabrication in
permanent documentation on the authority of a stale plan — which is exactly the failure mode the doc
step exists to prevent. **A doc writer's job is accuracy, not obedience.** He also flagged the
contradiction rather than quietly diverging, which is what makes it reviewable. This is the behavior
I want.

The principle, worth stating because it will recur: **the plan is a historical artifact, the code is
the truth, and the decision record is what outlives both.** `_tasks/` is immutable by convention and
Raymond correctly left `002`/`003` alone — do **not** retro-edit the plans. The decision record
carries the correct version, and this review records that the plan text at `002` §5 and `003` §8 is
stale on this one point, so nobody reads it in a year and "corrects" the docs back to the lie.

His shipped wording is also *better* than the plan's: most keys are replaced silently,
`disable_compression` is the exception that warns, and `--no-compression` is the durable answer. That
tells the truth **and** still routes the operator to the fix.

---

## 2. The TOML placement trap — **documenting is right, and our own rule says so**

I reproduced it myself against the real `fsStore.Load` rather than trusting the report. Appending the
key at EOF:

```
registry: unknown field in TOML: 32| container_name = "decloud-foo"
    33| last_deployed_by = "operator"
    35| disable_compression = true
      | ~~~~~~~~~~~~~~~~~~~ missing field
```

It binds to `[state]` — you can see `[state]`'s own fields in the error context — and exit 10.
Placed above the first `[table]`: loads clean, `DisableCompression = true`. **Raymond's probe
reproduces exactly.** (Probe deleted; tree clean.)

**Does a top-level scalar that breaks on the obvious edit indicate something about the config
surface?** Two honest answers, and neither is "reopen the field":

**First: the trap is not new and not specific to this key.** It applies to *every* top-level scalar —
`schema_version`, `name`, `strategy`, `last_deployed_at`. This change did not create it. What it
created is **reachability**: `usage.md:150` invites hand-editing, and everything it previously invited
you to edit was table-scoped (`[[run.mounts]]`, `[readiness]`). `disable_compression` is the first
top-level scalar we have ever told an operator to hand-set. The trap was always loaded; this task is
what put a finger near the trigger.

**Second: apply the rule we just wrote.** Is this failure silent? **No** — exit 10, the file refuses
to load. Misattributed? **No** — the error names the exact line and key with a `~~~ missing field`
marker pointing at it. Unworkaroundable? **No** — move the key up three lines. **0-for-3.** By the
rule now sitting in our own decision record, this earns **documentation, not engineering.**

That is worth pausing on. We wrote that rule two steps ago to justify a knob. It just answered a
completely different question — one nobody anticipated — without argument or taste. **That's what
distinguishes a rule from a rationalization**, and it's the strongest evidence yet that recording it
was worth the space.

For the record, since someone will propose it: a table-scoped knob (`[caddy]` `disable_compression =
true`) *would* be marginally more hand-edit-safe, because appending a **header** at EOF works — a new
table header resets binding context. It is still not worth it: inconsistent with `strategy`, a nested
struct for one bool, and it buys nothing against a loud failure. **Do not reopen a shipped, tested
field for this.**

The real insight is Raymond's closing line, and it's the correct posture: **"Treat the TOML as
something to read, not something to configure with."** The trap isn't a defect in the key — it's a
symptom of a doc paragraph inviting people to hand-edit a file the system owns and regenerates. His
rewrite pushes them toward the flag. And he generalized the rule into `_ai/apidocs.md` to cover
`strategy` and every future top-level scalar rather than documenting only his own key. Right
diagnosis, right scope.

---

## 3. Help text "streaming/SSE backends" — **right call, both of them**

Rob shipped it, Raymond kept it and matched `usage.md` to it verbatim. Both correct.

Don's §3.0 rule and the help text **serve different readers**, and Raymond identified the split
precisely:

- **§3.0 protects the contributor** who might reach for `match`. That person reads
  `_ai/caddyfile-generator-facts.md`, which now says outright: *"This is a headers-then-idle bug, not
  an SSE bug"* and *"Anyone who reaches for `match` here will ship a fix that does nothing."* The
  rule is enforced exactly where the mistake gets made.
- **`--help` serves an operator with a hanging `EventSource`.** That person will never write a `match`
  block — **they can't, the Caddyfile is generated.** The failure mode §3.0 guards against is not
  reachable from the surface Rob is writing on. What they need is *recognition*, and "SSE" is what
  they type into a search box at 1am.

And decisively: **"streaming" is first.** `streaming/SSE backends` offers SSE as an example, not a
cause. It never implies `text/event-stream` selects the failure — which is §3.0's actual concern, not
a ban on the three letters. Dropping "SSE" would cost a real operator the moment of recognition and
buy nothing, because the general term already leads.

Four-surface coherence holds — I checked the diff: `--help` says *"set this for streaming/SSE
backends"*, the `usage.md` flag row says *"Set it for streaming/SSE backends."* Surfaces 3 and 4 agree
verbatim, and the flag row carries the mechanism the help string has no room for.

**No change.** Raymond's instinct to ask rather than silently obey a terminology rule that didn't fit
his surface is the right one.

---

## 4. THE FINDING: test 14 does not lock the warning's condition

**The shipped code is correct. The suite would not catch it becoming incorrect.** Proven, not argued.

**Problem.** I mutated the shipped condition by deleting the middle term:

```go
// shipped (correct):
if hasPrev && prev.Config.DisableCompression && !req.DisableCompression {
// mutant:
if hasPrev && !req.DisableCompression {
```

**The entire `internal/deploy` package stays GREEN.** All three rows of test 14 pass.

Walk the rows against the mutant: `reset_without_flag` still warns ✓; `flag_passed_again` has
`req=true` so `!req` is false ✓; `first_deploy` has `hasPrev=false` ✓. **Every row passes.** The
table happens to test only states where the middle term doesn't change the answer.

**Impact.** That mutant warns on **every ordinary redeploy of every normal service** — any service
with a previous config that doesn't pass the flag. That is the single most common operation in the
product. Every operator, every deploy, every service that never streamed, told to pass
`--no-compression`.

And that destroys the feature. **Option C's entire value is that the warning is credible.** Joel
wrote it (§6.5): *"a warning that fires when it shouldn't is worse than one that never fires."* Kent
believed it enough to write it as an assert message: *"a warning nobody can act on is trained-to-ignore
within a week."* **Both of them named this property as the one that mattered most, and neither of them
tested it.** The suite defends the two negative cases nobody would plausibly break and misses the one
a plausible edit reaches — someone "simplifying" a three-term condition, which is exactly what a
reviewer who hasn't read this task would suggest.

**The gap is one row:**

```go
{"ordinary_redeploy_never_disabled", false, true, false, false},
```

*(prevDisabled=false, hasPrev=true, requested=false, wantWarning=false — an ordinary redeploy of a
service that never disabled compression.)*

**I proved the row works before proposing it:**

- new row + **shipped** condition → **PASS** (it doesn't break correct code)
- new row + **mutant** → **FAIL**, quoting Kent's own message back:
  `"...should not contain \"--no-compression\"` / `a warning nobody can act on is trained-to-ignore
  within a week"`

**Options:**

- **Option A (recommended): add the one row.** Pros: proven above, one line, no new mechanism, no new
  helper, lands in the existing table. Cons: none I can find.
- **Option B: leave it.** The code is correct and the mutant is hypothetical. Pros: ships now. Cons:
  it is the cheapest test in the entire task, and it guards the property both Joel and Kent named as
  the most important one. Declining costs a line and buys nothing.
- **Option C: mutation-test the package properly.** Out of scope for this task; a real idea for
  another one.

**My recommendation: Option A.** Kent adds one row. This is not a decision that needs Don.

*(Tree is clean — all probes and mutations reverted, `git status` empty, `go test ./...` green.)*

---

## 5. Closing Raymond's open item — the Caddy internals are verified

Raymond flagged this himself (`008` §8.4) and it's the most honest thing in his report:

> "The decision record's Caddy-internals claims… are **Don's and Linus's source reading of upstream
> Caddy, not mine.** I could not verify them against `encode.go` from this box… **If you want them
> independently confirmed, that is a real gap and I would rather you catch it than have it sit in a
> permanent decision record.**"

**Gap closed. I verified every one of them at source in `004` §1 and re-confirmed today** against
current `master` `encode.go`:

| Decision-record claim | Source | Verdict |
| --- | --- | --- |
| Wrapper installed on request `Accept-Encoding` alone, before Content-Type is knowable | `encode.go:162-168` | ✓ |
| `match` consulted only inside `init()`, only reached from `Write` | `encode.go:449-458` | ✓ |
| `FlushError()` → `if !rw.wroteHeader { return nil }` | `encode.go:302-308` | ✓ |
| `init()` only from `Write`, only above 512 bytes, never retried | `encode.go:337-350` | ✓ |
| `FlushError()` *does* now sync-flush the encoder (so "SSE completely broken" is wrong) | `encode.go:311-316` | ✓ |
| `Vary: Accept-Encoding` added in `init()`, guarded; 304 handled separately | `encode.go:463-464`, `:269-270` | ✓ |
| caddy#6293 open, created 2024-05-02, updated 2026-03-17 | `gh api` | ✓ |

**Every claim in that decision record is accurate.** Raymond phrasing them as mechanism rather than
line-number cites was also the right call — upstream line numbers rot, and the mechanism is what a
future reader needs. Flagging the limit of his own verification instead of laundering two agents'
reports into confident permanent prose is precisely correct, and it's the reason I could close it in
five minutes.

---

## 6. What was done right — recorded, because it's rare

- **Rob's `hasPrev` probe.** Three people (Joel, Kent, me) told him the guard was load-bearing. He
  removed it and got a panic trace anyway. **Being told by three seniors is not evidence.** He turned
  an argument into a fact, and the next person who thinks that first term looks redundant now has a
  stack trace instead of an appeal to authority.
- **Rob read the emitted bytes.** `Contains` passes on a structurally broken file. He generated a real
  Caddyfile from a two-service registry and *looked at it* — encode in both of alpha's blocks, absent
  from streamy's which kept its `reverse_proxy`, absent from the global block, 4-space indent — then
  deleted the temp test. Every property the plan asked for, visible at once, and he correctly refused
  to call it validation.
- **Rob declined his own latitude.** He was explicitly allowed to polish the warning's prose and
  chose not to, because the line already carried both tokens and three people had converged on it.
  Knowing when *not* to exercise permission is rarer than knowing when to.
- **Kent sharpened the spec.** Joel's §5.1 field comment said "streaming (SSE)"; Kent shipped
  "streaming (headers-then-idle)" — an unprompted improvement in exactly the direction Don's §3.0
  wanted, in the one comment a future maintainer reads before deleting the field.
- **Kent's `expectHappyPathDeploy`.** Three rows differing only in the `Load` result, with `Save` left
  to the caller so test 13 can capture and test 14 doesn't have to. The one line that varies is
  visible. That's what a test helper is for.
- **Raymond skipped `install.md` with a reason.** He read `:56`, found it's the *firewall* paragraph,
  and said a sentence about `encode` there would be a non-sequitur. A doc writer who reads the target
  and declines beats one who pads to satisfy a checklist item.
- **The tripwire worked as designed.** Joel predicted ~8, Rob hit 8, measured with `-w`, and explained
  the raw 15. I verified it independently. A number written down in advance, audited after, with the
  delta explained — that's the whole point of writing it down.

---

## 7. Status

**APPROVED.** The architecture that shipped is the architecture that was approved in `004`/`005`, and
the three questions escalated to me all resolve in favor of what shipped.

- **Required: nothing.**
- **Recommended (Kent, one line, proven in §4):** the `ordinary_redeploy_never_disabled` row.
- **Recorded, no action:** the plan text at `002` §5 / `003` §8 is stale on "silently" (§1). Do not
  retro-edit `_tasks/`; the decision record is correct and this review notes the divergence.
- **Deferred to Kevlin (his altitude, not mine):** Rob's two name-asymmetry comments (`007` §6.1) —
  CLI-site and struct-field. He's already flagged which one to drop if it reads as duplication.

Report discipline held throughout: every report says **"byte-asserted; pending operator `caddy
validate`"** and nobody claimed validation. Kent, Rob, and Raymond each independently verified a claim
they were handed rather than taking it on authority — the `hasPrev` panic, the emitted bytes, the TOML
placement probe, the `ErrSecretsMissing` fall-through. That's three for three, and it's why this
review found one missing test row instead of a design defect.

---

# 8. FINAL JUDGMENT (appended 2026-07-17, PLAN step)

> **Appended, not rewritten.** §1-§7 stand as written, **including §5, where I certified a false
> claim.** See §8.1. Same rule I imposed on everyone else: plans and reviews are history, the
> decision record is truth, and a review that quietly patches its own errors is worthless as
> evidence of what the reviewer actually caught.

## 8.1 CONCUR: **FULLY DONE.** But my own error goes first.

**In `004` §1 I put this in a table and marked it CONFIRMED:**

> | `Match` only reached from `init()`, only reached from `Write` | `encode.go:449-458` | **CONFIRMED.** |

**It is false.** `init()` has two call sites: `:349` (from `Write`) and `:423` (from `Close`). I
checked today: `grep -n "init()" encode.go` → three hits, two of them call sites. **One command. Three
seconds.** I never ran it.

Worse than making the error: **in `009` §5 I used it to close Raymond's honesty gap.** He said, in
writing, that he could not verify the upstream claims from his box and would rather someone catch it
than have it sit in a permanent decision record. I answered with a seven-row table certifying every
claim accurate — **including that one** — and wrote "Gap closed." He flagged the exact risk. I
laundered a false claim through my own authority and told him to stop worrying. Kevlin caught what I
certified.

**The shape of the error is the part worth keeping.** I verified a *positive* and asserted a
*negative*. I observed that `init()` **is** called from `Write` — true, `:349`. I concluded `init()`
is called **only** from `Write` — false. A presence claim is confirmed by an observation. **An
absence claim requires a search**, and I never ran the search because seeing the call site *felt* like
verification. Don named this about himself in nearly identical words ("I read one call site and
asserted exclusivity… I spent the task telling others that being told is not evidence, then wrote a
false universal from a single observation"). I did the same thing, in the same file, about the same
function, while being the loudest voice on this task demanding source verification.

**The conclusions survive — and I verified *why*, rather than accepting "both gate on
`!wroteHeader`":**

- `Close`'s `init()` is gated on `!rw.wroteHeader` (`:420`).
- `Write` sets `rw.wroteHeader = true` on the **first** write (`:359-364`).
- ⇒ For any response that ever wrote a byte, `Close`'s branch is **dead code**.
- ⇒ For a stream that wrote nothing, `Close` requires `Content-Length > MinLength`; a streaming
  response has no `Content-Length`, so `strconv.Atoi("")` errors and `init()` is not called.

So **"a streaming service is never actually compressed" holds**, "`match` cannot save streaming"
holds (it depends on the *wrapper install point*, not on `init()`'s call sites), the flush-swallow
argument holds (it depends on `FlushError`, untouched), and WebSockets holds. Kevlin's correction is
**accurate and non-consequential** — the best kind to receive: it costs nothing and makes the record
true. It is corrected in the record. Nothing else moves.

## 8.2 The three changes — verified, not accepted

| Change | My check | Result |
| --- | --- | --- |
| My §4 row shipped | `grep` `service_test.go:1114` | ✅ `{"ordinary_redeploy_never_disabled", false, true, false, false}` |
| Production condition untouched | `service.go:320` | ✅ still three terms, `hasPrev` first |
| Kevlin's `init()` correction | `grep -n "init()"` on current master | ✅ `:349` **and** `:423` — he's right, I was wrong |
| Conclusions survive | read `:420`, `:359-364` | ✅ both gated on `!wroteHeader`; verified the mechanism |
| CRIME in the durable record | `grep -ric crime _ai/ _docs/` | ✅ 2 (was **0**) — answered first, named in the heading |
| h3/Safari scar cross-ref | `caddy-runs-in-container.md:59`, `generator-facts.md:43-45` | ✅ both accurate; `Alt-Svc`-gated-on-listener is real |
| Suite | `go clean -testcache && go test ./...` | ✅ **9/9 uncached** |
| Tree / diff | `git status`, `git diff -w --numstat` | ✅ clean; 8 production statements outside CLI |

**Kent settled my contradiction with Kevlin the only way it should be settled — by running it**, and
caught that his own first run was `(cached)`. A cached result on mutated source is not evidence. That
detail is worth more than the finding.

**And Kent's diagnosis beats my finding.** I said a row was missing. He said *why*: **"I tabled the
cases the implementation suggested, not the cases the property required."** He enumerated around the
condition's shape — its guard terms — instead of asking "what states must NOT warn?" The most common
state in the entire product wasn't in his table because it is uninteresting from the *condition's*
point of view and the only interesting one from the *operator's*. That sentence is the durable
artifact of my review, and it's his, not mine.

**Raymond's two additions are good and rest on verified ground.** The zstd/gzip section stands on
this repo's own scar, and the contrast is exactly right: h3 broke Safari because Caddy **advertises**
it via `Alt-Svc` regardless of client health (server-driven); an encoding is only ever **negotiated**
(client-driven). Opposite mechanisms, opposite risk. The two changes look identical — "newer protocol
first in a Caddy directive" — and are structurally nothing alike. **That's the hazard, and it's now
answered where someone about to edit `encode zstd gzip` will actually look.**

## 8.3 The meta-finding: every error that survived this task was absence-shaped

Don and Joel are both right, and they're describing the same bug from two angles. I can close it,
because I now have **four** data points — including my own — and they are all the same error:

| # | Error | Whose | The absence claim, unverified |
| --- | --- | --- | --- |
| 1 | `init()` "only from `Write`" | **Mine**, certified twice | *no other call sites* — never grepped |
| 2 | Test 14's missing row | Kent's | *no unprotected states* — enumerated the code's branches, not the property's states |
| 3 | "All three terms pinned" | Kevlin's | *no unpinned terms* — counted his rows, not the operators |
| 4 | CRIME absent from the record | Don's + Joel's spec | *nothing missing from the list* — nobody asked what wasn't on it |

**Four errors, four agents, four different artifact types — upstream source, a test table, a mutation
report, a doc spec. Every one is the same bug.** And note what is *not* on this list: **not one false
positive survived.** Every affirmative claim anyone made got caught — by source reads, by mutation, by
grep, by probe. The verification culture on this task worked perfectly on everything that was
*there*. It has no mechanism at all for what isn't.

**The rule, for Ward:**

> **A presence claim is confirmed by an observation. An absence claim requires a search — and the
> search must range over the space the claim quantifies, not over the artifact in front of you.**
>
> - *"`init()` is only called from `Write`"* → search the **call sites** (`grep`); don't read one.
> - *"all three terms are pinned"* → enumerate the **operators**; don't count your rows.
> - *"the record is complete"* → grep the **originating request's own nouns** and look at the zeroes;
>   don't check the items on your own list.
> - *"these states must not warn"* → enumerate the **property's** states; don't enumerate the code's
>   branches.

Every one of those is a single command. Every one was skipped by someone who was, in that same
document, unusually rigorous. **That's the point: rigor about what is present generates the *feeling*
of having verified, and the feeling is what stops the search.**

**Joel's sentence is the sharpest thing produced on this task and should survive verbatim into Ward's
notes:** *"A numbered list is where completeness goes to die — it reads as rigor, so reviewers check
the items in it and never ask what isn't."* A numbered list is an **absence-hiding structure**: it
presents its own contents as the domain. Reviewers audit the items. Nobody audits the domain. I
reviewed that ten-item list in `005` and endorsed it item by item — which is exactly how it defeated
me.

**Joel's location of the drop is correct and I'll say so plainly:** it wasn't Raymond's. Don's §10
puts the loss at plan→record; Joel checked and found CRIME absent from *both* his §8 ten-item list
*and* Don's rev-2 §5.4. **Raymond shipped precisely what both planners specified.** They specified
wrong, and I approved the spec — `004` §3 singled out the CRIME dismissal as *"correct and worth
making"* and then I approved a record spec that didn't carry it. **Praising a thing is not the same as
requiring it.** That one's mine too.

## 8.4 On Joel's misattribution catch — verified, concur, no retro-edit

**Joel is right.** `grep -c "gzip vs zstd ordering" 001-user-request.md` → **0**. I read `001` in
full: the literal request is the one-liner, and its Interpretation lists *BREACH/CRIME-style attacks,
already-compressed payloads, streaming/SSE and long-lived responses, proxied backends that already
compress, CPU cost, and `Content-Length`/range-request interactions*. **Ordering is not there.** Don
quoted the coordinator's framing as the user's words.

This matters for the rule, not for the outcome:

- **GAP 1 (CRIME) rests on solid ground** — CRIME **is** named in `001`. Required, correctly.
- **GAP 2 was not user-asked** — but Raymond's section stands on the h3 scar, which I verified
  independently. **Keep it.** It answers the question this repo will actually ask.
- **No retro-edit.** Concur with Joel and Don. `_tasks/` is history.

And the irony is instructive: Don's own remedy is *"grep the durable artifact for the user's own
nouns"* — and he reconstructed the user's nouns from the framing he was handed instead of from `001`.
**The rule is right; he skipped it on the very input he was writing it about.** Aim it at `001`.

**So I ran it, on the full domain, rather than asserting coverage from memory** — which would have
been error #1 all over again:

```
BREACH 8   CRIME 2   already-compressed 1   streaming 8   double 1
CPU 1      Content-Length 1   range 2   per-hostname 1   Vary 3   zstd 7
```

**Every consideration named in `001` is answered in the durable record. Zero zeroes. The coverage
domain is closed.**

## 8.5 Verdict

**FULLY DONE. I concur with Joel; Don's two gaps are closed and his "NOT DONE" is satisfied.**

- Code: 8 production statements outside the CLI, three-term condition intact, **9/9 uncached**,
  `gofmt`/`vet`/tree clean, five non-goals absent. The code is the plan.
- My `009` §4 finding: closed, proven both directions, production untouched.
- Kevlin's `init()` correction: accurate, mechanism verified, conclusions survive, record corrected.
- CRIME: 0 → 2, answered first in the section and named in the heading.
- Ordering: its own section, resting on a verified in-repo scar.
- Coverage against `001`: complete, verified by grep, zero zeroes.

**Ship it.** Ward gets §8.3 — the absence rule and Joel's sentence — which is the only thing here that
generalizes beyond compression.

The one thing I'd want said about this task: **it found four real errors after the code was already
green, and every one was caught by a person who went and ran something instead of reading a report.**
Kent cleared his test cache. Rob removed the guard to get a panic. Raymond probed a TOML file and then
admitted what he couldn't verify. Kevlin grepped a function I had certified. **The two errors that
survived longest were mine and Don's — the two people who spent the most words telling everyone else
that being told is not evidence.** That is not ironic; it's the mechanism. Authority is the thing that
stops the search, and it stops it hardest in the person who has it. `caddy validate` remains the
maintainer's step on the Linux host, and **nobody on this task has run it.**
