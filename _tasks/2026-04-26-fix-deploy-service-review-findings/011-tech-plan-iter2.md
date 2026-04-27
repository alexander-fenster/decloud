# Joel's tech plan, iteration 2

Don's plan in `010-plan-iter2.md` is technically sound. I verified every
load-bearing claim against the working tree before signing off, because
"trust but verify" is what tech plans are for. This is a one-line fix;
the tech plan is correspondingly short.

**Approval: CONFIRMED. No flags raised. Ship it.**

---

## Verification of Don's claims

I checked each of Don's three claims directly against the codebase. All
three hold.

### Claim 1 — "the exact line is `internal/cli/deploy_service.go:55`"

CONFIRMED. The current file content at line 55 is exactly:

```
cmd.Flags().IntVar(&f.Port, "port", 0, "container listen port (required if --host set)")
```

This is the only occurrence of the string `"container listen port"` (or
`"required if --host set"`) anywhere under
`/Users/fenster/dev/decloud/internal/`,
`/Users/fenster/dev/decloud/cmd/`,
`/Users/fenster/dev/decloud/_docs/`, or
`/Users/fenster/dev/decloud/_ai/`. Old occurrences in
`_tasks/2026-04-26-m1-implementation/{03-tech-plan.md,06-tech-plan-v2.md}`
are historical task records — those are immutable and correctly should
not be touched. The new task's review files (`08-kevlin-review.md`,
`09-linus-review-impl.md`, `010-plan-iter2.md`) reference both the old
and new strings as part of their analysis; that is correct.

One file. One line. One occurrence. Single-source-of-truth holds.

### Claim 2 — change to `"container listen port (required)"`

CONFIRMED as the right call.

Three surfaces must tell the same story to the operator:
1. Runtime check: `internal/cli/deploy_service.go:73-75` —
   `if f.Port == 0 { return fmt.Errorf("--port is required: %w", errUsage) }`
2. Manual: `_docs/usage.md:59` — `Required: yes`, error
   `--port is required`, exit code 2.
3. CLI `--help`: this line.

After the change, all three say "required, full stop." Before the change,
surface 3 disagreed with surfaces 1 and 2. That is the precise bug class
Findings 1 and 2 fixed; we cannot ship a fresh instance of it.

Linus offered the alternative `"container listen port"` (no parenthetical
at all) as also acceptable. Don picked `"container listen port (required)"`.
I concur with Don's choice for two reasons:
- Consistency: line 53 (`--name`) already says `"service name (required, [a-z][a-z0-9-]{0,38})"`
  with `(required, ...)`. Mirroring that pattern keeps the help output
  uniform.
- Discoverability: an operator scanning `--help` sees `(required)` in the
  same column position as `--name`. A user pattern-matches faster against
  consistent syntax than against minimal syntax.

### Claim 3 — no tests need updating

CONFIRMED. The help string is a Cobra runtime artifact in
`cmd.Flags().IntVar(...)`. No test in `internal/cli/` (or anywhere in
the tree) asserts on it. I verified by inspecting the test files Kent
added and Rob updated; none reference `"container listen port"` or
`"required if --host set"`. The behavioral surface that *is* asserted —
`f.Port == 0` triggers `errUsage` and `ExitUsageError` — already has
two regression tests
(`TestDeployService_NoPortReturnsExitUsageError`,
`TestDeployService_PortZeroExplicitReturnsExitUsageError`).

A test for the help string would be a textbook change-detector test,
which CLAUDE.md explicitly bans. Correctly absent.

### Claim 4 — no other docs need updating

CONFIRMED. `_docs/usage.md:59` already documents `--port` as
`Required: yes` with the `--port is required` error and exit code 2.
Raymond's iteration-1 work already covers it. `_ai/cobra-init-pattern.md`
and `_ai/m1x-backlog.md` discuss `Init`/logging changes, not the
`--port` flag. Nothing else mentions the flag's required-ness.

---

## Implementation handbook

Trivial. Recording for completeness because that is what tech plans do.

### Exact change

File: `/Users/fenster/dev/decloud/internal/cli/deploy_service.go`
Line: 55

Replace:
```go
cmd.Flags().IntVar(&f.Port, "port", 0, "container listen port (required if --host set)")
```

With:
```go
cmd.Flags().IntVar(&f.Port, "port", 0, "container listen port (required)")
```

That is the entire diff. Lines 1-54 and 56-end are unchanged.

### Verification commands (Rob runs all three)

- `go test ./... -count=1` — must remain green tree-wide.
- `gofmt -l ./internal ./cmd` — must remain empty.
- `go vet ./...` — must remain empty.

Test results should be byte-identical to iteration 1's run, since no
test reads the help string. If any of the three commands changes color,
stop and surface — something else regressed independently.

---

## Gotchas and landmines

None. This is a string literal in a flag-registration call. No control
flow, no whitespace games, no Unicode subtleties (all ASCII, no smart
quotes), no escape sequences, no continuation lines.

The only failure mode I can construct: someone running the change in an
editor with a "smart quotes" feature enabled silently substitutes `"` →
`"` / `"`, which would still compile in Go (the bytes happen to be valid
UTF-8) but the help string would render with curly quotes. `gofmt` would
not catch this. Mitigation: Rob's editor is configured for Go; the diff
is reviewed; CI's `gofmt` check is unaffected by this class of bug but
the diff itself is small enough that human review catches it. Joel-Test
result: not a real risk for this team.

---

## Simplification opportunities

None applicable to a one-line help-string fix.

The only structural simplification I considered was Linus's alternative
`"container listen port"` (no parenthetical). It is shorter. It is also
inconsistent with the `--name` flag's help on line 53. Don's choice
preserves consistency at the cost of one word; the right call.

---

## Acceptance for iteration 2

Mirrors Don's six criteria. I add no new criteria; the plan is complete.

1. `internal/cli/deploy_service.go:55` reads `"container listen port (required)"`.
2. `go test ./... -count=1` green.
3. `gofmt -l ./internal ./cmd` empty.
4. `go vet ./...` empty.
5. Diff vs iteration 1 is exactly one line, in `deploy_service.go:55`.
6. Don/Joel/Linus all sign off FULLY DONE.

---

## Summary

Don's plan is correct on all four points the user asked me to verify:
- Line is `internal/cli/deploy_service.go:55` — verified.
- New help string is `"container listen port (required)"` — verified
  as the right wording (consistent with `--name`'s help).
- No tests need updating — verified; help string isn't asserted on.
- No other docs need updating — verified; `_docs/usage.md` already
  carries the new contract from Raymond's iteration-1 work.

One line. Ship it.

— Joel
