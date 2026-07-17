# 004 — Linus's Plan Review: HTTP compression in the generated Caddyfile

> Status: PLAN step 3. Review of `002-don-plan.md` + `003-joel-tech-plan.md`. No code exists yet.

## VERDICT: **APPROVE** — architecture stands. Ship it after the doc-level fixes in §3–§6.

The design is right: `encode zstd gzip` in every site block, per-service `disable_compression`
opt-out, zero value = ON. **I re-verified every load-bearing claim myself and found no
architectural defect.** The issues below are documentation, one wrong fact handed to Kent, and one
decision for Don. None of them move the design.

Let me say the unusual part first, because it doesn't happen often: **both of you actually verified
your facts at source instead of reciting them from memory, and it shows.** Don's §3.4 is a genuinely
non-obvious finding that most people would have gotten wrong (they'd have reached for `match` and
shipped a broken SSE experience believing it was fixed). Joel didn't take it on trust and re-derived
it. That is how this is supposed to work. Credit where it's due.

---

## 1. Independent verification — I checked, I did not take your word for it

Fetched current `master` `encode.go` (602 lines, matches Don's count) and queried the issue tracker.

| Claim | Source | Verdict |
| --- | --- | --- |
| Wrapper installed on **request** `Accept-Encoding` alone | `encode.go:162-168` — `if isEncodeAllowed(r.Header) { for _, encName := range AcceptedEncodings(r, enc.Prefer) { ... w = enc.openResponseWriter(...) } }` | **CONFIRMED.** No response state is reachable here. It doesn't exist yet. |
| `Match` only reached from `init()`, only reached from `Write` | `encode.go:449-458` — `rw.config.Match(rw)` inside `init()` | **CONFIRMED.** |
| Pre-body `Flush()` swallowed | `encode.go:302-308` — `if !rw.wroteHeader { return nil }` | **CONFIRMED.** |
| `WriteHeader` does **not** set `wroteHeader` for normal responses | `encode.go:259-286` — `rw.wroteHeader = true` is set **only** in the CONNECT-2xx branch (`:276-279`) | **CONFIRMED — and this is what makes the bug real.** A backend doing `WriteHeader(200); Flush()` gets nothing through. |
| Small first write ⇒ `init()` never called ⇒ body uncompressed | `encode.go:337-350` — `if gtMinLength { rw.init() }` | **CONFIRMED.** Don's §3.3 severity analysis is exactly right. |
| `defaultMinLength = 512` | `encode.go:594-595` | **CONFIRMED.** |
| Bare `encode` ⇒ `Prefer` = enabled ∩ `[zstd, br, gzip]` | `encode.go:130-136` | **CONFIRMED.** `encode zstd gzip` is equivalent-but-explicit. Fine. |
| Caddy #6293 **open** | `gh api` → `state: "open"`, `created: 2024-05-02`, `updated: 2026-03-17`, `closed_at: null` | **CONFIRMED.** Not a stale ticket. |
| `generator.go:35,41-45,46-53,57-84`; `types.go:9-26,25,66,69-71` | read directly | **CONFIRMED.** All accurate. |
| Strict decoder | `store.go:250-251` — `dec.DisallowUnknownFields()` | **CONFIRMED.** Joel's catch is real. |
| `caddy validate` in prod | `deploy/service.go:413` — `Reloader.Validate` before rename | **CONFIRMED.** Joel's correction is right. |

**§3.4 is confirmed at source. `match` cannot save SSE. Nobody re-litigates this.**

One thing I'll add that sharpens Don's framing: because the swallow at `:302` fires before
Content-Type is ever consulted, **this is not an SSE bug — it is a headers-then-idle bug.** SSE is
just its most common shape. Long-poll, chunked progress streams, and anything that writes a status
and waits are all in the blast radius. The knob's scope is correctly drawn anyway, but the decision
record should say "streaming" not "SSE" so nobody thinks `Content-Type: text/event-stream` is what
selects the failure.

---

## 2. Q1: Is the knob justified, or is it fear? — **JUSTIFIED. I argued the other side; it loses.**

You asked me to argue the other side properly. Here it is, at full strength, because it deserves it:

> Decloud is a single-host PaaS for personal projects. **Every config field is forever** — you can
> add one in an afternoon and you will carry it for a decade, in the docs, in the tests, in every
> future refactor of `ServiceConfig`, and in the head of every person who reads the TOML and asks
> "do I need this?". The justification here isn't even a Decloud bug — it's a *third party's*
> unfixed defect, which means we are permanently encoding someone else's mistake into our public
> config surface. No user has hit this. No user has asked for it. Nobody has deployed an SSE service
> on Decloud and complained. This is a knob added because a smart person read an issue tracker and
> got nervous. YAGNI. Ship compression globally, no field, and if somebody actually hits it, add the
> field *then* — with a real bug report to point at, which is worth more than a hypothetical.

That is a *good* argument. In most reviews I'd accept it and kill the knob. It loses here, on four
specific grounds:

1. **There is no escape hatch. At all.** The generated file says `do not edit by hand` and is
   atomically overwritten (`generator.go:54`, `service.go:410-419`) on every deploy *and* every
   `decloud caddy reload`. This is self-hosted — the user *is* the operator, and they still can't
   fix it. Without the knob, our answer to a broken SSE app is "wait for our next release." That is
   shipping shit, and Don is right to say so in those words.
2. **The failure is silent and misattributed.** Not a 500. Not a failed deploy. The app builds,
   deploys, passes readiness, looks healthy — and `EventSource.onopen` doesn't fire until the first
   event. The user will blame their app, their browser, their network, and their own code. They will
   *not* suspect the reverse proxy they never configured. The chance a bug report ever reaches us
   correctly diagnosed is close to zero — which destroys the "wait for a real report" strategy the
   anti-knob argument depends on.
3. **SSE is not exotic in 2026.** It's the default transport for LLM token streaming. "Personal
   projects on a small PaaS" in 2026 means *exactly* the workload that breaks.
4. **Upstream has not fixed it in two years** (verified: open, updated 2026-03-17). Waiting is not a
   strategy.

**The rule I'm applying, and I want it written down because it's reusable:** a knob earns permanent
config surface when the default has a known failure that is **(i) silent, (ii) misattributed, and
(iii) unworkaroundable**. This is 3-for-3. If the failure were *loud* — a failed deploy, a 502, an
obvious error — I would kill the knob and tell you to wait for a bug report. It isn't. The knob
stays.

That reasoning belongs in the decision record, not just in this review. It's the thing that tells
future-us whether the *next* proposed knob is legitimate or cowardice.

---

## 3. Q2: Is global-default-ON right, and did Don answer "is it safe"? — **YES, with one overstatement**

Don answered it. The BREACH analysis is not hand-waving — the preconditions are stated correctly
(compression + secret in body + attacker-controlled reflection into the *same* response + ability to
drive many cookie-bearing cross-origin requests and observe sizes), and the CRIME dismissal is
correct and worth making: CRIME was TLS-level compression, it's a different layer, and citing it here
is cargo-cult.

The load-bearing argument is the third one, and Don buried it under two weaker ones:

> **The knob is useless against BREACH.** A per-host toggle only helps if the operator knows their
> app reflects attacker input beside a secret — and if they knew that, they'd fix the reflection.

That argument alone decides it. It's airtight, and it's independent of how scary you think BREACH is.
The "whole industry ships it" point is an appeal to authority — true, but it's supporting evidence,
not the reason. The reason is that the mitigation is app-side and a reverse proxy cannot fix it.

**The one overstatement, which must not reach the decision record:**

> "`SameSite` cookies (Lax by default in every current browser, which **guts** condition 4)"

Too strong. `SameSite=Lax` **still sends cookies on top-level cross-site GET navigations.** It raises
the bar substantially; it does not gut the condition. The honest statement is: *BREACH remains
theoretically live for applications that reflect attacker-controlled input next to a secret in a
compressed response; modern cookie defaults raise the bar considerably; the mitigation is and always
was app-side.* Same conclusion, defensible wording. Don't put a claim in a permanent decision record
that a security-literate reader can knock down in one sentence — it discredits the correct parts
around it.

**Two safety facts neither of you mentioned. One matters for the record:**

- **`Vary: Accept-Encoding` is handled.** `encode.go:463-464` adds it in `init()`; `:269-270` covers
  the 304 case. This is the *first* question a competent reviewer asks about enabling compression
  behind any cache ("will an intermediary serve gzip to a client that didn't ask?"), and neither of
  you addressed it. The answer is "upstream handles it correctly" — so **no action, no code** — but
  it belongs as one line in the decision record. Unwritten answers get re-derived, and re-derived
  answers get re-derived *wrong*.
- **SSE is never actually compressed in practice anyway.** `init()` is only called from `Write` when
  the first write exceeds 512 bytes (`:337-350`), and it is never retried once `wroteHeader` is set.
  So a typical small-event stream runs with `rw.w == nil` — uncompressed, forever. **Consequence:
  `--no-compression` buys an SSE user zero bytes; it purely fixes header timing.** That is a strange
  shape for a flag named "disable compression," and if it isn't written down, someone will eventually
  "optimize" by removing the knob after measuring that SSE responses weren't compressed anyway. One
  line in the decision record. Don has the mechanism right in §3.3; I want it in the doc, not just
  the plan.

---

## 4. Q3: Polarity and naming — **RIGHT, and forever-proof. No change.**

- **Polarity is non-negotiable and both of you got it right.** Zero value = `false` = compression ON.
  `enable_compression` / `compression` would make the zero value mean "off" and would silently
  disable compression for every service TOML already on disk. That would be a real bug shipped as a
  naming preference. Correct as specified.
- **`disable_compression` beats the alternative neither of you considered: `streaming = true`.**
  Good — and I want the reasoning recorded, because `streaming` is the name someone will propose in
  six months. It's wrong: it would promise semantics we do not implement (we don't tune
  `flush_interval`, we don't touch `reverse_proxy`, we don't do anything else streaming-shaped). It
  would be a lie in the config file. `disable_compression` states what it mechanically does and stays
  coherent even if #6293 is fixed — at which point it merely loses its justification, which is
  precisely what the retirement condition captures. **Forever-proof. Keep it.**
