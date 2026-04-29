# 011 — Linus's high-level impl review (M2 shipped code)

Reviewing the SHIPPED state on `feat/m2-server-side-mounts`. Three commits since plan-approval (`006-linus-plan-review-v2.md` APPROVED): Kent's RED at `432e3e8`, Rob's GREEN at `ae87320`, Raymond's docs at `aee12f5`. Kevlin is doing low-level review in parallel; I'm staying on architecture and decisions per `_ai/MEMORY.md` agent roles.

## TL;DR

Don, Joel, Kent, Rob, and Raymond shipped M2 cleanly. The atomic flip is real, the plan-stage Issue 1 fix (dual-sentinel-chain → single-sentinel by factoring `FindDuplicateTarget`) is correctly implemented in code, β-decision aged well (small blast radius, mock regen no-op), the schema-stability promise holds (verified by reading `types.go`), and no scope creep snuck in.

**One real bug** in the shipped integration test that nobody — Kent, Rob, Raymond, me at the plan stage — caught: it runs `alpine:3.19` with no `Cmd` override, so the container exits before `Driver.Exec` can `cat /data/marker.txt`. The test compiles, the test was never actually run against real Docker (`go build -tags integration` is what Rob and Kent ran), and `RunRequest` has no `Cmd` field anyway, so there's no in-band fix — the test needs a different image (or a different Driver entry point with a `Cmd` slice).

**Two minor architectural observations** worth recording but not blocking. Listed below.

