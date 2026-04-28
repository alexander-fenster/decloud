# Kent's tests — deploy cleanup on interrupt (v2.1)

Status: tests written, compile clean, fail with EXPECTED-failure modes against the unimplemented v2.1 contract. Ready for Rob.

Plans referenced: `02-plan.md` v2.1, `03-tech-plan.md` v2.1, `06-linus-review-v2.md`, `007-don-final-lockdown.md`.

## Summary

- 8 new tests added in `internal/deploy/service_test.go`.
- 1 existing test in `internal/deploy/readiness_test.go` tightened (and `strings` import dropped).
- 4 new table cases added to `internal/cli/exit_codes_test.go`.
- 2 new tests added in `internal/dockerdrv/cli_driver_test.go`.
- Harness extensions in `service_test.go`: `notCancelledCtxMatcher` / `notCancelledCtx()`, `cancellingProbe`, `newDeployerHarnessWithProbe(t, probe, ...opts)`, harness option `withoutInspectAbsentDefault()`, plus the `Driver.Inspect → absent` `AnyTimes()` default in `newDeployerHarness`.
- Stub-only declarations added in production code so tests reference real symbols (no behaviour change yet): `deploy.ErrInterrupted`, `cli.ExitInterrupted`, `dockerdrv.InspectResult.Labels`.

`go build ./...` is clean. `gofmt -l internal/ cmd/` is clean. `grep -rn '%w: %v' internal/ cmd/` returns zero. `go test ./...` fails only on the new tests (and the new exit-code table rows); all pre-existing tests pass.

## Stub declarations (compile-only, no logic)

- `internal/deploy/service.go:30` — added `ErrInterrupted = errors.New("deploy: cancelled by user")` to the existing var block. No wrap sites — Rob wires §3.4 / §3.5.
- `internal/cli/exit_codes.go:23` — added `ExitInterrupted = 130` constant. `ExitCodeFor` is unchanged — Rob adds the case in §3.6.
- `internal/dockerdrv/driver.go:42` — added `Labels map[string]string` field on `InspectResult`. Existing `cliDriver.Inspect` still parses whitespace, so the field is always nil at runtime — Rob switches the `--format` to JSON in §3.5.1.

## Harness changes (`internal/deploy/service_test.go`)

- L34–L82 — `cancellingProbe` (blocks on `<-ctx.Done()`, returns raw `ctx.Err()`), `notCancelledCtxMatcher` / `notCancelledCtx()` (gomock matcher, rejects when `ctx.Err() != nil` at call time).
- L99–L172 — `newDeployerHarness(t, opts...)` delegates to `newDeployerHarnessWithProbe(t, nil, opts...)`. The `Inspect(Any, Any) → InspectResult{State:"absent"}.AnyTimes()` default is registered unless `withoutInspectAbsentDefault()` is passed.
- L297 — `TestDeploy_StopOldFailureAbortsAndDoesNotStartNew` now uses `newDeployerHarness(t, withoutInspectAbsentDefault())`. **Empirical note for Linus's v2 §4 caveat:** gomock's `callset.FindMatch` iterates expectations in FIFO insertion order (verified in `go.uber.org/mock@v0.4.0/gomock/callset.go:96-112`), not LIFO as the v2 plan assumed. The harness default is therefore registered FIRST and would otherwise win against `TestDeploy_StopOldFailureAbortsAndDoesNotStartNew`'s test-body-registered `Inspect("decloud-foo") → "running"` expectation. The opt-out option keeps the §5.9 “11 mechanical edits collapse to one” outcome for the new and majority-existing tests, while letting that one test override the default surgically. Rob: continue to use this option for any future test that needs a non-absent Inspect on the request path.

## Tests added or modified

### 1. `TestReadiness_ContextCancellationStopsProbe` — UPDATED

`internal/deploy/readiness_test.go:143-167`. Tightened per §5.7: now requires `errors.Is(err, context.Canceled)` AND `!errors.Is(err, deploy.ErrReadiness)`. The v1 `strings.Contains(...)` fallback is removed; the `strings` import is dropped at L8.

