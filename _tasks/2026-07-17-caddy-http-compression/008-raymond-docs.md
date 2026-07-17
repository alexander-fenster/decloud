# 008 — Raymond: docs for HTTP compression on by default

> EXECUTION step 3. Docs for `68bf792`. Every claim below is traceable to shipped code; the
> verification I actually ran is in §6.

## 1. What I changed — the audit trail

Four files. `git diff` for the docs is 46 insertions across three modified files plus one new file.
Nothing else in the working tree.

| File | Change |
| --- | --- |
| `_docs/usage.md` | `--no-compression` flag row; hand-edit trap rewrite + TOML placement example; deploy step 7; `caddy reload` upgrade note |
| `_ai/caddyfile-generator-facts.md` | Two new sections: `encode` is site-level-only; `match` cannot protect a streaming service |
| `_ai/apidocs.md` | Two new bullets: the per-service Caddy knob; TOML top-level-key placement rule |
| `_ai/decisions/http-compression-on-by-default.md` | **New.** The decision record, all 10 specified items |

### 1.1 `_docs/usage.md` — four edits, not one

1. **Flag table row** (after `--mount`, per Joel §8). `bool` / `false` / not required. Carries: on by
   default (`encode zstd gzip`), set it for streaming/SSE backends, the #6293 link, the mechanism in
   one clause, **the not-sticky caveat**, and that it persists as `disable_compression`.
2. **The hand-edit trap** (Linus Issue B — the required fix). Rewrote the invitation to hand-edit.
   Details in §2, including a correction to the plan's own wording.
3. **Deploy step 7** — the Caddyfile regeneration step now says every emitted site block carries
   `encode zstd gzip`, **one per hostname**, unless `--no-compression`.
4. **`decloud caddy reload` upgrade note** — the live behavior change, called out plainly next to the
   existing "discards manual edits" warning. §3.

**Exit codes table: untouched**, per Joel's explicit instruction. The warning is a warning; the deploy
still exits 0. No new failure mode. (One exit-10 path *is* now mentioned in prose — see §2 — but it is
the existing `ErrUnknownField` registry rejection, already row 10 in the table. No new row.)

### 1.2 `_docs/install.md` — deliberately NOT touched, with a reason

Joel called this a judgement call ("don't force it"). I read `install.md:56`: it is the **host
firewall** paragraph. It mentions the generated Caddyfile only to explain why UDP/443 is inert. There
is no sample Caddyfile and no site block anywhere in the file (`grep`ed: only lines 40, 56, 194, all
about the stub or ports). Compression has nothing to do with firewall ports; a sentence about `encode`
there would be a non-sequitur in the one paragraph an operator reads while deciding which ports to
open. **Skipped on purpose, not by omission.**

## 2. The hand-edit trap — and a stale claim in the plan I did not copy

Don §5 and Joel §8 both specified the wording: a hand-set `disable_compression` "survives
`decloud caddy reload` but is **silently** wiped by the next `decloud deploy`."

**The "silently" is stale, and I did not ship it.** It was written before Don's §7.1 Option C ruling.
The warning fires on `hasPrev && prev.Config.DisableCompression && !req.DisableCompression` — driven
by `prev.Config`, which is loaded from the TOML. It does not and cannot know whether that `true` came
from a flag or from a text editor. **So a hand-set `disable_compression = true` is warned about on
reset, not silently wiped.** Option C closed this hole for this key as a side effect.

What I shipped instead, which is what the code does: hand-edited values do not survive the next
deploy; for most keys that replacement is silent; `disable_compression` is **the exception** that
warns. That is a stronger version of the trap warning — it tells the truth *and* still routes the
operator to `--no-compression` as the durable answer.

**Flagging for Kevlin:** this is the one place my doc deliberately contradicts the literal plan text.
The plan's wording would have been a fabrication.

### 2.1 A trap I found that nobody specified — TOML key placement

Writing the example forced a question the plan never asked: *where in the file does this key go?*

