# Tabular CLI output: test the contract, not the bytes

When a CLI command renders rows via `text/tabwriter` (or any padding-aware writer), test that the **header field names appear**, that the **body rows appear in the expected order**, and that **each row's identity-field and state-field are present together** — but DO NOT assert on tabwriter's byte output (column widths, exact space counts, padding choices). Those bytes are stdlib output, not part of your operator contract; asserting on them is the textbook change-detector test CLAUDE.md bans.

## Live example

`internal/cli/lifecycle_commands_test.go` — `decloud status` no-arg tests. The helpers encode the contract one level below the test bodies:

- `headerFields()` — single source for the five column names.
- `assertHeaderPresent(t, stdout)` — substring-checks each header field without locking exact whitespace.
- `assertRowPresent(t, stdout, name, state)` — finds a body row by `(name, state)` via `strings.Fields` so the assertion is robust to tabwriter padding changes.
- `assertBodyRowOrder(t, stdout, names...)` — asserts row ordering by first-field, again ignoring whitespace.

A typical test reads:

```go
assertHeaderPresent(t, stdout)
assertRowPresent(t, stdout, "foo", "running")
assertRowPresent(t, stdout, "bar", "stopped")
assertBodyRowOrder(t, stdout, "bar", "foo")
```

What that locks: the columns exist, the rows exist, the order is right. What it does NOT lock: whether the CONTAINER column is 13 chars or 22, whether tabwriter pads with `' '` or `'\t'`, whether the trailing newline count matches today's exact value.

## Why this matters

Tabwriter's output bytes change based on the widest cell in any column. A new test fixture with a longer service name shifts every column. A regression test that asserts on byte-equal output of `decloud status` either breaks the moment fixtures grow OR forces the maintainer to add `\t` placeholders in expected strings that look nothing like real output. Both outcomes are worse than not having the assertion.

The exception is the single-row, fixed-width path: `runStatusOne`'s output is a `Fprintf` with a fixed format string. There, full-line `assert.Equal` IS the contract (the `Fprintf` template is the operator-visible promise). The tightening of `TestStatus_DelegatesToLifecycleAndPrintsResult` to full-line equality is the right call FOR THAT TEST — and is the wrong call for any tabwriter-using test.

## The carve-out: stderr-detail substring assertion

When a multi-row table is paired with a stderr companion line (`status: <name>: <detail>`), the stderr assertion is a substring contains-and-not-contains pair: `stderr` contains both the service name and the detail substring, `stdout` does NOT contain the detail substring. The not-contains row locks the five-column shape against accidental sixth-column drift — that's a real contract surface, unlike column widths.

See `TestStatus_NoArgs_RowErrorDetailRoutesToStderrButNotStdout`.

## Where this fits with `cli-flag-surface-coherence.md`

That doc names four surfaces (runtime, error message, --help, _docs). This one names what to test on surface 1 (runtime stdout) for tabular outputs specifically. Surface-3 (--help) tests follow the semantic-token rule (banned for prose, allowed for milestone/version tokens). Surface-4 (_docs example blocks) follows `doc-examples-verified-not-typed.md` — example blocks ARE byte-precise, but they're generated, not hand-typed.

## When to apply

Any CLI command that emits two-or-more rows via `text/tabwriter` (or any other column-aligning writer). The five-column status table is the first such site; future commands (`decloud caddy status`, `decloud deploys`, etc.) should reuse the same helper shape.

## Anti-pattern

```go
expected := "NAME    STATE    CONTAINER\n" +
    "foo     running  decloud-foo\n" +
    "bar     stopped  decloud-bar\n"
assert.Equal(t, expected, stdout.String())
```

Breaks the day a service called `mango-pickle-service` enters the test fixtures. Tells you nothing about whether the columns shifted because of a real bug or a fixture change.

## Originator

`_tasks/2026-05-13-status-list-all-services/{03-tech-plan.md §6.4, 005-kent-tests.md §2.3, 010-kevlin-review.md §"Tests are real tests, not change-detectors"}` — Joel's tech plan §6.4 carved tabwriter column widths out of the test contract; Kent built the helpers at the right abstraction level; Kevlin verified.
