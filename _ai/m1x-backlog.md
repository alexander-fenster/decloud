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

**Where:** `internal/logging/logging.go:29` and `:36` — `decloud: log dir unavailable, using stderr only: ...` fires once per CLI test that doesn't set `DECLOUD_LOG_TO_STDERR_ONLY=1`. Cosmetic; no test asserts stderr cleanliness, no test fails.

**Why deferred:** Workaround already documented in `_ai/decisions/m1-test-strategy.md` §5. Strictly bikeshed.

**Fix shape:** Add a `DECLOUD_TEST_QUIET` (or similar) env var that suppresses the warning while preserving the fallback behavior. One conditional in each fallback branch in `logging.Init`. Not the same lever as `DECLOUD_LOG_TO_STDERR_ONLY=1` — that one short-circuits the filesystem touch entirely; this one just silences the warning when the touch fails.

**Originator:** Kevlin S-NEW-2 + Rob subtle-behavior #1, `019-kevlin-rereview.md`.

---

## Maintenance note

When picking up an item, update this file: strike through, link to the commit, leave the entry one release before deletion so future-Don can correlate. Do not delete an entry the same iteration you fix it.
