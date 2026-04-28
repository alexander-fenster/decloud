# Raymond's docs — deploy cleanup on interrupt

Status: docs updated to reflect what shipped in `009-rob-impl.md`. Operator-facing changes in `_docs/usage.md`; four reusable patterns added to `_ai/`; `MEMORY.md` index updated.

## Files modified

### `_docs/usage.md`

- §3 exit codes — added a row for `130 / ExitInterrupted` with the POSIX rationale and a note distinguishing it from `ExitReadinessFail`.
- §3 `ExitRunFail` row — added a clause noting the new label-mismatched-orphan refusal path that returns 40, with a forward link to §8.
- §2 step 3 — extended to mention the defensive orphan check on a no-registry-entry deploy and the label-gated refusal.
- §8 (new) — "Interrupting a deploy (ctrl+c)". Documents: `ExitInterrupted = 130`, `Info`-level audit-log shape on cancellation vs `Error` on real readiness failure, the 30s cleanup window independent of the cancelled request ctx, the auto-recovery on next deploy via the labelled-orphan branch, the literal `docker rm -f decloud-<name>` recovery hint when label gating refuses a foreign container, and the SIGKILL/power-loss residual case that the same orphan branch covers.

## Files added

### `_ai/cleanup-context-discipline.md`

Pattern: orchestrator cleanup must not depend on the user-cancellable ctx. Documents the helper `newCleanupContext()` (`internal/deploy/service.go:32-42`), the three cleanup blocks where it's applied (probe-fail, run-fail, save-fail), the audit-log fork that pairs with it, the cancellation re-wrap as `ErrInterrupted`, and the cross-reference to backlog item 7 (caddy/manager.go).

### `_ai/label-gated-orphan-recovery.md`

Pattern: recover orphaned named artefacts on next run by gating on a creator-set label, not on the name alone. Documents the `decloud.service=<name>` label set at `cli_driver.go:61`, the three-way branch (absent / labelled-match / refuse), the verbatim refusal string, the `dockerdrv.InspectResult.Labels` field addition and JSON `--format` migration, and why the registry-only gate Linus rejected was insufficient.

### `_ai/exit-code-sentinel-not-context-err.md`

Pattern: CLI exit-code mapping matches the package sentinel only, not bare `context.*`. Documents the choke-point shape (`Deploy` is the only place that converts ctx errors to `ErrInterrupted`), the `ExitCodeFor` switch site, and the negative test rows (`context-canceled-bare`, `context-deadline-bare` → `ExitInternal`) that lock the contract.

### `_ai/gomock-fifo-matching.md`

Pattern: gomock matches expectations FIFO, not LIFO; harness `AnyTimes()` defaults need an explicit opt-out option. Documents Kent's empirical finding on `go.uber.org/mock@v0.4.0/gomock/callset.go:96-112`, the `withoutInspectAbsentDefault()` option, the rejected alternatives (`gomock.Not`, godoc-precedence assumption, drop-the-default), and cross-references `gomock-inorder-sequencing.md`.

### `_ai/MEMORY.md`

Index updated with one-line entries for each of the four new pattern files, in the existing "Implementation patterns (reusable)" section.

## Files deliberately NOT changed

- `_docs/install.md` — install steps are unaffected; the cleanup behaviour lives in `usage.md` where the `deploy service` command is documented.
- `_ai/m1x-backlog.md` — the planning step already added items 7 and 8 (caddy/manager.go cleanup-context, restoreOldContainer error surfacing). Nothing further to record.
- `_ai/decisions/` — none of the changes are architectural; they're reusable patterns. Patterns belong at the top level, decisions belong in the subdirectory.
- `CLAUDE.md` — out of scope per the user's hard rules.
- No CHANGELOG.md exists in `_docs/`; per the user's hard rules, did not add one.
- `_ai/gomock-inorder-sequencing.md` — kept untouched. The new `gomock-fifo-matching.md` cross-references it; the FIFO gotcha is a separate concern from `InOrder` ordering and merits its own focused file.

## Self-audit

I verified every symbol I cited via grep:

- `ErrInterrupted` (sentinel) — `internal/deploy/service.go:29`.
- `ExitInterrupted = 130` — `internal/cli/exit_codes.go:23`.
- `ExitCodeFor` switch case for `deploy.ErrInterrupted` — `internal/cli/exit_codes.go:37-38`.
- `cleanupTimeout = 30 * time.Second` and `newCleanupContext()` — `internal/deploy/service.go:32-42`.
- Cleanup site line ranges in `service.go` (run-fail ~240, probe-fail 260-287, save-fail ~323): all verified by reading the file in the cited ranges.
- Defensive-orphan branch range (`service.go:198-227`) and the parallel `hasPrev` branch (`service.go:185-197`): verified.
- Verbatim refusal string at `service.go:209`: verified character-by-character.
- Audit log strings ("removed orphan container from prior interrupted deploy", "deploy cancelled during readiness wait", "cleanup failed; please remove ... manually", "readiness failed"): all greppable hits in `service.go`.
- `decloud.service=` label set at `cli_driver.go:61`: verified, including the `strings.TrimPrefix(req.Name, "decloud-")` detail (label value strips the container-name prefix; the orchestrator's `req.Name` matches that stripped form, so the comparison is correct).
- `InspectResult.Labels map[string]string` field at `internal/dockerdrv/driver.go:43`: verified, including the comment "container labels; nil when State == "absent"".
- `cliDriver.Inspect` JSON `--format` template (`{"id":{{json .Id}},"state":{{json .State.Status}},"labels":{{json .Config.Labels}}}`): verified at `cli_driver.go:115`.
- Test names: `TestDeploy_ProbeCancellationCleansUpWithFreshContext`, `TestDeploy_DefensiveOrphanCleanupOnFreshDeployWhenContainerExists`, `TestDeploy_DefensiveOrphanCleanupSkippedWhenContainerAbsent`, `TestDeploy_DefensiveOrphanCleanupFailureWrapsErrRun`, `TestDeploy_RestoreOldContainerUsesFreshContextOnRedeployCancellation`, `TestDeploy_DefensiveOrphanRefusesContainerWithoutDecloudLabel`, `TestDeploy_DefensiveOrphanRefusesContainerWithMismatchedLabel`, `TestDeploy_DefensiveOrphanInspectCancelledReturnsErrInterrupted`, `TestExitCodeFor_AllSentinels`, `TestReadiness_ContextCancellationStopsProbe`, `TestCLIDriver_InspectParsesDecloudServiceLabel`, `TestCLIDriver_InspectReturnsNilLabelsWhenContainerHasNone` — all greppable.
- Test row names `interrupted`, `interrupted-wrapped`, `context-canceled-bare`, `context-deadline-bare` at `internal/cli/exit_codes_test.go:38-43`: verified.
- Harness symbols `withoutInspectAbsentDefault`, `harnessConfig`, `skipInspectAbsentDefault`, `cancellingProbe`, `passThroughProbe`, `notCancelledCtxMatcher` — all greppable in `internal/deploy/service_test.go`.
- Log file path `/opt/decloud/logs/decloud.log` — verified against `internal/logging/logging.go:14`.

— Raymond
