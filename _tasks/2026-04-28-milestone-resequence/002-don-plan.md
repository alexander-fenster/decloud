# 002 — Don's plan: milestone resequence (docs-only)

## TL;DR

The user has one hard constraint and two soft moves. The hard constraint wins. Everything else falls in line behind it.

**Hard constraint:** `--mount` ships in the next milestone — i.e., what we currently call M3a becomes the new M2.

**Soft moves:**
- Client binary (M3b today) goes far out — fold into M7 polish.
- Secret files go further out — split off from `--mount` into their own dedicated slot.

**The new sequence:**

```
M1 service deploy MVP                                   [SHIPPED]
M2 server-side --mount + env-file hardening            [was M3a, minus secret files]
M3 host bootstrap + Viper + caddy.image config         [was M2]
M4 zero-downtime blue/green via Caddy admin API        [unchanged]
M5 jobs (systemd timers)                                [unchanged]
M6 backups + image GC                                   [unchanged]
M7 secret files + client binary + operational polish   [absorbs M3b + secrets-files; was just polish]
```

Why M2 strips out the secret-files piece: those two pieces of "M3a" (mounts and secret-files-on-disk) are independent. Mounts are what the operator screams about today. Secret files are a security-hygiene improvement nobody is currently blocked on — env-vars-in-TOML covers the actual use case. Couple them only if there's a reason; there isn't.

Why bootstrap doesn't slip too: the user did NOT ask to slip it. `apt install docker && go install` is what `_docs/install.md` already documents and the user is happily living with it. But Viper-and-config-file is a precondition for M4's admin-API endpoint config, M6's GC retention knobs, and M7's lock paths — pushing it past M3 would force every subsequent milestone to keep growing the env-var surface. M3 is the right slot. Locked.

---

## Workflow: skip Kent and Rob, justified

**This is a docs-only task.** Every change in the file change list below is prose in `_ai/*.md` or `_docs/*.md`. No Go file is touched. No test is added or modified. No build runs. `go test ./...` pre- and post-task is identical because no code changes.

Kent's role (test author) presupposes a code change to test. There is nothing to test — the milestone roadmap is a planning document, not a runtime artefact. Adding a test that asserts "the string 'M3a' does not appear in `_ai/decisions/m1-scope.md`" would be a change-detector test, explicitly forbidden by `CLAUDE.md` §1.4.

Rob's role (implementation engineer) presupposes a code change to implement. Same logic. Raymond is the right writer for prose changes; he owns `_docs/` and is the appropriate author for `_ai/decisions/` updates that describe sequencing rather than runtime behavior.

The post-execution PLAN re-entry (Don/Joel/Linus reconfirm done) still happens. Kevlin and Linus still review Raymond's diff. The workflow shape is preserved; we are skipping two roles whose preconditions are not met by this task, not bypassing review.

If during Raymond's pass we discover that a docs change actually requires a code change (e.g., a milestone-gated runtime check that mentions M3a in an error string — see [Open Question 1](#open-questions-punted-to-joel)), Kent and Rob get re-added. Vote that contingency in writing now so we don't have to relitigate.

---

## What does NOT change

These are load-bearing claims elsewhere in the docs that survive the resequence intact. Calling them out so reviewers don't waste a cycle re-checking.

1. **Schema versioning's "shape doesn't change between milestones" promise survives.** `_ai/decisions/schema-versioning.md:11` says "M3 writes `schema_version = 1`. M3 only populates fields that M1 reserved (`Mounts`, future secret-file declarations under `mounts`); the schema *shape* doesn't change." The new M2 (mounts) populates `Mounts` — same field M1 reserved, same `schema_version = 1`, same load path. The new M7 (secret files) populates the same `mounts` shape. Both promises hold. **The escalation rule** (§"Escalation rule") still applies: if mid-implementation we find the schema actually does need to change, we stop and re-plan. That's a rule about discovery during implementation, not about milestone numbering.

