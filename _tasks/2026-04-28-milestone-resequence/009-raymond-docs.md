# 009 — Raymond's docs execution: milestone resequence

## Status

**DONE.** All prescribed doc edits applied per Joel v1 §B (B.1–B.10) with v2 §B and §C superseding v1 §B.1.5 and §B.1.6 respectively. Cross-reference sweep across `_ai/`, `_docs/`, and `README.md` is clean except for one survivor (`caddy-runs-in-container.md:52`) that I fixed in scope under fix-while-fresh — see §3 below.

Total: 13 Edit calls across 9 markdown files (12 prescribed + 1 fix-while-fresh).

## Plan reference

- `_tasks/2026-04-28-milestone-resequence/003-joel-tech-plan.md` §B.1 through §B.10 — verbatim before/after substitutions.
- `_tasks/2026-04-28-milestone-resequence/005-joel-tech-plan-v2.md` §B and §C — supersede v1 §B.1.5 and §B.1.6 respectively.
- `_tasks/2026-04-28-milestone-resequence/006-linus-rereview.md` — final approval.
- `_tasks/2026-04-28-milestone-resequence/008-rob-impl.md` — Rob's Go source edits (already shipped before I started; my scope is markdown only).

## 1. Files touched

In dependency order (canonical roadmap first, then files that quote it).

### `/Users/fenster/dev/decloud/_ai/decisions/m1-scope.md` — 6 Edits

| Edit | Lines | One-line summary |
|---|---|---|
| B.1.1 | 8 | "M2." → "M3." (bootstrap milestone label) |
| B.1.2 | 13 | "Client is M3b." → "Client is M7." |
| B.1.3 | 15 | "M5/M6/M6/M2" → "M5/M6/M6/M3" (jobs/backups/GC/bootstrap) |
| B.1.4 | 16 | "M3a" → "M2" (mount-rejection milestone) |
| v2 §B (supersedes B.1.5) | 18 | Viper line: M2→M3, plus C1 Option-C-trap warning naming Don's plan §"Justification" |
| v2 §C (supersedes B.1.6) | 32–34 | Canonical roadmap rewritten + C2 "M7 is provisional / do NOT repeat M3a/M3b mistake" paragraph + Linus-approval pointer with appended resequence sentence |

### `/Users/fenster/dev/decloud/_ai/decisions/schema-versioning.md` — 2 Edits

| Edit | Lines | One-line summary |
|---|---|---|
| B.2.1 | 10–11 | "M3 writes / only populates Mounts" → "M2 writes (populates Mounts), M7 writes (secret-files); shape unchanged across all" |
| B.2.2 | 16 | "M3 starts populating ... M3 binary" → "M2 starts populating Mounts ... M2 binary; M7 extends to secret-file declarations" |

### `/Users/fenster/dev/decloud/_ai/decisions/secrets-split.md` — 1 Edit

| Edit | Lines | One-line summary |
|---|---|---|
| B.3.1 | 6 | "M3 will add `secrets/<name>/files/`" → "M7 will add ... originally planned for M3, deferred per maintainer priority — see task ref" |

### `/Users/fenster/dev/decloud/_ai/decisions/caddy-runs-in-container.md` — 3 Edits (2 prescribed + 1 fix-while-fresh)

| Edit | Lines | One-line summary |
|---|---|---|
| B.4.1 | 15 | "M2 introduces Viper" → "M3 introduces Viper" |
| B.4.2 | 58 | "When M2 introduces Viper" → "When M3 introduces Viper" |
| **fix-while-fresh** | 52 | "until M2's config file lands" → "until M3's config file lands" (see §3) |

### `/Users/fenster/dev/decloud/_ai/decisions/m1-test-strategy.md` — 2 Edits

| Edit | Lines | One-line summary |
|---|---|---|
| B.5.1 | 7 | "M2's first feedback signal" → "the next milestone's first feedback signal" (terse per v1 §A.5) |
| B.5.2 | 49 | "M2's first priority" → "the next milestone's first priority" |

### `/Users/fenster/dev/decloud/_ai/MEMORY.md` — 2 Edits

| Edit | Lines | One-line summary |
|---|---|---|
| B.7.1 | 9 | "M1/M3 both write version 1" → "M1/M2/M7 all write version 1 (mounts populate at M2, secret-files at M7)" |
| B.7.2 | 56–57 | Appended new bullet under "Source-of-truth task artefacts" pointing at `_tasks/2026-04-28-milestone-resequence/` |

### `/Users/fenster/dev/decloud/_ai/m1x-backlog.md` — 1 Edit

