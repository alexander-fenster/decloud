# Kevlin — low-level review (journald log driver)

STEP 3d (EXECUTION — low-level code review) against branch
`task/journald-log-driver` at HEAD `4a931b8`. Diff base: `main`.

I read all six prior task reports, then audited every changed file against
the plan and the docs. I re-ran `gofmt`, `go vet`, `go vet
-tags=integration`, `go test -count=1 ./...`, and `go build
-tags=integration ./...` — all clean. I also web-fetched the docker journald
driver page and the journalctl manpage to verify the docs claims about
`CONTAINER_TAG=` matching semantics. Findings below.

## 1. Code review — `internal/dockerdrv/driver.go`

The two sentinels live in the same `var (…)` block as the existing
`ErrContainerNotFound` / `ErrNoBridgeIP` and follow the same naming and
phrasing discipline. The doc-comments are wordy but justified — the
`ErrInvalidService` message names the EXACT downstream failure mode
(journalctl prefix-query ambiguity) so the next reader doesn't have to
spelunk. This is the "code as if the next person has my home address"
move; on a sentinel that fires only for a programmer bug at test time
I'd normally want a one-liner, but the trade is correct here because
the rationale is non-obvious from the symptom alone.

The `Service` field is placed between `Name` and `Image` on both
`RunRequest` (driver.go:47) and `RunOptions` (driver.go:94), with an
inline comment that captures both the contract ("populates journald
tag decloud/<Service>") and the invariants ("required, must not
contain '/'"). The comment is data, not narration of code — it
documents the requirement the field exists to satisfy, which is the
kind of comment I do want.

No design smells in this file.

## 2. Code review — `internal/dockerdrv/cli_driver.go`

The implementation matches Joel's spec verbatim:

- `Run` (cli_driver.go:46-83): empty-`Service` guard at :47-49, then
  slash guard at :50-52, both BEFORE the args literal is constructed.
  Journald flags spliced into the literal at :58-59, immediately after
  `--restart`, before any conditional-append section. Pattern A
  ("unconditional fixed flag in the literal") matches the surrounding
  `--name`/`--network`/`--restart` discipline — the four new tokens
  read as "fixed, like `--restart`", not "appended like `--env`".
- `RunWithOptions` (cli_driver.go:220-267): mirror image of the same
  shape, guards at :221-226 and flags at :232-233.
- The `decloud.service` label is now `req.Service` (cli_driver.go:72),
  not `strings.TrimPrefix(req.Name, "decloud-")`. The single chokepoint
  that was the smell is gone.

The argv-order discipline is preserved — the guard runs before the
literal, the literal carries fixed flags, the conditional `append`
section follows. A future maintainer reading either function will see
the same reading pattern in both, which is the whole point of putting
both guards at the very top.

**Residual TrimPrefix fingerprints — checked, none found.**
`grep -rn 'TrimPrefix.*decloud-' internal/ cmd/` returns no matches in
production code. The only remaining hits are inside `_tasks/` markdown
documenting the prior smell, which is the right place for them. The
`strings` import in `cli_driver.go` is still required (used by
`isNotFound`, `ContainerIP`, and the new `strings.ContainsRune` slash
guard).

**Two minor stylistic notes, neither blocking.**

- The sentinel error messages embed the phrase "RunRequest/RunOptions"
  inside `ErrEmptyService`. If we ever rename one of those types
  (e.g. as item 11 of `_ai/m1x-backlog.md` consolidates `Run` and
  `RunWithOptions`), the error string drifts silently. Not worth
  fixing now — the type rename is the item 11 task's surface to
  touch — but the next contributor on item 11 should grep for these
  message strings and update them in the same commit.
- The four journald tokens in both functions are spelled out as
  string literals duplicated between `Run` and `RunWithOptions`. A
  package-level `journaldLogFlags(service string) []string` helper
  could eliminate the duplication. I deliberately do NOT recommend
  this change in this task: (a) the duplication is two lines per
  function, four lines total, and a helper introduces a layer of
  indirection that hides the simple shape "this is fixed argv, like
  `--restart`"; (b) the duplication will collapse to one site
  naturally when item 11 (consolidate `Run` + `RunWithOptions`)
  ships. Pre-emptive abstraction here would be the wrong call —
  premature wrapping costs more than it saves when the underlying
  duplication is two lines and scheduled to evaporate.

## 3. Code review — call sites populating `Service`

All four production call sites populate `Service`:

- `internal/deploy/service.go:246` — fresh deploy: `Service: req.Name`.
- `internal/deploy/service.go:379` — rollback: `Service: prev.Config.Name`.
- `internal/deploy/lifecycle.go:69` — absent-branch re-run: `Service: name`.
- `internal/caddy/manager.go:127` — caddy: `Service: "caddy"`.

`internal/integration/mount_test.go:69` also carries `Service:
"mounttest"` so the integration build still compiles.

I ran `grep -rn 'RunRequest{\|RunOptions{' internal/ cmd/` to look for
any literal that silently zero-values `Service`. Every hit is either:

- one of the four production call sites above (Service populated), OR
- inside `internal/dockerdrv/cli_driver_test.go` test bodies — most
  carry `Service:`; the four that don't (lines 552, 572, 594, 615)
  are the new rejection tests, which DELIBERATELY pass empty or
  slash-containing Service to exercise the guards, OR
- the two helper-returning functions (`caddyRunOptionsFixture` at
  line 702, `expectedCaddyRunOptions` at `manager_test.go:56`),
  both of which carry `Service: "caddy"`.

Zero zero-valued accidents. The new sentinels actively defend against
this class of bug now anyway — any future literal that forgets the
field will fail at the next test run with a legible error.

## 4. Test quality

Kent's six new tests + the `StartArgs` extension are genuinely
behavioural, not change-detector tests. Specifically:

- The rejection tests (§6.2.1–§6.2.4) carry the load-bearing
  `assert.Empty(t, records)` assertion. If a future refactor put the
  guard AFTER `cmd.Run`, this assertion would fail with a clear
  failure mode ("no docker process must be spawned when Service is
  empty (guard fires before cmd.Run)"). That's the assertion that
  distinguishes a real test from mock-theatre.
- The sentinel-discrimination assertions (`assert.True(errors.Is(err,
  X))` + `assert.False(errors.Is(err, Y))`) lock the contract that
  the two sentinels are distinct. If a future "cleanup" PR folds them
  into one, this assertion fails — and that's exactly the contract
  we want locked.
- The tag-literal tests (§6.2.5–§6.2.6) lock the EXACT tokens
  `--log-driver`, `journald`, `--log-opt`, `tag=decloud/<svc>` in
  that contiguous order. The failure messages name the alternative
  failures that this lock prevents ("NOT \"decloud-foo\", NOT
  \"{{.Name}}\", NOT bare \"foo\""), which is exactly the kind of
  invariant-naming I want in an assertion message.
- The `StartArgs` extension adds two `assert.NotContains` — one each
  for `--log-driver` and `--log-opt` — with the failure-mode message
  ("HostConfig.LogConfig is sealed at create time"). A future
  "consistency" refactor that adds log flags to `docker start` argv
  would now fail two assertions named with the invariant they defend.

The `indexOf` helper added for the tag-literal tests is six lines and
package-local. Could have used `slices.Index` (Go 1.21+), but the
hand-rolled version reads identically in this context and keeps the
file's dependency surface unchanged. Fine either way.

**One tiny test-readability suggestion, non-blocking.** The four
rejection test names are very long
(`TestCLIDriver_RunWithOptionsReturnsErrInvalidServiceWhenServiceContainsSlash`).
I'd normally push back on verbosity in test names, but Linus's R2.4.3
already made the case: when a test fails at 11pm, the verbose name +
verbose message tells the on-call exactly what changed. I concur.

The fixture sweeps in §6.1 / §6.3 / §6.4 / §6.5 are mechanical and
correct. I spot-checked `TestCLIDriver_RunArgsWithEnvSorted` and
`TestCLIDriver_RunWithOptionsCaddyShape` — both expected argvs now
include the four journald tokens in the right position, the
hand-typed comments above each test were refreshed to match (the
file preamble at `cli_driver_test.go:1-4` requires the comment to
match the asserted argv; both do).

No change-detector tests. No mock-theatre. No comments-as-symptoms in
new test code. Test surface is tight.

## 5. Architecture / abstraction review

The change introduces NO new abstraction layers. The new shape uses
the existing `RunRequest`/`RunOptions` structs (adds one field per
shape), the existing `var (…)` sentinel block (adds two entries),
the existing argv-literal pattern in both run paths. The driver-level
guard is the right home for the slash-rejection invariant — it's the
last line of defense between in-process data and the on-host tag
literal, and the invariant ("tag is unambiguous for journalctl") is
one the driver INTRODUCES. Upstream validation would be defense in
depth; the driver guard is the load-bearing piece. R2.2.2 of Linus's
review already made this case rigorously.

No unnecessary invention. No reinvented wheels. The existing
patterns (sentinel-error pattern, argv-literal pattern, gomock-based
test fixture) all carry the new functionality with minimum
extension. This is the kind of change where a junior developer
should be able to read the diff once and understand what's
happening.

## 6. Docs accuracy — verified VERY carefully (per CLAUDE.md)

Raymond's report at `07-raymond-docs.md` named six "key claims for
Kevlin to verify." I checked each against the merged code and
against upstream documentation. Results:

### 6.1. `--log-driver=journald --log-opt tag=decloud/<service>` spliced after `--restart` in `Run` (cli_driver.go:58) and `RunWithOptions` (:232)

VERIFIED. `grep -n 'log-driver\|log-opt' internal/dockerdrv/cli_driver.go`
returns exactly those four hits at the claimed lines:

```
cli_driver.go:58		"--log-driver", "journald",
cli_driver.go:59		"--log-opt", "tag=decloud/" + req.Service,
cli_driver.go:232		"--log-driver", "journald",
cli_driver.go:233		"--log-opt", "tag=decloud/" + opts.Service,
```

The argv literal at lines 53-60 in `Run` and 227-234 in `RunWithOptions`
places them immediately after `--restart` and before the conditional
`append`-driven loops. Matches the doc claim exactly.

### 6.2. Tag is `decloud/<service>` for services, `decloud/caddy` for Caddy (manager.go:127)

VERIFIED. `internal/caddy/manager.go:127` reads `Service: "caddy",`
inside the `runOpts()` return literal. Combined with `cli_driver.go:233`
(`"tag=decloud/" + opts.Service`), the caddy tag literal is exactly
`decloud/caddy`. The service tag path is `decloud/<req.Service>` which
matches what the callers pass (`req.Name` in fresh deploy, `prev.Config.Name`
in rollback, `name` in absent-branch re-run). All three of those are
the operator's service name from `--name`. Doc claim accurate.

### 6.3. `decloud logs` shows current container only; cross-redeploy history needs `journalctl CONTAINER_TAG=decloud/<service>`

VERIFIED. `Driver.Logs` (`cli_driver.go:148-174`) is a thin
pass-through to `docker logs <name>` (plus `-f`/`--tail`), targeting
the CURRENT container only. Journald is the only persistent record
across container redeployment, and the documented query shape
`journalctl CONTAINER_TAG=decloud/<service>` is the right one against
the literal `tag=decloud/<service>` the driver emits. Doc claim
accurate.

### 6.4. `CONTAINER_TAG=` is exact-match only (Raymond verified the journalctl man page mid-task)

VERIFIED via the systemd `journalctl` manpage (man7.org mirror; the
freedesktop.org mirror returns 403 from this box, but the man7
copy is the same upstream text). The manpage states explicitly:

> A match is in the format 'FIELD=VALUE', e.g. '_SYSTEMD_UNIT=httpd.service',
> referring to the components of a structured journal entry.

> If two matches apply to the same field, then they are automatically
> matched as alternatives, i.e. the resulting output will show entries
> matching any of the specified matches for the same field.

> If multiple matches are specified matching different fields, the log
> entries are filtered by both, i.e. the resulting output will show
> only entries matching all the specified matches.

The match is a LITERAL field=value comparison — no regex, no prefix,
no glob form. Same-field matches are OR'd; cross-field matches are
AND'd. Raymond's `_docs/usage.md` §6 paragraph at line 299-303 is
correct, including the multi-tag OR-by-repetition example. Raymond's
initial-draft error (claiming `CONTAINER_TAG=~^decloud/` works) was
caught and fixed before commit, and the corrected text matches
upstream behaviour exactly.

I also confirmed via the Docker journald driver page that the
`tag=` log-opt populates `CONTAINER_TAG` and `SYSLOG_IDENTIFIER`
(the Docker docs name both fields explicitly). The Docker docs do
NOT explicitly state that the tag is stored byte-literal including
`/`, but the journald format is byte-oriented (the systemd journal
fields are arbitrary byte sequences below a length cap and forbid
only NUL/newline), and Linus's R2.1.2 and §1.2 of the original
plan-review independently verified this from a live host. The
slash-rejection guard in the driver makes the literal-storage claim
moot for operator-facing behaviour — operators will never see a tag
with more than one slash. Doc claim accurate.

### 6.5. Empty service → `ErrEmptyService`, slash → `ErrInvalidService` (driver.go:22-34), both before exec

VERIFIED. Sentinels declared at `driver.go:22-34` as Raymond claims
(the comment block starts at :22, the `ErrInvalidService` line ends
at :34). Both guards fire at the top of `Run` (cli_driver.go:47-52)
and `RunWithOptions` (:221-226), BEFORE the args literal is built
and BEFORE `cmd.Run` is ever called. The behavioural tests at
§6.2.1–§6.2.4 each assert `assert.Empty(t, records)` — the recording
factory captures every `cmdFactory` invocation, and an empty
records slice proves the guard fired before any docker process was
spawned. The two sentinels are distinct (independent `errors.New`
calls, no shared parent), and the discrimination is locked by
`assert.False(errors.Is(err, <other>))` on every rejection test.
Doc claim accurate.

### 6.6. `decloud logs --history` is deferred (backlog item 12)

VERIFIED. `_ai/m1x-backlog.md` item 12 (lines 115-123) names this
follow-up, cites Don §6 and Joel §10.9, and lays out the fix shape
(flag design, journalctl-vs-docker-logs `-f` semantics, opt-in vs
default, integration test surface). No corresponding code change
in this commit set. The backlog entry is specific enough that a
future maintainer can pick it up without re-deriving the design
surface. Doc claim accurate.

### 6.7. Other doc claims I spot-checked

- **`_ai/decisions/journald-log-driver.md`:18** — "Caddy manager container:
  `decloud/caddy` (hardcoded in `internal/caddy/manager.go:127`)" —
  VERIFIED, `Service: "caddy",` at the named line.
- **`_ai/decisions/journald-log-driver.md`:22** — "emitted in both `Run`
  and `RunWithOptions` (`internal/dockerdrv/cli_driver.go:58` and
  `:232`), spliced immediately after `--restart` and before any
  env/label/port/volume flags" — VERIFIED at both line numbers, both
  positions in the literal.
- **`_ai/decisions/journald-log-driver.md`:34** — "Every caller
  (`internal/deploy/service.go:246` for fresh deploy, `:379` for
  rollback, `internal/deploy/lifecycle.go:69` for absent-branch re-run,
  `internal/caddy/manager.go:127` for Caddy)" — VERIFIED, all four
  line numbers carry the `Service:` field.
- **`_ai/decisions/journald-log-driver.md`:40** — "`internal/dockerdrv/driver.go:22-34`
  declares" — VERIFIED.
- **`_ai/decisions/journald-log-driver.md`:47** — "the documented regex
  `[a-z][a-z0-9-]{0,38}` in `internal/cli/deploy_service.go:57`" —
  VERIFIED; the regex appears in the Cobra help string at that line,
  and is NOT enforced anywhere else in code (the `validateForSave`
  check in `internal/registry/store.go:206-226` only checks
  non-empty, as the decision record states).
- **`_docs/usage.md`:164** — step-4 sentence: tags exactly match the
  emitted argv. VERIFIED.
- **`_docs/usage.md`:195** — `decloud logs` annotation describes the
  exact behaviour of `Driver.Logs` (`cli_driver.go:148-174`).
  VERIFIED.
- **`_docs/usage.md`:299-303** — same-field OR'd, cross-field AND'd,
  no regex form. VERIFIED against the journalctl manpage (see §6.4
  above).
- **`_docs/install.md`:14** — "The Docker daemon must run under systemd.
  Every container Decloud starts uses the journald log driver so logs
  survive container redeployment ...". VERIFIED against the
  decision record and the code.
- **`_ai/m1x-backlog.md`:115-123** — item 12 cross-references both
  `_tasks/2026-05-12-journald-log-driver/02-plan.md` and
  `03-tech-plan.md` for originator citations. VERIFIED.

No hallucinated field names, no hallucinated line numbers, no
hallucinated upstream behaviour, no example code that doesn't match
the real API. Raymond's mid-task catch of his own
`CONTAINER_TAG=~^decloud/` error (which he documents in
`07-raymond-docs.md`'s "Accuracy notes for Kevlin") is exactly the
self-correction shape that prevents this class of bug from reaching
the docs in the first place. The docs are tight.

## 7. gofmt / go vet / go test

- `gofmt -l .` — clean.
- `go vet ./...` — clean.
- `go vet -tags=integration ./...` — clean.
- `go build -tags=integration ./...` — clean.
- `go test -count=1 ./...` — 246 PASS, 0 FAIL (all packages including
  the six new driver-level tests).
- `go test -count=1 -v ./internal/dockerdrv/` — every test in the
  package passes, including all six new tests by name
  (`TestCLIDriver_RunReturnsErrEmptyServiceWhenServiceIsEmpty`,
  `…RunWithOptionsReturnsErrEmptyService…`,
  `…RunReturnsErrInvalidService…`,
  `…RunWithOptionsReturnsErrInvalidService…`,
  `…RunEmitsJournaldFlagsWithSlashTagLiteral`,
  `…RunWithOptionsEmitsJournaldFlagsWithCaddyTag`).

## 8. Forgotten / out-of-scope checks

- **Task scope coverage.** Don's §9 file list and Joel's §13 enumerate
  the exact files this task should touch. Every file listed is
  changed in the diff; no file outside the listed set is changed
  except the task-report files themselves. The `_tasks/current`
  pointer update (1 line) is expected workflow churn, not a code
  change.
- **API changes.** No public API surface change to the CLI. The
  `Driver` interface signature is unchanged (only field additions
  to existing struct types, which Joel §5.6 predicted would not
  affect the mock generation; Rob confirmed `go generate ./...`
  produces no diff). Documentation surface in `_docs/` was updated
  consistently and accurately (see §6 above).
- **TODOs / debug code / skipped tests / commented assertions.**
  None introduced. `grep -rn 'TODO\|FIXME\|XXX' internal/dockerdrv/`
  returns no hits in the new code.
- **Test consolidation.** The six new rejection/tag-literal tests
  have meaningfully different assertion shapes (sentinel-A vs
  sentinel-B, `Run` vs `RunWithOptions`, tag-literal `Run` vs
  tag-literal caddy). Consolidating any two of them would lose the
  specific assertion-message-names-the-invariant signal that makes
  each test individually load-bearing. No premature consolidation.
- **Helper duplication.** The `indexOf` helper is six lines and
  package-local. Not worth promoting to a shared testing package
  for two callers in one file.

## 9. Verdict

The implementation matches the plan verbatim. The guards fire in
the right order, in the right place, with distinct sentinels that
tests can `errors.Is`-discriminate. The journald flags are spliced
into the right position in both `Run` and `RunWithOptions`, the
TrimPrefix smell is killed at the only site it lived, and every
production call site populates `Service`. The new tests are
behavioural and load-bearing, not mock-theatre or change-detector.
The docs accurately describe the merged code and the upstream
journalctl behaviour, with no hallucinated field names or line
numbers — Raymond's mid-task self-correction on the
`CONTAINER_TAG=~^decloud/` regex error is the kind of doc-step
quality I want to see.

`gofmt`, `go vet`, `go vet -tags=integration`, and
`go test -count=1 ./...` all clean.

No design smells. No unnecessary invention. No comments-as-symptoms.
No duplication worth fixing in this task (the two-site journald
literal duplication collapses naturally under backlog item 11).

## VERDICT: APPROVED
