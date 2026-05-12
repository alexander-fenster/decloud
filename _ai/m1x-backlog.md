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

## 6. ~~Docker-compose-based smoke integration test for M1 deploy + Caddy ingress~~

**Status:** PARTIALLY DONE at M2. The `--mount`-only integration test shipped at M2 (see `_tasks/2026-04-28-m2-server-side-mounts/`); the curl-through-Caddy ingress half was split off and lives below as item 10. Reloader stderr `%q` revisit was split off and lives below as item 9. This entry stays one release per the maintenance note before deletion.

**M2 delivery:** `internal/integration/mount_test.go` with `//go:build integration` tag, gated on `DECLOUD_INTEGRATION=1`. Pulls `nginx:alpine` via the real `dockerdrv.CLIDriver`, calls `driver.Run` directly with a `Volumes: [...]` shape carrying one bind ro mount, and asserts `docker exec cat /data/marker.txt` returns the marker bytes. Cleanup via `t.Cleanup` with idempotent `docker rm -f decloud-mounttest`. Does NOT exercise the deploy orchestrator (build, readiness, Caddyfile generation, reload) — those are split to item 10 (curl-through-Caddy ingress test) per Joel decision 8 of the M2 tech plan. The `nginx:alpine` choice (rather than alpine) is deliberate: nginx idles in the foreground via `nginx -g daemon off;`, so the container stays alive long enough for `docker exec`; alpine's default `/bin/sh` CMD exits under `docker run -d` (Linus's catch in `011-linus-impl-review.md` §5, fix in EXECUTION v2).

**Originator:** Joel §8.5 of `_tasks/2026-04-27-caddy-container-connection-refused/006-joel-tech-plan-v2.md`. Acknowledged by Don in `_tasks/2026-04-27-caddy-container-connection-refused/012-don-final-review.md` §5.1. Reaffirmed in `_tasks/2026-04-27-caddy-container-connection-refused/013-joel-tech-plan-cycle2.md` §5. Split + partial-ship rationale: `_tasks/2026-04-28-m2-server-side-mounts/003-joel-tech-plan.md` Decisions 8 and 9.

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

## 9. Reloader stderr `%q` quoting revisit

**Where:** `internal/caddy/reloader.go:69`, `:72`, `:80` — three sites use `fmt.Errorf(... stderr=%q ...)` to surface Caddy's stderr. `%q` works for ASCII output but `strconv.Quote`-escapes Unicode in a way that may not match what operators want to read in a log line.

**Why deferred:** Originally bundled with item 6 (M1.x backlog) as something the next-real-Docker milestone would naturally cover. M2 declined to bundle: the `%q` issue is orthogonal to mounts (logging-formatting decision in the caddy reloader), and a fix would invite "did you audit the other `%q` sites?" — a different review surface. Split out of item 6 in `_tasks/2026-04-28-m2-server-side-mounts/003-joel-tech-plan.md` Decision 9.

**Fix shape:** audit the three sites, decide whether `%q` is right against alternatives (raw stderr appended with `\n` indent, JSON-quoted with explicit Unicode handling, `strings.TrimSpace` + bare). Pick one rendering and apply uniformly. Decision and writeup at fix time; not a precommitment.

**Originator:** Don §9 of `_tasks/2026-04-28-m2-server-side-mounts/002-don-plan.md`; Joel Decision 9 of `003-joel-tech-plan.md`.

## 10. Curl-through-Caddy integration test

**Where:** No file yet. Likely `internal/integration/ingress_test.go` (new), peer to the M2 `internal/integration/mount_test.go` that ships the `--mount`-only half.

**Why deferred:** Originally bundled with item 6 as the second half of the integration smoke test. M2 shipped the mount half only — the failure modes of mount verification (`docker exec cat`) and ingress verification (TLS, Caddyfile generation, port publishing) don't share a debugging surface, so bundling them compounds risk: a curl-through-Caddy failure on an IPv6-disabled host would block the M2 ship for a non-mount reason. Split out in `_tasks/2026-04-28-m2-server-side-mounts/003-joel-tech-plan.md` Decision 8.

**Fix shape:** new `internal/integration/ingress_test.go` build-tagged with `//go:build integration`, gated on `DECLOUD_INTEGRATION=1`. Brings up Caddy + a deploy with an HTTP host, asserts a curl through Caddy returns the expected upstream body. Idempotent cleanup of both containers and the named volumes. Picks up the same `t.TempDir()` + `t.Cleanup` discipline the M2 mount test already establishes.

**Originator:** Joel §8.5 of `_tasks/2026-04-27-caddy-container-connection-refused/006-joel-tech-plan-v2.md` (originally bundled in item 6). Split into its own entry per Joel Decision 8 of `_tasks/2026-04-28-m2-server-side-mounts/003-joel-tech-plan.md`.

