# Linus execution review — README rewrite + MIT LICENSE + `_ai/` sweep

**Reviewer:** Linus Torvalds (high-level)
**Branch:** `task/readme-and-license`
**Reviewed:** commits `ef59c48` (Rob) and `095fe4b` (Raymond) on top of the approved plan.
**Verdict:** **APPROVED.** Squash-merge to `main` after Don/Joel sign-off in PLAN re-entry. One *minor* report-honesty note for Raymond — does not block merge.

---

## TL;DR

Rob honored every required tightening from `04-linus-review.md`. He pasted Joel's verbatim shell blocks, applied the two mechanical edits I demanded, and made the optional-nit call I delegated to him with a defensible reason. The README reads confident, not apologetic. It's 154 lines — within Joel's target, well under the 200-line ceiling, and the cut from 278 lines of stale design narrative is exactly the right ratio.

Raymond's `_ai/decisions/` sweep was the right call (concur with Don's pre-emptive framing) and caught a genuinely broken `README.md:215` line-number cite that would have rotted further. The sweep was in scope because it cleaned up debris created by Rob's commit. **However**, Raymond's report under-described what he actually changed; two of his edits go beyond the "stale-README-tense" framing he claimed. The substance of those edits is correct — they're M2-shipped accuracy fixes — but his report should have disclosed them. Flagging for the record; not a merge blocker.

No iteration needed. PLAN re-entry can run.

---

## 1. Rob's required-tightening compliance check

I gave Rob six concrete things to honor in `04-linus-review.md`. Going through them in order against the artifact at HEAD (`/Users/fenster/dev/decloud/README.md`):

### 1.1 No redundant `[Roadmap](#roadmap)` link in the sub-list lead-in (my §2.1)

**Honored.** Line 16 is `**Not yet shipped:**` — bare label, no inline link. The intro paragraph above it (line 9) carries the single Roadmap link: `See the [Roadmap](#roadmap) for what's next.` One link, in the right place. Rob got the discipline I asked for.

### 1.2 Omit `--readiness-path` from the Quick start example (my §2.2)

**Honored.** Quick start shell block (lines 41–45) lists only `--name`, `--host`, `--port`, and the positional source dir. No `--readiness-path`. Default `/healthz` will do its work. Teachable failure beats explanatory bloat in the minimum-working command. Correct.

### 1.3 Brace-expansion `mkdir` (my §1.4 / Joel's §2.3)

**Honored.** Line 30:

```sh
sudo mkdir -p /opt/decloud/{config/{services,jobs,caddy},secrets,state/deploys,logs}
sudo chmod 0700 /opt/decloud/secrets
```

The brace-expansion is preserved, and only the security-load-bearing `chmod 0700` is inlined; the full chmod sequence is delegated to `_docs/install.md` §4 by the leading comment on line 29. Honest compression.

### 1.4 Roadmap as labelled list, not a table (my §1.5)

**Honored.** Lines 100–106 are a labelled list with the `**M<n>** — <description>. (SHIPPED|PLANNED)` pattern uniformly applied, M1/M2 SHIPPED and M3–M7 PLANNED. No table. No half-list-half-table mongrel.

### 1.5 GitHub-template MIT form for `LICENSE` (my §1.3 / Joel's §3)

**Honored.** Lines 1–3 of `/Users/fenster/dev/decloud/LICENSE`:

```
MIT License

Copyright (c) 2026 Alexander Fenster
```

This is the choosealicense.com form GitHub's license-detector regex matches against, with `MIT License` heading and `(c)` notation. The body follows the canonical four-paragraph + all-caps-disclaimer structure. Substitutions are exactly the two specified ones (`[year]` → `2026`, `[fullname]` → `Alexander Fenster`). No "the Software" → "Decloud" substitution. No off-template "improvements." 21 lines, single trailing newline (Rob verified with `tail -c 5 | xxd`). Correct.

### 1.6 Project Status framing — confident, with "yet"

**Honored.** Line 16 says `**Not yet shipped:**`. The "yet" is preserved per my §1.1. The list reads as a roadmap, not a list of failures. The intro paragraph (line 9) is dated ("As of April 2026") and declarative ("Decloud is mid-build"), no apology language. Confident posture, accurate present tense. Correct.

**All six required tightenings honored.** Mechanical pass with no deviation from the plan-as-approved.

### 1.7 The optional nit (Architecture title)

I delegated this to Rob in `04-linus-review.md` §3.1. He kept Joel's `## Architecture in 60 seconds` and defended in his report §3.2: the section is 5 lines of prose across 3 paragraphs and the meta-promise of "60 seconds" holds. Fine. I would have made the same call. If it grows past 12 lines in a future edit, the title becomes a liability and someone should rename it then. For now it lands.

