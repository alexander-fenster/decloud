# 006 — Linus's plan re-review (PLAN stage v2)

Re-reviewing `005-joel-tech-plan-addendum.md` against my three minor revisions in `004-linus-plan-review.md`. Branch `feat/m2-server-side-mounts`.

## TL;DR

Joel picked the right option on all three. The implementation shapes are correct, the line numbers check out, the error chains do what he says they do. One small naming nit on the new exported helper, one tiny editorial note on the `usage.md` paragraph, and one strong endorsement on Issue 10. Nothing blocks Kent.

Decision: **APPROVED — Kent can start.**

---

## Issue 1 — Dual-sentinel chain

**Option chosen: B (strip the wrap by factoring out `findDuplicateTarget`).** Right call. Option A relied on a comment that survives only as long as the next refactor reads it; Option C inflated `ValidateMounts`'s API for one sentinel.

**Implementation shape: correct.** Two clean surfaces:
- `ValidateMounts(...)` (loader path) wraps with `ErrInvalidMount` exactly once, in the function that owns that sentinel. Loader chain: only `ErrInvalidMount`. Exit 10.
- `parseMountFlags` (CLI path) consumes the bare `FindDuplicateTarget` helper directly, wraps with `errUsage`. CLI chain: only `errUsage`. Exit 2.

The `errors.Is` properties Joel claims at lines 105-117 are correct: each chain carries exactly one sentinel, and reordering `exit_codes.go` cases no longer flips the answer. Footgun is dead.

**Hidden bugs: none, but two small notes:**

1. **Naming.** Joel exports `FindDuplicateTarget` from `internal/registry`. The name is fine but the doc comment should explicitly call it "package-API for CLI use; internal callers should prefer the validated entry point `ValidateMounts`." Joel's addendum line 100 says exactly that already — good. No change needed.

2. **The new test assertion at lines 122-126.** "`assert.False(t, errors.Is(err, registry.ErrInvalidMount), ...)`" is the *load-bearing* test that locks Issue 1 against regression. Kent: this is non-negotiable. If it ever turns into "we removed it because it was redundant," the regression risk reopens.

3. **`TestRegistry_FindDuplicateTarget` (lines 127-128).** Joel's table covers (none, two-different, two-same, three-with-collision). Add one more row: `[]Mount{}` (empty slice → `(0,0,false)`). The `len(mounts)==0` path through the helper is trivial but the test should pin it; otherwise a future "optimise the empty case" change could drift.

None of these are blockers. Issue 1 is solved cleanly.

---

## Issue 5 — No-stat operator UX paragraph

**Option chosen: B (write the paragraph now).** Right call. A would have shipped a confusing first-deploy error with no breadcrumb back to "you typo'd the host path."

**Wording (lines 152-160).** Locked verbatim, which is correct — Raymond gets zero room to paraphrase. The wording does the three jobs it needs to do:
- Tells the operator what error to expect (with the typical text excerpt)
- Tells them *why* we don't pre-stat (TOCTOU + reboot-ordering race)
- Gives them the one-liner to verify before deploying (`ls -ld`)

**One small editorial note (not blocking):** the sentence "exit 40" appears mid-sentence and reads awkwardly inside the parenthetical. A reader skimming the table at line 71 sees `--mount` → wraps to this paragraph → has to parse the parenthetical to spot the exit code. Acceptable as written; not worth a re-write. If Don wants to polish, the form `"the deploy fails at the docker run step (exit 40) with a Docker daemon error referencing the path"` reads slightly better. Editorial preference, not a correctness issue.

