# 008 — Rob's Implementation Report: Disable Caddy HTTP/3

Implemented the production change to make Kent's failing tests pass. One file touched:
`internal/caddy/generator.go`. Tests untouched.

## What I changed

`func (g *textGenerator) Generate`, between the two header `Fprintln`s and the
`for _, in := range inputs` loop, I inserted the global options block exactly per the
§3 exact-bytes contract:

```go
fmt.Fprintln(&buf, "# Re-run `decloud caddy reload` to refresh.")
fmt.Fprintln(&buf)
fmt.Fprintln(&buf, "{")
fmt.Fprintln(&buf, "    servers {")
fmt.Fprintln(&buf, "        protocols h1 h2")
fmt.Fprintln(&buf, "    }")
fmt.Fprintln(&buf, "}")
for _, in := range inputs {
```

This mirrors the existing `Fprintln` emission style already in the function (no raw
string literal, no `Fprintf`). Resulting emitted bytes:

- a single leading blank line separating the header comments from the block,
- `{` at column 0,
- `    servers {` (4 spaces),
- `        protocols h1 h2` (8 spaces),
- `    }` closing `servers` (4 spaces),
- `}` closing the global block (column 0).

### The double-blank-line trap (§6.3) — avoided

I did NOT append a trailing blank line after the global block. The per-service loop's
first statement is already `fmt.Fprintln(&buf)`, which supplies exactly one blank line
before the first `host {`. So the global block ends in a single `}\n` and the loop's
existing separator produces exactly one blank line between the block and the first site —
matching the spacing already used between sites.

### Indentation: 4/8 spaces, NOT tabs (§2)

The brief's "tabs by convention" note is wrong for this codebase; the binding instruction
is "match existing generated output," which uses 4 spaces (`generator.go` `reverse_proxy`
line, `stub.go`). The literal spaces live inside Go string arguments, so `gofmt` does not
touch them — no gofmt-vs-Caddyfile tension.

### Emitted unconditionally (§5.1)

The block is emitted before the loop and outside any `len(inputs)` guard, so it appears
even for an empty registry — satisfying `TestGenerator_EmptyInputProducesHeaderAndGlobalBlock`.

## Out of scope — confirmed untouched

- `internal/caddy/stub.go` — unchanged (§5.2; the `:80` plaintext stub can't serve h3).
- `manager.go` `runOpts()` UDP/443 port maps — unchanged (§5.3; published-but-inert).

## Test results

`go test ./internal/caddy/...` → `ok` (all pass):
- `TestGenerator_DisablesHTTP3` (new) — PASS, including the `servers {` presence, the
  `protocols h1 h2\n` positive contract, the two scoped `h3` negatives, the 4/8-space
  indentation assertions, and the `protoIdx < siteIdx` ordering (global block precedes
  first site).
- `TestGenerator_EmptyInputProducesHeaderAndGlobalBlock` (renamed/updated) — PASS.
- `TestGenerator_OneServiceOneHost`, `TestGenerator_MultiServiceMultiHostSorted`,
  `TestGenerator_DropsZeroHostnameServices` (unchanged) — PASS, confirming the prepend
  did not disturb site-block content or ordering.

`gofmt -l internal/caddy/generator.go` → clean (no output).
`go build ./...` → OK.

## Honesty note (§6.4)

These are pure byte-level string assertions — a proxy, not proof. No `caddy validate` ran
(no Docker on this box). The real `caddy validate` + iPhone-Safari-over-real-network check
is the maintainer's manual integration step on the Linux host. This change is
**byte-asserted; pending operator `caddy validate`** — not "validated" here.