2. **Container-naming's M4 boundary holds.** `_ai/container-naming.md` says blue/green needs `decloud-<name>-<deploy-id>` and that's an M4 deliverable. M4 stays where it is in the new sequence. The migration-on-M4-ship-time obligation in `_ai/container-naming.md:9` is unchanged.

3. **Caddy-runs-in-container's "`caddy.image` lands when M2 introduces Viper" claim** at `_ai/decisions/caddy-runs-in-container.md:15` and §"Forward-looking notes" updates from "M2" to "M3" but the substance — that the config knob arrives with the Viper introduction, whichever milestone that is — is unchanged. Mechanical rename only.

4. **M1 is shipped and frozen.** `_ai/decisions/m1-scope.md` §"Why this and not the obvious alternatives" and §"Recreate downtime and step ordering" describe what M1 actually delivers. Nothing about M1's *content* changes. Only the forward-looking sentence at line 32 changes. M1's loader still rejects `--mount` and non-empty `Mounts` exactly as `_ai/decisions/secrets-split.md:24` describes; the rejection just unblocks one milestone earlier than previously planned (M2-new instead of M3a).

5. **m1x-backlog.md item 6** ("Docker-compose-based smoke integration test... M2 material") refers to "the next post-M1 milestone where we touch real Docker for the first time." The text says "M2 material" because that was the next milestone. After the swap, the next milestone is *also* still called M2 — it's just doing mounts instead of bootstrap. **The integration test still belongs there**: server-side `--mount` is the first feature post-M1 that actually exercises Docker volume semantics on a real daemon, so coupling the smoke test to M2-new is at least as natural as coupling it to M2-old. Update the file's mention of "M2 material" to clarify "post-M1 first real-Docker milestone" rather than tying it to bootstrap specifically.

6. **m1x-backlog.md items 1, 2, 3, 4, 5, 7, 8** are silent on milestone numbering. They survive untouched.

7. **`no-magic-zero-modes.md`'s claim that "M5 workers get a separate `deploy job` command"** — M5 stays M5, claim survives.

8. **All other `_ai/` content** that doesn't mention M2/M3 specifically (envcap-portable-bash, error-wrap-discipline, gomock-*, etc.). Untouched.

---

## Justification: why this ordering is load-bearing under the user's hard constraint

The user's "I need `--mount` next" is non-negotiable. Three orderings would satisfy it; only one is defensible:

**Option A (chosen):** Mounts → bootstrap → blue/green → jobs → backups → secrets-files+client+polish.

**Option B (rejected):** Mounts+secrets-files → bootstrap → blue/green → ... — keeps the old M3a bundle intact, just moves it ahead of bootstrap. Rejected because the user explicitly asked to push secret files further out. Bundling them with mounts means we either ship the user a milestone they didn't ask for ("here's mounts AND secret files when you only asked for mounts") or we delay mounts waiting for the secret-files implementation. Both are wrong answers.

**Option C (rejected):** Mounts → blue/green → bootstrap → ... — slips bootstrap past M4. Rejected because M4's admin-API endpoint needs a config file knob (per `_ai/decisions/caddy-runs-in-container.md` §"Forward-looking notes": "the obvious config knob is `caddy.image = ...`"), and M4's `decloud-<name>-<deploy-id>` migration would benefit from Viper-loadable config for the migration-window controls. Doing M4 before bootstrap forces M4 to invent its own config-loading shape, which then has to be unified with Viper later. Pay the bootstrap tax once, in M3, and M4-onwards inherits a stable config-file plumbing for free.

