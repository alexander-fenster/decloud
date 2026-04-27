# CLI flags have four surfaces; all four must agree

A CLI flag's contract is told to operators in four places. When you change one, audit the other three or you ship CLI/docs drift — exactly the bug class the review-findings task was fixing.

The four surfaces:

1. **Runtime check** — the validation in `RunE`/`PreRunE` that rejects bad values.
2. **Error message** — the string the operator sees on rejection. Must name the flag and wrap the right sentinel for `ExitCodeFor`.
3. **`--help` text** — the third argument to `cmd.Flags().IntVar(...)` etc. Operators read this BEFORE the manual.
4. **`_docs/usage.md`** — the operator-facing reference table.

## How drift sneaks in

The fix-deploy-service-review-findings task tightened `--port` validation, updated the error message, and updated `_docs/usage.md` — but the `--help` text in `deploy_service.go:55` still said `(required if --host set)`. Both reviewers (Kevlin Issue 1, Linus Issue 1) blocked on this independently because shipping it would have introduced a fresh instance of the exact bug class the task was fixing.

The trap: surfaces 1 and 2 live next to each other in the `RunE` body, surface 4 is a Markdown table, but surface 3 is one line in flag declaration far from the validation. Easy to miss in a diff review.

## Discipline

When you change a flag's *required-ness*, *type*, *default*, or *meaning*, grep for the flag name across all four surfaces in one pass:

```
git grep -n -- '--<flag-name>'
git grep -n '"<flag-name>"' internal/cli/
```

The second grep catches the `cmd.Flags().XVar(..., "<flag-name>", ...)` declaration site (surface 3) which `--<flag-name>` won't.

## Why we don't test surface 3

A test that asserts on the help string is a textbook change-detector test (CLAUDE.md bans these). The mitigation is review discipline, not test enforcement. Surfaces 1, 2, 4 are testable: typed-sentinel assertions (`errors.Is(err, errUsage)`), exit code (`ExitCodeFor`), and doc-claim cross-referencing during code review.

## Pattern for help text on required flags

Mirror existing patterns. `--name`'s help reads `"service name (required, [a-z][a-z0-9-]{0,38})"`; `--port`'s now reads `"container listen port (required)"`. Operators pattern-match `(required)` in the same column position; uniform syntax beats minimal syntax.

## Originator

`_tasks/2026-04-26-fix-deploy-service-review-findings/{08-kevlin-review.md,09-linus-review-impl.md,010-plan-iter2.md}` — the iter2 one-line fix that closed the loose thread.
