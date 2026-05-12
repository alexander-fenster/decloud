# Joel — final sign-off check (journald log driver)

Post-EXECUTION return to PLAN per workflow. Branch
`task/journald-log-driver`. Diff base: `main`. I am asked whether the
merged work is a faithful realisation of the tech plan I wrote
(REVISION 2 of `03-tech-plan.md`), or whether the gap warrants another
iteration.

I did NOT take Don/Kevlin/Linus at their word. I re-read my own tech
plan §5, §6, §11, §13. I walked the diff against each citation. I
re-ran the tests on the actual disk state. Findings below.

---

## 1. §5 (code shapes) — executed faithfully?

### §5.1 `internal/dockerdrv/driver.go` — YES

The diff at `driver.go:21-34` declares BOTH sentinels in the same
`var (…)` block as `ErrContainerNotFound` / `ErrNoBridgeIP`, with
doc-comments that name the failure mode AND the remediation. The
exact message text shipped is slightly tighter than my §5.1 example:

| What I specified | What shipped |
| --- | --- |
| `"dockerdrv: empty Service field (programmer bug; populate at every Run/RunWithOptions call site)"` | `"dockerdrv: Service is empty; populate Service in RunRequest/RunOptions"` |
| `"dockerdrv: invalid Service field (must not contain '/'; the journald tag \"decloud/<service>\" would otherwise be ambiguous under journalctl CONTAINER_TAG= prefix queries)"` | `"dockerdrv: Service contains '/'; journald tag would be ambiguous under journalctl CONTAINER_TAG= prefix queries"` |

The shipped wording is shorter and ships the same load-bearing
information (the failure mode + the remediation + the downstream
ambiguity). I would have approved either wording. Kent picked tighter
prose; I have no complaint.

`Service string` lands at `driver.go:47` (between `Name` and `Image`
in `RunRequest`) and `driver.go:94` (same placement in `RunOptions`),
exactly per my §11.5 ("physically co-locate related concepts"). The
inline doc-comment is data, not narration. Good.

### §5.2 `internal/dockerdrv/cli_driver.go` — YES, verbatim

Diff at `cli_driver.go:46-58` (Run) and `:218-232` (RunWithOptions):

- Empty-Service guard at the very top.
- Slash-Service guard immediately under it (declared order matches
  §11.1 — cheaper check first, more-common failure first).
- Four new tokens spliced INTO the args literal (not appended), right
  after `--restart`, before any conditional loop. This is the literal
  pattern from §3.2 I asked for, not the appended-flags pattern
  Don's first draft used.
- `strings.TrimPrefix(req.Name, "decloud-")` at the old line 64 is
  now `req.Service` at `cli_driver.go:72`. The smell is gone at the
  one site it lived. `strings` is still imported (used by
  `isNotFound`, `ContainerIP`, and the new `strings.ContainsRune`),
  as my §5.2 footnote predicted.

Both functions read in parallel — same guard pair, same literal-splice
position, same fixed-vs-conditional shape. That is the reading-pattern
discipline I asked for in §11.1; it survived.

### §5.3 / §5.4 / §5.5 — YES, all four production call sites wired

- `internal/deploy/service.go:246` — `Service: req.Name` (fresh deploy).
- `internal/deploy/service.go:379` — `Service: prev.Config.Name`
  (rollback).
- `internal/deploy/lifecycle.go:69` — `Service: name` (absent-branch
  re-run). Rob picked `name` over `prev.Config.Name` per my §11.6.
- `internal/caddy/manager.go:127` — `Service: "caddy"` (caddy manager).

Plus `internal/integration/mount_test.go:69` carries
`Service: "mounttest"` so the integration build still compiles per
§6.6.

### §5.6 — mock regeneration

Adding a struct field does NOT change the `Driver` interface, so the
mock did not need regeneration. Rob confirmed `go generate ./...`
produces no diff. Matches my prediction.

**§5 VERDICT: executed verbatim. No deviations.**

---

## 2. §6 (test surface) — executed faithfully?

### §6.1 fixture updates — YES

All five sub-items (§6.1.1–§6.1.5) shipped:

- `TestCLIDriver_RunArgsWithEnvSorted` — fixture gains `Service:
  "foo"`; expected slice gains the four tokens spliced after
  `--restart`; hand-typed comment at lines 67-69 refreshed.
