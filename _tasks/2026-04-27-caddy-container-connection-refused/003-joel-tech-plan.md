# 003 — Joel's Tech Plan: containerise Caddy onto the `decloud` network

Author: Joel Spolsky (planning agent)
Date: 2026-04-27
Status: Tech expansion of Don's `002-don-plan.md`. Awaiting Linus.

## 0. Editorial position

Don's diagnosis is correct and the chosen fix (Candidate C) is the only architecturally honest option. I am NOT rubber-stamping. Two places I want to push back on or extend before we hand to Linus:

1. **The reloader change is bigger than "minimal-blast-radius"** — Don frames it as one swap of the `argv[0]`. It is not. The reloader currently runs via a host `caddy` binary that has unrestricted access to the host filesystem; after this change, the reloader runs inside the `decloud-caddy` container which only sees the `/opt/decloud/config/caddy/` directory via bind-mount. That changes the path-resolution contract of `Reloader.Validate(configPath)`. Every call site that hands the reloader a path needs to be auditable for "is this path inside the bind-mount?" The atomic-rename trick in `service.go:310-318` happens to work because it only writes inside `Paths.CaddyDir`, but I want this property documented explicitly so a future caller doesn't pass `/tmp/foo.tmp` and get a baffling "no such file" from inside the container. See §4.4.

2. **I am rejecting Don's §6.4 suggestion to add an integration test in this task.** This task is a recovery from a shipped bug, the user is blocked, and the unit-test layer is sufficient to verify the new wiring. Adding a `-tags integration` test machinery now is exactly the kind of "while we're at it" creep `_ai/decisions/m1-test-strategy.md` was written to refuse. We log it as M1.x backlog item #6 and move on. See §13 for the explicit argument.

The rest of this document is the implementation handbook.

---

## 1. Functional spec — what the operator sees

### 1.1 New commands

```text
decloud caddy up [--image <ref>]
decloud caddy down
decloud caddy reload   (existing, semantics unchanged from operator POV)
```

### 1.2 `decloud caddy up`

**Synopsis:** Bring up the `decloud-caddy` container so Caddy can route to service containers by name.

**Behaviour, in order:**

1. Ensure the `decloud` Docker network exists. If absent, create it (default bridge driver, no flags).
2. Ensure the host Caddyfile path exists. If `/opt/decloud/config/caddy/Caddyfile` is missing, write the existing stub (`internal/caddy/stub.go::stubBody`).
3. Inspect the `decloud-caddy` container.
   - State `running` → emit `caddy already running` to stdout, return exit 0.
   - State `exited` → `docker start decloud-caddy`, emit `caddy started`, return exit 0.
   - State `absent` → pull `caddy:2`, run a new container (full `docker run` form in §3.2), emit `caddy up`, return exit 0.
4. On any docker error, return non-zero with an error wrapped as `%w: <ctx>: %w` against the appropriate sentinel (see §6).

**Flags:**

| Flag | Type | Default | Notes |
|---|---|---|---|
| `--image` | string | `caddy:2` | Operator override for air-gapped or pinned-tag installs. Read from Viper key `caddy.image` if set, otherwise the literal default. |

**Config file integration (Viper / TOML):** Optional `/opt/decloud/config/decloud.toml` MAY contain:

```toml
[caddy]
image = "caddy:2.7.6"
```

