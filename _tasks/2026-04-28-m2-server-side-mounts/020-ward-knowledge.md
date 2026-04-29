# 020 — Ward preserves M2 learnings

FINALIZATION step 4.1. Reviewed task files 001-019 plus the
integration-test-run-log-handoff note, the M2 branch diff, and the
existing `_ai/` library. Eight candidates considered; four kept (one
new file, one extension, two new files), four dropped with reasons.

## Candidates kept

### 1. NEW: `_ai/compile-clean-not-run-clean.md`

Captures TWO paired rules surfaced by the M2 integration-test arc:

- **Rule 1**: a test requiring external services (real Docker, real
  DB, real network) needs an actual `PASS` from a real run, not a
  `go build -tags integration` log. Compilation proves type-checks,
  not behaviour. M2 v1 shipped `alpine:3.19` + no Cmd, which exits
  under `docker run -d` before `docker exec` can read the marker
  file; nobody had run it. Linus caught it by code-read at impl
  review (`011 §5`), v2 swapped to `nginx:alpine`.
- **Rule 2**: an opt-in test (build-tag + env-var-gated) does NOT
  gate squash-merge when the user-visible surface is independently
  unit-tested. The first real-Docker deploy doubles as the smoke
  check. M2 closeout v1 demanded a maintainer-produced run-log;
  Linus reversed in `017 §2`, Don tie-broke to drop in `018 §1`.

The two rules look contradictory but complement each other: don't
claim a PASS without running, but also don't hold a feature ship
hostage to a maintainer-only run-log when the user-visible surface
is locked by unit tests. The carve-out: the gate IS load-bearing
when the user-visible surface CANNOT be unit-tested without real
infrastructure.

This is the most generalisable lesson from M2 — it'll bite again
the next time we add an integration test (m1x-item 10 curl-through-
Caddy, or any future `-tags integration` test).

### 2. EXTENSION to `_ai/error-wrap-discipline.md`

Added a "Companion rule: one sentinel per chain when the chain
crosses an exit-code router" section. This captures the dual-
sentinel-chain footgun caught by Linus at `004 §"Issue 1"` and
fixed by Joel addendum (`005 §"Issue 1"`) via `Option B` — strip the
inner `ErrInvalidMount` wrap from the CLI path so `parseMountFlags`
produces a chain that only carries `errUsage`.

The rule fits inside `error-wrap-discipline.md` because it's about
how `%w` chains compose multiple sentinels — the original file
covers `%w: %w` vs `%w: %v` (sentinel preservation); this extension
covers the inverse problem (sentinel multiplication at exit-code
routers). Same theme, opposite failure mode.

The "when the rule does NOT apply" carve-out is critical: multiple
sentinels are FINE when both are checked at the same dispatch layer
(`ErrReadiness` + `ErrNoBridgeIP`), problematic only when an
`errors.Is`-router has to choose ONE.

### 3. NEW: `_ai/cobra-stringarray-not-stringslice.md`

Joel caught at planning §8.9 that `pflag.StringSliceVar` splits on
commas — wrong for `--mount` because Linux paths can contain
commas. Use `StringArrayVar` for paths/mount-specs, `StringSliceVar`
only for hostnames/tags where comma-as-separator is desired.

