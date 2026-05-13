# Kent's failing tests — `decloud status` lists every registered service

Task slug: `status-list-all-services`. Branch: `task/status-list-all-services`.
Predecessor reports: `002-don-plan.md`, `03-tech-plan.md`, `04-linus-review.md`.

This is the test-first commit for STEP 3.1. Production code is untouched —
Rob owns STEP 3.2. The tests below were written to fail in the
"missing-implementation" mode Don and Joel planned for.

## 1. Files touched

- `internal/registry/store_test.go` — added `ListNames` coverage and a
  regression lock on `List`'s existing silent-skip contract.
- `internal/deploy/lifecycle_test.go` — added the seven-test `StatusAll`
  suite from tech plan §6.2. Imported `fmt` for error-chain construction.
- `internal/cli/lifecycle_commands_test.go` — tightened the existing
  single-service status test to full-line equality (Linus risk A,
  `04-linus-review.md` line 156) and added five new tests for the no-arg
  path. Imported `errors`, `strings`.

Production code (`internal/registry/store.go`, `internal/deploy/service.go`,
`internal/deploy/lifecycle.go`, `internal/cli/status.go`) and the generated
mocks (`internal/registry/mocks/mock_store.go`,
`internal/cli/mocks/mock_lifecycle.go`) are **deliberately unchanged**.
Mock regen is Rob's first step after he adds the interface methods (tech
plan §4).

## 2. What is asserted, per layer

### 2.1 `internal/registry/store_test.go`

- `TestStore_List_StillSilentlySkipsLoadErrors` — regression lock on the
  documented Caddyfile-regen contract: corrupt TOML for one of two
  services produces a one-service result with `err == nil`. Joel §6.1
  asks for this explicitly because the existing
  `TestStore_ListSkipsMalformedFiles` asserts the same intent but is named
  for the malformed-file scope; this new name pins the contract.
- `TestStore_ListNames_EmptyDirReturnsEmpty` — empty `ServicesDir` →
  `(empty, nil)`.
- `TestStore_ListNames_MissingDirReturnsNilNoError` — missing
  `ServicesDir` (a fresh `t.TempDir()` with no MkdirAll) → `(nil, nil)`
  (Joel §0 refinement; matches `List`).
- `TestStore_ListNames_FiltersNonTOMLAndInFlightTmpAndSubdirs` — exercises
  both halves of the filter (`!HasSuffix(.toml)` AND `HasSuffix(.tmp)`)
  and the directory skip. Plant `alpha.toml`, `beta.toml`,
  `alpha.toml.tmp`, `README`, `nested-dir/` → result is `[alpha, beta]`.
- `TestStore_ListNames_ResultIsSorted` — byte-order sort (Joel §3.6).
- `TestStore_ListNames_IncludesNamesEvenWhenLoadWouldFail` — the
  load-time error policy lives in `StatusAll`, not in `ListNames`. The
  broken service's name must appear so the status surface can synthesise
  an error row for it.
- `TestStore_ListAndListNamesAgreeWhenAllServicesLoadCleanly` —
  cross-check: when nothing is corrupt, the two methods produce the same
  set. Pins the refactor contract from §1.1 of the tech plan.

### 2.2 `internal/deploy/lifecycle_test.go`

All seven cases from tech plan §6.2, plus the two small helpers
`serviceNamed(name)` and `statusByName(statuses, name)` to keep the bodies
readable without duplicating fixture setup.

- `TestLifecycle_StatusAll_EmptyRegistryReturnsEmptySlice` — `ListNames`
  returns `(nil, nil)` ⇒ zero rows, zero Inspect calls (gomock fails the
  test if Inspect is invoked unexpectedly).
- `TestLifecycle_StatusAll_HappyPathOrderedByName` — two services, the
  `exited → stopped` rewrite from the single-service path is preserved.
- `TestLifecycle_StatusAll_AbsentContainerSurfacesAbsentState` — registry
  entry present but container gone: state `absent`, empty `ContainerID`.
- `TestLifecycle_StatusAll_ConfigOnlyOrphanSkipsInspect` — `Load` returns
  `ErrSecretsMissing` ⇒ row is `config-only`, **no Inspect call**,
  `ErrorDetail` empty (this is a documented row state, not an error row).
