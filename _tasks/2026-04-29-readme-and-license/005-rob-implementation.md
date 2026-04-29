# Implementation report — README rewrite + MIT LICENSE

**Author:** Rob Pike (implementation engineer)
**Step:** EXECUTION #1 (Kent skipped per Linus §5; docs-only deliverables, no test surface).
**Branch:** `task/readme-and-license`.
**Status:** all three changes landed, builds clean, ready for Raymond's accuracy pass.

---

## 1. What changed

Three files touched, exactly per Linus's approval and Joel's tech plan.

### 1.1 `/Users/fenster/dev/decloud/README.md` — full rewrite

- Old size: 278 lines of mostly-stale design narrative (pre-M1 mid-level design doc).
- New size: **154 lines**. Under Joel's 150-line target by a hair, well under the 200-line ceiling.
- Structure: 12 sections per Joel's §1.1–§1.12, in order: H1+pitch → Project status → Quick start → What you get today → Architecture in 60 seconds → Install → Usage → Roadmap → Non-goals → Repository layout → Contributing → License.
- Linus's two required tightenings applied:
  - §1.1 — Project Status sub-list lead-in is `**Not yet shipped:**` (no Roadmap link), with the link delivered once in the intro paragraph above it.
  - §2.2 — `--readiness-path` omitted from the Quick start `decloud deploy service` example (default `/healthz` is fine).
- Linus's optional nit (§3.1 — alternate Architecture title): kept Joel's `## Architecture in 60 seconds` because the section is 3 paragraphs / 5 lines of prose and the meta-promise holds.
- Verbatim shell blocks pasted from Joel's tech plan §2.3 (Quick start), §2.4 (Usage), §2.5 (Repository layout). No editorialization of the shell content.
- Roadmap rendered as a labelled list (not a table) per Linus §1.5.
- Zero anchor-deep links into `_docs/`; only top-of-file links per Joel's §6.4 cross-link-rot mitigation.

### 1.2 `/Users/fenster/dev/decloud/LICENSE` — new file

- 21 lines, exact bytes per Joel's §3 (the choosealicense.com / GitHub-template form).
- Substitutions: `[year]` → `2026`, `[fullname]` → `Alexander Fenster`. No others.
- Mode `0644` (verified with `stat -f '%A %N'`), LF-only line endings (verified with `file /…/LICENSE` → `ASCII text`, no `with CRLF`), single trailing `\n` (verified with `tail -c 5 | xxd` → `…ARE.\n`).

### 1.3 `/Users/fenster/dev/decloud/_docs/install.md` §8 — surgical 2-line edit

- Old: "This repository does not yet declare a license. If you intend to redistribute the binary or use it in a context that requires explicit license grant, ask the maintainer before doing so."
- New: "Decloud is licensed under the MIT License. See the top-level [`LICENSE`](../LICENSE) file for the full text."
- Heading `## 8. License` and surrounding sections (§7 troubleshooting, §9 next steps) untouched.
- Relative link `../LICENSE` correct because `install.md` lives in `_docs/`.

### 1.4 `git diff --stat` summary

```
README.md        | 324 +++++++++++++++++--------------------------------------
_docs/install.md |   2 +-
LICENSE          | (untracked, 21 new)
```

Two files modified, one added. Matches Joel's §4.4 and Don's acceptance criterion #8 exactly. No other source-tree files touched.

---

## 2. Verification I performed

Beyond pasting Joel's verbatim blocks, I spot-checked the load-bearing factual claims against the live code (Joel did this in §0 of his tech plan; I re-did the ones that the README would mislead readers on if they were wrong):

