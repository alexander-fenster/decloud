# 006 — Joel's Tech Plan v2: containerise Caddy onto the `decloud` network

Author: Joel Spolsky (planning agent)
Date: 2026-04-27
Status: Tech expansion of `005-don-plan-v2.md`. Standalone document; supersedes `003-joel-tech-plan.md`. Awaiting Linus re-review.

## 0. How to read this document

This is the implementation handbook for Don's v2 plan. Linus required seven revisions in `004-linus-review.md` and Don applied all seven in `005-don-plan-v2.md`. This document binds those revisions to file paths, function signatures, struct shapes, exact `docker` argv, and the test inventory. Section 14 maps each revision to the section that satisfies it.

There are **no open questions**. Every one of Joel-v1's eight open questions was answered by Linus and folded into Don v2. v2 has nothing left to ask.

The bones are unchanged from v1: a three-piece decomposition (driver primitives, caddy manager, reloader rewire), Cobra wiring under the existing `caddy` parent command, and a doc rewrite. What changed: the readiness probe, the rollback, the `--image` flag, and the `cmdFactory` test seam are all GONE. The dual-stack ports are baked into `RunOptions`. The deploy-failure error text gives one-command recovery. The migration story leads with the volume-copy recipe.

---

## 1. Functional spec — what the operator sees

### 1.1 New commands (zero flags)

```
decloud caddy up
decloud caddy down
decloud caddy reload   (existing; semantics unchanged from operator POV)
```

`up` and `down` take no flags. `reload` takes no flags (unchanged from today). There is no `--image`, no `--config-file`, no `--restart`, no `--network`. M1 scope (`_ai/decisions/m1-scope.md:18`) prohibits Viper; v1's `--image` proposal violated that and is cut.

### 1.2 `decloud caddy up`

**Synopsis:** Bring up the `decloud-caddy` container so Caddy can route to service containers by name.

**Behaviour, in order:**

1. `Driver.NetworkEnsure(ctx, "decloud")`. Idempotent.
2. `caddy.WriteStubIfMissing(paths.CaddyfilePath)`. Writes the existing stub if `/opt/decloud/config/caddy/Caddyfile` is missing. Idempotent.
3. `Driver.Inspect(ctx, "decloud-caddy")`.
   - `running` → emit `caddy already running` to stdout, return nil (exit 0).
   - `exited` → `Driver.Start(ctx, "decloud-caddy")`, emit `caddy started`, return nil (exit 0).
   - `absent` → `Driver.ImagePull(ctx, "caddy:2")` then `Driver.RunWithOptions(ctx, ...)` per §3.2, emit two log lines, return nil (exit 0).
   - any other state ("created", "paused", "restarting", "removing", "dead") → wrap as `ErrCaddyUp` with the unexpected state in the message; return non-zero (exit 40).
4. On any docker error, return non-zero with the error wrapped per §6.

**No flags. No Viper. No TOML config.** `caddy.DefaultImage = "caddy:2"` is the only image source.

**No readiness/admin-API probe.** `Up` returns when `RunWithOptions` returns. See §4.2.

**No rollback on partial failure.** Without the probe, there is no follow-up step to fail. See §4.2.

**Stdout shape on success (absent → run):**

```
caddy up: pulled caddy:2
caddy up: container decloud-caddy running on network decloud
```

**Stdout shape on success (exited → start):**

```
caddy started
```

**Stdout shape on success (already running):**

```
caddy already running
```

The `caddy up:` prefix on the cold-start lines is grep-friendly for operators who script around it. The shorter messages on the warm paths mirror the actual state transition (no pull, no run).

**Idempotency:** Running `decloud caddy up` ten times in a row is safe. Nine of those see "already running."

### 1.3 `decloud caddy down`

**Synopsis:** Stop and remove the `decloud-caddy` container. Volumes (`decloud_caddy_data`, `decloud_caddy_config`) are NOT removed.

**Behaviour, in order:**

1. `Driver.Stop(ctx, "decloud-caddy", 10*time.Second)`. If `errors.Is(err, ErrContainerNotFound)`, proceed.
2. `Driver.Remove(ctx, "decloud-caddy")`. If `errors.Is(err, ErrContainerNotFound)`, proceed.
3. Emit `caddy down: container removed (volumes retained)`.

**Exit code:** 0 on success including the absent-container case. 40 on real docker failures.

**Volume retention is a hard contract.** The help text on `decloud caddy down` says: "Stops and removes the decloud-caddy container. Named volumes decloud_caddy_data and decloud_caddy_config are NOT removed; remove them manually with `docker volume rm` if you intend to wipe ACME state."

### 1.4 `decloud caddy reload`

Operator-visible behaviour does NOT change. Internally, the reloader now `docker exec`s into `decloud-caddy` via `Driver.Exec` instead of shelling a host `caddy` binary. The error path when `decloud-caddy` is not running gets a more specific text — see §1.5.

### 1.5 Error texts

| Trigger | Text | Exit |
|---|---|---|
| `caddy up` and ports 80/443 already bound (host Caddy still running from M1.0) | `caddy up: ports 80/443 already in use; if you ran the M1.0 install, run 'systemctl disable --now caddy && systemctl mask caddy' or 'apt-get remove -y caddy' to make the change persistent` | 40 |
| `caddy up` and Docker daemon down | `caddy up: docker network ensure: <docker stderr>` | 40 |
| `caddy up` and `caddy:2` pull failure (network) | `caddy up: docker pull caddy:2: <docker stderr>` | 40 |
| `caddy up` and IPv6 listener fails (kernel IPv6 disabled) | `caddy up: docker run: listen tcp [::]:80: socket: address family not supported by protocol` (raw stderr passes through) | 40 |
| `caddy reload` and container missing | `caddy reload: container "decloud-caddy" is not running; run 'decloud caddy up' first` | 60 |
| `caddy reload` and Caddyfile syntactically invalid | `caddy reload: caddy validate: <validate stderr>` | 60 |
| `caddy down` and Docker daemon down | `caddy down: docker stop: <stderr>` | 40 |
| `decloud deploy service` and Caddy is not running | `deploy: caddy reload failed: container "decloud-caddy" is not running; service is registered and running but Caddy is not routing traffic; run 'decloud caddy up' (and then 'decloud caddy reload' if needed) to restore routing` | 60 |

The "ports already bound" detection is best-effort substring matching against `docker run` stderr (`address already in use` or `port is already allocated`). Substring miss → fall through to the generic message. We do NOT pre-flight `lsof` — adds a host dependency for one error case.

### 1.6 Help text

`decloud caddy --help`:

```
Caddy management

Usage:
  decloud caddy [command]

Available Commands:
  up      Run the decloud-caddy container on the decloud network
  down    Stop and remove the decloud-caddy container (volumes preserved)
  reload  Regenerate Caddyfile from registry, validate, and reload Caddy
```

`decloud caddy up --help`:

```
Run the decloud-caddy container on the decloud network.

Ensures the decloud Docker network exists, writes the Caddyfile stub if missing,
and starts (or runs) the decloud-caddy container with dual-stack publishing on
80/tcp, 443/tcp, and 443/udp. The container uses image caddy:2 and named
volumes decloud_caddy_data and decloud_caddy_config for ACME and runtime state.

Usage:
  decloud caddy up
```

`decloud caddy down --help`:

```
Stop and remove the decloud-caddy container.

The named volumes decloud_caddy_data and decloud_caddy_config are NOT removed;
remove them manually with `docker volume rm` if you intend to wipe ACME state.

Usage:
  decloud caddy down
```

---

## 2. File inventory (clean restatement)

### 2.1 Files added (7)

| Path | Purpose |
|---|---|
| `internal/caddy/manager.go` | `Manager` interface (`Up`, `Down`, `IsRunning`); `cliManager` impl on top of `Driver`; constants `ContainerName`, `NetworkName`, `DefaultImage`; sentinels `ErrCaddyUp`, `ErrCaddyDown`. |
| `internal/caddy/manager_test.go` | Gomock-against-`Driver` tests per §6.1. |
| `internal/caddy/mocks/mock_manager.go` | Generated by `go generate`. |
| `internal/cli/caddy_up.go` | Cobra command; takes no flags. |
| `internal/cli/caddy_down.go` | Cobra command; takes no flags. |
| `internal/cli/caddy_up_test.go` | CLI-layer tests for `up` per §6.4. |
| `internal/cli/caddy_down_test.go` | CLI-layer tests for `down` per §6.4. |

Note: `_ai/decisions/caddy-runs-in-container.md` is Raymond's deliverable, enumerated in §10.

### 2.2 Files modified (10)

