# Kevlin's review — `decloud status` lists every registered service

Task slug: `status-list-all-services`. Branch: `task/status-list-all-services`.
Predecessor reports: `002-don-plan.md`, `03-tech-plan.md`, `04-linus-review.md`,
`005-kent-tests.md`, `006-rob-implementation.md`, `007-raymond-docs.md`.

Review scope per task brief: STEP 3.4 low-level review of all EXECUTION
output (Kent + Rob + Raymond) against `main`. Special attention to (a)
plan-vs-implementation parity, (b) Go style (`%w`, gofmt, mock regen
correctness), (c) tests as behaviour-asserting not change-detector, and
(d) hallucination-check Raymond's three flagged risk areas.

## Verdict

**APPROVED.** Ship it.

No blockers. One doc nit worth fixing before squash-merge (or in a
follow-up; it does not affect operator-visible behaviour, only the
rendered example in the docs).

## What I verified

### Plan-vs-code parity (Joel §1 walked line-by-line)

- `internal/registry/store.go:38` — `ListNames(ctx) ([]string, error)`
  added to `Store` interface. Doc comment matches Joel §1.1 verbatim
  (the "does NOT Load each service" caveat is present).
- `internal/registry/store.go:186-207` — `(*fsStore).ListNames` is the
  exact readdir → filter → sort → return shape from Joel's sketch.
  `fs.ErrNotExist` returns `(nil, nil)`. Filter is
  `!HasSuffix(.toml) || HasSuffix(.tmp)`. `sort.Strings`.
- `internal/registry/store.go:209-224` — `List` rewritten to call
  `ListNames` then `Load` per name. The load-bearing comment is
  present on the `continue` (`store.go:218`):
  `// existing silent-skip contract; Caddyfile path depends on it`.
- `internal/deploy/service.go:65-77` — `ErrorDetail string` added to
  `Status` with the "NOT rendered in stdout" doc comment Joel
  specified. Zero value preserved for existing callers.
- `internal/deploy/service.go:103-112` — `StatusAll` line added to the
  `Lifecycle` interface, between `Status` and `Logs`.
- `internal/deploy/lifecycle.go:120-158` — `StatusAll` impl matches
  Joel §1.3 line for line. Per-service `ErrNotFound` is dropped
  before the synthesis branch. Host-level failure wraps with
  `fmt.Errorf("listing services: %w", err)` — plain context, no
  sentinel, so `ExitCodeFor` falls through to `ExitInternal` (70)
  exactly as the test pins.
- `internal/cli/status.go` — whole-file rewrite per Joel §1.4.
  `Use: "status [name]"` (line 17), `Args: cobra.MaximumNArgs(1)`
  (line 19), `RunE` dispatch on `len(args) == 1` (lines 25–28),
  extracted `runStatusOne` (lines 33–41), new `runStatusAll`
  (lines 43–68), tabwriter config `(0, 0, 2, ' ', 0)` (line 48),
  two-pass write (stdout flush at 59, then stderr loop at 62–66),
  `dashIfEmpty`/`rfc3339OrDash` helpers private to the file
  (lines 70–82). `runStatusOne`'s `Fprintf` is byte-for-byte
  identical to today's single-service output.

### Generated mocks (`go generate ./...`)

- `internal/cli/mocks/mock_lifecycle.go:115-128` — `StatusAll` mock
  method present with the correct signature
  `func (m *MockLifecycle) StatusAll(ctx context.Context) ([]deploy.Status, error)`.
- `internal/registry/mocks/mock_store.go:87-100` — `ListNames` mock
  method present with the correct signature
  `func (m *MockStore) ListNames(ctx context.Context) ([]string, error)`.
- `git diff` against `main` shows only the three expected mock files
  touched (`mock_lifecycle.go`, `mock_store.go`); no drift in
  `mock_deployer.go` or anywhere else. Joel's "stop if anything else
  moved" safety check passes.

### Go style

- `gofmt -l internal/` — empty output. Every touched file is gofmt-clean.
- Error wrapping discipline (`_ai/error-wrap-discipline.md`): every
  wrap in the changed files uses `%w` for chain preservation. Greps
  for `%w: %v`, `%v: %w`, and trailing `: %v` against
  `internal/cli/status.go`, `internal/deploy/lifecycle.go`, and
  `internal/registry/store.go` all return zero matches. The only
  non-`%w` is `ErrorDetail: err.Error()` in `StatusAll`, which is
  intentional — `ErrorDetail` is a presentation field, not an error
  chain, and Joel locked that decision at tech plan §0.
- Receiver names: `(d *serviceDeployer)` and `(s *fsStore)` — short,
  consistent with the rest of each file.
