# 022 — Ward's knowledge-librarian pass

Author: Ward Cunningham (knowledge librarian)
Date: 2026-04-27
Status: Distilled durable learnings from this task into `_ai/`. Three new files, one MEMORY.md update.

## Files touched

### Created

- `/Users/fenster/dev/decloud/_ai/docker-bridge-dns.md` — captures the bridge-DNS class-of-bug at a level reusable beyond Caddy. Names the three concrete checks for adding a new component that talks to service containers by name, and includes the meta-lesson Linus called for in `004-linus-review.md` §1.2: when a tech plan corrects an architectural assumption mid-stream, promote the correction to `_ai/decisions/` so future reviewers can audit new code against it. Cross-links to the existing `_ai/decisions/caddy-runs-in-container.md` rather than duplicating its rejected-alternatives list.

- `/Users/fenster/dev/decloud/_ai/doc-grep-discipline.md` — captures the doc-fab review class. Names both M1 incidents (`install.md:173` ports rendering, `:189` IPv6 rendering), the `grep -F` recipe, and the "if surrounding bytes are genuinely variable, show the diagnostic substring inline + frame the surrounding output as variable, do NOT fabricate a clean example" pattern. Originator chain points at Kevlin's cycle-1 catch + Raymond's cycle-2 fix + Linus's cycle-2 verification.

- `/Users/fenster/dev/decloud/_ai/stderr-substring-canary.md` — captures the substring-detection brittleness pattern with the test-as-canary mitigation. Names the five-step pattern (canonical strings only, co-locate with single caller, comment the brittleness, sub-tests per substring + negative branch assertion, drop the inner driver-wrap on the actionable branch). Live example points at `isPortsBoundErr` in `internal/caddy/manager.go` and `TestManager_UpPortsBoundActionableError`.

### Updated

- `/Users/fenster/dev/decloud/_ai/MEMORY.md` — indexed the three new files. Added a **Review discipline** section (new sub-heading; the doc-grep and substring-canary entries didn't fit cleanly under "Implementation patterns" or "Implementation gotchas"). Added a **Cross-references for shapes worth borrowing** section with one bullet for the `PortMap.HostBind` first-class-field pattern (dual-stack publishing as a type-level concern, no auto-bracketing in `formatPortMap`) — this didn't justify a standalone file because the rationale already lives in `_ai/decisions/caddy-runs-in-container.md`, but the *shape* (struct field over string-formatting trick) is worth flagging for future reviewers. Appended two entries to the "Source-of-truth task artefacts" trailer pointing at Don's plan-v2 and Linus's review for the rejected-alternatives enumeration.

## What I deliberately did NOT capture

- **Step-by-step Caddy-in-container fix recipes.** Already in `_ai/decisions/caddy-runs-in-container.md` and the commit. Recipes go stale; the decision record carries the durable "why."
- **Re-narrating M1-scope-as-shield.** `_ai/decisions/m1-scope.md:18` already reads "NOT Viper — plain Cobra + `os.Getenv` is three lines. M2 introduces Viper when there's a real `/etc/decloud/config.toml` to read." Linus's pushback on Joel's `--image` proposal cited that line verbatim. The principle is captured exactly where it needs to be; adding a meta-note "this is how we used `m1-scope.md` in practice" would be self-congratulation.
- **The seven rejected Caddy alternatives.** All in `_ai/decisions/caddy-runs-in-container.md` already. The MEMORY.md trailer now points at `004-linus-review.md` for anyone who wants to see how the enumeration was developed.
- **Cycle-1 implementation mechanics** (`Driver.Exec`, `RunWithOptions`, `cliManager`'s state machine). Derivable from `git log` and the source. The architecture decision record carries the why; the code carries the what.
- **Andy's agent-tuning territory.** Not my pass.

## Bar-check

Each new file teaches a future contributor something they could not derive from `git blame`:

- `docker-bridge-dns.md` — the resolver model is non-obvious from reading any single file; the "promote mid-stream corrections to Decisions" lesson is process, not code.
- `doc-grep-discipline.md` — the `grep -F` discipline is exactly the kind of review checklist that prevents the bug class but isn't visible in the diff that introduced the fab.
- `stderr-substring-canary.md` — the five-step pattern (especially "drop inner wrap on actionable branch + lock branch choice with `assert.NotContains`") is non-trivial; reading `manager.go` alone does not reveal the negative-assertion's purpose.

— Ward
