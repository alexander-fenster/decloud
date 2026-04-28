# Ward's finalization — durable learnings preserved

## Files added/modified

- **`_ai/cancellation-symmetry-audit.md`** (NEW). When fixing a `context.Canceled` mis-wrap at one call site, audit every sibling forward-progress branch on the same request ctx. Captures Linus's iter2 Issue 1 — §3.4.5 was the sibling of §3.5, missed in v2 plan-review and only caught on impl re-review. Cites `service.go:48-50` (helper), `:199-201` and `:207-209` (the two new pre-checks), and the two locking tests.
- **`_ai/fix-now-while-fresh.md`** (NEW). Don's repeated decision rule across v2.1 and v2.2 lockdowns: fix in scope when mechanical + same-file + <5-minute floor + on-theme. Defer when new architecture or a different package's review surface is required. Cites `007-don-final-lockdown.md`, `013-don-plan-iteration2.md`, `021-don-final-signoff.md`.
- **`_ai/doc-grep-discipline.md`** (APPENDED). Added one section: slog messages quoted in operator runbooks are also a contract; recipe = `grep -F "<old phrase>" _docs/` whenever a slog message changes. Cites the two iter2 doc-vs-code drifts (`usage.md:237` pre-flagged in §13.8; `usage.md:235` caught by Linus iter2).
- **`_ai/MEMORY.md`** (UPDATED). Two new index entries (`cancellation-symmetry-audit`, `fix-now-while-fresh`) and one extended entry (`doc-grep-discipline` slog-extension note).

## Self-audit

- **No duplication with Raymond's iter1 four**: I read all four (`cleanup-context-discipline`, `label-gated-orphan-recovery`, `exit-code-sentinel-not-context-err`, `gomock-fifo-matching`). My three additions cover orthogonal axes — review-discipline (audit recipe), process-judgment (when to fix in scope), and a doc-grep extension — none re-explain mechanism, label gates, sentinels, or gomock matching.
- **No hallucinations**: every cited line/symbol grepped against current source. `isCancellation` at `service.go:48-50` (10 occurrences total = 1 doc-comment + 1 def + 8 call sites) verified. `service.go:199, 207` are the iter2 Item 1 pre-checks. `_docs/usage.md:235, 237` quote `cleanup failed; manual removal may be required` and `removing orphan container from prior interrupted deploy` — both match production verbatim.
- **No invented learnings**: each pattern cites the originating task file (`12-linus-impl-review.md` Issue 1, `013-don-plan-iteration2.md`, `20-linus-impl-review-iter2.md`).

— Ward
