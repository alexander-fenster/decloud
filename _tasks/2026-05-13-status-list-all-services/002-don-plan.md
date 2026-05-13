# Don's plan — `decloud status` with no args lists every registered service

Task slug: `status-list-all-services`. User request at `_tasks/2026-05-13-status-list-all-services/01-user-request.md`.

## 1. Behaviour change (the contract)

Today:

- `decloud status <name>` — `cobra.ExactArgs(1)`, prints a single line about one service. Source: `internal/cli/status.go:11-30`. Documented in `_docs/usage.md` §4 "Lifecycle commands" and §4.1 "Status format" and in `README.md:104, 141`.

After this task:

- `decloud status` (no positional args) — list every registered service, one line per service, columns aligned, sorted by name. Exit 0 even when zero services are registered (prints nothing, or a single sentinel — see §3.4).
- `decloud status <name>` — unchanged. Same one-line output, same exit codes, same semantics.
- More than one positional arg remains a usage error (exit 2). Use `cobra.MaximumNArgs(1)`; the rest of the surface stays declarative.

No new flags. No `--all`, no `--format`, no JSON. Operator surface stays tight; if we ever need machine-readable output we add `--format=json` later as one new flag, not a flag matrix today.

## 2. What I actually traced (not assumed)

`internal/cli/status.go:11-30` — current command:

```
Use:   "status <name>"
Args:  cobra.ExactArgs(1)
RunE: → lifecycleFactory(paths) → lc.Status(ctx, args[0]) → fmt.Fprintf one line
```

The Fprintf is the only output sink. Format is fixed:

```
%s state=%s container=%s deploy=%s deployed_at=%s\n
```

`internal/deploy/lifecycle.go:91-118` — `(*serviceDeployer).Status(ctx, name)`:

1. `d.deps.Store.Load(ctx, name)`. If `errors.Is(err, registry.ErrSecretsMissing)` return `Status{Name: name, State: "config-only"}`. Other Load errors propagate.
2. `d.deps.Driver.Inspect(ctx, containerName)`. Any error wraps `ErrRun`.
3. Inspect's `State` is one of `"running"`, `"exited"`, `"absent"` (verified `internal/dockerdrv/cli_driver.go:125-146` — `isNotFound(stderr)` returns `InspectResult{State: "absent"}` with nil err, all other docker failures bubble up). `"exited"` is rewritten to `"stopped"`; `"running"` and `"absent"` are kept verbatim.
4. Returns `deploy.Status{Name, ContainerID, ContainerName, State, LastDeployID, LastDeployedAt}`.

`internal/registry/store.go:175-204` — `(*fsStore).List(ctx)`:

1. `os.ReadDir(s.paths.ServicesDir)`. `fs.ErrNotExist` returns `(nil, nil)` — zero-service freshly-installed host is already handled. Other ReadDir failures bubble up wrapped.
2. Filter `*.toml` entries (skip directories, skip `.tmp` files); collect bare names; `sort.Strings`.
3. For each name, `s.Load(ctx, name)`. **Per-service Load failures are silently dropped** (`continue` at line 199). I traced this — it is intentional: `regenerateAndReload` in `internal/deploy/service.go:395-418` is the only existing caller, and the Caddyfile generator must produce something even if one service file is corrupted. This silent-drop is wrong for `decloud status` — see §3.3.

`internal/deploy/service.go:98-106` — `Lifecycle` interface. This is what `lifecycleFactory` (in `internal/cli/deploy_service.go:29, 146-156`) hands back to every lifecycle command. We extend it; `MockLifecycle` regenerates from it via `go:generate` at `service.go:21`.

`internal/cli/mocks/mock_lifecycle.go` — generated. Will regenerate cleanly when we add a method to the interface.

`internal/cli/lifecycle_commands_test.go:58-75` — the existing `TestStatus_DelegatesToLifecycleAndPrintsResult` covers the one-arg path; I will keep it untouched and add new tests for the no-arg path.

## 3. Design decisions

### 3.1 Reuse, do not invent — the existing pattern

