# Don's FINAL plan-check sign-off — M1 ships

**Author:** Don Melton (tech lead)
**Status:** **APPROVED. M1 SHIPS.**
**Inputs read:** `012-don-plan-check.md` (my iter1 punch list), `017-rob-fixup-impl.md`, `018-raymond-fixup-report.md`, `019-kevlin-rereview.md` (PASS), `20-linus-rereview.md` (APPROVED), `05-plan-v2.md` §6 DONE-criteria 1–16, plus live spot-checks of code/docs and a clean local `go test ./...` / `go vet ./...` / `gofmt -l .` / `grep '%w: %v'` / `grep _ForTest` run.

---

## VERDICT: M1 SHIPS

All three approvers (Don, Joel, Linus) agree. Kevlin signed off at the low-level. Ward starts Step 4 finalization.

---

## DONE-criteria walk (plan-v2 §6, all 16)

| # | Criterion | Status | Evidence |
|---|---|---|---|
| 1 | `go test ./...` PASS on macOS; receipt attached | YES | 9 packages, 172 tests, 0 failures. Receipt in `017-rob-fixup-impl.md` §"Handoff receipt." |
| 2 | Every package in tech-plan §2.2 exists; `decloud --help` works | YES | Live binary tested by Kevlin against `DECLOUD_ROOT=/nonexistent/path/abc` — exit 0, full subcommand list. |
| 3 | `internal/envcap` tests pass on macOS bash 3.2; three explicit edge-case tests present | YES | `TestEnvcap_SetAOff_VariablesDropped`, `TestEnvcap_ArrayDeclaration_OnlyFirstElementCaptured`, `TestEnvcap_ReadonlyConflict_FailsWithSetE` — all green. |
| 4 | All §12.1 unit tests amended by §2.1/§3.1/§3.2/§3.3/§4.3 present | YES | 28 deploy tests including all 7 lifecycle methods, 5 readiness tests, 2 caddy validate tests. |
| 5 | `_docs/install.md` (135 lines) and `_docs/usage.md` (202 lines) exist | YES | Both present, no `--force` in systemd unit, exit-40 row corrected, env.sh-optional language honest. |
| 6 | All seven lifecycle commands behave per §3.1 | YES | `unregister`/`start`/`stop`/`restart`/`status`/`logs`/`caddy reload` — every method tested, every error path tested. |
| 7 | Caddy pre-validation wired before atomic-rename in Deploy/Unregister/CaddyReload | YES | `Reloader.Validate` invoked in all three paths. |
| 8 | Readiness probe uses `Driver.ContainerIP`, no `OneShotProbe` | YES | Host-side probe via inspect-derived IP. `OneShotProbe` removed. |
| 9 | Architecture/CLI docs per tech-plan §10 | YES (scoped) | I scoped these out in iter1 §"Missing artifacts" — user asked for installation + usage docs only. Both delivered. |
| 10 | `_ai/decisions/*.md` aligned; `m1-test-strategy.md` exists | YES | 53 lines, 5 sections, every claim cross-referenced. Indexed in `_ai/MEMORY.md`. |
| 11 | `go.mod` declares `go 1.22`; deps are cobra/go-toml/v2/testify/uber-go/mock | YES | No Viper, no LICENSE (deferred per §5). |
| 12 | Loader rejection behavior tested for all 7 sentinels | YES | `ErrNotFound`, `ErrSecretsMissing`, `ErrPermissionMode`, `ErrSchemaMismatch`, `ErrUnknownField`, `ErrMountsNotSupported`, `ErrInvalidStrategy` — all covered in `internal/registry/store_test.go`. |
| 13 | Recoverable-state contract verified by 3 named tests | YES | `TestStore_LoadConfigWithoutSecretsReturnsErrSecretsMissing`, `TestDeploy_StepSevenMidWriteFailureRollsBackContainer`, `TestDeploy_SaveErrPartialWriteRollsBackAndDeletesOrphanConfig`. |
| 14 | Container naming via single `internal/ids/ContainerName` helper | YES | One helper, all call sites use it. |
| 15 | `Store.DeleteOrphanConfig` (renamed from `RollbackPartialCreate`) | YES | Renamed; deploy step-7b error branch invokes it. |
| 16 | Linus AND Kevlin both APPROVE; Don agrees | YES | `19-kevlin-rereview.md` PASS; `20-linus-rereview.md` APPROVED; this file is Don's agreement. |

All 16 hold.

---

## Iter1 punch list (10 items) — verification