- **CLI `--no-compression` ↔ TOML `disable_compression` asymmetry:** fine. Both are idiomatic in their
  own surface, both keep zero value = on. Joel's instruction to comment it at the `Request` mapping is
  right — that's a question that gets asked once and should be answered in the code.
- **Per-service, not per-host:** correct and well-argued. `Route{Hostname string}` (`types.go:69-71`)
  means every route of a service hits the same container and port. A per-hostname flag could only ever
  be set identically across a service's hostnames. Per-service is the only honest granularity.

---

## 5. Q4: Did Joel's corrections land? — **THREE LANDED. ONE IS BACKWARDS.**

**Landed, verified by me:**

1. **Strict decoder (§2).** `store.go:250-251` `DisallowUnknownFields()`. Real catch — Don cited
   `LastDeployedAt` as precedent without checking the decoder. The forward-downgrade hazard (new TOML
   + old binary = `ErrUnknownField` → exit 10) is real. **The ruling is correct: accept, document,
   don't engineer around it.** There is no supported downgrade path, and bumping `schema_version` for
   an optional additive key would be flatly wrong — that's what schema versions are *not* for.
2. **`caddy validate` runs in prod (§3).** `service.go:413`, before the atomic rename. Don's wording
   was genuinely misleading and would have left Rob with a wrong mental model of the system. Joel
   fixed the *fact* while **keeping the report discipline unchanged**. That's the right instinct and
   the right split — the discipline was never the problem, the reasoning was.