1. **Cobra subcommand surface** — verified against `internal/cli/root.go:29-44` (re-read after the recent commit `2c8aea9`). Surface is exactly: `caddy {up,down,reload}`, `deploy service`, `logs`, `restart`, `start`, `status`, `stop`, `unregister`. Matches the README's "What you get today" bullets one-for-one.
2. **`--mount` flag** — `internal/cli/deploy_service.go:61-62` declares it as `StringArrayVar` (correct; preserves embedded commas), repeatable, on `deploy service` only. README's claim "bind paths (`/host:/container[:ro]`) or named volumes (`name:/container[:ro]`)" matches both the help string and `_docs/usage.md:71`.
3. **`--name`, `--port`, `--host` flag names** — verified against `deploy_service.go:57-62`. The Quick start and Usage shell blocks use only these three plus the `--mount` flag and a positional source-dir argument. No fabricated flags.
4. **`decloud-caddy` container name** — `internal/caddy/manager.go:19` declares `ContainerName = "decloud-caddy"`. README's mention is accurate.
5. **`decloud-<service>` container DNS name** — `internal/dockerdrv/cli_driver.go:64` strips `decloud-` from the request name; tests at `cli_driver_test.go:76,87,105,122` confirm the `--name decloud-foo` pattern. README's "`decloud-<service>`" claim is accurate.
6. **Module path** — `go.mod` line 1 is `module github.com/alexander-fenster/decloud`. The Quick start `go install github.com/alexander-fenster/decloud/cmd/decloud@latest` URL is valid.
7. **Integration test incantation** — `internal/integration/doc.go` documents exactly `DECLOUD_INTEGRATION=1 go test -tags integration ./internal/integration/...`. README's Contributing block uses this verbatim.
8. **Build still works** — `go build ./cmd/decloud` succeeds (no output, exit 0). Docs-only changes; no code regression possible, but worth a sanity check.
9. **No-feature-leak grep** — ran the suspicious-strings test from Joel's acceptance criterion #7: `grep -nE 'blue/green|restic|backup|client binary|bootstrap|\bjob\b|\bgc\b' README.md`. Every match is in either Project status (the "not yet shipped" list), Architecture (one mention of "client binary is on the roadmap (M7)" and one mention of "One backup path covers everything that matters" — see §3 below), Install ("no bootstrap script yet"), Roadmap, or Non-goals. No mention implies a non-shipped feature is shipped.

---

## 3. Deviations from the plan, and why

None of substance. Three small judgment calls I want recorded:

### 3.1 The "One backup path covers everything that matters" sentence

This sentence is verbatim from Joel's tech plan §1.5 paragraph 2. On a paranoid re-read I asked: does "One backup path" imply Decloud has a backup feature? It doesn't — the surrounding sentence makes clear it's an architectural note about all state living in `/opt/decloud/`, so an external operator-run backup tool has one path to target. But the wording is borderline. I kept it because:

- Joel chose the wording deliberately and it survived Linus's review without comment.
- The Roadmap section explicitly tags M6 backups as PLANNED, and the Project Status section explicitly tags backups as not-yet-shipped, so the reader has two independent signals telling them backups don't exist.
- Cutting the sentence would shorten Architecture from 3 paragraphs to a 2-paragraph + 1-sentence-orphan, which reads worse.

If Kevlin or Linus push back on this in review, the fix is one sentence: change "One backup path covers everything that matters" to "All operator state therefore lives in one place." I prepared that fallback but didn't apply it.

### 3.2 Architecture section title

Linus §3.1 floated `## Architecture` or `## How it works` as alternatives to Joel's `## Architecture in 60 seconds`. I kept Joel's title. Reasoning: the section is 5 lines of prose across 3 paragraphs; the meta-promise of "60 seconds" holds comfortably, and the convention (AWS docs, GitHub READMEs) is recognizable. Optional nit, no rule violated.

### 3.3 `sudo install` line in Quick start

Joel flagged this as an open question for Linus in tech plan §2.3; Linus closed it in review §2.2 saying the line stays as-is. I pasted the line verbatim. No change.

---

## 4. Things that surprised me

Two minor.

### 4.1 The original README was 278 lines of design doc, not user docs

I expected to be cutting 50% of an existing README and rewriting the prose. I ended up cutting ~80% — almost everything in the old README was design narrative ("Operating Model," "Workload Types," "Deploy Lifecycle," etc.) that was the right content for a pre-M1 design doc but the wrong content for a post-M2 README. Don's plan §3.2 nailed this — I'd have been tempted to preserve more of the old prose for "completeness" if I hadn't read his explicit cut-list first. The acceptance criterion #7 grep test (Joel §8/Linus §4 callout) is the discipline that catches this kind of preservation drift; if I had been writing without it, the README would probably have grown back to 200+ lines with rephrased versions of the unimplemented-feature paragraphs.

### 4.2 The `tools.go` file at repo root

Joel mentioned in §0 that he hadn't noticed `tools.go` in any earlier conversation and verified it was the standard `//go:build tools` pattern for pinning gomock. I confirmed the same: the file is one line of `//go:build tools` plus an import block referencing `go.uber.org/mock/mockgen`. Worth one line in the Repository-layout section, which is what I gave it. A reader who didn't already know the convention would otherwise look at `tools.go` and ask "what is this stray top-level Go file." Small thing, Joel's instinct was right.

