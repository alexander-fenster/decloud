# Tech Plan — README rewrite + MIT LICENSE

**Author:** Joel Spolsky (implementation planner)
**Status:** Plan-step #2. Expands Don's `02-plan.md`. Awaits Linus's review.
**Branch:** `task/readme-and-license`.
**Scope:** still documentation only. Two files written, one file lightly edited:
1. `/Users/fenster/dev/decloud/README.md` — full rewrite.
2. `/Users/fenster/dev/decloud/LICENSE` — new file, MIT, 2026 Alexander Fenster.
3. `/Users/fenster/dev/decloud/_docs/install.md` — surgical fix to §8 only.

This document does for Rob and Raymond what an IKEA assembly diagram does for the harried Saturday-morning shopper: every step concrete, every dimension named, every "where does this Allen wrench go" already answered. If Rob has to make a judgment call while implementing, I have failed.

---

## Section 0 — Verification: I checked the code

Don's plan claims certain facts are "verified." I re-verified the load-bearing ones rather than trusting summary:

- **Module path** is `github.com/alexander-fenster/decloud` (`go.mod:1`). The `go install` URL Don uses in the Quick start (`...@latest`) targets `cmd/decloud`, which exists at `cmd/decloud/main.go`. Confirmed.
- **Cobra subcommand surface** is exactly: `caddy {up,down,reload}`, `deploy service`, `logs`, `restart`, `start`, `status`, `stop`, `unregister`. Verified against `internal/cli/root.go:29-44`. The `deploy` group also exists as a Cobra parent with `service` underneath (`root.go:29-31`); `decloud --help` lists `deploy` not `deploy service`. Important for the README "what you get today" wording.
- **`--mount` flag** exists on `deploy service` only (`internal/cli/deploy_service.go:61-62`). Type is `StringArrayVar` (correct — preserves embedded commas; `StringSliceVar` would not). Repeatable. Already documented in `_docs/usage.md:71`.
- **Required flags on `deploy service`:** `--name` is marked required via `MarkFlagRequired("name")` at `deploy_service.go:67`. `--port` is enforced manually with an error message inside `runDeployService` (`deploy_service.go:79-81`) — Cobra default is `0`, the explicit zero check rejects with `errUsage`. So both `--name` and `--port` are effectively required. Don's plan already says this and is correct.
- **`--strategy=recreate` only.** Loader rejects anything else with `ErrInvalidStrategy` → exit 10. Verified at `deploy_service.go:76-78` and `internal/cli/exit_codes.go:42`.
- **Schema version `1`.** Confirmed at `internal/registry/types.go:5` (`CurrentSchemaVersion = 1`) and the rejection wrapping at `internal/registry/store.go:65`. M2 did not bump.
- **Exit codes set** is exactly `0, 2, 10, 20, 30, 40, 50, 60, 70, 130` — verified at `internal/cli/exit_codes.go:13-23`. Eleven distinct codes, matches `_docs/usage.md` §3 and Don's claim.
- **Go version** is `go 1.22` per `go.mod:3`. The Quick start prerequisite line should say "Go 1.22+" not "Go 1.22 only." Don wrote "Go 1.22+" — correct.
- **Integration tests** — `internal/integration/doc.go` confirms the gating: `DECLOUD_INTEGRATION=1 go test -tags integration ./internal/integration/...`. Don's plan says "build tag `integration` and `DECLOUD_INTEGRATION=1` env var." Both apply; doc.go shows the tag is `integration` and the env var is `DECLOUD_INTEGRATION=1`. The Contributing section will use the exact incantation from `doc.go`.
- **Repo top-level layout** (verified by `ls`): `_ai/`, `_docs/`, `_tasks/`, `cmd/`, `internal/`, `CLAUDE.md`, `README.md`, `go.mod`, `go.sum`, `tools.go`. The Repository-layout section will list `cmd/`, `internal/`, `_docs/`, `_ai/`, `_tasks/` plus a one-line note for `CLAUDE.md` and `tools.go`. No surprises.
- **`tools.go`** exists at the repo root. I had not noticed it in any earlier conversation. Verified: it is the standard Go `//go:build tools` pattern for pinning tool dependencies (`go.uber.org/mock` per `go.mod:13`). Worth a single line in the Repository-layout section so a reader doesn't ask "what is this stray file."
- **No client binary.** Verified: there is no `client/` directory, no `cmd/decloud-client`, no `internal/client`. `_docs/usage.md:5` is the canonical disclaimer.
- **No `decloud bootstrap` command.** Verified by inspection of `internal/cli/root.go:29-44`. Confirmed.
- **`_docs/install.md` §8** content: lines 212-214 read exactly as Don quotes, "This repository does not yet declare a license. If you intend to redistribute the binary or use it in a context that requires explicit license grant, ask the maintainer before doing so." Confirmed — needs the surgical fix.

**Things Don's plan got slightly off — adjustments here:**

1. **Status line wording.** Don's draft Project Status (plan §6) says "every M1+M2 deploy briefly stops the old container before starting the new one." The actual sequence per `_ai/decisions/m1-scope.md:21` is *capture env → docker build new → stop old → docker rm old → docker run new → poll readiness*. The "briefly stops the old before starting the new" framing is accurate at user-visible level (downtime starts at stop-old) but technically the build happens before the stop. README Project Status will say "Recreate strategy — there is brief downtime as the old container is stopped before the new one starts." This matches `_docs/usage.md:55` verbatim and avoids the "build new first" detail (which is interesting to engineers but distracting in a status paragraph).

2. **"M1.0 host-Caddy install" mention.** `_docs/install.md` §3.2 documents migrating from an earlier "M1.0" host-Caddy build that never made it into the README narrative. The README Quick start should NOT mention this. It's a maintainer-only artifact and pulling it forward would confuse fresh readers. Linked to from `_docs/install.md` §3 — that's enough.

3. **Don's `cmd/decloud/main.go` mention** in §2.1 is accurate but the path the user will interact with is the `go install` URL `github.com/alexander-fenster/decloud/cmd/decloud@latest`. README uses the URL form, not the in-repo path.

