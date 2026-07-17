# 002 — Don's Plan: HTTP compression in the generated Caddyfile

> Status: PLAN step 1 — **revised 2026-07-17 after Linus's review (`004-linus-plan-review.md`, APPROVE).**
> Discussion + decision + scope. No implementation.
>
> **Revision log (this document is the plan of record):**
> - §7.1 — **RULING on Linus's Issue A: Option C (warn on reset).** B rejected. The one open decision.
> - §2.1 — SameSite overstatement corrected ("raises the bar considerably", not "guts"); BREACH
>   argument reordered strongest-first.
> - §2.7 — **new:** `Vary: Accept-Encoding` handled upstream (checked, no action).
> - §3.0 — **new:** terminology — this is a **streaming / headers-then-idle** bug, not an SSE bug.
> - §3.3.1 — **new:** a streaming service is **never actually compressed**; the knob fixes header
>   timing, not bytes.
> - §4.2.1 — **new:** the reusable knob-justification rule (silent + misattributed + unworkaroundable).
> - §5.2 — correction for Kent: Joel's test-8 index warning is inverted (re-verified empirically).
> - §5.4 — **new:** required contents of the decision record, incl. retirement condition.
> - §6 — "no auto-detection" non-goal sharpened so it doesn't swallow the warn-on-reset.
> - §7.2 — my own errors, recorded.

## 0. TL;DR — the decision

**Global default ON, with a per-service opt-OUT knob, and the opt-out's removal is loud.**

- Every generated site block gets `encode zstd gzip`.
- New `ServiceConfig.DisableCompression bool` (`toml:"disable_compression"`), surfaced as
  `decloud deploy --no-compression`. Absent/zero (`false`) = compression ON = the default.
- **Redeploy without the flag still resets it to `false` (declarative house rule preserved) — but now
  warns** when it flips compression back on for a service that had it off. §7.1.
- BREACH is **not** the reason for the knob. **Streaming is.** See §2 and §3.

Anyone who wants to argue this should read §3.4 first. That's the load-bearing fact, and it's
not the one people expect. Linus independently confirmed it at source (`004`, §1).

---

## 1. What I actually verified (no assumptions)

### 1.1 Decloud side — traced, with line numbers

Caddy config is a **Caddyfile** (text, generated), not the JSON API.

- `internal/caddy/generator.go:35` — `textGenerator.Generate(outPath string, services []*registry.Service) error`.
  Writes 2 header comment lines → global options block → one site block per hostname → `writeFileAtomic`.
- `generator.go:41-45` — the global options block: `{ servers { protocols h1 h2 } }`. Emitted
  unconditionally, even for an empty registry.
- `generator.go:46-53` — the site loop. **This is the only place a site block is emitted**, and today
  its entire body is one line: `reverse_proxy <container>:<port>`.
- `generator.go:57-84` — `normalize(services) []GeneratorInput`. Drops services with zero hostnames
  (`:62-64`), sorts hostnames (`:70`), defaults container name to `decloud-<name>` (`:71-74`), sorts
  services by name (`:82`).
- `generator.go:16-21` — `GeneratorInput{ServiceName, ContainerName, Port, Hostnames}`. This is the
  struct a compression flag has to travel through.
- **Only one production caller**: `internal/deploy/service.go:410` → `d.deps.Generator.Generate(tmpPath, services)`.
  `Generate` receives `[]*registry.Service`, so it already has the **whole** `svc.Config`. Plumbing a
  new config field to the generator costs nothing — no new dependency, no signature change.

Config/registry:

- `internal/registry/types.go:9-26` — `ServiceConfig` (TOML, persisted at
  `<root>/config/services/<Name>.toml`, 0644). Top-level scalar `Strategy string` (`:17`) is the
  precedent for a top-level per-service scalar.
- `types.go:69-71` — `Route{Hostname string}`. A service's routes are **just hostnames**; they all
  proxy to the same `Run.Port` on the same container. **There is no such thing as a per-hostname
  backend in Decloud.** This matters for §4.
- `types.go:25` — `LastDeployedAt` is the documented precedent for a backward-compatible TOML
  addition: existing files without the field unmarshal to zero-value.

Plumbing precedent (`--mount`, commit `2c8aea9`):

- `internal/cli/deploy_service.go:40` field → `:61` `cmd.Flags().StringArrayVar(...)` → `:72` parse →
  `:105` into `deploy.Request`.
- `internal/deploy/service.go:52-63` — `deploy.Request` struct.
- `internal/deploy/service.go:317-337` — `registry.ServiceConfig{...}` construction. This is where a
  new field gets set from `req`.

Tests: `internal/caddy/generator_test.go` — pure byte-level assertions on the generated body
(`makeService` helper at the top builds a `*registry.Service`). `TestGenerator_DisablesHTTP3` is the
model for a "global behavior" test: asserts content, asserts exact indentation, asserts ordering
(`protoIdx < siteIdx`), and `TestGenerator_EmptyInputProducesHeaderAndGlobalBlock` locks the empty
case.

### 1.2 The established pattern for a global Caddy change (commit `6f24efc`)

HTTP/3 was disabled via the **global options block**, and `_ai/caddyfile-generator-facts.md` records
why that was possible: *"Protocol selection is global-options-only — there is no per-site knob."*

**`encode` is the mirror image of that, and this is the first thing to get straight:**

> `encode` is a **site-level directive**. There is **no** global-options `encode`. It cannot go in the
> `{ servers { ... } }` block.

So "enable compression globally" in Decloud does **not** mean "one line in the global block". It means
**"emit `encode` into every site block"** — the generator's site loop at `generator.go:46-53`. "Global"
here is a property of *our generator*, not of *Caddy's config model*. Any plan that proposes adding
`encode` to the global options block is wrong and will fail `caddy validate`.

### 1.3 Caddy side — verified against current source, not memory

Fetched current `master`: `modules/caddyhttp/encode/encode.go` (602 lines) and the live docs page
<https://caddyserver.com/docs/caddyfile/directives/encode>.

