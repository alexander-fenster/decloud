# Rob's implementation — deploy cleanup on interrupt (v2.1)

Status: implemented per Joel's tech plan v2.1 + Don's v2.1 lockdown. All tests
pass. `go build ./...`, `go vet ./...`, `gofmt -l internal/ cmd/`, and
`grep -rn '%w: %v' internal/ cmd/` are all clean.

Plans referenced: `02-plan.md` v2.1, `03-tech-plan.md` v2.1,
`06-linus-review-v2.md`, `007-don-final-lockdown.md`, `08-kent-tests.md`.

## Implementation order followed

Joel's P.P.S. v2 order:

1. Driver-level: `InspectResult.Labels` (already present from Kent),
   `cliDriver.Inspect` JSON `--format` + parser.
2. Sentinel `ErrInterrupted` (already present from Kent) + `cleanupTimeout`
   const + `newCleanupContext()` helper.
3. Harness `AnyTimes()` default — already present from Kent.
4. Three cleanup blocks: probe-failure (with audit-log fork), save-failure,
   run-failure.
5. Defensive orphan branch with label gating + cancellation discrimination at
   three sites.
6. Exit-code mapping: only `deploy.ErrInterrupted` (no bare `context.*`).
7. Pre-existing whitespace `Inspect` test fixture migrated to JSON shape.

## File-by-file summary

### `internal/dockerdrv/cli_driver.go`

Added `encoding/json` import. Rewrote `cliDriver.Inspect` to issue
`--format '{"id":{{json .Id}},"state":{{json .State.Status}},"labels":{{json .Config.Labels}}}'`
and parse the JSON envelope into `InspectResult{ContainerID, State, Labels}`.
JSON over the previous whitespace shape: labels can carry spaces, equals
signs, and JSON-special characters that the old `strings.Fields` parser would
have mangled. The not-found path is unchanged: `isNotFound(stderr)` →
`InspectResult{State: "absent"}, nil`.

### `internal/dockerdrv/cli_driver_test.go`

Migrated the `TestCLIDriver_InspectArgsAndParse` fixture from
`echo cid12345 running` to `echo '{"id":"cid12345","state":"running","labels":null}'`
and updated the comment to the new format string. Per Kent's report this
existing test is Rob's to migrate (§5.11 items 1–2) — the new label-parsing
tests `TestCLIDriver_InspectParsesDecloudServiceLabel` and
`TestCLIDriver_InspectReturnsNilLabelsWhenContainerHasNone` were already added
by Kent and now pass against the JSON parser. No other test files touched.

### `internal/deploy/service.go`