| Path | Change |
|---|---|
| `internal/dockerdrv/driver.go` | Add types `RunOptions`, `PortMap`, `VolumeMount`, `ExecOptions`. Extend `Driver` interface with three methods: `ImagePull`, `RunWithOptions`, `Exec`. Existing methods byte-identical. |
| `internal/dockerdrv/cli_driver.go` | Implement `ImagePull`, `RunWithOptions`, `Exec`. Existing methods byte-identical. |
| `internal/dockerdrv/cli_driver_test.go` | Add argv-shape tests per §6.3. Existing tests untouched. |
| `internal/dockerdrv/mocks/mock_driver.go` | Regenerated by `go generate`. |
| `internal/caddy/reloader.go` | `cliReloader` invokes `Driver.Exec` for `caddy validate` and `caddy reload` against `decloud-caddy`. Translates host paths to container paths. **Delete `cmdFactory` field, `newCLIReloaderWithFactory`, and the package-private `cmdFactory` type alias.** New constructor `NewCLIReloader(driver dockerdrv.Driver, hostCaddyDir string) Reloader`. New doc-comments on `Reloader.Validate`/`Reload` documenting the bind-mount path constraint. |
| `internal/caddy/reloader_test.go` | Drop the host-`caddy` argv tests (`TestReloader_InvokesCaddyValidate`, `TestReloader_InvokesCaddyReload`, `TestReloader_ValidateFailureReturnsError`); they assert against an obsolete contract. Drop the `recordingFactory` and `failingFactory` helpers (tied to `cmdFactory`). Add the six tests per §6.2. |
| `internal/cli/deploy_service.go` | `buildProductionDeployer` and `buildProductionLifecycle` change `caddy.NewCLIReloader()` (zero args) to `caddy.NewCLIReloader(driver, paths.CaddyDir)` (the same `driver` the deploy uses). Add `caddyManagerFactory` test seam mirroring `deployerFactory`/`lifecycleFactory`, with the same parallelism caveat. |
| `internal/cli/root.go` | Mount `newCaddyUpCmd(rc)` and `newCaddyDownCmd(rc)` as siblings of `newCaddyReloadCmd(rc)` under the existing `caddy` parent (line 40-42). |
| `internal/cli/exit_codes.go` | Add `caddy.ErrCaddyUp` and `caddy.ErrCaddyDown` to `ExitCodeFor` mapping → `ExitRunFail` (40). **No new constants.** |
| `internal/deploy/service.go` | Update the wrap text on the validate-failed and reload-failed legs of `regenerateAndReload` (lines 314-322) per §4.4. The wrap chain still uses `%w` against `ErrCaddyReload`; **no new sentinel**. |

### 2.3 Files NOT modified

- `internal/caddy/generator.go`, `internal/caddy/stub.go` — output unchanged under new architecture.
- `internal/deploy/service.go` outside lines 314-322 — orchestration logic, transaction shape, registry interaction unchanged.
- `internal/deploy/lifecycle.go`, `internal/deploy/readiness.go` — readiness already goes through bridge IP; no Caddy dependency.
- `internal/registry/*`, `internal/ids/*`, `internal/envcap/*`, `internal/config/paths.go` — unchanged.
- `internal/cli/exit_codes_test.go` — extension is mechanical (one new mapping case); existing tests untouched.

### 2.4 Doc files (Raymond's deliverables)

| Path | Action |
|---|---|
| `_docs/install.md` | §3 rewritten (Caddy install replaced); §5 cross-reference updated; **lines 61-62 paragraph DELETED, not edited**; new §3.1 migration block leads with volume-copy recipe; new §3.2 SELinux NIT. |
| `_docs/usage.md` | §1 quick start gains `decloud caddy up`; §6 paragraph "`docker run -p ...` is never invoked" rewritten — Caddy is the documented exception; §7 reload-recovery updated for `docker exec`. |
| `_ai/decisions/caddy-runs-in-container.md` | NEW. Decision record with full rejected-alternatives list (Don's A/B + Linus's seven), volume strategy, dual-stack rationale, migration recipes (full), M4 admin-API forward note. **Reviewed during Phase 4 alongside the CLI surface, not after Phase 7.** |
| `_ai/m1x-backlog.md` | Append item: real-Docker integration test for first happy-path deploy. |
| `_ai/MEMORY.md` | One line under "Architecture decisions" pointing at the new decision file. |
| `_ai/cli-flag-surface-coherence.md` | NO CHANGE in this task (no new flags). |

---

## 3. New CLI / driver surfaces

### 3.1 Driver extension

Three new methods, NOT bolted-on `RunRequest` fields. Linus approved this in his §3.1 answer to Joel v1 Q3.

```go
// internal/dockerdrv/driver.go (additions)

// PortMap describes a single host:container port publish, including the host
// bind interface. Dual-stack publishing is expressed by emitting one PortMap
// for "0.0.0.0" and another for "[::]" for the same container port. The
// driver's RunWithOptions emits one `-p` flag per PortMap, in declared order.
type PortMap struct {
    HostBind      string // "0.0.0.0", "[::]", or "" (Docker default; do not use in M1)
    HostPort      int    // 1..65535
    ContainerPort int    // 1..65535
    Proto         string // "tcp" or "udp"
}

// VolumeMount describes a single -v flag. Bind mounts use Source as a host
// directory path; named volumes use Source as the volume name and IsNamed=true.
// The driver does not interpret the path; it just splices into "src:dst[:ro]".
type VolumeMount struct {
    Source   string
    Target   string
    ReadOnly bool
    IsNamed  bool
}

// RunOptions is the full-control variant of RunRequest. Service deploys keep
// using Run(RunRequest); RunWithOptions is for callers that need port
// publishing, volume mounts, or labels (currently: caddy.Manager).
type RunOptions struct {
    Name    string
    Image   string
    Network string
    Restart string
    Ports   []PortMap         // emitted in declared order, one -p per entry
    Volumes []VolumeMount     // emitted in declared order, one -v per entry
    Labels  map[string]string // emitted with sorted keys
    Env     map[string]string // emitted with sorted keys
}

// ExecOptions describes a `docker exec` invocation. Stdin is never wired
// (Caddy's reload/validate are non-interactive); -i/-t are never passed.
type ExecOptions struct {
    Container string
    Cmd       []string  // argv inside the container, e.g. ["caddy","reload","--config","/etc/caddy/Caddyfile"]
    Stdout    io.Writer // optional
    Stderr    io.Writer // optional; if nil, stderr is captured for ErrContainerNotFound detection only
}

// Driver gains three methods.
type Driver interface {
    // ... existing methods unchanged ...

    // ImagePull invokes `docker pull <ref>`. Returns nil on success (including
    // the cache-hit case where Docker reports "Image is up to date").
    ImagePull(ctx context.Context, ref string) error

    // RunWithOptions invokes `docker run -d` with the full set of flags
    // described by opts. Returns the trimmed stdout (the container ID).
    RunWithOptions(ctx context.Context, opts RunOptions) (containerID string, err error)

    // Exec invokes `docker exec <container> <cmd...>`. Returns
    // ErrContainerNotFound when the container does not exist; returns a
    // wrapped error with stderr otherwise.
    Exec(ctx context.Context, opts ExecOptions) error
}
```

**Why three new methods, not three new fields on `RunRequest`:** Linus accepted the v1 argument. Service deploys do not need `Ports`/`Volumes`/`Labels`; smearing them into `RunRequest` makes every deploy test grow defensive `assert.Empty(t, req.Volumes)`. Two narrow types are cleaner than one polymorphic type.

**Why `HostBind` on `PortMap` (NEW in v2):** Linus revision #4 mandates dual-stack publishing. The cleanest expression of "bind to both `0.0.0.0` and `[::]`" is two `PortMap` entries with different `HostBind` values. The alternative — a `PortMap` with a `[]string` of binds — collapses two distinct publish operations into one knob and forces the driver to multiplex the `-p` emit. Two entries, one emit each; argv reads exactly like the operator would type it. This also lets us write the dual-stack test (`TestCLIDriver_RunWithOptionsDualStackPorts`) without Caddy specifics.

**Why `IsNamed` on `VolumeMount`:** Docker disambiguates by syntax — `-v /abs/path:/dst` is a bind mount, `-v name:/dst` is a named volume. The driver cannot tell them apart from the source string alone if the operator picked a non-absolute volume name (rare but possible). Explicit `IsNamed` removes the ambiguity at the type level.

### 3.2 The exact `docker run` for `decloud-caddy` (DUAL-STACK)

```
docker run -d \
  --name decloud-caddy \
  --network decloud \
  --restart unless-stopped \
  --label decloud.managed=caddy \
  -p 0.0.0.0:80:80/tcp \
  -p [::]:80:80/tcp \
  -p 0.0.0.0:443:443/tcp \
  -p [::]:443:443/tcp \
  -p 0.0.0.0:443:443/udp \
  -p [::]:443:443/udp \
  -v /opt/decloud/config/caddy:/etc/caddy:ro \
  -v decloud_caddy_data:/data \
  -v decloud_caddy_config:/config \
  caddy:2
```

The corresponding `RunOptions` literal that the manager hands to the driver:

```go
caddy.RunOptions = dockerdrv.RunOptions{
    Name:    caddy.ContainerName,    // "decloud-caddy"
    Image:   caddy.DefaultImage,     // "caddy:2"
    Network: caddy.NetworkName,      // "decloud"
    Restart: "unless-stopped",
    Labels:  map[string]string{"decloud.managed": "caddy"},
    Ports: []dockerdrv.PortMap{
        {HostBind: "0.0.0.0", HostPort: 80,  ContainerPort: 80,  Proto: "tcp"},
        {HostBind: "[::]",    HostPort: 80,  ContainerPort: 80,  Proto: "tcp"},
        {HostBind: "0.0.0.0", HostPort: 443, ContainerPort: 443, Proto: "tcp"},
        {HostBind: "[::]",    HostPort: 443, ContainerPort: 443, Proto: "tcp"},
        {HostBind: "0.0.0.0", HostPort: 443, ContainerPort: 443, Proto: "udp"},
        {HostBind: "[::]",    HostPort: 443, ContainerPort: 443, Proto: "udp"},
    },
    Volumes: []dockerdrv.VolumeMount{
        {Source: paths.CaddyDir,        Target: "/etc/caddy", ReadOnly: true,  IsNamed: false},
        {Source: "decloud_caddy_data",  Target: "/data",      ReadOnly: false, IsNamed: true},
        {Source: "decloud_caddy_config", Target: "/config",   ReadOnly: false, IsNamed: true},
    },
}
```

Notes per Linus revision #4:
- **All six port maps are unconditional.** No host-IPv6-detection step. Failure on IPv6-disabled hosts is loud and recognisable; that is acceptable per Don §4.3.
- The bind `/etc/caddy:ro` — Caddy reads the Caddyfile, never writes it. The deployer (running on the host as `decloud`) writes; the bind mount is the directory so atomic rename is visible inside the container.
- `decloud_caddy_data` and `decloud_caddy_config` are NEVER removed by `decloud caddy down`. Operator-only.
- HTTP/3 over QUIC needs UDP/443 — that's why three protos × two binds = six.
- No `--network-alias`. `decloud-caddy` is its own DNS name on the bridge by virtue of `--name`.

### 3.3 Argv expansion for `RunWithOptions`

Argv build order in `cliDriver.RunWithOptions`:

```
docker run -d
  --name <Name>
  --network <Network>
  --restart <Restart>
  [--env K=V ...]              (sorted by key; same shape as existing Run)
  [--label K=V ...]            (sorted by key)
  [-p <bind>:<host>:<container>[/<proto>] ...]
                                (DECLARED order; one -p per PortMap;
                                 if HostBind is "" the bind segment is omitted —
                                 not used in M1, but contract-clean)
  [-v <src>:<target>[:ro] ...] (DECLARED order; one -v per VolumeMount;
                                 IsNamed and bind path use the same syntax —
                                 docker disambiguates by source-shape)
  <Image>
```

Concretely, for the Caddy `RunOptions` above, the emitted argv is:

```go
[]string{
    "run", "-d",
    "--name", "decloud-caddy",
    "--network", "decloud",
    "--restart", "unless-stopped",
    "--label", "decloud.managed=caddy",
    "-p", "0.0.0.0:80:80/tcp",
    "-p", "[::]:80:80/tcp",
    "-p", "0.0.0.0:443:443/tcp",
    "-p", "[::]:443:443/tcp",
    "-p", "0.0.0.0:443:443/udp",
    "-p", "[::]:443:443/udp",
    "-v", "/opt/decloud/config/caddy:/etc/caddy:ro",
    "-v", "decloud_caddy_data:/data",
    "-v", "decloud_caddy_config:/config",
    "caddy:2",
}
```

Notes:
- Labels are sorted by key (one entry here, but the contract holds for N).
- Ports are declared-order (NOT sorted) — the operator-readability of the test argv tracks the input slice; sorting would obscure the dual-stack pairing.
- Volumes are declared-order — `:ro` overlap semantics depend on order if we ever need them; predictability beats sort.
- Env is unused for Caddy but follows the same sort-by-key shape as `Run` (`cli_driver.go:52-58`).
- Each `PortMap` renders as `<HostBind>:<HostPort>:<ContainerPort>/<Proto>`. Empty `HostBind` collapses to `<HostPort>:<ContainerPort>/<Proto>` (Docker default — used only as a non-M1 contract-clean fallback).

### 3.4 The exact `docker exec` for reload/validate

```
docker exec decloud-caddy caddy validate --config /etc/caddy/Caddyfile.tmp
docker exec decloud-caddy caddy reload   --config /etc/caddy/Caddyfile
```

Path translation: deployer writes to host path `/opt/decloud/config/caddy/Caddyfile.tmp` (= `Paths.CaddyfilePath + ".tmp"`). The reloader translates to the container path `/etc/caddy/Caddyfile.tmp` for the `--config` flag.

### 3.5 Cobra wiring

`internal/cli/root.go` after changes (the `caddy` block):

```go
caddy := &cobra.Command{Use: "caddy", Short: "Caddy management"}
caddy.AddCommand(newCaddyUpCmd(rc))
caddy.AddCommand(newCaddyDownCmd(rc))
caddy.AddCommand(newCaddyReloadCmd(rc))
root.AddCommand(caddy)
```

New test seam in `internal/cli/deploy_service.go`:

```go
// caddyManagerFactory mirrors deployerFactory/lifecycleFactory; same
// parallel-safety constraint. Tests reassign during setup, restore in teardown.
// Do NOT call t.Parallel() in any internal/cli test — concurrent reassignment
// is unsafe.
var caddyManagerFactory = buildProductionCaddyManager

func buildProductionCaddyManager(paths config.Paths) (caddy.Manager, error) {
    return caddy.NewCLIManager(caddy.ManagerConfig{
        Driver: dockerdrv.NewCLIDriver(),
        Paths:  paths,
        // Image: omitted — caddy.DefaultImage is the only source.
    }), nil
}
```

`buildProductionDeployer` and `buildProductionLifecycle` change the reloader construction:

```go
// before: caddy.NewCLIReloader()
// after:
driver := dockerdrv.NewCLIDriver()
return deploy.NewServiceDeployer(deploy.Dependencies{
    Paths:     paths,
    Store:     registry.NewFSStore(paths),
    Capturer:  envcap.New(),
    Driver:    driver,
    Generator: caddy.NewGenerator(),
    Reloader:  caddy.NewCLIReloader(driver, paths.CaddyDir),
})
```

Both `buildProductionDeployer` and `buildProductionLifecycle` (`internal/cli/deploy_service.go:127-147`) get the same edit. Note both must hold the **same** `driver` instance — the deploy and the reload share state at the `cliDriver` struct level (currently nothing, but the contract is "one driver per CLI invocation").

---

## 4. Behaviour spec for new/changed functions

### 4.1 `caddy.Manager` interface

```go
// internal/caddy/manager.go

//go:generate mockgen -source=manager.go -destination=mocks/mock_manager.go -package=mocks

const (
    ContainerName = "decloud-caddy"
    NetworkName   = "decloud"
    DefaultImage  = "caddy:2"
)

var (
    ErrCaddyUp   = errors.New("caddy: up failed")
    ErrCaddyDown = errors.New("caddy: down failed")
)

type Manager interface {
    // Up ensures the decloud network exists, the Caddyfile stub is on disk,
    // and the decloud-caddy container is running. Idempotent. Does NOT
    // probe Caddy's admin API; returns when docker run/start returns.
    Up(ctx context.Context) error

    // Down stops and removes the decloud-caddy container. Named volumes
    // (decloud_caddy_data, decloud_caddy_config) are NOT removed. Idempotent.
    Down(ctx context.Context) error

    // IsRunning reports whether the decloud-caddy container is in the
    // "running" state. Returns false (no error) when the container is absent
    // or in any non-running state.
    IsRunning(ctx context.Context) (bool, error)
}

type ManagerConfig struct {
    Driver dockerdrv.Driver
    Paths  config.Paths
    Stdout io.Writer // defaults to os.Stdout if nil
}

func NewCLIManager(cfg ManagerConfig) Manager {
    if cfg.Stdout == nil {
        cfg.Stdout = os.Stdout
    }
    return &cliManager{cfg: cfg}
}
```

**Note:** `ManagerConfig` does NOT carry an `Image` field. There is no override. `caddy.DefaultImage` is referenced directly inside the manager. If M2 introduces Viper-driven overrides, the field gets added then.

### 4.2 `Manager.Up(ctx)` — exact algorithm (NO PROBE, NO ROLLBACK)

```
1. m.cfg.Driver.NetworkEnsure(ctx, NetworkName)
   on err: return fmt.Errorf("%w: network ensure: %w", ErrCaddyUp, err)

2. caddy.WriteStubIfMissing(m.cfg.Paths.CaddyfilePath)
   on err: return fmt.Errorf("%w: stub write: %w", ErrCaddyUp, err)

3. inspect, err := m.cfg.Driver.Inspect(ctx, ContainerName)
   on err: return fmt.Errorf("%w: inspect: %w", ErrCaddyUp, err)

4. switch inspect.State {
   case "running":
       fmt.Fprintln(m.cfg.Stdout, "caddy already running")
       return nil
   case "exited":
       err := m.cfg.Driver.Start(ctx, ContainerName)
       if err != nil { return fmt.Errorf("%w: start: %w", ErrCaddyUp, err) }
       fmt.Fprintln(m.cfg.Stdout, "caddy started")
       return nil
   case "absent":
       // proceed to step 5
   default:
       return fmt.Errorf("%w: unexpected container state %q", ErrCaddyUp, inspect.State)
   }

5. err := m.cfg.Driver.ImagePull(ctx, DefaultImage)
   on err: return fmt.Errorf("%w: image pull: %w", ErrCaddyUp, err)
   fmt.Fprintf(m.cfg.Stdout, "caddy up: pulled %s\n", DefaultImage)

6. _, err := m.cfg.Driver.RunWithOptions(ctx, m.runOpts())
   on err: return fmt.Errorf("%w: docker run: %w", ErrCaddyUp, err)
   fmt.Fprintln(m.cfg.Stdout, "caddy up: container decloud-caddy running on network decloud")

7. return nil
```