3. **Test-4 off-by-one (§6.2).** `strings.Count(body, "encode zstd gzip") == 1` only holds for
   single-hostname services. Correct catch, and exactly the sort of plausible-looking assertion that
   rots the day someone adds a hostname to a fixture.

**BACKWARDS — and this one's ironic, given both of you preached verification:**

Joel's §6.2 test 8 warning to Kent:

> "the global block is `{\n    servers {\n        protocols h1 h2\n    }\n}\n`, so the **first**
> `"\n}\n"` is the inner `servers` close, not the outer one. This is a genuine off-by-one trap"

**It is not. It's inverted.** I ran it against the real emitted bytes from `generator.go:41-45`:

```
body  = '{\n    servers {\n        protocols h1 h2\n    }\n}\n'
body.find('\n}\n')  -> 45
body[:45]           -> '{\n    servers {\n        protocols h1 h2\n    }'
```

The inner `servers` close is `\n    }\n` — newline, **four spaces**, brace. It **cannot** match
`"\n}\n"`. The first (and only) `"\n}\n"` **is the outer close**, and `body[:globalEnd]` is exactly
the global options block. **Joel's simple form works as written; his warning about it is wrong.**

Impact is bounded — his fallback advice ("slice to `example.com {` instead") lands somewhere safe
anyway — but this is a confidently-stated wrong fact handed directly to Kent, who will now either
avoid a correct assertion or burn time "verifying" a trap that doesn't exist. **Joel: strike the
warning.** Kent: the simple form is correct; use it.

