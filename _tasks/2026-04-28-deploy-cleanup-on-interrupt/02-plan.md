# Don's plan — deploy cleanup on interrupt

Status: planning only. No code in this document. Joel writes the tech plan; Kent writes tests; Rob writes code. Linus reviews.

**Revision history:**
- v1: original plan.
- v2: Linus reviewed both this and Joel's tech plan and returned REVISE with six items. Resolutions for each are recorded inline below (see §12). Material strategy changes from v1 are §3.2 (probe wrap shape), §3.3 (defensive orphan label gating), and the new §12 (review resolutions). All other sections unchanged.
- v2.1 (this revision): Linus APPROVED v2 with one non-blocking flag: §3.5 wraps `context.Canceled` from Inspect/Stop/Remove as `ErrRun`, mismatched with §3.4.3's cancellation-discrimination pattern. Don's call: fix in scope (Option A). Six lines, three sites, one new test case. See `007-don-final-lockdown.md`.

---

## 1. The bug, traced end-to-end

The user reports: ctrl+c during the health-endpoint wait leaves a docker container behind, and the next deploy fails because the container name `decloud-<svc>` is already in use.

I traced it. The bug is real, the diagnosis is exact, and the fix surface is small.

### 1.1 Execution path on SIGINT during readiness wait

Entry point — `cmd/decloud/main.go:14`:

```
ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
```

This `ctx` is threaded through cobra to `runDeployService` (`internal/cli/deploy_service.go:54`), which calls `d.Deploy(ctx, req)` at `:111`. SIGINT cancels this `ctx`.

Inside `serviceDeployer.Deploy` (`internal/deploy/service.go:127`):

1. `NetworkEnsure` → ok
2. `Capture` → ok
3. `Store.Load` → on a first-time deploy, returns `registry.ErrNotFound`; `hasPrev = false`
4. `Driver.Build` → ok (`decloud-<svc>:<deploy-id>` image built)
5. `Driver.Run` → ok; container `decloud-<svc>` is now alive on the `decloud` network with `--restart unless-stopped`
6. `d.probe.Wait(ctx, ...)` blocks waiting for `/healthz`

User hits ctrl+c. `signal.NotifyContext` cancels `ctx`. Inside `httpProbe.Wait` (`internal/deploy/readiness.go:36`), the `select` at `:68-72` fires on `<-ctx.Done()` and returns **`ctx.Err()` raw — NOT wrapped in `ErrReadiness`** (`readiness.go:70`).

Back in `Deploy` at `service.go:213-224`:

```
if err := d.probe.Wait(ctx, ...); err != nil {
    _ = d.deps.Driver.Stop(ctx, containerName, 10*time.Second)
    _ = d.deps.Driver.Remove(ctx, containerName)
    if hasPrev { d.restoreOldContainer(ctx, prev) }
    ...
}
```

**These cleanup calls take the same `ctx` that was just cancelled.** Inside `cliDriver.Stop` (`internal/dockerdrv/cli_driver.go:73-85`) and `Remove` (`:100-111`), the call goes through `exec.CommandContext(ctx, "docker", ...)`. Go's `os/exec` contract: if the context is already cancelled when `cmd.Run()` is invoked, the child process is never started — `Run()` returns immediately with the context error. `docker stop` and `docker rm` **never execute on the host**. The errors are swallowed by the leading `_ =`.

Net effect: the container stays running. On the next deploy:

- First-deploy redo: `Store.Load` still returns `ErrNotFound` → `hasPrev = false` → no Stop/Remove of previous container → `Driver.Run` fails with `Conflict. The container name "/decloud-<svc>" is already in use`. **This is what the user observed.**
- Redeploy of an already-deployed service that was interrupted on its second deploy: `Load` returns the previous registry entry → the "if hasPrev" branch at `service.go:172-185` does try to `Stop` + `Remove` `decloud-<svc>` first, which would clear the orphan as a side effect. The user did not hit this case but it's only an accidental recovery — the orphan is the *new* deploy, not the *previous* one we have a record of, so any rollback intent is meaningless because we've lost the new container's state too.

### 1.2 Where the bug lives

**Server-side service**, not the client command. The CLI command does no signal handling beyond `signal.NotifyContext`; that's correct. The defect is in `internal/deploy/service.go`'s lifetime contract: it ties cleanup to the same context that triggers cleanup. That's a category error.

### 1.3 Two adjacent latent bugs found while tracing

I found two related defects in the same code path. Calling them out here so we fix them all in one task or punt explicitly with my sign-off.

