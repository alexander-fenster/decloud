# Joel's final sign-off — FULLY DONE

**Verdict: FULLY DONE.** Ship it.

---

## Verification against the four required gates

### 1. `internal/cli/deploy_service.go:55` reads `"container listen port (required)"`

Confirmed by direct read of the file. Line 55 in the working tree:

```go
cmd.Flags().IntVar(&f.Port, "port", 0, "container listen port (required)")
```

The stale "(required if --host set)" parenthetical that Kevlin and Linus
both flagged in `08-kevlin-review.md` §"Issue 1" and `09-linus-review-impl.md`
§"Issue 1 (BLOCKING)" is gone. CLI `--help` output now matches `_docs/usage.md`
(`Required: yes`) and runtime behaviour (`if f.Port == 0` unconditional check
at line 73). The CLI/docs/help triangle is coherent.

### 2. All three iter1 production fixes intact

I read each production file. Each fix is exactly where the plan put it,
unchanged from iter1:

- **Finding 1 (dockerfile resolution).** `deploy_service.go:80-86` — six-line
  block: empty-string guard → `IsAbs` short-circuit → `Join(abs, ...)`. Line 95
  uses the resolved `dockerfile` variable in the `deploy.Request`. Order
  preserved.
- **Finding 2 (explicit logging input).** `logging/logging.go:22` signature is
  `Init(root string) error`; line 27-29 falls back to `config.DefaultRoot` on
  empty-string root; stderr short-circuit at line 23-26 still precedes any
  filesystem access. `cli/root.go:23` calls `logging.Init(rc.ConfigRoot)`. The
  flag-wins-over-env precedence is preserved at the Cobra layer
  (`root.go:26-27`).
- **Finding 3 (unconditional --port).** `deploy_service.go:73-75` —
  `if f.Port == 0 { return fmt.Errorf("--port is required: %w", errUsage) }`.
  Wraps `errUsage`; mapped to `ExitUsageError` by `exit_codes.go`.

The validation order (mount → strategy → port → abs → envFile → dockerfile →
request) is preserved. None of the typed sentinels (`ErrMountsNotSupported`,
`ErrInvalidStrategy`) got shadowed by the new `--port` check.

### 3. All tests pass

`go test ./... -count=1`: tree-wide green. `gofmt -l internal cmd`: empty.
`go vet ./...`: empty. Identical to iter1's pass set, as the iter2 tech plan
predicted (no test asserts on the help string).

### 4. No technical debt or unfinished business

I checked for the temptations one more time. None taken:

- No `MarkFlagRequired("port")` (Cobra can't distinguish unset from
  `--port=0`; explicit RunE check is more robust).
- No env-fallback inside `logging.Init` (would defeat the explicit-input
  contract).
- No `port == 0` short-circuit in `internal/deploy/service.go` or
  `internal/deploy/readiness.go` (policy stays at the CLI boundary).
- No refactor of `runDeployService` validation into a helper (three checks,
  one function, fine where it is).
- No registry round-trip path-shape test (would be a change-detector test;
  CLAUDE.md bans those).

This task **retired** two pieces of debt and **introduced** zero:

- The hidden-input pattern in `logging.Init` (env read inside the function)
  is gone. New code added later cannot accidentally re-introduce the bug
  because the input is now a function argument.
- The CLI/docs drift on `--dockerfile` semantics is closed, with regression
  tests on both sides locking the contract in.

The only debt that survived iter1 — the stale `--port` flag help text — was
retired in iter2 by Rob's one-line fix. Linus's BLOCKING issue is closed.

---

## Non-blocking observations (deferred, acceptable)

Two items remain in the "noted, not blocking, do not fix" bucket. Both were
explicitly accepted by Linus in `09-linus-review-impl.md`:

1. **`TestDeployService_HostWithoutPortReturnsExitUsageError` mild misnomer.**
   After the diff, this test passes through the generic `--port is required`
   branch rather than a host-specific branch. The assertion
   (`ExitCodeFor(err) == ExitUsageError`) is still correct under the new
   semantics, and the scenario (operator passes `--host` but forgets `--port`)
   is still a real-world misuse case worth a test. Linus's "Option B (leave)"
   recommendation stands. No action.

2. **`_ai/cobra-init-pattern.md` pseudo-Go in a warning.** The text
   "Do not collapse to `Init(string) {}`" uses pseudo-Go that wouldn't compile
   (`{}` with no `error` return). Trivial doc nit. Raymond can fix on next
   touch; otherwise leave.

Neither blocks ship.

---

## Diff scope

```
 _ai/cobra-init-pattern.md           |  17 +++--
 _ai/m1x-backlog.md                  |   2 +-
 _docs/usage.md                      |   6 +-
 internal/cli/deploy_service.go      |  15 ++--
 internal/cli/deploy_service_test.go | 133 +++++++++++++++++++++++++++++++++++-
 internal/cli/root.go                |   2 +-
 internal/cli/root_test.go           |  34 ++++++++-
 internal/logging/logging.go         |  15 ++--
 internal/logging/logging_test.go    |  34 ++++++---
```

Three production files, three test files, three doc files. No surprise paths.
`internal/dockerdrv/`, `internal/deploy/`, `internal/registry/`, `cmd/decloud/`
all untouched. Scope discipline held.

Eleven net production lines (plus iter2's one-line help-text fix), three
layers, three findings, no new abstractions, no new flags, no new files.

---

## Sign-off

All three findings (CLI/docs Dockerfile drift, hidden env input in logging,
silent host-without-port acceptance) are properly closed. CLI, runtime, docs,
AI notes, and `--help` output are coherent at every boundary. Tests lock in
the contracts.

Ready for FINALIZATION. Pass to Ward for knowledge preservation.

— Joel
