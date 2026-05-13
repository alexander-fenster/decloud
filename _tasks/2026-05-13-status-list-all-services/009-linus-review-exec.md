# Linus's review of execution — `decloud status` lists every registered service

Task slug: `status-list-all-services`. Branch: `task/status-list-all-services`.
Reviewing what landed in commits `90980c3` (Kent), `d86973a` (Rob), `ef20993`
(Raymond) against the plan I approved at `04-linus-review.md`.

I read:
- All three execution reports (`005-kent-tests.md`, `006-rob-implementation.md`, `007-raymond-docs.md`).
- The full diff `git diff main...HEAD` across the touched code, tests, mocks, and docs.
- The actual on-disk source for `internal/cli/status.go`, `internal/deploy/lifecycle.go`, and `internal/registry/store.go` to cross-check the report claims.
- Ran `go build ./...` (clean) and `go test ./internal/cli/... ./internal/deploy/... ./internal/registry/...` (all pass) to verify the green-bar claim.

## Bottom line

**APPROVED. Ship it.**

This is the cleanest STEP-3 execution I've seen on this codebase. Kent wrote
the test list I would have written. Rob shipped Joel's tech plan §1
**verbatim** — I went looking for deviations and found none worth flagging.
Raymond cross-referenced every doc claim back to a code line and listed the
exact `lifecycle.go` line numbers for the five-value state enum.

## Did this ship the right thing?

Yes. The user asked for "a no parameters variant that would list all the
registered services with their statuses." What landed:

- `decloud status` (zero args) → header + one row per registered service, sorted, tabwriter-aligned. Exit 0.
- `decloud status <name>` (one arg) → byte-for-byte identical to the prior single-service output. Existing scripts unaffected.
- `decloud status foo bar` (two args) → routed through `MaximumNArgs(1)` → `isCobraUsageError` → `ExitUsageError` (2). No code change needed in `exit_codes.go` because the substring matcher already covers Cobra's `"accepts"` wording.

The contract that matters — operator runs `decloud status` and sees what's
on the host — is delivered. The contract that *also* matters — the
single-service form does not drift — is locked by the test Kent tightened
to full-line equality. Good.

## Were my two non-blocking concerns addressed?

Both. Explicitly.

### 1. `ErrorDetail string` vs typed `error`

My note in `04-linus-review.md` §3 was "ship it as `string`. The
architectural purity of `error` doesn't earn its keep here." That is what
shipped: `internal/deploy/service.go` adds `ErrorDetail string` with the
exact "NOT rendered in stdout; the CLI prints it to stderr" comment Joel
specified. The field is populated only inside `StatusAll` via
`err.Error()`. It is never re-routed on or compared against; it is pure
presentation text. Right call, kept.

### 2. Single-service status test tightened to full-line equality

This was my non-blocking note at `04-linus-review.md` lines 152-158
(Risk A). Kent took it. The diff at
`internal/cli/lifecycle_commands_test.go:71-78` replaces three
`assert.Contains` substring checks with one `assert.Equal` against the
exact format string:

```
"foo state=running container=decloud-foo deploy=20260426-120000-abc123 deployed_at=2026-04-26T12:00:00Z\n"
```

That is the regression lock I wanted. The `runStatusOne` extraction
cannot drift whitespace, field order, or the RFC3339 format without
tripping this test. Kent's commit message at `005-kent-tests.md` §2.3
even cites my review file line number. Good discipline.

The bonus: Kent also asserts `stderr` is empty on the single-service
success path, which closes a tiny gap I hadn't called out — the
two-pass `runStatusAll` writes to stderr only when `ErrorDetail` is
non-empty, but the single-service path uses `runStatusOne` which
should be silent on stderr regardless. Now it's tested.

## Architectural decisions that diverged from the plan

**None.** I went looking. The implementation report at
`006-rob-implementation.md` §3 says "Deviations from Joel's tech plan:
None." I verified:

- `Use: "status [name]"` — matches `status.go:17`.
- `Args: cobra.MaximumNArgs(1)` — matches `status.go:19`.
- `tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)` — matches `status.go:48`.
- Five-column header `NAME\tSTATE\tCONTAINER\tDEPLOY\tDEPLOYED_AT` — matches `status.go:49`.
- Two-pass stdout-then-stderr ordering — matches `status.go:50-66`.
- `dashIfEmpty` and `rfc3339OrDash` file-private helpers — matches `status.go:70-82`.
- `Status.ErrorDetail string` with the load-bearing comment — matches `service.go:72-76`.
- `Lifecycle.StatusAll` interface entry — matches `service.go:109`.
- `StatusAll` impl with `errors.Is(err, registry.ErrNotFound) → continue` race policy — matches `lifecycle.go:145-147`.
- `ListNames` factored out, `List` rewritten to call it, silent-skip comment in place — matches `store.go:186-218`.

Mock regen scope: exactly three files (`mock_store.go` adds `ListNames`,
`mock_lifecycle.go` adds `StatusAll`, `mock_deployer.go` no diff). Joel's
"stop if anything else moved" safety check passed.

