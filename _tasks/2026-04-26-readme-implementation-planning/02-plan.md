# Plan: Decomposing the README into Shippable Milestones

**Author:** Don Melton (tech lead)
**Status:** Draft for Joel/Linus
**Scope:** Planning only. No code this round.

---

## 1. What the README actually commits us to

Read the README as a contract, not an essay. Stripped to obligations, it specifies:

- **One Linux host.** No multi-node anything. SSH is the only management transport. No daemon, no HTTP API, no listening port for management.
- **Two binaries**, both named `decloud`, distributed as separate packages:
  - **Client** (laptop): package a source tree, push it over SSH to the server, proxy stdio, exit with the remote exit code. That is its entire job.
  - **Server-side** (host): the real CLI — `deploy`, `unregister`, `start`, `stop`, `restart`, `status`, `logs`, `caddy reload`, `backup {run,list,restore}`, `gc`.
- **Two workload types:** services (long-running containers, optional Caddy-routed hostnames) and jobs (containers run by systemd timers, exit on completion).
- **Persistence layout** rooted at `/opt/decloud/{config,secrets,state,logs}` so a single `restic` snapshot covers everything.
- **Caddy** as the only ingress. Caddyfile on disk is the persisted source of truth, regenerated from registrations and reloaded; hot-path upstream swaps go through Caddy's admin API.
- **Deploy lifecycle**: build → start new container on shared Docker network → wait for readiness (HEALTHCHECK or HTTP probe) → flip Caddy upstream via admin API → SIGTERM old, grace period, SIGKILL. Default strategy `blue/green`; `recreate` for services with exclusive resources.
- **Env model**: `env.sh` is sourced at deploy time; resulting environment is captured and persisted with the registration, then injected at `docker run`. Never baked into the image.
- **Mounts**: explicit host→container file/dir mounts, declared at deploy time, mostly read-only secret files (e.g. Google service account JSON).
- **Backups**: nightly `restic` to S3-compatible storage of `/opt/decloud/`, all declared service volumes, and Caddy's ACME data dir. Repo password lives off-host.
- **Image GC**: weekly `docker system prune -a --filter until=168h` and `docker builder prune --filter until=168h`, plus `decloud gc` on demand.

**Things the README explicitly does NOT promise** (Non-Goals section): autoscaling, scale-to-zero, prebuilt-image deploys, web UI, public management API, multi-node, K8s compat, full Cloud Run parity, per-service host systemd units for long-running services. Hold the line on these — every one of them is a tar pit.

## 2. Open Design Questions — which must be resolved before M1

The README leaves two open:

1. **Service metadata representation** — YAML, JSON, shell, or generated unit files.
2. **Whether logs need aggregation beyond `docker logs` / `journalctl`.**

**Verdict for M1:**
- (1) **MUST be resolved before M1 ships.** Every later milestone reads/writes this metadata. CLAUDE.md already mandates TOML for configuration files. Combine that with the README's "understandable over SSH in plain files" principle and the answer is **TOML, one file per service under `/opt/decloud/config/services/<name>.toml`, one file per job under `.../jobs/<name>.toml`**. Joel: write this into the tech plan as a decision, not an open question. Schema versioned with a top-level `schema_version` integer from day one — when (not if) we change the shape, we want the loader to refuse old files cleanly instead of silently mis-parsing.
- (2) **DEFER.** `docker logs` and `journalctl -u <job>` are sufficient for M1 and most likely forever. Revisit only when a real workload demands it.

## 3. The brutal call: what gets built first

**M1 = Server-side service registry + Caddy generation + a single-host service deploy that actually serves traffic.**

In one sentence: **the first thing we ship is `decloud deploy service` end-to-end on the host, executed by a human SSH'd in, building a Docker image from a local source dir, registering it in TOML, generating/reloading the Caddyfile, starting the container on the shared Docker network, and serving real HTTPS traffic on a real hostname.**

### Why this and nothing else

I considered three other "obvious" first picks and rejected each:

1. **"Build the client first."** No. The client is a glorified `tar | ssh`. It is worthless until there is a server-side `decloud` for it to invoke. Building it first means writing transport code with nothing on the other end — pure speculation, guaranteed rework.
2. **"Build the host bootstrap script first."** Tempting because the README's "Initial Direction" lists it as step 1. Wrong as M1. Bootstrap is `apt install docker caddy && systemctl enable decloud.service`. It is a shell script with five lines of substance. It does not exercise any of the design's hard parts and gives us no feedback on whether the registry/Caddy/deploy model actually works. Bootstrap belongs in M2 once we know what we are bootstrapping.
3. **"Build jobs first because they're simpler."** They are simpler — no Caddy, no zero-downtime, no readiness probe — but they are also less load-bearing on the design. Services exercise the registry, the Caddyfile generator, the container lifecycle, the readiness model, the env capture, and the mount model **all at once**. If services work, jobs are a small delta on top. If we built jobs first we would learn nothing about the parts of the design most likely to be wrong.

