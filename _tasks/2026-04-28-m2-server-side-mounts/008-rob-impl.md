# 008 — Rob's implementation (GREEN commit)

Branch: `feat/m2-server-side-mounts`. This is the GREEN flip on top of Kent's RED (commit `432e3e8`). All five surfaces of the `--mount` flag flip in one commit per `_ai/cli-flag-surface-coherence.md`. The 18 failing M2 tests Kent committed are now green; no M1 regression.

Anchors:
- Joel's tech plan: `_tasks/2026-04-28-m2-server-side-mounts/003-joel-tech-plan.md`
- Joel's addendum (Issues 1, 5, 10): `_tasks/2026-04-28-m2-server-side-mounts/005-joel-tech-plan-addendum.md`
- Linus v2 approval: `_tasks/2026-04-28-m2-server-side-mounts/006-linus-plan-review-v2.md`
- Kent's RED report: `_tasks/2026-04-28-m2-server-side-mounts/007-kent-tests.md`

## Files modified (8 production files, no docs)

### `internal/registry/mount.go`
Replaced all five stubs with real bodies. `IsNamed` derives from the `/` prefix (Joel decision 3). `ValidateMount` runs grammar-only checks (no stat per Joel decision 1). `ValidateMounts` iterates `ValidateMount` and then calls `FindDuplicateTarget`, wrapping with `ErrInvalidMount` (loader-side path). `ParseMountString` handles `<src>:<target>[:ro]` with explicit rejection of `:rw` and any other mode flag. `FindDuplicateTarget` is exported (capital F) per addendum line 100 because the CLI imports it from a different package. Added the `volumeNameRE` package-level regex (`^[a-zA-Z0-9][a-zA-Z0-9_.-]+$`, two-char minimum, matches Docker's volume.IsValidName).

### `internal/registry/errors.go`
Deleted `ErrMountsNotSupported`. `ErrInvalidMount` retained at the alphabetical-ish position the deleted sentinel previously occupied. Re-aligned the `var()` block field widths after the deletion (gofmt-driven).

### `internal/registry/store.go`
Replaced the `len(cfg.Run.Mounts) > 0` rejection block with `ValidateMounts(cfg.Run.Mounts, name, cfgPath)`. In the same hunk, fixed the strategy-block papercut at line 73: `cfg.Name` → `name` per addendum Issue 10 (the function parameter sourced from the filename is always populated; `cfg.Name` from the TOML body can be empty if the operator omits `name = "..."`).

### `internal/cli/deploy_service.go`
Three changes:
1. The `--mount` flag binding flipped from `StringSliceVar` to `StringArrayVar` (per Joel §8.9 — `StringSliceVar` would split values on `,`, eating commas from bind paths).
2. The help text replaced with Joel decision 6 wording: `"persistent volume; <host-path>:<container-path>[:ro] (bind) or <name>:<container-path>[:ro] (named volume); repeatable"`.
3. The `len(f.Mounts) > 0` rejection block replaced with a `parseMountFlags(f.Mounts)` call. The new `parseMountFlags` helper at the bottom of the file iterates `registry.ParseMountString` and wraps any per-mount error with `errUsage` (exit 2), then calls `registry.FindDuplicateTarget` directly (NOT `registry.ValidateMounts`) and wraps the duplicate-target case with `errUsage` ONLY — per addendum Issue 1 / Linus v2's "the dual-sentinel chain is dead" lock. The `Mounts: mounts` field now populates `deploy.Request`.

### `internal/cli/exit_codes.go`
Sentinel swap in the `ExitConfigError` case: `registry.ErrMountsNotSupported` → `registry.ErrInvalidMount`. The CLI's exit-2 path is already covered by the existing `errors.Is(err, errUsage)` case at line 39.

### `internal/dockerdrv/cli_driver.go`
Added the `for _, v := range req.Volumes { args = append(args, "-v", formatVolume(v)) }` loop in `Run`, between the env loop and the label append, mirroring `RunWithOptions`. Argv shape unchanged: `<source>:<target>[:ro]` via the existing `formatVolume`. The `Volumes []VolumeMount` struct field on `RunRequest` was already added by Kent in the RED commit.

### `internal/deploy/service.go`
Added `toVolumeMounts(mounts []registry.Mount) []dockerdrv.VolumeMount` helper at the bottom — converts persisted mounts into driver `VolumeMount`s with `IsNamed` derived per `Mount.IsNamed()`. Three call sites populate `Volumes`: the fresh deploy `runReq` at line ~244, the registry-save `RunSpec.Mounts` (changed from `[]registry.Mount{}` to `req.Mounts`), and the `restoreOldContainer` rollback `runReq`.

### `internal/deploy/lifecycle.go`
Added `Volumes: toVolumeMounts(prev.Config.Run.Mounts)` to the `default:` arm `runReq` in `Start` at lines 67-78 — covers the absent-branch path where a Stop/Start cycle has cleaned up the container and `Start` re-runs `docker run`. The helper is in the same package; no import changes.

## Five-surface flip (atomic)

Per `_ai/cli-flag-surface-coherence.md` and Joel §6, ALL of these land in one commit:

| Surface | Site | M2 change |
|---|---|---|
| CLI flag accept | `internal/cli/deploy_service.go:61` (`StringArrayVar`) | Flipped from `StringSliceVar` (split-on-`,` bug) and from "M1: rejected with ExitConfigError" wording |
| Loader accept | `internal/registry/store.go:68` (`ValidateMounts` call) | `len > 0` rejection deleted, replaced with grammar validation |
| Runtime pass | `internal/dockerdrv/cli_driver.go` `Run` for-loop + `internal/deploy/service.go` `toVolumeMounts` populating `runReq.Volumes` at three sites + `internal/deploy/lifecycle.go` `Start` absent branch | Volumes thread CLI → Request → RunRequest → docker argv |
| Sentinel deletion | `internal/registry/errors.go` (`ErrMountsNotSupported` deleted) + `internal/cli/exit_codes.go` (case-list entry swapped to `ErrInvalidMount`) | Both deletions in the same commit, no half-flipped state |
| Help-text wording | `internal/cli/deploy_service.go:61` (third arg to `StringArrayVar`) | Flipped to Joel decision 6 wording |

The half-flipped failure mode Joel called out in §6 is closed: at no commit in git history does the CLI accept `--mount` while the loader rejects, or vice versa.

## Final state

```
$ go build ./...
(no output)

$ go test ./...
ok    github.com/alexander-fenster/decloud/internal/caddy      0.019s
ok    github.com/alexander-fenster/decloud/internal/cli        0.021s
ok    github.com/alexander-fenster/decloud/internal/config     0.009s
ok    github.com/alexander-fenster/decloud/internal/deploy     12.097s
ok    github.com/alexander-fenster/decloud/internal/dockerdrv  0.077s
ok    github.com/alexander-fenster/decloud/internal/envcap     0.103s
ok    github.com/alexander-fenster/decloud/internal/ids        0.010s
ok    github.com/alexander-fenster/decloud/internal/logging    0.015s
ok    github.com/alexander-fenster/decloud/internal/registry   0.040s

$ go vet ./...
(no output)

$ gofmt -l .
(no output)

$ go generate ./...
(no diffs — Option-β payoff held; mock regen is a no-op)

$ go build -tags integration ./...
(no output — integration test compiles under the tag, gated on DECLOUD_INTEGRATION=1 at runtime)

$ git grep -F "ErrMountsNotSupported" -- '*.go'
(no matches — sentinel fully deleted from production code)
```

## Decisions where the plan was ambiguous

1. **Location of `volumeNameRE`.** Joel §3.2 placed the package-level `var volumeNameRE = regexp.MustCompile(...)` in `mount.go`. Kent's RED stub did NOT include it (the addendum did not respecify). I added it in `mount.go` per original §3.2 — it sits at file-top for init-time panic on any malformed regex, and it lives next to its single consumer (`ValidateMount`).

2. **`IsNamed` location.** Joel §3.1 originally proposed `types.go`, but the addendum's `mount.go` body and Kent's RED stub both placed it in `mount.go`. I kept it in `mount.go` per Kent's "leave it co-located with the M2 surface" recommendation in his §"Decisions I made where the plan was ambiguous" point 1.

3. **`errors.go` formatting.** Deleting `ErrMountsNotSupported` removed the longest identifier in the `var` block, so `gofmt` realigned all the `=` columns. Accepted the realignment.

4. **CLI duplicate-target error message format.** Joel's addendum gave the wording `"--mount %q: duplicate container_path (also at --mount[%d]): %w"`. I implemented exactly this. The CLI test asserts the chain (`errors.Is(err, errUsage)` true, `errors.Is(err, registry.ErrInvalidMount)` false) and the prefix (`--mount`); both pass.

5. **Mock regeneration.** Joel §3.11 said `go generate` would be a no-op (Option-β). Confirmed: ran `go generate ./...` after my edits, `git status` shows no new diffs.

## Notes for Raymond (next step)

You own the docs sweep (`_docs/usage.md`, `_ai/decisions/m1-scope.md`, `_ai/decisions/schema-versioning.md`, `_ai/decisions/secrets-split.md`, `_ai/MEMORY.md`, `_ai/m1x-backlog.md`, `_ai/cli-flag-surface-coherence.md`). The verbatim no-stat paragraph for `_docs/usage.md` between lines 73 and 74 is locked in addendum Issue 5. The full table of doc-side flips is at `003-joel-tech-plan.md` §11.

## Files modified (absolute paths)

- `/Users/fenster/dev/decloud/internal/registry/mount.go`
- `/Users/fenster/dev/decloud/internal/registry/errors.go`
- `/Users/fenster/dev/decloud/internal/registry/store.go`
- `/Users/fenster/dev/decloud/internal/cli/deploy_service.go`
- `/Users/fenster/dev/decloud/internal/cli/exit_codes.go`
- `/Users/fenster/dev/decloud/internal/dockerdrv/cli_driver.go`
- `/Users/fenster/dev/decloud/internal/deploy/service.go`
- `/Users/fenster/dev/decloud/internal/deploy/lifecycle.go`
