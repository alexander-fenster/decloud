# Joel's tech plan — deploy cleanup on interrupt

**Revision history:**
- v1: original tech plan.
- v2: incorporates Linus's REVISE feedback from `04-linus-review.md` and Don's resolutions in `02-plan.md` §12. Material changes: §3.3 probe wrap shape (raw `ctx.Err()`), §3.4.1 audit-log fork on cancellation, §3.5 label-gated orphan cleanup with new `InspectResult.Labels` field, §3.6 `ExitCodeFor` matches only `ErrInterrupted`, §4.4 + §5.9 harness `AnyTimes()` default for `Inspect`, §5.8 dropped context.* test cases, new §5.10 label-mismatch test. The cleanup-context discipline (§2 + §3.4) is unchanged — Linus endorsed it.
- v2.1: Linus APPROVED v2 (`06-linus-review-v2.md`) with one non-blocking flag — §3.5 wraps `context.Canceled` as `ErrRun` instead of `ErrInterrupted`. Don elected fix-in-scope. §3.5 code block now detects cancellation at three sites (inspect, stop, remove) before the `ErrRun` wrap; failure-mode matrix gains a cancellation row; new §5.10.1 Kent test case. See `007-don-final-lockdown.md`.
- v2.2 (this revision): v2.1 shipped; Kevlin (`011-kevlin-review.md`) and Linus (`12-linus-impl-review.md`) both APPROVED with six follow-up items (Linus Issue 1 strategic + Issue 2 doc, Kevlin nits 1-2 + style 3-4). Don's iteration-2 lockdown (`013-don-plan-iteration2.md`) elected fix-in-scope on all six. v2.2 is a DELTA section appended at §13 — earlier sections remain authoritative. Material changes: §3.4.5 cancellation symmetry (the v2.1 fix replicated to the `hasPrev` redeploy stop+remove branch), four mechanical service.go cleanups (`isCancellation` helper hoist, audit-log tense fix, slog message-vs-field convention), one `_ai/` line-range typo, and one `_docs/usage.md` accuracy fix. See §13 for the per-item details.

Status: tech plan, no code. Builds on Don's `02-plan.md`. Reviewed targets:
`internal/deploy/service.go`, `internal/deploy/readiness.go`, `internal/deploy/lifecycle.go`,
`internal/dockerdrv/cli_driver.go`, `internal/dockerdrv/driver.go`,
`internal/deploy/service_test.go`, `internal/deploy/readiness_test.go`,
`internal/cli/exit_codes.go`, `internal/cli/exit_codes_test.go`,
`cmd/decloud/main.go`.

The TL;DR for Rob: this is a context-discipline fix in `internal/deploy/service.go` plus
one error-wrap nudge in `internal/deploy/readiness.go` plus one new pre-Run defensive
branch in `internal/deploy/service.go` plus a new `ExitInterrupted` constant in
`internal/cli/exit_codes.go`. **Zero new methods on the `dockerdrv.Driver` interface; ONE field added to `dockerdrv.InspectResult` (`Labels map[string]string`) per v2 revision §3.5; one argv change to `cliDriver.Inspect`'s `--format`; mocks regenerated to pick up the new field (no new mock methods).**

---

## 1. Functional spec — what the user experiences

### 1.1 Pre-fix behavior (the bug)

```
$ decloud deploy service --name foo --port 8080 --readiness-path /healthz ./app
[build...]
[run...]
[probe waiting for /healthz...]
^C
Error: deploy: readiness probe failed: context canceled
$ echo $?
50                                  # ExitReadinessFail — WRONG, user cancelled
$ docker ps -a | grep decloud-foo
abc123  decloud-foo:...  Up 2 minutes  decloud-foo   # leaked
$ decloud deploy service --name foo --port 8080 ...
Error: deploy: docker run failed: ...
  Conflict. The container name "/decloud-foo" is already in use ...
```

### 1.2 Post-fix behavior

```
$ decloud deploy service --name foo --port 8080 --readiness-path /healthz ./app
[build...]
[run...]
[probe waiting for /healthz...]
^C
Error: deploy: cancelled by user                       # distinct from readiness fail
$ echo $?
130                                                    # ExitInterrupted (128 + SIGINT)
$ docker ps -a | grep decloud-foo
                                                       # gone — auto-cleaned
$ decloud deploy service --name foo --port 8080 ...    # works first time
```

If the docker daemon is hung when ctrl+c is pressed, cleanup waits up to 30s
total then surfaces a single warning to stderr (via `slog.Warn`) naming the
container the user must remove manually. The deploy still exits with
`ExitInterrupted`.

### 1.3 Edge cases the spec must cover

1. **SIGINT during readiness wait, first deploy.** Container removed, exit 130.
2. **SIGINT during readiness wait, redeploy.** New container removed AND old
   container restored, exit 130.
3. **SIGINT after `docker run` returns but before probe enters its first tick.**
   Same as case 1: container removed.
4. **SIGINT during `docker build` or `docker run`.** Build/run already
   propagate `context.Canceled`; the wrapping change at §3.4 maps these to
   `ErrInterrupted` so the exit code is 130 regardless of where SIGINT lands
   inside `Deploy`.
5. **SIGINT during pre-Run stop+remove of the previous container** (`hasPrev`
   redeploy, lines 172–185). Currently fail-fast leaves user with old
   container possibly half-stopped; cleanup-context discipline at §3.1 keeps
   stop+remove finishing.
6. **Defensive orphan cleanup on a fresh first-deploy after a previous
   interrupted deploy.** `Store.Load` → `ErrNotFound`, `decloud-<svc>` exists
   → `Stop` + `Remove` runs before `Run`, with `slog.Warn` audit line.
7. **Defensive orphan cleanup detection: container does not exist.** No-op,
   no log.
8. **Cleanup of cleanup failure.** If `docker stop` fails inside the cleanup
   path, `docker rm` still runs. If both fail, the deploy still surfaces the
   original error (cancellation or readiness failure), with a `slog.Warn`
   naming the container.
9. **Double ctrl+c.** First SIGINT cancels the request ctx and triggers
   cleanup with the cleanup ctx (which is independent). Second SIGINT does
   not cancel the cleanup ctx; the user must wait the 30s budget or
   kill -9. Documented.
10. **Probe non-cancellation failure** (real readiness timeout). Unchanged
    behavior — wraps as `ErrReadiness`, cleanup runs (with cleanup ctx),
    exit 50. The existing `TestDeploy_ReadinessFailureRollsBackToOld` covers
    this and must keep passing.

### 1.4 Exact error messages

(For Rob — paste these verbatim. The substrings are the contract.)

- Cancellation surfaced from `Deploy`:
  `"deploy: cancelled by user"` (the `ErrInterrupted.Error()` text). Wrapped
  inner `context.Canceled` is preserved via `%w` so
  `errors.Is(err, context.Canceled)` is true.

- Cleanup-failure warning (slog.Warn, structured):
  `"cleanup failed; please remove decloud-<svc> manually" container=decloud-<svc> error=<err>`

- Defensive orphan cleanup audit (slog.Warn, structured):
  `"removed orphan container from prior interrupted deploy" container=decloud-<svc> state=<running|exited>"`

- Defensive orphan cleanup failure (returned from `Deploy`):
  `fmt.Errorf("%w: cleaning up orphan container %s; please run 'docker rm -f %s' and retry: %w", ErrRun, name, name, err)`

---

## 2. The cleanup-context contract

This is the structural fix. Read it twice; everything else hangs off it.

### 2.1 Two contexts, two roles

Inside `Deploy`, two contexts coexist:

- **`ctx`** (parameter) — the *forward-progress* context. Caller cancels it
  via SIGINT. Used for: `NetworkEnsure`, `Capture`, `Store.Load`, `Build`,
  the **defensive** `Inspect` + `Stop` + `Remove` (§3.5; this is forward
  progress, not cleanup), the **scheduled redeploy** `Stop` + `Remove` at
  lines 172–185 (this stays on `ctx` — see §3.2 for why), `Run`, `probe.Wait`,
  `Store.Save`, and `regenerateAndReload`.

- **`cleanupCtx`** — derived locally inside `Deploy` from
  `context.Background()` plus a 30-second timeout. Used **only** for the
  three cleanup blocks: probe-failure cleanup (`:215–:218`), save-failure
  cleanup (`:265–:269`), and the body of `restoreOldContainer`.

### 2.2 Why the deploy service constructs the cleanup context, not the driver

Don asked Joel to settle this. The answer is **the deploy service constructs
it locally**. Reasons, ranked:

1. **Cleanup-context-ness is a policy of the orchestrator**, not of the
   driver. The driver does what it's told; if the orchestrator hands it a
   non-cancellable context, the driver respects it. Pushing this knowledge
   into `Driver.Stop`/`Remove` would force every driver implementation
   (current `cliDriver`, future `dockerdaemonDriver`, future fake drivers)
   to re-implement the same policy, with different timeout choices and
   different error styles.

2. **Driver methods are already context-correct**. They thread `ctx` to
   `exec.CommandContext` and that's it. No interface change needed; we just
   pass a different context value.

3. **Zero blast radius for the driver tests.** `cli_driver_test.go` uses
   `context.Background()` everywhere — the cleanup-context is also
   non-cancellable, so the existing argv assertions and exec behavior are
   untouched. No new mocks, no new driver methods, no `go generate` run for
   `mocks/mock_driver.go`.

4. **Symmetry with how lifecycle.go handles things.** `Unregister`, `Stop`,
   etc. use the request ctx without distinction because they're not in a
   "post-failure cleanup" position. This fix preserves that distinction
   and keeps a clean conceptual line: "cleanup paths take a cleanup ctx;
   forward-progress paths take the request ctx."

If Linus argues the other direction (driver-side timeout default), the
counter is: Caddy's reload logic, the registry's save logic, and any future
in-process subprocess will face the same cleanup question and they're not
all driver-shaped. The orchestrator-owned policy generalizes.

### 2.3 The helper

Single private helper in `internal/deploy/service.go`, defined near the
sentinels at line 23–29. Exact signature:

```go
// cleanupTimeout bounds post-failure cleanup work. Sized to permit a
// 10s docker stop grace + 10s buffer for docker rm + 10s buffer for
// rollback docker run. NEVER tied to the request context.
const cleanupTimeout = 30 * time.Second

// newCleanupContext returns a context derived from context.Background()
// with cleanupTimeout. It is intentionally NOT derived from the caller's
// ctx so that user-cancellation does not abort cleanup. Callers MUST
// invoke the returned cancel func via defer.
func newCleanupContext() (context.Context, context.CancelFunc) {
    return context.WithTimeout(context.Background(), cleanupTimeout)
}
```

`time` is already imported in `service.go`. `context` is already imported.
No new imports.

Each cleanup block in `Deploy` constructs its own cleanup context:

```go
cleanupCtx, cleanupCancel := newCleanupContext()
defer cleanupCancel()
_ = d.deps.Driver.Stop(cleanupCtx, containerName, 10*time.Second)
_ = d.deps.Driver.Remove(cleanupCtx, containerName)
if hasPrev {
    d.restoreOldContainer(cleanupCtx, prev)
}
```

Rob: do **not** lift `cleanupCtx` to a function-scoped variable created at
the top of `Deploy`. The cleanup ctx is created on demand at each cleanup
site so that:
- The 30s budget starts when cleanup starts, not when the deploy starts.
- A successful happy-path deploy that takes 5 minutes does not have a
  near-expired cleanup ctx if cleanup is later needed.

---

## 3. Function-by-function changes

For each change: file, line range, current text shape, new text shape, why.

### 3.1 New sentinel `ErrInterrupted` in `internal/deploy/service.go`

**File:** `internal/deploy/service.go`
**Location:** existing var block at line 23–29

**Current:**
```go
var (
    ErrEnvCapture  = errors.New("deploy: env capture failed")
    ErrBuild       = errors.New("deploy: docker build failed")
    ErrRun         = errors.New("deploy: docker run failed")
    ErrReadiness   = errors.New("deploy: readiness probe failed")
    ErrCaddyReload = errors.New("deploy: caddy reload failed")
)
```

**New (add one line):**
```go
var (
    ErrEnvCapture  = errors.New("deploy: env capture failed")
    ErrBuild       = errors.New("deploy: docker build failed")
    ErrRun         = errors.New("deploy: docker run failed")
    ErrReadiness   = errors.New("deploy: readiness probe failed")
    ErrCaddyReload = errors.New("deploy: caddy reload failed")
    ErrInterrupted = errors.New("deploy: cancelled by user")
)
```

