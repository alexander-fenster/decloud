# Ward's learnings report

I extracted three reusable learnings from this task and indexed them. The mechanical updates Raymond already shipped to `_ai/cobra-init-pattern.md` and `_ai/m1x-backlog.md` cover the line-number/signature freshness; my job was the *generalizable principle* layer above that.

---

## What I preserved

### 1. `_ai/explicit-inputs-not-globals.md` (NEW)

**The lesson:** Functions that consume already-resolved config values should take them as parameters, not re-read env/globals/Viper. Wrong call site can't compile; wrong runtime config can't sneak in.

**Why this earns a permanent file:** Finding 2 was a textbook instance. The fix (`logging.Init(root string) error`) is a structural contract change. The four rejected alternatives (setter pattern, Viper read, env-fallback, hidden global) are all the patterns that produce this bug class in the first place — naming them explicitly arms future-Don to reject them faster.

**What I did NOT include:** the line-by-line diff (already in `cobra-init-pattern.md`), the Cobra flag-default-from-env mechanism (already documented in `cobra-init-pattern.md` after Raymond's update). I cross-referenced the lock-in test (`TestInit_UsesPassedRootNotEnv`) without re-explaining what it does.

### 2. `_ai/decisions/no-magic-zero-modes.md` (NEW)

**The lesson:** `--port=0` is rejected at validation, NOT treated as "worker mode." M5 workers get a separate command (`decloud deploy job`). The dangerous variant — "skip readiness on port=0" — would record successful deploys for containers that exit immediately, which is the worst possible operator-facing outcome.

**Why this earns a permanent decision record:** This is exactly the kind of strategic call that should not be re-litigated when M5 lands. Without preservation, a future engineer will look at "deploy worker shape" and propose port=0 as a mode of `deploy service` because it's "less code." The decision record names *why* less code in that direction is more cost downstream.

**Where I placed it:** `_ai/decisions/` alongside `m1-scope.md` and `m1-test-strategy.md` — strategic, not tactical.

**What I did NOT include:** the Cobra `MarkFlagRequired` discussion (tactical, in tech plan, not worth surfacing). The "deployer/probe/driver stays simple" note (already captured implicitly in the layering work).

### 3. `_ai/cli-flag-surface-coherence.md` (NEW)

**The lesson:** Every CLI flag has FOUR surfaces — runtime check, error message, `--help` text, `_docs/usage.md` — that must agree. Change one, audit all four. Iter1 of this task fixed three findings of CLI/docs drift and nearly shipped a fresh instance of the same bug class via the `--port` help string. Both reviewers blocked independently.

**Why this earns a permanent file:** This is a class-of-bug insight, not a one-off. It's also a checklist: the `git grep` recipe gives future engineers a mechanical defense. The "no test on help string" note inoculates against well-meaning change-detector tests.

**What I did NOT include:** the specific iter2 one-line diff (it's in the task file). The `--name` flag's exact help string (cited as a pattern but not as a load-bearing detail).

### 4. `_ai/MEMORY.md` (UPDATED)

Added the three new entries with one-line index summaries. Kept ordering consistent with the existing taxonomy (decisions vs implementation patterns).

---

## What I considered and rejected

### Rejected: a file on "CLI-shaped contracts live in the CLI layer"

The Finding 1 fix (resolve `--dockerfile` against source-dir at the CLI, not in `dockerdrv`) is a layering insight. But it overlaps too much with `optional-input-two-layer.md`'s framing and would duplicate Linus's rationale in `04-linus-review.md`. The principle "the layer that documents the contract enforces the contract" is already implicit in how the existing files are written. Adding a fourth file would dilute, not strengthen.

### Rejected: a gotcha entry for the macOS symlink test fix

Rob's `filepath.EvalSymlinks(t.TempDir())` workaround for `/var` → `/private/var` is real, but it's a one-test detail any Go developer hits once and fixes. Not a recurring pattern. The diff itself is self-documenting (one line, one comment).

### Rejected: a separate file on "registry persistence of resolved paths is better provenance"

Linus made the point in `04-linus-review.md` that storing absolute Dockerfile paths in the registry is *better* provenance than storing the user-typed bare `"Dockerfile"`. True and worth knowing. But it's a second-order benefit of the Finding 1 fix, not a reusable pattern. Future engineers benefit from the absolute-path provenance whether they know about this rationale or not.

### Rejected: an entry on the empty-string `dockerfile` defensive guard

Joel's G8/R2 analysis (without `if dockerfile == ""` the `filepath.Join(abs, "")` collapses to `abs`, producing a confusing Docker error) is good. But it's specific to `filepath.Join` semantics — a Go-level gotcha, not a project-level one. Documenting every defensive-guard rationale at the `_ai/` level would balloon the library.

### Rejected: a fix to the `_ai/cobra-init-pattern.md` pseudo-Go nit

Both Kevlin and Linus flagged the line "Do not collapse to `Init(string) {}`" — `{}` with no return type isn't valid Go syntax for a function with an `error` return. Don explicitly chose not to fix it (`014-don-final.md` §"Non-blocking items I'm explicitly NOT acting on"). I respect that decision. If I'm touching the file again later for an unrelated reason, I'll fold it in then.

### Rejected: an update to `_ai/decisions/m1-scope.md` cross-referencing the new `no-magic-zero-modes.md`

`m1-scope.md` already says M5 = jobs via systemd timers. Cross-referencing inside the M1 scope doc would be backwards (the worker decision is downstream of M1, not part of it). The new `no-magic-zero-modes.md` cross-references `m1-scope.md` instead, which is the right direction.

---

## Files written

| Path | Status | Purpose |
|---|---|---|
| `/Users/fenster/dev/declouding/_ai/explicit-inputs-not-globals.md` | NEW | Why `Init(root string)` beats env-read; rejected alternatives |
| `/Users/fenster/dev/declouding/_ai/decisions/no-magic-zero-modes.md` | NEW | M1 port=0 policy; M5 worker shape; magic-mode anti-pattern |
| `/Users/fenster/dev/declouding/_ai/cli-flag-surface-coherence.md` | NEW | Four-surface drift class-of-bug + grep recipe |
| `/Users/fenster/dev/declouding/_ai/MEMORY.md` | UPDATED | Index entries for the three new files |

No files removed. No files renamed. No files written outside `_ai/` and this report.

— Ward
