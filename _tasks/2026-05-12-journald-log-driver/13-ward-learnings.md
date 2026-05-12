# Ward — preserved learnings (journald log driver)

STEP 4a (FINALIZATION — knowledge preservation). I read reports 01–12 plus
the live diff against `main` (`internal/dockerdrv/driver.go`,
`cli_driver.go`, the four caller sites, the existing `_ai/decisions/`
record, neighbouring `_ai/*.md` to match voice). I then extracted four
cross-cutting patterns worth keeping plus an index update.

The decision record `_ai/decisions/journald-log-driver.md` already carries
the *why* of the task verbatim — I did not duplicate it. The new entries
are the *generally-reusable patterns* extracted from the task, framed so
they apply to future work that isn't journald.

## Files touched

### New `_ai/*.md` entries (four)

- `_ai/two-sentinels-for-two-failure-modes.md` — when a guard rejects on
  two semantically distinct reasons, declare TWO sentinels and write
  `errors.Is`-positive + `errors.Is`-negative-on-the-other tests so a
  future "simplification" that folds them fails at PR time. Live shape:
  `ErrEmptyService` + `ErrInvalidService` in
  `internal/dockerdrv/driver.go:22-34`, locked by Kent's §6.2.1–§6.2.4
  test set. Cross-refs `error-wrap-discipline.md` (companion: one
  sentinel per chain across an exit-code router).

- `_ai/guard-fires-before-exec.md` — rejection tests for guards-before-
  side-effects assert `assert.Empty(t, records)` on the recording fake-exec,
  not just `require.Error`. A future refactor that moves the guard AFTER
  `cmd.Run` still returns an error and silently leaks the side effect; the
  negative-record assertion is the lock. Names the maintainer's no-Docker
  constraint (`MEMORY.md` `feedback_no_docker.md`) as the test-discipline
  enabler.

- `_ai/identity-as-field-not-derived.md` — generalises the `TrimPrefix`
  smell kill. Flow identity (`Service`) as an explicit struct field; do
  NOT recover it via `strings.TrimPrefix(otherField, "prefix-")` or
  `strings.Split(...)[N]`. Driver-layer guard at the leaf catches
  zero-value accidents on new call sites. Cross-refs
  `_ai/container-naming.md` for the M4 rename that motivated the
  forward-compat argument.

- `_ai/sealed-at-create-lock-with-notcontains.md` — when external state is
  sealed at create-time (Docker's `HostConfig.LogConfig`,
  systemd-daemon-reload, cloud-init), the lifecycle-path argv tests carry
  `assert.NotContains` rows for every create-time flag. Positive
  `Contains` on the create path + negative `NotContains` on every
  start/restart path. Locks both directions of the invariant. Live:
  `TestCLIDriver_StartArgs` extension at `internal/dockerdrv/cli_driver_test.go`.

### Updated `_ai/MEMORY.md` (the library index)

- Added a one-line entry under "Architecture decisions" pointing at
  `decisions/journald-log-driver.md` (Raymond's full record).
- Added the four new pattern entries under their appropriate sections
  ("Implementation patterns (reusable)" for the sentinel and identity
  patterns; "Review discipline" for the two test-pattern entries).
- Refreshed the `m1x-backlog.md` summary line — count went from "five" to
  "twelve items deferred or split off" with named items.

### Updated `_ai/m1x-backlog.md` item 11

Added the P3 note Don asked for in `10-don-final-check.md` §9: during the
`RunRequest`/`RunOptions` consolidation, grep `internal/dockerdrv/` for
the literal phrase `RunRequest/RunOptions` and update the matching error
message strings. `ErrEmptyService`'s message names both types verbatim; a
consolidation that doesn't touch the message leaves a stale literal that
the next maintainer will chase as a "hallucination" later. Cited Don's
report as originator.

### New report file (this file)

- `_tasks/2026-05-12-journald-log-driver/13-ward-learnings.md`.

## Patterns I deliberately did NOT extract

- **"Always-on `--log-driver=journald`, no flag"** — the always-on policy
  is task-specific (journald, Decloud, systemd hosts). The
  *generalisation* would be "don't add a config knob for a value the
  environment already determines" — already covered by
  `decisions/no-magic-zero-modes.md` and `decisions/caddy-runs-in-container.md`
  in their respective contexts. Not pulling it out as a third instance.

- **"`journalctl CONTAINER_TAG=` is exact-match, not regex"** — operator
  documentation. Lives in `_docs/usage.md` §6 + the decision record's
  "Why journald and not syslog" section. Not a generalisable pattern.

- **"Driver-level guard locks invariants that upstream validation merely
  documents"** — true, but mostly a restatement of the
  `identity-as-field-not-derived.md` rule plus the "Cobra help-string
  regex `[a-z][a-z0-9-]{0,38}` is unenforced" observation from the
  decision record. Pulling it out as a separate entry would mostly
  duplicate. The decision record's §"The two sentinel errors" carries
  this nuance for journald specifically; future code that adds another
  driver-level guard can read it there.

- **"Splice fixed argv tokens into the literal, don't `append`"** — true
  observation about `cli_driver.go` discipline, but it's a one-file
  micro-pattern with no obvious cross-cutting reuse. Kevlin §2 named it
  ("Pattern A: unconditional fixed flag in the literal"); the
  `internal/dockerdrv/cli_driver.go` source IS the canonical reference.
  No file needed.

## Voice and bar

Matched neighbouring `_ai/*.md` voice: dense one-line summary in `MEMORY.md`,
title-as-rule, "live example" section pointing at code + line numbers,
"when to apply" section, "anti-pattern" or "when NOT to apply" section,
"originator" footer citing the task report and reviewer who promoted the
invariant. The four new files are 36–48 lines each — in the same band as
`stderr-substring-canary.md` (27 lines), `cancellation-symmetry-audit.md`
(38 lines), `cleanup-context-discipline.md` (89 lines).

Bar applied: each entry passes "would a future developer be saved
debugging time / prevented from a mistake / shown a non-obvious approach."
Entries below that bar (the four bullets in the previous section) stayed
out.

## Hand-off to Andy (STEP 4b)

This task did not surface a "user wanted X but we delivered Y" mismatch.
Reports 01 (user request) and 10 (Don's final check) align on outcome —
the user asked for `--log-driver=journald --log-opt tag=…` on every
container with `decloud logs` and cross-redeploy history preserved, and
that is exactly what shipped.

The four-iteration plan-review cycle (Don → Joel → Linus → Linus rev 2)
caught four substantive issues (integration test fixture miss, regex-is-
help-string-not-enforced, slash-rejection invariant promotion, caddy
tag-literal test). Linus's `12-linus-final-check.md` noted that the
iteration depth was higher than the change size warranted but corrected
itself — "process worked" — so no agent-instruction tightening is
indicated. Andy's call.

## Hand-off to STEP 4c (squash-merge)

Branch `task/journald-log-driver` is ready. Suggested commit title:
`feat(dockerdrv): always emit --log-driver=journald with per-service tag`.
Body should mention the two new sentinels, the `Service` field, and the
`docker start` invariant, with cross-links to `_ai/decisions/journald-log-driver.md`
and the deferred `_ai/m1x-backlog.md` item 12 (`decloud logs --history`).
