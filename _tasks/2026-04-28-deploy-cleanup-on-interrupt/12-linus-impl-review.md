# Linus's high-level implementation review — deploy cleanup on interrupt

## Verdict: APPROVE

The implementation lines up with the v2.1 spec. The headline bug is fixed, the cleanup-context discipline is applied at every site Joel and Don nailed down, the cancellation-symmetry lockdown at §3.5 is wired in correctly at all three sites, the orchestrator audit-log fork is in the right package, and the tests verify observable contracts rather than ceremony. Build/vet/gofmt clean; full suite green; the `%w: %v` grep returns nothing. Kevlin's two trivial nits (doc line-range typo, past-tense audit log) are real but not blocking.

The codebase is structurally better than it was. Rob did not freelance.

What follows is a high-level audit of strategic decisions, not a re-review of Kevlin's low-level pass.

---

## What the implementation actually shipped, vs what was promised

### Cleanup-context discipline — applied at every promised site

Verified by reading `internal/deploy/service.go` end-to-end:

- **Probe-failure cleanup** (lines 260–287): `cleanupCtx, cleanupCancel := newCleanupContext()`; Stop, Remove, restoreOldContainer all run on `cleanupCtx`. Audit-log fork (Info on cancellation, Error on real failure) wired correctly. Cancellation re-wraps as `ErrInterrupted`; `ErrReadiness` preserved on real failure paths. Matches §3.4.1 of the tech plan verbatim. ✓
- **Save-failure cleanup** (lines 321–345): same shape — `cleanupCtx` derived once, reused for `DeleteOrphanConfig`, Stop, Remove, restoreOldContainer. Cancellation re-wraps as `ErrInterrupted`. Matches §3.4.2. ✓
- **Run-failure rollback** (lines 237–248): `cleanupCtx` derived only when `hasPrev`; restoreOldContainer runs on it. Cancellation re-wraps as `ErrInterrupted`. Matches §3.4.3. ✓
- **`restoreOldContainer` parameter rename** (line 356): the parameter is now `cleanupCtx`, self-documenting at every call site. Joel's preference, Rob took it. ✓

### §3.5 defensive orphan cleanup with label gating + cancellation discrimination

`internal/deploy/service.go:198–227`. Inspect → if absent, no-op; if present and `Labels["decloud.service"] == req.Name`, log + Stop + Remove on the request `ctx` (forward progress); if missing/mismatched label, refuse with the manual-`docker rm -f` recovery hint and `%q`-quote the offending label value so the user can see what was wrong. All three driver-call sites (Inspect, Stop, Remove) check `errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)` BEFORE the `ErrRun` wrap, so a ctrl+c during the orphan check surfaces exit 130 not exit 40 — the v2.1 lockdown.

The label-strip detail (`cli_driver.go:61` — `strings.TrimPrefix(req.Name, "decloud-")`) means the label value is the bare service name (e.g. `"foo"`), and the orchestrator compares `req.Name` (also bare, e.g. `"foo"`) against `inspect.Labels["decloud.service"]`. Symmetric. The label gate works. ✓

### Driver-side migration

