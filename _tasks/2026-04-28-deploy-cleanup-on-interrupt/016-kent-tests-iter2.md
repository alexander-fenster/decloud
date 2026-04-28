# Kent's tests — iteration 2 (v2.2 §13.1)

Status: one new test added, compiles clean, both subtests fail with the EXPECTED mode (redeploy path currently wraps `context.Canceled` as `ErrRun`).

## Test added

`internal/deploy/service_test.go:614-654` — `TestDeploy_RedeployStopRemovePreviousContainerCancelledReturnsErrInterrupted` with two subtests (`stop-cancelled`, `remove-cancelled`). Mirrors `TestDeploy_DefensiveOrphanInspectCancelledReturnsErrInterrupted` at L550-606. Harness: `newDeployerHarness(t, withoutInspectAbsentDefault())`, `Store.Load → newPrev(), nil` to enter the `hasPrev` branch at `service.go:185-197`.

- `stop-cancelled`: `Stop → context.Canceled`; defensive `Inspect → InspectResult{State:"running"}` so the inner "still-running" branch fires.
- `remove-cancelled`: `Stop → nil`; `Remove → context.Canceled`.

Both rows assert `errors.Is(err, deploy.ErrInterrupted)` AND `!errors.Is(err, deploy.ErrRun)`.

## Expected failure mode (verified)

```
stop-cancelled:    got "deploy: docker run failed: stop previous container: context canceled"
remove-cancelled:  got "deploy: docker run failed: remove previous container: context canceled"
```

Both wrap as `ErrRun` today — exactly the asymmetry §13.1 fixes. Once Rob lands Items 5+1 (`isCancellation` helper + two pre-checks at L191/L196), both subtests pass.

## Items 4 & 6 — no test churn (verified)

Grepped `internal/`, `cmd/`, `_docs/` for `"removed orphan container"`, `"removing orphan container"`, `"cleanup failed; please remove"`, `"cleanup failed; manual removal"`. Only matches: `service.go` itself (Rob's targets) and `_docs/usage.md:235,237` (Raymond's §13.8 follow-up). Zero test files match. Joel's claim confirmed.

## Item 5 — no test churn

Pure refactor (inline idiom → helper). All existing assertions are on the returned error chain (`errors.Is(err, deploy.ErrInterrupted)` etc.), unchanged by the helper hoist.

## Verification

- `go build ./...` — clean.
- `go test ./internal/deploy/ -run TestDeploy_RedeployStopRemovePreviousContainerCancelled -count=1 -v` — `FAIL` on both subtests with the messages quoted above.
- All pre-existing tests in `internal/deploy` continue to pass.

— Kent
