# Plan v2: Decomposing the README into Shippable Milestones

**Author:** Don Melton (tech lead)
**Status:** Revised draft for Joel/Linus. Supersedes `02-plan.md`.
**Scope:** Planning only. No code this round.

---

## Changes from v1 (read this first, Joel)

For Joel's diff convenience, here is what moved between v1 and v2 — everything else is intentionally preserved.

1. **Secrets are split out of the service config TOML.** v1 put `env` (and implicitly any future secret-file declarations) inside `/opt/declouding/config/services/<name>.toml`. That violates the README's "Handling secrets" contract. v2 mandates a two-file split: a non-secret config TOML in `config/services/<name>.toml` mode 0644, and a secrets TOML in `secrets/<name>/env.toml` mode 0600. Detailed in §3 (Secrets Architecture). Joel: rewrite `registry.Store` to load both and merge in memory, write both atomically. Strategy/build/run/route/readiness stay in config. `Env` map moves to secrets. State stays in config (it is not secret).
2. **`env.sh` capture must be macOS-portable.** v1 implicitly accepted Joel's `env -0` mechanism. That flag is a GNU coreutils extension and is absent from BSD env on macOS, which is the maintainer's dev box. Constraint added to §4 (Open Design Decisions): the capture mechanism must work unmodified on macOS and Linux. Joel picks the exact mechanism in the tech plan; my recommendation in §4 is the `compgen -e` + `printf '%s=%s\0'` bash-builtin approach Linus floated, but I am not nailing the implementation here — that is Joel's call provided the constraint is met.
3. **`schema_version` stays at 1 across M1 and M3.** v1 said M3 bumps to v2. That contradicted Joel's reservation strategy and would have forced migration code into M3 for no benefit. v2 commits to the reservation strategy: M1 declares the full schema shape including fields M1 does not populate (`Mounts`, secret-file declarations); M3 simply starts populating them. The schema_version only bumps when a field's *meaning* changes in a way that breaks old loaders. Joel's strict-mode `DisallowUnknownFields()` plus the schema_version field together give us forward-compat without migration code. Detailed in §5 (Schema Versioning).
4. **`--mount` and reserved-but-unpopulated TOML fields are also rejected by the loader, not just the CLI.** Closes the hand-edit loophole Linus flagged. M1 loader rejects non-empty `Mounts` with the same "M3 only" error as the CLI. Detailed in §7 (M1 Acceptance and Cuts).
5. **Caddy lifecycle on first deploy is documented and the deployer writes a stub Caddyfile if missing.** v1 left the operator to figure out how Caddy boots before the first deploy. v2 mandates that M1's deployer writes an empty/minimal Caddyfile if `/opt/declouding/config/caddy/Caddyfile` does not exist when the first deploy runs, so the operator's `caddy run --config <path>` does not crash on a missing file. Detailed in §7.
6. **Drop the `cache/docker-network-created` sentinel.** Premature optimization Linus correctly flagged. Just call `docker network inspect ... || docker network create ...` every deploy. Removed from the disk layout in §6.
7. **Defer Viper to M2.** v1 implicitly accepted Joel's wire-Viper-now plan. v2 defers it: M1 uses plain Cobra plus `os.Getenv("DECLOUD_ROOT")` for the one knob that matters (`--config-root`). M2 introduces Viper when there is an actual `/etc/decloud/config.toml` to read. Saves ~50 lines of code that would otherwise need maintenance for no current benefit. Detailed in §8.
8. **M3 is acknowledged as a fat milestone, with a planned subdivision into M3a/M3b.** No execution impact on M1. Detailed in §9 (Milestone Sequence).
9. **M4 explicitly owns the M1→M4 container-rename migration.** v1 said "flag it in code"; that is not enough. v2 makes "one-time recreation of all M1-era containers under the new naming convention" an explicit M4 deliverable. Detailed in §9.
10. **Operational deliverables (`go.mod` + Go version pin, LICENSE, CI workflow, `_docs/` and `_ai/` targets, `slog`-based structured logging) are added to M1.** Linus's "missing from the plan" list. None are technically interesting; all are easy to forget. Detailed in §10.

Everything else — the M1 cut, the milestone sequencing, the rejection of "client first" and "jobs first" and "bootstrap first" as M1 picks, the package boundaries Joel proposed, the env.sh capture *design philosophy* (`env -i` + `--noprofile --norc` + `set -a` + baseline-diff + NUL-separated parsing) — all stand. Linus approved the bones; we are fixing the broken ribs.

---

