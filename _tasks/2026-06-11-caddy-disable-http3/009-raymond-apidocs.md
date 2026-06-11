# 009 — Raymond's Documentation Report: Disable Caddy HTTP/3

Documentation pass for the HTTP/3-disable change. Reconciled the decision record and the
operator docs with shipped behavior per tech-plan §7. Three files touched; README left
unchanged (grep-verified, no h3 claim). No JSX/Next.js — `_docs/` in this project is plain
Markdown (`install.md`, `usage.md`); there is no build step to run.

## Step 1 — Grep reconfirmation (Linus's watch-item, `006` note 1)

Ran `grep -rniE 'h3|http/3|http3|quic|443|udp' _docs/ README.md` BEFORE editing. The
§7.2 hit set is still exact on this branch:

- `_docs/install.md:41` — "(HTTP/3 over QUIC)" parenthetical — EDITED.
- `_docs/install.md:56` — the active "my phone is slow" lie — EDITED.
- `_docs/usage.md:196` — port list, no h3 claim — light-touch qualifier added.
- `_docs/usage.md:320` — port bind list, no h3 claim — light-touch qualifier added.
- `README.md:61` — M1 feature line, port list, no h3 claim — LEFT AS-IS.
- `README.md:124` — prerequisites, port list, no h3 claim — LEFT AS-IS.

Other grep hits confirmed out of scope (no h3 assertion): `install.md:12` (IPv4/IPv6
dual-stack publishing of 80/443), `install.md:171`/`:176` (port-already-bound error),
`usage.md:7`/`:261` (quick-start cross-refs). Left untouched.

All four binding facts carried into every edit: (1) HTTP/3 disabled, only h1+h2
advertised; (2) UDP/443 still published but inert — never told the operator to stop
publishing it; (3) no invented config flag — protocol set stated as fixed/hardcoded;
(4) no "validated" claims.

## Step 2 — `_ai/decisions/caddy-runs-in-container.md` amended (§7.1)

Added a dated **"Amendment 2026-06-10 — HTTP/3 disabled (line-17 premise field-disproven)"**
subsection immediately before the existing "Forward-looking notes" section. The original
line-17 reasoning and all prior content are PRESERVED (history intact). The amendment:

- Records that the line-17 premise was reversed by field experience: iPhone Safari over
  QUIC/UDP-443 *broke* connectivity; broken QUIC with slow/absent TCP fallback presents as
  "my phone hangs," not "slow."
- States HTTP/3 is now disabled at the Caddyfile level via
  `{ servers { protocols h1 h2 } }` emitted by `internal/caddy/generator.go`; no
  `Alt-Svc: h3` header sent.
- States UDP/443 **remains published but inert** (`manager.go` `runOpts()` unchanged,
  six dual-stack `-p` entries intact); unpublishing it is a deferred, separate change
  requiring a `caddy up` recreate.
- Warns the next engineer NOT to re-enable HTTP/3 to "fix a mobile regression."
- Forward-looking M3 note (§7.3): protocol set becomes a natural `caddy.protocols`
  config knob under Viper/TOML, `h1 h2` default — future only, NOT built now, NOT a
  user-facing flag in M1/M2.
- Cross-references the task dir `_tasks/2026-06-11-caddy-disable-http3/`.

The forward-looking M3 `caddy.protocols` idea is quarantined to the decision record only;
it is NOT mentioned in `_docs/`.

## Step 3 — Operator-doc corrections (§7.2)

### `_docs/install.md:41` (§7.2.2) — parenthetical corrected

Before:
> ... dual-stack publishing on `80/tcp`, `443/tcp`, and `443/udp` (HTTP/3 over QUIC), bind-mounting ...

After:
> ... dual-stack publishing on `80/tcp`, `443/tcp`, and `443/udp` (the `443/udp` port is published but **inert** — HTTP/3 is disabled, so Caddy serves only HTTP/1.1 and HTTP/2), bind-mounting ...

