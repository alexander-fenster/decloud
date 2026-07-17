# HTTP compression on by default, with a per-service opt-out

Originating task: `_tasks/2026-07-17-caddy-http-compression/`. Plan: `002-don-plan.md` (rev 2). Tech plan: `003-joel-tech-plan.md` (rev 2). Linus approvals: `004-linus-plan-review.md`, `005-linus-plan-rereview.md`.

## Context

The generated Caddyfile emitted exactly one line per site block: `reverse_proxy <container>:<port>`. Nothing was compressed, ever, for any service. The obvious fix — turn compression on — is obvious enough that the interesting part of this decision is not "should we", it is the two things that bite afterwards: whether a knob was justified, and what the knob actually buys.

## Decision

**Every generated site block gets `encode zstd gzip`, using Caddy's default `match` list and default `minimum_length` (512).** Per-service opt-out via `ServiceConfig.DisableCompression` (`toml:"disable_compression"`), surfaced as `decloud deploy service --no-compression`.

| TOML | Value | Behavior |
| --- | --- | --- |
| *(absent — every service file written before this change)* | `false` | **compression ON** ← the default |
| `disable_compression = false` | `false` | compression ON |
| `disable_compression = true` | `true` | `encode` omitted from that service's site blocks |

The polarity is load-bearing. `false` must mean the default behavior, because an absent key unmarshals to `false` and **every existing service TOML lacks the key**. Naming it `compression` or `enable_compression` would make the zero value mean "off" and would silently disable compression for every service already on disk. Same backward-compatible-additive-key shape as `LastDeployedAt`.

`encode` is **site-level only** — there is no global-options `encode`. See `_ai/caddyfile-generator-facts.md` for the mechanics.

## Why on by default

Compression on HTML/JS/JSON/CSS is a 60–80% saving on the wire, and Caddy's own defaults handle the cases people worry about:

- **Already-compressed content** — the default `match` is an allow-list (`text/*` and friends), not a deny-list. JPEG, PNG, MP4, `.zip` never match. We do **not** write our own match list; ours would be a worse copy of one upstream maintains.
- **Tiny responses** — `minimum_length = 512` skips them.
- **Double compression** — a backend that sets its own `Content-Encoding` is passed through untouched.
- **Ranges / ETags** — `Accept-Ranges` is dropped and `206` disables encoding, so byte-range video is unaffected; ETags get the encoding suffix per RFC 9110 §8.8.3.3.
- **CPU** — single-digit milliseconds on text at Decloud's scale (a single-host PaaS for personal projects). `minimum_length = 512` skips the trivia, and we are trading those milliseconds against 60–80% fewer bytes on the wire. zstd is faster than gzip at comparable ratios. The trade is overwhelmingly positive; no compression-level or `minimum_length` tuning until a profile says otherwise.
- **WebSockets** — unaffected, and by a stronger guard than "a hijacked connection never writes through the wrapper" (true, but not the real mechanism): `AcceptedEncodings` skips non-identity encodings when the request carries `Sec-WebSocket-Key` (`encode.go:502,532-535`), so **the wrapper is never installed at all** on an upgrade request. Note this is a *request*-header check, consistent with everything else in this section.

Sane defaults are the house philosophy. An operator should not opt in to the obviously right thing.

## Why `zstd gzip` in that order — and why the HTTP/3 scar does not transfer

`encode zstd gzip` sets `Prefer = [zstd, gzip]`: where both are on offer, zstd wins. It is faster than gzip at comparable ratios, so preferring it is a free win.

**The question this repo will actually ask is whether that is safe**, because the one Caddy protocol change in this project's history broke a client: `_ai/decisions/caddy-runs-in-container.md` (Amendment 2026-06-10) records iPhone Safari negotiating **HTTP/3 over QUIC/UDP-443 breaking connectivity**, which is why the previous commit disabled h3. Anyone touching `encode zstd gzip` should ask "we broke Safari with a Caddy protocol change once — does preferring a newer encoding do it again?" **It does not, and the contrast is the reason:**