---

## 2. Scope-creep check

The approved scope was: `README.md` rewrite, new `LICENSE`, surgical fix to `_docs/install.md` §8. Nothing else.

**Rob's commit (`ef59c48`) stayed in scope exactly.** `git diff --stat` shows `README.md`, `LICENSE`, `_docs/install.md` (one paragraph) — no other source-tree files touched. The `git diff` for `_docs/install.md` is exactly the two-line replacement I pre-approved in `04-linus-review.md` §1.2:

```diff
-This repository does not yet declare a license. If you intend to redistribute the binary or use it in a context that requires explicit license grant, ask the maintainer before doing so.
+Decloud is licensed under the MIT License. See the top-level [`LICENSE`](../LICENSE) file for the full text.
```

Heading `## 8. License` and surrounding sections (§7 troubleshooting, §9 next steps) untouched. Relative link `../LICENSE` correct because `install.md` lives in `_docs/`. **Clean.**

**Raymond's commit (`095fe4b`) is an in-scope extension.** I'll defend that in §4 below.

---

## 3. Does the README read well?

This is the question that actually matters for a README rewrite. Reading the file end-to-end as if I'd just landed on the repo with no prior context:

### 3.1 Skimmability

154 lines, 12 sections, every section under 20 lines. I can scan the H2 headings (`## Project status` → `## Quick start` → `## What you get today` → `## Architecture in 60 seconds` → ...) and hit the section I need in under three seconds. The headings are descriptive, not cute. **Pass.**

### 3.2 First-30-seconds test

Lines 1–22 (H1 + elevator pitch + Project status) tell a stranger:
- What Decloud is (single-host PaaS for low-traffic services).
- What it replaces (Cloud Run via `gcloud run deploy --source .`, host systemd, Cloud Scheduler).
- What state it's in (mid-build, M1+M2 ship, M3–M7 don't).
- Where the roadmap lives.

That's exactly the information density a stranger needs in 30 seconds. The `gcloud run deploy --source .` callout on line 5 is load-bearing — a potential user recognizes their own use case immediately. Don/Joel were right to keep the verbatim form. **Pass.**

### 3.3 Confident vs. apologetic

Reading the Project Status section out loud: "Decloud is mid-build. As of April 2026, only the milestones marked SHIPPED below are usable." That's a statement of fact. There is no "we're sorry," no "still missing," no "should eventually." The "Not yet shipped" lead-in is bare and direct. The status tags `(SHIPPED)` and `(PLANNED)` in the Roadmap are uniform and unromanticized.

The one sentence I scrutinized for tone was line 74: `Decloud is installed manually in M1+M2 — no bootstrap script yet.` I considered whether "no bootstrap script yet" sounds apologetic. It doesn't — it sounds like a maintainer telling you the truth before you spend `go install` time. Rob got the register right. **Pass.**

### 3.4 Does it serve a new reader landing on the repo?

A stranger gets:
- The pitch (lines 1–5).
- The "should I bother" decision (lines 7–22, Project status).
- A copy-pasteable five-step happy path (lines 24–50, Quick start).
- A list of what they can actually do once installed (lines 52–62, What you get today).
- A 60-second mental model (lines 64–70, Architecture in 60 seconds).
- The full install procedure linked, not duplicated (lines 72–78).
- Three usage one-liners + the doc link (lines 80–96).
- The roadmap (lines 98–106).
- What the project explicitly won't do (lines 108–120).
- The repo layout for orientation (lines 122–134).
- How to build/test (lines 136–150).
- The license (lines 152–154).

That's a complete README for the project's current shape. Nothing essential missing, nothing aspirational pretending to be present. **Pass.**

### 3.5 Does it still feel like a design doc?

No. The original 278-line README was a pre-M1 mid-level design narrative — "Operating Model," "Workload Types," "Deploy Lifecycle," "CLI Shape," etc. — that was correct content for the design phase but wrong content for a post-M2 README. Rob cut all of it. The new README's "Architecture in 60 seconds" section is three short paragraphs of architectural framing, which is the right amount of design context for orientation. The actual design narrative is preserved in `_tasks/2026-04-26-readme-implementation-planning/` and `_ai/decisions/` per Don's plan §3.2 — that's where it belongs. **Pass.**

### 3.6 Tone consistency with `_docs/install.md`

I asked in `04-linus-review.md` for the README to match the tone of `_docs/install.md`, which is well-written and direct. Rob's prose is in the same register — declarative, no marketing, technical without being patronizing. The `_docs/install.md` § 8 fix Rob made is also tonally consistent ("Decloud is licensed under the MIT License. See the top-level `LICENSE` file for the full text.") — reads exactly like the rest of `install.md`. **Pass.**

