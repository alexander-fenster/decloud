# Linus Review v2: M1 Execution Plans (`05-plan-v2.md` + `06-tech-plan-v2.md`)

**Reviewer:** Linus Torvalds (high-level review, second round)
**Reviewing:** Don's `05-plan-v2.md` and Joel's `06-tech-plan-v2.md` after my `04-linus-review.md` REVISIONS REQUESTED.
**Prior review:** `_tasks/2026-04-26-m1-implementation/04-linus-review.md`.

---

## VERDICT: APPROVED

Proceed to execution. Kent → Rob → Raymond → Kevlin/Linus, per CLAUDE.md.

Don and Joel did real work. All four blocking items I flagged are closed with single, defensible answers — no three-options-no-decision text left anywhere. The non-blocking cleanups are all in. Joel's eight flags-for-Linus are defensible to-correct calls. The new design decisions Joel introduced (one struct / two interfaces, separate `lifecycle.go`, shared `regenerateAndReload` helper, `LastDeployedAt` schema extension, `Reloader.Validate` extension) are all the right calls and I would have made the same ones.

The 1879-line tech plan is large but proportionate — the §9.6 Lifecycle expansion alone is ~260 lines, and that is exactly what was missing from v1. There is no gold-plating; everything in §0..§17 traces to either v1 carryover or an explicit v2 delta. Joel did not invent scope; he closed gaps.

I have NO blockers and NO mandatory revisions. Six minor observations follow — all "do these in execution if convenient, log and move on if not." None of them blocks Kent from starting.

---

## Verification of v1 blockers

### Blocker #1 — Lifecycle methods unspecified — CLOSED

`06-tech-plan-v2.md` §9.6 (lines 1032–1291) gives every method:
- Signature
- Step-by-step sequence with which sentinel wraps which failure
- Exit code mapping (concrete numbers from §8.4 → cross-referenced)
- Operator-visible output rule (stdout silent on success, slog JSON to stderr with named step labels)
- Test names per branch

`Unregister` (§9.6.1) is the most ordering-sensitive method and gets the most thorough spec — six tests including the config-only-orphan path, the validate-failure-after-registry-deleted path, and the reload-failure path. `Start` (§9.6.3) correctly handles the three Inspect states (running/exited/absent) with the absent case re-running from `prev.Config.Build.ImageRef` and explicitly NOT rebuilding from source. `Restart` (§9.6.4) is stop-then-start (not `docker restart`), with `gomock.InOrder` asserting the sequence and the "stop tolerated as ErrNotFound, then re-run" test case.

Rob has zero design freedom left in lifecycle. He writes method bodies that match the spec. Good.

One small thing I want to note (NOT a blocker): §9.6.3 step 2 case "absent" assembles a `RunRequest` directly from `prev.Config.Build.ImageRef` + `prev.Secrets.Env`. This is functionally identical to `restoreOldContainer` (§9.3) — both reconstruct a RunRequest from a `*registry.Service`. There's a small DRY opportunity here for a private `runFromService(prev) RunRequest` builder, but the duplication is 8 lines and inlining is fine. Rob's call. Not flagging as a revision.

### Blocker #2 — Caddy reload safety — CLOSED

The `Reloader.Validate(ctx, configPath) error` extension (Joel §15.2) plus the `regenerateAndReload` helper (§9.6 preamble, lines 1038–1067) closes the failure mode correctly. The sequence is:

1. `Generator.Generate(tmpPath, services)`
2. `Reloader.Validate(ctx, tmpPath)` → on failure, `os.Remove(tmpPath)`, return `errCaddyReload`. **Old Caddyfile on disk untouched.**
3. `os.Rename(tmpPath, realPath)` (atomic)
4. `Reloader.Reload(ctx, realPath)` → on failure, `errCaddyReload`. New file IS on disk.

