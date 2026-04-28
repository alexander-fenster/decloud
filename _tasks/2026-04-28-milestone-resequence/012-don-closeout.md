# 012 — Don's closeout

## Verdict

**DONE.** Ward + Andy proceed to finalization.

I spot-verified the actual bytes in `m1-scope.md:1-36`, `caddy-runs-in-container.md:48-58`, `internal/cli/deploy_service.go:55-79`, and `internal/registry/store.go:63-74`. Reports match disk. Linus and Kevlin both signed off; their nits are correctly classified non-blocking.

## 1. User goal — all four shipped

| User ask | Status | Evidence |
|---|---|---|
| Swap M2/M3 | DONE | `m1-scope.md:32` — "M2 server-side mounts → M3 host bootstrap (introduces Viper)". |
| `--mount` is top priority | DONE | M2-new is `--mount` + env-file hardening; lifts in the very next milestone. Five surfaces (`--help`, CLI runtime, loader runtime, `usage.md:71`, roadmap line) all say M2 coherently. |
| Move client binary further out | DONE | `m1-scope.md:13` — "Client is M7." Folded into M7 polish bucket. |
| Move secret files further out | DONE | `secrets-split.md:6` — "M7 will add `secrets/<name>/files/`"; `schema-versioning.md:10-11` confirms M7 populates the secret-file substructure on the same shape. |

The user's hard constraint (`--mount` ships next) and both soft moves are honored. No scope drift.

## 2. Linus's plan-review conditions (006) — all addressed

