# 007 — Kent's tests (RED commit)

Branch: `feat/m2-server-side-mounts`. Resumed after stream timeout; re-verified the working tree against `003-joel-tech-plan.md` §3 (file diffs), §4 (test surface), §5 (deletion list), §6 (atomic-commit plan), the addendum `005-joel-tech-plan-addendum.md` (Issues 1, 5, 10), and Linus's three v2 notes in `006-linus-plan-review-v2.md`. Per the plan §6, this commit is **RED on purpose**: the new tests fail because the production code is stub-only. Rob's next commit is the GREEN.

## Files in this commit

### New test files

- `internal/registry/mount_test.go` — exercises `ValidateMount`, `ValidateMounts`, `ParseMountString`, `Mount.IsNamed`, `FindDuplicateTarget`. Six top-level tests, all table-driven where the matrix earns its keep:
  - `TestValidateMount_Table` — 10 rows: 4 valid (bind/named × rw/ro), 6 invalid (empty source, empty target, relative target, named-volume invalid chars, named starting with dash, named-too-short to flush the 2+ char regex floor in §8.1).
  - `TestValidateMounts_DuplicateContainerPath` — two mounts on the same container path; asserts `errors.Is(err, ErrInvalidMount)`, error names `mount[0]` and `mount[1]`.
  - `TestValidateMounts_FirstInvalidStops` — three mounts with mount[1] invalid; asserts the error names `mount[1]` and NOT `mount[2]`.
  - `TestValidateMounts_EmptyAndNilSliceAreNoOp` — both nil and `[]Mount{}` return nil from `ValidateMounts`.
  - `TestValidateMounts_LoaderErrorNamesPathAndService` — locks the `service "foo"` and `cfgPath` substrings on the loader-side wrap (the operator-debug context per Decision 7).
  - `TestParseMountString_Table` — 9 rows: 4 valid, 5 invalid (empty, 1 component, 4 components, explicit `:rw`, unknown mode `:zz`).
  - `TestMount_IsNamed` — three rows: `/host` is bind (false), `vol` is named (true), empty-source is false (defensive per §8.7).
  - `TestFindDuplicateTarget_Table` — six rows: empty slice, single mount, two distinct, two same, collision at indices 0 & 2, "first collision wins" four-mount case. **Linus v2 §"Minor non-blocking notes" called out the empty-slice row explicitly — it is present.**

- `internal/integration/mount_test.go` (new directory) — `TestIntegration_MountBindRoundTrip`. Build-tagged `//go:build integration`; skips unless `DECLOUD_INTEGRATION=1`. Calls the real `dockerdrv.NewCLIDriver()` directly (per Joel §4.8 revised approach — driver-only, not deploy-orchestrator end-to-end), pulls `alpine:3.19`, writes `marker.txt` to a `t.TempDir()` host dir, runs the container with that dir bind-mounted ro at `/data`, and `docker exec cat /data/marker.txt` to assert the bytes round-trip.
- `internal/integration/doc.go` — package-level doc and build tag explaining the env-var gate.

### Modified test files

- `internal/cli/deploy_service_test.go` — replaces `TestDeployService_MountFlagReturnsErrMountsNotSupported` (M1 rejection contract; deleted per §5 item 5) with `TestDeployService_MountFlagAcceptsValidMounts` (locks the three-mount happy path with bind-rw, bind-ro, named-ro). Replaces `TestDeployService_MountFlagHelpReferencesM2` (semantic-token contract; deleted per §5 item 6 and Linus v1 §6.3 — the token is dead post-M2-ship) with `TestDeployService_MountFlagPathWithCommaWorks` (locks the §8.9 `StringArrayVar` choice — paths with commas survive Cobra parsing). Adds `TestDeployService_MountFlagInvalidReturnsExitUsageError` (table-driven, 5 subtests). **The `duplicate_target` subtest carries the load-bearing `assert.False(t, errors.Is(err, registry.ErrInvalidMount), ...)` from addendum Issue 1 / Linus v2 §"Hidden bugs: none, but two small notes" point 2 — this is the only test that locks the dual-sentinel-chain regression Linus called out, and it must not be deleted later as redundant.**