The §9.2 step 8b table row makes the policy explicit: pre-validation guarantees the on-disk file is reload-able. The forward-only-after-step-7b semantics for the deploy orchestrator are preserved (new container stays up; the operator's recovery path covers the runtime-failure window).

**Runtime-failure case** (port-bound, cert provisioning) — Joel's claim that Don §2.2.2.7 covers it. **Verified.** `05-plan-v2.md` §2.2.2 step 7 (lines 100–102) has the explicit recovery paragraph: read Caddy error log → `decloud unregister <name>` to drop the failing stanza → `decloud caddy reload`. The doc paragraph is concrete enough that an operator can follow it without inventing.

The `regenerateAndReload` helper is reused by Deploy step 8 (Joel will refactor inline to call it), Unregister §9.6.1 step 5, and CaddyReload §9.6.7 step 1. Three call sites, one implementation. This is correct DRY.

### Don's other decisions — verified respected

- **Readiness via host-side `docker inspect` IP — VERIFIED.** `Driver.OneShotProbe` deleted from §11.1 interface (line 1412 confirms "OneShotProbe REMOVED"). `Driver.ContainerIP` added (line 1411) with the `--format '{{ .NetworkSettings.Networks.decloud.IPAddress }}'` invocation in §11.4 (lines 1491–1495). The macOS Docker Desktop reachability is documented in §9.4 (line 946) — bridge driver mandate cross-referenced to installation.md step 5 per Don §2.2.1. The `httpProbe` re-resolves IP per-tick (§9.4 lines 900–903) which correctly handles both the docker-run-vs-network-attach race AND container-restart IP reassignment without a cache-invalidation question.
- **CI deferred; handoff receipts mandated — VERIFIED.** §16.1 (lines 1774–1841) specifies the exact ten-item receipt format Don laid down. The `git status --porcelain` after `go generate ./...` idempotency check is item 9; this is a nice catch — that was implicit in Don's §3.4 but Joel made it concrete. The receipt is explicitly the M1 acceptance gate per §6 criterion #1 — meaning Don's PLAN-redux check fails without it.
- **`RollbackPartialCreate` → `DeleteOrphanConfig` — VERIFIED.** §9.5 line 997 has the renamed method body. §9.5 line 1012 updated interface. §13.1 lines 1551–1552 has the renamed test names. §9.2 step 7b row references `Store.DeleteOrphanConfig`. §9.5 lines 1018–1026 shows the caller in service.go using the new name. Five-minute rename done correctly.
- **LICENSE one-sentence note pre-specified for Raymond — VERIFIED.** Don §2.2.1 step 7 (line 83) has the verbatim sentence. Raymond doesn't write the words — he transcribes them.
- **Mockgen layout asymmetry documented with rationale — VERIFIED.** Don §4.7 (lines 384–392) specifies the comment text. Joel's §15.7 records the deviation. Both consistent.

---

## Joel's eight flagged items — my calls

### §15.1 — `LastDeployedAt time.Time` added to `ServiceConfig`

**ACCEPTED.** Backward-compatible TOML schema addition (existing files without the field unmarshal to zero-value), schema version stays at 1. Necessary for `Status.LastDeployedAt` being meaningful rather than always-zero. Three lines of struct change plus a `time.Now().UTC()` set at Save time. No semantic break per the schema-versioning rules in prior `06-tech-plan-v2.md` §5.

The only thing I would have Joel double-check: `pelletier/go-toml/v2` round-trips `time.Time` as RFC3339; verify in the Save/Load tests. If the round-trip is lossy in any way (timezone normalization, sub-second truncation), the Status output `time.Format(time.RFC3339)` will not equal what was written. Test `TestStore_RoundTripConfigAndSecrets` (§13.1 line 1536) should cover this implicitly; if it doesn't, add `TestStore_RoundTripsLastDeployedAt` explicitly. **Rob: confirm in implementation.**

### §15.2 — `Reloader.Validate` extension to prior `Reloader` interface

**ACCEPTED.** Necessary, not optional. The pre-validation step is the entire point of closing Hole #2. The interface was always going to need this method; v1's `Reloader` was incomplete. The extension cost is one method signature, one CLI driver implementation (`caddy validate --config <path>` with stderr capture), and two tests (§13.3 lines 1578–1579). Could it live elsewhere? In principle a free `caddy.Validate(ctx, path)` package function would work, but co-locating with `Reload` (same binary, same exec pattern, same error-wrap) is the right shape.

### §15.4 — One struct (`*serviceDeployer`) implements both `ServiceDeployer` and `Lifecycle`

**ACCEPTED.** Joel's reasoning (§9.1 lines 796–799) is correct: identical dependencies, shared `regenerateAndReload` helper, two structs would be pure duplication. The alternative (private `regenerator` substruct, or split structs each holding `Dependencies`) is over-engineered for M1. If we ever need to split (e.g., M3 introduces a per-deployer state machine that doesn't apply to lifecycle), splitting then is a straightforward refactor. KISS wins now.

