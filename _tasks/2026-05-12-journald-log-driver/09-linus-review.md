# Linus — high-level review (journald log driver, post-implementation)

STEP 3d (EXECUTION — high-level architectural review) against branch
`task/journald-log-driver` at HEAD `1d4b4c2`. Diff base: `main`. Read all
seven prior reports (`01-user-request.md` through `08-kevlin-review.md`),
the full plan iteration history including my own two prior plan reviews,
and the live diff (`git diff main...HEAD`) for every changed source and
docs file.

Bottom line up front: this is the rare case where I have nothing
substantive to push back on. The plan was right, the iteration on the
plan was right, the implementation followed the plan, and the tests
defend the right invariants. I will still walk the high-level questions
the user prompt asked, because that is the job and because there are
two or three small things worth flagging for the record even if none of
them are blockers.

---

## 1. Did we solve the user's actual problem?

The user asked for: every Decloud container started with
`--log-driver=journald` tagged by service name, so logs survive
container redeployment, with `decloud logs` continuing to work and
cross-redeploy history queryable via `journalctl`.

Walk the acceptance evidence:

- **Every `docker run` chokepoint emits the flags.** Verified at
  `internal/dockerdrv/cli_driver.go:58-59` (Run) and `:232-233`
  (RunWithOptions). These are the only two functions in the codebase
  that build a `docker run` argv — Don §2.1 enumerated this with a
  grep, and the grep still holds (no third site exists). Every caller
  of either function (`internal/deploy/service.go:246`, `:379`,
  `internal/deploy/lifecycle.go:69`, `internal/caddy/manager.go:127`)
  populates `Service` with the right value. The Caddy container is
  covered (`Service: "caddy"` → tag `decloud/caddy`), matching the
  user's "every Decloud-managed container" intent.

- **`decloud logs` keeps working.** The journald driver is one of
  Docker's dual-read drivers (the daemon shells out to `journalctl`
  internally for `docker logs`). `Driver.Logs` (`cli_driver.go:148-174`)
  is unchanged; it is a thin pass-through to `docker logs`. The user
  experience for `decloud logs <name> [-f] [--tail N]` is byte-identical
  to the json-file shape.

- **Cross-redeploy history.** The tag literal is `decloud/<service>`,
  byte-stored in `CONTAINER_TAG`. After redeploy, the new container
  writes under the same tag; `journalctl CONTAINER_TAG=decloud/<service>`
  shows both ranges. This was independently verified upstream by Kevlin
  against the journalctl manpage in §6.4 of his review.

The user's request is satisfied. The shape we shipped is exactly the
shape the request asked for — no scope creep, no half-measures, no
"we did the easier part and punted the rest."

VERDICT on user intent: SHIPPED.

---

## 2. Is `Service` on `RunRequest`/`RunOptions` the right shape long-term?

This is the architectural question worth examining. The alternative
shapes were:

- A. Derive the tag from `Name` by string-stripping (the smell that
     already existed for the `decloud.service` label).
- B. Pass `Service` as an explicit field (what we shipped).
- C. Defer the field entirely; pass the tag literal directly as
     `Tag string` on the request.

Option A repeats a smell. Option C punts the schema decision to the
caller — and every caller would still need to derive `decloud/<name>`
from a service name they already have, so it would either centralise
on the caller side (worse, because more sites) or push duplication into
each caller. Option B is the only one where the data flow is honest:
callers KNOW the service name, the driver KNOWS the tag schema, and the
two compose by name.

The long-term concern is whether `RunRequest` and `RunOptions` will
ever consolidate (m1x-backlog item 11). If they do, `Service` collapses
to one field on the unified type and the duplication evaporates. Until
then, the two-field duplication costs nothing (the field appears at
literal construction sites; it does not require any cross-type
plumbing). Kevlin's §2 flagged that the message string in
`ErrEmptyService` embeds the phrase "RunRequest/RunOptions" — if item
11 lands, that string drifts. Real, but the cost is one grep at item
11 ship time. Logging that here for the next person.

The decision to kill `strings.TrimPrefix(req.Name, "decloud-")` at the
same time was correct. Two derivations in the same function — one for
the label, one for the tag — would have left the smell in place under
a new name. Now the explicit field feeds both the label and the tag,
and the function reads as "the caller tells us what the service is,
and we use it in two well-named places." That is the right shape.