- `internal/cli/exit_codes_test.go` — deletes the `{"mounts", registry.ErrMountsNotSupported, ExitConfigError}` row (§5 item 8); adds `{"invalid-mount", registry.ErrInvalidMount, ExitConfigError}` for the loader-side exit-10 path; adds `{"cli-mount-dup-wraps-usage", fmt.Errorf("--mount \"/x\": duplicate container_path: %w", errUsage), ExitUsageError}` to lock that any CLI parse error wrapping `errUsage` — including a synthesised duplicate-target shape — lands on exit 2 even after `exit_codes.go` cases reorder.

- `internal/registry/store_test.go` — replaces `TestStore_LoadRejectsNonEmptyMounts` (M1 contract; deleted per §5 item 7) with `TestStore_LoadAcceptsValidMounts` (loads a TOML containing one bind mount and one named mount; round-trips both `host_path`, `container_path`, `read_only`). Adds `TestStore_LoadRejectsInvalidMounts` (table-driven, 3 subtests: relative_container_path, named_volume_invalid_chars, duplicate_container_path; each asserts `errors.Is(err, registry.ErrInvalidMount)`, error names the on-disk path and a `mount[N]` index per the regex `mount\[\d+\]`). Existing `TestStore_LoadAcceptsEmptyMountsArray` is kept unchanged as the empty-slice round-trip lock.

- `internal/dockerdrv/cli_driver_test.go` — adds `TestCLIDriver_RunPassesVolumeFlags` (one bind ro + one named rw, asserts `volumeFlagsFromArgs(...)` returns `["/host:/dst:ro", "vol:/dst2"]`). Mirrors `TestCLIDriver_RunWithOptionsBindReadOnly` (line 405) for the new `Run`-with-volumes path.

- `internal/deploy/service_test.go` — adds three tests plus two helpers:
  - Helpers: `newRequestWithMounts` (sugar over the existing `newRequest`) and `expectedVolumes` (the canonical `[]Mount` → `[]VolumeMount` conversion the production helper `toVolumeMounts` is contractually obliged to match, with `IsNamed` derived from `HostPath`).
  - `TestDeploy_DeployWithMountsPassesVolumesToDriver` — fresh deploy. Captures the `Driver.Run` `RunRequest` and asserts `Volumes` equals `expectedVolumes(req.Mounts)`. Locks the `IsNamed` derivation: `seenVolumes[0].IsNamed == false` for `/host`, `seenVolumes[1].IsNamed == true` for `vol`.
  - `TestDeploy_DeployWithMountsSavesMountsToRegistry` — captures `Store.Save` and asserts `svc.Config.Run.Mounts == req.Mounts`. Locks the round-trip-shape contract from §3.9(e).
  - `TestDeploy_RestoreOldContainerPassesVolumesToDriver` — forces a primary `Driver.Run` failure so the recreate-strategy rollback runs; asserts the second `Driver.Run` (rollback) carries `expectedVolumes(prev.Config.Run.Mounts)`. This is the test for §3.9(d).

- `internal/deploy/lifecycle_test.go` — adds `TestLifecycle_StartAbsentBranchPassesVolumesToDriver` for the `default:` arm at `lifecycle.go:67-78`: `Inspect` returns `State: "absent"`, `Driver.Run` is captured and asserted to carry `Volumes` derived from `prev.Config.Run.Mounts` with the `IsNamed` flag correctly set per source.

### Production-code stubs in this commit (Rob: replace with real bodies)

Two production files have stubs ONLY (no real behaviour). The list, exhaustive:

- `internal/registry/errors.go` — adds `ErrInvalidMount = errors.New("registry: invalid mount")` (§3.3). `ErrMountsNotSupported` is **NOT yet deleted** in this RED commit; Rob's GREEN commit deletes it together with the loader/CLI rejection blocks (per §6 atomic-commit list items 1, 4, and the deletion list §5).
- `internal/dockerdrv/driver.go` — adds `Volumes []VolumeMount` field to `RunRequest` (§3.7). No `cli_driver.go` changes yet; the for-loop that emits `-v` flags is Rob's responsibility.
- `internal/deploy/service.go` — adds `Mounts []registry.Mount` field to `Request` (§3.9(a)). The `toVolumeMounts` helper, the three `Volumes:` populations (sites 1, 2, 3 per §3.9(c)(d)(e)), and the lifecycle `default:` arm change at §3.10 are all Rob's.
- `internal/registry/mount.go` — NEW file containing **stubs only** for `IsNamed`, `ValidateMount`, `ValidateMounts`, `ParseMountString`, `FindDuplicateTarget`. Each stub returns the zero value; each function carries the documented contract per addendum §3.2 / Issue 1 / Linus v2 §"FindDuplicateTarget exposed surface". The `FindDuplicateTarget` doc comment explicitly states `"(firstIdx, dupIdx, true)" / "(0, 0, false)"` per Linus v2 (load-bearing per Linus v2 §"FindDuplicateTarget exposed surface" point 2).

The CLI's `parseMountFlags` helper (§3.5(d)) and the `--mount` help-text flip (§3.5(a)) are NOT in this RED commit — they are Rob's surface flip that lands together with the loader rejection delete in the atomic GREEN commit per §6.

## Build / test state at end of RED commit

```
$ go build ./...
(no output — build clean)

$ go vet ./...
(no output — vet clean)

$ gofmt -l .
(no output — formatting clean)

$ go generate ./...
(no output — mock-regen no-op, the Option-β payoff per §3.11)
```

```
$ go test ./...
ok    github.com/alexander-fenster/decloud/internal/caddy
ok    github.com/alexander-fenster/decloud/internal/config
ok    github.com/alexander-fenster/decloud/internal/envcap
ok    github.com/alexander-fenster/decloud/internal/ids
ok    github.com/alexander-fenster/decloud/internal/logging
FAIL  github.com/alexander-fenster/decloud/internal/cli
FAIL  github.com/alexander-fenster/decloud/internal/deploy
FAIL  github.com/alexander-fenster/decloud/internal/dockerdrv
FAIL  github.com/alexander-fenster/decloud/internal/registry
```

Test count: 222 PASS, 18 FAIL (top-level functions). Every failing top-level is one of the 17 new M2 tests plus `TestExitCodeFor_AllSentinels` (which fails only on its new `invalid-mount` subtest — the existing 22 subtests plus the new `cli-mount-dup-wraps-usage` row pass). No M1 regression: every pre-existing test that survived the deletion list still passes.

The expected failure modes each correspond to a stub returning the zero value:
- `ValidateMount` returns `nil` for invalid inputs → table rows wanting an error fail.
- `ParseMountString` returns `Mount{}, nil` always → wantErr rows fail; want-Mount rows fail because zero-value doesn't match the expected non-zero Mount.
- `FindDuplicateTarget` returns `0, 0, false` always → `wantOk=true` rows fail.
- `IsNamed` returns `false` always → `volume_name_is_named` row fails.
- `parseMountFlags` doesn't exist yet → CLI tests fail because the rejection block at `deploy_service.go:71-73` still wraps `ErrMountsNotSupported` (M1 behaviour), so the CLI tests that exercise valid mounts get exit 10 instead of accepting; the duplicate-target test gets exit 10 instead of exit 2.
- `Volumes` field is on `RunRequest` but `cli_driver.go` doesn't emit `-v` flags yet → the dockerdrv test sees zero `-v` flags in argv.
- `Mounts` field is on `Request` but `service.go`/`lifecycle.go` don't populate `RunRequest.Volumes` yet → the deploy/lifecycle tests see nil Volumes.
- `ErrInvalidMount` is defined but not in `exit_codes.go`'s case list yet → the `invalid-mount` exit-codes subtest fails. Loader still wraps with `ErrMountsNotSupported`, so the store tests asserting `errors.Is(err, ErrInvalidMount)` see the M1 chain and fail.

Each of these is a green-commit fix in Rob's diff. None reflect a test-bug or an unrelated regression.

## Decisions I made where the plan was ambiguous

1. **Location of `IsNamed`.** Joel §3.1 says "After line 63 of `internal/registry/types.go`, add ... `func (m Mount) IsNamed() bool`." The addendum's revised §3.2 `mount.go` body does not mention `IsNamed`. The stub is in `mount.go` to keep all M2-introduced helpers in one new file (one diff hunk for Rob and one mental model for the next reader). Rob can leave it in `mount.go` or move it to `types.go`; both files share the same package and the test surface (`registry_test`) exercises it identically. **Recommendation: leave it in `mount.go`** — that keeps the M2 surface co-located.

