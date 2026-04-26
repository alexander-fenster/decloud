# `fmt.Errorf` discipline: `%w: %w` not `%w: %v`

The inner error MUST traverse `errors.Is`/`errors.As`. `%w: %v` formats the inner as a string and severs the chain — sentinel detection at higher callers silently breaks.

## Rule

Wrap with `fmt.Errorf("%w: %w", OuterSentinel, innerErr)` or `fmt.Errorf("%w: <ctx>: %w", OuterSentinel, innerErr)`. Never `%v` for an `error` value when there's a sentinel above.

## Why this regresses easily

Go formatting muscle memory pushes `%v` for "just print it." Linters do not flag this; `go vet` does not flag this. The bug is invisible until a higher caller writes `errors.Is(err, deploy.ErrBuild) && errors.Is(err, dockerdrv.ErrSomething)` and only the outer one passes.

## Grep recipe

Run before every implementation report:

```bash
grep -rn '%w: %v' internal/ cmd/
```

Must return zero. Iter1 had 21 violations (caught by Linus in `13-linus-execution-review.md`); iter2 closed all 21.

## Test that locks it in

`internal/deploy/service_test.go:TestDeploy_BuildErrorPreservesInnerSentinel` is the regression test: synthetic `errors.New("synthetic build err")` survives the orchestrator's outer wrap and is recoverable via `errors.Is(err, sentinel)`. Add equivalents whenever a new outer sentinel is introduced.

## Exemplary site

`internal/deploy/readiness.go:64` — `fmt.Errorf("%w: %w", ErrReadiness, lastErr)`. Inner `lastErr` may be `dockerdrv.ErrNoBridgeIP`; both traverse. Caller can branch on either independently.