Option A is the only ordering that:
1. Ships `--mount` in the next milestone (hard constraint, satisfied).
2. Keeps secret files out of that milestone (soft move 2, satisfied).
3. Pushes the client binary to the last polish slot (soft move 1, satisfied).
4. Doesn't force any subsequent milestone to invent ad-hoc config-loading (architectural).
5. Preserves the schema-versioning shape promise (no shape change between milestones; only `Mounts` and the future `mounts.<name>.secret_files` substructure get populated, both of which were reserved at M1).
6. Preserves the M4 blue/green boundary (no milestone moves through M4's slot).

The only architectural concern Option A creates is that M2-new (mounts) ships before Viper exists. That's fine: mounts are a per-service thing configured via `--mount` flag and the per-service TOML, not via the global `/etc/decloud/config.toml`. There is no Viper-shaped reason mounts need bootstrap to land first. The current `os.Getenv("DECLOUD_ROOT")` plumbing handles everything M2-new touches.

---

## Complete file change list

Every file that mentions "M2" or "M3" or "M3a" or "M3b" in a way that the resequence affects. I've grouped by file and called out the exact substantive edit per location. Raymond will execute these.

### 1. `_ai/decisions/m1-scope.md`

The canonical roadmap. Single most important file in this change list.

- **Line 8**: "five lines of substance, exercises none of the design's hard parts. M2." → "five lines of substance, exercises none of the design's hard parts. M3."
- **Line 13**: "Client is M3b." → "Client is M7."
- **Line 15**: "M5/M6/M6/M2 respectively." → "M5/M6/M6/M3 respectively." (jobs / backups / image-GC / bootstrap)
- **Line 16**: "No `--mount` — flag rejected; loader also rejects non-empty `Mounts` (closes hand-edit loophole). M3a." → "No `--mount` — flag rejected; loader also rejects non-empty `Mounts` (closes hand-edit loophole). M2."
- **Line 18**: "M2 introduces Viper when there's a real `/etc/decloud/config.toml` to read." → "M3 introduces Viper when there's a real `/etc/decloud/config.toml` to read."
- **Line 32 (the canonical sequence)**: replace the entire line with:

  > M1 service deploy MVP → M2 server-side mounts + env-file hardening (`--mount` flag, loader populates `Mounts`) → M3 host bootstrap (introduces Viper, `caddy.image` config knob) → M4 zero-downtime blue/green via Caddy admin API → M5 jobs (systemd timers) → M6 backups + image GC → M7 operational polish (supervisor, deploy locks, secret-files-on-disk, client binary, etc.).

- **Line 34**: "Linus approved the bones in `_tasks/2026-04-26-readme-implementation-planning/07-linus-review-v2.md`." → leave the cross-reference, but APPEND a new sentence immediately after: "The 2026-04-28 resequence (`_tasks/2026-04-28-milestone-resequence/`) re-ordered M2/M3 and split former M3a/M3b across M2/M7 per maintainer priority; Linus's approval of the original bones still applies to M1's content, which is unchanged."

### 2. `_ai/decisions/schema-versioning.md`

- **Lines 10–11**: "M1 writes `schema_version = 1`. / M3 writes `schema_version = 1`. M3 only populates fields that M1 reserved..." → "M1 writes `schema_version = 1`. / M2 writes `schema_version = 1`. M2 populates `Mounts`. M7 (secret-files-on-disk) also writes `schema_version = 1` and populates the secret-file substructure under `mounts`. The schema *shape* doesn't change between any of these milestones."
- **Line 16**: "M1 declares the full schema shape... M3 starts populating; no file rewrite, no migration code. An M1-era TOML loads cleanly in an M3 binary..." → "M1 declares the full schema shape... M2 starts populating `Mounts`; no file rewrite, no migration code. An M1-era TOML loads cleanly in an M2 binary because the shape is identical, only the values differ. M7 extends populating to secret-file declarations on the same shape."
- **Line 20**: "If during M1 implementation Kent or Rob discovers..." — leave M1 alone. The escalation rule is M1-specific and survives.

### 3. `_ai/decisions/secrets-split.md`

- **Line 6**: "M3 will add `secrets/<name>/files/` for secret file contents." → "M7 will add `secrets/<name>/files/` for secret file contents (originally planned for M3, deferred per maintainer priority — see `_tasks/2026-04-28-milestone-resequence/`)."
- **Line 24**: "Other loader rejection classes (all map to exit code 10 = `ExitConfigError`): `ErrNotFound`, `ErrSecretsMissing`, `ErrSchemaMismatch` (cross-file mismatch is also rejected), `ErrUnknownField` (strict mode), `ErrMountsNotSupported` (M1), `ErrInvalidStrategy` (M1)." → unchanged. `ErrMountsNotSupported (M1)` stays accurate — that error is M1-specific and goes away in M2-new exactly as before.
- **Line 30**: "**C: defer the split to M3 with a schema bump** — ships M1 with a known security regression and forces M3 to do data migration. No." → unchanged. The rejected-alternative refers to deferring "the env/config split"; that argument doesn't depend on whether secret-files-on-disk is M3 or M7.

### 4. `_ai/decisions/caddy-runs-in-container.md`

- **Line 15**: "No flag, no env var, no TOML override in M1 — that comes when M2 introduces Viper and a real config file." → "No flag, no env var, no TOML override in M1 — that comes when M3 introduces Viper and a real config file." (substantive: was M2-introduces-Viper, now M3-introduces-Viper)
- **Line 58**: "When M2 introduces Viper, the obvious config knob is `caddy.image = "caddy:2.7.6"`. The `DefaultImage` constant becomes the fallback." → "When M3 introduces Viper, the obvious config knob is `caddy.image = "caddy:2.7.6"`. The `DefaultImage` constant becomes the fallback."

### 5. `_ai/decisions/m1-test-strategy.md`

- **Line 7**: "That smoke-test is M2's first feedback signal, not an M1 deliverable." → "That smoke-test is M2-new's first feedback signal (per the 2026-04-28 resequence: server-side mounts), not an M1 deliverable. The original plan tied it to M2-old (host bootstrap); see `_tasks/2026-04-28-milestone-resequence/`."

  (Slightly verbose, but the file specifically calls itself out as the place where future-Don looks; we want the resequence pointer here.)

- **Line 49**: "When the maintainer reports a real-system failure, the unit-test gap that allowed it through becomes M2's first priority." → "When the maintainer reports a real-system failure, the unit-test gap that allowed it through becomes M2-new's first priority." (or simply "the next milestone's first priority" — either works; pick one and be consistent across the file.)

### 6. `_ai/container-naming.md`

- **Lines 5–6**: M1 and M4 named; no M2/M3 mentioned in the body. Lines 1, 11–14 reference "M1–M3" or "M4". Specifically:
- **Line 1**: "Two different naming conventions across milestones." → unchanged.
- **Line 13**: "If you write code in M1–M3 that hard-codes `decloud-<name>` (Caddy `reverse_proxy` directive, stop/remove logic, status lookup), that code MUST be touched in M4." → unchanged. The "M1–M3" range here means "any milestone before blue/green lands"; the new M2 and M3 still both ship `decloud-<name>` (recreate strategy is unchanged through M3-new). Range stays correct.

  **Audit note for Raymond**: confirm by reading `_ai/container-naming.md` line by line that no M2/M3 reference is *content-specific* (i.e., refers to what M2 or M3 *does*). My read says line 13 is the only M2/M3 reference and it's purely a milestone-range bound, not a content claim. If Raymond finds otherwise, flag it.

### 7. `_ai/MEMORY.md`

- **Line 9**: "`decisions/schema-versioning.md` — `pelletier/go-toml/v2` strict mode + `schema_version` integer; bump only on semantic breaks, never preemptively; M1/M3 both write version 1." → "`decisions/schema-versioning.md` — `pelletier/go-toml/v2` strict mode + `schema_version` integer; bump only on semantic breaks, never preemptively; M1/M2/M7 all write version 1 (mounts populate at M2, secret-files at M7)."

  Other lines in this index file don't mention specific milestones beyond "M1 — M7" architectural boundaries that are unchanged.

### 8. `_ai/m1x-backlog.md`

- **Item 6, "Why deferred" paragraph** (around lines 59–61): "M2 material; M2 is also the milestone where reloader stderr `%q` quoting gets revisited, so the integration test naturally covers that improvement too." → "Post-M1 first real-Docker milestone (now M2-new in the 2026-04-28 resequence: `--mount` server-side); the integration test naturally covers that scope plus reloader stderr `%q` improvements when those happen."

  This one is delicate — the original wording bundled "first real-Docker milestone" with "M2 = bootstrap" and "reloader stderr improvements". After the resequence, "first real-Docker milestone" is M2-new (mounts) and "reloader stderr improvements" might still be M3 (bootstrap, where Viper plumbing for the reloader config could naturally happen). Decoupling them in the prose is correct; let Raymond decide the exact phrasing as long as the substance preserves "integration smoke test rides the next post-M1 real-Docker milestone, whichever it is."

### 9. `_docs/install.md`

- **Line 121**: "`state/deploys/` is created here but no M1 code populates it. M2 will write source bundles there for backup." → "`state/deploys/` is created here but no M1 code populates it. M6 will write source bundles there for backup."

  **Wait — verify this claim before edit.** Backups are M6. The original wording said "M2 will write source bundles" but `_ai/decisions/m1-scope.md` line 32 (original) put backups at M6, not M2. **This is a pre-existing bug in `install.md`, surfaced by the resequence audit but not caused by it.** Fix it as part of this task — it's mechanical, same file, on-theme (milestone-references audit), and Linus would (correctly) call us out for missing it during this very re-read.

  Edit becomes: "`state/deploys/` is created here but no M1 code populates it. M6 will write source bundles there for backup."

  Flag this fix-while-fresh in the change list so the diff isn't surprising during review. Cross-references: `_ai/m1x-backlog.md` is silent on `state/deploys/` population; `_ai/decisions/m1-scope.md:32` puts backups at M6.

### 10. `_docs/usage.md`

- **Line 71**: "`--mount` | string (repeatable) | none | no | Rejected with exit 10 in M1. Persistent volumes are M3." → "`--mount` | string (repeatable) | none | no | Rejected with exit 10 in M1. Persistent volumes are M2."

  This is the user-facing rejection message context. The CLI runtime error string itself (`internal/...`) may or may not mention "M3" explicitly — that's [Open Question 1](#open-questions-punted-to-joel). The doc here just says "M2" so the operator knows when to expect it.

### 11. NEW: cross-reference pointer in this task's Decisions index

Add an entry to `_ai/MEMORY.md` under "Architecture decisions" pointing at this task's directory so future-Don finds the resequence rationale without grepping. Wording (one-liner per the file's convention):