- **C1 (no global config in M2):** Shipped at `m1-scope.md:18`. The Viper bullet now carries the Option-C trap warning naming the rejected path by name (`/etc/decloud/config.toml` parsing for default-mount-options) and points to Don's plan §"Justification". Future-Don planning M2 reads it before touching code. Verified on disk.
- **C2 (M7 is provisional re-plan bucket):** Shipped at `m1-scope.md:34`. The "do NOT repeat the M3a/M3b mistake" paragraph sits adjacent to the canonical roadmap line where M7 is defined — exactly where future-Don will be when planning M7. Verified on disk.
- **C3 (Rob's single-commit constraint):** Honored. Rob's `008` report is explicit: three M3→M2 substitutions in one atomic commit, never half-landed. `gofmt`, `vet`, full test suite all clean post-edit. Five surfaces never disagreed with each other on operator-visible bytes.

Conditions discharged.

## 3. Linus FU#1–#3 and Kevlin §6.1–§6.4 — genuinely non-blocking

Walked each item against the actual diff and the doctrine they touch.

- **Linus FU#1 / Kevlin §6.3 — `TestDeployService_MountFlagHelpReferencesM2` vs `cli-flag-surface-coherence.md:29-31`:** Real doctrinal tension, NOT load-bearing on this task. The test catches real cross-surface drift; the doctrine line says help-text tests are change-detectors. Both have a defensible reading. Linus and I both lean toward **carve-out** (semantic-token-substring assertions on operator-visible bytes are a behavior contract, not a snapshot). Ward should land this as a clarifying amendment to `cli-flag-surface-coherence.md` during knowledge capture, citing this task. **NOT** revert C.3 — that test caught the M3-vs-M2 help drift that motivated this whole code-side excursion. Carve out, don't revert.
- **Linus FU#2 — Joel's audit missed `caddy-runs-in-container.md:52`; Raymond caught it:** Real process lesson. Joel grepped for "M2 introduces Viper" and got lines 15 and 58; line 52 said "M2's config file lands" — same architectural event, different surface phrasing. Raymond caught it by reading the file end-to-end during his sweep. **Discipline lesson worth recording**: when auditing a milestone-rename, audit by reading each touched file end-to-end, not just by grepping for the source token. Variant phrasings of the same event survive grep but not read. Ward should append this to `_ai/review-discipline/fix-now-while-fresh.md` (or a sibling file under `_ai/review-discipline/`). Non-blocking but a real refinement.
- **Linus FU#3 — `m1-scope.md:8` minor rhetorical bump:** I read the line on disk; the M2/M3 cut/cut/cut/cut pattern reads slightly less crisply post-resequence than pre. Trivial. Single-token replacement preserved the substance perfectly. Not worth a re-edit on its own; flag only if a future Don is editing that file for another reason.
- **Kevlin §6.1 — `secrets-split.md:29` rejected-alternative-C "M3":** Frozen rejected-alternative narrative. The substantive logic ("ships M1 with a known security regression — No.") survives regardless of which milestone the counterfactual-M3 labeled. Acceptable category for stale labels. Future-Don can reword to "a later milestone" if the bump matters; not today.
- **Kevlin §6.2 — C2 paragraph naming "M3a/M3b mistake":** Subjective decay-over-time. The trailing pointer at line 36 chains to this task directory for any reader who needs the breadcrumb. Forceful naming is the right call for the next 1–3 milestones, which is precisely the horizon where the warning matters. Soften later if needed.
- **Kevlin §6.4 — `secrets-split.md:29` cross-link absence:** Correctly handled by Joel — adding a resequence pointer to a frozen rejected-alternative would imply the resequence changes the rejection logic, which it doesn't. No action.

Nothing is subtly load-bearing. Verdict holds.

## 4. Andy escalations

**None.** Every agent worked inside their definition.

- Don: planned, traced, locked the strategic call.
- Joel: expanded with verbatim substitutions, surfaced the docs-only-became-code-touching grep result, owned the contingency clause.
- Linus: held the line on C1/C2/C3, signed off twice (plan + impl) with traced rationale.
- Kent: red-bar TDD with explicit "RED — expected" header, caught Joel's `newDeployServiceCommand` placeholder hallucination before it shipped.
- Rob: atomic single-commit, investigated-before-removing-constraints discipline, surfaced the `ErrMountsNotSupported.Error()` "in M1" vs wrap "until M2" co-existence question and got it right.
- Raymond: byte-exact substitutions, caught the line-52 survivor by reading end-to-end, invoked fix-while-fresh under the precedent Joel set.
- Kevlin: hallucination sweep clean, four nits all correctly classified.

The line-52 catch by Raymond is exactly the kind of behavior that justifies the role separation: a different reader doing a different sweep caught what the planner missed. Working as designed. No agent-config update warranted.

## 5. What Ward should capture (for the finalization step)

Two real lessons, both already named in 011 §7:

1. **Audit-by-read, not just audit-by-grep**, when renaming a milestone label across multiple files. Variant phrasings of the same architectural event (line 52: "M2's config file lands"; lines 15/58: "M2 introduces Viper") survive grep but not end-to-end read. Append to `_ai/review-discipline/fix-now-while-fresh.md` or sibling file.
2. **Carve-out for semantic-token substring assertions** on `--help` text — clarify `_ai/cli-flag-surface-coherence.md:29-31` so the doctrine matches current practice. Cite this task as the precedent.

Bonus minor: the fix-while-fresh rule fired twice on this task (Joel at `install.md:121`, Raymond at `caddy-runs-in-container.md:52`), both correctly. The rule is now load-tested across two distinct surfacing patterns (Joel's was a pre-existing bug surfaced by being on-theme; Raymond's was a same-architectural-event survivor of a same-file enumeration). Ward may want to note both flavors so future readers see the rule's full reach.

## Files relevant to this closeout (absolute paths)

- `/Users/fenster/dev/decloud/_tasks/2026-04-28-milestone-resequence/` (entire task directory, reports 001–011)
- `/Users/fenster/dev/decloud/_ai/decisions/m1-scope.md` (verified on disk)
- `/Users/fenster/dev/decloud/_ai/decisions/caddy-runs-in-container.md` (verified on disk, including the Raymond fix at line 52)
- `/Users/fenster/dev/decloud/internal/cli/deploy_service.go` (verified lines 61, 72)
- `/Users/fenster/dev/decloud/internal/registry/store.go` (verified line 69)
- `/Users/fenster/dev/decloud/_ai/cli-flag-surface-coherence.md` (Ward's carve-out target)
- `/Users/fenster/dev/decloud/_ai/review-discipline/fix-now-while-fresh.md` (Ward's audit-by-read target)
