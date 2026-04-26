# Linus Review v2: M1 Plan + Tech Plan (round 2)

**Reviewer:** Linus Torvalds (high-level review)
**Reviewing:** `05-plan-v2.md` (Don) and `06-tech-plan-v2.md` (Joel)
**Prior review:** `04-linus-review.md`

---

## VERDICT: APPROVED

All three blockers are fixed. The smaller items I flagged are addressed with substance, not hand-waving. I went looking for new problems the revision introduced and found one borderline thing worth noting (not a blocker) and several places where the revision is actually *better* than I would have written it. Ship the planning artifact. Hand it back to the user. Move to execution.

---

## Verification of the three blockers

### Issue 1 — Secrets architecture: FIXED, correctly

Don's plan §3 mandates the two-file split: `config/services/<name>.toml` mode 0644 (no `Env` field) and `secrets/<name>/env.toml` mode 0600 inside a 0700 directory. Joel's tech plan §4 implements it with the right shape — `ServiceConfig`, `ServiceSecrets`, merged `Service` — and `Env` lives **only** in `ServiceSecrets`, so the strict-mode loader will reject any config TOML that tries to smuggle env back in. Good.

`Store.Load` (§4.4) does enforce permission modes: rejects secrets file != 0600, rejects secrets dir != 0700, returns `ErrPermissionMode` with a specific message naming the offending path and observed mode. Critically, it does **not** silently fix permissions. That's the right call — silently fixing hides the audit signal.

The save/delete ordering analysis (§4.5–§4.7) is the part I most wanted to see done well, and Joel did it well:
- **Save: config first, then secrets.** Crash window leaves "config without secrets" — the loader returns `ErrSecretsMissing`, the operator re-runs, env is re-captured from source, system converges. Recoverable.
- **Delete: secrets first, then config.** Same crash window leaves "config without secrets" rather than "orphan secrets file with no registration pointing at it." Same recoverable failure mode.
- The justification in §4.6 specifically explains why the alternative (secrets-first on create) would produce an orphan secret with no registration — worse outcome for the same crash window. That reasoning is correct.

The unit test `TestStore_LoadConfigWithoutSecretsReturnsErrSecretsMissing` (§12.1) explicitly tests the recoverable state. Step-7 mid-write failure handling in §6.6 also correctly *deletes the just-written config file* on a failed initial create to avoid leaving deploy-failure orphans, while preserving "config without secrets" as a legitimate signal for *successful* prior writes that lost their secrets file. That's a subtle distinction and Joel got it right.

**Verdict on Issue 1: airtight.**

### Issue 2 — macOS `env -0` portability: FIXED, with verification

Joel didn't just claim it works on bash 3.2 — he actually ran it on his Mac (§3.1) and pasted the output, including the multi-line PEM, the unicode value, and the unexported `PLAIN_NO_EXPORT`. He also confirmed the failure mode of BSD `env -0` (silently treats `-0` as a `name=value` pair and does nothing — the classic "looks fine until you parse it" bug). This is the kind of due diligence I asked for; it's exactly right.

The mechanism uses `compgen -e` (bash builtin, bash 2+) and `${!name}` (bash indirect expansion, bash 2+) — both confirmed present on macOS bash 3.2. The hermetic wrapper is preserved verbatim: `env -i PATH=... HOME=/tmp /bin/bash --noprofile --norc -c '...'`. The baseline-diff strategy is unchanged and correct.

The test plan (§3.5, §12.1) drops the build-tag skip and mandates the envcap tests run on **both** macOS and Linux in CI. That's the structural fix that prevents regression — somebody can't merge a "clever" change that re-introduces a GNU dep without the macOS CI catching it.

One small craftsmanship win: the embedded `captureScript` uses the same script for baseline and full-capture runs, gated by `if [ -n "$1" ]`. Single source of truth for "what gets emitted." Less surface for divergence between baseline and capture.

**Verdict on Issue 2: fixed, and fixed *well*.**

### Issue 3 — `schema_version` contradiction: RESOLVED, internally consistent

Don §5: M1 writes 1, M3 writes 1, version bumps only when a field's *meaning* changes in a way that breaks old loaders. Joel §5: identical wording, same rule, same forward-compat backstop (`DisallowUnknownFields()` for unknown new fields, explicit version check for semantic breaks). I cross-read both files and found no contradiction.

