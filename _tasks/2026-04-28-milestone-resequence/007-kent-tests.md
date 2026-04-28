# 007 — Kent's test additions: lock the M2 milestone label

## Status

**RED — expected; will go green when Rob lands §B.11 of `005-joel-tech-plan-v2.md`.**

Per Joel's v2 §D / Linus's §C3 in `006-linus-rereview.md`, this commit deliberately leaves `main` red until Rob's atomic three-edit commit. The TDD red bar IS the discovery surface that locks Rob's edits in place. No CI gate exists in this repo (verified by `_docs/install.md` §3 — operator-runs-`go test` workflow), so the red bar is a developer-local signal. If Rob's edit is materially delayed (>1 working day), this commit gets reverted and re-applied as part of the TDD pair (Joel's §D mitigation, Linus-approved).

## Files touched (test code only — no Go source touched, that's Rob's commit)

1. `/Users/fenster/dev/decloud/internal/cli/deploy_service_test.go`
   - Lines 88–93: extended existing `TestDeployService_MountFlagReturnsErrMountsNotSupported` with one new substring assertion (line 92 in the new file).
   - Lines 96–103: NEW test `TestDeployService_MountFlagHelpReferencesM2` (8 lines, Cobra flag-help lookup + substring assertion).

2. `/Users/fenster/dev/decloud/internal/registry/store_test.go`
   - Lines 294–299: extended existing `TestStore_LoadRejectsNonEmptyMounts` with one new substring assertion (line 297 in the new file).

Total: 2 test files, 3 substring assertions, 0 source-file edits, 0 helper changes, 0 mock regenerations.

## Verbatim assertions added

### C.1 — `internal/cli/deploy_service_test.go`, inside `TestDeployService_MountFlagReturnsErrMountsNotSupported`

Added directly after the existing `assert.Equal(t, ExitConfigError, ExitCodeFor(err))`:

```go
assert.Contains(t, err.Error(), "--mount is not supported until M2",
    "runtime rejection must name the milestone where mounts ship; "+
        "keep in lockstep with _docs/usage.md and _ai/decisions/m1-scope.md")
```

### C.2 — `internal/registry/store_test.go`, inside `TestStore_LoadRejectsNonEmptyMounts`

Added directly after the existing `assert.ErrorIs(t, err, registry.ErrMountsNotSupported)`:

```go
assert.Contains(t, err.Error(), "mounts are not supported until M2",
    "loader rejection must name the milestone where mounts ship; "+
        "keep in lockstep with _docs/usage.md and _ai/decisions/m1-scope.md")
```

### C.3 — `internal/cli/deploy_service_test.go`, NEW test (placed between `TestDeployService_MountFlagReturnsErrMountsNotSupported` and `TestDeployService_StrategyBlueGreenReturnsErrInvalidStrategy`)

```go
func TestDeployService_MountFlagHelpReferencesM2(t *testing.T) {
    cmd := newDeployServiceCmd(&rootContext{})
    flag := cmd.Flags().Lookup("mount")
    require.NotNil(t, flag)
    assert.Contains(t, flag.Usage, "M2 only",
        "flag help must name the milestone where mounts ship; "+
            "keep in lockstep with the runtime rejection error and _docs/usage.md")
}
```