| Fact | Evidence |
| --- | --- |
| Bare `encode` = zstd + gzip; `Prefer` default order is `zstd, br, gzip` filtered to enabled | `encode.go:130-136`; docs |
| `defaultMinLength = 512` | `encode.go:594-595`, applied `encode.go:80-81` |
| Default match list includes `text/*` | `encode.go:~100-125` (list quoted in §3.4) |
| Backend-set `Content-Encoding` ⇒ **skipped** | `encode.go:457` — `if hdr.Get("Content-Encoding") == "" && isEncodeAllowed(hdr) && rw.config.Match(rw)` |
| `Content-Length` deleted when encoding | `encode.go:461` — `hdr.Del("Content-Length")` |
| `Accept-Ranges` deleted when encoding | `encode.go:466` — `hdr.Del("Accept-Ranges")` |
| `206 Partial Content` ⇒ encoding disabled | `encode.go:261-263` — `if status == http.StatusPartialContent { rw.disabled = true }` |
| Strong ETags get `-<encName>` appended; `If-None-Match` stripped on the way upstream | `encode.go:468-474` + `ServeHTTP` |
| `Cache-Control: no-transform` (request or response) ⇒ no encoding | `encode.go:157-159` `isEncodeAllowed` |
| Upstream handler error ⇒ encoding disabled (protects `handle_errors`) | `encode.go:191-196` |
| **Wrapper install is decided by the REQUEST's `Accept-Encoding` alone** | `encode.go:162-171` (see §3.4 — this is the big one) |
| Pre-body `Flush()` is **swallowed** | `encode.go:295-306` `FlushError()` — `if !rw.wroteHeader { return nil }` |
| Caddy issue #6293 (SSE + compression) | **OPEN.** Created 2024-05-02, last updated 2026-03-17, `closed: null`. Verified via `gh api repos/caddyserver/caddy/issues/6293`. No merged fix PR found. |

---

## 2. The discussion (this is a deliverable, not a footnote)

### 2.1 BREACH / CRIME — not our problem, and the knob wouldn't help anyway

**CRIME is irrelevant.** CRIME attacked *TLS-level* compression (and SPDY header compression). TLS
compression is gone in TLS 1.2 deployments and does not exist in TLS 1.3. Caddy's `encode` is
*HTTP response body* compression. Different layer. Citing CRIME here is cargo-cult security.

**BREACH is real but does not justify withholding compression.** BREACH needs **all** of:

1. Response body compressed over TLS — yes, that's what we'd be enabling; **and**
2. A **secret** (CSRF token, session id) in the response body; **and**
3. **Attacker-controlled input reflected into the same response** as that secret; **and**
4. The ability to drive many cross-origin, cookie-bearing requests and observe response sizes.

Conditions 2+3 together are the requirement — a compressed response is not enough. The attack measures
how the secret's compressibility changes as the attacker guesses it character by character, which only
works when guess and secret share a compression window.

Three reasons this doesn't move me, **strongest first**:

- **The knob is useless against it — this argument alone decides the question.** A per-host toggle only
  helps if the operator knows their app reflects attacker input beside a secret. If they knew that,
  they'd fix the reflection. We would be shipping a security theatre knob that nobody can correctly
  set. This is independent of how scary you think BREACH is, which is why it's the reason and the other
  two are merely support.
- **It's an application-layer bug.** If a service reflects attacker input next to a CSRF token, it is
  broken with or without our proxy. The mitigations are all app-side: token masking / per-request
  rotation, modern cookie defaults, and not reflecting input next to secrets. **A reverse proxy cannot
  fix any of that.**
- **The entire industry ships compression on by default** — nginx's stock distro configs, every Caddy
  tutorial, and Cloudflare compressing by default for every site it fronts. The whole web runs TLS+gzip.
  BREACH has been public since 2013 and nobody's answer was "turn off gzip". *Supporting evidence only —
  this is an appeal to authority and I'm labelling it as one rather than leaning on it.*

**On `SameSite` — stated carefully, because the sloppy version is wrong.** `SameSite=Lax` (the default
in every current browser) **still sends cookies on top-level cross-site GET navigations.** It raises the
bar on condition 4 considerably; it does **not** eliminate it. An earlier draft of this plan said Lax
"guts" condition 4 — that was an overstatement, Linus caught it, and it is corrected here.

**The honest, defensible statement:** BREACH remains theoretically live for applications that reflect
attacker-controlled input next to a secret in a compressed response; modern cookie defaults raise the
bar considerably; the mitigation is, and always was, app-side.

**Verdict: BREACH is not a reason for a knob and not a reason to withhold the default.**

### 2.2 Already-compressed content — genuinely handled, no action needed

Two independent guards:

1. **Content-Type allow-list.** Default match (`encode.go`) is an allow-list, not a deny-list. JPEG,
   PNG, MP4, `.zip`, `.gz` do not match and are never touched. Only text-ish types match. (`image/svg+xml`
   and `image/x-icon` are explicitly on the list because they *are* compressible.)
2. **`minimum_length = 512`.** Anything smaller is not worth a compression frame and is skipped.

**No `match` override needed.** Writing our own match list would mean maintaining a worse copy of a
list upstream already maintains. Don't.

### 2.3 Double compression — handled at `encode.go:457`

`if hdr.Get("Content-Encoding") == ""` — a backend that already gzips its own responses is passed
through untouched. No double compression. Note this is also why `encode` is safe in front of an app
that does its own compression: worst case we do nothing.

### 2.4 `Content-Length`, ranges, ETags — correct, with one real consequence

