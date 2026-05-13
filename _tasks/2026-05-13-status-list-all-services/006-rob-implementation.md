# Rob's implementation — `decloud status` lists every registered service

Task slug: `status-list-all-services`. Branch: `task/status-list-all-services`.
Predecessor reports: `002-don-plan.md`, `03-tech-plan.md`, `04-linus-review.md`,
`005-kent-tests.md`.

This is the STEP 3.2 commit. Every test Kent landed in commit `90980c3`
is now green; no test file was modified. The implementation follows
Joel's tech plan §1 verbatim — no deviations, no calls to Donald Knuth
were needed.

## 1. Files changed and why

### 1.1 `internal/registry/store.go`

- Added `ListNames(ctx context.Context) ([]string, error)` to the
  `Store` interface, with the doc comment Joel specified at tech
  plan §1.1 explaining the "no Load, no silent skip" contract distinction
  from `List`.
- Implemented `ListNames` on `fsStore` as the first half of the
  existing `List` (readdir → filter `.toml` / skip `.tmp` and dirs →
  `sort.Strings`). Missing `ServicesDir` returns `(nil, nil)` matching
  `List`'s existing behaviour (Joel §0 refinement).
- Rewrote `List` to call `ListNames` and then `Load` each name in a
  loop. The silent-skip-on-Load-failure `continue` is preserved with
  the load-bearing comment Joel called mandatory:
  `// existing silent-skip contract; Caddyfile path depends on it`.
- Kent's `TestStore_List_StillSilentlySkipsLoadErrors` regression-locks
  this contract.

### 1.2 `internal/deploy/service.go`

- Added `ErrorDetail string` field to the `Status` struct with the
  documented "NOT rendered in stdout; CLI prints it to stderr" comment
  Linus flagged as load-bearing (`04-linus-review.md` §3). Zero-value
  preserved for all existing single-service callers.
- Added `StatusAll(ctx context.Context) ([]Status, error)` to the
  `Lifecycle` interface.

### 1.3 `internal/deploy/lifecycle.go`

- Implemented `(*serviceDeployer).StatusAll` immediately after
  `Status`. Calls `Store.ListNames`, then loops `d.Status(ctx, name)`
  per name. Per-service errors absorb into a row with `State: "error"`
  and `ErrorDetail: err.Error()`; `errors.Is(err, registry.ErrNotFound)`
  matches through the `Status` wrap (`fmt.Errorf("loading service: %w", ...)`)
  and drops the row (concurrent-deploy race policy, tech plan §1.3).
- Host-level `ListNames` failure wraps as
  `fmt.Errorf("listing services: %w", err)` — plain context wrap, no
  sentinel, so `ExitCodeFor` falls through to `ExitInternal` (70) for
  the CLI test that asserts this.

### 1.4 `internal/cli/status.go`

- Whole-file rewrite per Joel §1.4. Surface changes:
  - `Use: "status <name>"` → `Use: "status [name]"`
  - `Short` → `"Show status of one or all registered services"`
  - `Args: cobra.ExactArgs(1)` → `Args: cobra.MaximumNArgs(1)`
  - `RunE` dispatches on `len(args) == 1` to `runStatusOne` or
    `runStatusAll`.
- `runStatusOne` is a verbatim extraction of the existing single-service
  `Fprintf` — byte-for-byte identical to today's output. Kent tightened
  `TestStatus_DelegatesToLifecycleAndPrintsResult` to full-line
  `assert.Equal` (Linus Risk A); it stays green.
- `runStatusAll` uses `tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)` and
  the five-column header `NAME\tSTATE\tCONTAINER\tDEPLOY\tDEPLOYED_AT`.
  Two-pass write: tabwriter rows to stdout first, flush, then stderr
  error-detail lines (`status: <name>: <detail>`) for any row with a
  non-empty `ErrorDetail`. Two-pass ordering is deterministic for
  test capture (Joel §1.4 rationale).
- `dashIfEmpty` and `rfc3339OrDash` helpers are file-private with one
  caller each, as Joel specified.

### 1.5 Regenerated mocks (via `go generate ./...`)

- `internal/registry/mocks/mock_store.go` — gained `ListNames` mock
  method.
- `internal/cli/mocks/mock_lifecycle.go` — gained `StatusAll` mock
  method.
- `internal/cli/mocks/mock_deployer.go` — **no diff** (matches Joel's
  prediction at tech plan §4; the `ServiceDeployer` interface was not
  touched). The "stop if anything else moved" safety check passes.

## 2. Verification

### 2.1 `go test ./...` (final summary, all packages)