The constructor is `newDeployServiceCmd` (verified against `deploy_service.go:47` — Joel's plan §C.3 had a placeholder `newDeployServiceCommand`, the actual symbol is the abbreviated `Cmd` form matching the rest of `internal/cli/`).

## Why these aren't change-detector tests

`_ai/cli-flag-surface-coherence.md` says "Why we don't test surface 3" — a test that asserts on the help string is normally a textbook change-detector test. The discriminator that makes these three assertions different:

- They lock the **semantic milestone token** ("M2"), not arbitrary prose. The "M2" substring carries meaning — it's the milestone where `--mount` ships per `_ai/decisions/m1-scope.md`. If Rob's source-edit accidentally lands "M3" or any other label, the assertions fail; if Rob's source-edit lands the correct label inside *any* prose, they pass. That's a behavior contract on the milestone label, not a snapshot of the prose.
- The trailing `assert.Contains` message names the cross-references (`_docs/usage.md`, `_ai/decisions/m1-scope.md`) so a future regressor sees the contract immediately.
- Surfaces 1, 2, 3, 4 are the four-surface coherence contract from `_ai/cli-flag-surface-coherence.md`. Three of these tests lock surfaces 1, 2, 3; surface 4 (`_docs/usage.md`) is locked by Raymond's §B.10 doc edit. The set of four moves together; no one surface is asserted in isolation.

If Linus or Kevlin push back on the surface-3 test as change-detector-shaped, the fallback is to drop C.3 and rely on the cross-reference grep discipline for the help text. C.1 and C.2 are uncontroversial — they assert on substrings of error messages that operators *do* read.

## `go test ./...` output (the expected red bar)

Ran `go test ./...` from repo root. Three failures, all in the tests above, all with the M3-vs-M2 mismatch as the failure reason. No other test failed for any other reason.

```
=== RUN   TestDeployService_MountFlagReturnsErrMountsNotSupported
    deploy_service_test.go:92:
        Error Trace: /Users/fenster/dev/decloud/internal/cli/deploy_service_test.go:92
        Error:       "--mount is not supported until M3: registry: mounts not supported in M1" does not contain "--mount is not supported until M2"
        Test:        TestDeployService_MountFlagReturnsErrMountsNotSupported
        Messages:    runtime rejection must name the milestone where mounts ship; keep in lockstep with _docs/usage.md and _ai/decisions/m1-scope.md
--- FAIL: TestDeployService_MountFlagReturnsErrMountsNotSupported (0.00s)

=== RUN   TestDeployService_MountFlagHelpReferencesM2
    deploy_service_test.go:101:
        Error Trace: /Users/fenster/dev/decloud/internal/cli/deploy_service_test.go:101
        Error:       "M1: rejected with ExitConfigError (M3 only)" does not contain "M2 only"
        Test:        TestDeployService_MountFlagHelpReferencesM2
        Messages:    flag help must name the milestone where mounts ship; keep in lockstep with the runtime rejection error and _docs/usage.md
--- FAIL: TestDeployService_MountFlagHelpReferencesM2 (0.00s)

=== RUN   TestStore_LoadRejectsNonEmptyMounts
    store_test.go:297:
        Error Trace: /Users/fenster/dev/decloud/internal/registry/store_test.go:297
        Error:       "registry: mounts not supported in M1: service \"foo\" declares 1 mount(s) in <tempdir>/foo.toml; mounts are not supported until M3" does not contain "mounts are not supported until M2"
        Test:        TestStore_LoadRejectsNonEmptyMounts
        Messages:    loader rejection must name the milestone where mounts ship; keep in lockstep with _docs/usage.md and _ai/decisions/m1-scope.md
--- FAIL: TestStore_LoadRejectsNonEmptyMounts (0.00s)
```

Per-package summary (post-Kent, pre-Rob):

```
ok      github.com/alexander-fenster/decloud/internal/caddy
FAIL    github.com/alexander-fenster/decloud/internal/cli       (TestDeployService_MountFlagReturnsErrMountsNotSupported, TestDeployService_MountFlagHelpReferencesM2)
ok      github.com/alexander-fenster/decloud/internal/config
ok      github.com/alexander-fenster/decloud/internal/deploy
ok      github.com/alexander-fenster/decloud/internal/dockerdrv
ok      github.com/alexander-fenster/decloud/internal/envcap
ok      github.com/alexander-fenster/decloud/internal/ids
ok      github.com/alexander-fenster/decloud/internal/logging
FAIL    github.com/alexander-fenster/decloud/internal/registry  (TestStore_LoadRejectsNonEmptyMounts)
```

## Confirmation: no unrelated tests failing

Three failures, all caused by the new substring assertions checking for "M2" against source bytes still saying "M3". Every other test in every other package passes. The failure messages name the M3-vs-M2 mismatch directly — there is no compile error, no panic, no test setup issue. The red bar is purely the TDD signal Joel and Linus described.

## What Rob does next

Per `005-joel-tech-plan-v2.md` §B.11 / §D, Rob's single atomic commit:

1. `internal/cli/deploy_service.go:61` — `(M3 only)` → `(M2 only)`.
2. `internal/cli/deploy_service.go:72` — `until M3` → `until M2`.
3. `internal/registry/store.go:69` — `until M3` → `until M2`.

After Rob's commit, all three new assertions go green. Rob also runs `gofmt -l .` (must be empty), `go vet ./...` (must be empty), `go generate ./...` followed by `git status --porcelain` (must show only this commit's diffs).

## What Raymond does after Rob

Per Joel's v2 §D, Raymond executes the doc edits in dependency order (B.1 → B.10), with v1 §B.1.5 superseded by v2 §B and v1 §B.1.6 superseded by v2 §C. None of the doc edits affect tests; the test contract is now locked.

## Files relevant to this report (absolute paths)

- `/Users/fenster/dev/decloud/internal/cli/deploy_service_test.go` (extended C.1 + new C.3)
- `/Users/fenster/dev/decloud/internal/registry/store_test.go` (extended C.2)
- `/Users/fenster/dev/decloud/internal/cli/deploy_service.go` (untouched — Rob's job)
- `/Users/fenster/dev/decloud/internal/registry/store.go` (untouched — Rob's job)
- `/Users/fenster/dev/decloud/_ai/cli-flag-surface-coherence.md` (the four-surface contract these assertions lock)
- `/Users/fenster/dev/decloud/_ai/decisions/m1-test-strategy.md` (unit-tests-only strategy these assertions sit inside)
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-milestone-resequence/003-joel-tech-plan.md` §C
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-milestone-resequence/005-joel-tech-plan-v2.md` §D
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-milestone-resequence/006-linus-rereview.md` §C3
