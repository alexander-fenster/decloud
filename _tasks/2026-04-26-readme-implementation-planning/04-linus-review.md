# Linus Review: M1 Plan + Tech Plan

**Reviewer:** Linus Torvalds (high-level review)
**Reviewing:** `02-plan.md` (Don) and `03-tech-plan.md` (Joel)
**User request being graded:** *"Discuss the documentation in README.md, carefully plan implementation steps, decide which part must be implemented first."*

---

## VERDICT: REVISIONS REQUESTED

Three things must be fixed before this goes to execution. Two are real (one is a security-correctness bug, one is a portability bug that will brick the dev loop on the maintainer's own laptop). The third is an internal contradiction between Don and Joel that will cause whiplash in M3. Everything else in the plan is either right or close enough to right that I would let it through. The bones are good. The skeleton has three broken ribs.

I want to be very clear up front: **the M1 cut is correct**. Don made the right call. Joel's tech plan is genuinely thorough and the package boundaries he picked are better than Don's opening bid. This is not a "send it back and start over" — this is "fix three specific things and ship."

---

## Did they answer the user's actual question?

The user asked for three things: (1) discuss the README, (2) decompose into implementation steps, (3) pick what to build first.

- Don's §1 (READ) does the discussion as a contract restatement, not an essay summary. Good.
- Don's §4 lays out M1–M7 with defended scope cuts per milestone. Good.
- Don's §3 picks M1 explicitly and rejects three alternatives with reasoning. Good.

Joel went well beyond "expand the plan" into a full M1 implementation specification (interfaces, exit codes, file layout, edge-case analysis). That is more than the user asked for, but it is also exactly what we need before Kent and Rob start work, so I am not going to penalize him for being thorough. The user asked for *planning*; Joel delivered *planning that's ready to execute*. That's a feature, not a bug.

---

## Answering the specific questions you both asked me

### Q1: Is "recreate-only in M1, blue/green in M4" throwaway work?

**No.** This is the question I expected to have to push back on, and after tracing the M1→M4 transition through Joel's interfaces, I am satisfied.

What changes M1→M4:
- `internal/deploy/service.go` orchestration sequence (start-then-stop instead of stop-then-start; flip Caddy between).
- Container naming convention (`decloud-<name>` → `decloud-<name>-<deploy-id>`).
- One new code path in `internal/caddy/` for admin-API PATCH.
- New `strategy = "blue_green"` accepted by the loader.

What does NOT change:
- The `Driver`, `Generator`, `Reloader`, `Store`, `Capturer` interface shapes.
- The TOML schema (because Joel reserved `Strategy` and `BuildSpec.ImageRef` already).
- The orchestration's overall failure-rollback structure.
- Anything in `internal/cli/`, `internal/config/`, `internal/ids/`.

The interfaces in §3.2/§3.3 of Joel's plan are shaped right. M4 is genuinely additive. The "throwaway" risk is contained to ~one file (`service.go` orchestration body) plus the rename of containers. Acceptable.

**The container naming question is the one real wrinkle.** M1 uses `decloud-<name>`; M4 uses `decloud-<name>-<deploy-id>`. Joel acknowledged this in §4.6 last paragraph. M4 will have to:
1. Detect M1-era containers (no deploy-id suffix) and either rename them (impossible — docker won't rename a running container without restart) or stop-and-recreate them under the new naming convention as part of the M4 upgrade.
2. Update the Caddyfile generator to point at the deploy-id-suffixed name.

That's a one-time M4 migration step. Fine. **Not throwaway, but DO write it down explicitly in M4's eventual tech plan as a known migration step.** Right now it's only mentioned as "flag it in code." That's not enough — it needs to be a tracked M4 deliverable, not an afterthought. Don should add a one-liner to M4's bullet in §4 of his plan: *"M4 upgrade includes a one-time recreation of all M1-era containers under the new naming convention."*

### Q2: No client binary in M1 — does it bake the wrong UX?

**No, with one caveat.**

Joel's read in his Q2 is correct: the M3 client will invoke `decloud deploy service --stdin --name foo ...` over SSH after `tar`-ing the source. The M1 surface is a strict subset of the M3 surface. The flag shape Joel picked accepts the source dir as a positional argument; M3 adds `--stdin` to read from stdin instead. Zero rework on the M1 server-side flag set.

**Caveat:** Joel's flag table in §4.2 uses `--env-file` defaulting to `<source-dir>/env.sh`. When the client tars the source dir and pipes it over SSH in M3, "the source dir" is now a stream, not a path. The default-to-`<source-dir>/env.sh` behavior either has to be re-implemented after extraction on the server, or the client has to be smart about extracting the source bundle to a tmpdir on the server first. Joel hasn't thought this through but neither has Don. **This is M3's problem, not M1's.** Just don't paint yourselves into a corner where the M1 flag shape can't handle "stdin source + env.sh inside the bundle." I think you're fine because M3 just extracts the bundle to a tmpdir on the server and runs the M1 logic against that path. Confirm this when M3 lands.

The bigger UX question — does M1 bias the *eventual* operator workflow? — answers itself: the README explicitly says "the server-side CLI is equally usable by a human who SSH'd in directly." The operator will use both modes (laptop client + direct SSH) for the lifetime of the project. M1 builds the SSH-direct path first. M3 layers the client on top. That's the right order because the client needs the server CLI to invoke; you can't do it the other way around without writing speculative client code.

### Q3: Container naming `decloud-<name>` for M1 vs `decloud-<name>-<deploy-id>` for M4

Already answered in Q1. Real but managed. M4 needs a one-time container-recreation step. Document it.

### Q4: Viper in M1 — premature?

**Yes, mildly.** This is the weakest call in Joel's plan.

Joel's defense: "two files (~50 lines), hooks `--config-root` to `DECLOUD_ROOT` env var, tests need it anyway." That defense doesn't hold up. Cobra alone supports flag-from-env binding via `viper.BindPFlag` is the Viper way, but Cobra has a simpler `cmd.Flags().StringVar(&v, "config-root", "/opt/decloud", "...")` plus reading `os.Getenv("DECLOUD_ROOT")` as a default in three lines of code. You don't need Viper for this.

The CLAUDE.md mandate is "YAML configuration with Viper." M1 has no YAML/TOML configuration that the operator edits. The TOML files in `/opt/decloud/config/services/` are written and read by `decloud` itself, not edited by humans, so they are NOT "configuration" in the Viper sense — they are persistent state.

That said: **this is not a blocker.** It's premature, but it's 50 lines that are easy to delete or keep. If Joel wants to wire it now to avoid retrofitting in M2 (when the bootstrap might genuinely add a `/etc/decloud/config.toml` for global settings), fine. I would rather see it deferred to M2 when there's an actual file for it to read, but I'm not going to fight on this one.

**Recommendation:** Defer Viper to M2. Use plain Cobra + `os.Getenv` for `--config-root` in M1. If Don and Joel disagree, I will not block on it. Note it as a known disagreement and move on.

### Q5: Writing source bundles to disk in M1 — premature or correct?

**Correct.** Keep it.

Joel's reasoning is solid: <50 lines, "what built this?" forensics for free, automatically backed up by M6 once M6 lands. The deploy-id concept exists in M1 anyway (it's the image tag suffix), so writing `state/deploys/<name>/<deploy-id>/source.tar.gz` is a trivial extension. The alternative — adding deploy-ids and source preservation later — would require retroactive renaming of existing containers and image tags. Build the muscle memory now.

---

## Issues I found on my own initiative — these are the actual problems

### ISSUE 1 (BLOCKER, security/correctness): The TOML registry stores plaintext env including secrets, in a world-readable file

**Problem:** Joel's §3.1 puts `Env map[string]string` directly into `/opt/decloud/config/services/<name>.toml`, mode 0644. The README's "Handling secrets" section explicitly says: *"`env.sh` and any deploy-provided secret files are stored on the host under `/opt/decloud/secrets/<service>/` with owner-read-only permissions."*

The README is unambiguous: env-vars-from-env.sh are secret-class data and live in `secrets/`, mode 0600. Joel's plan puts them in `config/`, mode 0644. **This is a direct violation of the README's secrets architecture.** Any service that does `export DATABASE_PASSWORD=hunter2` in its env.sh will end up with that password in a world-readable file under `/opt/decloud/config/services/foo.toml`.

**Impact:**
- Violates README contract.
- Creates a security regression that M3 will then have to migrate out of (move env from config TOML to secrets dir, schema bump).
- If anyone backs up `/opt/decloud/config/` separately from `/opt/decloud/secrets/` at any point in the project's lifetime (say, to share non-secret config in git per the README's deferred "Git-backed mirroring" idea), the secrets leak.

