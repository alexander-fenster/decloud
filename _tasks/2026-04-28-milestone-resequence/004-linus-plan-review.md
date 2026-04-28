# 004 — Linus's plan review: milestone resequence

## Verdict up front

**APPROVED WITH CONDITIONS.**

The strategic call is correct. The architecture-coupling reasoning is correct. The contingency triggered correctly. Joel's two overrides of Don are correct. The task that was supposed to be docs-only became docs-plus-three-Go-strings, which is exactly what the contingency was designed to catch — system worked.

But there are three things I want addressed before Kent/Rob/Raymond touch a key, plus one thing I want acknowledged in writing. None of them blow up the plan; they tighten it.

Conditions in §F at the bottom. Read the analysis first.

---

## A. Is the new sequence defensible?

Yes, and Don actually answered the architectural-coupling question correctly. I'm going to spell out why so the next archaeologist doesn't have to reconstruct it.

The user asked one question (is `--mount` next?) and Don answered it plus two more the user didn't ask but had to answer to ship `--mount`:

1. **Does new-M2 (mounts) need new-M3 (Viper) to land first?** No. Don's argument at his §"Justification" final paragraph is right: mounts are configured per-service via `--mount` and the per-service TOML (`Mounts` field already reserved at M1), not via the global `/etc/decloud/config.toml`. The `os.Getenv("DECLOUD_ROOT")` plumbing handles every path mounts touch. No Viper required. **I verified this**: the existing `internal/registry/store.go` and `internal/cli/deploy_service.go` already plumb the mount field through (just to reject it); enabling it is removing the rejection, not adding new config-loading.

2. **Does new-M3 (host bootstrap) need anything new-M2 produces?** No. Bootstrap is `apt install docker caddy && systemctl enable decloud.service` plus Viper plumbing. None of those depend on whether `--mount` works.

3. **Does M4 (blue/green) need bootstrap to land first?** Yes — Don's §"Justification" Option C analysis is correct. M4's admin API endpoint wants a config knob, M4's `decloud-<name>-<deploy-id>` migration wants Viper-loadable config, and forcing M4 to invent ad-hoc config-loading guarantees a refactor later. So bootstrap CAN'T slip past M4. M3 is the right slot.

The chain `mounts (M2-new, no Viper need) → bootstrap (M3-new, introduces Viper) → blue/green (M4, consumes Viper)` has zero hidden dependencies. The sequence holds.

**One thing Don glossed over and I'm flagging:** mount-related TOML config lives under `Run.Mounts` per-service. There's no global mount config in the new M2. If anyone is tempted to add `mount.default_options` or similar to a global TOML in M2 *before* Viper exists, that's a smell — global config without Viper is exactly the ad-hoc-loading trap Option C was rejected for. **The new M2 must not introduce any global config knob.** I want this written down somewhere — either Don's plan adds a sentence to `m1-scope.md`'s edit, or Joel adds it explicitly to the §B.1.6 new_string. See condition C1.

---

## B. The "absorb into M7" call

Lumping client binary + secret files + operational polish into M7 is bin-packing. The user explicitly said "client binary can come later" and "move secret files to a later milestone" — both got the same later. Is that OK?

**I think yes, with one piece of evidence to lock the call.**

The reason "M7 = polish + everything we deferred" is *not* the same overscoping mistake as M3a/M3b is structural: M3a and M3b were two pieces of *the same milestone* (the user-fronted server-side deploy work). They got bundled because they shipped together and shared review surface. M7-as-currently-defined was already the operational-polish bucket — supervisor, deploy locks, etc. Adding secret-files-on-disk and the client binary to it doesn't bundle two pieces of one feature; it bundles N pieces of "stuff that's not blocking M2-M6".

But I want to test whether M7 becomes a tar pit:

- **Supervisor (deploy locks, lockfile path):** small, mechanical, single-purpose.
- **Client binary:** depends on M2-M6 having stable server-side surfaces; can ship independently with no schema changes.
- **Secret files on disk (`secrets/<name>/files/`):** small data-shape addition, leverages M1's reserved schema, no migration code per `_ai/decisions/schema-versioning.md`.
- **Operational polish (whatever else accumulates):** undefined surface area.