**Latent bug A — readiness probe's context error escapes unwrapped.**
`readiness.go:70` returns `ctx.Err()` (i.e. `context.Canceled`). The deploy orchestrator at `service.go:220-223` checks `errors.Is(err, ErrReadiness)`. Cancellation is therefore not an `ErrReadiness`, and the wrapping fallthrough at `:223` produces `ErrReadiness: context canceled`. That's wrong on two counts: (i) cancellation isn't a readiness failure, it's a user-cancelled deploy, and (ii) the exit code mapping in `internal/cli/exit_codes.go` will treat it as a readiness exit. We should surface a distinct cancellation outcome (or at minimum a cleaner error chain) so a user can distinguish "I hit ctrl+c" from "the app failed its health check".

**Latent bug B — `restoreOldContainer` (`service.go:282-300`) takes the same cancelled `ctx`.**
On a redeploy that's interrupted during readiness, the rollback-restart of the previous container will also fail-fast on a cancelled context, leaving the user with no running container for that service at all. The orchestrator then proceeds to `regenerateAndReload` only if save succeeded, so the registry stays consistent with the *previous* deploy id, but the old container is *gone* (we removed it at step `:172-185`) and the new container is *alive but unhealthy* (we failed to remove it). Same root cause as the headline bug: cleanup must not depend on user-cancellable context.

These are the *same* shape of defect: cleanup paths sharing the request-scoped context. Fix them together.

---

## 2. Trade-off analysis: `--rm` vs explicit cleanup

The user asked us to evaluate `docker run --rm` as one possible mechanism. Here's the honest comparison.

### 2.1 `--rm` flag

Adds `--rm` to the `docker run -d` command at `cli_driver.go:46-71`. Docker auto-removes the container when its main process exits.

**Pros:**
- Self-cleaning if the process inside the container exits.

**Cons (disqualifying):**
- `--rm` only triggers when the **container's main process exits**. SIGINT to `decloud` does *not* exit the container's main process — the container keeps running happily. `--rm` does nothing for our scenario.
- We currently set `--restart unless-stopped` (`service.go:192`). `docker run` rejects `--rm` combined with `--restart` other than `no`. We'd have to drop the restart policy.
- Dropping `--restart` regresses normal-operation reliability: a healthy deployed service will not auto-restart after a docker daemon restart or host reboot.
- Even if we conditionally set `--rm` for the readiness window and then `docker update --restart=unless-stopped` after readiness passes, that's a multi-step dance that makes the steady-state guarantees weaker than what we have now, in service of a cleanup edge case that explicit cleanup handles cleanly.

**Verdict: rejected.** `--rm` is the wrong tool for SIGINT cleanup. It cleans up after process exit, not after caller cancellation.

### 2.2 Explicit cleanup with a non-cancellable context

The fix is structurally tiny: cleanup paths get a fresh `context.Background()`-derived context with a bounded timeout, instead of reusing the user-cancellable `ctx`. Combined with proper error propagation that distinguishes cancellation from failure, this addresses all three defects (headline + A + B) with one pattern.

**Pros:**
- Targeted fix to the actual defect (cleanup tied to user-cancellable context).
- Preserves `--restart unless-stopped` for normal operation.
- Pattern composes: same shape applies to `restoreOldContainer`, save-failure cleanup at `service.go:265-269`, partial-write cleanup at `:261-263`, and any future cleanup added.
- Bounded by an explicit timeout (10s grace + a small buffer) so a stuck docker daemon doesn't hang the CLI forever after ctrl+c.

**Cons:**
- Need to be careful: if the user double-ctrl+c (impatience), we still want to bail. The standard pattern is to wire the cleanup context to `os.Interrupt` *only after the first one* via `signal.Reset` or a second-signal channel. For M1 we accept that the second ctrl+c is a kill-9; the user's expectation when impatiently double-tapping ctrl+c is that we'll leak something. Document and move on.

**Verdict: this is the fix.**

### 2.3 Hybrid considered and rejected

What about `--rm` for the readiness window only, with an explicit cleanup as backup? Adds complexity (two mechanisms), still has the `--restart` conflict, doesn't help when the container's process is healthy-looking but the readiness HTTP isn't responding (the actual common case). Not worth it.

---

## 3. The fix — what changes and where

Three surfaces. Let Joel detail the precise function signatures; this section nails down behavior.

### 3.1 `internal/deploy/service.go` — cleanup context discipline

The `Deploy` method must distinguish two flavors of context:

- **Forward-progress context** — the caller's `ctx`. Cancellation should abort forward steps (build, run, probe, save, caddy reload). Status quo.
- **Cleanup context** — derived from `context.Background()`, with a timeout long enough to allow `docker stop -t 10s` plus a small buffer plus `docker rm` plus `docker run` (for restoreOldContainer). Suggested budget: 30s total. This context is **never** cancelled by user SIGINT. It IS bounded so a hung docker daemon doesn't pin the CLI forever.