**Why:** Don's Shape A vs Shape B trade-off. I pick **a lightly hybridized
Shape B**: introduce `ErrInterrupted` because we need a *named* sentinel for
the exit code mapping (§4) and we want the user-visible message to read
"cancelled by user" rather than "readiness probe failed: context canceled".
The probe still wraps with `ctx.Err()` as in Shape A (§3.3); the orchestrator
detects that wrapping and re-wraps as `ErrInterrupted` on its way out of
`Deploy`. That keeps the probe interface unchanged AND gives us a stable
sentinel.

### 3.2 Add helper `newCleanupContext` and `cleanupTimeout` const

**File:** `internal/deploy/service.go`
**Location:** immediately after the var block (so around line 30, before
`type Request struct`).

**New code:** see §2.3 verbatim.

### 3.3 `internal/deploy/readiness.go` — return raw `ctx.Err()` (v2 revision)

**File:** `internal/deploy/readiness.go`
**Location:** lines 68–72 (the `select` inside `Wait`).

**Current:**
```go
select {
case <-ctx.Done():
    return ctx.Err()
case <-time.After(interval):
}
```

**v1 (rejected) attempted:** `return fmt.Errorf("readiness: %w", ctx.Err())`. Linus correctly flagged that the "readiness:" prefix is misleading — the whole point of the change is to STOP treating cancellation as a readiness failure, and inserting "readiness:" into the error chain produces a user-visible message like "deploy: cancelled by user: readiness: context canceled" which lies about what happened. v1's rationale ("the existing test accepts 'context canceled' substrings") was self-cancelling because §5.7 changes that test.

**New (v2):**
```go
select {
case <-ctx.Done():
    return ctx.Err()
case <-time.After(interval):
}
```

**i.e. NO CHANGE** to `readiness.go` lines 68-72. Leave the existing `return ctx.Err()` exactly as-is. The fix moves entirely to the orchestrator side (§3.4.1), which now (a) detects cancellation by `errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)` and re-wraps as `ErrInterrupted`, and (b) forks the audit log line so a cancellation isn't logged as "readiness failed."

**Why this is right:** the probe's job is to report what it observed. It observed a context cancellation; reporting `ctx.Err()` is honest. Adding an audit prefix at the probe layer just gives the orchestrator more strings to discriminate around. The orchestrator already has full context (it knows the deploy ID, the service name, and that this is the readiness step) — let the orchestrator decorate the audit; let the probe stay trivial.

**Important:** do NOT change anything else in `readiness.go`. The other return sites at lines 64 and 66 wrap with `ErrReadiness` and are correct. The `fmt` import in `readiness.go` was already used by those existing wraps; no import changes needed.

### 3.4 `internal/deploy/service.go` `Deploy` — cleanup context discipline

This is the headline change. Walk through it block by block.

#### 3.4.1 Probe-failure cleanup (lines 213–224)

**Current:**
```go
if err := d.probe.Wait(ctx, containerName, spec, req.Port); err != nil {
    logger.Error("readiness failed", "step", "readiness", "error", err)
    _ = d.deps.Driver.Stop(ctx, containerName, 10*time.Second)
    _ = d.deps.Driver.Remove(ctx, containerName)
    if hasPrev {
        d.restoreOldContainer(ctx, prev)
    }
    if errors.Is(err, ErrReadiness) {
        return err
    }
    return fmt.Errorf("%w: %w", ErrReadiness, err)
}
```

**New (v2 — audit log forked on cancellation vs failure):**
```go
if err := d.probe.Wait(ctx, containerName, spec, req.Port); err != nil {
    if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
        logger.Info("deploy cancelled during readiness wait", "step", "readiness")
    } else {
        logger.Error("readiness failed", "step", "readiness", "error", err)
    }
    cleanupCtx, cleanupCancel := newCleanupContext()
    defer cleanupCancel()
    if stopErr := d.deps.Driver.Stop(cleanupCtx, containerName, 10*time.Second); stopErr != nil && !errors.Is(stopErr, dockerdrv.ErrContainerNotFound) {
        slog.Warn("cleanup failed; please remove "+containerName+" manually",
            "container", containerName, "error", stopErr)
    }
    if rmErr := d.deps.Driver.Remove(cleanupCtx, containerName); rmErr != nil && !errors.Is(rmErr, dockerdrv.ErrContainerNotFound) {
        slog.Warn("cleanup failed; please remove "+containerName+" manually",
            "container", containerName, "error", rmErr)
    }
    if hasPrev {
        d.restoreOldContainer(cleanupCtx, prev)
    }
    if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
        return fmt.Errorf("%w: %w", ErrInterrupted, err)
    }
    if errors.Is(err, ErrReadiness) {
        return err
    }
    return fmt.Errorf("%w: %w", ErrReadiness, err)
}
```

**v2 change:** the `logger.Error("readiness failed", ...)` line at the top of the block is now branched. Cancellation logs at Info, real readiness failure logs at Error. Per Linus Issue 3 — don't lie in the audit log. The user pressed ctrl+c; the readiness probe didn't *fail*, it was interrupted. Calling that an Error in the log is wrong.

The cancellation predicate is computed twice (once for the log fork, once for the return wrap). Rob: this is fine. Don't extract it to a local variable for two-call savings; the explicit form reads better. If Linus pushes back on duplication, hoist to `cancelled := errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)` at the top of the block.

**Notes for Rob:**
- The `defer cleanupCancel()` lives inside the `if err != nil` block, so it
  fires at the end of the `Deploy` function (defers are function-scoped, not
  block-scoped). That's fine — we want the context alive through the
  `restoreOldContainer` call.
- `slog.Warn` not `_ =` for cleanup errors. We surface, but never fail the
  deploy on cleanup failure (the deploy already failed).
- `dockerdrv.ErrContainerNotFound` is *not* an error worth warning about
  — the container was already gone, which is the desired post-state.
- The cancellation check uses `context.Canceled` AND `context.DeadlineExceeded`
  because `context.WithTimeout` upstream of us could surface either, and
  both are "user / OS cancelled" semantics distinct from readiness failure.

#### 3.4.2 Save-failure cleanup (lines 258–271)

**Current:**
```go
if err := d.deps.Store.Save(ctx, svc); err != nil {
    logger.Error("registry save failed", "step", "save_registry", "error", err)
    if errors.Is(err, registry.ErrPartialWrite) {
        if rbErr := d.deps.Store.DeleteOrphanConfig(ctx, req.Name); rbErr != nil {
            logger.Error("rollback: delete orphan config failed", "error", rbErr)
        }
    }
    _ = d.deps.Driver.Stop(ctx, containerName, 10*time.Second)
    _ = d.deps.Driver.Remove(ctx, containerName)
    if hasPrev {
        d.restoreOldContainer(ctx, prev)
    }
    return fmt.Errorf("registry save: %w", err)
}
```

**New:**
```go
if err := d.deps.Store.Save(ctx, svc); err != nil {
    logger.Error("registry save failed", "step", "save_registry", "error", err)
    cleanupCtx, cleanupCancel := newCleanupContext()
    defer cleanupCancel()
    if errors.Is(err, registry.ErrPartialWrite) {
        if rbErr := d.deps.Store.DeleteOrphanConfig(cleanupCtx, req.Name); rbErr != nil {
            logger.Error("rollback: delete orphan config failed", "error", rbErr)
        }
    }
    if stopErr := d.deps.Driver.Stop(cleanupCtx, containerName, 10*time.Second); stopErr != nil && !errors.Is(stopErr, dockerdrv.ErrContainerNotFound) {
        slog.Warn("cleanup failed; please remove "+containerName+" manually",
            "container", containerName, "error", stopErr)
    }
    if rmErr := d.deps.Driver.Remove(cleanupCtx, containerName); rmErr != nil && !errors.Is(rmErr, dockerdrv.ErrContainerNotFound) {
        slog.Warn("cleanup failed; please remove "+containerName+" manually",
            "container", containerName, "error", rmErr)
    }
    if hasPrev {
        d.restoreOldContainer(cleanupCtx, prev)
    }
    if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
        return fmt.Errorf("%w: %w", ErrInterrupted, err)
    }
    return fmt.Errorf("registry save: %w", err)
}
```

**Note:** The save path could also get cancelled (Store.Save honors `ctx`),
so we map that to `ErrInterrupted` too. `DeleteOrphanConfig` runs on
`cleanupCtx` for the same reason — it's cleanup, not forward progress.

#### 3.4.3 Run-failure rollback (lines 195–201)

**Current:**
```go
if _, err := d.deps.Driver.Run(ctx, runReq); err != nil {
    logger.Error("run new container failed", "step", "run_new", "error", err)
    if hasPrev {
        d.restoreOldContainer(ctx, prev)
    }
    return fmt.Errorf("%w: %w", ErrRun, err)
}
```

**New:**
```go
if _, err := d.deps.Driver.Run(ctx, runReq); err != nil {
    logger.Error("run new container failed", "step", "run_new", "error", err)
    if hasPrev {
        cleanupCtx, cleanupCancel := newCleanupContext()
        defer cleanupCancel()
        d.restoreOldContainer(cleanupCtx, prev)
    }
    if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
        return fmt.Errorf("%w: %w", ErrInterrupted, err)
    }
    return fmt.Errorf("%w: %w", ErrRun, err)
}
```

**Note:** the `defer` is fine inside the `if hasPrev` block — Go defers are
function-scoped, so the cancel fires at function return. Idiomatic.

#### 3.4.4 `restoreOldContainer` body (lines 282–300)

**Decision:** Do NOT change the signature. The function already takes a
`ctx context.Context`. The fix is to ensure callers pass the cleanup ctx,
which §3.4.1, §3.4.2, §3.4.3 now do.

**Optional defensive change Rob may include if it reads cleaner:** rename
the parameter from `ctx` to `cleanupCtx` to make the call-site contract
self-documenting:

```go
func (d *serviceDeployer) restoreOldContainer(cleanupCtx context.Context, prev *registry.Service) {
    ...
    if _, err := d.deps.Driver.Run(cleanupCtx, runReq); err != nil {
```

Joel's preference: yes, rename. It's a one-letter change, and the next
maintainer reading the body will not wonder "is this the request ctx or
the cleanup ctx?" — the parameter name answers it.

#### 3.4.5 The pre-Run scheduled-redeploy stop+remove (lines 172–185) STAYS ON `ctx`

This is the existing branch that runs on a planned redeploy when
`hasPrev == true`. Don's plan does not call this out as a target, and it
shouldn't be one. Rationale:

- This is **forward progress**, not post-failure cleanup. The user invoked
  `decloud deploy service`, expects the old container to stop, and is
  entitled to abort the deploy (ctrl+c) before the new container starts.
- If the user ctrl+c's during this window, the natural behavior is "deploy
  aborted, old container may be in a half-stopped state, no new container
  exists" — and the next deploy retry will encounter the same `hasPrev`
  branch and complete the stop+remove cleanly. No leak.
- Switching this to cleanup ctx would mean "ctrl+c during a planned redeploy
  is ignored until stop+remove finish" — surprising and against the user's
  explicit intent.

Rob: leave lines 172–185 alone except for what §3.5 adds *between* line 185
and line 195 (the defensive orphan branch).

### 3.5 Defensive orphan cleanup before `Driver.Run` — label-gated (v2 revision)

**File:** `internal/deploy/service.go`
**Location:** between line 185 (closing `}` of `if hasPrev`) and line 187
(`runReq := dockerdrv.RunRequest{`).

**v1 (rejected) shape:** stop+remove any container named `decloud-<svc>` whose registry entry is missing. Linus correctly flagged that this silently destroys ANY container with the right name — including a manually-`docker run`-named container or a still-running production container after a user blew away their registry dir thinking they were "starting fresh." See `04-linus-review.md` Issue 1.

**v2 fix:** add a label gate. Only destroy containers that carry the `decloud.service=<req.Name>` label that `cliDriver.Run` already attaches at `cli_driver.go:60`. If the container exists but the label is missing or mismatched, refuse with a clear error and a manual recovery hint.

**Required driver-side prerequisite:** extend `dockerdrv.InspectResult` with a `Labels map[string]string` field. See §3.5.1 below.

**New code:**

