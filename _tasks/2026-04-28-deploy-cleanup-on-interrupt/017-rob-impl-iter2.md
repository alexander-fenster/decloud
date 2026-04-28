# Rob — iteration-2 implementation (v2.2 §13)

Status: all four in-scope items landed in `internal/deploy/service.go`. `go test ./... -count=1` green. `gofmt -l internal/ cmd/` empty. `go vet ./...` clean. `go build ./...` clean. `grep -rn '%w: %v' internal/ cmd/` empty. New test `TestDeploy_RedeployStopRemovePreviousContainerCancelledReturnsErrInterrupted` flipped FAIL → PASS on both subtests.

Implementation order followed §13.7 verbatim: Item 5, then Item 1, then Item 4, then Item 6. After Item 5, all pre-existing tests stayed green (pure refactor checkpoint); after Item 1, the new redeploy-cancellation test went green.

## Per-item summary

**Item 5 — `isCancellation(err) bool` helper (`service.go:44-49` new; six call-site swaps)**. Added the helper immediately after `newCleanupContext`. Comment block names predicate and policy reason per Joel's exact signature in §13.5. Six existing inline `errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)` idioms (formerly at lines 201, 215, 221, 244, 261, 341 of the v2.1 file) now read `if isCancellation(err) {` (five sites) and `cancelled := isCancellation(err)` (the line-261 hoist). Verified by grep — only remaining `context.Canceled || context.DeadlineExceeded` is inside the helper itself.

**Item 1 — §3.4.5 cancellation symmetry (`service.go:185-203`)**. Two new `isCancellation(err)` pre-checks added: one inside the Stop "still-running" inner branch (gated by the existing `!ErrContainerNotFound` filter, per Linus's verification), one inside the Remove outer wrap (gated by the existing `err != nil && !ErrContainerNotFound` filter). Both wrap as `fmt.Errorf("%w: %w", ErrInterrupted, err)`, mirroring §3.5 verbatim. Eight cancellation-discrimination call sites in the Deploy orchestrator now uniformly use the new helper.

**Item 4 — `service.go:222` audit-log tense (`removed` → `removing`)**. One-word change to the `logger.Warn(...)` line at the head of the `!hasPrev` orphan-cleanup branch. Announces the action before it executes, so a subsequent failure no longer reads as "removed orphan" followed by an error.

**Item 6 — slog message-vs-field convention (four sites: `service.go:284, 288, 345, 349`)**. All four `logger.Warn("cleanup failed; please remove "+containerName+" manually", "container", containerName, "error", <stopErr|rmErr>)` calls rewritten to `logger.Warn("cleanup failed; manual removal may be required", "container", containerName, "error", <stopErr|rmErr>)` — fixed grep-stable message, container only in the structured field. Matches `lifecycle.go:25-30` exemplar. The recovery hint stays in the returned error chain (the user-facing surface) and in `_docs/usage.md` §8.

## Test results

`go test ./... -count=1` — all packages PASS. Full output (one line per package):

```
?   	github.com/alexander-fenster/decloud/cmd/decloud	[no test files]
ok  	github.com/alexander-fenster/decloud/internal/caddy	0.023s
?   	github.com/alexander-fenster/decloud/internal/caddy/mocks	[no test files]
ok  	github.com/alexander-fenster/decloud/internal/cli	0.022s
?   	github.com/alexander-fenster/decloud/internal/cli/mocks	[no test files]
ok  	github.com/alexander-fenster/decloud/internal/config	0.010s
ok  	github.com/alexander-fenster/decloud/internal/deploy	12.073s
ok  	github.com/alexander-fenster/decloud/internal/dockerdrv	0.075s
?   	github.com/alexander-fenster/decloud/internal/dockerdrv/mocks	[no test files]
ok  	github.com/alexander-fenster/decloud/internal/envcap	0.106s
?   	github.com/alexander-fenster/decloud/internal/envcap/mocks	[no test files]
ok  	github.com/alexander-fenster/decloud/internal/ids	0.012s
ok  	github.com/alexander-fenster/decloud/internal/logging	0.013s
ok  	github.com/alexander-fenster/decloud/internal/registry	0.038s
```

Targeted re-run of the new test confirms both subtests now PASS:

```
--- PASS: TestDeploy_RedeployStopRemovePreviousContainerCancelledReturnsErrInterrupted
    --- PASS: stop-cancelled
    --- PASS: remove-cancelled
```

`TestDeploy_StopOldFailureAbortsAndDoesNotStartNew` (existing `errors.New("stop timed out")` ErrRun assertion) still PASSES — confirms the new pre-check correctly discriminates cancellation from generic stop failures.

## Deviations

None. All four items implemented exactly per §13.1, §13.4, §13.5, §13.6 (verbatim text from Don's lockdown, Joel's delta, Linus's APPROVE). Items 2 and 3 not touched (Raymond's scope). No scope creep into §3.4.1, §3.4.2 cleanup-context blocks, `restoreOldContainer`, or `cmd/decloud/main.go`.

— Rob