## 1. What the README actually commits us to

Read the README as a contract, not an essay. Stripped to obligations, it specifies:

- **One Linux host.** No multi-node anything. SSH is the only management transport. No daemon, no HTTP API, no listening port for management.
- **Two binaries**, both named `decloud`, distributed as separate packages:
  - **Client** (laptop): package a source tree, push it over SSH to the server, proxy stdio, exit with the remote exit code. That is its entire job.
  - **Server-side** (host): the real CLI — `deploy`, `unregister`, `start`, `stop`, `restart`, `status`, `logs`, `caddy reload`, `backup {run,list,restore}`, `gc`.
- **Two workload types:** services (long-running containers, optional Caddy-routed hostnames) and jobs (containers run by systemd timers, exit on completion).
- **Persistence layout** rooted at `/opt/declouding/{config,secrets,state,logs}` so a single `restic` snapshot covers everything.
- **Caddy** as the only ingress. Caddyfile on disk is the persisted source of truth, regenerated from registrations and reloaded; hot-path upstream swaps go through Caddy's admin API.
- **Deploy lifecycle**: build → start new container on shared Docker network → wait for readiness (HEALTHCHECK or HTTP probe) → flip Caddy upstream via admin API → SIGTERM old, grace period, SIGKILL. Default strategy `blue/green`; `recreate` for services with exclusive resources.
- **Env model**: `env.sh` is sourced at deploy time; resulting environment is captured and persisted with the registration, then injected at `docker run`. Never baked into the image.
- **Secrets architecture (load-bearing for v2):** the README's "Handling secrets" section explicitly says env-vars-from-`env.sh` and any deploy-provided secret files live under `/opt/declouding/secrets/<service>/` with owner-read-only permissions. Secrets and non-secret config are structurally separated on disk, not just logically separated within the same file. This is the README contract that v1 violated; v2 honors it.
- **Mounts**: explicit host→container file/dir mounts, declared at deploy time, mostly read-only secret files (e.g. Google service account JSON).
- **Backups**: nightly `restic` to S3-compatible storage of `/opt/declouding/`, all declared service volumes, and Caddy's ACME data dir. Repo password lives off-host.
- **Image GC**: weekly `docker system prune -a --filter until=168h` and `docker builder prune --filter until=168h`, plus `decloud gc` on demand.

**Things the README explicitly does NOT promise** (Non-Goals section): autoscaling, scale-to-zero, prebuilt-image deploys, web UI, public management API, multi-node, K8s compat, full Cloud Run parity, per-service host systemd units for long-running services. Hold the line on these — every one of them is a tar pit.

## 2. Open Questions left by the README — resolved before M1

The README leaves two open:

1. **Service metadata representation** — YAML, JSON, shell, or generated unit files.
2. **Whether logs need aggregation beyond `docker logs` / `journalctl`.**

**Verdict for M1:**

- (1) **MUST be resolved before M1 ships.** Every later milestone reads/writes this metadata. CLAUDE.md already mandates TOML for configuration files. Combine that with the README's "understandable over SSH in plain files" principle and the answer is **TOML, two files per service** (config + secrets, see §3). One file per job under `config/jobs/<name>.toml` (jobs are M5; we just reserve the directory).
- (2) **DEFER.** `docker logs` and `journalctl -u <job>` are sufficient for M1 and most likely forever. Revisit only when a real workload demands it. M1 still gets structured logging for the *decloud binary itself* (see §10) — that is internal observability, not application log aggregation.

## 3. Secrets Architecture (the v2 fix)

The README's "Handling secrets" section is unambiguous: `env.sh` output and deploy-provided secret files live under `/opt/declouding/secrets/<service>/` with owner-read-only permissions. v1 violated this by stuffing the captured env into `/opt/declouding/config/services/<name>.toml` (mode 0644). v2 fixes this by splitting the registration into two files at write time.

### 3.1 The split

Per service `<name>`:

- **`/opt/declouding/config/services/<name>.toml`** — owner root, mode **0644**, world-readable. Contains:
  - `schema_version`
  - `name`
  - `source` (the absolute source dir path; not secret)
  - `build` (Dockerfile path, image ref; not secret)
  - `run` (network, port, restart policy, mounts — empty in M1; not secret in themselves, since the *paths* are operational metadata, the *contents* of mounted secret files live under `secrets/`)
  - `routes` (hostnames; not secret)
  - `strategy`
  - `readiness`
  - `state` (deploy ID, container ID, container name, last-deployed-at — not secret; useful to inspect without sudo)