```go
if !hasPrev {
    inspect, err := d.deps.Driver.Inspect(ctx, containerName)
    if err != nil {
        if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
            return fmt.Errorf("%w: %w", ErrInterrupted, err)
        }
        return fmt.Errorf("%w: inspect orphan check %s: %w", ErrRun, containerName, err)
    }
    if inspect.State != "absent" {
        if inspect.Labels["decloud.service"] != req.Name {
            return fmt.Errorf("%w: container %s exists but was not created by decloud (label decloud.service=%q does not match %q); refusing to remove. Run 'docker rm -f %s' manually if you want to claim this name, or pick a different service name",
                ErrRun, containerName, inspect.Labels["decloud.service"], req.Name, containerName)
        }
        logger.Warn("removed orphan container from prior interrupted deploy",
            "container", containerName, "state", inspect.State)
        if err := d.deps.Driver.Stop(ctx, containerName, 10*time.Second); err != nil && !errors.Is(err, dockerdrv.ErrContainerNotFound) {
            if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
                return fmt.Errorf("%w: %w", ErrInterrupted, err)
            }
            return fmt.Errorf("%w: cleaning up orphan container %s; please run 'docker rm -f %s' and retry: %w", ErrRun, containerName, containerName, err)
        }
        if err := d.deps.Driver.Remove(ctx, containerName); err != nil && !errors.Is(err, dockerdrv.ErrContainerNotFound) {
            if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
                return fmt.Errorf("%w: %w", ErrInterrupted, err)
            }
            return fmt.Errorf("%w: cleaning up orphan container %s; please run 'docker rm -f %s' and retry: %w", ErrRun, containerName, containerName, err)
        }
    }
}
```

**v2.1 cancellation discrimination at three sites (Linus 06-review follow-up):** the §3.5 branch runs on the request `ctx` (forward progress, user CAN abort). If the user ctrl+c's during the orphan inspect/stop/remove, the underlying call returns `context.Canceled`. Without the three new conditionals above, that error wraps as `ErrRun` and `ExitCodeFor` maps it to exit 40 — the user pressed ctrl+c but sees "run failure" semantics. The fix is symmetric to §3.4.3: detect cancellation, wrap as `ErrInterrupted`, surface exit 130. Three sites, six lines.

**Failure-mode matrix (this is the contract; Kent will test all four):**

| `inspect.State` | `Labels["decloud.service"]` | Behavior |
|---|---|---|
| `"absent"` | (irrelevant) | No-op. Most common case. No log. |
| `"running"` or `"exited"` | == `req.Name` | Stop+remove, audit log at Warn, proceed to Run. |
| `"running"` or `"exited"` | != `req.Name` (mismatched) | Return `ErrRun` with manual recovery hint. NO destruction. |
| `"running"` or `"exited"` | missing (nil map / no entry) | Same as mismatched: return `ErrRun`. `inspect.Labels["decloud.service"]` returns the empty string for a missing key in a `map[string]string`, which is != `req.Name` (which is non-empty by validation). |
| Inspect/Stop/Remove returns `context.Canceled` or `context.DeadlineExceeded` | (irrelevant) | Return `ErrInterrupted` wrapping the ctx error. User pressed ctrl+c during the orphan check — surface exit 130, not exit 40. |
| Inspect itself errors (non-cancellation) | (irrelevant) | Return `ErrRun` wrapping the inspect error. |

**Notes for Rob:**

- This block runs ONLY when `!hasPrev` — Don's "guardrail" in §3.3 of his plan. The `hasPrev` branch already handles its own stop+remove and a bare-eyes orphan and a registered-prev are different domains.
- Uses the request `ctx`, not cleanup ctx. This is forward progress. User CAN abort it.
- `Driver.Inspect` returns `InspectResult{State: "absent"}` when the container does not exist (see `cli_driver.go:120`). `state != "absent"` is the "orphan exists" condition; possible states are "running" or "exited" (per `dockerdrv/driver.go:42`).
- The error wraps use `ErrRun` (matches existing error taxonomy: failure to set up the run environment) and include a copy-pasteable `docker rm -f <name>` recovery hint.
- `logger.Warn` since `logger` already exists at line 129 with the deploy_id and service fields. Match the local convention.
- The label-mismatch error message includes the mismatched label value so the user can see WHY decloud refused (e.g. `decloud.service="bar"` when they're trying to deploy `foo`).

### 3.5.1 Driver interface widening — `InspectResult.Labels`

**File:** `internal/dockerdrv/driver.go`
**Location:** lines 40-43 (the `InspectResult` struct).

**Current:**
```go
type InspectResult struct {
    ContainerID string
    State       string // "running" | "exited" | "absent"
}
```

**New:**
```go
type InspectResult struct {
    ContainerID string
    State       string            // "running" | "exited" | "absent"
    Labels      map[string]string // container labels; nil when State == "absent"
}
```

**File:** `internal/dockerdrv/cli_driver.go`
**Location:** lines 113-129 (`cliDriver.Inspect`).

The current implementation uses `--format "{{.Id}} {{.State.Status}}"` and parses two whitespace-separated fields. v2 needs to also emit labels.

**Recommended new shape — JSON output for robustness:**

```go
func (d *cliDriver) Inspect(ctx context.Context, name string) (InspectResult, error) {
    var stdout, stderr bytes.Buffer
    cmd := d.cmd(ctx, "docker", "inspect", name,
        "--format", `{"id":{{json .Id}},"state":{{json .State.Status}},"labels":{{json .Config.Labels}}}`)
    cmd.Stdout = &stdout
    cmd.Stderr = &stderr
    if err := cmd.Run(); err != nil {
        if isNotFound(stderr.String()) {
            return InspectResult{State: "absent"}, nil
        }
        return InspectResult{}, fmt.Errorf("docker inspect: %w; stderr=%q", err, stderr.String())
    }
    var out struct {
        ID     string            `json:"id"`
        State  string            `json:"state"`
        Labels map[string]string `json:"labels"`
    }
    if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
        return InspectResult{}, fmt.Errorf("docker inspect: parse output %q: %w", stdout.String(), err)
    }
    return InspectResult{ContainerID: out.ID, State: out.State, Labels: out.Labels}, nil
}
```

This requires importing `encoding/json` in `cli_driver.go`. Rob: verify the import isn't already present; add if not.

**Why JSON over the old whitespace shape:** labels can contain spaces, equals signs, JSON-special characters. The old `{{.Id}} {{.State.Status}}` format works because IDs and states are space-free atoms; labels aren't. JSON-encoding the whole result removes any ambiguity.

**`cli_driver_test.go` impact:** the existing test expectations on `Inspect` argv (if any) widen by one format-string change. Rob: read the test file before editing; any test that asserts argv-byte-for-byte for `docker inspect` updates to the new format string. The test for the State parsing updates to the new JSON parsing.

**Mock regeneration:** run `go generate ./...` after editing `dockerdrv/driver.go`. This regenerates `internal/dockerdrv/mocks/mock_driver.go`. Since `InspectResult` is a value type with a new field, the regenerated mock should pick it up automatically — gomock generates per-method, not per-result-field. **No new mock methods.** Existing tests that construct `InspectResult{State: "absent"}` continue to compile (the new `Labels` field is zero-valued nil, which is fine).

**`internal/deploy/lifecycle.go` impact:** `Lifecycle.Start` and `Lifecycle.Status` call `Driver.Inspect`. Currently they only read `inspect.State`. They will continue to ignore `inspect.Labels`; no behavior change. Rob: no edits needed in lifecycle.go for v2.

### 3.6 New exit code `ExitInterrupted` in `internal/cli/exit_codes.go`

**File:** `internal/cli/exit_codes.go`

**Current (lines 13–23):**
```go
const (
    ExitOK              = 0
    ExitUsageError      = 2
    ExitConfigError     = 10
    ExitEnvCaptureFail  = 20
    ExitBuildFail       = 30
    ExitRunFail         = 40
    ExitReadinessFail   = 50
    ExitCaddyReloadFail = 60
    ExitInternal        = 70
)
```

**New:**
```go
const (
    ExitOK              = 0
    ExitUsageError      = 2
    ExitConfigError     = 10
    ExitEnvCaptureFail  = 20
    ExitBuildFail       = 30
    ExitRunFail         = 40
    ExitReadinessFail   = 50
    ExitCaddyReloadFail = 60
    ExitInternal        = 70
    ExitInterrupted     = 130 // 128 + SIGINT(2); POSIX convention
)
```

**And update `ExitCodeFor` (lines 32–66):** add a new case **before**
`ErrEnvCapture` so cancellation always wins over more-specific deploy
sentinels (a cancelled probe error chain would otherwise satisfy
`errors.Is(err, ErrReadiness)` if we ever stopped re-wrapping; defense in
depth):

```go
case errors.Is(err, deploy.ErrInterrupted):
    return ExitInterrupted
```

**Note (v2 revision):** v1 also matched `errors.Is(err, context.Canceled)` and `errors.Is(err, context.DeadlineExceeded)` here. Linus correctly flagged that as over-broad — those matches would catch ANY error chain containing a context error, not just user SIGINT. Concrete trap: a future caller using `context.WithTimeout` for some unrelated reason would see exit 130 if the timeout fires for any internal reason, and the user would think they pressed ctrl+c. Today this is harmless because the only path producing context errors goes through `Deploy` which now wraps as `ErrInterrupted`. Tomorrow it's a footgun — the same shape of trap the readiness probe fell into.

**v2 fix:** match only `deploy.ErrInterrupted`. `Deploy` is the choke point and wraps every cancellation path (§3.4.1, §3.4.2, §3.4.3) as `ErrInterrupted`. If a future code path bypasses `Deploy` and emits raw `context.Canceled`, we'll add a top-level `main.go` override (Linus's Option C) at that time — but until that exists, don't paint ourselves into the corner.

**Import note:** v1 said this requires importing `"context"` in `exit_codes.go`. After the v2 simplification, the `context` import is NOT needed unless other cases in `ExitCodeFor` already need it. Rob: verify the existing imports; add `context` only if some other case requires it.

---

## 4. Code archaeology — what to reuse, verbatim

These are the patterns and helpers Rob should NOT reinvent.

### 4.1 `slog.Warn`/`slog.Error` audit pattern

Existing call site: `internal/deploy/lifecycle.go:26-29` — `Unregister`
already uses the exact "stop failed during unregister" / "remove failed
during unregister" pattern. Match its shape.

### 4.2 `dockerdrv.ErrContainerNotFound` filter

Existing pattern: `internal/deploy/lifecycle.go:25-30` — `Stop`/`Remove`
errors filtered for `ErrContainerNotFound`. The new cleanup blocks must do
the same: a not-found container is desired post-state, not a warning event.

### 4.3 `%w: %w` wrap discipline

`_ai/error-wrap-discipline.md`. Every new `fmt.Errorf` in this fix must use
`%w`, never `%v`, for inner errors. Already enforced by
`TestDeploy_BuildErrorPreservesInnerSentinel` at `service_test.go:551-566`.

### 4.4 Test fixture helpers

Reuse from `internal/deploy/service_test.go`:
- `newDeployerHarness(t)` — line 69. **v2 revision:** add a default expectation for `Driver.Inspect` returning `InspectResult{State: "absent"}` with `.AnyTimes()`. See §5.9 below for rationale and the exact addition.
- `newRequest()` — line 107
- `newPrev()` — line 121
- `passThroughProbe` (line 38) — for tests that don't need probe behavior
- `stubGenerate` (line 30) — for happy-path Generator setup
- New helper `newDeployerHarnessWithProbe(t, probe)` (per §5.6) — accepts a probe override; `newDeployerHarness` delegates with `&passThroughProbe{driver: driver}`.

### 4.5 `gomock.InOrder` for sequencing

`_ai/gomock-inorder-sequencing.md`. New tests that assert "X happens
before Y" use `gomock.InOrder`, not state spying.

### 4.6 No new mocks

The existing `internal/dockerdrv/mocks/mock_driver.go` covers `Inspect`,
`Stop`, `Remove`, `Run` — everything we need. Do not run `go generate`.

---

## 5. Tests — what Kent writes (verbatim cases)

Kent: read this section twice. Each test below is a separate `func Test...`.
Naming follows the existing convention in `service_test.go` (descriptive
verb-phrase suffix).

### 5.1 New: `TestDeploy_ProbeCancellationCleansUpWithFreshContext`

**Location:** `internal/deploy/service_test.go`, append after
`TestDeploy_ReadinessFailureRollsBackToOld` (around line 287).

**Purpose:** Verify the headline bug is fixed.

**Probe substitution:** the harness uses `passThroughProbe` which calls
`Driver.ContainerIP`. For this test we need a probe that *blocks* until
ctx is cancelled. Two equally valid options:

- **Option A (preferred):** define a local `cancellingProbe` struct in
  the test file. Wait method blocks on `<-ctx.Done()` and returns
  `fmt.Errorf("readiness: %w", ctx.Err())` — exactly mimicking the new
  `httpProbe.Wait` cancellation path. Inject via `Dependencies.Probe`.
  Define this struct once in `service_test.go`; reuse for tests 5.1, 5.2, 5.4.