2. **`TestDeployService_MountFlagEmptyIsValid` omitted.** §4.3 listed three "ADD" tests including this one. The implicit empty-mount happy path is already covered by the unchanged `TestDeployService_BuildsExpectedRequest` (line 46) which runs `deploy service ... /srv/foo` without any `--mount` and (post-Rob) will pass exactly when `req.Mounts == nil`. Adding a third dedicated test would duplicate that assertion shape with no extra coverage. **If a reviewer wants the explicit nil-mounts test, it is a 4-line addition I can make on request.**

3. **Strategy-block papercut at `store.go:73` (addendum Issue 10).** Per the addendum's locked Option β at line 219, no new test. The fix-while-fresh rename is correct on the code level; the change-detector character of an "asserts the substring of an error message" test is not earned by the production payoff (a hand-edited TOML missing `name = ...`).

4. **`TestExitCodeFor_AllSentinels` `cli-mount-dup-wraps-usage` row.** Joel's §4.4 wrote "the existing `wrapped-usage` entry already covers wrapped `errUsage` for the CLI parse path. No additional test needed." I added one row anyway because Linus v2 §"Hidden bugs: none, but two small notes" point 2 declares the duplicate-target wrapping non-negotiable, and locking it at the *exit-code-mapping* layer (in addition to the deploy_service_test layer) catches a regression from a different angle: if someone reorders `exit_codes.go` cases or accidentally adds an `errors.Is(err, ErrInvalidMount)` clause that fires before the `errUsage` clause, the new row catches it independently of the CLI-parse-path tests. Two locks are not redundant when the regression vector is "case ordering."

5. **`SchemaVersion` lock test omitted (§8.12).** Joel concluded "don't add the test" at line 1015. Confirmed — the existing `TestStore_RoundTripConfigAndSecrets` already implicitly locks `schema_version = 1` round-trip. Adding a dedicated test would duplicate that assertion.

## Critical-checks audit (against the resume-prompt requirements)

- ✅ `internal/cli/deploy_service_test.go` `MountFlagInvalidReturnsExitUsageError` → `duplicate_target` subtest asserts `assert.False(t, errors.Is(err, registry.ErrInvalidMount), ...)`. Lines 137-140 of the post-edit file.
- ✅ `internal/registry/mount_test.go` `TestRegistry_FindDuplicateTarget` (named `TestFindDuplicateTarget_Table`) includes `[]Mount{}` empty-slice row at line 141.
- ✅ `internal/registry/mount.go` `FindDuplicateTarget` doc comment states `(firstIdx, dupIdx, true) on duplicate, (0, 0, false) otherwise`. Lines 52-58 of the new file.
- ✅ `MountFlagHelpReferencesM2` deleted (semantic-token contract is dead post-M2-ship per Linus v1 §6.3).
- ✅ `MountFlagReturnsErrMountsNotSupported` deleted.
- ✅ `LoadRejectsNonEmptyMounts` deleted from `internal/registry/store_test.go`.
- ✅ Integration test `internal/integration/mount_test.go` is build-tagged `//go:build integration` and skips unless `DECLOUD_INTEGRATION=1`.

## Notes for Rob (the GREEN commit)

When you flip the production surfaces:

1. **Atomicity.** The §6 atomic-commit file list (10 files) MUST land in one commit. The half-flipped state is not catchable by the unit-test suite (all packages mock their boundaries); a multi-commit landing means the suite is green at every commit individually but the cumulative semantic state at any cherry-pick is a CLI-writes-TOML-the-loader-rejects bug.

