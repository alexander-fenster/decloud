# 011 — Linus high-level review of execution

## Verdict

**APPROVED — task body complete; Don/Joel/Linus PLAN re-entry can declare done.**

Three follow-up items flagged below as non-blocking cleanups for the closeout step (Ward's learnings extraction, or a future-Don pass). They are not gates on this task.

---

## What I checked

Read `001` through `010` end-to-end. Spot-verified the actual edited bytes in `m1-scope.md`, `caddy-runs-in-container.md`, `schema-versioning.md`, `MEMORY.md`, plus Rob's three Go edits in `internal/cli/deploy_service.go:61,72` and `internal/registry/store.go:69`, plus Kent's three assertions in `deploy_service_test.go:80-104` and `store_test.go:297`. Re-grepped for survivors across `_ai/`, `_docs/`, and `README.md`.

Reports match reality. No drift between what the agents claim and what's on disk.

---

## 1. Did execution match the plan?

Yes, with one in-scope addition that's defensible.

- **Kent**: 3 substring assertions across 2 test files, exactly per Joel v2 §B.11 / §C. RED bar between Kent's commit and Rob's was acknowledged in the report header (`007:5-7`) per Joel's mitigation. The constructor-name verification (`newDeployServiceCmd` vs Joel's placeholder `newDeployServiceCommand`) was the right kind of pre-execution check — Kent caught a planning hallucination before it shipped.
- **Rob**: 3 byte-exact M3→M2 substitutions, single atomic commit per Linus C3 / Joel v2 §D. `gofmt -l`, `go vet`, `go test ./...` all clean post-edit. Constraint guards correctly preserved (Rob's report §"Constraints I investigated and did *not* remove" hits the CLAUDE.md "INVESTIGATE BEFORE REMOVING CONSTRAINTS" rule explicitly). The "not exiting M1 yet" reading is correct — this is a label flip, not a behavior flip.
- **Raymond**: 12 prescribed doc edits + 1 in-scope fix-while-fresh = 13 total, across 9 files (10 if you count `container-naming.md` which was confirmed to need zero edits).

### The Raymond fix-while-fresh on `caddy-runs-in-container.md:52`

"until M2's config file lands" → "until M3's config file lands". The right kind of in-scope fix. Three reasons it doesn't set a scope-creep precedent:

1. **Same architectural event as the prescribed edits at lines 15 and 58** — all three describe the introduction of `/etc/decloud/config.toml` via Viper. Joel's audit enumerated 15 and 58 but missed 52. Without the fix, the same file would have contradicted itself across three lines (15 says M3, 52 says M2, 58 says M3). Future-Don reads the file end-to-end and gets whiplash.
2. **Mechanical, single-token rename**, identical to the treatment of lines 15 and 58 — not a new prose decision.
3. **Direct parallel to Joel's own fix at `install.md:121`** ("M2 will write source bundles" → "M6"), which v1 §B.9 already invoked the fix-while-fresh rule on. Raymond is following the precedent the plan itself set.

This is exactly the discipline `_ai/review-discipline/fix-now-while-fresh.md` codifies. If anything, it surfaces a small process improvement (see §6 below): Joel's audit methodology missed a same-file occurrence that was three lines below an enumerated one. Worth Ward capturing; not worth blocking this task.

---

## 2. Strategic call still defensible post-execution?

Yes. The new sequence (M2 = `--mount`, M3 = bootstrap, M7 = absorbs deferred) reads coherently in the actual edited prose.

- **`m1-scope.md:32`** — the canonical roadmap one-liner now flows naturally: "M1 service deploy MVP → M2 server-side mounts + env-file hardening (`--mount` flag, loader populates `Mounts`) → M3 host bootstrap (introduces Viper, `caddy.image` config knob) → M4 …". The M2-before-Viper ordering is justified inline by the parenthetical. No reader hits this line and asks "wait, how do mounts work without Viper?" because the line answers it.
- **`m1-scope.md:18`** — the C1 trap warning lives where future-Don actually looks before touching M2. The Option-C-trap callout names the rejected path by name (`/etc/decloud/config.toml` parsing for default-mount-options) so anyone tempted to "helpfully" add config-loading sees the guardrail.
- **`m1-scope.md:34`** — the C2 paragraph on M7 provisional-bundling is forceful and exactly the failure mode I cited.
- **`schema-versioning.md:10-11,16`** — the shape-stability promise survives clean. M1 reserves, M2 populates `Mounts`, M7 extends to secret-files. One coherent sentence.

The only place where I read the new prose and felt a small bump was **`m1-scope.md:8`**:

> "NOT 'host bootstrap first' — bootstrap is `apt install docker caddy && systemctl enable decloud.service`; five lines of substance, exercises none of the design's hard parts. M3."

The terminal "M3." after "M2." historically meant "this is what we cut from M1 and that's where it lands instead." Under the new sequence, it still parses correctly but the rhetoric of "we cut this and it lands at the next milestone" is no longer technically true (M3 is *not* the next milestone now — M2-new is). The single-token replacement preserves the substance perfectly; it's just that the M2-cut/M3-cut/M2-cut/M3-cut pattern in lines 8–18 reads slightly less crisply than it did before. Trivial; flagging only because a cold reader might briefly stumble. Not worth a re-edit.

---

## 3. Four user-visible surfaces — coherent story?

Yes. Kevlin's §2 enumerated five surfaces (he correctly counted the loader rejection at `store.go:69` as a distinct surface from the CLI runtime rejection at `deploy_service.go:72`). All five say M2:

1. `--help`: `M1: rejected with ExitConfigError (M2 only)` — verified at `deploy_service.go:61`.
2. CLI runtime: `--mount is not supported until M2` — verified at `deploy_service.go:72`.
3. Loader runtime: `mounts are not supported until M2` — verified at `store.go:69`.
4. `_docs/usage.md:71`: `Persistent volumes are M2.`
5. `_ai/decisions/m1-scope.md:32`: M2 = "server-side mounts + env-file hardening (`--mount` flag, loader populates `Mounts`)".

The sentinel `ErrMountsNotSupported.Error()` at `internal/registry/errors.go` is correctly left as `"registry: mounts not supported in M1"` — current-milestone wording, deliberately distinct from the "until M2" forward-looking wrap. Rob's report §"Subtle effects" §2 and Kevlin's §2 both call this out and both are right: the two conventions co-exist consistently.

Operator hitting `--help` in M1 sees "(M2 only)", tries `--mount=...`, sees "until M2", greps usage.md, sees "Persistent volumes are M2". One coherent story. The `_ai/cli-flag-surface-coherence.md` four-surface contract holds.

---

## 4. What's missing?

I argued in the plan review (§A.4 / Joel v2 §A.2) that no separate `_ai/decisions/milestone-resequence.md` decision file was needed. **Re-checking after execution: I still agree, post-execution.**

The actual rationale-pointer surface ended up being:

- `m1-scope.md:36` — appended sentence pointing at `_tasks/2026-04-28-milestone-resequence/`, naming what changed.
- `MEMORY.md:57` — task-pointer bullet under "Source-of-truth task artefacts".
- `caddy-runs-in-container.md:15,52,58`, `m1-test-strategy.md:7,49`, `secrets-split.md:6`, `schema-versioning.md:10-11,16`, `m1x-backlog.md:61`, `MEMORY.md:9`, `usage.md:71`, `install.md:121` — every site that mentions a moved milestone, updated.
- `m1-scope.md:18` — the C1 trap warning, which is the future-Don guardrail against the most obvious post-resequence stupid move.

Future-Don reading `MEMORY.md` sees the task pointer in the right index section. Reading `m1-scope.md` sees the appended sentence in the canonical sequence section. Both give a clear pointer to the task directory where the full rationale lives. A separate decision file would duplicate the pointer and force readers to chase one extra hop.

The C2 warning at `m1-scope.md:34` is the specifically forward-looking guardrail — "do NOT repeat the M3a/M3b mistake by treating 'everything in M7' as a single deliverable." This is the prose that prevents future-Don from re-shuffling, more durably than any decision file would.

**No new doc is needed. The pointer surface is sufficient.**

---

## 5. Kevlin's nits — escalate any?

Read his §6.1 through §6.4 carefully. **None should escalate.** All four are correctly classified as nits.

### 5.1 — §6.1 (`secrets-split.md:29` rejected-alternative-C "M3")

Frozen rejected-alternative narrative. The substantive point ("ships M1 with a known security regression — No.") is the actual content of the bullet, and that survives unchanged. The "M3" label is the milestone the rejected alternative *wanted to push the split to* — itself a counterfactual. Under the new sequence "M3 = bootstrap" makes the counterfactual slightly less geometrically obvious, but the bullet still rejects the alternative on the same logic. Kevlin's recommended future rewording ("a later milestone with a schema bump") is fine for a future pass; not blocking now.

### 5.2 — §6.2 (C2 paragraph references "M3a/M3b mistake")

Subjective decay-over-time concern. The trailing-paragraph Linus-approval pointer at line 36 chains through to the resequence task, so a curious reader has the breadcrumb. Forceful naming was the right call for the immediate post-resequence horizon (next 1–3 milestones). If/when the references decay, future-Don can soften the language at M7-start time — which is exactly when the warning matters most anyway.

### 5.3 — §6.3 (`TestDeployService_MountFlagHelpReferencesM2` vs `cli-flag-surface-coherence.md:29-31`)

Real tension between past doctrine and current practice. Kent's argument (`007` §"Why these aren't change-detector tests") and Joel's plan both made the call that semantic-token-substring assertions are not change-detectors in the banned sense. I approved in `006` §C3. The right resolution is **either** amend `_ai/cli-flag-surface-coherence.md:29-31` to carve out semantic-token substring assertions as an explicit exception, **or** drop the C.3 test on a future pass. **I'd lean toward the carve-out** — the test catches real contract drift (label coherence across surfaces) and the cost is one substring assertion. **Ward should capture this as a learning** during finalization: the four-surface coherence doctrine has now been refined by practice to allow semantic-token assertions on surface 3. Follow-up for closeout, not a gate on this task.

### 5.4 — §6.4 (`secrets-split.md:29` cross-link absence)

Correctly handled — the rejected-alternative narrative shouldn't be cross-linked to a task that doesn't change the rejection logic. No action.

---

## 6. Architectural shortcomings surfaced during execution?

One small one, worth recording.

**Joel's audit methodology missed `caddy-runs-in-container.md:52` despite hitting lines 15 and 58 in the same file.** This is a same-file, same-architectural-event survivor — exactly the failure mode an audit-by-file approach would have caught but an audit-by-grep-pattern approach can miss if the wording varies. Line 52 says "M2's config file lands" while 15 and 58 say "M2 introduces Viper" — same event, different surface phrasing. Raymond caught it because he was reading the file end-to-end during his sweep, which is the discipline that made the fix-while-fresh possible.

**Recommendation for Ward's finalization step**: capture this as a refinement to `_ai/review-discipline/fix-now-while-fresh.md` or as a new lesson — "when auditing a milestone-rename across N files, audit by reading each file end-to-end at least once, not just by grepping for the source token. Variant phrasings of the same architectural event can survive the grep but not the read."

This isn't a blocker — Raymond caught the survivor — but the next analogous task could miss its own line-52 if we don't write down the lesson.

---

## 7. Recommended follow-ups (closeout step, non-blocking)

1. **`_ai/cli-flag-surface-coherence.md:29-31`** — Kevlin's §6.3. Either carve out an exception for semantic-token substring assertions (my preference), or document the inconsistency for a future pass that reverts C.3. Ward's call during finalization.
2. **Audit-by-read discipline** — capture the §6 lesson above into `_ai/review-discipline/fix-now-while-fresh.md` or as a new file. Surfaced by Raymond's `caddy-runs-in-container.md:52` catch.
3. **`m1-scope.md:8` minor rhetorical bump** — see §2 above. Trivial; flag only if a future Don is doing an unrelated edit on that file. Not worth a touch on its own.

None of these gate the task.

---

## What this approval covers

- Kent's three test additions (`007` §C.1, §C.2, §C.3) — verified on disk, locked the right contract.
- Rob's three Go-source edits (`008` §"The three edits") — verified on disk, atomic commit discipline honored, constraints not removed.
- Raymond's 12 prescribed doc edits + 1 in-scope fix-while-fresh (`009` §1, §3) — verified on disk, no hallucinations introduced.
- Kevlin's hallucination sweep, surface-coherence check, and four nits (`010` §1–§7) — independently re-verified, all four nits correctly classified non-blocking.
- The four (actually five) user-visible surfaces — all coherent at "M2".
- The strategic ordering — still defensible in the actual edited prose.
- The pointer surface for future-Don — sufficient without a separate decision file.

**APPROVED. PLAN re-entry can declare done.** Ward and Andy proceed to finalization.

---

## Files referenced

- `/Users/fenster/dev/decloud/_tasks/2026-04-28-milestone-resequence/001-user-request.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-milestone-resequence/002-don-plan.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-milestone-resequence/003-joel-tech-plan.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-milestone-resequence/004-linus-plan-review.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-milestone-resequence/005-joel-tech-plan-v2.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-milestone-resequence/006-linus-rereview.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-milestone-resequence/007-kent-tests.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-milestone-resequence/008-rob-impl.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-milestone-resequence/009-raymond-docs.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-milestone-resequence/010-kevlin-review.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-milestone-resequence/011-linus-impl-review.md` (this file)
- `/Users/fenster/dev/decloud/_ai/decisions/m1-scope.md`
- `/Users/fenster/dev/decloud/_ai/decisions/schema-versioning.md`
- `/Users/fenster/dev/decloud/_ai/decisions/secrets-split.md`
- `/Users/fenster/dev/decloud/_ai/decisions/caddy-runs-in-container.md`
- `/Users/fenster/dev/decloud/_ai/decisions/m1-test-strategy.md`
- `/Users/fenster/dev/decloud/_ai/MEMORY.md`
- `/Users/fenster/dev/decloud/_ai/m1x-backlog.md`
- `/Users/fenster/dev/decloud/_ai/container-naming.md`
- `/Users/fenster/dev/decloud/_ai/cli-flag-surface-coherence.md`
- `/Users/fenster/dev/decloud/_ai/review-discipline/fix-now-while-fresh.md`
- `/Users/fenster/dev/decloud/_docs/install.md`
- `/Users/fenster/dev/decloud/_docs/usage.md`
- `/Users/fenster/dev/decloud/internal/cli/deploy_service.go`
- `/Users/fenster/dev/decloud/internal/cli/deploy_service_test.go`
- `/Users/fenster/dev/decloud/internal/registry/store.go`
- `/Users/fenster/dev/decloud/internal/registry/store_test.go`
- `/Users/fenster/dev/decloud/internal/registry/errors.go`
