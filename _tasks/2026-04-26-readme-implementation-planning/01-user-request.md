# User Request

**Date:** 2026-04-26
**Task slug:** readme-implementation-planning
**Repo state at request time:** Only `README.md` and `CLAUDE.md` exist at the repository root. No Go code, no `go.mod`, no `_docs/`, no `_ai/`. Three prior commits, all documentation:

```
5786304 docs: add initial CLAUDE.md
997bcbc docs: expanded README
54ceed9 docs: document declouding platform direction
```

The README has just been finalized as the design spec for the Declouding platform — a single-host, Docker + Caddy + systemd-timers replacement for a small set of Cloud Run / host-systemd / Cloud Scheduler workloads, driven by a `decloud` CLI over SSH. CLAUDE.md fixes the implementation language as Go with Cobra/Viper/TOML/Testify/Gomock and mandates the subagent workflow.

## Verbatim request

> Discuss the documentation in README.md, carefully plan implementation steps, decide which part must be implemented first.

## Framing

This is the very first planning task on the repo. The user wants:

1. A discussion of what the README actually specifies (not a re-summary — a planner's read).
2. A carefully decomposed sequence of implementation milestones.
3. An explicit, defended choice of **what gets built first**.

No code is to be written this round. Output is the plan in `02-plan.md`. Joel Spolsky will then expand the plan with technical detail; Linus Torvalds will review.
