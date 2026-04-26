# Don's PLAN-redux check after M1 execution round 1

**Author:** Don Melton (tech lead)
**Status:** REVIEW of Kent (008), Rob (009), Raymond (010), Kevlin (011), and the user-supplied summary of Linus's high-level review (the file `12-linus-review.md` was NOT written to disk despite task #17 marked complete — flagged separately below).
**Plan basis:** `05-plan-v2.md` §6 DONE-criteria 1–16; `06-tech-plan-v2.md` §15/§16; Kevlin's `011-kevlin-review.md`; Linus's L1/L2/L3 summary from the user prompt.

---

## VERDICT: NEEDS ANOTHER EXECUTION ROUND

Two real Blockers (Kevlin B1, Kevlin B2 / Linus L2) plus one dead-code surface (Linus L1) plus one missing decision-record artifact (`m1-test-strategy.md`) keep us off DONE. The build is green and 144 tests pass — that's the floor, not the ceiling. We do not ship a binary whose `--help` doesn't run on a fresh box. That's exactly the kind of "works on my machine" rot that killed Netscape 4.

The good news: every blocker is small, surgical, and identified. One-iteration fix. No replan, no architectural backflip.

---

## Per-finding rulings

### Kevlin B1 — `logging.Init()` mkdir before flag parse breaks `--help` on fresh box

**RULING: BLOCKER. Fix this round.**

I confirmed by reading `cmd/decloud/main.go:14-18` and `internal/logging/logging.go:14-32`. `Init()` runs unconditionally before `cli.NewRootCmd().ExecuteContext()` parses flags. The first thing the install doc tells the operator to do (`_docs/install.md` §6) is `decloud --help`. That fails with `Exit 70: mkdir /opt/declouding: permission denied` on a fresh, root-less install where `/opt/declouding/` doesn't exist yet. This invalidates the install doc's verify step and breaks the operator's first-touch experience. NOT acceptable.

**Decision on fix approach:** Combine BOTH options Kevlin offered, prefer #2 (graceful fallback) as the primary fix:

1. **Primary fix — graceful fallback in `logging.Init()`:** if `os.MkdirAll(logsDir, 0o755)` returns a permission/not-exist error, log one warning line to stderr ("logs dir unavailable, using stderr only: <err>") and configure stderr-only slog handler. Return nil. The binary works regardless of `/opt/declouding/` state.
2. **Belt-and-suspenders — defer init in PersistentPreRunE:** wrap `logging.Init()` in a `cobra.Command.PersistentPreRunE` on the root command so help/completion paths don't even attempt init. Cobra's `--help` and `help` subcommand short-circuit before PreRunE runs.

Both changes together close the failure mode whether the operator hits `--help`, `--version`, or actual subcommands on a half-bootstrapped box.

**Owner: Rob** (code change in `cmd/decloud/main.go` + `internal/logging/logging.go` + `internal/cli/root.go`). **Kent extends:** add `TestInit_PermissionDeniedFallsBackToStderr` (use `t.TempDir()` chmod 0o500 to force EACCES) and `TestRoot_HelpDoesNotRequireFilesystem` (run `NewRootCmd().SetArgs([]string{"--help"}).Execute()` with `DECLOUD_ROOT=/nonexistent/path` — must not error).

### Kevlin B2 / Linus L2 — `--env-file` auto-discovery + `Capture("")` semantics

**RULING: BLOCKER. Fix this round.**

I confirmed by reading `internal/cli/deploy_service.go:53,84` and `internal/envcap/capture.go:35-38`. The flag's help text promises `default: <source-dir>/env.sh if present`. The quick-start in `_docs/usage.md` §1 deploys WITHOUT `--env-file`. The implementation passes empty `f.EnvFile` straight through; `Capture("")` calls `os.Stat("")` which errors immediately with `stat : no such file or directory`. Documented behavior is fictional. Operators copy-paste the quick-start, get an error on the first deploy, conclude the tool is broken.

**Define the contract explicitly:**

1. **Auto-discovery in `runDeployService`:** if `f.EnvFile == ""`, set it to `filepath.Join(abs, "env.sh")` IF `os.Stat` shows that file exists. If the file does not exist, leave `EnvFile` empty.
2. **`env.sh` is genuinely optional:** if `req.EnvFile == ""` reaches `Deploy`, skip the `Capturer.Capture` call entirely; pass an empty `Env` map (or nil) to `Driver.Run`. Do NOT make `Capture("")` magically succeed — that hides bugs. The empty-path branch lives in the orchestrator, where it's visible.
3. **Explicit `--env-file` errors loudly:** if the operator passes `--env-file=/some/path` and the file is missing, that's a hard error with `ExitConfigError` (10). Distinct from the auto-discovery miss.

