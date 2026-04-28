# Linus's plan review — deploy cleanup on interrupt

## Verdict: REVISE (with a clear path to APPROVE)

The diagnosis is excellent. Don traced the bug end-to-end, named the right
root cause ("cleanup is tied to the same context that triggers cleanup"),
correctly disqualified `--rm`, and built a fix shape that generalizes
(cleanup-context discipline) instead of patching one site. Joel's tech plan
is unusually detailed and lands the trade-offs honestly. I'm not rejecting
this; the architecture is right. But there are six items below that need
to be resolved before Kent writes a single test, and a couple of them are
not nits.

The **big one** is item 1 (defensive orphan idempotency / restart inheritance).
The rest are smaller — tighten or punt.

---

## What's RIGHT (so I'm not just complaining)

1. **Cleanup-context-as-orchestrator-policy is correct.** Joel's §2.2 framing
   wins. The driver must not own this — `cliDriver.Stop` doesn't know whether
   it's running under a forward-progress context or a cleanup context, nor
   should it. If you push a "cleanup timeout default" into the driver, you
   fork the policy across every future driver impl and force a reach-around
   for callers who want to honor cancellation (e.g. `lifecycle.go:Stop`).
   Joel is right; I am NOT going to make him write a `Driver.StopForCleanup`
   variant. **Endorsed.**

2. **`--rm` rejection is honest.** The `--restart unless-stopped`
   incompatibility alone disqualifies it. The deeper point — `--rm` only
   triggers on the container's main process exit, which is *not* what
   happens when `decloud` itself dies — is the one I would have led with.
   Don led with the daemon-restart regression and got there too. Either
   way: rejected for the right reasons.

3. **Defensive orphan cleanup pattern is the only honest answer to
   SIGKILL/power-loss.** No in-process cleanup catches kill -9. The choice
   to gate it on `!hasPrev` AND a non-absent inspect is the right
   conservative shape.

4. **`%w: %w` discipline is preserved.** Every new wrap in §3.4 uses
   `%w: %w`. Good.

---

## IDENTIFIED ISSUES

### Issue 1: Defensive orphan cleanup loses the previous container's `--restart` policy and risks a footgun — needs a guardrail

**Problem:** §3.5 inspects `decloud-<svc>`, finds it running, calls
`Stop` + `Remove`, and proceeds to `Run` a fresh container. But the
**orphan was created by a *prior* version of the binary or a prior
deploy whose registry entry is gone** — we have no idea what image it
was running, what its env was, or what user data it might be holding.
We are assuming "no registry entry → it's an abandoned orphan, kill it."

That's *probably* right for the user's reported scenario (interrupted
first deploy). It is NOT obviously right for:

- **A user who manually `docker run --name decloud-foo` for some other
  purpose** (unlikely but possible — the `decloud-` prefix is a
  convention, not a Docker-enforced reservation).
- **A user who blew away their `_tasks`/`registry` dir** thinking they
  were "starting fresh" with a still-running production container.
- **A future M2/M3 multi-host scenario** where the registry might be
  out of sync with the host.