- `TestLifecycle_StatusAll_PerServiceLoadErrorBecomesErrorRowWithoutAborting`
  — three services, middle one's `Load` fails with a wrapped
  `ErrSchemaMismatch`. All three rows present, in order; middle row has
  `State == "error"` and `ErrorDetail` contains `schema_version mismatch`.
  This is the operational bar (no lying by omission, no aborting on one
  bad service).
- `TestLifecycle_StatusAll_PerServiceInspectErrorBecomesErrorRow` —
  Inspect failure for one of two services produces an `error` row;
  other service unaffected.
- `TestLifecycle_StatusAll_VanishedServiceIsDroppedNotSynthesised` —
  the concurrent-deploy race policy (§1.3 of tech plan): `Load` returns
  `ErrNotFound` ⇒ row is **dropped**, not synthesised as `error`.
- `TestLifecycle_StatusAll_ListNamesFailureAbortsAndPropagates` —
  host-level failure aborts; no Load/Inspect calls; error text preserved
  through the wrap.

### 2.3 `internal/cli/lifecycle_commands_test.go`

- `TestStatus_DelegatesToLifecycleAndPrintsResult` — **tightened** to a
  full-line `assert.Equal` on stdout (was `assert.Contains` on three
  substrings). Linus flagged this as the one non-blocking thing to fix
  during the `runStatusOne` extraction (`04-linus-review.md` line 156,
  Risk A). Also asserts stderr is empty on the success path.
- `TestStatus_NoArgs_DelegatesToStatusAllAndPrintsTable` — two rows
  (`bar` stopped, `foo` running). Header has the five documented field
  names; both rows appear; body row order matches `StatusAll`'s return
  order; stderr empty. **Not** asserting tabwriter column widths — those
  are stdlib output, not our contract (avoid change-detector territory
  per CLAUDE.md).
- `TestStatus_NoArgs_EmptyListPrintsHeaderOnly` — `StatusAll` returns
  `(nil, nil)` ⇒ header is on stdout, body has zero lines. Verifies the
  grep/awk-friendly zero-services design (no `(no services registered)`
  sentinel).
- `TestStatus_NoArgs_StatusAllErrorIsReturnedAndMapsToExitInternal` —
  host-level `StatusAll` error is returned from `RunE` and `ExitCodeFor`
  maps it to `ExitInternal` (70). Matches Joel §3 risk B and the
  exit-code analysis in tech plan §6.3.
- `TestStatus_NoArgs_RowErrorDetailRoutesToStderrButNotStdout` — one
  good row + one row with `State:"error"` and `ErrorDetail` populated.
  Asserts: both names on stdout; the error-detail substring is on stderr
  AND **not** on stdout (the five-column shape is the stdout contract);
  the failing service's name appears on stderr for grep-ability.
- `TestStatus_TooManyArgsReturnsUsageError` — two positional args ⇒
  `ExitCodeFor == ExitUsageError`. Locks the
  `MaximumNArgs(1)` → `isCobraUsageError("accepts")` path Joel verified at
  tech plan §0. (Note: this test would also pass under the current
  `ExactArgs(1)`, because Cobra emits the same `"accepts"` substring for
  both — that's the regression value, not a current-behaviour gap.)

Helpers added to the CLI test file:

- `runStatusNoArgs(t)` — drops the variadic args, returns string buffers.
- `headerFields()` — single source for the five column names.
- `assertHeaderPresent(t, stdout)` — substring-checks each header field
  without locking exact whitespace (tabwriter is stdlib; its output
  bytes are not our contract).
- `assertRowPresent(t, stdout, name, state)` — finds a body row by
  (name, state) via `strings.Fields` so it's robust against tabwriter
  padding changes.
- `assertBodyRowOrder(t, stdout, names...)` — asserts the order of body
  rows by first-field name, again ignoring whitespace.
- `runningStatus(name)` — fixture for a healthy row, reused by two
  tests.

These helpers are deliberately at the "build a parser for one specific
tabular format" abstraction level — one rung below the tests — and
encode the behavioural contract (header field names, row identity)
rather than the rendering details (column padding, tab placement).

## 3. Observed failure mode

`go test ./...` from repo root, with no production-side changes, fails
cleanly in three packages with the expected "missing-implementation"
errors:

```
# github.com/alexander-fenster/decloud/internal/registry_test
internal/registry/store_test.go:533:22: store.ListNames undefined
internal/registry/store_test.go:543:22: store.ListNames undefined
internal/registry/store_test.go:557:22: store.ListNames undefined
internal/registry/store_test.go:568:22: store.ListNames undefined
internal/registry/store_test.go:579:22: store.ListNames undefined
internal/registry/store_test.go:594:22: store.ListNames undefined

# github.com/alexander-fenster/decloud/internal/deploy_test
internal/deploy/lifecycle_test.go:464:19: h.store.EXPECT().ListNames undefined
internal/deploy/lifecycle_test.go:466:24: h.lc.StatusAll undefined
... (and similar for every StatusAll test) ...

# github.com/alexander-fenster/decloud/internal/cli
internal/cli/lifecycle_commands_test.go:150:16: mock.EXPECT().StatusAll undefined
internal/cli/lifecycle_commands_test.go:193:40: unknown field ErrorDetail in struct literal of type deploy.Status
```

That maps 1:1 to Joel's tech plan §1 (`registry.Store.ListNames`,
`deploy.Lifecycle.StatusAll`, `deploy.Status.ErrorDetail`), §4 (mock
regen needed for `MockStore` and `MockLifecycle`). Production builds
clean (`go build ./...` returns no errors). All non-touched test
packages still pass (caddy, config, dockerdrv, envcap, ids, logging).