Three of four items are bounded. The fourth ("operational polish") is the tar-pit risk. **If by the time M7 starts, "polish" has accumulated more than what fits in two weeks of work, M7 itself gets resequenced.** That's not this task's problem.

**The "should secret files stay distinct as M7a/M7b?" question:** No. M7a/M7b was the M3a/M3b pattern, and that pattern is what the user just told us he didn't like. The right move when M7 starts is: re-plan it then, with then-current priorities, and split if needed at that time. We don't pre-split now.

Joel didn't say this explicitly. Don didn't say this explicitly. **I want one sentence in the plan acknowledging "M7 is the deferred-feature bucket and will be re-planned at M7-start time, possibly split into multiple milestones then."** See condition C2.

---

## C. Joel's two overrides of Don

### C.1 The decision-file question (Joel: no separate `_ai/decisions/milestone-resequence-2026-04.md`)

**Joel is right. Don's lean was correct, Joel's hardening of it is correct.**

`_ai/decisions/` is for enduring architectural choices. A milestone resequence is a *roadmap change*, not an architecture change. The shape of the schema didn't change. The container naming didn't change. The Caddy network model didn't change. What changed is the *order* features land. Putting that in the architecture-decisions folder confuses two different kinds of records: "what is the system's shape" vs "what are we building next."

Joel's specific argument is the strong one: if we add a decision file every time we resequence, `_ai/decisions/` becomes a journal of priority changes rather than an architecture record. That's the exact failure mode `_ai/MEMORY.md` and the index discipline exist to prevent.

The three breadcrumbs Joel cites (`m1-scope.md` appended sentence, `MEMORY.md` task pointer, the task directory itself) are sufficient. Future-Don finds the rationale by reading `m1-scope.md` (already on his lookup path) and following the appended pointer.

**Approved.**

### C.2 The verbosity of the `m1-test-strategy.md` footnote (Joel: terse, milestone-agnostic)

**Joel is right.** The substantive claim of `m1-test-strategy.md` is "M1 ships unit-tests-only; the manual smoke-test bridges to real-system reality." That claim is milestone-numbering-agnostic. Joel's "the next milestone's first feedback signal" wording carries the same meaning and survives future resequences without further edits. Don's verbose footnote couples the file to the 2026-04-28 task pointer, which adds maintenance debt for zero clarity gain.

**Approved.**

---

## D. The contingency that triggered (Kent + Rob re-added)

Don's contingency clause was: "if Joel's grep finds M3 in Go strings, Kent and Rob get re-added." Joel found three Go-source sites and re-added them.

**Three sites is enough.** Here's why:

- **Two of the three sites are user-facing error messages.** `deploy_service.go:72` is what the operator sees when they pass `--mount`. `store.go:69` is what the operator sees when they hand-edit a config TOML to include `Mounts`. Both reference "M3" by name. If the doc says "M2" and the binary says "M3", an operator's bug report is going to start "the docs lied to me" — exactly the doc-fab-class bug `_ai/doc-grep-discipline.md` exists to prevent. We're not giving up that discipline because the diff is "small."

- **One of the three is flag-help text.** `deploy_service.go:61` shows up in `decloud deploy service --help`. Same operator-visible surface. Same discipline applies.

**Could Raymond do the Go edits as part of the doc sweep with `go test ./...` as the gate?** No. Three reasons:

1. The workflow exists because docs-and-code changes need test coverage to lock the new wording. Joel correctly identified that no existing test asserts the literal "M2" substring, so without Kent's three new substring assertions the next docs-grep audit re-discovers the same drift class. Skipping Kent here re-introduces the bug class `cli-flag-surface-coherence.md` was written to prevent.

2. `go test ./...` passing is a *necessary* gate, not a sufficient one. Joel's own analysis at §A.1 makes the point: existing tests assert via `errors.Is` and never against wrapped text, so a binary that silently says "M5" or "next year" or "M3 still" passes `go test ./...` just fine. The discipline test is *new substring assertions*, not "the existing tests still pass."

3. Raymond's job is prose. Asking him to also touch Go source breaks the role separation. If Raymond's edits include Go code, who reviews them? Linus reviews the Go code as Linus, not as Raymond's-output-reviewer. Kent + Rob is the right answer because the Go edit needs a tested commit and the doc edits need a separate commit.

**Approved. Kent + Rob re-added is correct.** Joel's §A.1 reasoning matches mine.

