# 009 — Raymond's docs execution: M2 server-side mounts

## Status

**DONE.** All prescribed doc edits applied per Joel's tech plan §11 + addendum Issue 5 (verbatim no-stat paragraph). Bare-token sweep across `_ai/`, `_docs/`, and `README.md` returned no stale survivors after this pass. Eight files touched.

Anchors:
- Don's plan: `_tasks/2026-04-28-m2-server-side-mounts/002-don-plan.md`
- Joel's tech plan: `003-joel-tech-plan.md` §11 (consolidated docs sweep table)
- Joel's addendum: `005-joel-tech-plan-addendum.md` Issue 5 (locked verbatim no-stat paragraph)
- Rob's GREEN final state: `008-rob-impl.md`
- Methodology template: `_tasks/2026-04-28-milestone-resequence/009-raymond-docs.md`

## 1. Bare-token grep methodology

Before editing, I ran the following sweeps, scoped to `_ai/*.md`, `_docs/*.md`, `README.md`, and `CLAUDE.md` (excluding `_tasks/` per the resequence-task convention):

| Sweep | Tokens / phrases | Purpose |
|---|---|---|
| 1 | `M2`, `M3`, `M3a`, `M3b`, `Mounts`, `--mount`, `ErrMountsNotSupported`, `persistent volume` | Don's prescribed bare-token sweep |
| 2 | `Persistent volumes are M2`, `Rejected with exit 10 in M1`, `env-file hardening` | Joel's "phantom kill" markers (env-file hardening) and the M1-rejection prose |
| 3 | bare `M2` token (`grep -n -w "M2"`) post-edit verification | After-the-fact confirmation that every surviving `M2` token reads correctly under M2-shipped tense |
| 4 | bare `--mount` token post-edit verification | Confirms every operator-facing `--mount` reference is the M2 accept-and-validate semantic, not the M1 reject-with-exit-10 semantic |

For each hit I read the surrounding paragraph end-to-end (audit-by-read per `_ai/fix-now-while-fresh.md` Refinement) so paraphrases of the stale prose couldn't slip through.

## 2. Files edited

Eight files. Pre-edit and post-edit summaries.

### `/Users/fenster/dev/decloud/_docs/usage.md` — 4 substantive edits

| # | Lines | Summary |
|---|---|---|
| U1 | 71 (flag table row) | Full rewrite of the `--mount` row from M1-rejection prose to the M2 accept-and-validate spec — bind/named-volume syntax, absolute container path requirement, named-volume regex, `:ro`-only mode policy with rejected-mode list, parse-time exit 2 for CLI dup-target / load-time exit 10 for hand-edited TOML. |
| U2 | between old-73 and old-74 (verbatim no-stat paragraph + Mount examples + TOML example) | Inserted Joel's addendum Issue 5 paragraph **byte-exact** (no paraphrasing — Linus locked it). Added five mount examples (bind RW, bind RO, named volume, mixed, persistence note) and a TOML `[[run.mounts]]` example showing the three on-disk fields. |
| U3 | exit-code table, exit `2` row | Added the malformed-`--mount` parse-time case to `ExitUsageError` (bad component count, missing absolute container path, unsupported mode flag, duplicate container path across `--mount` flags). |
| U4 | exit-code table, exit `10` row | Replaced "`--mount` used" with "malformed `--mount` in a hand-edited TOML" — accurate post-M2 wording per Joel §11 prescription. |

### `/Users/fenster/dev/decloud/_ai/decisions/m1-scope.md` — 2 edits

| # | Line | Summary |
|---|---|---|
| S1 | 16 | "**No `--mount`** — flag rejected; loader also rejects non-empty `Mounts` (closes hand-edit loophole). M2." → "**No `--mount` in M1** — flag rejected; loader also rejected non-empty `Mounts` (closed hand-edit loophole). Shipped at M2; see `_tasks/2026-04-28-m2-server-side-mounts/`." (verbatim from Joel §11) |
| S2 | 32 | Stripped the env-file-hardening phantom phrase from the canonical roadmap line — "M2 server-side mounts + env-file hardening" → "M2 server-side mounts" (verbatim from Joel §11; phantom kill per Don §1a). |