- `TestCLIDriver_RunArgsWithEmptyEnv` — fixture gains `Service:
  "foo"`; four `assert.Contains` for the journald flags added per
  §6.1.2 (locks the empty-env branch).
- `TestCLIDriver_RunPassesVolumeFlags` — fixture gains `Service:
  "foo"`.
- `TestCLIDriver_RunWithOptionsCaddyShape` — expected slice gains
  the four tokens (`tag=decloud/caddy`); comment refreshed.
- All seven helper-based `RunWithOptions` tests gain `Service: "x"`
  (the bare minimum to avoid `ErrEmptyService` once Rob's guard
  shipped).

### §6.2 new tests — YES, all seven shipped with the EXACT shape I specified

| Test | §6.2 ref | t.Name() match | Load-bearing assertion check |
| --- | --- | --- | --- |
| Empty Service on `Run` | §6.2.1 | YES | `assert.Empty(records)` present ✓; discrimination present ✓ |
| Empty Service on `RunWithOptions` | §6.2.2 | YES | same ✓ |
| Slash on `Run` | §6.2.3 | YES | same ✓ |
| Slash on `RunWithOptions` | §6.2.4 | YES | same ✓ |
| Tag literal `Run` | §6.2.5 | YES | `indexOf` + adjacency check ✓ |
| Tag literal `RunWithOptions` caddy | §6.2.6 | YES | adjacency check ✓ |
| `docker start` NotContains extension | §6.2.7 | YES (extension, not new func) | two NotContains, one per flag ✓ |

The `indexOf` helper at the bottom of the test file is the six-line
package-local version I specified (not `slices.Index` from Go 1.21+).
Reads identically. Fine.

The discrimination assertions (`assert.False(errors.Is(err,
<other>))`) are present on all four rejection tests — the §11.7
two-sentinels promise is locked at the test layer. A future PR that
folds the sentinels into one would fire these assertions.

The `assert.Empty(t, records)` guard-before-exec assertion is present
on all four rejection tests. Without it the tests would pass even
if Rob put the guards after `cmd.Run`. This is the assertion that
distinguishes a real behavioural test from mock-theatre.

The tag-literal tests assert the four tokens appear in declared
contiguous order (`require.Equal(t, driverIdx+2, optIdx, ...)`).
This locks the schema other tools will depend on.

The `TestCLIDriver_StartArgs` extension carries TWO `NotContains` (one
per flag), with failure messages that NAME the invariant
("HostConfig.LogConfig is sealed at create time"). A future
"consistency" refactor that adds log flags to `docker start` argv
would fail two assertions named with what they defend.

### §6.3 / §6.4 / §6.5 — YES

- `internal/deploy/service_test.go` — `seen.Service == "foo"`
  assertion on the fresh-deploy capture; `rollbackSvc` captured from
  the rollback Run closure and asserted `== "foo"`.
- `internal/deploy/lifecycle_test.go` — `seen.Service == "foo"` on
  both `TestLifecycle_StartFromAbsentReRunsContainer` and
  `TestLifecycle_StartAbsentBranchPassesVolumesToDriver`.
- `internal/caddy/manager_test.go` — `expectedCaddyRunOptions` gains
  `Service: "caddy"`; every test using the fixture picks it up via
  gomock `Eq`.

These are ASSERTION extensions, not just field additions — without the
assertions, the field additions would be cosmetic. The assertion is
the contract.

**§6 VERDICT: executed verbatim. All seven new tests, all five
fixture sweeps, all four deploy/lifecycle/caddy assertion extensions.
No deviations.**

---

## 3. §11 (rationale items) — preserved?

All seven §11 rationale points survive in the shipped code:

- §11.1 (literal-splice over append, two-line guard pair preserves the
  two-distinct-sentinels promise) — YES.
- §11.2 (wordier error messages) — shipped wording is TIGHTER than my
  §11.2 example, not wordier; same load-bearing information; either
  shape acceptable.
- §11.3 (regex is help-string-only, not code-enforced; centralised
  validator stays a follow-up) — YES, documented in the decision
  record per Raymond §6.5; backlog entry exists.
- §11.4 (integration test miss) — YES, Kent shipped the compile-fix.
- §11.5 (field placement between `Name` and `Image`) — YES, both
  structs.
- §11.6 (`name` over `prev.Config.Name` in `lifecycle.Start`) — YES,
  Rob picked `name`.
- §11.7 (two sentinels not one; three reasons: test discrimination,
  stack-trace legibility, codebase symmetry) — YES, the assertion
  pattern locks the discrimination.

