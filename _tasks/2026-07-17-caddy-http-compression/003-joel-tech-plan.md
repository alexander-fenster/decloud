# 003 — Joel's Tech Plan: HTTP compression in the generated Caddyfile

> Status: PLAN step 2, **rev 2**. Expands `002-don-plan.md` (rev 2, `f9990ab`) into an
> implementation-ready spec. Incorporates `004-linus-plan-review.md` (APPROVE) and Don's §7.1 ruling.
> No implementation code here. **Kent executes from §6. Rob executes from §5.** Raymond: §8.

## Rev 2 changelog

- **§4 rewritten — Option C adopted.** My silent-reset defense was **overruled**, correctly. Deploy
  now warns when it flips compression back on. Full spec: call site, message, condition.
- **§5.3 — the warning's implementation spec**, incl. two facts I verified that change the shape of
  it (`logger` already carries `service`; the handler is JSON to stderr **and** file).
- **§6.2 test 8 — my index warning is STRUCK.** It was inverted. I re-ran it myself; Linus is right.
- **§6.5 — new test 14** for the warning transition.
- **§8 — Raymond's items grow**: hand-edit trap (`usage.md:150`), reset caveat in **both** places,
  and the decision record's now-specified contents.
- **§11 — diff tripwire restated: ~5 → ~8 production lines.** Deliberate, approved, not creep.

## 0. Verdict

**Architecture unchanged and now approved by all three of us.** Linus independently re-verified §3.4
and my re-verification of it; both hold. `encode zstd gzip` in every site block, per-service
`disable_compression` opt-out, zero value = ON, **plus a warning when the opt-out resets** (§4).

My four findings against rev 1 all landed: §2 (strict TOML decoder / downgrade hazard), §3 (the
`caddy validate` correction), §6.2 (Don's test-4 off-by-one), §6.1 (the `makeService` call he
delegated). Linus credited §6.5 (the untested `Save` seam) as the most valuable item; it's specified
as test 13 and stays.

**Two things I got wrong, recorded plainly:**

1. **The test-8 index warning was inverted** (§6.2). I told Kent the simple form was a trap. It
   isn't. I re-ran it against the real emitted bytes before striking it — `Index("\n}\n")` = 135 on a
   realistic body, and `body[:135]` is exactly the global options block, because the inner `servers`
   close is `"\n    }\n"` (four spaces) and cannot match `"\n}\n"`. **I preached verification and then
   shipped an unverified "gotcha."** Struck.
2. **My redeploy-reset defense was right on the facts and wrong on the conclusion** (§4). Overruled;
   see §4 for why I think the overrule is correct.

---

## 1. Independent verification of §3.4 — Don is correct

Don told me to verify §3.4 myself or move on. I verified it. Fetched current `master`
`modules/caddyhttp/encode/encode.go`.

**`Encode.ServeHTTP`** — wrapper install:

```go
func (enc *Encode) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	if isEncodeAllowed(r.Header) {
		for _, encName := range AcceptedEncodings(r, enc.Prefer) {
			if _, ok := enc.writerPools[encName]; !ok {
				continue // encoding not offered
			}
			w = enc.openResponseWriter(encName, w, r.Method == http.MethodConnect)
			defer w.(*responseWriter).Close()
			break
		}
	}
	err := next.ServeHTTP(w, r)
```

The gate is `isEncodeAllowed(r.Header)` + `AcceptedEncodings(r, enc.Prefer)`. **Request headers only.**
No Content-Type, no `Match`, no response state — none of it is reachable at this point, because the
response does not exist yet. `enc.Matcher` is consulted only in `responseWriter.init()`:

```go
func (rw *responseWriter) init() {
	if rw.disabled { return }
	hdr := rw.Header()
	if hdr.Get("Content-Encoding") == "" && isEncodeAllowed(hdr) && rw.config.Match(rw) {  // <- Match, here only
```

and `init()` is only reached from `Write`. **`FlushError`** still swallows the pre-body flush:

```go
func (rw *responseWriter) FlushError() error {
	if rw.isConnect && !rw.wroteHeader && rw.statusCode == 0 { rw.WriteHeader(http.StatusOK) }
	if !rw.wroteHeader { return nil }        // <- headers-then-idle SSE hangs here
	if rw.w != nil { if err := rw.w.Flush(); err != nil { return err } }
	return http.NewResponseController(rw.ResponseWriter).Flush()
}
```

Also confirmed: `defaultMinLength = 512`; default match Content-Type list is an **allow-list**
containing `text/*` (so `text/event-stream` matches) plus `application/json*`,
`application/atom+xml*`, `image/svg+xml*`, ~24 more.

**Conclusion: `match` cannot protect SSE.** The wrapper — and the swallowed flush — is installed
before Content-Type is knowable. Don's §3.4 is confirmed at source and is the correct justification
for the knob. **Non-negotiable for Rob: do not "improve" this with a `match` block. It does not work,
and §6.4 test 8 exists to stop it.**

**Decloud line numbers re-verified** (`generator.go:35,41-45,46-53,57-84`; `types.go:9-26,25,69-71`;
`deploy/service.go:52-63,317-343,410`; `cli/deploy_service.go:35-45,57-66,98-109`): **all accurate.**

## 1.1 Caddyfile directive order — verified, not assumed

Don flagged this as "verify, don't assume." Verified against
<https://caddyserver.com/docs/caddyfile/directives>:

> "Many directives manipulate the HTTP handler chain. The order in which those directives are
> evaluated matters, so a default ordering is hard-coded into Caddy."

**Written order in the file does NOT drive execution.** The Caddyfile adapter sorts directives into
Caddy's hard-coded order. In that order, `encode` (middleware phase) precedes `reverse_proxy`
(response-handler phase).

**Consequence for us — this is a nice result:** emitting `encode` *before* `reverse_proxy` is
(a) functionally irrelevant and (b) exactly Caddy's own canonical order, so the generated file reads
the way `caddy fmt` / the adapter would order it anyway. We get correctness for free and the file
looks right to anyone who knows Caddy. Emit it first. The ordering test in §6.4 locks file
*stability and readability*, **not** execution semantics — Kent must say so in the test comment, or
the next reader will think the order is load-bearing and be afraid to touch it.

---

## 2. What Don missed #1: the TOML decoder is STRICT (downgrade hazard)

Don cites `LastDeployedAt` as the backward-compatible-addition precedent and calls it a day. He did
not check the decoder. I did — `internal/registry/store.go:250-251` and `:264-265`:

```go
dec := toml.NewDecoder(bytes.NewReader(data))
dec.DisallowUnknownFields()
```

Unknown keys are a hard error → `registry.ErrUnknownField` → **exit 10**. Locked by
`TestStore_LoadRejectsUnknownConfigField` (`store_test.go:147-156`).

This cuts two ways and only one of them is in Don's plan:

- **Backward (old file → new binary): SAFE.** `disable_compression` absent unmarshals to `false` =
  compression ON. Exactly as Don says. No migration. This is the direction that matters.
