# Raymond doc-update pass — README rewrite + MIT LICENSE

**Author:** Raymond Chen (the doc writer)
**Step:** EXECUTION #2 (post-Rob, pre-Kevlin/Linus review)
**Branch:** `task/readme-and-license`
**Status:** four files reviewed against the new README; three small edits to `_ai/decisions/`; no edits to `_docs/install.md` or `_docs/usage.md` beyond Rob's §8 fix.

---

## 1. What I read

In order:

1. The new `README.md` (154 lines) at HEAD.
2. The new top-level `LICENSE` (21 lines) at HEAD.
3. `_docs/install.md` at HEAD — including Rob's surgical §8 edit.
4. `_docs/usage.md` at HEAD — untouched on this branch.
5. `_tasks/2026-04-29-readme-and-license/005-rob-implementation.md` — Rob's hand-off notes.
6. The full `_ai/` tree, looking for stale references to "no license," "missing README," license-ambiguity language, or claims about the README's content that the rewrite invalidated.

I did NOT re-verify Rob's load-bearing factual claims against the live code. Rob did that work in his §2; redoing it would be makework. I cross-referenced where the README and `_docs/` overlap, and where `_ai/` makes claims about the README.

## 2. README ↔ `_docs/` consistency check

Three areas where the README compresses what `_docs/` says in full. All three compressions are honest.

### 2.1 Quick-start `/opt/decloud/` tree creation

README:30–31:

```sh
sudo mkdir -p /opt/decloud/{config/{services,jobs,caddy},secrets,state/deploys,logs}
sudo chmod 0700 /opt/decloud/secrets
```

The brace expansion creates exactly the six directories that `install.md` §4 creates one-line-at-a-time: `config/services`, `config/jobs`, `config/caddy`, `secrets`, `state/deploys`, `logs`. The README explicitly hands off to `install.md` §4 for the full chmod sequence (`# 1. Create the Decloud state tree (full chmod sequence in _docs/install.md §4).`). The only `chmod` the README inlines is the `0700` on `secrets/` — the security-critical one that the loader rejects deployments over if wrong (`install.md` §4 paragraph 3). Honest compression.

### 2.2 `--mount` syntax

README:60 says:

> `--mount` for `deploy service` — bind paths (`/host:/container[:ro]`) or named volumes (`name:/container[:ro]`).

`usage.md` §2 row for `--mount` says:

> `<host-path>:<container-path>[:ro]` (bind) or `<name>:<container-path>[:ro]` (named volume); repeatable.

The README's two-form summary matches the `usage.md` flag-table cell shape-for-shape. The `usage.md` row carries additional validation rules (regex for named-volume names, mode-flag rejection list, duplicate-container-path rule) that the README correctly delegates rather than repeating.

### 2.3 Exit-code pointer

README:96 says "See `_docs/usage.md` §3 for exit codes". `usage.md` §3 is the exit-code table at line 171 onward; the section heading is `## 3. Exit codes`. Resolves correctly.

### 2.4 Architecture in 60 seconds

README:64–70 makes three architectural claims:

- "Docker runs every workload, including Caddy itself" — matches `install.md` §3 and `usage.md` §6.
- "Caddy reaches each service container by its Docker DNS name (`decloud-<service>`)" — matches `usage.md` §6 paragraph 1.
- "There is no Decloud daemon and no listening management port" — matches `install.md` §5 paragraph 4 ("There is no `decloud daemon`, no `decloud bootstrap`, and no `systemctl enable decloud`").

No drift.

## 3. Stale-reference sweep on `_docs/`

Ran `grep -n -E '(license|LICENSE|README)' _docs/install.md _docs/usage.md`. Single hit: `install.md:214`, the line Rob already replaced. No stale "no license" / "missing README" / "license-ambiguity" language remains in `_docs/`.

Ran `grep -rn -E '(no license|TODO|FIXME|XXX)' _docs/`. No hits in `_docs/`. (Two TODO mentions exist in `_ai/phantom-scope-kill.md` and `_ai/container-naming.md`, but those are about the *concept* of TODO comments in code, not actual stale TODOs in docs.)

## 4. Stale-reference sweep on `_ai/`

This is where I found genuine drift. Three files in `_ai/decisions/` make present-tense claims about README content that the rewrite removed.

### 4.1 `_ai/decisions/no-magic-zero-modes.md:25` — broken line-number reference

Old text included `(cross-referenced README.md:215 and _ai/decisions/m1-scope.md:32)`. The pre-rewrite README:215 was inside the client/server CLI surface section (verified via `git show 235e624:README.md`); the new README is 154 lines, so `README.md:215` no longer exists at all. Genuine broken cross-reference.

**Fix applied:** replaced the bare `README.md:215` cite with a content-based phrase ("the pre-rewrite README's CLI-surface section") and added a forward pointer to `_docs/usage.md` §2, which is now the user-facing home of the `--port`-required contract:

```
(cross-referenced the pre-rewrite README's CLI-surface section and `_ai/decisions/m1-scope.md:32`; the user-facing `--port`-required contract now lives in `_docs/usage.md` §2).
```

`_ai/decisions/m1-scope.md:32` is still the milestone-sequence line (verified); that half of the cite still resolves.

### 4.2 `_ai/decisions/secrets-split.md:3` — false present-tense claim

Old text: `The README's "Handling secrets" section requires structural separation...`. The new README has no "Handling secrets" section. The decision rationale is still load-bearing (the type system enforces the split), but the README citation no longer resolves.

