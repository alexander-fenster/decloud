# 02 — Don's Plan: Disable Caddy HTTP/3 (advertise only h1 + h2)

## The request, stated precisely

User: Caddy is advertising HTTP/3 (QUIC over UDP/443). Suspected to break iPhone
Safari. Restrict advertised protocols to HTTP/1.1 and HTTP/2 only — disable HTTP/3.

This is a one-knob change in concept. It is NOT a one-knob change in this codebase,
because the codebase has **two** places that emit Caddyfiles and **one** decision
record that deliberately argued the *opposite* of what the user now wants. Read on.

---

## What I traced (proof, not assumptions)

I traced every path that produces a Caddyfile and every path that opens a network
port. Here is what is actually true today. File:line references are load-bearing —
the implementer will verify each one.

### 1. Caddy's default protocol set

Caddy 2 (`caddy:2`, pinned as `caddy.DefaultImage`, `internal/caddy/manager.go:21`)
enables **h1, h2, and h3** by default on every TLS-enabled server. There is no
per-site directive to turn h3 off; HTTP/3 is controlled by the **global options**
block via:

```
{
    servers {
        protocols h1 h2
    }
}
```

Omitting `h3` from the `protocols` list is the documented mechanism to disable it.
This applies to **all** servers Caddy auto-creates from the Caddyfile. There is no
narrower per-site form in Caddyfile syntax — it is global-options or nothing.

### 2. There is NO global options block anywhere today — VERIFIED

`grep -rn "protocols\|servers {\|global\|h1\|h2\|h3\|quic"` across `internal/`
returns **zero** matches in production code. Confirmed: nothing in this repo
currently emits a global options block. So HTTP/3 is on purely by Caddy default.

### 3. TWO Caddyfile producers — both must change

There are two distinct code paths that write a Caddyfile to disk. A fix that touches
only one leaves HTTP/3 advertised in the other. Both are reached in normal operation.

**Producer A — the generated Caddyfile (`internal/caddy/generator.go`).**
`textGenerator.Generate` (`generator.go:35-49`) writes the file as:
- two header comment lines (`generator.go:38-39`)
- then, per service, per host: a `host { reverse_proxy container:port }` block
  (`generator.go:42-46`).

There is **no** global options block emitted. This is the file in force whenever ≥1
service with a hostname is registered. This is the file real production traffic hits.

**Producer B — the stub (`internal/caddy/stub.go`).**
`stubBody` (`stub.go:10-14`) is:
```
# decloud Caddyfile stub — no services registered yet.
:80 {
    respond "no services registered yet" 404
}
```
This is `:80` only — plain HTTP, no TLS, so **h3 is not in play here** (HTTP/3
requires TLS). The stub is written by `WriteStubIfMissing` (`stub.go:18`) on first
deploy and by `regenerateAndReload` (`internal/deploy/service.go:406`) before every
generate. **The stub does not need the protocols block to fix the bug** — but see
the consistency decision in the Open Questions section.

### 4. How both producers reach Caddy — VERIFIED flow

`regenerateAndReload` (`internal/deploy/service.go:401-424`) is the single funnel:
1. `Store.List` → services (`service.go:402`)
2. `WriteStubIfMissing` (`service.go:406`) — Producer B
3. `Generator.Generate(tmpPath, services)` (`service.go:410`) — Producer A, writes
   `Caddyfile.tmp`
4. `Reloader.Validate(ctx, tmpPath)` (`service.go:413`) → `caddy validate` inside the
   `decloud-caddy` container (`internal/caddy/reloader.go:46-47`)
5. atomic `os.Rename` tmp → `Caddyfile` (`service.go:417`)
6. `Reloader.Reload` (`service.go:420`) → `caddy reload` inside the container.

This funnel is called from THREE entry points:
- deploy success path (`service.go:371`)
- `Unregister` (`internal/deploy/lifecycle.go:34`)
- `decloud caddy reload` → `CaddyReload` (`internal/deploy/lifecycle.go:177-178`,
  CLI at `internal/cli/caddy_reload.go:20`).

So whatever the generator emits is what `caddy validate` checks and what `caddy
reload` activates. If the generator emits a global options block with
`protocols h1 h2`, **all three entry points** disable HTTP/3 with no further work.
That is the leverage point.

### 5. The UDP/443 port publish is a SEPARATE concern — do NOT touch it (yet)

`manager.go:124-145` `runOpts()` publishes six port maps, including:
```
{HostBind: "0.0.0.0", HostPort: 443, ContainerPort: 443, Proto: "udp"},
{HostBind: "[::]",     HostPort: 443, ContainerPort: 443, Proto: "udp"},
```
UDP/443 is the QUIC transport. **Disabling h3 in the Caddyfile stops Caddy from
advertising and serving HTTP/3, which is exactly and only what the user asked for.**
Leaving UDP/443 published but unused is harmless — nothing listens on it inside the
container once h3 is off, so the port is inert. See "Should we also unpublish UDP/443"
in Open Questions. **For this task: leave the port maps alone.** Removing them is a
separate, riskier change (container recreate required; affects `caddy up` not
`caddy reload`) and is not needed to satisfy the request.