- **`/opt/declouding/secrets/<name>/env.toml`** — owner root, mode **0600**. Contains:
  - `schema_version` (must match the config file's; loader enforces)
  - `env` (the captured `env.sh` output, the actual secret-class data)

Future M3 secret-file declarations also live under `secrets/<name>/files/` with mode 0600 on the files themselves and 0700 on the directory. The `mounts` block in the config TOML references those paths; the file *contents* are never in the config TOML.

### 3.2 Why this split, not the alternatives

Linus considered three options; v2 commits to his recommended Option B for the reasons he stated. Reproduced briefly so future readers don't have to chase the review file:

- **Option A (set mode 0600 on the single config TOML):** breaks the README's structural separation; non-secret operational metadata becomes inspectable only by root; future "git-mirror the non-secret config" idea (the README mentions it as out-of-scope-but-easy-to-add) becomes hard because secret and non-secret are mixed in one file.
- **Option C (defer the split to M3 with a schema bump):** ships M1 with a known security regression and forces M3 to do data migration. We are not doing that.
- **Option B (split now):** ~30 extra lines in `registry.Store`. Honors the README. M3 secret-files work is purely additive (just add `secrets/<name>/files/`). Future git-mirror story works. This is the only correct answer.

### 3.3 Loader semantics

`registry.Store.Load(name)`:

1. Read `config/services/<name>.toml` strict-mode (`DisallowUnknownFields`).
2. Read `secrets/<name>/env.toml` strict-mode.
3. Verify `schema_version` matches between the two and equals `CurrentSchemaVersion`. Reject loudly on mismatch.
4. Verify the secrets file is mode 0600 (and the secrets directory is 0700). Refuse to load (and refuse to deploy) if permissions are wrong — we do not silently fix them, because silently fixing permissions hides whatever process broke them.
5. Merge into the in-memory `ServiceSpec`.

`registry.Store.Save(spec)`:

1. Compute the two on-disk representations.
2. Write `config/services/<name>.toml.tmp` mode 0644, then `os.Rename` to the final name.
3. Write `secrets/<name>/env.toml.tmp` mode 0600 (use `os.OpenFile` with explicit perm + `os.Chmod` after rename to be paranoid about umask), then `os.Rename`.
4. Both renames are POSIX-atomic on the same filesystem. The two-file write is *not* atomic across both files — a crash between the two renames leaves an inconsistent registration. The loader detects this (schema_version mismatch or missing secrets file for an existing config file) and reports it; operator's recovery is to re-run the deploy. M1 acceptance includes a unit test for "config exists, secrets missing — load returns an explicit error."

`registry.Store.Delete(name)`:

1. Delete `secrets/<name>/env.toml`.
2. `rmdir` `secrets/<name>` (errors ignored if non-empty — M3 may leave secret files).
3. Delete `config/services/<name>.toml`.

Order matters: delete secrets first so a crash mid-delete leaves config without secrets (loader will surface the inconsistency on next read), not secrets without config (which would be an orphaned secrets file with no way to find it from registration).

### 3.4 Migration impact

Zero. M1 is the first version. Nothing to migrate from.

---

## 4. Open Design Decisions (carried into Joel's tech plan)

These are the things Joel must nail in his revised tech plan. v2 sets the constraints; Joel picks the implementation.

### 4.1 `env.sh` capture mechanism — must be macOS + Linux portable

**Constraint:** The capture mechanism must run unmodified on the maintainer's macOS dev box (Darwin, BSD coreutils, bash 3.2 by default at `/bin/bash`) and on the production Ubuntu LTS host (Linux, GNU coreutils, bash 5+). No GNU-only flags. No Homebrew dependencies. Maintainer should be able to clone the repo and run `go test ./internal/envcap/...` on a stock Mac without installing anything beyond Go and bash.

**Why this matters:** The dev loop is sacred. If the env-capture tests don't run on the maintainer's laptop, the maintainer can't iterate on env-capture changes without pushing to CI for every revision. That is the kind of friction that turns a one-day fix into a one-week fix. We catch this now or we live with it for the life of the project.

**Design philosophy unchanged from v1 / Joel's tech plan:** still `env -i` for hermetic input, still `--noprofile --norc` to skip dotfiles, still `set -a` to capture unexported assignments, still baseline-diff to drop bash internals, still NUL-separated parsing for newline-safe values. **Only the "how do we emit NUL-separated KEY=VALUE" step changes.**

**My recommended mechanism (Joel may pick differently if he can defend an alternative):** use bash builtins, no external `env` invocation:

```bash
set -a
source "$1"
while IFS= read -r name; do
    printf '%s=%s\0' "$name" "${!name}"
done < <(compgen -e)
```

`compgen -e` enumerates exported variables; `printf '\0'` emits NUL terminators; no GNU coreutils involved; works on bash 3.2 and bash 5+. Joel: confirm this works on macOS bash 3.2 specifically (the indirect expansion `${!name}` is bash 2+ and `compgen -e` is bash 2+, so we should be fine, but verify before committing).

**Alternative Joel may consider:** invoke `bash -c '... declare -px'` and parse `declare -p` output in Go. Avoids the bash printf dance. Downside: `declare -p` output format has bash-version variation and is a worse parsing target than NUL-separated KEY=VALUE. I lean against, but Joel decides.

### 4.2 TOML library

Stays at `github.com/pelletier/go-toml/v2` per Joel's v1 reasoning (strict mode, performance, active maintenance, already a Viper dep). No change.

### 4.3 Caddyfile generation strategy

Stays at `text/template` with deterministic output (sort by service name, then by hostname). No change.

### 4.4 Docker invocation

Stays at `os/exec` shelling out to the `docker` CLI for M1, behind the `dockerdrv.Driver` interface. No change. SDK swap is M4-or-later if ever needed.

### 4.5 Caddy reload mechanism

Stays at `caddy reload --config <path>` for M1. Admin API is M4. No change.

---

## 5. Schema Versioning (the v1 contradiction, resolved)

**Decision: M1 writes `schema_version = 1`. M3 also writes `schema_version = 1`. The version only bumps when a field's semantics change in a way that breaks an old loader.**

This resolves the v1 contradiction in favor of Joel's position (reserve fields, no bump) for the reasons Linus articulated and the secrets-split made airtight.

### 5.1 What this means concretely

- **M1 writes the full schema shape.** Including fields M1 does not populate: `Mounts` (always empty in M1), reserved secret-file declarations under `mounts` (always empty in M1), any other field we can confidently predict.
- **M3 starts populating those fields.** No file rewrite. No migration code. An M1-era TOML file loads cleanly in an M3 binary because the shape is identical — only the values differ.
- **The schema_version exists for the case we cannot avoid:** when M-something introduces a field whose *meaning* can't be expressed in the existing shape, or when we discover an M1 design mistake that the strict-mode loader can't gracefully tolerate. That bumps to v2 with explicit migration code at that time. But we do not pre-emptively bump; we bump only when forced.
- **Strict mode is the forward-compat mechanism for everything else.** `pelletier/go-toml/v2`'s `DisallowUnknownFields()` means an old binary reading a file with new fields fails loudly. Combined with `schema_version`, this is belt-and-braces.

### 5.2 Why this works given the secrets split

The biggest argument against Joel's position in v1 was "we can't predict M3's schema." The secrets split (§3) preempts the largest predictable schema disruption: env-leaving-config-going-to-secrets is no longer an M3 schema change, it's an M1 architectural decision. With that out of the way, what remains for M3 — `Mounts` field population, secret-file declarations under mounts — is well-understood from the README and reservable in M1.

### 5.3 Loader rule for M1

The loader accepts exactly `schema_version = 1`. Anything else is a hard error: `"unsupported schema_version N (this binary supports 1); upgrade or downgrade decloud to match the file"`. Both the config TOML and the secrets TOML must declare the same version; mismatch is a hard error.

### 5.4 What if we need to bump anyway?

If during M1 implementation Kent or Rob discovers that the schema fundamentally cannot work as designed and needs a v2, **stop and bring it back to plan.** A schema bump in M1 means we got the design wrong and we need to re-think before shipping. We do not silently introduce migration code mid-milestone.

---

## 6. The brutal call: what gets built first

**M1 = Server-side service registry + Caddy generation + a single-host service deploy that actually serves traffic.**

In one sentence: **the first thing we ship is `decloud deploy service` end-to-end on the host, executed by a human SSH'd in, building a Docker image from a local source dir, registering it as a config TOML and a secrets TOML, generating/reloading the Caddyfile, starting the container on the shared Docker network, and serving real HTTPS traffic on a real hostname.**

### 6.1 Why this and nothing else

Three "obvious" alternatives, rejected:

1. **"Build the client first."** No. The client is a glorified `tar | ssh`. It is worthless until there is a server-side `decloud` for it to invoke. Building it first means writing transport code with nothing on the other end — pure speculation, guaranteed rework.
2. **"Build the host bootstrap script first."** Tempting because the README's "Initial Direction" lists it as step 1. Wrong as M1. Bootstrap is `apt install docker caddy && systemctl enable decloud.service`. It is a shell script with five lines of substance. It does not exercise any of the design's hard parts and gives us no feedback on whether the registry/Caddy/deploy model actually works. Bootstrap belongs in M2 once we know what we are bootstrapping.
3. **"Build jobs first because they're simpler."** They are simpler — no Caddy, no zero-downtime, no readiness probe — but they are also less load-bearing on the design. Services exercise the registry, the Caddyfile generator, the container lifecycle, the readiness model, the env capture, the secrets split, and the mount model **all at once**. If services work, jobs are a small delta on top. If we built jobs first we would learn nothing about the parts of the design most likely to be wrong.

The first milestone has to be the one that puts the **most design risk under load** while producing something the operator can actually use. That is the service deploy path.

### 6.2 Disk layout under `/opt/declouding/` for M1

```
/opt/declouding/
  config/
    services/
      <name>.toml                # mode 0644 — non-secret registration
    jobs/                         # M5 — exists empty in M1
    caddy/
      Caddyfile                   # mode 0644 — generated by decloud
                                  # M1 deployer creates an empty stub on first run if missing
  secrets/
    <name>/
      env.toml                    # mode 0600 — captured env.sh output
                                  # directory mode 0700
                                  # files/ subdirectory reserved for M3 secret files
  state/
    deploys/
      <name>/
        <deploy-id>/
          source.tar.gz           # source bundle at deploy time, for forensics
                                  # auto-included in M6 backups
  logs/
    decloud.log                   # decloud binary's structured (slog/JSON) log
                                  # rotation via logrotate is M2 bootstrap's job
```

**Removed from v1:** the `cache/docker-network-created` sentinel file. Premature optimization. Just call `docker network inspect decloud >/dev/null 2>&1 || docker network create decloud` every deploy; sub-100ms, idempotent, self-healing.

**Permissions are set at directory creation, even for M1-empty directories.** `secrets/` is mode 0700 from day one. Any file ever written into `secrets/<name>/` is mode 0600. We do not retroactively tighten permissions when M3 lands; that is a security regression waiting to happen.

---

## 7. M1 Acceptance and Cuts

### 7.1 Acceptance criteria

On a fresh Ubuntu LTS host with Docker, Caddy, and the `decloud` server binary already present (installed by hand for M1 — bootstrap is M2), an operator SSH'd into the host can:

1. `cd` into a directory containing a `Dockerfile` and an `env.sh`.
2. Run `decloud deploy service --name foo --host foo.example.com --port 8080 --readiness-path /healthz` (Joel finalizes exact flag shape; my v1 phrasing held up under Linus review).
3. The command sources `env.sh` (using the portable mechanism per §4.1), captures the resulting env, builds the image (`decloud-foo:<deploy-id>`), writes the two registration files (`config/services/foo.toml` and `secrets/foo/env.toml`) with correct permissions, starts the container on the shared Docker network with the captured env injected at `docker run` (never baked into the image), polls the readiness probe, regenerates the Caddyfile from all registered services, runs `caddy reload`, and on success returns 0.
4. `curl https://foo.example.com/` works, served by Caddy with a real Let's Encrypt cert (assuming DNS is pointed correctly — DNS is operator's job per README).
5. `decloud status foo` and `decloud logs foo` return useful output.
6. `decloud stop foo` / `decloud start foo` / `decloud restart foo` / `decloud unregister foo` all behave per their names. `unregister` removes both registration files (in the §3.3 order), removes the container, regenerates the Caddyfile, reloads.
7. **First-deploy Caddy bootstrap:** if `/opt/declouding/config/caddy/Caddyfile` does not exist when the first `decloud deploy service` runs, the deployer writes a minimal valid Caddyfile (e.g. an empty file or a no-op stanza Joel finalizes) before invoking `caddy reload`. The operator's pre-installed Caddy systemd unit must reference the same path. If Caddy is not running when `caddy reload` is invoked, we surface a clear error pointing at the operator's setup checklist.
8. **Loader integrity:** the loader rejects (a) `schema_version` mismatch, (b) non-empty `Mounts` (M3-only), (c) `strategy != "recreate"` (blue/green is M4-only), (d) wrong file permissions on `secrets/<name>/env.toml`, (e) any unknown TOML field (strict mode), (f) config exists but secrets file missing. Each rejection has a specific, actionable error message.