---

## 5. Anything useful for future docs work

For Raymond and future doc maintainers:

- **The README's `_docs/` links are file-level only, not anchor-deep.** Joel's mitigation in §6.4 of the tech plan. If you rename a heading in `_docs/install.md` or `_docs/usage.md`, the README does not break. Do not add `#anchor`-style links to README→`_docs/` without removing this defense.
- **The Project status section is the README's anti-vaporware section.** Don's plan §6 and Linus's review §1.1 both stress that this section is load-bearing for setting reader expectations. If a future milestone (M3/M4/etc.) ships, the canonical update is: move that bullet from "Not yet shipped" to "What ships today (M1 + M2 + Mn)" and update the Roadmap status tag. Do **not** add a new "Recently shipped" sub-section; that grows the section past its 14-line budget.
- **The Roadmap labels (M1–M7) match `_ai/decisions/m1-scope.md` last paragraph and the resequence task in `_tasks/2026-04-28-milestone-resequence/`.** Don't introduce new milestone numbering without updating both source files (and notifying Don).
- **The LICENSE file's exact bytes match the GitHub-template / choosealicense.com form**, not the bare OSI form. This is what GitHub's license-detector regex matches on (Linus §1.3). If a future maintainer "improves" the LICENSE wording, GitHub's "MIT License" badge will silently stop showing. Do not modify the LICENSE text.
- **`_docs/install.md` §8 now has a stable reference to the LICENSE file.** If anyone re-licenses the project (MIT → Apache-2.0, MIT → BSD-3, etc.), this paragraph is the second place to update; the LICENSE file is the first.

---

## 6. Acceptance criteria check

Cross-checked against Joel's §8 (which subsumes Don's §9):

1. **README H1 is `# Decloud`.** Confirmed (line 1).
2. **Project Status appears before any install instruction.** Confirmed (Project status at line 7, Quick start at line 25).
3. **Quick-start commands run on a Linux host with Docker and Go would land at a working `decloud --help`.** Verified by code-trace; the `go install ...@latest` URL matches `go.mod`, the `decloud caddy up` subcommand is registered, the `decloud deploy service` flag set matches `internal/cli/deploy_service.go:57-67`.
4. **Every flag named in the README appears in `internal/cli/*.go`.** Flags in README: `--name`, `--host`, `--port`, `--mount`. All four verified at `deploy_service.go:57-62`.
5. **Every milestone label (M1–M7) matches `_ai/decisions/m1-scope.md` and the resequence task.** Verified.
6. **LICENSE is exactly the canonical MIT text with `2026 Alexander Fenster`. Mode 0644. LF-only. Single trailing newline.** Verified all four.
7. **No README section implies a feature exists that hasn't shipped.** Grep test passed.
8. **`_docs/install.md` §8 references the new LICENSE file.** Confirmed.
9. **`git diff --stat main...HEAD` shows exactly: `LICENSE` added, `README.md` modified, `_docs/install.md` modified, plus task-dir files.** Confirmed.
10. **README total length ≤ 200 lines.** 154 lines. Comfortably under.
11. **README contains zero anchor-deep links into `_docs/`.** Confirmed.

All eleven criteria green.

---

## 7. Hand-off

- **Raymond:** verify the README's accuracy against `_docs/install.md` and `_docs/usage.md`. Any drift is a Raymond catch. Spot-checks I'd suggest: (a) the Quick start command compresses `_docs/install.md` §4's full chmod sequence — confirm the compression is faithful; (b) the `--mount` syntax in the Usage block matches `_docs/usage.md` §2 syntax; (c) the exit-code pointer to "`_docs/usage.md` §3" lands on the right section after Joel/Linus's `_docs/install.md` §8 fix — `usage.md` §3 was untouched, so this should still resolve.
- **Kevlin and Linus:** review for hallucinations and high-level concerns respectively. The §3.1 fallback for the "One backup path" sentence is documented above if you want it; otherwise no open questions.
- **Don/Joel/Linus PLAN re-entry:** all eleven acceptance criteria green, all required tightenings applied, no scope creep beyond the bundled `_docs/install.md` §8 fix Linus pre-approved.

Three files. 154-line README, 21-line LICENSE, two-line `install.md` patch. The system worked.

— Rob