**Options:**

- **Option A (Minimal):** Set mode 0600 on `/opt/decloud/config/services/*.toml` and move on. Pros: one-line fix. Cons: violates README's structural separation of `config/` (non-secret) from `secrets/` (secret). The README's whole "Git-backed mirror of non-secret config" idea (mentioned as out-of-scope-but-easy-to-add) becomes hard because non-secret and secret are now mixed in one file.

- **Option B (Proper, RECOMMENDED):** Split the TOML into two files at write time. `/opt/decloud/config/services/<name>.toml` mode 0644 contains everything EXCEPT `env`. `/opt/decloud/secrets/<name>/env.toml` mode 0600 contains `env = { ... }`. Loader reads both and merges in memory. Pros: matches README architecture; M3 secrets work is additive (just add file mounts under `secrets/<name>/files/`); future git-mirror story works. Cons: two files per service; loader is slightly more complex; need to handle "secrets file missing" as a real error class.

- **Option C (Defer):** Document the limitation, ship M1 with everything in one mode-0600 file under `config/`, plan the split in M3 with a schema bump. Pros: ships M1 fast. Cons: M3 has to do data migration; we ship a known-violating M1 to production.

**My recommendation: Option B.** This is fundamental architecture, not optimization. Doing it right in M1 costs ~30 lines of additional code in `registry.Store`. Doing it wrong now costs a schema migration and a security audit later. The README is clear; honor it.

