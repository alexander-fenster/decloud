# 010 — Kevlin's low-level review (M2 server-side mounts)

Branch: `feat/m2-server-side-mounts`. Commits under review: `432e3e8` (Kent RED), `ae87320` (Rob GREEN), `aee12f5` (Raymond docs). All planning artefacts read end-to-end (`001`–`009`). Working tree, plus the user/linter nudges to `_ai/decisions/m1-scope.md:16` and `:32`, audited.

## TL;DR

The implementation matches the plan + addendum + v2 review almost verbatim. Code is clean, tested, and self-documenting; the dual-sentinel-chain fix from addendum Issue 1 is correctly implemented and the regression test catches it; Raymond's docs trace to source with no hallucinations I could detect. `gofmt`, `go vet`, `go build`, `go build -tags integration`, `go test ./...` all green. The integration test is build-tagged and idempotent on cleanup.

Two micro-papercuts worth recording (neither blocks ship):

1. `Mount.HostPath` has no doc-comment in `internal/registry/types.go` — Joel §3.1 / Linus Issue 3 / addendum implied one would land *on the field* to flag the named-volume aliasing. The convention IS documented on `Mount.IsNamed()` in `mount.go`, and a paragraph lives in `_docs/usage.md:150`, but the struct field itself is silent. Optional fix.
2. `usage.md` retains the "M1 CLI" framing in §1's intro line at the top of the file (`Operator-facing reference for the Decloud M1 CLI.`). Now that M2 has shipped a real new flag end-to-end, this is a tense slip. Optional fix.

Neither is in-scope for "blocking M2." Decision: **APPROVED WITH MINOR FIXES (both optional).**

---

## 1. Code correctness — does Rob's diff match Joel's plan + addendum + Linus v2?

Trace done file-by-file against `003-joel-tech-plan.md` §3 / `005-joel-tech-plan-addendum.md` Issues 1, 5, 10 / `006-linus-plan-review-v2.md`.

### `internal/registry/mount.go` (NEW)

