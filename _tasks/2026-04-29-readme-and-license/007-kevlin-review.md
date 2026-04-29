# Kevlin low-level review — README rewrite + MIT LICENSE

**Reviewer:** Kevlin Henney (low-level reviewer)
**Branch:** `task/readme-and-license`
**Reviewed commits:** `ef59c48` (Rob), `095fe4b` (Raymond)
**Verdict:** **APPROVED.** Two cosmetic nits, neither blocking. Ship it.

---

## Scope of this review

Docs-heavy task. No production code changed; no test surface. My checklist
collapses to: are the words in the new files true, and do they communicate
clearly? I verified every load-bearing factual claim against the live code,
not against Rob or Raymond's summaries.

The Kevlin Way for docs: every claim a reader might act on must be true; every
sentence must earn its place; broken links and stale cross-references are the
docs equivalent of dead code.

---

## 1. Hallucination check on `README.md`

I read the README end-to-end and verified every load-bearing claim against the
source tree. Findings below.

### 1.1 Cobra subcommand surface — VERIFIED

README §"What you get today" enumerates: `decloud deploy service`, `start`,
`stop`, `restart`, `status`, `logs`, `unregister`, and `caddy {up,down,reload}`.

Verified against `internal/cli/root.go:29-44`. Match exact: lines 30-31
register `deploy → service`, lines 33-38 register the six lifecycle commands,
lines 40-44 register the three `caddy` subcommands. No fabricated subcommands.

### 1.2 Flag names on `deploy service` — VERIFIED

README mentions `--name`, `--host`, `--port`, `--mount`. Verified at
`internal/cli/deploy_service.go:57-62`:

- `--name` — line 57 (`StringVar`, marked required line 67).
- `--host` — line 58 (`StringSliceVar`, repeatable).
- `--port` — line 59 (`IntVar`, default 0, manual zero-check at line 79).
- `--mount` — line 61 (`StringArrayVar`, repeatable).

All four flags exist. Help text in code matches the README's prose
description. The `[a-z][a-z0-9-]{0,38}` validation rule cited in `usage.md` is
mentioned in the help string (line 57) — README wisely doesn't repeat it.

### 1.3 `decloud-<service>` Docker DNS pattern — VERIFIED

README §Architecture: "Caddy reaches each service container by its Docker DNS
name (`decloud-<service>`)".

Verified at:
- `internal/ids/ids.go:24` — `return "decloud-" + serviceName` (the canonical
  builder).
- `internal/caddy/generator.go:67` — Caddyfile generator emits
  `decloud-<svc.Config.Name>` as the upstream.
- `internal/dockerdrv/cli_driver.go:64` — label `decloud.service=` strips the
  `decloud-` prefix when reverse-mapping.

The pattern is real, single-sourced, and honored throughout. Match.

### 1.4 `decloud-caddy` container name — VERIFIED

README mentions `decloud-caddy` four times. Verified at
`internal/caddy/manager.go:19`: `ContainerName = "decloud-caddy"`. Single
constant, used consistently. Match.

### 1.5 State paths under `/opt/decloud/` — VERIFIED

README §Architecture line 68 lists three paths:
- `config/services/<name>.toml`
- `secrets/<name>/env.toml`
- `config/caddy/Caddyfile`

Verified against `internal/config/paths.go:24-41`:
- Line 31: `ServicesDir = filepath.Join(root, "config", "services")` ✓
- Line 35: `SecretsDir = filepath.Join(root, "secrets")` ✓ (per-service file
  is `<name>/env.toml` per `internal/registry/types.go:29`)
- Line 34: `CaddyfilePath = filepath.Join(root, "config", "caddy", "Caddyfile")` ✓

Match exact.

### 1.6 Schema version — N/A in README

The README does not claim a specific `schema_version`. The user's prompt
mentioned verifying `m2-mount, schema_version=1`, but that detail is in
`_docs/usage.md:150`, not the README. The README correctly delegates schema
detail to `usage.md`. No claim to verify.

For belt-and-braces: `internal/registry/types.go:5` has
`CurrentSchemaVersion = 1`. The M2 mount work did not bump this (M2's
`Mount` struct lives under `RunSpec` which was already in the schema-1 shape).
Match.

### 1.7 Module path / `go install` URL — VERIFIED

README:34: `go install github.com/alexander-fenster/decloud/cmd/decloud@latest`

Verified against `go.mod:1`: `module github.com/alexander-fenster/decloud`.
The `cmd/decloud` package exists (`cmd/decloud/main.go`). URL resolves cleanly.

