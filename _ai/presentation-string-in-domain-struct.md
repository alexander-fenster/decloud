# `ErrorDetail string` beats `error` for presentation-only error fields

When a domain struct needs to carry a diagnostic-string for a row/entry that the CLI will print as-is (and nothing in the system will `errors.Is` against it), declare the field as `string`, not `error`. Saves type juggling in tests, prevents accidental sentinel re-routing, makes serialisation trivial. Pair with a load-bearing doc comment naming the field as presentation-only.

## Live example

`internal/deploy/service.go` — `Status` struct:

```go
type Status struct {
    Name           string
    ContainerID    string
    ContainerName  string
    State          string
    LastDeployID   string
    LastDeployedAt time.Time
    // ErrorDetail carries the wrapped error message when StatusAll could
    // not produce a real row for this service (State == "error"). Empty
    // for the single-service Status() path and for non-error multi-row
    // entries. NOT rendered in stdout; the CLI prints it to stderr.
    ErrorDetail string
}
```

Populated only by `StatusAll`'s synthesis branch (`internal/deploy/lifecycle.go:148-152`):

```go
out = append(out, Status{
    Name:        name,
    State:       "error",
    ErrorDetail: err.Error(),
})
```

Consumed at exactly one site, `internal/cli/status.go:62-66`:

```go
for _, st := range statuses {
    if st.ErrorDetail != "" {
        fmt.Fprintf(errw, "status: %s: %s\n", st.Name, st.ErrorDetail)
    }
}
```

The `err.Error()` call is the one-way demotion: by the time the CLI reads `ErrorDetail`, the chain is gone. That's the point.

## When `string` beats `error`

All three must hold:

1. **One consumer, presentation-only.** The field is printed verbatim and never matched. If anything would `errors.Is(st.ErrorDetail, ...)`, the answer is `error`.
2. **Operator does not need categorisation.** If the operator needs to grep on `error: schema_mismatch` versus `error: permission_denied`, you want a typed taxonomy in a separate field — not a wrapped error chain in this one.
3. **The struct is already presentation-shaped.** `Status` already carries `ContainerName` (derived string), `LastDeployedAt` (whose format the CLI chooses). Adding one more presentation-adjacent field is consistent; adding an `error` field to a hot-loop domain struct introduces a chain-traversal cost on every row read.

## The doc comment is load-bearing

`// NOT rendered in stdout; the CLI prints it to stderr.` is the only thing preventing future drift toward a "helpful" sixth column. Without it, the next maintainer who scans `Status` sees seven fields and a tabwriter writing five, assumes the sixth/seventh are bugs, "fixes" by widening the table. The comment is the lock that says "yes, this asymmetry is intentional."

Companion comment on the receiver method (`StatusAll`) names the policy in narrative form: "Per-service failures are absorbed into the result: the row is synthesised with State=\"error\" and ErrorDetail set to the wrapped error text."

## Why not a typed sentinel taxonomy

Five sub-states (`error: schema`, `error: permissions`, `error: config`, `error: docker`, bare `error`) was the rejected alternative. Reasons documented at `_ai/cli-flag-surface-coherence.md` (every operator-visible state is a contract surface) and at `two-sentinels-for-two-failure-modes.md` (sentinels are for callers that branch). The status surface has zero branching consumers — the operator reads stderr and decides. A taxonomy means N×4 surfaces to keep in sync for zero new operator capability. YAGNI.

## When NOT to apply

If any of these is true, use `error`:

- A future operator script will grep on category tokens, not free-form messages. (Then categorise, don't stringify.)
- The detail flows back into a retry/replay path. (Then preserve `errors.Is`.)
- The struct is persisted (TOML/JSON) and a downstream consumer wants to reconstruct the chain. (Then design for serialisation, not stringification.)

## Originator

`_tasks/2026-05-13-status-list-all-services/{03-tech-plan.md §0, 04-linus-review.md §3, 010-kevlin-review.md §C3}` — Joel's tech plan made the call, Linus's review pushed back ("the field is right, but `string` is the wrong type") then conceded ("the operator does not need to distinguish — ship as `string`"), Kevlin verified the load-bearing comment survives.
