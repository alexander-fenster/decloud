# 007 — Kent's Test Report: Disable Caddy HTTP/3

Test-first pass for the HTTP/3 disable feature. Tests written per the approved tech plan
(`03-tech-plan.md` §4 and §6) and Linus's re-review (`006-linus-plan-rereview.md`). No
production code touched — `internal/caddy/generator.go` is Rob's job.

## What I wrote

All edits in `internal/caddy/generator_test.go` (the test lives in the package under
test, not in `_tasks/`). Reused the existing helpers verbatim — `makeService`,
`generateToTemp`, `readFile` — no new helpers needed; the spec's assertions are simple
string contracts on the generated body and the existing helpers already cover setup.

### NEW: `TestGenerator_DisablesHTTP3` (the core regression test)

Generates one service (`makeService("foo", 8080, "foo.example.com")`), reads the body,
and asserts the global options block contract from §4.1:

- `Contains "servers {"` — wrapping block present.
- `Contains "protocols h1 h2\n"` — the positive contract: the directive terminates at
  `h2` immediately before the newline, so any trailing protocol (e.g. ` h3`) breaks it.
- `NotContains "protocols h1 h2 h3"` and `NotContains "h1 h2 h3"` — belt-and-suspenders
  negatives scoped to the directive text. Per §6.5 I deliberately did NOT use a whole-file
  `NotContains "h3"`; all three assertions match the literal `protocols`/`h1 h2` directive
  and cannot be tripped by an `h3`-containing hostname (e.g. `h3.example.com`).
- `Contains "\n    servers {\n"` and `Contains "\n        protocols h1 h2\n"` — locks the
  4-space / 8-space house style from §2 (NOT tabs), catching a future tab-vs-space
  regression.
- Ordering via `strings.Index` (mirroring `TestGenerator_MultiServiceMultiHostSorted`):
  `protoIdx < siteIdx` — the global block must precede the first site block (the Caddy
  "global block must be first" rule, §6.1). Guarded with `require.GreaterOrEqual(..., 0)`
  on both indices first.

### UPDATED + RENAMED: `TestGenerator_EmptyInputProducesHeaderAndGlobalBlock`

Was `TestGenerator_EmptyInputProducesHeaderOnly`. Renamed because the empty-registry
output is no longer header-only — it is header + global block (§5.1, emit-on-empty = YES).
Kept the two still-true assertions (`NotContains "reverse_proxy"`, `NotEmpty(TrimSpace)`)
and added `Contains "protocols h1 h2"`, which locks Open Question #1.

### Untouched (must still pass — §4.3)

`TestGenerator_OneServiceOneHost`, `TestGenerator_MultiServiceMultiHostSorted`,
`TestGenerator_DropsZeroHostnameServices` — assert only on site-block content/ordering,
which a prepended block does not disturb. Verified they pass unchanged.

## Observed failure (the right reason)

`go test ./internal/caddy/...` → the two new/updated tests FAIL on assertions, NOT on
compile errors:

- `TestGenerator_DisablesHTTP3` — `does not contain "servers {"`, `"protocols h1 h2\n"`,
  the two indentation strings, and `siteIdx`/`protoIdx` ordering fails (protoIdx = -1).
- `TestGenerator_EmptyInputProducesHeaderAndGlobalBlock` — `does not contain
  "protocols h1 h2"`.

This is exactly the missing-implementation failure: the generator does not yet emit the
global options block. The three unchanged site-block tests PASS, confirming the test edits
are correctly scoped. `gofmt -l` on the test file is clean.

## For Rob (implementation contract)

Emit, between the two header `Fprintln`s and the `for _, in := range inputs` loop, exactly
these bytes (§3): a leading blank line, then
```
{
    servers {
        protocols h1 h2
    }
}
```
(`{` at column 0, `servers` indented 4 spaces, `protocols h1 h2` indented 8 spaces,
closing `}`s.) Do NOT add a trailing separator after the block — the loop's existing
leading `fmt.Fprintln(&buf)` supplies the single blank line before the first site (§6.3).
Indentation is 4 spaces, NOT tabs (§2). The tests in `generator_test.go` are the arbiter.

Note: no `caddy validate` runs here (no Docker on this box). These are pure byte-level
string assertions — a proxy, not proof; the real `caddy validate` + iPhone-Safari check is
the maintainer's manual step on the Linux host.