- **Forward (new file → OLD binary): BREAKS.** A service TOML carrying `disable_compression = true`,
  loaded by a decloud binary predating this change, is **rejected with exit 10**, not ignored. Every
  command touching that service dies.

**Ruling: accept, document, do not engineer around it.** There is no supported downgrade path in
Decloud, `schema_version` stays at `1` (the key is optional and additive — this is not a schema
break), and this hazard is a property of the strict-decode decision, not of this task. But it is a
real operational fact and it is **not** written down anywhere. **Raymond: one line in the decision
record.** Nobody should rediscover this during an incident rollback at 2am.

## 3. What Don missed #2: `caddy validate` DOES run in production

Don's §5.3 says *"`caddy validate` is the maintainer's step on the Linux host — NOT claimed here"*
and threatens to reject reports claiming validation. **The report discipline is right. The technical
statement around it is half wrong, and if Rob believes it literally he'll misunderstand the system.**

`internal/deploy/service.go:401-424` `regenerateAndReload`:

```go
tmpPath := d.deps.Paths.CaddyfilePath + ".tmp"
if err := d.deps.Generator.Generate(tmpPath, services); err != nil { ... }
if err := d.deps.Reloader.Validate(ctx, tmpPath); err != nil {   // :413 — caddy validate, in prod
	_ = os.Remove(tmpPath)
	return fmt.Errorf("%w: caddy validate failed: ...", ErrCaddyReload)
}
if err := os.Rename(tmpPath, d.deps.Paths.CaddyfilePath); err != nil { ... }
```

`caddy validate` runs on **every deploy and every reload**, via `docker exec` into `decloud-caddy`,
against a temp file, *before* the atomic rename (documented at `_docs/usage.md:167,169`). A malformed
`encode` line cannot reach the live Caddyfile — it fails the deploy with exit 60 and the previous
Caddyfile keeps serving.

The precise statement, which is what belongs in reports:

> `caddy validate` is unavailable **on this dev box** (no Docker), so our tests are byte-level string
> assertions — a proxy, not proof. Production validates on every deploy/reload
> (`service.go:413`). Report wording stays **"byte-asserted; pending operator `caddy validate`."**
> Never "validated."

Same rule, accurate reasoning. Don's report bar stands unchanged.

## 4. Redeploy-reset semantics — Option C (RULED, rev 2)

### 4.1 The fact I confirmed (unchanged, and it stands)

`service.go:317-343` constructs a **fresh** `registry.ServiceConfig` from `req` on every deploy. It
never loads-and-merges the previous config (`prev` exists for container rollback,
`restoreOldContainer` at `:379`). Every field not carried on `deploy.Request` resets to its zero
value. `--mount` behaves identically; `_docs/usage.md:129` already states the contract.

So `deploy --no-compression` followed by a redeploy **without** the flag → compression back ON.

### 4.2 The conclusion I drew from it was wrong

I defended the silent reset as consistent with `--mount` / `--strategy` / `--readiness-path`. Linus
attacked it on **consequence asymmetry** and Don ruled **Option C**. The overrule is correct and I
want to be precise about my own error, because the error is more interesting than the fix:

**Consistency was the right test applied to the wrong axis.** I checked that the *mechanism* was
consistent — same reset, same declarative rule, same code path — and it is. What I never checked was
whether the *consequence* was consistent:

- Forget `--mount` → app can't find its file. **Loud, seconds.**
- Forget `--strategy` → visibly different deploy behavior. **Loud.**
- Forget `--no-compression` → deploys clean, passes readiness, looks healthy, then hangs on stream
  open. **Silent. Misattributed.**

That is the *exact* failure class — silent + misattributed — that Don's §4.2 and Linus's §2 rule
invoke to justify the knob's existence in the first place. **I used the silent-failure argument to
justify building the knob and then failed to apply it to the knob's own reset.** An escape hatch that
silently closes itself, restoring the bug it exists to prevent, is not an escape hatch. Either the
reasoning holds in both places or it holds in neither. Conceded without reservation.

**Option B (sticky / merge from `prev`) stays rejected and I'd have fought it too.** Per-field merge
semantics rot a config surface into "which flags are sticky?" — a question with no good answer, asked
forever, by everyone.

**Option C changes no semantics at all.** The flag still resets. Deploy stays declarative. We surface
a state transition we already compute. That's the cheapest possible conversion of a silent failure
into a loud one, and it costs ~3 lines.

**This is not the "no auto-detection" non-goal.** That non-goal forbids *guessing* whether a service
streams (sniffing Content-Type at deploy time). This compares two booleans already in memory. Don
sharpened the non-goal wording in rev 2 §6 so nobody conflates them. Rob: don't.

Implementation spec: §5.3.1. Test: §6.5 test 14. Docs: §8.

## 5. Field placement, naming, and plumbing

### 5.0 Stylistic precedent — one correction to Don

Don points at `Strategy string` (`types.go:17`) as the precedent. Right for **placement** (top-level
scalar in the same block), wrong for **polarity**. The real precedent for a bool-with-TOML-tag whose
zero value means "the normal case" is `Mount.ReadOnly bool \`toml:"read_only"\`` (`types.go:66`):
absent → `false` → read-write → the default. `DisableCompression` mirrors that discipline exactly.
Cite `ReadOnly` for the bool pattern and `LastDeployedAt` for the additive-field pattern.

Naming polarity is settled and I fully back it: `disable_compression`, **not** `compression` /
`enable_compression`. The zero value must mean "on." Anything else silently disables compression for
every service TOML already on disk. Non-negotiable.

### 5.1 `internal/registry/types.go` — the field

Add to `ServiceConfig` **after `Strategy` (`:17`)**, inside the existing scalar block, above
`Readiness`:

```go
	Strategy  string        `toml:"strategy"`
	// DisableCompression omits Caddy's `encode` directive from this service's
	// site blocks. Absent/false = compression ON (the default). Set it for
	// streaming (SSE) backends: Caddy installs its encoding responseWriter on
	// the request's Accept-Encoding alone, so a pre-body Flush() is swallowed
	// and an idle-first event stream hangs — see caddyserver/caddy#6293. The
	// `match` sub-directive cannot prevent this; only omitting `encode` can.
	// Backward-compatible TOML addition (cf. LastDeployedAt below): existing
	// files without the key unmarshal to false and gain compression.
	DisableCompression bool `toml:"disable_compression"`
	Readiness ReadinessSpec `toml:"readiness"`
```

That comment is long **on purpose**. It is the one place a future maintainer will look before
deleting this field, and it must carry the *why* and the issue link. This is exactly the "next
developer is a violent psychopath who knows where you live" case. `gofmt` will re-align the tags.

`schema_version` stays `1`. No migration. No loader validation — a bool has no invalid value
(contrast `Strategy`, which needs `TestStore_LoadRejectsInvalidStrategy`). **Do not add a validator
for this field.**

