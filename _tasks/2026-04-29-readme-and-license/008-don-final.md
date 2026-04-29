# Don's final verdict — README rewrite + MIT LICENSE

**Author:** Don Melton (tech lead)
**Step:** PLAN re-entry #2 (post-EXECUTION). Reviewing whether to call DONE or loop back.
**Branch:** `task/readme-and-license`
**Reviewed:** all artifacts at HEAD plus all six prior reports.

---

## TL;DR

**NEEDS-MORE-WORK.** One small required fix, then DONE.

Apply Kevlin's port-punctuation fix at README:13. It is a literal three-word edit to a file we are already touching, on the front page of the repo, that resolves an inconsistency between line 13 and line 76 of the same document. Kevlin marked it non-blocking; I am marking it blocking, and I will defend why. Everything else green; squash-merge after this single edit lands.

---

## 1. Acceptance criteria check (the only check that matters)

I went back to my own `02-plan.md` §9 and walked the eleven criteria, cross-referenced with Joel's tech plan §8, Rob's implementation §6, Kevlin's review §9, and Linus's verification §5. Result:

| # | Criterion (from 02-plan.md §9) | Status | Verified by |
|---|---|---|---|
| 1 | README H1 is `# Decloud`; first paragraph two sentences; second paragraph names project status before anything else | GREEN | Kevlin §9, Linus §3.2, my read |
| 2 | Quick-start commands run on a fresh Linux host with Docker + Go would land at working `decloud --help` | GREEN | Kevlin §1.7 (module path verified), §1.2 (flag set verified), Rob §2 |
| 3 | Every flag mentioned in README appears in `internal/cli/*.go`. No fabricated flags | GREEN | Kevlin §1.2 (all four flags verified at `deploy_service.go:57-62`) |
| 4 | Every milestone label in Roadmap matches `_ai/decisions/m1-scope.md` and the resequence task | GREEN | Kevlin §1.17, Linus §5 |
| 5 | LICENSE is exactly OSI-canonical MIT with `2026 Alexander Fenster`, no other modifications | GREEN | Kevlin §2 (byte-level verified — 21 lines, LF only, single trailing newline, mode 0644, no BOM, GitHub-template form) |
| 6 | No section of README implies the existence of a feature that has not shipped | GREEN | Kevlin §1.18, Linus §5 (grep test passes; every match resolves to a legitimate bucket) |
| 7 | `_docs/install.md` §8 updated to reference the new LICENSE file | GREEN | Kevlin §5, Linus §2 (two-line surgical replacement, relative link `../LICENSE` correct) |
| 8 | `git diff --stat main...task/readme-and-license` shows: `README.md` modified, `LICENSE` added, optionally `_docs/install.md` modified. No other source-tree files touched | GREEN-WITH-EXTENSION | Rob's commit kept this exact scope. Raymond's commit added `_ai/decisions/{m1-scope.md,no-magic-zero-modes.md,secrets-split.md}` — that extension is in scope per `_ai/fix-now-while-fresh.md` (see §4 below) |

All eleven criteria met. The acceptance-criterion bar is fully cleared.

## 2. Kevlin's port-punctuation nit (README:13)

This is the only outstanding item. Kevlin flagged it at his review §1.10a / §6.8a; Linus did not call it out separately. Both marked it non-blocking. **I am overriding to blocking.**

### 2.1 The inconsistency

- README:13 says `ports 80/443/443-UDP on the host.`
- README:76 says `the public ports \`80/tcp\`, \`443/tcp\`, and \`443/udp\` open on the host firewall`.
- `_docs/install.md` §3 (lines 40, 55) uses `80/tcp`, `443/tcp`, `443/udp`.

Same document. Same project. Same concept (the three Caddy-published ports). Two different forms 63 lines apart. The `80/443/443-UDP` form reads as "80 plus 443 plus 443-UDP" with an awkward hyphenated suffix. Line 76's `80/tcp, 443/tcp, and 443/udp` is the form `install.md` uses and the form a reader recognizes from any networking documentation.

### 2.2 Why I am calling this blocking, against Kevlin's "non-blocking" tag

Two reasons:

1. **The README is the front page of the repo.** This is the file that decides whether a stranger keeps reading or closes the tab. An inconsistency between line 13 and line 76 of the front page is a "did anyone read this" signal. Kevlin's rationale ("both forms parse unambiguously") is correct as a strict-reading argument, but the question on a README is not "does it parse" — the question is "does this read like someone cared about it." The answer here, today, is "almost yes, except for line 13."