Expected failure mode: **none today.** v2.1 leaves `readiness.go` unchanged (raw `ctx.Err()`), so this test currently passes and locks the contract going forward. If Rob accidentally regresses to `fmt.Errorf("readiness: %w", ctx.Err())`, the second assertion fires.

`go test -run TestReadiness_ContextCancellationStopsProbe`: `PASS` (5.01s).

### 2. `TestDeploy_ProbeCancellationCleansUpWithFreshContext` — NEW

`internal/deploy/service_test.go:364-388`. Combines §5.1 + §5.2 per Joel's "fold into 5.1" decision. Cancels the request ctx inside `Driver.Run`, expects `Stop` and `Remove` to receive a non-cancelled cleanup ctx (`notCancelledCtx()` matcher), expects the returned error to satisfy `ErrInterrupted` AND `context.Canceled` AND NOT `ErrReadiness`.

Expected failure: matcher rejects the cancelled context — `Got: context.Background.WithCancel; Want: is a context with Err() == nil at call time`. The current orchestrator at `service.go:215-216` passes the request ctx straight through. Rob's §3.4.1 fix (cleanup ctx derived from `context.Background()`) flips this to PASS.

### 3. `TestDeploy_DefensiveOrphanCleanupOnFreshDeployWhenContainerExists` — NEW

`internal/deploy/service_test.go:390-416`. §5.3 verbatim. `Inspect → running with decloud.service=foo`, expects `gomock.InOrder(Inspect, Stop, Remove, Run, ...)` for the full happy path with orphan cleanup before Run.

Expected failure: orchestrator skips the §3.5 orphan check and goes directly to `Run`, so gomock fails with `expected call ... doesn't have a prerequisite call satisfied: Remove ... should be called before Run`. PASS once Rob's §3.5 branch is in place.

### 4. `TestDeploy_DefensiveOrphanCleanupSkippedWhenContainerAbsent` — NEW

`internal/deploy/service_test.go:418-440`. §5.4 verbatim. `Inspect → absent`, expects InOrder `Inspect → Run → ContainerIP → Save → ...` (no Stop/Remove). Explicit `Stop(...).Times(0)` and `Remove(...).Times(0)` lock the no-op contract.

Expected failure: orchestrator never calls Inspect, so the InOrder prerequisite "Inspect should be called before Run" fires. PASS once §3.5 is implemented and short-circuits on `state == "absent"`.

### 5. `TestDeploy_DefensiveOrphanCleanupFailureWrapsErrRun` — NEW

`internal/deploy/service_test.go:442-465`. §5.5 verbatim. `Inspect → running, label match`, `Stop` returns `daemon hung`. Asserts `errors.Is(err, deploy.ErrRun)`, `errors.Is(err, stopErr)`, error message contains `docker rm -f decloud-foo`.

Expected failure: orchestrator never calls Inspect / Stop, calls `Run` directly which is unexpected — gomock reports `Unexpected call to Run ... there are no expected calls`. PASS once §3.5 stop-failure path is in place with the recovery hint string.

### 6. `TestDeploy_RestoreOldContainerUsesFreshContextOnRedeployCancellation` — NEW

`internal/deploy/service_test.go:467-496`. §5.6 verbatim. Uses `newDeployerHarnessWithProbe(t, cancellingProbe{})` + `prev` redeploy; cancels inside the new `Run`. Expects cleanup `Stop`, `Remove`, **and the rollback `Run`** to all receive `notCancelledCtx()`. Asserts the rollback `Run` is invoked with `prev.Config.Build.ImageRef`. Asserts `errors.Is(err, deploy.ErrInterrupted)`.

Expected failure: `Got: context.Background.WithCancel; Want: is a context with Err() == nil at call time` on the cleanup `Stop`. Defect B from Don's §1.3. PASS once §3.4.1 + §3.4.4 (`restoreOldContainer` parameter rename to `cleanupCtx`) are in place.