Added `cleanupTimeout = 30 * time.Second` and `newCleanupContext()` helper
deriving from `context.Background()` so cleanup survives request-ctx
cancellation. Cleaned up the v0 stub comment on `ErrInterrupted` (the
sentinel itself unchanged from Kent's stub). Three cleanup blocks rewritten:

- **Probe-failure (lines 260–287):** audit-log forked — `Info` for cancellation
  (`context.Canceled`/`context.DeadlineExceeded`) vs `Error` for real readiness
  failure. Stop/Remove now run on `cleanupCtx`; both filter
  `dockerdrv.ErrContainerNotFound` and surface real failures via
  `logger.Warn`. `restoreOldContainer` runs on `cleanupCtx`. Cancellation
  re-wraps as `ErrInterrupted`; readiness failures keep the existing
  `ErrReadiness` shape.
- **Save-failure (lines 327–349):** `cleanupCtx` derived once at the top of
  the block; `DeleteOrphanConfig`, Stop, Remove, and `restoreOldContainer` all
  use it. Cancellation re-wraps as `ErrInterrupted`. Other save errors keep
  the existing `registry save: %w` shape.
- **Run-failure rollback (lines 237–248):** `restoreOldContainer` runs on
  `cleanupCtx`. Cancellation re-wraps as `ErrInterrupted`.

Added the §3.5 defensive label-gated orphan branch (lines 198–227, gated on
`!hasPrev`). Inspect → if absent, no-op; if present and
`Labels["decloud.service"] == req.Name`, log Warn + Stop + Remove (using
request `ctx`, this is forward progress). If present with a missing or
mismatched label, refuse with the manual-`docker rm -f` recovery hint wrapped
as `ErrRun`. Per §3.5 / §3.4.3 cancellation symmetry (Don's v2.1 lockdown),
each of the three driver calls (Inspect, Stop, Remove) detects
`context.Canceled`/`context.DeadlineExceeded` and re-wraps as `ErrInterrupted`
before falling through to the `ErrRun` wrap.

`restoreOldContainer` parameter renamed `ctx` → `cleanupCtx` so the call-site
contract is self-documenting (Joel §3.4.4 preference). No signature change.

### `internal/cli/exit_codes.go`

Added one `case errors.Is(err, deploy.ErrInterrupted): return ExitInterrupted`
near the top of the `ExitCodeFor` switch (placed before `errUsage` — the
choke-point sentinel wins over more-specific exits). Per Linus Issue 2 and
Joel §3.6: matched **only** `deploy.ErrInterrupted`. Bare `context.Canceled`
and `context.DeadlineExceeded` correctly fall through to `ExitInternal`,
which Kent's negative table cases (`context-canceled-bare`,
`context-deadline-bare`) lock in. Cleaned up the v0 stub comment on
`ExitInterrupted` to a one-line POSIX rationale. No `context` import needed.

### `internal/deploy/readiness.go`

Per §3.3 of Joel's v2 plan and Linus Issue 3: **no change.** `httpProbe.Wait`
already returns raw `ctx.Err()` on cancellation. The discrimination /
audit-log fork moved to the orchestrator side as part of the §3.4.1 cleanup
block change.

## Test-suite output

```
$ go test ./... -count=1
?   	github.com/alexander-fenster/decloud/cmd/decloud	[no test files]
ok  	github.com/alexander-fenster/decloud/internal/caddy	0.017s
?   	github.com/alexander-fenster/decloud/internal/caddy/mocks	[no test files]
ok  	github.com/alexander-fenster/decloud/internal/cli	0.022s
?   	github.com/alexander-fenster/decloud/internal/cli/mocks	[no test files]
ok  	github.com/alexander-fenster/decloud/internal/config	0.009s
ok  	github.com/alexander-fenster/decloud/internal/deploy	12.080s
ok  	github.com/alexander-fenster/decloud/internal/dockerdrv	0.075s
?   	github.com/alexander-fenster/decloud/internal/dockerdrv/mocks	[no test files]
ok  	github.com/alexander-fenster/decloud/internal/envcap	0.102s
?   	github.com/alexander-fenster/decloud/internal/envcap/mocks	[no test files]
ok  	github.com/alexander-fenster/decloud/internal/ids	0.010s
ok  	github.com/alexander-fenster/decloud/internal/logging	0.015s
ok  	github.com/alexander-fenster/decloud/internal/registry	0.039s
?   	github.com/alexander-fenster/decloud/internal/registry/mocks	[no test files]
```

All Kent's new tests PASS:

- `TestDeploy_ProbeCancellationCleansUpWithFreshContext`
- `TestDeploy_DefensiveOrphanCleanupOnFreshDeployWhenContainerExists`
- `TestDeploy_DefensiveOrphanCleanupSkippedWhenContainerAbsent`
- `TestDeploy_DefensiveOrphanCleanupFailureWrapsErrRun`
- `TestDeploy_RestoreOldContainerUsesFreshContextOnRedeployCancellation`
- `TestDeploy_DefensiveOrphanRefusesContainerWithoutDecloudLabel`
- `TestDeploy_DefensiveOrphanRefusesContainerWithMismatchedLabel`
- `TestDeploy_DefensiveOrphanInspectCancelledReturnsErrInterrupted` (3 subtests:
  inspect-cancelled, stop-cancelled, remove-cancelled)
- `TestReadiness_ContextCancellationStopsProbe` (tightened)
- `TestExitCodeFor_AllSentinels` (with new rows: `interrupted`,
  `interrupted-wrapped`, `context-canceled-bare`, `context-deadline-bare`)
- `TestCLIDriver_InspectParsesDecloudServiceLabel`
- `TestCLIDriver_InspectReturnsNilLabelsWhenContainerHasNone`

All pre-existing tests continue to pass. `gofmt -l internal/ cmd/` empty,
`go vet ./...` clean, `grep -rn '%w: %v' internal/ cmd/` empty.

## Deviations from the tech plan

None substantive. Two cosmetic notes:

1. The probe-failure block hoists `cancelled := errors.Is(err, ...)` to a
   local once and reuses it for both the log fork and the return wrap, per
   Joel's "Rob: this is fine. Don't extract... If Linus pushes back on
   duplication, hoist..." note in §3.4.1. The duplication was awkward enough
   on read that I took the hoist option Joel pre-approved. No semantics
   change.

2. The pre-existing `TestCLIDriver_InspectArgsAndParse` test fixture was
   migrated from whitespace to JSON shape inline (one test). This was called
   out as Rob's responsibility in §5.11 items 1–2 and Kent's report. Test
   semantics unchanged: still asserts `cid12345`/`running` parse correctly,
   plus the existing `inspect` argv shape and `decloud-foo` argument
   propagation. No new assertions added.

— Rob
