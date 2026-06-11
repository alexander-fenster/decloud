# 005 — Joel's Tech Plan Revision (response to Linus review 004)

Linus returned **CHANGES REQUIRED** on `03-tech-plan.md`. Both findings are now fixed in
that file (committed `95049ac`). Summary of exactly what changed and why.

## Issue 1 (BLOCKING) — §7.2 rewritten: Raymond is tasked on `_docs/`, not excused

**Linus was right.** My prior §7.2 leaned on the decision record's note that "protocol
architecture lives in the decision record, not `_docs/`" and gave Raymond a documented out
to skip `_docs/`. I grepped and confirmed the operator docs actively document HTTP/3 as a
mobile benefit and will lie after this change. The "probably make no change" premise is
**withdrawn**. The verified hit set on this branch:

- `_docs/install.md:56` — the active lie. "mobile clients that negotiate HTTP/3 silently
  fall back and the symptom looks like 'TLS works but my phone is slow.'" → **required
  rewrite**: h3 disabled, only h1+h2 advertised, UDP/443 published-but-inert and now
  optional to open. Do not repeat the "open UDP/443 for mobile" reasoning.
- `_docs/install.md:41` — "`443/udp` (HTTP/3 over QUIC)" → **required parenthetical fix**:
  port stays published but is inert, HTTP/3 disabled.
- `_docs/usage.md:196` and `_docs/usage.md:320` — list `443/udp` as published; no h3 claim,
  so not lies, but **required one-clause inert qualifier** for consistency. Light touch.
- `README.md:61`, `README.md:124` — list `443/udp` as a host port; no h3 claim. **Grep-
  verify; default leave as-is** (port is still published); soften only if either reads as
  "open UDP/443 for HTTP/3." Raymond must confirm by grep, not assume.

Two guard rails written into every edit: (1) UDP/443 stays published — do NOT tell the
operator to stop publishing/opening it; frame it as "published but inert." (2) Do NOT
hallucinate a `caddy.protocols` config flag — none exists in M1/M2; the set is hardcoded.
Each subsection gives Raymond the current verbatim text and a binding-facts replacement so
there is nothing to invent. §6.4 honesty discipline ("don't write 'validated'") explicitly
carried into the doc edits.

## Issue 2 (non-blocking, addressed) — scoped the `h3` negative assertion

Replaced the whole-file `assert.NotContains(t, body, "h3")` (which false-positives on a
hostname like `h3.example.com`) with assertions scoped to the `protocols` directive:

- positive: `assert.Contains(t, body, "protocols h1 h2\n")` — pins the directive to end at
  `h2` before the newline, so any trailing protocol breaks it;
- negatives scoped to directive text: `NotContains "protocols h1 h2 h3"` and
  `NotContains "h1 h2 h3"`.

None can collide with a hostname. Updated §4.1 (test spec), §6.5 (gotcha now says "scope it,
don't rely on fixture discipline" instead of documenting the trap), and acceptance criterion
#2. The fixture-keep-clean comment approach is removed.

## Left intact (Linus-approved)

Generator-as-single-leverage-point; 4-space/8-space indentation with proof; emit-on-empty
= yes; exact-bytes output contract (§3); decision-record amendment (§7.1); UDP/443 left
published-but-inert; "no Docker here, byte-asserted not validated" honesty framing. Also
updated §9 (estimation) and §10 (acceptance criteria, now item 9) so the mandatory docs
obligation is reflected there too, not just in §7.

## Routing

Back to Linus for re-review of the PLAN, per workflow. If he approves, proceed to EXECUTION
(Kent → Rob → Raymond → Kevlin/Linus). Raymond's §7.2 obligations are the highest-risk part
of execution and Kevlin must hallucination-check the doc edits per CLAUDE.md.