---

## The collision you cannot ignore: the decision record argues the OPPOSITE

`_ai/decisions/caddy-runs-in-container.md:17` says, verbatim:

> "UDP/443 is HTTP/3 over QUIC — without it, mobile clients silently fall back and
> the symptom looks like 'TLS works but my phone is slow.'"

And `caddy-runs-in-container.md:47` lists dual-stack publishing (including UDP/443) as
a deliberate consequence.

**The prior team's documented belief was: HTTP/3 helps mobile.** The user's lived
experience is the reverse: HTTP/3 breaks iPhone Safari. Both can be true depending on
network path (QUIC/UDP is frequently blocked, rate-limited, or MTU-broken on real
networks, and broken QUIC with a slow/absent TCP fallback is a classic "my phone
hangs" signature). **We do not get to silently contradict a written Decision.** This
task MUST update that decision record so the next engineer doesn't relitigate it.
That is a documentation deliverable, not optional. See Raymond/Ward deliverables below.

This is the single most important non-code item in this plan. If the implementer
"just adds the protocols line" and ships, the decision record now lies, and the eighth
person to touch Caddy will reintroduce h3 thinking they're fixing a mobile regression.

---

## The plan

### Design decision: emit the global options block in the GENERATOR (Producer A)

**Chosen mechanism:** prepend a global options block to the generated Caddyfile in
`textGenerator.Generate`, emitted unconditionally and exactly once, before any site
block:

```
{
    servers {
        protocols h1 h2
    }
}
```

**Why the generator and not the stub:**
- The generator's output is the file that carries real TLS traffic. The stub is
  `:80` plaintext where h3 cannot exist anyway.
- One emission point, exactly once, ordered first — Caddyfile global options MUST be
  the first block in the file or `caddy validate` fails. The generator already
  controls byte order of the whole file (`generator.go:37-48`), so this is a clean,
  single insertion right after the header comments and before the per-service loop.

**Why a literal, not a config knob:** M1/M2 has no Viper/TOML config surface yet (the
decision record §"Forward-looking notes" defers config to M3). Adding a flag now is
scope creep. Hardcode `protocols h1 h2` as the generator's behavior. When M3 lands a
config file, the protocol set becomes a knob — note that as a forward-looking item,
do not build it now.

**Empty-registry case:** `TestGenerator_EmptyInputProducesHeaderOnly`
(`generator_test.go:81-86`) asserts the empty output is header-only and contains no
`reverse_proxy`. Decide deliberately: **do we emit the global block even with zero
services?** A global options block with zero site blocks is valid Caddyfile syntax
and harmless. Recommendation: **yes, always emit it** — it keeps the generator's
output shape invariant and means the protocols guarantee holds the instant the first
service appears. The implementer must update that existing test accordingly (it is a
legitimate contract change, not a change-detector hack — the contract genuinely
changed). This is flagged for Joel to spec and Kent to test.

### Files that change

1. **`internal/caddy/generator.go`** — `textGenerator.Generate`: insert the global
   options block once, after the two header `Fprintln` lines (`generator.go:38-39`)
   and before the `for _, in := range inputs` loop (`generator.go:40`). Mind the
   blank-line formatting so the output stays clean and `gofmt`-irrelevant (this is
   generated text, not Go source, but keep it tidy — Caddy's `fmt` is the real judge).

That is the ONLY production code file that must change to fix the bug. Everything
else below is tests and docs — which are NOT optional under our standard.

### Tests (Kent)

In `internal/caddy/generator_test.go`:
- **New test:** generated Caddyfile contains the global options block with
  `protocols h1 h2` and does **not** contain `h3`. Assert the block appears **before**
  the first site block (ordering is a correctness requirement, not cosmetic).
- **Update `TestGenerator_EmptyInputProducesHeaderOnly`** to reflect the deliberate
  decision above (global block present even when empty, still no `reverse_proxy`).
- Existing tests `TestGenerator_OneServiceOneHost`,
  `TestGenerator_MultiServiceMultiHostSorted`,
  `TestGenerator_DropsZeroHostnameServices` must still pass unchanged — the site
  blocks are unaffected. Verify they do.

**Validation gap to be honest about:** none of these unit tests run real `caddy
validate` — there is no Docker on the dev box (see MEMORY: "No Docker on this Mac").
The unit tests assert on emitted bytes only. **Real `caddy validate` against the
generated Caddyfile is a manual/integration step the maintainer runs on the Linux
host.** Call this out in the Rob/Raymond reports so it is not mistaken for verified.
The byte-level assertion that the block is first + well-formed is our proxy; it is not
a substitute for `caddy validate`. Do not claim the Caddyfile is "validated" anywhere
in docs.

### Docs (Raymond)

1. **`_ai/decisions/caddy-runs-in-container.md`** — MANDATORY. Add a follow-up
   note/amendment recording that HTTP/3 is now **disabled** at the Caddyfile level via
   `protocols h1 h2`, that this directly reverses the line-17 reasoning ("without
   UDP/443 mobile clients fall back…"), and **why**: real-world iPhone Safari over
   QUIC/UDP-443 was breaking connectivity for this operator; broken QUIC with slow TCP
   fallback presents as "my phone hangs." Note that UDP/443 remains **published but
   inert** (no listener once h3 is off) and that unpublishing it is a deferred,
   separate change. This closes the contradiction so #8 doesn't reopen it.
2. **`_docs/usage.md` and/or `_docs/install.md`** — if either documents the
   advertised protocol set or QUIC/UDP-443 behavior, update it. Raymond greps these
   first (per `_ai/doc-grep-discipline.md`) and only touches what's actually wrong.
   Do NOT hallucinate a config flag that doesn't exist.

### Knowledge (Ward, finalization)

Capture the reusable lesson: **HTTP/3 in Caddy is a global-options-only knob; there
is no per-site form; the generator is the single leverage point because it funnels all
three reload entry points.** And the meta-lesson: a Decision record can become wrong
when the field disproves its premise — amend it, never silently contradict it.

---

## Acceptance criteria

1. A generated Caddyfile (≥1 service) begins with a global options block containing
   `servers { protocols h1 h2 }`, placed before any site block.
2. The generated Caddyfile contains no `h3` token.
3. Per-service `reverse_proxy host:port` blocks are byte-identical to today (modulo
   the prepended global block and its trailing blank line).
4. All existing generator tests pass; the empty-input test is updated for the new,
   deliberate contract.
5. `internal/caddy/stub.go` is unchanged (it's `:80`, h3 is impossible there) UNLESS
   the implementer makes a deliberate, documented call to also add the block for
   uniformity — default is leave it.
6. `manager.go` `runOpts()` port maps are UNCHANGED (UDP/443 stays published; inert).
7. `_ai/decisions/caddy-runs-in-container.md` carries an amendment reconciling the
   reversal. The repo no longer contains a Decision that contradicts shipped behavior.
8. `go test ./...` is green.
9. Manual `caddy validate` + a real iPhone Safari check is performed by the maintainer
   on the Linux host (cannot be done here; flagged, not claimed).

---

## Open questions for Joel / Linus

1. **Emit global block on empty registry — yes or no?** I recommend yes (invariant
   output shape). Decide and lock it; it drives the test change.
2. **Should the stub (`:80`) also carry the block for uniformity?** I recommend no —
   h3 can't exist on plaintext :80, and touching the stub means updating
   `stub_test.go` for zero functional gain. But name the decision.
3. **Should we also unpublish UDP/443 in `runOpts()`?** I recommend **not in this
   task.** It requires a container recreate (`caddy up`), not just a reload; it's a
   bigger blast radius; and disabling h3 in the Caddyfile already fully satisfies the
   request. File it as a follow-up if the operator wants the UDP port closed for
   firewall-surface reasons. Leaving it inert is the conservative, correct M2 move.

---

## Research facts for downstream agents (code pointers)

- Generator: `internal/caddy/generator.go` — `textGenerator.Generate(outPath, services)`
  at `:35`; header lines `:38-39`; per-host site block loop `:40-47`; `writeFileAtomic`
  `:80`. **Insertion point: between `:39` and `:40`.**
- Stub: `internal/caddy/stub.go` — `stubBody` `:10-14` (`:80` plaintext);
  `WriteStubIfMissing` `:18`.
- Reload funnel: `internal/deploy/service.go` — `regenerateAndReload` `:401-424`.
  Entry points: deploy `:371`; unregister `internal/deploy/lifecycle.go:34`;
  `CaddyReload` `internal/deploy/lifecycle.go:177-178`; CLI
  `internal/cli/caddy_reload.go:20`.
- Reloader: `internal/caddy/reloader.go` — `Validate` `:46`, `Reload` `:50`, both
  `docker exec caddy <sub> --config <ctrPath>` `:60-64`.
- Manager / ports: `internal/caddy/manager.go` — `DefaultImage = "caddy:2"` `:21`;
  `runOpts()` `:124-145`; UDP/443 maps `:137-138`. **Do not touch in this task.**
- Tests: `internal/caddy/generator_test.go` (esp. `TestGenerator_EmptyInputProducesHeaderOnly`
  `:81-86`); `internal/caddy/stub_test.go`.
- Decision to amend: `_ai/decisions/caddy-runs-in-container.md` — line 17 (QUIC-helps-mobile
  claim) and line 47 (dual-stack consequence).
- Mechanism reference: Caddy global options `{ servers { protocols h1 h2 } }`,
  must be the first block in the Caddyfile; omitting `h3` disables HTTP/3 for all
  auto-created servers. No per-site equivalent exists.

---

## Don's bottom line

The code change is four lines of emitted text in one function. The reason this task is
NOT trivial is the decision record at `caddy-runs-in-container.md:17` that explicitly
calls HTTP/3 a mobile *benefit*. Ship the code without amending that doc and you've
planted a lie that someone will "fix" by turning h3 back on. The doc amendment is as
load-bearing as the generator change. Do both, or it isn't done.

And nobody claims this is "validated" until a human runs `caddy validate` and loads it
on a real iPhone over the real network. We have no Docker here. Byte assertions are our
proxy, not our proof.
