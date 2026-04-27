# 011 — Kevlin: low-level review (post-implementation)

Author: Kevlin Henney (low-level review agent)
Date: 2026-04-27
Reviews: code on disk against `006-joel-tech-plan-v2.md`, `007-linus-review-v2.md`, `008-kent-tests.md`, `009-rob-implementation.md`, `010-raymond-docs.md`.

---

## TL;DR

Implementation matches the v2 plan with high fidelity. Code reads cleanly, the algorithms in `Manager.Up`/`Down` and `Reloader.execCaddy` follow the spec faithfully, error wrap chains preserve `errors.Is` discipline, and tests exercise the contracts (not implementations). `gofmt -l internal/` empty; `go vet ./...` empty; `go generate ./...` produces no diff; `go test ./... -count=1` is green across all packages.

I found **two minor doc accuracy issues** in `_docs/install.md` §7 — fabricated example error texts that don't match what the code actually emits — and **one cosmetic duplication-of-wrap-text** that Joel already discussed and chose to leave (rule-of-three threshold).

None of these gate execution. **APPROVED**, with two doc nits Raymond may want to address before sign-off (or roll into a follow-up).

---

## Verification gate (Joel §7 Phase 7)

- `gofmt -l internal/` → empty.
- `go vet ./...` → empty.
- `go generate ./...` followed by `git status` → no new diffs (mocks committed in sync).
- `go test ./... -count=1` → all packages green:

```
ok   internal/caddy           0.018s
ok   internal/cli             0.021s
ok   internal/config          0.009s
ok   internal/deploy          12.077s
ok   internal/dockerdrv       0.077s
ok   internal/envcap          0.111s
ok   internal/ids             0.013s
ok   internal/logging         0.014s
ok   internal/registry        0.041s
```

All gates pass.

---

## Code correctness audit

### `internal/dockerdrv/cli_driver.go`

- `ImagePull` (lines 193-201): emits `docker pull <ref>`, wraps with stderr on failure. Matches Joel §4.7.
- `RunWithOptions` (lines 203-242): builds argv in the specified order — base flags, env (sorted), labels (sorted), ports (declared order), volumes (declared order), image. Wraps with stderr on failure, returns trimmed stdout. Matches Joel §4.8 exactly.
- `Exec` (lines 244-263): `MultiWriter` over caller's writer + internal buffer for `isNotFound` detection. Maps stderr `No such container` to `ErrContainerNotFound`. Wraps generic failures. Matches Joel §4.6.
- `formatPortMap` (lines 268-277): splices `HostBind` literally; defaults `Proto` to `tcp`; collapses to `<host>:<container>/<proto>` when `HostBind` is empty. The doc-comment explicitly documents the no-auto-bracket contract for IPv6. Matches Joel §9.9.
- `formatVolume` (lines 279-285): `<src>:<dst>[:ro]`; ignores `IsNamed` on render (Docker disambiguates by source-shape). Joel §3.1 / §4.8 explicitly endorses this — the field disambiguates at the type level for callers, not at the argv level.

`io` is correctly imported (already was; no spurious additions).

### `internal/caddy/manager.go`

- Constants `ContainerName`, `NetworkName`, `DefaultImage` declared exactly per Joel §5.
- Sentinels `ErrCaddyUp`, `ErrCaddyDown` declared exactly per Joel §4.1.
- `Up` (lines 64-99): five-step algorithm matches Joel §4.2 byte-for-byte. Both `%w: %w` legs wrap correctly (network-ensure, stub-write, inspect, start, image-pull, run). The `unexpected state` case uses `fmt.Errorf("%w: unexpected container state %q", ErrCaddyUp, inspect.State)` — single `%w`, the state literal goes through `%q` which is correct.
- `Down` (lines 101-110): two steps with `errors.Is(err, dockerdrv.ErrContainerNotFound)` guards, both wrapped with `ErrCaddyDown` on hard failures. Matches Joel §4.3.
- `IsRunning` (lines 112-118): `Inspect` then state comparison. Matches Joel §4.4.
- `runOpts` (lines 120-141): exact six-port dual-stack `RunOptions` literal, three volumes in declared order. Matches Joel §3.2 byte-for-byte.