The `go install ...@latest` line will only work if the GitHub repo at
`alexander-fenster/decloud` is publicly accessible. Joel flagged this in tech
plan §6.6 and Linus left it as the maintainer's responsibility. Not a Kevlin
concern; if the repo is private the README is misleading but the LICENSE and
docs still work for the maintainer.

### 1.8 Exit codes — N/A in README

The README points readers at `_docs/usage.md` §3 for exit codes (line 96)
rather than enumerating them. Smart. The 11-code surface (`0, 2, 10, 20, 30,
40, 50, 60, 70, 130`) lives at `internal/cli/exit_codes.go:13-23`; `usage.md`
§3 matches. README has no claim to verify.

### 1.9 Strategy: `recreate` — VERIFIED

README:62: "Strategy is `recreate` — brief downtime as the old container is
stopped before the new one starts. Blue/green is M4."

Verified at `internal/cli/deploy_service.go:65,76-78`: default `recreate`,
loader rejects anything else with `ErrInvalidStrategy` (exit 10). Match.

The "stopped before the new one starts" framing is user-visible-accurate,
matches `_docs/usage.md:55` verbatim. (Internally the build happens before the
stop, but that detail is interesting to engineers, not to README readers, and
Joel flagged this distinction in tech plan §0 adjustment 1. Correct call.)

### 1.10 Ports `80/443/443-UDP` claim — VERIFIED

README:13: "ports 80/443/443-UDP on the host."
README:76: "the public ports `80/tcp`, `443/tcp`, and `443/udp` open on the
host firewall".

Verified against `_docs/install.md:40,55`: Caddy publishes `80/tcp`, `443/tcp`,
and `443/udp` (HTTP/3 over QUIC). Match.

**Cosmetic nit (1.10a):** the line-13 form "443/443-UDP" is mildly
inconsistent with line-76's "443/tcp, 443/udp" (lowercase, slash-form). The
line-13 abbreviation reads as "443 plus 443-UDP" which is a true statement but
the punctuation is awkward. **Optional fix:** change line 13 to "ports
80/tcp, 443/tcp, and 443/udp on the host." This matches line 76 and
`install.md` style. Not blocking — both forms are unambiguous to a reader.

### 1.11 Docker network name `decloud` — VERIFIED

README §Architecture: "the shared `decloud` Docker network".

Verified at `internal/caddy/manager.go` (consistent `NetworkName` usage).
Match.

### 1.12 The `--mount` two-form summary — VERIFIED

README:60: "bind paths (`/host:/container[:ro]`) or named volumes
(`name:/container[:ro]`)."

Cross-referenced with `_docs/usage.md:71` flag table cell and the parser at
`internal/registry/parse_mount.go` (via `ParseMountString`, called from
`internal/cli/deploy_service.go:178`). The two forms match the `IsNamed()`
disambiguation in `internal/registry/types.go:60-67`. Match.

### 1.13 `env.sh` capture model — VERIFIED

README:61: "`env.sh` capture — sourced inside a hermetic bash, exported
variables become container environment, never baked into the image."

Verified against `internal/envcap/capture.go` and the file-resolver at
`internal/cli/deploy_service.go:117-132`. The "auto-discover `<source-dir>/env.sh`"
behavior is in `resolveEnvFile()`. Match.

### 1.14 `tools.go` description — VERIFIED

README:131: "tools.go — pinned tool dependencies (gomock)."

Verified by reading the file: `//go:build tools` constraint with a single
underscore-import of `go.uber.org/mock/mockgen`. The "(gomock)" parenthetical
is correct shorthand for `go.uber.org/mock`. Match.

### 1.15 `internal/` package list — VERIFIED

README:126: "internal/ — private Go packages: cli, deploy, registry, caddy,
dockerdrv, envcap, ..."

Verified by `ls /Users/fenster/dev/decloud/internal/`: ten packages exist
(`caddy, cli, config, deploy, dockerdrv, envcap, ids, integration, logging,
registry`). The README names six and trails `...`. Honest compression. Match.

### 1.16 Integration test incantation — VERIFIED

README:145: `DECLOUD_INTEGRATION=1 go test -tags integration ./internal/integration/...`

Joel claimed this is verbatim from `internal/integration/doc.go`. I confirmed
the env var name and build tag both apply (the env-var gate is enforced at
runtime; the build tag at compile time). Match.

### 1.17 Roadmap milestone labels (M1-M7) — VERIFIED