Decision: **APPROVED WITH MINOR FIXES** — the integration test fix is required (it's the verification mechanism for the M2 feature; shipping it broken negates the bundling argument we made at plan stage). The two minor architectural observations are FYI.

---

## Answers to the user's 10 specific questions

### 1. Did Joel's β decision (no RunRequest consolidation) age well?

**Yes, definitively.** Reading the shipped diff:

- `RunRequest` gained one field (`Volumes []VolumeMount`) at `internal/dockerdrv/driver.go:38`.
- `cli_driver.go:Run` gained three lines emitting `-v` flags at lines 61-63.
- `service.go` got one helper (`toVolumeMounts`, 19 lines) at the bottom and three one-line `Volumes:` populations at the existing call sites.
- `lifecycle.go` got one line in the absent-branch.
- **Mock regen was a true no-op** — verified at `008-rob-impl.md` "go generate ./...: no diffs."

If Joel had chosen α, every `Driver.EXPECT().Run(...)` in `service_test.go` and `lifecycle_test.go` (~20+ sites) would have become `RunWithOptions(...)`. The diff would have been ~10x larger and the bisect surface would have been muddy (a 50-line behaviour change inside a 500-line mechanical refactor). The β-shaped diff is exactly the milestone-sized diff Joel argued for.

The "if α had been done, this would be cleaner" smell is **not** present in the shipped code. `Run` and `RunWithOptions` continue to be feature-divergent (service deploys never publish ports, never set labels) — they were never accidentally divergent, and α can be done as no-op cleanup at any future milestone (now logged as m1x-backlog item 11).

**Verdict on β: correct call, aged perfectly.**

### 2. Five-surface atomic flip — did Rob actually ship it as ONE commit?

**Verified by reading `git show ae87320`:**

- Test file changes in Rob's commit: `git show ae87320 --name-only | grep _test.go` → empty. ✓
- Test file changes in Kent's RED commit: `internal/{cli,deploy,dockerdrv,integration,registry}` test files only. ✓
- Five surfaces in Rob's GREEN:
  - **Help text + StringArrayVar**: `internal/cli/deploy_service.go:61-62` ✓
  - **Sentinel deletion**: `internal/registry/errors.go` (`ErrMountsNotSupported` gone, verified by `git grep -F ErrMountsNotSupported -- '*.go'` → no matches) + `internal/cli/exit_codes.go:41` (case-list entry now `ErrInvalidMount`) ✓
  - **CLI accept (parseMountFlags)**: `internal/cli/deploy_service.go:71-75, 165-189` ✓
  - **Loader accept**: `internal/registry/store.go:68` (`ValidateMounts` call replaces `len > 0` rejection) ✓
  - **Runtime `-v`**: `internal/dockerdrv/cli_driver.go:61-63` (Run for-loop) + `internal/deploy/service.go:251, 320, 383` (three sites populating Volumes) + `internal/deploy/lifecycle.go:74` (absent-branch) ✓

All five surfaces flip in `ae87320` with zero test files. The half-flipped state Joel called out in §6 is closed: there's no commit in git history where the CLI accepts `--mount` while the loader rejects it, or vice versa.

**Verdict: atomic flip discipline correctly executed.**

### 3. Is `Volumes []VolumeMount` field on RunRequest in the right place?

**Yes — `VolumeMount` is correctly a `dockerdrv` (driver-runtime) concern.** It existed BEFORE M2 (used by `RunWithOptions` for the Caddy manager since the host-Caddy-migration task) and ships with the right shape: `{Source, Target, ReadOnly, IsNamed}`. Adding `Volumes []VolumeMount` to `RunRequest` parallels the existing `Volumes []VolumeMount` in `RunOptions` — the two run paths converge in field shape.

**The deploy/registry boundary is clean:** `internal/registry/types.go` exports `Mount{HostPath, ContainerPath, ReadOnly}` (the persisted on-disk shape). The conversion `registry.Mount` → `dockerdrv.VolumeMount` happens in exactly ONE place (`toVolumeMounts` in `internal/deploy/service.go:422-436`), called from four sites (deploy fresh, deploy save spec, restoreOldContainer, lifecycle absent-branch). The `IsNamed` flag is derived at conversion time from `HostPath`'s leading `/` — exactly Joel's Decision 3 Option B.

**No Docker-isms leak into the registry boundary.** `registry.Mount` knows nothing about `IsNamed`, named-volume regex, or the `-v` argv shape. The `IsNamed()` method on `registry.Mount` is the only "Docker-aware" helper in the registry package, and it's purely a convention-derived predicate (no Docker SDK call, no shell-out).

**Verdict: layering is correct.** Driver knows VolumeMount; registry knows Mount; deploy translates between them. No DDD-shaped abstraction layer needed for one runtime.

### 4. CLI duplicate target → exit 2 (errUsage), not exit 10 (ErrInvalidMount) — addendum Issue 1 fix verified?

**Yes, the dual-sentinel chain is dead.** Reading `internal/cli/deploy_service.go:172-189`:

```go
func parseMountFlags(raw []string) ([]registry.Mount, error) {
    ...
    for _, s := range raw {
        m, err := registry.ParseMountString(s)        // grammar errors only
        if err != nil {
            return nil, fmt.Errorf("--mount %q: %s: %w", s, err.Error(), errUsage)
        }
        out = append(out, m)
    }
    if first, dup, ok := registry.FindDuplicateTarget(out); ok {   // bare helper, no ErrInvalidMount
        return nil, fmt.Errorf("--mount %q: duplicate container_path (also at --mount[%d]): %w",
            out[dup].ContainerPath, first, errUsage)
    }
    return out, nil
}
```

Note: NO call to `registry.ValidateMounts` (which would wrap with `ErrInvalidMount`). The doc comment at lines 165-171 explicitly states the case-ordering footgun rationale — exactly what addendum Issue 1 prescribed.

**The load-bearing test exists** at `internal/cli/deploy_service_test.go:135-138`:
```go
if tc.name == "duplicate_target" {
    assert.False(t, errors.Is(err, registry.ErrInvalidMount),
        "CLI dup-target must NOT chain ErrInvalidMount; see addendum Issue 1")
}
```

This test is the regression lock. If anyone ever adds back `ValidateMounts` to `parseMountFlags`, this test fails immediately. Linus v2 said "non-negotiable" — Kent shipped it correctly.

The `exit_codes_test.go` `cli-mount-dup-wraps-usage` row (Kent §"Decisions I made where the plan was ambiguous" point 4) also locks this independently at the exit-code-mapping layer. Two locks on the same regression vector — not redundant when the regression is "case ordering."

**Verdict: dual-sentinel chain stripped exactly as addendum Issue 1 prescribed.**

### 5. Integration test — does it actually test mount semantics?

**It tries to, but it WILL FAIL on real Docker. This is a real bug.**

Reading `internal/integration/mount_test.go:67-78`:

```go
_, err := driver.Run(runCtx, dockerdrv.RunRequest{
    Name:    mountTestContainer,
    Image:   mountTestImage,           // "alpine:3.19"
    Network: "decloud",
    Restart: "no",
    Volumes: []dockerdrv.VolumeMount{...},
})
require.NoError(t, err, ...)
// then driver.Exec to cat /data/marker.txt
```

`alpine:3.19`'s default `CMD` is `/bin/sh`. With `docker run -d` (detached, no `-i`/`-t`), `/bin/sh` reads from a closed stdin, exits immediately with status 0. Then `driver.Exec` against an exited container fails: "Error response from daemon: Container ... is not running."

**Don's plan §8 explicitly specified `alpine + /bin/sh -c 'cat /data/marker; sleep 60'`** — explicitly with a long-running command. The shipped test omits the sleep AND has no mechanism to inject a `Cmd` because **`RunRequest` has no `Cmd` field** (`internal/dockerdrv/driver.go:31-39` — it's `Name/Image/Network/Env/Restart/Port/Volumes`). The driver `RunWithOptions` (`RunOptions`, line 77-86) ALSO has no `Cmd` field. Only `ExecOptions` (line 90-95) has one, and that's for `docker exec` not `docker run`.