- Exported-name doc comments: `Status.ErrorDetail`,
  `Store.ListNames`, and `(*serviceDeployer).StatusAll` all carry
  Go-doc-style comments. Each says **why** the contract is what it
  is (not what the code does), which is the right bar.

### Tests are real tests, not change-detectors

- `internal/registry/store_test.go:517-605` — seven new `ListNames`
  cases plus `TestStore_List_StillSilentlySkipsLoadErrors`. Each
  asserts a documented behaviour (filter, sort, missing-dir,
  contract distinction from `List`). The cross-check
  `TestStore_ListAndListNamesAgreeWhenAllServicesLoadCleanly` pins
  the refactor contract — that's a real invariant, not a snapshot.
- `internal/deploy/lifecycle_test.go:462-596` — eight `StatusAll`
  cases. Every one asserts a behaviour from the plan (ordered by
  name, exit→stopped rewrite, absent-state pass-through,
  config-only no-Inspect, per-service error absorption,
  vanished-service drop, host-level abort). The vanished-service
  test (`TestLifecycle_StatusAll_VanishedServiceIsDroppedNotSynthesised`)
  pins the `errors.Is(err, registry.ErrNotFound) → continue` branch
  through the real `Status` wrap, which is the right level of
  precision.
- `internal/cli/lifecycle_commands_test.go:60-216` — full-line
  equality on the single-service path (Linus Risk A locked),
  header-substring assertions on the multi-service path (avoids
  testing stdlib tabwriter output bytes), `assertRowPresent` uses
  `strings.Fields` so the assertion is robust to tabwriter padding
  changes. The `RowErrorDetailRoutesToStderrButNotStdout` test
  explicitly asserts `NotContains` on stdout for the error detail —
  that's a real contract surface (five-column shape on stdout) and
  the test pins it correctly.
- The new test helpers (`headerFields`, `assertHeaderPresent`,
  `assertRowPresent`, `assertBodyRowOrder`, `runningStatus`) live at
  the right abstraction level — one rung below the tests, encoding
  the behavioural contract rather than the rendering details. No
  duplication. No mis-use of `assert.OK` where `assert.Equal` would
  read better.

### Test suite

`go test ./...` — every package green:

```
ok  	github.com/alexander-fenster/decloud/internal/caddy
ok  	github.com/alexander-fenster/decloud/internal/cli
ok  	github.com/alexander-fenster/decloud/internal/config
ok  	github.com/alexander-fenster/decloud/internal/deploy
ok  	github.com/alexander-fenster/decloud/internal/dockerdrv
ok  	github.com/alexander-fenster/decloud/internal/envcap
ok  	github.com/alexander-fenster/decloud/internal/ids
ok  	github.com/alexander-fenster/decloud/internal/logging
ok  	github.com/alexander-fenster/decloud/internal/registry
```

### Raymond's three flagged hallucination risks

1. **Five-value STATE enum (`running`, `stopped`, `absent`, `config-only`, `error`)** — verified at five sites in `internal/deploy/lifecycle.go`:
   - `running` — passthrough from `Driver.Inspect` (`lifecycle.go:108`).
   - `stopped` — rewrite of `exited` (`lifecycle.go:106-107`).
   - `absent` — passthrough from `Driver.Inspect` (`lifecycle.go:108`).
   - `config-only` — `Status` synthesises when `Load` returns `ErrSecretsMissing` (`lifecycle.go:95`).
   - `error` — `StatusAll` synthesises on per-service failure (`lifecycle.go:150`).
   No sixth value exists. No drift between the five values listed in
   `_docs/usage.md:249-255` and the code. Confirmed no hallucination.

2. **Stderr prefix `status: <name>: <detail>`** — `internal/cli/status.go:64`
   reads `fmt.Fprintf(errw, "status: %s: %s\n", st.Name, st.ErrorDetail)`.
   Matches the literal claimed in `_docs/usage.md:240`
   (`status: <name>: <wrapped error text>`). Confirmed no hallucination.

3. **Example output block in §4.1 vs tabwriter `(0, 0, 2, ' ', 0)`** —
   see "nit" below. The example does not match what tabwriter
   actually produces for those exact rows. This is the one finding
   in this review.

## Findings

### BLOCKERS

None.

### Nits (nice-to-have, do not block ship)

#### N1: `_docs/usage.md` §4.1 example padding does not match real tabwriter output

`_docs/usage.md:227-230`:

```text
NAME        STATE    CONTAINER             DEPLOY                  DEPLOYED_AT
bar         stopped  decloud-bar           20260426-102001-aa11bb  2026-04-26T10:20:01Z
broken-svc  error    -                     -                       -
foo         running  decloud-foo           20260426-093214-7f3a9c  2026-04-26T09:32:14Z
```

What `tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)` actually produces
for those exact strings (verified by running the format directly
against the stdlib):