| Edit | Lines | One-line summary |
|---|---|---|
| B.8.1 | 61 | "M2 material; M2 is also..." → "next post-M1 first-real-Docker milestone (the new M2 — server-side `--mount` — per the 2026-04-28 resequence) ... reloader stderr improvements" (name-agnostic per v1 §A.2) |

### `/Users/fenster/dev/decloud/_docs/install.md` — 1 Edit

| Edit | Lines | One-line summary |
|---|---|---|
| B.9.1 | 121 | **Pre-existing-bug fix-while-fresh.** "M2 will write source bundles" → "M6 will write source bundles" (backups have always been M6 per `m1-scope.md:32`; this was wrong before the resequence too) |

### `/Users/fenster/dev/decloud/_docs/usage.md` — 1 Edit

| Edit | Lines | One-line summary |
|---|---|---|
| B.10.1 | 71 | "Persistent volumes are M3" → "Persistent volumes are M2" (operator-facing rejection-context table) |

### `/Users/fenster/dev/decloud/_ai/container-naming.md` — 0 Edits

Per Joel v1 §B.6, the only M2/M3 reference (line 14, "M1–M3" range bound) survives because the range still describes the right set (any milestone before blue/green). Confirmed by reading the file end-to-end. Not touched.

## 2. Cross-reference sweep results

I ran four sweeps across `_ai/`, `_docs/`, and `README.md` (excluding `_tasks/` per instructions).

### Sweep 1: `M3a` and `M3b`

Three matches; all are **intentional retrospective references** introduced by the prescribed edits:

- `_ai/MEMORY.md:57` — task pointer naming "M3b client deferred to M7"
- `_ai/decisions/m1-scope.md:34` — C2 warning ("do NOT repeat the M3a/M3b mistake")
- `_ai/decisions/m1-scope.md:36` — appended Linus-approval-pointer sentence ("split former M3a/M3b across M2/M7")

Verdict: **clean, all intentional.**

### Sweep 2: stale milestone wording (`until M3`, `M3 only`, `M2 host bootstrap`, `M3 will populate`, `M3 introduces Viper`, `M3 will add`, `M3 starts populating`, `M3 binary`)

Three matches, all post-edit and **correct under the new sequence**:

- `caddy-runs-in-container.md:15` — "M3 introduces Viper" ✓
- `caddy-runs-in-container.md:58` — "M3 introduces Viper" ✓
- `m1-scope.md:18` — "M3 introduces Viper" ✓

Verdict: **clean.**

### Sweep 3: alphanumeric milestone refs (`M[1-9][a-z]`)

Same three M3a/M3b retrospective references from Sweep 1. Verdict: **clean.**

### Sweep 4: bare `M2` / `M3` token survey

I dumped every line containing `\bM2\b` or `\bM3\b` and walked each one. Categories:

- **Post-edit, correct under new sequence:** `MEMORY.md:9, 57`, `m1x-backlog.md:61`, `m1-scope.md:8, 15, 16, 18, 32, 36`, `secrets-split.md:6`, `schema-versioning.md:11, 16`, `usage.md:71`, `caddy-runs-in-container.md:15, 58` — all confirmed correct.
- **Joel explicitly considered, intentionally left:**
  - `caddy-runs-in-container.md:53` ("Concurrent deploys (theoretical, M2+).") — Joel v1 §B.4 final note: "M2+ means 'M2 onwards', a milestone-range bound rather than a content claim ... the concurrency claim survives unchanged." ✓
  - `container-naming.md:14` ("If you write code in M1–M3...") — Joel v1 §B.6: "M1–M3 range here means 'any milestone before blue/green lands'; the new M2 and M3 still both ship `decloud-<name>`. Range stays correct." ✓
  - `secrets-split.md:29` ("**C: defer the split to M3 with a schema bump**") — Joel v1 §B.3 final note: "the rejected-alternative refers to deferring the env/config split; that argument doesn't depend on whether secret-files-on-disk is M3 or M7." ✓ (and this is rejected-alternative-C narrative, frozen as historical record)
- **One survivor caught and fixed in scope** (see §3 below): `caddy-runs-in-container.md:52`.

Verdict: **clean after the §3 in-scope fix.**

## 3. Survivor caught: `caddy-runs-in-container.md:52`

**Pattern:** `until M2's config file lands`

**Context:**
> - **`caddy:2` floats.** Operators who pin a tag retag locally as the workaround until M2's config file lands. Documented in `_docs/install.md`.

