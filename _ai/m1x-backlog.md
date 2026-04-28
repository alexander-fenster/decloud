# M1.x backlog — items deferred from M1, owned by future-Don

Five items punted out of M1 with explicit Don/Linus sign-off. Each was a non-blocker; do not reopen these as M1 regressions, they were known and documented at ship time. Originating reviews live under `_tasks/2026-04-26-m1-implementation/`.

## 1. Wire `NetworkEnsure` into `Lifecycle.Start`'s absent→Run branch

**Where:** `internal/deploy/lifecycle.go:66-78` — the `default:` arm of the `switch inspect.State` builds a `RunRequest` with `Network: "decloud"` and calls `Driver.Run`, but does NOT call `Driver.NetworkEnsure` first. Same gap shadows `Restart` since it routes through `Stop`+`Start`.

**Why deferred:** Iter2 was scoped to "Deploy step 0 only" per Don's punch list. Joel and Rob held that line — partial scoping a fix across two call sites is worse than punting both. Linus confirmed in `20-linus-rereview.md` §"Deferred items".

**Fix shape:** Prepend `NetworkEnsure(ctx, "decloud")` to the absent-branch in `Start`; wrap failure as `%w: ensuring decloud network: %w` against `ErrRun` for consistency with `service.go:131-135`. One line of code, one new gomock test mirroring `TestDeploy_NetworkEnsureCalledFirst`.

**Originator:** Linus, `20-linus-rereview.md`. Acknowledged by Don in `021-don-final-signoff.md` §"Deferred items" item 1.

## 2. Lifecycle command dedup via `withLifecycle` helper

**Where:** `internal/cli/{unregister,start,stop,restart,status,logs,caddy_reload}.go` — seven near-duplicate files, each ~12 lines, each with the identical `lc, err := lifecycleFactory(config.NewPaths(rc.ConfigRoot)); if err != nil { return fmt.Errorf("building lifecycle: %w", err) }` boilerplate. ~35 lines total of true duplication.

**Why deferred:** Cosmetic. Half-touching the seven files would be worse than touching none — Rob held that line correctly per Linus's praise.

**Fix shape:** Two-line `withLifecycle(rc, fn)` helper that returns the constructed `Lifecycle` or wraps the build error; each command body collapses to one `return withLifecycle(rc, func(lc deploy.Lifecycle) error { return lc.Stop(cmd.Context(), args[0]) })`.

**Originator:** Kevlin S1, `011-kevlin-review.md`. Reaffirmed Kevlin S1, `019-kevlin-rereview.md`.

## 3. `assert.True(t, errors.Is(err, X))` → `assert.ErrorIs(t, err, X)` migration

**Where:** ~30+ sites across `internal/deploy/lifecycle_test.go`, `internal/deploy/service_test.go`, `internal/deploy/readiness_test.go`. Find with: `grep -rn 'assert\.True(t, errors\.Is\|require\.True(t, errors\.Is' internal/`.

**Why deferred:** Mechanical. No behavior change. Half-converting is worse than not-converting — testify dual-form during transition reads worse than uniform old form.

**Fix shape:** Single-pass mechanical replacement. Run, regenerate mocks (no-op), run `go test ./...`, ship.

**Originator:** Kevlin S6, `011-kevlin-review.md`.

## 4. `Capture("")` defensive branch needs explanatory comment

**Where:** `internal/envcap/capture.go:46-49` — returns `(nil, nil)` for empty path. Contract is correct (orchestrator at `internal/deploy/service.go:139` guards `if envFile != ""` before calling). The branch is unreachable in production; a future bypass caller would currently see no warning that they're outside the documented contract.

**Why deferred:** Code is correct. Comment is a clarity improvement, not a behavior fix.

**Fix shape:** Three-line block comment above the early return explaining "production callers guard at the call site; this branch is a safety net so a future bypass doesn't crash with stat-empty-path". Wording suggested in `019-kevlin-rereview.md` §"Footgun" check.

**Originator:** Kevlin S-NEW-1, `019-kevlin-rereview.md`.

## 5. Logging warning leaks to test stderr; consider a quiet-mode env var

**Where:** `internal/logging/logging.go:32` and `:39` — `decloud: log dir unavailable, using stderr only: ...` fires once per CLI test that doesn't set `DECLOUD_LOG_TO_STDERR_ONLY=1` (and doesn't pass a writable `--config-root`). Mostly cosmetic; no test asserts stderr cleanliness, no test fails. After the `Init(root string)` change in `_tasks/2026-04-26-fix-deploy-service-review-findings/`, tests that pass `--config-root <t.TempDir()>` no longer trip the warning, so the noise is reduced but not gone.

**Why deferred:** Workaround already documented in `_ai/decisions/m1-test-strategy.md` §5. Strictly bikeshed.

