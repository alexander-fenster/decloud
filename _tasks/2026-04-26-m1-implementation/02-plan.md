# M1 Execution Plan

**Author:** Don Melton (tech lead)
**Status:** Execution-tailored plan. Inputs: `01-user-request.md` + the three approved planning files in `_tasks/2026-04-26-readme-implementation-planning/`.
**Scope:** Plan the EXECUTION of M1 (CLAUDE.md Step 3). Don't restate Joel's approved tech plan — cite it.

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

Don's plan-v2 §7.1 acceptance criteria still bind. The two user-driven changes in §3 below modify the test plan and elevate two doc deliverables; everything else stands.

---

## 2. The two user constraints (load-bearing)

### 2.1 Unit tests only — integration tests deferred

The user said "I will test it on a real system after M1 is done." That moves Joel's tech plan §12.2 (`internal/dockerdrv/integration_test.go`, `internal/caddy/integration_test.go`, `internal/deploy/integration_test.go`, all `-tags integration`) **out of M1 execution**. Recorded in `_ai/decisions/m1-test-strategy.md` so future Don knows why this was skipped (it's not laziness; it's an explicit user directive).

What stays:

- All §12.1 unit tests with Testify + Gomock per CLAUDE.md item 4.
- The `internal/envcap` tests run against the real `/bin/bash` on the maintainer's box — these ARE unit tests (`go test ./...` runs them with no extra tags). The portability fix from plan-v2 §4.1 is the whole point; we're not skipping these. macOS bash 3.2 stays in the loop.

What changes about coverage discipline because integration is gone:

- **`internal/dockerdrv`** — Joel's §12.1 already lists "argument-construction tests" with an injectable `exec.Command` factory. Without integration, that factory MUST exist and MUST be exercised so we have evidence that `Build`/`Run`/`Stop`/`Remove`/`Inspect`/`Logs` produce the expected `exec.Cmd`. Rob: don't half-mock this.
- **`internal/caddy`** — without `caddy validate` in the loop, the generator's correctness is asserted by golden-string equality on a small set of canonical inputs (one host, multi-host, zero hosts dropped, empty input → just the stub). Document explicitly in `_docs/architecture/m1-recreate-strategy.md` that `caddy validate` will run during the user's real-system test, not in CI.
- **`internal/deploy`** — every step of §6.6's sequence (1–8) gets a unit test using Gomock's `Store`/`Capturer`/`Driver`/`Generator`/`Reloader`. One happy-path test, one test per failure branch. The recoverable "config without secrets" path (Joel §4.5) needs an explicit test. The "step 7 mid-write rollback" path (config wrote, secrets failed → roll the new container back) needs an explicit test.

### 2.2 Installation instructions + short usage docs ARE deliverables

Raymond produces TWO operator-facing docs as M1 deliverables. Both go under `_docs/` per CLAUDE.md.

#### 2.2.1 Installation instructions: `_docs/operator/installation.md`

Honest scope for M1's no-daemon, no-supervisor world:

1. **Prerequisites** — Ubuntu LTS, root access, DNS pointed at the host for any hostnames you'll deploy. (No "decloud daemon" because there isn't one.)
2. **Install Docker** — `apt install docker.io` or the official Docker Engine repo; `systemctl enable --now docker`. Verify: `docker run hello-world`.
3. **Install Caddy** — official repo install per Caddy docs; do NOT enable the default Caddy service. Instead, configure Caddy to run as a systemd service that points at `/opt/declouding/config/caddy/Caddyfile` (the path `decloud` will write to). Provide the exact systemd unit drop-in. Note: Caddy will fail to start until the first `decloud deploy service` runs because the file doesn't exist yet — explain that's expected, the first deploy creates the stub from tech-plan §7.2.
4. **Create the `/opt/declouding/` tree** — exact `mkdir -p` + `chmod` commands for `config/services/`, `config/jobs/`, `config/caddy/`, `secrets/` (mode 0700), `state/deploys/`, `logs/`. The bootstrap script in M2 will eventually automate this; for M1 the operator runs the commands by hand.
5. **Create the shared Docker network** — `docker network create decloud`. The deployer also self-heals via `docker network inspect ... || docker network create ...` per tech-plan §13.6, but creating it once explicitly during install means the first deploy doesn't appear to "do magic."
6. **Install the `decloud` binary** — `go install github.com/alexander-fenster/decloud/cmd/decloud@latest` OR build locally and copy to `/usr/local/bin/decloud`. Verify: `decloud --help`. **There is no `systemctl enable decloud` step** because M1 has no daemon — `decloud` is a one-shot CLI invoked by the operator over SSH. The README's "Declouding host-level supervisor" is M7 and Docker's `--restart=unless-stopped` covers the boring 90% in the meantime.
7. **Verify the install** — sanity check `decloud --help` returns the expected subcommands.