The forward-compat story for M4 (container renaming to
`decloud-<name>-<deploy-id>`) is documented in
`_ai/decisions/journald-log-driver.md` and cross-referenced in
`_ai/container-naming.md`. The decision record explicitly names the
rule "Service stays the service name, not the container name," which
is the thing that would silently break if a future contributor tried
to re-derive the tag from `Name` after the M4 rename.

VERDICT on architectural shape: RIGHT.

---

## 3. Completeness — anything a user hits in week one?

Things a user might hit:

### 3.1 Host without systemd

If a host has Docker but not systemd (someone running docker via
dockerd directly, or Podman pretending to be Docker), `docker run` with
`--log-driver=journald` fails at container-start with a daemon-side
error. The user sees `docker run: exit status 125; stderr=...` from
the deploy.

Documented? YES. `_docs/install.md` §1 prerequisites carries the new
fourth bullet:

> The Docker daemon must run under systemd. Every container Decloud
> starts uses the journald log driver so logs survive container
> redeployment (see [`usage.md` §6](./usage.md#reading-logs-across-redeploys));
> the daemon needs systemd to write to journald. The default Docker
> Engine install (`systemctl enable --now docker` in §2) satisfies
> this.

Raymond delivered the prerequisite. The wording is precise — names the
mechanism ("journald log driver"), names the constraint ("daemon needs
systemd to write to journald"), and points at the install procedure
that satisfies it. This is exactly the line I asked about; verified at
`_docs/install.md:14`.

### 3.2 Operator running `decloud logs` and being surprised it doesn't show pre-redeploy history

Documented? YES. `_docs/usage.md` §4 carries the annotation:

> Shows logs from the **current** container instance only. The journald
> log driver stores everything in the host journal, so logs from previous
> container generations (before a redeploy or `decloud restart`) are not
> reachable through `decloud logs` — query the host journal directly
> with `journalctl CONTAINER_TAG=decloud/<name>` (see §6).

§6 then carries the full `journalctl` recipe with three concrete
invocations. This is the right level of operator hand-holding — names
the failure mode ("not reachable through `decloud logs`"), names the
fix (`journalctl CONTAINER_TAG=`), and points at the section with the
worked examples.

### 3.3 Operator running `journalctl` with a wildcard expectation

Documented? YES. Raymond's mid-task self-correction caught his initial
`CONTAINER_TAG=~^decloud/` claim and replaced it with the literal
multi-tag-OR form. The current §6 explicitly states:

> `journalctl CONTAINER_TAG=` matches the field value exactly — there
> is no prefix or glob form.

…and shows the operator how to OR multiple tags in one invocation.
This is the kind of "tell the operator the thing that will surprise
them" doc paragraph that prevents support pings.

### 3.4 Service name with unusual characters

Driver-level guards reject empty `Service` and `Service` containing
`/`. The other characters journalctl accepts as exact-match field
values literally (the journal field grammar permits any byte except
NUL and newline). The service-name regex documented in the Cobra help
string (`[a-z][a-z0-9-]{0,38}`) is the in-practice character set; the
driver guard is defense in depth for the specific failure mode this
task introduces (tag-ambiguity).

Anything more would be scope creep. The centralised `ids.ValidateServiceName`
follow-up is the right home for full char-set enforcement, logged
elsewhere as backlog work.

### 3.5 Caddy redeploy losing its logs

`decloud caddy down && up` previously wiped Caddy's logs. Tag now
`decloud/caddy`; logs survive. Test `TestCLIDriver_RunWithOptionsEmitsJournaldFlagsWithCaddyTag`
locks the literal at `tag=decloud/caddy` specifically — without this
test, a future refactor could plausibly hardcode `decloud/foo` in the
shared shape and only the integration smoke would catch it.

VERDICT on completeness: nothing missing for week one. The prerequisite
is in install.md, the operator-facing query recipe is in usage.md §6,
the failure modes are documented inline, the Caddy case is covered.

---

## 4. Maintainability — Kevlin's two observations

### 4.1 `ErrEmptyService` message drift under future consolidation

Kevlin's §2 first observation: the message string embeds
"RunRequest/RunOptions" — when item 11 (consolidate the two types)
ships, the string drifts silently. He flagged it as non-blocking.

Is this real or paranoia? It is real: a future grep of the codebase
for "RunRequest" or "RunOptions" during the consolidation would catch
it (the message is the only string-literal occurrence outside type
definitions and test fixtures). The cost of fixing now is one rewording
sentence to something like "populate Service in the run-request
literal"; the cost of fixing at item 11 ship time is one extra grep.
Either is fine.

The right place to lock this is item 11's checklist, not this task. I
note for the record that item 11's m1x-backlog entry (last updated by
Linus at M2 closeout) does not yet name the message-string drift as
a thing to grep for. Adding a one-line note there would be the cheapest
hedge.

**Options:**
- A. **Defer (as Kevlin proposed).** Cost: one grep at item 11 ship
     time. Risk: the next person doing item 11 might miss it. Mitigation:
     add a one-line note to item 11's backlog entry.
- B. **Reword the message now.** Cost: trivial edit, one re-test. Risk:
     none. Loses the explicitness about which struct types carry the
     `Service` field.

**My take:** Option A with the mitigation (add a note to item 11's
backlog entry naming this grep). The explicitness of the current
message ("populate Service in RunRequest/RunOptions") is genuinely
useful right now and the cost of the future grep is a couple of
seconds. The risk Kevlin flagged is real but the bar for a code change
here is "the future maintainer might forget"; a backlog note covers
that without changing the current behaviour.

**DON:** decide whether to add the one-line note to backlog item 11
in this task (small docs touch) or defer to item 11 itself. Either is
defensible.

### 4.2 Duplicated journald tokens between `Run` and `RunWithOptions`

Kevlin's §2 second observation: the four tokens
`"--log-driver", "journald", "--log-opt", "tag=decloud/" + Service`
appear in both functions. He explicitly recommended NOT extracting a
helper:

> the duplication is two lines per function, four lines total, and a
> helper introduces a layer of indirection that hides the simple shape
> "this is fixed argv, like `--restart`"; (b) the duplication will
> collapse to one site naturally when item 11 (consolidate `Run` +
> `RunWithOptions`) ships.

I concur. This is the right call. Extracting `journaldLogFlags(service
string) []string` would introduce a layer of indirection that hides
the argv literal shape, AND would have to be deleted again at item 11
ship time when there is only one Run path. Premature abstraction here
costs more than it saves. The reading-pattern argument (the four
tokens look like part of the fixed argv, alongside `--name`,
`--network`, `--restart`) is the load-bearing one and would be
weakened by a helper call.

If anyone tries to "tidy this up" before item 11 lands, push back. The
duplication is the right shape until consolidation makes it go away.

VERDICT on maintainability: clean. Both of Kevlin's observations are
non-blocking; one (the message string) has a small mitigation worth
considering, the other (the duplication) is the right shape and should
not be changed.

---

## 5. Are the deferred items the right ones, and are they captured?

Two deferrals:

### 5.1 `decloud logs --history` (m1x-backlog item 12)

Captured? YES, with the specificity I asked for at plan-review time.
The entry names:

- The fix shape (wrap `journalctl CONTAINER_TAG=` behind a CLI flag).
- The design surface (`-f` semantics across journald vs docker logs,
  opt-in vs default, what to do when ranges overlap).
- The integration-test surface (assert a pre-redeploy line is reachable
  through the new flag).
- The originator citations (Don §6 of the plan, Joel §10.9 of the tech
  plan).

This is what a useful backlog entry looks like. A future contributor
reading it has enough to decide whether to take the work without
having to re-read the original task in full.

### 5.2 Centralised `ids.ValidateServiceName` validator

Captured? Yes, in the existing centralised-validation follow-up backlog
position (referenced throughout the plan iteration). The slash-rejection
guard in the driver is defense in depth that becomes a never-fires
backstop when the centralised validator lands.

The interaction between the driver guard and the future centralised
validator is documented in `_ai/decisions/journald-log-driver.md`
under "The two sentinel errors":

> But the documented regex is NOT code-enforced anywhere
> (`internal/registry/store.go` validates non-empty only); the
> driver-level guard locks the journald-tag invariant against future
> regressions without depending on upstream validation we don't
> actually have.

A future contributor reading the decision record will know why both
the driver guard and the centralised validator should coexist.

VERDICT on deferrals: right things deferred, captured with the right
specificity.

---

## 6. "Code shouldn't look like this" — anything Kevlin's pass missed?

I scrutinised the diff with this question front and centre. Things I
looked at:

### 6.1 The guard ordering in `Run` / `RunWithOptions`

Empty-first, slash-second. Kevlin endorsed; Joel's rationale (cheaper
check first; empty is the more common test-fixture failure) is correct.
The order does not matter for behaviour but matters for code review
ergonomics. Reading both functions side by side, the same pattern
appears — same two guards, same order. That parallel reading is what
I want.

### 6.2 The `decloud.service` label change

`args = append(args, "--label", "decloud.service="+req.Service)` at
`cli_driver.go:72`. Previously the value came from
`strings.TrimPrefix(req.Name, "decloud-")`. The new value is identical
in practice (callers populate `Service` from the same source the old
TrimPrefix recovered), but the data flow is now explicit. Good change.
Locked by existing label-assertion tests (the `decloud.service=foo`
expected string in `TestCLIDriver_RunArgsWithEnvSorted` etc.).

### 6.3 The integration test edit

`internal/integration/mount_test.go:69` carries `Service: "mounttest"`.
This is a compile-time fix only — the integration test build was
broken without it. Don's §2.2 sweep missed it; Joel caught it; Kent
applied it; Rob verified the `-tags=integration` compile still passes.
Good chain.

### 6.4 The new sentinel errors

Two distinct sentinels (`ErrEmptyService`, `ErrInvalidService`) with
`errors.Is`-discrimination. The discrimination is locked by
`assert.False(errors.Is(err, <other>))` on every rejection test. If a
future "tidy up" PR folds the two sentinels into one shared error,
those assertions fire. That is exactly the contract I want locked.

### 6.5 The `docker start` invariant test

`TestCLIDriver_StartArgs` extended with two `assert.NotContains`
assertions (one per flag) with the failure-mode message
("HostConfig.LogConfig is sealed at create time"). This defends
against the future drive-by "consistency" refactor that thinks
`docker start` should also know about logging. Defensive against a
regression class that has not happened — but the four-line cost is
low and the regression class is genuine. Worth it.

### 6.6 The fixture sweep across the codebase

Kent and Rob between them updated every `RunRequest{...}` and
`RunOptions{...}` literal in the codebase to carry `Service`. I
ran `grep -rn 'RunRequest{\|RunOptions{' internal/ cmd/` mentally
through Kevlin's §3 audit — every hit is either a production call
site (Service populated), a deliberate rejection-test fixture (Service
empty or slash-containing), or a helper-returning function (Service
populated). Zero zero-value accidents. The sentinel errors actively
defend against the next zero-value accident at test time, which is
the right shape.

### 6.7 Tests-as-theatre check

Looked at each new test under the question "if I deleted the
production code, would this test fail for the right reason?"

- The rejection tests (`§6.2.1-§6.2.4`) fail with "expected error,
  got nil" if the guard is missing entirely, or "expected guard
  before exec" if the guard fires after `cmd.Run` (the
  `assert.Empty(records)` check). Either failure mode names the
  invariant being defended.
