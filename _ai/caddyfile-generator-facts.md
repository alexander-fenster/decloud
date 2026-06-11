# Caddyfile generator: protocol control and editing gotchas

Reusable facts for anyone editing the generated Caddyfile in `internal/caddy/generator.go:Generate`. The *why* of HTTP/3 being off is a Decision — see `_ai/decisions/caddy-runs-in-container.md` (Amendment 2026-06-10). This file is the *how it works* / *don't trip on this* companion.

## Protocol selection is global-options-only — there is no per-site knob

Caddy controls the served HTTP versions through the **global options block** (`{ servers { protocols ... } }`), not through any per-site directive. There is no `protocols` directive you can put inside a `host { ... }` site block. So the generator emits one global block at the very top of the file, before the first site:

```
{
    servers {
        protocols h1 h2
    }
}
```

Listing only `h1 h2` (omitting `h3`) is what disables HTTP/3 — it's an allow-list, not a deny-list. See `internal/caddy/generator.go:Generate` (emitted between the two header lines and the `for _, in := range inputs` site loop).

## The global block MUST be first

Caddy rejects a Caddyfile whose global options block is not the first block in the file. The generator's ordering (header comments → global block → site blocks) is load-bearing, and `internal/caddy/generator_test.go:TestGenerator_DisablesHTTP3` asserts `protoIdx < siteIdx` to lock it. Don't move it.

## `Alt-Svc: h3` is gated on the h3 listener, not on config text

Caddy only emits the `Alt-Svc: ...h3...` advertisement header when an HTTP/3 listener is actually active. Dropping `h3` from `protocols` stops the listener from starting, so the header is never sent and clients are never offered QUIC. This is why a config-only change (cheap `caddy reload`) fully stops HTTP/3 negotiation **even though** UDP/443 stays published on the host (`manager.go:runOpts()` is unchanged — the map points at a now-closed in-container port, which is harmless). Unpublishing UDP/443 is a separate container-recreate change, deliberately deferred; see the decision amendment.

## Generated Caddyfile is indented with SPACES, not tabs

The emitted strings use 4-space / 8-space indentation (`servers` at 4, `protocols h1 h2` at 8). This is the house style for the generated file and is NOT tabs. A test pins it literally — `Contains "\n    servers {\n"` and `Contains "\n        protocols h1 h2\n"` in `generator_test.go` — so a tab-vs-space slip fails CI. When you `fmt.Fprintln` new lines into the buffer, match the existing space count exactly.

## You cannot `caddy validate` on the dev box (no Docker)

These tests are pure byte-level string assertions on the generated body — a proxy, not proof. Real `caddy validate` + the iPhone-Safari/QUIC check run on the maintainer's Linux host. In reports, say "byte-asserted; pending operator `caddy validate`" — never "validated." (Same caveat captured for docs in `_ai/apidocs.md`.)