No factual errors in Don's plan. The above are tightening-and-pruning judgments.

---

## Section 1 — Concrete content outline for `README.md`

Each subsection below specifies for one README section: (a) the markdown heading, (b) the structure (paragraphs, bullets, code blocks), (c) the source of truth Rob copies/condenses from, (d) length budget in lines, (e) any "must-not" constraints. Rob's job is mechanical assembly. No prose-authorship judgment required.

### 1.1 Heading and elevator pitch

**Heading:** `# Decloud` (single H1; markdown linters flag multiple H1s and GitHub renders the second as plain text).

**Body:** two paragraphs, total 3-4 sentences.

- Paragraph 1: one sentence. "Decloud is a small, single-host platform-as-a-service for low-traffic services that don't need a full cloud runtime." (This compresses the existing `README.md:3` opening — keep "single-host," "low-traffic," "platform-as-a-service" as the load-bearing terms.)
- Paragraph 2: one or two sentences. The replaced workflows: "It replaces a few specific workflows: Cloud Run services deployed via `gcloud run deploy --source .`, long-running services running as host `systemd` units, and Cron jobs scheduled with Google Cloud Scheduler. The expected operator is one person with SSH access to one Linux host." (This compresses the existing `README.md:5-11` enumeration. Keep the `gcloud run deploy --source .` reference verbatim — it's how potential users will recognize their own use case.)

**Length budget:** 4 lines of prose, 0 code blocks.

**Must-not:** no marketing adjectives ("fast," "modern," "powerful," "cloud-native"). No badges. No logo. No "alternatives" comparison (one mention of Dokku/CapRover/Coolify is in the existing README at line 13 — Don's plan §3.2 cuts it, which I agree with: at this stage it adds noise without adding signal).

### 1.2 Project status

**Heading:** `## Project status`

**Body structure:**

- One sentence intro: "Decloud is mid-build. As of April 2026, only the milestones marked SHIPPED below are usable." (This sentence is load-bearing — it sets expectations before the reader scrolls down to the install instructions and types `go install`.)
- A "What ships today" sub-list (no sub-heading; just an inline `**What ships today (M1 + M2):**` lead-in, then bullets):
  - "**M1** — server-side `decloud deploy service` with the `recreate` strategy; lifecycle commands `start`, `stop`, `restart`, `status`, `logs`, `unregister`; `decloud-caddy` ingress on a Docker network; ports 80/443/443-UDP on the host."
  - "**M2** — persistent volumes via `--mount` (bind paths and Docker named volumes; `:ro` mode flag)."
- A "What does not yet ship" sub-list (same lead-in pattern: `**Not yet shipped — see [Roadmap](#roadmap):**`):
  - "**M3** — host bootstrap script (install Decloud manually for now; see [`_docs/install.md`](_docs/install.md))."
  - "**M4** — zero-downtime blue/green deploys."
  - "**M5** — scheduled jobs (`decloud deploy job`)."
  - "**M6** — encrypted backups via `restic`; image GC."
  - "**M7** — client binary for laptop-side `decloud`; deploy-time secret files."

**Length budget:** ~14 lines (1 intro + 3 lead-in lines + 7 bullets + blank lines).

**Source:** Don's plan §6 "What 'honest about half-baked' looks like" gives the rough draft; this section adopts it nearly verbatim, only re-formatted to fit the markdown rendering style of the rest of the README and with the M3/M4/M5/M6/M7 framing pulled from `_ai/decisions/m1-scope.md` last paragraph (the M1→M7 sequence sentence) and `_tasks/2026-04-28-milestone-resequence/002-don-plan.md:11-19`.

**Must-not:** no apologetic phrasing ("we're sorry," "still missing," "should eventually"). State the fact, link to the roadmap, move on. Don will rightly call out tone-of-voice problems on review.

### 1.3 Quick start

**Heading:** `## Quick start`

**Body structure:**

- One sentence: "On a fresh Linux host with Docker and a Go toolchain installed:"
- One fenced shell block (language tag `sh`) with the commands. See **§2.3 below** for the verbatim block. Length: ~14 shell lines.
- One sentence after the block: "DNS for `myservice.example.com` must already point at the host so Caddy can complete the ACME challenge."
- One sentence linking deeper: "For the full procedure (firewall, ACME rate limits, migrating from earlier installs), see [`_docs/install.md`](_docs/install.md). For the deploy flag reference, see [`_docs/usage.md`](_docs/usage.md)."

**Length budget:** 3-4 lines of prose + ~14 lines of shell = 18 lines.

**Source:** `_docs/install.md` §4 (mkdir tree), §5 (go install), §6 (bootstrap order), `_docs/usage.md` §1 (deploy command shape).

**Must-not:** do not duplicate the full chmod sequence from `_docs/install.md` §4 (that's seven `chmod` lines in `install.md`; the README compresses to one `mkdir -p` and links). Do not mention `M1.0 host-Caddy migration`. Do not include the SELinux note. Do not include `--readiness-path /healthz` in the example unless we also include `--readiness-timeout`; both have safe defaults so neither is needed for the minimum-working example.

### 1.4 What you get today (M1 + M2)

**Heading:** `## What you get today`

**Body structure:** a tight bulleted list, 7-9 bullets, one line each. Pulled from `_docs/usage.md` §4 (Lifecycle commands) cross-checked against `internal/cli/root.go:29-44`:

- "`decloud deploy service` — build a Docker image from a source dir, run it on the shared `decloud` network, route Caddy to it, persist the registration."
- "`decloud start | stop | restart <name>` — lifecycle controls. `start` re-runs the container from the saved image+env; `restart` is stop-then-start; neither rebuilds."
- "`decloud status <name>` — runtime + registry state on one line."
- "`decloud logs <name> [-f] [--tail N]` — pass-through to `docker logs`."
- "`decloud unregister <name>` — remove the container, both registry files, and the Caddy route."
- "`decloud caddy up | down | reload` — bring the `decloud-caddy` container up on the shared `decloud` network, take it down, or regenerate the Caddyfile from the registry and reload."
- "`--mount` for `deploy service` — bind paths (`/host:/container[:ro]`) or named volumes (`name:/container[:ro]`)."
- "`env.sh` capture — sourced inside a hermetic bash, exported variables become container environment, never baked into the image."
- "Strategy is `recreate` — brief downtime during the swap. Blue/green is M4."

**Length budget:** ~9 bullets + 1 sentence intro = 11 lines.

**Source:** `_docs/usage.md:186-198` (the §4 bullet list — the README list is a one-line-each compression of those, plus two bullets that summarize `--mount` and `env.sh` from `_docs/usage.md:71` and `_docs/usage.md:152-156`).

**Must-not:** do not include flag detail — leave that to `_docs/usage.md`. Do not mention exit codes (also in usage.md). Do not list every `caddy up` named volume (`decloud_caddy_data` etc.) — that's `_docs/install.md` §3 territory.

### 1.5 Architecture in 60 seconds

**Heading:** `## Architecture in 60 seconds`

**Body structure:** three short paragraphs, no sub-headings, no code blocks.

- Paragraph 1 (~3 sentences): "One Linux host. Docker runs every workload, including Caddy itself — `decloud-caddy` is just another container on the shared `decloud` Docker network. Caddy reaches each service container by its Docker DNS name (`decloud-<service>`); service containers are not host-port-published."
- Paragraph 2 (~3 sentences): "All persistent state lives under `/opt/decloud/`: per-service config TOML at `config/services/<name>.toml`, secrets at `secrets/<name>/env.toml`, the generated Caddyfile at `config/caddy/Caddyfile`. One backup path covers everything that matters."
- Paragraph 3 (~2 sentences): "There is no Decloud daemon and no listening management port. SSH is the management transport: the operator SSHes in and runs `decloud` directly. A laptop-side client binary is on the roadmap (M7); for now the SSH-and-run-directly path is the supported flow."

**Length budget:** 3 paragraphs, ~10 lines.

**Source:** the existing `README.md` "Operating Model" (lines 15-26) and "CLI Shape" (lines 202-209), distilled by ~70%; `_ai/decisions/caddy-runs-in-container.md` for the in-container-Caddy detail; `_docs/usage.md:5-6` for the no-client-binary statement.

**Must-not:** do not enumerate the `/opt/decloud/` subdirectories one-by-one — that's `_docs/install.md` §4. Do not mention `restic` or backup paths in this section (they're roadmap items, not architecture). Do not say "blue-green is the default" anywhere — that was the pre-M1 design statement and is now wrong.

### 1.6 Install (overview + link)

**Heading:** `## Install`

**Body structure:** three short paragraphs, no code blocks (the Quick start has the code; this section is orientation).

- Paragraph 1 (~2 sentences): "Decloud is installed manually in M1+M2 — no bootstrap script yet. Target OS: Linux with Docker and systemd; tested on Ubuntu LTS and Debian."
- Paragraph 2 (~3 sentences): "Prerequisites: a Linux host with root or sudo, outbound HTTPS, the public ports `80/tcp`, `443/tcp`, and `443/udp` open on the host firewall, and DNS for any hostnames you plan to deploy already pointing at the host (so Caddy can complete the ACME challenge). A Go toolchain on the host is convenient but not required — you can build the binary elsewhere and `scp` it in."
- Paragraph 3 (1 sentence): "Full procedure with the chmod sequence, ACME-rate-limit caveats, and migration notes for older installs: [`_docs/install.md`](_docs/install.md)."

**Length budget:** 3 paragraphs, ~7 lines.

**Source:** `_docs/install.md` §1 (Prerequisites), §3.1 (firewall), §5 (go install / cross-build).

**Must-not:** do not duplicate any actual command from `_docs/install.md`. The Install section in the README is an orientation paragraph plus a link, by design.

### 1.7 Usage (overview + link)

**Heading:** `## Usage`

**Body structure:**

- One opening sentence: "Three illustrative commands. The full flag reference and exit-code table live in [`_docs/usage.md`](_docs/usage.md)."
- One fenced shell block (language tag `sh`) with three one-liners. See **§2.4 below** for the verbatim block.
- One closing sentence pointing at `_docs/usage.md` §3 for exit codes and §4 for the lifecycle command reference.

**Length budget:** 2 lines of prose + ~10 lines of shell = 12 lines.

**Source:** `_docs/usage.md` §1 (deploy example), §4 (lifecycle commands), §5 (end-to-end example).

**Must-not:** do not include the full deploy example with all flags — that's `_docs/usage.md` §1. Do not show output (e.g., `deploy: myservice ready`) — output examples bloat without informing.

### 1.8 Roadmap

**Heading:** `## Roadmap`

**Body structure:** a labelled list (NOT a table — Don and I agree narrow-viewport survival matters more than columnar look). Format: each entry is a bullet, milestone label in bold, one-sentence description, status tag in parentheses.

- "**M1** — Server-side service deploy with `recreate` strategy. (SHIPPED)"
- "**M2** — Persistent volumes via `--mount`. (SHIPPED)"
- "**M3** — Host bootstrap script and config-file plumbing (Viper). (PLANNED)"
- "**M4** — Zero-downtime blue/green deploys via Caddy admin API. (PLANNED)"
- "**M5** — Scheduled jobs via systemd timers (`decloud deploy job`). (PLANNED)"
- "**M6** — Encrypted backups via `restic`; image GC (`decloud gc`). (PLANNED)"
- "**M7** — Laptop-side client binary; deploy-time secret files; operational polish. (PLANNED)"

**Length budget:** 7 bullets, ~7 lines.

**Source:** `_ai/decisions/m1-scope.md` last paragraph ("M1 service deploy MVP → M2 server-side mounts → M3 host bootstrap → M4 zero-downtime blue/green → M5 jobs → M6 backups + image GC → M7 operational polish"). The post-resequence wording is in `_tasks/2026-04-28-milestone-resequence/002-don-plan.md:11-19`. I am using the resequenced labels.

**Must-not:** no sub-bullets explaining each milestone in detail — the Project Status section already lists what's not shipped. Don't say "ETA Q3 2026" or any date — none have been committed. Don't claim M7 will ship as one milestone (it might split; per `_ai/decisions/m1-scope.md` last paragraph, "Bundling client binary + secret files + operational polish there is bin-packing convenience, not a commitment to ship them as one milestone").

### 1.9 Non-goals

**Heading:** `## Non-goals`

**Body structure:** lead-in sentence + bullet list. Lift the substance of the existing `README.md:245-258`, trim wording.

- Lead-in: "Decloud will not provide:"
- Bullets (one line each, lifted from `README.md:249-258` with light prose trim):
  - "Horizontal autoscaling"
  - "Scale-to-zero"
  - "Deploying prebuilt Docker images" *(keep — Decloud is source-to-container; the source-to-image path is intentional)*
  - "A web management UI"
  - "A public management API"
  - "Multi-node orchestration"
  - "Kubernetes compatibility"
  - "Full Cloud Run feature parity, or Cloud Run-compatible CLI flags" *(merge two existing bullets into one — they're the same point)*
  - "Per-application host `systemd` units (per-job timer units for scheduled jobs are fine)" *(condensed from existing line 258)*

**Length budget:** 1 lead-in + 9 bullets = 10 lines.

**Source:** existing `README.md:245-258` verbatim except for the merge of two Cloud Run bullets and the trim of the systemd bullet.

**Must-not:** no commentary on each non-goal. The list is intentionally curt — explanations belong in the design docs.

### 1.10 Repository layout

**Heading:** `## Repository layout`

**Body structure:** one fenced block (language tag `text`) showing the top-level tree, followed by a 1-2 sentence orientation paragraph.

The block:

```text
cmd/decloud/        # the decloud binary (single main package)
internal/           # private Go packages: cli, deploy, registry, caddy, dockerdrv, envcap, ...
_docs/              # user-facing documentation (install.md, usage.md)
_ai/                # decisions, conventions, agentic-development notes
_tasks/             # per-task workflow trail (planning, review, implementation reports)
CLAUDE.md           # contributor instructions (code style, agentic workflow)
tools.go            # pinned tool dependencies (gomock)
```

Followed by one orientation sentence: "M1+M2 ship from `cmd/` and `internal/`. The `_ai/` and `_tasks/` directories are the agentic-development trail and not required reading for users."

**Length budget:** ~9 lines (block) + 1 line prose = 10 lines.

**Source:** verified `ls` of `/Users/fenster/dev/decloud/`. The package list in the comment after `internal/` is verified by `ls /Users/fenster/dev/decloud/internal/`.

**Must-not:** do not enumerate every file in `internal/cli/`. Do not list every `_ai/` decision file. Do not list every `_tasks/` directory.

### 1.11 Contributing / development

**Heading:** `## Contributing`

**Body structure:** four short paragraphs, one fenced block.

- Paragraph 1 (~2 sentences): "Build and test:"
- Fenced block (language tag `sh`):

```sh
go build ./cmd/decloud
go test ./...

# Integration tests (require Docker):
DECLOUD_INTEGRATION=1 go test -tags integration ./internal/integration/...
```

- Paragraph 2 (~2 sentences): "Code style: format with `gofmt`. CLI flags use [Cobra](https://github.com/spf13/cobra); tests use [Testify](https://github.com/stretchr/testify) and [gomock](https://github.com/uber-go/mock). The agentic-development workflow is documented in [`CLAUDE.md`](CLAUDE.md)."
- Paragraph 3 (1 sentence): "The dev maintainer's machine has no Docker; integration tests run on a separate Linux host. Treat `DECLOUD_INTEGRATION=1` as opt-in, not the default test flow."

**Length budget:** 3 paragraphs + 5 lines of shell = ~10 lines.

**Source:** `CLAUDE.md` lines 1-17 (Code Style block); `internal/integration/doc.go` (the `DECLOUD_INTEGRATION=1 go test -tags integration` incantation, copied verbatim); `_ai/MEMORY.md` references the no-Docker-on-this-Mac fact.

**Must-not:** do not import the full agentic-team list ("Don, Joel, Kent, Rob, Donald, Kevlin, Linus, Raymond, Ward, Andy") into the README. That belongs in `CLAUDE.md`. The README only points at `CLAUDE.md`.

### 1.12 License

**Heading:** `## License`

**Body:** one sentence.

- "MIT — see [`LICENSE`](./LICENSE)."

**Length budget:** 1 line.

**Source:** new `LICENSE` file (see §3 below).

---

## Section 2 — Verbatim shell blocks Rob copies into the README

Rob does not need to think about flag names or paths — the blocks are pre-computed here and ready to paste.

### 2.1 README total length budget

Sum of the section budgets above:

| Section | Lines |
|---|---|
| H1 + elevator pitch | 6 |
| Project status | 14 |
| Quick start | 18 |
| What you get today | 11 |
| Architecture in 60 seconds | 12 |
| Install | 9 |
| Usage | 14 |
| Roadmap | 9 |
| Non-goals | 12 |
| Repository layout | 12 |
| Contributing | 12 |
| License | 3 |
| Section headings + blank-line padding | ~20 |
| **Total** | **~150 lines** |

**Target ceiling:** 200 lines. Don's plan said "skimmable"; 150 lines is one screen on a 14-inch monitor at default zoom. The current `README.md` is 279 lines of mostly stale design doc; we are coming in at ~54% of that and replacing the stale content with current content. That's the right ratio.

**Bloat-risk sections to watch in implementation review:**
- **Project status** — easy to over-explain "what's not shipped yet." If it grows past 14 lines, Rob should cut detail and lean harder on the Roadmap section's links.
- **Architecture in 60 seconds** — easy to slip from "60 seconds" to "five minutes." Hard limit: 3 paragraphs. If a fourth paragraph appears, refactor.
- **Quick start** — adding `chmod` lines from `_docs/install.md` §4 is the obvious mistake. Don't.

### 2.2 Anchor scheme

GitHub auto-generates anchors from headers as `lowercase-with-hyphens`, with non-alphanumerics dropped. The anchors that other sections of the README will link to:

| Heading | Anchor |
|---|---|
| `## Project status` | `#project-status` |
| `## Quick start` | `#quick-start` |
| `## What you get today` | `#what-you-get-today` |
| `## Architecture in 60 seconds` | `#architecture-in-60-seconds` |
| `## Install` | `#install` |
| `## Usage` | `#usage` |
| `## Roadmap` | `#roadmap` |
| `## Non-goals` | `#non-goals` |
| `## Repository layout` | `#repository-layout` |
| `## Contributing` | `#contributing` |
| `## License` | `#license` |

The only intra-document link in the new README is from "Project status" → "Roadmap" (`[Roadmap](#roadmap)`). Rob: copy that one link verbatim.

### 2.3 Quick start shell block (verbatim)

This block is what Rob pastes into the Quick start section. Each line is checked against `_docs/install.md` and `_docs/usage.md`:

```sh
# 1. Create the Decloud state tree (full chmod sequence in _docs/install.md §4).
sudo mkdir -p /opt/decloud/{config/{services,jobs,caddy},secrets,state/deploys,logs}
sudo chmod 0700 /opt/decloud/secrets

# 2. Install the binary.
go install github.com/alexander-fenster/decloud/cmd/decloud@latest
sudo install -m 0755 "$(go env GOPATH)/bin/decloud" /usr/local/bin/decloud

# 3. Bring up the Caddy ingress container.
decloud caddy up

# 4. Deploy a service. ./myservice/ contains a Dockerfile and (optionally) env.sh.
decloud deploy service \
  --name myservice \
  --host myservice.example.com \
  --port 8080 \
  ./myservice
```

**Verification trail:**

- Line 2 (`mkdir -p /opt/decloud/{...}`): brace-expansion equivalent of `_docs/install.md:101-106`. The directories created match: `config/services`, `config/jobs`, `config/caddy`, `secrets`, `state/deploys`, `logs`. Pattern reviewed for shell portability — bash 3.2+ (macOS) and bash 5+ (Linux) both support brace expansion. Operator using `dash` would need the explicit form; that's a known POSIX-vs-bash trade-off and bash is the assumed shell on Ubuntu/Debian.
- Line 3 (`chmod 0700 /opt/decloud/secrets`): the only chmod that the registry actually checks at load time (verified at `_docs/install.md:119`: "secrets/ must be 0700... the registry's loader rejects the service if the modes are wrong"). The other six chmod lines from `install.md` are good hygiene but not load-bearing for first-deploy success. README compresses to the one that matters; `install.md` retains the full set.
- Line 6 (`go install github.com/alexander-fenster/decloud/cmd/decloud@latest`): verbatim from `_docs/install.md:132`. URL verified against `go.mod:1`.
- Line 7 (`sudo install -m 0755 ...`): verbatim from `_docs/install.md:133`.
- Line 10 (`decloud caddy up`): verified at `internal/cli/caddy_up.go` exists and is registered at `root.go:41`.
- Lines 13-17 (`decloud deploy service`): flag names (`--name`, `--host`, `--port`) verified against `internal/cli/deploy_service.go:57-62`. Trailing positional `./myservice` matches the `cobra.ExactArgs(1)` constraint at `deploy_service.go:52`. No `--readiness-path` because default `/healthz` is fine for the example. No `--strategy` because default `recreate` is the only accepted value.

**One judgment call I'm flagging for Linus:** the `sudo install -m 0755 "$(go env GOPATH)/bin/decloud" /usr/local/bin/decloud` line uses `sudo` only on the install step, not on `go install`. That matches `_docs/install.md` §5 which assumes the operator's `$GOPATH` is in their home directory. If the operator runs `go install` as root, the binary lands in `root`'s GOPATH which is usually `/root/go/bin`. The `install` line still works (it reads from `$(go env GOPATH)/bin/decloud` and the same shell evaluates `$(go env GOPATH)` so `root` evaluates to `root`'s GOPATH). I don't think we should add a "run all of this as root" caveat — the `_docs/install.md` already covers it and the README is supposed to be the happy path. Linus: weigh in if you disagree.

### 2.4 Usage shell block (verbatim)

This block is what Rob pastes into the Usage section. Three illustrative one-liners; not exhaustive.

```sh
# Deploy a service.
decloud deploy service --name myservice --host myservice.example.com --port 8080 ./myservice

# Deploy with a persistent bind mount (volumes survive container recreation).
decloud deploy service --name myservice --host myservice.example.com --port 8080 \
  --mount /var/lib/myservice:/data ./myservice

# Inspect a deployed service.
decloud status myservice
```

**Verification trail:**

- Line 2: same shape as Quick start §2.3 line 13. Verified.
- Lines 5-6: `--mount` flag and `<host-path>:<container-path>` syntax verified at `_docs/usage.md:71` and `internal/cli/deploy_service.go:61-62`. The `:ro` suffix is omitted to keep the example minimal — `_docs/usage.md` §2 has the full mode-flag reference.
- Line 9 (`decloud status myservice`): verified at `internal/cli/status.go` exists and `_docs/usage.md:194` shows the output format.

### 2.5 Repository-layout block (verbatim)

```text
cmd/decloud/        # the decloud binary (single main package)
internal/           # private Go packages: cli, deploy, registry, caddy, dockerdrv, envcap, ...
_docs/              # user-facing documentation (install.md, usage.md)
_ai/                # decisions, conventions, agentic-development notes
_tasks/             # per-task workflow trail (planning, review, implementation reports)
CLAUDE.md           # contributor instructions (code style, agentic workflow)
tools.go            # pinned tool dependencies (gomock)
```

**Verification trail:** every line in the block was checked against `ls` of the repo root and the relevant subdirectory. The `internal/` enumeration is partial ("...") because listing all 11 subdirectories would defeat the purpose of a one-line orientation.

---

## Section 3 — `LICENSE` file: exact bytes Rob commits

**File path:** `/Users/fenster/dev/decloud/LICENSE` (no extension).
**File mode:** `0644` (git default; do not invoke `chmod` — let git store with normal mode).
**Encoding:** UTF-8, no BOM.
**Line endings:** LF only (`\n`), not CRLF.
**Trailing newline:** single `\n` at end of file (POSIX convention; `git diff` complains otherwise).

The exact text — Rob copies this verbatim into the file:

```
MIT License

Copyright (c) 2026 Alexander Fenster

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

**Source confirmation:** this is the canonical form distributed by https://choosealicense.com/licenses/mit/ — i.e., the form GitHub's "Add license" template uses, and the form GitHub's license-detection tooling looks for to display the "MIT License" badge in the repo header. The OSI page at https://opensource.org/license/mit publishes a near-identical text but omits the leading `MIT License` heading and uses `Copyright [year] [fullname]` without the `(c)`. The choosealicense.com / GitHub-template form is what every reader recognizes, what Don's plan §4 endorsed, and what the GitHub license-detector matches on. I am committing to the choosealicense form.

**Substitutions performed on the template:** exactly two.
- `[year]` → `2026`
- `[fullname]` → `Alexander Fenster`

**Substitutions NOT performed:** the body still says "the Software" and not "Decloud." This is correct — it's how every MIT-licensed project does it, every license-text linter expects it, and substitutions that say `THE DECLOUD IS PROVIDED "AS IS"` would (a) look ridiculous and (b) trip GitHub's license-detection regex.

**Year choice:** **2026**. Per `currentDate` 2026-04-29 in the harness, we are publishing in 2026. A single year is correct for a project whose first public release is happening now. Don't write `2024-2026` — there are no pre-2026 public releases to claim coverage of. Don't write `2026-present` — boilerplate that adds nothing for a project this young. If the project ships into 2027 and someone wants to update the year, the convention is to either change `2026` to `2026-2027` or to leave it; both are legally fine. Not our problem today.

**Copyright holder:** **Alexander Fenster**. Verified two ways:
1. `git log --format='%an %ae' | head -1` at the time of this writing returns `Alexander Fenster <github@fenster.name>` (per the recent commits `2c8aea9`, `28622d1`, etc.).
2. The user-context block in this conversation states "Git user: Alexander Fenster" and "user's email: alexander.fenster@gmail.com". The git author email used in commits (`github@fenster.name`) and the user's contact email (`alexander.fenster@gmail.com`) are different addresses for the same person; either is fine for a copyright line, and I am not putting an email in the copyright line at all because the canonical form doesn't require one.

**Why MIT and not Apache-2.0:** The user asked for MIT, explicitly, in the original task request (`01-user-request.md` line 3: "also, add MIT license"). Don's plan §4 last paragraph noted that an earlier internal planning doc had floated Apache-2.0 for patent-grant reasons; Don's call was to honor the user's pick. I agree with Don. User wins. MIT-to-Apache-2.0 relicensing is a one-commit change if the project ever needs patent grants and the maintainer is also the sole copyright holder; Apache-2.0-to-MIT in the other direction is harder. Choosing MIT is the lower-regret option.

---

## Section 4 — `_docs/install.md` §8 fix: bundle, don't punt

**My call: bundle.** Don's plan §10.1 noted I'd own this decision. Here is the defense.

### 4.1 The fix

`_docs/install.md` lines 212-214 currently read:

```markdown
## 8. License

This repository does not yet declare a license. If you intend to redistribute the binary or use it in a context that requires explicit license grant, ask the maintainer before doing so.
```

Replace with:

```markdown
## 8. License

Decloud is licensed under the MIT License. See the top-level [`LICENSE`](../LICENSE) file for the full text.
```

That is it. Two-line edit. Three minutes including the commit message.

### 4.2 Defense

I am bundling, not punting, for four reasons. Any one of them would be enough:

1. **Same theme.** This entire task is "the project's license story." Adding a LICENSE file and leaving the doc that talks about the absence of a license unchanged would be the textbook example of leaving a known inconsistency in the diff. `_ai/fix-now-while-fresh.md` is explicit about this pattern: when you touch a thing, the adjacent stale references to that thing get fixed in the same commit, because a follow-up task to fix them will lose 90% of the context.

2. **Cost is trivial.** Two lines changed. Same branch. Same review pass. Bundling adds zero days to the schedule. Punting would create a follow-up task with its own user-request file, plan, tech plan, Linus review, Kent (no-op for docs), Rob, Raymond, Kevlin, Linus, Don closeout, Joel closeout, Linus final, Ward, Andy — i.e., the full workflow overhead for a two-line edit. That ratio is absurd.

3. **Reader confusion is real.** A stranger reading `_docs/install.md` §8 today, after the LICENSE file lands, would see "this repository does not yet declare a license" two clicks away from the LICENSE file in the repo root. They would file an issue. We would explain. We would fix it then. Cheaper to fix now.

4. **Acceptance criterion #7 in Don's plan §9** already lists this fix as a required outcome: "`_docs/install.md` §8 is updated to reference the new LICENSE file (or this is explicitly punted to a follow-up with rationale)." Don asked me to defend, not to invent a new criterion. I am defending in favor of the first half of his disjunction.

### 4.3 What if a reviewer pushes back

The only argument for punting that doesn't fall to the four points above is "this widens scope from 'rewrite README + add LICENSE' to 'rewrite README + add LICENSE + edit install.md'." That argument is wrong because the install.md edit *is* part of the licensing story; it is not a separate concern. If we cared so much about scope-policing that we couldn't fix a doc inconsistency we created, we should not be doing the LICENSE add either. Linus: I expect you to either agree or tell me I'm wrong and I'll happily punt to a one-line follow-up if so.

### 4.4 Diff size after this task

After EXECUTION step:

- `README.md` modified (full rewrite, ~150 lines new vs. 279 lines deleted; net `-130` lines).
- `LICENSE` added (21 lines new).
- `_docs/install.md` modified (3 lines: 2 deleted, 2 added — Markdown allows the new text to fit in a single 2-line paragraph because the §8 stub is small).

Total: 2 files modified, 1 added. `git diff --stat main...task/readme-and-license` should show exactly that.

---

## Section 5 — Length budget rollup

Already covered in §2.1, repeated as a sanity check:

- README target: ~150 lines, ceiling 200.
- LICENSE: 21 lines (canonical MIT).
- `_docs/install.md` §8 net change: 0 lines (2-line replace).

The README ceiling matters because the value of a README is inversely proportional to how much of it the reader has to skip to find what they need. The 200-line ceiling is the threshold above which I would start cutting sections; the 150-line target leaves headroom for Rob's prose to breathe without forcing him to compress.

If during implementation Rob discovers a section ballooning past its budget:

- **Bloat in Project status (target 14 lines, ceiling 20):** delete the per-milestone parenthetical detail; lean on the Roadmap section's hyperlink.
- **Bloat in Architecture in 60 seconds (target 12 lines, ceiling 15):** cut paragraph 3 (the no-daemon paragraph); it's also stated in the Quick start section's link to `_docs/install.md`.
- **Bloat in Quick start (target 18 lines, ceiling 25):** if the shell block grows past 14 lines, something has crept in that doesn't belong. Revisit §2.3 and remove.
- **Bloat in any other section:** Linus call. None of the others are growth-prone.

---

## Section 6 — Open questions / risks / things that need Linus

The list, in priority order:

### 6.1 (For Linus) Project Status tone

Don asked Linus to review whether the "what doesn't ship" framing reads as a confident roadmap or as apologetic. My §1.2 wording leans toward confident — "Not yet shipped — see [Roadmap]" not "We haven't gotten around to..." If Linus reads it as apologetic, the fix is one of:

- Delete the "yet" from "Not yet shipped" (most common gripe).
- Reverse the order: list what's planned before what's shipped (I think this is wrong — readers want to know what works before what doesn't).
- Move the entire status section below Quick start (also wrong — readers about to type `go install` need to know the project's state before they spend time on it).

**My recommendation:** keep §1.2 as drafted. The "yet" stays. The order stays. If Linus disagrees, do option 1 (delete "yet"). I'll defer to him on the tone judgment because I deferred to Don's wording in §1.2 already and a tone-on-tone deferral chain isn't useful.

### 6.2 (For Linus) `--readiness-path` in the Quick start example

I left `--readiness-path` out of the Quick start `decloud deploy service` example. The default is `/healthz` (verified at `deploy_service.go:63`). The user's Dockerfile probably has a `/healthz` endpoint if they read the example carefully, but it's not guaranteed.

Two options:

- **Option A (chosen): omit `--readiness-path`.** Defaults to `/healthz`. Works if the example service has that endpoint. The user's first deploy might fail with exit 50 if it doesn't, and they'd then read `_docs/usage.md` and add `--readiness-path /` or whatever their app uses. That's a teachable failure, not a footgun.
- **Option B: include `--readiness-path /healthz` explicitly.** No behavior change but the user sees the flag and knows it exists.

I chose A because the README's job is the minimum-working command, not the explanatory one. `_docs/usage.md` §1 (line 47) DOES include `--readiness-path /healthz` explicitly because that's the explanatory doc. Linus: weigh in if you disagree.

### 6.3 (For Linus) Milestone label format

I went with `**M1** — Server-side service deploy ... (SHIPPED)` rather than `**M1** Server-side... [SHIPPED]` or a table. Don's plan §10.1 said he leans labelled list; I agreed. If Linus prefers a markdown table, the conversion is mechanical: column headers `Milestone | Description | Status`. I don't think it's better — narrow viewports squeeze tables — but it's a defensible alternative. Note: don't have Rob render BOTH and let the reader choose; pick one and ship it.

### 6.4 (Risk) Cross-link rot

The README includes ~6 inbound links to `_docs/install.md` (some with anchors like `#3-bring-up-caddy`) and `_docs/usage.md`. If Raymond renames a section or re-numbers a heading in EXECUTION step (he is not planning to per Don's plan §7), every anchor link breaks silently. GitHub's markdown rendering doesn't surface broken anchors at render time.

**Mitigation:** the README rewrite uses ZERO sub-anchor links into `_docs/`. Only top-of-file links (`_docs/install.md`, `_docs/usage.md`). This is a deliberate choice in the section drafts above — every `_docs/` link is bare-file, no `#anchor`. If a reader needs §3 of `install.md`, they scroll. Cheap.

### 6.5 (Risk) Quick-start `mkdir` command portability

The brace-expansion `mkdir -p /opt/decloud/{config/{services,jobs,caddy},secrets,state/deploys,logs}` works in bash 4+ and zsh, but not in `dash` (the default `/bin/sh` on Debian/Ubuntu). Operators ssh'd in and using `bash` are fine; if someone is in a `sh` script context, they would need the explicit form.

**Mitigation:** the README example is a copy-paste-into-an-interactive-shell scenario. Bash is the assumed login shell on Ubuntu/Debian. The full `_docs/install.md` §4 form is the explicit-line-by-line version; the README link to that doc covers the corner case. No further action.

### 6.6 (Risk) `go install ...@latest` requires that the maintainer pushes the repo public

The Quick start uses `go install github.com/alexander-fenster/decloud/cmd/decloud@latest`, which requires the GitHub repo at `alexander-fenster/decloud` to be public and the `cmd/decloud` package to be `go install`-able. As of the maintainer's recent activity (commits to `main`), this is presumably the case. If the repo is currently private, the README's Quick start would not work for someone other than the maintainer.

**Mitigation:** I am proceeding on the assumption that the repo is public. Verifying this would mean running `gh repo view alexander-fenster/decloud --json visibility` (a network call to GitHub). I am NOT doing this from the planning step — it would gate the plan on infra access I don't need to make a planning call on. Linus or Don should verify before merge if there's any doubt. If the repo is private at merge time, the README still works for the maintainer and the LICENSE still works for everyone; only the Quick-start `go install` line is misleading, and the fix is one line: `git clone && cd decloud && go install ./cmd/decloud`. Easy follow-up if needed.

### 6.7 (Resolved, flagged for the record) MIT license year

Don already settled this in his §8 ("Year on the LICENSE"). I am restating: **2026**, single year, no `-present`. If the maintainer wants to add 2027 in 2027, that's a one-line PR future-them.

### 6.8 (Resolved) Apache-2.0 vs MIT

Settled in Don's plan §4 and my §3. MIT. User wins. If anyone re-raises this, the answer is "user asked, user gets, decision-cost-of-changing-our-mind-now is higher than the value of a patent grant on a one-person Go tool."

---

## Section 7 — What I want from Linus

A focused review pass on:

1. **Tone of §1.2 (Project Status).** Confident or apologetic? My §6.1 above lays out the options.
2. **The bundled `_docs/install.md` §8 fix.** Defended in §4. Reject if you think it's scope creep — I'll punt.
3. **The MIT text in §3.** Specifically the choice of the choosealicense.com / GitHub-template form (with `MIT License` heading and `Copyright (c) ...`) over the bare OSI form. I argued the GitHub-template form in §3. If you have an opinion on the OSI vs. GitHub form, now is the time.
4. **The Quick-start shell block in §2.3.** Specifically the 1-line `mkdir -p` brace-expansion compression of `_docs/install.md` §4's seven-line `chmod` sequence. My argument is the README is the happy path and the linked doc has the full chmod story; if you want the README to repeat the chmod sequence, say so.
5. **The Roadmap as labelled list vs. table.** §6.3 above — pick one, I'll go with it.

Everything else is mechanical at this point. Rob's job after Linus signs off is copy-the-blocks-from-this-plan-into-the-files. No design decisions left.

---

## Section 8 — Acceptance criteria (cross-check with Don's §9)

After Rob's EXECUTION step, all of the following must be true. I'm reproducing Don's list and adding implementation-level checks:

1. README H1 is exactly `# Decloud`. **(Don §9.1)**
2. Project Status appears before any install instruction. **(Don §9.1)**
3. Quick-start commands run on a Linux host with Docker and Go would land at a working `decloud --help`. Specifically, the `go install ...@latest` URL resolves, the `decloud caddy up` command exists in the binary, and the `decloud deploy service` flag set matches `internal/cli/deploy_service.go`. **(Don §9.2, refined)**
4. Every flag named in the README appears in `internal/cli/*.go`. The flag names mentioned in the new README are: `--name`, `--host`, `--port`, `--mount`. All four verified at `deploy_service.go:57-62`. **(Don §9.3)**
5. Every milestone label (M1-M7) in the Roadmap matches `_ai/decisions/m1-scope.md` last paragraph and `_tasks/2026-04-28-milestone-resequence/002-don-plan.md:11-19`. **(Don §9.4)**
6. LICENSE is exactly the canonical MIT text, with `2026 Alexander Fenster` substituted, no other modifications. Mode 0644. LF-only line endings. Single trailing newline. **(Don §9.5, expanded)**
7. No README section implies a feature exists that hasn't shipped. Spot-check: search the rendered README for the strings `blue/green`, `restic`, `backup`, `gc`, `client binary`, `bootstrap`, `job` — every occurrence must be in either the Project Status "not yet shipped" list, the Roadmap, or a "see [Roadmap]" pointer. **(Don §9.6, with concrete test)**
8. `_docs/install.md` §8 references the new LICENSE file. **(Don §9.7)**
9. `git diff --stat main...task/readme-and-license` after EXECUTION shows exactly: `LICENSE` added, `README.md` modified, `_docs/install.md` modified, plus the task-directory files (`02-plan.md`, `03-tech-plan.md`, etc.). No other source-tree files touched. **(Don §9.8, expanded)**
10. README total length ≤ 200 lines. **(Joel §2.1)**
11. README contains zero anchor-deep links into `_docs/` (only file-level links). **(Joel §6.4)**

---

## Section 9 — What's NOT in this plan

Mirroring Don's plan §11, restating for the record:

- **Exact prose of every README section.** Rob's call within the structure I gave. If he produces a section that comes in 50% over the line budget, that's a review-cycle item, not a plan defect.
- **Whether code blocks use `sh` vs `bash` vs no language tag.** I specified `sh` in every block above; Rob can switch to `bash` if a reviewer prefers. Not load-bearing.
- **Whether the LICENSE file gets referenced from `go.mod`, the binary's `--version` output, or anywhere else.** Out of scope — user asked for `LICENSE` at repo root, that's what we deliver. Future task material.
- **Whether to add a `CHANGELOG.md`.** Not asked for. Out of scope. The agentic-team task trail in `_tasks/` is the de-facto changelog for this project.
- **Whether to git-tag M2 as `v0.2.0` or similar.** Out of scope; release engineering is its own task.

---

## Section 10 — Sign-off note for the next agent in the chain

**Linus:** review this against Don's `02-plan.md`. Three items I want your call on, listed in §7 above. If you sign off, the path is straight to Kent (no-op for docs-only task per the precedent in `_tasks/2026-04-28-milestone-resequence/002-don-plan.md` "Workflow: skip Kent and Rob, justified") and then Rob writes the three files per §1, §2, §3, §4 above.

**Rob:** if Linus signs off without changes, your work is mechanical:
1. Open `/Users/fenster/dev/decloud/README.md` and replace its entire contents with the structure in §1.1-§1.12, using the verbatim shell blocks from §2.3-§2.5 and the prose-structure budgets from §1.x.
2. Create `/Users/fenster/dev/decloud/LICENSE` with the exact bytes in §3.
3. Edit `/Users/fenster/dev/decloud/_docs/install.md` lines 212-214 with the replacement from §4.1.
4. `git add` only those three files. Verify with `git diff --stat main...HEAD`. Commit with a message that names all three changes.

**Raymond:** docs-only task; your role is to verify accuracy of the rewritten README against `_docs/install.md` and `_docs/usage.md` after Rob commits. Any drift between the README and the docs is a Raymond catch.

That is the tech plan. Ship it.