```go
type cancellingProbe struct{}

func (cancellingProbe) Wait(ctx context.Context, _ string, _ registry.ReadinessSpec, _ int) error {
    <-ctx.Done()
    return fmt.Errorf("readiness: %w", ctx.Err())
}
```

**Test body shape:**

```go
func TestDeploy_ProbeCancellationCleansUpWithFreshContext(t *testing.T) {
    ctrl := gomock.NewController(t)
    // build harness with cancellingProbe instead of passThroughProbe
    // (use direct deploy.NewServiceDeployer call, not newDeployerHarness,
    // because the harness hardcodes passThroughProbe; OR add a probe-override
    // parameter to newDeployerHarness — see §5.6)

    ctx, cancel := context.WithCancel(context.Background())

    h.driver.EXPECT().NetworkEnsure(gomock.Any(), "decloud").Return(nil)
    h.capturer.EXPECT().Capture(gomock.Any(), gomock.Any()).Return(map[string]string{"X": "1"}, nil)
    h.store.EXPECT().Load(gomock.Any(), "foo").Return(nil, registry.ErrNotFound)
    h.driver.EXPECT().Build(gomock.Any(), gomock.Any()).Return("img", nil)
    h.driver.EXPECT().Inspect(gomock.Any(), "decloud-foo").
        Return(dockerdrv.InspectResult{State: "absent"}, nil)            // §3.5 defensive
    h.driver.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(
        func(_ context.Context, _ dockerdrv.RunRequest) (string, error) {
            cancel()                                                      // simulate ^C
            return "cid", nil
        })
    // KEY ASSERTION: cleanup Stop/Remove must receive a non-cancelled ctx.
    h.driver.EXPECT().Stop(notCancelledCtx(), "decloud-foo", gomock.Any()).Return(nil)
    h.driver.EXPECT().Remove(notCancelledCtx(), "decloud-foo").Return(nil)

    err := h.deployer.Deploy(ctx, newRequest())
    require.Error(t, err)
    assert.True(t, errors.Is(err, deploy.ErrInterrupted))
    assert.True(t, errors.Is(err, context.Canceled))
    assert.False(t, errors.Is(err, deploy.ErrReadiness))
}
```

**The custom matcher** (define once at top of `service_test.go`, reuse):

```go
// notCancelledCtxMatcher asserts that the captured argument is a context
// whose Err() is nil at the moment the mock was invoked. Used to verify
// cleanup paths receive a fresh, non-cancelled context.
type notCancelledCtxMatcher struct{}

func (notCancelledCtxMatcher) Matches(x any) bool {
    ctx, ok := x.(context.Context)
    if !ok {
        return false
    }
    return ctx.Err() == nil
}

func (notCancelledCtxMatcher) String() string {
    return "is a context with Err() == nil at call time"
}

func notCancelledCtx() gomock.Matcher { return notCancelledCtxMatcher{} }
```

**Why this matcher works:** gomock evaluates matchers at the moment of
the call (when `cmd.Run()` would be invoked in production). At that point,
if the orchestrator has correctly created a fresh context, `ctx.Err()` is
nil. If it incorrectly reused the user's cancelled ctx, `ctx.Err()` is
`context.Canceled` and the matcher rejects.

**Why this is NOT a change-detector test:** it asserts a contract — "cleanup
runs with a non-cancelled context" — observable through the driver
boundary. The implementation is free to construct that context any way it
likes (helper, inline, derived from a different parent) so long as the
property holds. Aligns with `_ai/gomock-inorder-sequencing.md`'s "contract
test, not implementation test" framing.

### 5.2 New: `TestDeploy_ProbeCancellationReturnsErrInterruptedNotErrReadiness`

**Purpose:** Defect A from Don's §1.3. The error chain semantics.

This can be folded into 5.1 (the assertions are already there) OR kept
separate. Joel's preference: **fold into 5.1**. One test, two contracts:
"cleanup runs with fresh ctx" and "error chain is `ErrInterrupted` not
`ErrReadiness`". They're inseparable observations of the same fix.

If Kent prefers to split: separate test, same harness, no cleanup Driver
expectations — just probe cancellation, then assert error chain. But the
harness will still need Stop/Remove expectations or it'll fail with
"unexpected call." So splitting just duplicates setup.

**Decision: one combined test (5.1), drop 5.2.**

### 5.3 New: `TestDeploy_DefensiveOrphanCleanupOnFreshDeployWhenContainerExists`

**Purpose:** §3.5 verification, "orphan exists" case.

```go
func TestDeploy_DefensiveOrphanCleanupOnFreshDeployWhenContainerExists(t *testing.T) {
    h := newDeployerHarness(t)

    gomock.InOrder(
        h.driver.EXPECT().NetworkEnsure(gomock.Any(), "decloud").Return(nil),
        h.capturer.EXPECT().Capture(gomock.Any(), gomock.Any()).Return(map[string]string{"X": "1"}, nil),
        h.store.EXPECT().Load(gomock.Any(), "foo").Return(nil, registry.ErrNotFound),
        h.driver.EXPECT().Build(gomock.Any(), gomock.Any()).Return("img", nil),
        h.driver.EXPECT().Inspect(gomock.Any(), "decloud-foo").
            Return(dockerdrv.InspectResult{
                ContainerID: "orphan-id",
                State:       "running",
                Labels:      map[string]string{"decloud.service": "foo"}, // v2: must match req.Name
            }, nil),
        h.driver.EXPECT().Stop(gomock.Any(), "decloud-foo", gomock.Any()).Return(nil),
        h.driver.EXPECT().Remove(gomock.Any(), "decloud-foo").Return(nil),
        h.driver.EXPECT().Run(gomock.Any(), gomock.Any()).Return("new-cid", nil),
        h.driver.EXPECT().ContainerIP(gomock.Any(), gomock.Any()).Return("172.18.0.5", nil),
        h.store.EXPECT().Save(gomock.Any(), gomock.Any()).Return(nil),
        h.store.EXPECT().List(gomock.Any()).Return(nil, nil),
        h.generator.EXPECT().Generate(gomock.Any(), gomock.Any()).DoAndReturn(stubGenerate),
        h.reloader.EXPECT().Validate(gomock.Any(), gomock.Any()).Return(nil),
        h.reloader.EXPECT().Reload(gomock.Any(), gomock.Any()).Return(nil),
    )

    require.NoError(t, h.deployer.Deploy(context.Background(), newRequest()))
}
```

### 5.4 New: `TestDeploy_DefensiveOrphanCleanupSkippedWhenContainerAbsent`

**Purpose:** §3.5 verification, "no orphan" case. Critical: must NOT call
Stop or Remove.

```go
func TestDeploy_DefensiveOrphanCleanupSkippedWhenContainerAbsent(t *testing.T) {
    h := newDeployerHarness(t)

    gomock.InOrder(
        h.driver.EXPECT().NetworkEnsure(gomock.Any(), "decloud").Return(nil),
        h.capturer.EXPECT().Capture(gomock.Any(), gomock.Any()).Return(map[string]string{"X": "1"}, nil),
        h.store.EXPECT().Load(gomock.Any(), "foo").Return(nil, registry.ErrNotFound),
        h.driver.EXPECT().Build(gomock.Any(), gomock.Any()).Return("img", nil),
        h.driver.EXPECT().Inspect(gomock.Any(), "decloud-foo").
            Return(dockerdrv.InspectResult{State: "absent"}, nil),
        h.driver.EXPECT().Run(gomock.Any(), gomock.Any()).Return("cid", nil),
        h.driver.EXPECT().ContainerIP(gomock.Any(), gomock.Any()).Return("172.18.0.5", nil),
        h.store.EXPECT().Save(gomock.Any(), gomock.Any()).Return(nil),
        h.store.EXPECT().List(gomock.Any()).Return(nil, nil),
        h.generator.EXPECT().Generate(gomock.Any(), gomock.Any()).DoAndReturn(stubGenerate),
        h.reloader.EXPECT().Validate(gomock.Any(), gomock.Any()).Return(nil),
        h.reloader.EXPECT().Reload(gomock.Any(), gomock.Any()).Return(nil),
    )
    // explicit zero expectations on Stop/Remove for the absent case
    h.driver.EXPECT().Stop(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
    h.driver.EXPECT().Remove(gomock.Any(), gomock.Any()).Times(0)

    require.NoError(t, h.deployer.Deploy(context.Background(), newRequest()))
}
```

### 5.5 New: `TestDeploy_DefensiveOrphanCleanupFailureWrapsErrRun`

**Purpose:** §3.5 error path — orphan exists, `Stop` fails, deploy aborts
with `ErrRun` and a copy-pasteable recovery hint.

```go
func TestDeploy_DefensiveOrphanCleanupFailureWrapsErrRun(t *testing.T) {
    h := newDeployerHarness(t)

    h.driver.EXPECT().NetworkEnsure(gomock.Any(), "decloud").Return(nil)
    h.capturer.EXPECT().Capture(gomock.Any(), gomock.Any()).Return(map[string]string{"X": "1"}, nil)
    h.store.EXPECT().Load(gomock.Any(), "foo").Return(nil, registry.ErrNotFound)
    h.driver.EXPECT().Build(gomock.Any(), gomock.Any()).Return("img", nil)
    h.driver.EXPECT().Inspect(gomock.Any(), "decloud-foo").
        Return(dockerdrv.InspectResult{
            State:  "running",
            Labels: map[string]string{"decloud.service": "foo"}, // v2: passes label gate
        }, nil)
    stopErr := errors.New("daemon hung")
    h.driver.EXPECT().Stop(gomock.Any(), "decloud-foo", gomock.Any()).Return(stopErr)

    err := h.deployer.Deploy(context.Background(), newRequest())
    require.Error(t, err)
    assert.True(t, errors.Is(err, deploy.ErrRun))
    assert.True(t, errors.Is(err, stopErr))
    assert.Contains(t, err.Error(), "docker rm -f decloud-foo")
}
```

### 5.6 New: `TestDeploy_RestoreOldContainerUsesFreshContextOnRedeployCancellation`

**Purpose:** Defect B from Don's §1.3 — `restoreOldContainer` on a cancelled
ctx during redeploy.

```go
func TestDeploy_RestoreOldContainerUsesFreshContextOnRedeployCancellation(t *testing.T) {
    // harness with cancellingProbe; ctx wired to cancel inside Run.
    h := newDeployerHarnessWithProbe(t, cancellingProbe{})
    prev := newPrev()

    ctx, cancel := context.WithCancel(context.Background())

    h.driver.EXPECT().NetworkEnsure(gomock.Any(), "decloud").Return(nil)
    h.capturer.EXPECT().Capture(gomock.Any(), gomock.Any()).Return(map[string]string{"X": "1"}, nil)
    h.store.EXPECT().Load(gomock.Any(), "foo").Return(prev, nil)
    h.driver.EXPECT().Build(gomock.Any(), gomock.Any()).Return("img", nil)
    h.driver.EXPECT().Stop(gomock.Any(), "decloud-foo", gomock.Any()).Return(nil)
    h.driver.EXPECT().Remove(gomock.Any(), "decloud-foo").Return(nil)
    h.driver.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(
        func(_ context.Context, _ dockerdrv.RunRequest) (string, error) {
            cancel()                                                      // ^C while probe will block
            return "new-cid", nil
        })
    // Cleanup expectations: Stop+Remove of new container, then Run for restore.
    // ALL three must receive a non-cancelled ctx.
    h.driver.EXPECT().Stop(notCancelledCtx(), "decloud-foo", gomock.Any()).Return(nil)
    h.driver.EXPECT().Remove(notCancelledCtx(), "decloud-foo").Return(nil)
    h.driver.EXPECT().Run(notCancelledCtx(), gomock.Any()).DoAndReturn(
        func(_ context.Context, req dockerdrv.RunRequest) (string, error) {
            assert.Equal(t, prev.Config.Build.ImageRef, req.Image,
                "rollback restores the previous image")
            return "rb-cid", nil
        })

    err := h.deployer.Deploy(ctx, newRequest())
    require.Error(t, err)
    assert.True(t, errors.Is(err, deploy.ErrInterrupted))
}
```

**Harness extension:** `newDeployerHarness` currently hardcodes
`passThroughProbe`. Add `newDeployerHarnessWithProbe(t, probe)` that takes
a probe override; have the original `newDeployerHarness` delegate to it
with `&passThroughProbe{driver: driver}`. This is a one-time helper change,
not a per-test refactor.

