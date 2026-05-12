# Don — final sign-off check (journald log driver)

Post-EXECUTION return to PLAN step per workflow. Branch
`task/journald-log-driver` at HEAD `502bbf3`. Diff base: `main`.

Question: is this FULLY DONE, or does it need another iteration?

I read all nine prior reports (`01-user-request.md` through
`09-linus-review.md`), re-walked the live diff (`git diff main...HEAD`)
on every source and docs file, re-ran `go test -count=1 ./...`,
`gofmt -l .`, `go vet ./...`, `go vet -tags=integration ./...`, and
`go build -tags=integration ./...`. I did NOT take Kevlin and Linus
at their word — I checked the code myself.

---

## 1. Does the merged code actually achieve the user's outcome?

The user asked for: every Decloud container started with
`--log-driver=journald --log-opt tag=...` so logs survive container
redeployment, with `decloud logs` continuing to work and cross-redeploy
history queryable via `journalctl`.

Walked the evidence:

- **Every `docker run` chokepoint emits the flags.** Verified at
  `internal/dockerdrv/cli_driver.go:58-59` (Run) and `:232-233`
  (RunWithOptions). `grep -n 'log-driver\|log-opt'` on the file
  returns exactly four hits at those lines. The two-functions
  enumeration from Don §2.1 is still complete — no third `docker
  run` site exists.
- **Every caller populates Service explicitly.** Verified at
  `internal/deploy/service.go:246` (fresh deploy: `req.Name`),
  `:379` (rollback: `prev.Config.Name`), `internal/deploy/lifecycle.go:69`
  (absent-branch re-run: `name`), `internal/caddy/manager.go:127`
  (caddy: hardcoded `"caddy"`). Four production sites; all wired.
- **`decloud logs` keeps working.** `Driver.Logs` at
  `cli_driver.go:148-174` is unchanged; journald is one of Docker's
  dual-read drivers, so `docker logs` still works against the
  underlying journal. Unit-test surface locks the argv shape; the
  manual-Linux-smoke surface (acceptance criteria 3 and 4) confirms
  the journald behaviour on a real host.
- **Tag format is `decloud/<service>` literal.** Locked by two
  separate tag-literal tests
  (`TestCLIDriver_RunEmitsJournaldFlagsWithSlashTagLiteral` and
  `TestCLIDriver_RunWithOptionsEmitsJournaldFlagsWithCaddyTag`),
  the latter specifically protecting the `decloud/caddy` literal on
  the `RunWithOptions` path.
- **`docker start` does NOT re-emit log flags.** Locked by two
  `assert.NotContains` on the existing `TestCLIDriver_StartArgs`
  (one for `--log-driver`, one for `--log-opt`). HostConfig.LogConfig
  is sealed at create time; this regression class is now blocked.

VERDICT on user intent: SHIPPED. The user gets the exact shape they
asked for.

## 2. Anything Kevlin or Linus flagged as "non-blocking" that actually blocks?

I scrutinised the three non-blocking items the post-impl reviewers
surfaced:

### 2.1 `ErrEmptyService` message embeds "RunRequest/RunOptions"

(Kevlin §2, Linus §4.1.) The string drifts only IF backlog item 11
(consolidate `RunRequest` and `RunOptions`) lands. Today the message
is accurate. Mitigation options:

- A. Defer with a one-line note in backlog item 11 ("grep for
  RunRequest/RunOptions in error message strings during
  consolidation").
- B. Reword now to "populate Service in the run-request literal."
- C. Defer with no mitigation (Kevlin's recommendation).

My call: **Option A**, in this task, since I'm already commenting on
the workflow. The cost is one line in `_ai/m1x-backlog.md` and it
defends against a real but low-impact regression class. The reword
(B) would lose the explicit naming of which Go types carry the field
TODAY, which has actual value to current readers.

**However**: this is not a blocker. The risk is "future maintainer
forgets a one-grep fix when shipping a different feature." That is
not a ship-stopping condition. If we ship without the mitigation,
the worst case is one stale error string that someone fixes in 30
seconds. So I'm calling this a "would be nice" not a "must do." See
priority enumeration at the bottom — this lands as a P3 nice-to-have
during finalization, not a re-plan trigger.

### 2.2 Duplicated 4-token journald splice between `Run` and `RunWithOptions`

(Kevlin §2, Linus §4.2.) Both reviewers explicitly recommended NOT
extracting a helper. I agree: a helper hides the "fixed argv, like
`--restart`" reading pattern, and the duplication collapses
naturally when backlog item 11 unifies the two run paths. Premature
abstraction here costs more than it saves. Wave-off correct. Not a
blocker. Not even a follow-up.

### 2.3 Plan-iteration verbosity

(Linus §8.) Not a code issue, just an observation. The iteration
caught four substantive issues (the integration test fixture miss,
the regex-is-help-string-not-enforced catch, the slash-rejection
invariant promotion, the caddy tag-literal test promotion). Process
worked. No action.

VERDICT on wave-offs: none of them actually block ship.

## 3. Anything either reviewer missed?

Walked the diff with fresh eyes against the user's request and the
plan acceptance criteria. Specifically looked for:

- **Other shellout sites.** `grep -rn '"run"' internal/ cmd/`
  returns only the two `args := []string{"run", "-d", ...}` lines
  in `cli_driver.go`. No third `docker run` site exists. (Re-verified
  here; was verified in Don §2.1 originally; still holds.)
- **`RunRequest{` and `RunOptions{` literals that silently
  zero-value Service.** `grep -rn 'RunRequest{\|RunOptions{' internal/
  cmd/` — every hit is either a production call site with
  `Service:` populated, a deliberate rejection-test fixture (empty
  or slash-containing), or a helper-returning function. Zero zero-value
  accidents. Kevlin §3 already audited this; I re-verified.
- **Any `t.Skip` or `t.SkipNow` introduced in this task.**
  `grep -rn 't\.Skip\|t\.SkipNow' internal/dockerdrv/cli_driver_test.go
  internal/deploy/ internal/caddy/manager_test.go internal/integration/`
  on the changed files — none introduced in this task.
- **TODO/FIXME/XXX in changed code.** `grep -rn 'TODO\|FIXME\|XXX'`
  on the changed files — none introduced.
- **Documentation factual accuracy.** Kevlin's §6 audit
  cross-checked every docs claim against source and against the
  journalctl/Docker upstream documentation. I spot-checked his
  spot-checks: `_docs/install.md:14` matches what I read in
  `_docs/install.md`; `_docs/usage.md` §6 says "exact-match only"
  which matches the journalctl manpage; `_ai/decisions/journald-log-driver.md`
  cites line numbers that match the actual code. Raymond's
  mid-task self-correction on `CONTAINER_TAG=~^decloud/` is the
  right shape — he caught it before commit, fixed it in both
  affected files, and documented the catch in his report.
- **Mock regeneration.** `go generate ./...` is unchanged-by-claim
  (Rob §6) because struct-field additions don't change the `Driver`
  interface signature. I did not re-run `go generate` here, but the
  test suite is green which means the mocks compile and behave; if
  they had drifted, gomock would have failed loudly.
- **Integration test compile-fix in `internal/integration/mount_test.go:69`.**
  Verified — `Service: "mounttest"` is on the line. The
  `//go:build integration` tag means it does not run on the dev box
  (no Docker), but `go vet -tags=integration ./...` and `go build
  -tags=integration ./...` both pass, which is the canary that
  matters.

Nothing missed. The two reviewers did their job thoroughly.

## 4. Tests skipped, half-done, or papered over?

Walked Kent §5 and Rob §6 fixture lists against the diff:

- **All six new dockerdrv tests are real tests, not stubs.**
  Verified by reading the test bodies in the diff. Each has the
  `require.Error` + `errors.Is`-discrimination + `assert.Empty(records)`
  triad I asked Joel to spec, OR the tag-literal triad
  (`indexOf` + value equality check + named-failure-mode messages).
  No `t.Skip`. No `// TODO: implement this assertion`. No empty
  `func TestX(t *testing.T) {}` shells.
- **The `TestCLIDriver_StartArgs` extension is two real
  assertions** (one per flag), not a single soft assertion.
- **Fixture sweeps in §6.1 / §6.3 / §6.4 / §6.5 are mechanical
  but not papered over.** The deploy/lifecycle tests gain
  ASSERTION extensions (`assert.Equal(t, "foo", seen.Service, …)`),
  not just field additions — the assertion locks the contract at
  the deploy layer that the right service name flows through. Without
  the assertion, the field addition would be cosmetic. The assertion
  is the contract.
- **The acceptance criteria split between unit and manual.**
  Criteria 1, 2, 5, 6, 7 are unit-test-locked. Criteria 3 and 4 are
  Linux manual smokes (because no Docker on this box). That
  asymmetry is correct — only a live host can confirm the journald
  end-to-end behaviour; pretending otherwise would be the worst
  kind of test theatre.

VERDICT on tests: nothing papered over. The test surface defends
the contracts the plan named.

## 5. Test/code state right now

Re-ran on HEAD `502bbf3`:

- `go test -count=1 ./...` — every package OK:
  - `internal/caddy` — 0.022s
  - `internal/cli` — 0.022s
  - `internal/config` — 0.010s
  - `internal/deploy` — 12.091s
  - `internal/dockerdrv` — 0.079s
  - `internal/envcap` — 0.102s
  - `internal/ids` — 0.012s
  - `internal/logging` — 0.013s
  - `internal/registry` — 0.040s
  - All `mocks/` and `cmd/decloud` correctly report no test files.
- `gofmt -l .` — clean (zero output).
- `go vet ./...` — clean (zero output).
- `go vet -tags=integration ./...` — clean (zero output).
- `go build -tags=integration ./...` — clean (zero output).

Test pass count matches Rob's 246 PASS / 0 FAIL claim from
`06-rob-implementation.md`. Green right now, on the actual code on
disk, not on a stale report.

## 6. Cross-checks against the plan's own acceptance criteria

Don §6 lists seven acceptance criteria. Walking them:

1. `docker inspect decloud-foo --format '{{.HostConfig.LogConfig.Type}}'`
   = `journald` — locked by the argv shape; Linux manual smoke
   confirms the runtime behaviour.
2. `tag` = `decloud/<service>` — locked by
   `TestCLIDriver_RunEmitsJournaldFlagsWithSlashTagLiteral` AND
   `TestCLIDriver_RunWithOptionsEmitsJournaldFlagsWithCaddyTag` AND
   the existing argv-shape tests
   (`TestCLIDriver_RunArgsWithEnvSorted`,
   `TestCLIDriver_RunWithOptionsCaddyShape`, etc.) which now
   include the four-token splice.
3. `decloud logs <name>` and `-f --tail` work unchanged — manual
   Linux smoke, code path unchanged.
4. Pre-redeploy and post-redeploy log lines both visible under one
   tag — manual Linux smoke.
5. `go test ./...` passes — re-verified above. 246 PASS.
6. Empty Service → `ErrEmptyService`; slash → `ErrInvalidService` —
   locked by §6.2.1–§6.2.4 tests, each with the discrimination
   assertion and the `assert.Empty(records)` before-exec assertion.
7. `docker start` argv does NOT contain `--log-driver` or
   `--log-opt` — locked by the two `assert.NotContains` on
   `TestCLIDriver_StartArgs`.

All seven covered. The Linux-manual ones (3, 4) are correctly
documented as such, not faked with a unit test that would
test nothing.

## 7. Process discipline check

The workflow says PLAN → EXECUTION → PLAN. We're at the post-EXECUTION
PLAN step now, which is what this sign-off check is for. Joel does
not need to expand a new tech plan because there is no new work to
plan — the question is "is this done?" and the answer drives finalization
(STEP 4), not another iteration.

The plan iteration converged at REVISION 2 with explicit Linus
APPROVED. The implementation followed the plan with zero deviations
(Rob §6 confirmed, Kevlin §1-3 verified, I just re-verified).
Kevlin and Linus both APPROVED at code-review time. The two non-blocking
items they raised (§2.1 above) do not require re-planning — they're
either deferred to a different task (item 11 grep), or correctly
declined (the helper extraction).

If we kicked back to PLAN now over the message-string drift, we
would be inventing work to perform process theatre. That is exactly
the kind of "perfect is the enemy of right" trap I have to slap down.
Ship it.

## 8. The honest brutal verdict

Process worked. Plan was right. Implementation followed the plan.
Tests defend the right invariants. Docs accurately describe the
merged code. Reviewers found real things in the plan-iteration phase
(integration test miss, regex-is-help-string catch, slash-rejection
invariant, caddy tag-literal test) and nothing substantive in the
post-implementation phase. The user's stated request is satisfied
end to end. Code is green right now on the actual disk state.

Both non-blocking observations from the post-implementation reviews
are correctly non-blocking:

- The duplicated 4-token argv splice resolves naturally at backlog
  item 11. Premature extraction would be the wrong shape today.
- The `ErrEmptyService` message string is genuinely accurate today
  and only goes stale if a future consolidation lands. A one-line
  backlog note is the right hedge; it can be added at finalization
  (Ward's STEP 4a or as part of any small docs touch) without
  re-planning.

The only thing I'd ask Ward to consider during STEP 4a (preserve
learnings) is whether the `_ai/m1x-backlog.md` item 11 entry should
gain a one-liner "grep for RunRequest/RunOptions in dockerdrv error
message strings during consolidation." That is a finalization-time
touch, not a re-planning trigger.

## 9. Items to address (priority order)

**P3 (nice-to-have, can happen at finalization):**

1. Add a one-line note to `_ai/m1x-backlog.md` item 11: "during
   `RunRequest`/`RunOptions` consolidation, grep
   `internal/dockerdrv/driver.go` error-message strings for the
   phrase `RunRequest/RunOptions` and update." Cost: one line.
   Benefit: removes the "future maintainer might forget" risk
   Linus surfaced in §4.1. Can be done by Ward during STEP 4a or
   by any agent doing a small docs touch on the file.

**P1 and P2:** none. There are no blockers and no must-fix items.

## 10. Verdict

The plan is sound, the implementation matches the plan, the tests
defend the right invariants, the docs are accurate, the deferrals
are captured with the right specificity, and the test suite is
green on the actual disk state right now. The user's request is
satisfied. The two non-blocking items the post-implementation
reviewers surfaced are correctly non-blocking.

This is what shipping looks like. Move to STEP 4 (FINALIZATION):
Ward preserves learnings (and optionally adds the P3 backlog
one-liner above), Andy considers agent updates, branch
squash-merges to `main`.

I am NOT kicking this back to PLAN. There is nothing to re-plan.

## VERDICT: FULLY DONE