### §15.5 — Separate `lifecycle.go` file from `service.go`

**ACCEPTED.** Seven methods plus the `regenerateAndReload` helper in one file would push past 500 lines. Separation by concern (Deploy vs ongoing-lifecycle) maps cleanly to two files. Both in `package deploy`, methods on the same receiver. Standard Go pattern. No issue.

### §15.6 — `regenerateAndReload` shared helper across three call sites

**ACCEPTED.** This is correct DRY. Three call sites for the same five-step Caddy regeneration would otherwise be three copies of the same fifteen lines, with three opportunities for them to drift. Pulling into one method-private helper means the validate-then-rename invariant (the entire Hole #2 fix) is enforced in exactly one place. Future-proof for when M3 adds another regeneration trigger (e.g., job lifecycle).

### §15.8 — `Status` stdout format (space-separated `key=value`, no `--json` in M1)

**ACCEPTED for M1.** Don §3.1.5 already called this — single-line human format ships, `--json` is M1.5 if anyone asks. Joel's exact format `%s state=%s container=%s deploy=%s deployed_at=%s\n` (§8.3 line 613) is fine for human consumption.

One observation, NOT a blocker: the format mixes a positional first field (the name) with key=value pairs. A pure key=value format (`name=foo state=running container=decloud-foo deploy=20260426-1200-abc123 deployed_at=2026-04-26T12:00:00Z`) would be more parseable by `grep`/`awk` and slightly more consistent. Either works for an operator reading it. Joel's call; I accept.

### §13.6 readiness test — `TestReadiness_ContainerIPLookupFailureReturnsErrReadiness`

**ACCEPTED as currently specified (timeout-with-wrap).** Joel's §13.5 (lines 1624–1629; the readiness section is in §13.5, not §13.6 as you wrote — minor numbering nit) currently asserts the test does NOT fail-fast on a single ContainerIP error. It re-tries until the timeout deadline, then wraps the LAST inspect error with `errReadiness`.

I considered fail-fast and reject it for two reasons:
1. The race between `docker run` returning and the container being attached to the `decloud` network is genuinely transient — `ErrNoBridgeIP` on the first tick is normal, and §13.5 line 1626 has `TestReadiness_ContainerIPInitiallyEmptyThenReady` covering this case. Fail-fast on the first tick would break this normal startup path.
2. Even for hard errors (`ErrContainerNotFound`), persisting until the deadline gives the operator one timeout-period to fix (e.g., the container died mid-probe, the operator sees the readiness timeout, looks at `docker logs`, finds the container crash). Fail-fast would surface the same information faster but with a less-specific error message.

The current behavior is correct. Test name could arguably be more descriptive (`TestReadiness_PersistentContainerIPFailureTimesOutWithWrappedError`) but that's a stylistic nit, not a correctness issue.

### §15.3 + §15.7 — Driver method changes + mockgen layout

**Already covered in verification section above.** Both correct.

---

## Things I attacked on my own initiative

### A. Tech-plan size (1879 lines) — NOT a problem

I went in suspicious. M1 is one milestone; 1879 lines of tech plan smells like over-spec. Reading it end to end:
- §0..§5 (lines 1–326): change log, scope citations, file tree, deps, mockgen, slog. Foundational, terse. About 300 lines.
- §6..§8 (lines 327–722): cmd/decloud, internal/cli wiring including all seven lifecycle subcommands. Tight. Each lifecycle subcommand is ~25 lines of Go shown verbatim because they're nearly identical and the verbatim shape eliminates Rob's "did I get the cobra wiring right" question.
- §9 (lines 723–1293): the deploy + lifecycle orchestrator. THIS is where the v1→v2 expansion lives. §9.6 alone is ~260 lines, which is correct because v1 had ZERO behavior spec for these methods.
- §10..§12 (lines 1294–1530): mounts rejection + dockerdrv + ids. Mostly v1 carryover with surgical updates for the new Driver methods.
- §13 (lines 1531–1729): test plan. ~200 lines, one line per test name, organized by package. This is the right level — Kent gets a checklist.
- §14..§18: Knuth-call list, Linus-flag list, handoff format, no-change list, final word. Necessary boilerplate.

There is no padding. The expansion is proportional to the gap. **Approved.**

### B. Per-method exit codes — coherent across the seven lifecycle commands

Walked the §9.6 specs and the §8.4 mapper. Every method's error sentinels map cleanly:
- `registry.ErrNotFound` → `ExitConfigError` (10) — used by Stop, Start, Restart, Status, Logs when the service or container is absent and the operator should fix the registry first.
- `registry.ErrSecretsMissing` → `ExitConfigError` (10) — Start refuses (no env to run with); Status reports `config-only`. Consistent.
- `errCaddyReload` → `ExitCaddyReloadFail` (60) — Unregister steps 5–8, CaddyReload, Deploy step 8b/8c. Same code, same recovery story.
- `errRun` → `ExitRunFail` (40) — Stop unexpected driver errors; Start image-missing; Restart inherits.
- `errEnvCapture`, `errBuild`, `errReadiness` are Deploy-only and don't appear in lifecycle methods.

The thing I checked specifically: does `Restart` produce a coherent exit code when Stop succeeds (or is tolerated as ErrNotFound) and Start fails with ErrSecretsMissing? Walking §9.6.4: Stop returns nil-or-ErrNotFound (tolerated), then Start's step 1 returns `registry.ErrSecretsMissing` wrapped. The wrap propagates up; `ExitCodeFor` matches `ErrSecretsMissing` → `ExitConfigError` (10). Correct. The operator gets "config error" for a service that has a config-only orphan, which matches the meaning.

The only edge case worth noting: §9.6.4 `TestLifecycle_RestartStopFailureAbortsBeforeStart` covers Stop returning a real (non-ErrContainerNotFound) error. That gets wrapped with `errRun` (per §9.6.2 step 1's "other errors"), maps to `ExitRunFail` (40). Restart's caller sees `ExitRunFail`. Correct — the operator knows it's a runtime issue, not config.

**Exit codes are coherent.** No revisions.

### C. Anything missing from Raymond's doc requirements

Don's §2.2 covers installation + usage docs in detail. The README and architecture docs from prior tech-plan §10 also ship. Two things I want to confirm Raymond knows about (these may already be in his understanding from prior task; flagging defensively):

1. **The `caddy validate` PATH requirement** is in Don §2.2.1 step 3 (line 79). Good — Raymond writes it.
2. **The `decloud network create` MUST use default bridge driver** is in Don §2.2.1 step 5 (line 81). Good — Raymond writes the constraint, not just the command.
3. **One thing missing:** the `_docs/operator/usage.md` §6 says "If validation passed but the actual `caddy reload` failed (rare)..." — but Raymond should also mention that if the operator has manually edited `/opt/declouding/config/caddy/Caddyfile` (out-of-band), `decloud caddy reload` will REGENERATE from the registry and OVERWRITE the operator's edits. This is the trap an operator falls into when they hand-edit the Caddyfile to fix something and then run `caddy reload` expecting it to honor their edits. **Suggestion for Raymond's §6:** add one sentence: "Note: `decloud caddy reload` regenerates the Caddyfile from the registry; any manual edits to `/opt/declouding/config/caddy/Caddyfile` are discarded. If you need to inject custom Caddy directives per service, that's M5 work and currently unsupported."

This is NOT a blocker. Raymond may already know to write this from his prior task context. If he doesn't, it's a one-sentence add at doc-review time.

### D. Anything sketchy in the 1879 lines I haven't called out

Re-walked everything. Three minor observations, none rising to revision-required:

1. **§9.6.1 step 1 idempotence on `ErrSecretsMissing`** — Joel says "treat as 'config-only orphan'; proceed with steps 2–4 anyway, skipping anything that needs secrets (there's nothing in this path that does)." Correct, but "there's nothing in this path that does" is asserted not proven. Walking steps 2–4: Driver.Stop (just needs container name), Driver.Remove (same), Store.Delete (deletes both files; the secrets file is missing so the secrets-first delete is a no-op). All fine. Asserted-correctly. Not a problem.

2. **§9.6.2 `Stop` and §9.6.3 `Start` inconsistency on what "ErrContainerNotFound from Driver" means.** Stop's step 1 maps `ErrContainerNotFound` → `registry.ErrNotFound` (operator's container is gone; surface as "service not found"). Start's step 2 maps `inspect.State == "absent"` → re-run from registry. Different branches because Start can recover (registry has the spec) but Stop can't (nothing to stop). This is the right asymmetry but it's worth a one-line comment in `lifecycle.go` so a future reader doesn't assume the asymmetry is a bug.

3. **§13.5 `TestDeploy_CaddyValidateFailureLeavesOldFileAndKeepsNewContainer`** — this is the critical Hole #2 regression test. Asserts `os.Rename` NOT called, `Reload` NOT called, container NOT rolled back. Good. One refinement: the test name says "LeavesOldFileAndKeepsNewContainer" but the `os.Rename` assertion is what proves "leaves old file." Add an explicit filesystem assertion (`require.FileExists` on the old path with old contents) if the test setup uses a real temp dir, or accept the mock-level assertion as sufficient. Joel's call; the mock-level assertion is fine for unit-test scope.

**None of three are blockers.** All are "while you're in there" observations Rob can act on or skip.

### E. Things v2 did right that deserve called-out praise

- **§16.1 receipt format item 9 (`go generate ./...` idempotency check via `git status --porcelain`).** This is the M2-CI-when-it-arrives canary, enforced manually now. Joel made it concrete. Anyone else would have left this implicit and discovered the drift in M2.
- **§9.6 preamble `regenerateAndReload` helper.** Written ONCE, reused THREE times. The "future Caddy regeneration trigger" extensibility hook is implicit and free.
- **§13.5 `TestReadiness_ContainerIPInitiallyEmptyThenReady`.** This is the test that proves the per-tick re-resolution is doing real work, not just being defensive. The race it covers (docker-run-vs-network-attach) is the kind of thing that fails 1-in-100 deploys in production and is impossible to reproduce; locking it in with a deterministic test is correct.
- **§9.6.5 `Status` separation of `state="config-only"` from other states.** This is the operator's signal that something went wrong in a previous deploy and they need to `decloud unregister` to clean up. v1 didn't distinguish this case. v2 makes it visible.
- **Don §3.4 + Joel §16 together — receipt format with Bash version, Docker version, Caddy version recorded.** This is the kind of forensic discipline that pays off during the first user-reported "doesn't work on Linux" issue. The first question is always "what was the environment?" and now we have the answer in writing.

---

## Summary

All four blocking items from `04-linus-review.md` are closed:
1. **Lifecycle behavior** — §9.6 specifies all seven methods with sequences, sentinels, exit codes, tests. No spec gap remains.
2. **Caddy reload safety** — `Reloader.Validate` BEFORE atomic-rename, plus shared `regenerateAndReload` helper, plus documented runtime-failure recovery in usage.md §6.
3. **Readiness probe** — single answer: `Driver.ContainerIP` + host-side `httpProbe`. `OneShotProbe` deleted. No third-party image.
4. **CI deferral consolation prize** — receipt format defined to ten items; M1 acceptance gate per §6 cites it explicitly.

Joel's eight flagged items are all defensible to-correct calls and accepted.

The five `Driver`/`Reloader`/`ServiceConfig` interface extensions Joel called out (`Validate`, `Start`, `ContainerIP`, `LastDeployedAt`, removal of `OneShotProbe`) are all justified by the v2 decisions and are surgical.

No revisions required. Kent starts.

End of review v2.