- The tag-literal tests (`§6.2.5-§6.2.6`) fail with "--log-driver must
  appear in argv" if the flags are missing, or with the value-mismatch
  message if the tag literal drifts to `decloud-foo` or `{{.Name}}`
  or bare `foo`. All three regressions are named in the assertion
  failure messages.
- The `StartArgs` extension fails with "HostConfig.LogConfig is sealed"
  if a future refactor adds log flags to `docker start`.

These are real behavioural tests, not change-detector tests. Kent's
deliverable is the right shape.

### 6.8 Anything I would have done differently

Honestly, no. The plan-iteration loop converged on the right shape;
the implementation followed the plan with zero deviations; the docs
accurately describe the merged code; the deferrals are specific
enough to act on later. This is what the workflow is supposed to
produce.

The closest thing to a critique I can mount is that the plan-iteration
generated a LOT of words for what is, at the code level, a ~40-line
diff. But the words were not wasted — every iteration found something
substantive (the integration-test miss, the regex-is-help-string-not-
enforced catch, the slash-rejection invariant, the caddy-tag-literal
test promotion). The process did its job.

VERDICT on hidden smells: none found. Kevlin's low-level pass was
thorough; the high-level pass adds no new findings.

---

## 7. Cross-check: is the implementation aligned with the plan as it
ended up?