Pattern discovery (mandatory per CLAUDE.md preamble):

- "List every registered service" is **already implemented**: `registry.Store.List(ctx)` returns `[]*registry.Service` in sorted order. Used today by `regenerateAndReload` in `internal/deploy/service.go:396` for the Caddyfile.
- "Compute one service's status" is **already implemented**: `Lifecycle.Status(ctx, name)` produces a `deploy.Status` with exactly the fields the no-arg variant wants per row.

The new code is glue. No new "list services" mechanism, no new "compute many statuses" abstraction beyond the obvious for-loop, no new transport.

### 3.2 Where the new method lives

Add **one** method to `deploy.Lifecycle`:

```go
StatusAll(ctx context.Context) ([]Status, error)
```

placed in `internal/deploy/service.go` interface block alongside `Status`, implemented in `internal/deploy/lifecycle.go` next to `Status`. Implementation calls `d.deps.Store.List(ctx)` and then, for each `*registry.Service`, performs the same `Driver.Inspect` + state-rewrite the single-service path does, building a `[]Status`.

Why on `Lifecycle` and not a new helper in `cli/`:

- The CLI layer never imports `registry` directly for status logic today (it goes through `Lifecycle.Status`). Keeping the layering means tests using the existing `MockLifecycle` in `internal/cli/` continue to do mock-at-one-place.
- It keeps the test seam the same: `lifecycleFactory` in `internal/cli/deploy_service.go:29` already swaps for tests; we don't introduce a second factory.

Why not a `StatusFor(ctx, names []string)` overload:

- The set of "all registered services" is the registry's responsibility to enumerate; the CLI shouldn't first call `Store.List` to get names then pass them back in. That recreates the failure modes of the current `List`'s silent skip in the wrong layer.

### 3.3 Per-service failure handling — **not** silent drop

`registry.Store.List` silently drops services whose config or secrets are corrupted (§2 above, lines `service.go:198-200`). For `decloud status`, **silent drop is the wrong default**: an operator running `decloud status` to see what is on the host must not be lied to about a service that exists on disk.

Plan: `StatusAll` does **not** call `Store.List`. Instead it does the directory walk itself (or — preferred — we add a sibling method `Store.ListNames(ctx) ([]string, error)` that returns bare service names without loading them, and `StatusAll` iterates `Load` per name with the existing single-service error contract).

Decision: add `Store.ListNames(ctx) ([]string, error)`. Rationale:

- Cheap: it does exactly the first half of the existing `List` (readdir + sort), and there is real reuse value — any future code that wants registered-service names without paying the Load cost benefits.
- Keeps `Store.List`'s silent-drop contract intact for the Caddyfile path (which Linus and Don both signed off on at M1 — touching it is out of scope here).

Then `StatusAll`:

```
names, err := Store.ListNames(ctx)
if err != nil { return nil, err }     // host-level failure, abort
out := make([]Status, 0, len(names))
for _, name := range names {
    st, err := d.Status(ctx, name)    // reuse the EXACT same single-service path
    if err != nil {
        // Per-service failure: synthesise a row whose State explains the error.
        out = append(out, Status{Name: name, State: errorState(err)})
        continue
    }
    out = append(out, st)
}
return out, nil
```

`errorState(err)`:

- `registry.ErrSecretsMissing` → handled inside `Status` already, returns `State: "config-only"` (no synthesis needed; this branch is unreachable for that sentinel).
- `registry.ErrSchemaMismatch` → `"error: schema"`
- `registry.ErrUnknownField` → `"error: schema"`
- `registry.ErrPermissionMode` → `"error: permissions"`
- `registry.ErrInvalidMount`, `registry.ErrInvalidStrategy` → `"error: config"`
- `deploy.ErrRun` (the Inspect-failed wrap) → `"error: docker"` — but keep the error visible: surface it through a `Status.ErrorDetail string` field so the CLI can print it on stderr alongside the row (see §3.4).
- Any other error → `"error"` and the error text on stderr.