`m.runOpts()` builds the literal in §3.2. It is a pure function of `m.cfg.Paths.CaddyDir` (for the bind mount source) — no other inputs.

**No probe between steps 6 and 7.** `RunWithOptions` returns when `docker run` returns; that is the success boundary. Per Linus revision #1 / Don §4.2.

**No rollback.** If step 6 fails, the container either does not exist (clean state) or exists in a non-`running` state (which the next `Up` re-runs picks up via the step-3 switch). There is no follow-up step inside `Up` that could leave a partial state. Per Linus revision #2 / Don §4.2.

### 4.3 `Manager.Down(ctx)` — exact algorithm

```
1. err := m.cfg.Driver.Stop(ctx, ContainerName, 10*time.Second)
   if errors.Is(err, dockerdrv.ErrContainerNotFound): proceed
   else if err != nil: return fmt.Errorf("%w: stop: %w", ErrCaddyDown, err)

2. err := m.cfg.Driver.Remove(ctx, ContainerName)
   if errors.Is(err, dockerdrv.ErrContainerNotFound): proceed
   else if err != nil: return fmt.Errorf("%w: remove: %w", ErrCaddyDown, err)

3. fmt.Fprintln(m.cfg.Stdout, "caddy down: container removed (volumes retained)")
4. return nil
```

### 4.4 `Manager.IsRunning(ctx)` — exact algorithm

```
1. inspect, err := m.cfg.Driver.Inspect(ctx, ContainerName)
2. on err: return false, err  (not wrapped; caller decides; in M1 there is one caller and it's a test)
3. return inspect.State == "running", nil
```

### 4.5 Updated `Reloader` (NO `cmdFactory`)

```go
// internal/caddy/reloader.go

//go:generate mockgen -source=reloader.go -destination=mocks/mock_reloader.go -package=mocks

// Reloader wraps Caddy's validate + reload subcommands inside the
// decloud-caddy container.
//
// IMPORTANT CONTRACT: configPath passed to Validate/Reload MUST be a host
// path inside the bind-mounted Caddyfile directory (Paths.CaddyDir). Paths
// outside that directory return an error with no exec attempt.
type Reloader interface {
    // Validate runs `caddy validate` against configPath translated to the
    // container's view of the bind mount. configPath must be inside
    // hostCaddyDir; otherwise an error is returned without invoking Caddy.
    Validate(ctx context.Context, configPath string) error

    // Reload runs `caddy reload` against configPath translated to the
    // container's view of the bind mount. Same path constraint as Validate.
    Reload(ctx context.Context, configPath string) error
}

type cliReloader struct {
    driver       dockerdrv.Driver
    hostCaddyDir string
}

// NewCLIReloader returns the production reloader. driver is the same
// dockerdrv.Driver the deploy uses; hostCaddyDir is the host-side directory
// bind-mounted into the decloud-caddy container at /etc/caddy.
func NewCLIReloader(driver dockerdrv.Driver, hostCaddyDir string) Reloader {
    return &cliReloader{driver: driver, hostCaddyDir: hostCaddyDir}
}

func (r *cliReloader) Validate(ctx context.Context, configPath string) error {
    return r.execCaddy(ctx, "validate", configPath)
}

func (r *cliReloader) Reload(ctx context.Context, configPath string) error {
    return r.execCaddy(ctx, "reload", configPath)
}

func (r *cliReloader) execCaddy(ctx context.Context, sub, hostPath string) error {
    ctrPath, err := r.translatePath(hostPath)
    if err != nil {
        return err // already prefixed
    }
    var stderr bytes.Buffer
    err = r.driver.Exec(ctx, dockerdrv.ExecOptions{
        Container: ContainerName,
        Cmd:       []string{"caddy", sub, "--config", ctrPath},
        Stderr:    &stderr,
    })
    if err == nil {
        return nil
    }
    if errors.Is(err, dockerdrv.ErrContainerNotFound) || isNotRunningStderr(stderr.String()) {
        return fmt.Errorf("caddy %s: container %q is not running; run 'decloud caddy up' first",
            sub, ContainerName)
    }
    return fmt.Errorf("caddy %s: %w; stderr=%q", sub, err, stderr.String())
}

func (r *cliReloader) translatePath(hostPath string) (string, error) {
    cleanHost := filepath.Clean(hostPath)
    cleanRoot := filepath.Clean(r.hostCaddyDir)
    rel, err := filepath.Rel(cleanRoot, cleanHost)
    if err != nil || rel == ".." || strings.HasPrefix(rel, "../") {
        return "", fmt.Errorf("caddy reloader: path %q is outside the bind-mount %q",
            hostPath, r.hostCaddyDir)
    }
    return path.Join("/etc/caddy", rel), nil
}

// isNotRunningStderr matches the docker exec stderr signature when the
// container exists but is not in the running state. ErrContainerNotFound
// covers the absent case; this covers the exited/created/restarting cases.
func isNotRunningStderr(s string) bool {
    return strings.Contains(strings.ToLower(s), "is not running")
}
```

**`cmdFactory` is gone.** No struct field, no type alias, no `newCLIReloaderWithFactory`. Tests use `MockDriver.Exec`. Path translation is pure Go and tests itself.

**Doc-comments on the interface methods** spell out the bind-mount-path constraint per Linus §5.4.

### 4.6 `Driver.Exec` impl

```go
func (d *cliDriver) Exec(ctx context.Context, opts ExecOptions) error {
    args := append([]string{"exec", opts.Container}, opts.Cmd...)
    var stderr bytes.Buffer
    cmd := d.cmd(ctx, "docker", args...)
    cmd.Stdout = opts.Stdout
    if opts.Stderr != nil {
        cmd.Stderr = io.MultiWriter(opts.Stderr, &stderr)
    } else {
        cmd.Stderr = &stderr
    }
    if err := cmd.Run(); err != nil {
        if isNotFound(stderr.String()) {
            return ErrContainerNotFound
        }
        return fmt.Errorf("docker exec: %w; stderr=%q", err, stderr.String())
    }
    return nil
}
```

No `-i`, no `-t`. Reuses the existing `isNotFound` helper.

### 4.7 `Driver.ImagePull` impl

```go
func (d *cliDriver) ImagePull(ctx context.Context, ref string) error {
    var stderr bytes.Buffer
    cmd := d.cmd(ctx, "docker", "pull", ref)
    cmd.Stderr = &stderr
    if err := cmd.Run(); err != nil {
        return fmt.Errorf("docker pull %s: %w; stderr=%q", ref, err, stderr.String())
    }
    return nil
}
```

### 4.8 `Driver.RunWithOptions` impl

```go
func (d *cliDriver) RunWithOptions(ctx context.Context, opts RunOptions) (string, error) {
    args := []string{
        "run", "-d",
        "--name", opts.Name,
        "--network", opts.Network,
        "--restart", opts.Restart,
    }
    // Env, sorted by key — same shape as Run.
    envKeys := make([]string, 0, len(opts.Env))
    for k := range opts.Env {
        envKeys = append(envKeys, k)
    }
    sort.Strings(envKeys)
    for _, k := range envKeys {
        args = append(args, "--env", k+"="+opts.Env[k])
    }
    // Labels, sorted by key.
    labelKeys := make([]string, 0, len(opts.Labels))
    for k := range opts.Labels {
        labelKeys = append(labelKeys, k)
    }
    sort.Strings(labelKeys)
    for _, k := range labelKeys {
        args = append(args, "--label", k+"="+opts.Labels[k])
    }
    // Ports, declared order, one -p per entry.
    for _, p := range opts.Ports {
        args = append(args, "-p", formatPortMap(p))
    }
    // Volumes, declared order, one -v per entry.
    for _, v := range opts.Volumes {
        args = append(args, "-v", formatVolume(v))
    }
    args = append(args, opts.Image)

    var stdout, stderr bytes.Buffer
    cmd := d.cmd(ctx, "docker", args...)
    cmd.Stdout = &stdout
    cmd.Stderr = &stderr
    if err := cmd.Run(); err != nil {
        return "", fmt.Errorf("docker run: %w; stderr=%q", err, stderr.String())
    }
    return strings.TrimSpace(stdout.String()), nil
}

func formatPortMap(p PortMap) string {
    // <HostBind>:<HostPort>:<ContainerPort>/<Proto>; HostBind may be empty.
    proto := p.Proto
    if proto == "" {
        proto = "tcp"
    }
    if p.HostBind == "" {
        return fmt.Sprintf("%d:%d/%s", p.HostPort, p.ContainerPort, proto)
    }
    return fmt.Sprintf("%s:%d:%d/%s", p.HostBind, p.HostPort, p.ContainerPort, proto)
}

func formatVolume(v VolumeMount) string {
    s := v.Source + ":" + v.Target
    if v.ReadOnly {
        s += ":ro"
    }
    return s
}
```