Resolution order (highest wins): CLI flag → `decloud.toml` `caddy.image` → built-in default `caddy:2`. Both `caddy up` and any future Caddy-related command read through the same Viper instance, but since we have no other Viper config in M1, this is the FIRST place Viper is wired. Keep that wiring in `internal/cli/caddy_up.go` ONLY for now — do NOT preemptively introduce a global `internal/config/viper.go`. We add it when there is a second consumer (per `_ai/explicit-inputs-not-globals.md` discipline: don't invent infrastructure for one knob).

**Stdout shape on success:**

```
caddy up: pulled caddy:2
caddy up: container decloud-caddy running on network decloud
```

The two-line shape lets future-Don grep `caddy up:` in a script. One-line is harder to extend without breaking screen-scrapers; better to start with the structured prefix.

**Idempotency:** Running `decloud caddy up` ten times in a row is safe and emits `caddy already running` nine times.

### 1.3 `decloud caddy down`

**Synopsis:** Stop and remove the `decloud-caddy` container. Volumes (`decloud_caddy_data`, `decloud_caddy_config`) are NOT removed.

**Behaviour:**

1. `docker stop -t 10 decloud-caddy`. If `ErrContainerNotFound`, treat as success (idempotent).
2. `docker rm decloud-caddy`. If `ErrContainerNotFound`, treat as success.
3. Emit `caddy down: container removed (volumes retained)` to stdout.

**Exit code:** 0 on success including the no-op cases. Non-zero on a real docker failure (daemon down, permission denied) wrapped as below.

**Volume retention is a hard contract.** Operators who want to wipe ACME state must `docker volume rm decloud_caddy_data decloud_caddy_config` themselves. Document this in the help text AND in `_docs/usage.md`.

### 1.4 `decloud caddy reload`

Operator-visible behaviour does NOT change. Internally, the reloader now `docker exec`s into `decloud-caddy` instead of shelling a host `caddy` binary. The error `decloud caddy reload` returns when `decloud-caddy` is not running gets a more specific text than today's generic `caddy reload: exit 1` — see §4.3.

### 1.5 Error texts the operator will see

| Trigger | Text | Exit |
|---|---|---|
| `decloud caddy up` and ports 80/443 already bound (host Caddy still running from M1.0) | `caddy up: ports 80/443 already in use; if you ran the M1.0 install, run 'systemctl disable --now caddy' first` | 40 |
| `decloud caddy up` and Docker daemon down | `caddy up: docker network ensure: Cannot connect to the Docker daemon` | 40 |
| `decloud caddy up` and `caddy:2` pull failure (network) | `caddy up: docker pull caddy:2: <docker stderr>` | 40 |
| `decloud caddy reload` and container missing | `caddy reload: container 'decloud-caddy' is not running; run 'decloud caddy up' first` | 60 |
| `decloud caddy reload` and Caddyfile syntactically invalid | `caddy reload: caddy validate: <validate stderr>` (preserved from today) | 60 |
| `decloud caddy down` and Docker daemon down | `caddy down: docker stop: <stderr>` | 40 |

The "ports already bound" detection is best-effort: we look at `docker run` stderr for `address already in use` or `port is already allocated`. If the substring isn't there, fall through to the generic message. Do NOT pre-flight `lsof` — adds a host dependency for one error case.

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

Update `_docs/install.md` §3 to instruct `decloud caddy up` instead of writing a systemd unit. Raymond's job; checklist in §10.

---

## 2. Files to add / modify

### 2.1 Files added (8)

| Path | Purpose |
|---|---|
| `internal/caddy/manager.go` | New `Manager` interface + `cliManager` impl on top of `dockerdrv.Driver`. |
| `internal/caddy/manager_test.go` | Gomock tests for `Manager` against `mocks.MockDriver`. |
| `internal/caddy/mocks/mock_manager.go` | Generated. |
| `internal/cli/caddy_up.go` | Cobra command. |
| `internal/cli/caddy_down.go` | Cobra command. |
| `internal/cli/caddy_up_test.go` | CLI-layer tests for `up`. |
| `internal/cli/caddy_down_test.go` | CLI-layer tests for `down`. |
| `_ai/decisions/caddy-runs-in-container.md` | Decision record. (Raymond writes; we just enumerate it.) |

### 2.2 Files modified (12)

| Path | Change |
|---|---|
| `internal/dockerdrv/driver.go` | Extend interface with `ImagePull`, `Exec`, and (new) `RunOptions` (see §3.1). Keep existing `RunRequest` shape for service deploys. |
| `internal/dockerdrv/cli_driver.go` | Implement the three new methods. |
| `internal/dockerdrv/cli_driver_test.go` | Add argv-shape tests for new methods. |
| `internal/dockerdrv/mocks/mock_driver.go` | Regenerated by `go generate`. |
| `internal/caddy/reloader.go` | `cliReloader` now invokes `docker exec decloud-caddy caddy ...`. Translates host paths to container paths. Constructor takes the `Driver` so we don't reinvent the `docker exec` wheel. |
| `internal/caddy/reloader_test.go` | Update existing tests for new argv shape; add "container missing" failure-mode test. |
| `internal/cli/deploy_service.go` | `buildProductionLifecycle` and `buildProductionDeployer` pass the `Driver` into `caddy.NewCLIReloader(driver, paths)`. Add `caddyManagerFactory` test seam. |
| `internal/cli/root.go` | Mount `caddy up` and `caddy down` as children of the existing `caddy` parent command. |
| `internal/cli/exit_codes.go` | NO new constants. Reuse 40 and 60. (See §6.) |
| `_docs/install.md` | §3 rewritten; §5 cross-reference updated. |
| `_docs/usage.md` | §1 quick start gains a "before-first-deploy" `decloud caddy up` step; §6/7 updated for `docker exec`-based reload. |
| `tools.go` | No change. (`mockgen` already pinned.) |

### 2.3 Files NOT modified

- `internal/caddy/generator.go` and `internal/caddy/stub.go` — the rendered output is identical and correct under the new architecture. Don was right.
- `internal/deploy/service.go` — `regenerateAndReload` stays exactly as is. The reloader's behaviour change is invisible to the orchestrator.
- `internal/deploy/lifecycle.go` — same.
- `internal/cli/exit_codes.go` test — no new mappings to test.
- `internal/registry/*` — schema unchanged.
- `internal/ids` — `decloud-caddy` is a fixed singleton, NOT a service-derived name; lives in `internal/caddy` per §5 below.

---

## 3. New CLI / driver surfaces

### 3.1 Driver extension

I am NOT bolting fields onto `RunRequest`. Service deploys do not need `Ports`/`Volumes`/`Pull`; smearing them into `RunRequest` makes every deploy test grow defensive `assert.Empty(t, req.Volumes)`. Instead, three additions to the `Driver` interface:

```go
// internal/dockerdrv/driver.go (additions)

type RunOptions struct {
    Name        string
    Image       string
    Network     string
    Restart     string
    Ports       []PortMap     // host:container; empty means no -p flag
    Volumes     []VolumeMount // bind or named volume mounts
    Labels      map[string]string
    Env         map[string]string
}

type PortMap struct {
    HostPort      int    // 0 means "not set" — error at the driver if Ports is non-empty and any zero
    ContainerPort int
    Proto         string // "" defaults to tcp; we never need udp in M1
}

type VolumeMount struct {
    Source   string // host path for bind mounts, volume name for named volumes
    Target   string // path inside container
    ReadOnly bool
    IsNamed  bool   // true → named volume; false → bind mount; this disambiguates the docker syntax
}

type ExecOptions struct {
    Container string
    Cmd       []string
    Stdout    io.Writer
    Stderr    io.Writer
}

// New methods:
ImagePull(ctx context.Context, ref string) error
RunWithOptions(ctx context.Context, opts RunOptions) (containerID string, err error)
Exec(ctx context.Context, opts ExecOptions) error
```

**Why three new methods, not three new fields:**

- `RunWithOptions` is what the Caddy manager calls; service deploys keep using `Run(req RunRequest)`. The two share a private helper that builds the common `[]string` for `args` (the prefix that's identical: `run -d --name X --network Y --restart Z`), but their public surfaces stay narrow.
- `ImagePull` and `Exec` are genuinely new verbs the M1 driver doesn't expose. Adding them as `Driver` methods keeps the gomock-mockable seam consistent. (If we hide an `exec` call inside `Reloader`, we lose the ability to mock it cleanly in `internal/caddy/reloader_test.go` — we'd have to fall back to the `cmdFactory` recording trick, which is fine but inferior to a typed mock.)
- `Driver` already mocks every other docker verb. Asymmetry breeds bugs.

**Trade-off acknowledged:** This grows the `Driver` interface by three methods. The `_ai/explicit-inputs-not-globals.md` discipline says "don't invent infrastructure," but this is not invention — it's exposing primitives the production driver shells out to anyway. The mock surface grows by 3 methods; the test code grows by ~30 lines per method to assert argv. That's the right cost.

### 3.2 The exact `docker run` for Caddy

```
docker run -d \
  --name decloud-caddy \
  --network decloud \
  --restart unless-stopped \
  -p 80:80 \
  -p 443:443 \
  -p 443:443/udp \
  --label decloud.managed=caddy \
  -v /opt/decloud/config/caddy:/etc/caddy:ro \
  -v decloud_caddy_data:/data \
  -v decloud_caddy_config:/config \
  caddy:2
```

Notes on each flag:

- **`-p 443:443/udp`** — Caddy 2 supports HTTP/3 over QUIC. If we don't open UDP/443, HTTP/3 silently degrades, and operators see "TLS works but mobile is slow" reports we can't diagnose. Cost is one extra `PortMap` entry; benefit is real. Document in the decision record.
- **`-v /opt/decloud/config/caddy:/etc/caddy:ro`** — directory bind, read-only inside container. Caddy reads `/etc/caddy/Caddyfile`; the host writer (decloud's deployer) writes to `/opt/decloud/config/caddy/Caddyfile.tmp` and renames atomically. Bind is the same directory so the rename is visible. Read-only mount is correct because the **container** never writes the Caddyfile; only `decloud` running on the host does. (`docker exec ... caddy reload` does not write the file — it tells Caddy to re-read it.)
- **`-v decloud_caddy_data:/data`** — named volume for ACME certs and account state. Survives `down`/`up` cycles. Required for Let's Encrypt rate-limit safety.
- **`-v decloud_caddy_config:/config`** — Caddy's persisted runtime config (autosave). Required for `caddy reload` semantics across container restarts.
- **No `--network-alias`** — Caddy's container name `decloud-caddy` is its own DNS name on the bridge. Service containers don't need to resolve Caddy by name in M1.
- **`--label decloud.managed=caddy`** — diagnostic; mirrors the `decloud.service=<name>` pattern in `cli_driver.go:60`. Helpful for `docker ps --filter label=decloud.managed`.
- **`unless-stopped`** — survives daemon restart but respects an explicit `decloud caddy down`. Same restart policy services use.

### 3.3 The exact `docker exec` for reload/validate

```
docker exec decloud-caddy caddy validate --config /etc/caddy/Caddyfile.tmp
docker exec decloud-caddy caddy reload   --config /etc/caddy/Caddyfile
```

Path translation: the deployer writes to host path `/opt/decloud/config/caddy/Caddyfile.tmp` (= `Paths.CaddyfilePath + ".tmp"`). The reloader translates to the container path `/etc/caddy/Caddyfile.tmp` for the `--config` flag. Concretely:

```go
// internal/caddy/reloader.go (sketch)

const (
    caddyContainer       = "decloud-caddy"
    containerCaddyDir    = "/etc/caddy"
)

func (r *cliReloader) translatePath(hostPath string) (string, error) {
    if !strings.HasPrefix(hostPath, r.hostCaddyDir) {
        return "", fmt.Errorf("caddy reloader: path %q is outside the bind-mount %q", hostPath, r.hostCaddyDir)
    }
    rel, _ := filepath.Rel(r.hostCaddyDir, hostPath)
    return path.Join(containerCaddyDir, rel), nil
}
```

The reloader takes `r.hostCaddyDir` (`/opt/decloud/config/caddy`) at construction time so the translation is testable in isolation and the host path isn't hard-coded.

### 3.4 Cobra wiring

`internal/cli/root.go` after changes:

```go
caddy := &cobra.Command{Use: "caddy", Short: "Caddy management"}
caddy.AddCommand(newCaddyUpCmd(rc))
caddy.AddCommand(newCaddyDownCmd(rc))
caddy.AddCommand(newCaddyReloadCmd(rc))
root.AddCommand(caddy)
```

Three test seams in `internal/cli/deploy_service.go`:

```go
// caddyManagerFactory mirrors deployerFactory; same parallel-safety constraint.
var caddyManagerFactory = buildProductionCaddyManager

func buildProductionCaddyManager(paths config.Paths) (caddy.Manager, error) {
    return caddy.NewCLIManager(caddy.ManagerConfig{
        Driver: dockerdrv.NewCLIDriver(),
        Paths:  paths,
        Image:  resolveCaddyImage(paths), // viper read; default "caddy:2"
    }), nil
}
```

`buildProductionLifecycle` and `buildProductionDeployer` change to:

```go
driver := dockerdrv.NewCLIDriver()
paths  := paths
return deploy.NewLifecycle(deploy.Dependencies{
    Paths:     paths,
    Store:     registry.NewFSStore(paths),
    Capturer:  envcap.New(),
    Driver:    driver,
    Generator: caddy.NewGenerator(),
    Reloader:  caddy.NewCLIReloader(driver, paths.CaddyDir),
})
```

Note `caddy.NewCLIReloader` now takes two arguments. This is a **constructor signature change**; every call site needs to be updated. There are two production call sites (`buildProductionDeployer`, `buildProductionLifecycle`) and zero test call sites (tests use the `cmdFactory` form via `newCLIReloaderWithFactory` — that constructor stays internal).

---

## 4. Behaviour spec for new/changed functions

### 4.1 `caddy.Manager` interface

```go
// internal/caddy/manager.go

//go:generate mockgen -source=manager.go -destination=mocks/mock_manager.go -package=mocks

type Manager interface {
    // Up ensures the decloud network exists, the Caddyfile stub is on disk,
    // and the decloud-caddy container is running. Idempotent.
    Up(ctx context.Context) error

    // Down stops and removes the decloud-caddy container. Volumes are NOT
    // removed. Idempotent.
    Down(ctx context.Context) error

    // IsRunning reports whether the decloud-caddy container is in the
    // "running" state.
    IsRunning(ctx context.Context) (bool, error)
}

type ManagerConfig struct {
    Driver dockerdrv.Driver
    Paths  config.Paths
    Image  string // e.g. "caddy:2"
    Stdout io.Writer
}

func NewCLIManager(cfg ManagerConfig) Manager { ... }
```

Sentinels (mirrors `deploy.ErrCaddyReload` shape):

```go
var (
    ErrCaddyUp   = errors.New("caddy: up failed")
    ErrCaddyDown = errors.New("caddy: down failed")
)
```

### 4.2 `Manager.Up(ctx)` — exact algorithm

```
1. m.Driver.NetworkEnsure(ctx, "decloud")
   → wrap as fmt.Errorf("%w: network ensure: %w", ErrCaddyUp, err)
2. caddy.WriteStubIfMissing(m.Paths.CaddyfilePath)
   → wrap as fmt.Errorf("%w: stub write: %w", ErrCaddyUp, err)
3. inspect, err := m.Driver.Inspect(ctx, "decloud-caddy")
   → on err, wrap as fmt.Errorf("%w: inspect: %w", ErrCaddyUp, err)
4. switch inspect.State {
   case "running": fmt.Fprintln(m.Stdout, "caddy already running"); return nil
   case "exited":  m.Driver.Start(ctx, "decloud-caddy") + log "caddy started"
                   wrap as fmt.Errorf("%w: start: %w", ErrCaddyUp, err)
   case "absent":  go to step 5
   default: wrap unexpected state as %w: ErrCaddyUp + return
   }
5. m.Driver.ImagePull(ctx, m.Image)
   → wrap as fmt.Errorf("%w: image pull: %w", ErrCaddyUp, err)
   → log "caddy up: pulled <image>"
6. m.Driver.RunWithOptions(ctx, RunOptions{...})  // shape per §3.2
   → wrap as fmt.Errorf("%w: docker run: %w", ErrCaddyUp, err)
   → log "caddy up: container decloud-caddy running on network decloud"
7. return nil
```

The state machine is intentionally simple — no readiness probe of Caddy itself. Caddy's container reaching `running` does not prove the admin API is up, but the next thing the operator does is either nothing (in which case Caddy starts fine) or `decloud deploy service`, which will trigger a `caddy reload` via `docker exec`. If the admin API isn't up yet, that exec will fail with a clear error; we catch it there. Don't add a probe with retries here unless an actual failure mode demands it.

### 4.3 `Manager.Down(ctx)` — exact algorithm

```
1. m.Driver.Stop(ctx, "decloud-caddy", 10*time.Second)
   → if errors.Is(err, ErrContainerNotFound): proceed
   → else: wrap as fmt.Errorf("%w: stop: %w", ErrCaddyDown, err)
2. m.Driver.Remove(ctx, "decloud-caddy")
   → if errors.Is(err, ErrContainerNotFound): proceed
   → else: wrap as fmt.Errorf("%w: remove: %w", ErrCaddyDown, err)
3. log "caddy down: container removed (volumes retained)"
4. return nil
```

### 4.4 Updated `Reloader.Validate` / `Reload`

```go
// internal/caddy/reloader.go

type cliReloader struct {
    driver       dockerdrv.Driver  // NEW: was cmdFactory
    hostCaddyDir string            // NEW: needed for path translation
    cmd          cmdFactory        // KEPT: only for tests via newCLIReloaderWithFactory
}

func NewCLIReloader(driver dockerdrv.Driver, hostCaddyDir string) Reloader {
    return &cliReloader{driver: driver, hostCaddyDir: hostCaddyDir}
}

func (r *cliReloader) Validate(ctx context.Context, configPath string) error {
    ctrPath, err := r.translatePath(configPath)
    if err != nil {
        return err  // already prefixed
    }
    return r.execCaddy(ctx, "validate", ctrPath)
}

func (r *cliReloader) Reload(ctx context.Context, configPath string) error {
    ctrPath, err := r.translatePath(configPath)
    if err != nil {
        return err
    }
    return r.execCaddy(ctx, "reload", ctrPath)
}

func (r *cliReloader) execCaddy(ctx context.Context, sub, containerPath string) error {
    var stderr bytes.Buffer
    err := r.driver.Exec(ctx, dockerdrv.ExecOptions{
        Container: caddyContainer,
        Cmd:       []string{"caddy", sub, "--config", containerPath},
        Stderr:    &stderr,
    })
    if err == nil {
        return nil
    }
    // Surface "container not running" as the operator-actionable error.
    if isCaddyContainerMissing(stderr.String(), err) {
        return fmt.Errorf("caddy %s: container %q is not running; run 'decloud caddy up' first",
            sub, caddyContainer)
    }
    return fmt.Errorf("caddy %s: %w; stderr=%q", sub, err, stderr.String())
}
```

`isCaddyContainerMissing` looks for either the wrapped `ErrContainerNotFound` (returned by `Driver.Exec` when `docker exec` hits "no such container") OR the substring `is not running` in stderr (when the container exists but is exited). The OR is deliberate — `docker exec` against an exited container does not return "no such container" and we still want the friendly message.

**Important contract:** the host path passed to `Validate` and `Reload` MUST be inside `hostCaddyDir`. The orchestrator at `service.go:310-318` already complies (writes `Paths.CaddyfilePath + ".tmp"`, then renames to `Paths.CaddyfilePath`, both under `Paths.CaddyDir`). Document this contract in a doc-comment on `Reloader.Validate` so a future caller doesn't pass a `/tmp` path.

### 4.5 New `Driver.Exec` argv

```
docker exec decloud-caddy caddy validate --config /etc/caddy/Caddyfile.tmp
```

Build:

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

No `-i`, no `-t`. Caddy's `validate`/`reload` are non-interactive.

### 4.6 New `Driver.ImagePull` argv

```
docker pull caddy:2
```

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

### 4.7 New `Driver.RunWithOptions` argv (Caddy case)

```
docker run -d \
  --name decloud-caddy \
  --network decloud \
  --restart unless-stopped \
  --label decloud.managed=caddy \
  -p 80:80/tcp \
  -p 443:443/tcp \
  -p 443:443/udp \
  -v /opt/decloud/config/caddy:/etc/caddy:ro \
  -v decloud_caddy_data:/data \
  -v decloud_caddy_config:/config \
  caddy:2
```

Argv build order is locked: subcommand → name → network → restart → labels (sorted) → ports (declared order) → volumes (declared order) → image. Sorted labels and env keys mirror `cli_driver.go:52-58` so tests are deterministic. Volumes are NOT sorted — they're declared in `RunOptions.Volumes` and the order matters for `:ro` vs `:rw` overlap (Caddy doesn't have any, but the contract should be predictable).

---

## 5. Constants and naming

Three new exported names go in `internal/caddy/manager.go`:

```go
const (
    ContainerName = "decloud-caddy"
    NetworkName   = "decloud"
    DefaultImage  = "caddy:2"
)
```

`ContainerName` is the singleton-Caddy fixture name. It does **not** belong in `internal/ids` because:

- `ids.ContainerName(name)` derives `"decloud-"+name` from a service name. Caddy is not a service, has no service registration, has no `--name` flag at the CLI.
- Putting it in `ids` muddies the package's purpose ("identifiers derived from a service name") and forces every caller of `ids.ContainerName` to wonder which kind they want.
- Caddy is a Decloud-internal concern of the `caddy` package. The constant lives where the management code lives.

`NetworkName` deduplicates the `"decloud"` literal currently scattered across `internal/deploy/service.go:131`, `:190`, `:289`, `internal/dockerdrv/cli_driver.go:170`, and the new manager. We don't have to migrate all six call sites in this task — that's M1.x backlog material. Just don't introduce a seventh `"decloud"` literal in this task; reference the constant from new code.

`DefaultImage` is what `caddy up` uses if Viper resolves no override.

---

## 6. Exit-code mapping

Don suggested image-pull → 40, container-already-running → 0, network-ensure → 40, file-permission → 70. I agree with all of those and add the explicit mapping in `internal/cli/exit_codes.go`:

```go
case errors.Is(err, caddy.ErrCaddyUp), errors.Is(err, caddy.ErrCaddyDown):
    return ExitRunFail  // 40
```

Why 40 and not 60: 60 is `ExitCaddyReloadFail`, which is a **deploy-time** failure (operator deployed, validate/reload broke). `caddy up`/`caddy down` failures are management-plane failures; they live in the same conceptual bucket as `docker run` / `docker stop` failures, which are 40. If we ever ship `decloud doctor` we'll add a separate code; not now.

`caddy reload` failures continue to map to 60 via the existing `deploy.ErrCaddyReload` chain. The new "container not running" message is wrapped via `deploy.ErrCaddyReload` at `service.go:321-322` — no change there, just verify the wrap chain stays intact in tests (which it does, because the inner error from the reloader is wrapped in `regenerateAndReload`).

No new constants. The existing five (10/20/30/40/50/60/70) cover everything.

---

## 7. Migration story (operator-facing)

The user already has Caddy running as a host systemd unit per M1.0. Migration:

```sh
# 1. Stop and remove the host Caddy.
systemctl disable --now caddy
apt-get remove -y caddy   # or your distro's equivalent

# 2. Rebuild and install the new decloud binary (per _docs/install.md §6).

# 3. Bring up the containerised Caddy.
decloud caddy up
#   → ensures the `decloud` network exists
#   → writes the Caddyfile stub if missing
#   → pulls caddy:2
#   → starts decloud-caddy on the decloud network with 80/443 published

# 4. Trigger a reload to repopulate the Caddyfile from the registry.
decloud caddy reload

# 5. Verify.
docker network inspect decloud
#   → must show BOTH decloud-caddy and the service container as members
docker exec decloud-caddy nslookup decloud-<service>
#   → must return the bridge IPv4 (e.g. 172.18.0.2), NOT the host's public IPv6
curl -v https://<host>/healthz
#   → 200, valid TLS
```

ACME state caveat: the operator's previous Caddy ran as the `caddy` user with state at `/var/lib/caddy/`. We are NOT migrating that state into the new `decloud_caddy_data` volume. The new container will re-issue certificates on first request. This is a **minor downside** of the migration — first request per hostname will see a brief delay while Caddy obtains a fresh cert.

If the operator has hit Let's Encrypt rate limits or just wants to preserve state, advanced path:

```sh
docker volume create decloud_caddy_data
docker run --rm \
  -v /var/lib/caddy/.local/share/caddy:/from \
  -v decloud_caddy_data:/to \
  alpine sh -c 'cp -a /from/. /to/'
decloud caddy up
```

This is for the decision record (`_ai/decisions/caddy-runs-in-container.md`), NOT the install doc — too operator-specific. Install doc just says "first deploy will take an extra second to issue a fresh cert."

For fresh installs (no prior M1.0 host Caddy), the migration steps collapse to: `decloud caddy up` after the binary is in place. The install doc rewrite removes §3's systemd block entirely.

---

## 8. Test strategy

Per CLAUDE.md: Testify, Gomock, no change-detector tests. Tests live next to code. No integration tests in this task (see §13).

### 8.1 `internal/caddy/manager_test.go` (new)

Use `gomock.InOrder` for sequencing per `_ai/gomock-inorder-sequencing.md`. Mock `dockerdrv.Driver`. Drive a real `cliManager`; assert through the mock recorder.

| Test | What it asserts |
|---|---|
| `TestManager_UpFreshInstall` | `NetworkEnsure` → `Inspect`(absent) → `ImagePull` → `RunWithOptions` with **exact** `RunOptions` shape (name, network, restart, ports including 443/udp, three volume mounts, label). |
| `TestManager_UpAlreadyRunning` | `NetworkEnsure` → `Inspect`(running). No `ImagePull`, no `Run`, no `Start`. Stdout contains `caddy already running`. |
| `TestManager_UpAfterPriorStop` | `NetworkEnsure` → `Inspect`(exited) → `Start`. No `ImagePull`, no `Run`. |
| `TestManager_UpNetworkEnsureFails` | Driver returns sentinel; manager wraps as `ErrCaddyUp`. `errors.Is(err, ErrCaddyUp)` AND `errors.Is(err, theSentinel)` both true (locks `%w: %w` discipline). |
| `TestManager_UpImagePullFails` | Same shape for the pull leg. |
| `TestManager_UpStubWriteFailsContinuesGracefully` | Pre-create a file at `Paths.CaddyfilePath` with bad mode; assert `Up` either succeeds (stub-if-missing semantics) or fails with `ErrCaddyUp` wrapping the FS error. |
| `TestManager_DownHappyPath` | `Stop` → `Remove`. Inspect order. |
| `TestManager_DownContainerAbsent` | `Stop` returns `ErrContainerNotFound` → `Remove` returns `ErrContainerNotFound` → `Down` returns nil. |
| `TestManager_DownStopFailsHard` | Non-`ErrContainerNotFound` error from `Stop` → wrapped `ErrCaddyDown`, no `Remove` call. |
| `TestManager_IsRunningTrueFalseAbsent` | Three sub-cases: `running` → true; `exited` → false; `absent` → false. |

### 8.2 `internal/caddy/reloader_test.go` (updated)

Change the recording-factory tests to drive a `MockDriver` directly. The `cmdFactory` test seam stays as a fallback for path-translation isolation, but the primary surface is now the `Driver.Exec` mock.

| Test | What it asserts |
|---|---|
| `TestReloader_ValidateCallsDockerExec` | `Driver.Exec` called once with `Container=decloud-caddy`, `Cmd=["caddy","validate","--config","/etc/caddy/Caddyfile.tmp"]`. |
| `TestReloader_ReloadCallsDockerExec` | Same shape for `caddy reload --config /etc/caddy/Caddyfile`. |
| `TestReloader_PathTranslationOutsideBindMount` | Pass `/tmp/foo`; assert error mentions "outside the bind-mount". No exec call. |
| `TestReloader_ContainerNotRunningSurfacesActionableError` | Mock `Driver.Exec` returns `ErrContainerNotFound`; assert err string matches `container "decloud-caddy" is not running; run 'decloud caddy up' first`. |
| `TestReloader_ValidateExitNonzeroPreservesStderr` | Inner exec error survives the wrap (locks `%w: %w` discipline; same regression-test-shape as `TestDeploy_BuildErrorPreservesInnerSentinel`). |

Drop or rewrite the existing `TestReloader_InvokesCaddyValidate`/`TestReloader_InvokesCaddyReload` — they assert host-`caddy` argv shape that no longer exists. Their replacements are above.

### 8.3 `internal/dockerdrv/cli_driver_test.go` (additions)

Each new test pairs argv with the hand-typed `docker` equivalent in a comment per the file's existing convention.

| Test | Argv asserted |
|---|---|
| `TestCLIDriver_ImagePullArgs` | `pull caddy:2` |
| `TestCLIDriver_ExecArgsBasic` | `exec decloud-caddy caddy validate --config /etc/caddy/Caddyfile.tmp` |
| `TestCLIDriver_ExecPropagatesContainerNotFound` | `Exec` against absent container → `ErrContainerNotFound` |
| `TestCLIDriver_RunWithOptionsCaddyShape` | Full Caddy `docker run` argv per §4.7. |
| `TestCLIDriver_RunWithOptionsPortMapsTcpUdp` | Both `-p 443:443/tcp` and `-p 443:443/udp` appear. |
| `TestCLIDriver_RunWithOptionsBindReadOnly` | Bind volume with `ReadOnly=true` produces `:ro` suffix. |
| `TestCLIDriver_RunWithOptionsNamedVolume` | Named volume produces `name:target` (no `:` source path heuristic). |

### 8.4 `internal/cli/caddy_up_test.go` and `caddy_down_test.go` (new)

Use the `caddyManagerFactory` test seam, identical pattern to `installMockDeployer` in `deploy_service_test.go`.

| Test file | Test | Assertion |
|---|---|---|
| `caddy_up_test.go` | `TestCaddyUp_DelegatesToManager` | Mock `Manager.Up` returns nil; `decloud caddy up` returns nil and exit 0. |
| `caddy_up_test.go` | `TestCaddyUp_ManagerErrorReturnsExitRunFail` | Mock returns `ErrCaddyUp`-wrapped error; `ExitCodeFor` returns 40. |
| `caddy_up_test.go` | `TestCaddyUp_PassesImageOverrideFromFlag` | `--image caddy:2.7.6` propagates to manager constructor. |
| `caddy_down_test.go` | `TestCaddyDown_DelegatesToManager` | Mirror. |
| `caddy_down_test.go` | `TestCaddyDown_ManagerErrorReturnsExitRunFail` | Mirror. |

### 8.5 `internal/deploy/service_test.go` (no behaviour change, but verify)

`Reloader` is a mocked interface in deploy tests. The constructor change at the production wiring layer does not reach the deploy tests. Run `go test ./internal/deploy/...` and confirm zero diffs needed. (If a test asserts on `caddy.NewCLIReloader`, the test is over-specified and we cut it.)

### 8.6 Tests we are explicitly NOT adding

- Generator tests do not change.
- Real-Docker integration tests (deferred per §13).
- Caddy's own behaviour (cert provisioning, HTTP/3, ALPN). That's Caddy's test suite, not ours.
- Tests for `_docs/*.md` content (Raymond owns; Kevlin reviews).

---

## 9. Order of operations for the implementation

Kent (test) and Rob (implementation) handoff. Each step is one Bureau report. Don't merge across steps.

**Phase 1 — Driver primitives (foundation; nothing else compiles without these):**

- Kent: `internal/dockerdrv/cli_driver_test.go` argv tests for `ImagePull`, `Exec`, `RunWithOptions`. Tests fail (missing methods).
- Rob: implement the three methods in `cli_driver.go`; extend `driver.go` interface; regenerate `mocks/mock_driver.go`. Tests pass.

**Phase 2 — Caddy manager:**

- Kent: `internal/caddy/manager_test.go` per §8.1.
- Rob: `internal/caddy/manager.go` per §4.1-4.3; `go generate` for `mocks/mock_manager.go`.

**Phase 3 — Reloader rewire:**

- Kent: rewrite `internal/caddy/reloader_test.go` per §8.2 (NB: this deletes existing tests, which is correct — they assert against an obsolete contract).
- Rob: rewrite `internal/caddy/reloader.go` per §4.4. Update `buildProductionDeployer`/`buildProductionLifecycle` in `internal/cli/deploy_service.go` to pass the driver and host caddy dir.

**Phase 4 — CLI surface:**

- Kent: `internal/cli/caddy_up_test.go`, `caddy_down_test.go` per §8.4. Add `caddyManagerFactory` test seam.
- Rob: `internal/cli/caddy_up.go`, `caddy_down.go` per §1.2-1.3; wire into `internal/cli/root.go`.

**Phase 5 — Exit-code wiring:**

- Kent: extend `internal/cli/exit_codes_test.go` with `caddy.ErrCaddyUp` → 40 and `caddy.ErrCaddyDown` → 40.
- Rob: extend `ExitCodeFor` in `internal/cli/exit_codes.go` per §6.

**Phase 6 — Docs (Raymond):**

- Rewrite `_docs/install.md` §3 and §5 per §1.1-1.2 and §7.
- Update `_docs/usage.md` §1, §6, §7.
- Add `_ai/decisions/caddy-runs-in-container.md`.
- Append M1.x backlog item #6 (deferred integration test) to `_ai/m1x-backlog.md`.

**Phase 7 — Verification gate:**

- `gofmt -l .` (must be empty)
- `go vet ./...` (must be empty)
- `go generate ./...` followed by `git status --porcelain` (must be empty)
- `go test ./... -count=1 -v` (must be all green)
- Manual verification per Don's `002-don-plan.md` §5.

Dependencies:
- Phase 2 depends on Phase 1 (`Manager` calls `Driver.RunWithOptions`/`ImagePull`).
- Phase 3 depends on Phase 1 (`Reloader` calls `Driver.Exec`).
- Phase 4 depends on Phase 2 (CLI wires the manager).
- Phase 5 depends on Phase 2 (sentinels live in `caddy`).
- Phase 6 depends on phases 1-5 (docs reflect shipped behaviour).
- Phase 7 depends on everything.

Phases 2 and 3 are parallelisable if Rob has bandwidth. They both depend on Phase 1 only.

---

## 10. Doc-update checklist (for Raymond)

Don't write the docs yourself; Raymond owns rewriting. This is Raymond's task list.

### 10.1 `_docs/install.md`

- **§3 ("Install Caddy")** — replace the systemd block with: "Decloud manages Caddy as a container. After installing the binary (§6), run `decloud caddy up`. There is no host `caddy` package to install."
- **§5 ("Create the shared Docker network")** — keep, but note that `decloud caddy up` will create it if missing; the explicit step is for operators who want to inspect the network before bringing Caddy up.
- Add new §3.1 "Migrating from M1.0": the steps in §7 above.
- Add new §3.2 "Customising the Caddy image": brief mention of `--image` flag and the `caddy.image` TOML key.
- §1 "Prerequisites" — replace "Caddy install" with "DNS records pointing at the host"; the latter is the only true prereq now.

### 10.2 `_docs/usage.md`

- **§1 ("Quick start")** — prepend a step: "If you have not yet, run `decloud caddy up` once to bring Caddy online."
- **§4 lifecycle commands** — add `decloud caddy up` and `decloud caddy down` to the bullet list with one-line semantics each.
- **§6 ("Debugging a container directly")** — update the paragraph that mentions Caddy reaching upstreams "over the shared `decloud` network" to make explicit Caddy ITSELF runs on that network as the `decloud-caddy` container.
- **§7 ("Recovering from caddy reload failures")** — replace the "read journalctl -u caddy" step with "read `docker logs decloud-caddy`". Replace the "fix the underlying issue and run `decloud caddy reload`" with the same plus, "if the container is not running, run `decloud caddy up` first."

### 10.3 `_ai/decisions/caddy-runs-in-container.md` (new)

Topics to cover:
1. Decision: Caddy runs as `decloud-caddy` on the `decloud` Docker network, managed by `decloud caddy up/down`.
2. Why: Caddyfile uses container names; container names resolve only via embedded Docker DNS; embedded Docker DNS is only available to network members.
3. Rejected alternatives: Don's Candidates A and B, with one-paragraph rejection rationales each.
4. Volume strategy: `decloud_caddy_data:/data` (ACME) and `decloud_caddy_config:/config` (autosave). Removed only by manual `docker volume rm`.
5. HTTP/3: UDP/443 is published; document the rationale.
6. Migration from M1.0 host Caddy: §7 above.
7. Forward-looking note: when M4 introduces blue/green, Caddy admin API access from the `decloud` host process becomes a topic — does the host process talk to `decloud-caddy` over `localhost:2019` (admin API published) or via `docker exec`? Flag for M4 tech plan.

### 10.4 `_ai/MEMORY.md`

Add one line under "Architecture decisions" pointing at the new decision file. Keep concise.

### 10.5 `_ai/m1x-backlog.md`

Append item #6: "Real-Docker integration test for the first happy-path deploy (per Don §6.4 of `_tasks/2026-04-27-caddy-container-connection-refused/002-don-plan.md`)." Why deferred: §13 below.

### 10.6 `_ai/cli-flag-surface-coherence.md`

Add `--image` to the canonical flag-surface list. Three surfaces (runtime check, error text, --help text) now four (TOML key `caddy.image`).

---

## 11. Gotchas and landmines

### 11.1 Bind-mount-source must exist before container start

`docker run` will create a missing host bind-mount source as a directory if it doesn't exist, but with permissions inherited from the daemon (root:root, 0755). This is fine for `/opt/decloud/config/caddy` because the Decloud install creates it explicitly per `_docs/install.md` §4. If an operator skips that step and runs `decloud caddy up`, Docker creates the directory, then `caddy.WriteStubIfMissing` writes the Caddyfile, then Caddy starts and reads it. Works.

But: on SELinux-enabled hosts (RHEL family), bind-mounting `/opt/decloud/config/caddy` will fail with permission denied unless the operator either passes `:Z` on the volume mount or sets the file context. We are NOT supporting SELinux in M1; document as a known limitation in the install doc. Linus, your call on whether to gate this with a `--selinux` flag now or M2.

### 11.2 Atomic rename across bind mount

The deployer writes `<dir>/Caddyfile.tmp`, validates it, renames to `<dir>/Caddyfile`. Both inside `Paths.CaddyDir`. The bind mount is the directory itself, not a single file, so the rename is visible inside the container immediately after the host's `os.Rename`. Confirmed by reading the existing `service.go:310-318` flow. **Single-file binds would break this**; do NOT pivot to `-v <host>/Caddyfile:/etc/caddy/Caddyfile`.

### 11.3 `docker exec` against a not-yet-ready container

Just because `docker inspect` reports `running` does NOT mean the entrypoint has progressed past its startup phase. `caddy:2` starts fast (sub-second), but a slow disk on first run could open a window where `caddy reload` exec'd milliseconds after `docker run` succeeds gets `connect: connection refused` from the admin API. This window is small and only hits the **first** deploy after `caddy up`. Two options:

1. Tolerate it and document: "if first deploy fails with `caddy reload`, wait 1s and retry." (Cheap; cynical; works.)
2. Add a lazy retry loop inside the manager's `Up`: after `Run`, poll `docker exec decloud-caddy caddy version` until success or timeout (5s).

I propose option 2 in the manager's `Up`, with the concrete shape: 5 attempts at 200ms intervals, total 1s. Wrap a final failure as `ErrCaddyUp`. This adds ~10 lines and one test. The cost is small; the operator-experience benefit on first deploy is real. Linus, push back if you disagree.

### 11.4 Stale `decloud-caddy` container after a partial `up` failure

If `RunWithOptions` succeeds but a follow-up step fails (e.g., the §11.3 readiness retry), we have a running container in an inconsistent state. The next `decloud caddy up` will see it as `running` and short-circuit, masking the prior failure. Mitigation: on `Up`-time failure AFTER `Run` succeeded, the manager attempts `Stop`+`Remove` for `decloud-caddy` before returning the error. Mirror the rollback shape in `service.go:265-269`. One test: `TestManager_UpRollsBackOnPostRunFailure`.

### 11.5 Concurrent deploys racing the reloader

`docker exec decloud-caddy caddy reload` is not atomic with the file rename. If two deploys race, both write `.tmp`, both validate, both rename — the second rename wins, then both reloads happen. Caddy handles back-to-back reloads correctly (the second one supersedes the first), and the resulting state is whatever the second deploy wrote. **No issue in single-operator M1**, but flag as M2+ when multi-operator becomes plausible. Document in the decision record.

### 11.6 `docker exec` fan-out from the Decloud host

The new wiring assumes the operator's host has `docker` on PATH. Same assumption as today. Not new.

### 11.7 The image tag pinning trap

Defaulting to `caddy:2` floats. A surprise minor version bump on `docker pull` could change behaviour. Two trade-offs:

- Float `caddy:2` (chosen) → operators get bug fixes; small risk of breakage.
- Pin `caddy:2.7.6` → reproducible; operators must opt in to upgrades.

Default float, document the override. Operators who want pinning set `caddy.image = "caddy:2.7.6"` in `decloud.toml`.

### 11.8 `caddy reload` requires the admin API on `localhost:2019` inside the container

Caddy 2's `caddy reload` subcommand connects to its own admin API. The default address is `localhost:2019`, which is fine because we exec INSIDE the container — `localhost` is the container, not the host. The admin API is NOT published to the host (no `-p 2019:2019`). This is correct — exposing the admin API to the host is a security regression and we have no use for host-side admin access in M1. Lock in with a doc-comment.

### 11.9 Path translation case sensitivity

`filepath.Rel` on Linux is case-sensitive; the bind mount is case-sensitive on Linux ext4. macOS dev boxes with case-insensitive HFS could test-pass with a wrong-case host path that production-rejects. Mitigation: in the test for path translation, include a case-mismatched negative case to lock the contract.

### 11.10 Non-ASCII paths in `--config-root`

Per "There Ain't No Such Thing as Plain Text," operators with non-ASCII config-root paths could trip Docker's bind-mount handling. Docker handles UTF-8 paths fine on modern engines, but the literal we splice into `-v <src>:<dst>` flows through `os/exec` and inherits the host's locale. We don't need to worry about this in M1, but if an issue appears, the fix is to NFC-normalise the path before splicing. Backlog material.

---

## 12. Open questions for Linus

1. **§11.3 readiness loop in `Up`:** add the 1-second poll-for-admin-API loop, or punt and document the cold-start retry need? My vote: add it, ~10 lines, real UX benefit.

2. **§11.1 SELinux:** flag now (one-line warning in install doc) or backlog item? My vote: warning now.

3. **§3.1 Driver extension shape:** three new methods on `Driver` (`ImagePull`, `Exec`, `RunWithOptions`) versus extending `RunRequest`. I argued for the three methods. Push back if the symmetry argument wins for you.

4. **`decloud-caddy` constant location:** I put it in `internal/caddy/manager.go` (`caddy.ContainerName`). Don suggested the same. Confirm; otherwise where?

5. **Don's §6.4 integration test:** I am rejecting it for this task per §13 below. Confirm or override.

6. **Image float vs pin (§11.7):** float `caddy:2` by default with a TOML override, or pin a specific minor version in code? My vote: float.

7. **The `--restart unless-stopped` policy on Caddy:** unconditionally apply, or expose a flag? My vote: unconditional. Operators who don't want auto-restart can `docker update --restart=no decloud-caddy` themselves; we shouldn't proliferate flags for one user's edge case.

8. **`decloud caddy up` writing the Caddyfile stub** (step 2 of §4.2): or punt to deploy-time as today? My vote: write the stub, because Caddy needs *some* config file to start. Without it, the container starts and immediately exits; the operator's `caddy up` reports "running" then `Inspect` reports "exited" on the next call. Confusing.

---

## 13. Why I am rejecting Don's §6.4 integration test for this task

Don asked me to weigh in. I have, and I'm pushing back. Reasons:

1. **Scope creep.** This task is a recovery from a shipped bug. The user is blocked. Adding `-tags integration` plumbing is a separate, larger commitment that hauls in CI runner provisioning, Docker-in-Docker considerations, and a per-package integration-test convention we don't have.

2. **`_ai/decisions/m1-test-strategy.md` was a real decision.** That doc says: "the manual smoke-test the maintainer will run on a real Linux host once the binary lands. That smoke-test is M2's first feedback signal, not an M1 deliverable." The bug Don is responding to is **exactly** the kind of bug that doc anticipated — an architectural gap unit tests can't catch. The doc's response to "what about bugs unit tests can't catch?" was "the maintainer's first real-system run is the bridge." That's what just happened. Honouring the strategy means fixing the bug, NOT preemptively building the integration-test infrastructure that would have caught it.

3. **The fix is unit-testable end to end.** Argv-shape tests on `RunWithOptions` lock the `docker run` command we ship. Argv-shape tests on `Exec` lock the `docker exec` we ship. Manager tests lock the orchestration. CLI tests lock the wiring. The only thing unit tests can't lock is "does Docker actually do what its CLI says it does," and that's true of every command in the codebase already.

4. **Manual verification is in `002-don-plan.md` §5.** That's the gate. We're not skipping verification; we're skipping the test infrastructure that would automate it.

5. **The right place for the integration-test discussion is M2.** When the team starts M2, the FIRST tech plan should propose bringing back integration tests with a concrete target (e.g., "happy-path deploy + curl") and a runner story. That's a deliberate decision, not a "while we're at it" sneak-in here.

I am adding it as M1.x backlog item #6 so it is explicitly tracked. Linus, if you disagree, say so and I'll add it to this task's scope — but I want the disagreement on the record so future-Don can audit.

---

## 14. Acceptance criteria — bridge to Don's §8

Don's 11 acceptance criteria are correct and unchanged. I add three:

12. `internal/dockerdrv/driver.go` interface has exactly three new methods: `ImagePull`, `Exec`, `RunWithOptions`. Existing methods are byte-identical.
13. `internal/caddy/reloader.go` constructor signature is `NewCLIReloader(driver dockerdrv.Driver, hostCaddyDir string) Reloader`. Both production call sites (`buildProductionDeployer`, `buildProductionLifecycle`) updated.
14. No new exit-code constants in `internal/cli/exit_codes.go`. The `ErrCaddyUp`/`ErrCaddyDown` mappings reuse `ExitRunFail` (40) per §6.

If any of 1-14 is missing at sign-off, the task is not done.

---

## 15. Estimation reality check

Don gave no estimate. Mine:

- Phase 1 (driver primitives): 4 hours including tests.
- Phase 2 (manager): 4 hours.
- Phase 3 (reloader rewire): 3 hours.
- Phase 4 (CLI surface): 2 hours.
- Phase 5 (exit codes): 30 minutes.
- Phase 6 (docs): 3 hours (Raymond).
- Phase 7 (verification + manual): 2 hours.

Subtotal: ~18 hours. Joel's-π adjustment: ~57 hours wall-clock for a single human, accounting for the inevitable "while we're at it" requests, the "wait, what if SELinux" detour, and the last-10%-takes-90% rule. For an agentic team running concurrently, more like 2-3 review cycles' worth of Bureau roundtrips.

The number that matters: this is shippable in a single-day push if Linus approves the plan as-is and no major rework lands in Phase 7.

— Joel