That is what "installation" means in M1. Raymond MUST NOT invent a phantom systemd unit for `decloud` itself — there is no decloud process to manage.

#### 2.2.2 Usage documentation: `_docs/operator/usage.md`

End-to-end walkthrough of deploying ONE service. Exact contents:

1. **What you need on the source dir** — a `Dockerfile` and (optionally) an `env.sh`. Show a minimal example of each. The example service is something boring like `nginx:alpine` serving `/usr/share/nginx/html` with one env var swapped at deploy time, so the reader can copy-paste.
2. **The `env.sh` model** — sourced at deploy time in a hermetic bash; whatever it `export`s ends up in the container's environment, never baked into the image. Document the borderline cases Linus flagged in `07-linus-review-v2.md`: `set +a` interaction, arrays, readonly. Mention `_ai/envcap-portable-bash.md` exists for engineers; the user-facing doc just states the rules.
3. **The deploy command** — `decloud deploy service --name foo --host foo.example.com --port 8080 --readiness-path /healthz <source-dir>`. Show the actual flag table (cite tech-plan §6.2) and which are required.
4. **What the deploy actually does** — the §6.6 sequence in plain English (build → stop old → run new → wait readiness → write registration → reload Caddy). Explicitly say "M1 has downtime during recreate; M4 will introduce zero-downtime blue/green."
5. **Observed result** — what the operator sees on stdout (live `docker build` output), stderr (progress lines), and after success (`docker ps`, `curl https://foo.example.com/`, the two TOML files on disk).
6. **The other subcommands** — short reference for `status`, `logs`, `start`, `stop`, `restart`, `unregister`, `caddy reload`. One-line each. Pointer to `_docs/cli/decloud-deploy-service.md` for the deep flag reference.
7. **Common errors** — the M1 exit codes from tech-plan §6.4. For each non-zero exit code, one sentence of "this is what it means and how to fix it." Include the exact wording for `--mount` rejection, `strategy=blue_green` rejection, schema mismatch, secrets file permission errors.

Both docs target an operator who has read the README and is now actually using the system. Concise, complete, no fluff.

---

## 3. Decisions on the operational deliverables (the ambiguity the user asked me to settle)

Joel's tech-plan §10 listed `go.mod` (Go 1.22), LICENSE (Apache-2.0), `.github/workflows/test.yml`, `slog`, `_docs/`, `_ai/`. Settling each:

| Deliverable | Decision | Reasoning |
|---|---|---|
| `go.mod` with `go 1.22` | **YES — ship in M1.** | Required to even compile. Module path `github.com/alexander-fenster/decloud`. |
| `LICENSE` (Apache-2.0) | **DEFER.** | The user did not ask for a license. Adding one is a real legal decision that the maintainer makes, not me. The README doesn't reference one yet. Rob does NOT add a LICENSE file in this task. Andy/maintainer can drop one in later as a one-line task; nothing in M1 depends on it. |
| `.github/workflows/test.yml` | **DEFER.** | The user did not ask for CI. We have no GitHub repo confirmed-public yet (the module path implies one but the maintainer hasn't said "this is hosted at github.com/alexander-fenster/decloud and CI runs there"). Adding a GitHub Actions workflow bakes in GitHub-specific assumptions for zero current benefit — Rob runs `go test ./...` locally, which IS the M1 acceptance gate. If CI is wanted later it's another small task. |
| `slog`-based structured logging | **YES — ship in M1.** | Tech-plan §9.3 spec is good. The `DECLOUD_LOG_TO_STDERR_ONLY=1` test escape hatch is mandatory so unit tests don't write to `/opt/declouding/logs/`. |
| `_docs/` deliverables | **YES — ship in M1**, with the §2.2 framing above (installation + usage are first-class user-facing docs, not nice-to-haves). Architecture and CLI reference docs from tech-plan §10 also ship. |
| `_ai/decisions/*.md` | **MOSTLY ALREADY EXIST.** Raymond verifies `m1-scope.md`, `secrets-split.md`, `schema-versioning.md`, `envcap-portable-bash.md`, `container-naming.md` all align with the implementation Rob ships. If Rob's implementation diverges from these, EITHER fix the implementation OR update the decision record (and explain why). Add `_ai/decisions/m1-test-strategy.md` (new) capturing the unit-tests-only directive for M1 so future Don doesn't think it was an oversight. |

---

## 4. Execution sequencing (Step 3 of CLAUDE.md)

CLAUDE.md mandates TDD: tests first (Kent), then implementation (Rob), then docs (Raymond), then reviews (Kevlin + Linus in parallel), then a planning re-check (Don/Joel/Linus).

Per CLAUDE.md, Joel ALSO writes a tech plan for the EXECUTION step before Kent starts. Joel's job here is small (most of the heavy lifting is already done in `06-tech-plan-v2.md`); his execution-tech-plan only needs to:

- **Confirm the package directory tree** from tech-plan §2.2 is what Kent should write tests against.
- **Spell out the gomock generation strategy** — `mockgen -source=internal/registry/store.go -destination=internal/registry/mocks/store_mock.go` etc. Pin the gomock version. Decide whether mocks live under `mocks/` subdirs or `internal/mocks/`. (My preference: `<pkg>/mocks/` so each package owns its own.)
- **Confirm the §12.1 unit test list is the complete coverage gate** for M1 given the integration-deferred constraint.
- **Spell out the small deltas** to §12.1 introduced by §2.1 above (the dockerdrv argument-construction discipline; the caddy generator golden-string discipline).
- **Re-confirm or revise** the operational deliverables decisions in §3 above. If Joel disagrees on LICENSE or CI, defend the position; I will reconsider, but the burden of proof is on Joel.

### Sequence

1. **Step 2b — Joel writes execution tech plan** (small additive thing on top of `06-tech-plan-v2.md`). Output: `03-tech-plan.md` in this task dir.
2. **Step 2c — Linus reviews this execution plan + Joel's execution tech plan.** Output: `04-linus-review.md`. If Linus rejects, iterate (back to Don, then Joel, then Linus).
3. **Step 3a — Kent writes failing unit tests** for every package in tech-plan §2.2, against the §12.1 list as amended by §2.1 of this plan. Output: code committed to `internal/<pkg>/*_test.go` plus a report in this task dir. Kent commits ONLY tests; the implementation files are stubs (empty function bodies returning `errors.New("unimplemented")` or similar) so the tests compile-and-fail. If Kent gets stuck, call Knuth.
4. **Step 3b — Rob implements** every package against Kent's failing tests until they pass. Output: code in `internal/<pkg>/*.go`, `cmd/decloud/main.go`, plus a report. Rob ALSO ships `go.mod` + `slog` initialization. If Rob gets stuck on the bash-3.2 portability of envcap or the two-file write rollback in deploy, call Knuth.
5. **Step 3c — Raymond writes docs IN PARALLEL with Step 3d/3e.** Output: `_docs/operator/installation.md`, `_docs/operator/usage.md`, `_docs/cli/decloud-deploy-service.md`, `_docs/architecture/m1-recreate-strategy.md`, `_docs/architecture/secrets-layout.md`. Verifies and reconciles `_ai/decisions/*.md`. Adds `_ai/decisions/m1-test-strategy.md`. Plus a report.
6. **Step 3d — Kevlin (low-level) reviews Rob's code IN PARALLEL with 3c/3e.** Per CLAUDE.md, Kevlin reviews API doc updates VERY carefully for hallucinations. Output: `<seq>-kevlin-review.md`.
7. **Step 3e — Linus (high-level) reviews IN PARALLEL with 3c/3d.** Output: `<seq>-linus-review-execution.md`.
8. **Step 2-redux — Don/Joel/Linus PLAN check.** Don re-reads everything and determines DONE / NOT-DONE. If not done, iterate.
9. **Step 4 — Ward preserves learnings.** Output: updates to `_ai/MEMORY.md` and any new `_ai/<topic>.md` files capturing implementation gotchas Rob discovered.

### Parallelism

3c, 3d, 3e all run in parallel after 3b finishes. Do not start 3a until 2c approves. Do not start 3b until 3a's tests are written and committed (they should fail; that's the point).

---

## 5. Where Knuth might be needed

Tech-plan §14 already flagged `internal/envcap/capture.go` and `internal/deploy/service.go` as the trickiest pieces. The user constraint of unit-tests-only adds two more risk areas:

