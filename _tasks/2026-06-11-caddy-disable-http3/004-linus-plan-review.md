# 004 — Linus's Plan Review: Disable Caddy HTTP/3

Reviewing Don's plan (`02-plan.md`) and Joel's tech plan (`03-tech-plan.md`) against the
actual source on this branch.

## Verdict: CHANGES REQUIRED (one real defect, one must-fix omission)

The core mechanism is RIGHT. The architecture is RIGHT. The leverage point is RIGHT.
But the plan contains one factually wrong claim about the docs that, if followed,
ships an operator-facing lie — the exact failure mode Don spent 50 lines warning about
for the *decision record*, while missing the same problem sitting in `_docs/`. That has
to be fixed before this plan is approved. Details below.

---

## What they got RIGHT (and it's most of it)

**1. The mechanism is correct and complete.** `servers { protocols h1 h2 }` in the
global options block is the documented, only way to disable HTTP/3 in a Caddyfile.
There is no per-site form. I scrutinized the obvious objection — "you disabled the
listener but the UDP port is still mapped, won't Safari still try QUIC?" — and the
plan's answer holds up. Here's the chain, because it's the load-bearing question and
nobody spelled it out end-to-end:

- A client only attempts HTTP/3 because Caddy advertises it via the `Alt-Svc: h3=...`
  response header. Safari caches that and races QUIC on the next connection.
- Caddy emits the `Alt-Svc: h3` header **only when the h3 server is active**. Disable
  h3 in `protocols` and the listener never opens AND the Alt-Svc header is never sent.
- Therefore the client is never told h3 exists, never tries UDP/443, and the
  still-published UDP/443 port forwards packets to a container port with no listener —
  they get dropped. Genuinely inert.

So disabling at the Caddy config level IS sufficient. Leaving UDP/443 mapped does NOT
re-trigger the iPhone problem, because nothing advertises it. The plan's "inert" claim
is correct, not hand-waving. Good. This was the one place this task could have been
subtly, embarrassingly wrong, and they nailed it.

**2. Leaving UDP/443 published is the correct M2 call.** Unpublishing requires a
container recreate (`caddy up`), not the cheap `caddy reload` this rides on. Bigger
blast radius, zero functional benefit for the stated bug. Deferring it is conservative
and right. Don't gold-plate this.

**3. The generator is the correct single leverage point.** `regenerateAndReload`
funnels all three reload entry points (deploy, unregister, `caddy reload`) through the
generator's output. Fix the generator, fix every path. The stub is `:80` plaintext
where h3 is physically impossible — correctly left alone. This is the right level of
abstraction; no spaghetti, no scattering.

**4. Emit-on-empty = yes.** Locking the global block into the empty-registry output so
the protocols guarantee holds the instant the first service appears, with no "first
deploy still advertises h3" window, is the right invariant. The empty-input test change
is a legitimate contract change, not a change-detector hack — the contract genuinely
moved. Correct.

**5. The indentation correction is RIGHT, and I verified it.** Joel overrode the brief.
The brief said "Caddyfile uses tabs by convention." Joel said the *existing generated
output* uses 4 spaces and that consistency-with-the-file's-own-style wins. I checked the
bytes: `generator.go:44` emits four U+0020 spaces before `reverse_proxy` (the `.go`
source line is tab-indented per Go convention, but the string literal contents are
spaces). `stub.go:11` likewise. Mixing tabs into the global block would produce a file
that mixes tabs and spaces — ugly and fragile under any `caddy fmt` the operator runs.
4-space house style is correct. This is exactly the kind of "the brief is wrong, here's
proof" call I want to see. Joel did the forensics instead of obeying. Approved.

**6. The decision-record amendment obligation is correctly identified and load-bearing.**
`caddy-runs-in-container.md:17` literally frames HTTP/3 as a mobile *benefit* ("without
it... my phone is slow"). Shipping the reversal without amending that record plants a
landmine for engineer #8. Don is right that the doc amendment is as load-bearing as the
code. The "preserve the original reasoning, add a dated follow-up, never silently
contradict" instruction is exactly correct.

---

## CHANGES REQUIRED

### Issue 1 — BLOCKING: the plan's `_docs/` claim is factually wrong; operator docs will lie

**Problem.** Joel §7.2 says, citing the decision record: "If neither mentions it (likely
— `caddy-runs-in-container.md:60-62` explicitly says protocol architecture lives in the
decision record, not `_docs/`), make **no** change." That premise is FALSE. I grepped.
The operator-facing docs explicitly document HTTP/3 and repeat the exact "my phone is
slow" reasoning this task is reversing:

- `_docs/install.md:41` — "`443/udp` (HTTP/3 over QUIC)"
- `_docs/install.md:56` — "Without UDP/443 the listener still works for HTTP/1.1 and
  HTTP/2, but mobile clients that negotiate HTTP/3 silently fall back and the symptom
  looks like 'TLS works but my phone is slow.'"
- `_docs/usage.md:196`, `:320` and `_docs/cli/caddy_up.go:18` — describe `443/udp`
  publishing (these are about the *port publish*, which stays, so they're arguably still
  accurate; but `install.md:56` is now actively misleading).