### 7.2 Explicit non-goals for M1

Cut these or we do not ship this year:

- **No client binary.** Operator SSHes in. The README explicitly says the server CLI is "equally usable by a human who SSH'd in directly." Client is M3.
- **No zero-downtime deploy.** M1 uses `strategy: recreate` for everything. Blue/green via Caddy admin API is M4. Yes, the README says blue/green is the default; we are explicitly downgrading the default for M1 and will flip it in M4. Document this loudly in `_docs/`.
- **No jobs.** No systemd timer generation. M5.
- **No backups.** No `restic`. M6.
- **No image GC.** M6.
- **No host bootstrap script.** Manual install for M1. M2.
- **No persistent volumes / no `--mount` support.** The `--mount` CLI flag is rejected with "M3 only"; the `Mounts` field is reserved in the schema but the loader rejects non-empty values with the same "M3 only" error. (Closes the hand-edit loophole.)
- **No restart-on-crash supervisor.** Use Docker's `--restart=unless-stopped`. The README's "host-level Declouding supervisor" can wait — Docker already does the boring 90% of this.
- **No Viper.** M2 wires it when there's an actual `/etc/decloud/config.toml` to read.

### 7.3 Risks to flag at M1

- **Caddy admin API vs Caddyfile-on-disk dual sources of truth.** M1 only uses the file + reload path, which sidesteps this. M4 introduces the admin API for hot-path upstream swaps. M4's tech plan must commit to a single rule: file remains source of truth, regenerated on every change; admin API is used additionally only for the hot-path upstream swap during a blue/green deploy; after the swap, the Caddyfile is regenerated to match (so a Caddy restart converges).
- **`env.sh` execution is arbitrary shell.** That is by design (per README). The capture mechanism in §4.1 handles the long tail of edge cases; Joel's tech plan documents them. The one known limitation (script setting a value identical to the baseline is dropped from the captured env) is documented in operator-facing docs, not just godoc.
- **Schema versioning.** §5 commits to v1 across M1 and M3 with strict-mode forward-compat. Loader is strict on day one.
- **Atomicity of registration writes.** Each file write goes through `<name>.toml.tmp` then `os.Rename`. The two-file write (config + secrets) is *not* cross-atomic; the loader detects inconsistency and the operator recovers by re-running the deploy. Documented in §3.3.
- **One operator assumption.** README explicitly defers per-service deploy locks. Fine for M1. Document the assumption in code comments where it matters (the deploy orchestration).
- **macOS dev portability.** §4.1 closes this. No GNU-only flags in the env capture. Tests run on stock macOS bash 3.2.

