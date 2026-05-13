# Joel's tech plan — `decloud status` lists every registered service

Task slug: `status-list-all-services`. Branch: `task/status-list-all-services`. Don's plan: `_tasks/2026-05-13-status-list-all-services/002-don-plan.md`. User request: `_tasks/2026-05-13-status-list-all-services/01-user-request.md`.

This document expands Don's plan into the exact function signatures, the exact diff sketch for `internal/cli/status.go`, the exact tabwriter format, and the exact regen sequence Rob and Kent need.

## 0. What I confirmed, refined, and pushed back on

Confirmed (read-the-code verification, not assumption):

- `internal/cli/status.go:11-30` — `cobra.ExactArgs(1)` is the only thing blocking the no-arg form; the rest of the surface is fine.
- `internal/deploy/service.go:65-72` — `Status` struct has six fields; adding a seventh (`ErrorDetail`) is additive and zero-impact on existing callers (zero value preserved).
- `internal/deploy/service.go:98-106` — `Lifecycle` interface; adding `StatusAll` is one new line.
- `internal/deploy/lifecycle.go:91-118` — `Status(ctx, name)` is composable: pure dependency on `Store.Load` + `Driver.Inspect`, no side effects, no logging-per-call. Calling it inside a loop is safe.
- `internal/registry/store.go:175-204` — `List` already does readdir+filter+sort+Load. Factoring the first half out is mechanical (~6 LOC moved).
- `internal/registry/store.go:198-200` — the silent-skip on Load failure is real and is exactly one `continue`. Don's reading is correct and the Caddyfile caller depends on it (`internal/deploy/service.go:396`).
- `internal/cli/exit_codes.go:75-84` — `isCobraUsageError`'s substring match includes `"accepts"`, which is the leading word in Cobra's `MaximumNArgs` error (`"accepts at most 1 arg(s), received 2"`). So two-arg invocation routes to `ExitUsageError` (2) without any code change. Verified.

Refined:

- **`ErrorDetail` placement.** Struct field on `Status` (Don's preference). Parallel `[]error` is more idiomatic Go, but `Status.ErrorDetail` keeps the row+diagnostic as one unit, simplifies the CLI rendering loop, and the existing CaddyReload/single-service-Status tests don't observe the field. Decision: struct field, type `string` (not `error`) so it serialises cleanly and so we don't accidentally re-route on the wrapped sentinel later.
- **Tabwriter config.** `tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)` — minwidth 0, tabwidth 0, padding 2, padchar `' '`, flags 0. Two-space padding matches `docker ps`-style operator tables; no tabwriter flags (no right-align, no debug separators).
- **`StatusAll` aborts only on `ListNames` failure.** Per-service Load/Inspect failures are absorbed into the row (Don §3.3) — confirmed. Host-level (the registry directory unreadable) is the only abort path.
- **Concurrent-deploy race policy.** A service that vanishes between `ListNames` and `Load` (`ErrNotFound` on Load) is **dropped**, not surfaced as an error row (Don §5.10). Confirmed and made precise: `if errors.Is(err, registry.ErrNotFound) { continue }` is checked before the synthesis branch.

Pushed back on (real disagreements):

1. **The `errorState(err)` enum is over-engineered.** Don proposed five synthesized states: `error: schema`, `error: permissions`, `error: config`, `error: docker`, and bare `error`. I want **two** synthesized states only: `config-only` (already exists, handled inside `Status`) and **`error`**. Reasons:
   - The column-value enum is a contract surface (per `_ai/cli-flag-surface-coherence.md`); each value an operator might grep for is one more thing for docs to keep in sync and one more thing Kevlin has to hallucination-check. Five sub-categories is five contract values; one is one.
   - The detail an operator actually needs to triage (`schema_version=2, expected 1`, `secrets dir mode 0755, expected 0700`, etc.) is in the wrapped error text — which already goes to **stderr** in our design. Splitting into pre-canned buckets loses the specific detail anyway and forces the operator back to stderr.
   - Grep-friendly: `decloud status | grep '^foo.*error '` reliably finds the broken row. With five sub-states the operator must write `grep '^foo.*error'` (no trailing space), which matches `error: docker` AND `error: schema` AND any future addition. The single-state design is monotonically scriptable; the multi-state design has an open-ended enum.
   - YAGNI: nobody has asked for sub-categorisation. Add it if and when an operator does.
   - Net: this is a **simplification**. One state token, full detail on stderr.
2. **`Status.ErrorDetail` is NOT printed in the table.** Don already said this in §3.5; I am locking it in as a hard rule. The five-column stdout shape is the contract; rendering a sixth column conditionally would break alignment for scripts that read column counts.
3. **`Use:` string.** Don wrote `"status [name]"`. Cobra's convention for an optional positional is square brackets, which is what he has — good. Confirm we write exactly `Use: "status [name]"`, NOT `"status [<name>]"` and NOT `"status [name...]"` (no variadic).

Don did not miss anything else of substance. The bones of his plan are right; the contract surface needs trimming.

## 1. Exact function signatures

### 1.1 `internal/registry/store.go`

Interface change (file: `internal/registry/store.go:22-28`):

```go
// Store is the persistence layer for service registrations.
type Store interface {
    Load(ctx context.Context, name string) (*Service, error)
    Save(ctx context.Context, svc *Service) error
    DeleteOrphanConfig(ctx context.Context, name string) error
    List(ctx context.Context) ([]*Service, error)
    Delete(ctx context.Context, name string) error

    // ListNames returns the bare names of every registered service
    // (config TOML present in ServicesDir), sorted byte-order. Missing
    // ServicesDir is treated as "no services" — returns (nil, nil), not
    // an error, matching List's behaviour. Other directory-read errors
    // are wrapped and returned.
    //
    // Unlike List, ListNames does NOT Load each service, so it does not
    // silently skip services whose config or secrets are corrupted.
    // Callers that need the loaded *Service must call Load(name) per name
    // and handle per-service errors themselves.
    ListNames(ctx context.Context) ([]string, error)
}
```

Implementation refactor (file: `internal/registry/store.go:175-204`). Current `List` becomes:

```go
func (s *fsStore) ListNames(ctx context.Context) ([]string, error) {
    entries, err := os.ReadDir(s.paths.ServicesDir)
    if err != nil {
        if errors.Is(err, fs.ErrNotExist) {
            return nil, nil
        }
        return nil, fmt.Errorf("registry: reading services dir %s: %w", s.paths.ServicesDir, err)
    }
    names := make([]string, 0, len(entries))
    for _, e := range entries {
        if e.IsDir() {
            continue
        }
        n := e.Name()
        if !strings.HasSuffix(n, ".toml") || strings.HasSuffix(n, ".tmp") {
            continue
        }
        names = append(names, strings.TrimSuffix(n, ".toml"))
    }
    sort.Strings(names)
    return names, nil
}

func (s *fsStore) List(ctx context.Context) ([]*Service, error) {
    names, err := s.ListNames(ctx)
    if err != nil {
        return nil, err
    }
    out := make([]*Service, 0, len(names))
    for _, name := range names {
        svc, err := s.Load(ctx, name)
        if err != nil {
            continue // existing silent-skip contract; Caddyfile path depends on it
        }
        out = append(out, svc)
    }
    return out, nil
}
```

Notes for Rob:

- The comment `// existing silent-skip contract; Caddyfile path depends on it` is **mandatory**. Without it the next reviewer will rip it out as "obvious bug." See `_ai/error-wrap-discipline.md` — comments lie, but the test guarding this contract (Kent will add `TestFSStore_List_StillSilentlySkipsLoadErrors`) does not.
- `ctx` is unused inside the readdir; keep the parameter for API symmetry with the rest of `Store`. Future cancellable-walk implementations want it.
- The two functions share a single sort path. No call site outside this file should re-sort.

Call sites of `List` to **verify unchanged** after refactor:

- `internal/deploy/service.go:396` — `d.deps.Store.List(ctx)` for Caddyfile regen. Behaviour: unchanged. Silent skip preserved. **No code change here.**

No other call sites. `grep -rn 'Store.List\|store.List\|\.List(ctx)' internal/` should match only those two (the impl site and the regenerateAndReload caller). Rob runs this grep as a sanity check.

### 1.2 `internal/deploy/service.go`

Struct change (file: `internal/deploy/service.go:65-72`):

```go
type Status struct {
    Name           string
    ContainerID    string
    ContainerName  string
    State          string
    LastDeployID   string
    LastDeployedAt time.Time
    // ErrorDetail carries the wrapped error message when StatusAll could
    // not produce a real row for this service (State == "error"). Empty
    // for the single-service Status() path and for non-error multi-row
    // entries. NOT rendered in stdout; the CLI prints it to stderr.
    ErrorDetail string
}
```

Interface change (file: `internal/deploy/service.go:98-106`):

```go
type Lifecycle interface {
    Unregister(ctx context.Context, name string) error
    Start(ctx context.Context, name string) error
    Stop(ctx context.Context, name string) error
    Restart(ctx context.Context, name string) error
    Status(ctx context.Context, name string) (Status, error)
    StatusAll(ctx context.Context) ([]Status, error)
    Logs(ctx context.Context, name string, opts LogOptions) error
    CaddyReload(ctx context.Context) error
}
```

### 1.3 `internal/deploy/lifecycle.go`

New method, placed immediately after `Status` (`internal/deploy/lifecycle.go:91-118`):

```go
// StatusAll returns one Status per registered service, sorted by name.
//
// Per-service failures (Load fails after the name was listed, Inspect
// fails) are absorbed into the result: the row is synthesized with
// State="error" and ErrorDetail set to the wrapped error text. The
// listing itself does not abort — an operator running `decloud status`
// to see what is on the host must get every service's row even when
// one is broken.
//
// A service that disappears between ListNames and Load (Load returns
// ErrNotFound) is dropped from the result rather than synthesized as
// an error: by the time the operator reads the output the row would
// be misleading.
//
// Host-level failures (ListNames fails) abort and return the wrapped
// error.
func (d *serviceDeployer) StatusAll(ctx context.Context) ([]Status, error) {
    names, err := d.deps.Store.ListNames(ctx)
    if err != nil {
        return nil, fmt.Errorf("listing services: %w", err)
    }
    out := make([]Status, 0, len(names))
    for _, name := range names {
        st, err := d.Status(ctx, name)
        if err != nil {
            if errors.Is(err, registry.ErrNotFound) {
                continue
            }
            out = append(out, Status{
                Name:        name,
                State:       "error",
                ErrorDetail: err.Error(),
            })
            continue
        }
        out = append(out, st)
    }
    return out, nil
}
```

Notes for Rob:

- The `fmt.Errorf("listing services: %w", err)` wrap is **plain context, no sentinel**. The inner `err` already carries whatever the readdir surfaced (no registry sentinel — `ListNames` does not wrap with any of the typed sentinels in `internal/registry/errors.go`). This means `ExitCodeFor` falls through to `ExitInternal` (70) on host-level failure, which is the right exit code for "the registry directory is unreadable, the operator must investigate the host."
- `errors.Is(err, registry.ErrNotFound)` matches the chain because `Status` wraps it via `fmt.Errorf("loading service: %w", err)` at `internal/deploy/lifecycle.go:97`. Verified.
- Capacity hint `make([]Status, 0, len(names))` matters: typical operator has ~5-50 services; one pre-allocation avoids slice growth chatter without committing to allocating storage we won't use.
- Do NOT log per-row failures inside `StatusAll`. The single-service `Status` path doesn't log either; multi-row's log volume would otherwise grow linearly. The CLI surfaces detail to stderr (§2 below), which is the right place for operator-visible failure text.

### 1.4 `internal/cli/status.go`

Whole-file rewrite. Current 30 LOC become:

```go
package cli

import (
    "fmt"
    "io"
    "text/tabwriter"
    "time"

    "github.com/alexander-fenster/decloud/internal/config"
    "github.com/alexander-fenster/decloud/internal/deploy"
    "github.com/spf13/cobra"
)

func newStatusCmd(rc *rootContext) *cobra.Command {
    return &cobra.Command{
        Use:   "status [name]",
        Short: "Show status of one or all registered services",
        Args:  cobra.MaximumNArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            lc, err := lifecycleFactory(config.NewPaths(rc.ConfigRoot))
            if err != nil {
                return fmt.Errorf("building lifecycle: %w", err)
            }
            if len(args) == 1 {
                return runStatusOne(cmd.Context(), lc, cmd.OutOrStdout(), args[0])
            }
            return runStatusAll(cmd.Context(), lc, cmd.OutOrStdout(), cmd.ErrOrStderr())
        },
    }
}

func runStatusOne(ctx context.Context, lc deploy.Lifecycle, out io.Writer, name string) error {
    st, err := lc.Status(ctx, name)
    if err != nil {
        return err
    }
    fmt.Fprintf(out, "%s state=%s container=%s deploy=%s deployed_at=%s\n",
        st.Name, st.State, st.ContainerName, st.LastDeployID, st.LastDeployedAt.Format(time.RFC3339))
    return nil
}

func runStatusAll(ctx context.Context, lc deploy.Lifecycle, out, errw io.Writer) error {
    statuses, err := lc.StatusAll(ctx)
    if err != nil {
        return err
    }
    tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
    fmt.Fprintln(tw, "NAME\tSTATE\tCONTAINER\tDEPLOY\tDEPLOYED_AT")
    for _, st := range statuses {
        fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
            st.Name,
            st.State,
            dashIfEmpty(st.ContainerName),
            dashIfEmpty(st.LastDeployID),
            rfc3339OrDash(st.LastDeployedAt),
        )
    }
    if err := tw.Flush(); err != nil {
        return fmt.Errorf("flushing status table: %w", err)
    }
    for _, st := range statuses {
        if st.ErrorDetail != "" {
            fmt.Fprintf(errw, "status: %s: %s\n", st.Name, st.ErrorDetail)
        }
    }
    return nil
}

func dashIfEmpty(s string) string {
    if s == "" {
        return "-"
    }
    return s
}

func rfc3339OrDash(t time.Time) string {
    if t.IsZero() {
        return "-"
    }
    return t.Format(time.RFC3339)
}
```

Notes for Rob:

- The `context` import is needed in the new signatures — already imported by `package cli` elsewhere but not in this file currently. Goimports/gofmt handles this on save.
- `runStatusOne` is a verbatim extraction of the existing single-service `Fprintf` — bit-for-bit identical output. Run `git diff -U10 internal/cli/status.go` after the change and verify the single-service Fprintf format string is unchanged: `"%s state=%s container=%s deploy=%s deployed_at=%s\n"` and arg order `Name, State, ContainerName, LastDeployID, LastDeployedAt.Format(time.RFC3339)`. The existing test at `internal/cli/lifecycle_commands_test.go:58-75` asserts on the substrings; it must stay green untouched.
- The two-pass over `statuses` (once for tabwriter, once for stderr) is deliberate. Single-pass interleaved writes to `out` and `errw` would race in test capture against the tabwriter's internal buffering, and the test harness uses two separate `bytes.Buffer`s (`runRoot` at `internal/cli/deploy_service_test.go:34-44`) which are concurrent-safe individually but not relative to each other. Two passes keep ordering deterministic: stdout flushes fully, then stderr.
- `dashIfEmpty` and `rfc3339OrDash` are local helpers; keep them in `status.go` (file-private). They are NOT exported and NOT moved to a util package — they have one caller and one purpose.
- `Use: "status [name]"` is the operator-facing usage line. Square brackets around `name` is the Cobra convention for optional positional, mirroring stdlib `getopt`-style help.
- `Short` text changed to `"Show status of one or all registered services"`. The previous wording was `"Show runtime + registry status of a service"` — both the meaning AND the milestone-token assertion pattern need to remain correct (no token assertion exists on this Short string today, per a grep of `internal/cli/`). No semantic-token contract to worry about.

## 2. Output format — exact bytes

### 2.1 Multi-service path

Header line (always printed, even for zero services):

```
NAME    STATE    CONTAINER    DEPLOY    DEPLOYED_AT
```

The literal tab separators in the format string are `\t`; tabwriter renders them as space-padded alignment. With `padding=2`, each column ends with at least two spaces before the next column starts (or before EOL for the last column). The `padchar=' '` ensures script-friendly output (no embedded tabs in the rendered bytes).

Example with mixed states (illustrative; exact spacing is whatever tabwriter computes from the widest row):

```
NAME        STATE        CONTAINER             DEPLOY                  DEPLOYED_AT
bar         stopped      decloud-bar           20260426-102001-aa11bb  2026-04-26T10:20:01Z
broken-svc  error        -                     -                       -
foo         running      decloud-foo           20260426-093214-7f3a9c  2026-04-26T09:32:14Z
qux         config-only  -                     -                       -
```

Stderr companion for the `broken-svc` row:

```
status: broken-svc: loading service: registry: schema_version mismatch: ...
```

### 2.2 Column ordering rationale

The five columns reflect the single-service `key=value` order (`Name state= container= deploy= deployed_at=`). An operator can read down for "what is running" or across for "what is one service doing"; the column-to-key mapping is 1:1 so doc cross-references stay simple.

### 2.3 Zero services

```
NAME    STATE    CONTAINER    DEPLOY    DEPLOYED_AT
```

(Header only. No data rows. No sentinel sentence. Exit 0.) Rationale: scripts piping through `awk '$2 == "running"'` get an empty result, not a parse error on a `(no services registered)` sentinel line. Adding a sentinel would have to be parsed-around by every operator script forever.

### 2.4 Per-row error policy

`State == "error"` rows print `-` in CONTAINER, DEPLOY, DEPLOYED_AT (zero-value fields). The stderr line uses the format `status: <name>: <err.Error()>` and goes to `cmd.ErrOrStderr()`, NOT `os.Stderr` directly — the test harness captures both via `cmd.Set{Out,Err}`.

### 2.5 What does NOT change

The single-service path produces byte-identical output to today. The existing test `TestStatus_DelegatesToLifecycleAndPrintsResult` (`internal/cli/lifecycle_commands_test.go:58-75`) is the regression lock: it asserts on `"foo"`, `"running"`, `"decloud-foo"` substrings; all three remain present.

## 3. Diff sketch for `internal/cli/status.go`

Before (30 lines, current state on `main` and on this branch):

```go
return &cobra.Command{
    Use:   "status <name>",
    Short: "Show runtime + registry status of a service",
    Args:  cobra.ExactArgs(1),
    RunE: func(cmd *cobra.Command, args []string) error {
        lc, err := lifecycleFactory(config.NewPaths(rc.ConfigRoot))
        if err != nil {
            return fmt.Errorf("building lifecycle: %w", err)
        }
        st, err := lc.Status(cmd.Context(), args[0])
        if err != nil {
            return err
        }
        fmt.Fprintf(cmd.OutOrStdout(), "%s state=%s container=%s deploy=%s deployed_at=%s\n",
            st.Name, st.State, st.ContainerName, st.LastDeployID, st.LastDeployedAt.Format(time.RFC3339))
        return nil
    },
}
```

After: the structure in §1.4 above. Changes:

- `Use: "status <name>"` → `Use: "status [name]"`.
- `Short: "Show runtime + registry status of a service"` → `Short: "Show status of one or all registered services"`.
- `Args: cobra.ExactArgs(1)` → `Args: cobra.MaximumNArgs(1)`.
- `RunE` body becomes a dispatch on `len(args)`.
- Extract single-service body to `runStatusOne`; add new `runStatusAll`, `dashIfEmpty`, `rfc3339OrDash` helpers.
- New imports: `"context"`, `"io"`, `"text/tabwriter"`, `"github.com/alexander-fenster/decloud/internal/deploy"`. The `deploy` import is needed because `runStatusOne`/`runStatusAll` take `deploy.Lifecycle` as a parameter; it was not present in the old file because `lifecycleFactory`'s return type was inferred from the assignment. (`gofmt`/`goimports` will sort the import block — Rob runs `gofmt -w` at the end.)

## 4. Mock regeneration

Three mockgen directives exist:

1. `internal/registry/store.go:19` — `//go:generate mockgen -source=store.go -destination=mocks/mock_store.go -package=mocks` (regenerates `MockStore`; needed because we add `ListNames`).
2. `internal/deploy/service.go:20` — `ServiceDeployer` mock (no change needed — the `ServiceDeployer` interface is untouched). Will be regenerated as a side effect of `go generate ./...` but the diff should be empty.
3. `internal/deploy/service.go:21` — `Lifecycle` mock (regenerates `MockLifecycle`; needed because we add `StatusAll`).

**Rob's exact command** from repo root:

```bash
go generate ./...
gofmt -w internal/
```

Order does not matter — mockgen reads the current source, and we update the source files (`store.go`, `service.go`) before running `go generate`. Diff after regen:

- `internal/registry/mocks/mock_store.go` — adds `ListNames` mock method (~14 lines).
- `internal/cli/mocks/mock_lifecycle.go` — adds `StatusAll` mock method (~14 lines).
- `internal/cli/mocks/mock_deployer.go` — no diff expected.

If any other mock file shows a non-empty diff, **stop** — that means the regen picked up an unrelated drift and we need a separate task to address it (don't bundle mock drift fixes into this branch).

## 5. Patterns to reuse / not reinvent

- **`text/tabwriter`** — stdlib, no new dependency. Config `(0, 0, 2, ' ', 0)`. No other file in the codebase uses tabwriter today (grep `tabwriter` across `internal/` returns nothing); this is the first usage and it sets the convention. Future operator-table outputs (e.g., a hypothetical `decloud caddy status`) follow the same config.
- **Error wrap discipline** — `_ai/error-wrap-discipline.md`. All wraps in this task use `%w`. The only sites:
  - `internal/deploy/lifecycle.go` `StatusAll`: `fmt.Errorf("listing services: %w", err)` — plain context wrap, no sentinel.
  - `internal/cli/status.go` `runStatusAll`: `fmt.Errorf("flushing status table: %w", err)` — plain context wrap.
  - All other error paths just return the underlying error verbatim.
  - **`grep -rn '%w: %v' internal/cli/status.go internal/deploy/lifecycle.go internal/registry/store.go` must return zero.** Rob runs this before commit.
- **Test seam pattern** — `lifecycleFactory` (`internal/cli/deploy_service.go:29`). Kent's CLI tests assign a stub returning `*MockLifecycle`, exactly as `installMockLifecycle` at `internal/cli/lifecycle_commands_test.go:16-24`. New tests follow the same pattern.
- **Test fixtures** — `newLifecycleHarness` at `internal/deploy/lifecycle_test.go:33-66`. Kent's `StatusAll` unit tests in `internal/deploy/lifecycle_test.go` reuse this harness verbatim. No new harness needed.
- **Status struct fixture** — `newRegisteredService` at `internal/deploy/lifecycle_test.go:68-79`. Reuse.
- **Cobra usage error matching** — `isCobraUsageError` (`internal/cli/exit_codes.go:75-84`). The substring `"accepts"` already in the matcher handles `MaximumNArgs`'s `"accepts at most 1 arg(s)"` message. No exit-code-table change needed.

## 6. Test scenarios (for Kent — Don listed these; I add the precise assertions)

### 6.1 `internal/registry/store_test.go` (registry layer)

- `TestFSStore_ListNames_EmptyDirReturnsNil` — set up store with empty `ServicesDir`. Assert `names, err := store.ListNames(ctx)`; `err == nil`, `names == nil` or `len(names) == 0`.
- `TestFSStore_ListNames_MissingDirReturnsNilNoError` — delete `ServicesDir` (or use a fresh `t.TempDir()` without `MkdirAll`). Assert `err == nil` and `names` empty.
- `TestFSStore_ListNames_FiltersTOMLAndTmp` — write `a.toml`, `b.toml`, `c.tmp`, `d` (no extension), and a subdir `nested/`. Assert `names == []string{"a","b"}` exactly (sorted).
- `TestFSStore_ListNames_ResultIsSorted` — write `zebra.toml`, `apple.toml`, `mango.toml`. Assert `names == []string{"apple","mango","zebra"}`.
- `TestFSStore_List_StillSilentlySkipsLoadErrors` — write `good.toml` + matching secrets, write `broken.toml` with invalid TOML. Assert `services, err := store.List(ctx)`; `err == nil`, `len(services) == 1`, `services[0].Config.Name == "good"`. **This locks the contract Caddyfile generation depends on.**

### 6.2 `internal/deploy/lifecycle_test.go` (lifecycle layer)

All use `newLifecycleHarness(t)`.

- `TestLifecycle_StatusAll_EmptyRegistryReturnsEmptySlice` — `h.store.EXPECT().ListNames(gomock.Any()).Return(nil, nil)`. Assert `statuses, err := h.lc.StatusAll(ctx)`; `err == nil`, `len(statuses) == 0`. **Zero Inspect calls.** (gomock will fail the test if Inspect is called unexpectedly.)
- `TestLifecycle_StatusAll_HappyPathMixedStates` — `ListNames` returns `["bar","foo"]`. `Load("bar")` returns service, `Inspect("decloud-bar")` returns `{State:"running",ContainerID:"cid-bar"}`. `Load("foo")` returns service, `Inspect("decloud-foo")` returns `{State:"exited",ContainerID:"cid-foo"}`. Assert result has exactly 2 entries, ordered `bar` then `foo`, with states `running` and `stopped` respectively (exit→stopped rewrite).
- `TestLifecycle_StatusAll_AbsentContainerKept` — `Inspect` returns `{State:"absent"}` for one service. Assert that row has `State == "absent"`, `ContainerID == ""`.
- `TestLifecycle_StatusAll_ConfigOnlyOrphan` — `Load` returns `nil, registry.ErrSecretsMissing` for one service. Assert that row has `State == "config-only"`, `ContainerName == ""`, `ContainerID == ""`. Assert NO Inspect call is made for that service.
- `TestLifecycle_StatusAll_PerServiceLoadErrorIsSynthesised` — `ListNames` returns `["a","b","c"]`. `Load("a")` and `Load("c")` succeed (with matching Inspect calls). `Load("b")` returns `nil, fmt.Errorf("%w: ...", registry.ErrSchemaMismatch)`. Assert result has 3 rows in order `a,b,c`. Row `b` has `State == "error"`, `ErrorDetail` contains `"schema_version mismatch"`. Rows `a` and `c` are fully populated. **One failure does NOT abort the listing.**
- `TestLifecycle_StatusAll_PerServiceInspectErrorIsSynthesised` — `Load` succeeds for one service; `Inspect` for that service returns an arbitrary error. Row has `State == "error"`, `ErrorDetail` non-empty. Other services unaffected.
- `TestLifecycle_StatusAll_ServiceVanishedAfterListIsDropped` — `ListNames` returns `["a","b"]`. `Load("a")` succeeds; `Load("b")` returns `nil, fmt.Errorf("%w: b", registry.ErrNotFound)`. Assert result has exactly 1 row (`a` only). Row for `b` is **dropped** (concurrent-deploy race policy).
- `TestLifecycle_StatusAll_ListNamesErrorAbortsAndPropagates` — `ListNames` returns `nil, errors.New("permission denied")`. Assert `_, err := h.lc.StatusAll(ctx)`; `err != nil`, `err.Error()` contains `"permission denied"`. **Zero Load/Inspect calls.**

### 6.3 `internal/cli/lifecycle_commands_test.go` (CLI layer)

All use `installMockLifecycle(t)` and `runRoot(t, ...)`.

- `TestStatus_NoArgs_DelegatesToStatusAllAndPrintsTable` — mock returns 2 rows (foo running + bar stopped). `runRoot(t, "status")`. Assert no error. Assert stdout contains `"NAME"` (header), `"STATE"`, `"foo"`, `"running"`, `"bar"`, `"stopped"`. Assert stderr is empty.
- `TestStatus_NoArgs_EmptyListPrintsHeaderOnly` — mock returns `nil, nil`. Assert stdout contains `"NAME"` and `"DEPLOYED_AT"`. Assert stdout has exactly one line (the header). Use `strings.Count(stdout.String(), "\n") == 1`.
- `TestStatus_NoArgs_StatusAllErrorPropagates` — mock returns `nil, errors.New("registry: reading services dir /opt/decloud/config/services: permission denied")`. Assert `runRoot` returns a non-nil error whose message contains `"permission denied"`. Stdout body is empty (or just contains the header — header-first or error-first is a design choice; **decision: error-first, stdout empty**, because the error path returns before tabwriter writes anything). Tighten: implementation must check `err != nil` from `lc.StatusAll(ctx)` BEFORE writing the header.

  Wait — re-check §1.4: `runStatusAll` does check `err` first and returns before printing. Confirmed. Test assertion: `assert.Empty(t, stdout.String())`.
- `TestStatus_NoArgs_ErrorRowGoesToStderr` — mock returns 2 rows: one good, one with `State:"error"` and `ErrorDetail:"loading service: registry: schema_version mismatch"`. Assert stdout contains BOTH service names and the literal `"error"` state token (and `"-"` for the empty fields). Assert stderr contains `"status: <name>: "` followed by `"schema_version mismatch"`.
- `TestStatus_OneArg_StillPrintsSingleLine` — **unchanged from current test**. Keep verbatim. This locks single-service backward compat.
- `TestStatus_TwoArgs_FailsWithUsageError` — `runRoot(t, "status", "foo", "bar")`. Assert `err != nil`. Optional: assert `ExitCodeFor(err) == ExitUsageError`. The Cobra error message contains `"accepts"`, which `isCobraUsageError` matches. (Note: the lifecycle mock will NOT be installed in this test, because Cobra rejects before `RunE` runs — no mock expectations to set.)

### 6.4 What is NOT tested

- Tabwriter column-pixel-alignment (it's stdlib; testing stdlib's output bytes is a change-detector test).
- The `Use:` and `Short:` strings (no semantic-token contract per `_ai/cli-flag-surface-coherence.md`).
- The exact stderr prefix (`"status: "`) — substring match is enough; the prefix wording can evolve.

## 7. Edge cases the implementation MUST handle (recap)

Reproduced from Don §5 with my refinements:

1. **Zero services** — header only, exit 0. Code path: `ListNames` → `ReadDir` → `fs.ErrNotExist` → `(nil, nil)` → `StatusAll` → `(empty slice, nil)` → CLI prints header, flushes, returns nil.
2. **Orphan container without registry entry** — explicitly NOT shown. We list the registry. Documented in §8.
3. **Service in `config-only`** — `Load` returns `ErrSecretsMissing`; `Status` returns `Status{Name, State:"config-only"}`; multi-row path includes the row with `-` placeholders.
4. **Corrupted TOML on one of N** — row has `State:"error"`, `ErrorDetail` populated; other rows fine; stderr carries detail; exit 0.
5. **Inspect fails for one of N** — row has `State:"error"`, `ErrorDetail` populated; other rows fine; stderr carries detail; exit 0.
6. **`ListNames` fails outright** — abort, return wrapped error, non-zero exit (`ExitInternal` 70). No partial output.
7. **One positional arg** — unchanged path, bit-for-bit identical output.
8. **>1 positional args** — Cobra's `MaximumNArgs(1)` returns `"accepts at most 1 arg(s), received N"`; `isCobraUsageError` matches `"accepts"`; `ExitCodeFor` returns `ExitUsageError` (2).
9. **Long names** — service-name regex caps at 39 chars; container name at 47 (`decloud-` prefix + 39); deploy ID is fixed at 22 chars (`YYYYMMDD-HHMMSS-XXXXXX`). Tabwriter pads to widest cell — no truncation needed.
10. **Concurrent deploy race** — `Load` after `ListNames` returns `ErrNotFound` because the file was removed between calls. **Drop the row** (do not synthesize an error row). Documented at §1.3 and tested in §6.2.

## 8. Docs surfaces (Raymond, after Rob ships)

Per `_ai/cli-flag-surface-coherence.md`, four surfaces; for this task surface 1 (runtime) and surface 3 (help text) are in `status.go`, surfaces 2 (error message) and 4 (`_docs/usage.md`) are docs work:

1. `_docs/usage.md` §4 (line 194 today) — replace `decloud status <name>` bullet with `decloud status [name]` and add: "Without `<name>`: list every registered service as an aligned table (one row per service)."
2. `_docs/usage.md` §4 Status format (line 200 today) — split into "Single-service format" (existing, unchanged) and "Multi-service format" (new). The new subsection documents:
   - The five column headers in order.
   - The state values exactly: `running`, `stopped`, `absent`, `config-only`, `error`. **Five values total — not nine. Match the implementation.**
   - The `-` placeholder convention.
   - Zero-services behaviour (header only).
   - Stderr error-detail channel: `status: <name>: <error>` lines.
3. `_docs/usage.md` §5 End-to-end example (line 217) — optionally add a one-line `$ decloud status` example showing two rows.
4. `README.md:104` — change `decloud status <name>` to `decloud status [name]`. Append: "Without a name, prints one row per registered service."

Raymond MUST cross-check the state-value enum against `internal/deploy/lifecycle.go` (`Status` and `StatusAll`). Kevlin's hallucination check at review time independently verifies. The enum is exactly five values: `running`, `stopped`, `absent`, `config-only`, `error`. Drift = docs bug.

## 9. Risk register & landmines

1. **Mock regen surprise.** If `go generate ./...` picks up unrelated drift in other mock files, Rob's commit will contain unrelated mock changes. Mitigation: Rob runs `git status` after `go generate` and verifies only the three expected mock files changed (mock_store.go, mock_lifecycle.go, mock_deployer.go — last one diff-empty). If anything else moved, file a separate task.
2. **Two-args test expectation drift.** Cobra's error wording for `MaximumNArgs` is `"accepts at most 1 arg(s), received N"`. The `"accepts"` substring is the matcher; if Cobra ever changes that string we'll silently route to `ExitInternal` instead of `ExitUsageError`. There's no test today that locks this beyond `isCobraUsageError`'s callers; adding `TestStatus_TwoArgs_FailsWithUsageError` with an `ExitCodeFor` assertion gives us a regression lock.
3. **Tabwriter `Flush` error handling.** Tabwriter's `Flush` can return an error from the underlying writer; the existing code at single-service `Fprintf` ignores write errors (matches Go stdlib operator-tool convention). For multi-row, I wrap with `fmt.Errorf("flushing status table: %w", err)`. The chance of a real failure here is essentially zero (writing to a `bytes.Buffer` in tests, to stdout in production), but ignoring it would mean we'd silently truncate output on a closed-pipe — the wrap surfaces it.
4. **`%w: %v` slip on `ErrorDetail` synthesis.** Inside `StatusAll`, we set `ErrorDetail: err.Error()` — string, not wrapped. This is correct: `ErrorDetail` is a presentation field, not part of an error chain. But a future "helpful refactor" might switch to `ErrorDetail` as `error` type, at which point `%w` discipline applies. Comment in the `Status` struct calls this out.
5. **`Status` struct field addition and TOML roundtrip.** The `Status` struct is NOT persisted — it's a return value from `Lifecycle.Status`/`StatusAll`. Adding `ErrorDetail` does not touch any TOML-marshalled struct (`ServiceConfig`, `ServiceSecrets`, etc. are in `internal/registry/`, separate from `internal/deploy/Status`). Verified by grep: `Status{` is constructed only in `internal/deploy/lifecycle.go`, never marshalled.
6. **Sort key stability.** `sort.Strings` is byte-order; service names match `[a-z][a-z0-9-]{0,38}`, so byte-order == lexicographic == operator expectation. Locale-independent. No `LC_ALL` surprise.
7. **The Caddyfile path still uses `Store.List`.** After the refactor, `regenerateAndReload` at `internal/deploy/service.go:396` calls `List`, which now calls `ListNames` internally. The silent-skip-on-Load-failure behaviour MUST stay. Kent's `TestFSStore_List_StillSilentlySkipsLoadErrors` guards this. The Caddyfile generator never sees the broken service; the operator running `decloud status` sees it as an error row. **Two readers of the same registry, two different failure semantics, by design.** This is OK because `decloud status` and Caddy regen serve different operator intents.

## 10. Quality bar — what makes this RIGHT

- Single-service path: bit-for-bit identical stdout. Existing test `TestStatus_DelegatesToLifecycleAndPrintsResult` stays green untouched.
- Multi-service path: reuses `Status(ctx, name)` directly inside the loop. One source of truth for "what is one service's state."
- One row's failure does not poison the listing. The whole point of `decloud status` (no args) is "show me what's up."
- Output is grep- and awk-friendly: header on stdout, errors on stderr, no decoration that scripts must filter.
- Zero new dependencies. `text/tabwriter` is stdlib.
- No flag matrix. No `--all`, no `--format=json`, no `--quiet`.
- The synthesized state enum is exactly one new token (`error`). Future-extensible without breaking the existing five values.

## 11. What Rob and Kent execute, in order

1. **Kent first** (per workflow): writes the tests in §6 with `nil` implementations stubbed out where needed (or skips tests pending Rob's interface additions). On main this won't compile — that's fine, Kent's commit lands the failing tests and Rob's commit makes them pass. Standard task-flow.
2. **Rob next**: in this order to keep `go generate` and `go build` happy:
   - Edit `internal/registry/store.go` (interface + `ListNames` impl + `List` refactor).
   - Edit `internal/deploy/service.go` (`Status` struct, `Lifecycle` interface).
   - Edit `internal/deploy/lifecycle.go` (`StatusAll` impl).
   - Edit `internal/cli/status.go` (whole-file rewrite per §1.4).
   - Run `go generate ./...` from repo root.
   - Run `gofmt -w internal/`.
   - Run `go build ./...` and `go test ./...`.
   - Verify grep `'%w: %v'` returns zero across changed files.
   - Verify mock-file diff scope (3 expected files; 1 diff-empty).
3. **Raymond after**: docs in §8.

Do not ship until §6 tests pass AND `decloud status` (against a fake-registry temp dir) produces a header-only output when zero services, and a tabulated output when ≥1 services.

## 12. What I want from downstream agents

- **Linus review on this plan**: confirm no layering violation (CLI does not import `registry` for layout knowledge — it routes through `Lifecycle.StatusAll`). Confirm the state-enum trim from five to one is acceptable.
- **Kent**: own the test list in §6. Add `TestFSStore_List_StillSilentlySkipsLoadErrors` even if it duplicates intent with `TestStore_ListSkipsMalformedFiles` — the new test is a regression lock; the old test asserted same intent but is named for malformed-file scope, not for "we deliberately preserve the silent-skip contract."
- **Rob**: §11 execution order; grep `%w: %v` returns zero; mock diff scope.
- **Raymond**: §8 doc surfaces; five state values not nine.
- **Kevlin**: at review, hallucination-check the state-enum list (`running`, `stopped`, `absent`, `config-only`, `error`) against `internal/deploy/lifecycle.go` and any docs claiming otherwise. The trim from Don's nine to my five is intentional and must be reflected in every surface.

Don't ship shit.