`NewCLIManager` defaults nil `Stdout` to `os.Stdout` (line 58-60). Joel §4.1 specified this; without it the production wiring (`buildProductionCaddyManager` doesn't set Stdout) would panic on first `Fprintln`. Defensive and correct.

### `internal/caddy/reloader.go`

- `cliReloader` struct holds only `driver` and `hostCaddyDir`. `cmdFactory`, `newCLIReloaderWithFactory`, the `cmdFactory` type alias — all gone. Joel acceptance criterion #21 satisfied.
- `Reloader` interface doc-comment carries the bind-mount contract (lines 17-32) per Linus §5.4.
- `execCaddy` (lines 54-73): always allocates `bytes.Buffer` for stderr capture (never relies on caller passing one), invokes `Driver.Exec` with `["caddy", sub, "--config", ctrPath]`, branches on `errors.Is(err, ErrContainerNotFound)` AND the `isNotRunningStderr` substring detection. Both legs surface the actionable "container … is not running; run 'decloud caddy up' first" text. The generic-failure leg keeps `%w` wrap so inner sentinels survive `errors.Is`. Matches Joel §4.5.
- `translatePath` (lines 75-84): `filepath.Rel` against cleaned root, rejects `rel == ".."` AND `strings.HasPrefix(rel, ".."+string(filepath.Separator))`. Pipes the relative segment through `filepath.ToSlash` before `path.Join("/etc/caddy", …)` so Windows dev boxes don't emit backslashes. Rob's defensive `ToSlash` was a small extension over Joel's literal — correct, cheap insurance, and Linux/macOS happy paths are unaffected.
- `isNotRunningStderr` (lines 89-91): case-insensitive substring `"is not running"`. Linus's non-blocking nit #1 acknowledged the fragility; the test `TestReloader_ContainerExitedSurfacesActionableError` locks the current shape so a Docker upgrade that reworded the message would fail loudly and visibly.

### `internal/cli/caddy_up.go`, `caddy_down.go`

Both are 23-line Cobra command builders. `cobra.NoArgs` enforced; `caddyManagerFactory` invoked through the test seam; context propagation via `cmd.Context()`. Help text is `Short`-only — Raymond noted in his report (§Style §5) that the Joel-spec longer `Long` block is not present. That's a code-side question for Don/Joel/Linus to weigh; it's not a hallucination, it's a spec-code mismatch on optional polish text. Flagging without recommending — operators see the `Short` text, which is concise and accurate; the `Long` block in the spec is a nice-to-have.

### `internal/cli/exit_codes.go`

- Single `case` line (line 58) maps both `caddy.ErrCaddyUp` and `caddy.ErrCaddyDown` to `ExitRunFail` via `errors.Is`. Joel §1.5 / §6.5 satisfied with **no new constants**.

### `internal/deploy/service.go:314-322`

Both legs (validate, reload) carry the suffix `; service is registered and running but Caddy is not routing traffic; run 'decloud caddy up' (and then 'decloud caddy reload' if needed) to restore routing`. The `%w: %w` chain is preserved on both legs. Matches Joel §4.9 byte-for-byte.

The duplication is real (same suffix appears twice on lines 316 and 322). Joel §4.9 documented the choice not to factor it into a constant — two sites is below the rule-of-three threshold. I agree. If a third site appears in M4, refactor.

### `internal/cli/deploy_service.go`

- `caddyManagerFactory` test seam (line 33) mirrors `deployerFactory`/`lifecycleFactory` and shares the parallel-safety caveat (line 31-32 doc-comment).
- `buildProductionDeployer` and `buildProductionLifecycle` both pass `driver` and `paths.CaddyDir` to `caddy.NewCLIReloader` (lines 139, 151). The plan said both should hold the same driver instance per CLI invocation; both functions construct a fresh `dockerdrv.NewCLIDriver()` and reuse it for both `Driver` and the reloader. Correct.
- `buildProductionCaddyManager` (line 155) constructs `caddy.NewCLIManager` with `Driver` and `Paths`, leaves `Stdout` nil (defaulted to `os.Stdout` inside the constructor). Correct.

---

## Test quality audit

### `internal/caddy/manager_test.go`

- `gomock.InOrder` used for ordering-sensitive flows (fresh-install, already-running, after-prior-stop) per `_ai/gomock-inorder-sequencing.md`. Other tests use plain `EXPECT()` where order is irrelevant.
- `TestManager_UpFreshInstall` (lines 77-91): asserts the **exact** `RunOptions` literal via `expectedCaddyRunOptions(h.paths)` — six dual-stack PortMaps, three VolumeMounts, the label, restart policy, and image. This is the canonical contract test for §3.2. Stdout assertions are loose (`Contains`) which is correct — the operator-facing text shouldn't be a brittle exact-match.
- `TestManager_UpRunFailsWithoutRollback` (lines 159-176): explicit `Times(0)` on `Stop` and `Remove` — locks the no-rollback contract per Linus revision #2. Strong test.
- `TestManager_UpStubWriteFailsWrappedAsCaddyUp` (lines 178-188): pre-creates `Paths.CaddyDir` as a regular file so `MkdirAll` fails inside `WriteStubIfMissing`. Kent's report §216 flagged this read of Joel's spec; Rob's implementation passes the test, so the read is correct.
- `TestManager_UpStubWriteIdempotent` (lines 203-218): asserts pre-existing operator content survives `Up`. Important behavioural lock — `WriteStubIfMissing` must not clobber.
- `TestManager_DownStopFailsHard` (lines 245-256): asserts `Remove` is `Times(0)` on hard `Stop` failure. Locks the early-return contract.
- `IsRunning` covered three sub-cases (running/exited/absent). Joel §6.1 said three sub-tests; Kent split into three top-level tests, which is fine.

No change-detector tests. Helpers (`newManagerHarness`, `absentInspect`, `runningInspect`, `exitedInspect`, `expectedCaddyRunOptions`) are all reused across multiple tests and named for intent.

### `internal/caddy/reloader_test.go`

- The three obsolete `cmdFactory`-coupled tests (`TestReloader_InvokesCaddyValidate`, `TestReloader_InvokesCaddyReload`, `TestReloader_ValidateFailureReturnsError`) are gone. The `recordingFactory`/`failingFactory` helpers are gone. Per Joel §6.2.
- New tests cover: argv-shape (validate/reload), path translation (canonical, outside-bind-mount, parent-escape), container-state errors (not-found and exited), generic-error wrap with stderr preservation, and the implementation-side contract that `Stderr` is always non-nil.
- `captureExec` helper records the `ExecOptions` once and re-uses across tests — cleanly avoids per-test `EXPECT().Do()` boilerplate.
- `TestReloader_PathTranslationParentEscape` is the negative case Linus called out as unrequested-but-welcome. It locks `/opt/decloud/config/caddy/../../etc/passwd` as a rejected path.

### `internal/dockerdrv/cli_driver_test.go`

- Existing 16 tests untouched and pass. New tests follow the file's existing argv-comment-then-assertion style (e.g., line 324-332 documents the canonical `docker run` and lines 334-358 assert against the precise argv slice).
- `TestCLIDriver_RunWithOptionsCaddyShape` is the canonical end-to-end shape test. Direct argv-slice equality assertion — no helper indirection.
- `TestCLIDriver_RunWithOptionsDualStackPorts` (lines 360-377) tests dual-stack independent of Caddy specifics, locking the `[::]` literal-splice behavior.
- `TestFormatPortMap_DoesNotAutoBracketIPv6` and `TestFormatPortMap_EmptyHostBindOmitsBindSegment` are direct unit tests on the helper — Joel §9.9 mandated locking the no-auto-bracket contract; this satisfies it.
- Helpers `portFlagsFromArgs`, `volumeFlagsFromArgs`, `labelFlagsFromArgs`, `flagValuesByName` are reusable and well-named.

### `internal/cli/caddy_up_test.go`, `caddy_down_test.go`

- `installMockCaddyManager` test seam helper installs the mock factory, wires cleanup. Mirrors `installMockDeployer` style.
- `TestCaddyUp_NoFlags` (`--image caddy:2.7.6`) and `TestCaddyDown_NoFlags` (`--remove-volumes`) lock the "no flags exist" contract via Cobra's default unknown-flag rejection. Kent's report flags these as already-passing tests-as-regression-guards. Acceptable — the contract is locked.
- `TestCaddyUp_PassesContextThrough` is a simple but useful guard — captures the context inside `DoAndReturn` and asserts it's non-nil. Stops a refactor that drops `cmd.Context()`.

### `internal/cli/exit_codes_test.go`

Four new sub-cases (`caddy-up`, `caddy-up-wrapped`, `caddy-down`, `caddy-down-wrapped`) added to the existing parameterised `TestExitCodeFor_AllSentinels`. Both bare and wrapped variants. Matches Joel §6.5.

### `internal/deploy/service_test.go`

- `TestDeploy_CaddyValidateFailureMentionsCaddyUpRecovery` and `TestDeploy_CaddyReloadFailureMentionsCaddyUpRecovery` are the new tests for the recovery wrap text. They assert:
  - `errors.Is(err, deploy.ErrCaddyReload)` still holds (wrap chain preserved);
  - on the reload leg, `errors.Is(err, innerErr)` holds (inner sentinel survives);
  - error text contains "decloud caddy up", "registered", and "Caddy is not routing".
- The existing `TestDeploy_CaddyValidateFailureLeavesOldFileAndKeepsNewContainer` and `...DoesNotRollBackContainer` still pass without modification — the wrap text change is additive.

---

## Doc accuracy audit (the explicit hallucination check)

Per CLAUDE.md, I scrutinised every command, flag, env var, path, port, error code, and exit code in `_docs/install.md` and `_docs/usage.md` against the actual implementation.

### Verified accurate

- `decloud caddy up` / `down` / `reload` — all exist in `internal/cli/caddy_up.go`, `caddy_down.go`, and the existing `caddy_reload.go`.
- `decloud caddy up`/`down` take **no flags** — confirmed in source (both `cobra.NoArgs`).
- Container name `decloud-caddy` — matches `caddy.ContainerName` in `internal/caddy/manager.go:18`.
- Network name `decloud` — matches `caddy.NetworkName` in `internal/caddy/manager.go:19`.
- Image `caddy:2` — matches `caddy.DefaultImage` in `internal/caddy/manager.go:20`.
- Volumes `decloud_caddy_data` and `decloud_caddy_config` mounted at `/data` and `/config` — match `internal/caddy/manager.go:137-138`.
- Bind mount `/opt/decloud/config/caddy` → `/etc/caddy:ro` — `Paths.CaddyDir` resolves to `/opt/decloud/config/caddy` per `internal/config/paths.go:33`; target/readonly match `internal/caddy/manager.go:136`.
- Six dual-stack `-p` entries on `80/tcp`, `443/tcp`, `443/udp` for `0.0.0.0` and `[::]` — match `internal/caddy/manager.go:127-134`.
- `--restart=unless-stopped` — matches `internal/caddy/manager.go:125`.
- `caddy down` 10s grace period — matches `internal/caddy/manager.go:102`.
- Exit code 40 for `caddy up`/`down` failures — matches `internal/cli/exit_codes.go:58-59`.
- Exit code 60 for caddy-reload-via-deploy — matches `internal/cli/exit_codes.go:56-57`.
- The `docker exec decloud-caddy caddy validate --config /etc/caddy/Caddyfile.tmp` shape (usage.md §7 line 209) — matches the cmd assembled in `internal/caddy/reloader.go:62`.
- Recovery wrap text on validate/reload failure — matches `internal/deploy/service.go:316,322`.
- `caddy already running` / `caddy started` / `caddy down: container removed (volumes retained)` — match `internal/caddy/manager.go:77, 83, 108`.

### Two minor doc accuracy nits

These are not blockers; Raymond may roll them into a follow-up edit.

**Nit 1: `_docs/install.md:173` shows a fabricated error rendering.**

```
caddy up: ports 80/443 already in use
```

The plan's §1.5 row 1 (Joel) called for a bespoke "ports already bound" detection branch — best-effort substring match against `address already in use` or `port is already allocated`. **That branch was never implemented.** The current code path wraps the docker stderr verbatim:

```go
return fmt.Errorf("%w: docker run: %w", ErrCaddyUp, err)
```

So the actual error string an operator will see for a port conflict is:

```
caddy: up failed: docker run: docker run: exit status N; stderr="...address already in use..."
```

Not `caddy up: ports 80/443 already in use`. The displayed error is logically what's happening, but it's not a copy-paste of what the operator sees. The recovery instructions are still correct; only the example string is wrong.

**Nit 2: `_docs/install.md:189` shows a similarly fabricated IPv6 error rendering.**

```
caddy up: docker run: listen tcp [::]:80: socket: address family not supported by protocol
```

The actual error path produces:

```
caddy: up failed: docker run: docker run: exit status N; stderr="<docker output incl. listen tcp [::]:80...>"
```

Same class of mismatch — recovery instructions valid, displayed prefix doesn't match.

**Suggested resolution (for Raymond, optional, not blocking):** either change the displayed examples to use a phrase like "containing text similar to …" or implement the substring detection branch in `Manager.Up` per the original Joel §1.5. Implementing the detection is the right long-term answer (better UX) but it's a follow-up, not part of this task.

---

## Migration recipe cross-check (`_docs/install.md` §3.2 vs. code)

- Recipe creates `decloud_caddy_data` named volume — matches what `decloud caddy up` creates (Docker auto-creates the volume on `docker run -v decloud_caddy_data:/data`).
- Recipe copies from `/var/lib/caddy/.local/share/caddy` to `/data` — that's the standard XDG_DATA_HOME default for the host-Caddy package install (Caddy runs as user `caddy`, home `/var/lib/caddy`); inside the official `caddy:2` image the data dir is `/data`. The doc explicitly notes the source path may vary (snap, alternative installs) and instructs the operator to verify with `find`. Cross-check accurate.
- LE rate-limit numbers (50 certs/domain/week, 5 duplicate certs/SAN/week, 7-day recovery) — correct as of 2026 LE policy.
- `systemctl mask caddy` / `apt-get remove -y caddy` — both valid persistent-disable mechanisms; the doc correctly explains why `disable --now` alone is insufficient against package upgrades.

---

## `_ai/decisions/caddy-runs-in-container.md` cross-check

Verified against the implementation:

- Container name, network name, image, volumes, bind mount, ports — all match the code (cross-references above).
- Rejected alternatives list matches `004-linus-review.md` §1.1 plus Don's A/B.
- Forward-looking M4 admin-API note is consistent with `9.7` of Joel v2 — admin API NOT host-published in M1, deferred to M4.
- Concurrent-deploy race acknowledged.
- "Why this isn't in `_docs/`" framing is correct.

`_ai/MEMORY.md` line 12 is the appropriate one-line summary pointing at the decision file. `MEMORY.md` is the project's `_ai/` library index file (lines 1-3 confirm: "Tactical reference for the Decloud codebase"). Correctly placed.

---

## Linus's v2 non-blocking nits — status check

Linus listed seven non-blocking nits in `007-linus-review-v2.md`:

1. **`isNotRunningStderr` substring fragility.** Test `TestReloader_ContainerExitedSurfacesActionableError` locks the current shape. Status: handled (test-as-canary).
2. **`ports already bound` substring detection fragility.** Different status — the substring detection was never implemented (see Nit 1 above). The doc still references it as if it were. Worth flagging for a follow-up.
3. **Wrap-text duplication in `service.go:314-322`.** Joel §4.9 documented the choice. Two sites; below rule-of-three. Status: deliberate, correct.
4. **Stdout shape inconsistency** (cold-start two lines, warm one line). Raymond's usage doc §4 doesn't explicitly call out the prefix-difference — operators piping to `grep "caddy up:"` would miss the warm-path messages. Minor doc enhancement opportunity, not blocking.
5. **`docker exec` doesn't pass `-i`.** Confirmed in `cli_driver.go:244-263`. Correct for non-interactive caddy invocations.
6. **`PortMap` empty `HostBind` fallback not used in M1.** Test `TestCLIDriver_RunWithOptionsEmptyHostBind` and `TestFormatPortMap_EmptyHostBindOmitsBindSegment` lock the contract. Status: M2+ contract, locked by test.
7. **`_docs/usage.md` quick-start should put `caddy up` first.** Verified in `_docs/usage.md:13-17` — `caddy up` instruction is the first step under §1, before the example Dockerfile. Status: handled.

Of the seven, #2 and #4 could be tightened in a follow-up but neither blocks task sign-off.

---

## Issues found (none blocking)

1. **`_docs/install.md:173` displays a fabricated "ports 80/443 already in use" rendering.** The substring-detection branch was specced (Joel §1.5 row 1) but never implemented. Either implement the branch or rephrase the example to match the actual wrapped stderr.
2. **`_docs/install.md:189` displays a fabricated IPv6-listener-fails rendering.** Same root cause and same resolution options.
3. **`_docs/usage.md` §4 doesn't document the cold-start vs. warm-path stdout-prefix inconsistency** (Linus nit #4). One sentence would help operators who script around `decloud caddy up`. Optional.
4. **`internal/cli/caddy_up.go` and `caddy_down.go` use only `Short` help text** (no `Long` block). Joel §1.6 specified longer help text. Per Raymond's report, this is a code-side question. Operators see concise, accurate `Short` help today; the `Long` block is polish, not contract.

None of 1-4 are blockers. None affect functional correctness, test quality, format/lint, or `go test` pass status.

---

## Verdict

**APPROVED** — task is ready to leave EXECUTION subject to Linus's parallel review. The implementation is faithful to the v2 plan, error wrap chains are correct, tests lock the contracts (including the no-rollback positive assertion and the path-translation negative cases), docs are largely accurate, and the architectural decision record is in place.

The doc nits (fabricated example error renderings) are real but minor — they should be addressed in a follow-up, either by implementing the planned substring-detection branches or by rewording the displayed examples.

— Kevlin
