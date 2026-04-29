# Linus review — README rewrite + MIT LICENSE plans

**Reviewer:** Linus Torvalds (high-level)
**Branch:** `task/readme-and-license`
**Reviewed:** `02-plan.md` (commit 235e624) and `03-tech-plan.md` (commit 67b292f).
**Verdict:** **APPROVED with two small required tightenings and one optional nit.** No iteration required. Rob can proceed to EXECUTION.

---

## TL;DR

Don's plan is correct in scope, correct in tone-direction, and correct on the bundled `install.md` §8 fix. Joel's tech plan is the kind of IKEA-diagram detail I rarely see done well: every claim re-verified against the code, every shell line traceable to its source, every section of the README has a length budget and a "must-not" list. The two of you spent the planning budget exactly where it needed to be spent.

I have:
- **Two required tightenings** — small, mechanical, no re-plan needed.
- **One optional nit** — Rob's call.
- **No fundamental disagreement** with any of Don/Joel's load-bearing decisions.

I am explicitly approving so we can move to EXECUTION. Kent skip is justified (precedent + this task's shape). See §5.

---

## 1. Direct answers on the five focal points Joel surfaced

### 1.1 Project Status tone — confident vs. apologetic

**Joel's draft (§1.2) is correctly proportioned.** The framing "Decloud is mid-build. As of April 2026, only the milestones marked SHIPPED below are usable" is the right register: declarative, dated, no apologies, no marketing. The "What ships today" / "Not yet shipped" split tells the reader what they need before they spend `go install` time.

The "yet" in "Not yet shipped" stays. Joel was right to flag it as a candidate for cutting; he was also right to default to keeping it. "Not shipped" without "yet" reads like "will never ship" and that is a worse signal than mild apology. Confidence is not the absence of accurate present tense — it's the absence of self-flagellation. Joel's draft has zero self-flagellation; it just states what is and what isn't. Ship it.

The only thing I would tighten — and this is one of my two required tightenings — is the Project Status sub-list ordering. Joel has it: M1 SHIPPED, M2 SHIPPED, then M3 → M7 PLANNED. That's correct. But the lead-in for the planned list reads `**Not yet shipped — see [Roadmap](#roadmap):**`. Drop the colon and the link from the lead-in: it duplicates the Roadmap section's job. Make it `**Not yet shipped:**` and let the reader scroll. The Roadmap link belongs in the one-sentence intro paragraph, not on every sub-list label. Minor, mechanical, do it before commit.

**Verdict: keep §1.2 as drafted. One label-line trim.**

### 1.2 Bundling the `_docs/install.md` §8 fix

**Bundle. Joel's defense in §4 is exactly right.**

The four points Joel makes (same theme, trivial cost, real reader confusion, Don's acceptance criterion #7 already lists it) are independently sufficient. The only counter-argument worth taking seriously is "scope creep," and Joel pre-empted it correctly: a doc inconsistency you create in a task is part of that task's diff hygiene, not a separate concern.

This is a textbook application of `_ai/fix-now-while-fresh.md`. All four conditions of the decision rule are met:
1. Mechanical fix — replace two lines with two lines.
2. Same theme — the task IS the licensing story.
3. Below the 5-minute floor — three minutes including the commit message.
4. Punting costs more than fixing — a follow-up task has full workflow overhead for a two-line edit.

If Don had punted this, I would have called it cowardice in this review. Joel calling it correctly without my prompt is the system working as intended.

**Verdict: bundle. No discussion needed.**

### 1.3 GitHub-template MIT form vs. bare OSI form

**Joel chose correctly. Use the choosealicense.com / GitHub-template form.**

Three reasons:
1. **GitHub's license-detector regex matches the choosealicense form.** That's what produces the "MIT License" badge in the repo header. The bare OSI form (no `MIT License` heading, `Copyright [year] [fullname]` without the `(c)`) does not always trigger the detector cleanly. For a project that wants `gh repo view` to display the license, this is load-bearing.
2. **Reader recognition.** Every Go repo, every npm package, every Rust crate uses the choosealicense form. It is the canonical surface for MIT in 2026. The OSI text is the legal source-of-truth, but the choosealicense form is byte-equivalent in the body and adds only the heading + `(c)` notation.
3. **Linter compatibility.** SPDX scanners and `licensee` (the gem GitHub uses) expect the choosealicense form.

Joel's exact-bytes block in §3 is correct: `MIT License` heading, blank line, `Copyright (c) 2026 Alexander Fenster`, blank line, the standard four paragraphs ending with the all-caps disclaimer. LF line endings, single trailing newline, no BOM, mode 0644. This is the right specification down to the byte. Rob: paste it verbatim, do not re-type it, do not "improve" the wording.

**Verdict: choosealicense form, exact bytes per Joel's §3.**

### 1.4 `mkdir -p {...}` brace-expansion compression

**Compress. Joel's compression is the right call.**

The README is the happy path. The full seven-line `chmod` sequence in `_docs/install.md` §4 belongs in the full install doc, not in the README. The compressed `mkdir -p /opt/decloud/{config/{services,jobs,caddy},secrets,state/deploys,logs}` plus the one load-bearing `chmod 0700 /opt/decloud/secrets` is exactly the right level of detail for a Quick start.

Joel correctly identifies the trade-off:
- **Pro:** README stays scannable, ~14 shell lines instead of ~17.
- **Con:** brace expansion requires bash 4+ or zsh; `dash` users would need the explicit form.

The con is theoretical. The README's Quick start is a copy-paste-into-an-interactive-shell scenario. The login shell on Ubuntu/Debian is bash, and bash supports brace expansion since the early 1990s. Anyone running `decloud` setup in `dash` is by definition an unusual operator and they'll figure it out from `_docs/install.md` §4.

The `# 1. Create the Decloud state tree (full chmod sequence in _docs/install.md §4).` comment Joel has on line 1 of the shell block is exactly right — it tells the reader where the rest of the chmod story lives without spelling it out in the Quick start.

**Verdict: compress per Joel's §2.3. No change needed.**

### 1.5 Roadmap as labelled list vs. table

**Labelled list. Joel and Don already converged; I confirm.**

Three reasons:
1. **Narrow viewport survival.** Markdown tables collapse miserably on phones and in narrow GitHub PR diffs. Labelled lists wrap clean.
2. **Status-tag uniformity.** Every line is `**M<n>** — <one sentence>. (SHIPPED|PLANNED)`. That uniformity is easier to read in a list than in a table where the eye has to move column-to-column.
3. **No fake structure.** A table promises columnar data that justifies the columns. Roadmap has three columns (label, description, status) where the description is a one-sentence prose blob. That's a list pretending to be a table.

Joel's §1.8 rendering is right. Rob: paste it. Do not turn it into a table "because it would look more professional." Professionalism here is fitness-for-purpose, not visual structure for its own sake.

**Verdict: labelled list, no table.**

---

## 2. Required tightenings (two items, mechanical)

### 2.1 Project Status sub-list lead-in (already noted in §1.1 above)

`**Not yet shipped — see [Roadmap](#roadmap):**` → `**Not yet shipped:**`. The intro paragraph already links to Roadmap. The lead-in label doesn't need a duplicate link.

**Why this matters:** redundant inline links train the reader to skip them. The intro-paragraph link should be the one that lands.

**Cost:** one-line edit during Rob's pass. No re-plan.

### 2.2 The `sudo install` line in Quick start §2.3

Joel flagged this himself in §2.3 ("One judgment call I'm flagging for Linus") but didn't actually resolve it. I'm resolving it: **the line is fine as drafted.**

```sh
sudo install -m 0755 "$(go env GOPATH)/bin/decloud" /usr/local/bin/decloud
```

The `sudo` on the install step (not on `go install`) is correct. `go install` runs as the operator, lands the binary in their `$GOPATH/bin`, then `sudo install` copies it to `/usr/local/bin` with root ownership. This is the standard Go binary installation pattern and matches `_docs/install.md` §5 verbatim.

The corner case Joel worried about — "what if the operator runs `go install` as root" — is not a concern for the README. The README assumes the happy path: a regular operator with their own `$GOPATH`. Operators running `go install` as root are unusual enough to find the answer in `_docs/install.md`.

**Action: Joel: please add a one-line sentence to your tech plan §2.3 confirming this line stays as-is. Rob: ignore Joel's "Linus, weigh in if you disagree" parenthetical — I just did.** No code change to the shell block.

Wait — I just re-read Joel's §2.3 and the resolution is that I'm telling him I agree with his draft. The shell block doesn't change. The only deliverable here is that Joel's tech plan no longer has an open Linus question; my approval here resolves it.

**Cost:** zero edits to the shell block. Joel's open question §6.2 (the `--readiness-path` one) is also resolved here: **omit `--readiness-path`. Default `/healthz` is fine. Joel chose Option A; that's right.** A teachable failure on a service without `/healthz` beats a confusing mandatory flag in the minimum-working example.

---

## 3. Optional nit (Rob's call, not blocking)

### 3.1 The "Architecture in 60 seconds" section title

Joel kept Don's section title `## Architecture in 60 seconds` verbatim. It's fine. It's also the kind of clever title that ages poorly when the architecture takes 80 seconds to read because the prose grew. I would consider `## Architecture` plain or `## How it works` — either drops the meta-promise about reading time and lets the section's actual length speak for itself.

But I'm not going to mandate the change. "60 seconds" is a recognizable convention (AWS docs use the form, GitHub README templates use the form), and the section budget is tight enough that the promise will hold. Rob: pick whichever you prefer when you write the file. If you keep "60 seconds," police the budget at 12 lines hard.

**Cost:** zero edits required. Rob's discretion.

---

## 4. Things Don and Joel got right that deserve explicit acknowledgment

A high-level review that only flags problems is half a review. Here's what was load-bearing and correct:

1. **The "what was cut from the old README" decision (Don §3.2, Joel honored).** Cutting the full Operating Model, Workload Types, Configuration, Routing, Container Lifecycle, Deploy Lifecycle, Image Housekeeping, Backup, and CLI Shape sections is the right call. Those are design-doc material, the implemented subset is in `_docs/install.md` and `_docs/usage.md`, and the unimplemented subset has no business in the README. The original README is 279 lines of mostly-stale design narrative; cutting to ~150 lines of accurate present-tense is the right ratio. Don was right not to create a new design-doc home for the cut content — `_tasks/2026-04-26-readme-implementation-planning/` and `_ai/decisions/` already preserve it.

2. **The "no FAQ, no badges, no logo, no comparison matrix, no code-of-conduct" cuts (Don §7).** All correct. One-maintainer repo, no users yet beyond the maintainer. Premature to add any of those. The discipline of saying "we will add this when there's a reason" is the right discipline.

3. **The verification trail in Joel §0.** Joel re-verified Don's load-bearing factual claims against the code rather than trusting summary. That's the right behavior for a tech plan whose downstream reader is going to copy-paste shell commands. The two factual adjustments Joel made (the recreate-strategy "build before stop" sequencing, the `M1.0 host-Caddy` artifact) are both correct.

4. **The acceptance criteria, especially Joel's #7.** "Search the rendered README for the strings `blue/green`, `restic`, `backup`, `gc`, `client binary`, `bootstrap`, `job` — every occurrence must be in either the Project Status 'not yet shipped' list, the Roadmap, or a 'see [Roadmap]' pointer." This is exactly the kind of grep-test that catches the "README implies a feature exists that doesn't ship" failure mode. The original README fails this test in approximately twenty places. The new one had better not.

5. **Choosing MIT over Apache-2.0.** User asked for MIT. Don and Joel both honored that. The "patent grant might be useful" argument from `_tasks/2026-04-26-readme-implementation-planning/05-plan-v2.md` §10 was correctly overridden by user intent. Re-licensing MIT-to-Apache-2.0 is a one-commit change if the maintainer ever has a real patent-grant need; meanwhile MIT is what every reader expects for a small Go tool.

6. **Joel's bloat-risk callouts (§2.1 and §5).** Pre-identifying the three sections most likely to balloon (Project status, Architecture, Quick start) and giving Rob explicit "if X grows, cut Y" instructions is the right level of paranoia. The README has gone from 279 lines to 150 lines; if Rob lets it grow to 200 because "everything I added is useful," the discipline is lost. Joel's section budgets defend against that.

7. **Length budget honesty.** Joel's §2.1 budget table sums to ~150 lines. I checked: 6+14+18+11+12+9+14+9+12+12+12+3+20 = 152. The arithmetic is right. The 200-line ceiling is the right discipline; the 150-line target is the right comfort zone. This is one of the few length budgets I've seen that's neither padded nor wishful. Honest.

8. **The decision to put zero anchor-deep links into `_docs/`.** Joel's §6.4 mitigation. Cross-link rot is a real failure mode and the cheapest defense is "don't link to anchors, link to files." Bare-file links survive Raymond renaming a heading. This is exactly the kind of preemptive defense that planning is for.

---

## 5. Kent skip — justified for this task

Don's plan didn't explicitly invoke the Kent-skip but Joel's §10 mentions the precedent at `_tasks/2026-04-28-milestone-resequence/002-don-plan.md` "Workflow: skip Kent and Rob, justified."

**My view: Kent should be skipped here too. Same logic, more clearly applicable.**

Kent's role presupposes a code change to test. The deliverables in this task are:
1. `README.md` — markdown prose. No runtime artifact, no test surface.
2. `LICENSE` — MIT text. No runtime artifact, no test surface. (One could imagine a "license-detection regex matches" test, but that test is GitHub's job, not ours, and it isn't a test of our code.)
3. `_docs/install.md` §8 — three lines of markdown.

Adding a test that asserts `README.md contains the string "Decloud"` or `LICENSE matches the SHA256 of the canonical MIT text` is the textbook definition of a change-detector test, which `CLAUDE.md` §1.4 explicitly forbids.

The post-execution PLAN re-entry (Don/Joel/Linus reconfirm done) still happens. Kevlin still reviews Raymond's diff for accuracy/hallucinations. Workflow shape is preserved; we are skipping one role whose preconditions are not met.

**Action: in the EXECUTION step, Rob proceeds without a prior Kent step. Raymond verifies the README's accuracy against `_docs/install.md` and `_docs/usage.md` post-Rob. Kevlin reviews for hallucinations.**

If during Rob's pass we discover that a doc change requires a code change (e.g., a test that exercises the LICENSE file's existence — unlikely but flag), Kent gets re-added. Pre-vote in writing now per the resequence-task pattern.

---

## 6. Open questions Joel surfaced (not focal points): my resolution

Joel listed eight items in §6 of the tech plan. I'm closing them in order:

- **§6.1 Project Status tone** — addressed in my §1.1. Confident framing stays. "Yet" stays. Order stays.
- **§6.2 `--readiness-path` in Quick start** — addressed in my §2.2. Omit. Joel chose Option A; that's right.
- **§6.3 Milestone label format** — addressed in my §1.5. Labelled list. No table.
- **§6.4 Cross-link rot** — Joel's mitigation (zero anchor-deep links into `_docs/`) is correct. Apply it.
- **§6.5 `mkdir` portability** — addressed in my §1.4. Compress. Bash assumed.
- **§6.6 `go install ...@latest` requires public repo** — Joel's mitigation (proceed assuming public, add `git clone` fallback if needed) is fine. The verification check (`gh repo view alexander-fenster/decloud --json visibility`) is one network call and Don can run it before merge if there's any doubt. Don't gate the plan on this.
- **§6.7 MIT license year (2026)** — already resolved. Don §8, Joel §3.
- **§6.8 Apache-2.0 vs MIT** — already resolved. User asked, user gets.

All eight closed. Rob proceeds with no open architectural questions.

---

## 7. What Rob actually does in EXECUTION

Joel's §10 "sign-off note for the next agent in the chain" already specifies this. I'm endorsing it verbatim:

1. **Open `/Users/fenster/dev/decloud/README.md`** and replace its entire contents with the structure in Joel's §1.1-§1.12, using the verbatim shell blocks from §2.3-§2.5 and the prose-structure budgets from §1.x. Apply my Required Tightening §2.1 (drop the redundant Roadmap link in the Project Status sub-list lead-in).
2. **Create `/Users/fenster/dev/decloud/LICENSE`** with the exact bytes in Joel's §3. Single trailing newline. LF only. Mode 0644.
3. **Edit `/Users/fenster/dev/decloud/_docs/install.md`** lines 212-214 with the replacement from Joel's §4.1.
4. **`git add` only those three files.** Verify with `git diff --stat main...HEAD`. Commit with a message that names all three changes.

Then Raymond reviews for `_docs/` accuracy. Then Kevlin and I review in parallel. Then PLAN re-entry (Don/Joel/Linus reconfirm). Then Ward, Andy, squash-merge.

---

## 8. The final verdict, stated unambiguously

**APPROVED.** Don's plan and Joel's tech plan together are sufficient for Rob to execute mechanically. The two required tightenings are mechanical edits Rob can apply during his own pass without re-planning. The optional nit is Rob's discretion. The Kent skip is justified.

**Move to EXECUTION.** Rob writes the three files. Raymond verifies. Kevlin and I review in parallel. PLAN re-entry follows.

This is the kind of planning effort that earns the "no iteration needed" verdict. Don set the right scope, Joel filled in the right detail, and both of you flagged your own uncertainties for me to close rather than papering over them. The system worked.

Now let's stop talking about a README and ship one.

— Linus
