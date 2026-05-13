# Joel's final verdict — `decloud status` lists every registered service

Task slug: `status-list-all-services`. Branch: `task/status-list-all-services`.
Predecessor reports read: `01-user-request.md`, `002-don-plan.md`,
`03-tech-plan.md` (mine), `04-linus-review.md`, `005-kent-tests.md`,
`006-rob-implementation.md`, `007-raymond-docs.md`, `009-linus-review-exec.md`,
`010-kevlin-review.md` (with Lap-2 verification section),
`011-don-round2.md`, `012-raymond-padding-fix.md`, `013-don-final.md`.

## TL;DR

**VERDICT: DONE.** The shipped code matches my tech plan line-for-line.
The round-2 doc nit (Don's gate) was fixed and Kevlin verified it
byte-exact at Lap-2. `go build ./...` and `go test ./...` both green
on HEAD. Proceed to STEP 4 (Ward learnings, Andy agent review,
squash-merge to `main`).

## Plan-vs-code parity — what I re-checked just now

I refused to take the prior reviews on faith. I diffed each of the
four §1 sites in my tech plan against HEAD and verified the bytes.

### §1.1 `internal/registry/store.go`

- `Store` interface gains `ListNames(ctx) ([]string, error)`. Doc
  comment matches `03-tech-plan.md` §1.1 verbatim ("does NOT Load
  each service" caveat present).
- `(*fsStore).ListNames` is the readdir → filter → sort → return
  shape from my sketch. `fs.ErrNotExist` returns `(nil, nil)`. Filter
  is `!HasSuffix(.toml) || HasSuffix(.tmp)`. `sort.Strings`.
- `List` is rewritten to call `ListNames` then `Load` per name. The
  load-bearing comment is present on the `continue`:
  `// existing silent-skip contract; Caddyfile path depends on it`.
  This is the regression-lock comment I made mandatory in §1.1 notes.

### §1.2 `internal/deploy/service.go`

- `Status.ErrorDetail string` added with the "NOT rendered in
  stdout" doc comment I specified. Type is `string`, not `error` —
  the locked-in decision from §0 holds.
- `StatusAll(ctx) ([]Status, error)` line added to `Lifecycle`
  interface, between `Status` and `Logs`. Exact placement.

### §1.3 `internal/deploy/lifecycle.go`

- `StatusAll` impl matches my §1.3 sketch line-for-line. Concurrent-
  deploy race policy honoured (`errors.Is(err, registry.ErrNotFound)
  → continue` is checked before the synthesis branch). Host-level
  failure wraps with `fmt.Errorf("listing services: %w", err)` —
  plain context, no sentinel, so `ExitCodeFor` falls through to
  `ExitInternal` (70) as designed in §1.3 notes.
- Capacity hint `make([]Status, 0, len(names))` present.
- No per-row logging inside `StatusAll` (CLI surfaces detail to
  stderr) — as I locked in §1.3 notes.

### §1.4 `internal/cli/status.go`

- `Use: "status [name]"` (NOT `<name>`, NOT `[name...]`) — exact.
- `Short: "Show status of one or all registered services"` — exact.
- `Args: cobra.MaximumNArgs(1)` — exact.
- `RunE` dispatches on `len(args) == 1` to `runStatusOne` else
  `runStatusAll` — exact two-arm shape from §1.4.
- `runStatusOne` is a verbatim extraction of the original single-
  service Fprintf. The format string `"%s state=%s container=%s
  deploy=%s deployed_at=%s\n"` and arg order are bit-for-bit
  identical to today's output. The §10 quality-bar guarantee holds.
- `runStatusAll` uses `tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)` —
  exact config from §0 refinement.
- Two-pass over `statuses` (tabwriter flush first, then stderr loop)
  — exact, deliberate, locked in §1.4 notes against the buffer-race
  concern.
- Helpers `dashIfEmpty` and `rfc3339OrDash` are file-private with one
  caller each — exact.

### §4 Mock regen scope

`git diff main...HEAD --stat -- internal/registry/mocks/
internal/cli/mocks/` shows exactly:

- `internal/cli/mocks/mock_lifecycle.go` — +15 lines (StatusAll
  mock method).
- `internal/registry/mocks/mock_store.go` — +15 lines (ListNames
  mock method).
- `internal/cli/mocks/mock_deployer.go` — **not in the diff list at
  all** (diff-empty, as predicted in §4 of my tech plan).

Zero unrelated mock drift. The §9 risk #1 ("mock regen surprise") is
clean.

### §5 `%w: %v` grep — clean

`grep -rn '%w: %v' internal/cli/status.go internal/deploy/lifecycle.go
internal/registry/store.go` returns zero matches. My §5 grep-clean
verification holds.

## Build and test status

`go build ./...` → green.
`go test ./...` → all packages green (cli, deploy, registry, plus
the seven unchanged packages).

## The round-2 doc nit — closed

I read `011-don-round2.md` and `012-raymond-padding-fix.md` and the
Lap-2 verification section of `010-kevlin-review.md`.

- Don's verdict: ITERATE on two example blocks in `_docs/usage.md`
  (operator-visible docs that drifted from actual tabwriter output).
- Raymond's fix: byte-exact paste of Kevlin's verified blocks at
  `_docs/usage.md` lines 227-230 and 285-287.
- Kevlin's Lap-2: re-ran the rows through the actual
  `tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)` config and `diff -u`'d
  against the docs — byte-exact match. Scope of the fix commit
  (`6742106`) is exactly two files: `_docs/usage.md` and
  `012-raymond-padding-fix.md`. **PADDING NIT RESOLVED.**

I have nothing to add to this. Don's gate is satisfied.

## Other gaps I went hunting for and did not find

I went looking for excuses to flip to ITERATE. None held up:

1. **Single-service path regression.** Diffed `runStatusOne`'s
   Fprintf against the pre-task version. Identical. The §2.5
   "what does NOT change" guarantee holds.
2. **State enum drift.** Five values: `running`, `stopped`,
   `absent`, `config-only`, `error`. Verified across
   `internal/deploy/lifecycle.go` and `_docs/usage.md`. Kevlin
   already hallucination-checked this at five code sites in his
   first review. The trim from Don's nine to my five (§0 pushback
   #1) survived intact.
3. **Two-args usage error routing.** `cobra.MaximumNArgs(1)`
   produces `"accepts at most 1 arg(s)..."`; `isCobraUsageError`
   matches `"accepts"`; `ExitCodeFor` returns `ExitUsageError` (2).
   Verified in `03-tech-plan.md` §0 and locked by Kent's
   `TestStatus_TwoArgs_FailsWithUsageError`.
4. **`Status` struct TOML roundtrip.** `Status` is in
   `internal/deploy/`, never marshalled to TOML. Adding
   `ErrorDetail` is zero-impact on persistence. §9 risk #5 clean.
5. **The Caddyfile silent-skip contract.** `regenerateAndReload`
   still calls `Store.List`, which still silently skips Load
   failures. Kent's `TestFSStore_List_StillSilentlySkipsLoadErrors`
   regression-locks it. §9 risk #7 clean.

## Final verdict

**VERDICT: DONE.**

The shipped implementation matches my tech plan with zero drift.
Mock regen scope is exactly the three files I predicted (with the
one I predicted to be diff-empty actually being diff-empty). The
`%w: %v` discipline is clean. Build and tests are green. The one
round-2 iteration item (Don's doc-padding gate) was fixed and
byte-verified by Kevlin at Lap-2.

Per CLAUDE.md, all three of Don/Joel/Linus must agree FULLY DONE.
Linus APPROVED at execution. Don's round-2 verdict (post-fix) is
in `013-don-final.md`. My verdict is here. If Don's `013-don-final.md`
also says DONE, we proceed to STEP 4 — Ward (learnings), Andy
(agent instruction review), squash-merge to `main` with a
conventional commit.

— Joel
