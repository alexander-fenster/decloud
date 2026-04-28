# 005 — Joel's tech plan v2 (addendum: Linus C1/C2/C3)

This is an addendum to `003-joel-tech-plan.md`, addressing the three conditions Linus raised in `004-linus-plan-review.md` §F. Sections of v1 not mentioned here are unchanged.

## Section A: Pre-addendum verification

Before specifying inserts I re-read `_ai/decisions/m1-scope.md` (35 lines, current state on disk — no v1 substitutions have been applied yet, since Raymond hasn't executed). I verified the bytes of the lines I'm about to anchor against. I also re-checked Linus's four "other observations" from his §A and §E.

**Linus's other observations — confirm or push back:**

1. *"Don's coupling analysis is right (M2 needs no Viper; M3 introduces Viper; M4 needs Viper-shaped config-loading)."* **Confirmed.** I re-read `internal/registry/store.go` and `internal/cli/deploy_service.go` line ranges that handle `--mount` and the `Mounts` field; the existing plumbing uses per-service TOML and `os.Getenv("DECLOUD_ROOT")`. Enabling mounts in M2-new is removing the rejection path, not adding global config-loading. Linus's chain `mounts (M2, no Viper) → bootstrap (M3, introduces Viper) → blue/green (M4, consumes Viper)` is correct.

2. *"The no-decision-file call is right."* **Confirmed.** v1 §A.4 stands. `_ai/decisions/` is for architecture, not roadmap deltas.

3. *"The verbosity call (terse `m1-test-strategy.md` footnote) is right."* **Confirmed.** v1 §A.5 stands.

4. *"README has zero milestone references."* **Confirmed.** I ran `grep -E "M[1-9]" README.md` — exit 1, zero matches. Linus's claim verified independently.

No pushback on any of the four. v1 §E cross-reference audit also stands — re-checking `_docs/install.md` and `_docs/usage.md` milestone references confirms every site outside §B is either historical (M1.0 install, "M1 ships unit-tests-only") or names an unchanged future milestone (M4, M5, M6).

---

## Section B: Condition C1 — "no global config in new-M2"

**Where it goes:** Inside `_ai/decisions/m1-scope.md`'s "Explicit M1 cuts" section (§ at line 11), specifically as an addendum to the existing line-18 bullet about Viper. That bullet is the natural home — it's where the "no Viper in M1, M2 introduces Viper" claim lives, and the C1 sentence is a clarification of *which* milestone now introduces Viper plus a guard against ad-hoc M2 config-loading.

**Critically:** Raymond's v1 §B.1.5 already edits this same line to flip `M2 introduces Viper` → `M3 introduces Viper`. The C1 sentence rides on top of that v1 edit. **The v1 §B.1.5 substitution must be replaced by the v2 substitution below.** Single Edit, supersedes B.1.5.

**Edit (supersedes v1 §B.1.5):**

`old_string`:
```
- **No Viper** — plain Cobra + `os.Getenv("DECLOUD_ROOT")` is three lines. M2 introduces Viper when there's a real `/etc/decloud/config.toml` to read.
```

`new_string`:
```
- **No Viper** — plain Cobra + `os.Getenv("DECLOUD_ROOT")` is three lines. M3 introduces Viper when there's a real `/etc/decloud/config.toml` to read. M2 introduces no global config or Viper plumbing — that lands at M3. M2's mount config is per-service via `--mount` and the existing `Run.Mounts` field reserved at M1; do not "helpfully" add `/etc/decloud/config.toml` parsing in M2 for default-mount-options or similar (that is the Option C ad-hoc-loading trap rejected in `_tasks/2026-04-28-milestone-resequence/002-don-plan.md` §"Justification").
```

**Why this site, not §B.1.6:** Linus offered both as candidates ("§B.1.6 new_string, or as a separate edit to ... 'Explicit M1 cuts' section"). I'm choosing "Explicit M1 cuts" because:

- §B.1.6 is the canonical roadmap one-liner, already long; bolting an Option-C-trap warning onto it dilutes the roadmap signal.
- "Explicit M1 cuts" is the section future-Don re-reads when planning M2 (it's where every M1-scope cut points at the milestone that picks it up). The Viper-introduction claim is already there at line 18. C1 is "where Viper lands AND why M2 must not pre-empt it" — same bullet, one continuation sentence is the right home.
- Future contributor planning M2 reads "Explicit M1 cuts → Viper bullet" before they touch any code; the warning is exactly where they'll see it.

---

## Section C: Condition C2 — "M7 is a re-plan candidate"

**Where it goes:** Inside `_ai/decisions/m1-scope.md`'s "Milestone sequence (M1 → M7)" section, specifically appended to the new-line-32 wording produced by v1 §B.1.6. That new_string already mentions M7 as the "operational polish (supervisor, deploy locks, secret-files-on-disk, client binary, etc.)" bucket — the C2 sentence is the natural follow-on declaring that M7's bundling is provisional.

**Critically:** v1 §B.1.6 will be superseded by the v2 edit below. Single Edit, supersedes v1 §B.1.6.

**Edit (supersedes v1 §B.1.6):**

`old_string`:
```
M1 service deploy MVP → M2 host bootstrap (introduces Viper) → M3a server-side mounts/secret-files/env hardening + M3b client binary → M4 zero-downtime blue/green via Caddy admin API → M5 jobs (systemd timers) → M6 backups + image GC → M7 operational polish (supervisor, deploy locks, etc.).

Don't reopen this sequencing without a concrete reason. Linus approved the bones in `_tasks/2026-04-26-readme-implementation-planning/07-linus-review-v2.md`.
```

`new_string`:
```
M1 service deploy MVP → M2 server-side mounts + env-file hardening (`--mount` flag, loader populates `Mounts`) → M3 host bootstrap (introduces Viper, `caddy.image` config knob) → M4 zero-downtime blue/green via Caddy admin API → M5 jobs (systemd timers) → M6 backups + image GC → M7 operational polish (supervisor, deploy locks, secret-files-on-disk, client binary, etc.).

M7 is the deferred-feature bucket and will be re-planned at M7-start time, possibly split into multiple milestones then. Bundling client binary + secret files + operational polish there is bin-packing convenience, not a commitment to ship them as one milestone — do NOT repeat the M3a/M3b mistake by treating "everything in M7" as a single deliverable.

Don't reopen this sequencing without a concrete reason. Linus approved the bones in `_tasks/2026-04-26-readme-implementation-planning/07-linus-review-v2.md`. The 2026-04-28 resequence (`_tasks/2026-04-28-milestone-resequence/`) re-ordered M2/M3 and split former M3a/M3b across M2/M7 per maintainer priority; Linus's approval of the original bones still applies to M1's content, which is unchanged.
```

**Why this site, not "Explicit M1 cuts":** Linus offered both. I'm choosing the "Milestone sequence" section because:

- Anyone "planning M7" will land on the canonical roadmap line first (it's the one place that names every milestone in order). The C2 warning needs to live where the M7 label is defined, not buried in a different section.
- "Explicit M1 cuts" is about M1 deliberately *omitting* things; M7 is about *bundling* deferred things. Different rhetoric, different place.
- Linus's stated goal — "prevents the M3a/M3b mistake from re-occurring at M7" — is best served by putting the warning adjacent to the line that lists M7's contents. Future-Don reading the roadmap sees the bundle AND the disclaimer in the same eye-jump.

The C2 sentence specifically calls out "do NOT repeat the M3a/M3b mistake" as a name-named gotcha, because Linus's review specifically cited that pattern as the failure mode to prevent.

---

## Section D: Condition C3 — Rob's single-commit constraint and TDD-paired failure

v1 §D execution order is preserved with one explicit constraint added at step 2.

**Restated v1 §D execution order with C3 added:**

1. **Kent** writes the three new test additions (§C.1, §C.2, §C.3 of v1). Tests will FAIL today against the current source ("M3" wording). Kent commits them anyway. Kent's report explicitly notes the failures are TDD-expected: the source edits in §B.11 are paired commits that flip the failures to passes.

   **Between Kent's commit and Rob's commit, `go test ./...` is RED.** This is by design — the test red-bar IS the TDD discovery surface that locks Rob's edits in place. **No automated CI gate exists in this repo today** (verified by reading `_docs/install.md` §3 — operator-runs-`go test` workflow only), so the red bar is a developer-local signal, not a CI-blocking signal. If Rob's edit is delayed for any reason (review questions, Donald-Knuth escalation, etc.), the red bar persists until Rob lands. **No third party touches main between Kent's commit and Rob's** — workflow is sequential.

2. **Rob** lands the three Go-source edits (§B.11.1 flag-help, §B.11.2 runtime error, §B.11.3 loader error) **as a single atomic commit, never half-landed.** The three edits are one logical change: they flip the user-facing milestone label from "M3" to "M2" across all three operator-visible surfaces (`--help` text, `--mount` flag rejection, hand-edit-loophole rejection). Splitting them across commits creates an interim binary where, e.g., `--help` says `M2 only` but the runtime rejection still says `not supported until M3` — that's exactly the cross-surface incoherence `_ai/cli-flag-surface-coherence.md` was written to prevent.

   After Rob's single commit lands: `go test ./...` is GREEN (Kent's three new substring assertions now pass). Rob runs `gofmt -l .` (must be empty), `go vet ./...` (must be empty), `git status --porcelain` (must show only this commit's expected diffs). Rob's report confirms green.

3. **Raymond** executes the doc edits per v1 §D.3 sub-order (B.1 → B.10), with two substitutions:
   - v1 §B.1.5 is **superseded by Section B of this addendum** (v2 §B above).
   - v1 §B.1.6 is **superseded by Section C of this addendum** (v2 §C above).
   - All other v1 §B edits stand unchanged.

4. **Kevlin and Linus** review in parallel. v1 §D.4 unchanged.

5. **PLAN re-entry** per CLAUDE.md. v1 §D.5 unchanged.

**What happens if `go test ./...` fails between Kent's test additions and Rob's source edits?** It WILL fail — by construction. Kent's three new substring assertions check for "until M2" / "M2 only" / "until M2" against source bytes that currently say "M3". The red bar is the TDD signal, not a bug. Mitigation:

- Kent's report contains an explicit "RED — expected; will go green when Rob lands §B.11" line, so anyone reading the report knows.
- Rob's commit immediately follows Kent's. If something interrupts Rob (Donald-Knuth escalation, etc.), the in-progress branch sits red but is clearly labeled in the task directory.
- **No commit to `main` should leave the tree red.** If Rob's edit is materially delayed (>1 working day), Kent's commit is reverted from main and re-applied later as part of the same TDD pair. This is the standard TDD-commit-pairing discipline — it's worth stating because the alternative ("leave main red overnight") is exactly the kind of thing future-Don will curse our names for.

---

## Section E: Updated total change count

v1 §H.6 said: **24 distinct surgical changes** = 18 doc edits across 9 files + 3 source edits across 2 files + 3 new test assertions across 2 test files.

C1 and C2 are **substitutions of existing v1 edits, not additions.** v1 §B.1.5 is replaced by v2 §B; v1 §B.1.6 is replaced by v2 §C. The number of Edit calls Raymond makes against `m1-scope.md` is unchanged (still six: B.1.1, B.1.2, B.1.3, B.1.4, v2-§B-replacing-B.1.5, v2-§C-replacing-B.1.6).

**Updated total: still 24 distinct surgical changes.** The `old_string`/`new_string` payloads of two of those changes grew (more bytes in C1 and C2), but the count of edits and the count of touched files are unchanged.

If Linus interpreted the question as "two new sentences = two new edits", that mental model double-counts: a `new_string` that contains additional sentences is one Edit call, not two. The atomic unit Raymond executes is the Edit call, and the count of Edit calls is 24.

**Per-file breakdown (unchanged from v1):**

| File | Edit calls |
|---|---|
| `_ai/decisions/m1-scope.md` | 6 (two now carry C1/C2 inserts) |
| `_ai/decisions/schema-versioning.md` | 2 |
| `_ai/decisions/secrets-split.md` | 1 |
| `_ai/decisions/caddy-runs-in-container.md` | 2 |
| `_ai/decisions/m1-test-strategy.md` | 2 |
| `_ai/container-naming.md` | 0 |
| `_ai/MEMORY.md` | 2 |
| `_ai/m1x-backlog.md` | 1 |
| `_docs/install.md` | 1 |
| `_docs/usage.md` | 1 |
| `internal/cli/deploy_service.go` | 2 (Rob, single commit) |
| `internal/registry/store.go` | 1 (Rob, same commit) |
| `internal/cli/deploy_service_test.go` | 2 (Kent: extend §C.1 + new §C.3) |
| `internal/registry/store_test.go` | 1 (Kent: extend §C.2) |
| **Total** | **24** |

---

## Section F: What survives unchanged from v1

To save Linus and Don from re-reading v1:

- §A.1 (grep result, three Go strings, Kent+Rob re-added) — **unchanged.**
- §A.2 (m1x-backlog item 6 wording: name-agnostic) — **unchanged.**
- §A.3 (MEMORY.md schema-versioning entry placement) — **unchanged.**
- §A.4 (no separate `_ai/decisions/milestone-resequence-2026-04.md`) — **unchanged.** Linus approved.
- §A.5 (terse `m1-test-strategy.md` footnote) — **unchanged.** Linus approved.
- §B.1.1 through §B.1.4 — **unchanged.**
- §B.1.5 — **SUPERSEDED by §B of this addendum.**
- §B.1.6 — **SUPERSEDED by §C of this addendum.**
- §B.2, §B.3, §B.4, §B.5, §B.6, §B.7, §B.8, §B.9, §B.10 — **unchanged.**
- §B.11.1, §B.11.2, §B.11.3 — **unchanged** in content; **constrained** by §D of this addendum (single commit, never half-landed).
- §C.1, §C.2, §C.3 (Kent's new tests) — **unchanged.**
- §D execution order — **superseded by §D of this addendum** (Rob's single-commit constraint added; TDD-red-between-Kent-and-Rob explicitly acknowledged and bounded).
- §E cross-reference audit — **unchanged.** Re-verified during this addendum pass; no new sites surfaced.
- §F survives-unchanged sanity checks — **unchanged.**
- §G risk register — **unchanged**, with one note: §G.2 (order-of-edits failure mode) is now strictly tighter under the §D-of-this-addendum single-commit constraint.

## Section G: Sign-off readiness

- C1 addressed: §B above. Verbatim insertion text given. Anchor verified against current bytes of `_ai/decisions/m1-scope.md:18`.
- C2 addressed: §C above. Verbatim insertion text given. Anchor verified against current bytes of `_ai/decisions/m1-scope.md:32-34`.
- C3 addressed: §D above. Single-commit constraint explicit. TDD-red-between-Kent-and-Rob acknowledged, bounded, and mitigated.
- Linus's other four observations cross-checked: all confirmed (§A above).
- Total change count: 24 (unchanged).

Ready for Linus's sign-off review of this addendum, then Kent/Rob/Raymond execute per §D.

## Files relevant to this addendum (absolute paths)

- `/Users/fenster/dev/decloud/_tasks/2026-04-28-milestone-resequence/001-user-request.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-milestone-resequence/002-don-plan.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-milestone-resequence/003-joel-tech-plan.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-milestone-resequence/004-linus-plan-review.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-milestone-resequence/005-joel-tech-plan-v2.md` (this file)
- `/Users/fenster/dev/decloud/_ai/decisions/m1-scope.md` (C1 anchored at line 18, C2 anchored at lines 32-34)
- `/Users/fenster/dev/decloud/README.md` (Linus's "zero milestone refs" claim verified)
