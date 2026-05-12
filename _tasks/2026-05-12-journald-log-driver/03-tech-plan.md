# Joel's tech plan — journald log driver

Audience: Kent (writes the failing tests next) and Rob (implements after).
Everything in this doc is anchored to a `file:line` cite read this morning
at HEAD `fb4d026` of `task/journald-log-driver`. If a line number drifts
between now and Kent's PR, the cite is wrong by definition — fix the cite,
not the cite's referent.

This expands Don's plan
(`_tasks/2026-05-12-journald-log-driver/02-plan.md`). I refined three
points; they are listed in §11 "Where I refined Don's plan" so Don and
Linus can audit them on the next planning loop.

> **REVISION 2 (post Linus review at `04-linus-plan-review.md`,
> commit `8e2ee81`; Don's updated plan at commit `e453e43`).** Three
> changes folded into this doc in place. Sections changed:
>
> - §1.4 — acceptance criterion 6 split into empty-Service and
>   slash-Service rejection; added new criterion 7 for
>   `docker start` argv negative invariant.
> - §2.1 — guard shape expanded to include slash-rejection;
>   `ErrInvalidService` sentinel added.
> - §3.3 — sentinel-error pattern: `ErrInvalidService` joins
>   `ErrEmptyService` in the `var (…)` block.
> - §5.1 — driver.go change set: two sentinels not one.
> - §5.2 — `cli_driver.go` guard updated for both `Run` and
>   `RunWithOptions`.
> - §6.2 — new tests: §6.2.3 and §6.2.4 (slash-rejection x2) added;
>   tag-literal test renumbered to §6.2.5; §6.2.6 (caddy tag literal)
>   promoted from optional to REQUIRED. §6.2.7 new: extension to
>   `TestCLIDriver_StartArgs` with `assert.NotContains(…, "--log-driver")`.
> - §10 — gotcha §10.9 added: slash-in-Service is now a driver invariant.
> - §11.1 — guard shape now includes slash-rejection.
> - §11.7 — NEW: rationale for splitting empty vs invalid into two
>   sentinels (callers can `errors.Is`-discriminate).
> - §13 — file list updated; test list reflects one additional new
>   test pair (slash-rejection x2) plus the `StartArgs` extension.
>
> Anything not marked above carries over unchanged from REVISION 1.

---

## 1. Functional spec (user-visible behaviour)

### 1.1 What the user sees

After this change ships, every container started by `decloud` runs under
docker's `journald` log driver with a tag `decloud/<service>`.

- `decloud deploy service --name foo …` → container `decloud-foo` runs with
  `--log-driver=journald --log-opt tag=decloud/foo`.
- `decloud start foo` from the absent state (lifecycle `Start` re-run) →
  identical flags as above; the tag survives because it's set at
  `docker run` time.
- `decloud start foo` from the exited state → no new flags (we go through
  `docker start`, which preserves the existing `HostConfig.LogConfig`).
- `decloud restart foo` → `Stop` then `Start`; same as above by composition.
- Rollback path inside `Deploy` (`restoreOldContainer` after the new
  container fails) → previous container re-`run` with the same journald
  flags, tag derived from `prev.Config.Name`.
- `decloud caddy up` (first install or after `down`) → `decloud-caddy`
  container runs with `--log-driver=journald --log-opt tag=decloud/caddy`.
- `decloud logs <name>` and `decloud logs <name> -f --tail 50` behave
  identically to before. The journald driver is one of the few that still
  supports `docker logs`; no API change.
- `journalctl CONTAINER_TAG=decloud/foo` on the host shows logs from every
  container instance ever named `decloud-foo` for as long as journald
  retains them. Cross-redeploy.

### 1.2 What the user does NOT see

- No new flags on any `decloud` command.
- No new config file keys.
- No new error messages from `decloud` itself in the happy path. (One new
  internal error, `dockerdrv.ErrEmptyService`, surfaces only as a
  programmer bug, never a user-facing condition — see §4.2.)

### 1.3 Backwards-compat

Containers created before this change keep their original log driver
(`json-file`). Docker's per-container `HostConfig.LogConfig` is sealed at
create time; we never mutate it. Consequence:

- `decloud logs foo` against a pre-change container still works — the
  json-file driver supports `docker logs` natively too.
- On the NEXT `decloud deploy service --name foo …`, the old container is
  stopped and removed (`internal/deploy/service.go:195-211`), a new
  container is created with the journald driver, and from that point on
  `journalctl CONTAINER_TAG=decloud/foo` accumulates history. The
  pre-change container's json-file logs are lost when the old container is
  `docker rm`'d — same as before this change. No migration required, no
  migration possible.

### 1.4 Acceptance criteria (verbatim from Don §6, with Joel's additions)  *(REVISED — see preamble)*

1. `docker inspect decloud-foo --format '{{.HostConfig.LogConfig.Type}}'`
   prints `journald`. Same for `decloud-caddy`.
2. `docker inspect decloud-foo --format '{{index .HostConfig.LogConfig.Config "tag"}}'`
   prints `decloud/foo`. `decloud-caddy` prints `decloud/caddy`.
3. `decloud logs foo` and `decloud logs foo -f --tail 50` work identically
   to today.
4. After `decloud unregister foo` then `decloud deploy service --name foo
   …`, `journalctl CONTAINER_TAG=decloud/foo` shows lines from BOTH
   pre-redeploy and post-redeploy container instances. (Manual smoke on a
   Linux host; documented as Linux-only.)
5. `go test ./...` passes on macOS (Kent has no Docker — §6.4).
6. Constructing a `RunRequest{}` or `RunOptions{}` with an empty `Service`
   field and calling `Run` / `RunWithOptions` returns `ErrEmptyService`
   BEFORE shelling out (i.e. before `cmd.Run`); no docker process is
   spawned. Constructing the same with a `Service` field that contains
   `/` returns `ErrInvalidService` BEFORE shelling out; no docker
   process is spawned. The two sentinels are distinct (`errors.Is`
   matches the specific one and ONLY the specific one) so callers and
   tests can discriminate the two failure modes.
7. `docker start` argv MUST NOT contain `--log-driver` or `--log-opt`.
   The container's `HostConfig.LogConfig` is sealed at create time and
   `docker start` must not duplicate it. Locked by the negative
   assertion on `TestCLIDriver_StartArgs` (see §6.2.7).

---

## 2. Call sites and signatures — exact

### 2.1 The two chokepoints

```
internal/dockerdrv/cli_driver.go:46   func (d *cliDriver) Run(ctx, req RunRequest)
internal/dockerdrv/cli_driver.go:212  func (d *cliDriver) RunWithOptions(ctx, opts RunOptions)
```

These are the ONLY two functions that build a `docker run -d …` argv.
Verified by:

```
grep -rn '"run"' /Users/fenster/dev/decloud/internal /Users/fenster/dev/decloud/cmd
```

Returns only the two `args := []string{"run", "-d", …}` lines.
`docker start` (`cli_driver.go:91`) does not need the flags — they're
already on the container.

### 2.2 Every caller of `Run` / `RunWithOptions`

Read this morning at HEAD `fb4d026`:

| Call site | Line | Service-name source |
|---|---|---|
| `internal/deploy/service.go` | `253` (`Deploy`, fresh-or-redeploy `runReq`) | `req.Name` (the deploy `Request`) |
| `internal/deploy/service.go` | `385` (`restoreOldContainer`, rollback) | `prev.Config.Name` |
| `internal/deploy/lifecycle.go` | `76` (`Start`, absent branch) | `prev.Config.Name` |
| `internal/caddy/manager.go` | `95` (`Up`, fresh install) via `m.runOpts()` (line `124`) | literal `"caddy"` |
| `internal/integration/mount_test.go` | `69` (real-docker integration test, `//go:build integration`) | `"mounttest"` (NEW, see §6.5) |

There are NO callers of `Run` / `RunWithOptions` in `cmd/` or anywhere
else. Verified by the `grep -rn 'RunRequest{'` and `grep -rn 'RunOptions{'`
sweeps I did in §0 of my investigation (results in
`_tasks/2026-05-12-journald-log-driver/02-plan.md` §2.2 for Don's set,
plus the integration test that Don missed — see §6.5).

### 2.3 The `TrimPrefix(req.Name, "decloud-")` smell

Confirmed real. Single site, one line:

```
internal/dockerdrv/cli_driver.go:64
    args = append(args, "--label", "decloud.service="+strings.TrimPrefix(req.Name, "decloud-"))
```

This is the "stringly-typed" derivation Don wants gone. After this task:

```go
args = append(args, "--label", "decloud.service="+req.Service)
```

There is NO equivalent line in `RunWithOptions`: caddy doesn't carry a
`decloud.service` label, it carries `decloud.managed=caddy`
(`internal/caddy/manager.go:130`). So `RunWithOptions` does NOT change
shape on the label side; only the new journald flags are added.

---

## 3. Existing patterns to reuse

### 3.1 Service-name validation regex — IT DOES NOT EXIST AS CODE

Don wrote "Service names are constrained to `[a-z][a-z0-9-]{0,38}`
(`internal/cli/deploy_service.go:57`, `internal/ids/ids.go:23`)". I
verified: that regex appears ONLY in a Cobra flag help string at
`internal/cli/deploy_service.go:57`:

```go
cmd.Flags().StringVar(&f.Name, "name", "", "service name (required, [a-z][a-z0-9-]{0,38})")
```

It is NOT enforced by code. The Cobra layer doesn't validate the name
against any regex. The registry's `validateForSave`
(`internal/registry/store.go:206-226`) only checks
`Config.Name != ""`. There is no `regexp` import related to service
names anywhere in `internal/` (verified by `grep -rn 'regexp\|MatchString'
internal/` — only hits are `internal/ids/ids_test.go` for the
deploy-id format and `internal/registry/mount.go` for volume names).

**Implication for this task:** we cannot lean on "the regex already
proves the tag is safe". The tag IS safe in practice today because every
service name flows through `ids.ContainerName` and a TOML registry that
nobody hand-edits, but the validation gap is real. I am NOT making this
task widen its scope to add a regex (that's a separate hardening
ticket); I AM making the driver-level invariant explicit:

> `Service` must be non-empty. Empty `Service` → `ErrEmptyService`,
> returned before shelling out. We do NOT validate the character set in
> the driver; if a caller ever passes `"foo/bar"` we will emit
> `tag=decloud/foo/bar`, which journald accepts as a literal but which
> the operator running `journalctl CONTAINER_TAG=decloud/foo/bar` can
> still query unambiguously. The harm is bounded.

Add this as a follow-up in `_ai/m1x-backlog.md`:

> "Centralise service-name validation: today the regex
> `[a-z][a-z0-9-]{0,38}` lives only in a Cobra help string. Add a
> single validator (likely `ids.ValidateServiceName(string) error`) and
> call it from the CLI and the registry loader, so all downstream
> consumers — journald tag, label, container name — share one truth."

This is the only place I'm widening Don's deferred-list. He didn't see
the gap because his §2.1 sweep was on `docker run` argv builders, not on
the validation surface.

### 3.2 Conditional-flag patterns

The codebase has TWO patterns for adding flags conditionally to a
`docker` argv:

**A. Unconditional fixed flags after the base block** — used for
`--name`, `--network`, `--restart` in both `Run` and `RunWithOptions`.
The pattern is: declare them inside the initial `args := []string{…}`
literal, not in a later `append`.

```go
// internal/dockerdrv/cli_driver.go:47-52
args := []string{
    "run", "-d",
    "--name", req.Name,
    "--network", req.Network,
    "--restart", req.Restart,
}
```

**B. Conditional `append` after** — used for env (skip if empty map),
labels, volumes (skip if empty slice), ports.

```go
// internal/dockerdrv/cli_driver.go:53-63
keys := make([]string, 0, len(req.Env))
for k := range req.Env { keys = append(keys, k) }
sort.Strings(keys)
for _, k := range keys {
    args = append(args, "--env", k+"="+req.Env[k])
}
for _, v := range req.Volumes {
    args = append(args, "-v", formatVolume(v))
}
```

**Which to match for journald flags:** Pattern A. The flags are
unconditional — every container, always. Putting them inside the literal
keeps argv-order discipline mechanical (literal first, appended later)
and signals to a reader "this is fixed, like `--restart`".

```go
args := []string{
    "run", "-d",
    "--name", req.Name,
    "--network", req.Network,
    "--restart", req.Restart,
    "--log-driver", "journald",
    "--log-opt", "tag=decloud/" + req.Service,
}
```

This is a refinement of Don §3.2 ("insert immediately after the
`--restart` block … `args = append(args, "--log-driver", …)`"). His
position is identical; my insertion mechanism is a hair cleaner — same
spot, but inside the literal rather than as a follow-up `append`. The
behavioural result is byte-identical argv. See §11.1.

### 3.3 Sentinel-error pattern  *(REVISED — see preamble)*

The codebase already has `ErrContainerNotFound` and `ErrNoBridgeIP` at
`internal/dockerdrv/driver.go:12-21` in a `var (…)` block. Add
`ErrEmptyService` AND `ErrInvalidService` to the same block. Same
style:

```go
var (
    ErrContainerNotFound = errors.New("dockerdrv: container not found")
    ErrNoBridgeIP        = errors.New("dockerdrv: container has no bridge network IP")
    ErrEmptyService      = errors.New("dockerdrv: empty Service field (programmer bug; populate at every Run/RunWithOptions call site)")
    ErrInvalidService    = errors.New(`dockerdrv: invalid Service field (must not contain '/'; the journald tag "decloud/<service>" would otherwise be ambiguous under journalctl CONTAINER_TAG= prefix queries)`)
)
```

The longer error messages are a deliberate choice over Don's
`errors.New("dockerdrv: empty Service in RunRequest/RunOptions")`. The
caller will see these only as a programmer bug at test time; the wordy
"populate at every Run/RunWithOptions call site" tells them WHERE to
fix it, and the `ErrInvalidService` message names the EXACT downstream
failure mode (journalctl prefix-query ambiguity) so the next reader
doesn't have to grep for context. (Joel's rule: "write code as if the
next person to read it has my home address and a baseball bat.") See
§11.2 and §11.7.

**Why two sentinels and not one (e.g. `ErrInvalidService` only).**
Empty `Service` is a "you forgot to set the field" bug at the call
site; slash-in-`Service` is a "you set the field to something that
violates the journald-tag invariant" bug, which can only arise from
upstream (caller of `Deploy`/`Start`/`runOpts`) passing garbage. Two
distinct failure modes; two distinct sentinels; callers and tests can
`errors.Is`-discriminate. Folding them under a single sentinel would
make `assert.True(t, errors.Is(err, ErrInvalidService))` fail for the
empty case (or vice versa) and cost the discrimination, which is
exactly what tests in §6.2.1–§6.2.4 lock.

### 3.4 Returned-error, not panic

Don suggested in §10 "decide whether `ErrEmptyService` is a panic or
returned error (I say returned, but argue if you disagree)." I agree:
returned error. The codebase has no `panic()` outside of `panic(err)`
in `cmd/decloud/main.go` boot-strap; programmer bugs in driver methods
are uniformly returned errors that surface up the test stack. Match the
house style.

---

## 4. Exact argv insertion

### 4.1 Before/after — `Run` (service deploys)

**BEFORE** (`cli_driver.go:46-66`):

```
docker run -d --name decloud-foo --network decloud --restart unless-stopped
  --env A=1 --env B=2
  -v /host:/dst:ro
  --label decloud.service=foo
  decloud-foo:abc123
```

**AFTER**:

```
docker run -d --name decloud-foo --network decloud --restart unless-stopped
  --log-driver journald --log-opt tag=decloud/foo
  --env A=1 --env B=2
  -v /host:/dst:ro
  --label decloud.service=foo
  decloud-foo:abc123
```

Insertion point: between `--restart unless-stopped` and the first
`--env`. The two new flags are emitted as four argv tokens (`--log-driver`,
`journald`, `--log-opt`, `tag=decloud/foo`) — using the
SPACE-separated form of `--log-driver`, not `--log-driver=journald`,
to match the surrounding `--name name`, `--network n`, `--restart r`
style. Same for `--log-opt tag=…` (the `tag=…` is the value of
`--log-opt`, not an `=`-separated flag).

### 4.2 Before/after — `RunWithOptions` (caddy)

**BEFORE** (`cli_driver.go:212-241`):

```
docker run -d --name decloud-caddy --network decloud --restart unless-stopped
  --label decloud.managed=caddy
  -p 0.0.0.0:80:80/tcp -p [::]:80:80/tcp
  -p 0.0.0.0:443:443/tcp -p [::]:443:443/tcp
  -p 0.0.0.0:443:443/udp -p [::]:443:443/udp
  -v /opt/decloud/config/caddy:/etc/caddy:ro
  -v decloud_caddy_data:/data
  -v decloud_caddy_config:/config
  caddy:2
```

**AFTER**:

```
docker run -d --name decloud-caddy --network decloud --restart unless-stopped
  --log-driver journald --log-opt tag=decloud/caddy
  --label decloud.managed=caddy
  -p 0.0.0.0:80:80/tcp …                                  (unchanged)
  …
  caddy:2
```

Same insertion point: between `--restart unless-stopped` and the first
existing appended flag (which is `--label` in caddy's case, `--env`
or `-v` in services' case — the relative position is "right after the
fixed-flag literal, before the appended-by-loop section").

### 4.3 Why this position

Two reasons, both already implicit in Don §1.4:

1. **Argv-order discipline.** The fixed-flag literal at the top of each
   function describes the container's identity and lifecycle policy:
   name, network, restart. Logging belongs in that set conceptually —
   it's a "how this container behaves" policy, not a payload (env,
   labels, volumes, ports) and not the image. Putting it inside the
   literal next to `--restart` reads correctly.
2. **Test diff minimisation.** The three existing argv-assertion tests
   (`TestCLIDriver_RunArgsWithEnvSorted`, `TestCLIDriver_RunArgsWithEmptyEnv`,
   `TestCLIDriver_RunWithOptionsCaddyShape`) already lock argv slot
   positions for everything after `--restart`. Splicing in two flags
   immediately after `--restart` pushes every existing slot DOWN by
   four positions in a way Kent can mechanically splice into the
   expected slice. No reordering of existing flags.

### 4.4 Tag literal — confirmed against docker daemon docs

The journald docker logging driver writes `CONTAINER_TAG=<tag>` and
`SYSLOG_IDENTIFIER=<tag>` for each line. The tag passed to
`--log-opt tag=…` is byte-literal; the only template expansion happens
if the value contains `{{ }}` Go-template markers (e.g.
`tag={{.Name}}`). We pass a plain string with no `{{`, so no expansion.
`/` is allowed in tags; journald stores it verbatim; `journalctl
CONTAINER_TAG=decloud/foo` matches exactly.

Confirmed against: docker docs "Customize the log driver output" →
"tag" option (any release ≥ 20.10). Cross-checked with `man
systemd.journal-fields` for `CONTAINER_TAG`.

---

## 5. The change, file by file

### 5.1 `internal/dockerdrv/driver.go`  *(REVISED — see preamble)*

Two struct changes and TWO sentinels.

**Add `ErrEmptyService` AND `ErrInvalidService` to the `var (…)` block
at line 12-21:**

```go
var (
    ErrContainerNotFound = errors.New("dockerdrv: container not found")
    ErrNoBridgeIP        = errors.New("dockerdrv: container has no bridge network IP")
    ErrEmptyService      = errors.New("dockerdrv: empty Service field (programmer bug; populate at every Run/RunWithOptions call site)")
    ErrInvalidService    = errors.New(`dockerdrv: invalid Service field (must not contain '/'; the journald tag "decloud/<service>" would otherwise be ambiguous under journalctl CONTAINER_TAG= prefix queries)`)
)
```

**Add `Service string` to `RunRequest` (between `Name` and `Image`, lines 31-39):**

```go
type RunRequest struct {
    Name    string
    Service string         // service name; populates journald tag and the decloud.service label
    Image   string
    Network string
    Env     map[string]string
    Restart string
    Port    int
    Volumes []VolumeMount
}
```

**Add `Service string` to `RunOptions` (between `Name` and `Image`, lines 77-86):**

```go
type RunOptions struct {
    Name    string
    Service string         // service name; populates journald tag
    Image   string
    Network string
    Restart string
    Ports   []PortMap
    Volumes []VolumeMount
    Labels  map[string]string
    Env     map[string]string
}
```

Field placement (between `Name` and `Image`) is deliberate: visually
adjacent to `Name`, which is the related-but-distinct concept
(`Name` is the container name `decloud-foo`; `Service` is the bare
service name `foo`). Joel's rule: physically co-locate related fields so
the next maintainer sees the relationship without reading docs.

### 5.2 `internal/dockerdrv/cli_driver.go`  *(REVISED — see preamble)*

**`Run` — splice journald flags into the literal, replace TrimPrefix,
add BOTH guards:**

```go
func (d *cliDriver) Run(ctx context.Context, req RunRequest) (string, error) {
    if req.Service == "" {
        return "", ErrEmptyService
    }
    if strings.ContainsRune(req.Service, '/') {
        return "", ErrInvalidService
    }
    args := []string{
        "run", "-d",
        "--name", req.Name,
        "--network", req.Network,
        "--restart", req.Restart,
        "--log-driver", "journald",
        "--log-opt", "tag=decloud/" + req.Service,
    }
    keys := make([]string, 0, len(req.Env))
    for k := range req.Env {
        keys = append(keys, k)
    }
    sort.Strings(keys)
    for _, k := range keys {
        args = append(args, "--env", k+"="+req.Env[k])
    }
    for _, v := range req.Volumes {
        args = append(args, "-v", formatVolume(v))
    }
    args = append(args, "--label", "decloud.service="+req.Service)
    args = append(args, req.Image)
    // … rest unchanged (cmd.Run, stdout/stderr handling)
}
```

Three non-cosmetic edits vs. today:

1. New guard clauses at the very top, in this exact order: empty first,
   then slash. Order matters for test-readability — the `req.Service ==
   ""` check is the cheaper one and the "you forgot the field" failure
   mode is the more common one in test-fixture proliferation. The slash
   check sits behind it and only fires for non-empty-but-invalid input.
2. New flags inside the literal.
3. `strings.TrimPrefix(req.Name, "decloud-")` at line 64 → `req.Service`.

Note: the `strings` import is still needed (used by
`isNotFound` at `cli_driver.go:198` and `ContainerIP` at
`cli_driver.go:190`, plus the new `strings.ContainsRune` guard).
Don't remove it.

**`RunWithOptions` — splice journald flags into the literal, add BOTH
guards:**

```go
func (d *cliDriver) RunWithOptions(ctx context.Context, opts RunOptions) (string, error) {
    if opts.Service == "" {
        return "", ErrEmptyService
    }
    if strings.ContainsRune(opts.Service, '/') {
        return "", ErrInvalidService
    }
    args := []string{
        "run", "-d",
        "--name", opts.Name,
        "--network", opts.Network,
        "--restart", opts.Restart,
        "--log-driver", "journald",
        "--log-opt", "tag=decloud/" + opts.Service,
    }
    // … rest unchanged (env, labels, ports, volumes, image, cmd.Run)
}
```

### 5.3 `internal/deploy/service.go`

**Two `RunRequest` literals to update.**

At `service.go:244-252` (`Deploy`, fresh-or-redeploy):

```go
runReq := dockerdrv.RunRequest{
    Name:    containerName,
    Service: req.Name,                   // NEW
    Image:   imageRef,
    Network: "decloud",
    Env:     captured,
    Restart: "unless-stopped",
    Port:    req.Port,
    Volumes: toVolumeMounts(req.Mounts),
}
```

At `service.go:376-384` (`restoreOldContainer`, rollback):

```go
runReq := dockerdrv.RunRequest{
    Name:    ids.ContainerName(prev.Config.Name),
    Service: prev.Config.Name,           // NEW
    Image:   prev.Config.Build.ImageRef,
    Network: "decloud",
    Env:     prev.Secrets.Env,
    Restart: prev.Config.Run.Restart,
    Port:    prev.Config.Run.Port,
    Volumes: toVolumeMounts(prev.Config.Run.Mounts),
}
```

### 5.4 `internal/deploy/lifecycle.go`

**One `RunRequest` literal to update**, at `lifecycle.go:67-75`
(`Start`, absent branch):

```go
runReq := dockerdrv.RunRequest{
    Name:    containerName,
    Service: name,                       // NEW; `name` is the function's first arg
    Image:   prev.Config.Build.ImageRef,
    Network: "decloud",
    Env:     prev.Secrets.Env,
    Restart: prev.Config.Run.Restart,
    Port:    prev.Config.Run.Port,
    Volumes: toVolumeMounts(prev.Config.Run.Mounts),
}
```

Note: I prefer `name` (the function arg) to `prev.Config.Name` here
because the function already trusts `name` for the container-name
derivation at line 53 (`containerName := ids.ContainerName(name)`).
Two derivations from the same trust source is cleaner than mixing.
`prev.Config.Name` would also work and is identical in practice (the
store keys on filename = `name`); either is fine. Rob: pick `name`.

### 5.5 `internal/caddy/manager.go`

**One `RunOptions` literal to update**, at `manager.go:124-145`
(`runOpts()`):

```go
func (m *cliManager) runOpts() dockerdrv.RunOptions {
    return dockerdrv.RunOptions{
        Name:    ContainerName,
        Service: "caddy",                 // NEW; tag becomes decloud/caddy
        Image:   DefaultImage,
        Network: NetworkName,
        Restart: "unless-stopped",
        // … (Labels, Ports, Volumes unchanged)
    }
}
```

Confirmed: the call site `m.cfg.Driver.RunWithOptions(ctx, m.runOpts())`
at `manager.go:95` will pass the new `Service: "caddy"` through. No
plumbing needed. The hardcoded `"caddy"` literal is correct: the caddy
container is structurally singular, not a configurable per-service
entity. Don §1.3 already settled this.

### 5.6 Mock regeneration

`internal/dockerdrv/mocks/mock_driver.go` is generated by `go generate
./...` from the `//go:generate` directive at
`internal/dockerdrv/driver.go:10`. Adding a field to a struct does NOT
change the `Driver` interface, so mock regeneration is technically
unnecessary — the mock methods take `RunRequest` / `RunOptions` by
value and the new field is matched implicitly by `gomock.Eq`.

That said, run `go generate ./...` once and commit any whitespace
re-rendering for hygiene. Rob: include the regenerated file in the same
commit as the struct change.

---

## 6. Test surface — exactly what Kent writes

### 6.1 Existing argv-assertion tests to UPDATE

Three tests in `internal/dockerdrv/cli_driver_test.go` hold a fully
literal expected `[]string`. Each gets four new tokens spliced in after
`"--restart", "unless-stopped"`, plus the `RunRequest`/`RunOptions`
fixture gets a `Service:` field added. Below, "splice point" is the
index in the existing `[]string{…}` literal where the four new tokens
land — Kent will paste them in place. Slot positions in tests are
1-indexed against the `[]string{…}` literal as read in source.

#### 6.1.1 `TestCLIDriver_RunArgsWithEnvSorted` (`cli_driver_test.go:71-96`)

- Add to the `RunRequest{…}` fixture (lines 75-82):
  `Service: "foo",` (between `Name` and `Image`).
- In the expected `[]string{…}` (lines 85-95), splice four tokens
  immediately after `"--restart", "unless-stopped",`:

```go
assert.Equal(t, []string{
    "run", "-d",
    "--name", "decloud-foo",
    "--network", "decloud",
    "--restart", "unless-stopped",
    "--log-driver", "journald",                  // NEW
    "--log-opt", "tag=decloud/foo",              // NEW
    "--env", "A=1",
    "--env", "B=2",
    "--env", "C=3",
    "--label", "decloud.service=foo",
    "decloud-foo:abc123",
}, records[0].Args)
```

- Update the hand-typed comment above the test (lines 69-70) to include
  the new flags. Keep the comment in sync with the assertion (the file
  preamble at lines 1-4 makes this a hard rule).

#### 6.1.2 `TestCLIDriver_RunArgsWithEmptyEnv` (`cli_driver_test.go:100-115`)

This test does NOT assert the full slice; it iterates `records[0].Args`
and asserts no `--env` appears. It still needs:

- `Service: "foo",` in the `RunRequest{…}` fixture (lines 104-110).
- A new assertion that the journald flags ARE present (otherwise this
  test silently stops covering the empty-env branch's flag positioning
  when env is absent):

```go
assert.Contains(t, records[0].Args, "--log-driver")
assert.Contains(t, records[0].Args, "journald")
assert.Contains(t, records[0].Args, "--log-opt")
assert.Contains(t, records[0].Args, "tag=decloud/foo")
```

- Update the hand-typed comment (lines 98-99) accordingly.

#### 6.1.3 `TestCLIDriver_RunPassesVolumeFlags` (`cli_driver_test.go:408-425`)

This test uses `volumeFlagsFromArgs` to extract just the `-v` values —
it does NOT assert the full slice. The only required change is:

- `Service: "foo",` in the `RunRequest{…}` fixture (lines 412-421).

The volume-extraction assertion is unaffected.

- Update the hand-typed comment (lines 405-407) for consistency.

#### 6.1.4 `TestCLIDriver_RunWithOptionsCaddyShape` (`cli_driver_test.go:360-384`)

- Add to the `caddyRunOptionsFixture()` (lines 526-547):
  `Service: "caddy",` (between `Name` and `Image`, line 528-529).
- In the expected `[]string{…}` (lines 367-383), splice four tokens
  immediately after `"--restart", "unless-stopped",`:

```go
assert.Equal(t, []string{
    "run", "-d",
    "--name", "decloud-caddy",
    "--network", "decloud",
    "--restart", "unless-stopped",
    "--log-driver", "journald",                  // NEW
    "--log-opt", "tag=decloud/caddy",            // NEW
    "--label", "decloud.managed=caddy",
    "-p", "0.0.0.0:80:80/tcp",
    // … (rest unchanged)
}, records[0].Args)
```

- Update the hand-typed comment (lines 350-359) for consistency.

#### 6.1.5 `TestCLIDriver_RunWithOptions*` (the helper-based ones)

`TestCLIDriver_RunWithOptionsBindReadOnly` (line 427),
`TestCLIDriver_RunWithOptionsNamedVolumeNotReadOnly` (line 441),
`TestCLIDriver_RunWithOptionsLabelsSorted` (line 455),
`TestCLIDriver_RunWithOptionsPortsDeclaredOrder` (line 467),
`TestCLIDriver_RunWithOptionsPortDefaultProto` (line 483),
`TestCLIDriver_RunWithOptionsEmptyHostBind` (line 497),
`TestCLIDriver_RunWithOptionsDualStackPorts` (line 386) — these use
`RunOptions{Name: "x", Image: "img", …}` fixtures inline and assert
only the extracted ports/volumes/labels.

Each needs `Service: "x"` added to its `RunOptions{…}` (otherwise the
new guard clause fires and the test fails with `ErrEmptyService`).
Mechanical edit, one line each.

### 6.2 New tests in `internal/dockerdrv/cli_driver_test.go`  *(REVISED — see preamble)*

Six new tests plus one extension of an existing test. Names below are
the exact `t.Name()` Kent should use:

- §6.2.1 — empty-Service rejection on `Run`
- §6.2.2 — empty-Service rejection on `RunWithOptions`
- §6.2.3 — slash-in-Service rejection on `Run` (NEW per Linus §2.1 / Don §1.7)
- §6.2.4 — slash-in-Service rejection on `RunWithOptions` (NEW per Linus §2.1 / Don §1.7)
- §6.2.5 — tag-literal lock on `Run` (was §6.2.3)
- §6.2.6 — tag-literal lock on `RunWithOptions` (was §6.2.4 "optional",
  now REQUIRED per Linus §5.2 / Don §4.1)
- §6.2.7 — `TestCLIDriver_StartArgs` gains `assert.NotContains(…,
  "--log-driver")` (NEW per Linus §5.1 / Don §4.1)

#### 6.2.1 `TestCLIDriver_RunReturnsErrEmptyServiceWhenServiceIsEmpty`

```go
func TestCLIDriver_RunReturnsErrEmptyServiceWhenServiceIsEmpty(t *testing.T) {
    var records []recordedCmd
    d := driverWith(recordingFactory(&records))

    _, err := d.Run(context.Background(), RunRequest{
        Name:    "decloud-foo",
        Image:   "decloud-foo:abc123",
        Network: "decloud",
        Restart: "unless-stopped",
        // Service intentionally omitted
    })
    require.Error(t, err)
    assert.True(t, errors.Is(err, ErrEmptyService),
        "empty Service must return ErrEmptyService sentinel for caller error-chain matching")
    assert.False(t, errors.Is(err, ErrInvalidService),
        "empty-Service error must NOT match ErrInvalidService — the two sentinels are distinct")
    assert.Empty(t, records,
        "no docker process must be spawned when Service is empty (guard fires before cmd.Run)")
}
```

Note the `assert.Empty(t, records)`: that is the load-bearing assertion
that proves the guard fires before `cmd.Run`. Without it the test
would pass even if Rob put the guard after the exec. The
`assert.False(t, errors.Is(err, ErrInvalidService))` is the
discrimination assertion: it locks the §3.3 promise that the two
sentinels are distinct.

#### 6.2.2 `TestCLIDriver_RunWithOptionsReturnsErrEmptyServiceWhenServiceIsEmpty`

Same shape, against `RunWithOptions`:

```go
func TestCLIDriver_RunWithOptionsReturnsErrEmptyServiceWhenServiceIsEmpty(t *testing.T) {
    var records []recordedCmd
    d := driverWith(recordingFactory(&records))

    _, err := d.RunWithOptions(context.Background(), RunOptions{
        Name: "decloud-caddy", Image: "caddy:2", Network: "decloud", Restart: "unless-stopped",
        // Service intentionally omitted
    })
    require.Error(t, err)
    assert.True(t, errors.Is(err, ErrEmptyService))
    assert.False(t, errors.Is(err, ErrInvalidService),
        "empty-Service error must NOT match ErrInvalidService — the two sentinels are distinct")
    assert.Empty(t, records)
}
```

#### 6.2.3 `TestCLIDriver_RunReturnsErrInvalidServiceWhenServiceContainsSlash`  *(NEW)*

Per Linus §2.1 / Don §1.7. Defends the journald-tag-ambiguity
invariant (`journalctl CONTAINER_TAG=decloud/foo` must return ONLY
that service's lines, never `decloud/foo/bar` prefix matches). Same
shape as §6.2.1 — guard-before-exec proved by `assert.Empty(t, records)`,
plus the discrimination assertion proving `ErrInvalidService` is
distinct from `ErrEmptyService`:

```go
func TestCLIDriver_RunReturnsErrInvalidServiceWhenServiceContainsSlash(t *testing.T) {
    var records []recordedCmd
    d := driverWith(recordingFactory(&records))

    _, err := d.Run(context.Background(), RunRequest{
        Name:    "decloud-foo",
        Service: "foo/bar",                  // the failure mode under test
        Image:   "decloud-foo:abc123",
        Network: "decloud",
        Restart: "unless-stopped",
    })
    require.Error(t, err)
    assert.True(t, errors.Is(err, ErrInvalidService),
        "Service containing '/' must return ErrInvalidService sentinel for caller error-chain matching")
    assert.False(t, errors.Is(err, ErrEmptyService),
        "slash-in-Service error must NOT match ErrEmptyService — the two sentinels are distinct")
    assert.Empty(t, records,
        "no docker process must be spawned when Service is invalid (guard fires before cmd.Run)")
}
```

The `assert.Empty(t, records)` is load-bearing for the same reason as
§6.2.1: it proves the guard fires BEFORE `cmd.Run`. Without it the
test would pass even if Rob accidentally put the slash check after
the exec, leaving the bad tag in argv for that one transient invocation.

#### 6.2.4 `TestCLIDriver_RunWithOptionsReturnsErrInvalidServiceWhenServiceContainsSlash`  *(NEW)*

Same shape, against `RunWithOptions`:

```go
func TestCLIDriver_RunWithOptionsReturnsErrInvalidServiceWhenServiceContainsSlash(t *testing.T) {
    var records []recordedCmd
    d := driverWith(recordingFactory(&records))

    _, err := d.RunWithOptions(context.Background(), RunOptions{
        Name:    "decloud-caddy",
        Service: "caddy/v2",                 // the failure mode under test
        Image:   "caddy:2",
        Network: "decloud",
        Restart: "unless-stopped",
    })
    require.Error(t, err)
    assert.True(t, errors.Is(err, ErrInvalidService))
    assert.False(t, errors.Is(err, ErrEmptyService),
        "slash-in-Service error must NOT match ErrEmptyService — the two sentinels are distinct")
    assert.Empty(t, records)
}
```

Two paths, two tests. Don §1.7 explicitly called out that "the
slash-rejection tests are a near-mechanical copy" of the
empty-Service tests; this is that copy.

#### 6.2.5 `TestCLIDriver_RunEmitsJournaldFlagsWithSlashTagLiteral`  *(was §6.2.3)*

This is the byte-level lock on the tag literal. Don §4.1 listed it; my
version names what the assertion is actually defending against:

```go
func TestCLIDriver_RunEmitsJournaldFlagsWithSlashTagLiteral(t *testing.T) {
    var records []recordedCmd
    d := driverWith(recordingFactory(&records))

    _, err := d.Run(context.Background(), RunRequest{
        Name: "decloud-foo", Service: "foo",
        Image: "decloud-foo:abc123", Network: "decloud", Restart: "unless-stopped",
    })
    require.NoError(t, err)
    require.Len(t, records, 1)

    args := records[0].Args
    driverIdx := indexOf(args, "--log-driver")
    require.GreaterOrEqual(t, driverIdx, 0, "--log-driver must appear in argv")
    require.Equal(t, "journald", args[driverIdx+1], "--log-driver value must be exactly \"journald\"")

    optIdx := indexOf(args, "--log-opt")
    require.GreaterOrEqual(t, optIdx, 0, "--log-opt must appear in argv")
    require.Equal(t, "tag=decloud/foo", args[optIdx+1],
        "tag must be literal \"decloud/foo\" — NOT \"decloud-foo\", NOT \"{{.Name}}\", NOT bare \"foo\"")

    // The pair MUST be adjacent in this declared order:
    // --log-driver journald --log-opt tag=…
    require.Equal(t, driverIdx+2, optIdx,
        "--log-opt must immediately follow --log-driver journald (4 contiguous tokens)")
}

// helper local to test file:
func indexOf(args []string, needle string) int {
    for i, a := range args { if a == needle { return i } }
    return -1
}
```

This test is the single source of truth for "the tag format is
`decloud/<service>`". If a future refactor flips it to `decloud-<service>`
or `{{.Name}}` or `<service>` alone, this test fails loudly.

#### 6.2.6 `TestCLIDriver_RunWithOptionsEmitsJournaldFlagsWithCaddyTag`  *(was §6.2.4; now REQUIRED per Linus §5.2 / Don §4.1)*

Same shape as §6.2.5, asserts `tag=decloud/caddy` on the
`RunWithOptions` path. The caddy tag is the only journald-flag literal
that differs between the two `docker run` argv builders; without this
test a future refactor that hardcodes the wrong tag in
`RunWithOptions` (say, `decloud/foo` cargo-culted from §6.2.5) would
only surface in Linux integration smoke. Promoting from "optional,
recommended" to REQUIRED.

Test body mirrors §6.2.5 — the only differences are the
`RunWithOptions` fixture and the expected tag literal
`tag=decloud/caddy`:

```go
func TestCLIDriver_RunWithOptionsEmitsJournaldFlagsWithCaddyTag(t *testing.T) {
    var records []recordedCmd
    d := driverWith(recordingFactory(&records))

    _, err := d.RunWithOptions(context.Background(), caddyRunOptionsFixture())
    require.NoError(t, err)
    require.Len(t, records, 1)

    args := records[0].Args
    driverIdx := indexOf(args, "--log-driver")
    require.GreaterOrEqual(t, driverIdx, 0, "--log-driver must appear in argv")
    require.Equal(t, "journald", args[driverIdx+1], "--log-driver value must be exactly \"journald\"")

    optIdx := indexOf(args, "--log-opt")
    require.GreaterOrEqual(t, optIdx, 0, "--log-opt must appear in argv")
    require.Equal(t, "tag=decloud/caddy", args[optIdx+1],
        "caddy tag must be literal \"decloud/caddy\" — NOT \"decloud/foo\", NOT \"decloud-caddy\", NOT \"caddy\"")

    require.Equal(t, driverIdx+2, optIdx,
        "--log-opt must immediately follow --log-driver journald (4 contiguous tokens)")
}
```

Uses the same `indexOf` helper defined in §6.2.5; do NOT redeclare it.
Uses the existing `caddyRunOptionsFixture()` (`cli_driver_test.go:526`)
which §6.1.4 already updated to carry `Service: "caddy"`.

#### 6.2.7 `TestCLIDriver_StartArgs` — extension: assert `--log-driver` is NOT in argv  *(NEW per Linus §5.1 / Don §4.1)*

The existing test at `cli_driver_test.go:128-135`:

```go
// docker start decloud-foo
func TestCLIDriver_StartArgs(t *testing.T) {
    var records []recordedCmd
    d := driverWith(recordingFactory(&records))

    require.NoError(t, d.Start(context.Background(), "decloud-foo"))
    require.Len(t, records, 1)
    assert.Equal(t, []string{"start", "decloud-foo"}, records[0].Args)
}
```

The `assert.Equal(t, []string{"start", "decloud-foo"}, …)` already
locks the argv byte-for-byte, so a regression that adds
`"--log-driver"` to `docker start` argv would fail this test. Linus
still wants an EXPLICIT negative assertion right under the equality
check so the failure mode is named when (not if) someone in a future
"consistency" refactor wonders why `docker start` doesn't carry the
log flags:

```go
// docker start decloud-foo
func TestCLIDriver_StartArgs(t *testing.T) {
    var records []recordedCmd
    d := driverWith(recordingFactory(&records))

    require.NoError(t, d.Start(context.Background(), "decloud-foo"))
    require.Len(t, records, 1)
    assert.Equal(t, []string{"start", "decloud-foo"}, records[0].Args)
    assert.NotContains(t, records[0].Args, "--log-driver",
        "docker start must NOT re-emit journald flags; HostConfig.LogConfig is sealed at create time")
    assert.NotContains(t, records[0].Args, "--log-opt",
        "docker start must NOT re-emit journald flags; HostConfig.LogConfig is sealed at create time")
}
```

Two `NotContains` (one per flag) rather than one because the failure
mode story is symmetric and the messages name the EXACT invariant —
"`HostConfig.LogConfig` is sealed at create time" — that a future
refactor would need to violate to break it. Acceptance criterion 7
(§1.4) is the spec-level statement; this is its test-level lock.

Kent: this is an extension of an EXISTING test, not a new
`func Test…`. Add the two assertions to
`TestCLIDriver_StartArgs` in place. The hand-typed comment at line 127
(`// docker start decloud-foo`) is still accurate; don't change it.

### 6.3 Updates to `internal/deploy/service_test.go`

Three changes:

#### 6.3.1 Extend `TestDeploy_RunRequestUsesCapturedEnvAndDecloudNetwork` (line 831)

The test already captures `seen dockerdrv.RunRequest` via `DoAndReturn`.
Add ONE assertion after the existing ones (after line 855):

```go
assert.Equal(t, "foo", seen.Service,
    "RunRequest.Service must equal req.Name so the driver derives tag=decloud/foo")
```

This pins the contract that `Deploy` populates `Service` from
`req.Name`.

#### 6.3.2 Extend `TestDeploy_RestoreOldContainerPassesVolumesToDriver` (line 1011)

The rollback test captures `rollbackVolumes` via the SECOND `Run`
DoAndReturn (line 1028-1032). Extend that closure to also capture
`rollbackSvc string` and assert:

```go
var rollbackVolumes []dockerdrv.VolumeMount
var rollbackSvc string
// …
h.driver.EXPECT().Run(gomock.Any(), gomock.Any()).
    DoAndReturn(func(_ context.Context, req dockerdrv.RunRequest) (string, error) {
        rollbackVolumes = req.Volumes
        rollbackSvc = req.Service
        return "rb-cid", nil
    }),
// after err assertion:
assert.Equal(t, "foo", rollbackSvc,
    "rollback RunRequest.Service must equal prev.Config.Name so the restored container shares the journald tag")
```

This pins the contract that `restoreOldContainer` populates `Service`
from `prev.Config.Name`.

#### 6.3.3 Audit `newRequest()` and other fixtures

`newRequest()` at `service_test.go:182` already sets `Name: "foo"`, and
every test in `service_test.go` derives expected `Service: "foo"`. No
fixture file edit needed — the `Service` field is populated by
production code from `req.Name`. The new assertions above are the only
new test code. Kent: do NOT add `Service:` to the `deploy.Request`
struct; it's `dockerdrv.RunRequest` that gets the new field, and
production code threads `req.Name` → `runReq.Service`.

### 6.4 Updates to `internal/deploy/lifecycle_test.go`

One extension, to `TestLifecycle_StartFromAbsentReRunsContainer` at
`lifecycle_test.go:234`. The test captures `seen dockerdrv.RunRequest`.
Add ONE assertion after line 254 (`assert.Equal(t, prev.Secrets.Env,
seen.Env)`):

```go
assert.Equal(t, "foo", seen.Service,
    "absent-state Start must populate RunRequest.Service so the re-run container gets tag=decloud/foo")
```

`TestLifecycle_StartAbsentBranchPassesVolumesToDriver` at line 257 also
captures `seen` — add the same assertion there for symmetry (and to
catch a regression where someone forgets to propagate `Service`
through the volumes-carrying path):

```go
assert.Equal(t, "foo", seen.Service)
```

### 6.5 Updates to `internal/caddy/manager_test.go`

One edit to `expectedCaddyRunOptions` at `manager_test.go:55`:

```go
func expectedCaddyRunOptions(paths config.Paths) dockerdrv.RunOptions {
    return dockerdrv.RunOptions{
        Name:    caddy.ContainerName,
        Service: "caddy",                  // NEW
        Image:   caddy.DefaultImage,
        // … (rest unchanged)
    }
}
```

Every test that uses this fixture (`TestManager_UpFreshInstall` at
line 78, the `Up`-after-stop variants, etc.) picks up the change for
free via the gomock `Eq` matcher. No other edits in this file.

### 6.6 Integration test — `internal/integration/mount_test.go`

Don missed this caller in his §2.2 sweep. It's `//go:build integration`
and only runs with `DECLOUD_INTEGRATION=1`, so the maintainer's macOS
box never runs it. But the file imports `dockerdrv` and constructs a
`RunRequest{…}` at line 69. After the change, it MUST set `Service:`
or the integration test fails with `ErrEmptyService`:

```go
_, err := driver.Run(runCtx, dockerdrv.RunRequest{
    Name:    mountTestContainer,
    Service: "mounttest",                  // NEW
    Image:   mountTestImage,
    Network: "decloud",
    Restart: "no",
    Volumes: []dockerdrv.VolumeMount{ … },
})
```

Rob: include this edit in the implementation commit. Kent: this file is
`//go:build integration` so `go test ./...` on the dev box won't run
it, but `go vet ./...` and `go build ./...` will, so the file must
still compile. Use `go vet ./...` and `go build -tags=integration ./...`
in CI to catch this.

### 6.7 No Docker on dev box — confirmed

User memory note: "dev box has no Docker; maintainer runs integration
tests on a separate Linux host." Every test in §6.1–§6.5 above is an
argv-construction test (`recordingFactory` or `scriptedFactory` —
neither shells out to real docker). They run on the macOS dev box with
no Docker installed. Confirmed by reading `cli_driver_test.go:27-44`:
the recording factory runs `/usr/bin/true` (`exec.CommandContext(ctx,
"true")`), the scripted factory runs `/bin/sh -c …`. Neither requires
Docker. `go test ./...` is green on macOS.

The only Docker-dependent test is
`internal/integration/mount_test.go` (`//go:build integration`),
which the user runs on the separate Linux host. It picks up the
journald change automatically via §6.6 above.

### 6.8 Acceptance-criterion 6 — empty Service is rejected

Captured by §6.2.1 and §6.2.2. Don listed it in §6.6 of his plan; Kent's
two new tests are the unit-test embodiment.

---

## 7. Caddy edge case — confirmed

Don decided caddy gets `Service: "caddy"`, tag `decloud/caddy`. The call
site is `internal/caddy/manager.go:95`, which calls
`m.cfg.Driver.RunWithOptions(ctx, m.runOpts())`. `m.runOpts()` at line
124 is the ONLY place the `RunOptions` is constructed; setting
`Service: "caddy"` there propagates through every code path:

- `Up()` from `absent` → calls `runOpts()` (line 95).
- `Up()` from `exited` → calls `Driver.Start` (line 81); no `Run`,
  preserves existing log config from when the container was created.
- `Up()` from `running` → no-op (line 78).
- `Down()` → `Stop` then `Remove`; no `Run`.

The literal `"caddy"` is appropriate. Caddy is structurally singular.
No risk of name collision with a user service named `caddy`: the user's
service container is `decloud-caddy` via `ids.ContainerName("caddy")`,
which is the same name as the caddy proxy container. This is already a
forbidden collision — the registry would refuse to save a service
named `caddy` because it would clobber the caddy proxy's container
name. (Side note: that's another follow-up — add `caddy` to a
reserved-names blocklist in the validator §3.1 mentions. Out of scope
for this task.)

---

## 8. Backwards-compat — restated and pinned

- Pre-change containers: keep their `json-file` log driver until they
  are next stopped+removed by a redeploy. `decloud logs` works against
  them unchanged. The container's JSON log file lives until `docker rm`
  destroys it.
- The first redeploy of each service after this change ships: new
  container is journald-driven from that point onward. Pre-change
  json-file log history is lost on `docker rm` of the old container,
  which is the bug we're fixing for future redeploys.
- No migration script. No registry version bump. No flag-day. Operators
  see no change to any user-facing command.
- `decloud logs <name> -f` on a freshly-created container: journald
  driver supports follow-mode natively. No behaviour change.

---

## 9. Out of scope — restated from Don §8

- `decloud logs --history` / journalctl wrapper. Logged in
  `_ai/m1x-backlog.md` as a follow-up; DO NOT design it here.
- Log rotation / retention. Journald's own retention; operator's
  problem.
- Migrating pre-change json-file history. Not possible; documented.
- Centralising the service-name validator (§3.1). Worth doing, logged
  as a follow-up; out of scope here.
- A `caddy` reserved-name blocklist (§7). Logged in §3.1's follow-up
  bucket.

---

## 10. Gotchas and landmines

### 10.1 The integration test will silently rot

`internal/integration/mount_test.go` is `//go:build integration`. The
maintainer's macOS box runs `go test ./...` regularly but never sets
`DECLOUD_INTEGRATION=1`. If we forget §6.6 (add `Service: "mounttest"`
to the integration test's `RunRequest`), the rot won't surface until
the next Linux smoke. Mitigation: §6.6 is on Rob's commit checklist,
AND `go build -tags=integration ./...` should be run locally once before
the squash merge. The build will fail-compile the integration test if
the `RunRequest` field set is incomplete? No — Go struct literals don't
require all fields. The integration test will compile fine and only
fail at runtime with `ErrEmptyService`. There is no compile-time
safety net here. Rob: don't forget.

### 10.2 The `strings` import in `cli_driver.go`

Replacing `strings.TrimPrefix(req.Name, "decloud-")` with `req.Service`
at line 64 removes ONE use of `strings`. The package is still used
elsewhere (`strings.TrimSpace` at lines 74 and 190, `strings.Contains`
at line 199, `strings.ToLower` at line 198). Keep the import.

### 10.3 Mock regeneration is a no-op but commit it anyway

Adding `Service` to the struct does NOT change the `Driver` interface
shape, so `mockgen` produces an identical (or near-identical) file. If
the diff is empty, leave the file alone; if it changed
(e.g., whitespace), commit it in the same commit as the struct change.
Don't fight `mockgen`'s output.

### 10.4 Test fixture proliferation

Adding `Service:` to every `RunOptions{Name: "x", …}` test fixture in
`cli_driver_test.go` is mechanical but easy to miss one. Kent: run
`go test ./internal/dockerdrv -run RunWithOptions -v` first to confirm
every existing test fails with `ErrEmptyService` BEFORE you add the
`Service:` field. That is the failing-test step. THEN add `Service:`
and watch them turn green. This catches a missed fixture.

### 10.5 `decloud logs` is unchanged but test it anyway

The journald driver supports `docker logs` natively, so `decloud logs`
keeps working. There are no `decloud logs` test changes in this task.
But Linux smoke: run `decloud logs foo` against a journald-driven
container at least once, post-deploy, before declaring the task done.

### 10.6 `--log-driver` accepts `journald` only if dockerd is under systemd

The docker daemon detects this at container-start time. If it's not
under systemd, `docker run … --log-driver=journald …` returns a clear
stderr error from docker; our driver wraps it in `fmt.Errorf("docker
run: %w; stderr=%q", err, stderr.String())` at `cli_driver.go:72`.
Operator sees the stderr; no special-case handling needed. The
`install.md` doc note (§5.2 of Don's plan) makes the dependency explicit.

### 10.7 Tag with `/` and `journalctl` quoting

`journalctl CONTAINER_TAG=decloud/foo` works without shell escaping in
all common shells (bash, zsh, fish). The `/` is not special. If a future
doc example uses `*` (e.g. `decloud/*`), that's a glob and journalctl
doesn't support that field-value glob syntax — use `_SYSTEMD_UNIT` or
similar for cross-tag queries. Raymond: when you write the docs section
on `journalctl` usage, stick to exact-match queries.

### 10.8 The `tag` value is a Go-template

Docker's `--log-opt tag=` value is processed as a Go template if it
contains `{{` markers. We pass `decloud/<service>` with no markers, so
no expansion. Don't accidentally introduce `{{.Name}}` etc. — Don's
plan explicitly disclaims this and the new
`TestCLIDriver_RunEmitsJournaldFlagsWithSlashTagLiteral` test (§6.2.5)
locks it.

### 10.9 Slash-in-Service is now a DRIVER invariant  *(NEW per Linus §2.1 / Don §1.7)*

`Service` must not contain `/`. The driver guard rejects it with
`ErrInvalidService` BEFORE `cmd.Run`. The invariant is born in this
task — it's the price of writing the journald tag as the byte-literal
`decloud/<service>` (one `/` separator, no others), so
`journalctl CONTAINER_TAG=decloud/foo` is unambiguous under prefix
queries.

What this DOES protect against:
- A future caller of `Driver.Run` / `Driver.RunWithOptions` that
  passes a service name with `/` (e.g. a programmatic deploy API
  added in some later milestone). The guard fires; no docker process
  spawned; no bad tag written to journald; no operator confusion.

What this DOES NOT protect against (deliberately out of scope):
- A caller that passes a service name containing other characters that
  could still produce odd journald behaviour — newline, NUL,
  shell-meta. Today no caller can introduce any of these through the
  normal CLI/TOML flow, and the centralised-validator follow-up
  (§3.1) is the right place to defend the FULL char-set, not the
  driver. The driver's invariant is narrow on purpose:
  "the tag is unambiguous for `journalctl CONTAINER_TAG=` prefix queries."
  Anything broader would be scope creep without an inventoried surface.

What this means for Rob: when implementing §5.2, the guard order
matters — empty first, slash second. See the rationale in §5.2.

What this means for Kent: §6.2.3 and §6.2.4 cover the two paths;
§6.2.1 and §6.2.2 cover the empty case; all four assert
`assert.Empty(t, records)` to prove the guard fires before exec, and
all four assert the OPPOSITE sentinel does NOT match (per §3.3's
two-distinct-sentinels promise).

---

## 11. Where I refined Don's plan

### 11.1 Insertion mechanism: literal, not append; guard now covers BOTH empty and slash  *(REVISED)*

Don wrote `args = append(args, "--log-driver", "journald", "--log-opt",
"tag=decloud/"+req.Service)` after the existing `args := []string{…}`
literal. I'm splicing the four new tokens INTO the literal alongside
`--name`, `--network`, `--restart`. Same byte-identical argv; cleaner
read; tighter signal that "these are fixed flags, not appended-by-loop
flags". §3.2 explains the pattern match. Linus endorsed
literal-splice over append in his review §3.2.

Guard shape: empty-Service check, then slash-in-Service check, then
the args literal. See §5.2. Both checks return BEFORE shelling out.
The two-line guard pair pattern (rather than collapsing to a single
`if !valid(req.Service) { return ErrSomething }`) preserves the
two-distinct-sentinels promise from §3.3; see §11.7.

### 11.2 `ErrEmptyService` message is wordier

Don: `errors.New("dockerdrv: empty Service in RunRequest/RunOptions")`.
Joel: `errors.New("dockerdrv: empty Service field (programmer bug;
populate at every Run/RunWithOptions call site)")`. The audience is the
next developer who hits this in a test failure at 11pm; the wordier
message tells them what to do.

### 11.3 Service-name validation regex DOES NOT exist as code

Don §1.2 wrote "Service names are constrained to `[a-z][a-z0-9-]{0,38}`
(`internal/cli/deploy_service.go:57`, `internal/ids/ids.go:23`)". The
regex appears ONLY in a Cobra flag help string — it is not enforced.
This is a real but small gap. I'm logging a follow-up (§3.1) and NOT
widening this task to fix it. Note this in §9 of Don's next iteration
if he wants — the task as scoped is unaffected.

### 11.4 Integration test was missed in Don's caller sweep

`internal/integration/mount_test.go:69` constructs a `RunRequest{…}`
and was not in Don's §2.2 table. §6.6 captures the edit. The risk is
low (test is gated by `DECLOUD_INTEGRATION=1`), but the edit is required
for the next Linux smoke to pass.

### 11.5 Field placement: between `Name` and `Image`

Don's struct examples (§3.1) placed `Service` between `Name` and
`Image`. I'm explicitly locking that placement in §5.1 with a rationale
("physically co-locate related concepts"). Same outcome; the rationale
is for the next reader.

### 11.6 Use `name` not `prev.Config.Name` in lifecycle.Start

Don §3.4 said `Service: prev.Config.Name`. I'm recommending `Service:
name` (the function arg) instead, for the reason in §5.4 ("two
derivations from the same trust source is cleaner than mixing"). Same
value in practice. Rob can pick either; I prefer `name`. Linus
endorsed `name` in his review §3.3.

### 11.7 Two sentinels not one (REVISION 2)  *(NEW)*

Don's updated plan (§2.1, §3.1) picked Option B and explicitly
asked for `ErrInvalidService` as a sentinel distinct from
`ErrEmptyService`. The alternative — folding both failure modes
under a single sentinel with a wrapped-error variant per case — was
considered and rejected for three reasons:

1. **Test discrimination.** Tests assert that empty `Service` matches
   ONLY `ErrEmptyService` and slash `Service` matches ONLY
   `ErrInvalidService` (§6.2.1–§6.2.4 each carry an
   `assert.False(errors.Is(err, <other-sentinel>))`). Folded under one
   sentinel, that discrimination disappears, and the two failure modes
   become indistinguishable to callers — including future callers that
   might want to handle them differently (e.g. surface
   `ErrInvalidService` as a 400-class API error while
   `ErrEmptyService` is a 500-class).
2. **Stack-trace legibility.** When the guard fires in CI or in a
   future test someone writes against a real `RunRequest`, the
   sentinel name is the first thing the reader sees. "Empty Service"
   and "Invalid Service" are two different bugs; conflating them in
   one error string would force the reader to grep the error message
   for the distinguishing detail.
3. **Symmetry with existing sentinels.** The codebase already has
   `ErrContainerNotFound` and `ErrNoBridgeIP` as distinct sentinels
   for distinct conditions, even though both originate from the same
   `inspect`/`run` family. The "one sentinel per failure mode" pattern
   is the house style; do not break it.

The two sentinels are siblings in the same `var (…)` block, both
prefixed `dockerdrv:`, both with messages that name the failure mode
precisely. See §3.3 for the exact wording and §5.1 for the
declaration site.

---

## 12. Sequencing for Kent and Rob

### Kent (writes the failing tests next)

1. Read this doc end to end, plus Don's §3 and §4.
2. Write the §6.2 new tests against the CURRENT (unchanged) code. They
   must compile (use `RunRequest{Service: ""}`, which is a field that
   doesn't exist yet — so they won't compile YET). Add the
   `Service` field to the struct in `internal/dockerdrv/driver.go` ONLY
   AS A COMPILE FIX (no behaviour). This is acceptable because Don's plan §3.1
   requires the field eventually. Kent: add the field, write the
   failing tests, commit, hand to Rob.
3. Update the §6.1 existing tests to add `Service: "…"` to every
   fixture. They will fail because `ErrEmptyService` is not yet
   defined and the journald flags aren't yet emitted.
4. Update §6.3, §6.4, §6.5 deploy / lifecycle / caddy tests with the
   new assertions. They will fail because production code doesn't
   populate `Service` yet.
5. Commit the failing tests on this branch with conventional message,
   then hand to Rob.

### Rob (implements after)

1. Read this doc end to end, plus the failing test commit.
2. Add `ErrEmptyService` to `internal/dockerdrv/driver.go` (§5.1).
3. Implement the guard + flag emission in `cli_driver.go` (§5.2).
4. Update `service.go`, `lifecycle.go`, `manager.go` (§5.3–§5.5).
5. Update `internal/integration/mount_test.go` (§6.6) so the integration
   build compiles.
6. Run `go generate ./...` and commit any mock-regen diff (§5.6).
7. Run `go test ./...` — everything should be green.
8. Run `go vet ./...` and `go build -tags=integration ./...` to confirm
   the integration test compiles too.
9. Commit with conventional message, hand to Raymond for docs.

---

## 13. File list (final)  *(REVISED — see preamble)*

Code:

- `internal/dockerdrv/driver.go` — add `Service string` to `RunRequest`
  and `RunOptions`; add `ErrEmptyService` AND `ErrInvalidService`
  sentinels.
- `internal/dockerdrv/cli_driver.go` — `Run` and `RunWithOptions`:
  empty-Service guard AND slash-in-Service guard (in that order);
  splice journald flags into the args literal; replace
  `strings.TrimPrefix(req.Name, "decloud-")` with `req.Service`.
- `internal/deploy/service.go` — populate `Service: req.Name` and
  `Service: prev.Config.Name` in the two `RunRequest`s.
- `internal/deploy/lifecycle.go` — populate `Service: name` in the
  one `RunRequest`.
- `internal/caddy/manager.go` — populate `Service: "caddy"` in
  `runOpts()`.
- `internal/integration/mount_test.go` — populate `Service: "mounttest"`
  in the integration `RunRequest`.

Tests:

- `internal/dockerdrv/cli_driver_test.go` — update §6.1 fixtures and
  expected slices; add six new tests
  (§6.2.1 empty-Service `Run`, §6.2.2 empty-Service `RunWithOptions`,
  §6.2.3 slash-Service `Run`, §6.2.4 slash-Service `RunWithOptions`,
  §6.2.5 tag-literal `Run`, §6.2.6 tag-literal `RunWithOptions` [now
  REQUIRED]); extend `TestCLIDriver_StartArgs` with two
  `assert.NotContains` assertions per §6.2.7.
- `internal/deploy/service_test.go` — extend two existing tests
  (§6.3.1, §6.3.2) with `Service` assertions.
- `internal/deploy/lifecycle_test.go` — extend two existing tests
  (§6.4) with `Service` assertions.
- `internal/caddy/manager_test.go` — add `Service: "caddy"` to
  `expectedCaddyRunOptions` (§6.5).

Mocks: `internal/dockerdrv/mocks/mock_driver.go` regenerates from
`go generate ./...`; no hand-edit. Expected diff: trivial or empty.

Docs (Raymond's commit, not Rob's):

- `_docs/usage.md` — three small additions (§2 deploy steps, §4 logs
  annotation, §6 debugging) per Don §5.1.
- `_docs/install.md` — one sentence in §1 per Don §5.2.
- `_ai/decisions/journald-log-driver.md` — NEW per Don §5.3.
- `_ai/m1x-backlog.md` — add Don's deferred entry (`decloud logs
  --history`), and Joel's new entries (centralised name validator;
  caddy reserved-name blocklist).

---

## 14. Hand-off back to Linus  *(REVISED for REVISION 2 — see preamble)*

REVISION 1 (commit `23c7875`) asked Linus to second-guess three
points. He did, in `04-linus-plan-review.md` (commit `8e2ee81`):

1. **§3.1 — validation gap.** Linus agreed with defer (Option A in
   his §2.1) AS LONG AS the slash-rejection invariant is defended
   here. Don's updated plan (commit `e453e43`) picked Option B for
   the slash check; REVISION 2 folds that decision in (see preamble
   for the section map). Centralised validator stays as a follow-up
   in `_ai/m1x-backlog.md`.
2. **§11.1 — literal vs. append.** Linus endorsed literal (his §3.2).
   Settled.
3. **§11.6 — `name` vs. `prev.Config.Name`.** Linus endorsed `name`
   (his §3.3). Settled.

Three additional items from Linus's review folded into REVISION 2:

- **Slash-in-Service guard (Linus §2.1, Don §1.7).** Driver-level
  invariant born in this task; `ErrInvalidService` sentinel added;
  two new rejection tests (§6.2.3, §6.2.4) with the same
  guard-before-exec shape as the empty-Service tests; preamble
  enumerates all section changes; new gotcha at §10.9; new §11.7
  rationale for two distinct sentinels.
- **`docker start` negative test (Linus §5.1).** Folded into §6.2.7;
  acceptance criterion 7 added to §1.4. Two `assert.NotContains`
  assertions on the existing `TestCLIDriver_StartArgs` lock the
  invariant.
- **Caddy tag-literal test promoted to REQUIRED (Linus §5.2).** Was
  §6.2.4 "optional, recommended"; now §6.2.6 REQUIRED. Test body
  spelled out in full to remove any "I'll figure it out" risk for Kent.

REVISION 2 introduces no new decisions Linus has not already weighed
in on. The plan is now the union of Don's updated plan plus this
tech-plan expansion plus Linus's three items. If Linus has a new
objection to anything, fire NEEDS-CHANGES; otherwise this is ready
for Kent.

If Linus disagrees on any file:line citation, the working copy at
HEAD `fb4d026` (or the freshest commit on `task/journald-log-driver`
at review time) is the arbitrator.