**Fix applied:** past-tensed the README cite and added a one-clause note that the requirement now lives in the type system regardless of where it was originally documented:

> The pre-rewrite README's "Handling secrets" section (the design-narrative draft that drove M1 scoping) required structural separation... The requirement is now load-bearing in the M1 type system regardless of where it was originally documented.

### 4.3 `_ai/decisions/m1-scope.md:13, 14, 17` — false present-tense claims

Three bullets quoted or paraphrased the pre-rewrite README:

- Line 13: `README explicitly says server CLI is "equally usable by a human SSH'd in."` — the old README:209 had that exact phrase; the new README does not.
- Line 14: `README says blue-green is the default; M1 explicitly downgrades.` — the old README discussed blue-green as default; the new README does not.
- Line 17: `README's supervisor talk waits for M7.` — the old README discussed supervision; the new README does not.

**Fix applied:** changed `README` → `the pre-rewrite README` on all three lines. Decisions themselves untouched (they are still correct M1 scope decisions); only the citation tense moved from present to past so the doc reads as a historical record rather than a false present-tense claim.

### 4.4 What I deliberately did NOT touch in `_ai/`

`_tasks/` contains many README line-number references (e.g., `_tasks/2026-04-26-fix-deploy-service-review-findings/04-linus-review.md:147` cites `README.md:215`, the same broken anchor). These are historical task records — they describe the state of the world at a specific moment in time and are immutable by convention. Rewriting them would be falsifying the workflow trail. Left alone.

`_ai/decisions/m1-scope.md:36` references `_tasks/2026-04-26-readme-implementation-planning/07-linus-review-v2.md` — that file still exists and the cite still resolves. Untouched.

The other `_ai/` files (`cancellation-symmetry-audit.md`, `cli-flag-surface-coherence.md`, etc.) make no README-content claims; the `grep -rn` sweep returned zero matches against them.

## 5. What I changed

Three files, three small edits, all in `_ai/decisions/`:

- `_ai/decisions/no-magic-zero-modes.md` — one-line edit at line 25, replacing the broken `README.md:215` cite with a content-based phrase plus a forward pointer to `_docs/usage.md` §2.
- `_ai/decisions/secrets-split.md` — one-line edit at line 3, past-tensing the README cite and adding a one-clause note that the requirement is now load-bearing in the type system.
- `_ai/decisions/m1-scope.md` — three-token edit (lines 13, 14, 17), prepending "the pre-rewrite" to each `README` citation.

`git diff --stat` summary (approximate):

```
_ai/decisions/m1-scope.md          | 6 +++---
_ai/decisions/no-magic-zero-modes.md | 2 +-
_ai/decisions/secrets-split.md     | 2 +-
```

No code changes, no test changes, no `_docs/` changes.

## 6. What I did NOT change, and why

- `README.md` — Rob's text reads accurately against `_docs/install.md` and `_docs/usage.md`. The three §2.x compressions above are honest. No changes warranted.
- `LICENSE` — exactly the GitHub-template MIT form per Rob's verification. The `2026 Alexander Fenster` substitution is correct. No changes warranted.
- `_docs/install.md` — Rob's §8 fix is the single license-related update needed. The rest of `install.md` makes no license-ambiguity claims and no missing-README references. Verified by `grep`. No changes warranted.
- `_docs/usage.md` — never made license claims; never referenced a README. No changes warranted.
- `_ai/m1x-backlog.md`, `_ai/MEMORY.md`, and the rest of the `_ai/` flat files — `grep` returned no README-content claims. No changes warranted.
- `CLAUDE.md` — describes the workflow, makes no claims about README content. No changes warranted.

## 7. Verification I performed

- `grep -rn -E '(license|LICENSE|README)' _docs/install.md _docs/usage.md` — single hit, the post-fix §8 line.
- `grep -rn -E '(no license|undeclared license|license ambiguity|missing README|no README|TODO|FIXME|XXX)' _docs/ _ai/` — no matching stale references after my edits.
- `grep -rn -E 'README\.md:[0-9]+' _ai/` — only `no-magic-zero-modes.md:25` remained; my edit replaced the bare line-number with content-based phrasing.
- `go build ./cmd/decloud` — clean build (sanity check; no code changed but worth confirming).

## 8. Hand-off

- **Kevlin (low-level review):** the three `_ai/decisions/` edits are surface-level. The rationale of each decision is unchanged; only the README citation tense moved. The substantive question for review is whether the `secrets-split.md:3` rephrasing accurately describes the current source of truth (M1 type system enforcement) — I claim it does, but please verify against `internal/registry/types.go` if you want belt-and-braces.
- **Linus (high-level review):** the README itself is Rob's; my pass found no consistency issues with `_docs/install.md` or `_docs/usage.md`. The "One backup path covers everything that matters" sentence Rob flagged in his §3.1 fallback is also fine to my eye — Roadmap §M6 and Project Status both tag backups as not-yet-shipped, the architecture-note framing reads correctly. No fallback needed unless you disagree.
- **Don/Joel/Linus PLAN re-entry:** acceptance criterion #8 (install.md §8 references the new LICENSE) is still green; I did not regress it. No new acceptance criteria triggered.

Three small `_ai/decisions/` cleanups. The README is sound; `_docs/` is sound. The sweep was worth doing because the broken `README.md:215` line-number reference would have rotted further as people grep for "where is the port=0 contract documented."

— Raymond