### 5.2 `internal/caddy/generator.go` — struct, normalize, emit

**(a) `GeneratorInput` (`:16-21`)** — add the field:

```go
type GeneratorInput struct {
	ServiceName        string
	ContainerName      string
	Port               int
	Hostnames          []string
	DisableCompression bool
}
```

**(b) `normalize` (`:75-80`)** — carry it through:

```go
		out = append(out, GeneratorInput{
			ServiceName:        svc.Config.Name,
			ContainerName:      container,
			Port:               svc.Config.Run.Port,
			Hostnames:          hosts,
			DisableCompression: svc.Config.DisableCompression,
		})
```

**(c) The site loop (`:46-53`)** — emit inside the **hostname** loop, after the opening brace,
before `reverse_proxy`:

```go
	for _, in := range inputs {
		fmt.Fprintln(&buf)
		for _, host := range in.Hostnames {
			fmt.Fprintf(&buf, "%s {\n", host)
			if !in.DisableCompression {
				fmt.Fprintln(&buf, "    encode zstd gzip")
			}
			fmt.Fprintf(&buf, "    reverse_proxy %s:%d\n", in.ContainerName, in.Port)
			fmt.Fprintln(&buf, "}")
		}
	}
```

**Here be dragons:**

- **The `if` goes in the INNER loop, not the outer one.** A service with N hostnames emits N site
  blocks; each needs its own `encode`. Hoisting it out of the inner loop emits it once, in the first
  block only. §6.4 test 7 catches this.
- **4 spaces. Not a tab. Not 2.** Same level as `reverse_proxy`.
  `_ai/caddyfile-generator-facts.md` ("Generated Caddyfile is indented with SPACES, not tabs") and a
  literal `Contains "\n    encode zstd gzip\n"` assertion pin this.
- **`Fprintln`, not `Fprintf` with a trailing `\n`.** Match the neighbours.
- **Do not touch the global options block (`:41-45`).** `encode` is site-level. There is no
  global-options `encode`; it will fail `caddy validate` (exit 60) on the operator's host. §6.4
  test 8.
- **Exact string: `encode zstd gzip`.** Not `encode`, not `encode gzip zstd`. Bare `encode` happens
  to mean zstd+gzip today, but being explicit pins `Prefer = [zstd, gzip]` and documents intent.

**No mock regeneration.** `//go:generate mockgen -source=generator.go` (`:13`) mocks the `Generator`
**interface** (`:24-26`), whose signature is unchanged. `GeneratorInput` is a plain struct, not part
of the interface. `mocks/mock_generator.go` is untouched. Rob: do not re-run mockgen and do not
commit a churned mock file.

### 5.3 `internal/deploy/service.go` — Request → ServiceConfig

`Request` (`:52-63`), append after `Strategy`:

```go
	Strategy         string
	DisableCompression bool
```

`ServiceConfig` construction (`:317-343`), add after `Strategy: req.Strategy,` (`:330`):

```go
			Strategy:           req.Strategy,
			DisableCompression: req.DisableCompression,
			Readiness:          spec,
```

### 5.3.1 The warn-on-reset (Option C) — exact spec