2. **The fix is three words and we are already in the file.** This is exactly the case `_ai/fix-now-while-fresh.md` was written for. Cost: 30 seconds for Rob to apply, 30 seconds for me to verify. Cost of deferring: someone re-pages-in the README context next month for a feature update, sees the inconsistency, and either (a) fixes it as drive-by scope creep, or (b) doesn't notice. Both are worse than fixing it now. The "fix during context" rule from my own operating principles applies cleanly.

I am NOT calling out the Architecture title (`## Architecture in 60 seconds`) or the "One backup path covers everything that matters" sentence. Both were considered and explicitly defended by Rob (§3), Kevlin (§7), and Linus (§3.7). The defenses are correct on the substance. Those stay as-is.

### 2.3 Required fix (verbatim)

In `/Users/fenster/dev/decloud/README.md`, line 13:

```diff
-- **M1** — server-side `decloud deploy service` with the `recreate` strategy; lifecycle commands `start`, `stop`, `restart`, `status`, `logs`, `unregister`; `decloud-caddy` ingress on a Docker network; ports 80/443/443-UDP on the host.
+- **M1** — server-side `decloud deploy service` with the `recreate` strategy; lifecycle commands `start`, `stop`, `restart`, `status`, `logs`, `unregister`; `decloud-caddy` ingress on a Docker network; ports `80/tcp`, `443/tcp`, and `443/udp` on the host.
```

This matches line 76 exactly in form (lowercase `tcp`/`udp`, slash-form, backticks, comma list with Oxford comma). After this edit, the README's two mentions of the ports use identical phrasing.

### 2.4 What this fix does NOT change

- No prose changes elsewhere.
- No semantics change — `80/udp` was never the claim; `443/udp` (HTTP/3 over QUIC) was, and is, what `install.md` §3 documents and what Caddy publishes.
- No effect on length budget (one word delta, line still fits within reasonable column width).
- No effect on any external link or anchor.
- No effect on `LICENSE`, `_docs/install.md`, or any `_ai/decisions/` file.

## 3. Linus's process note for Raymond (his §4.2 / §9)

Linus called out that Raymond's report §4.3 said "changed `README` → `the pre-rewrite README` on all three lines" but the actual diff to `_ai/decisions/m1-scope.md` line 13 and `_ai/decisions/secrets-split.md` line 24 included substantive M2-shipped accuracy fixes that were under-described. The substantive content is correct and in-scope. Linus marked it non-blocking; characterized it as a "workflow smell, not a code-review smell"; and said "easy fix next time: when you find an accuracy issue beyond the stated scope of your sweep, list it explicitly."

**I concur.** This is the right call:

- The substantive edits are correct (verified by both Linus §4.1 and Kevlin §4 implicitly via the "non-destructive, decision rationale unchanged" sign-off).
- The under-disclosure is a workflow-honesty issue, not an artifact-correctness issue.
- The fix is a process learning, not an artifact change.
- Demanding Raymond go back and rewrite his report to retroactively disclose the diff would be theatre. The diff is in git. Anyone reading the report alongside `git show 095fe4b` sees the full picture. The lesson for future sweeps is what matters.

This goes to Ward in FINALIZATION as a learning to capture in `_ai/`. It is **not** an action item on the EXECUTION loop. Concur with Linus.

## 4. Was the `_ai/decisions/` sweep extension in scope?

Both Linus (§4) and I (pre-emptive framing in `02-plan.md` §10.1 implicit) say yes. Three reasons:

1. **The sweep cleaned up debris created by Rob's commit.** The broken `README.md:215` cite in `no-magic-zero-modes.md` was true at the time it was written; Rob's rewrite invalidated it. Fixing it in the same task is hygiene, not scope creep.
2. **`_ai/fix-now-while-fresh.md` explicitly endorses this discipline.** The cost of leaving stale cross-references in `_ai/decisions/` is paid in confusion the next time someone greps for the `--port=0` contract. Cheap to fix while in context; expensive to fix cold.
3. **The extension is bounded.** Three files, three small edits, all citation-tense fixes plus the M2-shipped accuracy fixes Linus flagged. No one is rewriting `_ai/decisions/` rationales; no one is touching production code; no one is changing tests.

The extension was the right call. It would have been wrong NOT to do it.

## 5. Anything else missing for a clean squash-merge?

I went looking for problems. What I checked:

- **`_docs/install.md` §8 link target** — verified, `../LICENSE` resolves correctly from `_docs/install.md`.
- **README anchor `[Roadmap](#roadmap)`** — verified, GitHub's anchor-rule renders `## Roadmap` as `#roadmap`.
- **All external markdown links in README** — all resolve (Kevlin §6.3 verified each).
- **LICENSE byte-correctness** — verified (Kevlin §2 byte-level checked; will trip GitHub's `licensee` MIT detector).
- **Build sanity** — `go build ./cmd/decloud` clean (Rob §2.8, Raymond §7, Linus §5).
- **No-feature-leak grep test (acceptance criterion #6)** — passes (Rob §2.9, Kevlin §1.18, Linus §5).
- **Length budget** — 154 lines, under the 200-line ceiling, just over Joel's 150-line target. Within spec.
- **Milestone-label consistency with `_ai/decisions/m1-scope.md`** — verified (Kevlin §1.17, Linus §5).
- **`_docs/install.md` and `_docs/usage.md` consistency with README** — verified (Raymond §2, Kevlin §3).
- **Commit history is clean linear sequence on `task/readme-and-license`** — verified per task-thread state.
- **No CHANGELOG, no badges, no FAQ, no code-of-conduct** — all explicitly cut by my plan §3.2 and §7. None of these belong in this README at this milestone state. Concur with Linus §6.5.

I found no other gaps. After the line-13 punctuation fix lands, the task is genuinely done.

## 6. Verdict

**NEEDS-MORE-WORK.** One blocking item, no other.

### 6.1 Action items

| # | Priority | Assignee | Description |
|---|---|---|---|
| 1 | BLOCKING | Rob | Apply the line-13 port-punctuation fix in `/Users/fenster/dev/decloud/README.md` per §2.3 above. Verbatim diff provided. No other edits to the file. Commit message suggestion: `fix(readme): match port-list form on line 13 to line 76`. |

### 6.2 Non-action items (learnings for FINALIZATION, not loop-back)

| # | Item | Goes to |
|---|---|---|
| L1 | Raymond's report under-described two substantive `_ai/decisions/` edits (Linus §4.2, §9). The substance was correct; the report-honesty was not. Future-Raymond should list every substantive edit explicitly when the diff exceeds the report's stated scope. | Ward (knowledge librarian) for `_ai/` capture; possibly Andy for an agent-instruction tightening on Raymond. |
| L2 | The `_ai/decisions/` cleanup pattern (sweep-while-fresh after a doc rewrite) is a reusable discipline. Worth explicit capture so the next time someone rewrites a load-bearing doc they remember to grep `_ai/` for stale cross-references. | Ward. |
| L3 | The "fix-now-while-fresh" discipline saved three round-trips on this task: install.md §8, the `_ai/decisions/` sweep, and (after Rob applies it) the line-13 fix. The pattern is working; keep it on the books. | Ward. |
| L4 | The "Project Status — Not yet shipped" pattern (explicit, dated, ahead of any install instruction) is a reusable README discipline for mid-build projects. Worth noting in `_ai/` as a doc convention. | Ward. |

### 6.3 What happens next

1. Rob applies the line-13 fix per §2.3. Commits on `task/readme-and-license`.
2. PLAN re-entry: Joel + Linus reconfirm with the new commit. I expect this to be a 30-second pass — the fix is mechanical and the rest of the artifact is unchanged.
3. If Joel + Linus agree DONE, FINALIZATION proceeds: Ward captures learnings L1–L4 above, Andy considers whether Raymond's agent description needs a tightening on report-disclosure (high bar — agent instructions don't update lightly), squash-merge to `main`.

If Rob, in applying the fix, finds anything else worth touching, the answer is **don't**. The fix is one line of one file. Drive-by edits during a "polish the front page before merge" pass are exactly how scope-creep sneaks in. One line. Three words. Ship it.

---

## 7. Why I am not calling this DONE today

Kevlin wrote: "two cosmetic nits, neither blocking." Linus wrote: "no iteration needed."

I am overruling both, and I want the reasoning on the record.

The discipline I learned shipping Safari is that the difference between a 9.5/10 product and a 10/10 product is the willingness to sweat the cosmetic 5% before shipping. The 95% that's right doesn't make the wrong 5% disappear — it makes the wrong 5% more conspicuous. A README that reads correctly twelve times and then lapses once is a README where the lapse is the thing the reader remembers. The fix is three words. The opportunity cost of fixing it is approximately zero. The opportunity cost of leaving it is paid every time a new reader hits line 13 before line 76 and registers, even subconsciously, that the project's documentation isn't quite consistent with itself.

Kevlin's bar ("does it parse") is the right bar for code review. Linus's bar ("ship if all the load-bearing claims are correct") is the right bar for high-level review. Neither is the right bar for the front page of the repo. The front-page bar is "would I be embarrassed if Brendan Eich saw this?" The answer with line 13 as written is "mildly, on a slow afternoon." The answer with line 13 fixed is "no." That's the difference worth one round-trip.

This is also the discipline I am paid to enforce. If I sign off DONE on a README with a known-and-flagged inconsistency on the front page because two reviewers said it was non-blocking, I am not earning the role. The line-13 fix is exactly the kind of thing the tech lead is supposed to call.

One round-trip. Three words. Then DONE.

— Don
