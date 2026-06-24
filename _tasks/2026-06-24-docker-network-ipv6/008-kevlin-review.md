# Kevlin low-level review — `docker-network-ipv6`

Reviewed: `git diff main...HEAD`, full task report set, and the live source.
Build, `go vet`, `gofmt -l`, and `go test ./internal/dockerdrv -run NetworkEnsure`
all pass on this box (no Docker needed — `scriptedFactory` stubs the daemon).

**VERDICT: CHANGES REQUESTED** — one real correctness/consistency finding
(incomplete consolidation), no doc hallucinations.

---

## DESIGN CLARITY: ⚠️ ADEQUATE
## SIMPLICITY: ✅ MINIMAL
## COMMUNICATION: ✅ CLEAR (docs are accurate)

The core change is genuinely minimal and correct: `NetworkEnsure` gains
`--ipv6 --subnet <const>`, captures create stderr (consistent with how
`ContainerIP`/other call sites in this file already wrap `stderr=%q`), and the
inspect early-return is byte-for-byte unchanged. The const is unexported and
lives in `dockerdrv` where it belongs. No `--driver`, IPv4 left auto-allocated —
`ContainerIP` reads `.NetworkSettings.Networks.decloud.IPAddress` (the v4 field),
verified unchanged. Good.

---

## CRITICAL ISSUE — incomplete literal consolidation (CHANGES REQUESTED)

The diff swaps **two** `"decloud"` literals in `internal/deploy/service.go`
(lines 159 `NetworkEnsure` arg, 163 log field) for `caddy.NetworkName` and the
`_ai` amendment calls this "pure consolidation onto the single source of truth."

But **three** structurally identical literals naming the *same* network remain
in the *same* file, untouched:

- `internal/deploy/service.go:254` — `RunRequest{… Network: "decloud" …}`
- `internal/deploy/service.go:324` — `registry.RunSpec{… Network: "decloud" …}`
- `internal/deploy/service.go:387` — `RunRequest{… Network: "decloud" …}`

These set the network a container is *attached to* — i.e. exactly the network
`NetworkEnsure(ctx, caddy.NetworkName)` just created. They MUST agree with it.
This is the precise wart Linus described in `004-linus-plan-review.md` §4 when he
justified the cleanup: *"Leaving a raw `\"decloud\"` literal … apart from the
place that's supposed to be canonical is exactly the wart that bites the next
person who renames the network."* That argument applies with full force to these
three; he only pointed at 159/163 because they were adjacent to `NetworkEnsure`,
not because the `RunSpec` ones were deliberately scoped out. His §2 note that
`RunSpec.Network` "controls *which* network" is the very reason it must track the
canonical name.

`caddy` is already imported (line 12); `caddy` does not import `deploy` — swapping
these three is zero-risk, value-identical, no cycle, same edit that was already
made twice. Either the consolidation claim is true and these three become
`caddy.NetworkName`, or the claim in the `_ai` amendment is overstated and should
be softened. The former is correct — make the three edits.

(Strictly: the IPv6 behavior is correct as-is; this is a consistency/maintenance
defect, not a runtime bug. But the task's stated correctness argument is "single
source of truth for the network," and the diff only half-delivers it in the one
file it touches. Fix it now while the file is open.)

---

## TEST QUALITY: ✅ real behavioral check, not a change-detector

`TestCLIDriver_NetworkEnsureWhenAbsent` pins the right things at the right grain:
- asserts `--ipv6` is present (behavior: IPv6 enabled),
- finds `--subnet` by index and asserts the **following** arg equals
  `decloudIPv6Subnet` — this is positional `--subnet <value>` pairing, the
  correct way to verify a flag/value pair without pinning whole-argv order,
- keeps the pre-existing `--driver`-absent guard.

It uses `indexOf` (existing helper, line 701), `require` for the
must-hold-before-indexing preconditions, `assert` for the leaf checks. No
`if`-branching, no whole-slice equality (which *would* be a change-detector). The
const is referenced rather than re-spelled, so the test can't drift from the
production value. The lead comment was updated to match. Good.

---

## DOC REVIEW (my special charge) — NO HALLUCINATIONS FOUND

Verified every command/flag/path/behavior claim against source:

- **No auto-upgrade implied — PASS.** `install.md` §3.3 and the troubleshooting
  entry, `usage.md` §0, and both `_ai` files all state plainly and repeatedly
  that an existing network is a strict no-op and is NOT auto-upgraded. This
  matches the code: `docker network inspect` success → bare `return nil`. No doc
  text claims `EnableIPv6` is toggled on an existing net. Correct and emphasized.
- **Subnet / flags — PASS.** Docs say `--ipv6 --subnet fd00:dec0:11d::/64`;
  code creates exactly `"docker","network","create","--ipv6","--subnet",
  decloudIPv6Subnet, name` with `decloudIPv6Subnet = "fd00:dec0:11d::/64"`.
  `fd…` is a valid RFC 4193 locally-assigned ULA (`fd00::/8`), `/64` standard.
- **Upgrade procedure — PASS.** `caddy down` → `docker network rm decloud` →
  `caddy up`. `caddy down` exists and detaches the container; `network rm`
  genuinely refuses while endpoints remain (docs correctly warn to stop service
  containers first); `caddy up` calls `NetworkEnsure` (manager.go:66) which now
  recreates IPv6-enabled. Verification step `docker network inspect decloud` →
  `EnableIPv6 true` + subnet present is accurate. The "brief outage" framing is
  honest.
- **NAT66 / ip6tables / Docker 27 — PASS, appropriately hedged.** Docs say egress
  works "via NAT66/masquerade … on hosts where Docker's `ip6tables` is on
  (default in Docker 27+)" and explicitly note that if disabled the network is
  still created but egress won't work. This is the correct boundary: the Go code
  only guarantees the `--ipv6` flag; the daemon NAT66 prerequisite is ops, not
  code. `ip6tables` default-on in Docker 27 is accurate and not overstated
  (every claim is conditioned, never promised). "Egress only / inbound
  terminates at Caddy / ULA routed nowhere off-host" matches the architecture.
- **`ContainerIP` unchanged claim — PASS.** Docs say IPv4 addressing and the
  readiness probe are unaffected; code confirms the probe reads the v4
  `IPAddress` field which Docker keeps populated.
- Cross-references (`#33-the-decloud-network-and-ipv6`, `install.md#…` from
  `usage.md`) resolve to the heading slugs added in the same diff.

No incorrect field names, no invented flags, no command that doesn't exist.

---

## REQUIRED CHANGES

1. Replace the three remaining `Network: "decloud"` literals in
   `internal/deploy/service.go` (lines 254, 324, 387) with `caddy.NetworkName`,
   OR soften the "single source of truth / pure consolidation" wording in
   `_ai/decisions/caddy-runs-in-container.md` and `_ai/MEMORY.md` to reflect that
   only the `NetworkEnsure` call site was consolidated. The edit is the better
   fix — zero risk, no new import, no cycle.

Everything else (driver code, stderr capture, const, comment rationale, test,
all docs) is approved.