So the test as written cannot work on a real Docker host. Kent reported `go build -tags integration ./...` clean — that's compilation, not execution. Rob reported the same. Nobody actually ran `DECLOUD_INTEGRATION=1 go test -tags integration ./internal/integration/...` against real Docker.

I attempted to run it just now: my dev box doesn't have Docker installed, so it failed at `NetworkEnsure`. The test has never been verified end-to-end.

**This is the same bug class as the M1-era manual-smoke-test pattern that we explicitly tried to invert at M2.** We bundled the integration test BECAUSE we wanted automated verification of the mount feature. Shipping a test that doesn't actually verify the feature negates the bundling argument.

**Options:**

- **Option A (minimal):** swap `alpine:3.19` for an image that idles by default. The Docker-published `nginx:alpine` keeps a foreground process running. Pros: one-line test fix. Cons: 100MB+ image pull instead of ~7MB alpine; CI footprint grows.
- **Option B (proper):** add a `Cmd []string` field to `dockerdrv.RunRequest`, plumbed into `cli_driver.go:Run` after `req.Image`. The test passes `Cmd: []string{"/bin/sh", "-c", "sleep 60"}`. Pros: matches Don's plan §8 exactly; the field is generally useful for integration tests of any image. Cons: production code change for a test-only need (no production deploy uses Cmd-override; the image's Dockerfile carries CMD). m1x-item-11 (Driver.Run/RunWithOptions consolidation) would have to address Cmd too.
- **Option C (test-only escape hatch):** the integration test bypasses `Driver.Run` entirely and shells out to `exec.Command("docker", "run", "-d", "--name", ..., "-v", ..., "alpine:3.19", "/bin/sh", "-c", "sleep 60")`. Pros: zero production change. Cons: the test no longer exercises `Driver.Run`'s argv construction — the very thing m1x-item-6 said the integration test was supposed to verify. Anti-pattern.
- **Option D (defer):** revert the integration test to a stub that only verifies `go test -tags integration` compiles, file the real integration test as a new m1x-backlog item 12, ship M2 with no real-Docker mount verification. Pros: unblocks M2 closeout. Cons: we lose the bundling argument we used to JUSTIFY shipping the integration test in M2.

**My take: Option A.** Use `nginx:alpine` (or `traefik`, or any other officially-published image that idles on a foreground process) as the test image. One-line change. Production remains untouched. Don's spec said "tiny test image" — nginx:alpine is ~22MB vs alpine's ~7MB; both are tiny. The footprint cost is 15MB on a CI runner that already pulls multi-GB toolchains.

