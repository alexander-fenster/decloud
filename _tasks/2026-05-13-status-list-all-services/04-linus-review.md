# Linus's review — `decloud status` lists every registered service

Task slug: `status-list-all-services`. Reviewed:
- `_tasks/2026-05-13-status-list-all-services/01-user-request.md`
- `_tasks/2026-05-13-status-list-all-services/002-don-plan.md`
- `_tasks/2026-05-13-status-list-all-services/03-tech-plan.md`
- `internal/cli/status.go`, `internal/deploy/lifecycle.go`, `internal/registry/store.go`, `internal/deploy/service.go`, `internal/cli/lifecycle_commands_test.go`, `internal/cli/exit_codes.go`

I read the actual code. The plans are anchored to file/line refs I verified — `Status` is at `internal/deploy/lifecycle.go:91-118`, `List` at `internal/registry/store.go:175-204`, `isCobraUsageError` includes the `"accepts"` substring at `internal/cli/exit_codes.go:84`, etc. Both plans match reality. Good.

## Bottom line up front

This is a small, well-scoped CLI change and the plan reflects that. Don's plan is solid. Joel's tech plan is **better** than Don's plan in the one place they disagree (the error-state enum). I'd ship this.

There are a couple of architectural questions worth a sentence each before approving, but none of them block. Each is a small "decide and move on" item, not a "tear up and replan" item.

## Answering your specific questions

### 1. `cobra.MaximumNArgs(1)` mixing one and zero args in one command — sound, or separate `decloud list`?

**Sound. Keep it as one command.**

Reasons:
- The two modes return the same information at different cardinalities. That is what optional positionals are for. `kubectl get pods` and `kubectl get pods <name>` use this exact pattern; it is the dominant operator-tool convention.
- A separate `decloud list` or `decloud ls` would force the operator to learn a second name for "the same data". Don's `status [name]` is the minimal surface that covers both cases.
- The dispatch in `RunE` is one `if len(args) == 1` — that is not "mixing modes", that is one trivial branch. Spinning a second command for one branch is more surface area, not less.
- Cobra's argument parser already does the work via `MaximumNArgs(1)`, and the existing `isCobraUsageError` matches the `"accepts"` substring it emits on 2+ args (verified at `internal/cli/exit_codes.go:84`). Two-arg invocation routes to exit 2 with no code change. Free.

The shape `decloud status [name]` is right. Approved.

### 2. Sync serial loop, or do we need concurrency / deadline budget?

**Serial is fine. Do not add concurrency.**

Math: `decloud-<name>` containers are inspected via the docker CLI (`internal/dockerdrv/cli_driver.go`). One `docker inspect` round-trip is on the order of 50-200ms on a healthy host. 30 services × 200ms = 6 seconds worst case. That is **at the edge** of operator-acceptable for an interactive command — but `decloud status` is not a hot path and an operator running 30 services is going to wait those few seconds gladly in exchange for a one-pager view.