**Did either of you miss anything architectural?** One thing, and it's §6 below.

**Joel's §6.5 addition is the best thing in his plan and I want it called out.** Every `Save`
expectation in `internal/deploy/service_test.go` uses `gomock.Any()` — so `Request.DisableCompression`
could be dropped on the floor at `service.go:317-343` and *every other test in the plan still
passes*. That's the actual seam where this feature breaks silently, it was untested, he found it
unprompted, and he specified **exactly one** test rather than retrofitting capture into all ten
expectations. That's the correct amount of discipline. Approved.

---

## 6. Q5: Scope — **RIGHT SIZE. One real gap, one doc trap.**

Scope is correct. ~5 production lines is the right size for this change, and Joel's "if Rob's diff is
materially bigger, something has gone wrong" heuristic is a good review tripwire. The non-goals list
is disciplined — no `match`, no `minimum_length`, no levels, no brotli, no auto-detection, no forked
encode module, no `reverse_proxy` changes. All correct. Don't let any of it creep back.

### Issue A: the escape hatch silently closes itself on every redeploy

**Problem.** Joel confirmed (§4, verified: `service.go:317-343` builds a *fresh* `ServiceConfig` from
`req`, never merges `prev`) that `DisableCompression` resets to `false` on any redeploy without the
flag. He defends this as consistent with `--mount` / `--strategy` / `--readiness-path`.

**The consistency claim is true. The defense is weaker than Joel thinks, and he didn't notice why.**
The *consequence class* is different:

- Forget `--mount` → immediate, obvious failure. App can't find its file. You know in seconds.
- Forget `--strategy` → visibly different deploy behavior. You can see it.
- Forget `--no-compression` → **app deploys clean, passes readiness, looks healthy, and then
  intermittently hangs on stream open for some clients.** Silent. Misattributed.

That is the *exact* failure class — silent, misattributed — that justified building the knob in §2.
So we're shipping an escape hatch that silently closes itself, restoring the precise bug it exists to
prevent, and the user's only warning is a doc line they read once six months ago. **That's not a
footgun; it's a foot-seeking gun.**

**Impact.** Correctness for the one workload this whole task exists to protect.

**Options:**

- **Option A (Minimal — as planned): accept + document.** Pros: matches the declarative-deploy house
  rule exactly, zero code, zero risk. Cons: the one flag where forgetting is invisible is protected
  only by documentation.
- **Option B (Sticky / merge from `prev`).** Pros: can't be forgotten. Cons: **breaks the declarative
  house rule for exactly one field.** Inventing per-field merge semantics is how config surfaces rot
  into "which of these flags are sticky?" — a question with no good answer. **I reject B.** Don't.
- **Option C (Keep the reset, make it loud).** Behavior stays declarative and identical — the flag
  still resets. But `deploy` emits a warning when it flips compression back ON for a service whose
  *previous* config had `disable_compression = true`:
  `note: compression re-enabled for <svc> (previous deploy used --no-compression); pass --no-compression to keep it off`.
  `prev` is **already loaded** on the deploy path (it exists for rollback, `restoreOldContainer` at
  `:379`), so the data is in hand. ~3 lines + one test. Pros: preserves the house rule perfectly,
  converts the one silent failure into a visible one, costs nothing. Cons: real scope growth on a
  ~5-line change; adjacent to Don's "no auto-detection" non-goal (though it isn't auto-detection —
  it's a warning on a state transition we already compute).

**My recommendation: Option C.** It's the cheapest possible conversion of a silent failure into a
loud one, and making this failure loud is the entire justification for the feature. If we're not
willing to spend 3 lines making the escape hatch's closure visible, we should question whether we
believed our own §2 argument.