**Condition (Don's §7.1, verbatim):**

```go
hasPrev && prev.Config.DisableCompression && !req.DisableCompression
```

All three terms are required. `hasPrev` guards the nil/first-deploy case; without it this panics on a
service's very first deploy, because `prev` is a `*registry.Service` that is **nil** when
`Store.Load` returned `ErrNotFound`. **Rob: `hasPrev` is not decoration. Check it first.**

**Data is already in hand — no new load.** `prev, loadErr := d.deps.Store.Load(ctx, req.Name)` at
`service.go:179`, `hasPrev := loadErr == nil` at `:180`. Both are already consumed downstream at
`:265`, `:301`, `:362`. `logger` is in scope from `:157`.

**Call site: immediately before the `svc := &registry.Service{` construction at `:317`** — i.e. after
`logger.Info("readiness passed", ...)` (`:311`) and the `routes` loop (`:313-316`), directly above the
struct literal whose field it explains.

**Why there and not at `:180`** (this is a real decision, not a coin flip): at `:180` the warning
fires before build and readiness, so a deploy that *fails* would still have warned about a transition
**that never happened** — a lie in the log, and the worst kind, because it describes state. At `:317`
readiness has passed and `Save` is next; the transition is real. It also sits directly above the line
that causes it, which is where the next reader will look.

**Two facts I verified that constrain the message — both would otherwise produce a wrong line:**

1. **`logger` already carries `service`.** `service.go:157`:
   `logger := slog.With("deploy_id", deployID, "service", req.Name)`. So the message must **not**
   re-add a `"service"` key and must **not** interpolate the name into the text. Linus's and Don's
   draft reads `compression re-enabled for <svc>` — that `<svc>` is **already an attribute** and
   would be duplicated. Drop it from the text.
2. **The handler is JSON, to stderr AND the log file.** `internal/logging/logging.go:43-44`:
   `w := io.MultiWriter(os.Stderr, f)` +
   `slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: slog.LevelInfo}))`. So `Warn` (above
   `LevelInfo`) **does** reach the operator's terminal — **Option C actually works**, which is the
   whole premise and which nobody had checked. It also means the output is a JSON object, not a
   prose line: a `note:` prefix would be noise inside `"msg"`, and slog level `WARN` already carries
   the severity.

**The line:**

```go
	if hasPrev && prev.Config.DisableCompression && !req.DisableCompression {
		logger.Warn("compression re-enabled: previous deploy set disable_compression; pass --no-compression to keep it off")
	}
```

Message content is fixed by the four-surface rule (§5.4): it must name **`--no-compression`** (the
fix) and **`disable_compression`** (what the operator will see in the TOML). Both tokens are load-
bearing — an operator greps for exactly one of them. Wording around them is Rob's to polish;
**those two tokens are not.**

**Not an error, not a failure, not `Stderr` directly.** `logger.Warn`, matching the house pattern at
`:233` (`"removing orphan container from prior interrupted deploy"`). Deploy proceeds normally.

That is the whole deploy-layer change. No validation, no error path, one `if`.

**Note the free win — state it in the report:** the flag lives in the **registry**, and every
Caddyfile regeneration path (`deploy`, `unregister`, `decloud caddy reload`, and the lifecycle
`Start`/`Stop`/`Restart` paths) funnels through `regenerateAndReload` → `Store.List` →
`Generator.Generate`. They all read the persisted field automatically. **Zero plumbing outside the
deploy path.** Rob: if you find yourself editing `lifecycle.go`, stop — you've gone wrong.

### 5.4 `internal/cli/deploy_service.go` — the flag

`deployServiceFlags` (`:35-45`), append:

```go
	Dockerfile       string
	NoCompression    bool
```

Registration, after the `--dockerfile` line (`:66`):

```go
	cmd.Flags().BoolVar(&f.NoCompression, "no-compression", false,
		"disable HTTP response compression (Caddy `encode`) for this service; set this for SSE/streaming backends")
```

Into `deploy.Request` (`:98-109`), after `Strategy: f.Strategy,`:

```go
		Strategy:           f.Strategy,
		DisableCompression: f.NoCompression,
```

**No parse step, no validation, no `errUsage` path.** Cobra's `BoolVar` cannot fail. Do not invent a
`parseCompressionFlag`. Contrast `parseMountFlags` (`:172-189`), which exists only because `--mount`
has a string grammar that can be malformed.

**Note the deliberate name asymmetry:** CLI `--no-compression` ↔ struct `NoCompression` ↔ TOML
`disable_compression` ↔ Go `DisableCompression`. `--no-<x>` is the idiomatic CLI negation; `disable_`
is the idiomatic TOML/Go field. Both keep the zero value = "on". This is intentional, not sloppiness;
Kevlin will ask, so **Rob: put a one-line comment at the `Request` mapping** noting the two names are
the same knob.

**Four-surface doctrine** (`_ai/cli-flag-surface-coherence.md`): a flag's contract lives in
(1) runtime check, (2) error message, (3) `--help` text, (4) `_docs/usage.md`. This flag has **no**
runtime check and **no** error message (nothing to reject), so only surfaces 3 and 4 exist and they
must agree: both must say *disables compression for this service* and both should point at streaming
as the reason. **Do not test the help string** — that's a change-detector, banned by CLAUDE.md; the
semantic-token carve-out does not apply here (there is no milestone token or shared sentinel wording
in play). Review discipline, not test enforcement.

---

## 6. Test surface (Kent owns this)

House style, confirmed by reading the files: `package caddy_test` (external), Testify
`assert`/`require`, table-driven only where there are real cases to table, byte-level `Contains` on
the generated body, `strings.Index` comparisons for ordering. Gomock only where an interface is
mocked (CLI/deploy). **No `t.Parallel()` in `internal/cli` tests** — `deployerFactory` is a
package-global test seam (`deploy_service.go:21-26`).

### 6.1 The `makeService` question — Don delegated this to me

Current helper (`generator_test.go:15-28`) is `makeService(name string, port int, hostnames ...string)`.
The variadic tail is the problem: **you cannot add a `bool` parameter without breaking the variadic
or every one of the existing call sites.**

**Decision: leave `makeService` exactly as it is. Mutate the returned struct in the two tests that
need the flag.**

```go
svc := makeService("streamy", 8080, "streamy.example.com")
svc.Config.DisableCompression = true
```

Rejected alternatives, for the record:

- `makeServiceNoCompression(...)` — a near-duplicate helper for one bool. Copy-paste rot.
- `makeService(name, port, disableCompression, hostnames...)` — churns all 6 existing call sites and
  puts an unexplained `false` in every one of them. Worse to read.
- Functional options — over-engineering a 13-line test helper. No.

Two mutation lines in two tests beats all of it. The struct field is public and the intent is
obvious at the call site. **Kent: two lines, no new helper.**

### 6.2 `internal/caddy/generator_test.go` — the primary surface

Model: `TestGenerator_DisablesHTTP3` (`:81-100`) — content + exact indentation + ordering.

1. **`TestGenerator_EmitsCompressionByDefault`** — default service ⇒
   `assert.Contains(body, "\n    encode zstd gzip\n")`. Literal, with the leading newline and the
   4 spaces. This one assertion pins content **and** indentation, exactly like the `protocols` line.
2. **`TestGenerator_CompressionPrecedesReverseProxy`** — `strings.Index(body, "encode zstd gzip") <
   strings.Index(body, "reverse_proxy")`. **Comment required:** file order is cosmetic/canonical;
   Caddy sorts directives itself (§1.1). Without that comment this test lies about its own purpose.
3. **`TestGenerator_DisableCompressionOmitsEncode`** — `DisableCompression: true` ⇒
   `assert.NotContains(body, "encode")` **and** `assert.Contains(body, "reverse_proxy decloud-streamy:8080")`.
   The second assertion is the point: proves we omitted *only* the `encode` line and didn't break the
   block. A test that only asserts absence would pass on an empty file.
4. **`TestGenerator_MixedCompressionSettings`** — service `alpha` (default) + service `zeta`
   (opted out), **one hostname each**. Assert `strings.Count(body, "encode zstd gzip") == 1`, and
   that the single occurrence sits **between** `alpha.example.com {` and `zeta.example.com {`.
   This is the flag-carried-to-the-wrong-input test and it's the most valuable one in the set —
   `normalize`'s `sort.Slice` (`:82`) reorders inputs after the field is copied, which is precisely
   how a per-service flag gets smeared onto the wrong service.
5. **`TestGenerator_MultiHostnameServiceEncodesEveryBlock`** — one service, two hostnames, default ⇒
   `strings.Count(body, "encode zstd gzip") == 2`. Catches the inner/outer loop bug from §5.2(c).
   Pair it: same service with `DisableCompression: true` ⇒ count `0`.
6. **`TestGenerator_EmptyInputHasNoEncode`** — extend the existing
   `TestGenerator_EmptyInputProducesHeaderAndGlobalBlock` (`:102-108`) with
   `assert.NotContains(body, "encode")`. No new test function; it belongs with the other empty-case
   assertions.
7. *(covered by 5)*
8. **`TestGenerator_EncodeIsNotInGlobalOptionsBlock`** — guards the §1.2 mistake and the "add a
   `match` block" temptation. Don's phrasing ("must appear after `protocols h1 h2`") is weaker than
   it needs to be — ordering alone would pass if `encode` were *also* in the global block. Assert on
   the global block itself:

   ```go
   globalEnd := strings.Index(body, "\n}\n")            // end of the global options block
   require.Greater(t, globalEnd, 0)
   assert.NotContains(t, body[:globalEnd], "encode",
       "encode is a site-level directive; in the global options block it fails caddy validate")
   ```

   **Kent: use this form as written. My rev-1 warning about it was wrong and is struck.** I claimed
   the first `"\n}\n"` was the inner `servers` close and sent you chasing an off-by-one that does not
   exist. Linus and Don each re-ran it; so did I, against a realistic emitted body:

   ```
   body.find('\n}\n')  -> 135
   body[:135]          -> '# Caddyfile generated ...\n\n{\n    servers {\n        protocols h1 h2\n    }'
   'encode' in body[:135] -> False
   ```

   The inner close is `"\n    }\n"` — newline, **four spaces**, brace — which cannot match `"\n}\n"`.
   The first `"\n}\n"` **is** the outer close and `body[:globalEnd]` is exactly the global block. No
   fallback needed; ignore the "slice to `example.com {`" advice from rev 1. Straightforward form,
   correct assertion.

**Correction to Don's §5.2 test 4:** as written ("exactly one `encode` in the file") it is only true
for **single-hostname** services. `strings.Count == 1` is correct for test 4 *because I specified one
hostname each* — Kent, if you give either service a second hostname, that assertion is wrong and the
count is 2. This is the off-by-one that hides behind a plausible-looking test.

### 6.3 `internal/registry/store_test.go` — TOML round-trip

Model: `TestStore_RoundTripsLastDeployedAt` (`:133-145`) — the exact precedent, same shape.

9. **`TestStore_RoundTripsDisableCompression`** — `svc.Config.DisableCompression = true` → `Save` →
   `Load` ⇒ `assert.True(loaded.Config.DisableCompression)`. Proves the TOML tag is right and, more
   importantly, that the **strict decoder accepts the new key** (§2) — a typo'd tag would round-trip
   `false` silently through `Save`, or blow up on `Load` with `ErrUnknownField`.
10. **`TestStore_LoadDefaultsDisableCompressionToFalse`** — write a config TOML with **no**
    `disable_compression` key (the existing `validConfigTOML` fixture is exactly this) → `Load` ⇒
    `assert.False(loaded.Config.DisableCompression)`. **This is the backward-compatibility contract
    from §2 and Don's §4.4 table, and it's the one that protects every service TOML already on disk.**
    It looks trivial. It is not: it's the assertion that fails the day someone "fixes" the polarity to
    `enable_compression`.

### 6.4 `internal/cli/deploy_service_test.go` — flag → Request

Model: `TestDeployService_MountFlagAcceptsValidMounts` (`:81-105`) — `installMockDeployer` +
`DoAndReturn` capturing `deploy.Request`.

11. **`TestDeployService_NoCompressionFlagSetsRequest`** — run with `--no-compression` ⇒
    `assert.True(got.DisableCompression)`.
12. **Default assertion** — add `assert.False(t, got.DisableCompression, "compression is on by
    default")` to the **existing** `TestDeployService_BuildsExpectedRequest` (`:46-72`), next to the
    other default assertions (`"recreate"`, `"/healthz"`). No new test function; that test exists to
    pin the default `Request` shape and this is a default.

### 6.5 `internal/deploy/service_test.go` — Request → saved ServiceConfig

**Gap worth closing.** Every `Save` expectation in this file uses `gomock.Any()` (`:220,244,263,408,
430,...`) — **nothing asserts the saved `ServiceConfig` contents**. So `Request.DisableCompression`
could be dropped on the floor at `:317-343` and tests 11 + 1 would both still pass. That's the
seam between "flag reaches Request" and "generator reads config," and it is currently untested.

13. **`TestDeploy_PersistsDisableCompression`** — one happy-path deploy with
    `Request{DisableCompression: true}`, `Save` expectation swapped to a `DoAndReturn` that captures
    the `*registry.Service` ⇒ `assert.True(saved.Config.DisableCompression)`.

Kent: **one** such test, on the existing happy-path harness. Do not retrofit capture into all ten
`Save` expectations — that's churn for no signal.

14. **`TestDeploy_WarnsWhenCompressionReEnabledOnRedeploy`** — the Option C guard (§5.3.1).

    **Setup:** `Store.Load` returns a `prev` with `Config.DisableCompression = true`; the `Request`
    omits the flag (`false`). Deploy happy-path. **Assert the warning is emitted.**

    **Capture:** `logger` is `slog.With(...)` off the **default** logger (`service.go:157`), so this
    test must swap `slog.SetDefault` to a handler writing into a `bytes.Buffer` and restore it via
    `t.Cleanup`. `internal/logging/logging.go:44` shows the handler construction to mirror; a
    `slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})` is enough.

    **Kent — two traps here, both mine to flag rather than yours to discover:**
    - **`slog.SetDefault` is process-global.** No `t.Parallel()` in this test, and restore in
      `t.Cleanup` or you'll silently poison every test that runs after it in the package.
    - **Assert on the two semantic tokens, not the prose:** `assert.Contains(buf.String(),
      "--no-compression")`. That's the operator's fix string and it participates in the
      multi-surface contract (§5.4), so it's a semantic-token assertion, **not** a change-detector —
      the carve-out in `_ai/cli-flag-surface-coherence.md` covers exactly this. Do **not** assert the
      full sentence; Rob is allowed to polish the wording around those tokens.

    **The negative case is the one that actually matters** — table this with two rows, because a
    warning that fires when it shouldn't is worse than one that never fires:
    - `prev.DisableCompression = true`, `req` **has** `--no-compression` ⇒ **no warning** (nothing
      changed).
    - **no `prev` at all** (`hasPrev == false`, first deploy) ⇒ **no warning, no panic**. This is the
      nil-`prev` dereference guard from §5.3.1 and it is the single most likely way Rob breaks this.

### 6.6 Explicitly NOT tested

- Help text (§5.4 — change-detector, banned).
- That Caddy actually compresses anything. We generate text; we don't run Caddy. Integration is the
  operator's Linux host.
- `caddy validate` on the dev box. Impossible here (no Docker). See §3 for the exact wording.

---

## 7. Order of work (TDD)

1. **Field stubs first** — `types.go`, `GeneratorInput`, `Request` (§5.1, 5.2a, 5.3). Compiles,
   changes no behavior. Kent needs these to reference or the tests won't build.
2. **Kent** writes tests 1-**14**. They compile and **fail** (generator emits nothing; deploy drops
   the field; no warning). Commit.
3. **Rob** implements §5.2(b)(c), §5.3, **§5.3.1 (the Option C warn)**, §5.4. Tests go green. Commit.
4. **Raymond** — §8. Commit.
5. **Kevlin + Linus** review in parallel.

## 8. Docs surface (Raymond)

Grepped. These files mention the Caddy config or the deploy flag table and **will** need touching:

- **`_docs/usage.md`** — the real work:
  - **`:61-72` flag table** — new `--no-compression` row after `--mount` (`:71`). Type `bool`,
    Default `false`, Required `no`. Notes must carry: compression is **on by default**
    (`encode zstd gzip`); set this for **streaming** backends; **the flag is not sticky — omitting it
    on a later redeploy turns compression back on, and the deploy warns when it does** (§4); link
    caddy#6293. **Reset caveat goes here AND at `:198` — both, not one** (Linus §6 Issue A).
  - **`:150` — the hand-edit trap. Required** (Linus §6 Issue B). The surrounding text invites
    hand-editing ("Edit the TOML by hand at your own risk"). `disable_compression` is a new legal
    key, and it must be introduced **together with its trap in the same breath**: a hand-set
    `disable_compression = true` **survives `decloud caddy reload`** (regenerates from the registry
    file) but is **silently wiped by the next `decloud deploy service`** (fresh `ServiceConfig` from
    `Request`, §4.1). `--no-compression` is the durable way. Documenting the key without the caveat
    manufactures a support thread.
  - **`:167`** — the numbered "what deploy does" list, step 7, describes Caddyfile regeneration.
    Mention that generated site blocks carry `encode zstd gzip` unless opted out.
  - **`:198` `decloud caddy reload`** — **the sharp edge.** This command regenerates from the
    registry, so it flips compression **on for every existing service** the first time it runs after
    an upgrade. That is intended (§4.4 polarity) but it *is* a live behavior change to running
    services. Say it plainly, next to the existing "discards manual edits" warning. An SSE operator
    who was fine yesterday needs to find `--no-compression` here.
  - **Exit codes (`:176-182`)** — **no change.** No new failure mode; the warning is a warning, not
    an error, and the deploy still exits 0. Raymond: resist the urge.
- **`_docs/install.md:56`** — describes the generated Caddyfile (`servers { protocols h1 h2 }`) and
  the h3 decision. Optional one-liner that site blocks now carry `encode zstd gzip` by default.
  Judgement call; don't force it.
- **`_ai/caddyfile-generator-facts.md`** — **required.** New section, mirroring the existing
  "Protocol selection is global-options-only" section, which is now half of a matched pair:
  > **`encode` is site-level-only — the mirror image of `protocols`.** No global-options `encode`
  > exists; it must be emitted into every site block. And **`match` cannot protect SSE** — Caddy
  > installs the encoding responseWriter on the request's `Accept-Encoding` alone
  > (`encode.go` `ServeHTTP`), before Content-Type is knowable, so the pre-body `Flush()` is
  > swallowed regardless of `match`. Only omitting `encode` works. caddy#6293.

  This is the single highest-value doc in the task: it's the fact that will otherwise be
  re-derived, wrongly, by the next person who reaches for `match`.
- **`_ai/apidocs.md:6`** — currently reads *"There is NO user-facing protocol config flag."* That
  sentence is about `protocols` and stays true, but the surrounding Caddy-facts bullet list is now
  incomplete: there **is** a user-facing per-service Caddy knob (`--no-compression`). Add a bullet.
  **Kevlin: this file is a hallucination-magnet — verify every claim against the code.**
- **New: `_ai/decisions/http-compression-on-by-default.md`** — the decision record. Model:
  `_ai/decisions/caddy-runs-in-container.md`. Contents are now **specified** by Don rev 2 §5.4 +
  Linus §7.3; Raymond writes all of it:
  1. Compression on by default, and why.
  2. **BREACH considered and rejected as a reason**, argued **strongest-first**: the knob is useless
     against BREACH because the mitigation is app-side and a reverse proxy cannot fix it. "The
     industry ships it" is **supporting evidence, not the reason**. Record it so it isn't
     re-litigated every six months.
  3. **Do not overstate SameSite** (Linus §3, §7.2). `Lax` **still sends cookies on top-level
     cross-site GET navigations**. Write "raises the bar considerably," **never "guts."** A claim a
     security-literate reader can knock down in one sentence discredits the correct reasoning around
     it.
  4. **Say "streaming", not "SSE"** (Linus §1). The swallow at `encode.go:302` fires **before**
     Content-Type is consulted, so this is a **headers-then-idle** bug. SSE is its common shape, not
     its cause — long-poll and chunked progress streams are in the same blast radius. Anyone who
     thinks `Content-Type: text/event-stream` selects the failure will reach for `match` and ship a
     fix that does nothing.
  5. **`Vary: Accept-Encoding` is handled upstream** (`encode.go:463-464`, `:269-270` for the 304
     case) — checked, safe, **no action**. This is the first question any competent reviewer asks
     about compression behind a cache; the answer must be written down or it gets re-derived, wrong.
  6. **A streaming service is never actually compressed anyway** (`encode.go:337-350` — a first write
     under 512 bytes skips `init()`, which is never retried, so `rw.w == nil` forever). **The knob
     buys zero bytes; it purely fixes header timing.** That is a strange shape for a flag named
     "disable compression" — record it, or someone measures zero savings and deletes the knob as
     dead weight.
  7. **The reusable knob rule** (Linus §2): a knob earns permanent config surface when the default's
     failure is **silent + misattributed + unworkaroundable**. This case is 3-for-3. If the failure
     is loud, kill the knob and wait for a bug report. This is the part that generalizes.
  8. **Why `streaming = true` was rejected as a name** — it would promise semantics we don't
     implement (no `flush_interval`, no `reverse_proxy` changes). A lie in the config file.
     `disable_compression` says what it mechanically does.
  9. **The retirement condition** — *if caddy#6293 is fixed upstream, revisit whether
     `disable_compression` still earns its place.* With the issue link. This is what tells future-us
     what would retire the field.
  10. **Redeploy-reset + the Option C warning** (§4), and **the downgrade hazard** (§2 — new TOML +
      old binary = `ErrUnknownField` → exit 10; no supported downgrade path; `schema_version` stays
      `1` because an optional additive key is not a schema break).

## 9. Acceptance criteria

- `go test ./...` green; `gofmt` clean.
- Existing service TOMLs (no `disable_compression`) load and gain compression. No migration,
  `schema_version` stays `1`.
- `--no-compression` round-trips: flag → `Request` → `ServiceConfig` → TOML → `Generator` → no
  `encode` in that service's blocks (tests 11, 13, 9, 3).
- Generated Caddyfile byte-stable for a given registry.
- **Redeploy without `--no-compression` over a service that had it set warns, and the warning names
  `--no-compression`** (test 14, §5.3.1). First deploy (no `prev`) does **not** warn and does **not**
  panic.
- `mocks/mock_generator.go` **unchanged** (§5.2).
- Production diff ~8 lines outside the CLI boundary (§11.1). None of the five named non-goals
  (`match`, loader validator, regenerated mock, lifecycle plumbing, merge semantics) present.
- Reports say **"byte-asserted; pending operator `caddy validate`"** (§3). Never "validated."
- Commit message calls out that existing services gain compression on the next
  `decloud caddy reload`.

## 10. Technical debt, recorded honestly

- **A knob whose only justification is an upstream bug.** Retirement condition is written into the
  decision record (§8). If #6293 lands, this field should be re-examined, not inherited forever.
  Undocumented knobs are how config surfaces rot.
- **Live behavior change on next reload.** Intended, but real. Docs + commit message (§8, §9).
- **The reset warning is a paper cut we chose over a silent failure** (§4). An operator who
  *intentionally* dropped `--no-compression` gets a warning they don't need, once per deploy. That's
  the correct trade — a false positive costs a log line, a false negative costs a hung stream the
  user blames on their own code — but it is a real cost and it is the kind of thing that accretes.
  If a second field ever wants the same treatment, that's the signal to reconsider declarative-deploy
  ergonomics wholesale rather than bolt on warning #2.
- **Downgrade hazard** (§2). Accepted, documented, not engineered around.
- **The abstraction leaks, and we're choosing where.** Decloud's pitch is "we manage Caddy for you."
  `disable_compression` is Caddy's `encode` leaking through — worse, it leaks an *upstream defect*
  into our config surface. That's the honest cost of not shipping a broken SSE experience. The name
  is at least Decloud-shaped (`disable_compression`, not `caddy_encode`), so if the leak ever seals,
  the field can be dropped without renaming anything user-facing.

## 11. Simplification opportunities (checked)

- **No `match` block.** Doesn't work (§1), and would be a worse copy of a list upstream maintains.
- **No `minimum_length`, no levels, no `prefer` tuning.** Defaults. `512` is right.
- **No loader validation.** A bool has no invalid value (§5.1).
- **No new test helper.** Two mutation lines (§6.1).
- **No mock regeneration.** Interface unchanged (§5.2).
- **No lifecycle plumbing.** Registry-resident field; regeneration paths get it free (§5.3).

### 11.1 The diff-size tripwire — restated for rev 2 (~5 → ~8 lines)

Linus called the tripwire a good review heuristic and Don asked me to **restate it rather than let it
fire on our own approved change**. Restating it deliberately, because a tripwire you silently move is
worse than no tripwire:

| Change | Lines |
| --- | --- |
| `ServiceConfig.DisableCompression` field (`types.go`) | 1 |
| `GeneratorInput.DisableCompression` field | 1 |
| `normalize` carries it | 1 |
| Generator `if !in.DisableCompression { ... }` | 2 |
| `deploy.Request.DisableCompression` field | 1 |
| `ServiceConfig{...}` mapping | 1 |
| **Option C warn (`if` + `logger.Warn`)** | **2** |
| CLI flag field + `BoolVar` + `Request` mapping | 3 |
| **Total** | **~12, of which ~8 outside the CLI boundary** |

**The number moved from ~5 to ~8 for exactly one reason: the Option C warning (§4), ruled by Don
after review.** That is a priced, argued, approved 3 lines — **not** creep. This is the whole point
of writing the tripwire down: it fired, we looked, we found a decision behind it, we moved it *on
purpose* and said so.

**The heuristic for Kevlin and Linus at code review is unchanged in spirit:** ~8 production lines
outside the CLI. If Rob's diff is materially bigger, something has gone wrong — most likely a `match`
block (§1 — doesn't work), a loader validator (§5.1 — a bool has no invalid value), a regenerated
mock (§5.2 — interface unchanged), lifecycle plumbing (§5.3 — free), or merge semantics for the reset
(§4 — Option B, rejected). **Every one of those has a named reason to be absent. If you see one in the
diff, it's a bug, not initiative.**

## 12. Status — PLAN closed, ready for EXECUTION

**No open questions.** Linus APPROVED (`004`); Don ruled Issue A (Option C, `002` rev 2 §7.1).
Everything escalated is resolved:

| Item | Resolution |
| --- | --- |
| Issue A — escape hatch closes silently | **Option C.** Spec: §4, §5.3.1, test 14. My defense overruled; conceded. |
| Issue B — hand-edit trap at `usage.md:150` | Raymond, §8. |
| My inverted test-8 index warning | **Struck** (§6.2). Kent uses the simple form. |
| SameSite overstatement | Decision record, §8 item 3. |
| `Vary` / never-compressed / knob-rule / "streaming" not "SSE" | Decision record, §8 items 4-7. |
| Strict decoder, prod `caddy validate`, test-4 off-by-one, `makeService` | Accepted as specified (§2, §3, §6.2, §6.1). |

**Kent starts at §6** (tests 1-14, after the §7 step-1 field stubs land). **Rob starts at §5**
(§5.1, 5.2, 5.3, 5.3.1, 5.4). **Raymond: §8.** Report discipline: **"byte-asserted; pending operator
`caddy validate`"** — never "validated" (§3).

---

# 13. SPEC→ARTIFACT DELTA (appended 2026-07-17, post-EXECUTION)

> **Appended, not rewritten.** §0-§12 stand as written, including §8's "silently" error that Raymond
> caught and corrected. Plans are history; the decision record is truth. I endorse Linus's
> no-retro-edit ruling and Don's `002` §10.2 reasoning without reservation — a plan that gets quietly
> patched stops being evidence of what anyone actually believed.

## 13.1 Verdict: **FULLY DONE.** Agreed.

Don's `002` §10.1 said NOT DONE pending two doc gaps. Raymond closed both in `92c0b15`. I verified
that independently (§13.3). Nothing in my specified surface remains.

## 13.2 The delta check, run against my own spec

Don's §10.3 is right that nobody audited the spec→artifact delta. So I ran it on **my** spec —
every item I specified, checked against the artifact rather than against the reports.

**Production (§5) — shipped exactly as specified, verified by reading `git diff main...HEAD`:**

| Spec | Artifact |
| --- | --- |
| §5.1 field after `Strategy`, TOML tag, long *why* comment, `ReadOnly` polarity cited | ✅ `types.go` |
| §5.2a/b `GeneratorInput` + `normalize` carry | ✅ |
| §5.2c `if` in the **inner** loop, 4-space, `Fprintln`, `encode zstd gzip`, before `reverse_proxy` | ✅ all five |
| §5.3 `Request` field + mapping | ✅ |
| **§5.3.1 warn: exact condition, `hasPrev` first** | ✅ `service.go:320`, verbatim |
| **§5.3.1 call site: above the `svc :=` literal, not at the `prev` load** | ✅ `:320`, above `:321` |
| §5.3.1 message names `--no-compression` **and** `disable_compression`; no `service` key | ✅ both tokens, no dup attribute |
| §5.4 CLI flag, `BoolVar`, no parse/validate path | ✅ |

**Tests (§6) — all 14 shipped**; 11 new functions + 3 assertions folded into existing tests exactly
where I asked (empty-input, `BuildsExpectedRequest`). Test 8 uses the simple `strings.Index("\n}\n")`
form — my struck warning did **not** mislead Kent. Test 14 shipped with both my negative rows
(`flag_passed_again`, `first_deploy_has_no_previous_config`) **plus a fourth** Kent added after
Linus's mutation (`ordinary_redeploy_never_disabled`) — the middle-term pin. That fourth row is the
one I should have specified and didn't; see §13.4.

**Independently re-verified, not taken from the reports:** suite green **9/9 uncached**
(`go clean -testcache && go test ./...`), `gofmt -l internal/` empty, `go vet` clean, tree clean.

**§11.1 tripwire — held, and I'll be precise about the arithmetic rather than claim a prettier
match than exists.** 8 production statements outside the CLI, which is my table's convention
(statements, not physical lines: `git diff -w --numstat` reports 5/7/12 raw, of which 12 are the
comment block and 3 are closing braces). Under the convention I wrote it in, it's exactly 8. **All
five named non-goals verified absent by grep, not by report:** no `match`/`minimum_length`/brotli in
`generator.go` (0 hits), no loader validator (0), `mocks/` untouched (0), `lifecycle.go` untouched
(0), and `prev.Config.DisableCompression` appears **only** in the warn condition and never in the
`ServiceConfig` literal — the field still resets, Option B stayed rejected, deploy is still
declarative.

## 13.3 The two gaps: Don located the drop one step too late — it was mine and his, at PLAN

Both gaps are real and both are now closed. But the causation in `002` §10.1 — CRIME *"evaporated
between plan and record"* — points at the plan→record step, i.e. Raymond's. **I checked. It didn't
evaporate there.**

- **My §8 specified the record's contents as ten numbered items. CRIME is not one of them.** Nor is
  the zstd/gzip ordering rationale.
- **Don's rev-2 §5.4 specified the record's contents too. It names neither** (verified:
  `git show f9990ab:...002-don-plan.md`, §5.4 — zero hits for either).

So Raymond shipped **precisely what both planners asked for, and both of us asked wrong.** He didn't
drop anything; we never handed it to him. The material existed in Don's §2.1 and §2.6 — it was alive
in the *discussion* and died in the *specs for the record*, one step earlier than §10.3 locates it.

That sharpens the finding rather than softening it, and it makes it worse for me specifically: I
wrote an explicit, numbered, "Raymond writes all of it" contents list. **A numbered list is exactly
the artifact that stops being audited** — it looks like rigor, so reviewers check the items *in* it
against the code and never ask what isn't in it. Linus praised the CRIME dismissal by name in `004`
§3 while approving a §8 that omitted it. **Completeness is not a property you can review by reading;
it's one you can only review by diffing against the source.** That's Don's §10.3 and I'm the case
study for it.

## 13.4 Three things landed against me. All three are correct.

1. **§8 "silently wiped" — false, and Raymond was right to ignore me.** The warning reads
   `prev.Config` straight from the TOML; no provenance tracking exists, so a **hand-set** value warns
   exactly like a flag-set one. Verified. My rev-1 §8 wording was written before Don's Option C
   ruling and I never back-propagated it — the same failure mode Don names for himself in §10.2. His
   shipped line (`usage.md:153`) is better than my instruction: it separates "for most keys that
   replacement is silent" from the `disable_compression` exception. **Raymond checked an instruction
   from two seniors against the code, found it false, and shipped the truth. That is the job.** He
   also found a trap neither Don nor I saw — `disable_compression` is a **top-level** key, so
   appending it to the end of a TOML binds it to `[state]` and the strict loader rejects it with exit
   10 (`usage.md:155`). Nobody specified that. It's the best thing in the docs.
2. **Kevlin's `init()` correction — accepted.** My §1 quoted `init()` as reached "only from `Write`."
   It's `Write` (`:349`) **and** `Close` (`:423`). The conclusion survives (both gate on
   `!wroteHeader`), but I inherited that "only" from Don's §3.3.1 and re-published it as my own
   verification. **I verified the claim I was checking (`match` can't save SSE) and relayed the
   claim I wasn't.** A re-verification that adopts the original's unexamined universals is a partial
   re-verification wearing the costume of a full one.
3. **Test 14's missing middle-term row.** Kent's diagnosis is the keeper — *"I tabled the cases the
   implementation suggested, not the cases the property required"* — and it indicts my spec, which
   is where his table came from. I wrote two negative rows derived from the condition's **shape**
   (`hasPrev` false; flag re-passed). Neither is the case that matters: **an ordinary redeploy of an
   ordinary service** — the most common operation in the product — which a mutant dropping the middle
   term would warn on every time, gutting Option C while keeping my suite green. I enumerated around
   the `&&` terms instead of asking "what must NOT warn?"

## 13.5 One finding of my own: `002` §10.1 GAP 2 misquotes the user

Stated because this task's standard is source-verification, and the assessment that faults everyone
for relaying unverified claims contains one. **The remediation was right; the justification isn't.**

`002` §10.1 GAP 2 asserts: *"The user explicitly asked about **"CPU cost, and gzip vs zstd
ordering"**."* Quoted, so it reads as verbatim. It isn't in `001` — in any section:

- What `001` actually says: *"CPU cost, and `Content-Length` / range-request interactions."*
- `grep -icE "gzip vs zstd|zstd ordering"` over `001` → **0**.
- The user's literal request is one line and names neither: *"discuss if it's safe to enable HTTP
  compression globally in Caddy config or if it's better to have it as a setting per host; implement
  the change."* The considerations list is the task-saver's **Interpretation**, not the user's words.
  (GAP 1's quote, by contrast, **is** verbatim in `001` — *"BREACH/CRIME-style attacks over TLS"* —
  so GAP 1 is clean. The user mentions "gzip/zstd" exactly once, as a gloss on what `encode` *is*,
  never as an ordering question.)

**Nothing false shipped.** Raymond justified the section on repo-scar grounds — the h3/Safari
Amendment — which is *stronger* than "the user asked," self-evidently true from the codebase, and
doesn't depend on the misquote. The section is excellent and I'd fight to keep it; Don labelled GAP 2
"recommended," and it earns its place on his real argument. **No retro-edit** — `002` is history and
this note is the record.

The reason I'm bothering: it's the third instance on this task of the same failure — Don's
`init()`-"only", my "silently", this quote — **a plausible detail asserted from memory adjacent to
correct reasoning, in a document whose surrounding rigor discourages anyone from checking it.** All
three were caught by the person downstream who checked instead of deferring. That's not three people
being sloppy; that's the shape of the thing.

## 13.6 For Ward — the durable lessons

1. **Audit the delta from the spec, not just the artifact** (Don `002` §10.3). Kevlin hunts false
   claims, Linus hunts design defects; **an omission is invisible to both** — it doesn't contradict a
   source, it just isn't there. A false claim announces itself to a source check; a missing one looks
   like nothing.
2. **A numbered contents list is where completeness goes to die.** It reads as rigor, so reviewers
   check its items against the code and never ask what isn't in it. Two seniors and two reviewers
   read a ten-item list past a topic the originating request named and the reviewer had praised by
   name. If a spec enumerates, someone must diff the enumeration against the source request.
3. **Re-verification inherits unexamined universals.** I re-derived Don's §3.4 at source and adopted
   his "only called from `Write`" without enumerating call sites. **"Only" is a claim about absence
   and absence requires a search.** Verify the claim you're checking *and* the quantifiers you're
   carrying with it.
4. **Table what must NOT happen, not what the condition suggests** (Kent). Negative cases derived
   from an `if`'s shape test the `if`; negative cases derived from the property test the product. The
   most common operation is the least interesting from the condition's point of view and the most
   interesting from the operator's.
5. **A doc writer's job is accuracy, not obedience** — and labelling a relayed claim as relayed
   (Raymond `008` §8.4) is what made it auditable. Both of Raymond's saves came from checking
   instructions handed down by seniors instead of executing them.

## 13.7 Status

**FULLY DONE — my agreement, on the record.** Code is right and matches spec line for line; tests
cover the surface including the property my own table missed; the durable record answers every
consideration the originating request named, CRIME and the zstd ordering included. Remaining: Linus's
concurrence, then FINALIZATION (Ward → Andy → squash-merge). Report discipline holds to the end —
**"byte-asserted; pending operator `caddy validate`"**; no one on this task has run Caddy, and the
record says so (`_ai/decisions/http-compression-on-by-default.md` §Verification status).