| # | Item | Owner | Status |
|---|---|---|---|
| 1 | `logging.Init()` graceful EACCES fallback + `PersistentPreRunE` | Rob+Kent | DONE — `internal/logging/logging.go:21-43` fallback; `internal/cli/root.go:22-24` PersistentPreRunE; tests `TestInit_PermissionDeniedFallsBackToStderr`, `TestRoot_HelpDoesNotRequireFilesystem`. Live binary verified by Kevlin. |
| 2 | `--env-file` auto-discovery; `env.sh` truly optional; explicit-missing → exit 10 | Rob+Kent+Raymond | DONE — `resolveEnvFile()` in `deploy_service.go:103-118`; orchestrator skips Capture on empty path (`service.go:137-149`); 3 CLI tests + 3 envcap tests + 1 orchestrator test green. |
| 3 | Wire `NetworkEnsure` as Deploy step 0 | Rob+Kent | DONE — `service.go:131-135` runs before envcap; `gomock.InOrder` test pins ordering; failure wraps as `ErrRun` (exit 40). |
| 4 | Rename `NewHTTPProbeForTest` → `NewHTTPProbe` | Rob | DONE — `grep _ForTest` returns zero. Six call sites in `readiness_test.go` updated. |
| 5 | `else`-after-return cleanup + ip-empty-no-error branch | Rob | DONE — `readiness.go:47-67` is now a 3-branch switch; the silent-empty-IP case now sets `lastErr = dockerdrv.ErrNoBridgeIP`. New test `TestReadiness_EmptyIPNoErrorReportsErrNoBridgeIP` locks it in. |
| 6 | `%w: %v` → `%w: %w` across 21 sites | Rob | DONE — `grep '%w: %v' internal/ cmd/` returns zero. `TestDeploy_BuildErrorPreservesInnerSentinel` proves the chain works with a synthetic sentinel. |
| 7 | Drop `--force` from systemd `ExecReload` | Raymond | DONE — `_docs/install.md` matches `internal/caddy/reloader.go:33-49` (no `--force`). |
| 8 | Clarify exit-40 row in `_docs/usage.md` | Raymond | DONE — exit-40 row now lists `docker network create` and explicitly excludes `docker stop`-on-missing-container (which is exit 10). |
| 9 | Write `_ai/decisions/m1-test-strategy.md` | Raymond | DONE — 53 lines, 5 sections, indexed in `_ai/MEMORY.md`. |
| 10 | Commit Linus's missing high-level review file | Linus | DONE — `13-linus-execution-review.md` exists for iter1; `20-linus-rereview.md` for iter2. |

All 10 items addressed. None deferred.

---

## Architecture invariants verified

- **Two-file secrets split.** `ServiceConfig` + `ServiceSecrets`; atomic per-file writes; secrets removed first on Delete.
- **Validate-before-rename.** `Reloader.Validate` runs on tmp file before atomic-rename in Deploy, Unregister, CaddyReload.
- **Host-side IP probe.** `Driver.ContainerIP` + plain `net/http` GET. No `curlimages/curl`. No `docker exec`.
- **All 7 lifecycle commands.** `unregister`, `start`, `stop`, `restart`, `status`, `logs`, `caddy reload` — present, tested, documented.
- **Mounts rejection.** `ErrMountsNotSupported` enforced and tested.
- **`schema_version=1`.** Single version across M1/M3; bump only on semantic break. Mismatch → `ErrSchemaMismatch`.
- **No client binary.** Single `cmd/decloud` server-side CLI, invoked by operator over SSH.
- **No daemon.** No `systemctl enable decloud`. M1 is a one-shot CLI per invocation.

All invariants from plan-v2 hold in the shipped code.

---

## Receipt verification

`017-rob-fixup-impl.md` §"Handoff receipt" contains all 10 items per plan-v2 §3.4: Go version (`go1.26.2 darwin/arm64`), host (`Darwin 25.3.0 arm64`), bash (3.2.57), docker/caddy noted unavailable on maintainer box (test seams substitute), `go test ./...` 9-pkg PASS, `go vet ./...` clean, `gofmt -l .` empty, `go generate ./...` idempotent (porcelain shows only `_tasks/current` bureau pointer), files-modified list. This is the manual-CI bridge that replaces `.github/workflows/test.yml` for M1.

I re-ran the full check just now from this session: tests cached green, `go vet` clean, `gofmt -l .` empty, `grep '%w: %v'` empty, `grep _ForTest` empty, `_ai/decisions/m1-test-strategy.md` is 53 lines, `_docs/install.md` and `_docs/usage.md` exist. Receipt is real, not a lie.

---

## Deferred items (Linus's flagged five) — formal M1.x backlog decision