**DON: This is the one I most need you to weigh in on. Re-read the "Handling secrets" section of the README and decide whether Joel's single-TOML-with-env-in-it is acceptable or whether the split-file approach is required. My read: the split is required.**

### ISSUE 2 (BLOCKER, portability): `env -0` does not exist on macOS, breaking dev loop

**Problem:** Joel's `envcap.Capture` in §3.5 invokes `/usr/bin/env -i ... bash --noprofile --norc -c 'set -a; source "$1"; env -0'`. The `env -0` flag is a GNU coreutils extension. **macOS ships BSD `env`, which does not support `-0`.** When the Go process invokes `bash` and bash invokes `env -0`, it picks up `/usr/bin/env` from the new PATH — which on macOS is BSD env. The capture will fail with "env: illegal option -- 0" or similar.

Joel's test-skip plan in §6.1 says "Skip on Windows builders via build tag (we only target Linux+macOS for dev)." This is the bug: he targets macOS for dev but the implementation doesn't run on macOS.

**Impact:**
- The maintainer (you, on a Mac) cannot run the env capture unit tests locally without installing GNU coreutils via Homebrew and shimming `gnubin` into PATH. That's an undocumented dev-environment requirement that will bite every future contributor.
- The integration test that actually deploys a service end-to-end will fail on macOS dev machines.
- CI on Linux works fine, so this won't be caught until someone tries to develop locally and finds the test suite broken.

**Options:**

- **Option A (Minimal):** Document the GNU coreutils dependency in the README/contributing guide, add a `darwin` build-tag-gated skip on the env tests, accept that env capture is "production = Linux only, dev tested in CI." Pros: zero code change. Cons: maintainer-on-Mac can't iterate on env capture; the deploy command itself can't be smoke-tested on a Mac dev box.

- **Option B (Proper, RECOMMENDED):** Replace `env -0` with a portable bash construct. The cleanest is using bash builtins to enumerate exported variables and emit them NUL-separated, no external `env` needed:
  ```bash
  while IFS= read -r name; do
      printf '%s=%s\0' "$name" "${!name}"
  done < <(compgen -e)
  ```
  This works on bash 3.2 (macOS default) and bash 5+ (Linux), no GNU coreutils dependency. Pros: portable; same logic on all platforms; no Homebrew requirement for contributors. Cons: slightly more bash to template into the `-c` arg (still tiny); `compgen` is a bash builtin so we depend on bash specifically (which we already do).

- **Option C:** Use Go to do the enumeration after a `bash --noprofile --norc -c 'set -a; source <script>; declare -px'` invocation, parse `declare -px` output. Pros: avoids the bash printf dance. Cons: `declare -px` output format is bash-version-dependent and a worse parsing target than NUL-separated `KEY=VALUE`.

