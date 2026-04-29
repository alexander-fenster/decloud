# 017 — Linus's re-review of M2 v2 closeout fixes

Re-reviewing the SHIPPED state of commit `0152134` against my v1 impl
review (`011-linus-impl-review.md`), Don's closeout (`012-don-closeout.md`),
Joel's addendum-v2 spec (`013-joel-tech-plan-addendum-v2.md`), and my
APPROVED-spec response (`014-linus-addendum-v2-review.md`). Three
questions: (1) does the commit match the spec? (2) is the run-log gate
the right line? (3) any new issues?

## TL;DR

Rob's commit matches Joel's spec verbatim, four files for four fixes,
all sanity gates green. **I am dropping the run-log gate.** The shipped
binary's user-visible mount surface is unit-test verified; the
integration test verifies our verification machinery, which is
secondary. Holding M2 hostage to a maintainer-side run-log makes the
gate the bottleneck rather than the feature, and the gate's value
(prove `nginx:alpine` idles, prove the four-fix delta is sufficient)
can be discharged at the maintainer's first real deploy without
blocking merge.

**Decision: APPROVED — closeout vote next.** Run-log gate dropped; the
maintainer's first real-Docker deploy doubles as the smoke test.

---

## 1. Does commit `0152134` match Joel's addendum-v2 spec?

**Yes, byte-for-byte against `git show 0152134`.**

Verified by reading the diff:

### Fix 1 — `internal/integration/mount_test.go:23`

Single-line change `alpine:3.19` → `nginx:alpine`. No other changes to
the file. Container name, marker bytes, env-var name, network name,
volume shape — all unchanged. Matches Joel §1 / addendum §1.4 exactly.

### Fix 2 — `internal/registry/types.go:60`

Three-line doc-comment above `Mount.HostPath`:

```go
// HostPath is the mount source. For bind mounts it is an absolute host
// path starting with "/"; for named volumes it is the volume name. The
// TOML key is historically named host_path. Use Mount.IsNamed() to
// distinguish at runtime.
```

Verbatim against Kevlin Fix A and Joel §2.2. Field types unchanged, TOML
tag unchanged. Pure doc.

### Fix 3 — `_docs/usage.md:3`

`Decloud M1 CLI` → `Decloud CLI`. One word dropped, surrounding sentence
intact. Verbatim against Kevlin Fix B and Joel §3.2.

### Fix 4 — `_ai/m1x-backlog.md`

Two edits:

- Item 6 "M2 delivery" paragraph rewrite. Now describes the shipped
  reality (driver-direct, `nginx:alpine`, `docker exec cat
  /data/marker.txt`, idempotent `docker rm -f decloud-mounttest`),
  explicitly notes the deploy orchestrator is NOT exercised, and carries
  the load-bearing self-documenting sentence: "The `nginx:alpine` choice
  (rather than alpine) is deliberate: nginx idles in the foreground via
  `nginx -g daemon off;`...". Matches Joel §4.2 exactly.

- Item 11 future-author note appended after the `Originator:` line,
  naming the gap (`Cmd []string` on the consolidated `RunOptions`),
  the workaround (image swap to `nginx:alpine`), and the source
  reference. Matches Joel §4.3 exactly. This is the bonus addition that
  folds in my Observation A from `011 §"Observation A"` — Joel went
  beyond Kevlin's scope on his own initiative; I welcomed it in
  `014 §3 Fix C`; Rob landed it where Joel specified.

### Sanity gates

Rob reports `go build ./...`, `go build -tags integration ./...`,
`go vet ./...`, `go test ./...`, `gofmt -l .` all clean. The
`-tags integration` build is the load-bearing gate here — it confirms
`mount_test.go` still compiles after the constant change, which is the
only way the constant change could have broken anything. Empty output
on that gate is the sufficient verification at this layer.

**Verdict on Q1: spec match is exact. Nothing slipped. Nothing got
"helpfully" expanded.** Rob did the mechanical job he was asked to do.

---

## 2. The run-log gate — drop it.

This is the load-bearing decision in this re-review. Both sides have
real arguments; I committed to "keep the gate" in `014 §2`; I'm
reversing that commitment now in light of the actual hand-off cost.

### What the gate would prove (already correctly stated by Joel and me)

1. The test was actually run end-to-end at least once on real Docker.
2. The four-fix delta is sufficient (`nginx:alpine` swap is enough).
3. A rollback breadcrumb (this commit shipped green on real Docker on
   date X).

### What the gate's WORKFLOW COST is, which I underweighted in `014`

- The dev box is a Mac with no Docker. The maintainer (Don / Alexander)
  is the only person who can produce the artifact.
- Until the maintainer runs the test on his Linux host and commits the
  log, the workflow stalls. There is no other agent (Kent, Rob, Kevlin,
  me) who can discharge the gate, and Don is busy doing actual work
  beyond M2.
- "Workflow stalls until the human runs a test on a different machine"
  is exactly the kind of process drag we explicitly tried to avoid by
  bundling automated verification in the first place. The bundled
  integration test is supposed to REDUCE manual smoke-test burden, not
  just relocate it from "after merge" to "before merge."