> `_tasks/2026-04-28-milestone-resequence/` — 2026-04-28 maintainer-priority resequence: M2/M3 swap, M3b client deferred to M7, secret-files-on-disk deferred to M7. Doesn't change M1, M4, M5, M6 in content.

  Place under "Source-of-truth task artefacts" (the existing list of `_tasks/` cross-references at the bottom of `MEMORY.md`), not under "Architecture decisions" (which is for files within `_ai/`).

---

## Pre-planning verification: execution traces I performed

Per Don-rules, I prove every claim about the current state with a code/file pointer.

**Trace 1: "Canonical milestone sequence lives at `_ai/decisions/m1-scope.md:32`."**
- Read `_ai/decisions/m1-scope.md` line 32: confirmed full M1→M7 sequence is on that single line. The sentence is the single source of truth; everywhere else mentioning a milestone is downstream.
- Other files mentioning milestones (`schema-versioning.md`, `secrets-split.md`, `caddy-runs-in-container.md`, `m1-test-strategy.md`, `container-naming.md`, `MEMORY.md`, `m1x-backlog.md`, `install.md`, `usage.md`) all refer to the canonical sequence by milestone label, not by re-defining the sequence. Verified by reading each of those files.

**Trace 2: "M3a does mounts; M3b does the client binary."**
- `_ai/decisions/m1-scope.md:13`: "Client is M3b."
- `_ai/decisions/m1-scope.md:16`: "No `--mount` — ... M3a."
- Confirmed: original M3 = mounts(a) + client(b) bundle.

