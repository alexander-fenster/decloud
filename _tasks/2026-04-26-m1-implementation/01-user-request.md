# User request: proceed with M1 implementation

**Date:** 2026-04-26
**Requester:** Alexander Fenster (alexander.fenster@gmail.com)
**Trigger:** This task immediately follows planning task `_tasks/2026-04-26-readme-implementation-planning/`, where Linus APPROVED Don's plan v2 and Joel's tech plan v2. The user has now greenlit the EXECUTION step.
**Workflow phase:** Steps 2 + 3 of CLAUDE.md (PLAN-for-execution, then EXECUTION).

## Verbatim request

> proceed with M1 implementation. for now, use unit tests to test the functionality; I will test it on a real system after M1 is done. please provide installation instructions and short usage documentation.

## What this changes vs the prior tech plan

Two new constraints from the user that override sections of the approved tech plan:

1. **Unit tests only.** The user explicitly says "I will test it on a real system after M1 is done." Joel's tech plan §12.2 called for integration tests gated by `-tags integration` (real Docker, real Caddy, real `caddy validate`). Those tests are **out of scope for M1 execution** and are deferred. M1 coverage comes entirely from unit tests with mocks/fakes (per CLAUDE.md item 4: Testify + Gomock). The `internal/envcap` package is the one carve-out — it must run against a real `/bin/bash` because that's the very thing under test, but that's still a unit test from `go test ./...`'s perspective.

2. **Installation instructions + short usage docs are explicit deliverables.** The user named these as outputs they want. Raymond's `_docs/` deliverables in tech plan §10 already cover this in spirit (`_docs/operator/manual-install.md`, `_docs/cli/decloud-deploy-service.md`), but the framing changes: these are not "nice to have" docs, they are the artifacts the user will read first when they pick up the binary to test on a real system. Quality bar is high.

## Anchors (read these, not just this file)

- `_tasks/2026-04-26-readme-implementation-planning/05-plan-v2.md` — Don's M1 scope and rationale.
- `_tasks/2026-04-26-readme-implementation-planning/06-tech-plan-v2.md` — Joel's full M1 tech plan (type shapes, exact loader semantics, capture mechanism).
- `_tasks/2026-04-26-readme-implementation-planning/07-linus-review-v2.md` — Linus's approval; flagged borderline-but-not-blocker edge cases on env capture (set +a, arrays, readonly).
- `/Users/fenster/dev/declouding/CLAUDE.md` — workflow + conventions (Cobra, Viper, Testify, Gomock, gofmt, no obvious comments).
- `/Users/fenster/dev/declouding/README.md` — system spec.
- `/Users/fenster/dev/declouding/_ai/MEMORY.md` and the files it indexes (`m1-scope.md`, `secrets-split.md`, `schema-versioning.md`, `envcap-portable-bash.md`, `container-naming.md`).

## Out of scope

Everything in plan-v2 §7.2 (M1 explicit non-goals) plus integration tests (per the new user constraint).
