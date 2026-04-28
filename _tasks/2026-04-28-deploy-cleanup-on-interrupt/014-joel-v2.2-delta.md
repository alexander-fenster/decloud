# Joel — v2.2 tech plan delta coordination note

Appended a §13 to `03-tech-plan.md` covering Don's six fix-in-scope items. Earlier sections untouched; v2.2 is a delta only. Revision history at top bumped to v2.2.

Per-item:

1. **§13.1 — §3.4.5 cancellation symmetry**: two new `isCancellation`-gated `ErrInterrupted` pre-checks at `service.go:185-197` (sites 191 stop, 196 remove). New test `TestDeploy_RedeployStopRemovePreviousContainerCancelledReturnsErrInterrupted` (two subtests: `stop-cancelled`, `remove-cancelled`) using `withoutInspectAbsentDefault()` and `Store.Load → newPrev()`.
2. **§13.2 — usage doc fix**: replacement sentence for `_docs/usage.md:240` per Don's lockdown text.
3. **§13.3 — `:38-41` → `:40-43`** in `_ai/exit-code-sentinel-not-context-err.md:69`.
4. **§13.4 — `removed` → `removing`** at `service.go:212`.
5. **§13.5 — `isCancellation(err) bool` helper** placed next to `newCleanupContext`; six call-site swaps (lines 201, 215, 221, 244, 261, 341).
6. **§13.6 — slog convention fix** at four sites (`service.go:270, 274, 331, 335`); fixed message `"cleanup failed; manual removal may be required"` + structured field.

Implementation order (Item 5 before Item 1) plus acceptance criteria delta included at §13.7 / §13.10. Test churn: only the new redeploy-cancellation test; all existing tests unchanged.