**Owner: Rob** (`internal/cli/deploy_service.go` runDeployService; `internal/deploy/service.go` Deploy step 1). **Kent extends:** `TestDeployService_AutoDiscoversEnvShWhenPresent`, `TestDeployService_NoEnvShIsValid`, `TestDeployService_ExplicitEnvFileMissingReturnsExitConfigError`. **Raymond updates:** `_docs/usage.md` §1 stays as-is (now correct); add one sentence to §1 explicitly stating "if you don't provide `--env-file`, decloud looks for `<source-dir>/env.sh` and uses it if present; otherwise the container runs with no extra env."

### Linus L1 — `NetworkEnsure` implemented but never called

**RULING: BLOCKER. Fix this round.**

I confirmed by `grep -rn NetworkEnsure` across the tree: defined in `dockerdrv.Driver`, implemented in `cli_driver.go:159`, mocked, tested twice — but ZERO production callers in `internal/deploy/` or `internal/cli/`. Plan-v2 §6.6 tech-plan §13.6 was explicit: "the deployer also self-heals via `docker network inspect ... || docker network create ...` per tech-plan §13.6." That self-heal does not happen. The install doc tells the operator to `docker network create decloud` manually (correct), but the plan ALSO promised resilience if the network disappears. Right now, if the operator (or another tool) removes the `decloud` network, every deploy fails with a docker error and decloud has no recovery.

**Decision: WIRE IT, do not delete.** This is exactly the kind of cheap insurance that makes a tool feel grown-up. Add `Driver.NetworkEnsure(ctx, "decloud")` as the FIRST orchestrator step in `Deploy` (before envcap), and as step 0 in `regenerateAndReload` callers that need the network. Cost: three lines, one new test (`TestDeploy_StepZeroEnsuresDecloudNetwork`).

**Owner: Rob.** **Kent extends:** one new test asserting `NetworkEnsure` is called before `Build`. The existing `TestCLIDriver_NetworkEnsureWhenAbsent`/`WhenPresent` already cover the driver behavior.

### Linus L3 — dead exported API `NewHTTPProbeForTest`

**RULING: REJECT Linus's "dead" claim. FIX-NOW the naming.**

I verified: `NewHTTPProbeForTest` IS used by 6 tests in `readiness_test.go` (lines 59, 68, 90, 101, 121, 141). It is the test seam Kent introduced and Kevlin already approved as Rob's deviation #1. Linus's "dead API" call is wrong. BUT Kevlin's S2 is right that the `_ForTest` suffix in a public symbol stinks. Two options:

- Rename `NewHTTPProbeForTest` → `NewHTTPProbe`, drop the unexported `newHTTPProbe` wrapper (or keep it private as a thin alias for production use).
- Keep `_ForTest` suffix.

**Decision: RENAME to `NewHTTPProbe`.** Public API, public name. Production code in `service.go:112` already calls `newHTTPProbe(deps.Driver)` privately; the rename + public constructor is a five-minute change with no behavioral effect. Tests update their import line.

**Owner: Rob.** No new tests needed; existing tests rename the symbol they call.

### Kevlin S1 — seven near-duplicate lifecycle command files

**RULING: SKIP this round. Defer to M1.x cleanup.**

The seven files (`unregister.go`, `start.go`, `stop.go`, `restart.go`, `status.go`, `logs.go`, `caddy_reload.go`) duplicate ~5 lines of `lifecycleFactory` boilerplate each. That's 35 lines of duplication. Kevlin's `withLifecycle` helper would shrink this. BUT:

- The duplication is honest, obvious, and self-contained. No correctness hazard.
- Refactoring during the same iteration as B1/B2/L1 fixes increases churn risk and review surface for no shipping benefit.
- The "fix obvious issues while code is fresh" rule applies to fixes that COST < 2 hours and IMPROVE maintainability. This refactor itself is small, but it touches every lifecycle command and every lifecycle CLI test — that's wider blast radius than the fix justifies under time pressure.

Queue for M1.x cleanup pass. NOT a DONE-criterion blocker.

### Kevlin S3 (`else` after `if-return`) and Kevlin S4 (`%w: %v` → `%w: %w`)

**RULING: MANDATE both. Fix-now.**

S3 is one local cleanup (`readiness.go:49-58`): drop the `else`, hoist `lastErr = err`, and add the `ipErr == nil && ip == ""` branch fix Kevlin flagged (today not a bug, tomorrow a silent lie). Five-line diff.

S4 is mechanical: 9 sites in `service.go`, 3 in `store.go` Save, 7 in `lifecycle.go`, 1 in `readiness.go` = 20 sites. Search-and-replace `%w: %v` → `%w: %w`. Go 1.20+ supports two `%w` verbs; `errors.Is`/`errors.As` callers downstream get the inner chain intact. That's not gold-plating — it's correctness for the FIRST debugger session that needs to walk the chain.