### `/Users/fenster/dev/decloud/_ai/decisions/schema-versioning.md` — 1 edit

| # | Lines | Summary |
|---|---|---|
| V1 | 16 | Past-tense rewrite of the "M1 declares the full schema shape" paragraph: M1 rejects-non-empty-Mounts becomes "M1 rejected ... `ErrMountsNotSupported` (deleted at M2)"; "M2 starts populating" becomes "At M2 the rejection becomes positive validation: the loader runs `registry.ValidateMounts` ... grammar-only checks per `internal/registry/mount.go`; no source stat. M2 populates `Mounts` ..." Line 11 already read present-tense correct ("M2 writes ... M2 populates `Mounts`") — no change needed. |

### `/Users/fenster/dev/decloud/_ai/decisions/secrets-split.md` — 1 edit

| # | Line | Summary |
|---|---|---|
| C1 | 24 | Loader rejection-classes list: "`ErrMountsNotSupported` (M1)" → "`ErrInvalidMount` (malformed `mounts` entry — replaced the M1 `ErrMountsNotSupported` blanket rejection at M2)". The sentinel was deleted by Rob; the list now names the live sentinel and notes the historical replacement. |

### `/Users/fenster/dev/decloud/_ai/decisions/m1-test-strategy.md` — 1 edit

| # | Line | Summary |
|---|---|---|
| T1 | 7 | Past-tense the "next milestone's first feedback signal" sentence and append the M2 ship-event: "M2 then shipped the first automated `-tags integration` test (`internal/integration/mount_test.go`, gated on `DECLOUD_INTEGRATION=1`), inverting the manual-CI bridge for the `--mount` surface." Line 49 ("next milestone's first priority") still reads correctly as a forward-looking discipline; left unchanged. |

### `/Users/fenster/dev/decloud/_ai/MEMORY.md` — 2 edits

| # | Line | Summary |
|---|---|---|
| M1 | 9 | "(mounts populate at M2, secret-files at M7)" → "(mounts populated since M2, secret-files at M7)" — past-tense for the now-shipped milestone (verbatim from Joel §11). |
| M2 | new line under "Source-of-truth task artefacts" | New bullet pointing at `_tasks/2026-04-28-m2-server-side-mounts/` with one-liner summary covering: `--mount` accepts bind+named with `:ro`, loader runs `ValidateMounts`, `RunRequest.Volumes` thread-through, `ErrMountsNotSupported` deletion + `ErrInvalidMount` replacement, integration test bundling, and the three split-to-backlog items (9 reloader `%q`, 11 `Driver.Run` consolidation, 10 curl-through-Caddy). |

### `/Users/fenster/dev/decloud/_ai/m1x-backlog.md` — 1 large edit (item 6 rewrite + items 9, 10, 11 added)

| # | Lines | Summary |
|---|---|---|
| B1 | item 6 (lines ~55–63) | Strikethrough heading; status changed to "PARTIALLY DONE at M2" with explicit split-out trail to items 9 (reloader `%q`) and 10 (curl-through-Caddy). Body replaced with M2-delivery summary covering the build-tag, gating env var, and what the test asserts. Originator chain extended with the M2 task's split rationale (Joel Decisions 8 and 9). |
| B2 | new item 9 | "Reloader stderr `%q` quoting revisit." Where: three sites in `internal/caddy/reloader.go`. Why deferred: orthogonal to mounts; would invite a different review surface. Originator: Don §9 + Joel Decision 9 of the M2 task. |
| B3 | new item 10 | "Curl-through-Caddy integration test." Where: new `internal/integration/ingress_test.go`. Why deferred: distinct failure modes from `--mount` test; bundling compounded risk. Originator: Joel decision 8 of the M2 task. |
| B4 | new item 11 | "Consolidate `Driver.Run` and `Driver.RunWithOptions`." Where: `internal/dockerdrv/driver.go` + ~20 mock-call sites. Why deferred: Decision 4 picked Option β to keep the M2 diff narrow; α is the cleaner end-state. Originator: Don §5 + Joel Decision 4. |