### 5.7 Updated: `TestReadiness_ContextCancellationStopsProbe`

**File:** `internal/deploy/readiness_test.go`, lines 144–166.

**Current assertion (lines 161–165):**
```go
assert.True(t,
    errors.Is(err, context.Canceled) ||
        strings.Contains(err.Error(), "context canceled") ||
        errors.Is(err, deploy.ErrReadiness),
    "expected context cancellation to terminate the probe; got %v", err)
```

**New assertion:**
```go
assert.True(t, errors.Is(err, context.Canceled),
    "context cancellation must surface as context.Canceled in the error chain; got %v", err)
assert.False(t, errors.Is(err, deploy.ErrReadiness),
    "context cancellation must NOT be wrapped as ErrReadiness; got %v", err)
```

**Why:** Don §1.3 Latent Bug A. After §3.3 the probe wraps with
`fmt.Errorf("readiness: %w", ctx.Err())`, so `errors.Is(err, context.Canceled)`
is true and `errors.Is(err, deploy.ErrReadiness)` is false. The
`strings.Contains` fallback in the old assertion accepted the wrong error
shape; tightening it locks in the contract.

The `strings` import in `readiness_test.go` becomes unused after this
change — Rob/Kent: remove the import or `goimports` will.

### 5.8 New: `TestExitCodeFor_InterruptedSentinels`

**File:** `internal/cli/exit_codes_test.go`, append after the existing
`TestExitCodeFor_AllSentinels` table cases.

**Add to the table (v2 revision):**
```go
{"interrupted", deploy.ErrInterrupted, ExitInterrupted},
{"interrupted-wrapped", fmt.Errorf("oops: %w", deploy.ErrInterrupted), ExitInterrupted},
```

**v1 also added:**
```go
{"context-canceled", context.Canceled, ExitInterrupted},
{"context-deadline", context.DeadlineExceeded, ExitInterrupted},
```

**These two cases are dropped in v2** because the corresponding `ExitCodeFor` cases were dropped per §3.6 (Linus Issue 2). Adding them back as test cases would assert behavior we explicitly don't want (mapping bare `context.Canceled` to `ExitInterrupted`).

**Optional v2 negative test** to lock in the new contract (recommended): assert that bare `context.Canceled` and `context.DeadlineExceeded` map to `ExitInternal`, NOT `ExitInterrupted`:
```go
{"context-canceled-bare", context.Canceled, ExitInternal},
{"context-deadline-bare", context.DeadlineExceeded, ExitInternal},
```

This requires the `context` import in `exit_codes_test.go`. Rob: add it. The negative tests guard against a future maintainer "helpfully" re-adding the v1 behavior.

### 5.9 Tests that must NOT change (v2 revision — single harness change)

**v1 said:** 11 existing first-deploy tests gain one `Driver.Inspect → absent` expectation each. Linus correctly flagged that as unnecessary — Issue 4 in `04-linus-review.md`.

**v2 fix:** add a single default expectation in `newDeployerHarness` and the 11 individual test edits collapse to zero.

**The harness change:**

```go
func newDeployerHarness(t *testing.T) *deployerHarness {
    // ... existing setup ...

    // Default expectation: any first-deploy test that exercises the §3.5
    // defensive orphan branch sees an "absent" container. Tests that
    // care about a non-absent inspect override this with explicit
    // InOrder(Inspect → ...) expectations; gomock matches the most
    // specific expectation first.
    driver.EXPECT().
        Inspect(gomock.Any(), gomock.Any()).
        Return(dockerdrv.InspectResult{State: "absent"}, nil).
        AnyTimes()

    // ... rest of existing setup ...
}
```

**gomock precedence guarantee:** gomock matches expectations in LIFO order — most recently added first. Tests that explicitly add `h.driver.EXPECT().Inspect(...).Return(...)` AFTER calling `newDeployerHarness` will match those before falling through to the AnyTimes default. The §5.3, §5.4, §5.5, and the new label-mismatch test (§5.10) all add explicit `Inspect` expectations and they take priority.

**One caveat for Kent:** the existing `TestDeploy_HappyPathRedeploy` and other `hasPrev=true` tests — the §3.5 branch is gated on `!hasPrev`, so it doesn't fire on those tests. The AnyTimes default is fine to leave in (it just won't match anything in those tests). No edits to redeploy tests.

**Tests confirmed unchanged after the harness fix:**

All of these tests pass without per-test Inspect expectation additions, because:
- First-deploy tests (`hasPrev=false`): §3.5 runs once, the harness AnyTimes default catches the call.
- Redeploy tests (`hasPrev=true`): §3.5 doesn't run, so no Inspect call to match.
- Tests that fail before `Build` (e.g. `NetworkEnsureFailureReturnsErrRun`): §3.5 unreached.

Specifically unchanged (sampling for sanity; Rob to verify the full file):

- `TestDeploy_HappyPathFirstDeploy`
- `TestDeploy_HappyPathRedeploy`
- `TestDeploy_LoadPreviousErrSecretsMissingTreatedAsFirstDeploy`
- `TestDeploy_SaveErrPartialWriteRollsBackAndDeletesOrphanConfig`
- `TestDeploy_SaveFailsBeforePartialWriteSkipsDeleteOrphanConfig`
- `TestDeploy_CaddyValidateFailureLeavesOldFileAndKeepsNewContainer`
- `TestDeploy_CaddyReloadFailureDoesNotRollBackContainer`
- `TestDeploy_CaddyValidateFailureMentionsCaddyUpRecovery`
- `TestDeploy_CaddyReloadFailureMentionsCaddyUpRecovery`
- `TestDeploy_DeployIDIsStableThroughoutOneDeploy`
- `TestDeploy_CapturedEnvNotLoggedAsKeysOrValues`
- `TestDeploy_RunRequestUsesCapturedEnvAndDecloudNetwork`
- `TestDeploy_NoEnvScript_SkipsCapturerEntirely`
- `TestDeploy_NetworkEnsureCalledFirst`
- `TestDeploy_NetworkEnsureFailureReturnsErrRun`
- `TestDeploy_BuildErrorPreservesInnerSentinel`
- `TestDeploy_StopOldFailureAbortsAndDoesNotStartNew`
- `TestDeploy_BuildFailureAbortsBeforeStoppingOld`
- `TestDeploy_RunNewFailureRollsBackToOld`
- `TestDeploy_ReadinessFailureRollsBackToOld`

**Summary of v2 test churn:** ONE harness edit, not 11 per-test edits. The orchestrator contract widening (§3.5) is now reflected in the harness's default behavior; tests that care about the contract specifics override with their own InOrder expectations.

### 5.10 New: `TestDeploy_DefensiveOrphanRefusesContainerWithoutDecloudLabel`

**Purpose:** §3.5 v2 label-gating verification (Linus Issue 1). When a non-decloud-created container squats on the name `decloud-<svc>`, the deploy must refuse to destroy it and surface a clear recovery hint.

```go
func TestDeploy_DefensiveOrphanRefusesContainerWithoutDecloudLabel(t *testing.T) {
    h := newDeployerHarness(t)

    h.driver.EXPECT().NetworkEnsure(gomock.Any(), "decloud").Return(nil)
    h.capturer.EXPECT().Capture(gomock.Any(), gomock.Any()).Return(map[string]string{"X": "1"}, nil)
    h.store.EXPECT().Load(gomock.Any(), "foo").Return(nil, registry.ErrNotFound)
    h.driver.EXPECT().Build(gomock.Any(), gomock.Any()).Return("img", nil)
    h.driver.EXPECT().Inspect(gomock.Any(), "decloud-foo").
        Return(dockerdrv.InspectResult{
            State:  "running",
            Labels: map[string]string{"some.other.label": "value"}, // no decloud.service
        }, nil)
    // Critical: NO Stop, NO Remove, NO Run.
    h.driver.EXPECT().Stop(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
    h.driver.EXPECT().Remove(gomock.Any(), gomock.Any()).Times(0)
    h.driver.EXPECT().Run(gomock.Any(), gomock.Any()).Times(0)

    err := h.deployer.Deploy(context.Background(), newRequest())
    require.Error(t, err)
    assert.True(t, errors.Is(err, deploy.ErrRun))
    assert.Contains(t, err.Error(), "was not created by decloud")
    assert.Contains(t, err.Error(), "docker rm -f decloud-foo")
}
```

**Companion test variant** — label present but mismatched value:

```go
func TestDeploy_DefensiveOrphanRefusesContainerWithMismatchedLabel(t *testing.T) {
    h := newDeployerHarness(t)

    h.driver.EXPECT().NetworkEnsure(gomock.Any(), "decloud").Return(nil)
    h.capturer.EXPECT().Capture(gomock.Any(), gomock.Any()).Return(map[string]string{"X": "1"}, nil)
    h.store.EXPECT().Load(gomock.Any(), "foo").Return(nil, registry.ErrNotFound)
    h.driver.EXPECT().Build(gomock.Any(), gomock.Any()).Return("img", nil)
    h.driver.EXPECT().Inspect(gomock.Any(), "decloud-foo").
        Return(dockerdrv.InspectResult{
            State:  "running",
            Labels: map[string]string{"decloud.service": "bar"}, // wrong service name
        }, nil)
    h.driver.EXPECT().Stop(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
    h.driver.EXPECT().Remove(gomock.Any(), gomock.Any()).Times(0)
    h.driver.EXPECT().Run(gomock.Any(), gomock.Any()).Times(0)

    err := h.deployer.Deploy(context.Background(), newRequest())
    require.Error(t, err)
    assert.True(t, errors.Is(err, deploy.ErrRun))
    assert.Contains(t, err.Error(), `decloud.service="bar"`)
    assert.Contains(t, err.Error(), "does not match")
}
```

Two tests, mechanically similar. Kent: keep them both — the first locks in "missing label refused," the second locks in "mismatched label refused with the offending value surfaced."

### 5.10.1 New: `TestDeploy_DefensiveOrphanInspectCancelledReturnsErrInterrupted`

**Purpose:** §3.5 v2.1 cancellation-discrimination verification (Linus 06-review follow-up). When the request `ctx` is cancelled mid-orphan-inspect, the deploy must surface `ErrInterrupted` (exit 130), not `ErrRun` (exit 40).