`InspectResult.Labels map[string]string` field added; `cliDriver.Inspect` now uses a JSON `--format` template and `json.Unmarshal`s the output. Whitespace-vs-JSON migration is the right call — labels can carry spaces, `=`, quotes, and the old `strings.Fields` parser was a footgun waiting to misfire. Existing `InspectResult{State: "absent"}` literals continue to compile (Labels zero-values to nil). Mocks needed no regeneration. `lifecycle.go` correctly continues to ignore `inspect.Labels` (it doesn't care). ✓

### Exit-code map

`internal/cli/exit_codes.go:37-38`. Single `case errors.Is(err, deploy.ErrInterrupted): return ExitInterrupted` placed BEFORE `errUsage` and all the `deploy.Err*` cases. Cancellation that wraps both `ErrInterrupted` and (theoretically) `ErrReadiness` correctly routes to 130 not 50. The bare `context.Canceled` and `context.DeadlineExceeded` correctly fall through to `ExitInternal`; Kent's `context-canceled-bare` and `context-deadline-bare` table rows lock that contract. The `context` import stayed out of `exit_codes.go` (only the test file imports it). ✓

### Audit-log fork

`service.go:262–266`. `cancelled := errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)` hoisted to a local once, used for both the log fork and the return wrap. Joel pre-approved the hoist; Rob took it where the duplication was awkwardest (two uses in a 25-line block) but did not extract a package-level helper for the six total sites. That's Kevlin's stylistic finding (#3); not a strategic decision worth re-litigating.

### Probe untouched

`readiness.go` is unchanged. The probe returns raw `ctx.Err()`. The audit-log decoration moved entirely to the orchestrator. This is right — the probe shouldn't know the difference between "user pressed ctrl+c" and "deploy was timed out by some future caller's `WithTimeout`"; the orchestrator has the deploy-id and step name and is the only place that can write a meaningful audit line. Linus Issue 3, fully resolved. ✓

---

## Tests — proportionate, not a fortress

12 new/updated tests: 8 in `service_test.go`, 1 tightened in `readiness_test.go`, 4 table rows in `exit_codes_test.go`, 2 in `cli_driver_test.go`. Each test asserts an observable contract:

- "Cleanup `Stop`/`Remove` receive a context whose `Err() == nil` at call time" (`notCancelledCtxMatcher`) — this is the observable shape of "cleanup runs on a fresh ctx", not a check that `service.go` happens to call `newCleanupContext()`. Passes any conforming implementation; rejects any cleanup-on-cancelled-ctx regression.
- "Refusal error chain satisfies `errors.Is(err, ErrRun)` AND contains `docker rm -f decloud-foo` substring" — the substring is the user's recovery hint and is the actual contract; an implementation that returned `ErrRun` without the hint would fail rightfully.
- "Cancellation table-driven test covers all three §3.5 sites (inspect, stop, remove)" — three subtests, three lines of coverage for three lines of production code. Proportionate; not over-tested.
- The two negative table rows (`context-canceled-bare` → `ExitInternal`) are the cheap insurance I asked for in v2 §"Issue 2"; they lock the contract against a future maintainer "helpfully" re-adding bare-`context.*` matching.

The harness extension (`cancellingProbe`, `notCancelledCtxMatcher`, `newDeployerHarnessWithProbe`, `withoutInspectAbsentDefault()`) is small (≈40 lines) and reused by exactly the tests that need it. Not a fortress. The new helpers earn their keep.

**One empirical correction worth recording:** I claimed in `06-linus-review-v2.md` §"Issue 4" that gomock's matcher precedence is LIFO based on the godoc + `WithOverridableExpectations` reading. Kent verified at `go.uber.org/mock@v0.4.0/gomock/callset.go:96-112` that it's actually FIFO. The harness's `Inspect(Any, Any) → absent` `AnyTimes()` default therefore had to be combined with an explicit `withoutInspectAbsentDefault()` opt-out at the seven tests that want a non-absent Inspect on the request path. Net: my "11 edits collapse to one" framing was wrong — it's "1 harness edit + 7 opt-outs = 8 total". Still better than 11 mechanical paste-ins, still the right call, and Raymond captured the gomock-FIFO gotcha in `_ai/gomock-fifo-matching.md` so the next person doesn't trip. Mea culpa, not a defect.

---

## User experience — does ctrl+c DTRT?

Walked through the spec'd flows mentally:

**First-deploy interrupted during readiness wait:**
1. SIGINT cancels request `ctx`.
2. Probe returns `ctx.Err()` = `context.Canceled`.
3. Orchestrator detects cancellation, logs `Info` "deploy cancelled during readiness wait" (NOT `Error` "readiness failed").
4. Cleanup ctx derived from `context.Background()` with 30s budget.
5. `Stop(cleanupCtx, ...)` and `Remove(cleanupCtx, ...)` actually execute against docker — no more silent `_ = ` swallowing.
6. Returned error wraps both `ErrInterrupted` and `context.Canceled`.
7. CLI `ExitCodeFor` matches `ErrInterrupted` → exit 130. POSIX-correct.
8. `docker ps -a | grep decloud-foo` returns no row.
9. The next `decloud deploy service --name foo` succeeds end-to-end.

**Same scenario, but cleanup itself has trouble:**
- `Stop` fails with non-`ErrContainerNotFound` error → `logger.Warn("cleanup failed; please remove decloud-foo manually", ...)`. `Remove` still attempts. The deploy still exits with the original error (`ErrInterrupted`). User sees the warning and knows the recovery action.
- On the next deploy, §3.5 detects the orphan, verifies the `decloud.service=foo` label, removes it, logs `Warn` "removed orphan container from prior interrupted deploy", proceeds.

**Mismatched-label refusal:**
- A container named `decloud-foo` exists with no `decloud.service` label (manual `docker run` by the operator, or some other tool).
- §3.5 inspects, computes `labelVal = ""`, sees `"" != "foo"`, returns `ErrRun` with the manual recovery hint quoted with `%q`. Exit 40. Test 7 (`TestDeploy_DefensiveOrphanRefusesContainerWithoutDecloudLabel`) locks this.
- The operator removes the foreign container by hand or picks a different `--name`. Documented in `_docs/usage.md` §8.

**SIGKILL / power loss:**
- No in-process cleanup runs. The orphan exists with the `decloud.service=<name>` label intact. The next deploy's §3.5 catches it.

**Race: SIGINT between `Run` returning success and probe entering its loop.**
- The probe calls `Driver.ContainerIP(ctx, ...)` first. If `ctx` is already cancelled, the driver's `exec.CommandContext` returns immediately with `context.Canceled`. `lastErr = ipErr`. Loop checks deadline (just started, far future). Enters `select`; `<-ctx.Done()` fires immediately; returns `ctx.Err()`. Orchestrator handles cancellation correctly.
- This race is NOT directly tested (the `cancellingProbe` test stub bypasses to `<-ctx.Done()` without going through `ContainerIP`). However, the integration path is covered indirectly: any path through the probe that ends in `ctx.Err()` produces the right downstream behavior, and the existing `TestReadiness_ContextCancellationStopsProbe` covers the real probe's behavior. Acceptable.

**Race: SIGINT during `Driver.Run` itself.**
- `exec.CommandContext` SIGKILLs the docker CLI client. The docker DAEMON may have already received the run request and started the container. The CLI returns `context.Canceled`. The orchestrator's run-failure block handles cancellation correctly (line 244–246 wraps as `ErrInterrupted`).
- BUT: the new container may be alive on the host with no registry entry. The §3.5 defensive cleanup on the next deploy catches this — same recovery as the SIGKILL case. Working as designed.

**The user experience is correct. The failure mode I worried about most — a "you fixed it but the user still sees exit 40 if their ctrl+c lands during the orphan inspect" — was the v2.1 lockdown, and Don/Rob/Kent shipped it. Three sites, six lines, one table-driven test. Done.**

---

## Strategic concerns

### IDENTIFIED ISSUES

#### Issue 1: §3.4.5 (`hasPrev` redeploy stop+remove) has the same shape of cancellation-mis-wrap that v2.1 fixed in §3.5

**Problem**: `service.go:185–197` — the `hasPrev` branch that runs on a planned redeploy stops and removes the previous container on the request `ctx`. If the user ctrl+c's during this window, `Stop` returns `context.Canceled`, the inner Inspect at :188 also returns `context.Canceled`, `ierr != nil` so the "is it still running" check is skipped, fall-through to `Remove(ctx, ...)` at :195 which also returns `context.Canceled`, which is not `ErrContainerNotFound`, so the function returns `fmt.Errorf("%w: remove previous container: %w", ErrRun, err)`. User sees exit 40 ("run failure"), not exit 130 ("interrupted").

This is the IDENTICAL shape of bug that v2.1 fixed at §3.5 (the orphan-inspect path). Same instinct, same fix shape, sibling code. The two adjacent forward-progress branches now have ASYMMETRIC cancellation contracts:
- §3.5 (`!hasPrev` orphan check): cancellation → `ErrInterrupted` → exit 130.
- §3.4.5 (`hasPrev` redeploy stop+remove): cancellation → `ErrRun` → exit 40.

A user who ctrl+c's a fresh first deploy during orphan check sees 130. A user who ctrl+c's a redeploy during the old-container stop sees 40. Same key combo, same intent, two different exit codes.

**Impact**: User-visible inconsistency. Low frequency (the redeploy stop+remove window is short — usually <1s), but the lockdown logic that justified §3.5's six-line fix applies here verbatim. "The whole point of this task is getting cancellation semantics right. Shipping with a known cancellation-mis-wrap inside the same task that's specifically about cancellation discipline would be cowardice." (Don, `007-don-final-lockdown.md`.) The same standard applies.

This was NOT raised as a v2 review item — I missed it. My fault, not Don's, not Rob's. Calling it out now because it's the right thing to do.

**Options**:
- **Option A (Minimal — fix in scope NOW)**: Add the cancellation discrimination to the two return paths at lines 190-191 and 195-196. ~6 lines, two sites. Mechanical, mirrors §3.5 exactly. Pros: cancellation contract is uniform across the entire `Deploy` orchestrator; future-Don doesn't revisit at 2 AM. Cons: one more iteration; we're already at v2.1.
- **Option B (Backlog — punt to follow-up)**: Add an `m1x-backlog.md` item alongside items 7 and 8. Pros: ships now, exits the loop. Cons: same code path, two trips to the same file, future-user reports exit 40 when they expected 130 during a redeploy interrupt.
- **Option C (Defer — accept the asymmetry as documented)**: Add a sentence to `_docs/usage.md` §8 noting that ctrl+c during a redeploy old-container stop surfaces exit 40 not 130. Pros: zero code change. Cons: documented inconsistency is still inconsistency; users won't read the doc; the audit log fork at §3.4.1 already lies (Info vs Error) about the same cause.

**My Recommendation**: Option A. The v2.1 lockdown precedent says "do it now while the code is fresh." Six lines, two sites, one new test row mirroring `TestDeploy_DefensiveOrphanInspectCancelledReturnsErrInterrupted` against the §3.4.5 sites. Total cost: ~30 minutes including the test. The diff is mechanical enough that Kevlin's review on the round trip would be five minutes.

If Don picks Option B, I won't block — it's narrow enough — but the precedent is uncomfortable.

**DON's Decision Required**: Pick A, B, or C. If A, route to Rob with a tight scope. If B, route to Andy for a backlog item.

#### Issue 2: `_docs/usage.md` line 240 is technically inaccurate — second ctrl+c does NOT bypass cleanup

**Problem**: The doc claims "A second ctrl+c (impatient double-tap) bypasses graceful cleanup and may leave the container behind." This is false. Per `signal.NotifyContext` semantics (verified against `pkg.go.dev/os/signal#NotifyContext`), the package's signal handler stays registered until `stop()` is called in `cmd/decloud/main.go:15`'s defer. `stop()` only fires after `cmd.ExecuteContext(ctx)` returns. So during cleanup, the handler is still active and absorbs the second SIGINT; it does NOT propagate to the OS default handler. The user has to wait for the 30s `cleanupCtx` timeout or `kill -9` the process. They cannot bypass cleanup with a second ctrl+c.

**Impact**: Misleading user documentation. A user who reads §8 and double-taps ctrl+c expecting to bail out will sit there frustrated, eventually `kill -9`. The intent the doc tries to communicate ("decloud doesn't make double-ctrl+c convenient") is correct; the mechanism it claims (second ctrl+c leaves cleanup behind) is wrong.

**Options**:
- **Option A (Minimal — fix the doc text)**: Change line 240 to: "A second ctrl+c during cleanup does not interrupt cleanup; Go's signal handler absorbs it. To force exit immediately, send SIGKILL (`kill -9 <pid>`). The orphan recovery in path (1) above still applies." One sentence change. Pros: accurate. Cons: trivially more verbose.
- **Option B (Add second-signal handling to `cmd/decloud/main.go`)**: Wire a watcher that, on the SECOND signal, calls `stop()` and exits non-gracefully. Pros: doc claim becomes true; user gets the bail-out they expect. Cons: real code change, real test surface, scope creep on a doc nit.
- **Option C (Defer — leave as-is)**: Accept that the doc is mildly wrong; nobody reads §8 anyway. Pros: zero change. Cons: documented falsehood.

**My Recommendation**: Option A. One-sentence doc fix; Raymond can do it in 30 seconds. Option B is the right product behavior eventually but not in scope for this task — file as a backlog item if Don wants the real fix.

**DON's Decision Required**: A or C; if A, route to Raymond. If Don wants the proper second-signal behavior (B), file it as a backlog item separately.

#### Issue 3: Kevlin's two trivial nits (doc line range, past-tense audit log)

Already raised by Kevlin in `011-kevlin-review.md` items 1 and 2. Both are <30 second fixes; neither blocks. I would take them inline before the merge but I won't insist. Don's call.

#### Issue 4: Six-site `errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)` repetition (Kevlin's #3)

Kevlin recommended a `func isCancellation(err error) bool` helper. I agree the idiom has earned a name (six occurrences is the threshold). Joel pre-approved this hoist in §3.4.1, Rob took it locally but not globally. Not blocking; trivially refactorable in a 5-line follow-up. File as a small "polish pass" backlog item, or leave for a future maintainer to extract when they touch the seventh site. Either is fine.

#### Issue 5: Kevlin's slog message-vs-field duplication (#4)

`logger.Warn("cleanup failed; please remove "+containerName+" manually", "container", containerName, "error", stopErr)` interpolates `containerName` into the message AND passes it as a structured field. Breaks grep-stability of the message string. Other slog calls in `service.go` use fixed message + structured field. Inconsistent. Kevlin recommends matching the convention; I agree. Four sites in `service.go` (lines 270, 274, 331, 335). No test changes. ~5 minute fix. Not blocking.

---

## Strategic non-issues (verified, not concerns)

- **Cleanup-context discipline correct end-to-end**: every cleanup site I traced — probe-fail Stop+Remove+restoreOldContainer, save-fail Stop+Remove+restoreOldContainer+DeleteOrphanConfig, run-fail-with-prev restoreOldContainer — uses `cleanupCtx`. The forward-progress sites — `NetworkEnsure`, `Capture`, `Load`, `Build`, `hasPrev` Stop+Remove (§3.4.5), §3.5 orphan check, `Run`, `probe.Wait`, `Save`, `regenerateAndReload` — use the request `ctx`. The split is principled and consistent.
- **Audit-log fork is correctly placed in the orchestrator**: this is the key shape question I asked in v2 ("does this leak policy from CLI to deploy package?"). The deploy package owns its own audit logging because it has the deploy-id, service-name, and step-name context that CLI doesn't. Logging Info-vs-Error is an audit decoration the orchestrator is uniquely positioned to make. Right placement.
- **Cancellation symmetry at three §3.5 sites + §3.4.3**: verified. Inspect (line 201), Stop (line 215), Remove (line 221), and run-failure rollback (line 244) all check `errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)` before falling through to the package-specific `ErrRun`/`ErrInterrupted` wrap. The lockdown landed everywhere it was supposed to. Test 9 (`TestDeploy_DefensiveOrphanInspectCancelledReturnsErrInterrupted`) is table-driven across all three §3.5 sites.
- **Driver interface widening is non-breaking**: the new `Labels` field on `InspectResult` zero-values to nil for existing callers. `lifecycle.go`'s `Start`/`Status` don't read it, no behavior change. Mocks didn't need regeneration. The `cli_driver.go` JSON `--format` migration is the right robustness call.
- **30s `cleanupTimeout`** is sized right: 10s `docker stop` grace + ~1s `docker rm` + ~1s `docker run` rollback + ~18s daemon-contention buffer. If a real ops report comes in saying it's too short, parameterize then; not now.
- **Test set is proportionate**: 12 new tests for ~50 lines of new production logic + a struct field. The new helpers (`cancellingProbe`, `notCancelledCtxMatcher`, `newDeployerHarnessWithProbe`, `withoutInspectAbsentDefault`) are small and well-justified. Future engineers will not resent this. No fortress.
- **No race between SIGINT and the request goroutine**: walked through the timing in §"User experience" above. Every window I could find lands somewhere safe.
- **Cleanup itself being interrupted by a second ctrl+c**: the `cleanupCtx` is derived from `context.Background()`, so it does NOT cancel on a second signal. Cleanup runs to completion or 30s timeout. Documented (incorrectly — see Issue 2 above).
- **No new mocks required**: confirmed by Kevlin and verified by reading `internal/dockerdrv/mocks/mock_driver.go`.
- **All `_ai/error-wrap-discipline.md` rules followed**: zero `%w: %v` in the diff. Sentinel-with-context shapes (`%w: <ctx>: %w`) used correctly in `service.go:204`, :218, :224, :523-equivalent.
- **`ExitCodeFor` ordering is right**: `ErrInterrupted` placed before all `deploy.Err*` cases ensures cancellation-with-an-inner-readiness-error (theoretical, doesn't happen today, defended against) routes to 130 not 50.

---

## What other agents got right (rare praise)

- **Don** stuck the v2.1 landing. The cancellation-symmetry call was the right call; "ship as-is, fix in follow-up" was the wrong call and Don knew it. The lockdown doc is two paragraphs and a deviation table. That's how you make decisions.
- **Joel** translated three review items in `04-linus-review.md` and one follow-up flag in `06-linus-review-v2.md` into a tech plan with verbatim code blocks, line-range citations, a failure-mode matrix, and an implementation order that lets `go test ./...` stay green at every checkpoint. The estimation-reality-check section (12 hours via π × 4) is the kind of honesty most planners skip.
- **Kent** caught the gomock FIFO surprise empirically and fixed it with the opt-out option rather than fighting it. Documented at `_ai/gomock-fifo-matching.md` so the next person doesn't trip. The negative test rows (`context-canceled-bare`, `context-deadline-bare` → `ExitInternal`) are exactly the cheap insurance I asked for.
- **Rob** followed Joel's P.P.S. v2 implementation order, took the local hoist Joel pre-approved, did NOT extract a function-level helper without clearance, did NOT change `readiness.go` (correctly), did NOT add behaviors not specified, did NOT touch lifecycle.go (correctly). The deviations report (`009-rob-impl.md` §"Deviations") cites exactly two cosmetic notes and zero substantive changes. That is the standard.
- **Raymond** wrote four focused `_ai/` pattern files (89, 86, 69, 57 lines), updated the `MEMORY.md` index, added a usable §8 to `_docs/usage.md`, and self-audited every line-range citation. Kevlin caught one off-by-two (out of dozens); the audit was thorough.
- **Kevlin** APPROVED with two doc/log nits and two stylistic follow-ups. His finding #4 (slog message-vs-field duplication) is a real consistency issue worth fixing; the others are minor. His "code reads better after this change" assessment is correct.

I do not generally write praise sections. The team executed.

---

## Recommendation

**APPROVE.** Issue 1 (§3.4.5 cancellation asymmetry) is the only strategic concern I'd consider blocking on, and even there I'm marginal — the precedent says fix it now, but the headline fix lands either way. Don picks Option A, B, or C; my recommendation is A.

If Don picks B or C for Issue 1 and A for Issue 2, the task is done after Raymond updates the doc and Andy files the backlog entries. If Don picks A for Issue 1, route to Rob with a tight scope (six lines, two sites, one table row), then back through Kevlin and me for a confirmation pass. Either way, this is closer to "ship it" than to "iterate again."

The cleanup-on-interrupt fix itself is solid. The user's headline complaint ("ctrl+c leaves an orphan") is fixed three ways: cleanup actually runs, exit code is correct, and the next deploy recovers any orphan that escaped — including from SIGKILL/power-loss, which the original report didn't even ask about. That's getting more from a fix than you put in. Rare and welcome.

— Linus
