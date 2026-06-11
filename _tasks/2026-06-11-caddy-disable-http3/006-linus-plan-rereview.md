# 006 — Linus's Plan RE-REVIEW: Disable Caddy HTTP/3

Re-review pass after Joel's revision (`005-joel-plan-revision.md`, tech plan committed
`95049ac`). I raised two findings in `004`. I verified both fixes against the revised
`03-tech-plan.md` AND spot-checked every prescribed correction against the actual bytes
of `_docs/install.md`, `_docs/usage.md`, and `README.md` on this branch. No taking Joel's
word for it.

## Verdict: APPROVED — proceed to EXECUTION

Both findings are properly resolved. The factual claims in the revised §7.2 are correct
against source. No new defects introduced. The engineering remains right (it was already
right in `004`).

---

## Issue 1 (BLOCKING) — RESOLVED

The old §7.2 "probably make no change, the decision record says docs live elsewhere"
out is **gone**. The revised §7.2 (tech plan lines 382–490) now tasks Raymond with
line-specific, verbatim-quoted corrections. I spot-checked each quoted line against the
real file:

- **`_docs/install.md:56`** — the active lie. Joel's verbatim quote ("Without UDP/443 the
  listener still works for HTTP/1.1 and HTTP/2, but mobile clients that negotiate HTTP/3
  silently fall back and the symptom looks like 'TLS works but my phone is slow.'")
  matches the file **exactly**. The prescribed replacement (lines 420–426) is accurate:
  it states h3 disabled, only h1+h2 advertised, UDP/443 published-but-inert, opening
  UDP/443 now optional/harmless, and references the decision record. Correct.

- **`_docs/install.md:41`** — Joel's verbatim quote ("dual-stack publishing on `80/tcp`,
  `443/tcp`, and `443/udp` (HTTP/3 over QUIC), bind-mounting…") matches the file exactly.
  The prescribed parenthetical fix keeps the port published and replaces the
  "(HTTP/3 over QUIC)" benefit framing with "published but inert — HTTP/3 disabled."
  Correct.

- **`_docs/usage.md:196`** — matches; lists `443/udp` publishing with no h3 claim. Joel
  correctly classifies it as not-a-lie and prescribes a one-clause inert qualifier, light
  touch. Correct call — it's about the published port, which stays.

- **`_docs/usage.md:320`** — matches verbatim ("it binds `80/tcp`, `443/tcp`, and
  `443/udp` on both `0.0.0.0` and `[::]`"). Same light-touch inert qualifier. Correct.

- **`README.md:61`, `README.md:124`** — both verified: they list `80/tcp`, `443/tcp`,
  `443/udp` as host ports and make **no** h3 claim. Joel's instruction — default leave
  as-is, grep-verify, soften only if they read as "open UDP/443 for HTTP/3" — is exactly
  right. Neither does, so no change is mandated. Don't remove the port from the list. Good.

**The two guard rails I demanded are present and correct (§7.2 lines 397–404):**
1. "The UDP/443 port stays published… Do NOT tell the operator to stop publishing or stop
   opening UDP/443… 'published but inert.'" — The prescribed replacement text for
   install.md:56 honors this: it says opening UDP/443 is "optional and harmless," NOT
   "stop publishing it." Confirmed it does NOT tell the operator to stop publishing
   UDP/443. PASS.
2. "Do NOT hallucinate a config flag. There is no `caddy.protocols` knob in M1/M2; the
   protocol set is hardcoded." — The prescribed text states the behavior as fixed, not
   configurable. The M3 `caddy.protocols` idea is correctly quarantined to the
   forward-looking decision-record note (§7.3), explicitly NOT to be built now and NOT in
   `_docs/`. Confirmed it does NOT invent a config flag in operator docs. PASS.

The honesty discipline ("nothing in `_docs/` may claim the Caddyfile is 'validated'",
§7.2.6 / §6.4) survived into the doc edits. Good — that was non-negotiable on a box with
no Docker.

This was the blocking defect. It is closed correctly, and the corrected prose is accurate
against the real files, not just plausible-sounding.

## Issue 2 (minor) — RESOLVED

The whole-file `assert.NotContains(t, body, "h3")` and its fixture-discipline comment are
**gone**. §4.1 (lines 206–218) and §6.5 (lines 335–344) now scope the negative to the
`protocols` directive:

- positive: `assert.Contains(t, body, "protocols h1 h2\n")` — pins the directive to
  terminate at `h2` before the newline, so any trailing protocol breaks it;
- negatives: `NotContains "protocols h1 h2 h3"` and `NotContains "h1 h2 h3"`.

None of these three match a bare `h3` substring, so a hostname like `h3.example.com`
cannot trip them. §6.5 now says "scope it, don't rely on fixture discipline" instead of
documenting the trap. Acceptance criterion #2 (§10) updated to match. This is robustness
by construction, exactly what I asked for. PASS.

The positive `"protocols h1 h2\n"` assertion is genuinely the better contract than the
negatives — it catches *any* trailing protocol, not just h3. Good instinct.

---

## Everything else: still right, untouched

The `004`-approved core stands: correct mechanism (`servers { protocols h1 h2 }`,
global-options-only, no per-site form), generator as single leverage point, emit-on-empty
= yes, UDP/443 left published-but-inert (no port-map churn, no `caddy up` recreate),
4-space/8-space indentation with byte-proof, exact-bytes output contract (§3),
decision-record amendment as load-bearing (§7.1), and the Alt-Svc reasoning (h3 disabled
⇒ no `Alt-Svc: h3` ⇒ UDP/443 truly inert). No regressions in the revision. §9 and §10
were correctly updated so the docs obligation is reflected in estimation and acceptance,
not just buried in §7.

---

## Note for EXECUTION (not a blocker — a watch item)

Raymond's §7.2 doc edits are the highest-risk part of this task, precisely because the
code diff is ~6 lines and the prose is where the lie lives. Two things must hold during
execution, and Kevlin must hallucination-check them per CLAUDE.md:

1. Raymond must actually GREP first (terms in §7.2 line 392) before editing — the plan
   says the hit set is exhaustive *as of this branch*, but execution may land after other
   commits. Confirm the hit set, don't assume it.
2. The replacement prose in §7.2.1 is prescribed as "facts binding, wording Raymond's."
   Kevlin: verify the shipped wording still carries all four binding facts (h3 disabled;
   only h1+h2; UDP/443 published-but-inert; opening it now optional) and introduces no new
   claim — especially no "validated," no config flag.

This is guidance for the EXECUTION reviewers, not a plan defect. The plan is sound.

---

## Summary

Issue 1 (blocking): RESOLVED — §7.2 now tasks Raymond with accurate, verbatim-anchored,
line-specific corrections; both guard rails (keep publishing UDP/443; no invented config
flag) are present and the prescribed text honors them; every quoted line matches the real
file. Issue 2 (minor): RESOLVED — h3 negative assertion scoped to the `protocols`
directive, cannot collide with a hostname.

**Verdict: APPROVED. Proceed to EXECUTION (Kent → Rob → Raymond → Kevlin/Linus).**

— Linus