## Are the tests testing the contract or locking the implementation?

Testing the contract. Specifically:

- The CLI tests check **which fields are in the header** (`headerFields()`), **whether a row exists for a given (name, state) pair** (`assertRowPresent` via `strings.Fields`), and **the order of body rows** (`assertBodyRowOrder` via first field). They do **not** assert pixel-exact tabwriter padding. Right call — `text/tabwriter` is stdlib output and locking it is change-detector territory.
- The lifecycle tests (`StatusAll_*`) check the operator-visible contract (per-row error becomes `State="error"` with non-empty `ErrorDetail`, vanished service is dropped, host-level error aborts) without locking the implementation. Kent's choice to substring-match on the `schema_version mismatch` detail rather than on a typed sentinel is exactly Joel's "one synthesized state, detail is presentation-string-only" lock.
- The registry tests check the **silent-skip contract is preserved** (`TestStore_List_StillSilentlySkipsLoadErrors`) and the **new no-skip contract** (`TestStore_ListNames_IncludesNamesEvenWhenLoadWouldFail`) side by side. Two readers of the same registry, two different failure semantics, both tested. That is the architectural keystone of this design and it now has a regression lock.

One thing I particularly liked: Kent's
`TestStore_ListAndListNamesAgreeWhenAllServicesLoadCleanly` pins the
refactor invariant ("when nothing is broken, `List` and `ListNames`
return the same set"). If a future change accidentally drifts the
filter logic in one but not the other, this test catches it. That is
the kind of cross-cutting invariant test that's worth its weight.

## Operational / maintainability gotchas

None that block. Two small observations, neither requires action:

### Observation A: stderr line verbosity

`ErrorDetail` is populated with `err.Error()` from a `Status(ctx, name)`
call that already wraps with `"loading service: %w"`. So a corrupted
TOML produces a stderr line like:

```
status: broken-svc: loading service: decoding registry: schema_version mismatch: ...
```

The double `"loading service: "` prefix is slightly noisy but not
wrong. It's the natural consequence of routing `Status` (which wraps
for its own callers) through the multi-row path. A future cleanup
could strip the inner wrap before stashing into `ErrorDetail`, but
that's a polish, not a bug. Skip.

### Observation B: tabwriter and very long error detail strings

If a `Status` ever produces an error chain that happens to contain a
literal `\t`, tabwriter would interpret it. The current sources of
`ErrorDetail` are wrapped registry / docker errors which never
contain tabs. Not a real problem today. If it ever becomes one, the
fix is one `strings.ReplaceAll(detail, "\t", " ")` — but `ErrorDetail`
is rendered to **stderr**, not through the tabwriter, so this isn't
even a problem for the table. Disregard. Mentioned only for the
record.

## Rough edges a real operator would hit

I asked myself: what would I trip over in production?

1. **Empty registry vs missing services dir produce identical output (header only, exit 0).** Joel and I both signed off on this at plan time (matches `List` semantics, fresh-install path needs it). If an operator deletes `/opt/decloud/config/services` and runs `decloud status`, they get a silent "header only" output. The fix-tomorrow would be a `decloud doctor`/preflight surface, not a `status` surface change. Documented in §4.1 implicitly — left as-is.

2. **`decloud status | grep error` finds the `error` state token AND any service whose name contains "error".** Operators who care about this will write `awk '$2 == "error"'` or `grep '^foo *error '` instead. The column-by-position design makes this trivial. No fix needed.

3. **No JSON output.** Deliberately out of scope (my §"What I want the next-pass agents to NOT do" §2). If someone needs machine-readable, that's one future flag added to one command.

None of these is a real rough edge — they are documented design choices.

## What I want Don and Joel to know

Things I am happy about, in priority order:

1. **The architectural keystone held.** Two readers of the same registry, two failure semantics. `List` keeps its silent-skip for the Caddyfile generator; `ListNames` lets the status surface see broken rows. Locked by two named tests. This is the most important decision in the whole task and it landed exactly as planned.

2. **One synthesized error state, not five.** Joel pushed back on Don, Don accepted, the implementation has one switch arm setting `State: "error"`. Docs list five state values, not nine. Surface coherence checks pass. No drift across stdout / stderr / docs / tests.

3. **Single-service path is bit-for-bit identical.** The `runStatusOne` extraction did not change a single character of the format string. Kent's tightened test will catch any future drift.

4. **No new flags, no `--all`, no `--format=json`.** Surface stays tight. If we want JSON later it is one flag added to one command, not a flag matrix decided today.

5. **Mock regen scope was clean.** Three expected diffs, one of them empty (`mock_deployer.go`). The "stop if anything else moved" safety check Joel wrote was not just paranoia — it's how we know future tasks won't accidentally smuggle in unrelated mock drift.

## Verdict

**APPROVED.**

Top concern: **none, ship it.**

Both non-blocking notes from my plan review (`ErrorDetail` as `string`,
single-service test tightened to full-line equality) were addressed.
Don, Joel: declare done at the next PLAN round.

— Linus