**§11 VERDICT: rationale preserved. No regressions.**

---

## 4. §13 (file list) — every entry executed?

Walked the diff `git diff main...HEAD --stat` against §13:

| §13 entry | Diff hit | Status |
| --- | --- | --- |
| `internal/dockerdrv/driver.go` (Service field x2 + 2 sentinels) | +16 lines | DONE |
| `internal/dockerdrv/cli_driver.go` (guards + flags + TrimPrefix removal) | +16/-2 lines | DONE |
| `internal/deploy/service.go` (Service on both literals) | +2 lines | DONE |
| `internal/deploy/lifecycle.go` (Service on absent branch) | +1 line | DONE |
| `internal/caddy/manager.go` (Service: "caddy") | +1 line | DONE |
| `internal/integration/mount_test.go` (compile fix) | +1 line | DONE |
| `internal/dockerdrv/cli_driver_test.go` (six tests + StartArgs extension + fixtures) | +178/-10 lines | DONE |
| `internal/deploy/service_test.go` (Service assertions) | +6 lines | DONE |
| `internal/deploy/lifecycle_test.go` (Service assertions) | +3 lines | DONE |
| `internal/caddy/manager_test.go` (fixture update) | +1 line | DONE |
| Mock regen — expected trivial/empty diff | no change | DONE (as predicted) |
| `_docs/usage.md` (3 sections) | +35/-2 lines | DONE |
| `_docs/install.md` (one sentence in §1) | +1 line | DONE |
| `_ai/decisions/journald-log-driver.md` (NEW) | +69 lines new file | DONE |
| `_ai/m1x-backlog.md` (deferred items) | +10 lines | DONE |

Nothing on the §13 list is missing. Nothing landed that I did NOT
specify, except the task-report markdown files themselves and the
`_tasks/current` pointer (one-line workflow churn).

**§13 VERDICT: 15/15 entries executed. Zero unauthorised additions.**

---

## 5. Spot-check: did anything I specified NOT make it?

Walked my own spec for "thou shalt"-style asks and verified each
shipped:

- "Splice the four tokens INTO the literal, not append after" — YES.
- "Empty check first, then slash check, then the args literal" — YES,
  declared order preserved in both `Run` and `RunWithOptions`.
- "Replace `strings.TrimPrefix(req.Name, "decloud-")` with
  `req.Service`" — YES, at `cli_driver.go:72`.
- "Service field BETWEEN `Name` and `Image`" — YES on both structs.
- "Two distinct sentinels; assertion pattern locks discrimination" —
  YES.
- "`docker start` must NOT re-emit log flags; assert this with two
  NotContains" — YES on `TestCLIDriver_StartArgs`.
- "Caddy tag-literal test promoted to REQUIRED" — YES, shipped as
  §6.2.6.
- "Integration test compile-fix" — YES.
- "Hand-typed comments above tests must match the new argv" — YES,
  three test comments refreshed (lines 70, 102, 366 of
  `cli_driver_test.go`).
- "Mock regen should produce trivial diff" — YES, zero diff produced
  (per Rob §6, re-verified by Don).

Nothing I asked for went missing.

## 6. Spot-check: did anything land that I did NOT specify (and would have rejected)?

Walked the diff for files I did NOT name. Hits:

- `_tasks/2026-05-12-journald-log-driver/*.md` — workflow churn,
  expected, not code.
- `_tasks/current` — one-line pointer update, workflow churn.

No production-code or test-code file was touched outside my §13 list.
No "while we're at it" feature crept in. No helper extraction
(despite the journald-token duplication between `Run` and
`RunWithOptions`, which Kevlin §2 and Linus §4.2 BOTH explicitly
recommended NOT extracting — the right call).

**No rejection-worthy additions.**

---

## 7. Kevlin/Linus non-blocking items — should I have caught these in §11?

### 7.1 Message string drift under future `RunRequest`/`RunOptions` consolidation

Kevlin §2 / Linus §4.1 noted that `ErrEmptyService` embeds the literal
phrase "RunRequest/RunOptions" in its message. If backlog item 11
(consolidate the two types) lands, the string drifts.

**Should I have caught this in §11?** Honestly, yes. My §11.2 spec'd
"the audience is the next developer who hits this in a test failure
at 11pm; the wordier message tells them what to do" — and naming the
struct types BY NAME in the message is part of that wordiness. But I
did not flag the forward-compatibility cost with item 11. That is a
gap in my spec.

