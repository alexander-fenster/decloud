# Linus iteration-2 high-level re-review

## Verdict: APPROVE

Iteration 2 closes both strategic items I left on the table.

**Issue 1 (§3.4.5 cancellation symmetry) — RESOLVED.** `service.go:199-201` (Stop) and `:207-209` (Remove) now layer `if isCancellation(err) { return fmt.Errorf("%w: %w", ErrInterrupted, err) }` ahead of the existing `ErrRun` wraps, structurally identical to §3.5. Eight cancellation-discrimination sites in `Deploy` use the same predicate; ctrl+c during a redeploy stop+remove now surfaces exit 130, matching the fresh-deploy path. The new test `TestDeploy_RedeployStopRemovePreviousContainerCancelledReturnsErrInterrupted` (two subtests) flipped FAIL→PASS exactly as Kent set up. The asymmetry I missed in v2 is gone.

**Issue 2 (`usage.md:240`) — RESOLVED.** Raymond's rewrite correctly attributes second-SIGINT absorption to `signal.NotifyContext`, names the 30s `cleanupTimeout` symbol, and points users at SIGKILL as the bail. Every cited symbol verified; backlog item 9 captures the real second-signal product fix.

**`isCancellation` placement and semantics — CORRECT.** Defined adjacent to `newCleanupContext` (`service.go:44-50`), package-private, no exported surface widening. Uses `errors.Is`, so wrapped cancellations (e.g. driver-side `fmt.Errorf("...: %w", ctx.Err())`) traverse the Unwrap chain — observably equivalent to the inline idiom it replaces. Pure refactor; existing tests didn't move because they assert on the returned error chain, not on which predicate spelled the check. Six original sites collapsed plus two new (Item 1) sites, all uniform.

**Cumulative diff coherent, not patchwork.** v2.1 fixed mechanism (cleanup-context, audit fork, exit-code, label gate); v2.2 fixed remaining symmetry, named the idiom, and trued up doc/log strings. Each layer made the file shorter to read.

**One micro-finding, NOT blocking:** `_docs/usage.md:235` still quotes the OLD slog phrase `cleanup failed; please remove decloud-<name> manually`; production now logs `cleanup failed; manual removal may be required`. Same class of doc-vs-code drift §13.8 caught for Item 4. Trivial follow-up for Raymond — one-sentence edit, no code change. Mention to Don; do not bounce iteration 2.

Final architectural signoff. Ship.

— Linus