- **h3 broke Safari because Caddy *advertises* it.** `Alt-Svc: ...h3...` is pushed at the client regardless of whether that client's QUIC path actually works. The client is *told* to try something the server cannot know is broken. **Server-driven.**
- **Compression cannot do that, because encoding is *negotiated*.** A client only ever receives an encoding it advertised in its own `Accept-Encoding`. zstd is Chrome 123+, Firefox 126+, Safari 18.4+; every older client transparently gets gzip, and one that offers neither gets identity. **Client-driven.**

**Opposite mechanisms, opposite risk.** Preferring zstd carries no client-compatibility risk, and **the h3 precedent does not transfer** — the two changes look similar ("newer protocol first in a Caddy directive") and are structurally nothing alike. `_ai/caddyfile-generator-facts.md` documents the `Alt-Svc`-is-gated-on-the-listener half of this; together they close the question.

(No brotli: not a built-in Caddy core encoder — `Prefer` lists `br` only if a pool is registered, and a stock build has none.)

## `Vary: Accept-Encoding` is handled upstream — checked, no action

This is the first question any competent reviewer asks about compression behind a cache ("will an intermediary serve a gzipped body to a client that never asked for it?"), so the answer is recorded rather than re-derived: **upstream gets it right.** `encode.go` adds `Vary: Accept-Encoding` in `init()` when it encodes, guarded against duplicating an existing one, and handles the `304 Not Modified` case separately (per RFC 9110 §15.4.5 a 304 must carry the header as if it were a 200, which the body-write path alone would miss, because that path only reaches `init()` once a body is actually written). No code, no config, no action.

## BREACH and CRIME were considered and rejected as reasons — strongest argument first

**CRIME first, because it is the fastest to dismiss and the most likely to be raised.** CRIME attacked **TLS-level** compression (and SPDY header compression) — a layer that does not exist in TLS 1.3 and is disabled in TLS 1.2 deployments. Caddy's `encode` is HTTP **response-body** compression. **Different layer.** Citing CRIME against `encode` is cargo-cult security, and the per-service knob is irrelevant to it — there is nothing for the knob to switch off, because Decloud never enabled TLS compression in the first place. It is recorded here only because "compression over TLS" reliably summons it, and an unanswered objection gets re-litigated forever.

BREACH is the real one, and it still does not justify a knob:

**The knob is useless against BREACH. This argument alone decides it.** A per-service toggle only helps if the operator knows their app reflects attacker-controlled input next to a secret in a compressed response. If they knew that, they would fix the reflection. Shipping a toggle nobody can correctly set is security theatre.

Supporting, in descending weight:

- **It is an application-layer bug.** The mitigations are all app-side: token masking, per-request rotation, not reflecting input beside secrets. **A reverse proxy cannot fix any of them.**
- **The entire industry ships compression on by default** — nginx stock configs, every Caddy tutorial, Cloudflare for every site it fronts. BREACH has been public since 2013 and nobody's answer was "turn off gzip". *This is an appeal to authority and is labelled as one; it is supporting evidence, not the reason.*

BREACH needs **all** of: a compressed response over TLS, **and** a secret in the body, **and** attacker-controlled input reflected into that same response, **and** many observable cross-origin cookie-bearing requests. A compressed response alone is not enough.

**On `SameSite`, stated carefully, because the sloppy version is wrong.** `SameSite=Lax` (the default in current browsers) **still sends cookies on top-level cross-site GET navigations.** It raises the bar on the last condition **considerably**; it does **not** eliminate it. An earlier draft said Lax "guts" that condition — that was an overstatement and is corrected here. Do not restore it: a claim a security-literate reader can knock down in one sentence discredits the correct reasoning around it, and the argument above is airtight without it.

**Honest statement:** BREACH remains theoretically live for applications that reflect attacker-controlled input next to a secret in a compressed response; modern cookie defaults raise the bar considerably; the mitigation is, and always was, app-side.

