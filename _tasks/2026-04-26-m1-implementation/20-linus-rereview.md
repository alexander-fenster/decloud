# Linus — iter2 architectural re-review

## VERDICT: **APPROVED.** M1 ships.

All three architectural blockers from `13-linus-execution-review.md` are
fixed with the right fix shape, the missing artifact exists and is
real, and the iter2 deltas introduced no new drift. 172 tests, 9
packages, all green. `go vet` clean. `gofmt` clean. `go generate`
idempotent.

---

## Verification of prior findings

### L1 — `NetworkEnsure` wired as Deploy step 0 → **FIXED**

`internal/deploy/service.go:131-135`. The call happens **before**
envcap (line 137), build (line 160), and every container action.
`gomock.InOrder` in `TestDeploy_NetworkEnsureCalledFirst` pins the
ordering as a contract test, not an implementation test —
`NetworkEnsure` precedes `Capture`, `Load`, `Build`, etc. The failure
maps to `ErrRun` (exit 40) per Joel's architectural decision #2,
which I approved in `15-linus-fixup-plan-review.md`. No new exit
sentinel, no `usage.md` surface expansion beyond Raymond's already-
spec'd one-line edit. Wrap is `%w: ensuring decloud network: %w` —
both `ErrRun` and the inner driver error traverse `errors.Is`. The
opaque "exit 40 when someone removed the network out of band" failure
mode is gone. `TestDeploy_NetworkEnsureFailureReturnsErrRun` proves
the failure path; `errors.Is(err, deploy.ErrRun)` returns true.

### L2 — `Capture("")` returns `(nil, nil)`; orchestrator skips → **FIXED**

`internal/envcap/capture.go:46-49`: empty path returns `(nil, nil)`
defensively. `internal/deploy/service.go:137-149`: orchestrator wraps
`Capture` in `if envFile != ""` and logs `env capture skipped: no
env script` on the empty branch. Two-layer defense without coupling:
the Capturer's contract stays simple (real path = real work, empty =
no work), and the orchestrator owns the decision. CLI side is also
correct: `internal/cli/deploy_service.go` `resolveEnvFile()`
implements the auto-discovery precedence I asked for — explicit flag
must succeed (exit 10 on missing/unreadable), empty flag stats
`<source-dir>/env.sh` and falls through to `""` if absent. Three new
sentinels (`ErrEnvScriptMissing`, `ErrEnvScriptUnreadable`) map
through `internal/cli/exit_codes.go` to `ExitConfigError` (10).
End-to-end flow tested by `TestDeployService_AutoDiscoversEnvSh`,
`TestDeployService_NoEnvShIsValid`,
`TestDeployService_ExplicitEnvFileMissingReturnsExitConfigError`,
`TestDeploy_NoEnvScript_SkipsCapturerEntirely`, plus three envcap
unit tests. Joel's "no env-var fallback, no fancy precedence"
restraint held — the precedence rule is one screen of code.

### L3 — `NewHTTPProbeForTest` rename → **FIXED**

`internal/deploy/readiness.go:19-22` exports `NewHTTPProbe`. The
unexported `newHTTPProbe` is the production constructor; the exported
`NewHTTPProbe` is a thin wrapper for `package deploy_test` callers.
`grep -rn NewHTTPProbeForTest internal/ cmd/` returns zero hits. Six
test call sites in `readiness_test.go` updated. The doc comment is
honest now ("constructs the production HTTP readiness probe"); no
more `_ForTest` apologetic naming. Two seams collapsed into one with
clear roles: `Dependencies.Probe` for inject-a-fake (unit tests of
the orchestrator), `NewHTTPProbe` for exercise-the-real-probe-with-
mocked-driver (probe-loop unit tests). Production code uses neither
exported name; `newServiceDeployer` calls the unexported
`newHTTPProbe(deps.Driver)` directly.

### Missing artifact — `_ai/decisions/m1-test-strategy.md` → **EXISTS, REAL DOCUMENTATION**

53 lines. Five sections: user directive (with the actual quote),
per-package test seam (every Driver method, every orchestrator step,
every lifecycle method named explicitly), the ten-item handoff
receipt as the manual-CI bridge until GitHub Actions arrives,
expected-breakage-class predictions for the user's real-system smoke
test (docker version skew, bash 3.2-vs-5 differences, caddy
semantic differences, network-namespace assumptions), and the
`DECLOUD_LOG_TO_STDERR_ONLY=1` local-test-quiet tip.

This is real documentation, not aspirational fluff. Every claim
cross-references to code (`Driver` method names match
`internal/dockerdrv/driver.go`; the `compgen -e` mechanism matches
`internal/envcap/capture.go`). The §4 breakage predictions read like
a debugging starter pack for future-Don when the user reports the
inevitable real-system failure. Indexed in `_ai/MEMORY.md:10`.

---

## Deferred items: NetworkEnsure-in-Start gap

