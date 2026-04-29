# 003 — Joel's tech plan: M2 server-side mounts

## TL;DR

Don's plan §1–§9 stand. The 12 open decisions in §10 are locked in §1 below with rationale. The five M2 surfaces (CLI flag accept, loader accept, runtime `-v` pass, sentinel deletion, help-text rewording) flip in **one atomic commit** so the test suite never sees a half-flipped state — Don's instinct in §14 item 9 is the right call (defended in §6).

This plan is the contract Kent and Rob implement against. Where Don gave options, this plan picks one and justifies it. Where Don left a wording open, this plan supplies the bytes.

Anchors / cross-refs:
- Don's plan: `_tasks/2026-04-28-m2-server-side-mounts/002-don-plan.md`
- M1 surface-coherence pattern: `_ai/cli-flag-surface-coherence.md`
- Cleanup-context discipline (re-applied in M2 cleanup paths if any): `_ai/cleanup-context-discipline.md`
- Schema-versioning lock: `_ai/decisions/schema-versioning.md` (no bump in M2)

---

## 1. Resolution of Don's 12 open decisions

For each: **answer + 2-3 sentence rationale rooted in existing patterns**.

### Decision 1 — Mount source-existence check at deploy time?

**Locked: NO. Do not stat bind-mount sources during CLI parsing or loader validation.**

Rationale: M1's `--port` validation rejects `0` because that's a *grammar* error (no port can be 0). Bind-source-exists is a *world* error: it can be true at deploy time and false at `decloud start` time. Stat-checking introduces a TOCTOU race we'd then have to talk about. Docker itself surfaces a clear error at `docker run` time when a bind source is missing (`error response from daemon: stat /foo: no such file or directory`); that error passes through `dockerdrv.cliDriver.RunWithOptions` → `ErrRun` → exit 40 today, with no extra plumbing. **Failing fast** here is doing the operator a favor only on the *first* deploy, and at the cost of doing them no favor on `start` after a host reboot where `/mnt/data` has not auto-mounted yet. We let Docker speak.

This overrides Don's lean. The rule generalises: validation rejects what can never become valid (grammar, syntax, semantics-locked-by-the-spec). The world is not in scope.

Implication for Decision 11: trivially NO at the loader as well. Both surfaces stay grammar-only.

### Decision 2 — Where does `validateMount` live?