### 4.9 Updated wrap text in `internal/deploy/service.go:314-322`

Current code:

```go
if err := d.deps.Reloader.Validate(ctx, tmpPath); err != nil {
    _ = os.Remove(tmpPath)
    return fmt.Errorf("%w: caddy validate failed: %w", ErrCaddyReload, err)
}
// ...
if err := d.deps.Reloader.Reload(ctx, d.deps.Paths.CaddyfilePath); err != nil {
    return fmt.Errorf("%w: caddy reload failed: %w", ErrCaddyReload, err)
}
```

Updated code (per Linus revision #5 / Don §4.4):

```go
if err := d.deps.Reloader.Validate(ctx, tmpPath); err != nil {
    _ = os.Remove(tmpPath)
    return fmt.Errorf("%w: caddy validate failed: %w; service is registered and running but Caddy is not routing traffic; run 'decloud caddy up' (and then 'decloud caddy reload' if needed) to restore routing", ErrCaddyReload, err)
}
// ...
if err := d.deps.Reloader.Reload(ctx, d.deps.Paths.CaddyfilePath); err != nil {
    return fmt.Errorf("%w: caddy reload failed: %w; service is registered and running but Caddy is not routing traffic; run 'decloud caddy up' (and then 'decloud caddy reload' if needed) to restore routing", ErrCaddyReload, err)
}
```

**No new sentinel.** The wrap chain still uses `ErrCaddyReload`. The reloader's own actionable error ("container ... is not running; run 'decloud caddy up' first") is preserved through the inner `%w`. The deployer's wrap text adds the registry-state context — the reloader can't say "the service is registered" because the reloader doesn't know that; only the deployer does.

**Operator perception:** the error grows by ~140 characters. CLI users who scrape exit codes are unaffected (still 60). Users who read text get useful guidance. Net positive per Linus's review.

**Note on text duplication:** the same suffix appears twice (validate leg, reload leg). Considered factoring into a constant; chose not to because (a) two sites is below the rule-of-three threshold, (b) the validate/reload distinction in the prefix matters for diagnosis and inlining the suffix keeps the message readable, (c) future-Don is more likely to adjust one of the two messages independently than both. If a third site appears, refactor.

---

## 5. Constants and naming

```go
// internal/caddy/manager.go

const (
    ContainerName = "decloud-caddy"
    NetworkName   = "decloud"
    DefaultImage  = "caddy:2"
)
```

Rationale:

- `ContainerName` is a singleton fixture name. It does NOT belong in `internal/ids` because `ids.ContainerName(name)` derives `"decloud-"+name` from a service name; Caddy is not a service.
- `NetworkName` deduplicates the `"decloud"` literal scattered across `internal/deploy/service.go:131,190,289` and `internal/dockerdrv/cli_driver.go:170`. Per Linus §5.6: existing literal cleanup is M1.x backlog material, BUT new code (manager + reloader + caddy_up.go + caddy_down.go) MUST reference `caddy.NetworkName`. Do not introduce a seventh `"decloud"` literal.
- `DefaultImage` is the only image source. No flag, no env var, no TOML key (Linus revision #3 / Don §4.1).

---

## 6. Test inventory

Per CLAUDE.md: Testify, Gomock, no change-detector tests. Tests live next to the code they exercise. No integration tests in this task (Linus §7 confirms `_ai/decisions/m1-test-strategy.md`).

### 6.1 `internal/caddy/manager_test.go` (new)

Use `gomock.InOrder` per `_ai/gomock-inorder-sequencing.md`. Mock `dockerdrv.Driver`. Drive a real `cliManager`; assert through the recorder. Use `t.TempDir()` for `Paths.CaddyfilePath`.

| Test | Asserts |
|---|---|
| `TestManager_UpFreshInstall` | Order: `NetworkEnsure(decloud)` → `Inspect(decloud-caddy)` returns `absent` → `ImagePull(caddy:2)` → `RunWithOptions` with **exact** `RunOptions` matching §3.2 (name, network, restart, label, six dual-stack PortMap entries, three VolumeMount entries). Stdout contains both "pulled caddy:2" and "running on network decloud". |
| `TestManager_UpAlreadyRunning` | Order: `NetworkEnsure` → `Inspect` returns `running`. **No** `ImagePull`, **no** `RunWithOptions`, **no** `Start`. Stdout contains "caddy already running". |
| `TestManager_UpAfterPriorStop` | Order: `NetworkEnsure` → `Inspect` returns `exited` → `Start(decloud-caddy)`. **No** `ImagePull`, **no** `RunWithOptions`. Stdout contains "caddy started". |
| `TestManager_UpUnexpectedStateWraps` | `Inspect` returns `paused` (or any non-handled state). Returned error is `ErrCaddyUp`-wrapped and message contains the unexpected state literal. |
| `TestManager_UpNetworkEnsureFails` | `NetworkEnsure` returns sentinel `errFake`. Manager returns error where `errors.Is(err, ErrCaddyUp)` AND `errors.Is(err, errFake)` are both true (locks `%w: %w` discipline per `_ai/error-wrap-discipline.md`). |
| `TestManager_UpImagePullFails` | Same wrap shape on the pull leg. |
| `TestManager_UpRunFailsWithoutRollback` | `RunWithOptions` returns sentinel; manager returns `ErrCaddyUp`-wrapped error. Verifies that **no** subsequent `Stop`/`Remove` is called (locks the no-rollback contract). |
| `TestManager_UpStubWriteFailsWrappedAsCaddyUp` | Pre-create `Paths.CaddyfilePath`'s parent as a regular file (so `MkdirAll` fails). Assert `Up` returns `ErrCaddyUp`-wrapped. |
| `TestManager_DownHappyPath` | Order: `Stop(decloud-caddy, 10s)` → `Remove(decloud-caddy)`. |
| `TestManager_DownContainerAbsent` | `Stop` returns `ErrContainerNotFound` → `Remove` returns `ErrContainerNotFound` → `Down` returns nil. |
| `TestManager_DownStopFailsHard` | Non-`ErrContainerNotFound` from `Stop` → wrapped `ErrCaddyDown`, `Remove` not called. |
| `TestManager_IsRunningTrueFalseAbsent` | Three sub-tests: `running` → true, nil; `exited` → false, nil; `absent` → false, nil. |

**Tests explicitly NOT in v2:** No `TestManager_UpRollsBackOnPostRunFailure`. No `TestManager_UpReadinessProbeRetries`. No `TestManager_UpReadinessProbeTimeout`. No `TestCaddyUp_PassesImageOverrideFromFlag`. These all lived in Joel-v1 §8.1/§8.4 and are vacuous after Linus revisions #1, #2, #3.

### 6.2 `internal/caddy/reloader_test.go` (rewritten)

**Drop:** `TestReloader_InvokesCaddyValidate`, `TestReloader_InvokesCaddyReload`, `TestReloader_ValidateFailureReturnsError`. They assert against the host-`caddy` argv contract that no longer exists. Drop the helpers `recordingFactory` and `failingFactory` (tied to the deleted `cmdFactory`).

**Add:**

| Test | Asserts |
|---|---|
| `TestReloader_ValidateCallsDockerExec` | Mock `Driver.Exec` expects `ExecOptions{Container: "decloud-caddy", Cmd: ["caddy","validate","--config","/etc/caddy/Caddyfile.tmp"]}`. Returns nil. `Validate` returns nil. |
| `TestReloader_ReloadCallsDockerExec` | Mock `Driver.Exec` expects `Cmd: ["caddy","reload","--config","/etc/caddy/Caddyfile"]`. Returns nil. `Reload` returns nil. |
| `TestReloader_PathTranslationCanonicalForm` | Construct reloader with `hostCaddyDir = "/opt/decloud/config/caddy"`. Pass `/opt/decloud/config/caddy/Caddyfile.tmp` to `Validate`; capture `Driver.Exec` `Cmd`; assert `Cmd[3] == "/etc/caddy/Caddyfile.tmp"`. (Linus §7 positive case.) |
| `TestReloader_PathTranslationOutsideBindMount` | Pass `/tmp/foo` to `Validate`; assert returned error message contains `outside the bind-mount`; assert **zero** calls to `Driver.Exec` (`gomock.Times(0)`). |
| `TestReloader_PathTranslationParentEscape` | Pass `/opt/decloud/config/caddy/../../etc/passwd` to `Validate`; assert outside-bind-mount error; zero `Exec` calls. (Locks the `..` rejection in `translatePath`.) |
| `TestReloader_ContainerNotRunningSurfacesActionableError` | Mock `Driver.Exec` returns `dockerdrv.ErrContainerNotFound`. `Validate` returns error whose message contains `container "decloud-caddy" is not running; run 'decloud caddy up' first`. |
| `TestReloader_ContainerExitedSurfacesActionableError` | Mock `Driver.Exec` returns a non-`ErrContainerNotFound` error AND writes `Error response from daemon: Container decloud-caddy is not running` to the stderr writer. Assert the actionable error. (Locks the stderr substring branch.) |
| `TestReloader_ValidateExitNonzeroPreservesStderr` | Mock `Driver.Exec` returns a generic exec error AND writes `bad caddyfile syntax` to stderr. Assert returned error wraps the inner error AND contains `bad caddyfile syntax`. (Locks `%w: %w` discipline; mirrors `TestDeploy_BuildErrorPreservesInnerSentinel`.) |

**No `cmdFactory`-based tests.** No `newCLIReloaderWithFactory`.

### 6.3 `internal/dockerdrv/cli_driver_test.go` (additions; existing tests untouched)

Each new test pairs argv with the hand-typed `docker` equivalent in a comment, per the file's existing convention.

| Test | Argv asserted |
|---|---|
| `TestCLIDriver_ImagePullArgs` | `pull caddy:2` |
| `TestCLIDriver_ImagePullPropagatesStderrOnFailure` | `pull` stderr surfaces in returned error. |
| `TestCLIDriver_ExecArgsBasic` | `exec decloud-caddy caddy validate --config /etc/caddy/Caddyfile.tmp` |
| `TestCLIDriver_ExecPropagatesContainerNotFound` | `Exec` against absent container (stderr `No such container`) → `ErrContainerNotFound`. |
| `TestCLIDriver_ExecPropagatesGenericError` | `Exec` failure not matching `isNotFound` → wrapped error containing stderr. |
| `TestCLIDriver_RunWithOptionsCaddyShape` | Full Caddy `docker run` argv per §3.3, **including all six dual-stack port maps**. This is the canonical end-to-end shape test. |
| `TestCLIDriver_RunWithOptionsDualStackPorts` | Independent of Caddy: `RunOptions` with `Ports: [{HostBind:"0.0.0.0",HostPort:1234,ContainerPort:5678,Proto:"tcp"}, {HostBind:"[::]",HostPort:1234,ContainerPort:5678,Proto:"tcp"}]` produces argv segments `-p 0.0.0.0:1234:5678/tcp -p [::]:1234:5678/tcp` in declared order. (Linus §10 non-blocking-but-encouraged; bake it in.) |
| `TestCLIDriver_RunWithOptionsBindReadOnly` | `VolumeMount{Source:"/host",Target:"/dst",ReadOnly:true,IsNamed:false}` → `-v /host:/dst:ro`. |
| `TestCLIDriver_RunWithOptionsNamedVolumeNotReadOnly` | `VolumeMount{Source:"vol",Target:"/dst",ReadOnly:false,IsNamed:true}` → `-v vol:/dst`. |
| `TestCLIDriver_RunWithOptionsLabelsSorted` | Labels `{"b":"2","a":"1"}` emit `--label a=1 --label b=2` (sorted). |
| `TestCLIDriver_RunWithOptionsPortsDeclaredOrder` | `Ports: [{HostPort:443,...},{HostPort:80,...}]` emits in declared order — NOT sorted. |
| `TestCLIDriver_RunWithOptionsPortDefaultProto` | `PortMap{Proto:""}` defaults to `tcp` in argv. |
| `TestCLIDriver_RunWithOptionsEmptyHostBind` | `PortMap{HostBind:"",HostPort:80,ContainerPort:80,Proto:"tcp"}` → `-p 80:80/tcp` (no bind segment). Contract-clean fallback; not used in M1. |

### 6.4 `internal/cli/caddy_up_test.go`, `caddy_down_test.go` (new)

`caddyManagerFactory` test seam, identical pattern to `deployerFactory` / `lifecycleFactory`. NO `t.Parallel()`. Mock `caddy.Manager` via gomock.

| Test file | Test | Assertion |
|---|---|---|
| `caddy_up_test.go` | `TestCaddyUp_DelegatesToManager` | Mock `Manager.Up` returns nil; `decloud caddy up` returns nil. |
| `caddy_up_test.go` | `TestCaddyUp_ManagerErrorReturnsExitRunFail` | Mock returns `fmt.Errorf("%w: ...", caddy.ErrCaddyUp)`; `ExitCodeFor(err)` returns 40. |
| `caddy_up_test.go` | `TestCaddyUp_NoFlags` | `decloud caddy up --image foo` exits with usage error (locks "no flags exist" contract). |
| `caddy_down_test.go` | `TestCaddyDown_DelegatesToManager` | Mirror. |
| `caddy_down_test.go` | `TestCaddyDown_ManagerErrorReturnsExitRunFail` | Mirror. |
| `caddy_down_test.go` | `TestCaddyDown_NoFlags` | Mirror. |

**No `TestCaddyUp_PassesImageOverrideFromFlag`** — there is no `--image` flag.

### 6.5 `internal/cli/exit_codes_test.go` (extended)

Add two cases to the existing parameterised test:

| Input error | Expected exit code |
|---|---|
| `fmt.Errorf("%w: ...", caddy.ErrCaddyUp)` | 40 (`ExitRunFail`) |
| `fmt.Errorf("%w: ...", caddy.ErrCaddyDown)` | 40 (`ExitRunFail`) |

### 6.6 `internal/deploy/service_test.go` (verify only)

Reloader is mocked through `caddy.Reloader`. The constructor signature change at the production wiring layer (§3.5) does NOT reach the deploy tests. After the change: `go test ./internal/deploy/...` must pass without diff. If a test asserts on `caddy.NewCLIReloader(...)`, it is over-specified and we cut it.

**One specific check:** the existing test for `regenerateAndReload` failure assertions (in `service_test.go`) checks that `errors.Is(err, ErrCaddyReload)` holds. After the wrap-text change in §4.9, that property is preserved (the `%w` chain is unchanged); the test must continue to pass without modification.

### 6.7 Tests we are explicitly NOT adding

- Generator tests (output unchanged).
- Real-Docker integration tests (deferred per `_ai/decisions/m1-test-strategy.md`; backlog item per §10).
- Caddy's own behaviour (cert provisioning, HTTP/3, ALPN).
- Tests for `_docs/*.md` content (Raymond owns; Kevlin reviews).

---

## 7. Order of operations for the implementation

Phases per Don v2. Decision-doc review is in **Phase 4**, not Phase 7 (Linus §5.7).

**Phase 1 — Driver primitives (foundation):**
- Kent: `internal/dockerdrv/cli_driver_test.go` argv tests for `ImagePull`, `Exec`, `RunWithOptions` per §6.3. Tests fail (missing methods).
- Rob: extend `internal/dockerdrv/driver.go` interface; implement in `cli_driver.go`; `go generate` regenerates `mocks/mock_driver.go`. Tests pass.

**Phase 2 — Caddy manager:**
- Kent: `internal/caddy/manager_test.go` per §6.1.
- Rob: `internal/caddy/manager.go` per §4.1-4.4; `go generate` for `mocks/mock_manager.go`.

**Phase 3 — Reloader rewire:**
- Kent: rewrite `internal/caddy/reloader_test.go` per §6.2 (this DELETES three existing tests and the `recordingFactory`/`failingFactory` helpers — correct because they assert against an obsolete contract).
- Rob: rewrite `internal/caddy/reloader.go` per §4.5. Update `buildProductionDeployer` and `buildProductionLifecycle` in `internal/cli/deploy_service.go` to pass `driver` and `paths.CaddyDir`.

**Phase 4 — CLI surface (and decision-doc review):**
- Kent: `internal/cli/caddy_up_test.go`, `caddy_down_test.go` per §6.4. Add `caddyManagerFactory` test seam.
- Rob: `internal/cli/caddy_up.go`, `caddy_down.go` per §1.2-1.3; wire into `internal/cli/root.go`.
- Raymond: deliver a draft of `_ai/decisions/caddy-runs-in-container.md` per §10.3, reviewed alongside the CLI surface in this phase. The decision doc is the authoritative ground-truth for Kevlin's hallucination check.

**Phase 5 — Exit-code wiring + deploy error text:**
- Kent: extend `internal/cli/exit_codes_test.go` per §6.5.
- Rob: extend `ExitCodeFor` in `internal/cli/exit_codes.go`. Update wrap text in `internal/deploy/service.go:314-322` per §4.9.

**Phase 6 — Docs (Raymond):**
- Rewrite `_docs/install.md` per §10.1.
- Update `_docs/usage.md` per §10.2.
- Finalise `_ai/decisions/caddy-runs-in-container.md` per §10.3 (after draft from Phase 4).
- Append `_ai/m1x-backlog.md` item.
- Add the one MEMORY.md line.

**Phase 7 — Verification gate:**
- `gofmt -l .` (must be empty)
- `go vet ./...` (must be empty)
- `go generate ./...` followed by `git status --porcelain` (must be empty)
- `go test ./... -count=1` (all green)
- Manual verification per Don v2 §7.

Dependencies:
- Phase 2 depends on Phase 1.
- Phase 3 depends on Phase 1.
- Phase 4 depends on Phase 2.
- Phase 5 depends on Phase 2 (sentinels live in `caddy`).
- Phase 6 depends on Phases 1-5.
- Phase 7 depends on everything.
- Phases 2 and 3 are parallelisable.

---

## 8. Doc-update checklist (for Raymond)

Don't write docs at the top level; this is Raymond's task list.

### 8.1 `_docs/install.md`

- **§3 ("Install Caddy")** — replace the systemd block with: "Decloud manages Caddy as a container. After installing the binary (§6), run `decloud caddy up`. There is no host `caddy` package to install." No `--image` flag mentioned. No TOML config mentioned.
- **§5 ("Create the shared Docker network")** — keep. Note that `decloud caddy up` will create it if missing; the explicit step is for operators who want to inspect the network before bringing Caddy up.
- **NEW §3.1 "Migrating from M1.0"** — leads with the **volume-copy recipe as the recommended path** per Linus revision #7 / Don §4.6:
  ```
  RECOMMENDED MIGRATION (preserves ACME state — DO THIS unless you have only 1-2 hostnames):

    systemctl disable --now caddy && systemctl mask caddy
    # OR: apt-get remove -y caddy

    docker volume create decloud_caddy_data
    docker run --rm \
      -v /var/lib/caddy/.local/share/caddy:/from \
      -v decloud_caddy_data:/to \
      alpine sh -c 'cp -a /from/. /to/'

    decloud caddy up

  ALTERNATIVE (cold restart — only if you have 1-2 hostnames or no production traffic):

    systemctl disable --now caddy && systemctl mask caddy
    decloud caddy up
    # First request per hostname will pause for ~1-3 seconds while Caddy obtains
    # a fresh Let's Encrypt cert. With many hostnames you risk tripping LE rate
    # limits, including the 50-certs-per-domain-per-week cap, with up to a
    # 7-day recovery window.
  ```
  The `systemctl mask` / `apt-get remove` step is mandatory (not just `disable --now`) because `disable --now` does NOT prevent the unit from being re-enabled by a package upgrade — Linus §6 #2.

- **NEW §3.2 "SELinux note"** — one-line warning per Linus's answer to Joel-v1 Q2: "On SELinux-enforcing hosts, you may need to relabel `/opt/decloud/config/caddy` (`chcon -Rt container_file_t /opt/decloud/config/caddy`); we don't ship SELinux support in M1."
- **NEW §3.3 "Firewall note"** — UDP/443 must be open in any host-level firewall (`ufw`, `firewalld`) for HTTP/3.
- **§1 "Prerequisites"** — replace "Caddy install" with "DNS records pointing at the host"; the latter is the only true prereq now.
- **Lines 61-62 paragraph** ("Caddy will fail to start until the Caddyfile exists. The first `decloud deploy service` writes a stub Caddyfile, after which `systemctl start caddy` succeeds.") — **DELETE, do not edit** per Linus §6 #3 / Don §10 criterion #12. The paragraph is structurally obsolete; editing it leaves stale assumptions in place.

### 8.2 `_docs/usage.md`

- **§1 ("Quick start")** — prepend a step: "If you have not yet, run `decloud caddy up` once to bring Caddy online."
- **§4 lifecycle commands** — add `decloud caddy up` and `decloud caddy down` to the bullet list with one-line semantics each.
- **§6 ("Debugging a container directly")** — the paragraph at lines 181-192 currently says "Decloud deliberately does not publish container ports to the host (`docker run -p ...` is never invoked)." Per Linus §5.5, **rewrite** that paragraph (do not patch) to something like: "Decloud deliberately does not publish container ports to the host for service containers; service-to-Caddy traffic flows over the shared `decloud` Docker network. The one documented exception is the `decloud-caddy` container itself, which DOES publish 80/tcp, 443/tcp, and 443/udp on both `0.0.0.0` and `[::]` because Caddy is the public ingress."
- **§7 ("Recovering from caddy reload failures")** — replace the "read `journalctl -u caddy`" step with "read `docker logs decloud-caddy`". Add: "If the failure message says the container is not running, run `decloud caddy up` first, then `decloud caddy reload`."

### 8.3 `_ai/decisions/caddy-runs-in-container.md` (NEW)

Topics to cover:

1. **Decision:** Caddy runs as `decloud-caddy` on the `decloud` Docker network, managed by `decloud caddy up/down`.
2. **Why:** Caddyfile uses container names; container names resolve only via embedded Docker DNS (`127.0.0.11`); embedded Docker DNS is only available to network members.
3. **Rejected alternatives:** Don's Candidates A and B PLUS Linus's seven (per `004-linus-review.md` §1.1):
   - `host.docker.internal` / extra_hosts — wrong direction.
   - `--network host` — same bug, different paint.
   - `--network container:decloud-<service>` — catastrophic coupling.
   - Sidecar (Caddy-per-service) — Kubernetes thinking on a single-host MVP.
   - `/etc/hosts` injection — race-prone, operator-trust violation.
   - `--resolvers 127.0.0.11` from a host Caddy — embedded DNS only answers in-container.
   - Host-local `dnsmasq`/`unbound` — bigger surface than C.
4. **Volume strategy:** `decloud_caddy_data:/data` (ACME) and `decloud_caddy_config:/config` (autosave). NEVER removed by `decloud caddy down`. Operator-only via `docker volume rm`.
5. **HTTP/3 (UDP/443):** published on both stacks. Without it, mobile clients silently degrade.
6. **Dual-stack publishing:** rationale per §4.3 of Don v2. Failure mode on IPv6-disabled hosts is loud and acceptable.
7. **Migration from M1.0 host Caddy:** full recipes (the install doc has the operator-facing summary; this doc has the full rationale, including the LE rate-limit math).
8. **No Viper, no `--image` flag in M1:** explicit reference to `_ai/decisions/m1-scope.md:18`. M2 is where overrides land.
9. **No readiness probe in `Manager.Up`:** explicit reasoning per Don v2 §4.2. The right place for a retry, if it ever becomes needed, is the reloader's `docker exec` path.
10. **No rollback in `Manager.Up`:** vacuous after #9 (Linus revision #2).
11. **Concurrent-deploy race:** acknowledged M2+ concern per Don v2 §5.10.
12. **Forward-looking M4 admin-API note:** when blue/green lands, decide whether the Decloud host process talks to `decloud-caddy` over `localhost:2019` (admin API published) or via `docker exec`. Flag for M4 tech plan; in M1, the admin API is NOT host-published.

### 8.4 `_ai/MEMORY.md`

Add one line under "Architecture decisions": "Caddy runs as a Decloud-managed container on the `decloud` network — see `_ai/decisions/caddy-runs-in-container.md`."

### 8.5 `_ai/m1x-backlog.md`

Append item #N: "Real-Docker integration test for the first happy-path deploy. Per Don's §6.4 of `_tasks/2026-04-27-caddy-container-connection-refused/002-don-plan.md` and Linus's confirmation in `004-linus-review.md` §7. M1 test strategy stands; this is M2 material."

### 8.6 `_ai/cli-flag-surface-coherence.md`

**No change in this task.** v1 proposed adding `--image` to the canonical flag-surface list; that proposal is cut.

---

## 9. Gotchas and landmines

### 9.1 Bind-mount-source must exist before container start

`docker run -v /opt/decloud/config/caddy:/etc/caddy:ro` will create `/opt/decloud/config/caddy` if it does not exist, with permissions `root:root 0755` from the daemon. Decloud's install creates it explicitly (`_docs/install.md` §4 today). If skipped, Docker creates it, then `caddy.WriteStubIfMissing` writes the Caddyfile, then Caddy starts and reads it. Works.

SELinux-enforcing hosts (RHEL family) require relabel; documented as a §3.2 NIT in install.md per §8.1.

### 9.2 Atomic rename across bind mount

The deployer writes `<dir>/Caddyfile.tmp`, validates it via `docker exec`, renames host-side to `<dir>/Caddyfile`. Both inside `Paths.CaddyDir`. The bind mount is the **directory**, not a single file, so the rename is visible inside the container immediately after the host's `os.Rename`. **Single-file binds break this**; do NOT pivot to `-v <host>/Caddyfile:/etc/caddy/Caddyfile`. The path-translation tests in §6.2 enforce that `Caddyfile.tmp` and `Caddyfile` translate correctly under the directory bind.

### 9.3 `docker exec` against a not-yet-ready Caddy admin API

`docker inspect` reports `running` before Caddy's admin API on `localhost:2019` (inside the container) is ready. v1 proposed a poll loop in `Up`; v2 cuts it (Linus revision #1, see §4.2). The race is architecturally pre-empted: between `caddy up` returning and the next `caddy reload`, the operator runs `decloud deploy service` which spends 5-60 seconds on `docker build`/`docker run`/readiness probe before ever invoking the reloader. Caddy has had ample warm-up.

If a real failure ever surfaces in the wild, the right home for the retry is `cliReloader.execCaddy` (a tiny back-off on a "container starting" stderr signature, Linus's Option C). Not in this task.

### 9.4 `docker exec` requires the container in `running` state

`docker exec` against an `exited` container returns "is not running" (NOT "no such container"). The reloader catches both shapes: `errors.Is(err, ErrContainerNotFound)` for absent, `isNotRunningStderr(stderr)` for exited. Both surface the same actionable error. Tested in `TestReloader_ContainerExitedSurfacesActionableError` (§6.2).

### 9.5 Concurrent deploys racing the reloader

`docker exec ... caddy reload` is not atomic with the file rename. Two simultaneous deploys both write `.tmp`, validate, rename — last-rename-wins. Caddy handles back-to-back reloads correctly. No issue in single-operator M1; flagged for M2+ in the decision record.

### 9.6 Image tag pinning trap

`caddy:2` floats. Surprise minor-version bumps on `docker pull` could change behaviour. The fallback for an operator who needs a fixed tag is `docker pull caddy:2.7.6 && docker tag caddy:2.7.6 caddy:2` (per Don v2 §4.1). M2 introduces `--image`/Viper for real.

### 9.7 Caddy admin API not host-published

Inside the container, Caddy's admin API listens on `localhost:2019`. We `docker exec` INSIDE the container, so `localhost` is the container, not the host. The admin API is NOT published with `-p` (no `2019:2019` entry in `RunOptions.Ports`). This is correct — exposing the admin API to the host is a security regression and we have no use for host-side admin access in M1. Locked in via the §3.2 `RunOptions` literal and tested via `TestCLIDriver_RunWithOptionsCaddyShape` (which asserts the EXACT six-port set).

### 9.8 Path translation case sensitivity

`filepath.Rel` on Linux is case-sensitive; the bind mount is case-sensitive on Linux ext4. macOS dev boxes (case-insensitive HFS) could test-pass with a wrong-case host path that production would reject. Mitigation: the path-translation tests (§6.2) include a parent-escape negative case that locks the contract; case-mismatch is a M2 nit if it ever bites.

### 9.9 `formatPortMap` IPv6 host-bind syntax

The Docker CLI accepts `[::]:80:80/tcp` (with the `[::]` brackets). `formatPortMap` splices `HostBind` literally; the manager's `RunOptions` puts `[::]` already-bracketed (see §3.2). DO NOT auto-bracket in `formatPortMap` — that would break IPv4 binds (`0.0.0.0` is not bracketed) and double-bracket IPv6 if the caller does the right thing. Test `TestCLIDriver_RunWithOptionsDualStackPorts` (§6.3) locks the literal-splice behaviour.

### 9.10 ACME state and Let's Encrypt rate limits (operator-facing, doc nit)

Per Linus revision #7: a fresh-state migration on an operator with many hostnames can hit the LE 50-certs-per-domain-per-week cap. Recovery is up to 7 days. This is exactly why the install doc leads with the volume-copy recipe (§8.1).

---

## 10. Migration story (operator-facing)

The user already has Caddy running as a host systemd unit per M1.0. The migration goes through `_docs/install.md` §3.1 (per §8.1 above). The summary, restated for completeness:

```sh
# 1. Persistently stop the host Caddy.
systemctl disable --now caddy && systemctl mask caddy
# OR
apt-get remove -y caddy

# 2. RECOMMENDED: migrate ACME state.
docker volume create decloud_caddy_data
docker run --rm \
  -v /var/lib/caddy/.local/share/caddy:/from \
  -v decloud_caddy_data:/to \
  alpine sh -c 'cp -a /from/. /to/'

# 3. Rebuild and install the new decloud binary (per _docs/install.md §6).

# 4. Bring up the containerised Caddy.
decloud caddy up

# 5. Trigger a reload to repopulate the Caddyfile from the registry.
decloud caddy reload

# 6. Verify (see §11 / Don v2 §7).
```

**Volume-copy is the recommended path, not the alternative.** The cold-restart alternative is for operators with 1-2 hostnames or no production traffic.

---

## 11. Manual verification (operator runs on the actual host)

Don v2 §7 is canonical. The two new steps over Don v1 (steps 5 and 6 in v2) are the dual-stack listener check and the `curl -4` / `curl -6` parity check; they exist specifically because of revision #4. If `ss -tlnp` shows only `0.0.0.0` listeners, the dual-stack publishing didn't take effect and the IPv6 regression is shipping; halt and investigate.

---

## 12. Acceptance criteria (Don v2 §10 + tech-plan additions)

Don v2's 19 acceptance criteria stand. I add two tech-level criteria specific to the implementation:

20. `internal/dockerdrv/driver.go` interface gains exactly three methods: `ImagePull`, `Exec`, `RunWithOptions`. Existing methods are byte-identical (no signature changes, no doc-comment churn outside of the additions).
21. `internal/caddy/reloader.go` constructor signature is `NewCLIReloader(driver dockerdrv.Driver, hostCaddyDir string) Reloader`. The `cmdFactory` field, the `cmdFactory` type alias, and `newCLIReloaderWithFactory` are all DELETED.

If any of 1-21 is missing at sign-off, the task is not done.

---

## 13. Why no integration test in this task (unchanged from v1 §13)

Linus §7 confirmed Joel-v1's rejection. Reasons (compressed):

1. Scope creep. This task is a recovery from a shipped bug; the user is blocked. `-tags integration` plumbing is a separate, larger commitment (CI runner provisioning, Docker-in-Docker, per-package convention).
2. `_ai/decisions/m1-test-strategy.md` was a real decision. The bug we're fixing is exactly the class that strategy doc anticipated would slip through; the response is "fix the bug, log the gap as M2's first integration-test target," not "expand scope now."
3. The fix is unit-testable end-to-end. Argv-shape tests on `RunWithOptions` lock the `docker run` shape. Argv-shape tests on `Exec` lock the `docker exec` shape. Manager tests lock the orchestration. CLI tests lock the wiring.
4. Manual verification per §11 is the gate.
5. The right place for the integration-test discussion is M2's first tech plan.

Backlog item per §8.5 makes the deferral explicit.

---

## 14. Revisions applied — Linus map

| # | Linus required revision | Where applied in v2 |
|---|---|---|
| 1 | Cut readiness/admin-API polling from `Manager.Up` | §1.2; §4.2 (algorithm); §6.1 (no probe tests); §9.3 |
| 2 | Cut rollback on partial failure (vacuous after #1) | §4.2; §6.1 (`TestManager_UpRunFailsWithoutRollback` locks the no-rollback contract) |
| 3 | Cut `--image` flag, Viper wiring, TOML `caddy.image`; hardcode `caddy.DefaultImage = "caddy:2"` | §1.1; §1.2; §1.6; §3.5 (no Image in `ManagerConfig`); §4.1; §5; §6.4 (`TestCaddyUp_NoFlags`); §8.6 |
| 4 | Add dual-stack IPv6 publishing (six `-p` flags); update `dockerdrv.PortMap` and `Driver.RunWithOptions` accordingly | §3.1 (`HostBind` field on `PortMap`); §3.2 (six-entry `Ports` slice); §3.3 (argv expansion); §4.8 (`formatPortMap` impl); §6.1 (`TestManager_UpFreshInstall` asserts six entries); §6.3 (`TestCLIDriver_RunWithOptionsCaddyShape` + `TestCLIDriver_RunWithOptionsDualStackPorts`); §9.9 |
| 5 | Update deploy-failure error text (`internal/deploy/service.go:314-322`) for one-command operator recovery; reuse `%w` chain, no new sentinel | §1.5 (deploy error row); §4.9 (exact wrap text on both legs); §6.6 (verify `errors.Is` chain still holds) |
| 6 | Delete `cmdFactory` from `cliReloader`; `Driver.Exec` is the only seam | §2.2 (reloader change row); §4.5 (struct has only `driver` and `hostCaddyDir`); §6.2 (no `cmdFactory`-based tests; helpers dropped) |
| 7 | Strengthen ACME migration warning; volume-copy recipe inline as recommended path; mention LE 7-day rate-limit | §1.5 (`systemctl mask` / `apt-get remove` in error text); §8.1 (full §3.1 install-doc block leads with volume-copy); §10 (operator-facing summary) |

Linus also confirmed:
- §6.4 integration test rejected — §13.
- `_docs/install.md:61-62` paragraph DELETED, not edited — §8.1.
- Decision-doc review folded into Phase 4 — §7 (Phase 4 includes Raymond's draft).

---

## 15. Open questions

**None.** All eight Joel-v1 open questions were answered by Linus and folded into Don v2:

1. Readiness loop in `Up` — CUT (Linus #1).
2. SELinux warning — NIT, one line in install doc (§8.1, §9.1).
3. Driver extension shape — APPROVED, three methods (§3.1).
4. `decloud-caddy` constant location — APPROVED in `internal/caddy/manager.go` (§5).
5. Don's §6.4 integration test — APPROVED REJECTION (§13).
6. Image float vs pin — N/A given #3 cut; default `caddy:2` floats; operator retags for pinning (§9.6).
7. `--restart unless-stopped` policy — APPROVED unconditional (§3.2).
8. `caddy up` writing the stub Caddyfile — APPROVED (§4.2 step 2).

If Linus surfaces new questions on re-review, they get an answer in v3 (or a tightening edit to v2). Until then, this plan is execution-ready for Kent and Rob.

— Joel