Walked Don §9 (file list) and Joel §13 against the diff:

| Plan item | File | Status |
| --- | --- | --- |
| Service field on RunRequest | driver.go:47 | DONE |
| Service field on RunOptions | driver.go:94 | DONE |
| ErrEmptyService, ErrInvalidService | driver.go:22-34 | DONE |
| Run guard + flag emission | cli_driver.go:46-83 | DONE |
| RunWithOptions guard + flag emission | cli_driver.go:218-267 | DONE |
| TrimPrefix removal | cli_driver.go:72 | DONE |
| Deploy fresh-deploy Service | service.go:246 | DONE |
| Deploy rollback Service | service.go:379 | DONE |
| Lifecycle absent-branch Service | lifecycle.go:69 | DONE |
| Caddy Service | manager.go:127 | DONE |
| Integration test compile-fix | mount_test.go:69 | DONE |
| 6 new dockerdrv tests + StartArgs extension | cli_driver_test.go | DONE |
| Deploy service Service-field assertions | service_test.go | DONE |
| Lifecycle Service-field assertions | lifecycle_test.go | DONE |
| Caddy Service field on fixture | manager_test.go | DONE |
| usage.md §2/§4/§6 | _docs/usage.md | DONE |
| install.md §1 prerequisite | _docs/install.md:14 | DONE |
| _ai/decisions/journald-log-driver.md | new file | DONE |
| m1x-backlog item 12 | _ai/m1x-backlog.md:115 | DONE |