### 7. `TestDeploy_DefensiveOrphanRefusesContainerWithoutDecloudLabel` — NEW

`internal/deploy/service_test.go:498-522`. §5.10 first variant. `Inspect → running, Labels = {"some.other.label": "value"}`. Asserts `Stop/Remove/Run` are never called and that the error satisfies `ErrRun` AND mentions both `was not created by decloud` and `docker rm -f decloud-foo`.

Expected failure: orchestrator skips the §3.5 branch entirely and calls `Run` (which is `Times(0)`), so gomock reports `Unexpected call to Run`. PASS once §3.5's label-gate refusal path lands.

### 8. `TestDeploy_DefensiveOrphanRefusesContainerWithMismatchedLabel` — NEW

`internal/deploy/service_test.go:524-548`. §5.10 second variant. `Labels = {"decloud.service": "bar"}` while `req.Name == "foo"`. Asserts `ErrRun` plus that the error message surfaces the offending label value (`decloud.service="bar"`) and the phrase `does not match`.

Expected failure: same as test 7 — orchestrator calls `Run` unexpectedly. PASS once §3.5's label-gate refusal correctly formats the error with `%q`-quoted offending value.

### 9. `TestDeploy_DefensiveOrphanInspectCancelledReturnsErrInterrupted` — NEW

`internal/deploy/service_test.go:550-606`. §5.10.1 verbatim, table-driven across the three cancellation sites Joel called out (`inspect-cancelled`, `stop-cancelled`, `remove-cancelled`) per Don's "table-driven recommended" note. Each row injects `context.Canceled` from the corresponding driver call and asserts `errors.Is(err, deploy.ErrInterrupted)` AND `!errors.Is(err, deploy.ErrRun)`.

Expected failure: all three subtests fail with `Unexpected call to Run` (orchestrator never reaches §3.5). PASS once §3.5's three cancellation pre-checks (six lines, three sites) are in place per Don's v2.1 lockdown.

### 10. `TestExitCodeFor_AllSentinels/{interrupted, interrupted-wrapped, context-canceled-bare, context-deadline-bare}` — NEW

`internal/cli/exit_codes_test.go:38-41`. Four new table cases added.

Expected failures: **only the two `interrupted` rows.** `expected: 130; actual: 70` — `ExitCodeFor` falls through to `ExitInternal` because there is no `errors.Is(err, deploy.ErrInterrupted)` case yet. PASS once Rob adds the §3.6 case. The two `*-bare` negative rows already PASS today (bare `context.Canceled` / `context.DeadlineExceeded` route to `ExitInternal`); they lock that contract per Linus's "take them, cheap insurance" v2 review.

### 11. `TestCLIDriver_InspectParsesDecloudServiceLabel` — NEW

`internal/dockerdrv/cli_driver_test.go:267-277`. §5.11 item 3. Scripted stdout `{"id":"cid12345","state":"running","labels":{"decloud.service":"foo"}}`. Asserts `res.Labels["decloud.service"] == "foo"`.

Expected failure: current `cliDriver.Inspect` parses `strings.Fields` and returns `docker inspect: unexpected output "{\"id\":..."`. PASS once Rob switches the `--format` template to JSON and adds the `json.Unmarshal` parser.

### 12. `TestCLIDriver_InspectReturnsNilLabelsWhenContainerHasNone` — NEW

`internal/dockerdrv/cli_driver_test.go:279-288`. §5.11 item 4. Scripted stdout with `"labels":null`. Asserts `res.Labels == nil`.