Small file, narrow gotcha, but it's the kind of thing that compiles
fine, passes happy-path tests, and only bites when a real operator's
path contains a comma. The grep recipe (find every `*Var` in
`internal/cli/`, verify values can't contain commas) is the lock.

I considered folding this into `cobra-init-pattern.md` but that
file is about `PersistentPreRunE` for FS-touching init — orthogonal
to flag-type selection. Keeping them separate.

### 4. NEW: `_ai/phantom-scope-kill.md`

Don §1a killed "env-file hardening" as residue from the M3a-bundle
resequence with no runtime/test referent. The discipline: when a
milestone description contains a phrase like "X hardening" or "Y
improvements" without naming a code path, trace it four ways
(grep for TODO/FIXME, read implementation, read backlog, read
upstream task) BEFORE expanding scope. If all four traces come up
empty: kill the phrase explicitly in writing AND strip from the
carrying doc as part of the docs sweep.

This is the inverse of `fix-now-while-fresh.md`: that rule says
fix real defects in scope; this rule says don't invent fake
defects from vague prose. They live next to each other in MEMORY's
"Review discipline" section.

I picked this as a separate file because the action shape differs
fundamentally — fix-while-fresh adds code/diff; phantom-scope-kill
removes nominal scope. Folding them would muddy both rules.

## Candidates rejected

### Image-CMD assumption (candidate c)

Already captured fully in `_ai/m1x-backlog.md` item 6 ("M2 delivery"
paragraph names the `nginx:alpine`-vs-`alpine:3.19` rationale) and
item 11 (future-author note about adding `Cmd []string` to
consolidated `RunOptions`). Adding a third write-up would be
duplicate. The lesson IS preserved — just in the right surface
(backlog, where future-author looking at the integration test or
the consolidation task will find it). Drop.

### Mock-regen-no-op confirms β over α (candidate d)

Useful but narrow. The β-decision-confirmation pattern is one data
point; not yet a doctrine. Adjacent doctrine already exists
implicitly in `m1-test-strategy.md` (the receipt's `go generate
./...` empty-diff item) and explicitly in `m1x-backlog.md` item 11
(Joel decision 4 picked β; mock regen no-op confirmed at impl).
Promoting to its own file would be doctrine inflation on a single
data point. Drop.

### Five-surface flip carve-out deletion (candidate g)

`_ai/cli-flag-surface-coherence.md:42` was already updated by
Raymond at `009-raymond-docs.md` with the "delete the test when
the token disappears from all surfaces" rule explicit and the M2
ship cited. Linus verified at `011 §7`. The doctrine is
preserved; nothing to add. Drop.

### Workflow learnings (candidate i)

Considered: hand-off notes when an agent can't satisfy a
verification gate, iteration count to closeout, the v2 EXECUTION
re-entry shape. None rises to "future-Ward will need this." The
hand-off note pattern is a one-off for the run-log; if it
recurs, it'll get its own entry. Drop.

## Files created/modified

NEW:
- `/Users/fenster/dev/decloud/_ai/compile-clean-not-run-clean.md`
- `/Users/fenster/dev/decloud/_ai/cobra-stringarray-not-stringslice.md`
- `/Users/fenster/dev/decloud/_ai/phantom-scope-kill.md`

MODIFIED:
- `/Users/fenster/dev/decloud/_ai/error-wrap-discipline.md`
  (companion-rule extension after the exemplary-site section)
- `/Users/fenster/dev/decloud/_ai/MEMORY.md`
  (three new index entries + extended `error-wrap-discipline.md`
  summary)

REPORT:
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/020-ward-knowledge.md`
  (this file)

## Why selectivity

Per `_ai/MEMORY.md`'s admonition that not every observation is
library-worthy, and per the existing library being substantial: I
rejected as many candidates as I kept. The four kept entries each
have a distinct future-trigger — the next integration-test
addition triggers compile-clean-not-run-clean; the next CLI flag
that wraps validation triggers error-wrap-discipline's companion
rule; the next repeatable flag declaration triggers
cobra-stringarray; the next milestone-resequence-with-vague-prose
triggers phantom-scope-kill. None of the rejected candidates have
a future-trigger that isn't already covered.

## Files relevant to this task

Task files read end-to-end:
- `_tasks/2026-04-28-m2-server-side-mounts/{001..019}.md`
- `_tasks/2026-04-28-m2-server-side-mounts/integration-test-run-log-handoff.md`

Existing library files reviewed for fit:
- `_ai/MEMORY.md`
- `_ai/cli-flag-surface-coherence.md`
- `_ai/error-wrap-discipline.md`
- `_ai/fix-now-while-fresh.md`
- `_ai/cobra-init-pattern.md`
- `_ai/exit-code-sentinel-not-context-err.md`
- `_ai/m1x-backlog.md`
- `_ai/decisions/m1-test-strategy.md`

Branch diff scope: `git diff main..HEAD --stat` — 48 files changed,
+7518 / -59 lines. Library deltas are 5 files of those 48 (3
created, 2 edited).