**Option A is acceptable** if Don wants to hold the line on diff size — but *only* if the docs say it
in the strongest terms at **both** the flag row **and** the `decloud caddy reload` section, not one of
them.

**DON'S DECISION REQUIRED.**

### Issue B: the doc trap Joel is about to create

**Problem.** Joel instructs Raymond (§8) to note at `_docs/usage.md:150` that `disable_compression` is
a new legal hand-editable key. True — and I verified the surrounding text does invite hand-editing
("Edit the TOML by hand at your own risk"). But a subsequent `decloud deploy service` **silently
wipes a hand-set `disable_compression = true`** (fresh `ServiceConfig` from `req`, §4). So a user who
reads "you can hand-edit the TOML," sets the key, and later redeploys gets it reverted with no signal.

**Impact.** We'd be documenting a trap. This is exactly the shape of thing that becomes a support
thread.

**Options:**

- **Option A:** One sentence at `:150` — hand-setting `disable_compression` survives
  `decloud caddy reload` but **not** a redeploy; `--no-compression` is the durable way. Pros: one
  line, kills the trap. Cons: none.
- **Option B:** Don't mention the key at `:150` at all. Pros: no trap. Cons: dishonest by omission —
  the key *is* in the file and people will find it.

**My recommendation: Option A.** This isn't really a decision, it's a fix. Raymond does it.

### Nothing is missing structurally

I looked for a better mechanism and there isn't one. (For the record, so nobody proposes it later:
`request_header -Accept-Encoding` in the site block *would* also suppress the wrapper — `header` sorts
before `encode` in Caddy's directive order — but it's strictly more obscure than simply not emitting
`encode`. Omitting the directive is the correct, obvious answer. Don't get clever.)

---

## 7. Required changes before EXECUTION

Not architectural. None block the design. All are cheap.

1. **Joel: strike the §6.2 test-8 index warning.** It's inverted (§5). The simple form works;
   `body[:strings.Index(body, "\n}\n")]` is exactly the global block. Don't send Kent chasing a trap
   that doesn't exist.
2. **Decision record must not overstate SameSite** (§3). "Raises the bar considerably," not "guts."
3. **Decision record gains three lines** it currently lacks:
   - `Vary: Accept-Encoding` is handled upstream (`encode.go:463-464`, `:269-270`) — checked, safe.
     Records the answer to the first question any reviewer asks.
   - SSE is **never actually compressed** in practice (`:337-350` — small writes skip `init()`, never
     retried). The knob fixes *header timing*, not bytes. Prevents a future "optimization" that
     deletes the knob.
   - The knob-justification rule from §2: **silent + misattributed + unworkaroundable = a knob earns
     its place.** This is the reusable part. Write it down.
   - Say **"streaming"**, not "SSE" (§1). The bug is headers-then-idle; SSE is just its common shape.
4. **Raymond: the `:150` hand-edit caveat** (§6 Issue B).
5. **Don: decide Issue A** (§6) — warn-on-reset (C) vs document-only (A). B is rejected.

Everything else in both plans is approved as written: field placement and the long `why` comment
(§5.1), the `if` in the **inner** hostname loop, 4 spaces, `encode zstd gzip` explicit, no mock
regeneration, no loader validation, no new test helper, no lifecycle plumbing, Joel's test surface
1-13 including the §6.5 seam test, and the report discipline
(**"byte-asserted; pending operator `caddy validate`"** — never "validated"). I'll enforce that last
one at code review, same as Don.

## 8. Summary

The hard thinking is done and it's correct. Don found the non-obvious load-bearing fact (`match`
can't save SSE) and verified it at source; Joel didn't take it on faith, re-derived it, and caught
three real things Don missed. My independent verification confirms every load-bearing claim in both
documents. The knob is justified under a rule I can defend in the abstract, not just in this case.
The polarity is right. The scope is right.

Fix the inverted index warning, don't overstate SameSite, write down the three things that were
checked but not recorded, kill the hand-edit trap, and Don decides whether the escape hatch closes
loudly or silently.

**APPROVED. Proceed to EXECUTION once §7 items 1-4 are addressed and Don rules on §7 item 5.**