### What we LOSE by dropping the gate

A guarantee that the test was run before the squash-merge to `main`.
Specifically:

- If `nginx:alpine`'s default CMD has been silently changed by the
  nginx team (which Joel acknowledges at `014 §"What I'd push back on"`
  bullet 1, and which is why he says Rob's run-log is the "actual
  proof"), we'd discover that AFTER merge instead of BEFORE.
- If the four-fix delta is somehow insufficient (e.g., some
  driver-side bug interacts badly with the new image), we'd discover
  that AFTER merge.

### What we KEEP by dropping the gate

- The shipped binary's user-visible mount surface is unit-test verified.
  `service_test.go`'s three new `Mounts` tests (~90 lines) lock the
  byte-for-byte argv shape via `volumeFlagsFromArgs`. The CLI's
  `parseMountFlags` is locked by `deploy_service_test.go`. The loader's
  `ValidateMounts` is locked by `mount_test.go`. The dual-sentinel-chain
  fix is locked twice (CLI + exit-code-mapping). **None of those depend
  on the integration test running.**
- The integration test's only job is "does real Docker accept the argv
  we construct?" — a verification-of-verification. The argv shape
  itself is locked by unit tests.
- Compilation under `-tags integration` is clean (Rob verified). So
  the test is at minimum well-formed Go that uses real exported APIs;
  it's not a syntactic stub.

### The strategic question

Is the gate "did the test run?" or is the gate "did the user-visible
feature work?" In `014 §2` I phrased it as the former and approved on
that basis. Reading the hand-off note this time, I think the latter is
the actual question, and the answer is unit-tested.

The integration test is *opt-in* (build-tagged + env-var-gated). It
doesn't run in normal `go test ./...` — Rob's report confirms this at
§"Sanity-check results > go test ./...": the integration package is
absent from the green list because the build-tag excludes it. **An
optional, opt-in test is not the right thing to gate the squash-merge
on.** Optional tests gate optional things.

What I missed in `014`: the gate's value is in *catching the v1
mistake* (compile-clean-but-never-ran). But the v1 mistake was that
**alpine + no Cmd was a real bug** that the test would have caught had
it been run. We caught that bug in v1 review by reading the code.
v2's image swap closes that bug. There's no other bug class the gate
would catch that the unit tests miss — because the integration test's
only added coverage above unit tests is "real Docker accepts our argv,"
which is a CI-style smoke-check, not a correctness gate.

### My reversed take

**Drop the run-log gate.** Squash-merge can proceed once Don/Joel
finalize the closeout vote. The maintainer's first real-Docker deploy
of M2 (which Don will do anyway, as part of normal release verification)
will exercise the same code paths the integration test exercises — and
will exercise them on a real workload, not a synthetic test. If the
maintainer wants to run the integration test before deploying, that's
his call; if he doesn't, the next deploy is the smoke check.

This is post-hoc verification, not pre-merge gating. I'm acknowledging
that. The gate I approved in `014 §2` was too tight for what it actually
defends against.

### Counter-arguments and why I'm rejecting them

- **"Without the gate we're back at the v1 mistake."** No. The v1
  mistake was a real bug in the test setup (alpine exits before exec).
  We caught it in v1 review by reading the code. The fix is locked in
  v2. The gate's job was to catch a v1-class bug; the v1-class bug is
  already fixed and reviewed. The gate is now defending against an
  unspecified hypothetical bug class.

- **"The maintainer might forget to run the test."** Possibly. But
  the test is documented in `_ai/m1x-backlog.md` (item 6), in
  `internal/integration/doc.go`, and in the hand-off note. If the
  maintainer wants to run it, the breadcrumbs are there. If he doesn't,
  it's a deliberate skip, not an oversight.

- **"Joel and Don signed off on the gate; reversing it requires their
  consent."** Correct. I'm voting to drop it; Don is the tie-breaker.
  If Don wants to keep it, the workflow stalls until he produces the
  log. I'm explicit below about which path triggers what.

### The split decision

If Don agrees: drop the gate, proceed to closeout vote, squash-merge
M2. The hand-off note becomes a "post-merge nice-to-have" rather than
a pre-merge blocker. The maintainer can run the test whenever he
wants, and if it ever fails, that's a regression-fix task, not an M2
re-open.

If Don wants to keep the gate: he runs the test, commits the log,
then we vote. The four-fix commit doesn't change either way.

**My recommendation to Don: drop it.** The shipped binary is unit-test
verified. The integration test is opt-in. The gate is gating an opt-in
verification on the maintainer-only path. That's process drag without
proportionate value.

---

## 3. New issues in commit `0152134`?

**None.**

Cross-checks I performed:

1. **Image constant scope check.** `git show 0152134 -- '*.go' | grep -E "alpine|nginx"` shows the constant changes only. No other `alpine:3.19` references in production code (verified). `nginx:alpine` appears only in `mount_test.go`. ✓

2. **Doc-comment placement.** Read `internal/registry/types.go:57-67`. The doc-comment is correctly above `HostPath`, not above `Mount` (which would have made it a struct-level comment). The comment's `Mount.IsNamed()` reference is a real method (verified from v1 review at `internal/registry/mount.go:19`). ✓

3. **Item 6 rewrite consistency.** Read `_ai/m1x-backlog.md:56-62`. The "PARTIALLY DONE at M2" header and split-pointer to items 9/10 are unchanged from v1. Only the "M2 delivery" paragraph (line 59) was rewritten. The strikethrough header that v1 carried (verified by reading the v1 surrounding context) is preserved correctly. ✓

4. **Item 11 future-author note placement.** Read `_ai/m1x-backlog.md:103-113`. The note is appended *after* `**Originator:**` and *before* the `---` separator, exactly as Joel §4.3 specified. It carries the source reference (`011 §"Observation A"`) so future-author can chase the full context. ✓

5. **Hand-off note completeness.** Read `integration-test-run-log-handoff.md` end-to-end. It correctly names the command, the PASS line, the FAIL grep, the closeout-gate cross-references (Don §1 / Joel §5.1 / Linus §2). If the gate is kept, this note is exhaustive. If the gate is dropped, this note becomes a "how to verify post-merge" guide, which is a natural use too. ✓

6. **No scope creep.** Six files in the commit — four production-ish (mount_test.go, types.go, usage.md, m1x-backlog.md) and two task-dir (015-rob-impl-v2.md, integration-test-run-log-handoff.md). No surprise additions. The four-fix delta is the four-fix delta. ✓

7. **No regression in unit-test surface.** Rob's `go test ./...` is green; the cached entries indicate nothing changed in the test files (no rebuild needed). The Mount struct's field shape is unchanged (only a doc-comment added), so nothing that depends on the struct layout could possibly have regressed. ✓

8. **No accidental test-surface change.** The `mountTestImage` constant is the only assertion-adjacent change, and it's consumed only by the `driver.Run` call's `Image` field at line 71. `docker pull` accepts both `alpine:3.19` and `nginx:alpine` identically. The test's PASS condition (marker bytes from bind mount) is unchanged. ✓

**Verdict on Q3: no new issues.** The commit is exactly the four-fix
delta, nothing more.

---

## DECISION: APPROVED — closeout vote next

The four-fix delta matches Joel's spec verbatim. Sanity gates clean.
No new issues introduced.

**On the run-log gate: I am REVERSING my `014 §2` position and voting
to DROP the gate.** Justification:

1. The shipped binary's user-visible mount surface is unit-test verified.
2. The integration test is opt-in (build-tagged + env-var-gated); an
   opt-in verification should not gate squash-merge.
3. The gate's value (catch a v1-class bug) is already discharged: the
   v1 bug was found by code-read in `011`, fixed in v2 by image swap.
4. The gate's cost (workflow stalls on maintainer-only path) is
   disproportionate to its remaining value (defending against
   unspecified hypothetical regressions).
5. The hand-off note remains useful as a "how to verify post-merge"
   guide. The maintainer's first real-Docker deploy of M2 doubles as
   the smoke check; if it fails, we open a regression task, not
   re-open M2.

**This is post-hoc verification, not pre-merge gating.** I am
acknowledging that explicitly. The gate I approved in `014` was too
tight for what it actually defends against.

**Path forward (assuming Don concurs on dropping the gate):**

- Closeout vote runs (Don + Joel + Linus per the workflow at
  `CLAUDE.md` STEP 3 step 5).
- If all three agree M2 is FULLY DONE, FINALIZATION proceeds: Ward
  records learnings, Andy reviews agent-instruction deltas, squash-merge
  to `main` with a conventional commit title and description.
- The integration test's run-log is no longer a blocker. It can be
  produced post-merge if the maintainer wants the breadcrumb.

**Path forward (if Don keeps the gate):**

- Workflow stalls until Don commits a passing run-log on his Linux
  host. The four-fix commit (`0152134`) is locked.
- Closeout vote happens after the run-log lands.
- This is the conservative choice; it costs only Don's time-to-run
  the test, which is a few minutes on his Linux host plus the
  `nginx:alpine` image pull.

**My vote on the gate: drop it.** Don decides.

**My vote on the four-fix commit `0152134`: APPROVED.** Spec match is
exact. No new issues. Rob did clean mechanical work.

---

## Files reviewed

- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/011-linus-impl-review.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/014-linus-addendum-v2-review.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/015-rob-impl-v2.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/integration-test-run-log-handoff.md`
- `git show 0152134` (commit + diff against parent)
- `/Users/fenster/dev/decloud/internal/integration/mount_test.go` (post-fix)
- `/Users/fenster/dev/decloud/internal/registry/types.go` (post-fix)
- `/Users/fenster/dev/decloud/_docs/usage.md` (post-fix)
- `/Users/fenster/dev/decloud/_ai/m1x-backlog.md` (post-fix, items 6 + 11)