**One failed service must NOT abort the listing.** That is the brutal-honesty bar: an operator with five services and one corrupted TOML still gets four real status lines and a clearly marked broken row, not a single exit-10 with no information.

Host-level errors (the `os.ReadDir` failure inside `ListNames`) do abort with the wrapped error — those are not "one service is broken", they are "the registry is unreadable", and surfacing all-zeros would mislead the operator about what's running.

### 3.4 Output format

Multi-row output, aligned columns, with header. Use `text/tabwriter` (stdlib; padding-aligned; no external dep).

```
NAME          STATE        CONTAINER             DEPLOY                       DEPLOYED_AT
foo           running      decloud-foo           20260426-093214-7f3a9c       2026-04-26T09:32:14Z
bar           stopped      decloud-bar           20260426-102001-aa11bb       2026-04-26T10:20:01Z
baz           absent       decloud-baz           20260425-180000-cc22dd       2026-04-25T18:00:00Z
qux           config-only  -                     -                            -
broken-svc    error: schema -                    -                            -
```

Reasoning:

- Header row makes the columns self-documenting. The single-service path doesn't have a header because it's `key=value` already self-labelled; the multi-row path uses positional columns and needs labels.
- Order matches the single-service `key=value` order: `name`, `state`, `container`, `deploy`, `deployed_at`. Reading "down" the first column gives names, reading "across" gives the same data the operator already knows from the single-service form.
- `-` placeholder for missing values (container/deploy/deployed_at unknown for `config-only` and `error:*` rows) — keeps the column count uniform; tabwriter aligns.
- Sorted by name (free from `Store.ListNames`, which already sorts).
- Zero services: print the header alone, with **no body rows**, exit 0. Operator interpretation: "Decloud is up, registry is readable, nothing is registered." Adding a `(no services registered)` sentinel was tempting but it complicates scripting — header alone is unambiguous and grep/awk-friendly.

**Surface coherence (per `_ai/cli-flag-surface-coherence.md`):** the four surfaces here are (1) runtime command, (2) help/usage text, (3) `_docs/usage.md` §4 reference, (4) `_docs/usage.md` §4.1 format section. All four must reflect both the one-arg and no-arg forms.

### 3.5 The error column carries error text, but only on stderr

`Status.ErrorDetail` (new optional field) is **not** printed in the aligned table — the table stays a fixed five-column shape. Each per-service error is also written to **stderr** in the form `decloud status: <name>: <error>` so the operator sees what went wrong without breaking column-parseable stdout. This mirrors the discipline at `internal/deploy/service.go` of using stderr for warnings and audit-log for detail.

### 3.6 Sorting / determinism

