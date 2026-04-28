# 008 — Rob's implementation: M3 → M2 milestone-label swap

## Status

**GREEN.** All three of Kent's red-bar assertions (007 §C.1, §C.2, §C.3) now pass. `go test ./...` reports `ok` for every package. `gofmt -l .` and `go vet ./...` are both empty. `go generate ./...` produces no diff. Working tree is consistent — the binary's `--help` text, the runtime error on `--mount`, and the loader rejection error all name the same milestone (M2).

## Plan reference

Per `005-joel-tech-plan-v2.md` §B.11 / §D and `006-linus-rereview.md` §C3: a single atomic three-edit change, with no test-file or doc-file edits inside this commit. Constraints honored:

- Only Go source touched (`internal/cli/deploy_service.go`, `internal/registry/store.go`).
- No test files modified (Kent owns those — verified `git diff --stat` only shows two `.go` files outside `*_test.go`).
- No docs modified (Raymond owns those).
- `gofmt` clean.

## The three edits (verbatim before/after)

### Edit 1 — `internal/cli/deploy_service.go:61` (Cobra flag-help text)

**Before:**
```go
cmd.Flags().StringSliceVar(&f.Mounts, "mount", nil, "M1: rejected with ExitConfigError (M3 only)")
```

**After:**
```go
cmd.Flags().StringSliceVar(&f.Mounts, "mount", nil, "M1: rejected with ExitConfigError (M2 only)")
```

Locks Kent's §C.3 assertion: `flag.Usage` must `Contains` the substring `"M2 only"`.

### Edit 2 — `internal/cli/deploy_service.go:72` (runtime rejection on `--mount`)

**Before:**
```go
return fmt.Errorf("--mount is not supported until M3: %w", registry.ErrMountsNotSupported)
```

**After:**
```go
return fmt.Errorf("--mount is not supported until M2: %w", registry.ErrMountsNotSupported)
```

Locks Kent's §C.1 assertion: the wrapped error string must `Contains` the substring `"--mount is not supported until M2"`.

### Edit 3 — `internal/registry/store.go:69` (loader rejection on non-empty `Run.Mounts`)

**Before:**
```go
return nil, fmt.Errorf("%w: service %q declares %d mount(s) in %s; mounts are not supported until M3",
    ErrMountsNotSupported, cfg.Name, len(cfg.Run.Mounts), cfgPath)
```

**After:**
```go
return nil, fmt.Errorf("%w: service %q declares %d mount(s) in %s; mounts are not supported until M2",
    ErrMountsNotSupported, cfg.Name, len(cfg.Run.Mounts), cfgPath)
```

Locks Kent's §C.2 assertion: the wrapped error string must `Contains` the substring `"mounts are not supported until M2"`.

## Verification

### `gofmt -l .`

```
(empty)
```

### `go vet ./...`

```
(empty — no findings)
```

### `go test ./...` (from repo root)

```
?   	github.com/alexander-fenster/decloud/cmd/decloud	[no test files]
ok  	github.com/alexander-fenster/decloud/internal/caddy	(cached)
?   	github.com/alexander-fenster/decloud/internal/caddy/mocks	[no test files]
ok  	github.com/alexander-fenster/decloud/internal/cli	0.015s
?   	github.com/alexander-fenster/decloud/internal/cli/mocks	[no test files]
ok  	github.com/alexander-fenster/decloud/internal/config	(cached)
ok  	github.com/alexander-fenster/decloud/internal/deploy	(cached)
ok  	github.com/alexander-fenster/decloud/internal/dockerdrv	(cached)
?   	github.com/alexander-fenster/decloud/internal/dockerdrv/mocks	[no test files]
ok  	github.com/alexander-fenster/decloud/internal/envcap	(cached)
?   	github.com/alexander-fenster/decloud/internal/envcap/mocks	[no test files]
ok  	github.com/alexander-fenster/decloud/internal/ids	(cached)
ok  	github.com/alexander-fenster/decloud/internal/logging	(cached)
ok  	github.com/alexander-fenster/decloud/internal/registry	0.028s
?   	github.com/alexander-fenster/decloud/internal/registry/mocks	[no test files]
```

Compare to Kent's pre-Rob per-package summary in 007 — the two `FAIL` entries (`internal/cli`, `internal/registry`) are now `ok`. Every other package was `ok` before and remains `ok`. Zero regressions.

### Targeted re-runs of Kent's three assertions

```
=== RUN   TestDeployService_MountFlagReturnsErrMountsNotSupported
--- PASS: TestDeployService_MountFlagReturnsErrMountsNotSupported (0.00s)
=== RUN   TestDeployService_MountFlagHelpReferencesM2
--- PASS: TestDeployService_MountFlagHelpReferencesM2 (0.00s)
=== RUN   TestStore_LoadRejectsNonEmptyMounts
--- PASS: TestStore_LoadRejectsNonEmptyMounts (0.00s)
```