**My recommendation: Option B.** Use the `compgen -e` + `printf '\0'` approach. It's the same conceptual mechanism (NUL-separated KEY=VALUE) without the GNU dependency. Total change to Joel's plan: replace `env -0` with the `while/compgen/printf` block in two places. Five lines.

**DON: Verify on your Mac whether `/usr/bin/env -0` works (I'm 99% sure it doesn't on macOS, but check). Then approve Option B.**

### ISSUE 3 (REAL, internal contradiction): Don and Joel disagree on whether M3 bumps schema_version

**Problem:**
- Don, §4 M3: *"`schema_version` bumps to 2."*
- Joel, §3.1 commentary: *"Reserving the field shape now means M3 doesn't bump the version — it just starts populating empty fields."*

These are directly contradictory and the answer matters: it's the difference between "M1-era TOML files load cleanly in M3 binary" (Joel) and "M1-era TOML files require migration in M3 binary" (Don).

**Impact:** Whichever rule wins, it has to be enforced from day one in the loader logic. If Joel wins, the M1 loader's strict-mode rejection of unknown fields is the only forward-compat mechanism. If Don wins, M1 needs to write `schema_version = 1` and the M3 loader needs migration code.

**Options:**

- **Option A:** Joel wins. Reserve all M3 fields (`Mounts`, secrets-file declarations, etc.) in the M1 schema as empty defaults. M3 bumps no version, just starts populating. Pros: zero migration code ever; smoothest possible upgrade. Cons: requires us to accurately predict M3's schema during M1 design (we mostly can — mounts and secret-file mounts are well understood from the README).

- **Option B:** Don wins. M1 writes v1, M3 writes v2 with a migration path. Pros: each milestone owns its schema cleanly; no need to predict M3 fields now. Cons: requires migration code in M3 (read v1, transform to v2, atomic-rewrite); higher risk of bugs at the upgrade boundary.

- **Option C:** Combine: reserve fields where we can confidently predict shape (mounts), bump version when we need to change a field's *meaning* (e.g. moving env out of the config file per Issue 1 above). Pros: pragmatic. Cons: requires explicit decision-making per field at M3 time.

**My recommendation: Option C, leaning toward A.** Reserve the `Mounts` field shape now (Joel is right that we know what mounts look like). If Issue 1 is fixed via Option B (split files), then env-moving-to-secrets-file is also pre-decided, no M3 schema bump needed. M3 bumps the version only if it introduces a field whose *semantics* break old loaders, which seems unlikely given what M3 actually adds.

**DON: You and Joel need to reconcile this in writing before execution. Pick one, update both plans to match.**

---

## Smaller issues — fix if cheap, document if not

### Caddy lifecycle in M1 is implicit

**Problem:** M1 says "operator manually installs Caddy." But: how is Caddy configured to read `/opt/decloud/config/caddy/Caddyfile`? When the first `decloud deploy service` runs, is Caddy already running? Joel's `caddyCLIReloader` does `caddy reload --config <path>`, which requires Caddy to already be running with that config. The operator setup is undocumented.

**Recommendation:** Add a single paragraph to the M1 plan documenting the manual operator setup: *"Operator must install Caddy with a systemd unit pointing `caddy run --config /opt/decloud/config/caddy/Caddyfile` and ensure the file exists (start with an empty file or `decloud` writes one on first deploy)."* And: **the M1 deployer should write an empty/minimal Caddyfile if `/opt/decloud/config/caddy/Caddyfile` does not exist when the first deploy runs**, otherwise Caddy is started against a missing file and crashes. Cheap fix; prevents a confusing first-deploy failure.

### `--mount` flag rejected at CLI but `Mounts` field accepted by TOML loader?

**Problem:** Joel's §4.2 says `--mount` is rejected with "M3 only" error. But §3.1 says `Mounts []Mount` is in the schema (reserved for M3). What happens if an operator hand-edits `<name>.toml` to add a `[[run.mounts]]` block in M1?

**Recommendation:** M1 loader should reject non-empty `Mounts` with a "mounts not supported until M3" error to match the CLI behavior. Otherwise the schema-vs-CLI gap creates a "feature works partially through hand-edit" trap. Two-line fix in `registry.Store.Load`.