`gofmt -l internal/` reports no diffs.

## 4. Notes for Rob

- After your interface changes land, run `go generate ./...` once from
  repo root. The three expected mock diffs are:
  `internal/registry/mocks/mock_store.go` (adds `ListNames`),
  `internal/cli/mocks/mock_lifecycle.go` (adds `StatusAll`),
  `internal/cli/mocks/mock_deployer.go` (no diff — `ServiceDeployer`
  unchanged). Joel's tech plan §4 has the "stop if anything else moved"
  safety check.
- The vanishing-service test
  (`TestLifecycle_StatusAll_VanishedServiceIsDroppedNotSynthesised`)
  pins the `errors.Is(err, registry.ErrNotFound) → continue` branch in
  `StatusAll`. The wrap I use in the mock matches what `Status` does
  today (`fmt.Errorf("loading service: %w", ...)`), so your
  `errors.Is` will succeed through the chain. Don't simplify this to a
  `==` check.
- The per-service Load-error test wraps with `%w` on
  `registry.ErrSchemaMismatch`. The assertion only checks the
  `ErrorDetail` substring `schema_version mismatch`, not the sentinel.
  This is on purpose (Joel §0: one synthesised state, the detail is
  presentation-string-only).
- The CLI "exit internal" test uses a bare `errors.New(...)` so the
  default case in `ExitCodeFor` returns `ExitInternal`. Don't add a
  typed sentinel to the host-level `ListNames` wrap on Rob's path —
  Joel's analysis at §1.3 is that this should fall through to 70.
- The single-service status test is now full-line equality. Any
  whitespace, field-order, or RFC3339-format drift in `runStatusOne`'s
  Fprintf will trip it. That's intentional.

## 5. Anything Don/Joel/Linus missed

Nothing material. Three minor observations during test-writing — none
warrant Donald Knuth:

1. **`ListNames` filter robustness.** Joel's tech plan §1.1
   implementation sketch filters via `!HasSuffix(.toml) ||
   HasSuffix(.tmp)`. Since a `.tmp` file won't have a `.toml` suffix
   either (it's `<name>.toml.tmp` per `writeAtomic`), the `.tmp` branch
   alone is what catches them. I added `alpha.toml.tmp` in the filter
   test to exercise both legs of the `||`. No behaviour change; just
   wanted the test to actually cover the second condition rather than
   trusting the first to short-circuit on every realistic input.

2. **Empty-list rendering.** With tabwriter and only a header row, the
   header still gets padded to the field widths. My
   `EmptyListPrintsHeaderOnly` test uses `strings.Count(stdout, "\n")`
   after trimming the trailing newline rather than a byte-equal
   comparison, so any reasonable padding choice passes. If Rob picks a
   different tabwriter config (he won't — Joel locked it at
   `(0, 0, 2, ' ', 0)`), this test still holds.

3. **Test seam reuse.** The CLI tests use the existing
   `installMockLifecycle` test seam (same as the single-arg path) and
   the existing `runRoot` harness — no new seams, no parallel
   factories. Tech plan §5 called this out as the right reuse pattern;
   confirmed in practice.

— Kent