**Fix shape:** Add a `DECLOUD_TEST_QUIET` (or similar) env var that suppresses the warning while preserving the fallback behavior. One conditional in each fallback branch in `logging.Init`. Not the same lever as `DECLOUD_LOG_TO_STDERR_ONLY=1` — that one short-circuits the filesystem touch entirely; this one just silences the warning when the touch fails.

**Originator:** Kevlin S-NEW-2 + Rob subtle-behavior #1, `019-kevlin-rereview.md`.

## 6. Docker-compose-based smoke integration test for M1 deploy + Caddy ingress

**Where:** No file yet. Likely lives at `internal/integration/` (new package) or `_test/integration/`. Test invokes `decloud caddy up`, `decloud deploy service` against a real Docker daemon (CI runner with Docker-in-Docker, or a tagged opt-in test that requires `DECLOUD_INTEGRATION=1`), asserts a real HTTP request through Caddy reaches a real upstream container.

**Why deferred:** Per `_ai/decisions/m1-test-strategy.md`, M1 is unit-tests-only against the gomock'd `Driver`. The bridge-DNS resolution path is locked architecturally by the `decloud-caddy`-on-`decloud`-network design (`_ai/decisions/caddy-runs-in-container.md`); the only thing a real-Docker test catches that unit tests miss is "is our argv actually accepted by docker?", and the argv-shape tests in `internal/dockerdrv/cli_driver_test.go` lock that argv byte-for-byte. Deferred from the caddy-container-connection-refused task per `_ai/decisions/m1-test-strategy.md`.

**Fix shape:** New `integration_test.go` build-tagged with `//go:build integration`, requires `DECLOUD_INTEGRATION=1` to run, brings up Caddy, deploys a one-line nginx service, curls through Caddy, asserts 200 OK with nginx body. Tear down both containers and the network on completion. Cleanup must be idempotent (test failures must not leave dangling containers). M2 material; M2 is also the milestone where reloader stderr `%q` quoting gets revisited, so the integration test naturally covers that improvement too.

**Originator:** Joel §8.5 of `_tasks/2026-04-27-caddy-container-connection-refused/006-joel-tech-plan-v2.md`. Acknowledged by Don in `_tasks/2026-04-27-caddy-container-connection-refused/012-don-final-review.md` §5.1. Reaffirmed in `_tasks/2026-04-27-caddy-container-connection-refused/013-joel-tech-plan-cycle2.md` §5.

## 7. Apply cleanup-context pattern to caddy/manager.go

**Where:** `internal/caddy/manager.go` — `Manager.Up`/`Down`/`Reload` and any `docker run`/`docker stop`/`docker rm` invocations therein.

**Why deferred:** scoped tight per `_tasks/2026-04-28-deploy-cleanup-on-interrupt/`. Same shape of bug as the deploy-service cleanup-on-interrupt fix (cleanup tied to user-cancellable ctx); user reported the deploy variant, not the caddy variant. The cleanup-context pattern (cleanup ctx derived from `context.Background()` with a 30s timeout, distinct from the request ctx) is locked in by that task and re-applies cleanly to the caddy code path.

**Fix shape:** identify cleanup blocks in `manager.go`, replace request-ctx with `newCleanupContext()`-derived ctx (move the helper from `internal/deploy/service.go` to a shared location if both packages need it, OR copy locally — bikeshed). Mirror the audit-log-on-cleanup-failure pattern from `_tasks/2026-04-28-deploy-cleanup-on-interrupt/03-tech-plan.md` §3.4.1.

**Originator:** Linus, `_tasks/2026-04-28-deploy-cleanup-on-interrupt/04-linus-review.md` Issue 5. Acknowledged by Don in `_tasks/2026-04-28-deploy-cleanup-on-interrupt/02-plan.md` §12.5.

## 8. `restoreOldContainer` failures should surface in the error chain

**Where:** `internal/deploy/service.go:282-300` (`restoreOldContainer`). Currently logs via `slog.Error` and returns silently.

**Why deferred:** pre-existing bug, scoped tight per `_tasks/2026-04-28-deploy-cleanup-on-interrupt/`. The cleanup-context fix in that task strictly improves the failure mode (rollback now actually runs on a non-cancelled ctx) but doesn't fix the surfacing. Doing both in one task expanded scope; punt.

**Fix shape:** change `restoreOldContainer` signature to return `error`. At each call site (3 in `Deploy` after that task), if the cleanup-path err is non-nil and `restoreOldContainer` ALSO returns an error, the surfaced error to the user should mention both ("readiness failed AND rollback to previous container failed"). `errors.Join` is the right tool. New test asserting both errors surface.

**Originator:** Linus, `_tasks/2026-04-28-deploy-cleanup-on-interrupt/04-linus-review.md` Issue 6. Acknowledged by Don in `_tasks/2026-04-28-deploy-cleanup-on-interrupt/02-plan.md` §12.6.

---

## Maintenance note

When picking up an item, update this file: strike through, link to the commit, leave the entry one release before deletion so future-Don can correlate. Do not delete an entry the same iteration you fix it.