### The `cache/docker-network-created` sentinel file is over-engineered

**Problem:** Joel adds a sentinel file in `cache/` to avoid calling `NetworkEnsure` every deploy. This is premature optimization. `docker network inspect decloud >/dev/null 2>&1 || docker network create decloud` runs in <100ms and is idempotent. The sentinel can drift from reality (someone runs `docker network rm decloud` manually).

**Recommendation:** Drop the sentinel. Just call `NetworkEnsure` every deploy; it's cheap and self-healing.

### M3 is a fat milestone

**Problem:** Don's M3 bundles "client binary + env hardening + mounts + secret files + schema bump (per Don) + secret-files-on-disk infrastructure" into one milestone. That's at least three weeks of work and four distinct concerns.

**Recommendation:** Plan to split M3 into M3a (server-side: env hardening, mounts, secret files, secrets dir layout) and M3b (client binary + SSH transport) when M3's tech plan gets written. Not a blocker for M1 execution, but Don should be aware that M3 will likely subdivide.

### Caddyfile generator output for zero services is undefined

**Problem:** Joel's caddy template produces an empty file if no services have hostnames. Will Caddy accept `caddy reload --config /path/to/empty/file`? Probably yes (empty config = no routes), but untested. Worth one line in the integration test.

**Recommendation:** Add an explicit unit test: "generator produces a valid (Caddy-acceptable) Caddyfile when input is the empty slice." If Caddy rejects empty config, generate a no-op stanza like `:80 { respond 404 }` instead.

### env.sh capture: the "re-set to baseline value" limitation deserves a louder warning

**Problem:** Joel notes that if `env.sh` sets a variable to a value that happens to equal the baseline, it's dropped. He calls this "vanishingly rare." It's not — the most common case is `export PATH="$PATH:/opt/whatever"` where `/opt/whatever` was already in PATH from somewhere else, OR the baseline PATH already includes `/opt/whatever` because Joel hardcoded a long PATH in `runBash`. In particular: any env.sh that does `export PATH=$PATH` (no-op, but operators write it) is silently dropped.

**Recommendation:** Either accept this as a documented limitation in operator-facing docs (not just godoc), or change the baseline-diff to "anything bash printed AFTER sourcing the script that wasn't there before, OR was there with a different value." If Joel is using map difference, that's already what he does — re-read his code. Actually his code is `if bv, ok := baseline[k]; ok && bv == v { continue }` — that's "skip if baseline had the same key with the same value." If a script does `export PATH=$PATH`, baseline PATH equals new PATH, so it's skipped. Correct behavior, since nothing actually changed. The limitation only bites if the script *intends* to set a value that happens to match the baseline — extremely rare. Joel is right; my concern was unfounded. Keep as-is.

---

## Honest take on Joel's `env -i ... bash --noprofile --norc` env.sh capture

Aside from the macOS `env -0` portability bug (Issue 2 above), the design is **correct and well-thought-out**. The combination of:

- `env -i` for hermetic input
- `--noprofile --norc` to skip dotfiles
- `set -a` to capture unexported assignments
- baseline-diff to drop bash internals
- regex filter for valid var names
- NUL-separated parsing for newline-safe values

…is the right answer. This is not paranoid theater. Each element handles a real failure mode that would otherwise produce subtle, intermittent bugs:

- Without `env -i`: operator's SSH session env leaks into the captured env. Different operators get different env vars in the same service. Nightmare to debug.
- Without `--noprofile --norc`: operator's `~/.bashrc` runs at deploy time. Includes `alias ls='ls -G'` (harmless), `eval "$(direnv hook bash)"` (suddenly your env is direnv-controlled), `source ~/.aws/credentials` (you just exfiltrated AWS creds into the service env). Real, common, dangerous.
- Without `set -a`: operators who write `FOO=bar` (without `export`, common in env.sh files) silently get nothing. Worst kind of bug — the script "succeeds" but the var isn't set.
- Without baseline-diff: 7+ bash internals pollute every service's env. The container sees `BASH=/bin/bash`, `SHLVL=1`, `_=/usr/bin/env` etc. Confusing, sometimes harmful (apps that read SHLVL).
- Without NUL separation: any env var containing a newline (multi-line PEM keys, JSON blobs, etc.) gets split into multiple bogus vars. This is the single most common production env-capture bug; Joel correctly avoided it.