**Trace 3: "Schema-versioning's M3 claim is shape-stable."**
- `_ai/decisions/schema-versioning.md:11`: "M3 writes `schema_version = 1`. M3 only populates fields that M1 reserved (`Mounts`, future secret-file declarations under `mounts`); the schema *shape* doesn't change."
- Same file line 16: "An M1-era TOML loads cleanly in an M3 binary because the shape is identical, only the values differ."
- Confirmed: the shape-stability promise is what makes the resequence safe. M2-new and M7 each populate a different subset of M1's reserved shape; shape stability is preserved either way.

**Trace 4: "Secrets split says M3 will add secret-files-on-disk."**
- `_ai/decisions/secrets-split.md:6`: "M3 will add `secrets/<name>/files/` for secret file contents."
- Confirmed.

**Trace 5: "Caddy-runs-in-container says M2 introduces Viper and `caddy.image`."**
- `_ai/decisions/caddy-runs-in-container.md:15`: "that comes when M2 introduces Viper and a real config file."
- Same file line 58: "When M2 introduces Viper, the obvious config knob is `caddy.image = ...`."
- Confirmed: Viper introduction is bundled with bootstrap (M2-old, M3-new).

**Trace 6: "`install.md:121` says M2 will write source bundles."**
- Read `_docs/install.md:121`: "`state/deploys/` is created here but no M1 code populates it. M2 will write source bundles there for backup."
- **Cross-checked against `m1-scope.md:32`**: original sequence puts backups at **M6**, not M2. Contradiction in pre-existing docs, not introduced by this task. See File Change List item 9.

