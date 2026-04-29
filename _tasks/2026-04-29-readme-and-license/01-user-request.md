# User Request

> read through the docs in _docs and update README.md accordingly - currently, README.md is more like a high level design, let's make it into a real README. also, add MIT license

## Interpretation

Two deliverables:

1. **Rewrite `README.md`** — current file reads like a high-level design doc; user wants it converted into a proper README (project intro, install, usage, quick start, etc.). The accurate, up-to-date project information should be sourced from the `_docs/` directory.
2. **Add MIT license** — add a `LICENSE` file with the standard MIT license text. Author/copyright holder is Alexander Fenster (per git config).

## Constraints

- Workflow: PLAN → EXECUTION → FINALIZATION (per CLAUDE.md).
- All work via subagents.
- Commit after each workflow step.
- README content must reflect the actual current state of the project (don't fabricate features that don't exist; verify against `_docs/` and code).
