# M1 scope: server-side service deploy with `recreate` strategy

**M1 = `decloud deploy service` end-to-end on the host, executed by a human SSH'd in.** Build a Docker image from a local source dir, register as config TOML + secrets TOML, generate/reload the Caddyfile, start container on shared Docker network, serve real HTTPS traffic on a real hostname. Strategy is `recreate` only.

## Why this and not the obvious alternatives

- **NOT "client first"** — the client is a glorified `tar | ssh`; worthless until there's a server-side `decloud` for it to invoke. Building it first guarantees rework.
- **NOT "host bootstrap first"** — bootstrap is `apt install docker caddy && systemctl enable decloud.service`; five lines of substance, exercises none of the design's hard parts. M3.
- **NOT "jobs first because they're simpler"** — jobs ARE simpler (no Caddy, no zero-downtime, no readiness) but exercise less of the design. Services exercise the registry, Caddyfile generator, container lifecycle, readiness model, env capture, secrets split, and mount model **all at once**. First milestone must put the most design risk under load.

## Explicit M1 cuts (each one is a tar pit if relaxed)

- **No client binary** — operator SSHes in. README explicitly says server CLI is "equally usable by a human SSH'd in." Client is M7.
- **No zero-downtime / no blue-green** — `strategy: recreate` only. README says blue-green is the default; M1 explicitly downgrades. Loader rejects `strategy = "blue_green"`. Blue-green via Caddy admin API is M4.
- **No jobs** / **No backups** / **No image GC** / **No bootstrap script** — M5/M6/M6/M3 respectively.
- **No `--mount`** — flag rejected; loader also rejects non-empty `Mounts` (closes hand-edit loophole). M2.
- **No restart-on-crash supervisor** — Docker `--restart=unless-stopped` does the boring 90%. README's supervisor talk waits for M7.
- **No Viper** — plain Cobra + `os.Getenv("DECLOUD_ROOT")` is three lines. M3 introduces Viper when there's a real `/etc/decloud/config.toml` to read. M2 introduces no global config or Viper plumbing — that lands at M3. M2's mount config is per-service via `--mount` and the existing `Run.Mounts` field reserved at M1; do not "helpfully" add `/etc/decloud/config.toml` parsing in M2 for default-mount-options or similar (that is the Option C ad-hoc-loading trap rejected in `_tasks/2026-04-28-milestone-resequence/002-don-plan.md` §"Justification").

## Recreate downtime and step ordering

Sequence: capture env → docker build new → stop old (downtime starts) → docker rm old → docker run new → poll readiness → registry Save (config first, then secrets) → Caddyfile regenerate (stub if missing) → `caddy reload`.

Downtime is intentional in M1; flipping to blue/green is M4's whole point. Document loudly in `_docs/architecture/m1-recreate-strategy.md`.

## First-deploy Caddyfile bootstrap

If `/opt/decloud/config/caddy/Caddyfile` doesn't exist when first deploy runs, deployer writes a minimal valid stanza (`:80 { respond "decloud: no services registered yet" 404 }`) before invoking `caddy reload`. Avoids the operator's pre-installed `caddy run --config <path>` crashing on missing file. Bytes-exact stub; not just "an empty file" because Caddy v2 has historically been "accept but warn" on empty configs and we want a clear "alive but unconfigured" signal on first `curl`.

## Milestone sequence (M1 → M7)

M1 service deploy MVP → M2 server-side mounts + env-file hardening (`--mount` flag, loader populates `Mounts`) → M3 host bootstrap (introduces Viper, `caddy.image` config knob) → M4 zero-downtime blue/green via Caddy admin API → M5 jobs (systemd timers) → M6 backups + image GC → M7 operational polish (supervisor, deploy locks, secret-files-on-disk, client binary, etc.).

M7 is the deferred-feature bucket and will be re-planned at M7-start time, possibly split into multiple milestones then. Bundling client binary + secret files + operational polish there is bin-packing convenience, not a commitment to ship them as one milestone — do NOT repeat the M3a/M3b mistake by treating "everything in M7" as a single deliverable.

Don't reopen this sequencing without a concrete reason. Linus approved the bones in `_tasks/2026-04-26-readme-implementation-planning/07-linus-review-v2.md`. The 2026-04-28 resequence (`_tasks/2026-04-28-milestone-resequence/`) re-ordered M2/M3 and split former M3a/M3b across M2/M7 per maintainer priority; Linus's approval of the original bones still applies to M1's content, which is unchanged.