README:100-106 milestone descriptions match `_ai/decisions/m1-scope.md` last
paragraph and the resequence task in
`_tasks/2026-04-28-milestone-resequence/`. M2 and M3 are correctly swapped per
the resequence (M2 = mounts, M3 = host bootstrap). M3b → M7 deferral is also
honored (M7 = client binary + secret files + polish). Match.

### 1.18 No-feature-leak grep — PASS

Ran the suspicious-strings test: every match for `blue/green`, `restic`,
`backup`, `bootstrap`, `client binary`, `\bjob\b`, `\bgc\b` falls into one of
four legitimate buckets:

1. Project Status "Not yet shipped" list (lines 18-22)
2. Architecture explicitly tied to roadmap pointer (line 70: "client binary
   is on the roadmap (M7)")
3. Install disclaimer about M3 absence (line 74: "no bootstrap script yet")
4. Roadmap section (lines 102-106), Non-goals (line 120)

Line 68 mentions "One backup path covers everything that matters" —
architectural framing about state location, not a feature claim. Roadmap M6
explicitly tags backups as PLANNED. Reader gets the right signal. Rob already
flagged this in implementation report §3.1 with a fallback ready; I agree
with him: no fallback needed. The sentence is honest.

**Hallucination summary: ZERO.** Every factual claim in the README is backed
by source code or by `_docs/` content that is itself backed by source code.

---

## 2. LICENSE byte-check

Verified against the canonical GitHub-template / choosealicense.com MIT form.

### 2.1 Substitutions

- `[year]` → `2026` — correct (per `currentDate` 2026-04-29 in this harness).
- `[fullname]` → `Alexander Fenster` — correct (per `git log` author and the
  `go.mod` module path `github.com/alexander-fenster/decloud`).

No other substitutions. The body retains "the Software" / "SOFTWARE" (not
"Decloud" / "DECLOUD"). Match.

### 2.2 Byte-level checks

- **Line count:** 21. Correct (canonical MIT template is 21 lines including
  the `MIT License` heading and trailing `SOFTWARE.`).
- **Heading:** Line 1 is `MIT License`. Required for GitHub's `licensee`
  detector to match the MIT regex.
- **Copyright form:** Line 3 is `Copyright (c) 2026 Alexander Fenster`. The
  `(c)` notation is the GitHub-template form (not the bare-OSI form which
  uses `Copyright [year] [fullname]` without `(c)`). Correct per Linus's
  review §1.3.
- **Blank lines:** lines 2, 4, 11, 14 are empty — matches canonical paragraph
  separators.
- **All-caps disclaimer:** lines 15-21, ends with `SOFTWARE.` and a single
  trailing newline.
- **File mode:** `0644` (verified via `stat -f '%A %N'`).
- **Encoding:** ASCII text (verified via `file LICENSE`).
- **Line endings:** LF only, no CRLF.
- **Trailing newline:** single `\n` after `SOFTWARE.` (verified via
  `tail -c 5 | xxd` → `4152 452e 0a` = "ARE.\n").
- **No BOM:** confirmed.

The exact bytes match the canonical form GitHub's license-detection regex
expects. The "MIT License" badge will display in the GitHub repo header. No
issues.

### 2.3 Why this matters

If any byte drifts (e.g., a misplaced `[year]` placeholder, "Decloud" instead
of "Software" in the body, an extra blank line, CRLF endings), GitHub's
`licensee` detector falls back to "other" and the project loses its MIT badge.
For a project whose entire license story rides on this one file, byte
exactness is load-bearing. Rob and Joel got it right.

---

## 3. Cross-document consistency

Three places where README, `_docs/install.md`, and `_docs/usage.md` overlap.
Reviewed each for drift.

### 3.1 `/opt/decloud/` tree creation

- README:30 (Quick start): brace-expansion `mkdir -p` for the six
  directories.
- `install.md` §4 (lines 100-117): seven explicit `mkdir -p` lines + nine
  explicit `chmod` lines.

The brace expansion in README expands to exactly the six directories
`install.md` creates: `config/services`, `config/jobs`, `config/caddy`,
`secrets`, `state/deploys`, `logs`. The README inlines only the load-bearing
`chmod 0700 /opt/decloud/secrets` and points at `install.md` §4 for the
others. **No drift.**

### 3.2 `--mount` syntax

- README:60 — two-form summary.
- `install.md` — does not document `--mount` (correct: install is for setup,
  not deploy semantics).
- `usage.md` §2 (line 71) — full flag-table row with named-volume regex,
  mode-flag rejection list, and duplicate-target rule.

README compresses `usage.md`'s row faithfully. **No drift.**

### 3.3 Exit codes

- README:96 — pointer to `usage.md` §3.
- `usage.md` §3 (line 173 onward) — full 11-code table.

README correctly delegates. The pointer resolves to the right section heading
("## 3. Exit codes" at `usage.md:171`). **No drift.**

### 3.4 `decloud caddy up` semantics

- README:38 — single-line note ("Bring up the Caddy ingress container").
- `install.md` §3 (lines 26-51) — full procedure with idempotency and named
  volumes.
- `usage.md` §4 (line 196) — runtime semantics for the lifecycle command.

All three are mutually consistent. README points at `install.md` for setup
(line 50 link). **No drift.**

### 3.5 Architecture claims

README §Architecture makes three claims; Raymond verified each in his
report §2.4. I cross-checked his work:

- "Docker runs every workload, including Caddy itself" — `install.md` §3
  paragraph 1 ("Caddy runs as a Decloud-managed container named
  `decloud-caddy`") and `usage.md` §6 paragraph 1 ("The only container that
  publishes ports is `decloud-caddy` itself"). Match.
- "Caddy reaches each service container by its Docker DNS name
  (`decloud-<service>`)" — `usage.md` §6 paragraph 1 ("Caddy reaches each
  upstream by container name (`decloud-<service>`) via Docker's embedded
  DNS"). Match.
- "There is no Decloud daemon and no listening management port" —
  `install.md` §5 paragraph 4 ("There is no `decloud daemon`, no `decloud
  bootstrap`, and no `systemctl enable decloud`"). Match.

**No drift across all three documents.**

### 3.6 Readiness path default

The README's Quick start example does NOT pass `--readiness-path` (per
Linus's §2.2 ruling — default `/healthz` is fine). The Usage block also
omits it. `usage.md` §1 (line 47) DOES include `--readiness-path /healthz`
explicitly. This is intentional — `usage.md` §1 is the explanatory doc, the
README is the minimum-working command. The default is `/healthz` per
`internal/cli/deploy_service.go:63`. **No drift; intentional asymmetry,
documented.**

### 3.7 Strategy default

- README:62 — "Strategy is `recreate`".
- `usage.md` §2 table — `--strategy` default `recreate`.
- Code — `internal/cli/deploy_service.go:65` default `"recreate"`, line
  76-78 rejects anything else.

**No drift.**

---

## 4. Raymond's `_ai/decisions/` edits

I reviewed the diff in commit `095fe4b` against Raymond's report claims.

### 4.1 `no-magic-zero-modes.md` line 25

**Diff:** replaced `(cross-referenced README.md:215 and \`_ai/decisions/m1-scope.md:32\`)`
with `(cross-referenced the pre-rewrite README's CLI-surface section and \`_ai/decisions/m1-scope.md:32\`; the user-facing \`--port\`-required contract now lives in \`_docs/usage.md\` §2)`.

**Assessment:** non-destructive. The decision (reject `--port=0`) is intact.
The cross-reference is correctly past-tensed (the line-215 reference no
longer resolves; new README is 154 lines) and a forward pointer to the new
home (`usage.md` §2) is added. The forward pointer is accurate — `usage.md`
§2 line 65 documents exactly the `--port` required contract.

**Verdict: correct citation-tense fix; the decision rationale is unchanged.**

### 4.2 `secrets-split.md` line 3

**Diff:** "The README's 'Handling secrets' section requires structural
separation..." → "The pre-rewrite README's 'Handling secrets' section (the
design-narrative draft that drove M1 scoping) required structural
separation... The requirement is now load-bearing in the M1 type system
regardless of where it was originally documented."

**Assessment:** non-destructive. The decision (two-file split: config 0644
+ secrets 0600) is intact. The README citation is correctly past-tensed.
The added clause "The requirement is now load-bearing in the M1 type system"
is verifiable: `internal/registry/types.go` defines `ServiceConfig` and
`ServiceSecrets` as separate types (lines 9-26 and 30-34) with file-mode
comments on each. The type system does enforce the split. Raymond's claim
holds.

**Verdict: correct citation-tense fix with an accurate added clause.**

### 4.3 `m1-scope.md` lines 13, 14, 17

**Diff:** three "README" → "the pre-rewrite README" changes. No other text
changed.

**Assessment:** non-destructive. The three M1 cuts (no client binary, no
zero-downtime, no restart-on-crash supervisor) are unchanged in content.
Only the citation tense moves from present to past. The new README does not
contain the cited phrases ("equally usable by a human SSH'd in", blue-green
default, supervisor talk) — Raymond is right that the past-tense form is
the only honest framing.

**Verdict: correct citation-tense fix, no decision drift.**

### 4.4 What Raymond did NOT touch

Raymond explicitly preserved historical `_tasks/` records (which contain
their own `README.md:215` references). This is correct — task records are
immutable historical artifacts; rewriting them would falsify the workflow
trail. I cross-checked: the broken README line-numbers in `_tasks/` do not
get acted upon by any reader, they're just historical citations. Leaving
them alone is the right call.

**Summary of Raymond's edits:** three surface-level citation-tense fixes,
zero decision changes, zero unintended side effects. Exactly what the report
claims.

---

## 5. install.md §8 fix

**Old:**
```
## 8. License

This repository does not yet declare a license. If you intend to redistribute
the binary or use it in a context that requires explicit license grant, ask
the maintainer before doing so.
```

**New:**
```
## 8. License

Decloud is licensed under the MIT License. See the top-level
[`LICENSE`](../LICENSE) file for the full text.
```

### 5.1 Surrounding context

I read lines 208-218 to verify the surgical fix lands cleanly between §7
troubleshooting and §9 next steps. The headings and surrounding paragraphs
are untouched. The `## 8. License` heading is preserved, the fix is exactly
two lines of new text replacing two lines of stale text. **No collateral
damage.**

### 5.2 Relative link

The link `../LICENSE` is correct because `install.md` lives in `_docs/`. From
`/Users/fenster/dev/decloud/_docs/install.md`, `../LICENSE` resolves to
`/Users/fenster/dev/decloud/LICENSE`. Verified: file exists and matches the
expected content.

### 5.3 Tone consistency

The new text matches `install.md`'s declarative, no-marketing tone.
"Licensed under the MIT License" is the standard phrasing every doc reader
recognizes. **No drift in voice.**

**Verdict: clean fix. Lands exactly as the tech plan §4.1 specified.**

---

## 6. Markdown hygiene

### 6.1 Heading hierarchy

```
H1: # Decloud (line 1)
H2: ## Project status (line 7)
H2: ## Quick start (line 24)
H2: ## What you get today (line 52)
H2: ## Architecture in 60 seconds (line 64)
H2: ## Install (line 72)
H2: ## Usage (line 80)
H2: ## Roadmap (line 98)
H2: ## Non-goals (line 108)
H2: ## Repository layout (line 122)
H2: ## Contributing (line 136)
H2: ## License (line 152)
```

One H1, eleven H2s. No H3 or deeper. No markdown linter would flag this.
**Match Joel's tech plan §1 structure exactly.**

### 6.2 Anchor links

Only one intra-document anchor link: `[Roadmap](#roadmap)` at line 9. Verified
that GitHub's anchor-generation rule (lowercase, hyphens for spaces,
non-alphanumerics dropped) produces `#roadmap` from `## Roadmap`. **Resolves
correctly.**

### 6.3 External links

All external links checked:
- `_docs/install.md` (4 occurrences) — file exists.
- `_docs/usage.md` (3 occurrences) — file exists.
- `CLAUDE.md` (1 occurrence) — file exists.
- `LICENSE` (1 occurrence) — file exists.
- `https://github.com/spf13/cobra` — public URL.
- `https://github.com/stretchr/testify` — public URL.
- `https://github.com/uber-go/mock` — public URL.

**All links resolve.** Per Joel's §6.4 cross-link-rot mitigation, zero
anchor-deep links into `_docs/`. If a heading in `install.md` or `usage.md`
gets renamed in a future task, the README does not break.

### 6.4 Code-fence language tags

- Quick start: ` ```sh ` — correct.
- Usage: ` ```sh ` — correct.
- Repository layout: ` ```text ` — correct (not a runnable shell).
- Contributing: ` ```sh ` — correct.

All fences use language tags consistently. GitHub's syntax highlighter will
render each block correctly.

### 6.5 List formatting

All bullet lists use `- ` (hyphen-space) not `* ` or `+ `. Indentation is
two-space within nested contexts (none in this file — the README is flat).
**Consistent.**

### 6.6 Emphasis usage

`**bold**` for milestone labels and section lead-ins. Used consistently. No
mixed `__bold__` / `**bold**` styles.

### 6.7 Trailing whitespace / final newline

I did not find evidence of trailing whitespace. The file ends with a final
newline (line 154 has content, file ends after it).

### 6.8 Cosmetic nit (6.8a): port punctuation inconsistency

README:13 says `ports 80/443/443-UDP on the host`.
README:76 says `the public ports \`80/tcp\`, \`443/tcp\`, and \`443/udp\``.

The line-13 form reads as "443 plus 443-UDP" with awkward hyphenation; the
line-76 form is the canonical lowercase slash-form that `install.md` §3
uses. **Optional fix:** change line 13 to `ports 80/tcp, 443/tcp, and
443/udp on the host.` for consistency with line 76 and `install.md`. Not
blocking — both forms parse unambiguously.

---

## 7. The "One backup path covers everything that matters" sentence

Rob flagged this at implementation §3.1 as a borderline phrasing that could
be read as implying a backup feature exists. I evaluated it again:

- The sentence is in the Architecture section, immediately after the
  enumeration of `/opt/decloud/` paths. The framing is "all state lives in
  one tree, so one backup path covers it." Grammatically and structurally,
  it's an architectural property of the state layout, not a claim that
  Decloud ships a backup tool.
- The Project Status section (line 21) explicitly tags M6 backups as
  PLANNED.
- The Roadmap (line 105) re-tags M6 backups as PLANNED.
- The Non-goals section does not exclude backups (correct — M6 will ship
  them).

A reader who reads the README in order encounters two PLANNED-backup signals
before reaching this sentence. The architectural framing then reads as "good
design enables future backups," not "Decloud has backups today." Rob's
fallback rewording ("All operator state therefore lives in one place") is
also defensible but slightly less informative — it loses the architectural
why.

**Verdict: the sentence stays. Honest framing in the right context. No
change needed.**

---

## 8. The "Architecture in 60 seconds" title

Linus floated `## Architecture` or `## How it works` as alternatives at his
review §3.1. Rob kept Joel's title. I agree with Rob:

- The section is 3 paragraphs / ~7 lines of prose. The "60 seconds" promise
  holds comfortably.
- The convention is recognizable (AWS docs, GitHub READMEs both use this
  form).
- Changing it would invalidate Joel's tech plan reference and add zero
  reader value.

**Verdict: keep the title. Optional nit, not blocking.**

---

## 9. Acceptance criteria cross-check (Joel §8 / Don §9)

I independently re-verified each criterion:

| # | Criterion | Status | Evidence |
|---|---|---|---|
| 1 | README H1 is `# Decloud` | ✅ | Line 1 verified |
| 2 | Project Status before any install instruction | ✅ | Lines 7 vs 24 |
| 3 | Quick-start commands work copy-pasted | ✅ | Module path, flags, subcommand all verified §1.2-1.7 |
| 4 | Every flag in README appears in `internal/cli/*.go` | ✅ | §1.2 above |
| 5 | Milestone labels match `m1-scope.md` and resequence | ✅ | §1.17 above |
| 6 | LICENSE is canonical MIT, year 2026, name correct, mode 0644, LF, single trailing newline | ✅ | §2 above |
| 7 | No README section implies an unshipped feature exists | ✅ | §1.18 grep test passes |
| 8 | `_docs/install.md` §8 references the new LICENSE | ✅ | §5 above |
| 9 | `git diff --stat` shows exactly the expected files | ✅ | Verified via `git log` and diff |
| 10 | README ≤ 200 lines | ✅ | 154 lines |
| 11 | Zero anchor-deep links into `_docs/` | ✅ | §6.3 above |

**All eleven green. No regressions detected.**

---

## 10. Verdict

**APPROVED.**

Two cosmetic nits flagged (1.10a port-punctuation inconsistency, 6.8a
duplicate of same), neither blocking. Rob can apply the line-13 punctuation
fix at his discretion when next touching the file; or Linus can decide
during PLAN re-entry. No code changes, no doc-content changes, no LICENSE
changes required.

The README rewrite is honest, scannable, and tells a coherent story about a
half-built system without apology or marketing. The LICENSE is byte-correct
canonical MIT. Raymond's `_ai/decisions/` edits are surface-only
citation-tense fixes that preserve every decision rationale unchanged.
`install.md` §8 is fixed cleanly.

I would happily maintain this README. That's my bar.

Three files changed in commit `ef59c48`, three files refreshed in commit
`095fe4b`. No code touched. The system worked exactly as the planning chain
designed it to.

— Kevlin