Confirmed NOT done in iter2. `internal/deploy/lifecycle.go:66-78`
(`Start`'s `default` branch, the absent→Run path) calls
`Driver.Run` with `Network: "decloud"` but does not call
`Driver.NetworkEnsure` first. Same gap in `Restart` (which routes
through `Stop`+`Start`). This is the M1.x backlog item I noted in my
plan-review, and it stays deferred — Joel scoped iter2 to "Deploy
step 0 only" per Don's punch list, and Joel/Rob held that line.

**Visibility caveat (NOT a blocker):** there is no formal M1.x
backlog file in `_ai/` or anywhere else. All five deferred items
(Start-network gap, S1 lifecycle command duplication, S6
`assert.ErrorIs` migration, S-NEW-1 `Capture("")` comment, S-NEW-2
warning-quiet env var) only live in task-directory review files
buried under `_tasks/2026-04-26-m1-implementation/`. When future-Don
goes looking for "what did we punt out of M1?" he will have to
re-read the entire task tree. **Recommendation for M1.x kickoff:
extract these into a single `_ai/m1x-backlog.md` or equivalent
before the task directory rotates.** Don's call; not a M1 ship
blocker.

---

## Iter2 architectural drift check

**None.** Spot-checked the things I cared about:

- One struct, two interfaces still holds: `serviceDeployer` implements
  both `ServiceDeployer` and `Lifecycle`. No new types added.
- No new exit codes. `NetworkEnsure` failure rides on existing
  `ExitRunFail` (40); `ErrEnvScriptMissing/Unreadable` ride on
  existing `ExitConfigError` (10). Surface is unchanged at the
  integer-constant level.
- Test architecture (Gomock for store/capturer/driver/generator/
  reloader; real bash for envcap; no real Docker/Caddy) holding up
  exactly as `m1-test-strategy.md` documents. No integration test
  files snuck in.
- `%w: %v` → `%w: %w` audit complete across 21 sites; `grep -rn '%w:
  %v' internal/ cmd/` returns empty. Inner sentinels now traverse
  the chain. `TestDeploy_BuildErrorPreservesInnerSentinel` proves
  the fix is real with a synthetic `errors.New` survivor.

The lifecycle/deploy split is still clean. Single `serviceDeployer`
struct, two interfaces, the two CLI factories `NewServiceDeployer`
and `NewLifecycle` both delegate to `newServiceDeployer`. No
spaghetti. No scattered state machine. Rob held the line I asked
for in plan review.

---

## Operator on a fresh box: end-to-end mental trace

`_docs/install.md`:
1. Install Docker (§2) — upstream procedure.
2. Install Caddy (§3) — systemd unit drop-in, no `--force` (matches
   `internal/caddy/reloader.go`).
3. Bootstrap directory layout (§4).
4. `docker network create decloud` (§5).
5. Install the binary (§6).
6. Smoke-test (§7).

`_docs/usage.md`:
1. `decloud --help` works on the fresh box even before step 5 of
   install — confirmed via live binary at
   `DECLOUD_ROOT=/nonexistent/path/abc /tmp/decloud --help` exiting 0.
2. `decloud deploy service --name x --host x.example.com --port 8080
   ./svc` with no `env.sh` deploys cleanly (auto-discovery silently
   skips, deploy proceeds).
3. If the operator runs `docker network rm decloud` for some reason
   between deploys, the next deploy self-heals at step 0
   (`NetworkEnsure`). The `usage.md` exit-40 row tells the truth
   about the failure mode.
4. Exit code table is precise in both directions: docker-stop-on-
   missing-container → exit 10 (not 40); explicit
   `--env-file=<missing>` → exit 10 (not 70).

The operator path holds together. Nothing in the README's M1 scope
is missing — `deploy service`, all seven lifecycle commands
(`unregister`/`start`/`stop`/`restart`/`status`/`logs`/`caddy
reload`), env capture from `env.sh`, recreate strategy, host-side
readiness probe, atomic Caddyfile swap — all present.

---

## Praise

- `Capture("")` defensive return + orchestrator `if envFile != ""`
  guard. Two-layer defense, no coupling. The pattern Kevlin called
  out in his B2 was implemented cleanly across both layers.
- `gomock.InOrder` in `TestDeploy_NetworkEnsureCalledFirst` is the
  right kind of contract test — it pins ordering, not state. Future
  changes that move `NetworkEnsure` later in the sequence will fail
  loudly with "missing call" gomock errors. That's the test I
  asked for in L1 option B.
- `m1-test-strategy.md` §4 expected-breakage predictions. Future-Don
  reading this when the user files a bug report will know exactly
  where to look first. Not aspirational; specific failure modes by
  package.
- Restraint on deferred items. S1 (lifecycle dedup), S6 (assert
  migration), and the Start-network gap all stayed deferred. No
  half-done refactors. Half-done refactors are worse than not-done
  refactors and Rob held that line.
- The `--force` removal in `_docs/install.md` step 3 is the smallest
  possible doc fix that closes a real consistency gap with
  `internal/caddy/reloader.go:38-48`.

---

## Final concerns (non-blocking)

1. **No M1.x backlog tracker.** Five deferred items live only in task
   review files. When the task directory archives, discovery becomes
   archaeology. Recommend Ward extracts them into
   `_ai/m1x-backlog.md` during finalization.
2. **`Capture("")` defensive branch lacks an inline comment.** Kevlin's
   S-NEW-1. Three lines of comment explaining "production callers
   guard at the call site; this branch is a safety net" would make
   the contract self-documenting. M1.x candidate, not blocker.
3. **Logging warning leaks to test stderr.** Cosmetic. Rob's subtle-
   behavior #1 and Kevlin's S-NEW-2. The strategy doc §5 names the
   `DECLOUD_LOG_TO_STDERR_ONLY=1` workaround. M1.x candidate.

None of these block M1.

---

## Final word

Ship M1. Hand it to the user. Joel's iter2 plan was the cleanest
delta-spec we have produced this whole task; Rob's implementation
matched it verbatim with one-deviation honesty (`c` variable name
in service.go, noted and dismissed); Kevlin's re-review was thorough
on every layer; Raymond's docs are precise and free of
hallucinations.

Iter2 closed three real architectural blockers without introducing
new ones. The receipt-as-CI-bridge holds water. Operators can use
the binary on a fresh box. M1 is done.

Linus signs off. Onward to Step 4 finalization.