One detail Joel got right and I want to call out: Joel proposes the test names match the four-surface contract (`cli-flag-surface-coherence.md`). The existing `_ai/cli-flag-surface-coherence.md` documents the four surfaces (runtime check, error string, `--help` text, `_docs/usage.md`). Three of those four are now locked by new tests in §C of Joel's plan; the fourth is locked by the doc edit in §B.10. **No new `_ai/` content is needed** — Joel's call to not write a new pattern doc is right; this task exercises the existing one.

---

## E. Anything Don and Joel missed

I checked:

- **README.md milestone references:** **Zero.** Joel's claim verified. `grep -n "M[1-9]\|milestone" README.md` returns empty.
- **`_docs/install.md` milestone references beyond line 121:** other M1 / M1.0 references are historical or current-state, not future-milestone mismatches. Verified by reading the grep output.
- **`_docs/usage.md` milestone references beyond line 71:** other M1 / M4 / M5 references all point at unchanged milestones. Verified.
- **`_ai/decisions/secrets-split.md:24` (`ErrMountsNotSupported (M1)`):** Don and Joel both flagged this and concluded "no edit". I concur — it's an M1-specific reference (the *rejection* is M1-specific, the *enabling* is M2-new). Stays.
- **`_ai/decisions/secrets-split.md:29` (rejected alternative C "defer the split to M3"):** Don and Joel both flagged. I concur it's about a different deferral (the env/config split, not secret-files-on-disk). Stays.
- **`_ai/decisions/caddy-runs-in-container.md:53` ("Concurrent deploys, theoretical M2+"):** Joel flagged in §B.4. I concur — "M2+" is "after M1", which is true under both old and new sequences. Stays.
- **`_ai/m1x-backlog.md` items 1–5, 7, 8:** silent on M2/M3 in any way that needs editing. Verified by reading the file. Stays.
- **Comments in Go source naming M2/M3:** `internal/ids/ids.go:20-21` names M1 and M4 (M4 unchanged); `internal/caddy/manager.go:97` names M1.0 (historical). Joel triaged correctly. Stays.

**Joel's cross-reference audit (§E of his plan) is accurate. I find no missed sites.**

The blog-post-style wording in `_docs/install.md` and `_docs/usage.md` doesn't change in *substance* under the new sequence beyond the literal M2/M3 tokens and the install.md:121 fix. The only operator-visible meaning shift is "if you pass `--mount`, the rejection mentions M2 not M3" — exactly what Joel's plan covers via §B.10 + §B.11.

---

## F. Conditions before execution

Three things to fix in the plan before Kent/Rob/Raymond touch anything. None require a re-iteration of the whole plan; they're surgical additions to existing edits.

### Condition C1: Lock down "no global config in new-M2"

Add to `m1-scope.md`'s new line-32 wording (Joel's §B.1.6 new_string), or add as a separate edit to `_ai/decisions/m1-scope.md`'s "Explicit M1 cuts" section, a sentence stating:

> M2 introduces no global config file or Viper plumbing — that lands at M3. M2's mount config is per-service via `--mount` and the existing `Run.Mounts` field reserved at M1.

Why: prevents a future contributor from "helpfully" adding `/etc/decloud/config.toml` parsing in M2 because they need a default-mount-options knob or similar, which would force the M3 Viper introduction to merge with whatever ad-hoc loading M2 invented. This is the exact Option C trap Don rejected; we want it on the page.

### Condition C2: Acknowledge M7 is a re-plan candidate

Add to `m1-scope.md`'s new line-32 wording (Joel's §B.1.6 new_string), or as a footnote in the same file's "Explicit M1 cuts" section, a sentence stating:

> M7 is the deferred-feature bucket and will be re-planned at M7-start time, possibly split into multiple milestones then. Bundling client binary + secret files + operational polish there is bin-packing convenience, not a commitment to ship them as one milestone.

Why: prevents the M3a/M3b mistake from re-occurring at M7. The current bundle is fine for *roadmap* purposes; what we don't want is a future-Don trying to ship one giant M7 because "the doc said it's all M7". Re-plan at M7-start is the contract.

### Condition C3: Order the three Go-source edits so the binary never disagrees with itself mid-task

Joel's §D execution order has Kent first (tests fail), Rob second (tests pass), Raymond third (docs). That's correct for the doc/test/source coherence story.