## Why the knob exists: streaming, not SSE

Say **"streaming"**, not "SSE". The failure is **headers-then-idle** and nothing about `Content-Type: text/event-stream` selects it — the swallow fires before Content-Type is ever consulted. Long-poll and chunked progress streams are in the same blast radius. SSE is merely the most common shape (and in 2026, the default transport for LLM token streaming — exactly the "personal projects on a small PaaS" workload).

The mechanism, and why no Caddyfile we could generate fixes it:

1. Caddy installs its encoding `responseWriter` from the **request headers alone** (`encode.go` `ServeHTTP`) — the request's `Accept-Encoding`, gated by `isEncodeAllowed` on request `Cache-Control: no-transform` and by a `Sec-WebSocket-Key` check. All request-scoped: **the response's Content-Type is not knowable yet.** Every browser sends `Accept-Encoding`, including `EventSource`.
2. That wrapper swallows a pre-body flush: `FlushError()` → `if !rw.wroteHeader { return nil }`.
3. So a backend that writes `200 text/event-stream`, flushes, and *then* waits for its first event has its flush **silently dropped**. The client's `EventSource` `onopen` does not fire until the first body byte arrives; an idle-first stream hangs and can hit client/intermediary timeouts.

**`match` cannot fix this** (step 1 precedes any Content-Type check — see `_ai/caddyfile-generator-facts.md`). **Omitting `encode` from the site block is the only fix.** That is the entire justification for the knob. Not BREACH. This.

**Upstream's own structure proves the point.** `init()` re-checks `isEncodeAllowed` against the **response** header (`encode.go:457`), so a backend that sets `Cache-Control: no-transform` on its response stops the *compression* — but **not the wrapper**, which was installed from the request long before that header existed. Every response-side lever, ours or the backend's, is downstream of the wrapper. Only not emitting `encode` is upstream of it. That asymmetry is the strongest single piece of evidence for this design, and it is why no clever response-side configuration can substitute for the knob.

Note the honest severity: Caddy's `FlushError()` *does* now sync-flush the encoder before flushing the underlying writer, so "SSE is completely broken" would be **wrong** — events do reach the client. The surviving defect is the header-flush delay in step 3.

### The knob buys ZERO bytes — it fixes header timing, not compression

Counter-intuitive and easy to lose, so it is recorded explicitly. `init()` is called from `Write` (and, on the bodyless path, from `Close`); from `Write` it runs only when the first write exceeds the 512-byte `minimum_length` (strictly `>`) **or the response declares a `Content-Length` above it**, and it is **never retried** once `wroteHeader` is set — both call sites are gated on `!rw.wroteHeader`. Typical stream events are far under 512 bytes and a chunked stream declares no `Content-Length`, so a typical streaming service runs with `rw.w == nil` — **uncompressed, forever, whether or not you pass the flag.**

**Therefore `--no-compression` saves a streaming user no bytes at all. It purely fixes header timing.** That is a strange shape for a flag named "disable compression", and it is exactly the kind of fact that gets a feature deleted by someone being diligent: a future engineer measures streaming responses, observes they were never compressed anyway, concludes the knob is dead weight, removes it, and reintroduces the hang. The knob's justification was never bytes — it is the swallowed flush, which is installed regardless of whether compression ever engages.

## The rule that generalizes: when a knob earns permanent config surface

The anti-knob argument was made at full strength and it is a good one: every config field is forever; this is not even our bug, it is a third party's unfixed defect permanently encoded into our public config surface; no user has hit it; YAGNI. **In most reviews that argument wins.** It loses here, and the reason generalizes:

> **A knob earns permanent config surface when the default has a known failure that is (i) silent, (ii) misattributed, and (iii) unworkaroundable. If the failure is loud — a failed deploy, a 502, an obvious error — kill the knob and wait for a real bug report.**