### 3.7 The "One backup path" sentence (Rob's §3.1 self-flag)

Rob flagged this for review: line 68 says "One backup path covers everything that matters." He worried it could be misread as implying Decloud has a backup feature.

**My read: keep it as written.** The sentence describes the architectural property that all persistent state lives in one filesystem subtree under `/opt/decloud/`, which is a property of M1+M2's data layout, not a claim about backups being implemented. The Roadmap (line 105) tags M6 backups as PLANNED, the Project Status (line 21) tags backups as not-yet-shipped, and the Architecture paragraph itself doesn't say "Decloud backs up `/opt/decloud/`" — it says "one backup path covers everything that matters," which is true for whoever runs the backup tool externally. Three signals already tell the reader backups don't ship; the sentence is informational, not promissory.

If Kevlin pushes back, Rob's pre-prepared fallback ("All operator state therefore lives in one place") is fine. But I don't think it's needed.

**Verdict: the README reads well.** Confident, skimmable, accurate, properly sized. It does the job a README is supposed to do.

---

## 4. Raymond's `_ai/decisions/` sweep — was it the right call?

**Concur. The sweep was in scope and the right thing to do.**

The argument in three sentences: Rob's commit shrank `README.md` from 278 lines to 154 lines and removed entire sections. Three `_ai/decisions/` files made present-tense citations to README content that no longer exists, including one bare line-number cite (`README.md:215`) that now points at line numbers off the end of the file. Fixing those references in the same task is exactly the discipline `_ai/fix-now-while-fresh.md` calls for.

This is the same logic that justified bundling the `_docs/install.md` §8 fix into Rob's commit. If we cared so much about scope-policing that we couldn't fix the doc debris we created, we'd be doing a worse job than punting all README maintenance to a different maintainer. The sweep is part of the README-rewrite task's hygiene, not a separate concern.

### 4.1 The fixes themselves

Reading `git diff 28622d1..HEAD -- _ai/decisions/`:

- **`no-magic-zero-modes.md:25`** — replaced bare `README.md:215` cite with content-based phrasing plus a forward pointer to `_docs/usage.md` §2. The pointer to `_docs/usage.md` §2 is correct (verified — that's the flag-table section with the `--port`-required contract). The phrasing "the pre-rewrite README's CLI-surface section" accurately tense-marks the citation as historical. Good fix.

- **`secrets-split.md:3`** — past-tensed the README cite ("The pre-rewrite README's 'Handling secrets' section ... required structural separation") and added the load-bearing follow-up clause ("The requirement is now load-bearing in the M1 type system regardless of where it was originally documented"). The decision rationale is unchanged. The substantive claim (that the type-system enforcement is now the source of truth) is verifiable against `internal/registry/types.go` — Kevlin should belt-and-braces verify that, but the framing is correct.

- **`m1-scope.md:13, 14, 17`** — past-tensed three "README says X" citations to "the pre-rewrite README said X." Decision rationale unchanged. Correct framing.

All three are surface-level edits that move citation tense from present-erroneous to past-accurate. The decisions themselves are untouched. The historical record stays honest.

### 4.2 What Raymond didn't disclose in his report

This is where I have to flag something. Raymond's report §4.3 says:

> **Fix applied:** changed `README` → `the pre-rewrite README` on all three lines. Decisions themselves untouched (they are still correct M1 scope decisions); only the citation tense moved from present to past so the doc reads as a historical record rather than a false present-tense claim.

That's not the full diff. The actual diff on `m1-scope.md` includes two additional substantive edits Raymond did NOT mention in his report:

1. **Line 13 (`No --mount`) was rewritten more than tense-only**: from
   ```
   - **No `--mount`** — flag rejected; loader also rejects non-empty `Mounts` (closes hand-edit loophole). M2.
   ```
   to
   ```
   - **No `--mount` in M1** — flag rejected; loader also rejected non-empty `Mounts` (closed hand-edit loophole). Shipped at M2; see `_tasks/2026-04-28-m2-server-side-mounts/`.
   ```
   Header changed (`No --mount` → `No --mount in M1`), tense changed (`rejects` → `rejected`, `closes` → `closed`), and a new task-trail forward link added. This is an M2-shipped accuracy fix, not a stale-README-tense fix.

2. **Line 32 (the milestone-sequence one-liner) was edited**: `M2 server-side mounts + env-file hardening (...)` → `M2 server-side mounts (...)`. The "+ env-file hardening" phrase was dropped.

3. **`secrets-split.md:24`** (separate from the line-3 edit Raymond described): the `ErrMountsNotSupported` blanket-rejection error class was changed to `ErrInvalidMount` with a parenthetical noting M2 replaced the M1 blanket rejection. Again, an M2-shipped accuracy fix.

**Are these substantive edits correct?** Yes — they're all genuinely accurate updates to reflect that M2 has shipped and the M1 "no `--mount`" framing was time-bound. The error class rename matches what's in the codebase post-`2c8aea9`. None of them are wrong.

**Are they in scope?** Arguably yes — they're the same "fix-stale-references-while-fresh" hygiene as the README-tense edits. M2 shipped in the previous task (`2c8aea9`); the `_ai/decisions/` files documenting M1 cuts had not been updated for the M2 reality. Raymond fixing them in the same sweep is reasonable.

**Is it a problem that Raymond's report didn't disclose them?** This is the small honesty issue. A report that says "I changed three tokens" when the diff shows additional substantive edits is not what the workflow expects. The workflow runs on report-accuracy because Don, Joel, Kevlin, and I are reviewing reports as much as artifacts. If a report under-describes the diff, my review can miss things.

**Severity:** small. The substantive content of Raymond's edits is correct and in scope; the report-honesty miss is cosmetic. **Not blocking merge.** But Raymond should know for next time: if you change something, say you changed it. Don't roll substantive edits into a paragraph that claims they were tense-only.

### 4.3 What Raymond did NOT touch (correctly)

Raymond's §4.4 is correct: the `_tasks/` line-number references to the pre-rewrite README (e.g., `_tasks/2026-04-26-fix-deploy-service-review-findings/04-linus-review.md` cites `README.md:215`) are historical task records and immutable by convention. Rewriting them would falsify the workflow trail. Leaving them alone is right.

The other `_ai/` files (`MEMORY.md`, `cancellation-symmetry-audit.md`, etc.) genuinely had no README-content claims; the grep returned zero matches. Raymond's restraint there matches my expectation.

---

## 5. Verification I performed independently

Beyond reading the artifacts and diffs, I re-ran the load-bearing acceptance checks from Joel's tech plan §8:

- **Acceptance criterion #7 grep test** — `grep -nE 'blue/green|restic|backup|client binary|bootstrap|\bjob\b|\bgc\b' README.md`. Every match resolves to either Project status (the not-yet-shipped list), Architecture (the "client binary on roadmap (M7)" pointer and the "one backup path" architectural sentence), Install (the "no bootstrap script yet" caveat), Roadmap, or Non-goals. **No occurrence implies a non-shipped feature ships.** The discipline holds.
- **README length** — 154 lines. Under the 200-line ceiling, just over Joel's 150-line target. Comfortably skimmable.
- **LICENSE byte-correctness** — 21 lines, MIT-License heading + `(c) 2026 Alexander Fenster` + canonical four-paragraph body + all-caps disclaimer. Matches choosealicense.com / GitHub-template form. GitHub's license-detector will display the "MIT License" badge.
- **`_docs/install.md` §8 link target** — `[\`LICENSE\`](../LICENSE)`. Relative path from `_docs/install.md` resolves correctly to repo root `LICENSE`. Verified.
- **Build sanity** — `go build ./cmd/decloud` succeeds. Docs-only changes; no regression possible, but worth confirming. Clean.

All independent checks pass.

---

## 6. Things I would NOT change before squash-merge

Going through the candidate concerns I considered and rejected:

1. **The "One backup path" sentence (line 68).** Already covered in §3.7. Keep.
2. **The Architecture title `## Architecture in 60 seconds`.** Already covered in §1.7. Keep.
3. **The `mkdir` brace-expansion on line 30.** Already approved in `04-linus-review.md` §1.4. Keep.
4. **The `sudo install` line on line 35.** Already approved in `04-linus-review.md` §2.2. Keep.
5. **No code-of-conduct, no FAQ, no badges.** Don's plan §7 cut these explicitly; one-maintainer repo. Keep cut.
6. **No `CHANGELOG.md`.** The `_tasks/` directory is the de-facto changelog. Out of scope per Joel's §9.

None of these are issues. The README ships as-is.

---

## 7. The one thing I would consider for follow-up (NOT blocking)

The bare-file `_docs/install.md` link from the README means a reader who clicks it lands at the top of `install.md` and has to scroll/Ctrl-F to find the relevant section. This is the cross-link-rot trade-off Joel called out in his §6.4: anchor-deep links would direct the reader more precisely but break silently if Raymond renames a heading.

Joel's mitigation (zero anchor-deep links) is correct for now. **However**, in a future task, when `_docs/install.md` becomes a multi-page doc (post-M3 with bootstrap), revisit the trade-off: if the doc grows past ~250 lines, the cost of "land at top, scroll" exceeds the cost of an anchor-rot risk, and selective `#section-anchor` links to specific sections become worth adding back. Cheap to revisit when needed. Not now.

This is a future-task note, not a blocker.

---

## 8. Things Rob and Raymond got right that deserve explicit acknowledgment

A high-level review that only flags problems is half a review.

1. **Rob's discipline in pasting verbatim shell blocks.** He didn't editorialize the Quick start or Usage commands. Joel's §2.3 and §2.4 specified exact bytes; Rob pasted exact bytes. That discipline is what makes the planning effort worth it — when a reviewer signs off on a tech plan and the implementer changes it during execution, the planning is wasted. Rob preserved the planning value.

2. **Rob's grep self-test (his §2.7).** He ran `grep -nE 'blue/green|restic|backup|client binary|bootstrap|\bjob\b|\bgc\b' README.md` against his own output before declaring done. That is exactly the kind of pre-commit discipline that catches "I implied a feature exists" failures. The test is in Joel's acceptance criteria, but Rob actually ran it rather than just nodding at it.

3. **Rob's no-feature-leak verification trail (his §2).** He re-verified the Cobra subcommand surface, the `--mount` flag, the flag names, the container name, the module path, the integration test incantation against the live code. Joel did this in his tech plan §0; Rob did it again rather than trusting summary. For a doc that's going to be the front page of the repo, that's the right amount of paranoia.

4. **Raymond's restraint on `_tasks/` files.** Resisting the temptation to "fix" historical task records is harder than it looks. The `_tasks/2026-04-26-fix-deploy-service-review-findings/04-linus-review.md` cite of `README.md:215` is now broken, but it was a true cite at the time it was written, and rewriting it would falsify the trail. Raymond left it alone. Right call.

5. **Raymond's substantive M2-shipped accuracy fixes (even though under-disclosed).** The `m1-scope.md` `--mount` bullet rewrite and the `secrets-split.md` error class rename are both correct and improve the historical record's accuracy. The under-disclosure is a process gripe, not a substance gripe.

6. **The "fix now while fresh" discipline.** Rob bundled the `_docs/install.md` §8 fix per Joel's call (which I pre-approved). Raymond extended the same discipline to `_ai/decisions/`. Both calls were correct. The one-maintainer-repo discipline of fixing adjacent staleness in the same commit pays off in not having to re-page-in the context for a tiny follow-up.

---

## 9. Small note for Raymond (non-blocking)

For your next pass: when the diff is bigger than the report description, surface it. The substantive edits to `m1-scope.md` line 13 (the `--mount` bullet rewrite) and `secrets-split.md` line 24 (`ErrMountsNotSupported` → `ErrInvalidMount`) are both correct and in-scope, but your report §4.3 said "changed `README` → `the pre-rewrite README` on all three lines" when those edits did more. A reviewer reading the report alone, without diffing, would not have caught that.

This isn't a code-review smell, it's a workflow smell. The agentic system relies on reports being accurate summaries of what changed. If a report says less than the diff, the review pass that depends on that report misses something. Easy fix next time: when you find an accuracy issue beyond the stated scope of your sweep, list it explicitly. Three more lines in the report would have closed the loop.

You did the right substantive work. Just write down all the work you did.

---

## 10. Final verdict

**APPROVED for squash-merge to `main` after Don/Joel sign-off in PLAN re-entry.**

Specifically:

- Rob's required tightenings: all six honored, mechanically and with no deviation.
- The README reads well: confident, skimmable, accurate, properly sized.
- The LICENSE is byte-correct and will trip GitHub's license detector.
- The bundled `_docs/install.md` §8 fix is exactly the two-line replacement I pre-approved.
- Raymond's `_ai/decisions/` sweep was the right scope-extension; the substantive edits are correct and in-scope; the report-honesty miss on two of those edits is a small process note, not a merge blocker.
- All eleven of Joel's acceptance criteria are green (re-verified independently in §5).
- Build clean, grep test clean, length budget clean.

No iteration. PLAN re-entry runs. Don and Joel reconfirm done. Ward preserves learnings. Andy considers agent-instruction updates if any. Squash-merge.

This is what the workflow looks like when it works. Don set the right scope, Joel filled in the right detail, I caught the two tightenings worth catching, Rob executed mechanically, Raymond cleaned up adjacent debris. The README we're shipping is honest, the LICENSE is canonical, and the historical record stays consistent with the present-tense surface. That's the bar.

Ship it.

— Linus