`disable_compression` is a **top-level** `ServiceConfig` scalar. TOML binds a bare key to the table
that precedes it. The real file layout (`validConfigTOML`, and what `Save` emits) is top-level scalars
first, then `[source]`, `[build]`, `[run]`, `[[routes]]`, `[readiness]`, `[state]`. So an operator who
appends `disable_compression = true` to the end of the file — **the obvious thing to do** — produces
`state.disable_compression`, which the strict decoder rejects.

I did not reason my way to this. I probed it (§6.2). Appending at end of file:

```
registry: decoding config .../foo.toml: registry: unknown field in TOML: 35| disable_compression = true
  | ~~~~~~~~~~~~~~~~~~~ missing field
```

→ exit 10. Correct placement (above the first `[table]` header) loads fine with
`DisableCompression == true`.

So the usage.md example **shows the key in its correct position** and names the failure. A naive
example would have handed the operator a service that will not load. Recorded as a general rule in
`_ai/apidocs.md` — it applies to `strategy` and every future top-level scalar, not just this key.

## 3. The live behavior change

Documented under `decloud caddy reload` as an **Upgrade note**, in the operator's language: this
command turns compression on for **every existing service**, including ones deployed before the flag
existed, because old TOMLs have no key and an absent key reads `false` = on. Stated as intended,
stated as *also* a live change to services running fine yesterday, with the symptom (streams hanging
on open) and the fix (redeploy with `--no-compression`) adjacent.

Also documented: **there is no `--no-compression` on `decloud caddy reload`.** The setting lives in
the registry and only a deploy writes it. Verified — `caddy_reload.go` declares `Args: cobra.NoArgs`
and no flags.

**Reset caveat is in both places** Joel demanded: the flag row *and* the `caddy reload` bullet
(third place too — the hand-edit paragraph). Not one.

## 4. The decision record

`_ai/decisions/http-compression-on-by-default.md`, modeled on `journald-log-driver.md` (context →
decision → why → rejected alternatives → why-not-in-`_docs`). All 10 of Don §5.4 / Joel §8:

1. On by default + why; the polarity table and why `false` must mean "on".
2. **BREACH rejected, strongest-first** — the knob is useless against it, mitigation is app-side, a
   reverse proxy cannot fix it. "Industry ships it" is explicitly **labelled** as an appeal to
   authority and supporting evidence only.
3. **SameSite stated carefully** — "`Lax` **still sends cookies on top-level cross-site GET
   navigations**… raises the bar **considerably**". The word "guts" appears only in a note saying it
   was an overstatement and must not be restored.
4. **"Streaming", not "SSE"** — headers-then-idle; nothing about `text/event-stream` selects the
   failure; long-poll and chunked progress in the same blast radius.
5. **`Vary: Accept-Encoding` handled upstream** — checked, safe, no action, including the 304 case.
6. **The knob buys ZERO bytes** — `init()` only from `Write`, only above 512 bytes, never retried, so
   a typical stream runs `rw.w == nil` forever. Fixes header timing, not bytes. With the explicit
   warning that this is what gets the knob deleted as dead weight.
7. **The reusable knob rule** — silent + misattributed + unworkaroundable; 3-for-3 here; if the
   failure is loud, kill the knob. Includes why the anti-knob argument is good and normally wins.
8. **Why `streaming = true` was rejected** — promises semantics we don't implement; a lie in the
   config file. Plus `request_header -Accept-Encoding` rejected as too clever.
9. **The retirement condition** — with the #6293 link and its verified state (open, created
   2024-05-02, updated 2026-03-17). Notes retirement is a deprecation, not a delete.