**Trace 7: "`usage.md:71` says persistent volumes are M3."**
- Read `_docs/usage.md:71`: "Persistent volumes are M3."
- Confirmed. Updates to "M2" (M2-new) under the resequence.

**Trace 8: "Container-naming says M4 owns the rename."**
- Read `_ai/container-naming.md:5-6,9-10`: confirmed M1 = `decloud-<name>`, M4 = `decloud-<name>-<deploy-id>`, with M4 explicitly owning the migration.
- M4 stays at M4 in the new sequence; this claim is untouched.

**Trace 9: "m1x-backlog item 6 is M2 material because of bootstrap."**
- Read `_ai/m1x-backlog.md` lines 55–63: item 6 says the smoke test is "M2 material; M2 is also the milestone where reloader stderr `%q` quoting gets revisited."
- The justification is "first real-Docker milestone post-M1." Under the new sequence, M2-new (mounts) is also a real-Docker milestone — actually MORE so than bootstrap, since `--mount` exercises Docker volume semantics that bootstrap doesn't touch. The "M2 material" label survives but the supporting argument needs rephrasing.

**Trace 10: "Caddy `caddy.image` config knob is the only Viper-dependent claim downstream of M2-old."**
- Searched `_ai/decisions/` for "Viper": appears in `m1-scope.md:18` ("M2 introduces Viper") and `caddy-runs-in-container.md:15,58` (`caddy.image` knob).
- No other decision file ties to Viper. Confirmed: the resequence's only architectural ripple is the Viper-introduction milestone moving from M2-old to M3-new.

---

## Open questions — punted to Joel

These are decisions Joel should lock in his tech plan before Linus sees the diff.

**1. Does the M1 runtime error string for `--mount` mention "M3"?**

The doc at `_docs/usage.md:71` says "Persistent volumes are M3" — that's prose. The runtime error string returned by `--mount` flag rejection (and by the loader's `ErrMountsNotSupported`) lives in Go source. If the source string includes the literal text "M3" (e.g., "`--mount` rejected; persistent volumes are M3 work"), changing the doc to "M2" while leaving the binary saying "M3" creates a mismatch the operator will hit.