---

## 8. Process Config: deferred to M2

**v1 wired Viper for `--config-root` overriding. v2 defers Viper to M2.**

Rationale (Linus's argument, accepted): Cobra alone supports flag-from-env-default in three lines (`StringVar` + read `os.Getenv("DECLOUD_ROOT")` for the default). M1 has no `/etc/decloud/config.toml` for Viper to read. Wiring Viper now buys nothing M1 needs and is ~50 lines that someone has to maintain in the meantime.

M1 implementation: `internal/cli/root.go` defines `--config-root` defaulting to `os.Getenv("DECLOUD_ROOT")` if non-empty, else `/opt/declouding`. The `internal/config/` package becomes a tiny thing that holds path constants and the config root override. M2 retrofits Viper and wires it into the same package without touching any other code.

If Joel disagrees and wants Viper now to "avoid retrofitting," I'll accept it as a non-blocker, but I want him to defend that view in the revised tech plan rather than carrying it forward by default.

---

## 9. Milestone sequence

Each milestone is a shippable increment. Do not start the next one until the previous one has tests and Raymond has updated `_docs/`.

- **M1 — Service deploy MVP (server-side, recreate strategy).** Defined in detail above. The bedrock. Everything else is built on the registry (config + secrets split), the Caddyfile generator, and the container lifecycle code that lands here.

- **M2 — Host bootstrap.** A `decloud bootstrap` subcommand (or a separate small script — Joel decides) that on a fresh Ubuntu host: installs Docker and Caddy from their official repos, creates the `/opt/declouding/` directory tree with correct permissions (including `secrets/` at 0700), creates the shared Docker network, installs and enables the single Declouding host systemd unit, installs the systemd unit for Caddy pointing at `/opt/declouding/config/caddy/Caddyfile`, drops a logrotate config for `/opt/declouding/logs/decloud.log`, and verifies the install by deploying a built-in trivial "hello" service. **Also: introduces Viper** (the deferral target from §8) when the bootstrap optionally writes a `/etc/decloud/config.toml`. Acceptance: a clean cloud VM is fully usable by the operator within one command + DNS setup.

- **M3 — Server-side env/mounts/secret-files hardening + client binary.** Don's note: Linus called this fat and he was right. **Plan to subdivide into M3a and M3b when M3's tech plan is written.** Sketch:
  - **M3a (server-side):** harden env-capture with the full edge-case test suite; implement `--mount` end-to-end (TOML schema becomes populated, loader stops rejecting non-empty `Mounts`, `dockerdrv.RunRequest.Mounts` actually wires through to `docker run -v`); add secret-file declarations stored under `/opt/declouding/secrets/<service>/files/` mode 0600 with the directory at 0700; the `mounts` block in the config TOML can reference paths under `secrets/<name>/files/` and the runtime mounts them read-only into the container.
  - **M3b (client):** the `decloud` client binary: package source tree honoring `.dockerignore`, upload via SSH (`tar c <dir> | ssh host decloud deploy service --stdin --name foo ...`), proxy stdio, exit with remote exit code. Distributed via `go install`. Server adds `--stdin` flag that extracts the bundle to a tmpdir and runs the M1 deploy logic against that path; the existing `<source-dir>/env.sh` default discovery applies post-extraction.
  - **`schema_version` stays at 1** (per §5). M3 only populates fields that M1 reserved.

- **M4 — Zero-downtime blue/green deploy via Caddy admin API.** Replace M1's recreate-everything with the real lifecycle: build → start new on shared network with deploy-id-suffixed container name (`decloud-<name>-<deploy-id>`) → poll readiness → PATCH Caddy admin API to swap upstream → SIGTERM old, grace period, SIGKILL, remove. Add the `strategy = "recreate"` opt-out for services with exclusive resources; loader stops rejecting `strategy = "blue_green"`. Regenerate the on-disk Caddyfile after every deploy so the persisted source of truth stays accurate; on Caddy restart it reloads from disk and converges. Rollback on readiness failure: kill new, leave old. **Explicit M4 deliverable: one-time recreation of all M1-era containers under the new naming convention** (`decloud-<name>` → `decloud-<name>-<deploy-id>`). The M4 tech plan documents this as a tracked migration step, not an afterthought.

- **M5 — Jobs.** `decloud deploy job` (client) and the server-side machinery: per-job `<name>.timer` and `<name>.service` systemd units written to `/etc/systemd/system/`, where the `.service` is a oneshot that invokes `decloud run-job <name>`. `decloud unregister <name>` understands jobs too. Status/logs work via `systemctl list-timers` and `journalctl -u`. Job registrations live at `config/jobs/<name>.toml` + `secrets/<name>/env.toml` (same split rule as services; the secrets path is shared between services and jobs for a given `<name>`, so we should reject service-and-job-with-same-name early — add to M5's tech plan).

- **M6 — Backups + image housekeeping.** Install `restic` during bootstrap (retroactively update M2). Write a `decloud-backup.timer` + `.service` pair that runs `decloud backup run` nightly. `decloud backup run` invokes `restic backup` against `/opt/declouding/` plus all volumes discovered from registrations plus Caddy's ACME data dir; repo URL and password come from a host-side config file (NOT from the backed-up secrets dir — chicken and egg). `decloud backup list` and `decloud backup restore` are thin wrappers over `restic`. Independently: a `decloud-gc.timer` runs `decloud gc` weekly, which shells out to `docker system prune -a --filter until=168h` and `docker builder prune --filter until=168h`. `decloud gc` runs the same on demand.

- **M7 — Operational polish.** Anything that genuinely emerged as missing during M1–M6 use. Candidates, in priority order: per-service deploy lock, crash-budget auto-revert, the Declouding host-level supervisor (if Docker's `--restart=unless-stopped` proved insufficient), better `decloud status` output, optional log aggregation if (1) was wrong and we actually need it. Resist scope creep. Anything that smells like a Non-Goal from the README gets killed on sight.

---

## 10. M1 operational deliverables (the easy-to-forget list)

Linus surfaced these. None are technically interesting; all need explicit owners.

- **`go.mod` with Go 1.22+ pin.** Use 1.22 minimum; allows `range int`, `slices` package, `cmp.Or`. Joel's tech plan §1.1 already specifies module path; just add the `go 1.22` directive.
- **LICENSE file at repo root.** Maintainer's call on MIT vs Apache-2.0. Pick one before M1 ships. I lean Apache-2.0 because we may want patent grants given the deploy-orchestration nature, but this is the maintainer's preference.
- **`.github/workflows/test.yml`** running `go test ./...` on push/PR. Linux runner only for M1; integration tests (`-tags integration`) gated to nightly; macOS runner added in M2 once Docker-on-Mac concerns are clear. M1 CI gate is unit tests on Linux.
- **`_docs/` deliverables (Raymond):**
  - `_docs/cli/decloud-deploy-service.md` — operator-facing reference for the M1 command.
  - `_docs/architecture/m1-recreate-strategy.md` — explains why M1 uses recreate, how to tell when to expect blue/green (M4).
  - `_docs/architecture/secrets-layout.md` — the §3 split, why it exists, how to inspect.
  - `_docs/operator/manual-install.md` — the manual install steps that M2's bootstrap will eventually automate (Docker, Caddy, decloud binary, Caddy systemd unit pointing at `/opt/declouding/config/caddy/Caddyfile`).
- **`_ai/` deliverables (Raymond):**
  - `_ai/decisions/m1-scope.md` — captures why M1 is what it is (this document, summarized).
  - `_ai/decisions/secrets-split.md` — captures the §3 decision and rejected alternatives.
  - `_ai/decisions/schema-versioning.md` — captures §5.
- **Structured logging.** The decloud binary writes structured JSON logs via `log/slog` (Go stdlib since 1.21) to both stderr and `/opt/declouding/logs/decloud.log`. Format documented in `_docs/`. Logrotate config is M2's bootstrap problem; for M1 the log just appends.

---

## 11. What I want from Joel and Linus

### 11.1 Joel

Take this revised plan and update `03-tech-plan.md` to a v2 (`05-tech-plan-v2.md` or whatever the bureau gives you). Specifically:

- **Rewrite §3.1 (TOML schema)** to reflect the split: a `ServiceConfigSpec` for the config TOML and a `ServiceSecretsSpec` for the secrets TOML, with an in-memory `ServiceSpec` that merges them. Document the loader/saver semantics from §3.3 of this plan precisely.
- **Rewrite §3.5 (envcap.Capture)** to use the portable mechanism (the `compgen -e` + `printf '%s=%s\0'` block, or your defended alternative). Update the test plan in §6.1 to drop the "skip on Windows" hedge and replace it with "runs on macOS dev box and Linux CI without modification."
- **Update §3.4 (registry.Store interface)** to take/return the merged `ServiceSpec` but write/read both files internally. Add the inconsistency-detection error path.
- **Update §4.2 (CLI flag table)** to drop Viper from M1 and use plain Cobra + `os.Getenv`. The `--config-root` flag stays, just with a simpler implementation.
- **Update §5 (file layout)** to match §6.2 of this plan (drop the cache sentinel; show the split).
- **Update §4.6 (partial failure behavior)** for the two-file write — specifically, what happens if the config write succeeds and the secrets write fails, or vice versa. Pre-step-7 cancellation already leaves the registry untouched; the new question is mid-step-7 cancellation between the two file writes.
- **Add a §11 (or wherever fits)** documenting the M1 operational deliverables from §10 of this plan.
- **Confirm or push back** on §4.1 portability constraint, §5 schema versioning rule, §8 Viper deferral, §3 secrets split. If you push back on any of these, do it with reasoning; I will read it carefully and either capitulate or escalate to a third-pass review.

### 11.2 Linus

After Joel's revised tech plan lands, re-review with these focal points:

- Is the secrets split in §3 actually airtight, or did I miss an edge case (multiple deploys racing on the secrets file? unregister leaving an orphan?)?
- Is the `compgen -e` mechanism (or whatever Joel picks) actually portable on macOS bash 3.2? You floated it; confirm or reject.
- Is the schema-version-stays-at-1 commitment defensible given what we now know about M3a/M3b's likely shape?
- Anything else surfaced by the revisions that we haven't seen.

---

## 12. Things I'm explicitly NOT deciding here

- The exact Cobra command tree shape (Joel).
- Whether the supervisor is its own binary or a long-running mode of `decloud` (defer to M7 if at all).
- The exact health-probe semantics — timeout, interval, max attempts (Joel, in M1 tech plan).
- The exact form of the M2/M3/M4/M5/M6/M7 tech plans — those get written when those milestones start, not now.
- Whether M3 splits into M3a/M3b formally or as an internal sequencing convention. Decide when M3's tech plan gets written.

---

That is the revised plan. Build M1. Ship M1. Then we talk about M2.