Expected failure: same `unexpected output` path as test 11. PASS once the JSON parser handles a literal `null` (which `json.Unmarshal` correctly produces a nil `map[string]string` for, per Joel's note).

## Pre-existing tests confirmed unchanged & still passing

`internal/deploy`: `TestDeploy_HappyPathFirstDeploy`, `TestDeploy_HappyPathRedeploy`, `TestDeploy_LoadPreviousErrSecretsMissingTreatedAsFirstDeploy`, `TestDeploy_EnvCaptureFailureAbortsBeforeAnythingChanges`, `TestDeploy_BuildFailureAbortsBeforeStoppingOld`, `TestDeploy_StopOldFailureAbortsAndDoesNotStartNew` (with the `withoutInspectAbsentDefault()` opt-out), `TestDeploy_RunNewFailureRollsBackToOld`, `TestDeploy_ReadinessFailureRollsBackToOld`, `TestDeploy_SaveErrPartialWriteRollsBackAndDeletesOrphanConfig`, `TestDeploy_SaveFailsBeforePartialWriteSkipsDeleteOrphanConfig`, `TestDeploy_CaddyValidateFailureLeavesOldFileAndKeepsNewContainer`, `TestDeploy_CaddyReloadFailureDoesNotRollBackContainer`, `TestDeploy_CaddyValidateFailureMentionsCaddyUpRecovery`, `TestDeploy_CaddyReloadFailureMentionsCaddyUpRecovery`, `TestDeploy_DeployIDIsStableThroughoutOneDeploy`, `TestDeploy_CapturedEnvNotLoggedAsKeysOrValues`, `TestDeploy_RunRequestUsesCapturedEnvAndDecloudNetwork`, `TestDeploy_NoEnvScript_SkipsCapturerEntirely`, `TestDeploy_NetworkEnsureCalledFirst`, `TestDeploy_NetworkEnsureFailureReturnsErrRun`, `TestDeploy_BuildErrorPreservesInnerSentinel` — all PASS.

`internal/dockerdrv`: existing `TestCLIDriver_InspectArgsAndParse` / `TestCLIDriver_InspectAbsentContainerReturnsAbsentState` PASS unchanged. Per §5.11 items 1–2, Rob will rewrite their argv expectation and stdout fixture when switching to the JSON `--format`; not Kent's responsibility.

## Actual `go test` summary

```
ok    internal/caddy
ok    internal/config
ok    internal/envcap
ok    internal/ids
ok    internal/logging
ok    internal/registry
FAIL  internal/cli (TestExitCodeFor_AllSentinels: 2 of 21 sub-cases — interrupted, interrupted-wrapped)
FAIL  internal/deploy (8 of N — all 8 new tests, including 3 sub-cases of TestDeploy_DefensiveOrphanInspectCancelled)
FAIL  internal/dockerdrv (2 of N — TestCLIDriver_InspectParsesDecloudServiceLabel, TestCLIDriver_InspectReturnsNilLabelsWhenContainerHasNone)
```

All failures are EXPECTED. None are compile errors. Pre-existing tests continue to pass.

## Notes for Rob

1. **Order of implementation per Joel's P.P.S. v2** still applies; the harness `AnyTimes()` default Joel mentioned in step 3 is already in place from this commit. You should NOT need to add per-test Inspect expectations; the only test that opts out (`TestDeploy_StopOldFailureAbortsAndDoesNotStartNew`) is already opted out via `withoutInspectAbsentDefault()`.
2. **gomock FIFO behaviour** caught us — see harness change note above. If a future test needs a non-`absent` Inspect on the request path, use the same opt-out.
3. **Test 9 (`TestDeploy_DefensiveOrphanInspectCancelledReturnsErrInterrupted`)** — three subtests cover three sites. Don's v2.1 fix is "three sites, six lines"; getting only two right will leave one subtest red.
4. **Test 11 / 12 stdout fixtures** use the exact JSON shape Joel sketched in §3.5.1. If your `--format` template differs, update the fixtures to match — but keep the JSON envelope so the real-docker shape is still asserted.
5. **No production code logic changed**; the three stub declarations (`ErrInterrupted`, `ExitInterrupted`, `InspectResult.Labels`) compile clean and have zero call sites today, so they can't accidentally affect production behaviour. They're there so Kent's tests reference real symbols.

— Kent
