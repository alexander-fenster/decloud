# Two distinct sentinels for two distinct failure modes, not one umbrella

When a single guard function rejects on two *semantically different* reasons, declare TWO sentinel errors — one per reason — and write the tests so each sentinel matches its case AND does NOT match the other. One umbrella sentinel ("ErrBadService") that covers both empty-and-slash collapses the caller's ability to discriminate; the symmetric pair (`ErrEmptyService` + `ErrInvalidService`) lets a future caller branch.

## Live example

`internal/dockerdrv/driver.go:22-34` declares:

- `ErrEmptyService` — `Service` field is empty (programmer error at the call site).
- `ErrInvalidService` — `Service` contains `/` (would make `journalctl CONTAINER_TAG=decloud/<svc>` ambiguous).

Both fire from `Run` and `RunWithOptions`. Both are independent `errors.New` calls — no shared parent. The pair sits in the same `var (…)` block as the existing `ErrContainerNotFound` / `ErrNoBridgeIP` for visual locality.

## Test shape that locks the contract

Each rejection test asserts BOTH directions:

```go
assert.True(t, errors.Is(err, dockerdrv.ErrEmptyService))
assert.False(t, errors.Is(err, dockerdrv.ErrInvalidService)) // load-bearing
```

The `False` row is the load-bearing one. Without it, a future "cleanup" PR that folds both sentinels into one (`ErrBadService`) passes the suite — the positive `True` row would still match. The negative row is what fails on the consolidation attempt, surfacing the contract loss at PR time. See `internal/dockerdrv/cli_driver_test.go::TestCLIDriver_RunReturnsErrEmptyServiceWhenServiceIsEmpty` (and its three siblings).

## When to apply

Multiple-reason guards where:

1. A caller might legitimately want to react differently per reason (retry empty, fail-hard on slash).
2. The reasons have different *operator-facing* phrasing in error messages (an umbrella sentinel forces phrasing into the wrap context, splitting one source-of-truth into many).
3. The reasons could plausibly be conflated by a future "simplification" — the discrimination test is the lock.

When the reasons are truly the same shape (e.g., "input out of range" with N specific bounds), a single sentinel is fine. The pattern is *semantic distinctness*, not *count of branches*.

## Anti-pattern

Single `ErrBadService` carrying the reason in the error string only. Callers can't branch without substring matching (which is the `stderr-substring-canary.md` shape — fine when wrapping a third-party tool's stderr, *wrong* when you control the sentinel surface).

## Originator

`_tasks/2026-05-12-journald-log-driver/03-tech-plan.md` §3.3 / §5.1 specified the pair; Linus's plan review (`04-linus-plan-review.md` R2.2) promoted the slash-rejection invariant to a hard driver guard rather than a comment; Kent's tests (`05-kent-tests.md` §6.2.1–§6.2.4) locked the symmetric discrimination.
