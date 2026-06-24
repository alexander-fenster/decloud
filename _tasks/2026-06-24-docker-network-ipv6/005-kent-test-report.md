# Kent — Test Report: Docker network IPv6 support

## Summary

Per the approved plan (Don `002`, Joel `003`, Linus `004`), this is a minimal
TDD change: extend ONE existing test to pin the new `docker network create`
argv. No new tests added, no reconcile tests, no optional create-fails test.

## Test file location

`internal/dockerdrv/cli_driver_test.go` — same package `dockerdrv` (white-box),
so the soon-to-exist `decloudIPv6Subnet` const is directly reachable.

## What I changed

EXTENDED `TestCLIDriver_NetworkEnsureWhenAbsent` (was lines 202-215). The
existing assertions are untouched (a `create` call happens; `--driver` is NOT
passed — default bridge required by the readiness probe). Added:

```go
assert.Contains(t, createCall.Args, "--ipv6",
    "fresh network must be created with --ipv6 for container IPv6 egress")

subnetIdx := indexOf(createCall.Args, "--subnet")
require.GreaterOrEqual(t, subnetIdx, 0, "--subnet must be present on create")
require.Less(t, subnetIdx+1, len(createCall.Args), "--subnet must have a value")
assert.Equal(t, decloudIPv6Subnet, createCall.Args[subnetIdx+1],
    "--subnet value must be the decloud ULA const")
```

Also updated the test's pairing comment (the file's house convention, lines
1-4) from `docker network create decloud` to:

```
// docker network create --ipv6 --subnet fd00:dec0:11d::/64 decloud
```

`TestCLIDriver_NetworkEnsureWhenPresent` is UNTOUCHED — it still pins the
no-op-on-existing behavior we preserve.

## Why I reference the const, not a re-typed literal (the called-out decision)

I assert against the package-level `decloudIPv6Subnet` const, not a re-typed
`"fd00:dec0:11d::/64"` string. Rationale:

1. **Single source of truth.** Impl and test share one definition, so a subnet
   typo fails in exactly one place — Joel `003` §3 and the §8 reality-check both
   mandate this precisely to make a typo impossible to silently diverge.
2. **Cleaner failing state for TDD.** Referencing the const makes the test
   COMPILE-FAIL with `undefined: decloudIPv6Subnet` until Rob adds the const —
   an unambiguous "implementation missing" signal, not a confusing assert-fail
   on a value. A re-typed literal would compile and then assert-fail, but it
   would also violate the file's argv↔comment single-source convention and
   require Rob to delete the literal later. The const reference needs zero
   follow-up from Rob in the test.

This is white-box same-package; the const is reachable the moment Rob declares
it. No interface change, no mock regen.

## No new helpers

The positional flag-value idiom (`indexOf(args, flag)` then assert
`args[idx+1]`) already exists and is the established pattern in this file
(`TestCLIDriver_RunEmitsJournaldFlagsWithSlashTagLiteral`, lines 650-661).
`indexOf` is at line 692. No new helper warranted or added.

## Expected-failure output (verified)

`go build ./...` → exit 0 (production code untouched, so the package and both
callers still build).

`gofmt -l internal/dockerdrv/cli_driver_test.go` → empty (clean).

`go test ./internal/dockerdrv/...`:

```
# github.com/alexander-fenster/decloud/internal/dockerdrv [github.com/alexander-fenster/decloud/internal/dockerdrv.test]
internal/dockerdrv/cli_driver_test.go:222:18: undefined: decloudIPv6Subnet
FAIL	github.com/alexander-fenster/decloud/internal/dockerdrv [build failed]
?   	github.com/alexander-fenster/decloud/internal/dockerdrv/mocks	[no test files]
FAIL
```

This is the EXPECTED failure: the test references `decloudIPv6Subnet`, which
does not exist yet. It is the right reason (missing implementation), not an
unrelated breakage. No real Docker daemon is required — `scriptedFactory` stubs
the `docker` binary via `/bin/sh -c`, so this runs on the Docker-less dev box.

## What Rob must implement to make this pass (from Joel `003` §1-2)

1. Add unexported const in `internal/dockerdrv/cli_driver.go`, immediately above
   `NetworkEnsure` (~line 175):
   ```go
   const decloudIPv6Subnet = "fd00:dec0:11d::/64"
   ```
   (with the WHY comment about ULA + NAT66 from the tech plan).

2. Change the create call (currently line 180) to:
   ```go
   "docker", "network", "create", "--ipv6", "--subnet", decloudIPv6Subnet, name
   ```
   Lines 177-179 (inspect + early-return) stay byte-for-byte. Joel's free
   `stderr=%q` completion of the create error-wrap (§7) is also expected here but
   is NOT pinned by a test (deliberately — that surface was trimmed; the
   `stderr=%q` pattern is already exercised by
   `TestCLIDriver_ImagePullPropagatesStderrOnFailure`).

3. (Separate cleanup, Joel §4) `internal/deploy/service.go:159,163`:
   `"decloud"` → `caddy.NetworkName`. Not covered by this test (no behavior
   change; value-identical).

## After Rob implements

`go test ./internal/dockerdrv/...` must go green. The new assertions then pin
the externally-meaningful contract: a freshly created `decloud` network actually
requests `--ipv6` with the ULA subnet as `--subnet`'s immediately-following
token. This survives any refactor preserving that contract and fails only on a
real regression (flag dropped, wrong/typo'd subnet, IPv4-only create). It is a
behavioral assertion, not a change-detector.