**Impact.** `install.md:56` tells the operator that HTTP/3 negotiation helps mobile and
implies they should open UDP/443 for it. After this change, Decloud no longer advertises
h3 at all — that sentence is now false and points the operator at exactly the behavior
the task exists to kill. An operator reading it will be confused about why UDP/443 is
open but h3 is gone, or worse, will "fix" the discrepancy. This is the identical
failure mode Don spent half his plan guarding against for the decision record — and the
plan walks right past the same disease in `_docs/` because it trusted the decision
record's "why this isn't in `_docs/`" note instead of grepping.

**Options.**
- **Option A (Minimal):** Amend `_docs/install.md:56` to state h3 is now disabled
  (advertise h1/h2 only), and that UDP/443 stays open-but-inert. Touch `usage.md` port
  lines only if they assert h3 behavior (they mostly just list the published port — keep
  them). Pros: small, accurate, ships. Cons: none.
- **Option B (Thorough):** A + audit every `443/udp` / `HTTP/3` / `QUIC` mention across
  `_docs/` and `README.md` for the new behavior and reconcile each. Pros: complete.
  Cons: marginally more grepping; README already flagged historically for port
  punctuation (`_ai/review-bar-by-surface.md`) so it's worth the look.
- **Option C (Defer):** Ship code + decision-record amendment, leave `install.md:56`
  wrong. Pros: none real. Cons: ships an operator-facing contradiction; unacceptable
  under the project's own "never contradict a written claim" standard.

**My recommendation:** Option B. It's barely more work than A and `install.md:56` is a
genuine, active lie post-change. The plan's §7.2 instruction ("make no change if it
doesn't mention it") must be rewritten — it DOES mention it, in three files. Raymond
must be explicitly tasked to fix `install.md:56`, not given an out.

**DON'S DECISION REQUIRED:** Confirm Raymond is tasked to correct `_docs/install.md:56`
(at minimum) and re-grep `_docs/` + `README.md`. The plan currently gives Raymond a
documented excuse to skip this. Close that hole.

### Issue 2 — NON-BLOCKING but fix it: the `NotContains(body, "h3")` assertion is a future trap, and the plan already knows it

**Problem.** Joel §6.5 himself flags that `assert.NotContains(t, body, "h3")` will
false-positive the day a hostname legitimately contains `h3` (e.g. `h3.example.com`),
and "mitigates" it with a code comment telling Kent to keep fixtures `h3`-free. That's a
test that's correct only by fixture discipline — a tripwire for whoever adds a fixture in
two years.

**Impact.** Low now, annoying later. A brittle assertion that fails for the wrong reason
trains people to "just edit the test," which is how real regressions slip through.

**Options.**
- **Option A:** Keep the loose `NotContains "h3"` + the fixture-discipline comment (the
  plan's current position). Pros: simplest. Cons: latent false-positive.
- **Option B:** Assert on the *token* that actually matters: `assert.NotContains(t,
  body, "protocols h1 h2 h3")` is useless, but `assert.NotContains(t, body, " h3")` or
  better, assert the positive contract precisely — `protocols h1 h2\n` is present and
  the `protocols` line does NOT contain `h3`. Scope the negative assertion to the
  global-block line, not the whole file. Pros: robust against hostname collisions,
  asserts the real contract. Cons: two more lines of test.

**My recommendation:** Option B — scope the `h3` negative assertion to the `protocols`
directive line, not the whole file body. It's the same effort as writing the warning
comment and it actually can't false-positive. The plan's own gotcha §6.5 is the tell
that the loose assertion is wrong; fix the assertion instead of documenting the trap.

**DON'S DECISION REQUIRED:** Minor. Tell Kent to scope the negative h3 assertion to the
protocols line. Or accept the fixture-discipline approach if you genuinely don't care.
Not a blocker.

---

## Over/under-engineering check

- **Not over-engineered.** No config flag (correctly deferred to M3 — adding Viper
  surface now is scope creep and the plan says so). No touching the port maps. No
  touching the stub. The plan resists every temptation to gold-plate. Good.
- **Not under-engineered.** The doc-reconciliation obligation (decision record) is
  treated as load-bearing, not skipped. The "no Docker here, byte-asserted not
  validated, don't write 'validated' anywhere" honesty discipline is exactly right and
  must survive into Rob's and Raymond's reports.
- The estimate is honest: ~6 lines of emitted text, one new test, one updated test, doc
  reconciliation. The risk is entirely in the docs, not the code. Joel's §9 says this
  outright. Correct read.

---

## Summary

The engineering is right: correct mechanism, correct leverage point, correct decision to
leave UDP/443 inert, verified-correct indentation call, correct Alt-Svc reasoning (h3
disabled ⇒ no Alt-Svc ⇒ port truly dead). This is a well-traced plan.

The one real defect is that the plan tells Raymond he can probably skip `_docs/` —
and `_docs/install.md:56` is exactly the kind of field-disproven claim that, left in
place, recreates the bug-in-prose this whole task is built to eliminate. Fix the doc
obligation (Issue 1, blocking) and tighten the `h3` assertion (Issue 2, optional), and
this is ready to execute.

**Verdict: CHANGES REQUIRED — fix Issue 1 before EXECUTION. Re-route through Don/Joel to
update §7.2 so Raymond is tasked, not excused.**

— Linus
