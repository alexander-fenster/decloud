# Exit-code mapping matches OUR sentinel, not bare `context.*`

When a CLI maps Go errors to numeric exit codes, match a package-defined sentinel that the orchestrator wraps cancellation in — NOT bare `context.Canceled` / `context.DeadlineExceeded`. The bare-ctx-err match is the same shape of trap that the readiness probe fell into ("any ctx error must mean user cancelled") and bites the next time someone uses `context.WithTimeout` for an unrelated deadline.

## The trap

A naive `ExitCodeFor` adds, for "defence in depth":

```go
case errors.Is(err, deploy.ErrInterrupted),
     errors.Is(err, context.Canceled),
     errors.Is(err, context.DeadlineExceeded):
    return ExitInterrupted
```

It's harmless today because every cancellation flows through `Deploy` and gets wrapped as `ErrInterrupted`. Tomorrow a future caller adds `context.WithTimeout(ctx, 5*time.Second)` somewhere unrelated — say, a registry-list operation — and that timeout fires. The user sees exit `130` and reasonably concludes they pressed ctrl+c. They didn't.

Linus's argument in `_tasks/2026-04-28-deploy-cleanup-on-interrupt/04-linus-review.md` Issue 2: defence in depth in the wrong direction. The orchestrator IS the choke point; let it be the choke point.

## The pattern

The orchestrator (`internal/deploy/service.go`) is the only place that converts `context.Canceled` / `context.DeadlineExceeded` into `ErrInterrupted`. Three sites in `Deploy` do this conversion (probe-fail, run-fail, save-fail), plus three sites in the defensive-orphan branch (Inspect, Stop, Remove). All converge on:

```go
if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
    return fmt.Errorf("%w: %w", ErrInterrupted, err)
}
```

The CLI mapping (`internal/cli/exit_codes.go:37-38`) matches only the sentinel:

```go
case errors.Is(err, deploy.ErrInterrupted):
    return ExitInterrupted
```

Bare `context.Canceled` / `context.DeadlineExceeded` correctly fall through to `ExitInternal` (70). That contract is locked in by negative test rows, not just left as "well, nothing matches today":

```go
{"context-canceled-bare", context.Canceled, ExitInternal}
{"context-deadline-bare", context.DeadlineExceeded, ExitInternal}
```

If a future "helpful" PR adds `errors.Is(err, context.Canceled)` to the `ExitInterrupted` case, those rows fail and the regression is caught at PR time.

## Why the choke-point pattern beats end-of-pipeline detection

- **Choke-point wraps mean every cancellation has the orchestrator's full context attached** (deploy_id, service name, current step in the slog audit log). End-of-pipeline detection in `main.go` has none of that.
- **A future code path that legitimately uses `context.WithTimeout` for a non-cancellation reason** (a network probe with a deadline, a test fixture with a timeout) doesn't accidentally surface as "user cancelled" to the CLI.
- **The negative tests document the contract.** Anyone reading `exit_codes_test.go` sees that bare ctx errors route to `ExitInternal` deliberately, not by accident.

## When to apply

Any CLI that:

1. Translates Go errors to numeric exit codes via `errors.Is`.
2. Has at least one outer sentinel that the orchestrator wraps cancellation in.
3. Wants the exit code to mean exactly what it says — not "we guessed."

Pair with the cleanup-context pattern (`cleanup-context-discipline.md`): the orchestrator first switches cleanup off the cancelled ctx, then re-classifies the error as `ErrInterrupted`, then returns. The CLI sees one canonical sentinel.

## Locked in by

- `TestExitCodeFor_AllSentinels/interrupted` — `deploy.ErrInterrupted` → 130.
- `TestExitCodeFor_AllSentinels/interrupted-wrapped` — `fmt.Errorf("oops: %w", deploy.ErrInterrupted)` → 130 (wrap-traversal).
- `TestExitCodeFor_AllSentinels/context-canceled-bare` — `context.Canceled` → 70 (negative test, locks the choke-point contract).
- `TestExitCodeFor_AllSentinels/context-deadline-bare` — `context.DeadlineExceeded` → 70 (same).

All four in `internal/cli/exit_codes_test.go:40-43`.