Concretely the cleanup blocks at `service.go:215-216`, `:218`, `:265-269`, `:261-263` (DeleteOrphanConfig is registry, but same principle), and the body of `restoreOldContainer` (`:282-300`) all switch from the request `ctx` to a cleanup context.

The error at `:222-223` should preserve the cancellation cause (so callers can detect "user cancelled") and not bury it under `ErrReadiness`.

### 3.2 `internal/deploy/readiness.go` — ctx error discrimination

`httpProbe.Wait` at `:69-70` currently returns `ctx.Err()` raw. The orchestrator can't distinguish that from a probe failure.

**Decision (revised after Linus review):** the probe returns raw `ctx.Err()`. Joel's v1 §3.3 wrapped it as `fmt.Errorf("readiness: %w", ctx.Err())` for "audit prefix" reasons; Linus correctly pointed out that the prefix is misleading — the whole point of the change is to STOP treating cancellation as a readiness failure, and the v1 wrap reads "deploy: cancelled by user: readiness: context canceled" which inserts a confusing word into the user-visible chain. Raw `ctx.Err()` is cleaner.

The orchestrator side then does two things:
1. Detects cancellation via `errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)` and re-wraps as `ErrInterrupted` (§3.6 of Joel's tech plan, unchanged).
2. **Forks the audit log line at `service.go:214`** — currently `logger.Error("readiness failed", ...)` fires unconditionally on any probe error. After the fix, log "readiness failed" only when it actually failed (the `errors.Is(err, ErrReadiness)` or non-cancellation path), and log "deploy cancelled during readiness wait" at `slog.Info` (NOT Error) when the cause is cancellation. Two branches, three lines.

This keeps the probe's contract trivial (it returns ctx errors as ctx errors, readiness errors wrapped as ErrReadiness) and pushes the audit decoration to the orchestrator that has full context.

Joel: in your tech plan revision, drop the "readiness:" prefix in §3.3 and add the audit-log fork in §3.4.1.

### 3.3 `internal/deploy/service.go` — defensive orphan cleanup at deploy start

Even after we fix the cleanup-on-interrupt path, three failure modes still leak orphans:

- A SIGKILL (kill -9) of `decloud` between `Run` and `Save` skips ALL cleanup defers.
- A power loss between `Run` and `Save` (host crash) leaves a container running with no registry entry.
- An older `decloud` binary running a buggy version of the code on the same host.

The user *also* asked us to handle this case explicitly: "should a fresh deploy defensively clean up an orphaned container from a previous interrupted run".

Yes. Add a defensive pre-clean immediately before `Driver.Run` at `service.go:195`:

- If `Store.Load` returned `ErrNotFound` (i.e. `hasPrev == false`), AND a container named `decloud-<svc>` exists, AND **that container carries our `decloud.service=<svc>` label** — `Stop` + `Remove` it. Log a warning at `slog.Warn` level: "removed orphan container from prior interrupted deploy".
- If `Driver.Inspect(ctx, containerName)` returns `state == "absent"`, do nothing (the common case).
- **If the container exists but does NOT carry our label, refuse with a clear error:** "container `decloud-<svc>` exists but was not created by decloud; refusing to remove. Run `docker rm -f decloud-<svc>` manually if you want to claim this name, or pick a different service name." Wrap as `ErrRun`.
- Use the request `ctx` here — this is forward-progress, not cleanup, and the user CAN cancel it.

**This is the label-gating revision after Linus review.** v1 of this plan said "label/name does not correspond to any registry entry" but Joel's v1 tech plan softened to just "the registry has no entry for this name." Linus correctly flagged the gap: that softer check would silently destroy ANY container named `decloud-<svc>` whose registry entry happens to be missing — including a manually-`docker run`-named container or a still-running container after a user blew away their registry dir thinking they were "starting fresh." The label gate converts "we silently nuke any container with the right name" into "we only nuke containers we provably created." That's the right contract. The `decloud.service` label is already attached at `cli_driver.go:60`; we just need to inspect it.

**The driver interface change (small):** extend `dockerdrv.InspectResult` with a `Labels map[string]string` field. The `cli_driver.Inspect` implementation changes its `--format` string from `"{{.Id}} {{.State.Status}}"` to one that also emits labels (e.g. `--format='{{json .Config.Labels}} {{.Id}} {{.State.Status}}'` or two separate `docker inspect` calls — Joel/Rob pick the cleaner shape). This is a NEW field on an existing struct, NOT a new method, so the mock interface widens by one field but no new mock methods are needed. `go generate ./...` regenerates `mocks/mock_driver.go` to pick up the field — it should be a no-op for mocks because gomock generates per-method, not per-result-field.

**Important guardrail:** the defensive clean only runs when `hasPrev == false` (no registry entry). If `hasPrev == true`, the existing branch at `service.go:172-185` already handles stop+remove, and we MUST NOT double-act — the `hasPrev` path is the planned re-deploy and removing without verifying `hasPrev` would be a footgun. The two branches stay disjoint.

**Failure-mode coverage for the label gate (for the test plan):**
- Inspect → `state == "absent"`: no-op (common case).
- Inspect → state exists, label `decloud.service` matches `req.Name`: stop+remove, audit log, proceed.
- Inspect → state exists, label `decloud.service` is missing or mismatched: return `ErrRun` with the manual-`docker rm -f` recovery hint.
- Inspect/Stop/Remove returns `context.Canceled` or `context.DeadlineExceeded`: return `ErrInterrupted` wrapping the ctx error (exit 130). The §3.5 branch runs on the request `ctx` — the user CAN ctrl+c during orphan check, and they deserve "interrupted" semantics, not "run failure."
- Inspect itself errors (non-cancellation): return `ErrRun` wrapping the inspect error.

**v2.1 revision (Linus 06-review follow-up):** the three return-error sites in §3.5 (inspect, stop, remove) each detect cancellation BEFORE wrapping as `ErrRun`. Symmetric to §3.4.3. Six lines, three sites. Kent adds one cancellation table case; see Joel's §5.10.1.

### 3.4 Lifecycle paths — out of scope, confirmed

I checked `internal/deploy/lifecycle.go`. `Stop`, `Start`, `Restart`, `Unregister` operate on already-registered services. Their failure modes don't leak containers in the same way (the registry tracks them, so any future `Stop`/`Unregister` finds them by name). Not part of this fix. Linus will confirm or push back.

---

## 4. Cleanup ordering — stop, then remove, force where appropriate

Existing code at `service.go:215-216` does the right ordering: `Stop(ctx, name, 10*time.Second)` then `Remove(ctx, name)`. The 10s grace is correct for an app that may have an in-flight HTTP request. Keep it.

For the **defensive orphan cleanup** at deploy start, I want a slightly more aggressive shape: `Stop` with grace of 10s, fall through to `Remove` regardless. If `Stop` fails because the container isn't running (`exited`), `Remove` still works. We do NOT need `docker rm -f` (force-kill) here — `Stop` is graceful; if it fails for some other reason we'd rather surface the error than kill -9 a container the user might want to inspect.

**Exception — should we add `--force` removal?** Only if `Stop` succeeds but `Remove` fails with a non-NotFound error. That's a rare race (container restarting between stop and rm). I'd punt that to a follow-up; if Linus pushes for it, it's a one-line addition (`docker rm -f`) gated on a retry.

---

## 5. Guardrails — cleanup must run on EVERY exit path, not just SIGINT

The user's headline bug is SIGINT. But a `kill -TERM`, an `os.Exit` call from a panic, or a context-deadline-exceeded all need the same cleanup. The cleanup-context pattern (§3.1) handles SIGINT and SIGTERM (both are wired through `signal.NotifyContext`). It also handles any error path in `Deploy` that already calls cleanup (currently: readiness failure, save failure, run failure with rollback).

**Things it does NOT handle, by design:**
- SIGKILL (kill -9). No process can clean up after this. Defensive orphan cleanup at next-deploy start (§3.3) is the only mitigation. Documented.
- Host power loss. Same answer as SIGKILL.
- A panic that bypasses `Deploy`'s error returns. The fix is `defer`-based cleanup. Joel should consider whether to wrap the post-`Run` body in a defer that runs cleanup on `recover()`. I lean: yes, do it, it's a cheap belt-and-suspenders. Two `defer` blocks in `Deploy`: one that fires the new-container cleanup if we panic between Run and the success return, one that fires the old-container restore.

**One subtle guardrail Linus will ask about:** what if cleanup itself fails? Today, cleanup errors are swallowed via `_ =`. After the fix, we should at minimum `slog.Error` with the container name so the user has actionable info: "I tried to clean up `decloud-<svc>` and it failed; please run `docker rm decloud-<svc>` and retry". Don't fail the deploy on cleanup failure (the deploy already failed for the original reason).

---

## 6. Tests — what Kent writes

Three new behaviors to lock in:

1. **Probe cancellation triggers cleanup with a fresh context.** Cancel `ctx` during probe, assert `Driver.Stop` and `Driver.Remove` are called *and* are called with a context that is **not** the cancelled one. Gomock matcher: a custom `gomock.Matcher` that asserts `ctx.Err() == nil`.
2. **Probe cancellation surfaces as cancellation, not as readiness failure.** Assert the returned error satisfies `errors.Is(err, context.Canceled)` and does NOT satisfy `errors.Is(err, deploy.ErrReadiness)`.
3. **Defensive orphan cleanup runs on first deploy when an orphan exists.** Mock `Store.Load` → `ErrNotFound`, mock `Driver.Inspect(decloud-foo)` → `state == "running"`, assert `Driver.Stop` + `Driver.Remove` are called before `Driver.Run`. The reverse case: `Inspect` → `absent` → no Stop/Remove called.
4. **`restoreOldContainer` runs with a fresh context after probe cancellation on a redeploy.** Same matcher as test 1, applied to the rollback `Run` call.

Existing test that needs to be updated:

- `TestReadiness_ContextCancellationStopsProbe` (`readiness_test.go:144-166`) currently accepts any of `context.Canceled` / "context canceled" string / `ErrReadiness`. Tighten it: must satisfy `errors.Is(err, context.Canceled)` after the fix. The orchestrator-side test that asserts "not `ErrReadiness`" lives in service_test.

The existing `TestDeploy_ReadinessFailureRollsBackToOld` (`service_test.go:266-287`) covers the *non-cancelled* readiness-fail path. Keep it; ensure the fix doesn't break it.

Explicitly NOT a "change-detector test" — these all assert observable behaviors (which `Driver` method gets called with which kind of context, which sentinel the user sees), not implementation details.

---

## 7. Documentation — what Raymond touches

- `_docs/usage.md`: add a one-paragraph "What happens if I ctrl+c during deploy?" entry. Document: "we attempt to clean up the new container automatically; if cleanup fails (rare; usually a hung docker daemon), `decloud deploy service` on retry will detect and remove the orphan automatically".
- `_ai/`: a new note on the cleanup-context pattern. This is a reusable shape (cleanup MUST NOT depend on user-cancellable context). Cross-link to `error-wrap-discipline.md` and `optional-input-two-layer.md` since it's the same family of "structural contracts beat procedural ones" pattern.
- `_ai/m1x-backlog.md`: no entry needed. This isn't a punt; we're fixing it.

---

## 8. Acceptance criteria

The plan is done when, on the maintainer's host:

1. `decloud deploy service --name foo --port 8080 --readiness-path /healthz <broken-app-source>` is started.
2. After build completes and the container is running, ctrl+c during the readiness wait is pressed.
3. `docker ps -a | grep decloud-foo` returns no row.
4. The exit code is `ExitInterrupted` (or whatever maps to "user cancelled" — Joel decides), distinct from `ExitReadinessFailed`.
5. Re-running the same command (with a fixed app) succeeds end-to-end.
6. `docker kill -9 decloud` partway through `Run` (simulated SIGKILL): the next `decloud deploy service` invocation detects the orphan, logs a warning, removes it, and proceeds.
7. `go test ./...` passes.

---

## 9. Research info — facts about the codebase, for the implementation agents

Cross-reference for Joel/Kent/Rob.

### 9.1 Files

- `cmd/decloud/main.go` — wires `signal.NotifyContext(SIGINT, SIGTERM)` into the cobra root. Do not touch.
- `internal/cli/deploy_service.go:54,111` — `runDeployService` calls `d.Deploy(ctx, req)`. The CLI surface itself is correct.
- `internal/deploy/service.go` — the orchestrator. Bug lives at `:213-224` (probe-fail cleanup) and `:282-300` (`restoreOldContainer`). Defensive-orphan addition slots in just before `:195` (`Driver.Run`).
- `internal/deploy/readiness.go:69-70` — returns `ctx.Err()` raw. Wrap it.
- `internal/dockerdrv/cli_driver.go:34-186` — driver. `Stop`, `Remove`, `Inspect`, `Run` are the relevant methods. Argv shape is locked by `cli_driver_test.go` — do not change argv unless adding a NEW argv (i.e. don't touch existing tests that grep the docker argv).
- `internal/dockerdrv/driver.go:95-116` — `Driver` interface. No new methods needed; we already have `Stop`/`Remove`/`Inspect`.
- `internal/ids/ids.go:23` — `ContainerName(name) = "decloud-" + name`. Single source of truth for the orphan-detection name.
- `internal/registry/store.go:51,119` — `Load` returns `ErrNotFound` when the service is unknown. This is the signal "first-deploy or registry-drift; consider defensive orphan cleanup".

### 9.2 Sentinels and conventions

- `internal/deploy/service.go:23-29` — package-level `Err*` sentinels. If we add `ErrCancelled` (Shape B from §3.2), it goes here.
- `internal/dockerdrv/driver.go:13-15` — `ErrContainerNotFound` returned by `Stop`/`Remove`/etc when the container doesn't exist. Useful for the defensive cleanup branch.
- `internal/cli/exit_codes.go` — exit code mapping. If a new sentinel is introduced for cancellation, add a mapping. There's already an `ExitInterrupted` per the test file `exit_codes_test.go` — Rob should grep for it.

### 9.3 Test conventions

- Mocks live in `internal/{deploy,dockerdrv,registry,caddy,envcap}/mocks/` and are regenerated by `go generate ./...`.
- Test ordering uses `gomock.InOrder` (`_ai/gomock-inorder-sequencing.md`).
- Error wrapping is `%w: %w`, not `%w: %v` (`_ai/error-wrap-discipline.md`). The new error wraps must follow this.
- Test fixtures in `service_test.go`: `newDeployerHarness`, `newRequest`, `newPrev` — reuse them.

### 9.4 What I've verified, vs assumed

Verified:
- `signal.NotifyContext` cancels the cobra `cmd.Context()` on SIGINT/SIGTERM (read `main.go`).
- The probe returns `ctx.Err()` raw on cancellation (read `readiness.go:69-70`).
- The orchestrator passes the same `ctx` to `Stop`/`Remove` after probe failure (read `service.go:213-216`).
- `exec.CommandContext` with a pre-cancelled context returns the context error without invoking the binary (Go stdlib documented behavior; also empirically: a cancelled `CommandContext` invocation produces `signal: killed` only if the process started; if cancelled before start, it returns `context.Canceled` from `Run()` with no syscall).
- `Run`uses `--restart unless-stopped` (`cli_driver.go:50`), which is incompatible with `--rm`.
- `ContainerName` is `decloud-<svc>` (`ids/ids.go:23`).
- First-deploy: `Store.Load` returns `ErrNotFound`, so `hasPrev = false`, so the existing pre-Run stop+remove branch at `service.go:172-185` is skipped. This is exactly why the user's first-deploy retry fails.

Assumed (Joel/Kent should verify when implementing):
- That `slog` is the right log surface for the cleanup-failure warning. (Probably yes; rest of `service.go` uses `slog.With(...).Error/Info`.)
- That a 30s budget for the cleanup context is enough. Driven by the existing 10s `Stop` grace + buffer. Joel can adjust.
- That existing exit-code wiring includes a "cancelled" code. `internal/cli/exit_codes.go` will tell us.

---

## 10. Out of scope

- Cleanup on `decloud caddy up/down/reload` interrupt. Different code path; if defective, separate task.
- Lifecycle commands (`stop`, `start`, `restart`, `unregister`). Not affected by this bug.
- M1.x backlog item #1 (`NetworkEnsure` in `Lifecycle.Start`). Stays deferred.
- Replacing `--restart unless-stopped`. Stays as-is.
- Container naming change (`decloud-<svc>-<deploy-id>`). M4 deliverable.

---

## 11. Risk assessment

Low risk. The fix is constrained: three call sites in `service.go` change context source, one call site in `readiness.go` wraps an error, one new defensive branch is added pre-`Run`. No interface changes. No new docker driver methods. No new dependencies.

The biggest risk is breaking existing tests. The harness uses `context.Background()` in all tests, so the cleanup-context change is invisible to them — they'll still pass. The probe-cancellation test (`TestReadiness_ContextCancellationStopsProbe`) is the only one with a cancelled context; it currently accepts `context.Canceled` OR `ErrReadiness` (`:161-164`), and after the fix only `context.Canceled` will be true. That's the intended tightening.

I want Joel to put the precise function signatures in the tech plan, including the helper that creates the cleanup context (factor it out — it'll be used 4+ times). I want Kent to write the test that asserts cleanup-context-is-not-cancelled before Rob writes the implementation. I want Linus to push back on Shape A vs Shape B for the cancellation sentinel — that's the one place I'd accept being talked out of my position.

Don't ship this until it's RIGHT.

— Don

---

## 12. Linus review resolutions (v2 revision)

Linus returned REVISE on `04-linus-review.md` with six items. My decisions, with reasoning:

### 12.1 Issue 1 — defensive orphan label-gating: **Option B, label gate**

Linus is right and Joel's v1 §3.5 was wrong. v1 of this plan (§3.3 above) said the orphan check should verify "that container's image label/name does not correspond to any registry entry" — Joel softened this to "the registry has no entry for this name," which is weaker. The fix is to inspect the `decloud.service` label that `cli_driver.go:60` already attaches to every container we create.

Cost: one new field on `InspectResult` (`Labels map[string]string`), one format-string change in `cliDriver.Inspect`. NOT a new interface method. The mock surface widens by a field but doesn't need new methods. Joel: revise §3.5 of your tech plan and add the field to your driver-interface section. The "zero interface changes" claim becomes "one field added, no new methods" — that's fine, the safety win is worth it.

Test additions: a new test that the orphan-with-mismatched-label path returns `ErrRun` with the recovery hint. See §3.3 above for the failure-mode matrix.

### 12.2 Issue 2 — `ExitCodeFor` over-broad: **Option A, drop the context.* matches**

Linus is right. Joel's v1 §3.6 added `errors.Is(err, context.Canceled)` and `errors.Is(err, context.DeadlineExceeded)` to the `ExitInterrupted` case as belt-and-suspenders. That's defense-in-depth in the wrong direction — it's the same shape of trap that the readiness probe fell into (assuming `ctx.Err()` means user-cancelled). Today it's harmless because every cancellation flows through `Deploy` which wraps as `ErrInterrupted`. Tomorrow a future caller might use `context.WithTimeout` for unrelated reasons; if that timeout fires, the user sees exit 130 and thinks they pressed ctrl+c.

The fix: match only `deploy.ErrInterrupted` in `ExitCodeFor`. The `Deploy` orchestrator is the choke point and already wraps all cancellation paths (§3.4.1, §3.4.2, §3.4.3 of Joel's tech plan). If a future code path bypasses `Deploy` and returns raw `context.Canceled`, we'll add Option C (top-level `main.go` override) surgically at that time — but until then, don't paint ourselves into the corner.

Joel: in §3.6 of your tech plan, drop the `context.Canceled` and `context.DeadlineExceeded` lines from `ExitCodeFor`. Match only `deploy.ErrInterrupted`. The `context` import in `exit_codes.go` may also drop out — verify.

The corresponding test changes in Joel's §5.8: drop `{"context-canceled", context.Canceled, ExitInterrupted}` and `{"context-deadline", context.DeadlineExceeded, ExitInterrupted}` from the table cases. Keep `{"interrupted", deploy.ErrInterrupted, ExitInterrupted}` and `{"interrupted-wrapped", fmt.Errorf("oops: %w", deploy.ErrInterrupted), ExitInterrupted}`.

### 12.3 Issue 3 — probe wrap shape: **raw `ctx.Err()`, plus log-line fork**

Resolved in §3.2 above. Probe returns raw `ctx.Err()`. Orchestrator's audit log line at `service.go:214` forks: log "readiness failed" at Error only when it's a real readiness failure, log "deploy cancelled during readiness wait" at Info when the cause is `errors.Is(err, context.Canceled)` or `errors.Is(err, context.DeadlineExceeded)`.

Joel: in §3.3 of your tech plan, replace `fmt.Errorf("readiness: %w", ctx.Err())` with raw `return ctx.Err()`. In §3.4.1, the `logger.Error("readiness failed", ...)` line becomes:

```
if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
    logger.Info("deploy cancelled during readiness wait", "step", "readiness")
} else {
    logger.Error("readiness failed", "step", "readiness", "error", err)
}
```

Test impact: the §5.7 update in Joel's plan tightens to require `errors.Is(err, context.Canceled)` — that still works with raw `ctx.Err()` (it IS `context.Canceled`). The `strings.Contains` fallback was already being removed.

### 12.4 Issue 4 — test churn / harness default: **Option C, harness `AnyTimes()` default**

Linus is right that 11 mechanical edits is a signal worth investigating. The orchestrator-contract widening (§3.5 always calls `Inspect`) is real and the tests SHOULD reflect it, but they don't all need to spell it out individually. Add a default expectation in `newDeployerHarness`:

```
driver.EXPECT().Inspect(gomock.Any(), gomock.Any()).
    Return(dockerdrv.InspectResult{State: "absent"}, nil).
    AnyTimes()
```

Tests that DO care about the inspect call (5.3 orphan-exists, 5.4 orphan-absent-explicit, 5.5 orphan-cleanup-failure, the new label-mismatch test from §12.1) override with explicit InOrder expectations for that specific argument shape. gomock's matcher precedence handles this correctly — specific InOrder expectations take priority over the AnyTimes default for matching calls.

This collapses Joel's §5.9 list of 11 mechanical edits to one harness change. Joel: revise §5.9 to drop the 11-test paste-in list, and add the harness default expectation to §4.4 (Test fixture helpers). Note the precedence behavior in a sentence so Kent doesn't get confused.

### 12.5 Issue 5 — caddy cleanup symmetry: **backlog entry**

Linus is right. The cleanup-context pattern Joel introduces in `internal/deploy/service.go` is going to want re-application to `internal/caddy/manager.go` eventually — same shape of bug, same shape of fix, different code path. Don't expand this task's scope; do add a backlog entry to `_ai/m1x-backlog.md` so Andy can prioritize. Entry text should reference this task (`_tasks/2026-04-28-deploy-cleanup-on-interrupt/`) so future-Don can find the originating pattern.

Joel: don't change your tech plan for this; it's a pure backlog action. I (Don) will draft the backlog entry text in this plan (see §12.7 below) so Raymond can append it during the docs phase.

### 12.6 Issue 6 — `restoreOldContainer` failure surfacing: **backlog entry**

Pre-existing bug, out of scope for this task. Linus correctly flagged. The cleanup-context fix at least ensures `restoreOldContainer` *runs* on a non-cancelled context, so the failure mode strictly improves from "rollback was skipped" to "rollback hit a real docker error and was logged but not surfaced." That's progress. The further refinement — propagate the rollback failure up the error chain so the user sees both "the deploy failed" AND "the rollback also failed" — is its own task. Add to `_ai/m1x-backlog.md`.

### 12.7 Backlog entries to be added (Raymond's task)

Two new entries in `_ai/m1x-backlog.md`. Drop them at the bottom (item 7 and item 8). Approximate text:

**Item 7 — Apply cleanup-context pattern to caddy/manager.go**

Where: `internal/caddy/manager.go` — `Manager.Up`/`Down`/`Reload` and any `docker run`/`docker stop`/`docker rm` invocations therein.

Why deferred: scoped tight per `_tasks/2026-04-28-deploy-cleanup-on-interrupt/`. Same shape of bug as the deploy-service cleanup-on-interrupt fix (cleanup tied to user-cancellable ctx); user reported the deploy variant, not the caddy variant. The pattern (cleanup ctx derived from `context.Background()` with a 30s timeout, distinct from the request ctx) is locked in by that task and re-applies cleanly.

Fix shape: identify cleanup blocks in `manager.go`, replace request-ctx with `newCleanupContext()`-derived ctx (move the helper from `internal/deploy/service.go` to a shared location if both packages need it, OR copy locally — bikeshed). Mirror the audit-log-on-cleanup-failure pattern.

Originator: Linus, `04-linus-review.md` Issue 5.

**Item 8 — `restoreOldContainer` failures should surface in the error chain**

Where: `internal/deploy/service.go:282-300` (`restoreOldContainer`). Currently logs via `slog.Error` and returns silently.

Why deferred: pre-existing bug, scoped tight per `_tasks/2026-04-28-deploy-cleanup-on-interrupt/`. The cleanup-context fix in that task strictly improves the failure mode (rollback now actually runs on a non-cancelled ctx) but doesn't fix the surfacing. Doing both in one task expanded scope; punt.

Fix shape: change `restoreOldContainer` signature to return `error`. At each call site (3 in `Deploy` after that task), if the cleanup-path err is non-nil and `restoreOldContainer` ALSO returns an error, the surfaced error to the user should mention both ("readiness failed AND rollback to previous container failed"). `errors.Join` is the right tool. New test asserting both errors surface.

Originator: Linus, `04-linus-review.md` Issue 6.

### 12.8 Summary of changes for Joel's tech plan revision

Joel: please revise `03-tech-plan.md` to incorporate:

1. **§3.3 (probe wrap):** drop the "readiness:" prefix; return raw `ctx.Err()`.
2. **§3.4.1 (probe-failure cleanup block):** fork the `logger.Error("readiness failed", ...)` audit line on cancellation vs failure; cancellation logs at Info level, real failure logs at Error.
3. **§3.5 (defensive orphan cleanup):** add label gating using a new `InspectResult.Labels map[string]string` field. The orphan branch becomes: Inspect → if absent, no-op; if present and label `decloud.service == req.Name`, stop+remove; if present and label missing/mismatched, return `ErrRun` with manual-`docker rm -f` hint.
4. **§3.6 (`ExitCodeFor`):** drop the `context.Canceled` and `context.DeadlineExceeded` cases. Match only `deploy.ErrInterrupted`. Verify whether the `context` import drops.
5. **§4.4 + §5.9 (test infrastructure):** add a default `Inspect → absent` AnyTimes expectation to `newDeployerHarness`. Drop the §5.9 list of 11 mechanical edits as no longer needed.
6. **§5.8 (exit code test cases):** drop the `context.Canceled` and `context.DeadlineExceeded` table cases.
7. **New §5.X test:** orphan exists with mismatched/missing `decloud.service` label → `ErrRun` with recovery hint, no Stop/Remove called. Add to the test list.
8. **§3.5 driver interface widening note:** explicitly call out the `InspectResult.Labels` field addition. Update the "no new mocks" claim to "regenerate mocks to pick up the new field; no new methods."

Everything else in `03-tech-plan.md` holds. The cleanup-context discipline in §2 and §3.4 is correct; the helper in §2.3 is fine; the driver-policy-ownership argument in §2.2 is correct. Linus endorsed those.