The guardrail Don described in his §3.3 ("the `decloud-` label/name does
not correspond to any registry entry") got *softened* in Joel's §3.5 to
just "the registry has no entry for this name." Joel's check is
**weaker** than Don's spec. There's no label inspection.

**Impact:** Low blast radius for M1 (single-host, single-user, deploy
service is the only thing creating `decloud-*` containers). But we're
adding a "we will silently destroy any running container named
`decloud-<svc>` if its registry entry is missing" behavior that the docs
need to call out loudly, and that the implementation should make
*observable* enough that a confused user can recognize it.

**Options:**

- **Option A (Minimal):** Ship as planned. Document loudly in
  `_docs/usage.md` that decloud-prefixed container names are reserved
  and may be reaped on deploy. Pro: ships now. Con: someone someday
  loses a container they cared about. The slog.Warn audit line is
  there, so they can find it post-mortem.
- **Option B (Tightened guardrail):** Before stop+remove, additionally
  verify the container has the `decloud.service=<name>` label that
  `cli_driver.go:60` puts on every container we run. If the label is
  missing, fail with a clear error: "container `decloud-foo` exists but
  was not created by decloud; refusing to remove. Run `docker rm -f
  decloud-foo` manually if you want to claim this name." Pro: makes the
  destruction explicit and label-gated. Con: requires inspecting labels,
  which means either (a) extending `InspectResult` to include labels, or
  (b) adding a `docker inspect ... --format '{{.Config.Labels}}'` call.
  Joel said "zero changes to dockerdrv interface" — Option B contradicts
  that, but it's a 5-line addition (a new field on `InspectResult`).
- **Option C (Defer):** Strip §3.5 from this task entirely. Fix only the
  cleanup-context discipline, which is the user's actual headline bug.
  Defer "defensive orphan cleanup" to a follow-up task where the label
  guard, the docs warning, and the test matrix can be done together.
  Pro: smallest blast radius, clearest scope. Con: a SIGKILL between
  Run and Save still leaves the user stuck — but that's a much rarer
  case than ctrl+c during readiness.

**My take:** **Option B.** The label check is structurally tiny (one
extra format string in `cli_driver.Inspect` and a new field on
`InspectResult`) and it converts "we silently nuke any container with
the right name" into "we nuke any container we know we created." That's
the right contract. Option C is also defensible if Don wants to scope
this task tight; I'd accept it. Option A I push back on — the silent
destruction on label mismatch is a footgun.

**Don's decision required:** Option A vs B vs C. If B, decide on
`InspectResult.Labels` map shape (probably `map[string]string`) — that
*is* a Driver interface widening, and Joel's "zero interface changes"
boast becomes "one field added, no new methods."

---

### Issue 2: `ErrInterrupted` adds real value — but the context.Canceled fast path in `ExitCodeFor` is over-broad

**Problem:** Joel's §3.6 adds:

```go
case errors.Is(err, deploy.ErrInterrupted),
    errors.Is(err, context.Canceled),
    errors.Is(err, context.DeadlineExceeded):
    return ExitInterrupted
```

The `context.Canceled` and `context.DeadlineExceeded` branches will
match **any** error chain that contains a context error — including
things that have nothing to do with user SIGINT. Concrete example: a
future caller that uses a `context.WithTimeout(ctx, 30*time.Second)`
internally for some unrelated reason and the timeout fires; that's
NOT a user interrupt, but `ExitCodeFor` will report 130.

Today this isn't a problem because nobody upstream wraps the deploy in
a separate timeout. But this is the **same shape of trap** that the
readiness probe fell into: assuming `ctx.Err()` means "user cancelled."
We're paving over one occurrence of that mistake while planting another
in the exit-code mapper.

**Impact:** Today: zero — there's no path that leaks a non-interrupt
context error into `ExitCodeFor`. Tomorrow: a future caller adds a
timeout, deadline fires for an internal reason, user sees exit 130 and
thinks they pressed ctrl+c. Confusing but not catastrophic.

Also worth noting on the value question: Joel's pre-rebuttal in §11.1
("pure Shape A means the user-visible error message is 'deploy:
readiness probe failed: readiness: context canceled'") is correct and
settles it. Yes, `ErrInterrupted` adds real value — it carries the
human-readable "cancelled by user" message AND lets the exit code map
key on a deploy-package sentinel rather than a generic `context.Canceled`.
**The sentinel is justified.** It's the *additional* `context.Canceled`
fallback in the exit mapper that I'm questioning.

**Options:**

- **Option A (Minimal):** Drop `context.Canceled` and `context.DeadlineExceeded`
  from the `ExitCodeFor` case. Match only `deploy.ErrInterrupted`. The
  `Deploy` orchestrator already wraps cancellation as `ErrInterrupted` at
  every exit point per §3.4, so any cancellation that escapes `Deploy` is
  already tagged. Pro: precise mapping, no false-positive 130s. Con: a
  future caller bypassing `Deploy` and returning raw `context.Canceled`
  gets `ExitInternal` (70) instead of 130.
- **Option B (Joel's plan):** Keep all three. Pro: belt-and-suspenders.
  Con: paints us into a corner if any non-deploy code path uses
  context timeouts.
- **Option C (Hybrid):** Match `deploy.ErrInterrupted` explicitly, and
  add a top-level check in `main.go` (NOT `ExitCodeFor`) that if
  `ctx.Err() == context.Canceled` after `ExecuteContext` returns,
  override the exit code to 130. Pro: separates "the deploy package
  reported a cancellation" from "the user actually pressed ctrl+c." Con:
  more moving parts.

**My take:** **Option A.** The `Deploy` package is the choke point;
every cancellation flows through `ErrInterrupted` per §3.4.1/3.4.2/3.4.3.
Don't double-cover; it's defense-in-depth in the wrong direction. If we
ever DO have a non-deploy cancellation that should be exit-130, we'll
add Option C surgically at that time.

**Don's decision required:** A vs B vs C. I lean A; Joel's argument for
B is "future code path returns cancellation without going through
Deploy" — fine, but until that exists, B is over-spec.

---

### Issue 3: Probe error wrap shape is right but the rationale is muddled

**Problem:** §3.3 changes `return ctx.Err()` to
`return fmt.Errorf("readiness: %w", ctx.Err())`. The "readiness:" prefix
is ironic given the *whole point* of the change is to STOP the
orchestrator from treating cancellation as a readiness failure. The
prefix is an audit string only and the orchestrator now keys off
`errors.Is(err, context.Canceled)`, not the prefix. So functionally fine,
but the prefix is misleading to a future reader who diffs this and goes
"wait, why are we still saying it's a readiness error?"

Joel's pre-rebuttal acknowledges this and keeps the prefix anyway, with
the rationale "the existing test accepts strings containing 'context
canceled'." That's a test-driven rationale, and Joel's own §5.7 *changes*
the test to no longer rely on string contents. So that rationale is
self-cancelling.

The substantive question Don asked me to push on: should the probe
return raw `ctx.Err()` instead?

**Yes, raw is cleaner.** The orchestrator wraps with `ErrInterrupted` at
its layer; the probe doesn't need to add commentary that the orchestrator
will then re-wrap. The current wrapping creates a chain like
`fmt.Errorf("%w: %w", ErrInterrupted, fmt.Errorf("readiness: %w", context.Canceled))`
which prints as "deploy: cancelled by user: readiness: context canceled."
The "readiness:" word in there is **wrong** — the readiness probe didn't
fail; it was cancelled.

**Impact:** User-visible error message clarity. A user reading `decloud`'s
stderr after ctrl+c sees a misleading word "readiness" in the chain.

**Options:**

- **Option A (Minimal):** Probe returns `ctx.Err()` raw. Orchestrator
  wraps as `ErrInterrupted`. User sees `"deploy: cancelled by user:
  context canceled"`. Pro: clean. Con: the audit log line at
  `service.go:214` (`logger.Error("readiness failed", ...)` ) prints
  even on cancellation, which is misleading. The fix is to also reword
  that log line to "readiness step terminated" or similar based on
  whether it's a cancellation. (Joel doesn't address this.)
- **Option B (Joel's plan):** Keep `fmt.Errorf("readiness: %w",
  ctx.Err())` for "audit prefix." Pro: traceability of where the
  cancellation was observed. Con: misleading word "readiness" in the
  user-visible chain.
- **Option C (Compromise):** Probe returns `fmt.Errorf("probe wait: %w",
  ctx.Err())`. The audit prefix is "probe wait" not "readiness," so the
  user-visible chain reads "deploy: cancelled by user: probe wait:
  context canceled." That at least doesn't claim a readiness *failure*.

**My take:** **Option A.** The probe is a tiny function; the layer above
it can do the audit logging. And the orchestrator log line at
`service.go:214` should be conditionalized — log "readiness failed" only
when it actually failed, log "deploy cancelled during readiness wait"
when ctx was cancelled. Two info-level branches, three lines total.
Joel's plan should either pick A and add the log-line fork, OR adopt C
and explicitly justify the prefix as "step name, not failure mode."

**Don's decision required:** Pick the wrap shape, and decide whether to
update the `logger.Error("readiness failed", ...)` line at service.go:214
to discriminate cancellation from real failure. I'd say yes.

---

### Issue 4: Test churn for §5.9 is acceptable but tells me §3.5 is at the wrong layer

**Problem:** Joel admits 11 existing tests need an extra `Inspect →
absent` expectation because §3.5 unconditionally calls `Inspect` on
every `!hasPrev` deploy. That's not "fragile" (Joel's anticipated
objection 11.4 dismisses) — it's a **signal that the orchestrator
contract is being widened**, and every test that asserts the contract
must be updated. That's correct! But the question is: should §3.5 be
its own helper that the orchestrator *delegates* to, so that the test
can mock the helper instead of mocking `Inspect`?

The contract being added is: "before `Run`, ensure the container name
is free." That's a separable concern. The current implementation
inlines it into `Deploy`; the alternative is a `claimContainerName(ctx,
name)` private method that the harness can stub.

**Impact:** Test-maintenance. 11 mechanical updates in this task; same
mechanical update for any future test that hits the `!hasPrev` path.

**Options:**

- **Option A (Joel's plan):** Inline the §3.5 logic in `Deploy`.
  11 tests update mechanically. Pro: simple, no new abstraction. Con:
  every future first-deploy test must remember to add the Inspect
  expectation.
- **Option B (Helper method):** Extract to `(d *serviceDeployer)
  ensureContainerNameFree(ctx, containerName)`. The orchestrator calls
  it; the existing tests stub `Inspect → absent` via an unconditional
  AnyTimes. Or better: **don't stub Inspect at all** in the existing
  tests by making `ensureContainerNameFree` take a `Driver` reference
  AND falling through silently when the configured deployer's policy
  is "no-op for tests." That's gross; reject.
- **Option C (Helper + AnyTimes):** Extract the helper, then in tests
  use `h.driver.EXPECT().Inspect(gomock.Any(), "decloud-foo").Return(
  dockerdrv.InspectResult{State: "absent"}, nil).AnyTimes()` in the
  harness setup function so tests don't have to specify it inline. Pro:
  one-line change in `newDeployerHarness`, fixes all 11 tests at once.
  Con: AnyTimes hides ordering — but the §5.3/§5.4 tests that DO care
  about ordering use `gomock.InOrder` and a specific Inspect
  expectation, which overrides the harness default.

**My take:** **Option C if it works mechanically, else Option A.** The
test churn isn't really 11 separate edits if the harness has a default
`Inspect → absent` expectation. Worth Kent investigating before
swallowing 11 mechanical edits as "boring but correct."

**Don's decision required:** Whether to add a default
`Inspect → absent` expectation to `newDeployerHarness` (Option C) or
let Kent paste the line into 11 tests (Option A). Probably C.

---

### Issue 5: Caddy cleanup, panic recovery, signal-handler races — what's missing

**Problem:** Don's §10 lists "out of scope: Cleanup on `decloud caddy
up/down/reload` interrupt." Joel's §11.5 punts panic-recovery to
follow-up. Neither plan addresses **signal-handler races**: what if
SIGINT fires between `Driver.Run` succeeding and the orchestrator
storing the container ID anywhere? Today the answer is "the §3.5
defensive cleanup catches it on the next deploy" — fine, IF §3.5 ships.
If it doesn't ship (Issue 1 Option C), then we have a regression vs.
Joel's spec.

The caddy point is more interesting. `internal/caddy/manager.go` does
its own `docker run` for the Caddy container. If a user ctrl+c's during
`decloud caddy up` while Caddy is starting, do we leak? This task
explicitly punts that. **Don is right to scope tight** — the user
reported deploy cleanup, not caddy cleanup. But the cleanup-context
pattern Joel introduces should be re-used in caddy/manager.go in a
follow-up, and **`_ai/m1x-backlog.md` should record that** as a sibling
task.

**Impact:** None for this task. But if Andy/Don don't put a backlog
entry for "apply cleanup-context pattern to caddy.Manager," the same
bug will show up in the caddy code path eventually.

**Options:**

- **Option A (Minimal):** Ship as planned. Add ONE line to
  `_ai/m1x-backlog.md`: "apply cleanup-context pattern to
  caddy/manager.go to handle SIGINT during caddy up/down." Pro: tracks
  the debt. Con: nothing material.
- **Option B (Expand scope):** Also fix caddy in this task. Pro:
  symmetry. Con: doubles the test surface.
- **Option C (Defer silently):** Don't even backlog it. Pro: smallest
  ask now. Con: future bug.

**My take:** **Option A.** Backlog entry, no code change. Andy can
prioritize.

**Don's decision required:** Add backlog entry yes/no. Almost certainly
yes.

---

### Issue 6: `restoreOldContainer` doesn't surface its failures upward

**Problem:** This is a pre-existing bug, and §3.4.4 explicitly does
NOT change the function's signature. But if `restoreOldContainer` fails
(its `Run` call returns an error), the failure is logged via
`slog.Error` and the function returns silently. After the fix, the
caller already returns `ErrInterrupted` or `ErrReadiness` — fine, the
*headline* error is correct. But the user has now lost their old
container AND failed to restore it AND we exited with an error that
doesn't mention the rollback failure.

This is OUT OF SCOPE per Don's plan. I'm calling it out so it's
acknowledged, not so it's fixed in this task.

**Options:**

- **Option A:** Out of scope. Document in `_ai/m1x-backlog.md`.
- **Option B:** Fix in this task. Add return value to
  `restoreOldContainer`, surface in the error chain at every cleanup
  call site.
- **Option C:** Ignore.

**My take:** **Option A.** Backlog. The cleanup-context fix at least
ensures `restoreOldContainer` *runs* on a non-cancelled context, so
the failure mode is now "rollback hit a real docker error" rather than
"rollback was skipped because ctx was cancelled." That's a strict
improvement; the further refinement can wait.

**Don's decision required:** Backlog entry, yes/no.

---

## Direct answers to the questions you asked me

### "Constructing a fresh `context.Background()`-derived timeout in the deploy service" — right call?

**Yes.** Joel's §2.2 argument is correct. Cleanup-context-ness is
orchestrator policy, not driver policy. The driver respects what it's
handed; the orchestrator decides what to hand it. Pushing this into the
driver forks the policy across every future driver impl and adds
nothing — `cliDriver` already correctly threads `ctx` to
`exec.CommandContext`, that's its job.

The only counter-argument I'd entertain is "what if the driver wants to
log a `cleanup attempted with timeout` line?" — and the answer is "no,
the orchestrator logs that, the driver just runs commands."

**Endorsed.**

### "Is `--rm` truly the wrong choice given `--restart unless-stopped`?"

**Yes, the trade-off was evaluated honestly.** Don's §2 nails it:
`docker run` rejects `--rm` with `--restart` other than `no`. To use
`--rm` you'd have to drop `--restart unless-stopped`, which regresses
host-restart behavior. The hybrid ("--rm during readiness window, then
docker update --restart=unless-stopped after") is genuinely worse than
explicit cleanup.

The deeper point — `--rm` only cleans up when the **container's main
process exits**, and SIGINT to `decloud` does not exit that process —
should have been led with. Don gets there in §2.1. Either way: `--rm`
is the wrong tool.

**Endorsed.**

### "Does `ErrInterrupted` + exit 130 add real value, or is it scope creep?"

**Real value, not scope creep.** Two reasons:

1. The exit-code mapper needs a deploy-package sentinel to switch on.
   Pure `context.Canceled` matching is fragile (Issue 2 above).
2. The user-visible error message reads "deploy: cancelled by user"
   instead of "deploy: readiness probe failed: context canceled."
   That's the difference between "I pressed ctrl+c" and "my app is
   broken."

The `errors.Is(err, context.Canceled)` traversal at the CLI boundary
**could** do most of the work — but you still need a sentinel for the
human-readable string. Joel's hybrid (Shape B for the outer sentinel,
Shape A wrapping at the probe layer) is the right call.

What I push back on is the **exit-code mapper also matching raw
context.Canceled** (Issue 2). That's over-spec. Trim it.

### "Defensive orphan cleanup — test churn justified?"

**The cleanup is justified; the inline implementation may not be.**
See Issue 4 — Option C (harness default expectation) likely makes the
11 mechanical edits collapse to one harness change. Worth Kent
checking before doing the boring 11x paste.

The deeper guardrail concern (Issue 1: label-gating the destruction)
is more important than the test-churn question. Get the safety right
first; the test-churn falls out.

### "Probe error-wrapping shape — `fmt.Errorf("readiness: %w", ctx.Err())` vs raw `ctx.Err()`"

**Raw is cleaner** (Issue 3). The "readiness:" prefix is misleading
for a cancellation, and Joel's rationale ("existing test accepts
strings containing 'context canceled'") is self-cancelling because
§5.7 changes that test. Either drop the prefix entirely (my pick) or
rename it to "probe wait:" so it doesn't claim a failure mode that
didn't happen.

Also: the orchestrator's `logger.Error("readiness failed", ...)` line
at service.go:214 should fork on cancellation vs failure. Don't log
"readiness failed" when the user just hit ctrl+c.

### "What's missing entirely?"

1. **Label-gated orphan destruction.** Issue 1. The biggest gap.
2. **Caddy cleanup symmetry.** Issue 5. Backlog entry suffices.
3. **Panic recovery.** Joel punted to §11.5; that's the right call IF
   §3.5 ships with label gating, since the next-deploy detect-and-clean
   path then catches panic-leaked containers safely. If §3.5 is
   stripped (Issue 1 Option C), panic recovery becomes a real gap.
4. **Signal-handler race between Run and Save.** Same answer as (3) —
   §3.5 catches it on the next deploy. If §3.5 is stripped, this is a
   gap.
5. **`logger.Error("readiness failed")` at service.go:214** does not
   discriminate cancellation from real failure. Issue 3. Small.
6. **`restoreOldContainer` failure surfacing.** Issue 6. Pre-existing
   bug. Out of scope; backlog.

Nothing else. The plan covers the surface area.

---

## What needs to happen for me to APPROVE

In order:

1. **Resolve Issue 1** (label-gated orphan destruction). Pick A, B, or
   C. If A or C, approved as-is on this point (with backlog entry for
   B). If B, decide on `InspectResult.Labels` shape.
2. **Trim Issue 2** to my Option A (drop the `context.Canceled` /
   `context.DeadlineExceeded` cases from `ExitCodeFor`; rely on
   `ErrInterrupted` wrapping in `Deploy`). Or argue convincingly for B.
3. **Resolve Issue 3** (probe wrap shape) — drop the "readiness:"
   prefix OR rename it. Update the orchestrator log line at
   service.go:214 to fork on cancellation vs failure.
4. **Investigate Issue 4 Option C** (harness default Inspect → absent).
   If feasible, take it. Else accept the 11 mechanical edits.
5. **Add backlog entries** per Issues 5 (caddy) and 6 (restoreOldContainer).

That's it. Once those are settled, Kent can write tests and we ship.

---

## What I'd say to the team out loud

Don, the trace and the trade-off analysis are excellent. The
`--rm`-rejection writeup is the clearest I've seen on this codebase.
Joel, the tech plan is detailed enough that Rob can implement from it
without asking questions, and §11 (anticipated objections) is
genuinely useful. The hybrid Shape A+B for the cancellation sentinel
is the right call.

The one thing I want both of you to internalize: **§3.5 is a behavior
the user did NOT ask for**, and it's quietly destructive. The user
asked for "cleanup on ctrl+c." Defensive orphan cleanup is a
reasonable add to handle SIGKILL/power-loss, but it deserves a
guardrail (label check) and a docs warning. Don't ship it as
"silently nukes anything named decloud-*" — that's how you get an
angry user later.

Everything else is small. Fix the six items above and we ship.

— Linus
