# M1 Execution Plan — v2

**Author:** Don Melton (tech lead)
**Status:** Standalone revision of `02-plan.md` after Linus's `04-linus-review.md` REQUESTED REVISIONS. This file replaces `02-plan.md` for execution purposes; `02-plan.md` remains as history. Joel revises `03-tech-plan.md` next based on this.
**Scope:** Same as v1 — plan the EXECUTION of M1. Cite, don't restate, the prior planning artifacts (`_tasks/2026-04-26-readme-implementation-planning/06-tech-plan-v2.md` is canonical for M1 type shapes and behavior).

---

## 0. Changes from v1 (so Joel can diff)

For every change, I name what moved and why. Ten items, in the order Linus listed them.

**Blocking fixes (Linus's holes):**

1. **Lifecycle command scope decided in §3.1 (NEW).** v1 §6 DONE-criterion #6 implicitly required `unregister/start/stop/restart/status/logs/caddy reload` to "behave per their names" but specified no behavior. v2 keeps ALL of them in M1 and requires Joel's tech-plan §9.6 (NEW) to specify each. This is the Hole #1 blocker.
2. **Caddyfile pre-validation chosen in §3.2 (NEW).** v1 had no recovery story for `caddy reload` failure leaving an unrecoverable on-disk Caddyfile. v2 mandates `caddy validate` on the tmp file BEFORE atomic-rename in step 8b, plus the documented recovery procedure as backstop. This is the Hole #2 blocker.
3. **Readiness probe transport chosen in §3.3 (NEW).** v1 inherited Joel's three-options-no-decision §9.4. v2 picks Linus's Option D: probe from the host using `docker inspect`-derived bridge IP. No third-party image. Joel rewrites §9.4 around `Driver.ContainerIP(ctx, name)` plus a plain `httpProbe`. Linus's Option B (`docker exec`) is rejected for reasons in §3.3.
4. **CI vs. handoff receipts decided in §3.4 (NEW).** v1 deferred CI without specifying the consolation prize. v2 confirms defer (Linus's Option C / "consolation prize") and DEFINES the receipt format Rob attaches to his step-3b report. CI itself is a one-line follow-up task post-M1 once the maintainer confirms the GitHub-hosting question.

**Non-blocking fixes (also from Linus's review body):**

5. **`Store.RollbackPartialCreate` → `DeleteOrphanConfig`.** Rename in implementation per Linus answer #1. Documented in §4.1.
6. **`state/deploys/<name>/<deploy-id>/source.tar.gz` deferral made explicit in §4.2 (NEW row in deferrals table).** Linus Hole #3.
7. **Three explicit envcap edge-case test names added in §4.3 (NEW).** Linus Hole #4 → §13.2 of Joel's tech plan must list `TestEnvcap_SetAOff_VariablesDropped`, `TestEnvcap_ArrayDeclaration_OnlyFirstElementCaptured`, `TestEnvcap_ReadonlyConflict_FailsWithSetE` by name, not by table reference.
8. **`deployerFactory` parallel-safety note added in §4.4 (NEW).** Linus Hole #5. v2 picks Option A (accept package-global, document constraint with comment, never `t.Parallel()` in `internal/cli` tests). Functional-options refactor deferred — small enough win, big enough churn.
9. **No-port-publish docs paragraph added to §2.2.2 (UPDATED).** Linus answer #3. Raymond's `_docs/operator/usage.md` §6 ("Common errors") gains a `docker exec` debug paragraph explaining why ports are not published and how to probe directly.
10. **No-license-yet sentence added to §2.2.1 (UPDATED).** Linus answer #5. Raymond's `_docs/operator/installation.md` gets one sentence noting the repo has no LICENSE yet and the operator should ask before redistribution.

Mockgen layout (Linus answer #4) — accept Joel's asymmetry, no v2 change. Already documented; the asymmetry comment in `internal/cli/mocks/mock_deployer.go`'s generated header covers the future-maintainer surprise. Recorded here for completeness.

Everything in v1 not contradicted by the above STANDS. The §1 scope citation, the §2 user constraints, the §4 sequencing, the §5 Knuth-risk areas, the §6 DONE criteria (with two updates noted in §6 below), the §7 Joel-decisions, and the §8 final word all carry forward.

---

## 1. M1 scope (cited, not repeated)

The contract is in `_tasks/2026-04-26-readme-implementation-planning/06-tech-plan-v2.md` §1, structured per package in §2.2, with:

- Type shapes and persistence rules in §4 (`ServiceConfig`/`ServiceSecrets`/`Service`, two-file split, atomic per-file writes, save/delete ordering).
- Portable env capture in §3 (`compgen -e` + `printf '\0'`; `_ai/envcap-portable-bash.md` is the canonical short-form reference).
- CLI surface in §6 (flags, exit codes, partial-failure behavior).
- Caddyfile generator + first-deploy stub in §7.
- Schema versioning in §5 (stays at 1 across M1 and M3; bump only on semantic break).
- Operational deliverables (`go.mod`, LICENSE, `slog`) in §10.
- Test plan in §12.

Don's plan-v2 §7.1 acceptance criteria still bind. The two user-driven changes from v1 §3 (unit-tests-only; docs as deliverables) still apply, with the §3 additions in this v2 layered on top.

---

## 2. The two user constraints (load-bearing, unchanged from v1)

### 2.1 Unit tests only — integration tests deferred

The user said "I will test it on a real system after M1 is done." That moves Joel's tech plan §12.2 (`internal/dockerdrv/integration_test.go`, `internal/caddy/integration_test.go`, `internal/deploy/integration_test.go`, all `-tags integration`) **out of M1 execution**. Recorded in `_ai/decisions/m1-test-strategy.md` so future Don knows why this was skipped (it's not laziness; it's an explicit user directive).

What stays:

- All §12.1 unit tests with Testify + Gomock per CLAUDE.md item 4.
- The `internal/envcap` tests run against the real `/bin/bash` on the maintainer's box — these ARE unit tests (`go test ./...` runs them with no extra tags). The portability fix from plan-v2 §4.1 is the whole point; we're not skipping these. macOS bash 3.2 stays in the loop.

What changes about coverage discipline because integration is gone:

- **`internal/dockerdrv`** — Joel's §12.1 already lists "argument-construction tests" with an injectable `exec.Command` factory. Without integration, that factory MUST exist and MUST be exercised so we have evidence that `Build`/`Run`/`Stop`/`Remove`/`Inspect`/`Logs`/`ContainerIP` produce the expected `exec.Cmd`. Rob: don't half-mock this. (Note `ContainerIP` is new per §3.3.)
- **`internal/caddy`** — without `caddy validate` in the loop, the generator's correctness is asserted by golden-string equality on a small set of canonical inputs (one host, multi-host, zero hosts dropped, empty input → just the stub). v2 ALSO requires the pre-validation invocation (`caddy validate --config <tmpfile>`) to be exercised by a unit test using a recording cmdFactory — see §3.2.
- **`internal/deploy`** — every step of §6.6's sequence (1–8) gets a unit test using Gomock's `Store`/`Capturer`/`Driver`/`Generator`/`Reloader`. One happy-path test, one test per failure branch. The recoverable "config without secrets" path (Joel §4.5) needs an explicit test. The "step 7 mid-write rollback" path (config wrote, secrets failed → roll the new container back) needs an explicit test.
- **`internal/deploy` lifecycle** — per §3.1, every Lifecycle method gets at minimum one happy-path and one failure-mode test using the same Gomock dependencies.

### 2.2 Installation instructions + short usage docs ARE deliverables

Raymond produces TWO operator-facing docs as M1 deliverables. Both go under `_docs/` per CLAUDE.md.

#### 2.2.1 Installation instructions: `_docs/operator/installation.md`

Honest scope for M1's no-daemon, no-supervisor world:

1. **Prerequisites** — Ubuntu LTS, root access, DNS pointed at the host for any hostnames you'll deploy. (No "decloud daemon" because there isn't one.)
2. **Install Docker** — `apt install docker.io` or the official Docker Engine repo; `systemctl enable --now docker`. Verify: `docker run hello-world`.
3. **Install Caddy** — official repo install per Caddy docs; do NOT enable the default Caddy service. Instead, configure Caddy to run as a systemd service that points at `/opt/declouding/config/caddy/Caddyfile` (the path `decloud` will write to). Provide the exact systemd unit drop-in. Note: Caddy will fail to start until the first `decloud deploy service` runs because the file doesn't exist yet — explain that's expected, the first deploy creates the stub from tech-plan §7.2. **Caddy must be on the operator's PATH for the deployer to invoke `caddy validate` (per §3.2).** The systemd unit drop-in does this implicitly; the install doc explicitly notes that `which caddy` must succeed for `decloud deploy service` to work.
4. **Create the `/opt/declouding/` tree** — exact `mkdir -p` + `chmod` commands for `config/services/`, `config/jobs/`, `config/caddy/`, `secrets/` (mode 0700), `state/deploys/`, `logs/`. The `state/deploys/` tree is created here but populated by no M1 code — this is intentional (see §4.2). The bootstrap script in M2 will eventually automate the whole tree; for M1 the operator runs the commands by hand.
5. **Create the shared Docker network** — `docker network create decloud`. The deployer also self-heals via `docker network inspect ... || docker network create ...` per tech-plan §13.6, but creating it once explicitly during install means the first deploy doesn't appear to "do magic." **Explicit constraint:** the network MUST use the default `bridge` driver (no `--driver` flag) so the host can reach container IPs directly — this is what the readiness probe relies on per §3.3.
6. **Install the `decloud` binary** — `go install github.com/alexander-fenster/decloud/cmd/decloud@latest` OR build locally and copy to `/usr/local/bin/decloud`. Verify: `decloud --help`. **There is no `systemctl enable decloud` step** because M1 has no daemon — `decloud` is a one-shot CLI invoked by the operator over SSH. The README's "Declouding host-level supervisor" is M7 and Docker's `--restart=unless-stopped` covers the boring 90% in the meantime.
7. **One-sentence license note (NEW per Linus answer #5)** — "Note: this repository does not yet declare a license. If you intend to redistribute the binary or use it in a context that requires explicit license grant, ask the maintainer before doing so." Plain English, no FUD, no legalese.
8. **Verify the install** — sanity check `decloud --help` returns the expected subcommands.

That is what "installation" means in M1. Raymond MUST NOT invent a phantom systemd unit for `decloud` itself — there is no decloud process to manage.

#### 2.2.2 Usage documentation: `_docs/operator/usage.md`

End-to-end walkthrough of deploying ONE service. Exact contents:

1. **What you need on the source dir** — a `Dockerfile` and (optionally) an `env.sh`. Show a minimal example of each. The example service is something boring like `nginx:alpine` serving `/usr/share/nginx/html` with one env var swapped at deploy time, so the reader can copy-paste.
2. **The `env.sh` model** — sourced at deploy time in a hermetic bash; whatever it `export`s ends up in the container's environment, never baked into the image. Document the borderline cases Linus flagged in `07-linus-review-v2.md`: `set +a` interaction, arrays, readonly. Mention `_ai/envcap-portable-bash.md` exists for engineers; the user-facing doc just states the rules.
3. **The deploy command** — `decloud deploy service --name foo --host foo.example.com --port 8080 --readiness-path /healthz <source-dir>`. Show the actual flag table (cite tech-plan §6.2) and which are required.
4. **What the deploy actually does** — the §6.6 sequence in plain English (build → stop old → run new → wait readiness → write registration → validate Caddyfile → reload Caddy). Explicitly say "M1 has downtime during recreate; M4 will introduce zero-downtime blue/green." The "validate Caddyfile" step (NEW per §3.2) is named explicitly so the operator knows what to look for in logs if it fails.
5. **Observed result** — what the operator sees on stdout (live `docker build` output), stderr (progress lines), and after success (`docker ps`, `curl https://foo.example.com/`, the two TOML files on disk).
6. **The other subcommands** — short reference for `status`, `logs`, `start`, `stop`, `restart`, `unregister`, `caddy reload`. One-line each with the behavior contract from §3.1. Pointer to `_docs/cli/decloud-deploy-service.md` for the deep flag reference.
7. **Common errors** — the M1 exit codes from tech-plan §6.4. For each non-zero exit code, one sentence of "this is what it means and how to fix it." Include the exact wording for `--mount` rejection, `strategy=blue_green` rejection, schema mismatch, secrets file permission errors.

   **Two new paragraphs in §6 (NEW per Linus answers #3 and §3.2 below):**
   - **"Why ports aren't published, and how to debug a container directly"** — "Decloud deliberately does not publish container ports to the host (`docker run -p ...`). Caddy is the only public ingress, and it reaches each container over the shared `decloud` Docker network. If you need to probe a container directly from the host (for example, the readiness probe is failing and you want to bypass Caddy), use `docker exec <container-name> wget -q -O- http://localhost:<port>/<path>` or `docker exec <container-name> curl -fsS http://localhost:<port>/<path>` — substitute whichever HTTP client your image has. Do not modify the deploy to add `-p` mappings; Caddy's network model is part of M1 by design."
   - **"What to do if `caddy reload` fails (`ExitCaddyReloadFail`)"** — "The deploy validates the new Caddyfile with `caddy validate` BEFORE writing it to disk and reloading (per §3.2 of the M1 plan). If validation fails, the deploy aborts at that step with `ExitCaddyReloadFail` (60), the previous Caddyfile on disk is untouched, and Caddy continues serving the old config. Investigate by running `caddy validate --config <tmp-path-from-the-error-message>`. If validation passed but the actual `caddy reload` failed (rare — usually a runtime issue like a port already bound), the new Caddyfile IS on disk and reflects the new state; Caddy is still serving the old config in memory. To recover: (1) read the Caddy error log, (2) if the failure is in a specific service's stanza, `decloud unregister <name>` will remove that stanza and regenerate, (3) re-run `decloud caddy reload`."

Both docs target an operator who has read the README and is now actually using the system. Concise, complete, no fluff.

---

## 3. The four blocking decisions (the v1 holes Linus flagged)

This section is the substance of the revision. Each subsection makes one decision, gives the reasoning, and tells Joel exactly what to put in `03-tech-plan.md` v2.

### 3.1 Lifecycle command scope: ALL IN M1, fully specified

**Decision: keep all seven Lifecycle methods in M1.** Joel writes a new §9.6 in his tech plan v2 specifying behavior for each.

**Why not defer to M1.5:**
- The README §"Server-side commands" (lines 218–230) lists `unregister`, `start`, `stop`, `restart`, `status`, `logs`, `caddy reload` as the M1 server-side surface. Cutting them from M1 would break the operator contract documented in the README before we ship a single deploy.
- v1 DONE-criterion #6 already required them to "behave per their names." The user expectation is set.
- Without `unregister`, the operator who fat-fingers a deploy has no way to clean up. That's a usability cliff, not an M1.5 nice-to-have.
- Without `status` and `logs`, the operator's ONLY tool for debugging is raw `docker ps` / `docker logs`, which defeats the point of having a CLI.
- The implementations are mechanical once the orchestrator (`*serviceDeployer`) exists. The risk is in unspecified BEHAVIOR (the Hole #1 problem), not in implementation effort.

**What Joel writes in tech-plan §9.6 (one subsection per method):**

For EACH of the seven methods below, Joel specifies: input, output, side-effect ordering, error wrapping (which sentinel from `internal/cli/exit_codes.go`), what rolls back if a step fails, what the operator sees on stdout/stderr, and which unit tests exercise which branch.

**§9.6.1 `Unregister(ctx, name)`** — full removal of a registered service. Sequence:
1. `prev, err := Store.Load(ctx, name)`. `ErrNotFound` → return `ErrNotFound` wrapped (operator sees `ExitConfigError`). `ErrSecretsMissing` → treat as "config-only orphan from a partial deploy"; proceed with steps 2–4 anyway, skipping anything that needs secrets (there's nothing in this path that does).
2. `Driver.Stop(ctx, ContainerName(name), 10*time.Second)`. Ignore "no such container" (idempotent — operator may have removed the container manually).
3. `Driver.Remove(ctx, ContainerName(name))`. Same idempotence on "no such container."
4. `Store.Delete(ctx, name)`. Per `06-tech-plan-v2.md` §4.7, Delete removes secrets FIRST, then config. Forward-only after step 4 — if Delete fails partway, the registry is in a partial state but the container is already gone, so operator runs `decloud unregister` again (idempotent on the container side) until clean.
5. Regenerate Caddyfile from `Store.List(ctx)` (the just-removed service is now absent from the list) → `Generator.Generate(file, inputs)`.
6. **Pre-validate the new Caddyfile** (per §3.2): write to tmp, run `caddy validate --config <tmp>`. If validate fails, log error and return `errCaddyReload` — but the registry is already deleted at this point (forward-only), so the operator must investigate the generator output. This is unlikely (we control the generator) but the failure mode must be honest.
7. Atomic rename tmp → real Caddyfile path.
8. `Reloader.Reload(ctx, paths.CaddyfilePath)`. If reload fails, log warning, return `errCaddyReload`. Do NOT restore — the registry is the source of truth, and re-running `decloud unregister` won't help (already done). Operator follows the recovery path in `_docs/operator/usage.md` §6.

   Rollback semantics: forward-only after step 4. Steps 2–3 are idempotent so partial failure there means re-run; steps 4–8 commit the registry change before the Caddy change, mirroring the deploy orchestrator's "registry-then-Caddy" ordering.

   Tests: `TestLifecycle_UnregisterHappyPath`, `TestLifecycle_UnregisterServiceNotFoundReturnsErrNotFound`, `TestLifecycle_UnregisterContinuesIfContainerAlreadyGone`, `TestLifecycle_UnregisterCaddyReloadFailureReturnsErrCaddyReload`.

**§9.6.2 `Stop(ctx, name)`** — graceful container halt without unregistering.
1. `Driver.Stop(ctx, ContainerName(name), 10*time.Second)`. "No such container" returns `ErrNotFound` (operator's container is already gone — they probably wanted that).
2. **No registry mutation in M1.** Container state is queried live via `docker inspect`, not persisted. `decloud status foo` after `decloud stop foo` reads docker, sees "exited," reports "stopped." No `state.toml` writes for stop/start in M1. Rationale: keeping all runtime state in `docker inspect` is one less thing to keep in sync; M2/M3 can add a persisted state if we find a real need.
3. **No Caddy reload.** A stopped container is unreachable on the network; Caddy will return 502 for routes pointing at it. That's correct user-visible behavior — the operator stopped the service, the public URL fails fast. If we wanted "stopped service returns 503-with-message," that's M5 (planned content), not M1.

   Tests: `TestLifecycle_StopHappyPath`, `TestLifecycle_StopAlreadyStoppedIsIdempotent`, `TestLifecycle_StopServiceNotFoundReturnsErrNotFound`.

**§9.6.3 `Start(ctx, name)`** — restart a stopped container.
1. `prev, err := Store.Load(ctx, name)`. `ErrNotFound` → return wrapped (operator can't `start` what isn't registered). `ErrSecretsMissing` → return wrapped (operator should `decloud unregister` then redeploy; we won't `docker run` without env). 
2. `inspect, err := Driver.Inspect(ctx, ContainerName(name))`.
   - State `running` → no-op, return nil. Idempotent.
   - State `exited` → `Driver.Start(ctx, ContainerName(name))` (NEW driver method, see §3.1.1 below). Container resumes with its previous env baked in (Docker preserves env on stopped containers).
   - State `absent` → the container was removed (perhaps by `docker rm` outside decloud, or by an earlier failed deploy that forgot to restore). Re-`docker run` from `prev.Config.Build.ImageRef` with `prev.Secrets.Env`. If the image is no longer in the local cache (M6 GC removed it), `Driver.Run` fails with the docker error; return wrapped `errRun`. **Start does NOT rebuild from source** — that's `decloud deploy service`'s job. Honest separation.
3. **No Caddy reload.** Caddy already routes to this container's name on the shared network; once it's up, requests start succeeding again.

   Tests: `TestLifecycle_StartFromExited`, `TestLifecycle_StartFromAbsentReRunsContainer`, `TestLifecycle_StartFromRunningIsNoOp`, `TestLifecycle_StartServiceNotFoundReturnsErrNotFound`, `TestLifecycle_StartImageMissingReturnsErrRun`.

   **§3.1.1 New driver method:** `Driver.Start(ctx, containerName) error` — wraps `docker start <name>`. Distinct from `Driver.Run` which is `docker run -d ...`. Joel adds to the §11.1 interface. Test `TestCLIDriver_StartArgs` asserts the args.

**§9.6.4 `Restart(ctx, name)`** — stop-then-start, preserving the container.
1. `Driver.Stop(ctx, ContainerName(name), 10*time.Second)`.
2. `Driver.Start(ctx, ContainerName(name))` per §3.1.1.

   **NOT `docker restart`** — using stop+start lets us reuse the same logic and gives the operator the same 10s grace period as a deploy. `docker restart` is a single command but its grace handling is implicit; ours is explicit.
   
   **NOT recreate.** Operator who wants a fresh container should re-run `decloud deploy service` — the deploy is the recreate path.

   Tests: `TestLifecycle_RestartHappyPath` (assert stop-then-start ordering with `gomock.InOrder`), `TestLifecycle_RestartFromAbsentReturnsErrNotFound` (stop succeeded with idempotence on no-such-container, then start fails because no container; surface that as `ErrNotFound`).

**§9.6.5 `Status(ctx, name) (Status, error)`** — runtime + registry view.
1. `prev, err := Store.Load(ctx, name)`. `ErrNotFound` → return `ErrNotFound`. `ErrSecretsMissing` → return Status with `State: "config-only"` and a `LastDeployedAt: time.Time{}` (operator sees the orphan and can `decloud unregister` to clean up). Other errors propagate.
2. `inspect, err := Driver.Inspect(ctx, ContainerName(name))`. State `absent` → `Status{State: "absent"}`. State `exited` → `Status{State: "stopped"}`. State `running` → `Status{State: "running"}`.
3. Populate Status fields:
   - `Name`: `prev.Config.Name`
   - `ContainerID`: `inspect.ContainerID` (empty if absent)
   - `ContainerName`: `ContainerName(name)`
   - `State`: per step 2
   - `LastDeployID`: parse from `prev.Config.Build.ImageRef` (the tag is the deploy ID per `internal/ids.ImageRef`)
   - `LastDeployedAt`: from `prev.Config.LastDeployedAt` (Joel adds this field to `ServiceConfig` if it's not already there per `06-tech-plan-v2.md` §4.2 — verify and either cite or add)

   Output to stdout: short human-readable single-line format for the default case; `--json` flag for machine-readable. M1 ships only the default; `--json` is M1.5 if anyone asks.

   Tests: `TestLifecycle_StatusRunning`, `TestLifecycle_StatusStopped`, `TestLifecycle_StatusAbsentContainer`, `TestLifecycle_StatusConfigOnlyOrphan`, `TestLifecycle_StatusServiceNotFoundReturnsErrNotFound`.

**§9.6.6 `Logs(ctx, name, opts) error`** — stream container logs.
1. `Driver.Logs(ctx, ContainerName(name), dockerdrv.LogsOptions{Follow: opts.Follow, Tail: opts.Tail, Stdout: os.Stdout, Stderr: os.Stderr})`. Pass-through to `docker logs <name> [-f] [--tail N]`.
2. No registry interaction. If the container doesn't exist, `docker logs` exits non-zero with its own error message; the deployer returns that wrapped as `ErrNotFound` (mapped to `ExitConfigError`).

   Tests: `TestLifecycle_LogsTailN`, `TestLifecycle_LogsFollow`, `TestLifecycle_LogsServiceNotFoundReturnsErrNotFound`.

**§9.6.7 `CaddyReload(ctx)`** — regenerate-from-registry then reload.
1. `services, err := Store.List(ctx)`. Other errors propagate.
2. `Generator.Generate(file, services)` — write to a tmp file.
3. **Pre-validate** per §3.2: `caddy validate --config <tmp>`. Fail → return `errCaddyReload`, leave existing Caddyfile untouched.
4. `caddy.WriteStubIfMissing(paths.CaddyfilePath)` — ensures the stub exists if Caddy hasn't been bootstrapped (idempotent no-op when present).
5. Atomic rename tmp → real Caddyfile path.
6. `Reloader.Reload(ctx, paths.CaddyfilePath)`. Fail → return `errCaddyReload`.

   The `decloud caddy reload` subcommand is the operator's "I edited something out-of-band, regenerate from truth" escape hatch. It's also what `Unregister` calls internally for steps 5–8 (Joel may factor a private helper — `regenerateAndReload(ctx)` — used by both, his call).

   Tests: `TestLifecycle_CaddyReloadHappyPath`, `TestLifecycle_CaddyReloadValidateFailureLeavesOldFileIntact`, `TestLifecycle_CaddyReloadReloadFailureNewFileOnDisk`, `TestLifecycle_CaddyReloadEmptyRegistryWritesStubOnly`.

**Joel's deliverables for §9.6:**
- One subsection per method, contents above.
- Add `Driver.Start` to the §11.1 interface plus its arg-construction test.
- Add `Status` field shape if not already in `06-tech-plan-v2.md` §4.2 — verify (`LastDeployedAt`) and either cite or extend.
- Add `internal/deploy/lifecycle.go` (NEW file) to the §2 file tree, separate from `service.go`. The `*serviceDeployer` struct is shared but the Lifecycle methods cluster cleanly in their own file. Joel: confirm or rebut.
- Extend §13.5 with the test names enumerated above (twenty-some new test cases).

**Open question for Joel that I am NOT deciding here:** does `*serviceDeployer` (singular type) implement BOTH `ServiceDeployer` and `Lifecycle`, or are they two structs sharing `Dependencies`? Joel §9.1 already says "shared state, separate interfaces" — confirm in §9.6 that one struct is the right call. If two structs, justify the duplication.

### 3.2 Caddyfile pre-validation: `caddy validate` BEFORE atomic-rename

**Decision: Linus's Option B (pre-validate) PLUS Option A (document recovery).**

**Implementation:** Joel updates §9.2 step 8b in his tech plan v2 to read:

> 8b. Generate new Caddyfile from `Store.List(ctx)` → `caddy.Generator.Generate(tmpFile, inputs)`. **Then `Reloader.Validate(ctx, tmpFile)` — wraps `caddy validate --config <tmp>`, returns nil on exit 0.** If validate fails, log error including stderr from `caddy validate`, exit `errCaddyReload`. The OLD Caddyfile on disk is untouched. The new container is up and serving (Caddy still routes to it via the OLD Caddyfile, which doesn't reference it; in practice this means the new service is unreachable through Caddy, but the previous services keep working). Then atomic-rename tmp → real Caddyfile path.

**New driver/reloader method:** `caddy.Reloader.Validate(ctx, configPath string) error`. Lives next to `caddy.Reloader.Reload`. Implementation: `caddy validate --config <path>`. Test `TestReloader_InvokesCaddyValidate` asserts the recorded args; `TestReloader_ValidateFailureReturnsError` asserts error wrap on non-zero exit.

**Why not Option C (last-known-good `Caddyfile.lkg` rename):** adds a file lifecycle to manage, has a race condition footprint for concurrent deploys (M3+ might support), and pre-validation closes the same failure window with three lines of code. KISS wins.

**Why I require BOTH validate AND the recovery doc:** validate catches generator bugs and config syntax. It does NOT catch runtime failures during `caddy reload` (e.g., port already bound by another process, certificate provisioning failure, upstream DNS resolution issue at reload time). For those cases, the new Caddyfile IS on disk and reload failed; that's where the doc path in `_docs/operator/usage.md` §6 (per §2.2.2.7 above) catches the operator.

**Caddy must be on PATH** — added to installation doc per §2.2.1 step 3. The `which caddy` check is the operator's own pre-flight; the deployer just invokes `caddy validate` and surfaces the error if it's missing.

**Joel's deliverables for §9.2:**
- Update step 8b text per above.
- Add `Reloader.Validate` to the §7.1 interface citation (note this is an EXTENSION to `06-tech-plan-v2.md` §7.1; flag for Linus).
- Update §13.3 with the two new tests.
- Update the Lifecycle methods §9.6.1 (Unregister) and §9.6.7 (CaddyReload) to also call `Validate` before `Reload` (consistent with deploy orchestrator).

### 3.3 Readiness probe transport: host-side probe via `docker inspect`-derived bridge IP

**Decision: Linus's Option D.** No `curlimages/curl`. No `docker exec` user-image dependency. Probe from the host using the container's bridge IP discovered via `docker inspect`.

**Why not Option B (`docker exec`):** pushes the supply-chain problem onto the user's image. Many minimal images (distroless, scratch, alpine without `apk add curl`) lack any HTTP client. Decloud's contract is "your image, your choice" — we shouldn't make readiness depend on what packages the user installed. The `docker exec` debug paragraph in `_docs/operator/usage.md` §6 (per §2.2.2 above) is for OPERATOR debugging where the user knows their image; it's not for an automated probe in the deploy critical path.

**Why Option D works:**
- The default `bridge` network driver makes container IPs reachable from the host's network namespace on Linux. Docker Desktop on macOS supports it via its VM bridge; the maintainer's dev box (macOS) and the production target (Linux) both work.
- `docker inspect <name> --format '{{ .NetworkSettings.Networks.decloud.IPAddress }}'` returns the IP on the `decloud` network in one call. Deterministic.
- The probe becomes a plain `net/http` GET from the host process — Joel's original `httpProbe` design works as-is, with the URL constructed from the inspect-derived IP instead of from the container name.
- No third-party image, no Docker Hub dependency, no first-use latency, no rate-limiting risk.

**Joel's deliverables for §9.4 rewrite:**

Replace the entire §9.4 contradictory three-options text with this single answer:

> **§9.4 `internal/deploy/readiness.go`**
>
> Probes the new container by HTTP GET from the host process. Container IP discovered via `Driver.ContainerIP(ctx, name) (string, error)` (NEW driver method, see §11.1).
>
> ```go
> type httpProbe struct {
>     client *http.Client
>     driver dockerdrv.Driver  // injected for ContainerIP lookup
> }
>
> func (p *httpProbe) Wait(ctx context.Context, containerName string, spec registry.ReadinessSpec, port int) error {
>     ip, err := p.driver.ContainerIP(ctx, containerName)
>     if err != nil {
>         return fmt.Errorf("readiness: looking up container IP: %w", errReadiness)
>     }
>     url := fmt.Sprintf("http://%s:%d%s", ip, port, spec.HTTPPath)
>     // ... same retry/timeout loop as v1, against the resolved URL ...
> }
> ```
>
> **`Driver.ContainerIP(ctx, name)` implementation:** wraps `docker inspect <name> --format '{{ .NetworkSettings.Networks.decloud.IPAddress }}'`. Returns the trimmed stdout. Empty string → return `ErrNoBridgeIP` (NEW sentinel) so the probe fails fast and the deploy aborts at the readiness step with `errReadiness`. The empty-string case happens when the container isn't yet attached to the `decloud` network (race with `docker run`); the probe's retry loop handles transient cases by re-calling `ContainerIP` on each tick.
>
> **Why not run inspect once and cache the IP:** the IP CAN change if the container restarts (Docker reassigns); for a one-deploy probe within seconds this won't happen, but re-inspecting per-tick is one syscall per second and removes the cache-invalidation question entirely.
>
> **The `OneShotProbe` driver method from v1 §11.1 is REMOVED.** No `curlimages/curl`. No `docker run --rm`. Joel deletes the method from the `Driver` interface and the corresponding test (`TestCLIDriver_OneShotProbeArgs`).

**Joel's tech-plan §11.1 update:**
- ADD `Driver.ContainerIP(ctx, name) (string, error)` to the interface.
- REMOVE `Driver.OneShotProbe(ctx, network, target string) error`.
- ADD test `TestCLIDriver_ContainerIPArgsAndParse` — assert the recorded args (`docker inspect <name> --format ...`) AND that the parser returns the correct IP given a fake stdout.
- ADD test `TestCLIDriver_ContainerIPEmptyReturnsErrNoBridgeIP`.

**Joel's tech-plan §13.5 readiness test rewrite:**
- `TestReadiness_HTTPSuccessReturnsNil` — mock Driver.ContainerIP returns "172.18.0.5", mock HTTP server returns 200, probe succeeds.
- `TestReadiness_ContainerIPLookupFailureReturnsErrReadiness` — mock Driver.ContainerIP returns error, probe fails fast.
- `TestReadiness_HTTPTimeoutReturnsErrReadiness` — Driver returns IP, HTTP server never responds, probe times out.
- `TestReadiness_HTTPRetriesOnTransientFailure` — Driver returns IP, HTTP server returns 503 then 200, probe succeeds on retry.
- `TestReadiness_ContextCancellationStopsProbe` — Driver returns IP, ctx canceled, probe returns ctx.Err().

### 3.4 CI vs. handoff receipts: defer CI, define receipt format

**Decision: defer CI (no `.github/workflows/test.yml` in M1). REQUIRE Rob to attach the receipt format below to his step-3b report.** This is Linus's Option C with the receipt format explicitly nailed down.

**Why defer CI rather than add 20 lines of YAML:**
- I have not confirmed with the maintainer (`alexander-fenster`) that the public GitHub repo at `github.com/alexander-fenster/decloud` is the canonical home and that GitHub Actions is the CI system of choice. The module path implies it; "implies" is not "confirmed." Adding a workflow file before that confirmation embeds a GitHub-specific assumption.
- The maintainer has not asked for CI in any user request to date. CLAUDE.md doesn't mandate it. The user's directive for M1 was "unit tests only, I'll test on a real system."
- Adding CI later is a 30-line task: the workflow file, plus updating `_docs/operator/installation.md` to reference the badge. Trivial. It's the kind of follow-up that takes 5 minutes once the maintainer says "yes, GitHub, yes, Actions."
- Linus's Option C ("defer with mandatory pre-handoff signoff") is the explicit consolation prize. I'm taking it AND defining the receipt format so Rob doesn't have to invent it.

**The receipt format Rob attaches to his step-3b report:**

Rob's report (`_tasks/2026-04-26-m1-implementation/<seq>-rob-implementation.md`) MUST include a section titled "Test pass receipt" containing ALL of the following:

1. **Command run:** the exact command line, e.g. `cd /Users/fenster/dev/declouding && go test ./... -v -count=1 2>&1`.
2. **Go version:** output of `go version` (e.g., `go version go1.22.4 darwin/arm64`).
3. **Host:** `uname -a` output, plus `sw_vers` if macOS (Darwin version is in uname; the macOS marketing version is not).
4. **Bash version on host:** output of `bash --version | head -1` (load-bearing for `internal/envcap` tests; macOS bash 3.2 is what we're claiming portability against).
5. **Docker version:** `docker version --format '{{.Server.Version}}'` (NOT used by unit tests but recorded so future-Don can correlate any test-vs-real-system divergence the user finds).
6. **Caddy version:** `caddy version` (same rationale).
7. **Test output:** the full stdout+stderr of the `go test ./... -v -count=1 2>&1` invocation. Verbatim. No editing. If it's long, that's fine — the report is a markdown file; collapsible-details-disclosure or just inline; reader's choice.
8. **Test summary line:** the final `ok`/`FAIL` summary lines from the test output, extracted to a top-level summary at the start of the receipt section so a reviewer doesn't have to scroll. Example:
   ```
   ok      github.com/alexander-fenster/decloud/internal/registry  0.123s
   ok      github.com/alexander-fenster/decloud/internal/envcap    1.456s
   ok      github.com/alexander-fenster/decloud/internal/caddy     0.089s
   ok      github.com/alexander-fenster/decloud/internal/dockerdrv 0.234s
   ok      github.com/alexander-fenster/decloud/internal/deploy    0.567s
   ok      github.com/alexander-fenster/decloud/internal/cli       0.345s
   ok      github.com/alexander-fenster/decloud/internal/ids       0.012s
   ok      github.com/alexander-fenster/decloud/internal/logging   0.023s
   ok      github.com/alexander-fenster/decloud/internal/config    0.011s
   ```
9. **Vet result:** `go vet ./...` output (must be empty / no findings). One line.
10. **`go generate ./...` was idempotent:** Rob runs `go generate ./...`, then `git status --porcelain`. The receipt includes the porcelain output (must be empty). This is the M2-CI-when-it-arrives canary; we're enforcing it manually now.

**This receipt is a M1 acceptance gate.** Don's PLAN-redux check (CLAUDE.md Step 2-redux) does NOT pass without it. Joel notes this in his §16 handoff section.

**Future:** if the maintainer confirms GitHub-Actions, the M2 task that adds CI also deletes the receipt requirement (or keeps it as a one-line `make test-receipt` for manual runs — maintainer's call).

---

## 4. Non-blocking fixes (the rest of Linus's review body)

### 4.1 `Store.RollbackPartialCreate` → `DeleteOrphanConfig`

Per Linus's first answer: rename in implementation. The interface method becomes `Store.DeleteOrphanConfig(ctx context.Context, name string) error`. Same body. Joel updates §9.5 of the tech plan and the test names (`TestStore_DeleteOrphanConfigRemovesConfig`, `TestStore_DeleteOrphanConfigIsIdempotent`). The deploy orchestrator's call site (§9.2 step 7b) uses the new name. Five-minute rename.

### 4.2 `state/deploys/<name>/<deploy-id>/source.tar.gz` — DEFERRED, made explicit

Add to the v1 §3 deferrals table:

| Deliverable | Decision | Reasoning |
|---|---|---|
| `state/deploys/<name>/<deploy-id>/source.tar.gz` source bundling | NO — DEFER to M2 (or later) | Listed in `_tasks/2026-04-26-readme-implementation-planning/05-plan-v2.md` §6.2 disk layout but not in any acceptance criterion. Implementing it requires a streaming-tarball-write step in the deploy orchestrator (cleanly placed between step 3 and step 4, but new code with new failure modes). M6 backups will sweep an empty tree harmlessly. The directory is created by `_docs/operator/installation.md` step 4 (per §2.2.1 above) so the backup logic later can find it. **Re-evaluate when M6 (backups) is planned**, since that's the first consumer; it may turn out we don't need the tarball at all if we can rebuild from git source URL stored in `ServiceConfig`. |

Joel confirms in his tech-plan v2 that no source-bundling step appears in §9.2; this is intentional per the row above.

### 4.3 Three explicit envcap edge-case test names

Joel's tech-plan §13.2 currently reads "per `06-tech-plan-v2.md` §3.5 (full table — Kent copies the table into test names)." Linus's Hole #4 correctly objects: a chain-of-handoff that can lose entries.

**Joel updates §13.2 to enumerate by name AT MINIMUM these three (in addition to whatever §3.5 lists — Joel verifies overlap):**

- `TestEnvcap_SetAOff_VariablesDropped` — script does `set +a` after the auto-export region; assert the variables exported before it are captured but those after are not.
- `TestEnvcap_ArrayDeclaration_OnlyFirstElementCaptured` — script declares `MY_ARR=(a b c); export MY_ARR`; assert capture behavior is documented (likely "MY_ARR=a"). The point is to LOCK IN the actual behavior with a test, not change it.
- `TestEnvcap_ReadonlyConflict_FailsWithSetE` — script declares `readonly FOO=bar` and then assigns `FOO=baz`; assert capture exits non-zero with a useful error wrapped via `errEnvCapture` (or whatever the chosen sentinel is per §6 of v1).

Joel verifies these aren't duplicates of §3.5 names, and adds whatever else §3.5 enumerates. The point is no implicit handoff: every borderline case has a named test in this plan.

### 4.4 `deployerFactory` parallel-safety: accept package-global, document

Linus's Hole #5 flagged that `var deployerFactory = buildProductionDeployer` is not safe under `t.Parallel()`. v2 picks Option A:

- Joel's §8.2 keeps the package-global pattern (no functional-options refactor).
- Joel ADDS a comment in `internal/cli/deploy_service.go` directly above the `var deployerFactory = ...` line:
  ```go
  // deployerFactory is a package-global test seam. Tests reassign it during setup
  // and restore in teardown. Do NOT call t.Parallel() in any internal/cli test —
  // concurrent reassignment is unsafe. If parallel CLI tests are ever needed,
  // refactor to functional options on NewRootCmd. Same applies to lifecycleFactory.
  ```
- Same comment block above `lifecycleFactory`.
- The functional-options refactor is a deferred clean-up — not a hard M1 requirement, not a hard M2 requirement, queued for "if we ever need parallel CLI tests."

### 4.5 No-port-publish doc paragraph

Already covered in §2.2.2 step 6 above. Raymond writes the "Why ports aren't published" paragraph verbatim from the text in §2.2.2.

### 4.6 No-license-yet sentence

Already covered in §2.2.1 step 7 above. Raymond writes the sentence verbatim from the text in §2.2.1.

### 4.7 Mockgen layout asymmetry

No change. Joel's v1 §5.1 stands. The asymmetric `internal/cli/mocks/mock_deployer.go` placement gets a comment in the generated file's header explaining the deviation:

```go
// MOCK LAYOUT NOTE: This mock lives next to the consumer (internal/cli) rather
// than next to the interface (internal/deploy.ServiceDeployer) because CLI is
// the only consumer. Convention is "mocks live next to interface"; this is the
// documented exception. If a second consumer of ServiceDeployer ever appears,
// move this mock to internal/deploy/mocks/ at that time.
```

Joel adds the comment text to his §5.1 so the mockgen directive's `-package=mocks` setup includes the header comment as a `//go:generate` post-step (likely a separate file in `internal/cli/mocks/` named `doc.go` or appended manually after generation — Joel's call on the mechanism).

---

## 5. Operational deliverables — final decisions (updated from v1 §3)

| Deliverable | Decision | Reasoning |
|---|---|---|
| `go.mod` with `go 1.22` | YES — ship in M1. | Required to even compile. |
| `tools.go` (mockgen pin) | YES. | Necessary for `go generate ./...` reproducibility. |
| `LICENSE` (Apache-2.0) | DEFER. | Maintainer decision, not mine. One-sentence note in installation doc per §2.2.1.7 covers the operator. |
| `.github/workflows/test.yml` | DEFER. | Per §3.4. Replaced in M1 by the receipt format Rob attaches. |
| `slog`-based structured logging | YES. | Tech-plan §9.3 unchanged. `DECLOUD_LOG_TO_STDERR_ONLY=1` test escape hatch mandatory. |
| `_docs/` deliverables | YES. | `_docs/operator/installation.md` and `_docs/operator/usage.md` per §2.2 are first-class. Architecture and CLI reference docs from prior tech-plan §10 also ship. |
| `_ai/decisions/*.md` | MOSTLY EXIST. | Raymond verifies `m1-scope.md`, `secrets-split.md`, `schema-versioning.md`, `envcap-portable-bash.md`, `container-naming.md` align with shipped implementation. Adds `_ai/decisions/m1-test-strategy.md` (NEW) per v1 §3, capturing the unit-tests-only directive. |
| `state/deploys/<name>/<deploy-id>/source.tar.gz` | DEFER. | Per §4.2. Directory is created by installation; no M1 code populates it. |

---

## 6. DONE criteria for M1 (updated from v1 §6)

Don/Joel/Linus sign off in the post-execution PLAN check (Step 2-redux) when ALL of these are true:

1. **`go test ./...` passes on macOS** (no `-tags` flag, `-count=1`). Maintainer's macOS box is the gate. **Receipt per §3.4 is attached to Rob's step-3b report.**
2. **Every package in tech-plan §2.2 exists** with the type shapes and behavior in §4/§6/§7/§9. `cmd/decloud/main.go` builds; `decloud --help` shows the subcommands from §6 and §9.2.
3. **`internal/envcap` tests pass on macOS bash 3.2** without skip — the v2 portability fix is verified by green local test runs. The three explicit edge-case tests from §4.3 are present and green.
4. **The §12.1 unit tests, as amended by §2.1, §3.1, §3.2, §3.3, §4.3**, are all present and green. Coverage is by behavior, not line count — every failure branch in §6.6 has a dedicated test, every Lifecycle method has at least one happy-path and one failure test, the readiness probe has its five tests, the Caddy validate path has its two tests.
5. **The two user-facing docs exist:** `_docs/operator/installation.md` (per §2.2.1, including the no-license sentence and the `caddy validate` PATH note) and `_docs/operator/usage.md` (per §2.2.2, including the `docker exec` debug paragraph and the Caddy reload recovery paragraph).
6. **Lifecycle commands behave per §3.1 (UPDATED):** `decloud unregister foo` removes the registration in §3.3 order, regenerates the Caddyfile, validates and reloads. `decloud start/stop/restart foo` operate per §3.1.2/3/4. `decloud status [foo]` reports per §3.1.5. `decloud logs foo` per §3.1.6. `decloud caddy reload` per §3.1.7.
7. **Caddy pre-validation is wired (NEW):** every code path that writes a Caddyfile (`Deploy`, `Unregister`, `CaddyReload`) calls `Reloader.Validate` BEFORE the atomic-rename. Verified by tests.
8. **Readiness probe uses `Driver.ContainerIP` (NEW):** no `OneShotProbe`, no `curlimages/curl`. The `httpProbe` resolves the container IP via the driver and probes from the host process. Verified by §3.3 tests.
9. **Architecture and CLI docs exist** per tech-plan §10's `_docs/` table.
10. **`_ai/decisions/*.md` files** are aligned with the shipped implementation. New `_ai/decisions/m1-test-strategy.md` captures the unit-tests-only call.
11. **`go.mod` declares `go 1.22`**, deps are `cobra`, `pelletier/go-toml/v2`, `testify`, `go.uber.org/mock`. No Viper. No license file (deferred per §5).
12. **Loader rejection behavior is verified by tests** for: `ErrNotFound`, `ErrSecretsMissing`, `ErrPermissionMode` (file 0644, dir 0755), `ErrSchemaMismatch` (within file and across files), `ErrUnknownField`, `ErrMountsNotSupported`, `ErrInvalidStrategy`.
13. **The recoverable-state contract is verified by tests:** `TestStore_LoadConfigWithoutSecretsReturnsErrSecretsMissing` plus `TestDeploy_StepSevenMidWriteFailureRollsBackContainer` plus `TestDeploy_SaveErrPartialWriteRollsBackAndDeletesOrphanConfig`.
14. **Container naming uses ONE helper** (`internal/ids/ContainerName`) per `_ai/container-naming.md` so M4 changes one function body, not every call site.
15. **`Store` has `DeleteOrphanConfig` (NEW NAME — was `RollbackPartialCreate`):** per §4.1. The deploy orchestrator's step-7b error branch invokes it.
16. **Linus and Kevlin both APPROVE** their respective reviews. Don agrees the work matches plan + tech-plan + this execution plan. Joel agrees no spec gaps were papered over.

If any of 1–16 is missing, M1 is NOT done. Iterate per CLAUDE.md.

---

## 7. Execution sequencing (unchanged from v1 §4)

Per CLAUDE.md, the order of operations remains:

1. **Step 2b — Joel writes execution tech plan v2** (`06-tech-plan.md` in this task dir, OR overwrite `03-tech-plan.md` — Joel picks; my preference: NEW file `06-tech-plan-v2-of-execution.md` so the diff against v1 is a file-diff). Output reflects the §3.1, §3.2, §3.3, §3.4, §4 deltas in this plan.
2. **Step 2c — Linus reviews v2.** Output: `<seq>-linus-review-v2.md`. If still REVISIONS REQUESTED, iterate.
3. **Step 3a — Kent writes failing unit tests** for every package per Joel's revised tech plan. Stubs implementation files with `panic("unimplemented")` bodies so tests compile-and-fail. Reports.
4. **Step 3b — Rob implements** every package against Kent's failing tests. Runs `go generate ./...`. Attaches the §3.4 receipt to his report.
5. **Step 3c — Raymond writes docs** per §2.2 plus `_ai/decisions/m1-test-strategy.md`. Reports.
6. **Steps 3d/3e — Kevlin (low-level) + Linus (high-level) review IN PARALLEL with 3c.** Reports.
7. **Step 2-redux — Don/Joel/Linus PLAN re-check.** Walk down §6 DONE criteria 1–16. If any missing, iterate.
8. **Step 4 — Ward preserves learnings.**

Parallelism: 3c, 3d, 3e all run in parallel after 3b finishes. Do not start 3a until 2c approves v2. Do not start 3b until 3a's tests are written and committed (failing).

---

## 8. Where Knuth might be needed (updated from v1 §5)

Tech-plan §14 already flagged `internal/envcap/capture.go` and `internal/deploy/service.go` as the trickiest pieces. v2 KEEPS those and adjusts:

- **`internal/dockerdrv/cli_driver.go ContainerIP` (NEW per §3.3)** — the inspect-and-parse-stdout pattern is straightforward, but if Docker Desktop on macOS reports a different network shape than Docker on Linux, the parser may need conditional handling. Mitigation: the Format string `{{ .NetworkSettings.Networks.decloud.IPAddress }}` is supported on both platforms per Docker docs. If Rob hits any platform divergence, call Knuth before forking the parser.
- **`internal/deploy` Lifecycle methods (NEW per §3.1)** — none individually are tricky, but the cluster of seven methods sharing `*serviceDeployer` state may grow into a single 600-line file. If it crosses 400 lines or the test setup boilerplate becomes painful, call Knuth before refactoring blindly. (Joel's Open Question in §3.1 — "one struct or two?" — may answer itself if the file gets unwieldy.)
- **`Reloader.Validate` (NEW per §3.2)** — three lines of code. Should not need Knuth.
- Everything from v1 §5 still applies (envcap portability, deploy step-7 mid-write rollback, cmdFactory shape).

---

## 9. What I am explicitly NOT deciding here (for Joel and Linus)

- **Mockgen pinning version + mock layout** — already settled in Joel's v1 §5.1. v2 accepts.
- **Whether `*serviceDeployer` is one struct or two (one for Deploy, one for Lifecycle)** — Joel's call in §9.6, with my preference being one struct (shared deps, separate methods clustered in `service.go` vs `lifecycle.go`).
- **Whether `Lifecycle.Status` ships a `--json` flag in M1** — my call: NO (`--json` is M1.5 if anyone asks); single-line human format is M1.
- **Whether `internal/deploy/lifecycle.go` is a separate file or part of `service.go`** — Joel's call. My preference: separate file.
- **Whether `Driver.Start` (NEW per §3.1.1) lives in `cli_driver.go` next to `Run` or in its own file** — Joel's call. Same file is fine.
- **Whether the `_docs/operator/usage.md` §6 paragraphs count as one section "§6 Common errors" or get split into `§6 Common errors`, `§7 Direct-probing containers`, `§8 Caddy reload recovery`** — Raymond's call on document structure; content is what matters.
- **The exact wording of the no-license sentence in installation.md §7** — Raymond's call on phrasing; meaning is what matters (per §2.2.1.7 above).

---

## 10. Final word

Two real holes were in v1 (Lifecycle behavior, Caddy reload recovery), one decision was three-options-and-no-answer (readiness), and one was deferred-without-consolation-prize (CI). v2 fixes all four with single answers, plus six non-blocking cleanups Linus flagged in the review body. The rest of v1 stands.

Joel's job in tech-plan v2: take the §3.1 (Lifecycle), §3.2 (validate), §3.3 (readiness), §3.4 (receipt) decisions and write the implementation-level §9.6 + §9.2-step-8b update + §9.4 rewrite + §16 handoff update. The §4 non-blocking fixes are interface renames, comment additions, and test-name enumerations — mechanical.

Don't gold-plate. Don't reopen settled questions. Ship M1, hand it to the user with the test receipt, let them break it on a real Linux host, then plan M2 (which probably starts with adding GitHub Actions once the maintainer confirms the GitHub-hosting question).

End of plan v2.