Concurrency arguments to reject:
- Adding a worker pool means error aggregation, panic propagation, context-cancellation plumbing, and bounded parallelism (you cannot `go d.Status(...)` 30 times — that's 30 simultaneous `docker inspect` invocations and a forkbomb on any tight VM). All of that for a non-hot interactive command is over-engineering.
- The order of `Driver.Inspect` calls in tests is *observable* via gomock — switching to concurrent calls means changing mock setup to `.AnyOrder()` for every test, polluting the test layer with concurrency knowledge it does not need.
- If 30 services makes this command too slow to be useful, that is a docker-CLI throughput problem (we shell out to `docker` instead of using the SDK), and the fix is at that layer, not by piling goroutines on top.

Deadline budget: also no. If `docker inspect` hangs, that is a host-level problem the operator must see (e.g., docker daemon wedged). A blanket timeout would mask the diagnosis. The context passed through from `cmd.Context()` will inherit any SIGINT cancellation, which is the only timeout we need.

**Where this gets revisited:** if and when an operator reports >5 seconds on a real fleet AND the docker-SDK migration is on the roadmap. Until then, serial.

### 3. `Status.ErrorDetail string` — right field, or presentation leakage?

**The field is right, but `string` is the wrong type.**

Don and Joel both chose `string`. Joel's stated reason (§0 of tech plan): "so it serialises cleanly and so we don't accidentally re-route on the wrapped sentinel later." That's a defensible position, but it bothers me. The whole point of typed error chains in this codebase is `errors.Is`/`errors.As` discipline (`_ai/error-wrap-discipline.md`). Stuffing `err.Error()` into a string field is a one-way demotion of diagnostic information — by the time the CLI reads `ErrorDetail`, the operator cannot distinguish `ErrSchemaMismatch` from `ErrPermissionMode` from a docker EOF, except by substring-matching the message text.

But here's the thing: **the operator does not need to distinguish.** That is exactly Joel's argument for collapsing nine error sub-states to one. If the only consumer of `ErrorDetail` is the stderr line `status: <name>: <detail>`, then we have already decided the operator gets the raw text, not a categorised error. In that world, `string` is correct because the only thing we will do with it is `Fprintf`.

The "leak presentation into the deploy package" concern is real but minor:
- `Status` is already a presentation-shaped struct. It carries `ContainerName` (a derived string the CLI prints verbatim), `LastDeployedAt` (a `time.Time` whose presentation format is the CLI's choice, not deploy's). Adding `ErrorDetail` as one more presentation-adjacent field is consistent.
- The field is documented as "NOT rendered in stdout; the CLI prints it to stderr" — that comment is the only thing preventing future drift. It is load-bearing. Joel's plan has this comment; Rob must keep it intact.

**My take:** ship it as `string`. The architectural purity of `error` doesn't earn its keep here.

### 4. Column set (NAME STATE CONTAINER DEPLOY DEPLOYED_AT) — right, missing anything, anything to drop?

**Right set. Don't add fields. Don't drop fields.**

Walking the question:

- **NAME** — required. Sort key, primary identity.
- **STATE** — required. The point of the command.
- **CONTAINER** — useful. An operator running `docker logs <container>` or `docker exec` wants this name without re-deriving it. Slight redundancy with NAME (it is always `decloud-<name>`) but the redundancy is genuinely useful when copy-pasting into `docker ...` commands. Keep.
- **DEPLOY** — required for "is this the current artifact?" triage. When an operator suspects a stale container, the deploy ID is the first thing they check.
- **DEPLOYED_AT** — required. "When did this last move?" is the second thing they check.

Candidates the plans considered and rejected (correctly):
- HOSTS / ROUTES — operator-relevant but variable-length (a service can have 1-N hostnames). Multi-value columns wreck tabwriter alignment. If Caddy routing is the question, `decloud caddy ...` is the answer, not `status`.
- IMAGE — `decloud-<name>:<deployID>` is fully derivable from NAME+DEPLOY. Redundant.
- PORT — internal-only; not actionable from a status row.
- HEALTH (last readiness probe result) — would require persisting probe state. Not free, not in scope.

One thing genuinely worth flagging: **CONTAINER is "always `decloud-<name>`"** — so for services in good states the column is pure redundant noise. The justification (copy-paste convenience) is real but the column eats horizontal budget. I would not fight to remove it, but Don/Joel should know they are paying ~12 characters of width for a noise column on the happy path. If an operator complains later, drop CONTAINER and call it `decloud-<name>` in docs.

**Verdict: ship the five columns as specified.**

### 5. Per-service error policy — error row + listing completes, vs fail-fast?

**Error-row policy is correct. Definitively.**

The whole point of `decloud status` with no args is "show me what is on this host." Aborting the listing because one of fifteen services has a corrupted TOML is *exactly* the operator-hostile behavior that makes ops tools miserable. The operator wants to see fourteen good rows and one row that says "this one is broken, here is why." Failing the whole command and printing nothing to stdout is worse than useless.

Joel locked this in with the seven-test suite at §6.2 — `TestLifecycle_StatusAll_PerServiceLoadErrorIsSynthesised` is the regression lock.

Exit code: 0 on per-service failure. Also correct. The exit code is for the *command*, not for the *services*. The command succeeded (it listed every service). The services that failed surface via stderr + the `error` token in the state column.

One refinement Don and Joel both already cover but I will reinforce: **the concurrent-deploy race** (a service vanishes between `ListNames` and `Load`) gets the row **dropped**, not synthesized. Right call. By the time the operator reads the output, that service is gone — surfacing a transient-error row for it would be misleading.

### 6. Over-/under-engineering smell test

**This plan is on the right side of every line.** I want to flag two real disagreements with Don that Joel correctly already resolved, plus one minor wart:

#### 6.1 Don's nine-state error enum was over-engineered. Joel was right to push back.

Don's `errorState(err)` proposed `error: schema`, `error: permissions`, `error: config`, `error: docker`, `error`. That is FIVE values where one suffices — and Don even acknowledged in §3.3 that "any other error → `error`", admitting the long tail.

Joel collapsed this to one state token (`error`) with the detail on stderr. That is the right answer because:

- **Every error-enum value is a contract surface** — operator scripts will grep on it, docs must list it, Kevlin must hallucination-check it, Raymond must keep it in sync across surfaces. Five values means five surfaces × four contract locations = twenty things to keep aligned. One value means four. 5× tax for **zero new operator capability** is over-engineering by definition.
- **The categorisation buckets are leaky.** `ErrInvalidStrategy` is "config" in Don's mapping, but a schema_version mismatch on the secrets file (also `ErrSchemaMismatch`) is "schema". An operator can't tell from the column whether to look at the TOML or run `chmod`. They go to stderr anyway. So why categorise?
- **YAGNI applies.** No operator has asked to grep on `error: schema`. If one does, we add the enum then — and add it to one place because the implementation is one switch.

I agree with Joel. Lock in **one synthesized state** (`error`).

#### 6.2 `ListNames` factored out of `List` — necessary, not over-engineering.

Don's instinct to factor `ListNames` out and have `StatusAll` use the name-only path (instead of going through `Store.List` which silently drops broken services) is correct and important. This is the architectural keystone of the plan: **the registry's `List` is for the Caddyfile generator (silent drop is right there), the registry's `ListNames` is for the status surface (no drop is right here).** Two readers of the same registry with different failure semantics, by design.

The 6-line refactor is the cheapest possible way to support both contracts. No code duplication. The existing `List` becomes `ListNames + Load loop`, which is what it already was, just with the first half exposed. Approved.

#### 6.3 Minor wart: `tabwriter.Flush` error wrap

Joel wraps `tw.Flush()` with `fmt.Errorf("flushing status table: %w", err)`. The chance of `Flush` failing on stdout is essentially zero (broken pipe is the only real failure mode, and at that point the operator is gone). I wouldn't object to this being a bare `return tw.Flush()`. It is the right *amount* of paranoia, not too much — but it does pad an already-tiny function by a few lines. Not worth changing. Keep as-is.

#### 6.4 Test seam observation (not an objection)

The mock regeneration triple — `MockStore` (add `ListNames`), `MockLifecycle` (add `StatusAll`), `MockServiceDeployer` (no change) — is the standard ripple for adding one method to two interfaces. Joel's §4 walks through it correctly, including the "if any other mock file shows a non-empty diff, **stop**" safety net. Good. That is the kind of "watch for invisible drift" instinct other agents skip.

## Layering check

This was an explicit ask. The plan does **not** violate layering:

- CLI (`internal/cli/status.go`) imports `internal/deploy` (the `Lifecycle` interface). Same as it did before.
- CLI does NOT import `internal/registry`. The status surface routes through `Lifecycle.StatusAll`. Confirmed by reading Joel's §1.4 sketch — no `registry` import in the new `status.go`.
- Deploy (`internal/deploy/lifecycle.go`) imports `internal/registry` for the `Store` interface and `ErrNotFound`/`ErrSecretsMissing` sentinels. Same as the existing `Status` method. No layering change.
- Registry's new `ListNames` lives in `internal/registry/store.go` next to `List`. Same package, same file, same idiom. No new boundary.

Layering passes. The CLI does not reach into `registry` for "what services exist"; it goes through `Lifecycle.StatusAll`, which goes through `Store.ListNames`. Three layers, three responsibilities, one direction.

## What I want the next-pass agents to NOT do

A list of things I will reject in code review if I see them:

1. **Adding a `--all` flag.** The no-arg form IS the `--all` form. Adding a flag is redundancy.
2. **Adding a `--format=json` flag in this task.** Out of scope. If we add it later it is *one* flag added to *one* command, not a flag matrix.
3. **Adding a "no services registered" sentinel line.** Header alone is the right zero-services output. Don't break grep/awk for cuteness.
4. **Splitting the error state into sub-tokens.** Joel collapsed Don's five to one. Don't re-expand it.
5. **Concurrent `Status` calls inside `StatusAll`.** Serial is correct. Do not introduce goroutines.
6. **Adding a HOSTS column.** Variable-arity column. Different command's job.
7. **Removing the silent-drop in `Store.List`.** That contract is depended on by the Caddyfile path. Touching it is a separate task with its own review.
8. **Logging per-service inside `StatusAll`.** Stderr printing is the CLI's job, not the deploy package's. Joel called this out in §1.3.

## Issues that could become risks

None of these block approval. Flagging for awareness.

### Risk A: `runStatusOne` extraction breaks single-service byte-for-byte parity

**Impact**: An operator-script that greps the single-service `status` output stops working silently.

**Mitigation in plan**: Joel pins this with `TestStatus_DelegatesToLifecycleAndPrintsResult` staying green untouched and reaffirms the format string at §1.4. The existing test only checks substring containment, not full-line equality — so there is a small gap where Rob could accidentally change whitespace or field order and the test would still pass. Kent should consider tightening that ONE test to a full-line equality check on the format string, OR adding a new test that asserts the exact `fmt.Sprintf` template result. Joel doesn't propose this; I would.

**Recommendation**: nice-to-have, not required. Don't block on it.

### Risk B: `tw.Flush` order vs. stderr printing

**Impact**: If an operator pipes both stdout and stderr to the same file with `2>&1`, the table rows and the `status: <name>: <detail>` lines may interleave unpredictably.

**Mitigation in plan**: Joel's two-pass approach (§1.4: flush stdout entirely, *then* print to stderr) is deterministic *for the process* — first stdout writes complete, then stderr writes happen. But the OS file-descriptor scheduling can still interleave them at the kernel level for terminal output. This is a real-world ops nit, not a correctness bug.

**Recommendation**: ship as planned. If operators complain that `2>&1` produces interleaved output, we revisit by writing the stderr lines after the row directly inline (one row + maybe-one-stderr-line per service, in a loop). For now Joel's choice is the right default.

### Risk C: `ListNames` returning `(nil, nil)` for missing dir

**Impact**: A misconfigured host where `/opt/decloud/config/services` truly does not exist (vs. is empty) gives the operator a silent "header only" output, identical to a freshly-installed empty host. They cannot tell from the command output that the path is missing.

**Mitigation in plan**: Joel preserves Don's `(nil, nil)` for `fs.ErrNotExist` — matching existing `List` semantics. This is consistent and Don justifies it as "fresh-install host works without a special-case bootstrap step". I agree.

**Recommendation**: keep `(nil, nil)`. If an operator hits the "I deleted the services dir and got silence" footgun, that is a `decloud doctor`/preflight concern, not a status-command concern.

## Verdict

**APPROVED.**

The plan is right-sized. Don's foundation is solid; Joel's pushback on the error enum is correct and lands the plan in a tighter, more contract-friendly place. The layering is clean. The test plan (Kent's scope) covers the operator-visible contracts without descending into change-detector territory. Mocks regenerate from a known-mockgen pattern with a "stop if other mocks moved" safety check.

Things I want Don and Joel to know I agree with explicitly:
- Single command with optional positional, not a separate `list`/`ls`.
- Serial inspect loop, no concurrency.
- One synthesized error state, not nine.
- `ListNames` factored out, with `List`'s silent-drop contract preserved for the Caddyfile path.
- Five columns: NAME STATE CONTAINER DEPLOY DEPLOYED_AT. No more, no less.
- Header on stdout, error detail on stderr. Grep-friendly. Exit 0 on per-service failure.

One thing I want them to think about (not blocking):
- Consider whether `TestStatus_DelegatesToLifecycleAndPrintsResult` should be tightened to assert exact-line single-service output, to lock in byte-for-byte parity during the `runStatusOne` extraction. Substring `Contains` is the current bar; full-line equality would be sturdier.

Rob and Kent: proceed. Don't ship shit.

— Linus