The future bump policy is documented in both files (Don §5.4, Joel §5.4): if Kent or Rob discovers during M1 that the schema is wrong and needs v2, **stop and re-plan, do not silently introduce migration code mid-milestone**. That's the right escalation rule.

The strict-mode catch (`DisallowUnknownFields` + `ErrUnknownField` mapped to `ExitConfigError`) gives an old binary a clear "unknown field <name>" message when it encounters a future-schema file. Combined with the explicit `schema_version` check, this is belt-and-braces in the way I asked for.

**Verdict on Issue 3: resolved, both plans agree.**

---

## Smaller items — verified

For each thing I flagged as "fix if cheap, document if not," check whether they actually did it:

- **Cache sentinel dropped** — Yes. Don §6.2 explicitly removes `cache/docker-network-created` from the disk layout. Joel §13.6 documents the runtime call (`docker network inspect ... || docker network create ...`) and explains the failure-mode reasoning. Done.
- **Viper deferred to M2** — Yes. Don §8 commits to plain Cobra + `os.Getenv` for M1. Joel §9.1 implements `config.RootFromEnv()` as three lines, no Viper import. §9.2 wires it via Cobra `StringVar`. Done.
- **Loader rejects non-empty `mounts`** — Yes. Joel §4.8 has the explicit code with `ErrMountsNotSupported`, mapped to `ExitConfigError` (same exit code as the CLI's `--mount` rejection in §8). Closes the hand-edit loophole. Done.
- **Stub Caddyfile defined** — Yes. Joel §7.2 specifies the exact bytes: a `:80 { respond "decloud: no services registered yet" 404 }` stanza. Not just "an empty file" — an actually-valid Caddyfile that gives the operator a clear "system alive but unconfigured" signal on first `curl`. Better than what I asked for. Done.
- **M3 split into M3a/M3b** — Yes. Don §9 explicitly carves the milestone in two with concrete deliverables for each half. Joel §11 confirms M1 abstractions stay shaped right for both halves. Done.
- **M1→M4 container rename as explicit M4 deliverable** — Yes. Don §9 elevates it from "flag in code" to a tracked deliverable: *"Explicit M4 deliverable: one-time recreation of all M1-era containers under the new naming convention."* Joel §6.6 closing note repeats it. Done.
- **`go.mod` (Go 1.22), LICENSE, CI workflow, `_docs/`/`_ai/` targets, `slog` logging** — Yes. Don §10 lists all of them with owners. Joel §10 has a full deliverables table and §9.3 implements the `slog` initializer (JSON to stderr + `/opt/declouding/logs/decloud.log`, with a `DECLOUD_LOG_TO_STDERR_ONLY=1` test escape hatch — nice touch). Done.

Every smaller item from my prior review is addressed with substance.

---

## New problems introduced by the revision

I deliberately went hunting for over-corrections, new inconsistencies, and bash cleverness that won't survive a real-world `env.sh`. Here's what I found:

### Borderline (NOT a blocker): `compgen -e` enumerates *exported* variables only

Joel uses `set -a` to auto-export every assignment in the sourced script (so `FOO=bar` without `export` becomes `export FOO=bar` for the duration of that subshell). He then uses `compgen -e` to enumerate exported variables. This is correct **as long as the script doesn't `set +a` partway through.**

If an operator writes:
```bash
set +a
SECRET=hunter2  # NOT exported, NOT auto-exported
set -a
PUBLIC=hello
```
…then `SECRET` is silently dropped from the capture. This is consistent with bash's own semantics ("if you didn't export it, it's not in the environment"), so the operator arguably got what they asked for. But it's a sharp edge.

**Options:**
- **Option A (accept):** Document the `set -a` / `set +a` interaction in `_docs/cli/decloud-deploy-service.md`. The operator's mental model is "env.sh is sourced; whatever it `export`s ends up in the container," and that's exactly what we deliver.
- **Option B (paranoid):** Replace `compgen -e` with `compgen -v` (all variables, exported or not) and emit them all. Pros: catches the `set +a` case. Cons: enumerates a much larger set including bash internals like `BASHOPTS`, `BASH_ALIASES` (associative arrays!), `FUNCNAME`, etc. The `${!name}` indirect expansion will fail on associative array names. We'd need additional filtering.

**My take:** Option A. Joel's mechanism matches the operator's mental model. Documenting the `set +a` edge case in operator docs is sufficient. Option B opens more cans of worms than it closes.

**Not a blocker.** Just flagging it so future-Don knows the ground rules if a user complains.

### Borderline (NOT a blocker): `${!name}` on bash 3.2 with arrays

If an operator's `env.sh` declares an array (`declare -a FOO=(a b c)` or `declare -A BAR=([k]=v)`), `compgen -e` may or may not include it depending on whether it was exported, and `${!FOO}` on a non-scalar gives `${FOO[0]}` on bash 3.2 (just the first element). The captured value would be wrong.

In practice: arrays in env.sh files are extraordinarily rare — env vars passed to `docker run -e` are scalars by definition, so even if an operator declared an array, it could not survive the trip into the container env. The capture would just silently drop the array's tail elements; the operator would notice immediately on first deploy.

**My take:** Not worth defending against. Document in operator docs ("env.sh should set scalar variables only; arrays are not portable to container env"). Not a blocker.

### Borderline (NOT a blocker): readonly variables

If `env.sh` does `readonly FOO=bar` (or worse, the operator's hardcoded `seedPATH`/`seedHOME` ends up `readonly` in some bash version), a subsequent assignment in the script fails with "FOO: readonly variable" and `set -e` would cause `bash` to exit non-zero. Without `set -e`, bash continues but the assignment is silently ignored.

Joel's `Capture` propagates non-zero exit from bash as `envcap: sourcing %s failed: %w` with stderr included. So `set -e + readonly conflict` is detected; silent-readonly-conflict is not, but again the operator notices on first deploy when their var doesn't show up.

**My take:** No action. Documented behavior. Not a blocker.

### Inconsistency check between plan and tech plan

I read both files end-to-end looking for places where Don says one thing and Joel says another:
- Schema version: both say 1, both cite the same forward-compat backstop. Match.
- Save/delete ordering: both say config-first-on-create, secrets-first-on-delete. Match.
- Secrets file/dir modes: both say 0600/0700. Match.
- Mount loader rejection: both say loader rejects non-empty Mounts with same error class as CLI. Match.
- Caddy stub: Don says "minimal valid file" with the `:80 { respond 404 }` example; Joel commits to that exact stanza. Match (Joel slightly more specific, which is correct division of labor).
- Viper deferral: both say defer. Match.
- M3 subdivision: both acknowledge M3a/M3b. Match.
- Container naming for M1: both say `decloud-<name>` with M4 owning the rename. Match.

**No new contradictions introduced by the revision.**

---

## Things the revision does notably *well* (rare praise)

- **Joel's §3.1 verification command.** Showing the actual macOS bash 3.2 output, including the BSD `env -0` failure-mode confirmation, is exactly the level of rigor I want to see when someone claims portability. This is "I verified" not "I assume." Future maintainers can re-run that one shell command to spot-check on their own boxes.
- **The recoverable-state contract in Joel §4.5–§4.7.** "Config without secrets" is now a named, documented, tested state with a defined operator recovery path. That's the right level of rigor for a system that will absolutely have crashes mid-deploy at some point in its life.
- **Joel §6.6's distinction between "successful initial create that lost its secrets" (preserved as a recoverable signal) and "failed deploy attempt that wrote partial files" (cleaned up to avoid orphan)** is subtle and correct. I would not have caught this distinction on first pass.
- **Don §10 enumerating the operational deliverables with explicit owners.** Owner column means each one has a name attached; nobody can claim "I didn't know that was mine." Right structure.
- **The `DECLOUD_LOG_TO_STDERR_ONLY=1` test escape hatch in §9.3.** Tiny detail; means the test suite doesn't write to `/opt/declouding/logs/decloud.log` on the maintainer's machine. The kind of paper cut that catches you on day one of running the test suite if it isn't anticipated.

---

## Summary

Three blockers, three fixes, all substantive. Smaller items addressed with substance. No new contradictions introduced. The bash cleverness around `env.sh` capture is portable, verified on the maintainer's actual hardware, and the residual edge cases (`set +a`, arrays, readonly) are either operator-noticeable on first deploy or already detected by bash's own exit semantics.

**APPROVED.** The planning task is complete. Hand the plan artifact back to the user. When execution starts, Kent writes tests against §12, Rob implements against §2/§3/§4/§6/§7/§9, Raymond owns the §10 doc deliverables. If anyone gets stuck, Knuth gets the call.

End of review.