### `/Users/fenster/dev/decloud/_ai/cli-flag-surface-coherence.md` — 1 edit

| # | Line | Summary |
|---|---|---|
| F1 | 42 | Live example block reframed from "`TestDeployService_MountFlagHelpReferencesM2` ... asserts" to "Historical live example: ... asserted ... until M2 shipped." Explains why the test was deleted (no remaining contract surface) per Don §7 / Joel Decision 9, and reaffirms the carve-out as a pattern for any future milestone-token assertion. The lines 35 and 40 mentions of `"M2"` are pedagogical examples of the carve-out's shape (allowed semantic-token contract) and remain correct without change. |

## 3. Files considered but NOT edited

For reviewer completeness:

- `/Users/fenster/dev/decloud/README.md` — `grep -E "M[1-9]|--mount|Mounts" README.md` exits 1; zero milestone refs and zero mount refs (the README's "Mounted files" prose talks about the architectural concept, not the flag). No edits needed.
- `/Users/fenster/dev/decloud/_docs/install.md` — re-read end-to-end. No M2-specific mount refs. The `M1.0` historical anchors at lines 5, 28, 30, 57, 64, 67, 70, 80, 132, 175, 178, 186 are all about the host-Caddy migration; unaffected by M2. No edits needed.
- `/Users/fenster/dev/decloud/_ai/decisions/caddy-runs-in-container.md` — re-read end-to-end. The "Concurrent deploys (theoretical, M2+)" anchor at line 53 is a milestone-range bound (Joel previously confirmed in the resequence task §B.4 final note); the M3-Viper references at lines 15, 52, 58 are correct under the post-resequence sequence. M3 still owns Viper introduction; M2 doesn't touch this file's claims. No edits needed.
- `/Users/fenster/dev/decloud/_ai/container-naming.md` — only milestone ref is the "M1–M3" range bound at line 14, still correct (any milestone before blue/green ships `decloud-<name>`). No edits needed.
- `/Users/fenster/dev/decloud/_ai/fix-now-while-fresh.md:48` — references the historical resequence-task example of "until M2's config file lands" as the canonical "audit-by-read catches paraphrases" demonstration. Historical context; reads correctly as a teaching example. No edits needed.
- `/Users/fenster/dev/decloud/CLAUDE.md` — no M2 / `--mount` / `Mounts` references. No edits needed.

## 4. Hallucination self-check (CLI behaviour, TOML field names, error messages, exit codes)

Every concrete claim I wrote into the docs is traced to source. Kevlin will review for hallucinations very very carefully — these are the citations that back each claim, drawn from Rob's shipped GREEN code rather than from Joel's plan or Don's plan (which are upstream documents, not the source of truth).

### CLI flag declaration and help text

| Doc claim | Source-of-truth |
|---|---|
| Flag is `--mount`, repeatable, type `string` | `internal/cli/deploy_service.go:61` (`StringArrayVar(&f.Mounts, "mount", nil, ...)`) |
| Help-text mention of `<host-path>:<container-path>[:ro]` (bind) and `<name>:<container-path>[:ro]` (named volume), repeatable | `internal/cli/deploy_service.go:62` (the literal third arg to `StringArrayVar`) |
| `StringArrayVar` (paths-with-commas safe), not `StringSliceVar` | `internal/cli/deploy_service.go:61` confirms `StringArrayVar`. Also locked by Rob §3 atomic-flip table item "CLI flag accept" and Joel §8.9. |

### Validation rules (CLI parse path → exit 2)

| Doc claim | Source-of-truth |
|---|---|
| Container path must be absolute | `internal/registry/mount.go:36` (`if !filepath.IsAbs(m.ContainerPath)`) |
| Named-volume source must match `[a-zA-Z0-9][a-zA-Z0-9_.-]+` | `internal/registry/mount.go:12` (`volumeNameRE = regexp.MustCompile(\`^[a-zA-Z0-9][a-zA-Z0-9_.-]+$\`)`); used at `mount.go:40-43` |
| Default mode is RW; only `:ro` accepted; `:rw` etc. rejected | `internal/registry/mount.go:84-89` (the `switch parts[2]` arm: only `"ro"` sets `ro = true`; `default` returns `"unsupported mode flag %q (only \"ro\" is supported)"`) |
| Empty source rejected | `internal/registry/mount.go:30-32` |
| Empty container path rejected | `internal/registry/mount.go:33-35` |
| Wrong number of `:`-components (1 or 4+) rejected | `internal/registry/mount.go:90-92` (`default: return ..."expected <source>:<target>[:ro], got %d component(s)"`) |
| CLI dup-target → `errUsage` exit 2 (NOT `ErrInvalidMount`) | `internal/cli/deploy_service.go:184-187` wraps with `errUsage` only; `internal/cli/exit_codes.go:39-40` maps `errUsage` → `ExitUsageError` (= 2) |
| Loader-side dup-target → `ErrInvalidMount` exit 10 | `internal/registry/mount.go:58-61` wraps with `ErrInvalidMount`; `internal/cli/exit_codes.go:41` maps `ErrInvalidMount` → `ExitConfigError` (= 10) |

### Loader behaviour

| Doc claim | Source-of-truth |
|---|---|
| Loader runs `ValidateMounts` (grammar-only) | `internal/registry/store.go:68` (`if err := ValidateMounts(cfg.Run.Mounts, name, cfgPath); err != nil`) |
| Loader does NOT stat bind sources | `internal/registry/mount.go:23-29` (doc-comment explicitly says "It does NOT stat the host path") + the function body has no `os.Stat` call |
| Same validation rules apply CLI-side and loader-side | `internal/registry/mount.go:94` — `ParseMountString` calls `ValidateMount(m)` after building the struct; `store.go:68` calls `ValidateMounts` which iterates `ValidateMount`. Single source of truth. |

### Schema / TOML field names

| Doc claim | Source-of-truth |
|---|---|
| TOML keys: `host_path`, `container_path`, `read_only` | `internal/registry/types.go:59-63` (the `Mount` struct's `toml:"..."` tags) |
| `[[run.mounts]]` is the array-of-tables form | `internal/registry/types.go:56` (`Mounts []Mount \`toml:"mounts"\`` inside `RunSpec` which is `[run]`) |
| `schema_version` stays at 1 | `internal/registry/types.go:5` (`const CurrentSchemaVersion = 1`); also locked at `internal/registry/store.go:64-66` (loader's `cfg.SchemaVersion != CurrentSchemaVersion` check) |

### Runtime behaviour (Volumes thread-through)

| Doc claim | Source-of-truth |
|---|---|
| Mounts persist with the registration; `decloud start` and `decloud restart` re-attach the same set | `internal/deploy/lifecycle.go` `Start` absent-branch populates `runReq.Volumes` from `prev.Config.Run.Mounts` (Rob §"`internal/deploy/lifecycle.go`" — added at lines 67-78) |
| Re-deploy replaces the previous mount list | `internal/deploy/service.go` deploy-time `runReq` populated from `req.Mounts` (Rob §"`internal/deploy/service.go`"); `RunSpec.Mounts` written from `req.Mounts` not `[]registry.Mount{}` (same Rob entry); `Save` overwrites the previous registration. |
| `docker run -v <source>:<target>[:ro]` argv shape | `internal/dockerdrv/cli_driver.go` `Run` for-loop appends `-v` + `formatVolume(v)` (Rob §"`internal/dockerdrv/cli_driver.go`"); `formatVolume` documented in tech plan §3.7-3.8. |

### Error messages (verbatim strings)

The doc paragraph "typical text: `error while creating mount source path '/missing-path': mkdir ...`" is the **Docker daemon's own** error string emitted at `docker run` time, not a string Decloud constructs. I reviewed Joel's addendum Issue 5 — Linus locked the paragraph verbatim. The wording is "typical text" (not "exact text"), giving the operator a fingerprint without claiming byte-for-byte fidelity. This is the same shape as `_docs/install.md:175` ("the error already names the recovery commands") which I read as a precedent for naming-not-quoting Docker-side strings.

### Exit codes

| Doc claim | Source-of-truth |
|---|---|
| Exit 2 for parse-time `--mount` errors | `internal/cli/exit_codes.go:39-40` (errUsage → ExitUsageError = 2 at `internal/cli/exit_codes.go:15`) |
| Exit 10 for loader-time hand-edited-TOML `--mount` errors | `internal/cli/exit_codes.go:41` (ErrInvalidMount → ExitConfigError = 10 at line 16) |
| Exit 40 for Docker-run failure on missing bind source | `internal/cli/exit_codes.go:55-56` (deploy.ErrRun → ExitRunFail = 40 at line 19); plus `internal/dockerdrv/cli_driver.go` Run wraps daemon errors. The "exit 40" claim in the no-stat paragraph is Joel's locked wording — verified against the existing exit-code table at `_docs/usage.md:180` which already attributes `docker run` failures to exit 40. |

## 5. Things flagged but NOT fixed (impl/doc mismatches I did NOT change)

None. Every doc claim I wrote traces to source as listed in §4. No impl/doc mismatch surfaced during the audit-by-read pass.

The closest borderline call: the no-stat-paragraph "typical text: `error while creating mount source path '/missing-path': mkdir ...`" claim. I did NOT verify Docker's exact wording against a real `docker run` against a missing bind source on the maintainer's box — it's the Docker daemon's error string, not Decloud code. Joel's addendum Issue 5 says "typical text:" rather than "exact text:" precisely because Docker version skew makes byte-exactness fragile. The integration test (`internal/integration/mount_test.go`) doesn't exercise the missing-source path either. If Kevlin or Linus wants this verified, the maintainer's M2 manual smoke-test on a real Linux box is the right surface for the verification — not a doc edit.

## 6. Files relevant to this report (absolute paths)

Edited:
- `/Users/fenster/dev/decloud/_docs/usage.md`
- `/Users/fenster/dev/decloud/_ai/decisions/m1-scope.md`
- `/Users/fenster/dev/decloud/_ai/decisions/schema-versioning.md`
- `/Users/fenster/dev/decloud/_ai/decisions/secrets-split.md`
- `/Users/fenster/dev/decloud/_ai/decisions/m1-test-strategy.md`
- `/Users/fenster/dev/decloud/_ai/MEMORY.md`
- `/Users/fenster/dev/decloud/_ai/m1x-backlog.md`
- `/Users/fenster/dev/decloud/_ai/cli-flag-surface-coherence.md`

Verified-but-not-edited:
- `/Users/fenster/dev/decloud/README.md`
- `/Users/fenster/dev/decloud/_docs/install.md`
- `/Users/fenster/dev/decloud/_ai/decisions/caddy-runs-in-container.md`
- `/Users/fenster/dev/decloud/_ai/container-naming.md`
- `/Users/fenster/dev/decloud/_ai/fix-now-while-fresh.md`
- `/Users/fenster/dev/decloud/CLAUDE.md`

Source-of-truth code (for hallucination citations):
- `/Users/fenster/dev/decloud/internal/cli/deploy_service.go`
- `/Users/fenster/dev/decloud/internal/cli/exit_codes.go`
- `/Users/fenster/dev/decloud/internal/registry/mount.go`
- `/Users/fenster/dev/decloud/internal/registry/errors.go`
- `/Users/fenster/dev/decloud/internal/registry/types.go`
- `/Users/fenster/dev/decloud/internal/registry/store.go`
- `/Users/fenster/dev/decloud/internal/dockerdrv/cli_driver.go`
- `/Users/fenster/dev/decloud/internal/deploy/service.go`
- `/Users/fenster/dev/decloud/internal/deploy/lifecycle.go`

Plan + impl artefacts read:
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/001-user-request.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/002-don-plan.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/003-joel-tech-plan.md` §11
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/005-joel-tech-plan-addendum.md` Issue 5
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/008-rob-impl.md`
