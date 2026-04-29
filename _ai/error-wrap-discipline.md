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

## Companion rule: one sentinel per chain when the chain crosses an exit-code router

The `%w: %w` rule says "preserve the inner sentinel for `errors.Is`." That's correct everywhere — except at boundaries where TWO sentinels in one chain create a case-ordering footgun.

The bite: a CLI parse path constructs `ValidateMounts(...)` (which wraps with `ErrInvalidMount` for the loader's exit-10 routing) and then re-wraps with `errUsage` (for the CLI's exit-2 routing). The result chain `errors.Is`-matches **both** sentinels. `internal/cli/exit_codes.go` is a `switch errors.Is(...)` on case-order; whichever case appears first in the source wins. Reordering cases for cleanliness silently flips the exit code from 2 to 10 (or vice versa). The test suite catches it, but only as "I reordered some cases; suddenly an unrelated test broke."

**Rule.** Each error chain must carry exactly ONE sentinel that the exit-code router (or any `errors.Is`-based dispatcher) can match. If the inner construction already wraps with sentinel A and the outer site needs sentinel B, **either**:

1. **Strip the inner wrap.** Factor a bare-error helper out of the inner-wrapping function; the outer caller calls the helper and wraps with B; the original A-wrapping function stays for callers who want A. M2 example: `registry.FindDuplicateTarget(mounts) (i, j int, ok bool)` — bare result, no error sentinel. `ValidateMounts` calls it internally and wraps with `ErrInvalidMount`; CLI's `parseMountFlags` calls it directly and wraps with `errUsage`. Each surface produces exactly one sentinel.

2. **Accept the case-ordering coupling and lock it with a regression test.** Less robust; comments lie eventually; reviewers eventually alphabetise. M2 explicitly rejected this option.

The CLI duplicate-target path lock: `internal/cli/deploy_service_test.go` carries `assert.False(t, errors.Is(err, registry.ErrInvalidMount), "CLI dup-target must NOT chain ErrInvalidMount")`. Belt: `internal/cli/exit_codes_test.go` `cli-mount-dup-wraps-usage` row asserts `ExitCodeFor` returns `ExitUsageError` regardless of case order. Two locks on the same regression vector — not redundant when the regression is "case ordering."

### When the rule does NOT apply

`%w: %w` chains where BOTH sentinels are checked at the same dispatch layer (e.g., `ErrReadiness` outer + `ErrNoBridgeIP` inner — the readiness probe wraps both, the caller branches on whichever it cares about) are FINE. Multiple sentinels are a problem only when an `errors.Is`-based router sees the chain and has to choose ONE.

### Originator

Joel addendum Decision in `_tasks/2026-04-28-m2-server-side-mounts/005-joel-tech-plan-addendum.md` Issue 1; Linus's footgun catch in `004-linus-plan-review.md` Issue 1; lock test prescribed by Joel and shipped by Kent (`007-kent-tests.md`).