**Locked: `internal/registry/mount.go`** (Don's lean).

Rationale: the validation rules ARE the registry's data-integrity contract — the loader-side validation is the second-line-of-defence for a hand-edited TOML, and the registry is the package that owns `Mount` already (`internal/registry/types.go:59-63`). A separate `internal/mount/` package is overkill for one type and three small functions. The CLI imports `registry` already (`internal/cli/deploy_service.go:17`), so sharing is free.

### Decision 3 — `Mount` Go-field rename or `IsNamed` derivation?

**Locked: Option B — derive from `HostPath`** (Don's lean).

Rationale: the `host_path` TOML key is locked by `_ai/decisions/schema-versioning.md:11` ("shape doesn't change"). Renaming the Go field to `Source` while keeping the TOML tag `host_path` is allowed but is a paper cut: every reader of the struct flips between two names for the same value. Don's option B uses one helper `(m Mount) IsNamed() bool { return !strings.HasPrefix(m.HostPath, "/") }` and a doc-comment on `Mount.HostPath` that names the convention. Single source of truth for "what is this string?" lives where the struct is declared.

Cost-benefit: the rename touches every struct literal in tests + `internal/deploy/service.go:318`. The derive option touches one new helper. Mechanical sympathy for the conversion site: `registryMountToVolumeMount(m Mount) dockerdrv.VolumeMount` hides the bridge in a function with one call site.

### Decision 4 — `Driver.Run` consolidation (Option α) vs `RunRequest.Volumes` field (Option β)?

**Locked: Option β — add `Volumes []VolumeMount` to `RunRequest`.**

This **overrides Don's lean toward α**. Rationale, three points:

1. **Blast radius is real.** Option α touches `Driver.Run` (interface), `cliDriver.Run` (production), `MockDriver.Run` (regenerated), and every `Driver.EXPECT().Run(...)` call in `internal/deploy/service_test.go` and `internal/deploy/lifecycle_test.go`. A grep for `Driver.EXPECT().Run(` shows 20+ hit sites — every one becomes `Driver.EXPECT().RunWithOptions(...)`. Kent rewrites a test file's worth of expectations for a milestone whose theme is `--mount`.
2. **β is locally minimal and structurally honest.** Don's argument for α is "two run paths is a divergence bug factory." The two paths exist today *because of port publishing*: `Run` for service deploys (no host ports), `RunWithOptions` for Caddy (publishes 6 ports + 3 volumes + 1 label). Adding `Volumes` to `RunRequest` does NOT widen that divergence — it brings them closer together. Once the only remaining difference between `Run` and `RunWithOptions` is "service deploys never publish ports / never pass labels," α can be revisited in M3 or M4 as a no-op cleanup.
3. **Joel's milestone-discipline rule from M1 (`_tasks/2026-04-26-readme-implementation-planning/06-tech-plan-v2.md`): "make the milestone smaller, not bigger."** α turns M2 from "ship `--mount`" into "ship `--mount` AND consolidate the run path." That's two features. Two features fit two milestones, not one.

**Punt-to-future-α observation, recorded here so it doesn't get lost:** add a follow-up backlog item 9 in `_ai/m1x-backlog.md` titled "consolidate `Driver.Run` + `RunWithOptions` into one method." Raymond carries this in the docs sweep — see §11 of this plan.

### Decision 5 — Exit-code split for malformed `--mount`

**Locked: `errUsage` → exit 2 from CLI; `ErrInvalidMount` → exit 10 from loader.**

Rationale: M1 set the precedent at `internal/cli/deploy_service.go:71-78`. `--port=0` wraps `errUsage` (exit 2) because zero ports is "you used the flag wrong"; `--strategy=blue_green` wraps `registry.ErrInvalidStrategy` (exit 10) because that's a registry-layer policy. Malformed `--mount` is the same shape as `--port=0` *at the CLI*: the operator typed garbage. A hand-edited TOML with the same garbage is the same shape as `--strategy=blue_green` *at the loader*: the disk says something the registry won't accept. The split mirrors the existing `errUsage` vs `ErrInvalidStrategy` split exactly.

### Decision 6 — Exact `--mount` help-text wording

**Locked:**

```
"persistent volume; <host-path>:<container-path>[:ro] (bind) or <name>:<container-path>[:ro] (named volume); repeatable"
```

Rationale: mirrors Don §7 "plain wins" (no future-pointing prose). Mirrors `--name`'s parenthetical-pattern style (`"service name (required, [a-z][a-z0-9-]{0,38})"` — see `cli-flag-surface-coherence.md:46`). Names both supported source forms (bind + named) inline so an operator reading `--help` knows what they can pass without going to `_docs/usage.md`. Names `:ro` as the only mode flag inline — the long tail of unsupported flags (`:z`, `:Z`, `:cached`, ...) is documented by absence; a positive list is shorter than a negative one.

**Specifically rejected wordings:**

- `"persistent volume (host:container[:ro] or volname:container[:ro], repeatable)"` — Don's first option, but `host:container` is ambiguous (is `host` literal? a placeholder?). The angle-bracket convention `<host-path>` is borrowed from `--strategy`'s "(M1: recreate only)" placeholder discipline.
- Anything containing `"M2"`, `"M-future"`, `"long-form"`. Don's §7 plain-wins argument applies.

### Decision 7 — Exact `--mount` error wording

**Two error messages, share a wording template — `"--mount %q: <reason>"`.**

For CLI parse failure (`errUsage`):

```go
fmt.Errorf("--mount %q: %s: %w", raw, reason, errUsage)
// e.g. "--mount \"/host\": expected <source>:<target>[:ro], got 1 component(s): usage error"
```

For loader rejection (`ErrInvalidMount`):

```go
fmt.Errorf("%w: service %q mount[%d] in %s: %s", ErrInvalidMount, name, idx, path, reason)
// e.g. "registry: invalid mount: service \"foo\" mount[0] in /opt/decloud/config/services/foo.toml: container_path must be absolute, got \"data\""
```

Reason strings (single lowercase clause, no trailing period; see `error-wrap-discipline.md` for sentence shape):

| Failure | Reason string |
|---|---|
| Empty raw string | `"empty"` (CLI only — loader can't see this) |
| Wrong number of `:`-components (n=1 or n=4+) | `"expected <source>:<target>[:ro], got %d component(s)"` |
| Empty source | `"source must be non-empty"` |
| Empty target | `"container_path must be non-empty"` |
| Relative target | `"container_path must be absolute, got %q"` |
| Invalid named-volume name | `"named-volume source %q must match [a-zA-Z0-9][a-zA-Z0-9_.-]+"` |
| Mode flag != `ro` | `"unsupported mode flag %q (only \"ro\" is supported)"` |
| Duplicate target | `"duplicate container_path %q (also at mount[%d])"` |

Rationale: every reason names the offending component or value. This is the operator-debugging context Don §10.10 asked for: when an operator stares at "`--mount /foo`: expected <source>:<target>[:ro], got 1 component(s)", they immediately see they forgot the colon. Same shape as M1's `--strategy` error which names the offending value verbatim.

### Decision 8 — One integration test or two?

**Locked: ONE — mount-only, no curl-through-Caddy.**

Rationale: Don's lean. M2's theme is `--mount`. The curl-through-Caddy half of m1x-backlog item 6 is a different feature (ingress verification) with a different failure mode (Caddyfile generation, TLS cert acquisition, port publishing). Bundling both compounds risk: if curl-through-Caddy fails on an operator's IPv6-disabled host while `--mount` works, the test fails for a non-mount reason and the M2 ship gets blocked. Split clean. The other half becomes m1x-backlog item 10 (see §11 Raymond's docs sweep).

### Decision 9 — Reloader `%q` punt + m1x-backlog renumber

**Locked: Don's §9 punt stands. m1x-backlog item 6 splits as Don described.**

After M2 ships:
- **Item 6 (was: "integration smoke test + reloader %q")** — strikethrough; replace body with "shipped at M2; see `_tasks/2026-04-28-m2-server-side-mounts/`". Keep one release per `m1x-backlog.md` maintenance note.
- **Item 9 (NEW)** — "Reloader stderr `%q` quoting revisit." Fix shape: audit `internal/caddy/reloader.go:69`, `:72`, `:80` for whether `%q` (Go `strconv.Quote`-style) is the right rendering for operator-facing stderr. Investigate alternatives: raw stderr appended with `\n` indent, JSON-quoted with explicit Unicode handling, `strings.TrimSpace` + bare. Decision made when the item is picked up; **not in M2.**
- **Item 10 (NEW)** — "Curl-through-Caddy integration test." Fix shape: new file `internal/integration/ingress_test.go`, build tag `//go:build integration`, gated on `DECLOUD_INTEGRATION=1`. Brings up Caddy + a deploy with an HTTP host, asserts a curl through Caddy returns the expected upstream body. Distinct from M2's `--mount` test because the failure modes don't share a debugging surface (mount = `docker exec cat`, ingress = TLS / Caddyfile generation / DNS).
- **Item 11 (NEW, from Decision 4 above)** — "Consolidate `Driver.Run` + `RunWithOptions`." Fix shape: drop `Driver.Run`, switch all `service.go` / `lifecycle.go` callers to `RunWithOptions`, regenerate `MockDriver`, rewrite ~20 `Driver.EXPECT().Run(...)` calls to `Driver.EXPECT().RunWithOptions(...)`. ~1 hour mechanical work, zero behaviour change.

### Decision 10 — `ErrInvalidMount` exact wording

Covered in Decision 7. The sentinel itself:

```go
// internal/registry/errors.go
var ErrInvalidMount = errors.New("registry: invalid mount")
```

Rationale: matches the existing `ErrInvalidStrategy = errors.New("registry: strategy not supported in M1")` shape but DROPS the `"M1"` time-marker (the rule is now permanent, not milestone-bound). The wrap message naming index, path, and reason supplies the operator-debug context.

### Decision 11 — Stat the bind-mount source path during loader validation?

**Locked: NO at both surfaces** (Don's lean, locked harder by Decision 1).

Rationale: see Decision 1. Loader-time stat is strictly worse than CLI-time stat — it punishes `decloud start` after a host reboot where the disk hasn't auto-mounted yet, even though the original deploy was correct. The world is not in scope of validation.

### Decision 12 — Duplicate container_paths: hard reject or warn-and-pick-last?

**Locked: HARD REJECT** (Don's lean).

Rationale: `--mount /a:/data --mount /b:/data` is silently last-wins in `docker run` (`-v /a:/data -v /b:/data`). Last-wins is exactly the kind of "silently does the wrong thing" footgun M1 tightened against (`--port=0` was M1's example: silently used to mean "worker mode," explicitly rejected per `_ai/decisions/no-magic-zero-modes.md`). Container-path is the natural primary key; reject duplicates with a message naming both indices (Decision 7 reason string).

The validation runs over the slice in order, building a `seen map[string]int{containerPath -> firstIndex}`. Second occurrence triggers the duplicate error.

---

## 2. Reusable patterns from M1

Rob and Kent: use these patterns. Don't reinvent.

### 2a. Env-capture validation surface (the parallel I want you to mirror)

`internal/envcap/capture.go` is the canonical example of a validating leaf-package shared by CLI and loader-orchestration:

- **Sentinel-only return:** `envcap.ErrEnvScriptMissing` and `envcap.ErrEnvScriptUnreadable` are package-level `errors.New(...)`. Wrappers pile context with `%w: %s: %w` (CLI at `deploy_service.go:118-120`).
- **CLI surface guards:** `resolveEnvFile(...)` in `deploy_service.go:114-129` translates string flag values into validated paths-or-empty; the caller is the orchestrator. Same shape applies for `parseMountFlags(rawSlice []string) ([]registry.Mount, error)` — a function in `internal/cli/deploy_service.go` (not in `registry`) that consumes the raw `--mount` repeatable strings and returns validated `[]registry.Mount` or `errUsage`-wrapped error.
- **Loader-side validation lives in the package that owns the type.** Env's loader equivalent is the `pelletier/go-toml/v2` strict-mode + schema_version check in `store.go:64-67`. Mount-equivalent: the `for i, m := range cfg.Run.Mounts { validateMount(m) }` in `store.go`, calling `registry.validateMount` lives in the same package as `Mount`.

### 2b. Sentinel-error structure

From `internal/registry/errors.go` and `internal/deploy/service.go:23-30`:

- Every sentinel is `var ErrXxx = errors.New("package: human description")`.
- The package prefix matches the import path's leaf (`registry: ...`, `deploy: ...`, `envcap: ...`).
- Wrap with `fmt.Errorf("%w: <ctx>: %w", OuterSentinel, innerErr)` per `error-wrap-discipline.md`. **Never `%v` for an error.** This rule is locked by `TestDeploy_BuildErrorPreservesInnerSentinel`.

### 2c. CLI flag wiring

Surface-coherence from `cli-flag-surface-coherence.md`. Every flag has 4 surfaces; M2 has a 5th transient surface (the milestone-token semantic test) that disappears. The four permanent surfaces:

| Surface | File | M2 change |
|---|---|---|
| Runtime check (validation) | `internal/cli/deploy_service.go` `runDeployService` body | M1 rejection deleted; M2 calls `parseMountFlags` |
| Error message | same place; uses the templates in Decision 7 | flips |
| `--help` text | `internal/cli/deploy_service.go:61` (the `StringSliceVar` third arg) | flips to Decision 6 wording |
| `_docs/usage.md` | line 71 (and exit-code-table line 99) | Raymond rewrites in his step |

### 2d. Cleanup-context discipline

`internal/deploy/service.go:32-42` already has `newCleanupContext()`. M2 doesn't add new cleanup paths because the recreate strategy is unchanged — the cleanup blocks at `service.go:254-256`, `:281-293`, `:337-354` already work. **However**: when those cleanup blocks call `restoreOldContainer`, M2 must populate `runReq.Volumes` from `prev.Config.Run.Mounts` so the rolled-back container gets the same mounts. Don §5 names this site (`internal/deploy/service.go:374-382`).

### 2e. Gomock InOrder for orchestrator step ordering

Per `_ai/gomock-inorder-sequencing.md`. Mount-passing tests follow the same pattern: the test pins `gomock.InOrder(driver.EXPECT().Run(ctx, runReqWithVolumes), ...)` where `runReqWithVolumes` matches a `RunRequest` whose `Volumes` field carries the expected slice. See "test surface" §4 for the exact matcher shape.

---

## 3. Exact code-level diffs

This section is the implementation contract. Pseudocode where the diff is short; full Go where exactness matters.

### 3.1. `internal/registry/types.go` — add `IsNamed` helper

After line 63, add:

```go
// IsNamed reports whether m references a Docker named volume (rather than a
// host bind path). The convention: Source paths starting with "/" are bind
// mounts; all other forms are named-volume references. The TOML field name
// (host_path) is historical from M1; the Go-side meaning is "source", which
// may be either kind.
func (m Mount) IsNamed() bool {
    return m.HostPath != "" && !strings.HasPrefix(m.HostPath, "/")
}
```

Add `"strings"` to the imports. Note `"time"` is already imported.

### 3.2. `internal/registry/mount.go` — NEW FILE

```go
package registry

import (
    "errors"
    "fmt"
    "path/filepath"
    "regexp"
    "strings"
)

// ErrInvalidMount is returned by ValidateMount and ValidateMounts when a Mount
// fails grammar/syntax validation. CLI wraps the parse error with errUsage
// (exit 2); the loader wraps with this sentinel (exit 10) when the failure
// is on disk in a hand-edited TOML.
var ErrInvalidMount = errors.New("registry: invalid mount")

// volumeNameRE locks Docker's named-volume name format. Sourced from
// docker/docker source (volume.IsValidName uses [a-zA-Z0-9][a-zA-Z0-9_.-]+).
// First char is alphanumeric; subsequent chars add underscore, dot, hyphen.
var volumeNameRE = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]+$`)

// ValidateMount reports whether m is a well-formed Mount. It performs
// grammar-only checks: source non-empty, container_path absolute, named-volume
// regex, mode flag absent (callers express :ro via the bool field, not a
// string flag inside the Mount). It does NOT stat the host path (see
// _tasks/2026-04-28-m2-server-side-mounts/003-joel-tech-plan.md §1 Decision 1).
func ValidateMount(m Mount) error {
    if m.HostPath == "" {
        return fmt.Errorf("source must be non-empty")
    }
    if m.ContainerPath == "" {
        return fmt.Errorf("container_path must be non-empty")
    }
    if !filepath.IsAbs(m.ContainerPath) {
        return fmt.Errorf("container_path must be absolute, got %q", m.ContainerPath)
    }
    if !strings.HasPrefix(m.HostPath, "/") {
        // Named volume.
        if !volumeNameRE.MatchString(m.HostPath) {
            return fmt.Errorf("named-volume source %q must match [a-zA-Z0-9][a-zA-Z0-9_.-]+", m.HostPath)
        }
    }
    return nil
}

// ValidateMounts validates each entry and additionally rejects duplicate
// container_paths within the slice. Returns the index, name, and the per-mount
// error wrapped with ErrInvalidMount.
//
// `name` is the service name; `path` is the on-disk TOML path (loader use) or
// the literal "<command-line>" (CLI use). Both appear in the wrapped error
// for operator-debug context.
func ValidateMounts(mounts []Mount, name, path string) error {
    seen := make(map[string]int, len(mounts))
    for i, m := range mounts {
        if err := ValidateMount(m); err != nil {
            return fmt.Errorf("%w: service %q mount[%d] in %s: %w", ErrInvalidMount, name, i, path, err)
        }
        if first, dup := seen[m.ContainerPath]; dup {
            return fmt.Errorf("%w: service %q mount[%d] in %s: duplicate container_path %q (also at mount[%d])",
                ErrInvalidMount, name, i, path, m.ContainerPath, first)
        }
        seen[m.ContainerPath] = i
    }
    return nil
}

// ParseMountString parses one --mount flag value into a Mount. Format:
//
//	<source>:<container-path>[:ro]
//
// where <source> is either an absolute host path (starts with "/") or a
// Docker named-volume name. Returns the constructed Mount and a non-nil error
// (NOT wrapped with ErrInvalidMount) if the grammar fails. The CLI is
// responsible for wrapping with errUsage; the loader does not call this
// helper (it operates on already-deserialised []Mount).
func ParseMountString(raw string) (Mount, error) {
    if raw == "" {
        return Mount{}, fmt.Errorf("empty")
    }
    parts := strings.Split(raw, ":")
    var src, target string
    var ro bool
    switch len(parts) {
    case 2:
        src, target = parts[0], parts[1]
    case 3:
        src, target = parts[0], parts[1]
        switch parts[2] {
        case "ro":
            ro = true
        case "rw":
            return Mount{}, fmt.Errorf("unsupported mode flag %q (only \"ro\" is supported)", parts[2])
        default:
            return Mount{}, fmt.Errorf("unsupported mode flag %q (only \"ro\" is supported)", parts[2])
        }
    default:
        return Mount{}, fmt.Errorf("expected <source>:<target>[:ro], got %d component(s)", len(parts))
    }
    m := Mount{HostPath: src, ContainerPath: target, ReadOnly: ro}
    if err := ValidateMount(m); err != nil {
        return Mount{}, err
    }
    return m, nil
}
```

**Why `ParseMountString` is exported:** it lives in `registry` for shared discoverability with `ValidateMount`, and the CLI imports `registry` already. Not exporting it would force the CLI to duplicate the split-on-`:` logic.

**Why `rw` is explicitly named:** an operator typing `--mount /foo:/bar:rw` is making a category error (rw is the default; saying it does not pretend mode flags work). Naming `"rw"` in the error is more helpful than the generic "unsupported".

### 3.3. `internal/registry/errors.go` — sentinel swap

```go
package registry

import "errors"

var (
    ErrNotFound       = errors.New("registry: service not found")
    ErrSecretsMissing = errors.New("registry: config exists but secrets file is missing")
    ErrPermissionMode = errors.New("registry: file or directory has wrong permission mode")
    ErrSchemaMismatch = errors.New("registry: schema_version mismatch")
    ErrUnknownField   = errors.New("registry: unknown field in TOML")
    ErrInvalidMount   = errors.New("registry: invalid mount")    // NEW (M2)
    ErrInvalidStrategy = errors.New("registry: strategy not supported in M1")
    ErrPartialWrite   = errors.New("registry: partial write (config wrote, secrets failed)")
)
```

`ErrMountsNotSupported` is **deleted**. (Don §6 list confirms no other consumer.)

The order: `ErrInvalidMount` slots in the same alphabetical-ish position the deleted sentinel occupied, between `ErrUnknownField` and `ErrInvalidStrategy`, to minimize diff churn.

**Note on duplication:** `ValidateMount` (in `mount.go`) defines `ErrInvalidMount` body inline above as part of the new file. Final answer: define `ErrInvalidMount` in `errors.go` (alongside the rest) and remove its definition from `mount.go`. Same package, no import issue. Update §3.2 accordingly: drop the `var ErrInvalidMount = errors.New(...)` line from `mount.go`.

### 3.4. `internal/registry/store.go:64-71` — replace the rejection branch

Replace lines 68-71:

```go
// OLD:
if len(cfg.Run.Mounts) > 0 {
    return nil, fmt.Errorf("%w: service %q declares %d mount(s) in %s; mounts are not supported until M2",
        ErrMountsNotSupported, cfg.Name, len(cfg.Run.Mounts), cfgPath)
}
```

with:

```go
// NEW:
if err := ValidateMounts(cfg.Run.Mounts, name, cfgPath); err != nil {
    return nil, err
}
```

Note: `cfg.Name` is overwritten from the filename by line 76 (`cfg.Name = name`) AFTER the existing check — but the rejection message already used `cfg.Name`, which can be empty if the TOML has no `name = "..."` line. Use `name` (the function parameter) instead, which is always populated. This is a fix-while-fresh papercut, in scope per §7.

### 3.5. `internal/cli/deploy_service.go` — flag flip and parse-and-validate

Three changes in the file:

**(a) Line 61 — help text:**

```go
cmd.Flags().StringSliceVar(&f.Mounts, "mount",
    nil,
    "persistent volume; <host-path>:<container-path>[:ro] (bind) or <name>:<container-path>[:ro] (named volume); repeatable")
```

**(b) Lines 71-73 — replace the rejection block with parse+validate:**

```go
// OLD (lines 71-73):
if len(f.Mounts) > 0 {
    return fmt.Errorf("--mount is not supported until M2: %w", registry.ErrMountsNotSupported)
}

// NEW:
mounts, err := parseMountFlags(f.Mounts)
if err != nil {
    return err
}
```

The block goes **before** the strategy check, preserving the "first-error wins" order: usage errors (mount, port) before config errors (strategy).

Hmm wait — re-checking Don §2 + Decision 5: malformed `--mount` is `errUsage` (exit 2), `--strategy=blue_green` is `ErrInvalidStrategy` (exit 10), `--port=0` is `errUsage` (exit 2). Original M1 order at `deploy_service.go:71-78` is: mount, strategy, port. We should preserve "validate inputs in declared-flag order so errors are predictable." Mount stays first.

**(c) Lines 96-106 — populate `req.Mounts`:**

```go
req := deploy.Request{
    Name:             f.Name,
    SourceDir:        abs,
    Dockerfile:       dockerfile,
    Hosts:            f.Hosts,
    Port:             f.Port,
    EnvFile:          envFile,
    Mounts:           mounts,             // NEW
    ReadinessPath:    f.ReadinessPath,
    ReadinessTimeout: f.ReadinessTimeout,
    Strategy:         f.Strategy,
}
```

**(d) NEW helper at the bottom of the file:**

```go
// parseMountFlags converts repeatable --mount string values into validated
// []registry.Mount entries. The same registry.ValidateMounts the loader uses
// runs here so the CLI and on-disk surfaces enforce identical rules. CLI
// failures wrap errUsage (exit 2); the loader wraps ErrInvalidMount (exit 10).
func parseMountFlags(raw []string) ([]registry.Mount, error) {
    if len(raw) == 0 {
        return nil, nil
    }
    out := make([]registry.Mount, 0, len(raw))
    for _, s := range raw {
        m, err := registry.ParseMountString(s)
        if err != nil {
            return nil, fmt.Errorf("--mount %q: %s: %w", s, err.Error(), errUsage)
        }
        out = append(out, m)
    }
    if err := registry.ValidateMounts(out, "<command-line>", "<command-line>"); err != nil {
        // ValidateMounts wraps with ErrInvalidMount; for CLI use we re-wrap
        // with errUsage so the exit code is 2, not 10. The duplicate-target
        // case is the one ParseMountString cannot detect; this is its catcher.
        return nil, fmt.Errorf("--mount: %s: %w", err.Error(), errUsage)
    }
    return out, nil
}
```

Why call `ValidateMounts` after the parse loop: `ParseMountString` operates on one mount; the duplicate-target check is cross-mount. Two passes, single source of truth (the cross-mount check happens in exactly one place: `ValidateMounts`).

**On error chain:** `errors.Is(err, errUsage)` is the assertion that maps to exit 2 (`exit_codes.go:39`). The wrapped chain is `--mount %q: <reason>: usage error`, where `<reason>` is the `ParseMountString`/`ValidateMounts` error string (no sentinel — they were `errors.New`-style local errors inside `mount.go`). The CLI does NOT need `errors.Is(err, ErrInvalidMount)` to match — exit 2 is the right code regardless of which validation rule failed. **Internal-error sentinel inside `mount.go` parse helpers is a bare `errors.New` with no exported name** — they only exist as message strings in the wrap chain.

### 3.6. `internal/cli/exit_codes.go` — drop `ErrMountsNotSupported`

Lines 41-50 currently:

```go
case errors.Is(err, registry.ErrMountsNotSupported),
    errors.Is(err, registry.ErrInvalidStrategy),
    errors.Is(err, registry.ErrSchemaMismatch),
    errors.Is(err, registry.ErrUnknownField),
    errors.Is(err, registry.ErrPermissionMode),
    errors.Is(err, registry.ErrSecretsMissing),
    errors.Is(err, registry.ErrNotFound),
    errors.Is(err, envcap.ErrEnvScriptMissing),
    errors.Is(err, envcap.ErrEnvScriptUnreadable):
    return ExitConfigError
```

Replace with:

```go
case errors.Is(err, registry.ErrInvalidMount),
    errors.Is(err, registry.ErrInvalidStrategy),
    errors.Is(err, registry.ErrSchemaMismatch),
    errors.Is(err, registry.ErrUnknownField),
    errors.Is(err, registry.ErrPermissionMode),
    errors.Is(err, registry.ErrSecretsMissing),
    errors.Is(err, registry.ErrNotFound),
    errors.Is(err, envcap.ErrEnvScriptMissing),
    errors.Is(err, envcap.ErrEnvScriptUnreadable):
    return ExitConfigError
```

The `ErrInvalidMount` entry is for the loader's exit-10 path. The CLI's exit-2 path is already covered by the existing `errors.Is(err, errUsage)` case at line 39.

### 3.7. `internal/dockerdrv/driver.go` — `RunRequest.Volumes` field

Add to `RunRequest` struct (lines 31-39):

```go
type RunRequest struct {
    Name    string
    Image   string
    Network string
    Env     map[string]string
    Restart string
    Port    int
    Volumes []VolumeMount  // NEW: emitted in declared order, one -v per entry
}
```

`VolumeMount` already exists at line 66-71; no change there.

### 3.8. `internal/dockerdrv/cli_driver.go` — wire `Volumes` through `Run`

Add to `cliDriver.Run` between line 60 and 61 (after the env loop, before the label append, mirroring `RunWithOptions`):

```go
for _, v := range req.Volumes {
    args = append(args, "-v", formatVolume(v))
}
```

The `formatVolume` helper at line 285-291 is reused as-is. No change to argv shape: `-v <source>:<target>[:ro]`, locked by `TestCLIDriver_RunWithOptionsBindReadOnly` and `TestCLIDriver_RunWithOptionsNamedVolumeNotReadOnly`.

**Argv order:** `--name`, `--network`, `--restart`, `--env` (sorted), `-v` (declared order), `--label`, `<image>`. Matches `RunWithOptions`'s declared order: env-keys-sorted, labels-keys-sorted, ports-declared, volumes-declared, image. Same shape.

### 3.9. `internal/deploy/service.go` — three sites populate `Volumes`

**(a) Add `Mounts` field to `Request`** (lines 52-62):

```go
type Request struct {
    Name             string
    SourceDir        string
    Dockerfile       string
    Hosts            []string
    Port             int
    EnvFile          string
    Mounts           []registry.Mount  // NEW
    ReadinessPath    string
    ReadinessTimeout time.Duration
    Strategy         string
}
```

**(b) Helper for `[]registry.Mount` → `[]dockerdrv.VolumeMount`:**

Add at the end of `service.go`:

```go
// toVolumeMounts converts persisted registry mounts into the driver's
// VolumeMount shape. The IsNamed flag is derived from the source string
// using the convention documented on registry.Mount.IsNamed: bind sources
// start with "/", named-volume sources do not.
func toVolumeMounts(mounts []registry.Mount) []dockerdrv.VolumeMount {
    if len(mounts) == 0 {
        return nil
    }
    out := make([]dockerdrv.VolumeMount, 0, len(mounts))
    for _, m := range mounts {
        out = append(out, dockerdrv.VolumeMount{
            Source:   m.HostPath,
            Target:   m.ContainerPath,
            ReadOnly: m.ReadOnly,
            IsNamed:  m.IsNamed(),
        })
    }
    return out
}
```

**(c) Site 1 — fresh deploy, lines 243-251:**

```go
runReq := dockerdrv.RunRequest{
    Name:    containerName,
    Image:   imageRef,
    Network: "decloud",
    Env:     captured,
    Restart: "unless-stopped",
    Port:    req.Port,
    Volumes: toVolumeMounts(req.Mounts),  // NEW
}
```

**(d) Site 2 — `restoreOldContainer`, lines 374-382:**

```go
runReq := dockerdrv.RunRequest{
    Name:    ids.ContainerName(prev.Config.Name),
    Image:   prev.Config.Build.ImageRef,
    Network: "decloud",
    Env:     prev.Secrets.Env,
    Restart: prev.Config.Run.Restart,
    Port:    prev.Config.Run.Port,
    Volumes: toVolumeMounts(prev.Config.Run.Mounts),  // NEW
}
```

**(e) Site 3 — registry save, line 314-319:**

```go
Run: registry.RunSpec{
    Network: "decloud",
    Port:    req.Port,
    Restart: "unless-stopped",
    Mounts:  req.Mounts,        // CHANGED from []registry.Mount{}
},
```

If `req.Mounts == nil`, this stores `nil` rather than `[]registry.Mount{}`. TOML marshalling of nil slice is the same as empty slice (`mounts = []`); decode round-trips through `cfg.Run.Mounts == nil` (the M1 default for a fresh struct). Loader's `ValidateMounts(nil, ...)` returns nil — no validation runs over zero entries. **Round-trip-safe.**

### 3.10. `internal/deploy/lifecycle.go:67-78` — Start absent-branch

The `default:` arm at lines 67-78 needs the same `Volumes` field:

```go
default:
    runReq := dockerdrv.RunRequest{
        Name:    containerName,
        Image:   prev.Config.Build.ImageRef,
        Network: "decloud",
        Env:     prev.Secrets.Env,
        Restart: prev.Config.Run.Restart,
        Port:    prev.Config.Run.Port,
        Volumes: toVolumeMounts(prev.Config.Run.Mounts),  // NEW
    }
    if _, err := d.deps.Driver.Run(ctx, runReq); err != nil {
        return fmt.Errorf("%w: run %s: %w", ErrRun, containerName, err)
    }
    return nil
```

`toVolumeMounts` is in `service.go` (same package); accessible directly. No import changes.

### 3.11. Mock regeneration

After Rob lands the production diff, regenerate:

```bash
go generate ./...
```

Files affected:
- `internal/dockerdrv/mocks/mock_driver.go` — `Run` signature unchanged at the Go level (RunRequest is a struct, the field is internal). No regen needed. Verify with `grep "Run(ctx context.Context, req dockerdrv.RunRequest)"` — should match the existing line.
- `internal/cli/mocks/mock_deployer.go`, `mock_lifecycle.go` — `deploy.Request` shape changes (added `Mounts` field). No interface signature change either. **No regen needed.**
- `internal/registry/mocks/mock_store.go` — `Service` shape unchanged; only values inside `RunSpec.Mounts` change. **No regen needed.**

**Net mock impact: zero files.** This is a happy outcome of using struct-field additions instead of interface-method changes (the Option-β payoff).

Rob still runs `go generate ./...` for hygiene; the diff should be empty.

---

## 4. Test surface for Kent

Subject names follow existing patterns. Don't write the tests — specify them.

### 4.1. `internal/registry/mount_test.go` — NEW file (table-driven)

`TestValidateMount_Table`:

| Subtest name | Input | Expected |
|---|---|---|
| `valid_bind_rw` | `Mount{"/host", "/data", false}` | nil |
| `valid_bind_ro` | `Mount{"/host", "/data", true}` | nil |
| `valid_named_rw` | `Mount{"mydata", "/var", false}` | nil |
| `valid_named_ro` | `Mount{"mydata", "/var", true}` | nil |
| `empty_source` | `Mount{"", "/data", false}` | err containing `"source must be non-empty"` |
| `empty_target` | `Mount{"/host", "", false}` | err containing `"container_path must be non-empty"` |
| `relative_target` | `Mount{"/host", "data", false}` | err containing `"container_path must be absolute"` |
| `named_invalid_chars` | `Mount{"my data", "/var", false}` | err containing `"named-volume source"` |
| `named_starts_with_dash` | `Mount{"-x", "/var", false}` | err containing `"named-volume source"` |
| `named_too_short` | `Mount{"x", "/var", false}` | err containing `"named-volume source"` (regex requires >=2 chars) |

`TestValidateMounts_DuplicateContainerPath`: two mounts with same `ContainerPath`. Asserts `errors.Is(err, ErrInvalidMount)` and `err.Error()` contains `"duplicate container_path"` and both mount indices `[0]` and `[1]`.

`TestValidateMounts_FirstInvalidStops`: three mounts; mount[1] is invalid. Asserts the error names `mount[1]`, not `mount[2]`.

`TestValidateMounts_EmptyAndNilSliceAreNoOp`: `ValidateMounts(nil, "n", "p")` returns nil; `ValidateMounts([]Mount{}, "n", "p")` returns nil.

`TestParseMountString_Table`:

| Subtest | Raw | Expected Mount or err |
|---|---|---|
| `bind_rw` | `/host:/data` | `Mount{"/host", "/data", false}` |
| `bind_ro` | `/host:/data:ro` | `Mount{"/host", "/data", true}` |
| `named_rw` | `mydata:/var` | `Mount{"mydata", "/var", false}` |
| `named_ro` | `mydata:/var:ro` | `Mount{"mydata", "/var", true}` |
| `empty` | `""` | err `"empty"` |
| `single_component` | `/host` | err containing `"got 1 component(s)"` |
| `four_components` | `/a:/b:ro:extra` | err containing `"got 4 component(s)"` |
| `mode_rw_explicit` | `/h:/d:rw` | err containing `"unsupported mode flag \"rw\""` |
| `mode_unknown` | `/h:/d:zz` | err containing `"unsupported mode flag \"zz\""` |

`TestMount_IsNamed`: `Mount{"/host", ...}.IsNamed()` is false; `Mount{"vol", ...}.IsNamed()` is true; `Mount{"", ...}.IsNamed()` is false (empty defensive).

### 4.2. `internal/registry/store_test.go` — DELETE one test, ADD two

**DELETE:** `TestStore_LoadRejectsNonEmptyMounts` (lines 261-300). The contract it asserts is gone.

**KEEP unchanged:** `TestStore_LoadAcceptsEmptyMountsArray` (lines 251-259). Still meaningful — empty mounts must still be a valid round-trip.

**ADD:**

`TestStore_LoadAcceptsValidMounts`:
- TOML body with two mounts: one bind (`host_path = "/host/data"`, `container_path = "/data"`, `read_only = false`) and one named volume (`host_path = "mydata"`, `container_path = "/var/lib"`, `read_only = true`).
- `store.Load("foo")` returns no error.
- `svc.Config.Run.Mounts` length is 2.
- Round-trip both fields.

`TestStore_LoadRejectsInvalidMounts` (table-driven):
- Subtests: `relative_container_path`, `named_volume_invalid_chars`, `duplicate_container_path`.
- Each writes a config TOML with the invalid shape.
- Each asserts `errors.Is(err, registry.ErrInvalidMount)`.
- Each asserts `err.Error()` contains the path `cfgPath` (via the `services/foo.toml` substring) and the mount index (`mount[N]`).

### 4.3. `internal/cli/deploy_service_test.go` — DELETE two, ADD three

**DELETE:**
- `TestDeployService_MountFlagReturnsErrMountsNotSupported` (lines 81-95). Contract gone.
- `TestDeployService_MountFlagHelpReferencesM2` (lines 97-104). Semantic-token contract has no token left (Don §7).

**ADD:**

`TestDeployService_MountFlagAccepted_PassesMountsThrough`:
- Args: `--mount /h1:/c1 --mount /h2:/c2:ro --mount vol1:/c3:ro`.
- Mock deployer captures `req`.
- Asserts `len(req.Mounts) == 3`.
- Asserts `req.Mounts[0] == registry.Mount{"/h1", "/c1", false}`.
- Asserts `req.Mounts[1] == registry.Mount{"/h2", "/c2", true}`.
- Asserts `req.Mounts[2] == registry.Mount{"vol1", "/c3", true}`.
- Asserts `req.Mounts[2].IsNamed()` is true.

`TestDeployService_MountFlagInvalidReturnsExitUsageError` (table-driven):
- Subtests: `single_component` (`--mount /foo`), `unknown_mode` (`--mount /h:/c:zz`), `empty_target` (`--mount /h:`), `relative_target` (`--mount /h:relative`), `duplicate_target` (`--mount /a:/x --mount /b:/x`).
- Each asserts `errors.Is(err, errUsage)` and `ExitCodeFor(err) == ExitUsageError`.
- Each asserts `err.Error()` starts with `--mount`.

`TestDeployService_MountFlagEmptyIsValid`:
- No `--mount` flags.
- Mock deployer captures `req`.
- Asserts `req.Mounts` is nil or zero-length (matches M1 behaviour for a missing flag).

### 4.4. `internal/cli/exit_codes_test.go` — DELETE one entry, ADD one

In the table at line 24:

**DELETE:** `{"mounts", registry.ErrMountsNotSupported, ExitConfigError},`

**ADD:** `{"invalid-mount", registry.ErrInvalidMount, ExitConfigError},`

`TestExitCodeFor_AllSentinels` covers both the wrapped and bare sentinel cases — the existing `wrapped-usage` entry already covers wrapped `errUsage` for the CLI parse path. No additional test needed.

### 4.5. `internal/dockerdrv/cli_driver_test.go` — ADD one

`TestCLIDriver_RunPassesVolumeFlags`:
- Build a `RunRequest` with two `Volumes`: one bind ro, one named rw.
- Call `Run(ctx, req)` via the recording factory.
- Assert `volumeFlagsFromArgs(records[0].Args)` returns `["/host:/dst:ro", "vol:/dst"]`.

This is the parallel of `TestCLIDriver_RunWithOptionsBindReadOnly` (line 405) for the new `Run`-with-volumes path. Locks the argv shape byte-for-byte. **Same test discipline as the existing port-publishing tests.**

### 4.6. `internal/deploy/service_test.go` — ADD three

`TestDeploy_DeployWithMountsPassesVolumesToDriver`:
- `req.Mounts = []registry.Mount{{"/host", "/data", true}, {"vol", "/var", false}}`.
- gomock matcher on `Driver.Run`: capture the `RunRequest`, assert `Volumes` slice has the equivalent `[]VolumeMount` after `toVolumeMounts` conversion.
- Assert `IsNamed` derivation: `Volumes[0].IsNamed == false`, `Volumes[1].IsNamed == true`.

`TestDeploy_DeployWithMountsSavesMountsToRegistry`:
- Same `req.Mounts` as above.
- Capture `Store.Save` call.
- Assert `svc.Config.Run.Mounts` equals `req.Mounts` (round-trip-shape).

`TestDeploy_RestoreOldContainerPassesVolumesToDriver`:
- `prev.Config.Run.Mounts` populated.
- Force a `Driver.Run` failure on the new container so cleanup invokes `restoreOldContainer`.
- gomock matcher on the second `Driver.Run` (for `prev`): assert `Volumes` matches `toVolumeMounts(prev.Config.Run.Mounts)`.

The matcher follows the `notCancelledCtxMatcher` shape at `service_test.go:71-85`: a custom struct implementing `gomock.Matcher` whose `Matches(x any) bool` checks `req.Volumes` deep-equality.

### 4.7. `internal/deploy/lifecycle_test.go` — ADD one

`TestLifecycle_StartAbsentBranchPassesVolumesToDriver`:
- `Store.Load` returns a service with `Config.Run.Mounts` populated.
- `Driver.Inspect` returns `State: "absent"` (drives the `default:` arm at lifecycle.go:67-78).
- Assert `Driver.Run` is called with `runReq.Volumes` == converted mounts.

### 4.8. `internal/integration/mount_test.go` — NEW (build-tagged)

```go
//go:build integration

package integration_test
```

Per Don §8 + Decision 8 above (one integration test, mount-only):

- `TestIntegration_MountBindRoundTrip`:
  - Skip if `os.Getenv("DECLOUD_INTEGRATION") != "1"`.
  - `t.Setenv("DECLOUD_ROOT", t.TempDir())` (hermetic config root).
  - Bring up Caddy: shell out to `decloud caddy up`. `t.Cleanup(decloud caddy down)`.
  - Build a tiny inline image: `docker build -t decloud-mounttest-img -f - .` with a heredoc Dockerfile (`FROM alpine:3.19`, `CMD sleep 3600`).
  - Create host directory `t.TempDir()` containing `marker.txt` with bytes `"hello-mount-m2"`.
  - Run: `decloud deploy service --name mounttest --port 80 --readiness-timeout 5s --mount <tmpdir>:/data:ro <source-dir>` against the real daemon. (The `--port=80` / readiness will fail because nothing's listening — this is OK because the test asserts on `docker exec` AFTER the deploy attempt, but **wait**: deploy failure stops + removes the container. Need a different approach: pre-deploy a no-readiness-required service.)
  - **Revised approach:** the test deploys a service whose Dockerfile runs an HTTP healthz responder (e.g. `nc -lk -p 80 -e 'echo -e "HTTP/1.1 200 OK\r\n\r"'` or a 5-line Go binary). Or — more honest — skip the deploy orchestrator entirely for this test and call `dockerdrv.NewCLIDriver().Run(ctx, RunRequest{... Volumes: [...]})` directly. **Decision: call the driver directly.** The test's purpose is "real Docker accepts our argv with mounts" — not "the deploy orchestrator end-to-end works." End-to-end is a different test (m1x-backlog item 10).
  - `Driver.Run(ctx, RunRequest{Name: "decloud-mounttest", Image: "alpine:3.19", Network: "decloud", Restart: "no", Volumes: [...one bind ro mount...]})`.
  - `Driver.Exec(ctx, ExecOptions{Container: "decloud-mounttest", Cmd: []string{"cat", "/data/marker.txt"}, Stdout: &buf})`.
  - Assert `buf.String() == "hello-mount-m2"`.
  - `t.Cleanup` with idempotent `docker rm -f decloud-mounttest`.

This is **simpler** than the deploy-orchestrator integration test Don sketched, because it tests one thing only: real Docker, real driver, real bind mount, real `docker exec` cat. The deploy-orchestrator integration test (which would also hit `docker build` + readiness + Caddyfile generation + reload) is the natural home for a future `TestIntegration_DeployServiceEndToEnd` — m1x-backlog item 10.

- `internal/integration/doc.go`:

```go
// Package integration contains build-tagged integration tests that exercise
// real Docker. Run with:
//
//   DECLOUD_INTEGRATION=1 go test -tags integration ./internal/integration/...
//
// Tests skip if DECLOUD_INTEGRATION is not set to "1".
//
//go:build integration

package integration_test
```

The `//go:build integration` tag on `doc.go` means the package only compiles under the tag, which keeps `go test ./...` (no tag) from even seeing the directory.

---

## 5. Explicit deletion list (the M2 cuts of M1 cuts)

1. **`registry.ErrMountsNotSupported`** — `internal/registry/errors.go:11` line. Deleted entirely.
2. **Loader rejection block** — `internal/registry/store.go:68-71` (the `len(cfg.Run.Mounts) > 0` branch). Replaced with `ValidateMounts` call.
3. **CLI rejection block** — `internal/cli/deploy_service.go:71-73` (the `len(f.Mounts) > 0` branch). Replaced with `parseMountFlags` call.
4. **Exit-code mapping entry** — `internal/cli/exit_codes.go:41` (the `errors.Is(err, registry.ErrMountsNotSupported)` line). Replaced by `errors.Is(err, registry.ErrInvalidMount)` line in the same case.
5. **Test: `TestDeployService_MountFlagReturnsErrMountsNotSupported`** — `internal/cli/deploy_service_test.go:81-95`. Deleted.
6. **Test: `TestDeployService_MountFlagHelpReferencesM2`** — `internal/cli/deploy_service_test.go:97-104`. Deleted, NOT replaced (Don §7 rule: semantic-token tests die when the token dies; we don't replace with a prose-snapshot test).
7. **Test: `TestStore_LoadRejectsNonEmptyMounts`** — `internal/registry/store_test.go:261-300`. Deleted.
8. **Test-table entry: `{"mounts", registry.ErrMountsNotSupported, ExitConfigError}`** — `internal/cli/exit_codes_test.go:24`. Deleted, replaced with `{"invalid-mount", registry.ErrInvalidMount, ExitConfigError}`.

---

## 6. Surface-flip atomic commit plan

Don §14 item 9 says "five surfaces flip together in one commit." This is correct. Confirming with rationale:

**The half-flipped problem.** If Rob lands surface 1 (CLI accepts the flag) before surface 2 (loader accepts populated mounts), the CLI would write a TOML with non-empty `mounts` that the loader would reject on the next `decloud start`. The test suite would NOT catch this — `internal/cli/` tests use a mock deployer; `internal/deploy/` tests use a mock store; nothing in unit-test land round-trips through the on-disk loader. A multi-commit landing means the suite is green at every commit individually, and yet the cumulative diff has a bug class.

**The atomic commit MUST contain:**

1. `internal/registry/errors.go` — sentinel swap.
2. `internal/registry/types.go` — `IsNamed` helper.
3. `internal/registry/mount.go` — NEW file with `ValidateMount`, `ValidateMounts`, `ParseMountString`.
4. `internal/registry/store.go` — replace rejection with `ValidateMounts`.
5. `internal/cli/deploy_service.go` — flag flip (help text + parse call) + `parseMountFlags` helper.
6. `internal/cli/exit_codes.go` — sentinel swap in case-list.
7. `internal/dockerdrv/driver.go` — `Volumes []VolumeMount` field on `RunRequest`.
8. `internal/dockerdrv/cli_driver.go` — `for _, v := range req.Volumes` loop in `Run`.
9. `internal/deploy/service.go` — `Mounts` field on `Request`, `toVolumeMounts` helper, three `Volumes` populations.
10. `internal/deploy/lifecycle.go` — `Volumes` population in `Start` `default:` arm.

**Tests** can be a separate commit (the test-first discipline says Kent commits tests before Rob commits production, but Kent's test file additions/deletions still need to land all-together for the suite to be green). Recommended commit shape:

- **Commit A (Kent):** all test-file changes — new `mount_test.go`, modifications to `store_test.go`/`deploy_service_test.go`/`exit_codes_test.go`/`service_test.go`/`lifecycle_test.go`/`cli_driver_test.go`, plus the new `internal/integration/mount_test.go`. **At this commit, the suite is RED** because the production sentinels and surfaces don't match the new tests yet. Per CLAUDE.md test-first discipline, this is the expected state.
- **Commit B (Rob):** all 10 production files in §6. **At this commit, the suite is GREEN.**

If the workflow forbids a red-suite intermediate commit (CLAUDE.md doesn't explicitly), Kent's tests can be split: tests for *deletions* in Commit A (which still pass against the M1 production), then Commit B production + new positive tests. But this complicates Kent's deliverable. I recommend **two commits, one red, one green**, with Rob's commit message naming the test-first ordering.

**What I am NOT recommending:** ten micro-commits, one per file. The test suite would pass at most of them but the cumulative semantic state would be incoherent at any cherry-pick.

---

## 7. Fix-while-fresh sweep

Don's plan §12 listed the audit-trail flips. I verify and add any survivors.

### Verified Don-listed flips

- `_docs/usage.md:71` — confirmed.
- `_docs/usage.md:99` (the `--mount used` exit-code-table entry) — confirmed.
- `_ai/decisions/m1-scope.md:32` (env-file hardening phantom) — confirmed.
- `_ai/decisions/m1-scope.md:16` (the M2-future cut paragraph) — confirmed.
- `_ai/decisions/secrets-split.md:24` (the `ErrMountsNotSupported (M1)` rejection-class entry) — confirmed.
- `_ai/decisions/schema-versioning.md:11` and `:16` — confirmed.
- `_ai/MEMORY.md:7` (env-file hardening) and `:9` (mounts populate at M2) — confirmed.
- `_ai/m1x-backlog.md` item 6 — confirmed; per Decision 9 it splits into items 6 (struck through), 9 (reloader %q), 10 (curl-through-Caddy), 11 (Driver.Run consolidation).

### Pre-existing-bug papercut found during my trace pass

**`internal/registry/store.go:68-71` — error message uses `cfg.Name`, which is empty if the TOML omits `name = "..."`.** Fix-while-fresh applies (mechanical, same file, <5min, on-theme since the line is being rewritten). The new `ValidateMounts(cfg.Run.Mounts, name, cfgPath)` call uses `name` (the function parameter from the filename), which is always populated. **This is not a bug fix in the strict sense** — by the time this code runs, all M1 happy-path TOMLs have a `name` field, and the strict-mode TOML decoder doesn't enforce required fields. But naming the file's stem is clearly the right operator-debug context.

Already incorporated into §3.4 above.

### Survivors my trace pass found

- **`_ai/decisions/schema-versioning.md:16`** — the line "M1's loader rejects non-empty `Mounts` with the same `ErrMountsNotSupported` as the CLI's `--mount` flag (closes the hand-edit loophole)." This is M2-already-shipped wording. Raymond rewrites to "M1's loader rejected non-empty `Mounts` with `ErrMountsNotSupported` (deleted at M2 ship); M2's loader runs `ValidateMounts` to enforce the same data-integrity rules as the CLI parser."
- **`_ai/MEMORY.md:9`** — Don listed it. Confirmed. Same shape: "(mounts populate at M2, secret-files at M7)" → "(mounts populated since M2, secret-files at M7)."
- **`_ai/cli-flag-surface-coherence.md:42`** — Don listed it. Confirmed. The carve-out narrates: "`TestDeployService_MountFlagHelpReferencesM2` was the live example; deleted at M2 ship per `_tasks/2026-04-28-m2-server-side-mounts/`. The carve-out remains valid for any future milestone-token contract." 
- **`_ai/decisions/m1-test-strategy.md`** — needs spot-check for any "m1.x integration test deferred" wording. Raymond reads end-to-end per `fix-now-while-fresh.md` Refinement section.
- **`_ai/m1x-backlog.md` item 4 (`Capture("")` comment)** — Don asked confirmation. Read confirmed: still applies post-M2 (the comment-only clarification is unchanged by mounts work). NO action.

### Pre-existing bug in `_ai/decisions/secrets-split.md`?

Don §12 asked me to verify. Read end-to-end: `secrets-split.md:6` mentions M7 secret-files ("M7 will add `secrets/<name>/files/` ..."). No "M2" lurking. NO action.

### Stale references in `_docs/install.md`?

Quick grep confirms `_docs/install.md` has no `--mount` mentions. NO action.

---

## 8. Risks, gotchas, landmines

### 8.1. Docker volume name validation regex

The regex `^[a-zA-Z0-9][a-zA-Z0-9_.-]+$` is sourced from Docker's `pkg/volume/volume.go`. It requires AT LEAST two characters (first alphanumeric, then one or more from the broader set). A single-character volume name like `x` will fail my regex. **This is consistent with Docker** (verified: `docker volume create x` fails with `volume name is too short, names should be at least two alphanumeric characters`). The regex stays.

If we wanted single-char support, change to `^[a-zA-Z0-9][a-zA-Z0-9_.-]*$` (replace `+` with `*`). I am NOT making this change; Docker doesn't support it, and matching Docker is the correct policy.

### 8.2. Path normalisation

We do NOT normalise the host path. `--mount /foo/../bar:/data` passes through to `docker run -v /foo/../bar:/data` which Docker handles. If the operator types `--mount foo/bar:/data` (relative source, no leading `/`), our regex check rejects it as an invalid named-volume name (the slash makes it not match the volume regex). Operator gets a clear error. Good.

**Edge case:** `--mount C:/Users/foo:/data` on a Windows host. We're not running on Windows (decloud is Linux-host-only per `_docs/install.md`). M1 doesn't support Windows. Out of scope.

### 8.3. What happens if a bind source doesn't exist on disk?

Decision 1 above: we don't stat. `docker run -v /missing:/data <image>` returns:

```
docker: Error response from daemon: error while creating mount source path '/missing': mkdir /missing: read-only file system.
```

(Or similar, depending on Docker version and host filesystem.) This stderr flows through `cliDriver.Run` → `fmt.Errorf("docker run: %w; stderr=%q", err, stderr.String())` → `ErrRun` → exit 40. The operator sees "docker run: ... mkdir /missing: read-only file system." That's enough context to debug. **No change.**

### 8.4. `:ro` vs `:rw` default

Default is `rw` (no third component, ReadOnly=false). This matches Docker's default. M1 reading the TOML round-trips a `read_only = false` mount cleanly through `formatVolume` (omits the `:ro` suffix per `cli_driver.go:285-291`).

If an operator types `--mount /h:/c:rw` we reject explicitly (Decision 7 reason). The reason is they're confused about defaults — better to fail than to silently accept and have them think `:rw` was load-bearing.

### 8.5. Recreate strategy + mount reuse on container recreate

The recreate flow stops + removes the old container, then runs the new one. **For named volumes:** the volume persists across `docker rm`. The new `docker run -v vol:/path` reuses the existing volume. Data preserved across redeploys. ✓

**For bind mounts:** the host directory is unaffected by container lifecycle. `docker rm` removes the container, the bind-mount source on the host is untouched. New container's `docker run -v /host:/data` re-binds. Data preserved across redeploys. ✓

**For `restoreOldContainer`:** the rolled-back container gets `prev.Config.Run.Mounts` re-applied. Since the rollback runs the SAME image as before with the SAME mounts, the rollback container is functionally identical to the pre-deploy state. ✓

### 8.6. Concurrent access (theoretical)

Two operators deploying the same service with different `--mount` sets simultaneously: M1 is single-operator (per `caddy-runs-in-container.md:53`). Out of scope.

### 8.7. The `Mount.IsNamed()` definition: empty source

If `Mount.HostPath == ""`, `IsNamed()` returns false. But `ValidateMount` rejects empty source first — so an empty-source `Mount` shouldn't survive validation to reach `IsNamed()`. **Defence-in-depth:** `IsNamed()` returns false on empty so a hypothetical bypass of validation doesn't claim it's a named volume. Safe.

### 8.8. TOML round-trip of `ReadOnly`

`pelletier/go-toml/v2` serialises `bool` as `true`/`false` literals. Round-trip locked by existing `TestStore_RoundTripConfigAndSecrets` (registry/store_test.go:115). Adding mounts to the round-trip fixture (the new `TestStore_LoadAcceptsValidMounts` test) extends the lock. ✓

### 8.9. Cobra StringSliceVar gotcha

Cobra's `StringSliceVar` parses comma-separated values inside one occurrence. So `--mount /a:/b,/c:/d` would be split into two strings on the comma! **This is wrong** — bind paths can contain commas (`/path/with,comma:/data`).

Verify: read `pflag.StringSliceVar` docs. `StringSliceVar` does in fact split on commas. **The fix:** use `StringArrayVar` instead. `StringArrayVar` does NOT split — each `--mount X` adds one element to the slice.

**This is a load-bearing change.** Update §3.5(a):

```go
// CHANGED from StringSliceVar to StringArrayVar:
cmd.Flags().StringArrayVar(&f.Mounts, "mount", nil,
    "persistent volume; <host-path>:<container-path>[:ro] (bind) or <name>:<container-path>[:ro] (named volume); repeatable")
```

The `f.Mounts` field type stays `[]string`. The behaviour difference is invisible for normal mount strings (no commas), critical for paths with commas.

**Test addition:** add a subtest `path_with_comma` to `TestDeployService_MountFlagAccepted_PassesMountsThrough`:
- `--mount /path/with,comma:/data`
- Assert `req.Mounts[0].HostPath == "/path/with,comma"`.

This test FAILS with `StringSliceVar` and PASSES with `StringArrayVar`. It locks the choice.

**Pre-existing similar gap:** `--host` uses `StringSliceVar` (line 58). Do hostnames contain commas? No (DNS forbids it). `--host` is correctly using `StringSliceVar`. Don't touch it.

### 8.10. `parseMountFlags` empty input

If `f.Mounts` is `[]string{}` or nil (no `--mount` flags), `parseMountFlags` returns `(nil, nil)`. The orchestrator stores `nil` in `req.Mounts`, which `toVolumeMounts` returns `nil` for, which the driver's range loop runs zero times for. ✓

### 8.11. Mount order preservation

Docker applies `-v` flags in declared order. If two mounts overlap (which we reject as "duplicate container_path"), the later wins. Our reject rule means this doesn't matter — but for non-overlapping mounts, declared order is preserved by:

1. Cobra's `StringArrayVar` → `[]string` in CLI order.
2. `parseMountFlags` ranges over the slice in order.
3. `req.Mounts` is the same `[]Mount` in order.
4. `toVolumeMounts` ranges in order.
5. `formatVolume` is called per-volume in order.
6. `docker run -v ... -v ...` in declared order.

Round-trip-locked through TOML: TOML arrays preserve order. ✓

### 8.12. Schema_version remains 1 — but test it

Don §14 item 2: schema unchanged. Add a test that locks this: in `mount_test.go`, `TestMount_SchemaVersionUnchangedByMountsFeature`:

- Marshal a `ServiceConfig` with populated mounts.
- Decode the bytes.
- Assert `cfg.SchemaVersion == 1`.

This is a change-detector test on a constant... actually no, it's a contract test on the `schema-versioning.md:11` rule. Allowed under the "semantic-token contract" carve-out (the version integer IS the contract). But it's also asserting nothing the existing `TestStore_RoundTripConfigAndSecrets` doesn't already implicitly assert via its `assert.Equal(t, original.Config.SchemaVersion, loaded.Config.SchemaVersion)`.

**Decision: don't add the test.** The lock is implicit in the existing round-trip test plus the M2 fixture using `SchemaVersion: 1` in `newServiceFixture`. Adding it would be a change-detector for a value that's already locked.

### 8.13. The IsNamed convention bites if anyone ever mounts `/` directly

`Mount{"/", "/data", false}.IsNamed()` returns false (correctly — the bind source IS `/`). But binding `/` is operationally insane. Docker allows it; we don't validate against it; if an operator does this, they get exactly what they asked for.

### 8.14. `volumeNameRE` minimum length and Docker quirks

The regex `^[a-zA-Z0-9][a-zA-Z0-9_.-]+$` requires 2+ characters. Docker's actual rule from source: the volume.IsValidName function in modern Docker matches `[a-zA-Z0-9][a-zA-Z0-9_.-]+`, same as ours. ✓

If Docker tightens this rule in a future version (e.g., adds Unicode support), our regex would diverge. Mitigation: our regex is the current Docker rule; if it diverges in 5 years, an operator gets a clean error and we update the regex. **Acceptable.**

---

## 9. Performance, complexity, and the boring stuff that breaks things

- **Allocation cost:** parsing a `--mount` is one `strings.Split` (allocates), one `regexp.MatchString` for named volumes (compiled once at init via `var volumeNameRE = regexp.MustCompile`). ~negligible. Operators have 1-5 mounts in practice.
- **Init-order dependency:** `volumeNameRE` is a package-level `var` that calls `regexp.MustCompile`. Will panic at import time if the regex is malformed. This is the desired behaviour — a broken regex is a bug, not a runtime failure.
- **No new goroutines, no new channels, no concurrency.** The mount validation is straight-line synchronous code.

---

## 10. Workflow contingencies

**None.** Don §11 is correct. Kent and Rob are obviously in scope. Raymond does the docs sweep + audit-by-read; Kevlin and Linus review in parallel.

The only PLAN re-entry trigger is the schema-versioning escalation (§schema-versioning.md:20) — if Kent or Rob discovers `Mount` shape needs a v2 bump (e.g., the named-volume-from-HostPath derivation can't survive TOML round-trip for some pathological input). I do not expect this. If it happens: stop, return to PLAN, do not silently introduce migration code.

A second possible trigger: Cobra StringSliceVar/StringArrayVar misbehaviour on the `--mount` flag (the §8.9 issue) turns out to be different from documented. If `StringArrayVar` ALSO splits on commas in some Cobra/pflag version, a different parsing approach is needed (e.g., reset the flag and parse manually from `os.Args`). I ran the Cobra docs trace; the contract is clear; this should not surface, but I name it because §8.9 is the load-bearing assumption.

---

## 11. Consolidated docs sweep (Raymond's deliverable, locked here)

For Raymond's reference, the canonical list of doc-side flips. Audit-by-read each file end-to-end (`fix-now-while-fresh.md` Refinement).

### Code-touching edits (Rob's commit, NOT Raymond's)

(See §3 above.)

### Doc-only edits (Raymond's commit)

| File:line | Old | New |
|---|---|---|
| `_docs/usage.md:71` | `Rejected with exit 10 in M1. Persistent volumes are M2.` | `Persistent volume; \`<host-path>:<container-path>[:ro]\` (bind) or \`<name>:<container-path>[:ro]\` (named volume); repeatable. Bind sources must be absolute paths starting with \`/\`; named-volume sources must match \`[a-zA-Z0-9][a-zA-Z0-9_.-]+\`. Modes other than \`:ro\` are rejected.` |
| `_docs/usage.md:99` | `..., \`--mount\` used, \`--strategy\` other than \`recreate\`...` | `..., \`--strategy\` other than \`recreate\`, malformed \`--mount\` in a hand-edited TOML...` |
| `_ai/decisions/m1-scope.md:16` | `**No \`--mount\`** — flag rejected; loader also rejects non-empty \`Mounts\` (closes hand-edit loophole). M2.` | `**No \`--mount\` in M1** — flag rejected; loader also rejected non-empty \`Mounts\` (closed hand-edit loophole). Shipped at M2; see \`_tasks/2026-04-28-m2-server-side-mounts/\`.` |
| `_ai/decisions/m1-scope.md:32` | `M1 service deploy MVP → M2 server-side mounts + env-file hardening (\`--mount\` flag, loader populates \`Mounts\`) → M3 host bootstrap...` | `M1 service deploy MVP → M2 server-side mounts (\`--mount\` flag, loader populates \`Mounts\`) → M3 host bootstrap...` |
| `_ai/decisions/secrets-split.md:24` | `..., \`ErrUnknownField\` (strict mode), \`ErrMountsNotSupported\` (M1), \`ErrInvalidStrategy\` (M1).` | `..., \`ErrUnknownField\` (strict mode), \`ErrInvalidMount\` (malformed \`mounts\` entry), \`ErrInvalidStrategy\` (M1).` |
| `_ai/decisions/schema-versioning.md:11` | `M1 writes \`schema_version = 1\`. M2 writes \`schema_version = 1\`. M2 populates \`Mounts\`...` | `M1 writes \`schema_version = 1\`. M2 writes \`schema_version = 1\` and populates \`Mounts\`...` |
| `_ai/decisions/schema-versioning.md:16` | `M1 declares the full schema shape — including fields M1 won't populate (\`Mounts\` always empty in M1). M1's loader rejects non-empty \`Mounts\` with the same \`ErrMountsNotSupported\` as the CLI's \`--mount\` flag (closes the hand-edit loophole). M2 starts populating \`Mounts\`...` | `M1 declared the full schema shape — including fields M1 wouldn't populate (\`Mounts\` always empty in M1). M1's loader rejected non-empty \`Mounts\` with \`ErrMountsNotSupported\` (deleted at M2). M2's loader runs \`registry.ValidateMounts\` to enforce the same data-integrity rules as the CLI parser; M2 populates \`Mounts\`...` |
| `_ai/MEMORY.md:7` | Same env-file hardening phantom. | Strip the phantom phrase. |
| `_ai/MEMORY.md:9` | `(mounts populate at M2, secret-files at M7)` | `(mounts populated since M2, secret-files at M7)` |
| `_ai/cli-flag-surface-coherence.md:42` | `Live example: \`TestDeployService_MountFlagHelpReferencesM2\` ... asserts on the substring \`"M2"\` ...` | `Historical live example: \`TestDeployService_MountFlagHelpReferencesM2\` asserted on the substring \`"M2"\` until M2 shipped, when the milestone token had no remaining contract surface and the test was deleted. The carve-out remains valid for any future milestone-token assertion.` |
| `_ai/m1x-backlog.md` item 6 | Bundled wording. | Strikethrough; replace body with "Shipped at M2; see \`_tasks/2026-04-28-m2-server-side-mounts/\` (squash-merge commit TBD)." Add new items 9 (reloader %q), 10 (curl-through-Caddy integration), 11 (Driver.Run consolidation). |

Items 9, 10, 11 of m1x-backlog need authored bodies. Templates per Decision 9 above.

---

## 12. Bug class: things to NOT do mid-implementation

- Do NOT add a `--mount` long-form parser. M2 short-form only.
- Do NOT add `:cached`, `:delegated`, `:z`, `:Z` mode flag support. The error explicitly names `"only \"ro\" is supported"` because saying so loudly is what an operator on macOS who reflexively types `:cached` needs to see.
- Do NOT pre-create named volumes. Docker auto-creates on first `-v <name>:/path`.
- Do NOT touch `internal/envcap/`. Don §1a phantom kill stands.
- Do NOT introduce Viper, `/etc/decloud/config.toml`, or any global config. M3.
- Do NOT introduce a mount-defaults TOML. M3+.
- Do NOT add SELinux relabeling support. Real-world demand: zero.
- Do NOT switch `--mount` to `StringSliceVar` "to be consistent with `--host`." See §8.9; `StringSliceVar` splits on commas which is wrong for paths.

---

## 13. What Linus will look for in his review

I'm pre-empting:

1. **Sentinel-chain-shape:** `errors.Is(err, ErrInvalidMount)` works on the loader-side wrapped error. Locked by `TestStore_LoadRejectsInvalidMounts`. CLI-side `errors.Is(err, errUsage)` works. Locked by `TestDeployService_MountFlagInvalidReturnsExitUsageError`.
2. **Five-surface coherence:** §3 enumerates all five flips; §6 commits them atomically. The `TestDeployService_MountFlagHelpReferencesM2` deletion is justified by Don §7's "no token, no carve-out" argument; surface 5 dies the legitimate way.
3. **Round-trip-shape:** `req.Mounts` survives Save → Load via TOML, locked by the new `TestStore_LoadAcceptsValidMounts`.
4. **Cleanup-context-discipline:** unchanged; no new cleanup paths in M2. The `restoreOldContainer` path that DOES exist correctly carries `prev.Config.Run.Mounts` per §3.9(d).
5. **Argv shape:** locked by `TestCLIDriver_RunPassesVolumeFlags` (new) parallel to existing `TestCLIDriver_RunWithOptionsBindReadOnly`.
6. **No schema bump:** locked by §8.12 reasoning + existing round-trip test.
7. **Comma-in-path edge case:** §8.9; locked by the new `path_with_comma` subtest.
8. **`ErrMountsNotSupported` deletion across all surfaces:** §5 enumerates 8 sites; ruling out missed sites by a final `git grep -F "ErrMountsNotSupported"` returning zero AFTER Rob's commit lands.

---

## 14. Files relevant to this task (absolute paths)

(Same list as Don §"Files relevant to this task" — adding the integration test files.)

Code:
- `/Users/fenster/dev/decloud/internal/registry/types.go`
- `/Users/fenster/dev/decloud/internal/registry/errors.go`
- `/Users/fenster/dev/decloud/internal/registry/mount.go` (NEW)
- `/Users/fenster/dev/decloud/internal/registry/store.go`
- `/Users/fenster/dev/decloud/internal/cli/deploy_service.go`
- `/Users/fenster/dev/decloud/internal/cli/exit_codes.go`
- `/Users/fenster/dev/decloud/internal/dockerdrv/driver.go`
- `/Users/fenster/dev/decloud/internal/dockerdrv/cli_driver.go`
- `/Users/fenster/dev/decloud/internal/deploy/service.go`
- `/Users/fenster/dev/decloud/internal/deploy/lifecycle.go`

Tests:
- `/Users/fenster/dev/decloud/internal/registry/mount_test.go` (NEW)
- `/Users/fenster/dev/decloud/internal/registry/store_test.go`
- `/Users/fenster/dev/decloud/internal/cli/deploy_service_test.go`
- `/Users/fenster/dev/decloud/internal/cli/exit_codes_test.go`
- `/Users/fenster/dev/decloud/internal/dockerdrv/cli_driver_test.go`
- `/Users/fenster/dev/decloud/internal/deploy/service_test.go`
- `/Users/fenster/dev/decloud/internal/deploy/lifecycle_test.go`
- `/Users/fenster/dev/decloud/internal/integration/doc.go` (NEW)
- `/Users/fenster/dev/decloud/internal/integration/mount_test.go` (NEW)

Docs (Raymond):
- `/Users/fenster/dev/decloud/_docs/usage.md`
- `/Users/fenster/dev/decloud/_ai/decisions/m1-scope.md`
- `/Users/fenster/dev/decloud/_ai/decisions/schema-versioning.md`
- `/Users/fenster/dev/decloud/_ai/decisions/secrets-split.md`
- `/Users/fenster/dev/decloud/_ai/MEMORY.md`
- `/Users/fenster/dev/decloud/_ai/m1x-backlog.md`
- `/Users/fenster/dev/decloud/_ai/cli-flag-surface-coherence.md`

Task:
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/001-user-request.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/002-don-plan.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/003-joel-tech-plan.md` (this file)

---

## 15. Estimation reality check

Don's plan didn't carry an explicit estimate. Mine, by step:

- Kent: ~2 hours. New `mount_test.go` with 2 table-driven tests + 1 IsNamed test. Edits to 5 existing test files. New integration test (small).
- Rob: ~2.5 hours. 10 production files; mostly mechanical. The `parseMountFlags` helper + `toVolumeMounts` helper + `ValidateMount`/`ParseMountString` writes are straightforward Go.
- Raymond: ~1.5 hours. The end-to-end audit-by-read of 7 doc files is the time sink; the actual edits are small.
- Kevlin + Linus reviews: ~1.5 hours each, parallel.
- Iteration buffer (probability of a one-iteration revisit, weighted): ~2 hours.

**Total: ~9.5-12 hours of agent work.** Joel's M1-tech-plan budget was 12 hours; this is comparable. Realistic. No alarm bells.

The π-multiplier rule (`Original × π = realistic`) suggests if I'd guessed 4 hours, reality would be 12. I'm starting at 12, so the multiplier is implicit.

---

## 16. Sign-off shape for Linus

When Linus reviews this plan, the checklist:

- Decisions 1-12 each locked with a specific answer + 2-3 sentence rationale.
- Code shape pinned at file:line granularity.
- Test surface enumerated with subject names.
- Deletion list explicit (8 items).
- Atomic-commit shape defended.
- Fix-while-fresh sweep audited.
- Risks enumerated (14 of them).
- Workflow contingencies named (one schema-bump, one Cobra-quirk).

If Linus blocks on any of the 12 decisions, we go to PLAN v2. I expect blocks on Decision 4 (overriding Don's lean to α) and Decision 1 (overriding Don's lean to stat-the-source). I argued both rooted in M1 patterns; Linus may argue back. The pattern from M1 is iterate-until-Linus-approves; that's fine.