Every plan item is in the diff. No file was touched outside the plan
file list except the task-report files themselves (expected workflow
churn) and the `_tasks/current` symlink (one-line workflow pointer
update).

`go test -count=1 ./...` — 246 PASS, 0 FAIL.

---

## 8. Summary of options surfaced for Don

Two items for Don to consider, neither blocking:

### Issue: `ErrEmptyService` message drift under future RunRequest/RunOptions consolidation

**Problem:** The message string embeds "RunRequest/RunOptions"; when
m1x-backlog item 11 (consolidate the two types) ships, the string is
stale.

**Impact:** Low. One stale doc string in an internal error nobody sees
unless they write broken test fixtures.

**Options:**
- A. **Defer with mitigation.** Leave message as-is; add a one-line
     note to m1x-backlog item 11 saying "grep for RunRequest/RunOptions
     in error message strings during consolidation." Pros: cheap; cons:
     relies on the next maintainer reading the backlog entry carefully.
- B. **Reword now.** Change message to "populate Service in the
     run-request literal." Pros: no future grep needed; cons: less
     specific about which Go types carry the field today.
- C. **Defer with no mitigation (Kevlin's recommendation).** Pros:
     zero effort; cons: depends on the next maintainer noticing.

**My take:** Option A. The current message is genuinely useful right
now; the mitigation cost is one line in the backlog and removes the
"future maintainer might forget" risk. But this is a small enough
call that B and C are also fine.

**DON:** pick A, B, or C. None block shipping.

### Issue: Plan-iteration verbosity vs code change size

**Problem:** This task produced ~3,500 lines of task-report markdown
for a ~40-line production code diff. (Not a code issue — a workflow
observation.)

**Impact:** None, technically — the workflow is supposed to produce
extensive planning when the change touches a sensitive surface (every
container start). But the cost/benefit ratio of further iteration past
revision 2 of the tech plan would have been negative.

**Options:**
- None. Just an observation. The iteration loop converged at the right
  point. The substantive catches (integration test miss, regex-is-help-
  string catch, slash-rejection invariant, caddy tag-literal test
  promotion) all happened in the iteration phase, which is exactly
  where they should happen.

**My take:** No action. The workflow worked as intended. The reason I
mention this at all is so that future tasks of similar scope can
calibrate — "this is what plan iteration looks like when the change is
small but the surface is sensitive" is a useful reference point.

---

## 9. Verdict

The implementation matches the plan verbatim, the tests defend the
right invariants, the docs accurately describe the merged code, and
the deferrals are captured with enough specificity to be acted on
later. Both of Kevlin's non-blocking observations are real, neither
warrants a change in this task, and the more impactful of the two
(message string drift) has a one-line mitigation Don can choose to
take or defer.

The user's actual problem is solved. Logs survive container
redeployment. `decloud logs` works unchanged. The journald query
recipe is documented in operator-readable form. The Caddy case is
covered. The install prerequisite is on the install page.

This is good work. Nothing more for me to push back on.

## VERDICT: APPROVED