What's missing: a one-line note in Joel's §D that **Rob commits the three source edits in §B.11 as a single commit** (or three commits in immediate sequence with no review delay between them). The risk if not: if Rob commits §B.11.1 (flag help "M2 only") and stops for review, but `deploy_service.go:72` still says "M3" and `store.go:69` still says "M3", the binary's `--help` output disagrees with its runtime error. Three Go source edits, one logical change, one commit. Make this explicit so Rob doesn't half-land it.

### Acknowledgement (not a blocker)

Joel's §G.1 already flags "the docs-only-became-code-touching" risk for Linus's review surface. **Acknowledged.** The contingency clause Don wrote did its job. I'm reading Joel's plan §A.1 before the Go diff exists, so I know what's coming. This is the workflow working as designed. No further action needed; just noting that the contingency is now battle-tested and worth keeping in Don's playbook.

---

## G. What this approval covers

1. The new milestone sequence (M1 / M2 mounts+env / M3 bootstrap+Viper / M4 blue-green / M5 jobs / M6 backups+GC / M7 polish+secrets+client). **Locked.**
2. Skipping Kent and Rob is *not* approved — Don's contingency triggered correctly, Kent and Rob ARE included. **Locked.**
3. Three new test substring assertions per §C of Joel's plan. **Approved.**
4. Three Go-source edits per §B.11 of Joel's plan. **Approved with C3.**
5. Doc edits §B.1 through §B.10 of Joel's plan. **Approved with C1 and C2.**
6. The `_docs/install.md:121` "M2 will write source bundles" → "M6" fix as fix-while-fresh. **Approved.** Joel's two-sentence justification (couple to canonical roadmap, don't lose the signal that catches future drift) is correct.
7. No new `_ai/decisions/` file. **Approved.**
8. Terse milestone-agnostic phrasing for `m1-test-strategy.md`. **Approved.**

## H. What happens next

1. Don and Joel address C1, C2, C3 — three small additions, no architectural change. Single edit pass.
2. Joel's tech plan gets a 005-marked addendum or in-place revision capturing C1/C2/C3.
3. Kent writes the three test additions per §C.
4. Rob lands the three source edits per §B.11 as a single commit, runs `go test ./...`, confirms Kent's tests pass.
5. Raymond executes the doc edits per §D's order.
6. Kevlin + Linus review in parallel. Per CLAUDE.md, Kevlin gets the doc-hallucination check; I get the architectural one.
7. PLAN re-entry, Don/Joel/Linus reconfirm.
8. Ward extracts learnings. The big one: "docs-only audits MUST grep source for milestone labels before declaring docs-only" — this task added that gate via Don's contingency clause and Joel's grep, and it caught real divergence.

If Don and Joel disagree with C1, C2, or C3, the burden is on them to argue why. C3 is mechanical and uncontroversial. C1 and C2 are about preventing future Don from re-walking ground we just covered; the cost of writing them down is one sentence each.

---

## Files referenced

- `/Users/fenster/dev/decloud/_tasks/2026-04-28-milestone-resequence/001-user-request.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-milestone-resequence/002-don-plan.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-milestone-resequence/003-joel-tech-plan.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-milestone-resequence/004-linus-plan-review.md` (this file)
- `/Users/fenster/dev/decloud/_ai/decisions/m1-scope.md`
- `/Users/fenster/dev/decloud/_ai/decisions/schema-versioning.md`
- `/Users/fenster/dev/decloud/_ai/decisions/secrets-split.md`
- `/Users/fenster/dev/decloud/_ai/decisions/caddy-runs-in-container.md`
- `/Users/fenster/dev/decloud/_ai/decisions/m1-test-strategy.md`
- `/Users/fenster/dev/decloud/_ai/container-naming.md`
- `/Users/fenster/dev/decloud/_ai/MEMORY.md`
- `/Users/fenster/dev/decloud/_ai/m1x-backlog.md`
- `/Users/fenster/dev/decloud/_docs/install.md`
- `/Users/fenster/dev/decloud/_docs/usage.md`
- `/Users/fenster/dev/decloud/internal/cli/deploy_service.go`
- `/Users/fenster/dev/decloud/internal/registry/store.go`
- `/Users/fenster/dev/decloud/README.md` (verified zero milestone references)