That said: the cost is one grep at item 11 ship time. The mitigation
Don §9 proposed (a one-line note in the item 11 backlog entry) is
appropriate and can be added at finalisation (STEP 4) without
re-planning. The right place to lock this is the item 11 entry, not
this task's code. So: real miss in my §11, but the consequence is
small enough that it doesn't justify another iteration. I'll log this
as something my future tech plans should anticipate (cross-reference
backlog items that will later need to grep for current decisions).

### 7.2 Duplicated four-token journald splice between `Run` and `RunWithOptions`

Kevlin §2 / Linus §4.2 noted that the four-token splice is duplicated
between the two functions. Both reviewers explicitly recommended NOT
extracting a helper now, because (a) duplication is two lines per
function, (b) helper hides the "fixed argv, like `--restart`"
reading pattern, (c) the duplication evaporates naturally at item 11
ship time.

**Should I have caught this in §11?** I did, implicitly. My §11.1
endorsed the literal-splice pattern over append precisely because it
preserves the "these are fixed flags, not appended-by-loop flags"
reading signal. Extracting a helper would weaken that signal. Not
extracting was the right call, and my §11.1 rationale is
load-bearing for that decision.

If I had been more explicit, I would have added §11.1.1 ("do not
extract a `journaldLogFlags(service) []string` helper now; the
duplication is the right shape until item 11 consolidates the run
paths"). But the rationale is already there in the §11.1 text. Not a
gap, just terser than ideal.

**§7 VERDICT: 7.1 is a real (small) miss in my §11; 7.2 is implicit
but could have been more explicit. Neither warrants another
iteration.**

---

## 8. Tests and code state right now

Re-ran on HEAD:

- `go test -count=1 ./...` — all packages OK; `internal/dockerdrv`
  passes including the six new tests by name.
- `gofmt -l .` — clean.
- `go vet ./...` — clean.

Green on the actual disk state, not a stale report.

---

## 9. Process discipline check

The workflow says PLAN → EXECUTION → PLAN. We are at the
post-EXECUTION PLAN step. The plan iteration converged at REVISION 2
with explicit Linus APPROVED. The implementation followed the plan
verbatim (Rob §6, Kevlin §1-3, Linus §6.1-6.7, Don §1-6, all
independently verified). Kevlin and Linus APPROVED at code-review
time. Don signed off FULLY DONE.

The two non-blocking observations are correctly non-blocking:

- The duplicated four-token splice — recommended NOT to change.
- The `ErrEmptyService` message drift — fixable in 30 seconds at item
  11 ship time; can also be hedged by a one-line backlog note at
  finalisation. Don §2.1 and §9 captured this as a P3 nice-to-have.

There is no new work to plan. Kicking back to PLAN over the message
string would be process theatre over a real-but-tiny gap that has a
better home elsewhere (backlog item 11). Joel's rule: "the cost of
shipping is sometimes a known small debt with a known small cleanup
cost." That is what we have here.

---

## 10. The honest brutal verdict

The merged work IS an accurate, complete realisation of the tech plan
I wrote.

- Every §5 code shape shipped verbatim.
- Every §6 test shipped with the load-bearing assertions intact.
- Every §11 rationale point survives.
- Every §13 file appears in the diff; no unauthorised additions.
- The two reviewer-flagged non-blocking items are correctly
  non-blocking; the one that touches my §11 gap (message-string drift)
  has a home in backlog item 11 and can be hedged with a one-line note
  at finalisation, without re-planning.

The implementation engineers (Kent, Rob) and reviewers (Kevlin, Linus,
Don) did their jobs. The plan iteration caught the substantive issues
(integration-test miss, regex-help-string catch, slash-rejection
invariant, caddy tag-literal test promotion) BEFORE execution started,
which is exactly where they should have been caught. The
post-execution review pass surfaced two appropriately-small items,
both of which are correctly deferred.

I am NOT kicking this back to PLAN. There is nothing material to
re-plan.

The optional finalisation-time touch I'd endorse (matching Don's P3):
add a single line to `_ai/m1x-backlog.md` item 11 reading "during
`RunRequest`/`RunOptions` consolidation, grep
`internal/dockerdrv/driver.go` error-message strings for the phrase
`RunRequest/RunOptions` and update." This addresses my §11 gap
without a code change. Ward can fold it into STEP 4a.

## VERDICT: FULLY DONE