```text
NAME        STATE    CONTAINER    DEPLOY                  DEPLOYED_AT
bar         stopped  decloud-bar  20260426-102001-aa11bb  2026-04-26T10:20:01Z
broken-svc  error    -            -                       -
foo         running  decloud-foo  20260426-093214-7f3a9c  2026-04-26T09:32:14Z
```

The CONTAINER column in the docs example is padded to ~22 chars
(`decloud-bar` + 11 spaces); in reality tabwriter pads it to 13
(`decloud-bar` + 2 spaces, where 2 is the configured `padding`).
The `broken-svc` row's `-` shows the same drift.

Same drift in `_docs/usage.md:285-287` (the §5 end-to-end example):

```text
NAME       STATE    CONTAINER           DEPLOY                  DEPLOYED_AT
myservice  running  decloud-myservice   20260426-093214-7f3a9c  2026-04-26T09:32:14Z
other      stopped  decloud-other       20260425-110000-def456  2026-04-25T11:00:00Z
```

Actual:

```text
NAME       STATE    CONTAINER          DEPLOY                  DEPLOYED_AT
myservice  running  decloud-myservice  20260426-093214-7f3a9c  2026-04-26T09:32:14Z
other      stopped  decloud-other      20260425-110000-def456  2026-04-25T11:00:00Z
```

Off-by-one on the CONTAINER column (3 spaces vs 2).

**Severity**: nit. Raymond acknowledged the approximation in his
report (§5.3) and Joel's tech plan §6.4 explicitly carved tabwriter
column widths out of the test contract. The example is internally
consistent and an operator can read the column labels and won't
miss which value lands where. But "this is exactly what you will
see on stdout" is the implicit contract of an example block, and
the diff is small enough to fix.

**Fix**: paste the two byte-precise blocks above over the current
lines `usage.md:227-230` and `usage.md:285-287`. No code change, no
test change. Trivial to do in this branch or a follow-up.

### Confirmations (things I checked and are fine)

- C1: `Use: "status [name]"` matches across `status.go:17`,
  `README.md:104`, and `_docs/usage.md:194`. Single-service shorthand
  `<name>` still appears at `usage.md:206, 334` referring to the
  one-service form (not the CLI usage string) — that is correct
  usage, not drift.
- C2: Exit-code mapping. `TestStatus_NoArgs_StatusAllErrorIsReturnedAndMapsToExitInternal`
  pins host-level error → 70; `TestStatus_TooManyArgsReturnsUsageError`
  pins `>1 args` → 2. Both match the contract documented at
  `usage.md:245`.
- C3: The `ErrorDetail: err.Error()` string-not-`%w` choice is
  correct (presentation field, not chain). Joel called this out at
  risk register §4; the comment on the struct field at
  `service.go:72-76` documents it.
- C4: The silent-skip contract on `Store.List` is preserved at
  `store.go:217-219`, with the load-bearing comment Joel specified
  and the regression test `TestStore_List_StillSilentlySkipsLoadErrors`
  at `store_test.go:517-528`.
- C5: No layering violation. `internal/cli/status.go` imports
  `deploy` (for `deploy.Lifecycle`) but NOT `registry` directly. The
  `ListNames`-vs-`List` distinction lives in the registry package
  where it belongs.
- C6: No new dependencies introduced. `text/tabwriter` is stdlib.
- C7: No comments-as-symptoms-of-bad-structure anywhere in the diff.
  The two long doc comments (`StatusAll`, `ListNames` interface,
  `Status.ErrorDetail`) explain **why** the contract is what it is —
  they document operator-visible behaviour and the
  `ErrNotFound`-drop policy. Removing them would erase real design
  rationale, not redundant prose.
- C8: No TODOs, no skipped tests, no commented-out assertions, no
  debug logging in the changed files.
- C9: Tests are placed in the right packages (registry tests in
  `internal/registry`, lifecycle tests in `internal/deploy`, CLI
  tests in `internal/cli`) — none of this needs to be in
  `integrationtests/`.

## What's next

PLAN round 2: Don/Joel/Linus reconvene. The doc nit above does not
require revision — it is a small follow-up at most. From a low-level
review perspective this branch is ready to squash-merge.

— Kevlin

## Lap-2 verification

Don promoted N1 to a blocker; Raymond shipped the fix in HEAD
(`674210661`). Re-ran the two row sets through
`tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)` (the exact config at
`internal/cli/status.go:48`) and `diff -u`'d against
`_docs/usage.md` lines 227-230 and 285-287 — byte-exact match.
`git show HEAD --name-only` confirms scope: only `_docs/usage.md`
and `012-raymond-padding-fix.md` were touched. **PADDING NIT
RESOLVED.**