`Store.ListNames` already calls `sort.Strings`. Locale-independent (Go's `sort.Strings` is byte-order). Good — operator-visible ordering is reproducible across hosts and reboots.

## 4. Files to change

### Code

1. `internal/registry/store.go`
   - Add `ListNames(ctx context.Context) ([]string, error)` to the `Store` interface.
   - Implement on `fsStore`: factor the readdir/filter/sort half of the existing `List` into `ListNames`; rewrite `List` to call `ListNames` and then `Load` each in a loop (no behaviour change to `List`; the silent-skip continue stays).
   - Regenerate `internal/registry/mocks/mock_store.go` via `go generate`.

2. `internal/deploy/service.go`
   - Add `StatusAll(ctx context.Context) ([]Status, error)` to the `Lifecycle` interface.
   - Add optional `ErrorDetail string` to `Status` struct. Zero value preserved for single-service callers — existing tests at `internal/deploy/lifecycle_test.go` (StatusRunning/Stopped/Absent/ConfigOnly/ServiceNotFound) keep passing because the new field defaults to empty.

3. `internal/deploy/lifecycle.go`
   - Implement `StatusAll` next to `Status`. Reuse `Status(ctx, name)` per-service; trap per-service errors and synthesise `Status{Name, State: "error: …", ErrorDetail: err.Error()}`.

4. `internal/cli/mocks/mock_lifecycle.go` and `internal/cli/mocks/mock_deployer.go` — regenerate via `go generate ./...`. The `StatusAll` mock entry must appear.

5. `internal/cli/status.go`
   - Switch `Args: cobra.ExactArgs(1)` → `Args: cobra.MaximumNArgs(1)`.
   - `RunE`: if `len(args) == 1` → existing single-service path. Else → new multi-service path:
     - `statuses, err := lc.StatusAll(ctx)`.
     - On host-level err: return wrapped err (exit code follows existing `ExitCodeFor` mapping).
     - On success: write the header to a `tabwriter.NewWriter(cmd.OutOrStdout(), ...)`, one row per status, `Flush`.
     - For any row with non-empty `ErrorDetail`, also write `status: <name>: <detail>` to `cmd.ErrOrStderr()`.
   - Update help text: `Use: "status [name]"`, `Short: "Show status of one or all registered services"`.

### Tests (Kent writes; I identify scope here)

`internal/deploy/lifecycle_test.go` — add `StatusAll` cases:

- `TestLifecycle_StatusAll_EmptyRegistryReturnsEmptySlice` — `ListNames` returns `nil, nil`; result is `[]`, no Driver.Inspect calls.
- `TestLifecycle_StatusAll_HappyPathMixedStates` — `ListNames` returns `["bar","foo"]`; `bar` is `running`, `foo` is `exited`→`stopped`. Result order matches sort order. Both Inspect calls made.
- `TestLifecycle_StatusAll_AbsentContainerKept` — service registered, Inspect returns `absent`. Row has `State == "absent"` and `ContainerID == ""`.
- `TestLifecycle_StatusAll_ConfigOnlyOrphan` — `Load` returns `ErrSecretsMissing`; row is `State: "config-only"`, no Inspect call for that service.
- `TestLifecycle_StatusAll_PerServiceLoadErrorIsSynthesised` — `Load` for one of three services returns `ErrSchemaMismatch`; other two rows present and correct; the third row has `State: "error: schema"` and non-empty `ErrorDetail`. **One failure does NOT abort the listing.**
- `TestLifecycle_StatusAll_PerServiceInspectErrorIsSynthesised` — `Driver.Inspect` returns an arbitrary error for one service; row has `State: "error: docker"`, `ErrorDetail` non-empty; other services unaffected.
- `TestLifecycle_StatusAll_ListNamesErrorAbortsAndPropagates` — `ListNames` returns `(nil, errors.New(...))`; `StatusAll` returns that error wrapped; no Inspect calls.

`internal/registry/store_test.go` — add `ListNames` cases:

- `TestFSStore_ListNames_EmptyDirReturnsNil`.
- `TestFSStore_ListNames_MissingDirReturnsNilNoError` (consistent with `List`).
- `TestFSStore_ListNames_FiltersTOMLAndTmp` — skip `*.tmp`, skip directories, return bare names.
- `TestFSStore_ListNames_ResultIsSorted`.
- `TestFSStore_List_StillSilentlySkipsLoadErrors` — guard against accidental change to existing `List` contract.

`internal/cli/lifecycle_commands_test.go` — add no-arg status cases:

- `TestStatus_NoArgs_DelegatesToStatusAllAndPrintsTable` — mock returns 2 rows; assert stdout contains header `NAME` and both service names; assert columns are tab-aligned (substring match on rendered header + service rows).
- `TestStatus_NoArgs_EmptyListPrintsHeaderOnly` — mock returns `[]Status{}`; stdout has header, no data rows.
- `TestStatus_NoArgs_StatusAllErrorPropagates` — mock returns host-level error; CLI returns that error; stdout body empty.
- `TestStatus_NoArgs_ErrorRowGoesToStderr` — mock returns one good row + one row with `ErrorDetail`; stdout has both rows; stderr contains `status: <name>:` and the detail substring.
- `TestStatus_OneArg_StillPrintsSingleLine` — existing test, untouched.
- `TestStatus_TwoArgs_FailsWithUsageError` — `cobra.MaximumNArgs(1)` enforcement; expect exit-code-2-class error from Cobra (existing `isCobraUsageError` substring match handles this).

These are not change-detector tests — they assert on the typed contract (which method gets called with what args, which sentinel surfaces), not on prose phrasing. Per `_ai/cli-flag-surface-coherence.md` the help-text strings themselves are NOT under test.

### Docs (Raymond will write; flagged here)

1. `_docs/usage.md` §4 — add a bullet for `decloud status` (no args) listing all services. Keep the existing `decloud status <name>` bullet unchanged.
2. `_docs/usage.md` §4.1 "Status format" — split into "Single-service format" (unchanged) and "Multi-service format" (new). Document the columns, the header, the `-` placeholder, the `error: …` state values, the zero-services behaviour, and the stderr error-detail channel.
3. `_docs/usage.md` §5 — leave the single-service end-to-end example alone; optionally add a one-line `$ decloud status` example showing two rows.
4. `README.md:104` — extend the bullet to read along the lines of `decloud status [name]` — runtime + registry state. With a name, one line for that service; without, an aligned table of every registered service.

Raymond MUST cross-check the `State` values list (`running`, `stopped`, `absent`, `config-only`, `error: schema`, `error: permissions`, `error: config`, `error: docker`, `error`) against the implementation. Kevlin's hallucination check at review time will catch a drift here.

## 5. Edge cases the implementation MUST handle

1. **Zero services registered** — header only, no body, exit 0. Verified path: `ListNames` → `os.ReadDir` returns `fs.ErrNotExist` on fresh-install host → `(nil, nil)` → `StatusAll` returns `(nil, nil)` → CLI prints header, flushes, exit 0.

2. **A `decloud-<name>` container exists with no registry entry** — explicitly NOT shown. We list the registry, not Docker. The reverse case (registry entry but container absent) IS shown as `state=absent`. This is consistent with the M1 contract that the registry is source of truth for "what is a service"; orphan containers are a deploy-path concern, not a status-listing concern.

3. **Service in `config-only` state** — `Load` returns `ErrSecretsMissing`; `Status` already produces `State: "config-only"`; the multi-row path includes the row with `-` in the deploy/container columns.

4. **Corrupted TOML on one of N services** — that row shows `State: "error: schema"` (or similar) with `-` in the data columns; the other N-1 rows are computed normally; stderr carries the error detail; exit 0. (No partial failure of a multi-row command leaves the operator confused.)

5. **Inspect fails for one of N services** (Docker daemon hiccup, network unreachable) — row shows `State: "error: docker"` with `-`; other rows fine; stderr carries detail; exit 0. Justification: in a multi-service query, a transient docker daemon issue against one container should not nuke the entire listing.

6. **`ListNames` fails outright** (`/opt/decloud/config/services` exists but is unreadable) — host-level failure; abort with wrapped error; non-zero exit. Operator must fix the registry directory before status can work at all.

7. **One positional arg** — unchanged path. All current single-service behaviour (config-only, absent, secrets-missing → exit 10, etc.) preserved bit-for-bit.

8. **Two or more positional args** — Cobra rejects via `MaximumNArgs(1)`; the existing `isCobraUsageError` substring match in `internal/cli/exit_codes.go:75-84` already maps the Cobra error to `ExitUsageError` (2).

9. **Long service names / long deploy IDs / long container names** — `tabwriter` pads to the widest cell in each column. Service names are bounded by the regex `[a-z][a-z0-9-]{0,38}` (max 39 chars); container names are `decloud-<name>` (max 47); deploy IDs are fixed-width (22 chars: `YYYYMMDD-HHMMSS-XXXXXX`). No truncation needed.

10. **Concurrent deploy in progress while listing** — `ListNames` snapshots names at directory-read time; `Load` per name happens later. If a service is unregistered between the readdir and the Load, the per-service Load fails with `ErrNotFound` (file gone) — synthesise a row with `error` state, OR drop the row. Decision: **drop the row**. Rationale: by the time the operator reads the output, that service is genuinely gone; showing a transient-error row for it is more confusing than absence. Implementation: `if errors.Is(err, registry.ErrNotFound) { continue }` inside `StatusAll`'s loop.

## 6. Quality bar — what makes this RIGHT, not just working

- **The single-service path stays bit-for-bit identical.** No reformatting, no new fields in stdout, no change to error chains. Existing tests assert this. (`internal/cli/lifecycle_commands_test.go:58-75` and `internal/deploy/lifecycle_test.go:379-439` all stay green untouched.)
- **The multi-service path reuses `Status(ctx, name)` directly.** No second implementation of the docker-inspect-then-rewrite logic. One source of truth for "what's the state of one service"; one source of truth for "give me all the names".
- **One service's failure does not poison the listing.** This is the operational bar. The whole point of `decloud status` without args is "show me what's up", and lying-by-omission or aborting-on-one-bad-row both violate that.
- **Output is grep- and awk-friendly.** Header on stdout, errors on stderr, no decoration in stdout that would have to be filtered by scripts. tabwriter padding is whitespace, not boxes/ASCII-art.
- **Zero new dependencies.** `text/tabwriter` is stdlib.
- **No flag matrix.** No `--all`, no `--format=json`, no `--quiet`. If we want JSON later, that is one flag added to one command, not a design we lock in now.

## 7. Brutally honest: what makes this *not* harder than it should be

The existing code is already structured correctly:

- `Lifecycle` is an interface, test-seamed via `lifecycleFactory`. Adding one method is mechanical.
- `Store.List` already does most of the work; factoring `ListNames` out of it is ~6 lines.
- `Status(ctx, name)` cleanly composes inside a loop — no hidden side effects, no Caddy-reload-as-side-effect, no logging that would multiply log lines per service.

The one wart: `Store.List`'s silent-drop-on-Load-error is the wrong contract for the status surface, which is why we deliberately do not use it. That's documented above and respected by going through `ListNames` + per-service `Load` (re-used inside `Status`). No new tech debt; the M1 silent-drop contract for `List` stays as-is for its one Caddyfile caller.

## 8. Research info passed to Joel and downstream agents

### Files and key locations

- `internal/cli/status.go` — current command, 30 LOC. Whole file changes.
- `internal/cli/root.go:37` — `root.AddCommand(newStatusCmd(rc))`. No change.
- `internal/cli/deploy_service.go:29` (`var lifecycleFactory = buildProductionLifecycle`) — test seam, no change.
- `internal/cli/exit_codes.go:33-69` (`ExitCodeFor`) — no change; existing mappings cover all error paths the new code emits.
- `internal/deploy/service.go:65-72` (`Status` struct), `:98-106` (`Lifecycle` interface), `:144-147` (`NewLifecycle`). Add `ErrorDetail string` to `Status`; add `StatusAll` to `Lifecycle`. `NewLifecycle` body unchanged.
- `internal/deploy/lifecycle.go:91-118` (`Status` impl). Add `StatusAll` below it.
- `internal/registry/store.go:22-28` (`Store` interface), `:175-204` (`List` impl). Add `ListNames` to interface; factor `ListNames` out of `List`.
- `internal/registry/mocks/mock_store.go` — regenerated.
- `internal/cli/mocks/mock_lifecycle.go` — regenerated.

### Mockgen invocations (already declared in source)

- `internal/registry/store.go:19` — `//go:generate mockgen -source=store.go -destination=mocks/mock_store.go -package=mocks`.
- `internal/deploy/service.go:20-21` — mockgen for `ServiceDeployer` and `Lifecycle`.

Run `go generate ./...` from repo root after interface changes; Rob and Kent both pick up regenerated mocks.

### Types and signatures (concrete)

```go
// internal/registry/store.go
type Store interface {
    // existing methods unchanged
    Load(ctx context.Context, name string) (*Service, error)
    Save(ctx context.Context, svc *Service) error
    DeleteOrphanConfig(ctx context.Context, name string) error
    List(ctx context.Context) ([]*Service, error)
    Delete(ctx context.Context, name string) error

    // new
    ListNames(ctx context.Context) ([]string, error)
}
```

```go
// internal/deploy/service.go
type Status struct {
    Name           string
    ContainerID    string
    ContainerName  string
    State          string
    LastDeployID   string
    LastDeployedAt time.Time
    ErrorDetail    string // empty unless State is error:*; not part of stdout output
}

type Lifecycle interface {
    Unregister(ctx context.Context, name string) error
    Start(ctx context.Context, name string) error
    Stop(ctx context.Context, name string) error
    Restart(ctx context.Context, name string) error
    Status(ctx context.Context, name string) (Status, error)
    StatusAll(ctx context.Context) ([]Status, error) // new
    Logs(ctx context.Context, name string, opts LogOptions) error
    CaddyReload(ctx context.Context) error
}
```

```go
// internal/cli/status.go (rough shape — Rob writes the real impl)
return &cobra.Command{
    Use:   "status [name]",
    Short: "Show status of one or all registered services",
    Args:  cobra.MaximumNArgs(1),
    RunE: func(cmd *cobra.Command, args []string) error {
        lc, err := lifecycleFactory(config.NewPaths(rc.ConfigRoot))
        if err != nil { return fmt.Errorf("building lifecycle: %w", err) }
        if len(args) == 1 {
            // existing path, unchanged Fprintf
        }
        // new multi-row path: lc.StatusAll, tabwriter, stderr error detail
    },
}
```

### Known tested patterns we reuse

- `text/tabwriter` is stdlib; choose `tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)` — minwidth 0, tabwidth 0, padding 2, padchar space, no flags. Matches what most operator-tool tables look like in Go stdlib examples.
- Single-line single-service `key=value` shape stays. Multi-row positional shape is new and lives next to it.
- State enum extension is additive: existing values (`running`, `stopped`, `absent`, `config-only`) keep their semantics; new `error:*` values appear only on the multi-row path because the single-service path either returns a `deploy.Status` with a known state OR returns an error (which the CLI surfaces via `ExitCodeFor`).

### Exit-code mapping

- Multi-row success (even with per-service `error:*` rows) → exit 0. Per-service errors are informational, not fatal.
- Host-level error from `StatusAll` (`ListNames` failed, etc.) → propagated; `ExitCodeFor` maps via existing rules: unwrapped registry sentinel → exit 10, anything else → exit 70 (`ExitInternal`).
- Two-args usage error → exit 2 via `isCobraUsageError` fallback (existing).

### Not touched

- `internal/deploy/service.go` orchestration of `Deploy` — out of scope.
- `internal/caddy/` — out of scope.
- `Store.List`'s silent-drop-on-Load-error — deliberately preserved for the Caddyfile generation path; do not change.
- The single-service status Fprintf format — operator scripts may grep on it; do not modify.

## 9. What I want Joel to do next

Expand this into a tech plan with:

- The exact tabwriter column widths and padchar (or argument for changing my pick).
- The exact `errorState(err)` mapping table (above is a starting point — confirm or trim).
- A decision on whether `ListNames` returns `(nil, nil)` for missing-dir (matching `List`) or `(nil, err)` (matching strict). My read: `(nil, nil)`, to keep zero-services-installed hosts working.
- The mockgen regeneration order (registry mocks before deploy mocks; deploy interface change ripples to the cli mocks).
- Whether the `ErrorDetail` field belongs on `Status` (my preference) or on a parallel `[]error` returned alongside `[]Status` from `StatusAll`. The struct-field approach is simpler; the parallel-slice approach is more idiomatic Go. I lean struct-field because the row + its diagnostic are naturally one unit.

Linus will review for layering violations (he will not like `cli/` reaching into `registry/` for layout knowledge — confirm we don't; we route through `Lifecycle`). Kent writes the test list in §4 above; Rob implements; Raymond does the doc surfaces in §4 "Docs" above; Kevlin hallucination-checks the state-value list against the implementation.

Don't ship shit.