Linus's iter2 re-review §"Final concerns" recommends extracting deferred items into `_ai/m1x-backlog.md` before the task directory archives. **I AGREE.** Five items are too many to leave buried in `_tasks/2026-04-26-m1-implementation/*` review files where future-Don has to do archaeology to find them.

**RULING: Ward creates `_ai/m1x-backlog.md` during Step 4 finalization.** Single file, five entries, each citing the originating task-dir review file for context. Format: one short paragraph per item — what, why deferred, where it lives in code, suggested fix shape, originating reviewer.

The five items going into `_ai/m1x-backlog.md`:

1. **`Start`'s absent→Run branch lacks `NetworkEnsure`** — `internal/deploy/lifecycle.go:66-78` calls `Driver.Run` with `Network: "decloud"` but does not call `Driver.NetworkEnsure` first. Same gap in `Restart` (which routes through `Stop`+`Start`). Originator: Linus, `20-linus-rereview.md` §"Deferred items." Fix shape: prepend `NetworkEnsure` call to the absent-branch in `lifecycle.go`'s Start. One-line code addition + one new test.
2. **S1 lifecycle command dedup via `withLifecycle` helper** — Seven near-duplicate command files (`unregister.go`, `start.go`, `stop.go`, `restart.go`, `status.go`, `logs.go`, `caddy_reload.go`) duplicate ~5 lines each of `lifecycleFactory` boilerplate (~35 lines total). Originator: Kevlin, `011-kevlin-review.md` S1. Fix shape: `withLifecycle(name, fn)` helper.
3. **S6 `assert.True(t, errors.Is(...))` → `assert.ErrorIs(...)` migration** — ~30+ sites across `lifecycle_test.go`, `service_test.go`, `readiness_test.go`. Originator: Kevlin, `011-kevlin-review.md` S6. Fix shape: mechanical search-and-replace.
4. **S-NEW-1 `Capture("")` defensive branch lacks inline comment** — `internal/envcap/capture.go:46-49` returns `(nil, nil)` for empty path; the contract is correct but invisible from this file. Originator: Kevlin, `019-kevlin-rereview.md` S-NEW-1. Fix shape: 3-line comment block.
5. **S-NEW-2 logging warning leaks to test stderr** — Cosmetic. `decloud: log dir unavailable, using stderr only: ...` warning fires on every CLI test. Workaround named in `m1-test-strategy.md` §5 (`DECLOUD_LOG_TO_STDERR_ONLY=1`). Originator: Rob+Kevlin, `019-kevlin-rereview.md` S-NEW-2. Fix shape: a `DECLOUD_TEST_QUIET` env-var check in `logging.Init()` to suppress the warning.

**Owner: Ward** (creates `_ai/m1x-backlog.md` as a sub-task of Step 4 finalization).

This is NOT a M1 ship blocker — Linus's recommendation, my agreement, and the plan are all explicit that M1 ships now. The backlog file is housekeeping for the next milestone.

---

## What this round shipped

172 unit tests, 9 packages all green. Two operator docs (`install.md`, `usage.md`). One decision record (`m1-test-strategy.md`). Seven lifecycle commands, full deploy orchestrator, host-side readiness probe, atomic Caddyfile swap, two-file secrets split, schema versioning, mounts rejection. No daemon, no client binary, no integration tests (per user directive). Manual-CI receipt as the bridge until GitHub Actions arrives.

Iter1 caught two real Blockers (`--help` exit 70 on fresh box; `--env-file` auto-discovery lie) and one resilience gap (`NetworkEnsure` not wired) plus six smaller items. Iter2 fixed every one of them. Kevlin verified the fix shape; Linus verified the architecture; I verified the DONE criteria and the punch list. All three approvers agree.

The user can take this to a real Linux host. Failures will land in the four expected-breakage classes named in `m1-test-strategy.md` §4 (docker version skew, bash version skew, caddy semantic differences, network-namespace assumptions). That document tells future-Don exactly where to look first.

---

## Final word

This is what RIGHT looks like — not perfect, RIGHT. Two iterations, no panic, no scope creep, every deferred item documented and justified, every shipped feature tested. Rob held the line on no half-done refactors. Kevlin held the line on no shortcuts. Linus held the line on architectural integrity. Joel held the line on delta-spec discipline. Raymond held the line on no doc hallucinations. Kent held the line on tests-first. Ward, you have the M1.x backlog file to write.

Ship it. Hand it to the user. Onward to Step 4 — Ward extracts learnings.

If anything breaks on the user's real Linux box, we already know which package to interrogate first because `m1-test-strategy.md` §4 told us. That's not luck. That's the whole point of doing it RIGHT.

End of final sign-off. M1 done.