**Why it's a survivor:** Same architectural event as lines 15 and 58 — the introduction of `/etc/decloud/config.toml` via Viper, which under the new sequence lands at M3, not M2. Joel's audit (v1 §B.4) enumerated lines 15 and 58 but missed line 52. Without the fix the same file would have said "M3 introduces Viper" at lines 15/58 while still saying "M2's config file lands" at line 52 — a same-file contradiction.

**Decision:** Fix in scope under the fix-while-fresh rule (`_ai/review-discipline/fix-now-while-fresh.md`):

- **Mechanical:** single-token rename M2→M3, identical to the treatment of lines 15 and 58.
- **Same file, on theme:** literally next to two lines Joel already prescribed.
- **<5-minute floor:** trivial.
- **Cost of NOT fixing:** future-Don reads the file, sees the contradiction, has to relitigate which milestone owns Viper.

This is exactly the same class of fix as Joel's `install.md:121` "M2 will write source bundles" → "M6" pre-existing-bug fix (v1 §B.9), surfaced by the audit but not caused by the resequence. I'm flagging it here so reviewers see the diff isn't a surprise.

**No deviation from Joel's prescription** — every B.1 through B.10 substitution and the v2 §B / §C supersedes were applied **byte-exact** as written. The §3 fix is one extra Edit beyond Joel's plan, applied under the fix-while-fresh rule that Joel himself invoked at v1 §B.9.

## 4. Files considered but NOT edited

For reviewer completeness, I read or grepped each of these and confirmed no edits needed:

- `_ai/container-naming.md` — Joel v1 §B.6 said "zero edits"; I confirmed (only M2/M3 reference is the "M1–M3" range bound at line 14, which still describes the right set under the new sequence).
- `_ai/cli-flag-surface-coherence.md` — no milestone refs (cited only by Joel/Rob/Linus prose, file itself is mechanism-level).
- `_ai/explicit-inputs-not-globals.md` — mentions "Viper" generically (rejected anti-pattern), no milestone refs.
- `_ai/cobra-init-pattern.md`, `_ai/optional-input-two-layer.md`, `_ai/error-wrap-discipline.md`, `_ai/gomock-*.md`, `_ai/cleanup-context-discipline.md`, `_ai/exit-code-sentinel-not-context-err.md`, `_ai/cancellation-symmetry-audit.md`, `_ai/label-gated-orphan-recovery.md`, `_ai/envcap-portable-bash.md`, `_ai/docker-bridge-dns.md`, `_ai/stderr-substring-canary.md`, `_ai/doc-grep-discipline.md`, `_ai/review-discipline/fix-now-while-fresh.md` — surveyed; only mention M1, M1.0, M4, M5, M6 historically/architecturally; no M2/M3 wording affected by the resequence.
- `_ai/decisions/no-magic-zero-modes.md` — names M5 only; unchanged.
- `_docs/install.md` — only milestone refs other than line 121 are M1/M1.0 historical or unchanged-future-milestone (M4, M5, M6) labels; no other edits needed.
- `_docs/usage.md` — only milestone refs other than line 71 are M1/M4/M5 (all unchanged by resequence); no other edits needed.
- `README.md` — Joel v2 §A and Linus v2 §A independently verified zero milestone refs (`grep -E "M[1-9]" README.md` exits 1). Re-confirmed during my sweep.

## 5. Deviations from Joel's prescription

**One:** the in-scope fix at `caddy-runs-in-container.md:52` (§3 above), justified under fix-while-fresh, parallel to Joel's own fix at `install.md:121`.

Every prescribed B.1–B.10 substitution and v2 §B / §C supersede was applied byte-exact. No paraphrasing.

## 6. Files relevant to this report (absolute paths)

- `/Users/fenster/dev/decloud/_ai/decisions/m1-scope.md`
- `/Users/fenster/dev/decloud/_ai/decisions/schema-versioning.md`
- `/Users/fenster/dev/decloud/_ai/decisions/secrets-split.md`
- `/Users/fenster/dev/decloud/_ai/decisions/caddy-runs-in-container.md`
- `/Users/fenster/dev/decloud/_ai/decisions/m1-test-strategy.md`
- `/Users/fenster/dev/decloud/_ai/MEMORY.md`
- `/Users/fenster/dev/decloud/_ai/m1x-backlog.md`
- `/Users/fenster/dev/decloud/_docs/install.md`
- `/Users/fenster/dev/decloud/_docs/usage.md`
- `/Users/fenster/dev/decloud/_ai/container-naming.md` (considered, zero edits per Joel v1 §B.6)
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-milestone-resequence/003-joel-tech-plan.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-milestone-resequence/005-joel-tech-plan-v2.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-milestone-resequence/006-linus-rereview.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-milestone-resequence/008-rob-impl.md`