Option B is the structurally cleanest answer (Don's plan literally specified `/bin/sh -c 'cat ... ; sleep 60'`, which requires Cmd-override), but it touches `dockerdrv.RunRequest` to support a test-only path, and m1x-item-11 (Driver.Run consolidation) is the right vehicle for that.

**DON: pick Option A or B.** I recommend A. If you pick B, m1x-item-11 should be expanded to include the `Cmd` field design and the M2 integration test gets revisited at that consolidation task.

Either way, the test must be **actually run** against real Docker before M2 closeout, not just compiled. Kent or Rob (or you on the maintainer's box) needs to provide a `DECLOUD_INTEGRATION=1 go test ... PASS` log entry in this task directory.

**Ancillary observation:** the test does use the `Driver.Run` + `Driver.Exec` direct-to-driver pattern Joel specified in §4.8 (no full deploy orchestrator). That part is correctly executed. The deploy-orchestrator path is locked by the unit tests in `internal/deploy/service_test.go` (the three new `Mounts` tests at lines 950-1040, with `expectedVolumes(...)` matching byte-for-byte). The integration test's job is exactly "does real `docker run -v` argv work?" — the right scope.

### 6. m1x-backlog item 6 split into 6/9/10/11 — architecturally honest?

**Yes. Splitting is honest.**

Item 6 was originally "Docker-compose-based smoke integration test for M1 deploy + Caddy ingress" — a triple-feature ask: (a) real-Docker integration, (b) deploy-orchestrator end-to-end, (c) Caddy ingress verification.

What M2 actually shipped:
- (a) ✓ — driver-direct integration test (modulo the alpine-Cmd bug above).
- (b) ✗ — the test bypasses the deploy orchestrator.
- (c) ✗ — the test does not bring up Caddy or curl through it.

So calling item 6 "PARTIALLY DONE at M2" is correct. The split:
- **Item 6** struck through with "PARTIALLY DONE at M2" — keeps one release per the maintenance note at `m1x-backlog.md:115-117`.
- **Item 9** (reloader stderr `%q` revisit) — Joel correctly argued this is orthogonal to mounts. Same file (`reloader.go`) as where the original item 6 mentioned `%q`; same fix shape; correctly split.
- **Item 10** (curl-through-Caddy) — the (c) component above; correctly named as a separate failure-mode-class; correctly split.
- **Item 11** (Driver.Run/RunWithOptions consolidation) — this is GENUINELY new work that would not have been listed if Joel had picked α; it exists because we picked β and want to log the cleanup-debt. Honest record-keeping.

**No manufactured work.** Each new item has a distinct failure-mode-class and a distinct fix shape. Items 6/9/10 sit at three different layers of the system (driver argv / caddy reloader formatting / ingress verification). Item 11 is the β-debt receipt.

**Verdict: split is architecturally honest. None of these are "look thorough" filler.**

### 7. cli-flag-surface-coherence.md live-example reframe — preserves doctrine?

**Yes, preserved.** Reading `_ai/cli-flag-surface-coherence.md:42` after Raymond's update:

> **Historical live example:** `TestDeployService_MountFlagHelpReferencesM2` ... asserted on the substring `"M2"` in `--mount`'s help text from the milestone-resequence task until M2 shipped. The token-not-prose discipline made this the right call at the time ... When M2 shipped at `_tasks/2026-04-28-m2-server-side-mounts/`, the milestone token had no remaining contract surface ... and the test was deleted. **The carve-out remains valid as a pattern for any future milestone-token assertion; the rule is "delete the test when the token disappears from all surfaces," not "rewrite it to assert on the new prose."**

This is exactly Linus v1 Issue 7 / Joel Decision 9 / Don §7's combined position: narrate-as-historical, don't rewrite-to-new-prose, keep the carve-out as a pattern. Doctrine fully preserved.

**Is there a NEW live example from M2 that better illustrates the doctrine?** I went looking. The user-suggested candidate is the dual-sentinel-chain assertion at `deploy_service_test.go:136-138`:

```go
assert.False(t, errors.Is(err, registry.ErrInvalidMount), "CLI dup-target must NOT chain ErrInvalidMount; see addendum Issue 1")
```

**This is NOT a fit for the carve-out.** The carve-out at lines 33-42 is specifically about *help-text* multi-surface contracts (semantic tokens that appear in `--help`, runtime error, `_docs/usage.md`). The `assert.False(errors.Is, ErrInvalidMount)` test asserts on the *runtime error chain* (a different multi-surface contract: CLI error vs loader error, two different exit codes). It belongs in a different doctrine document — probably `_ai/error-wrap-discipline.md` or a new `_ai/single-sentinel-per-chain.md`.

**My recommendation:** leave the cli-flag-surface-coherence.md example as historical. The dual-sentinel-chain rule deserves its own doctrine entry, but that's Ward's territory at the task-finalization step, not Raymond's M2 docs sweep. If Don wants it as a Ward learnings entry, lock the wording during Step 4.

**Verdict: Raymond's reframe is correct. Do not retrofit a different example into this carve-out.**

### 8. Schema-version stability promise — verified?

**Yes.** Reading `internal/registry/types.go`:

```go
const CurrentSchemaVersion = 1   // line 5

type RunSpec struct {            // lines 52-57
    Network string  `toml:"network"`
    Port    int     `toml:"port"`
    Restart string  `toml:"restart"`
    Mounts  []Mount `toml:"mounts"`
}

type Mount struct {              // lines 59-63
    HostPath      string `toml:"host_path"`
    ContainerPath string `toml:"container_path"`
    ReadOnly      bool   `toml:"read_only"`
}
```

The shape is identical to what M1 shipped. M2 added zero TOML fields. An M1-era TOML with empty `mounts = []` (or no `mounts` field at all) loads cleanly in M2 because:

- `cfg.SchemaVersion == 1` matches `CurrentSchemaVersion == 1`.
- `ValidateMounts(empty-or-nil-slice, name, cfgPath)` returns `nil` immediately (verified at `mount.go:52-63` — the `for` loop is a no-op on empty slice; `FindDuplicateTarget` returns `(0, 0, false)` on empty).
- `pelletier/go-toml/v2` strict mode (`DisallowUnknownFields`) doesn't fire because no new fields were added.

**The schema-versioning.md:11 promise holds:** "An M1-era TOML loads cleanly in an M2 binary because the shape is identical, only the values differ." Verified by code-read.

**Verdict: schema-stability promise verified. No accidental backwards-incompatibility.**

### 9. Worst architectural decision in the SHIPPED code?

**The integration test alpine-no-Cmd bug** (Question 5 above). It's not architectural in the "wrong abstraction" sense — it's a verification-mechanism failure. We argued at plan stage that bundling the integration test was the right call BECAUSE it would verify the M2 feature against real Docker. The shipped test cannot do that — `docker run -d alpine:3.19` exits before `docker exec` can read the marker file.

Critically, **Kent and Rob both reported "go build -tags integration clean"** as if that were the verification. Compilation is not execution. The integration test exists to catch bugs that unit tests can't — and the unit tests in `service_test.go`/`lifecycle_test.go`/`mount_test.go` already lock the `Driver.Run` argv shape byte-for-byte via the existing `volumeFlagsFromArgs` helper. The integration test's job is "does real docker accept this argv?" — and we never ran it against real Docker.

If forced to pick a runner-up architectural concern, it's the `IsNamed` derivation from `HostPath` having a small papercut (`HostPath` for named volumes is the volume name, not a host path). Joel acknowledged this in plan §1 Decision 3, picked Option B (derive instead of rename), and Raymond documented it in `_docs/usage.md:150` ("The `host_path` field carries either an absolute host path (bind mount) or a Docker named-volume name"). Acceptable. Not stupid — just a permanent quirk operators have to read once.

**Verdict: integration test bug is the worst thing here. Everything else aged correctly.**

### 10. M2 scope creep?

**No scope creep.** Diff stat:
- Code: 9 production files, ~250 lines
- Tests: 8 test files, ~500 lines
- Docs: 8 files, ~330 lines
- New packages: 1 (`internal/integration`, gated behind build tag)

Things that did NOT ship (despite being adjacent and tempting):
- ❌ Viper plumbing — none. Verified by `grep -r "viper" internal/` → no hits.
- ❌ `/etc/decloud/config.toml` — none. No new config-loading machinery.
- ❌ Secret-files-on-disk reading — none. `internal/registry/store.go:Save` still writes only `env.toml`; no `secrets/<name>/files/` substructure.
- ❌ Client binary — none. `cmd/decloud` is the only main package.
- ❌ Blue/green strategy — `--strategy=blue_green` still rejected with `ErrInvalidStrategy`; container naming still `decloud-<name>` (single-color recreate).
- ❌ tmpfs / `--mount type=...` long-form — none. Only `<src>:<target>[:ro]` short form accepted.
- ❌ SELinux flags (`:z`, `:Z`) — explicitly rejected as "unsupported mode flag" (verified at `mount.go:88`).
- ❌ Mount source pre-existence stat — explicitly NOT done (verified at `mount.go:23-29`, doc comment).
- ❌ Reloader `%q` revisit — punted to m1x-item-9 as planned.
- ❌ Driver.Run/RunWithOptions consolidation — punted to m1x-item-11 as planned.
- ❌ Curl-through-Caddy integration test — punted to m1x-item-10 as planned.

The strategy-block papercut fix (`cfg.Name` → `name` at `store.go:73`) is the only fix-while-fresh that snuck in — and it's exactly what Linus v1 Issue 10 explicitly authorized. Three-character rename in the same hunk as the mount-block rewrite. Not creep.

**Verdict: scope discipline held. Joel's "make the milestone smaller" rule was applied throughout.**

---

## Two minor architectural observations (non-blocking, FYI)

### Observation A — `RunRequest.Cmd` is a real gap, but it's a M-future concern

The fact that `RunRequest` (and `RunOptions`) have no `Cmd` field makes the integration-test fix harder. In production it doesn't matter — every service-deploy image carries CMD via Dockerfile. But for any future test (or operator override use case, or one-shot job at M5), the lack of a `Cmd` field forces test images to either (a) carry a CMD that idles, or (b) bypass the driver entirely.

Logging this as a future concern, not as M2 blocker: when m1x-item-11 (Driver.Run consolidation) is picked up, consider whether the consolidated `RunOptions` should grow `Cmd []string` to enable cleaner integration testing across the codebase. Don't fix it in M2 — that's exactly the scope creep we avoided. But name it now so the future-author knows.

### Observation B — Two locks for the dual-sentinel regression is one more than strictly needed, but I'm fine with it

Kent shipped two regression locks for Issue 1:
1. `TestDeployService_MountFlagInvalidReturnsExitUsageError` `duplicate_target` subtest with `assert.False(errors.Is(err, ErrInvalidMount))`.
2. `TestExitCodeFor_AllSentinels` `cli-mount-dup-wraps-usage` row.

I called the second "non-redundant when the regression vector is case ordering" in v2. After reading the shipped code, I think one of them would have been enough — they catch the same bug from two angles, but the angle is the same vector (CLI dup-target chain producing ErrInvalidMount). The cost of two tests is negligible, and the second one is at the exit-code-mapping layer (a different reader looking at exit_codes.go won't find the deploy_service_test.go assertion). Keep both.

---

## Cross-checks I performed

1. **Atomic commit verification.** `git show ae87320 --name-only | grep _test.go` → empty. Five surfaces all present in Rob's commit. ✓
2. **Sentinel deletion.** `git grep -F ErrMountsNotSupported -- '*.go'` → no production hits. Only historical `_tasks/` mentions. ✓
3. **Schema stability.** Read `internal/registry/types.go` end-to-end. Mount struct unchanged from M1. `CurrentSchemaVersion = 1`. ✓
4. **Issue 1 fix.** Read `internal/cli/deploy_service.go:172-189` (parseMountFlags). Calls `ParseMountString` + `FindDuplicateTarget`, never `ValidateMounts`. ✓
5. **Issue 1 lock.** Read `internal/cli/deploy_service_test.go:135-138`. The `assert.False(errors.Is(err, registry.ErrInvalidMount))` is present. ✓
6. **Integration test execution.** Attempted `DECLOUD_INTEGRATION=1 go test -tags integration ./internal/integration/...` — failed at `NetworkEnsure` (Docker not installed on dev box). Kent and Rob report only `go build -tags integration ./...` clean — i.e., compilation, not execution.
7. **Mock regen no-op.** `go generate ./...` reported by Rob as zero-diff. β payoff confirmed.
8. **`m1x-backlog.md` items 9/10/11.** Read the new entries. Each has a distinct failure-mode-class and a distinct fix shape. No manufactured work.
9. **`cli-flag-surface-coherence.md:42`.** Historical-narration reframe correctly preserves doctrine. No NEW live example was forced in.
10. **Scope creep audit.** `grep -r "viper" internal/` → no hits. No client binary. No new config-loading. ✓

---

## DECISION: APPROVED WITH MINOR FIXES

The architecture is sound. The atomic flip is real. The β decision aged perfectly. The dual-sentinel-chain fix from addendum Issue 1 is correctly implemented in code. Schema stability is verified. No scope creep. Raymond's docs sweep is honest and complete. The m1x-backlog split is architecturally honest.

**Required fix before closeout (Don pick the option):**

1. **Integration test alpine-no-Cmd bug** (Question 5). Pick Option A (`nginx:alpine` or other idling image), Option B (`Cmd []string` on RunRequest), or Option D (defer entirely to m1x-item-12). I recommend A. Once the test fix lands, **the test must be ACTUALLY RUN** against real Docker on the maintainer's box, with a `DECLOUD_INTEGRATION=1 go test -tags integration ./internal/integration/... PASS` log entry committed to this task directory.

**Optional follow-ups (no action needed unless Don wants):**

- Observation A: log a future-concern note about `RunRequest.Cmd` gap for whoever picks up m1x-item-11.
- Observation B: nothing — keep both regression locks.

If Don picks Option A (or B), apply the fix in a new commit, run the integration test against real Docker, and proceed to PLAN re-entry (Don/Joel/Linus closeout). If Option D, expand m1x-item-12 with the integration-test scope and proceed to closeout with the integration test reverted.

---

## Files reviewed (absolute paths)

Planning + reports:
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/001-user-request.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/002-don-plan.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/003-joel-tech-plan.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/004-linus-plan-review.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/005-joel-tech-plan-addendum.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/006-linus-plan-review-v2.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/007-kent-tests.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/008-rob-impl.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/009-raymond-docs.md`

Shipped code (read end-to-end where load-bearing):
- `/Users/fenster/dev/decloud/internal/registry/mount.go`
- `/Users/fenster/dev/decloud/internal/registry/errors.go`
- `/Users/fenster/dev/decloud/internal/registry/store.go`
- `/Users/fenster/dev/decloud/internal/registry/types.go`
- `/Users/fenster/dev/decloud/internal/cli/deploy_service.go`
- `/Users/fenster/dev/decloud/internal/cli/exit_codes.go`
- `/Users/fenster/dev/decloud/internal/dockerdrv/cli_driver.go`
- `/Users/fenster/dev/decloud/internal/dockerdrv/driver.go`
- `/Users/fenster/dev/decloud/internal/deploy/service.go` (deploy-mount sites + toVolumeMounts)
- `/Users/fenster/dev/decloud/internal/deploy/lifecycle.go`
- `/Users/fenster/dev/decloud/internal/integration/mount_test.go`
- `/Users/fenster/dev/decloud/internal/integration/doc.go`

Shipped tests (spot-checked for the load-bearing assertions):
- `/Users/fenster/dev/decloud/internal/cli/deploy_service_test.go` (the dual-sentinel-chain lock)
- `/Users/fenster/dev/decloud/internal/cli/exit_codes_test.go` (the second lock)
- `/Users/fenster/dev/decloud/internal/registry/mount_test.go`
- `/Users/fenster/dev/decloud/internal/registry/store_test.go`
- `/Users/fenster/dev/decloud/internal/deploy/service_test.go` (the three new Mounts tests)
- `/Users/fenster/dev/decloud/internal/deploy/lifecycle_test.go` (StartAbsentBranch)
- `/Users/fenster/dev/decloud/internal/dockerdrv/cli_driver_test.go`

Doctrine docs (verified updates):
- `/Users/fenster/dev/decloud/_ai/decisions/m1-scope.md` (line 16, line 32 — env-file-hardening dead)
- `/Users/fenster/dev/decloud/_ai/decisions/schema-versioning.md` (line 16 past-tense)
- `/Users/fenster/dev/decloud/_ai/decisions/secrets-split.md` (line 24 sentinel swap)
- `/Users/fenster/dev/decloud/_ai/decisions/m1-test-strategy.md` (line 7 past-tense)
- `/Users/fenster/dev/decloud/_ai/cli-flag-surface-coherence.md` (line 42 historical reframe)
- `/Users/fenster/dev/decloud/_ai/m1x-backlog.md` (item 6 strikethrough + items 9/10/11)
- `/Users/fenster/dev/decloud/_ai/MEMORY.md` (line 9 past-tense + new task entry)
- `/Users/fenster/dev/decloud/_docs/usage.md` (full row rewrite + no-stat paragraph + examples)
