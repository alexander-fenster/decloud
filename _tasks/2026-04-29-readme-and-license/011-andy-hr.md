# Andy HR review — agent-instruction update consideration

**Reviewer:** Andy Grove (HR / agent architect)
**Step:** FINALIZATION (post-Ward, pre-squash-merge)
**Branch:** `task/readme-and-license`
**Inputs read:** `01-user-request.md`, `005-rob-implementation.md`, `006-raymond-docs.md`, `007-linus-execution-review.md` (specifically §4.2 and §9), `010-ward-finalization.md`, `/Users/fenster/.claude/agents/raymond.md`.

---

## Verdict

**UPDATE-RAYMOND.** One surgical addition to `/Users/fenster/.claude/agents/raymond.md` covering report–diff fidelity. No other agent definitions changed. No user-correction-driven updates (none warranted; see §1).

---

## 1. User-correction track — NO UPDATE

I re-read the chain end-to-end. The user's only message in this task is the four-line initial request in `01-user-request.md`. There is no mid-task pushback, no redirection, no clarification, no correction. By the very-high-bar rule in CLAUDE.md ("agent definitions should not be updated lightly"), there is no user-correction signal that justifies any agent-definition change.

This branch is closed. No update on user-correction grounds.

---

## 2. Raymond report-honesty track — UPDATE-RAYMOND

### 2.1 The signal

Linus §4.2 and §9 of `007-linus-execution-review.md` document that Raymond's report `006-raymond-docs.md` §4.3 said:

> "changed `README` → `the pre-rewrite README` on all three lines. Decisions themselves untouched (they are still correct M1 scope decisions); only the citation tense moved..."

The actual diff on the same files contains three additional substantive edits Raymond did NOT mention:

1. `_ai/decisions/m1-scope.md` line 13 — the `No --mount` bullet was rewritten beyond tense: header changed (`No --mount` → `No --mount in M1`), tense flipped, and a new forward link to `_tasks/2026-04-28-m2-server-side-mounts/` was added. This is an M2-shipped accuracy fix, not a tense-only fix.
2. `_ai/decisions/m1-scope.md` line 32 — `M2 server-side mounts + env-file hardening` → `M2 server-side mounts`. Phrase removed.
3. `_ai/decisions/secrets-split.md` line 24 — `ErrMountsNotSupported` renamed to `ErrInvalidMount` with a parenthetical noting M2 superseded the M1 blanket rejection.

Linus's own characterization: substance correct, in scope, all three edits genuinely accurate; report under-described what changed. He called it "small," "cosmetic," "non-blocking" — but also a "workflow smell" because the agentic system "runs on report-accuracy" and reviewers reading the report alone, without diffing, would have missed the unstated edits.

### 2.2 Root cause analysis

Before deciding whether to patch, I asked: WHY did Raymond report this way?

Reading `/Users/fenster/.claude/agents/raymond.md` carefully, the agent definition is dominated by content-accuracy guards — anti-hallucination on field names, JSON tag verification, "verify every example would actually parse with the real API." These rightly emphasize getting the artifact correct. But the definition's **AT END** section says only:

> "Call `mcp__bureau__start_new_report_file` with suffix like `apidocs` or `apidocs-fix`. Write documentation update summary to that file."

That is the entirety of the report-writing instruction. There is **no requirement** that the report match the diff. There is no instruction to enumerate every edit. There is no warning that the report is itself a workflow artifact that downstream reviewers consume.

So the root cause is not "Raymond was sloppy" — it is "Raymond's definition treats the report as a summary, not as an audit trail." When Raymond grouped four edits under a single mental category ("stale-README-tense fixes in `_ai/decisions/`"), the definition gave him no instruction to break that summary down into per-line accountability. He wrote the summary the definition asked for. The definition asked the wrong thing.

This is a structural gap, not a behavior slip. That distinction matters for the very-high-bar test.

### 2.3 Pattern vs. one-off

A one-off would be: Raymond made one ambiguous judgment call this task. A pattern would be: the definition lacks a guard that this kind of failure can recur on every future Raymond pass. This is the second case. The gap is in the definition itself, not in this run's execution. Every future Raymond invocation that touches multiple files will face the same temptation to summarize at category level rather than per-edit, because the definition tells him to "write a documentation update summary," not "write a per-edit changelog of the diff."

Adding one paragraph closes the gap. Not adding it leaves it open for every future Raymond invocation.

### 2.4 The very-high-bar test

CLAUDE.md says agent definitions should not be updated lightly. I considered three reasons NOT to update:

- **(a) Linus already caught it.** Yes — but the workflow's reliance on Linus catching every under-described report is itself a fragility. Linus's review §9 explicitly says he wrote down the gripe BECAUSE the system depends on report-accuracy. He is asking, structurally, for the report to match the diff — that is the inverse of "I caught it, so the system worked."
- **(b) The substance was correct.** Yes — but the workflow requires both substance correctness AND report fidelity. Substance correctness alone makes the next reviewer's job harder, not easier.
- **(c) First occurrence; no demonstrated pattern yet.** True. But the gap is structural and the fix is surgical (one paragraph). The very-high-bar test does not require a pattern of failures — it requires that the change be principled rather than reactive. Filling a structural gap with a surgical instruction is principled.

I considered one reason TO update beyond the above:

- **(d) The instruction generalizes the existing accuracy discipline.** Raymond's definition already screams about FIELD-NAME VERIFICATION as the load-bearing accuracy concern. Extending that same discipline from "examples in the artifact" to "edits described in the report" is not a new principle — it is the existing principle applied to the report itself. The agent already understands "verify everything you claim." The patch tells him: that includes claims about your own diff.

Verdict: principled surgical update warranted.

### 2.5 The change

File: `/Users/fenster/.claude/agents/raymond.md`

Insert one new instruction block in the **Bureau MCP Workflow / AT END** section — the place that currently says "Write documentation update summary to that file." That is the exact location where the instruction is missing.

Specific change (full text added below in §3): a new line item under AT END requiring report–diff fidelity, plus one item appended to the Quality Checklist.

The substantive principle: a report must enumerate every non-trivial edit in the diff, not summarize edits at a single mental category. If a sweep finds something beyond its stated scope, the report explicitly lists it.

This does not break any existing successful behavior. It does not contradict the field-name verification discipline (it extends it). It does not affect the artifact-quality guards. It does not change Raymond's domain (still _docs/ first-class, _ai/ secondary). It strictly adds a missing guard.

---

## 3. Specific edit applied

`/Users/fenster/.claude/agents/raymond.md`

**Change 1** — append two bullets to the **AT END** subsection of the Bureau MCP Workflow:

```
3. Before submitting your report, run `git diff` against the previous commit and confirm that every non-trivial edit in the diff appears in your report. The report is not a summary — it is the audit trail downstream reviewers consume in lieu of re-reading the diff. If a sweep finds something beyond its stated scope (a substantive edit you bundled in, an unexpected fix, an in-scope-by-extension change), list it explicitly. Never roll substantive edits into a paragraph that describes only the simplest change.
4. Apply the same accuracy discipline to your report that you apply to your artifacts: if a field-name claim must be verified before it ships, a diff-content claim must be verified too.
```

**Change 2** — append one line to the **Quality Checklist**:

```
- **REPORT–DIFF FIDELITY: Does every non-trivial edit in `git diff` appear in the report? Does the report describe what the diff actually contains, not just the cleanest mental summary?**
```

Rationale appears in §2.2–§2.4 above. Both changes are surgical. Neither contradicts existing instruction.

---

## 4. Knowledge-base recommendations

**RECOMMENDATION for knowledge-librarian (Ward):** add a one-line entry to the project-level operational lessons section of `_ai/` (Ward's discretion which file — likely `_ai/MEMORY.md` or a new `_ai/agentic-workflow-lessons.md`) capturing the principle this task surfaced:

> Reports are workflow artifacts, not summaries. A report that under-describes its diff hands the next reviewer a worse task. Every non-trivial edit in the diff must appear in the report. This applies to all agents that write reports, not just Raymond — but Raymond's role (multi-file sweeps across `_docs/` and `_ai/`) makes him the most exposed.

This is the reusable principle. Ward should preserve it because it generalizes — Rob, Kent, and others doing multi-file work face the same temptation, even if they have not stumbled on it yet.

I am not updating other agent definitions for this reason: doing so on speculation rather than observation would violate the very-high-bar rule. Raymond's definition is updated because Raymond's behavior was the observed failure. If Rob or Kent later under-describe a diff, that is the time to extend the discipline to them. For now, the lesson lives in `_ai/` as a cross-agent principle, and only Raymond's individual definition gets the surgical patch.

---

## 5. Summary

- **User-correction track:** NO UPDATE. User did not push back during this task.
- **Raymond report-honesty track:** UPDATE-RAYMOND, surgical, two additions to the existing `raymond.md`.
- **Other agents:** NO UPDATE. Speculation across agents fails the very-high-bar test.
- **Ward / knowledge-librarian:** one recommendation to capture the cross-agent principle in `_ai/` documentation.

The change is principled (fills a structural definition gap), surgical (two short blocks in the existing file), backward-compatible (does not break or contradict existing instruction), and rooted in observation (Linus §4.2/§9 of this task's review). It passes the very-high-bar test.

— Andy
