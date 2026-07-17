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

## `encode` is site-level-only — the mirror image of `protocols`

The exact opposite of the section above, and the pair is worth holding in your head together: **there is no global-options `encode`.** `encode` is a site directive, so "compression globally" in Decloud does not mean one line in the global block — it means the generator emits `    encode zstd gzip` into **every site block**, once per hostname, from the site loop in `internal/caddy/generator.go:Generate`. Putting `encode` in the `{ servers { ... } }` block fails `caddy validate`. `TestGenerator_EncodeIsNotInGlobalOptionsBlock` guards that mistake specifically.

Emitted unless `GeneratorInput.DisableCompression` (carried from `registry.ServiceConfig.DisableCompression`, TOML `disable_compression`, CLI `--no-compression`). Absent/false = compression ON; that polarity is deliberate, so existing service TOMLs — which have no such key — gain compression rather than lose it.

## `match` CANNOT protect a streaming service — only omitting `encode` works

The one that gets re-derived wrong. Caddy installs its encoding `responseWriter` from the **request headers alone** (`encode.go` `ServeHTTP`, ~`:162-171`: the request's `Accept-Encoding`, gated by `isEncodeAllowed` on request `Cache-Control: no-transform`, plus a `Sec-WebSocket-Key` check) — *before* the response's Content-Type is knowable. The `match` sub-directive is only consulted later, inside `init()`, which is reached only from `Write` and `Close` — **never before the wrapper is installed**. So:

```
encode zstd gzip {
    match { ... exclude text/event-stream ... }   # ← does NOT fix the header-flush delay
}
```

`match` prevents *compression*, but the wrapper is already installed, and the wrapper swallows a pre-body `Flush()` (`encode.go` `FlushError()`: `if !rw.wroteHeader { return nil }`). A backend that writes headers and then idles before its first body byte has its flush dropped and the client waits. **The only fix available to us is not emitting `encode` in that site block at all** — which is what `--no-compression` does. See caddy#6293 (open) and `_ai/decisions/http-compression-on-by-default.md` for the full reasoning, including why the knob buys zero bytes.

This is a **headers-then-idle** bug, not an SSE bug: nothing about `Content-Type: text/event-stream` selects the failure. SSE is just its most common shape. Anyone who reaches for `match` here will ship a fix that does nothing.

## The global block MUST be first

Caddy rejects a Caddyfile whose global options block is not the first block in the file. The generator's ordering (header comments → global block → site blocks) is load-bearing, and `internal/caddy/generator_test.go:TestGenerator_DisablesHTTP3` asserts `protoIdx < siteIdx` to lock it. Don't move it.

## `Alt-Svc: h3` is gated on the h3 listener, not on config text

Caddy only emits the `Alt-Svc: ...h3...` advertisement header when an HTTP/3 listener is actually active. Dropping `h3` from `protocols` stops the listener from starting, so the header is never sent and clients are never offered QUIC. This is why a config-only change (cheap `caddy reload`) fully stops HTTP/3 negotiation **even though** UDP/443 stays published on the host (`manager.go:runOpts()` is unchanged — the map points at a now-closed in-container port, which is harmless). Unpublishing UDP/443 is a separate container-recreate change, deliberately deferred; see the decision amendment.

## Generated Caddyfile is indented with SPACES, not tabs

The emitted strings use 4-space / 8-space indentation (`servers` at 4, `protocols h1 h2` at 8). This is the house style for the generated file and is NOT tabs. A test pins it literally — `Contains "\n    servers {\n"` and `Contains "\n        protocols h1 h2\n"` in `generator_test.go` — so a tab-vs-space slip fails CI. When you `fmt.Fprintln` new lines into the buffer, match the existing space count exactly.

## You cannot `caddy validate` on the dev box (no Docker)

These tests are pure byte-level string assertions on the generated body — a proxy, not proof. Real `caddy validate` + the iPhone-Safari/QUIC check run on the maintainer's Linux host. In reports, say "byte-asserted; pending operator `caddy validate`" — never "validated." (Same caveat captured for docs in `_ai/apidocs.md`.)