(Note: `TestDeployService_MountFlagReturnsErrMountsNotSupported` emits a benign stderr line `decloud: log dir unavailable, using stderr only: mkdir /opt/decloud: permission denied` — this is unrelated to the assertion and is the expected unsandboxed-fallback path the test exercises. The test still PASSes.)

### `go generate ./...` followed by `git status --porcelain`

```
 M _tasks/current
 M internal/cli/deploy_service.go
 M internal/cli/deploy_service_test.go
 M internal/registry/store.go
 M internal/registry/store_test.go
?? _tasks/2026-04-28-milestone-resequence/
```

The two `*_test.go` entries are Kent's pre-existing changes from 007 (not mine). The two non-test `.go` entries are exactly my three edits across two files. No generated artifacts are dirty. `_tasks/` entries are bureau bookkeeping.

## Behavior surfaces touched (operator-facing)

The three strings are operator-visible UX surfaces. After this change:

1. **`decloud deploy service --help`** — the `--mount` line now reads `M1: rejected with ExitConfigError (M2 only)`. Operators reading `--help` to discover when mounts ship will see "M2".

2. **Runtime error on `decloud deploy service --mount=...`** — the wrapped error now reads `--mount is not supported until M2: registry: mounts not supported in M1`. This bubbles up to the operator's terminal as the deploy fails with `ExitConfigError`.

3. **Loader rejection when a `service.toml` declares non-empty `[[run.mounts]]`** — the wrapped error now reads `registry: mounts not supported in M1: service "foo" declares 1 mount(s) in /path/to/foo.toml; mounts are not supported until M2`. This surfaces during `decloud apply` / config-load paths.

## Subtle effects to flag for downstream agents

1. **Log-grep patterns.** Anyone (operators, runbooks, ops-tooling) grepping logs for the literal string `"until M3"` or `"M3 only"` to spot mount-rejection events will need to update their patterns to `"until M2"` / `"M2 only"`. I did not search the docs tree (`_docs/`, `_ai/`) for such grep patterns — that's Raymond's §B doc-sweep scope per Joel v2 §D. Flagging here for Raymond's awareness: if any runbook documents a grep pattern against these specific strings, those patterns must move M3→M2 in the same logical change-set.

2. **The `ErrMountsNotSupported` sentinel is unchanged.** Its `.Error()` is still `"registry: mounts not supported in M1"` (defined in `internal/registry/errors.go`, untouched). That string says "M1" because mounts are not supported *in the current milestone* (M1) — the "until M2" suffix is the forward-looking promise about when they ship. Both conventions co-exist deliberately and consistently after this commit. I did not touch the sentinel's `Error()` text, and Kent's assertions correctly use `errors.Is(err, ErrMountsNotSupported)` (identity) for the sentinel check and substring `Contains` for the milestone-label check. The two checks are independent and remain so.

3. **No exit-code change.** `ExitConfigError` is still the exit code for both rejection paths. Only the human-readable rationale string moved.

4. **No public Go API change.** No exported symbols renamed, no signatures changed, no new sentinel errors. Downstream packages that import `internal/cli` or `internal/registry` are unaffected at the type/symbol level.

## Constraints I investigated and did *not* remove

Per CLAUDE.md "INVESTIGATE BEFORE REMOVING CONSTRAINTS": I considered whether the `if len(f.Mounts) > 0` guard at `deploy_service.go:71` and the `if len(cfg.Run.Mounts) > 0` guard at `store.go:68` should be removed (since mounts are now coming "in M2", which is the next milestone). They should NOT be removed — the guards are the *enforcement* of the M1-rejection contract, and the rejection contract is the very thing the task is preserving. The task is purely a *label* change ("when will this constraint lift"), not a *behavior* change ("does this constraint still apply"). The constraint stands; only its forward-looking rationale string moves M3→M2.

## What I did NOT do (scope discipline per Joel v2 §D)

- Did not edit any `*_test.go` file.
- Did not edit any `_docs/*.md` file (Raymond's §B.1–§B.10).
- Did not edit any `_ai/*.md` file (Raymond's scope, plus Ward's §finalization scope).
- Did not edit `internal/registry/errors.go` (the sentinel stays as-is — see "Subtle effects" §2 above).
- Did not run `git commit` — the task instructions say "we'll commit later — for now just leave the working tree consistent."

## Files relevant to this report (absolute paths)

- `/Users/fenster/dev/decloud/internal/cli/deploy_service.go` — edited (lines 61, 72)
- `/Users/fenster/dev/decloud/internal/registry/store.go` — edited (line 69)
- `/Users/fenster/dev/decloud/internal/cli/deploy_service_test.go` — Kent's, untouched by me
- `/Users/fenster/dev/decloud/internal/registry/store_test.go` — Kent's, untouched by me
- `/Users/fenster/dev/decloud/internal/registry/errors.go` — sentinel definition, intentionally untouched
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-milestone-resequence/005-joel-tech-plan-v2.md` §B.11, §D
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-milestone-resequence/006-linus-rereview.md` §C3
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-milestone-resequence/007-kent-tests.md` §C.1, §C.2, §C.3