Once Issue 2 (portability) is fixed, this section of the plan is exemplary. **Tell Joel he did good work here.** It's the kind of detail that separates "we'll ship something that mostly works and bites us in six months" from "we'll ship something that handles the long tail correctly the first time."

---

## What's actually missing from the plan

These are things that would bite us in week one of M1 execution that nobody mentioned:

1. **`go.mod` initialization and Go version pin.** Joel says module path is `github.com/alexander-fenster/decloud` but never says which Go version. Pin to Go 1.22+ (for the `range int` and `slices` package usage that will inevitably creep in). One line; trivial; easy to forget.

2. **License file.** Greenfield project, no LICENSE file at root. Don't ship M1 without it. Probably MIT or Apache-2.0 since this is a personal tool, but the maintainer's call.

3. **CI configuration.** Joel's test plan §6.2 mentions "CI matrix: Linux runner with docker installed runs integration tests." But there's no GitHub Actions workflow in the plan. M1 should include `.github/workflows/test.yml` for unit tests at minimum. Integration test runner can come in M2 with bootstrap.

4. **The `_docs/` and `_ai/` directories.** CLAUDE.md and the workflow reference these but the plan never says what goes in them for M1. Raymond will need a target. Suggest: `_docs/cli/decloud-deploy-service.md`, `_docs/architecture/m1-recreate-strategy.md`, `_ai/decisions/m1-scope.md`. Don should add a "Raymond's deliverables for M1" section to the plan.

5. **Error logging / structured output to `/opt/decloud/logs/decloud.log`.** Joel mentions this file in the layout but never defines log format, library, or rotation. Use `slog` (Go stdlib since 1.21), JSON format, write to both stderr and the log file. Let Raymond document the format. Logrotate config is M2's bootstrap problem.

---

## Things done RIGHT (rare praise, but earned)

- **The package layout in Joel §1.2 is genuinely better than Don's opening bid.** Adding `internal/cli/`, `internal/envcap/`, `internal/ids/`, renaming to `internal/dockerdrv/` — all solid calls. The rationale for each split is sound.
- **The §4.6 "behavior on partial failure" analysis is excellent.** Most plans hand-wave failure modes. Joel walked through every step's failure case and wrote down the exact rollback. This is what separates plans-that-work from plans-that-don't.
- **The interface boundaries (Driver, Generator, Reloader, Store, Capturer) are correctly minimal.** No premature generality, no leaky abstractions. M4 will slot in cleanly.
- **The exit-code taxonomy (§4.4) is the right level of detail.** Specific enough to be useful in scripts; not so specific that every internal error needs a code.
- **Don rejecting "build the client first" is the correct call** and his reasoning is the right reasoning ("worthless until there is a server-side decloud for it to invoke"). Good architectural instinct.
- **Don rejecting "build jobs first" with the "services exercise more design risk" argument** is also correct. Building the higher-risk thing first when both are required is always right.
- **The "no bootstrap in M1" call** (manual install of docker/caddy/binary) is correct. Bootstrap is plumbing; the platform is the deliverable. Get the platform working, then automate the install.

---

## Summary of required revisions

Before this plan can go to execution, Don and Joel must:

1. **Resolve Issue 1 (env-in-config-toml secrets violation).** I recommend Option B (split into config TOML + secrets TOML at write time). Don makes the call.
2. **Resolve Issue 2 (macOS env -0 portability).** I recommend Option B (replace `env -0` with `compgen -e` + `printf '\0'` bash construct). Joel updates §3.5.
3. **Resolve Issue 3 (schema_version bump in M3).** Don and Joel pick one position and update both plans to match. I recommend Joel's position (reserve fields, no bump) provided Issue 1 is fixed via the split-file approach.

The smaller issues (Caddy lifecycle docs, mount field rejection in loader, sentinel file removal, M3 fat milestone awareness, empty-Caddyfile test, missing `go.mod`/LICENSE/CI/`_docs/` plans) should be addressed inline in revised plan documents but are not, individually, blockers.

After Don and Joel iterate on these, send the revised `02-plan.md` and `03-tech-plan.md` back for one more review pass. If the three blockers are addressed, I will approve.

End of review.
