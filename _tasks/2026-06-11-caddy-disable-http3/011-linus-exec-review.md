# 011 — Linus's High-Level EXECUTION Review: Disable Caddy HTTP/3

High-level / architectural review of the shipped EXECUTION pass on
`task/caddy-disable-http3`. I read the whole task history (01 → 010), then reviewed the
actual diff (`git diff main...HEAD`) — code, tests, decision record, operator docs — and
re-ran the tests. I did NOT take the reports' word for anything; every claim below is
checked against the real bytes on this branch.

## Verdict: FULLY DONE — APPROVED, ship it.

This is what a four-line bug fix is supposed to look like when the team actually does the
hard part (the prose) instead of just the easy part (the code). The mechanism is correct,
the scope is correct, the docs no longer lie, and the decision record is fixed so engineer
#8 can't undo it by accident. No changes required.

I'll answer the four questions I was asked, because that's where the architectural risk
lived.

---

## 1. Did the implementation match the approved plan? Any drift, any cut corners?

No drift. The shipped diff is byte-for-byte what Joel's §3 exact-bytes contract specified
and what I approved in `006`:

```go
fmt.Fprintln(&buf)
fmt.Fprintln(&buf, "{")
fmt.Fprintln(&buf, "    servers {")
fmt.Fprintln(&buf, "        protocols h1 h2")
fmt.Fprintln(&buf, "    }")
fmt.Fprintln(&buf, "}")
```

- Inserted in the right place: after the two header comments, **before** the
  `for _, in := range inputs` loop — so the global block is the first block in the file.
  That ordering is not cosmetic; Caddy *rejects* a Caddyfile where global options follow a
  site block. The plan flagged it, the code honors it, and the test (`protoIdx < siteIdx`)
  guards it.
- 4-space / 8-space indentation, literal spaces inside Go string args — matches the
  existing `reverse_proxy` line, doesn't fight gofmt, won't mix tabs and spaces. This was
  Joel overriding a wrong instruction in the brief; the shipped code honors the override.
- No trailing blank line after the block — the loop's leading `Fprintln(&buf)` supplies the
  single separator. The double-blank trap I worried about in `006` was avoided.
- Scope held EXACTLY where I demanded it hold: `stub.go` untouched, `manager.go`
  `runOpts()` untouched (UDP/443 maps at lines 137–138 still published). Verified by
  `git diff --name-only` — neither file is in the change set.

No corners cut. The estimate (~6 emitted lines, one new test, one updated test, doc
reconciliation) is exactly what landed.

## 2. Is the change COMPLETE — does it actually solve the user's problem?

Yes, and the Alt-Svc reasoning I validated at plan time holds for the shipped code. The
chain end-to-end:

- Caddy advertises HTTP/3 only via the `Alt-Svc: h3=...` response header, emitted only when
  the h3 server is active.
- The generated Caddyfile now carries `{ servers { protocols h1 h2 } }`. With `h3` omitted
  from `protocols`, Caddy never opens the h3 listener and never sends `Alt-Svc: h3`.
- Therefore iPhone Safari is never told h3 exists, never races QUIC on UDP/443, and the
  still-published UDP/443 port forwards to a container port with no listener — packets are
  dropped. Genuinely inert.

So leaving UDP/443 published does NOT re-trigger the bug — nothing advertises it. This was
the one place the task could have been subtly wrong, and the shipped behavior is consistent
with the reasoning: h3 disabled ⇒ no Alt-Svc ⇒ UDP/443 dead.

The "always emit, even on empty registry" decision is the right invariant: the protocols
guarantee holds from the very first deploy, with no "first deploy still advertises h3"
window. The renamed empty-input test locks that contract. Good.

One honest caveat that is NOT a defect: no `caddy validate` and no real iPhone-over-QUIC
test ran here — no Docker on this box. The unit tests assert on emitted bytes only; that is
a proxy, not proof. The team did not pretend otherwise — Rob, Raymond, and Kevlin all
carry the "byte-asserted; pending operator `caddy validate`" framing, and no doc claims the
Caddyfile is "validated." That discipline is exactly what I required. The real
`caddy validate` + iPhone check remains the maintainer's manual step on the Linux host.
That is a deployment-time verification, not a code-review blocker.

## 3. Is the decision-record amendment honest, and does it prevent accidental re-enable?

Yes. This was the single most important non-code deliverable, and it's done right.

`_ai/decisions/caddy-runs-in-container.md` gets a dated **"Amendment 2026-06-10 — HTTP/3
disabled (line-17 premise field-disproven)"** subsection that:

- **Preserves** the original line-17 reasoning (never silently contradict a written
  Decision) and explicitly records that its premise was reversed by field experience.
