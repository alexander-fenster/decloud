# Hand-off note — integration-test run-log NOT produced here

## Why this file is missing

Joel's addendum-v2 §6 and Don's closeout §3 require a real-Docker run-log
artifact named `integration-test-run-log.txt` in this directory, capturing
the output of `TestIntegration_MountBindRoundTrip` against real Docker.

That artifact **cannot be produced in the current dev environment**:

- The dev box is a Mac (Darwin 25.3.0).
- Docker is not installed and there is no plan to install it on this box.
- The integration test is gated on `DECLOUD_INTEGRATION=1` and shells out
  to `docker` (`exec.CommandContext(ctx, "docker", ...)` plus driver-side
  `docker run`/`docker exec`); it cannot run without a Docker daemon.

Rob (this commit) applied the four-fix delta and verified compile-clean
+ unit-test-clean (`go build ./...`, `go build -tags integration ./...`,
`go vet ./...`, `go test ./...`, `gofmt -l .` — all clean). What he
**could not do** is execute the integration test itself end-to-end. That
is the maintainer's job, on a Linux host with Docker.

## What the maintainer needs to run

From the repo root, on a Linux host with Docker daemon running and
accessible to the current user (`docker ps` succeeds without sudo) and
network reachability to Docker Hub for the `nginx:alpine` pull:

```
DECLOUD_INTEGRATION=1 go test -tags integration -v ./internal/integration/... 2>&1 | tee _tasks/2026-04-28-m2-server-side-mounts/integration-test-run-log.txt
```

The `tee` writes the output to the gating artifact path while also
streaming to the terminal so the maintainer sees progress live.

The host needs:

- Docker daemon running and accessible without sudo.
- Network reachability to Docker Hub for the `nginx:alpine` pull (~22MB).
- The `decloud` Docker network either pre-existing or createable
  (the test calls `NetworkEnsure` which creates it idempotently).

## What PASS line to look for

The log MUST contain:

```
--- PASS: TestIntegration_MountBindRoundTrip (...s)
PASS
ok  	github.com/alexander-fenster/decloud/internal/integration	...
```

And specifically must NOT contain:

```
--- FAIL: TestIntegration_MountBindRoundTrip ...
```

If `--- FAIL:` appears anywhere in the output, **do not commit the log
as a PASS artifact**. Diagnose and re-run; the FAIL is a closeout
blocker, not a paperwork formality.

## Closeout gate

Per Don §3 and Joel §5, the squash-merge of `feat/m2-server-side-mounts`
into `main` MUST NOT happen until:

1. The `integration-test-run-log.txt` file exists in this directory.
2. It contains the PASS line described above.
3. It is committed on the branch (separate commit from the four-fix
   commit that this hand-off accompanies).

Then PLAN re-entry v3 (Don/Joel/Linus closeout vote) proceeds, followed
by FINALIZATION (Ward → Andy → squash-merge).

If the maintainer's first run shows FAIL — for any reason (image-pull
network glitch, Docker daemon misconfiguration, or a real bug surfaced
by the image swap) — re-open the plan rather than retrying blindly. The
v2 path forward is conditional on PASS.

## Why the branch can't be auto-merged from this point

Compile-clean ≠ run-clean. The four-fix commit landing makes the test
*capable* of passing; the run-log proves it *did* pass on real Docker.
Don, Joel, and Linus all signed off on the gate explicitly:

- Don §1 (`012-don-closeout.md`): "the integration test must be
  ACTUALLY RUN against real Docker before close-out."
- Joel §5.1 (`013-joel-tech-plan-addendum-v2.md`): "No PASS log → no
  closeout."
- Linus §2 (`014-linus-addendum-v2-review.md`): "the gate is the
  smallest verification step that's better than zero."

The hand-off is therefore: Rob ships the fixes, the maintainer ships
the run-log on his Linux host, then closeout proceeds.
