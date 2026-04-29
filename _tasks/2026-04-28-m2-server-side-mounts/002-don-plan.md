# 002 — Don's plan: M2 server-side mounts

## TL;DR

M2 ships **`--mount` server-side, end-to-end**: flag accepted on `decloud deploy service`, loader accepts non-empty `Mounts`, runtime passes `-v` to `docker run`, registry round-trips. Schema shape doesn't change (locked by `_ai/decisions/schema-versioning.md:11`). Five surfaces (`--help`, CLI runtime, loader runtime, `_docs/usage.md`, roadmap) flip from "rejected, M2-future" to "accepted, M2-shipping" in lockstep.

**Env-file hardening is a phantom item.** I traced it. It's a residue from the M3a-bundle prose that survived the resequence and was carried forward as boilerplate. Killing it explicitly here so Joel doesn't expand a non-existent scope.

**Integration smoke test (m1x-backlog item 6) IS bundled into this task.** Reloader stderr `%q` quoting (also m1x-backlog item 6) is **PUNTED**. Argued below.

This is a real-code task. Full Kent-Rob-Raymond-Kevlin-Linus workflow applies. NO contingencies.

---

## 1. Exact M2 scope — what's IN, what's OUT

### IN

1. **`--mount` flag accepted** on `decloud deploy service`. Repeatable. Docker bind syntax `<host-path>:<container-path>[:ro]`. Validated client-side.
2. **Loader accepts non-empty `Mounts`** in `services/<name>.toml`. Same validation rules as `--mount` (the loader rejection becomes positive validation; can't trust the disk).
3. **Runtime passes `-v` flags** to `docker run` for the new container. Both Deploy (`internal/deploy/service.go:243-251`) and `Lifecycle.Start` absent-branch (`internal/deploy/lifecycle.go:67-78`) and `restoreOldContainer` (`internal/deploy/service.go:374-382`) — all three sites build `RunRequest`, none of them pass volumes today.
4. **Registry round-trip**: `Deploy` writes `Run.Mounts` populated with what the operator passed; `Load` reads it back; `Start`/`Restart`/`restoreOldContainer` use the loaded mounts to re-run the container. M1 left `Run.Mounts` reserved as `[]registry.Mount{}` (`internal/deploy/service.go:318`); M2 fills it.
5. **`ErrMountsNotSupported` deleted**, along with the three tests that asserted M1-era rejection (`internal/registry/store_test.go:296`, `internal/cli/deploy_service_test.go:81-95`, `internal/cli/exit_codes_test.go:24`). Replaced with positive tests asserting M2 acceptance and Docker-arg-shape lock.
6. **Help text, error wording, `_docs/usage.md`, the M2 substring contract**: all flip together. Details in §7.
7. **Integration smoke test** added under `//go:build integration` build tag, gated on `DECLOUD_INTEGRATION=1`. See §8.

### OUT — NON-NEGOTIABLE

- **No Viper, no global config file.** That is M3. `_ai/decisions/m1-scope.md:18` names this trap by name as the "Option C ad-hoc-loading trap." Don't even create a file like `internal/config/decloudconfig.go` "in preparation."
- **No secret-files-on-disk.** `secrets/<name>/files/` substructure stays unimplemented. M7. `_ai/decisions/secrets-split.md:6` says so.
- **No client binary.** M7.
- **No blue/green.** M4. The recreate strategy is unchanged.
- **No tmpfs, no `--mount type=tmpfs`, no anonymous volumes.** M2 is bind-and-named-volume only. Anonymous-volume support has zero operator demand and would force a `Mount.Source == ""` decoding rule that's a bigger feature than the value it returns.
- **No mount-options beyond `:ro`.** No `:z`, `:Z` (SELinux relabeling), no `:cached`/`:delegated` (macOS perf hints), no `--mount=type=bind,bind-propagation=slave` long-form. `:ro` is the only opt-in switch operators actually need on Linux production. Adding more options is pure scope creep.
- **No Docker `--mount` long-form syntax** (`type=bind,source=...,target=...,readonly`). Operator-facing flag accepts only the short `-v`-style syntax. We can add long-form later if anyone asks; nobody has.
- **No reloader stderr `%q` quoting fix.** Punted; argued in §9.
- **No env-file hardening.** Phantom. See §1a below.

### 1a. Env-file hardening: the phantom

The roadmap line at `_ai/decisions/m1-scope.md:32` reads "M2 server-side mounts + env-file hardening (`--mount` flag, loader populates `Mounts`)". The parenthetical names ONLY mounts work. "Env-file hardening" is loose prose that survived from when M3a bundled mounts + secret-files + env work into one milestone.

I traced what "env-file hardening" could plausibly mean today:

- **Trace A — read `internal/envcap/capture.go`.** The capture mechanism is portable bash 3.2+, hermetic (`env -i` + `--noprofile --norc`), uses `compgen -e` + `${!name}` + `printf '\0'` — exactly what `_ai/envcap-portable-bash.md` documents as the M1-stable design. There is no known bug. Tests pass on macOS bash 3.2 and Linux bash 5+ per `_ai/decisions/m1-test-strategy.md:18`.
- **Trace B — read `_ai/m1x-backlog.md` items 1-8.** Item 4 is the only env-file-adjacent backlog entry: a comment-only clarification on `Capture("")` (`internal/envcap/capture.go:46-49`). Three-line block comment. Not "hardening" by any normal use of the word.
- **Trace C — read `_tasks/2026-04-28-milestone-resequence/`.** Don/Joel/Linus signed off on the M2 scope as `--mount` + the prose phrase "env-file hardening" without ever defining what the latter contained. The phrase came forward from the M3a-bundle prose and nobody pruned it.
- **Trace D — grep `internal/envcap` for TODOs or FIXMEs.** None.

**Verdict: there is no env-file hardening work in M2.** The phrase is a phantom. I am calling it dead, in writing, here. If a real env-capture issue surfaces during M2 implementation (Kent's tests, Rob's bench, Linus's review), it gets logged as a new m1x-backlog item or its own future task — NOT folded into M2 retroactively.

**Action for Raymond at the doc-update step:** strip "env-file hardening" from `_ai/decisions/m1-scope.md:32` and the corresponding `MEMORY.md:7` line as part of the "M2 has shipped, update tense" sweep. The roadmap line becomes just "M2 server-side mounts (`--mount` flag, loader populates `Mounts`)". This is fix-while-fresh on stale prose, same shape as `install.md:121` was in the resequence task.

---

## 2. `--mount` flag syntax

### Decision: Docker `-v`-style short syntax. Bind mounts AND named volumes. `:ro` opt-in.

```
--mount /host/path:/container/path        # bind mount, read-write
--mount /host/path:/container/path:ro     # bind mount, read-only
--mount mydata:/var/lib/app               # named volume, read-write
--mount mydata:/var/lib/app:ro            # named volume, read-only
```

Repeatable. Each occurrence emits one `-v` flag in `docker run`, in declared order (matching how `RunWithOptions.Volumes` already works at `internal/dockerdrv/cli_driver.go:235-237`).

### Why this syntax (not `--mount type=bind,source=...,target=...`)

Docker's modern `--mount` long-form is more explicit but verbose. The `-v` short-form is what `docker run` documentation has used for a decade and what every operator already knows. Joel's tech-plan call: which form is cited in `_docs/usage.md` examples — long or short? Short, because the Caddy `RunWithOptions` fixture at `internal/dockerdrv/cli_driver_test.go:519-523` already produces short-form output (`/host:/dst:ro` and `vol:/dst`), and operators copying examples from `caddy up` will pattern-match.

**Naming the flag `--mount`** (not `--volume` / `-v`) is locked by the help text and roadmap; we don't get to rename. Don't shorten to `-v` either — short-flag namespace pollution for a flag operators run once per service is wrong.

### Validation rules

Both `--mount` parser and loader (`internal/registry/store.go:Load`) apply the SAME rules. Any divergence is a security hole — the loader is the second line of defence against someone who hand-edits a TOML and bypasses the CLI.

1. **Format**: must split on `:` into 2 or 3 components. 0, 1, or 4+ components → `errUsage` (exit 2 from CLI; loader treats as `ErrInvalidMount` exit 10).
2. **Container path**: must be absolute (`filepath.IsAbs(containerPath)`). Empty or relative container path → reject. Operators have no business mounting at relative paths inside a container.
3. **Source**:
   - Starts with `/` → bind mount. Source path must be absolute (it already is by being `/`-rooted). MUST exist on host at deploy time. (Joel decides: stat the source dir, reject if missing? My lean: yes. Mounts that don't exist will break the container at startup; failing fast at deploy time is RIGHT. But Joel has the call.)
   - Does NOT start with `/` → named volume. Source must match Docker's volume-name regex (`[a-zA-Z0-9][a-zA-Z0-9_.-]+`). Reject empty source explicitly.
4. **Mode flag** (third component): only `ro` accepted. Anything else (`rw`, `Z`, `z`, `cached`, ...) → reject with explicit message naming the offender.
5. **No duplicate container paths** in the same service's mount list. Two mounts targeting the same `/foo` is silently last-wins in Docker; we reject explicitly. Container-path uniqueness is the natural primary key.

### What we don't validate (deliberately)

- **Source path "exists" beyond stat**: don't check ownership, don't check it's a directory vs a file, don't check perms. Bind mounting a file (e.g., `/etc/timezone`) is legitimate.
- **Container-path conflict with `Dockerfile` `WORKDIR` or other in-image paths.** Operator's responsibility.
- **Named volume "exists"**: Docker auto-creates named volumes on first `docker run -v name:/path`. We don't pre-create.

### Error sentinel

Add `ErrInvalidMount` to `internal/registry/errors.go`. CLI wraps via `errUsage` for command-line input (exit 2) — actually, let me check, because the M1 pattern was different.

Tracing M1 precedent at `internal/cli/deploy_service.go:71-78`: `--mount` was wrapped with `registry.ErrMountsNotSupported` (exit 10), but `--port` zero-value was wrapped with `errUsage` (exit 2). The two flags were treated as different *classes* of error: "M1 doesn't accept this at all" vs "you used the flag wrong." For M2, malformed `--mount` is the latter class — exit 2 (`ExitUsageError`) for CLI-side, exit 10 (`ExitConfigError`) for loader-side rejection of a hand-edited TOML. Joel locks the exit-code split in his tech plan.

---

## 3. Loader behaviour

### Today (M1 rejection)

`internal/registry/store.go:68-71`:

```go
if len(cfg.Run.Mounts) > 0 {
    return nil, fmt.Errorf("%w: service %q declares %d mount(s) in %s; mounts are not supported until M2",
        ErrMountsNotSupported, cfg.Name, len(cfg.Run.Mounts), cfgPath)
}
```

### M2

Replace the "non-empty Mounts is an error" branch with **per-mount validation**. Same rules as §2. Loader's job is to reject malformed disk state — not just absent state.

Pseudocode:

```go
for i, m := range cfg.Run.Mounts {
    if err := validateMount(m); err != nil {
        return nil, fmt.Errorf("%w: service %q mount[%d] in %s: %w",
            ErrInvalidMount, cfg.Name, i, cfgPath, err)
    }
}
```

The `validateMount` helper lives in `internal/registry/` and is shared with the CLI parser via export. (Joel: where exactly — `internal/registry/mount.go`? Or a new `internal/mount/` package? My lean: `internal/registry/mount.go`. The validation rules are part of the registry's data-integrity contract, and the CLI imports `registry` already. A separate package is overkill for one type and one func.)

The CLI's `--mount` parser converts the string forms into `[]registry.Mount` and then calls the same `validateMount` for each. Single source of truth for what constitutes a valid mount.

### What `ErrMountsNotSupported` becomes

**Deleted.** `internal/registry/errors.go:11`, the wrapping at `internal/cli/deploy_service.go:72`, the loader rejection at `internal/registry/store.go:68-71`, the exit-code mapping at `internal/cli/exit_codes.go:41`, and the three tests asserting M1 rejection — all gone.

This is the M1-cut undoing pattern documented at `_ai/decisions/secrets-split.md:24` ("`ErrMountsNotSupported` (M1)") — that line is itself stale post-M2 and gets fixed-while-fresh as part of Raymond's sweep.

---

## 4. Schema — `Run.Mounts` shape

Already correct. Verified at `internal/registry/types.go:52-63`:

```go
type RunSpec struct {
    Network string  `toml:"network"`
    Port    int     `toml:"port"`
    Restart string  `toml:"restart"`
    Mounts  []Mount `toml:"mounts"`
}

type Mount struct {
    HostPath      string `toml:"host_path"`
    ContainerPath string `toml:"container_path"`
    ReadOnly      bool   `toml:"read_only"`
}
```

**No shape change.** The `schema-versioning.md` "shape doesn't change between milestones" promise (locked by `_ai/decisions/schema-versioning.md:11`) holds. M2 still writes `schema_version = 1`. An M1-era TOML with empty `[run]` mounts (or zero `mounts` array) loads cleanly in M2 binary; an M2 TOML with populated mounts loads cleanly in any future binary that hasn't bumped the schema version.

**One gap to flag for Joel**: the `Mount` type has no `IsNamed` field, but the driver's `dockerdrv.VolumeMount` does (`internal/dockerdrv/driver.go:66-71`). The on-disk `Mount.HostPath` is named `host_path` — confusingly so for named volumes (where the source is a volume name, not a host path). Two clean options:

- **Option A**: keep `HostPath` field name on disk (don't change the TOML key — that WOULD be a shape change), but rename the Go field to `Source` and let the TOML tag stay `host_path`. The disk format is locked; the in-memory accessor name can be cleaner.
- **Option B**: treat `HostPath` semantically as "source" — derive `IsNamed` at conversion time by checking `strings.HasPrefix(HostPath, "/")`.

I lean Option B. It avoids any rename churn, keeps the on-disk name stable (it WAS named `host_path` because M1's reservation assumed bind mounts only — that's a small papercut, not a real bug), and the IsNamed determination is one helper. But Joel has the call. This is the kind of subtle nomenclature choice that decays into 2-AM-debugging fodder if we get it wrong.

**Either way, no schema_version bump.** The escalation rule at `schema-versioning.md:20` only fires for *semantic breaks*. Adding an `IsNamed`-derived Go field is not a semantic break.

---

## 5. Runtime behaviour — passing `-v` flags

Currently the deploy and lifecycle paths use `Driver.Run(RunRequest)` (no volumes). The driver also exposes `RunWithOptions(RunOptions)` which DOES support `Volumes []VolumeMount`. Two clean options for Joel:

### Option α: switch service deploys to `RunWithOptions`

The Caddy manager already uses `RunWithOptions` (`internal/dockerdrv/cli_driver_test.go:504-525`). Service deploys join the same path. Drop `RunRequest`'s `Run` method and forward the existing call sites to `RunWithOptions`. Cleaner long-term — there will be ONE run path in the driver instead of two.

### Option β: add a `Volumes []VolumeMount` field to `RunRequest`

Smaller diff. Both methods coexist. `RunRequest.Volumes` is forwarded to the same `formatVolume` helper.

I lean **α** for the same reasons we don't keep parallel error-wrapping paths: divergence is a bug factory. The Caddy manager and the service deployer share the underlying docker run; making them share the same Driver method is structurally honest.

But α is a bigger blast radius — it touches `Driver.Run` (and `cliDriver.Run` and the gomock'd `MockDriver.Run`) which is called from three sites in `service.go`/`lifecycle.go` and re-mocked in every test in `internal/deploy/`. Joel decides whether the increased Kent/Rob load is worth the cleanup, or whether β's narrower diff is the right call for THIS milestone (with α punted to a future cleanup task).

If Joel picks β, the explicit punt-to-future-α observation goes in his tech plan as a follow-up so we don't lose track.

### Three call sites that build RunRequest today

All three need to learn about mounts:

1. **`internal/deploy/service.go:243-251`** — the new-container run on a fresh deploy. `runReq.Volumes` populated from `req.Mounts` (after CLI parses and validates).
2. **`internal/deploy/service.go:374-382`** — `restoreOldContainer`. `runReq.Volumes` populated from `prev.Config.Run.Mounts` (loaded from disk).
3. **`internal/deploy/lifecycle.go:67-78`** — `Start` absent-branch. Same as #2: `prev.Config.Run.Mounts` from disk.

All three need new tests.

### `deploy.Request` shape change

Add `Mounts []registry.Mount` field to `deploy.Request` (`internal/deploy/service.go:52-62`). The CLI populates it after parsing+validating `--mount` strings.

---

## 6. `ErrMountsNotSupported` removal

### Files where `ErrMountsNotSupported` appears today

```
internal/registry/errors.go:11             — definition
internal/registry/store.go:69              — wrap site (loader)
internal/cli/deploy_service.go:72          — wrap site (CLI)
internal/cli/exit_codes.go:41              — exit-code mapping
internal/cli/exit_codes_test.go:24         — exit-code mapping test
internal/registry/store_test.go:296        — loader rejection test
internal/cli/deploy_service_test.go:81-104 — two tests: runtime rejection + help-text M2 substring
```

### What flips

- **Definition**: deleted.
- **Loader**: replaced with per-mount `validateMount` per §3.
- **CLI**: deleted (just don't reject the flag at all).
- **Exit-code mapping**: deleted entry. NOT replaced — `ErrInvalidMount` (the new sentinel for malformed mounts) maps to `ExitConfigError` for loader-side and `ExitUsageError` for CLI-side, both via the existing wrapper paths.
- **Tests**: `TestDeployService_MountFlagReturnsErrMountsNotSupported` (`deploy_service_test.go:81`) deleted. Replaced with `TestDeployService_MountFlagAccepted_PassesMountsThrough` (positive: mounts make it into the deploy.Request and into the Driver.Run/RunWithOptions arg-shape). `TestStore_LoadRejectsNonEmptyMounts` (`store_test.go:296`) replaced with `TestStore_LoadAcceptsValidMounts` and `TestStore_LoadRejectsInvalidMounts` (positive + negative). Exit-code test entry removed.

The principle: each test that asserted a M1-era cut becomes a test that asserts the M2-era acceptance + validation. The lines counted in the diff balance — we don't lose test coverage, we INVERT it.

---

## 7. The "M2" substring contract in `--mount` help text

`TestDeployService_MountFlagHelpReferencesM2` at `internal/cli/deploy_service_test.go:97-104` asserts the help text contains `"M2 only"`. The semantic-token carve-out at `_ai/cli-flag-surface-coherence.md:32-42` allows this — it's a multi-surface coherence check, not prose-snapshot.

### Once M2 ships, what does the help text say?

Two reasonable answers:

- **Plain**: `"persistent volume (host:container[:ro] or volname:container[:ro], repeatable)"`
- **Future-pointing**: `"persistent volume (host:container[:ro] or volname:container[:ro], repeatable; long-form --mount=type=... is M-future)"`

Plain wins. We don't pre-document M-future syntax that nobody has asked for — that's the same trap as M3a/M3b prematurely-bundling. If/when long-form lands, the help text gets updated then.

### What does the test become?

**Deleted.** With M2 shipping, the milestone-token assertion has no contract to lock — `--mount` is accepted, no future-milestone tense remains. The semantic-token carve-out exists *because* there's a milestone token whose coherence across surfaces matters; once the feature ships, there's no token, no carve-out applies, no test.

We don't replace it with a help-text test for the new prose, per the change-detector rule (`_ai/cli-flag-surface-coherence.md:24-31`). The four-surface coherence (runtime check, error message, `--help` text, `_docs/usage.md`) is now enforced by review discipline, not test.

### Renaming / cleanup

`TestDeployService_MountFlagHelpReferencesM2` is the test name. Just delete it. Don't try to "update" it — there's nothing to assert.

### Surface audit

Per `_ai/cli-flag-surface-coherence.md` — when M2 ships, ALL FOUR surfaces flip together:

1. **Runtime check** at `internal/cli/deploy_service.go:71-73` — replaced with parse-and-validate.
2. **Error message** — `ErrMountsNotSupported` deleted; `ErrInvalidMount` for malformed mounts.
3. **`--help` text** at `internal/cli/deploy_service.go:61` — flipped from "M1: rejected with ExitConfigError (M2 only)" to the plain volume description above.
4. **`_docs/usage.md:71`** — flipped from "Rejected with exit 10 in M1. Persistent volumes are M2." to "host:container[:ro] or volname:container[:ro], repeatable. Bind mounts, named volumes." Plus update of `_docs/usage.md:99` (the exit-code table mentions `--mount used` as a `ExitConfigError` cause; that line gets pruned).

Joel: lock exact wording in the tech plan. Five surfaces for the FLIP — not four — because the semantic-token contract ALSO required one helper test (`TestDeployService_MountFlagHelpReferencesM2`) which is the fifth surface that disappears.

---

## 8. Integration smoke test — m1x-backlog item 6 — BUNDLED

`_ai/m1x-backlog.md` item 6 says the integration smoke test belongs to "the next post-M1 milestone where we touch real Docker for the first time (the new M2 — server-side `--mount` ...)". That's THIS milestone.

### Argument for bundling

1. **Real-Docker volume semantics are exactly what unit tests can't cover.** The argv-shape tests at `internal/dockerdrv/cli_driver_test.go:405-431` lock the `-v /host:/dst:ro` syntax byte-for-byte, but they can't tell us whether `docker run -v /tmp/test:/data:ro nginx` actually mounts the host path successfully on the maintainer's machine. The integration test fills exactly that gap, and `--mount` is the first feature in the codebase where the gap matters.
2. **Cleanup-on-interrupt + bind-mount-leftover is a real failure mode.** A deploy that fails mid-run with a bind mount attached can leave `/host/path` modified (the container ran and wrote to it). The integration test exercises the cleanup path with mounts present.
3. **Skipping it kicks the can to M3.** M3 is host bootstrap — even less Docker-volume-shaped than M2. The integration-test backlog drift compounds.

### Argument against (steelmanned)

- "M2 already has a big enough scope: flag + loader + runtime + tests + docs."
- "Integration tests are slow and require Docker. They'd add CI infrastructure work."
- "The maintainer can run a manual smoke test — that's the M1-test-strategy bridge per `_ai/decisions/m1-test-strategy.md:23-38`."

### Resolution

I'm bundling. Reasons:

- The build-tagged opt-in (`//go:build integration` + `DECLOUD_INTEGRATION=1`) means it doesn't slow `go test ./...` for anyone. Default test run unaffected.
- The maintainer's manual smoke test is the M1 bridge. M2 onwards we should be inverting that bridge — automated tests catching real-system bugs before the maintainer does. M2 is exactly the right time to land the first one.
- "Big scope" arguments killed M3a. We're not doing that again. Cut secret-files and the client binary and Viper to keep M2 focused on `--mount`; the integration test is in-scope because it's the verification mechanism for the feature, not a separate feature.

### Test shape (Joel locks specifics)

- New file: `internal/integration/mount_test.go`. Build tag `//go:build integration`. Skip if `DECLOUD_INTEGRATION` env var not set.
- `t.Setenv` for hermetic config root.
- Bring up `decloud caddy up`. Assert no error.
- Build a tiny test image (`alpine` + `/bin/sh -c 'cat /data/marker; sleep 60'`) — no `Dockerfile` flavors needed beyond what M1's deploy already supports.
- Create a host directory under `t.TempDir()`, write a marker file in it.
- `decloud deploy service --name=mounttest --mount=<tmpdir>:/data:ro --port=80 ...` against the real daemon.
- Assert container is running, `docker exec decloud-mounttest cat /data/marker` returns the expected bytes.
- `decloud unregister mounttest`.
- `decloud caddy down`.
- Cleanup: `t.Cleanup` with idempotent `docker rm -f decloud-mounttest` and `docker rm -f decloud-caddy` so a panicked test run leaves nothing behind. Critical — see m1x-backlog item 6 §"Fix shape": "Cleanup must be idempotent".
- The test does NOT exercise Caddy ingress (no curl-through-Caddy step). Reason: the m1x-backlog item described both a curl-through-Caddy assertion AND a `--mount` assertion in one test. Splitting them simplifies the test. M2 ships the `--mount` half. The curl-through-Caddy half can be a separate `internal/integration/ingress_test.go` in the same task or punted — Joel's call.

### Why not punt the integration test to its own task post-M2

Considered. The case for punting: keeps M2 focused on the feature. Case against: the feature isn't really "shipped" if we don't have a real-Docker test exercising it. Splitting "ship the code" from "verify it works on real Docker" is exactly what got us into the M1 manual-smoke-test pattern. Land them together this time.

If during execution Kent / Rob / Joel discover the integration test setup is more than a 1-day diversion, we re-plan in the closeout PLAN re-entry — but I don't expect that. The shape is straightforward.

---

## 9. Reloader stderr `%q` quoting — m1x-backlog item 6 — PUNTED

m1x-backlog item 6 §"Fix shape" mentions "revisiting reloader stderr `%q` quoting" as something M2 (the integration-test milestone) would naturally cover.

I'm punting it. Reasons:

1. **It's not coupled to mounts.** The `%q` issue is at `internal/caddy/reloader.go:72` — `fmt.Errorf("caddy %s: %w; stderr=%q", sub, err, stderr.String())`. The `%q` works for ASCII stderr but escapes Unicode in a way that may or may not match what operators want to see. It's a logging-formatting decision in the caddy reloader — entirely orthogonal to volume mounts.
2. **The integration test will exercise the reloader** (the `caddy up`+deploy+`caddy reload` sequence is in scope), but that's the only coupling. Whether `%q` is the right format string for the stderr string is a separate question that the integration test doesn't help answer (the assertion would be on `error == nil`, not on the formatting of the stderr-included error string).
3. **Scope creep risk.** If we touch caddy reloader formatting, Linus will (correctly) ask "did you audit the OTHER `%q` sites in the reloader?" (`reloader.go:69` and `:80` both use `%q`). That's a separate task.

**Action**: leave `_ai/m1x-backlog.md` item 6 in place after M2 ships, BUT update the wording to remove the bundling implication. Item 6 becomes "integration smoke test" only; the reloader `%q` revisit is split into its own backlog entry (item 9) so a future-Don picking it up doesn't have to disentangle them. Raymond does this update as part of the M2 docs sweep.

---

## 10. Decisions for Joel

These are the design choices Joel needs to lock in his tech plan before Linus reviews. I have leans on most; Joel can override with argument.

1. **Mount source-existence check at deploy time?** My lean: yes, stat bind-mount sources, reject missing. Fail-fast > broken-container-at-startup. Joel: confirm or counter.
2. **Where does `validateMount` live?** My lean: `internal/registry/mount.go`. Joel: confirm or propose alternative.
3. **`Mount` Go-field rename or `IsNamed` derivation?** My lean: derive (Option B in §4). Joel: pick A or B.
4. **`Driver.Run` consolidation (Option α) or new `RunRequest.Volumes` field (Option β)?** My lean: α for cleanliness. Joel: weigh against blast radius.
5. **Exit-code split for malformed `--mount`**: `ExitUsageError` (CLI-side) vs `ExitConfigError` (loader-side)?
6. **Exact `--mount` help-text wording**.
7. **Exact `--mount` error wording** (CLI parse failure, loader validation failure — should they share a wording template?).
8. **Integration test: does the M2 task ship ONE integration test (mounts only) or TWO (mounts + curl-through-Caddy)?** My lean: one. Joel: argue or accept.
9. **Reloader `%q` issue: confirm the punt and decide whether m1x-backlog item 6 split into items 6 (integration) and 9 (reloader fmt) is the right re-shape.**
10. **`ErrInvalidMount` exact wording** — must name the offending mount index, the offending component, and the path being parsed (operator-debugging context).
11. **Stat the bind-mount source path during loader validation, or only during CLI validation?** My lean: only CLI (loader-time stat fails on `decloud start` for a service whose source-dir was un-mounted overnight, which is a recoverable state). Joel: weigh.
12. **The "no duplicate container paths" rule (§2 rule 5): is that a hard reject or a warn-and-pick-last?** My lean: hard reject. Joel: confirm.

---

## 11. Workflow contingencies

**None.** This is a straightforward code task. Full Kent (write tests first) → Rob (implementation) → Raymond (docs) → Kevlin + Linus (review in parallel) → PLAN re-entry → Ward → Andy → squash-merge.

The only thing that could send us back to PLAN is if Kent/Rob discovers the schema actually does need a v2 bump (e.g., the named-volume vs bind distinction can't be derived from `HostPath` because TOML serialization round-trips wrong, or some other not-yet-foreseen issue). Per `_ai/decisions/schema-versioning.md:20` ("Escalation rule"), that's a stop-and-replan signal. We are explicitly NOT touching the schema shape in M2; if discovery says we have to, we stop.

---

## 12. Files to be touched (exhaustive, absolute paths)

### Code (Kent + Rob)

- `/Users/fenster/dev/decloud/internal/cli/deploy_service.go` — flag flip, error flip, parse-and-validate logic.
- `/Users/fenster/dev/decloud/internal/cli/deploy_service_test.go` — delete two tests, add positive/negative tests for parse + pass-through.
- `/Users/fenster/dev/decloud/internal/cli/exit_codes.go` — drop `ErrMountsNotSupported` mapping; add `ErrInvalidMount` mapping if exit-code split per §10 decision 5 needs it.
- `/Users/fenster/dev/decloud/internal/cli/exit_codes_test.go` — drop and replace test entry.
- `/Users/fenster/dev/decloud/internal/registry/errors.go` — delete `ErrMountsNotSupported`, add `ErrInvalidMount`.
- `/Users/fenster/dev/decloud/internal/registry/store.go` — replace `len(cfg.Run.Mounts) > 0` rejection with per-mount `validateMount`.
- `/Users/fenster/dev/decloud/internal/registry/store_test.go` — delete `TestStore_LoadRejectsNonEmptyMounts` (line 296), add `TestStore_LoadAcceptsValidMounts` and `TestStore_LoadRejectsInvalidMounts`.
- `/Users/fenster/dev/decloud/internal/registry/mount.go` — NEW file: `Mount` validation logic, `IsNamed` helper, parse-from-string helper for the CLI to share.
- `/Users/fenster/dev/decloud/internal/registry/mount_test.go` — NEW file: validation table tests (absolute path required, ro-only mode, no-duplicate-targets, named-volume regex).
- `/Users/fenster/dev/decloud/internal/deploy/service.go` — add `Mounts []registry.Mount` to `Request`; populate `runReq.Volumes` (Option α/β) at line 243; populate same at line 374 in `restoreOldContainer`; populate `Run.Mounts` in the `registry.Service{...}` literal at line 314-319 (currently `[]registry.Mount{}`).
- `/Users/fenster/dev/decloud/internal/deploy/service_test.go` — new tests: deploy-with-mounts passes them through to driver; deploy-with-mounts saves them to registry; restoreOldContainer-with-mounts passes them through.
- `/Users/fenster/dev/decloud/internal/deploy/lifecycle.go` — populate `runReq.Volumes` in `Start` absent-branch at line 67-78.
- `/Users/fenster/dev/decloud/internal/deploy/lifecycle_test.go` — new test: Start-with-mounts on a freshly absent container.
- `/Users/fenster/dev/decloud/internal/dockerdrv/driver.go` — IF Joel picks Option α: `Driver.Run` removed, callers switched to `RunWithOptions`. IF Option β: add `Volumes []VolumeMount` to `RunRequest`.
- `/Users/fenster/dev/decloud/internal/dockerdrv/cli_driver.go` — corresponding change to the production driver.
- `/Users/fenster/dev/decloud/internal/dockerdrv/mocks/mock_driver.go` — regenerated.
- `/Users/fenster/dev/decloud/internal/cli/mocks/mock_deployer.go`, `mock_lifecycle.go` — regenerated if `deploy.Request` shape changes (it does).
- `/Users/fenster/dev/decloud/internal/registry/mocks/mock_store.go` — regenerated if `Service` shape changes (it doesn't — only values).

### Integration test (Kent + Rob)

- `/Users/fenster/dev/decloud/internal/integration/mount_test.go` — NEW file with `//go:build integration` tag.
- `/Users/fenster/dev/decloud/internal/integration/doc.go` — NEW file (package doc + build-tag header for the package itself if needed).

### Docs (Raymond)

- `/Users/fenster/dev/decloud/_docs/usage.md:71` — flip the `--mount` row.
- `/Users/fenster/dev/decloud/_docs/usage.md:99` — drop "`--mount` used" from the `ExitConfigError` causes list.
- `/Users/fenster/dev/decloud/_ai/decisions/m1-scope.md:32` — strip "+ env-file hardening" from the M2 entry (phantom kill).
- `/Users/fenster/dev/decloud/_ai/decisions/m1-scope.md:16` — flip "No `--mount`" cut paragraph from "M2." to "shipped at M2; see `_tasks/2026-04-28-m2-server-side-mounts/`."
- `/Users/fenster/dev/decloud/_ai/decisions/secrets-split.md:24` — drop `ErrMountsNotSupported (M1)` from the loader rejection-classes list (sentinel deleted).
- `/Users/fenster/dev/decloud/_ai/decisions/schema-versioning.md:11` — flip M2 entry from future tense ("M2 populates") to past tense ("M2 shipped populating `Mounts`"); update line 16 same way.
- `/Users/fenster/dev/decloud/_ai/MEMORY.md:7` — strip "+ env-file hardening" from the `m1-scope.md` summary line.
- `/Users/fenster/dev/decloud/_ai/MEMORY.md:9` — change "mounts populate at M2" to past tense.
- `/Users/fenster/dev/decloud/_ai/m1x-backlog.md` item 6 — split into "item 6: integration smoke test (shipped at M2)" and "item 9: reloader stderr `%q` quoting" per §9 decision; mark item 6 with strike-through and link to this task's eventual squash-merge commit.
- `/Users/fenster/dev/decloud/_ai/cli-flag-surface-coherence.md:42` — the live example pointer at the carve-out updates: `TestDeployService_MountFlagHelpReferencesM2` no longer exists; either replace with a different live example (none currently in the codebase) or just narrate ("the precedent was `TestDeployService_MountFlagHelpReferencesM2` from the milestone-resequence task, deleted at M2 ship per `_tasks/2026-04-28-m2-server-side-mounts/`"). My lean: narrate. Joel/Raymond decide.
- `/Users/fenster/dev/decloud/CLAUDE.md` — no changes (it doesn't mention M2 by name).

### Fix-while-fresh sweep (Raymond, applying `_ai/fix-now-while-fresh.md`)

Per the audit-by-read refinement (`_ai/fix-now-while-fresh.md` Refinement section per Ward's update at `_tasks/2026-04-28-milestone-resequence/015-ward-knowledge.md`): Raymond reads each touched file end-to-end, not just greps for "M2", to catch paraphrases of "mounts ship at M2" that survived from the resequence. Specific candidates:

- `_ai/decisions/secrets-split.md:6` mentions M7 secret-files; verify M2 mention isn't lurking.
- `_ai/m1x-backlog.md` item 4 (the `Capture("")` comment) — confirm still applies post-M2.

---

## 13. Pre-planning verification: execution traces I performed

Per Don-rules, every claim about current state has a code citation.

**Trace 1: "M1 rejects `--mount` at three sites."**
- `internal/cli/deploy_service.go:71-73` — CLI rejection.
- `internal/registry/store.go:68-71` — loader rejection.
- `internal/registry/errors.go:11` — sentinel definition.
- All three confirmed by reading.

**Trace 2: "Three sites build `RunRequest`, none pass volumes."**
- `internal/deploy/service.go:243-251` — fresh deploy.
- `internal/deploy/service.go:374-382` — restoreOldContainer.
- `internal/deploy/lifecycle.go:67-78` — Start absent-branch.
- Confirmed by reading. None of the three constructs `Volumes` because `RunRequest` has no `Volumes` field (`internal/dockerdrv/driver.go:31-39`).

**Trace 3: "`Driver.RunWithOptions` already supports `Volumes []VolumeMount` and emits `-v` flags via `formatVolume`."**
- `internal/dockerdrv/driver.go:76-85` — `RunOptions` struct definition.
- `internal/dockerdrv/cli_driver.go:235-237` — `for _, v := range opts.Volumes { args = append(args, "-v", formatVolume(v)) }`.
- `internal/dockerdrv/cli_driver.go:285-291` — `formatVolume` returns `Source + ":" + Target [+ ":ro"]`.
- Caddy manager uses this path: `internal/dockerdrv/cli_driver_test.go:519-523`.
- Confirmed by reading.

**Trace 4: "`Mount` schema is in place since M1, no shape change needed."**
- `internal/registry/types.go:52-63` — `RunSpec.Mounts []Mount` and `Mount{HostPath, ContainerPath, ReadOnly}`.
- `_ai/decisions/schema-versioning.md:11` — "M2 writes `schema_version = 1`. M2 populates `Mounts`."
- M1 reservation: `internal/deploy/service.go:318` — explicitly initialised as `[]registry.Mount{}`.
- Confirmed.

**Trace 5: "`TestDeployService_MountFlagHelpReferencesM2` asserts `M2 only` substring in `--mount` help."**
- `internal/cli/deploy_service_test.go:97-104` confirmed.
- `internal/cli/deploy_service.go:61` — `"M1: rejected with ExitConfigError (M2 only)"`.
- Substring `"M2 only"` is what's asserted.

**Trace 6: "Env-file hardening is a phantom — there's no concrete work hiding in `internal/envcap/`."**
- Read `internal/envcap/capture.go` end-to-end. Implements the documented mechanism cleanly.
- Read `_ai/envcap-portable-bash.md` end-to-end. Mechanism documented, sharp edges enumerated, no "TODO M2" or "harden later" markers.
- Read `_ai/m1x-backlog.md` items 1-8. Item 4 is the only env-related entry — a comment-only clarification. Items 1-3 are unrelated lifecycle and assertion work.
- `grep -rn "TODO\|FIXME\|XXX\|harden" /Users/fenster/dev/decloud/internal/envcap/` returns nothing actionable.
- Confirmed: phantom.

**Trace 7: "Caddy manager's `RunOptions` fixture is the existing test for `Volumes` argv shape."**
- `internal/dockerdrv/cli_driver_test.go:504-525` — `caddyRunOptionsFixture()` includes three `VolumeMount` entries.
- `internal/dockerdrv/cli_driver_test.go:405-431` — explicit per-volume tests for `bind+ReadOnly` and `named+!ReadOnly`.
- Confirmed.

**Trace 8: "m1x-backlog item 6 says the integration test belongs to M2-new."**
- `_ai/m1x-backlog.md:55-63`. Confirmed verbatim: "Belongs to the next post-M1 milestone where we touch real Docker for the first time (the new M2 — server-side `--mount` — per the 2026-04-28 resequence)."

**Trace 9: "The carve-out at `cli-flag-surface-coherence.md:32-42` cites `TestDeployService_MountFlagHelpReferencesM2` as the live example."**
- Read confirmed. The carve-out exists *because* of this test. When the test goes away, the carve-out section's live example needs updating (or narrating-as-historical, my lean).

**Trace 10: "Schema version stays 1 across M1, M2, M7 by design."**
- `_ai/decisions/schema-versioning.md:11` — "M1 writes `schema_version = 1`. M2 writes `schema_version = 1`. M2 populates `Mounts`. M7 (secret-files-on-disk) also writes `schema_version = 1`..."
- Confirmed. M2 must NOT bump.

---

## 14. Things I'm locking in now (no Joel/Linus reopening unless concrete counter-evidence)

1. **Env-file hardening is dead.** Phantom. Removed from M2 scope. Fix-while-fresh strikes the phrase from `m1-scope.md:32` and `MEMORY.md:7`.
2. **Schema shape unchanged.** `schema_version = 1`. No bump. Locked by `schema-versioning.md:20` escalation rule.
3. **`ErrMountsNotSupported` deleted.** No "deprecated, kept for backward compatibility" garbage. Old TOMLs still load (empty `mounts` array is fine); new TOMLs use the populated field.
4. **`TestDeployService_MountFlagHelpReferencesM2` deleted** when M2 ships. The semantic-token contract has no token left.
5. **Integration test BUNDLED.** One test, `--mount` only, opt-in via `DECLOUD_INTEGRATION=1`.
6. **Reloader `%q` quoting PUNTED.** Split m1x-backlog item 6 into items 6 and 9.
7. **Docker `-v`-style short syntax** for `--mount`. No long-form. No tmpfs. No SELinux flags. Bind + named volumes + `:ro` only.
8. **No Viper.** No global config file. No `/etc/decloud/config.toml` "in preparation."
9. **Five surfaces flip together** in one commit (per Rob's lockstep discipline at `_tasks/2026-04-28-milestone-resequence/008-rob-impl.md`): runtime check, error message, `--help` text, `_docs/usage.md`, semantic-token test removal.
10. **Fix-while-fresh on the audit trail**: `m1-scope.md:32`, `MEMORY.md:7`, `secrets-split.md:24`, `schema-versioning.md:11/16`. All flip from future-tense-M2 to past-tense-M2.

---

## 15. What Joel does next

1. Read this plan top to bottom.
2. Resolve the 12 open decisions in §10. Lock specific design choices, with rationale, in the tech plan.
3. Specify exact code-level shapes: function signatures for `validateMount`, `Mount.IsNamed()`, `parseMountString`. The CLI/loader sharing a single validation source.
4. Specify the integration-test setup (image build, hermetic cleanup, `DECLOUD_INTEGRATION` gating).
5. Plan the `Driver.Run` α/β decision with concrete test-rewriting cost estimate so Linus can weigh it.
6. Identify any pre-existing bugs surfaced during the trace pass (the way Joel found `internal/cli/deploy_service.go:55` `--port (required)` drift in the M1 review-findings task) and apply fix-while-fresh.

Linus reviews after Joel ships v1. Standard PLAN-iterate-until-Linus-approves loop.

---

## Files relevant to this task (absolute paths)

- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/001-user-request.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/002-don-plan.md` (this file)
- `/Users/fenster/dev/decloud/internal/cli/deploy_service.go`
- `/Users/fenster/dev/decloud/internal/cli/deploy_service_test.go`
- `/Users/fenster/dev/decloud/internal/cli/exit_codes.go`
- `/Users/fenster/dev/decloud/internal/cli/exit_codes_test.go`
- `/Users/fenster/dev/decloud/internal/registry/errors.go`
- `/Users/fenster/dev/decloud/internal/registry/store.go`
- `/Users/fenster/dev/decloud/internal/registry/store_test.go`
- `/Users/fenster/dev/decloud/internal/registry/types.go`
- `/Users/fenster/dev/decloud/internal/deploy/service.go`
- `/Users/fenster/dev/decloud/internal/deploy/service_test.go`
- `/Users/fenster/dev/decloud/internal/deploy/lifecycle.go`
- `/Users/fenster/dev/decloud/internal/deploy/lifecycle_test.go`
- `/Users/fenster/dev/decloud/internal/dockerdrv/driver.go`
- `/Users/fenster/dev/decloud/internal/dockerdrv/cli_driver.go`
- `/Users/fenster/dev/decloud/internal/dockerdrv/cli_driver_test.go`
- `/Users/fenster/dev/decloud/internal/envcap/capture.go`
- `/Users/fenster/dev/decloud/_docs/usage.md`
- `/Users/fenster/dev/decloud/_ai/decisions/m1-scope.md`
- `/Users/fenster/dev/decloud/_ai/decisions/schema-versioning.md`
- `/Users/fenster/dev/decloud/_ai/decisions/secrets-split.md`
- `/Users/fenster/dev/decloud/_ai/decisions/m1-test-strategy.md`
- `/Users/fenster/dev/decloud/_ai/MEMORY.md`
- `/Users/fenster/dev/decloud/_ai/m1x-backlog.md`
- `/Users/fenster/dev/decloud/_ai/cli-flag-surface-coherence.md`
- `/Users/fenster/dev/decloud/_ai/fix-now-while-fresh.md`
- `/Users/fenster/dev/decloud/_ai/envcap-portable-bash.md`
