# Raymond — iteration-2 doc fixes (v2.2 §13.2, §13.3, §13.8)

Three edits. No scope creep. Cross-file grep verified.

## Diffs

### §13.3 — `_ai/exit-code-sentinel-not-context-err.md:69`

Before: `All four in `internal/cli/exit_codes_test.go:38-41`.`

After:  `All four in `internal/cli/exit_codes_test.go:40-43`.`

### §13.8 — `_docs/usage.md:237` (audit-log quote sync with Rob's tense fix)

Before: ``The audit log records `removed orphan container from prior interrupted deploy` at warn level.``

After:  ``The audit log records `removing orphan container from prior interrupted deploy` at warn level.``

### §13.2 — `_docs/usage.md:240` (second-ctrl+c falsehood)

Before: `A second ctrl+c (impatient double-tap) bypasses graceful cleanup and may leave the container behind. Path (1) above still recovers on the next deploy.`

After:  ``A second ctrl+c during cleanup does not interrupt cleanup; the Go signal handler installed by `signal.NotifyContext` absorbs it. To force exit before the 30-second `cleanupTimeout` window completes, send SIGKILL (`kill -9 <pid>`); path (1) above still recovers on the next deploy.``

## Self-audit — every claim in the §13.2 rewrite cross-referenced

- "the Go signal handler installed by `signal.NotifyContext`" — verified at `cmd/decloud/main.go:14`: `ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)`. The handler stays registered until `stop()` runs in the deferred path at line 15.
- "absorbs it" — `signal.NotifyContext` semantics: subsequent matched signals are absorbed by the package handler until `stop()` is called; they do not propagate to the OS default handler.
- "30-second `cleanupTimeout`" — verified at `internal/deploy/service.go:34`: `const cleanupTimeout = 30 * time.Second`. Symbol name and value match verbatim.
- "SIGKILL (`kill -9 <pid>`)" — only signal Go cannot intercept; the sole non-wait bail path.
- "path (1) above still recovers on the next deploy" — preserves the prior sentence's recovery hand-off.

## Cross-file string check

`grep -n 'orphan container' _docs/usage.md internal/deploy/service.go` shows `_docs/usage.md:237` and `service.go:226` both quote the literal `removing orphan container from prior interrupted deploy`. Operators grepping the doc phrase against the audit log will hit.

— Raymond