**Owner: Rob.** No new tests required (existing `errors.Is` tests still pass; new chain depth doesn't break them). Kent COULD add one `TestDeploy_BuildErrorPreservesInnerSentinel` to lock in the chained-wrap contract, but it's not a DONE-criterion.

### Kevlin S2 — `NewHTTPProbeForTest` naming

**RULING: COVERED by my L3 ruling above (rename to `NewHTTPProbe`).** Same fix.

### Kevlin S5, S6, S7 — stub-write-then-overwrite comment, `assert.ErrorIs`, test name drift

**RULING: SKIP this round. M1.x cleanup.**

S5 is a one-comment annotation; nice to have, not blocking. S6 is style cleanup that doesn't affect behavior. S7 (test rename `TestLifecycle_RestartFromAbsentReRunsContainer`) is already done and the new name is more truthful than the plan name — Kevlin flagged it for ME to record, not for Rob to change. **I am recording it here:** the test name drift is approved; Kent's name reflects shipped semantics.

### Kevlin Rob-deviation rulings — already approved by Kevlin

**RULING: ACCEPT all four** (Probe injection seam, filename overrides Config.Name, wider ErrUnknownField mapping, isCobraUsageError substring fallback). Kevlin walked each and approved. I concur. Raymond records the filename-as-canonical-key in `_ai/decisions/` (existing `secrets-split.md` is the right home — appended note, not a new file).

### Linus's high-level review file MISSING

**RULING: BLOCKER for process integrity, NOT for ship readiness.**

The user's prompt described Linus's L1/L2/L3 findings, but `12-linus-review.md` (or any file matching `*linus*`) is NOT on disk. Task #17 in the bureau task list says "Step 3e — Linus reviews changes (high-level) — completed" but no file exists. The L1/L2/L3 summary in the user prompt IS the review for our purposes — I've ruled on each finding. But for traceability, Linus must commit his review file in this iteration so the next Step 2-redux has the artifact to read. Andy: flag the missing-file pattern for HR.

### Missing artifacts: `_ai/decisions/m1-test-strategy.md`, `_docs/architecture/m1-recreate-strategy.md`, `_docs/cli/decloud-deploy-service.md`

**RULING:**

1. **`_ai/decisions/m1-test-strategy.md`** — **MANDATORY this round.** Plan-v2 §2.1 explicitly required this file ("Recorded in `_ai/decisions/m1-test-strategy.md` so future Don knows why this was skipped"). DONE-criterion #10 cites it by name. Raymond writes it. Owner: **Raymond.** Three short paragraphs: (a) integration tests deferred per user directive; (b) what unit-tests-only means for `dockerdrv`/`caddy`/`deploy` packages; (c) the receipt format Rob produced is the manual-CI bridge. Two pages max.

2. **`_docs/architecture/m1-recreate-strategy.md`** — **NOT REQUIRED.** Tech-plan §10 listed it as part of an "_docs/" doc table, but the USER's M1 ask was "installation instructions + short usage docs." Raymond shipped `_docs/install.md` + `_docs/usage.md` and they cover the operator's needs. Architecture docs are M2/M3 polish when the system grows past one deploy strategy. Skip. The "recreate vs blue/green" distinction is one paragraph in `_docs/usage.md` already.

3. **`_docs/cli/decloud-deploy-service.md`** — **NOT REQUIRED.** Same call. The flag table, exit codes, and behavior are all in `_docs/usage.md` §2/§3. A separate per-subcommand reference doc is M2 work when the surface area grows or when we generate from `cobra` automatically. The user gets full reference today via `decloud deploy service --help` and `_docs/usage.md`.

This is a DELIBERATE scope call. The user asked for "installation instructions + short usage docs" — Raymond delivered both. We are NOT failing M1 over architecture-doc absence the user did not request.

### Raymond doc nits N1, N2 + Linus's `caddy reload --force` mismatch

**RULING: FIX-NOW (small).**

N1: `_docs/install.md` §3 systemd unit's `ExecReload` includes `--force`, but `decloud caddy reload` doesn't. Either harmonize (drop `--force` from the systemd unit OR add it to `cliReloader.Reload`) OR add a one-sentence note explaining the difference. **Decision: harmonize — drop `--force` from the systemd unit drop-in.** `--force` is for first-time-load edge cases, not standard reload. Owner: **Raymond** (`_docs/install.md` §3 only).

N2: `_docs/usage.md` §3 row for exit 40. Easy doc fix — clarify that `Stop` mostly maps to exit 10, exit 40 is for true driver-level failures. Owner: **Raymond.**

### Linus's "Status format positional vs key=value" observation (review v2 line 91)

**RULING: ACCEPTED AS SHIPPED, no change.** Linus accepted Joel's format in v2 review. Kevlin verified it ships byte-identical to plan. No need to revisit.

---

## Punch list for next round (compressed)

For Joel to expand into tech-plan-v3 and Rob to implement:

| # | Item | Owner | Fix kind |
|---|------|-------|----------|
| 1 | `logging.Init()` graceful fallback on EACCES + PersistentPreRunE deferral | Rob + Kent | Code + 2 tests |
| 2 | `--env-file` auto-discovery; `env.sh` truly optional; explicit-missing → exit 10 | Rob + Kent + Raymond | Code + 3 tests + 1 doc sentence |
| 3 | Wire `NetworkEnsure` as Deploy step 0 | Rob + Kent | Code + 1 test |
| 4 | Rename `NewHTTPProbeForTest` → `NewHTTPProbe` | Rob | Rename only |
| 5 | `else`-after-return cleanup + ip-empty-no-error branch in `readiness.go` | Rob | Code only |
| 6 | `%w: %v` → `%w: %w` across 20 sites | Rob | Mechanical |
| 7 | Drop `--force` from systemd `ExecReload` in install doc | Raymond | Doc only |
| 8 | Clarify exit-40 row in usage doc | Raymond | Doc only |
| 9 | Write `_ai/decisions/m1-test-strategy.md` | Raymond | New file |
| 10 | Commit Linus's missing high-level review file | Linus | Process |

**SKIPPED this round:** S1 (`withLifecycle` helper), S5 (stub comment), S6 (`assert.ErrorIs`), L3-as-stated (Linus's "dead API" claim — it's not dead, just badly named, covered by item 4).

**SCOPE-CREEP rejected:** `_docs/architecture/*` and `_docs/cli/*` — user did not ask for them; `_docs/install.md` + `_docs/usage.md` are the operator-facing M1 deliverables.

---

## DONE-criteria for the next round (what Linus must verify)

When Step 2-redux fires next time, ALL of these must be true for M1 to ship:

1. `decloud --help` runs cleanly on a box where `/opt/declouding/` does not exist (verified by new test + manual smoke).
2. `decloud deploy service --name x --host y.example.com --port 8080 ./src` works when `./src/env.sh` exists (auto-discovered) AND when it doesn't (env.sh truly optional).
3. `decloud deploy service` calls `Driver.NetworkEnsure(ctx, "decloud")` before `Build` — verified by `TestDeploy_StepZeroEnsuresDecloudNetwork` with `gomock.InOrder`.
4. No exported symbol contains `_ForTest` or `ForTest` suffix.
5. `grep -n "%w: %v" internal/` returns nothing.
6. `_ai/decisions/m1-test-strategy.md` exists with the three paragraphs above.
7. `_docs/install.md` systemd unit `ExecReload` does NOT include `--force` (or if it does, includes a one-sentence rationale).
8. `_docs/usage.md` exit-40 row reflects shipped semantics.
9. `go test ./...` still green; `go vet ./...` still clean; `go generate ./...` still idempotent. Receipt re-attached.
10. Linus's high-level review file exists on disk in this task dir.
11. All 16 plan-v2 §6 DONE-criteria still hold (none regressed by these fixes).

If 1–11 all hold, Don/Joel/Linus sign off, Ward extracts learnings, M1 ships.

---

## Sequencing for next round

1. **Step 2b — Joel writes tech-plan-v3** (delta from v2, NOT a rewrite). Specifies the 10 punch-list items above with file/line precision.
2. **Step 2c — Linus reviews v3.** Should be one pass, no iteration — the deltas are mechanical.
3. **Step 3a — Kent writes the 6 new failing tests** (items 1, 2, 3 from punch list).
4. **Step 3b — Rob implements** items 1–6.
5. **Step 3c — Raymond writes** items 7, 8, 9.
6. **Steps 3d/3e — Kevlin and Linus review IN PARALLEL.** Linus MUST commit his file this time.
7. **Step 2-redux — Don walks the 11 DONE-criteria above.** If green, Step 4.

Estimated cycle time: half what round 1 took. Most punch-list items are mechanical.

---

## Final word

Round 1 shipped 144 green tests, a working orchestrator, complete lifecycle commands, two operator docs, and a 10-item test receipt that proves the build state. That's real work and Rob/Kent/Raymond did it well. Two real holes remain — both small, both surgical, both fixed in one short iteration.

The `--help` failure is the kind of bug that gets a tool laughed off Hacker News. The `--env-file` documentation lie is the kind of bug that gets a maintainer flooded with issues. Both fixed this round. The `NetworkEnsure` wiring closes the resilience promise the plan made. The dead-name rename closes a public-API papercut.

We do not ship until 1–11 are green. Kent, Rob, Raymond, Kevlin, Linus — back to it.

End of plan-check.