This case is 3-for-3:

- **Silent** — no 500, no failed deploy. Builds, deploys, passes readiness, looks healthy.
- **Misattributed** — the user blames their app, their browser, their network. They will **not** suspect a reverse proxy they never configured. This is what destroys the "just wait for a real bug report" strategy: the report never arrives correctly diagnosed.
- **Unworkaroundable** — the generated file says `do not edit by hand` and is atomically overwritten on every deploy *and* every `decloud caddy reload`. This is self-hosted: **the user *is* the operator and still cannot fix it.**

Plus: upstream has not fixed it in two years. Waiting is not a strategy.

**The rule is what tells future-us whether the *next* proposed knob is legitimate engineering or cowardice** — and it is also the rule that would have killed this one had the failure been loud.

## Redeploy resets the flag, and warns (Option C)

`decloud deploy service` builds a fresh `ServiceConfig` from the `Request` and never merges the previous config, so **omitting `--no-compression` on a redeploy resets `disable_compression` to `false`** — exactly like `--mount`, `--strategy`, and `--readiness-path`. The declarative house rule is preserved.

**But it warns.** Condition (`internal/deploy/service.go`, adjacent to the config construction, after the readiness gate):

```go
hasPrev && prev.Config.DisableCompression && !req.DisableCompression
```

→ `logger.Warn("compression re-enabled: previous deploy set disable_compression; pass --no-compression to keep it off")`

Both tokens are load-bearing: `--no-compression` is the fix, `disable_compression` is what the operator sees in the TOML. The message deliberately does **not** interpolate the service name — `logger` already carries `"service"` as an attribute, and the output is a JSON object. `Warn` clears the `Info` level filter, so it reaches both stderr and the log file.

**Why warn rather than make it sticky.** Per-field merge semantics are how a config surface rots into "which of these flags are sticky?" — a question with no good answer, asked forever, by everyone. Option C changes no semantics; it makes a state transition *we already compute* visible. That is the cheapest possible conversion of a silent failure into a loud one.

**Why warn at all, when `--mount` doesn't.** The consistency argument ("every other flag resets silently") is true and is not the point. The *consequence class* differs: forget `--mount` and the app can't find its file — loud, you know in seconds. Forget `--no-compression` and the deploy is clean, readiness passes, it looks healthy, and then streams hang for some clients — **silent and misattributed**, which is the precise failure class that justified building the knob. An escape hatch that silently closes itself and restores the exact bug it exists to prevent is not an escape hatch.

### Known false negative — accepted, do not "fix" it

The warning depends on `hasPrev`, so **a previous config that fails to load produces no warning** — including the non-fatal `ErrSecretsMissing` path in `Deploy`'s previous-registration load (`internal/deploy/service.go`, the `loadErr != nil && !errors.Is(..., ErrNotFound) && !errors.Is(..., ErrSecretsMissing)` guard), which falls through with `hasPrev == false` because `Load` returns `nil` on every error path. A service whose secrets dir was deleted, and which previously had `disable_compression = true`, redeploys with no warning: a false negative in the exact case the warning exists for.

**Deliberately not fixed.** Warning there would require a config-only load purely to feed a log line, on a path where the service is already broken and the deploy is effectively a fresh registration. New load, new code path, for a warning in a state almost nobody reaches. Wrong trade. Recorded so nobody files it as a defect in a year and "fixes" it.

## Why `streaming = true` was rejected as a name

It is the name someone proposes in six months, so: it would **promise semantics we do not implement.** We do not tune `flush_interval`, do not touch `reverse_proxy`, do nothing else streaming-shaped. `streaming = true` would be a lie in the config file. `disable_compression` states what it mechanically does — omit `encode` — and stays coherent even after caddy#6293 is fixed.