**Joel's job**: grep `internal/` for the literal string "M3" and "M3a". If found in user-facing error text, flag it — that's a code change, which means Kent and Rob get re-added to the workflow. If found only in test fixtures or comments, judgment call (probably leave; no operator-visible mismatch).

**2. Naming of the "post-M1 first real-Docker milestone" in `m1x-backlog.md` item 6.**

Cosmetic. Either:
- (a) "M2-new (mounts)" — explicit, but makes the backlog file dependent on the resequence label that may itself get re-named later.
- (b) "the next post-M1 milestone where we touch real Docker for the first time" — name-agnostic but wordier.

I lean (b). Joel's choice; Linus will tell us if he disagrees.

**3. Does the new task-cross-reference go in `MEMORY.md`'s "Architecture decisions" or "Source-of-truth task artefacts" section?**

I argued for "Source-of-truth task artefacts" above (it's a `_tasks/` reference, which is what that section is for). Joel can override if he sees a stronger argument. Linus's tiebreaker.

**4. Should we add a one-line decision record at `_ai/decisions/milestone-resequence.md` capturing the rationale?**

Argument for: future-Don reading `MEMORY.md`'s decisions list shouldn't have to dig into `_tasks/` to see the rationale.

Argument against: `m1-scope.md` already gets a sentence appended pointing at the task; adding a separate decision file duplicates the pointer.

I lean against (don't create a new decision file; the appended sentence in `m1-scope.md` plus the `MEMORY.md` task-cross-reference is sufficient). Joel's call.

**5. m1-test-strategy.md, line 7: how verbose should the resequence-pointer be?**

The file is described in `MEMORY.md` as "future-Don's lookup spot for why M1 was unit-tests-only." Its M2 references are about *when the maintainer's smoke-test runs*, not about *what M2 contains*. So the resequence pointer is a footnote, not a content change. Joel decides whether to be terse ("now M2-new per resequence") or verbose (full sentence pointing at the task).

---

## Things I'm locking in now (no Joel/Linus reopening unless Linus brings new evidence)

1. **The new sequence (M1→M7 as listed in TL;DR).** Three orderings considered, only one defensible under the user's stated priorities. Locked.

2. **Skip Kent and Rob.** Justified above, with the contingency clause that Joel's grep result (Open Q1) can re-add them. Locked.

3. **Bootstrap stays at M3 (not slipped further).** User did not ask to slip; M4-onwards depends on Viper-shaped config-loading; argument made above. Locked unless Linus produces a counter-architectural argument.

4. **Secret-files-on-disk goes to M7, NOT bundled with `--mount` in M2-new.** User's explicit ask. Locked.

5. **Client binary goes to M7, NOT a new M3b/M4b/etc.** User's explicit ask, plus pragmatic ("git archive | ssh" is fine for now). Locked.

6. **Schema-version stays at 1 across M2-new and M7.** Shape-stability promise unchanged. Locked.

7. **`install.md:121` "M2 will write source bundles" gets fixed to M6 in this same task.** Pre-existing bug, mechanical fix, on-theme. The fix-while-fresh rule applies (`_ai/MEMORY.md` review-discipline `fix-now-while-fresh.md`).

---

## What Raymond does next

1. Read this plan top to bottom.
2. Wait for Joel's tech plan (which expands the file change list with exact diff text where ambiguous, resolves the open questions above) and Linus's review.
3. Once approved, execute the file changes in the order listed (1 through 11). Item 11 (the new MEMORY.md cross-reference entry) goes last.
4. After the doc edits, Kevlin and Linus review in parallel.
5. PLAN re-entry: Don/Joel/Linus confirm done.
6. Ward extracts learnings (one obvious one: the "fix-while-fresh" rule applies to milestone-reference audits as much as code).
7. Andy considers whether agent definitions need updates (probably not — this task exercised the workflow exactly as designed for docs-only work).

## Files relevant to this task (absolute paths)

- `/Users/fenster/dev/decloud/_tasks/2026-04-28-milestone-resequence/001-user-request.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-milestone-resequence/002-don-plan.md` (this file)
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