Shape: arrange `Inspect(gomock.Any(), "decloud-foo")` to return `(InspectResult{}, context.Canceled)`. Assert `errors.Is(err, deploy.ErrInterrupted)` and `!errors.Is(err, deploy.ErrRun)`. One test covers the inspect-error site; the stop-error and remove-error sites are mechanically identical and Kent may either fold them into a table-driven test or trust the inspect-site test as the contract anchor (Kent's call — both are defensible). Recommend table-driven with three rows for completeness; the harness scaffolding is already in place.

### 5.11 Driver-level test for `Inspect` label parsing

**File:** `internal/dockerdrv/cli_driver_test.go`

The argv-shape tests in `cli_driver_test.go` lock the docker argv byte-for-byte. The §3.5.1 change to `cliDriver.Inspect`'s `--format` string from the whitespace shape to the JSON shape will break those tests.

**Action for Rob:**
1. Update the argv expectation for `Inspect` from `["inspect", name, "--format", "{{.Id}} {{.State.Status}}"]` to the new JSON format string verbatim.
2. Update the stdout fixture from the whitespace `"abc123 running\n"` to JSON: `'{"id":"abc123","state":"running","labels":{"decloud.service":"foo"}}'`.
3. Add one new test that asserts `inspect.Labels["decloud.service"]` is parsed correctly from the JSON output.
4. Add one new test that asserts `inspect.Labels` is nil when the container has no labels (the JSON output is `"labels":null` in that case; `json.Unmarshal` correctly produces a nil map).

These are driver-level tests; they live in `internal/dockerdrv/cli_driver_test.go`, not in the deploy package.

---

## 6. Estimation reality check

Original mental estimate: 4 hours (3 sites change context + wrap an error).

Realistic estimate: 4 × π ≈ 12.5 hours. Spread:

- §3.1 sentinel + §3.2 helper: 15 min.
- §3.3 readiness wrap + test update §5.7: 30 min.
- §3.4 three cleanup blocks: 90 min including reading/writing the
  cleanup-context discipline carefully. The `restoreOldContainer` rename
  ripples to one call site only.
- §3.5 defensive orphan branch: 60 min including the 11 test updates listed
  in §5.9. The test churn is the bulk of the work.
- §3.6 exit code: 20 min including adding `context` import to `exit_codes.go`
  and updating `exit_codes_test.go`.
- New tests §5.1–§5.6, §5.8: 4 hours including the `notCancelledCtxMatcher`,
  `cancellingProbe`, and `newDeployerHarnessWithProbe` infrastructure. The
  matcher and probe are simple, but writing the cancellation-timing tests
  cleanly takes care.
- Doc updates (Raymond's task, Don §7): 60 min.
- Linus + Kevlin review iteration: 90 min.
- The "while we're at it" tax for inevitable scope creep that Linus or
  Kevlin will find: 90 min.

Total: ~12 hours, matching the πx multiplier.

---

## 7. Gotchas and landmines (for Rob, in priority order)

1. **`gomock` matcher evaluation timing.** The `notCancelledCtxMatcher`
   evaluates at the moment of the recorded call. If Rob accidentally reuses
   the request ctx and then explicitly cancels it AFTER the cleanup call
   completes, the test would still pass. This is fine in practice because
   the bug is "ctx is already cancelled when cleanup is reached" — by
   construction in test 5.1, the cancel happens *before* cleanup is
   reached, so a buggy implementation reads the cancelled state and the
   matcher rejects.

2. **Defer scope.** `defer cleanupCancel()` inside an `if err != nil {}`
   block fires at function return, not block return. That's what we want
   (the cleanup ctx must outlive `restoreOldContainer`). Do not move the
   defer outside; do not refactor to inline call.

3. **Don't accidentally derive the cleanup ctx from `ctx`.** Use
   `context.Background()` as the parent, period. Code-review for the
   pattern `context.WithTimeout(ctx, ...)` — that's the bug, not the fix.

4. **`exec.CommandContext` semantics.** Once the implementation is right,
   Rob may be tempted to "verify" the fix by running a `cli_driver_test`
   with a cancelled context. Don't bother — the existing
   `cli_driver_test.go` uses `context.Background()` for everything and
   doesn't need to test this. The fix is at the orchestrator layer; the
   driver layer is correct as-is.

5. **`Inspect` for orphan check vs. `Inspect` in `lifecycle.go:Start`.**
   The latter takes registered services. The former (§3.5) takes whatever
   container name the user is about to claim. Both call the same
   `Driver.Inspect` and both correctly handle `state == "absent"`. No
   conflict.

6. **`logger` vs `slog` in §3.5.** Use `logger.Warn(...)` not
   `slog.Warn(...)` — `logger` is already scoped with `deploy_id` and
   `service` fields (line 129) and is in scope. The existing code uses
   raw `slog.Error`/`slog.Info` only in `restoreOldContainer` (lines 295,
   299) where there's no logger in scope. Match the local convention.

7. **`context.DeadlineExceeded` is a real risk.** A future caller might
   pass `context.WithTimeout(parentCtx, 10*time.Minute)`. If the timeout
   expires during `probe.Wait`, the probe returns
   `fmt.Errorf("readiness: %w", context.DeadlineExceeded)`. Our
   `errors.Is(err, context.DeadlineExceeded)` check catches it and maps to
   `ErrInterrupted`. Tests 5.8 cover this.

8. **The `dockerdrv.ErrContainerNotFound` filter must not become
   "swallow all errors".** Be explicit:
   `err != nil && !errors.Is(err, dockerdrv.ErrContainerNotFound)` →
   `slog.Warn`. Don't write `_ = ` (current code) and don't write a bare
   `if err != nil { slog.Warn(...) }` either; the not-found case is
   expected silence.

9. **The `strings` import in `readiness_test.go` becomes unused** after
   the §5.7 update. `goimports` removes it; `go test ./...` will fail with
   "imported and not used" otherwise.

10. **Adding `Inspect` expectations to 11 happy-path tests is mechanical
    but tedious.** Rob: do them as one commit so the diff reviews cleanly.
    Each is the same insertion: `Inspect(gomock.Any(), "decloud-foo")
    .Return(dockerdrv.InspectResult{State: "absent"}, nil)` between Build
    and Run in the existing `gomock.InOrder` block (or as a stand-alone
    expectation if the test doesn't use InOrder).

11. **The `prev` from `Store.Load` may be `nil` when `loadErr` is
    `ErrSecretsMissing`** (Don §3.3 mentions this). Verify: `service.go:151`
    — `prev, loadErr := d.deps.Store.Load(...)`. If `loadErr` is
    `ErrSecretsMissing`, `hasPrev` becomes false (the check at :153
    excludes `ErrSecretsMissing` from the fatal branch but does not set
    hasPrev=true), so the `restoreOldContainer(prev)` calls never run for
    that case, so prev being nil is safe. Confirmed by reading the code.
    No change needed; just don't touch this.

12. **Don's §1.3 Defect A vs Joel's hybrid Shape.** Joel chose
    `ErrInterrupted` (Shape B) for the *outer* sentinel (so the exit code
    map has something to grep) AND kept `ctx.Err()` wrapped via `%w`
    (Shape A) at the probe layer (so `errors.Is(err, context.Canceled)`
    traverses for callers that want it). This is deliberate. Don't
    "simplify" by picking one. Linus may push back; if so, defer to
    Linus.

---

## 8. Simplification opportunities (the best code is no code)

1. **Could we just use `context.WithoutCancel(ctx)` (Go 1.21+)?** Yes, and
   it's tempting. But it inherits the deadline from `ctx`, and we
   explicitly want a *new* 30s budget unrelated to the request deadline
   (which doesn't exist today but might in M4). `context.Background()` +
   `WithTimeout` is the right shape. **Reject simplification.**

2. **Could the cleanup ctx be a single function-scoped variable?** Tempting,
   but the budget timing matters (§2.3). **Reject.**

3. **Could we skip the §3.5 defensive branch and just trust the cleanup
   discipline?** No — Don §3.3 covers the kill -9 / power-loss case which
   no in-process cleanup can handle. **Reject.**

4. **Could the slog cleanup-failure warning be a single
   `cleanupWithWarn(cleanupCtx, name)` helper?** Yes, modest win.
   Joel's preference: don't bother. The cleanup blocks at §3.4.1 and
   §3.4.2 are the only two callers; a helper saves 6 lines and adds an
   indirection. If Linus wants it, fine, but it's not in the spec.

---

## 9. Acceptance criteria (mirror of Don's §8 with concrete commands)

The plan is done when, on the maintainer's host, all of these hold:

1. **Manual: SIGINT during readiness wait, first deploy.**
   ```
   $ rm -rf /tmp/decloud-test && mkdir /tmp/decloud-test
   $ DECLOUD_ROOT=/tmp/decloud-test /tmp/decloud deploy service --name foo --port 8080 --readiness-path /healthz /path/to/never-ready-app &
   $ sleep 8 && kill -INT %1
   $ wait %1; echo "exit=$?"
   exit=130
   $ docker ps -a --filter name=decloud-foo
   CONTAINER ID  IMAGE  ...    (no rows)
   ```

2. **Manual: re-deploy after step 1 succeeds end-to-end against a healthy
   app.**
   ```
   $ DECLOUD_ROOT=/tmp/decloud-test /tmp/decloud deploy service --name foo --port 8080 --readiness-path /healthz /path/to/healthy-app
   $ echo $?
   0
   ```

3. **Manual: SIGKILL simulation + retry detects orphan.**
   ```
   $ DECLOUD_ROOT=/tmp/decloud-test /tmp/decloud deploy service ... &
   $ sleep 5 && kill -9 %1     # decloud dies between Run and Save
   $ docker ps --filter name=decloud-foo  # confirm container exists
   $ rm -rf /tmp/decloud-test/registry/foo  # simulate "no registry entry"
   $ DECLOUD_ROOT=/tmp/decloud-test /tmp/decloud deploy service --name foo ...
   # logs include: "removed orphan container from prior interrupted deploy"
   ```

4. **Automated: `go test ./...` passes.** Specifically:
   - `internal/deploy/...` — including new tests 5.1, 5.3, 5.4, 5.5, 5.6
     and updated 5.7.
   - `internal/cli/...` — including updated `exit_codes_test.go` with new
     table cases (5.8).
   - `internal/dockerdrv/...` — must pass unchanged. If any
     `cli_driver_test.go` test fails, Rob has accidentally changed argv
     or driver behavior, which is out of scope.

5. **Automated: `gofmt -l internal/ cmd/` returns no files.**

6. **Automated: `grep -rn '%w: %v' internal/ cmd/` returns zero rows**
   (the existing discipline gate from `_ai/error-wrap-discipline.md`).

---

## 10. Out of scope (for clarity)

- Changes to `dockerdrv.Driver` interface — none.
- Changes to `cli_driver.go` argv — none.
- Changes to `lifecycle.go` — none. Don §3.4 confirmed.
- New mocks via `go generate` — none.
- Changes to caddy `manager.go` cleanup — that's a different code path and
  was not reported. If Linus wants symmetry there, it's a follow-up task.
- A `--no-cleanup` debugging flag — not needed.
- Configurable cleanup timeout — 30s hard-coded. If a user reports that's
  too short, a future task adds a flag.

---

## 11. Linus's anticipated objections (and Joel's pre-rebuttals)

1. **"Why a hybrid Shape A+B for the cancellation sentinel?"** Because the
   exit code mapping needs a *named* sentinel to switch on, and the
   error-chain traversal needs `context.Canceled` to be `errors.Is`-able.
   Pure Shape A means the user-visible error message is "deploy: readiness
   probe failed: readiness: context canceled" which is wrong on two counts.
   Pure Shape B means writing `errors.Is(err, deploy.ErrInterrupted)`
   instead of `errors.Is(err, context.Canceled)` for callers who want to
   know "did this fail because of cancellation?" without knowing about our
   sentinel taxonomy. Both are useful.

2. **"30s timeout is too long / too short."** It's a balance. 10s wouldn't
   cover stop-grace + rm + restore-run, and 60s starts to feel like
   "decloud is hung." Adjustable in a follow-up if real ops feedback
   demands it.

3. **"Defensive orphan check will hit `docker inspect` on every deploy."**
   Yes. It's a single subprocess call, ~50ms typical. Worth it for the
   power-loss recovery case.

4. **"You're adding 11 `Inspect` expectations to existing tests; that's
   fragile."** It IS fragile, but the alternative is making §3.5
   conditional on something the tests can opt into, which adds more
   surface than it removes. The §5.9 list is mechanical enough that Rob
   gets it right in one pass.

5. **"What about a panic between Run and Save?"** Don §5 raised this.
   Joel: defer-based recovery is a separate (larger) shape — the existing
   code has no `defer` in `Deploy`, and adding one to recover panics is a
   new pattern. **Punt to follow-up.** The defensive orphan cleanup at
   §3.5 covers the *next-deploy* path for any panic-leaked container, so
   the user is never stuck. Document in `_ai/m1x-backlog.md` as
   "panic-aware cleanup defer in deploy" — Andy: small enough to wait.

---

— Joel

P.S. for Rob (v1): I know the test churn in §5.9 is the boring part. Do it
first — the `Inspect → absent` insert is mechanical, and if you do it
before any of the "interesting" changes, you'll be running green tests
the whole way and the headline fix lands as a single coherent diff
rather than a thrash. Tests, then `ErrInterrupted` + helper, then the
three cleanup blocks, then the §3.5 branch, then exit code, then new
tests. Mechanical. Boring. Correct.

P.P.S. for Rob (v2 update): the §5.9 mechanical paste is no longer needed — the harness `AnyTimes()` default at §5.9 / §4.4 absorbs it. Updated implementation order:

1. **Driver-level changes first** — `InspectResult.Labels` field (§3.5.1), `cliDriver.Inspect` JSON format (§3.5.1), `cli_driver_test.go` argv + parsing tests (§5.11). Run `go generate ./...` to refresh mocks.
2. **Sentinel + helper** — `ErrInterrupted` (§3.1), `newCleanupContext` + `cleanupTimeout` (§3.2).
3. **Harness AnyTimes default** — single edit to `newDeployerHarness` per §5.9 / §4.4. All existing tests must still pass at this point.
4. **Three cleanup blocks** — §3.4.1 (probe-failure with audit-log fork), §3.4.2 (save-failure), §3.4.3 (run-failure).
5. **Defensive orphan branch** — §3.5 with label gating.
6. **Exit code mapping** — §3.6, only `ErrInterrupted`.
7. **New tests** — §5.1, §5.3, §5.4, §5.5, §5.6, §5.7 (update), §5.8 (table additions and the negative-cases), §5.10 (label refusal, two variants).

Run `go test ./...` after each step. If any pre-existing test breaks at steps 1-3, stop — something is structurally wrong. The headline behavior change starts at step 4.

---

## 12. v2 revision summary (Linus REVISE → revised plan)

This v2 of the tech plan addresses six items from `04-linus-review.md`. Cross-referenced with Don's resolutions in `02-plan.md` §12.

| Linus issue | v1 plan | v2 plan |
|---|---|---|
| 1: orphan label gating | "no registry entry → destroy any container with the right name" | Label-gated: only destroy containers whose `decloud.service` label matches `req.Name`. Refuse otherwise with manual-`docker rm -f` hint. |
| 2: ExitCodeFor over-broad | Match `ErrInterrupted`, `context.Canceled`, `context.DeadlineExceeded` | Match only `ErrInterrupted`. Negative test cases lock the contract. |
| 3: probe wrap shape | `fmt.Errorf("readiness: %w", ctx.Err())` | Raw `ctx.Err()`. Plus orchestrator audit-log forks on cancellation vs failure. |
| 4: 11 test edits | 11 mechanical Inspect expectation paste-ins | Single `AnyTimes()` default in `newDeployerHarness`. |
| 5: caddy backlog | Not addressed | Backlog entry to `_ai/m1x-backlog.md` (Don's plan §12.7 item 7). |
| 6: restoreOldContainer surfacing | Acknowledged out-of-scope, no entry | Backlog entry to `_ai/m1x-backlog.md` (Don's plan §12.7 item 8). |

**One driver interface change in v2 (was zero in v1):** `InspectResult` gains a `Labels map[string]string` field. NOT a new method. Mocks regenerate transparently. The `cliDriver.Inspect` argv changes from whitespace format to JSON format.

**One test file change in v2 driver layer (was zero in v1):** `internal/dockerdrv/cli_driver_test.go` updates to the new `--format` string and adds two label-parsing tests (§5.11).

Linus's anticipated objections from v1 §11 still apply, except:
- §11.1 (Shape A vs B sentinel) — still defended; the hybrid is the right call.
- §11.2 (30s timeout) — unchanged.
- §11.3 (defensive orphan check on every deploy) — still a single subprocess per deploy, but now the format string parses JSON instead of whitespace; same rough cost.
- §11.4 (test churn) — moot in v2; collapsed to the harness default.
- §11.5 (panic recovery) — still punted to backlog.

---

## 13. v2.2 delta — six fix-in-scope items (Kevlin + Linus impl-review follow-ups)

This section is a DELTA against v2.1. Earlier sections (§1–§12) are authoritative for everything not explicitly overridden here. The implementation already shipped against v2.1; v2.2 narrows in on six items raised in `011-kevlin-review.md` (two doc/log nits + two style follow-ups) and `12-linus-impl-review.md` (one strategic Issue 1 + one doc Issue 2). Don's lockdown in `013-don-plan-iteration2.md` elected fix-in-scope on all six. The verbatim code/text replacements below come from Don's §"Reasoning, item by item".

**Source-of-truth pre-state**: line citations below were re-grepped against the current `internal/deploy/service.go` (399 lines, post-v2.1) and `_docs/usage.md` (254 lines) on the iteration-2 branch — they reflect the file as it stands AFTER v2.1 shipped, which is what Rob will be editing. Joel: do not trust the v2.1-era line numbers in §3.4 / §3.5 above; they predate the v2.1 implementation. The numbers in §13 are the live ones.

### 13.1 §3.4.5 cancellation symmetry — Linus Issue 1 (strategic, fix-in-scope)

**Problem**: the v2.1 lockdown fixed cancellation discrimination at the §3.5 (`!hasPrev`) orphan-check branch (Inspect, Stop, Remove → check `isCancellation`, return `ErrInterrupted` not `ErrRun`). The sibling §3.4.5 (`hasPrev`) redeploy stop+remove branch at `service.go:185–197` did NOT get the same fix. Result: the two adjacent forward-progress branches now have ASYMMETRIC cancellation contracts — ctrl+c during a fresh-deploy orphan check returns exit 130; ctrl+c during a redeploy old-container stop returns exit 40. Same key combo, same intent, two exit codes.

This is the identical bug shape v2.1 fixed at §3.5. Linus missed it in v2-review and called it out in `12-linus-impl-review.md` Issue 1 with a mea culpa. Don's lockdown precedent ("the whole point of this task is getting cancellation semantics right; shipping with a known cancellation-mis-wrap inside the same task that's specifically about cancellation discipline would be cowardice") binds him to fix-in-scope.

**Pre-state (verified by grep against `internal/deploy/service.go:185-197`)**:

```go
if hasPrev {
    if err := d.deps.Driver.Stop(ctx, containerName, 10*time.Second); err != nil {
        if !errors.Is(err, dockerdrv.ErrContainerNotFound) {
            inspect, ierr := d.deps.Driver.Inspect(ctx, containerName)
            if ierr == nil && inspect.State == "running" {
                logger.Error("stop old container failed and still running", "step", "stop_old", "error", err)
                return fmt.Errorf("%w: stop previous container: %w", ErrRun, err)
            }
        }
    }
    if err := d.deps.Driver.Remove(ctx, containerName); err != nil && !errors.Is(err, dockerdrv.ErrContainerNotFound) {
        return fmt.Errorf("%w: remove previous container: %w", ErrRun, err)
    }
}
```

Two return-error sites (line 191 stop, line 196 remove) wrap as `ErrRun` without checking for cancellation first.

**Post-state (after Item 5's `isCancellation` helper lands — see §13.5; if implemented in a different order, substitute the inline `errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)` idiom)**:

```go
if hasPrev {
    if err := d.deps.Driver.Stop(ctx, containerName, 10*time.Second); err != nil {
        if !errors.Is(err, dockerdrv.ErrContainerNotFound) {
            inspect, ierr := d.deps.Driver.Inspect(ctx, containerName)
            if ierr == nil && inspect.State == "running" {
                logger.Error("stop old container failed and still running", "step", "stop_old", "error", err)
                if isCancellation(err) {
                    return fmt.Errorf("%w: %w", ErrInterrupted, err)
                }
                return fmt.Errorf("%w: stop previous container: %w", ErrRun, err)
            }
        }
    }
    if err := d.deps.Driver.Remove(ctx, containerName); err != nil && !errors.Is(err, dockerdrv.ErrContainerNotFound) {
        if isCancellation(err) {
            return fmt.Errorf("%w: %w", ErrInterrupted, err)
        }
        return fmt.Errorf("%w: remove previous container: %w", ErrRun, err)
    }
}
```

Two new `isCancellation(err) → ErrInterrupted` pre-checks. Six lines. Mirrors §3.5 verbatim.

**Why the cancellation pre-check goes inside the existing `if !errors.Is(err, dockerdrv.ErrContainerNotFound)` block at line 187 (and not earlier)**: an `ErrContainerNotFound` from `Stop` is the desired post-state (the container was already gone), and the existing fall-through to `Remove` handles it correctly. Cancellation is a different shape and only matters on the path that would otherwise wrap as `ErrRun`. Keep the existing not-found filter; layer cancellation on top.

**Why Stop's "still-running" inner branch is the cancellation-relevant site, not the outer `if err != nil`**: when `ctx` is cancelled, `Driver.Stop` returns `context.Canceled` immediately (per `exec.CommandContext` semantics from §1.1 of Don's plan). The defensive Inspect at line 188 then ALSO returns `context.Canceled`, so `ierr != nil`, so the inner `if ierr == nil && inspect.State == "running"` block is SKIPPED, control falls through to the Remove at line 195 which ALSO returns `context.Canceled`, which fails the `errors.Is(..., ErrContainerNotFound)` check, and we return `ErrRun: remove previous container: context canceled`. The fix at the Remove site at line 196 is therefore the load-bearing one for the cancellation contract; the Stop site at line 191 is defensive (only fires if the cancellation race lets `Inspect` succeed, which is unlikely but not impossible). Both sites get the fix anyway because the symmetry argument applies to both.

**New test — Kent**: extend `TestDeploy_DefensiveOrphanInspectCancelledReturnsErrInterrupted` (currently at `service_test.go:550-606`, three subtests: inspect-cancelled, stop-cancelled, remove-cancelled) with a fourth subtest, OR write a sibling table-driven test for the §3.4.5 sites. Don's lockdown §"Item 1" left this as Kent's call ("table-driven test row OR two subtests if Kent prefers parity with §3.5's three-subtest shape"). Joel's preference: write a sibling test, **not** an extension of the §3.5 test, because the harness setup is materially different — §3.5 needs `withoutInspectAbsentDefault()` and `Store.Load → ErrNotFound`; §3.4.5 needs a `hasPrev=true` setup with `Store.Load → prev` and the §3.5 AnyTimes default Inspect SHOULD match (the §3.4.5 branch doesn't hit the `!hasPrev` orphan-inspect site). Mixing them in one table forces conditional setup logic that the existing test cleanly avoids.

**Test name**: `TestDeploy_RedeployStopRemovePreviousContainerCancelledReturnsErrInterrupted`.

**Test shape (mirroring `TestDeploy_DefensiveOrphanInspectCancelledReturnsErrInterrupted`'s three-subtest table)**: two subtests are sufficient given the §3.4.5 control flow:

- `stop-cancelled`: `Driver.Stop(ctx, "decloud-foo", any) → context.Canceled`; defensive `Driver.Inspect` returns `InspectResult{State: "running"}` (so the inner "still-running" branch is entered and the cancellation pre-check fires). Assert `errors.Is(err, deploy.ErrInterrupted)` and `!errors.Is(err, deploy.ErrRun)`.
- `remove-cancelled`: `Driver.Stop(ctx, "decloud-foo", any) → nil`; `Driver.Remove(ctx, "decloud-foo") → context.Canceled`. Assert `errors.Is(err, deploy.ErrInterrupted)` and `!errors.Is(err, deploy.ErrRun)`.

Harness: `newDeployerHarness(t, withoutInspectAbsentDefault())` (Stop's defensive Inspect on the `stop-cancelled` subtest needs the explicit InspectResult expectation; `withoutInspectAbsentDefault()` disables the AnyTimes default that would otherwise interfere). `Store.Load → newPrev(), nil`. `prev := newPrev()` per the existing harness conventions.

**Test churn (existing tests)**: `TestDeploy_StopOldFailureAbortsAndDoesNotStartNew` at `service_test.go:297-312` exercises this exact code path with a non-cancellation error (`errors.New("stop timed out")`) and asserts `errors.Is(err, deploy.ErrRun)`. After the fix, `errors.New("stop timed out")` does NOT satisfy `isCancellation`, so the existing `ErrRun` assertion still holds. **No edits needed**. Verified by reading the test body.

**Cost**: 6 lines of production code + 1 new test (~40 lines mirroring §550-606). Don's estimate: 30 minutes.

### 13.2 `_docs/usage.md:240` second-ctrl+c falsehood — Linus Issue 2

**Problem**: line 240 currently reads:

> "A second ctrl+c (impatient double-tap) bypasses graceful cleanup and may leave the container behind. Path (1) above still recovers on the next deploy."

This is technically false. Per `signal.NotifyContext` semantics (verified at `cmd/decloud/main.go:14-15`), the package's signal handler stays registered until `stop()` is called in main's defer, and `stop()` only fires after `cmd.ExecuteContext(ctx)` returns. During the cleanup window the handler is still active and absorbs the second SIGINT; it does NOT propagate to the OS default handler. The user CANNOT bypass cleanup with a second ctrl+c — they must wait the 30s `cleanupTimeout` or send SIGKILL.

**Pre-state (verified at `_docs/usage.md:240`)**: see quote above.

**Post-state — replacement text per Don's lockdown §"Item 2"**:

> "A second ctrl+c during cleanup does not interrupt cleanup; the Go signal handler absorbs it. To force exit before the 30-second cleanup window completes, send SIGKILL (`kill -9 <pid>`); the orphan recovery in path (1) above still applies on the next deploy."

**Whose task**: Raymond (doc edit only).

**Why this isn't a code fix in `cmd/decloud/main.go`**: the "real" fix (Linus's Option B — wire actual second-signal exit-fast behavior) is captured as backlog Item 9 per Don's lockdown; v2.2 is the doc-truthfulness fix only. Don explicitly took Linus's Option A.

**Cost**: one sentence. 30 seconds.

### 13.3 `_ai/exit-code-sentinel-not-context-err.md:69` line-range typo — Kevlin nit 1

**Problem**: the doc cites "All four in `internal/cli/exit_codes_test.go:38-41`". The four rows (`interrupted`, `interrupted-wrapped`, `context-canceled-bare`, `context-deadline-bare`) actually live at `exit_codes_test.go:40-43`. Lines 38-39 are the `caddy-down` rows. Verified by grep.

**Pre-state (verified at `_ai/exit-code-sentinel-not-context-err.md:69`)**: `All four in internal/cli/exit_codes_test.go:38-41.`

**Post-state**: `All four in internal/cli/exit_codes_test.go:40-43.`

**Whose task**: Raymond. Two-character edit (`38-41` → `40-43`).

**Cost**: 60 seconds.

### 13.4 `service.go:212` audit-log tense — Kevlin nit 2

**Problem**: the `logger.Warn("removed orphan container from prior interrupted deploy", ...)` line at `service.go:212-213` fires BEFORE `Stop` (line 214) and `Remove` (line 220) execute. If either fails or is cancelled, the operator's log shows "removed orphan container ..." followed by a failure — misleading during incident review. Past-tense before the work is done.

**Pre-state (verified at `service.go:212`)**:

```go
logger.Warn("removed orphan container from prior interrupted deploy",
    "container", containerName, "state", inspect.State)
```

**Post-state — per Don's lockdown §"Item 4" (Kevlin's Option B, present-tense rewrite)**:

```go
logger.Warn("removing orphan container from prior interrupted deploy",
    "container", containerName, "state", inspect.State)
```

One word: `removed` → `removing`.

**Why not Kevlin's Option A (move the log line after Remove succeeds)**: Don picked Option B explicitly. The log at the head of the orphan branch tells the operator "this is where decloud is starting to act on the orphan" — a useful flow signal even if the action subsequently fails. Moving the log down separates the announcement from the action it announces by error-handling code. Smallest possible diff. No tests reference the `"removed orphan container"` substring (verified — `_docs/usage.md:237` references `"removed orphan container from prior interrupted deploy"` as user-facing prose, but that doc string SHOULD also update; see §13.7).

**Whose task**: Rob.

**Cost**: 60 seconds.

### 13.5 `isCancellation(err) bool` helper — Kevlin style 3

**Problem**: 6 occurrences of `errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)` in `service.go`. Verified by grep at lines 201, 215, 221, 244, 261, 341. The Kevlin Henney threshold ("six invocations of an idiom is the threshold where the idiom deserves a name") is met. Joel pre-approved the hoist in §3.4.1 of v2.1; Rob took the local hoist at line 261 (`cancelled := ...`) but not the package-level helper.

**Pre-state (verified by grep)**:

```
internal/deploy/service.go:201:                if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
internal/deploy/service.go:215:                    if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
internal/deploy/service.go:221:                    if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
internal/deploy/service.go:244:        if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
internal/deploy/service.go:261:        cancelled := errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
internal/deploy/service.go:341:        if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
```

**Post-state — new package-private helper**:

**Placement**: immediately after the existing `newCleanupContext` helper (currently at the top of `service.go`, near the sentinels and `cleanupTimeout` const at lines 23–42 region — see v2.1 §2.3 / §3.2). Joel's preference: place it right below `newCleanupContext` so all "cancellation-discipline" helpers cluster together. Rob's call if he wants a different placement, but keep it package-private and adjacent to the cleanup-context helper.

**Exact signature**:

```go
// isCancellation reports whether err is a context cancellation or deadline.
// Used by Deploy and its helpers to discriminate user-cancelled paths from
// genuine failures so the orchestrator can wrap as ErrInterrupted (exit 130)
// rather than the step-specific ErrRun/ErrReadiness sentinels.
func isCancellation(err error) bool {
    return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}
```

**Six call-site swaps**:

- Lines 201, 215, 221, 244, 341: `if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {` → `if isCancellation(err) {`.
- Line 261: `cancelled := errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)` → `cancelled := isCancellation(err)`.

After the swap, Item 1's two new sites (§13.1) read as `if isCancellation(err) {` — the helper is in place when those edits land, so the Item 1 diff naturally uses the helper rather than the inline idiom. Implementation order: apply Item 5 first, then Item 1.

**Whose task**: Rob.

**Test churn (existing tests)**: NONE. The helper is observably equivalent to the idiom it replaces. All existing tests assert on `errors.Is(err, deploy.ErrInterrupted)` and `errors.Is(err, context.Canceled)` against the *returned* error chain, which is unchanged by the refactor. Verified by reviewing the test names in §5 of v2.1. No mock regeneration needed — the helper is package-private and not exposed through any interface.

**Cost**: 5 minutes. One file, one new helper, six mechanical swaps.

### 13.6 slog message-vs-field convention — Kevlin style 4

**Problem**: 4 sites in `service.go` (lines 270, 274, 331, 335) currently concatenate `containerName` into the slog message string AND pass it as a structured `container` field. The container name appears twice — once in the message text, once in the kv pairs. This breaks `slog`'s grep-stability promise (the message string should be a fixed identifier; variable bits live in structured fields). Inconsistent with the rest of `service.go` (e.g. `"network ensure failed", "step", "network", "error", err` at line 145) and with `lifecycle.go:25-30` (the exemplar Joel cited in §4.1 — `"stop failed during unregister", "error", err` with no name interpolation).

**Pre-state (verified at lines 270-271, 273-275, 330-332, 334-336)**:

```go
// service.go:270-271 (probe-failure cleanup, Stop branch)
logger.Warn("cleanup failed; please remove "+containerName+" manually",
    "container", containerName, "error", stopErr)

// service.go:273-275 (probe-failure cleanup, Remove branch)
logger.Warn("cleanup failed; please remove "+containerName+" manually",
    "container", containerName, "error", rmErr)

// service.go:330-332 (save-failure cleanup, Stop branch)
logger.Warn("cleanup failed; please remove "+containerName+" manually",
    "container", containerName, "error", stopErr)

// service.go:334-336 (save-failure cleanup, Remove branch)
logger.Warn("cleanup failed; please remove "+containerName+" manually",
    "container", containerName, "error", rmErr)
```

**Post-state — per Don's lockdown §"Item 6"**:

```go
// All four sites:
logger.Warn("cleanup failed; manual removal may be required",
    "container", containerName, "error", <stopErr|rmErr>)
```

The new fixed message string `"cleanup failed; manual removal may be required"` is grep-stable; the container name lives only in the structured field. Operators querying the log for `container=decloud-foo` find the row regardless. The recovery action (`docker rm -f decloud-<name>`) is documented in `_docs/usage.md` §8 (the user-facing recovery doc), so it does NOT need to live in every per-event log message.

**Whose task**: Rob.

**Test churn (existing tests)**: NONE. Verified by Kevlin in `011-kevlin-review.md` finding #4: `TestDeploy_DefensiveOrphanCleanupFailureWrapsErrRun` asserts on the *returned* error (`err.Error()` contains `docker rm -f decloud-foo`), not on the slog message. The slog Warn fires in different paths than that test exercises — the test path returns `ErrRun` from §3.5, not from the §3.4.1/§3.4.2 cleanup blocks. No tests assert on the `"cleanup failed; please remove"` substring. Confirmed by greppable absence.

**Cost**: 5 minutes. Four mechanical edits in one file.

### 13.7 Implementation order

Order matters because Item 5 (`isCancellation` helper) is a prerequisite for Item 1's mechanical form. Recommended order for Rob:

1. **Item 5** — Add `isCancellation` helper. Swap the six existing call sites (lines 201, 215, 221, 244, 261, 341). `go test ./...` MUST still be green after this step; this is a pure refactor with no behavior change.
2. **Item 1** — Add the two new cancellation pre-checks at `service.go:185-197` using the new `isCancellation` helper. Kent in parallel writes `TestDeploy_RedeployStopRemovePreviousContainerCancelledReturnsErrInterrupted` against the two sites.
3. **Item 4** — One-word change to `service.go:212` audit log (`removed` → `removing`).
4. **Item 6** — Four slog message rewrites at `service.go:270, 274, 331, 335`.
5. **Item 2** — Raymond rewrites `_docs/usage.md:240`.
6. **Item 3** — Raymond corrects `_ai/exit-code-sentinel-not-context-err.md:69` line range.

After step 4, run `go test ./...`, `go vet ./...`, `gofmt -l internal/ cmd/`, and `grep -rn '%w: %v' internal/ cmd/`. All must be clean per Don's iteration-2 acceptance criteria.

### 13.8 Documentation churn worth a glance

**Note for Raymond (out-of-scope-for-this-tech-plan but worth flagging)**: `_docs/usage.md:237` says

> "The audit log records `removed orphan container from prior interrupted deploy` at warn level."

After Item 4 the actual log message changes to `"removing orphan container ..."` (present tense). The doc should mirror the production string verbatim or operators grepping the log will miss the row. This is a Raymond-task addition for iteration 2 — flag it in `010-raymond-docs.md` v2 as "synchronize the doc's quoted log string with §13.4's tense fix." NOT part of the six items above; it's a downstream consistency check that falls out of Item 4.

### 13.9 Out-of-scope for v2.2 (recorded for posterity)

- Linus's Issue 2 Option B (real second-signal exit-fast behavior in `cmd/decloud/main.go`) — Don's lockdown captures this as backlog item 9. Raymond appends to `_ai/m1x-backlog.md`. NOT a code fix in v2.2.
- Kevlin's nested-indent observation on `service.go:198-227` (the §3.5 defensive orphan branch is four levels deep). Kevlin explicitly DECIDED AGAINST demanding a refactor. Don did not list it. Out of scope.
- Re-running the full §3.4 cleanup blocks for any other consistency pass. The cleanup-context discipline shipped clean in v2.1; Items 1, 4, 5, 6 are surface-polish on top of it, not a structural revisit.

### 13.10 v2.2 acceptance criteria delta

In addition to v2.1's acceptance criteria (§9 of this tech plan), iteration-2 EXECUTION is done when:

1. `service.go` defines `func isCancellation(err error) bool` adjacent to `newCleanupContext`. The six pre-existing inline idioms (lines 201, 215, 221, 244, 261, 341) are replaced. Line 261 reads `cancelled := isCancellation(err)`.
2. `service.go:185-197` (the `hasPrev` redeploy stop+remove branch) checks `isCancellation(err)` before each `ErrRun` wrap; cancellation re-wraps as `ErrInterrupted`. Two new sites.
3. New test `TestDeploy_RedeployStopRemovePreviousContainerCancelledReturnsErrInterrupted` exists in `service_test.go` with two subtests (`stop-cancelled`, `remove-cancelled`), mirroring the shape of `TestDeploy_DefensiveOrphanInspectCancelledReturnsErrInterrupted` at lines 550-606.
4. `service.go:212` audit-log message reads `"removing orphan container from prior interrupted deploy"`.
5. Four slog.Warn sites (`service.go:270, 274, 331, 335`) use the fixed message `"cleanup failed; manual removal may be required"` with `containerName` as a structured field only.
6. `_docs/usage.md:240` second-ctrl+c sentence rewritten per §13.2's replacement text.
7. `_ai/exit-code-sentinel-not-context-err.md:69` line range corrected from `:38-41` to `:40-43`.
8. `go test ./...` passes; `go vet ./...` clean; `gofmt -l internal/ cmd/` empty; `grep -rn '%w: %v' internal/ cmd/` empty.
9. Kevlin re-review: APPROVE with no new findings on the six items.
10. Linus re-review: APPROVE with no new findings.

### 13.11 v2.2 summary table

| Item | Source | File:Line (pre-state) | Edit shape | Whose task | Test churn |
|---|---|---|---|---|---|
| 1 | Linus Issue 1 | `service.go:185-197` (two sites: 191, 196) | Two `isCancellation` pre-checks → `ErrInterrupted` | Rob impl + Kent test | New 2-subtest table-driven test; existing `TestDeploy_StopOldFailureAbortsAndDoesNotStartNew` unchanged |
| 2 | Linus Issue 2 | `_docs/usage.md:240` | One-sentence rewrite per §13.2 | Raymond | None |
| 3 | Kevlin nit 1 | `_ai/exit-code-sentinel-not-context-err.md:69` | `:38-41` → `:40-43` | Raymond | None |
| 4 | Kevlin nit 2 | `service.go:212` | `"removed"` → `"removing"` (one word) | Rob | None |
| 5 | Kevlin style 3 | `service.go:201, 215, 221, 244, 261, 341` | Hoist to `isCancellation` helper; six call-site swaps | Rob | None (pure refactor) |
| 6 | Kevlin style 4 | `service.go:270, 274, 331, 335` | Fixed message + structured field; four mechanical edits | Rob | None |

Total surface: 1 file (`service.go`) for 4 of 6 items, 1 doc file each for items 2 and 3. Test additions: 1 new test (~40 lines). All other tests unchanged.

— Joel