The first milestone has to be the one that puts the **most design risk under load** while producing something the operator can actually use. That is the service deploy path.

### What "M1 done" means concretely

Acceptance: on a fresh Ubuntu LTS host with Docker, Caddy, and the `decloud` server binary already present (installed by hand for M1 — bootstrap is M2), an operator SSH'd into the host can:

1. `cd` into a directory containing a `Dockerfile` and an `env.sh`.
2. Run `decloud deploy service --name foo --host foo.example.com --port 8080 --readiness http:/healthz` (exact flag shape is Joel's to nail down — keep it short).
3. The command sources `env.sh`, captures the resulting env, builds the image (`decloud-foo:<deploy-id>`), writes `/opt/decloud/config/services/foo.toml`, starts the container on the shared Docker network with the captured env injected, polls the readiness probe, regenerates the Caddyfile from all registered services, runs `caddy reload`, and on success returns 0.
4. `curl https://foo.example.com/` works, served by Caddy with a real Let's Encrypt cert (assuming DNS is pointed correctly — DNS is operator's job per README).
5. `decloud status foo` and `decloud logs foo` return useful output.
6. `decloud stop foo` / `decloud start foo` / `decloud restart foo` / `decloud unregister foo` all behave per their names. `unregister` removes the registration, removes the container, regenerates the Caddyfile, reloads.

### Explicit non-goals for M1

Cut these or we do not ship this year:

- **No client binary.** Operator SSHes in. That is fine — the README explicitly says the server CLI is "equally usable by a human who SSH'd in directly." Ship the client in M3.
- **No zero-downtime deploy.** M1 uses `strategy: recreate` for everything: stop old container, start new, regenerate Caddyfile, reload. The blue/green dance with the Caddy admin API is M4. Yes the README says blue/green is the default; we are explicitly downgrading the default for M1 and will flip it in M4. Document this loudly.
- **No jobs.** No systemd timer generation. Jobs are M5.
- **No backups.** No `restic`. M6.
- **No image GC.** M6.
- **No host bootstrap script.** Manual install for M1. M2.
- **No persistent volumes.** M1 services are stateless. Mount declarations come in M3 alongside the env/secret-file machinery; the Caddy/lifecycle path in M1 should be designed to accommodate them but not implement them.
- **No restart-on-crash supervisor.** Use Docker's own `--restart=unless-stopped`. The README's "host-level Decloud supervisor" can wait — Docker already does the boring 90% of this.

### Risks to flag at M1

- **Caddy admin API vs Caddyfile-on-disk dual sources of truth.** M1 only uses the file + reload path, which sidesteps this. M4 introduces the admin API for hot-path upstream swaps and we must decide then whether the file is regenerated *and* the admin API patched, or whether the file is regenerated and a full reload still happens after a successful admin-API flip. Pick one model in M4 and stick to it.
- **`env.sh` execution is arbitrary shell.** That is by design (per README) but it means we shell out to `bash -c 'set -a; source env.sh; env -0'` or similar and parse NUL-separated output. Joel: nail down the exact capture mechanism in the tech plan, including how multi-line values and unset-vs-empty are handled. This is the kind of detail that bites later if hand-waved now.
- **Schema versioning.** Get this right on day one. Top-level `schema_version = 1` in every TOML file; loader refuses unknown versions.
- **Atomicity of registration writes.** Write to `<name>.toml.tmp` then `rename(2)`. Anything else risks half-written files when the process is killed.
- **One operator assumption.** README explicitly defers per-service deploy locks. Fine for M1 but document the assumption in code comments where it matters.

## 4. Milestone sequence

Each milestone is a shippable increment. Do not start the next one until the previous one has tests and Raymond has updated `_docs/`.

- **M1 — Service deploy MVP (server-side, recreate strategy).** Defined in detail above. The bedrock. Everything else is built on the registry, the Caddyfile generator, and the container lifecycle code that lands here.

- **M2 — Host bootstrap.** A `decloud bootstrap` subcommand (or a separate small script — Joel decides) that on a fresh Ubuntu host: installs Docker and Caddy from their official repos, creates the `/opt/decloud/` directory tree with correct permissions, creates the shared Docker network, installs and enables the single Decloud host systemd unit (which exists mostly to own the supervisor process and to be a stable place for the platform to live across reboots), and verifies the install by deploying a built-in trivial "hello" service. Acceptance: a clean cloud VM is fully usable by the operator within one command + DNS setup.

