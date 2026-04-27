# User Request

Review and fix findings in the following code review feedback; when ready, commit and push.

## Findings

### 1. High: --dockerfile path resolution mismatch

`--dockerfile` is documented and parsed as "relative to <source-dir>", but the implementation
forwards it unchanged to `docker build -f`, which resolves relative paths from the current
working directory instead.

- `internal/cli/deploy_service.go:61` stores `f.Dockerfile` as-is
- `internal/cli/deploy_service.go:88` passes it through
- `internal/dockerdrv/cli_driver.go:35` executes `docker build -f <that> <abs source dir>`

A normal invocation like `decloud deploy service ./myservice` from outside `./myservice`
will therefore look for `./Dockerfile` in the caller's cwd, not in `./myservice`, and fail.

### 2. Medium: --config-root does not control log placement

The command tree resolves the alternate root into `rc.ConfigRoot` and uses it for
registry/Caddy paths, but logging still hardcodes `config.RootFromEnv()`.

- `internal/cli/root.go:22` calls `logging.Init()` with no root
- `internal/logging/logging.go:26` derives the log directory only from `DECLOUD_ROOT` /
  default root

In practice, `decloud --config-root /tmp/testroot ...` will write state under
`/tmp/testroot` but try to write logs under `/opt/decloud/logs` (or the env root),
which is an inconsistent and surprising split.

### 3. Medium: deploy without --port can only fail at runtime

The CLI accepts a deploy with no `--host` and no `--port`, but the deployer always
performs an HTTP readiness probe against `req.Port`, so this shape can only fail at
runtime.

- Validation only rejects "host set without port" in `internal/cli/deploy_service.go:73`
- The deploy path always calls `Wait(..., req.Port)` in `internal/deploy/service.go:213`
- The probe unconditionally builds `http://<ip>:<port><path>` in
  `internal/deploy/readiness.go:55`

So `decloud deploy service --name worker ./svc` gets through argument validation, builds
and runs, then probes port 0 and fails later with a misleading readiness error.

## Process

Follow the workflow in CLAUDE.md. Take no shortcuts. Use subagents for everything.
