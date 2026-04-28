# Cleanup paths must NOT depend on the user-cancellable ctx

When an orchestrator wraps a sequence of side-effecting steps and rolls back on failure, the rollback / cleanup blocks MUST run on a context derived from `context.Background()` with a bounded timeout — NOT the caller's request context. Otherwise SIGINT (ctrl+c) cancels both the forward step and the cleanup, leaving side effects half-done.

## The bug class

`cmd/decloud/main.go:14` wires SIGINT/SIGTERM into the cobra root via `signal.NotifyContext`. The cancelled ctx flows through `runDeployService` to `serviceDeployer.Deploy`. Inside `Deploy`, post-failure cleanup that re-uses the same ctx — `Driver.Stop(ctx, ...)`, `Driver.Remove(ctx, ...)` — calls `exec.CommandContext` with an already-cancelled context. Per the `os/exec` contract, the child process is never started; `Run()` returns the ctx error immediately. `docker stop` and `docker rm` never execute on the host.

The user observes: ctrl+c during readiness leaves `decloud-<name>` running, and the next deploy fails with `Conflict. The container name "/decloud-<name>" is already in use`. Originating bug: `_tasks/2026-04-28-deploy-cleanup-on-interrupt/01-user-request.md`.

## The pattern

Two flavours of context inside one orchestrator:

- **Forward-progress context** — caller's `ctx`. Cancellation aborts forward steps (build, run, probe, save, reload). Status quo.
- **Cleanup context** — fresh, derived from `context.Background()` with a bounded timeout. Never cancelled by user SIGINT, but bounded so a hung daemon does not pin the CLI forever.

Helper, `internal/deploy/service.go:32-42`:

```go
const cleanupTimeout = 30 * time.Second

func newCleanupContext() (context.Context, context.CancelFunc) {
    return context.WithTimeout(context.Background(), cleanupTimeout)
}
```

30s budget = 10s `docker stop` grace + headroom for `docker rm` and a rollback `docker run`.

Call-site shape (probe-failure block, `service.go:260-287`):

```go
if err := d.probe.Wait(ctx, ...); err != nil {
    cleanupCtx, cleanupCancel := newCleanupContext()
    defer cleanupCancel()
    if stopErr := d.deps.Driver.Stop(cleanupCtx, containerName, 10*time.Second); ... { /* warn */ }
    if rmErr  := d.deps.Driver.Remove(cleanupCtx, containerName);                ... { /* warn */ }
    if hasPrev {
        d.restoreOldContainer(cleanupCtx, prev)
    }
    ...
}
```

`restoreOldContainer`'s parameter is named `cleanupCtx` (not `ctx`) so the call-site contract is self-documenting: every caller passes a `newCleanupContext()`-derived value, and a future caller that tries to pass the request ctx is rejected at the diff stage.

## Where this is applied today

Three sites in `internal/deploy/service.go`, all switched in this task:

- Probe-failure cleanup (line ~267).
- Run-failure rollback to old container (line ~240).
- Save-failure rollback (line ~323).

Also: the audit log line forks alongside the cleanup ctx. Cancellation logs `deploy cancelled during readiness wait` at `slog.Info`; real readiness failure logs `readiness failed` at `slog.Error`. Same fork shape generalises to any cleanup block.

## Cancellation re-wrap

After cleanup runs, the orchestrator re-classifies the original error:

```go
if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
    return fmt.Errorf("%w: %w", ErrInterrupted, err)
}
```

`deploy.ErrInterrupted` is the choke-point sentinel (see `exit-code-sentinel-not-context-err.md`). The CLI maps only that sentinel to exit 130, not the bare `context.*` errors.

## When to apply

Any orchestrator that:

1. Takes a request ctx that may be cancelled by the caller (signals, deadlines, web-request abort).
2. Runs side-effecting steps (containers, files, network) that must be undone on failure.
3. Has at least one failure path where "cleanup cancelled because the user cancelled" is a worse outcome than "cleanup ran for a few extra seconds."

Backlog item 7 in `m1x-backlog.md` carries this pattern to `internal/caddy/manager.go` — same shape of bug, different code path.

## What this does NOT cover (by design)

- `SIGKILL` of the orchestrator process. No defer can reach.
- Host power loss between forward step and cleanup defer.

The mitigation for both is label-gated orphan recovery on the next run; see `label-gated-orphan-recovery.md`.

## Locked in by

- `TestDeploy_ProbeCancellationCleansUpWithFreshContext` (`internal/deploy/service_test.go:364-388`) — cancels request ctx inside `Driver.Run`; asserts `Stop`/`Remove` receive a context with `Err() == nil` at call time via `notCancelledCtxMatcher`.
- `TestDeploy_RestoreOldContainerUsesFreshContextOnRedeployCancellation` (`service_test.go:467-496`) — same matcher applied to the rollback `Run` call; covers latent defect B from the originating plan.