- **M3 — Client binary + env/mounts/secrets feature-complete.** Two threads, parallelizable:
  - The `decloud` client: package source tree honoring `.dockerignore`, upload via `ssh` (probably `tar | ssh host decloud deploy service --stdin ...`), proxy stdio, exit with remote exit code. Distributed via `go install`.
  - Server-side: full env-capture from `env.sh` (was minimally working in M1, now hardened with edge-case tests), mounted-file declarations with secrets stored under `/opt/decloud/secrets/<service>/` mode 0600, and explicit volume mount declarations in the TOML schema. `schema_version` bumps to 2.

- **M4 — Zero-downtime blue/green deploy via Caddy admin API.** Replace M1's recreate-everything with the real lifecycle: build → start new on shared network with a generated container name → poll readiness (HEALTHCHECK or HTTP probe) → PATCH Caddy admin API to swap upstream → SIGTERM old, grace period, SIGKILL, remove. Add the `strategy = "recreate"` opt-out for services with exclusive resources. Regenerate the on-disk Caddyfile after every deploy so the persisted source of truth stays accurate even though the hot path went through the admin API; on Caddy restart it reloads from disk and converges. Rollback on readiness failure: kill new, leave old.

- **M5 — Jobs.** `decloud deploy job` (client) and the server-side machinery: per-job `<name>.timer` and `<name>.service` systemd units written to `/etc/systemd/system/`, where the `.service` is a oneshot that invokes `decloud run-job <name>` which launches the container with the registered image/env/mounts. `decloud unregister <name>` understands jobs too: stop/disable/remove the timer and service unit, `systemctl daemon-reload`. Status/logs work via `systemctl list-timers` and `journalctl -u`.

- **M6 — Backups + image housekeeping.** Install `restic` during bootstrap (retroactively update M2). Write a `decloud-backup.timer` + `.service` pair that runs `decloud backup run` nightly. `decloud backup run` invokes `restic backup` against `/opt/decloud/` plus all volumes discovered from registrations plus Caddy's ACME data dir; repo URL and password come from a host-side config file (NOT from the backed-up secrets dir — chicken and egg). `decloud backup list` and `decloud backup restore` are thin wrappers over `restic`. Independently: a `decloud-gc.timer` runs `decloud gc` weekly, which shells out to `docker system prune -a --filter until=168h` and `docker builder prune --filter until=168h`. `decloud gc` runs the same on demand.

- **M7 — Operational polish.** Anything that genuinely emerged as missing during M1–M6 use. Candidates, in priority order: per-service deploy lock, crash-budget auto-revert, the Decloud host-level supervisor (if Docker's `--restart=unless-stopped` proved insufficient), better `decloud status` output, optional log aggregation if (1) was wrong and we actually need it. Resist scope creep. Anything that smells like a Non-Goal from the README gets killed on sight.

## 5. What I want from Joel and Linus

- **Joel:** Take M1 and produce the tech plan. Specifically nail down: the exact TOML schema for a service registration (with `schema_version = 1`); the exact `env.sh` capture mechanism (subprocess invocation + parse format); the Caddyfile generation strategy (template? programmatic builder?); the Go package layout for `cmd/decloud/` and the internal packages (`internal/registry`, `internal/caddy`, `internal/docker`, `internal/deploy` are my opening bid — change them if you have a better idea); the test strategy split between unit (Testify + Gomock for the Docker and Caddy interfaces) and a small set of integration tests that actually shell out to real `docker` and `caddy` binaries on a CI runner. Confirm or push back on the M1 cut — if you think something I cut is actually load-bearing for M1, say so with reasoning.
- **Linus:** Tear into the milestone ordering and the M1 scope cut. The big questions for you: is "recreate-only in M1, blue/green in M4" the right trade, or am I building something we throw away? Is the "no client in M1, operator SSHes in" call defensible, or does it create a usability gap that biases M1 into something other than what the eventual workflow needs? Is there a Non-Goal I am sneaking in by accident?

## 6. Things I'm explicitly NOT deciding here

- The exact Cobra command tree shape (Joel).
- Whether the supervisor is its own binary or a long-running mode of `decloud` (defer to M7 if at all).
- The exact health-probe semantics — timeout, interval, max attempts (Joel, in M1 tech plan).
- How the schema-version bump from 1 to 2 in M3 handles M1-era files (Joel, in M3 tech plan; for M1, just make the loader strict).

That is the plan. Build M1. Ship M1. Then we talk about M2.
