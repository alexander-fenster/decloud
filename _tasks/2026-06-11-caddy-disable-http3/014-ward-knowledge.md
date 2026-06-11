# 014 — Ward's Knowledge Capture: Disable Caddy HTTP/3

Finalization knowledge pass. Goal: preserve durable, reusable, non-obvious learnings so a future agent editing the Caddyfile generator doesn't relearn them — without duplicating what Raymond already wrote during EXECUTION.

## What was already captured (reviewed, not duplicated)

- `_ai/decisions/caddy-runs-in-container.md` — Amendment 2026-06-10 records the **why**: line-17 mobile-benefit premise was field-disproven (iPhone Safari + broken QUIC = hang, not slow), HTTP/3 now disabled at the Caddyfile level, UDP/443 stays published-but-inert, "do NOT re-enable," M3 config-knob forward note. Comprehensive — left as-is.
- `_ai/apidocs.md` — Raymond's doc-writing notes: `_docs/` is plain Markdown (no build step), where operator port/protocol facts live, amend-don't-delete a field-disproven Decision, and the no-Docker "byte-asserted not validated" caveat.

## What I added (the missing *how-it-works* layer)

The decision record covers the architectural *why*. The reusable *engineering facts* a future generator-editor needs had no clean home, so I created one focused file rather than bloating the decision:

**NEW: `_ai/caddyfile-generator-facts.md`** — captures:
- Protocol selection is **global-options-only** — there is NO per-site `protocols` directive; it's an allow-list (`h1 h2`), omitting `h3` disables it. Points at `generator.go:Generate`.
- The **global block must be first** or Caddy rejects the file; `TestGenerator_DisablesHTTP3` locks ordering via `protoIdx < siteIdx`.
- `Alt-Svc: h3` is **gated on the active h3 listener**, not on config text — so the config-only `caddy reload` change fully stops QUIC negotiation even though UDP/443 stays published (inert, harmless).
- The generated Caddyfile is indented with **4-space/8-space spaces, NOT tabs** — a concrete editing trap; pinned literally by the test's `Contains "\n    servers {\n"` assertions.
- No-Docker dev box → byte-asserted, not validated.

**UPDATED: `_ai/decisions/caddy-runs-in-container.md`** — added one "How-it-works companion" line at the end of the amendment cross-linking to the new facts file (bidirectional ref; keeps mechanics out of the decision record).

## Deliberately NOT captured

- Task-completion narrative, test names beyond the one load-bearing ordering assertion, and the literal emitted bytes — those live in the git diff, `008-rob-impl.md`, and `007-kent-tests.md`. No value re-stating them in the durable library.
- A separate "indentation" file — folded into the generator-facts file where the editor will already be looking.

## Files touched
- `_ai/caddyfile-generator-facts.md` (new)
- `_ai/decisions/caddy-runs-in-container.md` (one cross-link line)