```
?   	github.com/alexander-fenster/decloud/cmd/decloud	[no test files]
ok  	github.com/alexander-fenster/decloud/internal/caddy	(cached)
ok  	github.com/alexander-fenster/decloud/internal/cli	0.016s
ok  	github.com/alexander-fenster/decloud/internal/config	(cached)
ok  	github.com/alexander-fenster/decloud/internal/deploy	12.089s
ok  	github.com/alexander-fenster/decloud/internal/dockerdrv	(cached)
ok  	github.com/alexander-fenster/decloud/internal/envcap	(cached)
ok  	github.com/alexander-fenster/decloud/internal/ids	(cached)
ok  	github.com/alexander-fenster/decloud/internal/logging	(cached)
ok  	github.com/alexander-fenster/decloud/internal/registry	0.041s
```

Every Kent test from `005-kent-tests.md` passes:

- `internal/registry` — `TestStore_List_StillSilentlySkipsLoadErrors`,
  `TestStore_ListNames_EmptyDirReturnsEmpty`,
  `TestStore_ListNames_MissingDirReturnsNilNoError`,
  `TestStore_ListNames_FiltersNonTOMLAndInFlightTmpAndSubdirs`,
  `TestStore_ListNames_ResultIsSorted`,
  `TestStore_ListNames_IncludesNamesEvenWhenLoadWouldFail`,
  `TestStore_ListAndListNamesAgreeWhenAllServicesLoadCleanly` — all PASS.
- `internal/deploy` — all eight `TestLifecycle_StatusAll_*` PASS.
- `internal/cli` — `TestStatus_DelegatesToLifecycleAndPrintsResult`
  (tightened to full-line equality, Linus Risk A),
  `TestStatus_NoArgs_DelegatesToStatusAllAndPrintsTable`,
  `TestStatus_NoArgs_EmptyListPrintsHeaderOnly`,
  `TestStatus_NoArgs_StatusAllErrorIsReturnedAndMapsToExitInternal`,
  `TestStatus_NoArgs_RowErrorDetailRoutesToStderrButNotStdout`,
  `TestStatus_TooManyArgsReturnsUsageError` — all PASS.

No previously-green test regressed. The pre-existing
`TestStore_ListSkipsMalformedFiles` and the single-service status path
continue to pass alongside Kent's tighter assertions.

### 2.2 `gofmt -l internal/`

Empty output — every touched file is gofmt-clean.

### 2.3 `%w: %v` discipline check (Joel §5)

```
$ grep -rn '%w: %v' internal/cli/status.go internal/deploy/lifecycle.go internal/registry/store.go
(no matches, exit 1)
```

Zero matches in the touched files. All wraps use `%w` for chain
preservation; the only non-`%w` is `ErrorDetail: err.Error()` inside
`StatusAll`, which is intentional (presentation-string-only, Joel §0
and risk register §4).

### 2.4 Mock diff scope

```
$ git status --porcelain
 M internal/cli/mocks/mock_lifecycle.go
 M internal/cli/status.go
 M internal/deploy/lifecycle.go
 M internal/deploy/service.go
 M internal/registry/mocks/mock_store.go
 M internal/registry/store.go
```

Six files, all expected. No drift in `mock_deployer.go` or any other
generated artefact. Joel's safety check at §4 passes.

### 2.5 `go build ./...`

Clean build, no errors.

## 3. Deviations from Joel's tech plan

None. The implementation follows §1.1 / §1.2 / §1.3 / §1.4 line-for-line.

Three small things worth flagging for the next reviewer (Kevlin /
Linus) — these are matters of execution detail, not departures from
the plan:

1. **Comment wording on `List`'s silent skip.** Joel mandated a comment
   on the silent-skip `continue`. I wrote `// existing silent-skip
   contract; Caddyfile path depends on it`, which matches his
   verbatim sketch at §1.1. The phrase is load-bearing —
   `TestStore_List_StillSilentlySkipsLoadErrors` is the test, but the
   next maintainer who scans `List` without the test in front of them
   needs the comment to not rip the `continue` out as "obvious bug."
2. **Doc comments on the new interface method and `Status` field.**
   Both carry the full doc Joel specified — `ListNames`'s "does NOT
   Load each service" caveat and `ErrorDetail`'s "NOT rendered in
   stdout; CLI prints it to stderr." These are contract surfaces and
   stay verbatim.
3. **Two-pass stdout-then-stderr write in `runStatusAll`.** Joel
   called this deterministic for test capture; Linus flagged it
   non-blocking re: `2>&1` interleaving at the kernel level. I kept
   the two-pass shape; if operators ever complain about `2>&1`
   interleaving we revisit by inlining stderr lines per row.

## 4. What's next

- Raymond updates `_docs/usage.md` §4 / §4.1 / §5 and `README.md` per
  tech plan §8. The state enum across all surfaces is exactly five
  values: `running`, `stopped`, `absent`, `config-only`, `error`.
- Kevlin / Linus run parallel review.
- Don / Joel / Linus iterate at PLAN round 2 (or declare done).

— Rob