Also rejected, so nobody proposes it later: `request_header -Accept-Encoding` in the site block *would* also suppress the wrapper (`header` sorts before `encode` in Caddy's directive order). It is strictly more obscure than simply not emitting `encode`. **Omitting the directive is the correct, obvious answer. Don't get clever.**

## Retirement condition

**This knob exists because of an upstream bug.** If [caddy#6293](https://github.com/caddyserver/caddy/issues/6293) is fixed upstream — specifically, if the pre-body flush is no longer swallowed — `disable_compression` loses its justification and **should be revisited**. Status at time of writing: **open**, created 2024-05-02, last updated 2026-03-17, no merged fix. Undocumented knobs are how config surfaces rot; this paragraph is what tells future-us exactly what would retire the field.

Retiring it means deleting a public config key, so it is a deprecation, not a delete. But the *reason* to keep it would be gone, and that is the thing worth knowing.

## The downgrade hazard

`fsStore.Load` uses a strict decoder (`DisallowUnknownFields()`). A TOML written by a new binary carrying `disable_compression` and then read by an **old** binary fails with `ErrUnknownField` → exit 10. **There is no supported downgrade path**, and `schema_version` deliberately stays `1` — an optional additive key whose zero value is the old behavior is not a schema break, and bumping the version would force a migration for nothing.

## Live behavior change on upgrade

**Existing services gain compression on the next `decloud caddy reload` or deploy.** The Caddyfile is regenerated from registry state, existing TOMLs have no `disable_compression` key, and an absent key means compression on. This is intended — it is the whole point of the polarity — but it *is* a live change to running services. A streaming service that was fine yesterday can regress; `--no-compression` is the fix. Documented in `_docs/usage.md` under `decloud caddy reload`.

## Non-goals

- No global-options `encode` (it does not exist — site-level only).
- No custom `match` list, no `minimum_length` tuning, no compression levels, no `prefer` tuning. Upstream defaults until a profile says otherwise.
- No brotli (not a built-in Caddy core encoder).
- No per-hostname granularity — `Route` is `{Hostname}` and every route of a service proxies to the same port on the same container, so a per-hostname flag could only ever be set identically for every hostname of a service. **Per-service is the correct unit**, which is why the original "per host" framing resolves to per-service.
- No attempt to fix, patch, or work around caddy#6293 (no SSE sniffing, no forked encode module, no `flush_interval` fiddling). Upstream's bug is upstream's.
- **No auto-detection** of streaming services — no Content-Type sniffing or endpoint probing at deploy time. Explicit flag, operator's call. The reset warning is **not** auto-detection: it guesses nothing, it reports a transition between two values already in hand (`prev.Config` vs `req`).

## Verification status

Generator output is **byte-asserted by unit tests; pending operator `caddy validate`** on the Linux host. There is no Docker on the dev box, so no one on this task ran `caddy validate` or observed a real stream — do not upgrade this wording to "validated" without doing it. See `_ai/caddyfile-generator-facts.md`.

## Why this isn't in `_docs/`

`_docs/usage.md` documents the `--no-compression` flag, the default, the reset caveat, and the upgrade behavior — what an operator needs. This file is the *why*: the BREACH rejection, the knob rule, the zero-bytes finding, the rejected names, and the retirement condition. Operators do not need it; future contributors need it to avoid relitigating "should compression be off by default", reaching for `match`, or deleting a knob that looks like dead weight.

## Sources

- Caddy `encode` directive — <https://caddyserver.com/docs/caddyfile/directives/encode>
- `modules/caddyhttp/encode/encode.go` (master) — <https://github.com/caddyserver/caddy/blob/master/modules/caddyhttp/encode/encode.go>
- caddy#6293, "Problems with reverse proxied server sent events and compression" (**open**) — <https://github.com/caddyserver/caddy/issues/6293>
- caddy#4314 (referenced in the `FlushError` comment) — <https://github.com/caddyserver/caddy/issues/4314>
- RFC 9110 §8.8.3.3 (ETag / selected representation) — <https://www.rfc-editor.org/rfc/rfc9110.html#name-example-entity-tags-varying>