**Insertion point (line 164).** Verified. `_docs/usage.md` line 71 is the `--mount` row in the flag table; line 73 is the `--config-root` row (the table's last row); line 74 is the `env.sh` model paragraph. Inserting between 73 and 74 lands the paragraph immediately after the table, which is exactly the right anchor — operator's eyes are on the `--mount` row, the paragraph follows the table they were just reading.

Note that line 71's wording itself ("Rejected with exit 10 in M1. Persistent volumes are M2.") flips in M2 to operator-facing syntax wording. Joel acknowledges this at line 166. The insertion point survives the flip because it's anchored to the *table's* end, not to line 71's text.

**Hidden bugs: none.** Doc-only change, no test surface, no production code surface.

---

## Issue 10 — Strategy-block papercut

**Option chosen: A (fix both blocks).** Right call. Three characters, same hunk, same reasoning. Fix-while-fresh means *fully* fresh, not "fresh in the block I touched, stale next door."

**Implementation shape: correct.** Joel's diff at lines 191-201 is exactly right:
- `cfg.Name` → `name` at line 73 inside the strategy-rejection block.
- Line 76's `cfg.Name = name` assignment is *not* moved earlier (which was the wrong way to fix it — `name` is in scope at lines 68 and 73 already, and moving the assignment up changes the semantics for the rest of the function).

**Verified by reading the file.** `internal/registry/store.go:51-76`:
- Line 51: `func (s *fsStore) Load(ctx context.Context, name string)` — `name` parameter in scope.
- Line 70: current code uses `cfg.Name` (which can be empty before line 76).
- Line 74: current code uses `cfg.Name` (same problem).
- Line 76: `cfg.Name = name` (the fixup that papers over both bugs *for the rest* of the function but not the two earlier blocks).

Joel's analysis at lines 206-207 is exactly right.

**Bundling instruction (lines 208-212): correct.** Both edits in one hunk in Rob's GREEN commit. Don't split. No argument.

**Test-surface decision: locked Option β (no new test).** I agree — the rename has zero behaviour change for tests with TOML fixtures that include `name = "..."` (which is all of them today). Adding a fixture-without-`name` subtest is exactly the change-detector character we tell Kent to avoid. The fix is correct on the code level; the production payoff is for the operator who hand-edits a TOML and omits `name`. No test can express "this error message would be more helpful" without locking the message itself.

**Hidden bugs: none.** It's a three-character rename inside an `fmt.Errorf` argument list; the only failure mode is typo, which `gofmt` and `go vet` catch.

---

## Anything else in the addendum

**Two cross-cutting observations:**

1. **`FindDuplicateTarget` exposed surface (line 100, addendum).** Joel renames the helper from unexported `findDuplicateTarget` (in `mount.go` per original §3.2) to exported `FindDuplicateTarget` because the CLI lives in `internal/cli` and needs to call it. Fine. But the doc comment at addendum lines 56-58 markets it as "the bare-error shape lets CLI callers wrap with errUsage without re-routing through ErrInvalidMount" — that's the *reason* but not the *contract*. The contract is "returns (firstIdx, dupIdx, true) on first duplicate; (0, 0, false) on no duplicate." Kent: the test should pin the contract; the doc comment should state the contract; the rationale-comment is a bonus.

2. **The addendum is well-scoped.** It deltas exactly the three items I called out, doesn't sneak in fourth-thoughts, doesn't re-litigate Decisions 1 or 4. Joel resisted the temptation to expand. That's discipline; it's what an addendum should look like. If we get more of these in M3, M4, this is the template.

**One thing the addendum doesn't touch that I want to flag explicitly:** Issue 2 (the integration-test narrowing) and Issue 3 (the `HostPath`/named-volume operator confusion). Both were "no action needed" or "Don's call" from my v1 review. Don can address Issue 3 in Raymond's docs sweep without further plan churn. Issue 2 is genuinely a Don decision — if he's OK shipping M2 with the narrow integration test, we ship; if he wants the broader test, that's a §4.8 plan amendment, which would require a Joel revision. **Reading the addendum as Don's tacit acceptance of my "leans" on Issues 2 and 3 (Option A on both):** fine.

---

## Cross-checks I performed for this re-review

1. **`store.go:64-76` line numbers.** Verified. Mount block at 68-71, strategy block at 72-75, `cfg.Name = name` at line 76. Joel's diff matches.
2. **`_docs/usage.md:71-74` insertion anchor.** Verified. Line 71 is the `--mount` row, line 73 is `--config-root`, line 74 is `env.sh`. Joel's "between 73 and 74" lands the paragraph correctly.
3. **`name` parameter in scope.** Verified at line 51 (`Load(ctx, name string)`); `name` is reachable everywhere from line 52 onward.

No other spot-checks needed; Joel's claims hold.

---

## DECISION: APPROVED — Kent can start

All three revisions are correctly resolved. The error-chain redesign in Issue 1 is the only one with non-trivial code shape, and Joel's separation of `ValidateMounts` (loader-only, wraps `ErrInvalidMount`) from `FindDuplicateTarget` (bare helper, CLI-consumable) is structurally honest.

Kent: read `003-joel-tech-plan.md` plus `005-joel-tech-plan-addendum.md`, in that order. Where they conflict, the addendum wins (Joel's stipulation, line 3 of the addendum, correct).

Rob: same order. The §3.4 file-diff in the original tech plan is *replaced* by the addendum's Issue 10 diff for `store.go`; the §3.2 `mount.go` block is *replaced* by the addendum's Issue 1 structure; the §3.5(d) `parseMountFlags` body is *replaced* by the addendum's Issue 1 body.

No PLAN v3 needed.

---

## Files reviewed (absolute paths)

Planning:
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/003-joel-tech-plan.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/004-linus-plan-review.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/005-joel-tech-plan-addendum.md`

Code (spot-checked for line numbers and scope):
- `/Users/fenster/dev/decloud/internal/registry/store.go`
- `/Users/fenster/dev/decloud/_docs/usage.md`
