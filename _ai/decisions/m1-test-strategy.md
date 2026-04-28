# M1 test strategy: unit tests only, manual receipt as the CI bridge

M1 ships with `go test ./...` as the only automated gate. No integration tests, no real Docker daemon, no real Caddy invocation, no real container runtime, no real network. The tradeoff is recorded here because plan-v2 §2.1 and DONE-criterion #10 cite this file by name; future-Don should not have to reverse-engineer the call.

## 1. The user directive

The maintainer's instruction was explicit: "I will test it on a real system after M1 is done." That moves every `-tags integration` test out of M1 execution scope. Joel's tech-plan §12.2 listed `internal/dockerdrv/integration_test.go`, `internal/caddy/integration_test.go`, and `internal/deploy/integration_test.go`; none of those files exist in the M1 tree. Their replacement is the manual smoke-test the maintainer will run on a real Linux host once the binary lands. That smoke-test is the next milestone's first feedback signal, not an M1 deliverable.

This is not laziness. Integration tests against a real Docker daemon and a real Caddy install slow the inner loop, require platform-specific CI runners, and would not have caught either of the two operator-blocking bugs Kevlin found in iter1 (logging-init mkdir on `--help`, `Capture("")` masquerading as a working default). Both were unit-testable and are now unit-tested.

## 2. What unit-tests-only means per package

Every package has a test seam built specifically so production code paths are exercised in `go test ./...` without invoking the real-world dependency.

- `internal/dockerdrv` — argument-construction tests via an injectable `exec.Command` factory. Every `Driver` method (`Build`, `Run`, `Stop`, `Remove`, `Start`, `Inspect`, `Logs`, `ContainerIP`, `NetworkEnsure`) has at least one test asserting the recorded `*exec.Cmd` arg shape. No real `docker` invocations during `go test`.
- `internal/caddy` — generator output is asserted by golden-string equality on canonical inputs (one host, multi-host, zero hosts, empty input). `Reloader.Validate` and `Reloader.Reload` are exercised through a recording `cmdFactory`. No real `caddy` invocations during `go test`.
- `internal/deploy` — every step of the `Deploy` orchestrator (network ensure, env capture, build, stop-old, run-new, readiness, save, regenerate-and-reload) is mocked via Gomock for `Store`, `Capturer`, `Driver`, `Generator`, `Reloader`. Each lifecycle method (`Unregister`, `Start`, `Stop`, `Restart`, `Status`, `Logs`, `CaddyReload`) has at least one happy-path and one failure test. The HTTP readiness probe runs against `httptest.NewServer` with `Driver.ContainerIP` mocked to return the test server's address.
- `internal/envcap` — runs against the real `/bin/bash` on the maintainer's box. These ARE unit tests (`go test ./...` runs them with no extra tags); macOS bash 3.2 is the portability baseline. The real-bash subprocess is the only "live" thing in any test run. The `compgen -e` + `printf '\0'` mechanism documented in `_ai/envcap-portable-bash.md` is what the test suite locks in.
- `internal/cli`, `internal/registry`, `internal/logging`, `internal/config`, `internal/ids` — pure-Go unit tests with table-driven cases or temp directories.

Mock generation is Gomock via `go generate ./...`. The `tools.go` pin makes the mockgen version reproducible; CI (when it lands) will run `go generate ./...` and fail if the working tree changes, catching unintended mock drift.

## 3. The handoff receipt as the manual-CI bridge

The implementation engineer attaches a ten-item receipt to every implementation report (per plan-v2 §3.4):

1. Go version (`go version`).
2. Host (`uname -a`, `sw_vers` on macOS).
3. Bash version on host (`bash --version | head -1`).
4. Docker version (`docker version --format '{{.Server.Version}}'`) — recorded for future correlation, not used by tests.
5. Caddy version (`caddy version`) — same caveat.
6. Full `go test ./... -v -count=1` output, plus the per-package summary lines.
7. `go vet ./...` output (must be empty).
8. `gofmt -l .` output (must be empty).
9. `go generate ./...` followed by `git status --porcelain` (must be empty other than unrelated bureau pointer files).
10. The list of files modified in the iteration.

The receipt is the M1 acceptance gate. Don/Joel/Linus do not sign off without it. When GitHub Actions arrives post-M1, the workflow file replaces items 1–9 directly; item 10 stays as a habit for the implementation report. Items 4 and 5 are the bridge points where unit-test claims meet real-system reality — when the maintainer reports a real-system failure, the docker/caddy versions in the receipt are the first thing future-Don correlates against.

## 4. What the maintainer is signed up for

The maintainer will run `decloud deploy service` against a real Linux host and report whatever breaks. Expected breakage classes, so future-Don knows where to look first:

- Docker version skew between the maintainer's macOS dev box and production Linux. The argument-construction tests pin the CLI args we send, but a docker version that interprets a flag differently or changes its stdout format is invisible to a stub `exec.Command` factory.
- Bash 3.2-vs-bash-5 differences in env-capture edge cases beyond what `internal/envcap` already tests. The portability tests cover `set +a`, arrays, and readonly conflicts; rarer combinations (`shopt`-driven locale changes, `LC_ALL` interactions with non-ASCII env values) are not covered.
- `caddy validate` and `caddy reload` semantic differences across Caddy versions. The reloader is mocked; a Caddy version that emits a syntactically-valid Caddyfile-but-fails-at-reload-with-a-new-error is invisible to unit tests.
- Network-namespace reachability. The host-side readiness probe assumes the default bridge driver routes from the host's netns to the container's bridge IP. Docker Desktop on macOS supports this via its VM bridge; production Linux supports it natively. A custom network configuration on the host could break the probe path entirely.

When the maintainer reports a real-system failure, the unit-test gap that allowed it through becomes the next milestone's first priority. Add the missing test, then add the integration test that would have caught it earlier, then ship the fix.

## 5. Local test-run tip

`DECLOUD_LOG_TO_STDERR_ONLY=1` short-circuits `logging.Init` before any filesystem access. The iter2 fallback warning (`decloud: log dir unavailable, using stderr only: ...`) prints to `os.Stderr` for every CLI test run on a fresh box; setting the env var silences it. Use this when running `go test ./internal/cli/...` locally if the noise is distracting; CI will prefer the env var so test logs stay clean.