10. **Redeploy reset + Option C warning** (with the exact shipped message, why no `<svc>`
    interpolation, why not sticky, why warn when `--mount` doesn't) **and the downgrade hazard**
    (strict decoder, new TOML + old binary → exit 10, no supported downgrade, `schema_version` stays
    `1`).

Plus, per Linus `005` §5 and Rob §7: **the known false negative**, in its own subsection —
`ErrSecretsMissing` falls through with `hasPrev == false`, so a service whose secrets dir was deleted
and which had `disable_compression = true` redeploys with **no warning**. Recorded as *deliberately
not fixed*, with the reasoning, explicitly so nobody files it as a defect and "fixes" it. **I verified
this at source rather than trusting the report** (§6.1).

Non-goals and the `Verification status` section ("byte-asserted; **pending operator `caddy
validate`**" — nobody on this task ran `caddy validate` or observed a real stream) are both in.

## 5. Rob's open item — the help text. My recommendation: keep it, docs now match

Rob's help text: `set this for streaming/SSE backends`. Don §3.0 says the failure is
headers-then-idle, not SSE-specific.

**These do not conflict, and I think Rob got it right.** Don's §3.0 concern is precise: nobody should
conclude that `Content-Type: text/event-stream` *selects* the failure, because that leads them to
`match`, which does nothing. The help text leads with **"streaming"** and offers SSE as the second
term — it never implies SSE is the cause. Meanwhile "SSE" is what an operator with a hanging
`EventSource` actually types into a search box. Dropping it to satisfy a terminology rule would cost a
real operator the moment of recognition and buy nothing, because "streaming" is already first.

**`_docs/usage.md` now matches the shipped help text verbatim** ("streaming/SSE backends") — surfaces
3 and 4 agree, per Joel §8. The doc carries the framing the help text has no room for: the flag row
explains the headers-then-idle mechanism, and `_ai/caddyfile-generator-facts.md` states outright that
this is not an SSE bug and that `match` will not fix it.

**No change requested.** If Don or Linus overrule me, the change is one line in
`deploy_service.go` + one in `usage.md`, and they must move together.

## 6. Verification — what I actually ran

Accuracy was the job. I did not take any report's word for a code fact.

### 6.1 Read at source, not from reports

- `internal/caddy/generator.go` — the `encode zstd gzip` literal, the `!in.DisableCompression` guard,
  4-space indent, position before `reverse_proxy`, **inside the per-hostname loop** (which is why the
  docs say "one per hostname").
- `internal/registry/types.go` — `DisableCompression bool \`toml:"disable_compression"\``, top-level.
- `internal/cli/deploy_service.go` — `--no-compression`, `BoolVar`, default `false`, and the exact
  help string I matched.
- `internal/deploy/service.go` — the warning condition and message; **and the `ErrSecretsMissing`
  fall-through, which I read myself.** Confirmed: the guard excludes both `ErrNotFound` and
  `ErrSecretsMissing` from the fatal path, so `hasPrev == false` and no warning. Linus's recorded false
  negative is real.
- **`decloud caddy reload` → `Generate`, traced end to end** — because my entire upgrade note depends
  on it. `caddy_reload.go` → `lc.CaddyReload` → `lifecycle.go:177` → `regenerateAndReload` →
  `Store.List` → `Generator.Generate`. `Generate` has exactly one production caller. The note is sound.

### 6.2 Empirical probes

- **TOML key placement** (§2.1) — throwaway test against the real `fsStore.Load`, both placements.
  Correct placement loads; appended-at-end fails with `ErrUnknownField`. **Probe file deleted**; it
  was a Raymond-only investigation, not a test I'm asking anyone to maintain (and it would have been a
  change-detector).
- `go build ./...` clean; `go test ./internal/{caddy,deploy,cli,registry}/` green.

### 6.3 `grep -F` on every quoted literal (`_ai/doc-grep-discipline.md`)

Three literals appear in my docs as bytes someone will compare against reality:

| Literal | Source |
| --- | --- |
| `compression re-enabled: previous deploy set disable_compression; pass --no-compression to keep it off` | `internal/deploy/service.go` ✓ |
| `unknown field in TOML` | `internal/registry/errors.go` (`ErrUnknownField`) ✓ |
| `encode zstd gzip` | `internal/caddy/generator.go` ✓ |

All matched. Note I quote the slog message **only in the decision record**, not in `usage.md` — the
operator-facing doc says "the deploy logs a `WARN`" and does not commit to bytes, so a future wording
polish cannot rot `_docs/`. The one place the literal lives is the doc a contributor changing that line
would already be reading.

### 6.4 Cross-reference discipline (`_ai/cross-ref-content-not-line-number.md`)

No bare line-number cites into prose files. The one code-side cite I drafted
(`service.go:184` for the `ErrSecretsMissing` guard) I converted to content-based phrasing naming the
guard expression — line-stable per rule 2, but the guard is inside a function body that will move, and
a `grep`-able anchor costs nothing.

## 7. Style notes

- "coupons"/Shopify conventions in my agent brief are Bubblehouse-specific and do not apply to this
  Go PaaS repo; house style here is `_docs/*.md` prose. `_ai/apidocs.md` already records that `_docs/`
  is **plain Markdown, not a Next.js/JSX app — no `next build`**. I did not run it.
- Said "streaming" first everywhere, per Don §3.0.
- Did not touch `_tasks/` history (immutable by convention).

## 8. For Kevlin — where to aim

1. **§2's contradiction of the plan text** — I dropped "silently" from the hand-edit trap because
   Option C makes it false for this key. If you think the warning does *not* fire for a hand-set
   value, that is a real bug in my doc; the claim rests on the condition reading `prev.Config`, which
   has no idea where the value came from.
2. **§2.1's TOML placement claim** — probed, output quoted above, probe deleted. Re-run it if you want;
   it takes 30 seconds.
3. **The `caddy reload` upgrade note** — the strongest claim in the docs ("turns compression on for
   every existing service"). Trace §6.1's chain if you doubt it.
4. **The decision record's Caddy-internals claims** (`init()` never retried, `match` consulted only
   inside `init()`, `Vary` on the 304 path) — these are **Don's and Linus's source reading of
   upstream Caddy, not mine.** I could not verify them against `encode.go` from this box; I have not
   fetched upstream source. I phrased them as mechanism rather than line-number cites for that reason.
   **If you want them independently confirmed, that is a real gap and I would rather you catch it than
   have it sit in a permanent decision record.**

---

# Addendum — Kevlin's §3.2 correction applied (post-`010-kevlin-review.md`)

Kevlin: **APPROVED conditional on one doc correction — mine.** Applied, plus his three non-blocking
imprecisions. Docs only; `internal/` untouched (Kent and Rob are working there in parallel).

## A1. The required fix — my §8.4 flag was a real bug, not caution theatre

I flagged the upstream-Caddy claims in §8.4 as **relayed from Don and Linus, not verified by me**. That
flag caught a false claim. Kevlin fetched `encode.go` from master:

> `init()` is called from **two** places, not one: `Write` (`:349`) **and `Close` (`:423`)**, on the
> bodyless `Content-Length` path.

It appeared in **both** `_ai/decisions/http-compression-on-by-default.md` and
`_ai/caddyfile-generator-facts.md`. I had written it as "only called from `Write`" — inherited from
Don §3.3.1, relayed without independent verification, exactly as I said I had done.

**The conclusion is unaffected** (`Close`'s call is gated on `!wroteHeader`, so it cannot rescue a
stream that already wrote headers; `match` still cannot prevent wrapper installation; the knob's
justification stands entirely). **The harm was the record's credibility**: a reader who checks it
against upstream finds it false and discounts the correct paragraphs around it — the precise failure
the record exists to prevent.

**Kevlin supplied exact replacement text and I used it**, adding only the `>`-not-`>=` precision and
the "both call sites are gated on `!rw.wroteHeader`" clause, which is what makes "never retried" true
*given* two call sites:

- **Decision record, "The knob buys ZERO bytes"** — `init()` called from `Write` **and, on the bodyless
  path, `Close`**; from `Write` only when the first write exceeds 512 (strictly `>`) **or the response
  declares a `Content-Length` above it**; never retried, both sites gated on `!rw.wroteHeader`.
  Kevlin's "a chunked stream declares no `Content-Length`" clause is in — it is what keeps the
  zero-bytes conclusion airtight now that `Content-Length` is a second trigger.
- **`caddyfile-generator-facts.md`** — "only reached from `Write`" → "reached only from `Write` and
  `Close` — **never before the wrapper is installed**". His wording; it makes the load-bearing point
  ordering, not call-site count, which is the thing that actually matters.

## A2. The three minor imprecisions — all three fixed

1. **"`Accept-Encoding` alone"** → **"request headers alone"** in both files, naming the other gates
   (`isEncodeAllowed` on request `Cache-Control: no-transform`, `Sec-WebSocket-Key`). All
   request-scoped, so "before the response Content-Type is knowable" is untouched — but "alone" was
   simply false.
2. **512-byte threshold** — folded into A1. Also fires on a **declared `Content-Length`**, and it is
   `>`, not `>=`.
3. **WebSockets** — my mechanism was understated. I cited "a `101` + hijack never writes through the
   wrapper" (true, but not the guard). The real one is stronger: `AcceptedEncodings` skips non-identity
   encodings when `Sec-WebSocket-Key` is present (`encode.go:502,532-535`), so **the wrapper is never
   installed at all**. Now cited, with a note that it is a *request*-header check — consistent with
   everything else in that section.

### A2.1 Kevlin's "bonus" — added, and he undersold it

He noted parenthetically that `init()` re-checks `isEncodeAllowed` on the **response** header
(`:457`), so a backend setting `Cache-Control: no-transform` stops compression **but not the wrapper**
— and called it "worth a sentence; the strongest evidence for the whole design."

**He is right, and the record did not contain it.** Added to "Why the knob exists" as its own
paragraph, framed as the general asymmetry rather than one header's quirk: *every* response-side lever
— ours, the backend's, `match`, `no-transform` — is downstream of the wrapper; only not emitting
`encode` is upstream of it. That is the whole design in one sentence, and it is upstream's own
structure arguing our case. It closes the "surely there's a cleverer response-side fix" question
permanently, which is exactly what a decision record is for.

## A3. One consistency edit Kevlin did not ask for — flagging it

His §3.2 fix created a tension with a paragraph he separately verified as "exactly right": the
`Vary`/304 text said the 304 case is handled separately "because `init()` **only runs when a body is
written**". With `Close` now documented as a second call site, that reads as self-contradictory.

The reasoning is still correct — upstream added explicit 304 handling at `:265-271` precisely because
the body-write path misses it — so I made the **minimal** precision edit rather than rewriting a
verified paragraph: "because **the body-write path** only reaches `init()` once a body is actually
written". True regardless of `Close`, and Kevlin's verified reasoning is preserved intact.

**Called out because it is my judgment, not his instruction.** If anyone thinks I should have left his
verified paragraph untouched, revert this one line — nothing else depends on it.

## A4. What I did NOT change, and why

- **`_ai/apidocs.md` TOML trap** — no change. Linus asked it be generalized if it implied the trap was
  specific to `disable_compression`. It does not: it already reads "top-level `ServiceConfig` scalars
  (`strategy`, `disable_compression`, ...)" and names the strict-decoder mechanism, not the key. Linus
  §2 independently ratified this ("covers `strategy` and every future top-level scalar rather than
  documenting only his own key — right diagnosis, right scope"). Re-read to confirm; nothing to fix.
- **`_docs/usage.md`** — **untouched by this addendum.** Every correction was to upstream-Caddy
  internals, which appear only in `_ai/`. The operator doc deliberately never committed to those bytes
  (§6.3), so it could not rot. That separation just paid for itself.
- **The "silently" wording** — stays. Both Kevlin (§4) and Linus confirm the plan text is stale and my
  version is correct; Kevlin grepped for provenance tracking (`provenance|handSet|fromFlag`) across
  `internal/registry` and `internal/deploy` and found **nothing**, so the code cannot distinguish a
  hand-set value from a flag-set one. Linus adds the detail that clinches it: **Kent's test 14 sets
  `prev.Config.DisableCompression` directly on the struct, never through a flag — that *is* the
  hand-edit case, and it is already green.** The behavior I documented is under test.
- **`_tasks/` history** — untouched, per Linus's explicit ruling: plans are history, code is truth, the
  decision record outlives both. Correcting Don §5 / Joel §8 is the PLAN step's call, not mine.

## A5. Verification of the addendum

- `grep`ed both `_ai/` files for residual instances of every corrected phrase: **zero** remaining hits
  for "only called from `Write`", "only reached from `Write`", or "`Accept-Encoding` alone".
- Re-read every surviving `init()` mention (4 across both files) for consistency with the two-call-site
  fact — that sweep is what surfaced A3.
- `_docs/` confirmed clean of upstream-internals claims.
- Line-number cites in this round (`:349`, `:423`, `:457`, `:502`, `:532-535`, `:265-271`) are
  **Kevlin's, from his fetch of upstream master** — I still have not fetched `encode.go` from this box.
  Same honesty as §8.4: they are relayed, now from a reviewer who verified them at source against the
  specific claim in question, which is a materially better provenance than the first round had. The
  standing caveat holds: **anything in this record cited to `encode.go` line numbers is upstream
  reading, not mine.**

**Nothing else in the docs changed.** The `Vary`/304 paragraph, the TOML trap, the polarity table, the
BREACH argument, the knob rule, and the false-negative record all stood up to Kevlin's source check
unmodified.

---

# Addendum 2 — Don's two discussion gaps closed (post-`002-don-plan.md` §10)

Don: code approved as shipped; **the discussion has two holes.** Both are in the decision record —
**my document, my omissions.** Closed. Docs only; no code, no tests.

## B1. GAP 1 (required) — CRIME was answered in the plan and absent from the record

`grep -ri "crime" _ai/ _docs/` → **zero hits**, against a user request that literally says
*"BREACH/CRIME-style attacks over TLS"*. Don §2.1 dismissed it; Linus `004` §3 singled the dismissal
out as "correct and worth making"; it then evaporated between plan and record. The durable record
answered BREACH seven times and CRIME **not at all**.

Fixed in the BREACH section, **including the heading** — it now reads "BREACH **and CRIME** were
considered and rejected as reasons", because a reader scanning `##` headings for "is CRIME handled?"
was the exact reader who would miss it. CRIME goes **first**: it is the fastest to dismiss and the
most likely to be raised.

> CRIME attacked **TLS-level** compression (and SPDY header compression) — a layer that does not exist
> in TLS 1.3 and is disabled in TLS 1.2 deployments. Caddy's `encode` is HTTP **response-body**
> compression. **Different layer.** Citing CRIME against `encode` is cargo-cult security.

I added one point beyond Don's two sentences, because the record's job is to answer the *knob*
question and CRIME's dismissal is otherwise incomplete: **the knob is irrelevant to CRIME** — there is
nothing for it to switch off, since Decloud never enabled TLS compression in the first place. That
parallels the BREACH argument's own strongest move ("the knob is useless against it") and keeps both
threat dismissals answering the same question. Also recorded *why* it is in the record at all: an
unanswered objection gets re-litigated forever, and "compression over TLS" reliably summons this one.

## B2. GAP 2 (recommended) — the ordering rationale, and the scar that makes it a real question

The user asked about *"CPU cost, and gzip vs zstd ordering"*. The record stated `encode zstd gzip` and
"zstd is faster" but never said **why preferring the newer encoding is safe** — which is the actual
question, and one this repo has a specific reason to ask.

Given the weight (a user-asked question plus a repo scar), I gave it **its own `##` section** rather
than a bullet — a bullet inside "Why on by default" is not where someone about to touch
`encode zstd gzip` will look:

**"Why `zstd gzip` in that order — and why the HTTP/3 scar does not transfer"**

- **h3 broke Safari because Caddy *advertises* it** — `Alt-Svc` is pushed at the client regardless of
  whether that client's QUIC path works. The client is *told* to try something the server cannot know
  is broken. **Server-driven.**
- **Compression cannot do that, because encoding is *negotiated*** — a client only ever receives an
  encoding it advertised in its own `Accept-Encoding`. zstd is Chrome 123+/Firefox 126+/Safari 18.4+;
  older clients transparently get gzip, one offering neither gets identity. **Client-driven.**
- **Opposite mechanisms, opposite risk. The h3 precedent does not transfer.**

The framing I added: the two changes **look similar** ("newer protocol first in a Caddy directive")
and are **structurally nothing alike**. That resemblance is the whole hazard — it is why the question
gets asked and why a bare "zstd is fine" would not have settled it. Cross-referenced to
`_ai/caddyfile-generator-facts.md`, which already documents the `Alt-Svc`-gated-on-the-listener half;
Don was right that the two docs were one sentence apart from closing this permanently.

Also folded in while there, since both were user-asked and thinly answered: the **CPU** bullet now
carries the actual trade (single-digit ms against 60–80% fewer bytes, `minimum_length` skipping the
trivia, no tuning until a profile says otherwise), and the **brotli** exclusion now carries its
mechanism (`Prefer` lists `br` only if a pool is registered; a stock build has none) rather than
sitting only as a bare non-goal.

## B3. Don's framing — why both slipped, and what it means for my own checklist

Both reviewers audited **the artifact**. Kevlin hunted false claims and found one; Linus hunted design
defects and found one. **Nobody audited the delta from the originating request.** Don's line is the
one worth keeping: *a false claim announces itself to a source check; a missing one sits there looking
like nothing.*

That is a gap in **my** process specifically, not the reviewers'. My §6 verification was thorough
about everything I *wrote* — `grep -F` on every literal, source-reading every Decloud claim, an
empirical probe — and had **no step that walked the originating request and asked "is each thing it
asked for answered in the durable record?"** Every check I ran was artifact-inward. The request named
three things (BREACH/CRIME, CPU, gzip-vs-zstd ordering); the record fully answered one.

`grep -ri "crime" _ai/` is a two-second check that would have caught GAP 1 before Kevlin's review, let
alone Don's. The reason it never ran is that **I was verifying claims rather than auditing coverage** —
and only one of those has an obvious failure signal. **Recommending to Ward** as a norm for any doc
whose source is a user request: before reporting done, grep the durable record for each noun the
request used. Omission-shaped gaps need a checklist because they produce no error to notice.

## B4. Provenance — unchanged caveat

**Neither addition needs an `encode.go` line cite, and neither has one.** CRIME is a layer distinction
(TLS-level vs response-body); the h3 contrast is architectural (`Alt-Svc` advertisement vs
`Accept-Encoding` negotiation) and rests on `_ai/decisions/caddy-runs-in-container.md` and
`_ai/caddyfile-generator-facts.md`, both in-repo and read by me. The zstd browser-version floors
(Chrome 123+/Firefox 126+/Safari 18.4+) are **relayed from Don §2.6, not verified by me** — flagged
per the standing rule. They are load-bearing only for "older clients get gzip", which holds on the
negotiation mechanism regardless of where the floors sit exactly.

The standing caveat is unchanged: **anything in this record cited to `encode.go` line numbers is
upstream reading (Don's, Linus's, or Kevlin's), not mine.**

## B5. What I did not touch

- **`_docs/usage.md`** — untouched again. CRIME and encoding-negotiation are *why*, not operator
  procedure; `_ai/decisions/` is their home per the house split.
- **`_tasks/`** — untouched. Don accepted the no-retro-edit ruling; §5's stale "silently" stays as
  history.
- **Everything Kevlin and Linus verified** — untouched.
