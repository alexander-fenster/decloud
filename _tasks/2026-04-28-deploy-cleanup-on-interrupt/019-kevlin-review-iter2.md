# Kevlin's iteration-2 re-review

## Verdict: APPROVE

All six v2.2 items landed correctly. Build/vet/gofmt clean; full suite green (`internal/deploy` 12.08s); `%w: %v` grep empty. No new findings.

## Per-item verification (live grep)

- **§13.1 redeploy stop+remove cancellation symmetry** — `service.go:199-201` and `:207-209` add `if isCancellation(err) { return fmt.Errorf("%w: %w", ErrInterrupted, err) }` ahead of the existing `ErrRun` wraps. Mirrors §3.5 verbatim. Asymmetry resolved.
- **§13.2 second-ctrl+c sentence** — `_docs/usage.md:240` now correctly attributes the absorption to `signal.NotifyContext`. Every symbol cited verifies: `cmd/decloud/main.go:14` (`signal.NotifyContext` call), `internal/deploy/service.go:34` (`cleanupTimeout = 30 * time.Second`). No hallucinations.
- **§13.3 line-range typo** — `_ai/exit-code-sentinel-not-context-err.md:69` reads `:40-43`. `internal/cli/exit_codes_test.go:40-43` are exactly the four cited rows; lines 38-39 are the `caddy-down` rows. Correct.
- **§13.4 audit-log tense** — `service.go:226` reads `"removing orphan container from prior interrupted deploy"`. `_docs/usage.md:237` quote synced (Joel's §13.8 downstream check landed). (Rob's report cites line 222; actual is 226 because Items 5/1 inserted lines ahead. Cosmetic only — the edit itself is correct.)
- **§13.5 `isCancellation` helper** — `service.go:48-50` defines the helper next to `newCleanupContext`; eight call sites now use it (lines 199, 207, 215, 229, 235, 258, 275 hoist, 355). The only remaining `errors.Is(err, context.Canceled)` in the file is inside the helper itself. Six pre-existing sites collapsed + two new (Item 1) sites uniformly use the helper. The Henney threshold is satisfied.
- **§13.6 slog convention** — four sites (`service.go:284, 288, 345, 349`) read `logger.Warn("cleanup failed; manual removal may be required", "container", containerName, "error", <stopErr|rmErr>)`. Grep-stable fixed message; container only in structured field. Matches `lifecycle.go:25-30` exemplar.

## Cross-doc grep
- `grep 'orphan container' _docs/usage.md internal/deploy/service.go` — both files quote `removing orphan container from prior interrupted deploy` (line 237 / 226). Operators grepping the doc phrase against the audit log will hit.
- `grep 'cleanup failed; manual removal' internal/deploy/service.go` — four hits.

## Pattern adherence
`error-wrap-discipline.md`, `cleanup-context-discipline.md`, `label-gated-orphan-recovery.md`, `gomock-fifo-matching.md`, `exit-code-sentinel-not-context-err.md` — all still self-consistent. v2.2 polish does not perturb any pattern's invariants.

Ship it.

— Kevlin