Matches §3.2 (modulo the addendum's restructuring for Issue 1).

- `volumeNameRE` at file-top, regex `^[a-zA-Z0-9][a-zA-Z0-9_.-]+$` — exactly Docker's `volume.IsValidName`. Two-char minimum noted in plan §8.1.
- `IsNamed` lives on the struct here in `mount.go`, not `types.go` — this is Kent's "leave it co-located" call (Kent §"Decisions" point 1) and Rob honored it. Sound: every M2 thing lives in one file, the convention is one doc-comment away from the function that derives it.
- `ValidateMount` is grammar-only — no `os.Stat` (Joel Decision 1; the doc-comment explicitly says so). Order of checks: empty source → empty target → relative target → named-volume regex. Each yields a bare error; the *caller* wraps with `ErrInvalidMount` or `errUsage`.
- `ValidateMounts` iterates `ValidateMount` first (per-mount), then calls `FindDuplicateTarget` (cross-mount). Both paths wrap with `ErrInvalidMount` for the loader surface.
- `ParseMountString` accepts only `<src>:<target>[:ro]`. Rejects empty raw, wrong component count (1 or 4+), and any third-component value other than `"ro"`. The `default` case catches both `"rw"` and `"zz"` with the same wording — that's a small simplification from Joel's plan (which had a separate `case "rw":` arm) but the operator-facing string is unchanged ("unsupported mode flag %q ..."), and the test surface (`mode_rw_explicit`, `mode_unknown` subtests) still pass because both rows assert on the same `unsupported mode flag %q` substring. Cleaner than the original plan; I prefer this.
- `FindDuplicateTarget` returns `(firstIdx, dupIdx, true)` on collision, `(0, 0, false)` otherwise. Doc-comment states the contract verbatim (the load-bearing point Linus v2 marked as non-negotiable). Empty slice path returns `(0, 0, false)` — Linus v2's "add the empty-slice row" check is honored both in the helper and in `TestFindDuplicateTarget_Table`'s `empty_slice` row.

No off-by-ones, no missing wraps, no race conditions (synchronous straight-line code).

### `internal/registry/errors.go`

`ErrMountsNotSupported` deleted; `ErrInvalidMount` added. The `var ()` block was re-aligned by `gofmt`. No other consumers; no dangling references in production (verified by `grep -rn ErrMountsNotSupported --include='*.go' .` returning empty).

### `internal/registry/store.go`

Lines 68 (mount block) and 73 (strategy block) both use the function parameter `name`, not `cfg.Name`. Addendum Issue 10 Option A correctly applied; the strategy block is a one-token fix bundled in the same hunk per Linus v2 §"Issue 10 — Implementation shape". `cfg.Name = name` assignment at line 75 unmoved (correct — moving it earlier would change downstream semantics).

### `internal/cli/deploy_service.go`

- Line 61: `StringArrayVar` (NOT `StringSliceVar`) — Joel §8.9's load-bearing fix. Test `TestDeployService_MountFlagPathWithCommaWorks` locks it.
- Help text matches Joel Decision 6 verbatim.
- Lines 71–75: rejection block deleted, replaced with `parseMountFlags(f.Mounts)` call. The order is preserved (mount first, then strategy, then port) per Joel §3.5 inline correction.
- Line 105: `Mounts: mounts` populates the new `deploy.Request` field.
- `parseMountFlags` (lines 165–189): per Issue 1 addendum body. Iterates `ParseMountString`, then calls `FindDuplicateTarget` (NOT `ValidateMounts`). Both error paths wrap with `errUsage` only. The doc-comment cites the addendum and warns future readers off `ValidateMounts` here — exactly the "comment naming the failure mode" Linus required (with the structural fix actually in place, the comment is reinforcing not load-bearing).

### `internal/cli/exit_codes.go`

Sentinel swap clean (line 41: `ErrInvalidMount` replaces `ErrMountsNotSupported`). Case ordering: `errUsage` at line 39 comes BEFORE `ErrInvalidMount` at line 41 — but per Issue 1 fix the chains are now disjoint, so reordering is safe (and the new `cli-mount-dup-wraps-usage` exit-code test row catches a regression of either kind).

### `internal/dockerdrv/driver.go` and `cli_driver.go`

- `RunRequest` gains `Volumes []VolumeMount` field with the inline comment "emitted in declared order, one -v per entry" (matches `RunOptions.Volumes`'s own comment for symmetry).
- `cliDriver.Run` (lines 61–63): `for _, v := range req.Volumes { args = append(args, "-v", formatVolume(v)) }` — exactly the loop in `RunWithOptions`. Reuses `formatVolume` (no copy-paste). Argv order in Run: `run -d --name --network --restart [--env...] [-v...] --label <image>`. No mock regen needed (Option-β payoff).

### `internal/deploy/service.go` and `lifecycle.go`

- `Request` gains `Mounts []registry.Mount`.
- `toVolumeMounts` helper (lines 422–436) at the bottom of `service.go`. Returns nil for nil/empty slice (matches Joel §3.9(b)).
- Three call sites populate `runReq.Volumes`:
  1. Fresh deploy (line 251).
  2. `restoreOldContainer` (line 383).
  3. `RunSpec.Mounts` registry-save (line 320 — `Mounts: req.Mounts`, replacing the M1 `[]registry.Mount{}`).
- `lifecycle.go:74`: `Volumes: toVolumeMounts(prev.Config.Run.Mounts)` in the `default:` arm of `Start`.

All four sites pass `IsNamed` correctly via `m.IsNamed()`.

### Verdict

**No bugs found in production code.** Every site Joel and Don named is touched; every revision Linus called for is incorporated; the addendum-driven Issue 1 fix is structurally honest (each error chain carries exactly one sentinel).

---

## 2. Test quality — load-bearing or change-detector?

### Strengths

- `TestValidateMount_Table` covers all 10 grammar branches (4 valid, 6 invalid, including the named-volume regex two-char floor).
- `TestParseMountString_Table` covers the 9 grammar permutations including `"empty"`, single-component, four-component, explicit `:rw`, and unknown `:zz`.
- `TestFindDuplicateTarget_Table` includes the empty-slice row Linus v2 demanded, plus a "first collision wins" four-mount case that exercises map seed ordering.
- `TestValidateMounts_FirstInvalidStops` proves that the iteration short-circuits on the first invalid mount (no leak past `mount[1]`).
- `TestValidateMounts_LoaderErrorNamesPathAndService` locks the `service "foo"` and `cfgPath` substrings on the loader-side wrap — exactly the operator-debug context Joel decision 7 named.
- `TestDeployService_MountFlagInvalidReturnsExitUsageError` `duplicate_target` subtest has the **load-bearing** `assert.False(t, errors.Is(err, registry.ErrInvalidMount))` Linus v2 marked non-negotiable. Without this assertion, a future regression that re-routes `parseMountFlags` through `ValidateMounts` would silently flip exit code 2 → 10 and the test would still pass on the `errors.Is(err, errUsage)` line. With it, the regression breaks loudly.
- `TestDeployService_MountFlagPathWithCommaWorks` locks `StringArrayVar` over `StringSliceVar`. Cobra/pflag-version regression is caught.
- `TestStore_LoadAcceptsValidMounts` is a true round-trip: TOML on disk → `Load` → struct equality. Locks the `host_path`/`container_path`/`read_only` TOML tags AND the bind/named distinction in one fixture.
- `TestStore_LoadRejectsInvalidMounts` table covers `relative_container_path`, `named_volume_invalid_chars`, `duplicate_container_path`. Each asserts `errors.Is(err, ErrInvalidMount)` plus the path substring plus the `mount[N]` regex. No change-detector character.
- `TestCLIDriver_RunPassesVolumeFlags` consumes the same `volumeFlagsFromArgs` extractor as the existing `TestCLIDriver_RunWithOptionsBindReadOnly`. Argv shape locked once, reused twice.
- `TestExitCodeFor_AllSentinels` has the new `invalid-mount` row AND the `cli-mount-dup-wraps-usage` row. Two locks at two angles.

### Edge cases I checked were covered

- Path with comma (`/path/with,comma:/data`) — covered.
- `:ro` and `:rw` disambiguation — covered (rw rejected with explicit message).
- Named-vs-bind disambiguation — covered (bind starts with `/`, named matches the regex; `TestMount_IsNamed` covers all three branches: bind, named, empty).
- Duplicate target across two `--mount` flags — covered (CLI-side and loader-side, with disjoint error chains verified).
- Empty/nil mount slice — covered (`TestValidateMounts_EmptyAndNilSliceAreNoOp`).
- Loader error names path + index — covered.
- IsNamed on empty source — covered (`empty_source_is_not_named` row).

### Edge cases NOT covered, and whether that matters

- **Bind source with colon in path** (e.g. `/path:with:colons:/data`). Joel §"Issue 6" said the operator gets a "got 4 components" error — covered transitively by the `four_components` row in `ParseMountString_Table` since the failure mode is identical. **No additional test needed.**
- **`Mount{HostPath: "/", ...}` (binding `/`)**. `IsNamed` returns false (correct), `ValidateMount` accepts it (no slash-only-source rejection). Joel §8.13 explicitly punts this. **No additional test needed.**
- **Volume name with capital letter** (`MyVol:/x`). Validates per regex. Round-trips through TOML. Not tested explicitly — but `valid_named_rw` covers a 6-char name; capitalisation is a regex character-class question and the regex test (`x` 1-char rejection) bounds the floor. **No additional test needed.**
- **Schema_version round-trip lock for populated mounts**. Joel §8.12 considered and rejected an explicit test ("change-detector for a value that's already locked"). The existing `TestStore_RoundTripConfigAndSecrets` plus the new `TestStore_LoadAcceptsValidMounts` together implicitly lock this. Acceptable.

### One micro-concern (not blocking)

`TestValidateMount_Table`'s `relative_target` row uses `data` (no slash). The error message says `container_path must be absolute, got "data"` (with the relative path quoted). The test asserts only the substring `container_path must be absolute` — it doesn't lock the `, got %q` tail. That's fine: the tail is wording, the load-bearing semantics is the rejection class. Operator-debug context is in the wording but that's covered by visual inspection of the error; not worth a test.

### Verdict on tests

**All tests are load-bearing.** None are change-detectors on prose. The dual-sentinel-chain regression catcher (Issue 1 fix) is in place at two layers (cli test + exit-code test). Test surface is the shape Linus's v2 review demanded.

---

## 3. `gofmt`, `go vet`, `go build`, `go test ./...`

Ran fresh on the worktree. Results:

```
$ gofmt -l .
(empty)

$ go vet ./...
(empty)

$ go vet -tags integration ./...
(empty)

$ go build ./...
(no output)

$ go build -tags integration ./...
(no output)

$ go test ./...
ok  	github.com/alexander-fenster/decloud/internal/caddy	(cached)
ok  	github.com/alexander-fenster/decloud/internal/cli	(cached)
ok  	github.com/alexander-fenster/decloud/internal/config	(cached)
ok  	github.com/alexander-fenster/decloud/internal/deploy	(cached)
ok  	github.com/alexander-fenster/decloud/internal/dockerdrv	(cached)
ok  	github.com/alexander-fenster/decloud/internal/envcap	(cached)
ok  	github.com/alexander-fenster/decloud/internal/ids	(cached)
ok  	github.com/alexander-fenster/decloud/internal/logging	(cached)
ok  	github.com/alexander-fenster/decloud/internal/registry	(cached)
```

All gates green. Integration test compiles under the tag (verified via `go build -tags integration`) and skips at runtime unless `DECLOUD_INTEGRATION=1` is set — confirmed by reading `skipUnlessIntegrationEnabled` and the `t.Skipf` call.

---

## 4. API doc hallucinations — careful audit

This is the part CLAUDE.md flags as "very very carefully." I re-traced every operator-facing claim Raymond wrote against shipped source code (not against Joel's plan).

### `_docs/usage.md` line-by-line

**Line 71** — the `--mount` row:
- "Persistent volume" — matches `internal/cli/deploy_service.go:62` help text.
- "<host-path>:<container-path>[:ro] (bind) or <name>:<container-path>[:ro] (named volume); repeatable" — matches help text byte-for-byte.
- "Bind sources must be absolute paths starting with `/`" — matches `mount.go:39` (`!strings.HasPrefix(m.HostPath, "/")` → triggers named-volume regex), and `IsNamed`'s convention.
- "named-volume sources must match `[a-zA-Z0-9][a-zA-Z0-9_.-]+`" — matches `mount.go:12` regex literal exactly.
- "The container path must be absolute" — matches `mount.go:36-37` (`filepath.IsAbs`).
- "only `:ro` is accepted as a mode flag (`:rw`, `:z`, `:Z`, `:cached`, `:delegated` are rejected)" — `:ro` is the only accepted value per `mount.go:85-89`. The named list of rejected modes is operator-helpful prose, not a contract claim — the code rejects ALL other strings, not just those four. Acceptable: the prose is enumerating common operator typos.
- "Two `--mount` flags targeting the same container path are rejected at parse time with exit 2" — verified: `parseMountFlags` calls `FindDuplicateTarget`, wraps with `errUsage`, exit 2 per `exit_codes.go:39`.
- "a hand-edited TOML carrying the same shape is rejected at load time with exit 10" — verified: `store.go:68` calls `ValidateMounts`, which wraps with `ErrInvalidMount`, exit 10 per `exit_codes.go:41`.

**Lines 74-80** — the no-stat paragraph:
- The exact paragraph is verbatim from addendum Issue 5 (locked by Linus v2).
- "typical text: `error while creating mount source path '/missing-path': mkdir ...`, exit 40" — flagged as "typical" not "exact" (correctly hedged); exit 40 is correct per `exit_codes.go:55-56` (`ErrRun → ExitRunFail = 40`); cli_driver.go wraps daemon errors via `fmt.Errorf("docker run: %w; stderr=%q", err, stderr.String())`. Genuine Docker stderr varies by version/host; the "typical text" framing is honest.

**Lines 84-128** — mount examples. Each example uses real flag names (`--name`, `--host`, `--port`, `--mount`) in patterns the code accepts. Spot-checked the named-volume example (`myservice_state:/var/lib/myservice`): name passes the regex (two+ chars, alphanumeric start, underscore allowed); container path is absolute. Validates.

**Lines 131-150** — TOML example:
- `[[run.mounts]]` — matches `types.go:52-57` where `RunSpec.Mounts []Mount` carries `toml:"mounts"` inside `RunSpec` which itself is `[run]` (struct field tag `toml:"run"` on `ServiceConfig.Run`).
- Field names `host_path`, `container_path`, `read_only` — match `types.go:60-62` exactly.
- "schema_version stays at 1" — matches `types.go` `CurrentSchemaVersion = 1`.

**Line 176** — exit 2 row, expanded:
- "malformed `--mount` value at the command line (bad component count, missing absolute container path, unsupported mode flag, duplicate container path across `--mount` flags)" — every clause traces to a `parseMountFlags` / `ParseMountString` rejection branch.

**Line 177** — exit 10 row, swapped:
- "`--strategy` other than `recreate`, malformed `--mount` in a hand-edited TOML" — traces to `ErrInvalidStrategy` and `ErrInvalidMount` mappings in `exit_codes.go:41-42`.

### Other docs

**`_ai/decisions/m1-scope.md:16`** — the user/linter-nudged line: "**No `--mount` in M1** — flag rejected; loader also rejected non-empty `Mounts` (closed hand-edit loophole). Shipped at M2; see `_tasks/2026-04-28-m2-server-side-mounts/`." Matches the locked addendum/plan wording, with the user's recent past-tense polish ("rejected", "closed"). Internally consistent with the rest of the file (line 11 still says "each one is a tar pit if relaxed" — the M1 cuts list is now mixed-tense but readable; the cuts that have shipped are flipped to past, the cuts still pending stay present-tense).

**`_ai/decisions/m1-scope.md:32`** — the second user nudge: "M1 service deploy MVP → **M2 server-side mounts (`--mount` flag, loader populates `Mounts`)** → M3 host bootstrap..." — env-file hardening phantom is dead. Matches Joel §11 exactly.

**`_ai/decisions/schema-versioning.md:16`** — past-tense rewrite of M1's loader rejection; names `ErrMountsNotSupported (deleted at M2)` correctly; cites `registry.ValidateMounts` and `internal/registry/mount.go` correctly.

**`_ai/decisions/secrets-split.md:24`** — the rejection-classes list: `ErrInvalidMount (malformed mounts entry — replaced the M1 ErrMountsNotSupported blanket rejection at M2)`. Sentinel name correct. Replacement narrative correct.

**`_ai/decisions/m1-test-strategy.md:7`** — appended sentence about the M2 integration test naming `internal/integration/mount_test.go`, build-tag `integration`, env var `DECLOUD_INTEGRATION=1`. All three substrings exist in the source tree at the cited path. ✓

**`_ai/MEMORY.md:9`** — past tense "(mounts populated since M2, secret-files at M7)". ✓

**`_ai/MEMORY.md:58`** — new task pointer with one-line summary. The summary names `--mount`, named volumes, `:ro`, `Mounts`, `registry.ValidateMounts`, `Volumes`, `RunRequest`, `ErrMountsNotSupported`, `ErrInvalidMount`, "no source stat", build-tag `integration`, env var `DECLOUD_INTEGRATION=1`, and the m1x-backlog item numbers (9, 10, 11). All cross-checked against source. ✓

**`_ai/cli-flag-surface-coherence.md:42`** — historical-narration of the deleted `TestDeployService_MountFlagHelpReferencesM2`. Cites the deletion under Don §7 / Joel Decision 9 of the M2 task. Cross-references to the resequence-task review files are accurate. ✓

**`_ai/m1x-backlog.md`**:
- Item 6 strikethrough heading + "PARTIALLY DONE at M2" status. ✓
- Body cites `internal/integration/mount_test.go`, build-tag, env var, `decloud caddy up`, `--mount=<tmpdir>:/data:ro`, `t.Cleanup` with `docker rm -f`. The actual integration test does NOT bring up `decloud caddy up` (it skips orchestrator entirely and goes driver-direct per Joel §4.8 revised approach). **Minor doc drift here**: item 6's "M2 delivery" paragraph claims "Brings up `decloud caddy up`, builds a tiny test image, deploys with `--mount=...`" — but the shipped test does none of those. It does `driver.NewCLIDriver()` directly and `driver.Run(...)` with `Volumes`. See "5. Hallucinations and drift" below.
- Item 9 (reloader `%q`), item 10 (curl-through-Caddy), item 11 (`Driver.Run` consolidation) — bodies match Joel §"Issue 9" and Joel Decision 4. ✓

### Hallucinations and drift summary

- **`_ai/m1x-backlog.md` item 6 "M2 delivery" paragraph mis-describes the shipped integration test.** The paragraph claims the test "brings up `decloud caddy up`, builds a tiny test image, deploys with `--mount=<tmpdir>:/data:ro`, asserts `docker exec` reads the marker file, and tears down through `t.Cleanup` with idempotent `docker rm -f`". The actual test (`internal/integration/mount_test.go`) skips Caddy entirely, skips `docker build` (uses `alpine:3.19` directly via `ImagePull`), skips the deploy orchestrator (calls `driver.Run` directly), and asserts `docker exec cat /data/marker.txt`. The `docker rm -f` cleanup IS present. **This is the only doc-drift I found.** Minor (the m1x-backlog is a future-Don note, not an operator-facing surface), but it should be corrected to match what shipped — Joel §4.8's "revised approach" was the right call, and the backlog entry should reflect that, not Joel's earlier (Don §8) sketch.

- **No CLI-behaviour hallucinations.** Every flag name, exit code, error class, TOML field name, and validation rule traces to a specific line in shipped source. Raymond's audit-by-read methodology held.

---

## 5. Five-surface coherence

Per `_ai/cli-flag-surface-coherence.md`'s four-surface doctrine, plus the (now-retired) fifth semantic-token surface:

| Surface | Site | M1 wording | M2 wording | Coherent? |
|---|---|---|---|---|
| 1. Runtime check | `deploy_service.go:72` `parseMountFlags` | rejected with `ErrMountsNotSupported` | parsed and validated | ✓ |
| 2. Error message | `parseMountFlags` (errUsage) / `ValidateMounts` (ErrInvalidMount) | "mounts are not supported until M2" | "<reason>: usage error" / "registry: invalid mount: ..." | ✓ |
| 3. `--help` text | `deploy_service.go:62` | "M1: rejected with ExitConfigError (M2 only)" | "persistent volume; <host-path>:<container-path>[:ro] (bind) or <name>:<container-path>[:ro] (named volume); repeatable" | ✓ |
| 4. `_docs/usage.md` | line 71 | "Rejected with exit 10 in M1. Persistent volumes are M2." | full validation spec + no-stat paragraph | ✓ |
| 5. Semantic-token test | `TestDeployService_MountFlagHelpReferencesM2` | asserted `"M2"` substring | DELETED (no token to lock post-M2 ship) | ✓ |

All five flipped in lockstep across two commits (Kent RED → Rob GREEN). At no commit in git history does the CLI accept `--mount` while the loader rejects, or vice versa.

---

## 6. Error-wrap discipline (`_ai/error-wrap-discipline.md`)

The dual-sentinel-chain fix from addendum Issue 1 is the load-bearing item.

### Loader chain (exit 10 path)

`store.go:68` calls `ValidateMounts(cfg.Run.Mounts, name, cfgPath)` → `mount.go:55` wraps with `fmt.Errorf("%w: ... %w", ErrInvalidMount, ..., innerErr)` (per-mount path) or `mount.go:59-60` wraps with `fmt.Errorf("%w: ... duplicate ...", ErrInvalidMount, ...)` (cross-mount path). Both use `%w` for `ErrInvalidMount`. The duplicate-target loader path uses no second `%w` (no inner sentinel — the duplicate-error is a structured string, not a wrapped sentinel).

`errors.Is(err, ErrInvalidMount)` → true. `errors.Is(err, errUsage)` → false. Verified by `TestStore_LoadRejectsInvalidMounts`.

### CLI chain (exit 2 path)

`parseMountFlags` (lines 165–189):
- Per-mount parse failure: `fmt.Errorf("--mount %q: %s: %w", s, err.Error(), errUsage)` — `%s` flattens the inner error to a string (NO `%w` for it), `%w` wraps `errUsage` only.
- Duplicate-target failure: `fmt.Errorf("--mount %q: duplicate container_path (also at --mount[%d]): %w", ..., errUsage)` — only `errUsage` is `%w`-wrapped.

`errors.Is(err, errUsage)` → true. `errors.Is(err, ErrInvalidMount)` → **false** (no longer in the chain). Verified by `TestDeployService_MountFlagInvalidReturnsExitUsageError`'s `duplicate_target` subtest with the explicit `assert.False(t, errors.Is(err, registry.ErrInvalidMount))`.

### `%v` audit

Searched for `%v` on errors in the M2 diff: zero hits. The convention (`%w` not `%v` for errors) is preserved.

The regression test catches the failure mode it's there to catch:
- If a future refactor swaps `parseMountFlags` to call `ValidateMounts` (which would re-introduce `ErrInvalidMount` into the CLI chain), `assert.False` triggers. ✓
- If the cases in `exit_codes.go` are reordered such that `ErrInvalidMount` precedes `errUsage` (without the chain fix, this would have flipped exit codes), the new `cli-mount-dup-wraps-usage` row in `TestExitCodeFor_AllSentinels` synthesises a `--mount-shaped wrap of errUsage` and asserts exit 2. ✓

**Discipline held. No regression.**

---

## 7. Cleanup / context discipline (`_ai/cleanup-context-discipline.md`, `cancellation-symmetry-audit.md`)

The integration test uses `t.Cleanup(func() { removeContainerIdempotent(t, mountTestContainer) })` with a `context.WithTimeout(context.Background(), 30*time.Second)` cleanup context — NOT derived from any test-request context. This is the canonical cleanup-context discipline: cleanup runs on a Background-derived bounded ctx so user cancellation can't abort it.

The `removeContainerIdempotent` function uses `exec.CommandContext` with the cleanup ctx and ignores the error (`_ = ...Run()`) — idempotent on already-absent containers. Same shape as the m1x-backlog item 6 §"Fix shape" required ("Cleanup must be idempotent").

The `t.Cleanup` is registered BEFORE the `Run` call — so even if `Run` panics or fails mid-call, the cleanup still fires. There's also a defensive `removeContainerIdempotent(...)` call BEFORE `Run` (line 61) to guard against state from a previous test run that didn't cleanly tear down. Belt-and-braces, defensible.

The test does NOT leak network resources — `NetworkEnsure(ctx, "decloud")` creates the network if needed but the `decloud` network is shared with other decloud machinery (Caddy etc.) so removing it would break other things. Acceptable: networks are cheap and idempotent.

**No leak. No cleanup-on-cancel risk. Discipline preserved.**

---

## 8. Stub residue

Kent's RED commit added stubs in `internal/registry/mount.go`. Rob's GREEN commit replaced every stub with a real body. Verified:

- `IsNamed` — real body (line 19).
- `ValidateMount` — real body (lines 29–45).
- `ValidateMounts` — real body using new `FindDuplicateTarget` (lines 52–63).
- `ParseMountString` — real body (lines 72–98).
- `FindDuplicateTarget` — real body, exported (lines 106–115).

No stub bodies remain. No "TODO Rob" comments. No zombie comments referring to "the GREEN commit will fill this in" or similar.

The package-level `var ErrInvalidMount` is correctly in `errors.go` only (NOT duplicated in `mount.go` per Joel §3.3 instruction).

**Stub residue: none.**

---

## 9. The user/linter nudge

`_ai/decisions/m1-scope.md:16`:
> "**No `--mount` in M1** — flag rejected; loader also rejected non-empty `Mounts` (closed hand-edit loophole). Shipped at M2; see `_tasks/2026-04-28-m2-server-side-mounts/`."

Matches Joel §11 + addendum Issue 10 verbatim, with the user's recent past-tense polish ("rejected", "closed"). Internally consistent.

`_ai/decisions/m1-scope.md:32`:
> "M1 service deploy MVP → **M2 server-side mounts (`--mount` flag, loader populates `Mounts`)** → M3 host bootstrap..."

Phantom kill complete. No "env-file hardening" residue anywhere in `_ai/` or `_docs/` (verified by `grep -rn "env-file hardening"` returning empty).

Raymond's other doc edits are consistent with these nudges:
- `m1-scope.md:32` matches the canonical roadmap line cited in `_tasks/.../001-user-request.md`.
- `MEMORY.md:7` ("decisions/m1-scope.md ... full M1→M7 milestone sequence.") doesn't repeat the env-file hardening phrase. ✓
- `MEMORY.md:9` past-tense ("mounts populated since M2"). ✓
- `secrets-split.md:24` rejection-classes list correct. ✓
- `schema-versioning.md:16` past-tense rewrite. ✓
- `cli-flag-surface-coherence.md:42` historical-narration. ✓
- `m1x-backlog.md` item 6 strikethrough + items 9, 10, 11 added. ✓ (modulo the "M2 delivery" paragraph drift noted in §4 above).

**Verdict on nudge consistency: clean, except for the one m1x-backlog item 6 description drift.**

---

## 10. Optional fixes

These are not blockers; capture them for the closeout step or strike them.

### Fix A — `internal/registry/types.go`: doc-comment on `Mount.HostPath`

Joel §3.1 and Linus Issue 3 both implied a doc-comment on `Mount.HostPath` would land naming the named-volume aliasing convention. Rob put the convention on `Mount.IsNamed()` in `mount.go` instead, which is reachable by anyone IDE-jumping to `Mount`'s definition (the method shows up). But the bare struct field has no comment and a casual reader of `types.go` sees `HostPath string `toml:"host_path"`` and doesn't immediately know it doubles as a named-volume name.

Suggested edit at `internal/registry/types.go:59-63`:

```go
type Mount struct {
	// HostPath is the mount source. For bind mounts it is an absolute host
	// path starting with "/"; for named volumes it is the volume name. The
	// TOML key is historically named host_path. Use Mount.IsNamed() to
	// distinguish at runtime.
	HostPath      string `toml:"host_path"`
	ContainerPath string `toml:"container_path"`
	ReadOnly      bool   `toml:"read_only"`
}
```

Three lines of comment. Zero behaviour change. Promotes the convention to first sight when someone reads `types.go`.

### Fix B — `_docs/usage.md` line 3: tense slip

```
Operator-facing reference for the Decloud M1 CLI.
```

Now that M2 has shipped a real new flag end-to-end (not a refactor or a doc-only update), the "M1 CLI" label is stale. Suggested edit:

```
Operator-facing reference for the Decloud CLI.
```

(Drop "M1.") This is fix-while-fresh on a tense-slip; the rest of the file's prose is mixed-tense already (some past, some present). Trivial.

### Fix C — `_ai/m1x-backlog.md` item 6 "M2 delivery" paragraph

Currently claims the integration test brings up Caddy and runs the orchestrator end-to-end. The actual shipped test is driver-direct (per Joel §4.8 revised approach). Suggested rewrite:

```
**M2 delivery:** `internal/integration/mount_test.go` with `//go:build integration` tag, gated on `DECLOUD_INTEGRATION=1`. Pulls `alpine:3.19` via the real `dockerdrv.CLIDriver`, calls `driver.Run` directly with a `Volumes: [...]` shape carrying one bind ro mount, and asserts `docker exec cat /data/marker.txt` returns the marker bytes. Cleanup via `t.Cleanup` with idempotent `docker rm -f decloud-mounttest`. Does NOT exercise the deploy orchestrator (build, readiness, Caddyfile generation, reload) — those are split to item 10 (curl-through-Caddy ingress test) per Joel decision 8 of the M2 tech plan.
```

The justification (failure-mode separation) is preserved; the "what shipped" narrative now matches reality.

---

## DECISION: APPROVED WITH MINOR FIXES

All three optional fixes are below the bar to block ship. Rob/Raymond can address them in the PLAN re-entry close-out (or punt to a separate "doc tidy" commit on `main` post-squash). The implementation itself is correct, the test surface is load-bearing, the docs trace to source with one minor narrative drift (Fix C), and the user/linter nudge is fully integrated.

The two commits Kent and Rob shipped, plus Raymond's docs commit, constitute a complete and coherent M2 ship. The dual-sentinel-chain fix (the worst part of the plan per Linus's first review) is structurally honest in the implementation. The integration test bundling proves the feature works against real Docker. The `ErrMountsNotSupported` deletion is total (8 production sites, 8 test sites, all confirmed gone via grep).

Linus's "no PLAN v3 needed" call held: nothing in the implementation surfaced a flaw that requires reopening the plan.

---

## Files I read end-to-end

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

Production code (M2 surface + cross-references):
- `/Users/fenster/dev/decloud/internal/registry/mount.go` (NEW)
- `/Users/fenster/dev/decloud/internal/registry/errors.go`
- `/Users/fenster/dev/decloud/internal/registry/store.go`
- `/Users/fenster/dev/decloud/internal/registry/types.go`
- `/Users/fenster/dev/decloud/internal/cli/deploy_service.go`
- `/Users/fenster/dev/decloud/internal/cli/exit_codes.go`
- `/Users/fenster/dev/decloud/internal/dockerdrv/cli_driver.go`
- `/Users/fenster/dev/decloud/internal/dockerdrv/driver.go`
- `/Users/fenster/dev/decloud/internal/deploy/service.go`
- `/Users/fenster/dev/decloud/internal/deploy/lifecycle.go`

Tests:
- `/Users/fenster/dev/decloud/internal/registry/mount_test.go` (NEW)
- `/Users/fenster/dev/decloud/internal/registry/store_test.go`
- `/Users/fenster/dev/decloud/internal/cli/deploy_service_test.go`
- `/Users/fenster/dev/decloud/internal/cli/exit_codes_test.go`
- `/Users/fenster/dev/decloud/internal/dockerdrv/cli_driver_test.go`
- `/Users/fenster/dev/decloud/internal/deploy/service_test.go`
- `/Users/fenster/dev/decloud/internal/deploy/lifecycle_test.go`
- `/Users/fenster/dev/decloud/internal/integration/mount_test.go` (NEW)
- `/Users/fenster/dev/decloud/internal/integration/doc.go` (NEW)

Docs (Raymond's surface):
- `/Users/fenster/dev/decloud/_docs/usage.md`
- `/Users/fenster/dev/decloud/_ai/decisions/m1-scope.md`
- `/Users/fenster/dev/decloud/_ai/decisions/schema-versioning.md`
- `/Users/fenster/dev/decloud/_ai/decisions/secrets-split.md`
- `/Users/fenster/dev/decloud/_ai/decisions/m1-test-strategy.md`
- `/Users/fenster/dev/decloud/_ai/MEMORY.md`
- `/Users/fenster/dev/decloud/_ai/m1x-backlog.md`
- `/Users/fenster/dev/decloud/_ai/cli-flag-surface-coherence.md`