- **`internal/dockerdrv` argument-construction tests** — without integration tests, the unit tests are the only thing proving the docker CLI args are right. Risk: tests confirm "we send these args" but the args don't actually work against real docker. Mitigation: cross-reference each constructed arg list against a hand-typed `docker run` command in a comment at the top of each test, so the reviewer can spot a typo without booting docker. If Rob can't get a clean shape for the injectable `exec.Command` factory, call Knuth.
- **`internal/deploy` orchestration with mocks** — the §6.6 step-7 mid-write rollback path is subtle (config wrote, secrets failed → roll new container back AND delete just-written config to avoid orphan). Easy to write a test that "passes" without actually exercising the rollback. Mitigation: assertions verify the EXACT mock call ordering (`InOrder` in gomock), not just call counts. If Rob's orchestration code grows past ~300 lines or the test setup boilerplate becomes painful, call Knuth before refactoring blindly.

---

## 6. DONE criteria for M1

Don/Joel/Linus sign off in the post-execution PLAN check (Step 2-redux) when ALL of these are true:

1. **`go test ./...` passes on macOS and Linux** without any `-tags` flag. Run on the maintainer's macOS box at minimum (no Linux CI in M1; the maintainer's laptop is the gate).
2. **Every package in tech-plan §2.2 exists** with the type shapes and behavior in §4/§6/§7/§9. `cmd/decloud/main.go` builds; `decloud --help` shows the subcommands from §6 and §9.2.
3. **`internal/envcap` tests pass on macOS bash 3.2** without skip — the v2 portability fix is verified by green CI, locally.
4. **The §12.1 unit tests, as amended by §2.1**, are all present and green. Coverage is by behavior, not line count — every failure branch in §6.6 has a dedicated test.
5. **The two user-facing docs exist:** `_docs/operator/installation.md` (per §2.2.1) and `_docs/operator/usage.md` (per §2.2.2). Both readable by an operator who has read the README; both honest about what M1 doesn't do.
6. **Architecture and CLI docs exist** per tech-plan §10's `_docs/` table.
7. **`_ai/decisions/*.md` files** are aligned with the shipped implementation. New `_ai/decisions/m1-test-strategy.md` captures the unit-tests-only call.
8. **`go.mod` declares `go 1.22`**, deps are `cobra`, `pelletier/go-toml/v2`, `testify`, `go.uber.org/mock`. No Viper. No license file (deferred).
9. **Loader rejection behavior is verified by tests** for: `ErrNotFound`, `ErrSecretsMissing`, `ErrPermissionMode` (file 0644, dir 0755), `ErrSchemaMismatch` (within file and across files), `ErrUnknownField`, `ErrMountsNotSupported`, `ErrInvalidStrategy`.
10. **The recoverable-state contract is verified by tests:** `TestStore_LoadConfigWithoutSecretsReturnsErrSecretsMissing` (Joel §12.1) plus `TestDeploy_StepSevenMidWriteFailureRollsBackContainer`.
11. **Container naming uses ONE helper** (`internal/ids/ContainerName`) per `_ai/container-naming.md` so M4 changes one function body, not every call site.
12. **Linus and Kevlin both APPROVE** their respective reviews. Don agrees the work matches plan + tech-plan + this execution plan. Joel agrees no spec gaps were papered over.

If any of 1–12 is missing, M1 is NOT done. Iterate per CLAUDE.md.

---

## 7. What I am explicitly NOT deciding here (for Joel and Linus)

- **Mockgen pinning version + mock layout** — Joel decides in `03-tech-plan.md`.
- **Whether the `internal/envcap` tests should additionally fake out `/bin/bash` for hermeticity reasons** — my call is no (the whole point is exercising real bash), but Joel can argue otherwise.
- **Whether `cmd/decloud/main.go`'s `logging.Init()` failure should be `ExitInternal` (70) or a separate `ExitLoggingInit` code** — tech-plan §2.3 says `ExitInternal`; I accept that. Mention only because it's the kind of nit Linus might surface.
- **Whether the M3a/M3b reservation fields (empty `Mounts` array) get serialized in M1's TOML output** — the strict-mode loader rejects `mounts = []` with the same error as `mounts = [{...}]`? Or does the loader accept the field's presence with an empty array? Tech-plan §4.8 reads "non-empty Mounts is rejected" so empty IS allowed. Joel: confirm in execution tech plan.

---

## 8. Final word

This is straightforward execution of an already-approved plan with two surgical user-driven changes (no integration tests; treat docs as first-class deliverables). Don't gold-plate. Don't reopen settled questions (Viper, schema versioning, secrets split, container naming). Ship M1, hand it to the user, let them break it on a real system, then plan M2.

Joel: write `03-tech-plan.md` next. Keep it tight — most of the work is in `06-tech-plan-v2.md` already; you're confirming the execution-time deltas in §2.1, §3, §4, §5, §7 of this plan.