2. **Stubs to replace, in order of how they read.** All in `internal/registry/mount.go`:
   - `IsNamed` → real body per addendum §3.2 (or §3.1 of original tech plan if you prefer types.go).
   - `ValidateMount` → real body per addendum §3.2 (the original tech plan's body, unchanged by addendum).
   - `ValidateMounts` → addendum §"Updated `internal/registry/mount.go`" body (NOT the original §3.2 body — the addendum factors `findDuplicateTarget` out).
   - `ParseMountString` → original §3.2 body. Unchanged by addendum.
   - `FindDuplicateTarget` → addendum §"Updated `internal/registry/mount.go`" body. Exported (capital F) per addendum line 100 because the CLI (`internal/cli`) is a different package.
   - The package-level `volumeNameRE` (regexp.MustCompile) is NOT in the stubs — add it per original §3.2.
   - The package-level `var ErrInvalidMount = errors.New(...)` is in `errors.go` only — do NOT re-declare in `mount.go` per original §3.3 ("drop the `var ErrInvalidMount = errors.New(...)` line from `mount.go`").

3. **Production-code lines you still need to add (not stubbed):**
   - `internal/cli/deploy_service.go` — `parseMountFlags` helper at the bottom (addendum §"Updated `internal/cli/deploy_service.go` `parseMountFlags` helper"), the help-text flip at line 61 (with `StringArrayVar` not `StringSliceVar` per §8.9), the parse-and-validate replacement of lines 71-73, and the `Mounts: mounts` field in the `deploy.Request` literal at lines 96-106.
   - `internal/cli/exit_codes.go` — sentinel swap per §3.6 (delete `ErrMountsNotSupported`, add `ErrInvalidMount`).
   - `internal/registry/store.go` — replace `len(cfg.Run.Mounts) > 0` rejection block at lines 68-71 with `ValidateMounts(cfg.Run.Mounts, name, cfgPath)` call (§3.4); same hunk also flips `cfg.Name` → `name` at line 73 inside the strategy-rejection block (addendum Issue 10).
   - `internal/registry/errors.go` — delete `ErrMountsNotSupported` (it's still here from the RED commit because the M1 tests referenced it; both deletion sites land together in your GREEN commit).
   - `internal/dockerdrv/cli_driver.go` — the `for _, v := range req.Volumes` loop in `Run` per §3.8.
   - `internal/deploy/service.go` — `toVolumeMounts` helper, three `Volumes: toVolumeMounts(...)` populations per §3.9(b)(c)(d)(e).
   - `internal/deploy/lifecycle.go` — `Volumes: toVolumeMounts(prev.Config.Run.Mounts)` in the `default:` arm at lines 67-78 per §3.10.

4. **Mock regen is a no-op.** I ran `go generate ./...` after my edits and `git status` showed zero new diffs. The Option-β payoff held: `RunRequest`/`Request` are structs, the field additions don't change interface signatures, no mock file needs to regenerate.

5. **Argv shape lock.** The `volumeFlagsFromArgs` helper at `internal/dockerdrv/cli_driver_test.go` is the existing extractor used by `TestCLIDriver_RunWithOptionsBindReadOnly` (line 405). The new `TestCLIDriver_RunPassesVolumeFlags` consumes the same helper, so the argv format `<source>:<target>[:ro]` is locked twice (RunWithOptions test + Run test). Don't change `formatVolume`.

## Files reviewed / spot-checked while writing this report (absolute paths)

- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/003-joel-tech-plan.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/005-joel-tech-plan-addendum.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/006-linus-plan-review-v2.md`
- `/Users/fenster/dev/decloud/internal/registry/mount.go`
- `/Users/fenster/dev/decloud/internal/registry/mount_test.go`
- `/Users/fenster/dev/decloud/internal/registry/store_test.go`
- `/Users/fenster/dev/decloud/internal/registry/errors.go`
- `/Users/fenster/dev/decloud/internal/cli/deploy_service_test.go`
- `/Users/fenster/dev/decloud/internal/cli/exit_codes_test.go`
- `/Users/fenster/dev/decloud/internal/deploy/service.go`
- `/Users/fenster/dev/decloud/internal/deploy/service_test.go`
- `/Users/fenster/dev/decloud/internal/deploy/lifecycle_test.go`
- `/Users/fenster/dev/decloud/internal/dockerdrv/driver.go`
- `/Users/fenster/dev/decloud/internal/dockerdrv/cli_driver_test.go`
- `/Users/fenster/dev/decloud/internal/integration/doc.go`
- `/Users/fenster/dev/decloud/internal/integration/mount_test.go`