- States the actual change (`{ servers { protocols h1 h2 } }`, no `Alt-Svc: h3`).
- States UDP/443 is **published but inert**, `runOpts()` unchanged, and that unpublishing it
  is a deferred change requiring `caddy up` recreate — so nobody is confused by an open port
  with no listener.
- Carries the explicit warning to the next engineer: **do NOT "fix a mobile regression" by
  turning HTTP/3 back on.** That is the landmine-defuser. The exact failure mode Don spent
  half his plan guarding against is now closed in prose.
- Quarantines the M3 `caddy.protocols` config idea as forward-looking only, not built, not a
  user-facing flag — keeping it out of operator docs.

This is the right way to amend a field-disproven Decision: dated, additive, honest, with a
direct instruction to the next person not to relitigate it.

## 4. Over- / under-engineered? Did Raymond's docs stay truthful (no scope creep, no invented flags)?

**Not over-engineered.** No config flag (correctly deferred to M3), no port-map churn, no
stub change, no gold-plating. The change is the minimum that solves the problem.

**Not under-engineered.** The doc reconciliation — the real risk — was treated as
load-bearing, not skipped. This was my blocking finding in `004` (the plan originally gave
Raymond an out to skip `_docs/`); it was fixed in the plan and is now fully executed:

- `_docs/install.md:56` — the active "my phone is slow ⇒ open UDP/443" lie is **gone**,
  replaced with "HTTP/3 disabled, only h1+h2 advertised, UDP/443 published-but-inert,
  opening it is optional and harmless." Correct, and it does NOT tell the operator to stop
  publishing the port.
- `_docs/install.md:41` — "(HTTP/3 over QUIC)" benefit framing corrected to
  "published but inert — HTTP/3 disabled."
- `_docs/usage.md:196`, `:320` — one-clause inert qualifiers, light touch, port list intact.
- `README.md:61, :124` — grep-verified, left as-is (they list the port, make no h3 claim;
  the port really is still published). Correct call — not gratuitously churned.

Truthfulness checks all pass:
- **No invented config flag.** `caddy.protocols` appears only in the decision record and
  `_ai/apidocs.md`, both labelling it an M3 forward-looking idea, NOT a shipped flag.
  Operator docs state the protocol set as fixed/hardcoded. Correct.
- **No false "validated" claim** anywhere in `_docs/`.
- **The quoted `servers { protocols h1 h2 }`** in install.md:56 matches the emitted bytes
  exactly — no hallucination.

The new `_ai/apidocs.md` is a small doc-writer notes file (no scope creep into the
codebase) that records the plain-Markdown-not-Next.js fact and the honesty discipline. Fine.

Kevlin's low-level pass (`010`) already byte-dumped the output (including the
`h3.example.com` hostname-collision case to prove the scoped negative assertions don't
false-positive) and approved. I concur — and my high-level concern (does this actually kill
the advertisement, completely, on every reload path) is satisfied by the
generator-as-single-funnel architecture: `regenerateAndReload` routes deploy, unregister,
and `caddy reload` all through the generator's output. Fix the generator, fix every path.

---

## Tests: do they test the right thing?

Yes. These are contract tests, not change-detectors:

- `protocols h1 h2\n` positive assertion pins the directive to terminate at `h2` before the
  newline — catches *any* trailing protocol, not just h3. Strongest single assertion, and
  the right one.
- h3 negatives scoped to the directive text (`protocols h1 h2 h3`, `h1 h2 h3`), NOT a
  whole-file `NotContains "h3"`. The hostname-collision trap I flagged in `004` is closed by
  construction — Kevlin verified it with a `h3.example.com` fixture.
- Ordering assertion (`protoIdx < siteIdx`) guards the Caddy must-be-first rule we can't
  check with a real `caddy validate` here.
- The three untouched site-block tests still pass, confirming the prepend didn't disturb
  existing output.

`go test ./internal/caddy/...` → ok. `gofmt -l` clean. `go build ./...` ok.

---

## Summary

The engineering was right at plan time and the execution honored it without drift. The
hard part of this task was never the four lines of emitted text — it was the prose: a
decision record that argued the opposite of the user's lived experience, and operator docs
that repeated the field-disproven "open UDP/443 for mobile" reasoning. Both are now fixed,
honestly, with history preserved and a direct warning to the next engineer. No invented
flags, no false "validated" claims, UDP/443 correctly described as published-but-inert with
no instruction to stop publishing it.

The only thing that has NOT happened — and cannot happen on this box — is the real
`caddy validate` + iPhone-Safari-over-QUIC check on the Linux host. That is the maintainer's
manual deployment-time step, correctly flagged, never falsely claimed. It is not a
code-review blocker.

**Verdict: FULLY DONE from a high-level standpoint. APPROVED.** Proceed to PLAN
(Don/Joel/Linus close-out) and then finalization (Ward knowledge capture, squash-merge).

— Linus