- `Content-Length` is deleted (`:461`) and the response becomes chunked. That's correct and unavoidable
  (you can't know the compressed length before compressing). Consequence: **progress bars on
  compressed responses lose their total.** For text/JSON/HTML this is noise.
- `Accept-Ranges` deleted (`:466`) and `206` disables encoding (`:261-263`). Range requests are
  therefore only meaningfully used on non-matching content (video, big binaries) — which we never
  compress anyway. **Byte-range video streaming is unaffected.** This is the correct pairing and Caddy
  gets it right.
- ETags get `-zstd`/`-gzip` appended and `If-None-Match` is un-appended before going upstream
  (`:468-474`, `ServeHTTP`). Per RFC 9110 §8.8.3.3 this is *required* — different encodings are
  different selected representations. Caching still works.

### 2.5 CPU — a non-issue at Decloud's scale

Decloud is a single-host PaaS for personal projects. gzip default level on text is single-digit
milliseconds for typical page/JSON payloads, against a network path where we're saving 60-80% of
bytes on HTML/JS/JSON. `minimum_length=512` skips the trivia. The trade is overwhelmingly positive.
zstd is *faster* than gzip at comparable ratios.

### 2.6 zstd vs gzip ordering

`encode zstd gzip` sets `Prefer = [zstd, gzip]`. Selection is **negotiated** — a client only ever
receives an encoding it advertised in `Accept-Encoding`. zstd is Chrome 123+, Firefox 126+, Safari 18.4+;
anything older simply gets gzip. **There is no client-compatibility risk here**, unlike the h3/Safari
incident in `6f24efc` (that broke because Caddy *advertises* h3 via `Alt-Svc` regardless of client
health; `Accept-Encoding` is client-driven, so it's the opposite situation). Preferring zstd is a free
win where supported.

### 2.7 `Vary: Accept-Encoding` — handled upstream, no action

**This is the first question any competent reviewer asks about enabling compression behind a cache**
("will an intermediary serve a gzipped body to a client that never asked for it?"), and my first draft
didn't answer it. Linus was right to flag the omission.

The answer: **upstream handles it correctly.** `encode.go:463-464` adds `Vary: Accept-Encoding` in
`init()` when it encodes (guarded by `hasVaryValue` so it won't duplicate an existing one), and
`encode.go:265-270` covers the `304 Not Modified` case specifically — per RFC 9110 §15.4.5, a 304 must
carry the header as if it were a 200, which `init()` alone would miss because `init()` only runs when a
body is written.

**No code, no config, no action.** It's recorded because unwritten answers get re-derived, and
re-derived answers get re-derived *wrong*.

### 2.8 WebSockets — not affected

A WebSocket upgrade is a `101` + connection hijack. `init()` is only ever reached from
`responseWriter.Write` (`encode.go:337-351`), and a hijacked connection never writes its payload
through the wrapper. Nothing gets compressed. (Caddy's `encode` responseWriter also passes hijack
through via `http.NewResponseController`.) Not a concern.

---

## 3. The actual problem: streaming (headers-then-idle)

This is the only finding that changes the design, and it is worse and subtler than the public issue
suggests.

### 3.0 Terminology: this is a STREAMING bug, not an SSE bug

Say **"streaming"**, not "SSE". Linus sharpened my framing here and he's right, so I'm adopting his
wording wholesale:

The swallow at `encode.go:302` fires **before Content-Type is ever consulted** (§3.4). Nothing about
`Content-Type: text/event-stream` selects the failure. **This is a headers-then-idle bug** — any
handler that writes a status and then waits before its first body byte is in the blast radius:
long-poll, chunked progress streams, anything that establishes then idles. SSE is simply its most
common shape, and in 2026 it's the default transport for LLM token streaming, which is exactly the
"personal projects on a small PaaS" workload.

The knob's scope is drawn correctly either way, but the docs and decision record must say "streaming"
so nobody concludes that matching on `text/event-stream` is what identifies an affected service.

### 3.1 `text/event-stream` matches `text/*`

The default match list ends with `"text/*"`. **`text/event-stream` matches it.** There is **zero**
special-casing of `event-stream` anywhere in `encode.go` (grepped the whole 602-line file). SSE is, by
default, a compression candidate.

### 3.2 Caddy issue #6293 is open, and has been for over two years

`gh api repos/caddyserver/caddy/issues/6293` → `state: "open"`, `created: 2024-05-02`,
`updated: 2026-03-17`, `closed: null`. Two reported problems: headers not flushed until first body
byte, and events not reaching the client due to compression framing. This is **not** a stale ticket
nobody hit; it's a known, unfixed, actively-discussed defect.

### 3.3 What's actually broken vs. what's been fixed — I checked, don't take the issue at face value

I will not repeat the issue's claims without verifying them. Reading current `master`:

- **Problem 2 (events never delivered) appears ADDRESSED.** `FlushError()` at `encode.go:311-317` now
  calls `rw.w.Flush()` on the encoder before flushing the underlying writer, with a comment
  referencing `caddy-slow-gzip`. A gzip/zstd sync-flush emits the pending frame, so data *does* reach
  the client. Reporting this as "SSE is completely broken" would be **wrong**.
- **Problem 1 (header flush swallowed) is STILL PRESENT.** `encode.go:300-306`:

  ```go
  if !rw.wroteHeader {
      // flushing the underlying ResponseWriter will write header and status code,
      // but we need to delay that until we can determine if we must encode and
      // therefore add the Content-Encoding header; this happens in the first call
      // to rw.Write (see bug in #4314)
      return nil
  }
  ```

  A backend that writes `200 text/event-stream`, flushes, and *then* waits for its first event gets
  its flush **silently dropped**. The client's `EventSource` `onopen` does not fire until the first
  body byte arrives. An idle-first SSE stream hangs, and can hit client/intermediary timeouts.

  Honest severity: **most SSE survives**, because typical events are far under 512 bytes, so `init()`
  is never called (`encode.go:337-345`: `gtMinLength` false ⇒ no `init()` ⇒ `rw.w == nil` ⇒ body passes
  through uncompressed) and the header lands on the first `Write`. The failure window is
  **headers-then-idle** streams and any stream whose first write exceeds 512 bytes. That's a real
  window, not a theoretical one — "connect now, push later" is an extremely normal streaming shape.

### 3.3.1 The consequence nobody wrote down: a streaming service is never actually compressed

Follow the mechanism in §3.3 to its conclusion, because it produces a genuinely counter-intuitive
result that **must** be in the decision record:

`init()` is only ever called from `Write`, and only when the first write exceeds 512 bytes
(`encode.go:337-345`). It is **never retried** once `wroteHeader` is set. So a typical small-event
stream runs with `rw.w == nil` — **uncompressed, forever.**

**Therefore `--no-compression` buys a streaming user exactly zero bytes. It purely fixes header
timing.**

That is a strange shape for a flag named "disable compression," and it is precisely the kind of fact
that gets a feature deleted by someone who is being diligent: a future engineer measures SSE responses,
observes they were never compressed anyway, concludes the knob is dead weight, and removes it —
reintroducing the hang. **Write it down or lose the knob to a well-intentioned optimization.** (This
does not weaken §4.2 — the knob's justification was never bytes, it was the swallowed flush at
`encode.go:302`, which is installed regardless of whether compression ever engages.)

### 3.4 The fact that decides the architecture

**`match` cannot save SSE.** Look at `encode.go:162-171`:

```go
func (enc *Encode) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
    if isEncodeAllowed(r.Header) {
        for _, encName := range AcceptedEncodings(r, enc.Prefer) {
            ...
            w = enc.openResponseWriter(encName, w, r.Method == http.MethodConnect)
```

The wrapper is installed based **only on the request's `Accept-Encoding`** (plus request
`Cache-Control: no-transform`). The response `match` / Content-Type is not consulted here — it's only
consulted later, inside `init()` (`encode.go:457`, `rw.config.Match(rw)`), which is only reached from
`Write`.

**Therefore:** the moment `encode` is on a site block and a browser sends `Accept-Encoding: gzip, zstd`
(all of them do, including `EventSource`), **every response on that site is wrapped**, and the
pre-body flush is swallowed — *even for content types that would never be compressed*.

So this does **not** work:

```
encode zstd gzip {
    match { ... exclude text/event-stream ... }   # ← does NOT fix the header-flush delay
}
```

Excluding `text/event-stream` via `match` prevents *compression*, but the wrapper — and the swallowed
flush — is already installed before Content-Type is known. **The only way to protect a streaming
service is to not emit `encode` in its site block at all.**

That is the entire justification for the knob. Not BREACH. This.

---

## 4. Decision and rationale

### 4.1 Global default ON

`encode zstd gzip` in every site block, using Caddy's **default** match and `minimum_length`.

- The compression win on HTML/JS/JSON/CSS is large and it's what every user wants.
- Already-compressed and small content is handled by upstream's own defaults (§2.2).
- Double-compression, ranges, ETags, no-transform, and error paths are all handled correctly by
  upstream (§2.3, §2.4).
- BREACH doesn't justify withholding it (§2.1).
- Sane defaults are the Decloud philosophy. An operator should not have to opt in to the obviously
  right thing.

### 4.2 Per-service opt-OUT knob — justified, narrowly

Normally I'd fight a knob. Knobs are complexity and most of them exist because someone couldn't make a
decision. This one earns its place:

- The failure is **real and verified at source level**, not speculative (§3.3, §3.4).
- It is **not fixable by any Caddy-side configuration** we could generate (§3.4). `match` doesn't work.
- Upstream has not fixed it in **two years** (§3.2). Waiting is not a strategy.
- **Without a knob the user has no escape hatch at all.** The generated file's first line literally says
  `# Caddyfile generated by decloud — do not edit by hand.` and it's atomically overwritten on every
  `decloud caddy reload` (`generator.go:54`, `deploy/service.go:410`). Hand-editing is not an out. We'd
  be telling an SSE user "your app is broken, sorry." That is shipping shit.

**A one-line knob that has a correct, verified, non-hypothetical reason to exist is not scope creep.**

### 4.2.1 The rule — this is the reusable part, and it goes in the decision record

Linus argued the anti-knob case at full strength (every config field is forever; this isn't even our
bug, it's a third party's unfixed defect permanently encoded into our public config surface; no user
has hit it; YAGNI; add it when someone actually complains). **That is a good argument and in most
reviews it wins.** It loses here, and the reason it loses generalises:

> **A knob earns permanent config surface when the default has a known failure that is
> (i) silent, (ii) misattributed, and (iii) unworkaroundable. If the failure is loud — a failed
> deploy, a 502, an obvious error — kill the knob and wait for a real bug report.**

This case is 3-for-3:

- **Silent** — no 500, no failed deploy. Builds, deploys, passes readiness, looks healthy.
- **Misattributed** — the user blames their app, their browser, their network, their own code. They
  will **not** suspect a reverse proxy they never configured. This is what destroys the "just wait for
  a real bug report" strategy the anti-knob argument depends on: the report will never arrive
  correctly diagnosed.
- **Unworkaroundable** — the generated file says `do not edit by hand` and is atomically overwritten
  (`generator.go:54`, `service.go:410-419`) on every deploy *and* every `decloud caddy reload`. This is
  self-hosted: **the user *is* the operator and still cannot fix it.**

Plus: upstream hasn't fixed it in two years (verified open, updated 2026-03-17). Waiting is not a
strategy.

Write the rule down, not just the conclusion. **The rule is what tells future-us whether the *next*
proposed knob is legitimate engineering or cowardice** — and it's also the rule that would have killed
this one if the failure had been loud.

### 4.3 Why per-SERVICE, not per-HOST

`Route` is `{Hostname string}` (`types.go:69-71`) and every route of a service proxies to the **same**
`Run.Port` on the **same** container. Two hostnames on one service are the same backend with the same
streaming behavior. **A per-hostname compression flag would be a knob that can only ever be set the
same way for every hostname of a service.** Per-service is the correct granularity. (This is why the
user's "per host" framing resolves to per-service — in Decloud a "host" is not an independent config
unit.)

### 4.4 The name, and the zero value

```go
DisableCompression bool `toml:"disable_compression"`
```

TOML zero value discipline: an absent bool unmarshals to `false`. So:

| TOML | Value | Behavior |
| --- | --- | --- |
| *(absent — every existing service file)* | `false` | **compression ON** ← the default |
| `disable_compression = false` | `false` | compression ON |
| `disable_compression = true` | `true` | `encode` omitted from the site block |

`false` = default behavior. **Every service TOML already on disk keeps working and silently gains
compression on next reload**, exactly like `LastDeployedAt` (`types.go:25`) did. That is the required
naming polarity and it's why the field is `disable_compression` and **not** `compression` /
`enable_compression` (both of which would make the zero value mean "off" and would silently disable
compression for every existing service — unacceptable).

CLI: `--no-compression` (bool, default false).

---

## 5. Scope of changes

1. **`internal/registry/types.go`** — add `DisableCompression bool \`toml:"disable_compression"\`` to
   `ServiceConfig` (top-level, next to `Strategy`). Backward-compatible addition; cite the
   `LastDeployedAt` precedent in the comment.
2. **`internal/caddy/generator.go`**
   - `GeneratorInput`: add `DisableCompression bool`.
   - `normalize()`: carry `svc.Config.DisableCompression` into the input.
   - Site loop (`:46-53`): emit `    encode zstd gzip` inside the site block, **before**
     `reverse_proxy`, unless `DisableCompression`. **4-space indent** — same level as `reverse_proxy`.
     (`_ai/caddyfile-generator-facts.md`: spaces, not tabs; a tab slip fails CI.)
   - **Do not** touch the global options block. `encode` is site-level (§1.2).
3. **`internal/deploy/service.go`** — `Request` (`:52-63`) gains `DisableCompression bool`; wire into
   `registry.ServiceConfig{...}` at `:317-343`.
   **Plus the warn-on-reset (§7.1 ruling).** Adjacent to the config construction, after the readiness
   gate: if `hasPrev && prev.Config.DisableCompression && !req.DisableCompression`, emit a structured
   `logger.Warn` telling the operator compression was re-enabled and that `--no-compression` keeps it
   off. `prev` (`:179`) and `hasPrev` (`:180`) are already in scope — **verified, no new load**.
4. **`internal/cli/deploy_service.go`** — `--no-compression` bool flag, following the `--mount`
   plumbing shape (field → `cmd.Flags().BoolVar` → `deploy.Request`). No parsing/validation needed;
   it's a bool, so **no `errUsage` path** (unlike `parseMountFlags`).
5. **Tests** (`internal/caddy/generator_test.go`) — Kent owns these. `makeService` will need a way to
   set the flag (extra helper or direct struct edit — implementer's call, keep it clean).
6. **Docs** — Raymond. `_docs/usage.md` (`--no-compression`), `_docs/install.md` if it shows a sample
   Caddyfile, `_ai/caddyfile-generator-facts.md` (add a section: `encode` is site-level, `match` can't
   fix streaming, why the knob exists), and a decision record — contents specified in §5.4.

   **Raymond: kill the hand-edit trap** (Linus Issue B, and he's right that this isn't a decision, it's
   a fix). `_docs/usage.md:150` invites hand-editing the TOML ("Edit the TOML by hand at your own
   risk"). A hand-set `disable_compression = true` **survives `decloud caddy reload` but is silently
   wiped by the next `decloud deploy`** (fresh `ServiceConfig` from `req`). One sentence at `:150`
   saying exactly that, and that **`--no-compression` is the durable way**. Do not omit the key to dodge
   the problem — it's in the file, people will find it, and omission is dishonest.

   **Raymond: the reset caveat goes in BOTH places**, not one — the `--no-compression` flag row **and**
   the `decloud caddy reload` section. Say "streaming", not "SSE" (§3.0).

### 5.1 Order of work (TDD)

Stubs → Kent's failing tests → Rob's implementation. Field additions are effectively stubs.

### 5.2 Test scenarios I want to see

Byte-level, matching the house style of `TestGenerator_DisablesHTTP3`:

1. Default service ⇒ body contains `encode zstd gzip`, indented exactly `"\n    encode zstd gzip\n"`.
2. **Ordering**: `encode` index < `reverse_proxy` index **within the same site block** (Caddyfile
   directive order in the file doesn't drive execution — Caddy sorts by its own directive order — but
   the generated file must read sensibly and stay stable).
3. `DisableCompression: true` ⇒ that site block has **no** `encode`, and **still has**
   `reverse_proxy decloud-x:8080`.
4. **Mixed registry** — service A default, service B opted out ⇒ exactly one `encode` in the file, and
   it's in A's block, not B's. (This is the test that catches a flag-carried-to-the-wrong-input bug,
   which is exactly the sort of thing `normalize`'s sorting could mask.)
5. `encode` must **not** appear in the global options block / must appear after `protocols h1 h2`
   (guards the §1.2 mistake).
6. Empty registry ⇒ unchanged behavior (global block only, no `encode`).
7. Multi-hostname service ⇒ `encode` in **every** one of its site blocks (the loop at `:48` emits one
   block per hostname — easy to get wrong).
8. **Warn-on-reset (§7.1)** — `internal/deploy`: prev config has `DisableCompression: true`, request
   omits the flag ⇒ warning emitted. Plus the negative cases: no warning when `prev` had it `false`,
   when the request *sets* the flag, or when there is no `prev` at all (first deploy must be silent).
   Kent's call on whether that's one table test or several — but the first-deploy silence matters, it's
   the case that would spam every new user.

**Correction for Kent (via Linus, and I re-ran it myself before passing it on):** Joel's §6.2 test-8
warning is **inverted**. He warns that the first `"\n}\n"` is the inner `servers` close. It is not — the
inner close is `"\n    }\n"` (newline, **four spaces**, brace) and **cannot** match `"\n}\n"`. Against
the real bytes from `generator.go:41-45`: `strings.Index(body, "\n}\n")` → `45`, and `body[:45]` is
exactly `"{\n    servers {\n        protocols h1 h2\n    }"` — the whole global block. **The simple form
is correct; use it.** Don't burn time chasing a trap that doesn't exist. (Joel: strike the warning.)

### 5.3 Acceptance criteria

- `go test ./...` green.
- Existing service TOMLs (no `disable_compression` key) load and get compression. **No migration.**
- `decloud deploy --no-compression` round-trips: flag → `Request` → `ServiceConfig` → TOML → generator
  → no `encode` in that service's blocks.
- Generated Caddyfile is byte-stable for a given registry (no map iteration order leaking in).
- **`caddy validate` is the maintainer's step on the Linux host — NOT claimed here.** Per
  `_ai/caddyfile-generator-facts.md`: no Docker on this box. Reports say **"byte-asserted; pending
  operator `caddy validate`"**. Nobody writes "validated". I will personally reject any report that
  claims validation that didn't happen.

### 5.4 What the decision record must contain (non-negotiable)

Four things were *verified but unwritten* in my first draft. Unwritten answers get re-derived, and
re-derived answers get re-derived wrong:

1. **The knob-justification rule** (§4.2.1): silent + misattributed + unworkaroundable = a knob earns
   permanent config surface. The reusable part, and the one that governs the *next* proposed knob.
2. **`Vary: Accept-Encoding` is handled upstream** (`encode.go:463-464`, `:265-270`) — checked, safe,
   no action. The first question any reviewer asks.
3. **A streaming service is never actually compressed** (§3.3.1) — the knob fixes header timing, not
   bytes. Without this, someone deletes the knob as dead weight after measuring that streaming
   responses were never compressed anyway.
4. **The retirement condition** — link Caddy #6293. If upstream fixes the swallowed flush, this knob
   loses its justification and should be revisited. Also record why **`streaming = true` was rejected**
   as a name (Linus's point, and it's the name someone proposes in six months): it would promise
   semantics we don't implement — we don't tune `flush_interval`, don't touch `reverse_proxy`, do
   nothing else streaming-shaped. It would be a lie in the config file. `disable_compression` states
   what it mechanically does and stays coherent even after #6293 is fixed.

Also record, so nobody proposes it later (Linus): `request_header -Accept-Encoding` in the site block
*would* also suppress the wrapper (`header` sorts before `encode` in Caddy's directive order) — but
it's strictly more obscure than simply not emitting `encode`. **Omitting the directive is the correct,
obvious answer. Don't get clever.**

---

## 6. Non-goals (explicit — do not let these creep in)

- **No global-options `encode`.** It doesn't exist. Site-level only.
- **No custom `match` list.** Upstream's default allow-list is better than one we'd maintain. §2.2.
- **No `minimum_length`, no compression levels, no `prefer` tuning.** Defaults (512, default levels)
  until someone shows me a profile that says otherwise.
- **No brotli.** Not built into Caddy core as an encoder. Out of scope.
- **No per-hostname granularity.** §4.3. Per-service is the correct unit.
- **No attempt to fix / patch / work around Caddy issue #6293** (no custom SSE detection, no forked
  encode module, no `flush_interval` fiddling). Our answer is the opt-out. Upstream's bug is upstream's.
- **No `reverse_proxy` changes** of any kind.
- **No auto-detection** of streaming services. We are not sniffing Content-Type, probing endpoints, or
  inspecting traffic at deploy time to *guess* whether a service streams. Explicit flag, operator's call.
  **Sharpened per §7.1:** the warn-on-reset is **not** auto-detection and does not violate this. It
  guesses nothing — it reports a transition between two values already in hand (`prev.Config` vs `req`).
  Detection infers a fact we don't have; the warning reports one we already computed. Don't conflate
  them, in either direction.
- **No changes to the h3/`protocols` behavior** from `6f24efc`.

---

## 7. Trade-offs and debt, recorded honestly

- **We are shipping a knob whose reason is an upstream bug.** If #6293 ever gets fixed, the honest thing
  is to revisit whether `disable_compression` still earns its place. **Write that in the decision record
  with the issue link** so future-us knows exactly what would retire this field. Undocumented knobs are
  how config surfaces rot.
- **Existing services silently gain compression on next reload.** This is intended (that's the point of
  the polarity in §4.4), but it *is* a behavior change to running services on a `decloud caddy reload`.
  It must be called out in the docs and the commit message. An SSE user who was previously fine could
  regress; `--no-compression` is their fix. Say so plainly.
- **`--no-compression` absent on redeploy resets the field to `false`.** Joel confirmed the mechanism
  (`service.go:317-343` builds a fresh `ServiceConfig` from `req`, never merges `prev`) and defended it
  on consistency-with-`--mount` grounds. **I overruled that — see §7.1.** The reset stays; it now warns.

---

## 7.1 RULING — Linus's Issue A: the escape hatch that silently closes itself

**Decision: Option C — keep the reset, make it loud.** Linus's recommendation. Adopted.

### The ruling

`deploy` emits a warning when it flips compression back ON for a service whose *previous* config had
`disable_compression = true`:

```
note: compression re-enabled for <svc> (previous deploy used --no-compression);
pass --no-compression to keep it off
```

Condition: `hasPrev && prev.Config.DisableCompression && !req.DisableCompression`.

### Why C, and why I overruled Joel

Joel defended the silent reset on consistency: `--mount`, `--strategy`, `--readiness-path` all reset
too. **The consistency claim is true and it is not the point.** Linus caught what Joel missed — the
*consequence class* is different:

- Forget `--mount` → app can't find its file. **Loud.** You know in seconds.
- Forget `--strategy` → visibly different deploy behavior. **Loud.**
- Forget `--no-compression` → deploys clean, passes readiness, looks healthy, then hangs on stream
  open for some clients. **Silent. Misattributed.**

That is precisely the failure class — silent + misattributed — that justified building the knob in
the first place (§4.2, and the rule now recorded in §4.2.1). **An escape hatch that silently closes
itself and restores the exact bug it exists to prevent is not an escape hatch.** If I accept Option A
here, I am not applying my own §4.2 argument to my own feature. Either the silent-failure reasoning is
sound — in which case it applies here too — or the knob shouldn't exist at all. I'm not going to
believe it on Tuesday and forget it on Wednesday.

**Option B (sticky / merge from `prev`) is rejected**, and I agree with Linus's reasoning without
reservation. Per-field merge semantics are how a config surface rots into "which of these flags are
sticky?" — a question with no good answer, asked forever, by everyone. Declarative deploy is the house
rule. Breaking it for exactly one field would be the worst kind of special case: invisible, unprincipled,
and permanent.

**Option C preserves the house rule perfectly.** The flag still resets. Behavior is identical and
still declarative. We are not changing semantics — we are making a state transition *we already
compute* visible. That is the cheapest possible conversion of a silent failure into a loud one.

**Cost, verified — I checked before ruling rather than taking "~3 lines" on trust:**

- `prev, loadErr := d.deps.Store.Load(ctx, req.Name)` — `service.go:179`
- `hasPrev := loadErr == nil` — `service.go:180`
- `prev` is function-scoped and live at the construction site (already used at `:265`, `:301`, `:362`)
- `logger` is in scope and structured (`logger.Warn(msg, "key", val, ...)`)

**No new load, no new dependency, no signature change.** The data is already in hand. Linus's estimate
holds.

### Placement and shape (for Joel to pin down)

- **Where:** adjacent to the `ServiceConfig` construction (`service.go:~317`), *after* the readiness
  gate. Two reasons: it's the most legible place — right where the reset physically happens — and by
  then the deploy is actually going to persist, so we don't warn about a reset that never lands on a
  deploy that subsequently fails.
- **How:** the existing structured `logger.Warn`, house style (`"service", req.Name`). Not `fmt.Println`.
- Exact wording is Joel's to pin; the semantics above are not negotiable.

### Scope honesty

This is **real scope growth on a ~5-line change**, and I'm naming it rather than pretending it's free:
production diff goes from ~5 lines to ~8, plus one test. Linus's "if Rob's diff is materially bigger,
something has gone wrong" tripwire is **adjusted accordingly** — Joel must restate the expected diff
size so the tripwire stays meaningful instead of firing on our own approved change.

**This is not a violation of the "no auto-detection" non-goal.** That non-goal forbids *sniffing
Content-Type at deploy time to guess whether a service streams*. This warning guesses nothing: it
reports a transition between two values we already have on hand. The non-goal wording is sharpened in
§6 so nobody conflates the two later.

---

## 7.2 Corrections to my own plan (from Linus's review — all accepted)

I got one thing wrong and left three things unwritten. Recording both, because a plan that hides its
own corrections is worthless.

1. **SameSite overstatement — my error, corrected in §2.1.** I wrote that `SameSite=Lax` "guts"
   BREACH's condition 4. It does not: **Lax still sends cookies on top-level cross-site GET
   navigations.** It raises the bar considerably. Linus is right that this must not reach a permanent
   decision record — a claim a security-literate reader can knock down in one sentence discredits the
   correct reasoning around it. The BREACH argument does not need it and is airtight without it.
2. **I buried the load-bearing BREACH argument** under two weaker ones. The reason is *"the knob is
   useless against BREACH — the mitigation is app-side and a reverse proxy cannot fix it."* "The whole
   industry ships it" is supporting evidence, not the reason. Reordered in §2.1.
3. **`Vary: Accept-Encoding` — checked but never written down.** Now §2.7. Linus is right that it's the
   first question a competent reviewer asks about compression behind a cache, and I'd verified the
   answer without recording it. Verified ≠ recorded.
4. **"Streaming", not "SSE"** — Linus's sharpening is correct and it's better than my framing. The
   swallow at `encode.go:302` fires *before* Content-Type is ever consulted, so this is a
   **headers-then-idle bug**, not an SSE bug. SSE is merely its most common shape; long-poll and
   chunked progress streams are in the same blast radius. Nothing about `Content-Type: text/event-stream`
   selects the failure. Terminology corrected throughout §3.

## 8. Notes for Joel (revised post-Linus)

Design is **approved and closed**. Don't reopen it. Outstanding items:

1. **Strike the §6.2 test-8 index warning.** It's inverted — see §5.2. Verified twice (Linus, then me,
   against the real emitted bytes). It's sending Kent after a trap that doesn't exist.
2. **Spec the warn-on-reset** per the §7.1 ruling: placement (adjacent to config construction, after
   the readiness gate), condition (`hasPrev && prev.Config.DisableCompression && !req.DisableCompression`),
   structured `logger.Warn`, exact wording yours to pin. The semantics are not negotiable.
3. **Restate your diff-size tripwire.** "~5 production lines; materially bigger means something went
   wrong" was a *good* heuristic and I don't want to lose it — but the ruling makes it ~8 + a test. Update
   the number so the tripwire keeps working instead of firing on our own approved change.
4. Decide where `makeService` gains the flag without making the test helper ugly.
5. Pin the exact emitted string and indentation. `_ai/caddyfile-generator-facts.md` warns: 4-space
   indent, spaces not tabs, and a test asserts it literally.

**Your three catches all landed and I've folded them in** — the strict decoder (`store.go:250-251`
`DisallowUnknownFields()`; I cited `LastDeployedAt` as precedent without checking the decoder, and the
forward-downgrade hazard is real: accept, document, do **not** bump `schema_version` for an optional
additive key), the `caddy validate`-runs-in-prod correction (`service.go:413` — my wording was
genuinely misleading and would have left Rob with a wrong mental model; you fixed the fact while
correctly leaving the report discipline alone), and the test-4 off-by-one. The §6.5 `gomock.Any()` seam
test is the best thing in your plan: `Request.DisableCompression` could be dropped on the floor at
`:317-343` and every other test still passes. You found that unprompted and specified exactly one test
instead of retrofitting capture into all ten expectations. That's the right amount of discipline.

Where I overruled you — the silent reset (§7.1) — the consistency argument was correct; it just wasn't
the deciding factor. Consequence asymmetry beats surface consistency.

## 9. Sources

- Caddy `encode` directive docs — <https://caddyserver.com/docs/caddyfile/directives/encode>
- `modules/caddyhttp/encode/encode.go` (current master) —
  <https://github.com/caddyserver/caddy/blob/master/modules/caddyhttp/encode/encode.go>
- Caddy issue #6293, "Problems with reverse proxied server sent events and compression" (**open**,
  created 2024-05-02, updated 2026-03-17) — <https://github.com/caddyserver/caddy/issues/6293>
- Caddy issue #4314 (referenced in the `FlushError` comment) — <https://github.com/caddyserver/caddy/issues/4314>
- RFC 9110 §8.8.3.3 (ETag / selected representation) — <https://www.rfc-editor.org/rfc/rfc9110.html#name-example-entity-tags-varying>

---

# 10. COMPLETION ASSESSMENT (appended 2026-07-17, post-EXECUTION)

> **Appended, not rewritten.** §1-§9 above are left exactly as written, including the parts Linus and
> Kevlin proved wrong. See §10.2 on why.

## 10.1 Verdict: **NOT FULLY DONE.** Code is done and RIGHT. The *discussion* has two holes.

**The implementation is finished and I approve it.** What is not finished is the deliverable the user
called first-class.

I verified rather than accepting the summary handed to me:

| Claim | My check | Result |
| --- | --- | --- |
| 8 production lines outside CLI | `git diff main...HEAD -w` | ✅ exactly 8 |
| Suite green | `go clean -testcache && go test ./...` | ✅ 9/9 uncached |
| gofmt / vet / tree | `gofmt -l`, `go vet`, `git status` | ✅ all clean |
| Five non-goals absent | read the diff | ✅ no `match`, no validator, no mock churn, no `lifecycle.go`, field still resets |
| Code is the plan | read `generator.go`, `service.go`, `types.go` | ✅ `encode` in the inner loop, guarded, 4-space, before `reverse_proxy` |

*(Aside: I used `grep` to test emptiness rather than `gofmt -l`'s exit code — Rob's `007` §9 finding
that `gofmt -l` in an `&&` chain is a false green is real and I applied it.)*

**But the user asked for two things**, and the second is where this is short:

> "**discuss** if it's safe to enable HTTP compression globally in Caddy config or if it's better to
> have it as a setting per host; **implement** the change"

The discussion's durable home is `_ai/decisions/http-compression-on-by-default.md` — correct choice,
`_tasks/` is archive. It is an excellent document. And **it drops two things the user explicitly
asked for.**

### GAP 1 (required): CRIME is nowhere in the durable record

`grep -ri "crime" _ai/ _docs/` → **zero hits.**

The user's own request names *"BREACH/CRIME-style attacks over TLS"*. My §2.1 dismissed CRIME
explicitly. Linus `004` §3 singled it out: *"the CRIME dismissal is correct and worth making."* And it
**evaporated between plan and record.** The record answers BREACH seven times over and CRIME zero
times.

This is not pedantry. CRIME is the *most* cargo-culted objection to enabling compression — it is the
first thing a security-minded reader raises, and the record currently has no answer. Someone will
reopen this. **Fix (Raymond, ~2 sentences, in the BREACH section):** CRIME attacked *TLS-level*
compression (and SPDY header compression); TLS compression does not exist in TLS 1.3 and is disabled
in TLS 1.2 deployments. Caddy's `encode` is HTTP *response-body* compression — **a different layer**.
Citing CRIME against `encode` is cargo-cult.

### GAP 2 (recommended): the zstd/gzip ordering rationale, and the Safari question this repo will ask

The user explicitly asked about *"CPU cost, and gzip vs zstd ordering"*. The record states
`encode zstd gzip` and "zstd is faster than gzip at comparable ratios" — it never records **why the
ordering is safe**, which is the actual question.

And this repo has a **scar** that makes it the obvious question. `_ai/decisions/caddy-runs-in-container.md:59`:
*"iPhone Safari negotiating HTTP/3 over QUIC/UDP-443 **broke** connectivity."* The one Caddy-protocol
change in this project's history broke Safari. The next person to touch `encode zstd gzip` will ask
"we broke Safari with a Caddy protocol change once — does preferring a newer encoding do it again?"

**The answer is instructive, and it's a contrast worth recording** (my §2.6, also dropped):

- **h3 broke Safari because Caddy *advertises* it** — `Alt-Svc` is pushed at the client regardless of
  whether the client's QUIC path actually works. **Server-driven.**
- **Compression cannot do that** — encoding is *negotiated*: a client only ever receives an encoding
  it advertised in `Accept-Encoding`. zstd is Chrome 123+/Firefox 126+/Safari 18.4+; every older
  client transparently gets gzip, and one that offers neither gets identity. **Client-driven.**

Opposite mechanisms, opposite risk. **Preferring zstd carries no client-compatibility risk, and the
h3 precedent does not transfer.** That contrast is precisely what a decision record is for, and
`caddyfile-generator-facts.md:45` already explains the `Alt-Svc` half of it — the two documents are
one sentence apart from closing the question permanently.

### Scope of remaining work

**Docs only. Raymond. ~5 sentences. No code, no tests, no re-review of design.** Then done.

Everything else — code, tests, `_docs/usage.md`, the rest of the record — is **approved**.

## 10.2 On the three things put to me

**1. Stale plan text — I accept Linus's ruling without reservation. Do not retro-edit `_tasks/`.**

Plans are history, code is truth, the decision record outlives both. A plan that gets silently
corrected is **worthless as history**, because you can no longer see what people actually believed
when they decided — and the whole value of an audit trail is that it records the wrong turns. The
"silently" error is now recorded in four places (Raymond `008` §2, Kevlin `010` §4, Linus `009` §1,
and here). **Recorded beats erased.** This append notes the staleness; it does not rewrite §5.

**And the error is mine.** I wrote "silently" in rev 1 when the reset *was* silent. My own §7.1 Option
C ruling falsified it. I did not back-propagate it, Joel copied it, Linus reviewed both revisions and
read past it, and I re-read my own document twice and read past it. **Three seniors shipped a plan
that contradicted itself.** Raymond — handed explicit instructions by two of us — checked them against
the code, found them false, and shipped the truth instead of the instruction. That is exactly right,
and the fact that the most junior surface caught what three reviewers missed is the useful part. **A
doc writer's job is accuracy, not obedience.** His shipped wording is also better than mine.

**2. Kevlin's `init()` correction — accepted, and I want to be precise about how I earned it.**

My §3.3.1 said `init()` is *"only ever called from `Write`"*. False: also `Close` (`:423`), on the
bodyless `Content-Length` path. The conclusion survives — both call sites gate on `!wroteHeader`, so
`Close` cannot rescue a stream that already wrote headers, and `match` still cannot prevent wrapper
installation.

**How I got it wrong matters more than the fact.** I read `Write`'s call site and asserted
exclusivity. **I never enumerated the call sites.** Seeing one caller is not evidence there is only
one — that is a claim about absence, and absence requires a search I did not run. I have spent this
entire task telling other people that being told by three seniors is not evidence and that you must
verify at source. I then wrote a false universal into a permanent record from a single observation.
The rule I keep preaching applied to me and I didn't apply it.

Raymond's `008` §8.4 self-flag — *"these are Don's and Linus's source reading, not mine; I would
rather you catch it"* — is the only reason this didn't become permanent. **Labelling a relayed claim
as relayed is what made it auditable.** That belongs in the knowledge base as a norm.

**And Raymond's promoted finding strengthens my own §3.4**: every response-side lever — ours, the
backend's, `match`, `no-transform` — is *downstream* of the wrapper; only not emitting `encode` is
*upstream* of it. That's upstream's own structure arguing our design, and it closes "surely there's a
cleverer response-side fix" permanently. Better than the argument I made.

**3. The test gap — Linus was right, and Kent's diagnosis is the keeper.**

Linus proved by mutation what Kevlin asserted by counting. Kent settled it by *running the mutant*
rather than adjudicating prose, and found the middle term unpinned — a mutant that warns on **every
ordinary redeploy of every normal service**, destroying Option C's entire value, kept the suite green.

Kent's self-diagnosis is the reusable part, and it's better than "add more rows": *"I tabled the cases
the implementation suggested, not the cases the property required."* He enumerated around the
condition's shape instead of asking **"what states must NOT warn?"** The most common operation in the
product wasn't in his table because it isn't interesting *from the condition's* point of view — it is
the only interesting one *from the operator's*.

Kevlin's slip is worth recording precisely because his review was otherwise the sharpest on the task:
he ran three mutations, two of which were term deletions and one a whole-statement deletion, and wrote
the conclusion as "all three terms are pinned." **Three mutations is not three terms.** Mutation
testing proves what you mutate and nothing else.

## 10.3 The process finding nobody has named

**Both reviewers audited the artifact. Neither audited the delta from the spec.**

- Kevlin hunted **false claims** — things written that are wrong. Found one. Excellent.
- Linus hunted **design defects and untested properties**. Found one. Excellent.
- **Nobody asked: "what did the plan promise that the record doesn't deliver?"**

Both gaps in §10.1 are **omission-shaped**, and omissions are invisible to a source check: a false
claim announces itself the moment you diff it against upstream, but a missing one sits there looking
like nothing at all. CRIME was in my plan, was praised by name in review, and silently failed to make
the trip — and it passed two of the most rigorous reviews this project has seen, plus my own two
re-reads, because everyone was checking *what was on the page*.

**The lesson for Ward:** a permanent record needs a **coverage check against the originating request**,
not only an accuracy check against the code. The cheapest form is a literal one — take the user's own
words, grep the durable artifact for each noun they used, and look at the zeroes. That is exactly how
I found both of these, and it took one command.

## 10.4 What was done right — recorded because it's the reason this worked

I've spent this task being hard on people. This part is not decoration:

- **Raymond refused an instruction from two seniors and was right.** Both gaps in §10.1 are mine.
  Both errors in §10.2 are mine. The most junior surface on this task has the best accuracy record on
  it, because he checked what he was handed and labelled what he couldn't check.
- **Rob turned three seniors' assertion into a panic trace**, then declined his own latitude to polish
  a line three people had already converged on. Knowing when *not* to exercise permission is rarer
  than knowing when to.
- **Kent ran the mutant instead of arguing**, then diagnosed his own testing philosophy rather than
  patching the symptom.
- **The tripwire worked**: a number written down in advance (~8), audited after (8), with the raw-15
  delta explained and independently re-verified. That's why it was worth writing down.
- **"No Docker on this box" held everywhere.** Nobody wrote "validated". Not once, across five agents.

## 10.5 Ruling

- **Required before FULLY DONE:** GAP 1 (CRIME). Raymond. ~2 sentences.
- **Recommended, same edit:** GAP 2 (zstd ordering / negotiation-vs-advertisement contrast). ~3
  sentences.
- **Then:** Joel and Linus confirm; if all three of us agree, finalize (Ward → Andy → squash-merge).
- **Not required:** anything else. Code, tests, and the rest of the docs are approved as shipped.
- **`caddy validate` on the Linux host remains the maintainer's step.** Nothing on this task claimed
  it and nothing should start now.
