# User request — 2026-04-28

## Verbatim

> change the project milestones: swap M2 and M3, move M3b to a further milestone because it's very easy to just use git archive | ssh, the client binary can come later. also, move secret files to a later milestone. the most important feature I need right now is the --mount flag support. don't write code for now, just rearrange the milestones

## Constraints called out by the user

- **Docs-only task.** No code changes. Reshuffle the milestone roadmap in the relevant docs.
- **Swap M2 and M3.** What M1-scope.md currently calls M3 (server-side mounts/secret-files/env hardening + client) jumps ahead of M2 (host bootstrap).
- **Move M3b (client binary) further out.** Justification: `git archive | ssh` covers the operator's needs for now; the client binary is not load-bearing.
- **Move secret files further out.** No specific milestone given — needs a placement decision.
- **Top priority: `--mount` flag support.** This is what the user actually needs *right now*. Whatever rearrangement we land on, `--mount` ships in the next milestone.

## Open questions for planning

1. Which existing milestone slot does the deferred M3b client binary land in? A new one, or fold into M7 polish?
2. Same question for secret files — do they pair with M3a hardening or split out?
3. Does host-bootstrap (former M2) become M3, or is bootstrap also pushed back since the operator can `apt install` manually for now?
4. Schema-versioning decision (`_ai/decisions/schema-versioning.md`) hard-codes "M3 will populate Mounts". After the swap, does the populating milestone keep `schema_version = 1` (the existing decision says it should — no shape change) or does the rename invalidate any wording?
5. Files known to mention M2/M3 today (must all be touched for consistency):
   - `_ai/decisions/m1-scope.md` (the canonical sequence at line 32)
   - `_ai/decisions/schema-versioning.md`
   - `_ai/decisions/secrets-split.md`
   - `_ai/decisions/caddy-runs-in-container.md`
   - `_ai/decisions/m1-test-strategy.md`
   - `_ai/container-naming.md`
   - `_ai/MEMORY.md`
   - `_ai/m1x-backlog.md` (item 6 references "M2 material")
   - `_docs/install.md`
   - `_docs/usage.md`

## Workflow

Per CLAUDE.md: PLAN (Don → Joel → Linus, iterate to consensus) → EXECUTION (Kent/Rob skipped since docs-only; Raymond updates docs; Kevlin + Linus review in parallel) → PLAN re-entry to confirm done → FINALIZATION (Ward, Andy).

Kent/Rob have no role on a docs-only task. Don's plan must explicitly justify skipping them so we don't bypass the workflow by accident.