Port stays published; "HTTP/3 over QUIC" benefit framing replaced with published-but-inert.

### `_docs/install.md:56` (§7.2.1) — the active lie, full rewrite

Before:
> Open `80/tcp`, `443/tcp`, and `443/udp` (HTTP/3) on any host firewall (`ufw`, `firewalld`, cloud security group). Without UDP/443 the listener still works for HTTP/1.1 and HTTP/2, but mobile clients that negotiate HTTP/3 silently fall back and the symptom looks like "TLS works but my phone is slow."

After:
> Open `80/tcp` and `443/tcp` on any host firewall (`ufw`, `firewalld`, cloud security group). Decloud also publishes `443/udp`, but **HTTP/3 (QUIC) is disabled** — Caddy advertises only HTTP/1.1 and HTTP/2 (`servers { protocols h1 h2 }` in the generated Caddyfile). UDP/443 is therefore **published but inert**: nothing listens on it once HTTP/3 is off, so opening it on the firewall is optional and harmless. HTTP/3 was disabled deliberately because iPhone Safari over QUIC/UDP-443 was breaking connectivity in the field; see `_ai/decisions/caddy-runs-in-container.md`.

The "my phone is slow ⇒ open UDP/443" reasoning is fully removed. UDP/443 framed as
optional/harmless to open, never "stop publishing."

### `_docs/usage.md:196` (§7.2.3) — one-clause inert qualifier

Appended to the existing port-publishing clause of the `decloud caddy up` bullet:
> ... dual-stack publishing on `80/tcp`, `443/tcp`, `443/udp` (`443/udp` is published but inert — HTTP/3 is disabled; Caddy advertises only HTTP/1.1 and HTTP/2). Idempotent: ...

Rest of bullet unchanged.

### `_docs/usage.md:320` (§7.2.4) — one-clause inert qualifier

Appended to the existing `decloud-caddy` port-bind sentence in §6:
> ... it binds `80/tcp`, `443/tcp`, and `443/udp` on both `0.0.0.0` and `[::]` (the UDP/443 bind is inert — HTTP/3 is disabled). Every service container ...

Rest of paragraph unchanged.

### `README.md:61`, `README.md:124` (§7.2.5) — LEFT AS-IS

Grep-confirmed: both are plain `80/tcp`/`443/tcp`/`443/udp` port lists with NO h3 claim
(`:61` = M1 feature line; `:124` = firewall prerequisites). Neither reads as "open UDP/443
for HTTP/3," so per §7.2.5 default no change is mandated. The port really is still
published, so `443/udp` stays in both lists.

## Honesty discipline (§6.4 / §7.2.6)

No doc or this report claims the Caddyfile is "validated." The edits describe the
*intended* behavior (h3 disabled, h1+h2 advertised). The real `caddy validate` +
iPhone-Safari-over-real-network check is the maintainer's manual integration step on the
Linux host (no Docker on this dev box). The code change is byte-asserted; pending operator
`caddy validate`.

## Verification

- `git diff --stat`: `_ai/decisions/caddy-runs-in-container.md` (+14),
  `_docs/install.md` (4 changed), `_docs/usage.md` (4 changed). 3 files, no README change.
- `go test ./internal/caddy/...` → `ok` (unaffected by docs; confirms nothing collateral).
- No JSX/Next build: `_docs/` is plain Markdown in this project; no build step exists.

## Files changed

- `/Users/fenster/dev/decloud/_ai/decisions/caddy-runs-in-container.md` (new dated amendment)
- `/Users/fenster/dev/decloud/_docs/install.md` (lines 41, 56)
- `/Users/fenster/dev/decloud/_docs/usage.md` (lines 196, 320)
- `/Users/fenster/dev/decloud/_tasks/2026-06-11-caddy-disable-http3/009-raymond-apidocs.md` (this report)