## 11. Consolidate `Driver.Run` and `Driver.RunWithOptions`

**Where:** `internal/dockerdrv/driver.go` (`Driver` interface), `internal/dockerdrv/cli_driver.go` (`Run` and `RunWithOptions` impls), `internal/dockerdrv/mocks/mock_driver.go` (regen), and ~20 `Driver.EXPECT().Run(...)` call sites in `internal/deploy/service_test.go` + `internal/deploy/lifecycle_test.go`.

**Why deferred:** Decision 4 of `_tasks/2026-04-28-m2-server-side-mounts/003-joel-tech-plan.md` picked Option β (add `Volumes []VolumeMount` field to existing `RunRequest`) over Option α (drop `Run` and route everything through `RunWithOptions`). β kept the M2 diff narrow and avoided rewriting every existing mock expectation; α is the structurally cleaner end-state where the driver has one run path instead of two.

**Fix shape:** remove `Driver.Run`, switch every caller in `service.go`/`lifecycle.go` to `RunWithOptions`, regenerate `MockDriver`, rewrite `Driver.EXPECT().Run(...)` to `Driver.EXPECT().RunWithOptions(...)`, retire `RunRequest` (or keep as a thin alias). Roughly one hour of mechanical work, zero behaviour change.

**Originator:** Don §5 Option α of `_tasks/2026-04-28-m2-server-side-mounts/002-don-plan.md`; Joel Decision 4 of `003-joel-tech-plan.md`.

**Future-author note (Linus Observation A, recorded at M2 closeout):** When picking up this consolidation, the unified `RunOptions` should grow `Cmd []string` so future integration tests (or one-shot job/migration runners at M5+) don't need to pick a specific image with an idle CMD. The M2 integration test exposed this gap: `alpine:3.19` exits under `docker run -d` because its default CMD is `/bin/sh` reading closed stdin; M2 worked around this by switching the test to `nginx:alpine` (which idles in the foreground). Adding `Cmd []string` to the consolidated `RunOptions` removes that constraint and aligns the run path with `ExecOptions.Cmd`. Source: `_tasks/2026-04-28-m2-server-side-mounts/011-linus-impl-review.md` §"Observation A".

**Future-author note (journald-log-driver closeout, Don's P3):** When consolidating, `grep -F 'RunRequest/RunOptions' internal/dockerdrv/` and update the matching error-message strings. `ErrEmptyService`'s message ("populate Service in RunRequest/RunOptions") names both types verbatim; folding to one type leaves a stale literal that future maintainers will have to chase as a hallucination later. One-grep fix at consolidation time. Source: `_tasks/2026-05-12-journald-log-driver/10-don-final-check.md` §9 P3.

## 12. `decloud logs --history` to surface the journald cross-redeploy archive

**Where:** `internal/cli/logs.go` (current `decloud logs` is a thin pass-through to `docker logs` via `Driver.Logs`). The host journal already holds every line every Decloud-managed container has ever written, tagged `decloud/<service>` — the operator can already query it with `journalctl CONTAINER_TAG=decloud/<service>` per `_docs/usage.md` §6. The follow-up is to wrap that query behind a CLI flag so operators don't need to know the tag scheme.

**Why deferred:** Out of scope for the journald-log-driver task per Don's plan §6 and Joel's tech plan §10.9. The journald change itself is the load-bearing part (every container now writes to the host journal under a stable tag); the UX wrapper is a separate concern with its own design surface (flag shape, output formatting, `-f` semantics across journald vs `docker logs`, what to do when both ranges overlap).

**Fix shape:** add a `--history` (or `--since`, or both) flag to `decloud logs <name>`. Under the hood, shell out to `journalctl CONTAINER_TAG=decloud/<name>` with the operator's flags translated (`--tail N` → `-n N`, `-f` → `-f`, plus a new `--since` pass-through). Detect journald-driver containers via `docker inspect --format '{{.HostConfig.LogConfig.Type}}'` and fall back to `docker logs` if the driver is something else (defensive — Decloud always sets journald, but a hand-attached container or a future driver change shouldn't break the CLI). Decide whether `--history` is an explicit opt-in (preserves the "current container only" default `decloud logs` behaviour) or whether `decloud logs` switches to journald-by-default with a flag to opt out. Pick at fix time. Add an integration test under `internal/integration/` that asserts log lines from a pre-redeploy container are reachable through the new flag.

**Originator:** Don §6 of `_tasks/2026-05-12-journald-log-driver/02-plan.md`; Joel §10.9 of `_tasks/2026-05-12-journald-log-driver/03-tech-plan.md`. Acknowledged at task ship time as deferred follow-up.

---

## Maintenance note

When picking up an item, update this file: strike through, link to the commit, leave the entry one release before deletion so future-Don can correlate. Do not delete an entry the same iteration you fix it.
