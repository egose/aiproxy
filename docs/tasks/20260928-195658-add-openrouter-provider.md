# Add `openrouter` provider type (zenmux-pattern)

Created: 20260928-195658

## Objective and scope

Add a dedicated `openrouter` provider type to aiproxy, following the existing
`zenmux` precedent: reuse the OpenAI passthrough adapter (`doOpenAI`:
`model` rewrite + `Authorization: Bearer`, JSON and SSE copy-back), default
base URL `https://openrouter.ai/api/v1`, and OpenRouter attribution headers
(`HTTP-Referer: https://opencode.ai/`, `X-Title: opencode`, matching opencode's
`openai-compatible-profile.ts` convention for openrouter).

Capability policy mirrors `openai`/`zenmux`: default
`chat, responses, embeddings`; supported adds
`images, audio_transcriptions, audio_speech`. No `messages` protocol, no new
wire adapter, no OAuth/device flow.

## Working rules and non-goals

- Stay within this repo (`<repo-root>`) only. Do not touch
  any other checkout.
- Do NOT update `CHANGELOG.md` under any circumstances.
- Do NOT revert unrelated work. The worktree has uncommitted changes from the
  reasoning-effort + `POST /v1/messages` work (37 modified files at plan time,
  e.g. `internal/provider/anthropic.go`, `internal/httpapi/handler.go`,
  `internal/config/capabilities.go`). Build on top; preserve all of it.
- No comments in source files unless the surrounding code dictates otherwise
  (repo convention; rationale lives in `docs/design.md`).
- Sequential execution: one task item per isolated sub-agent session, in order.
  After each completes, update its `Status` + append `Completion evidence`
  before starting the next.
- Verification uses repo commands: `make vet`, `go test ./...`,
  `go test -race`, `make docs-contract`, `gofmt -l`, web-ui `vitest` where
  touched (not expected here).

## Baseline verification

At plan time: `go test ./...` pass, `go vet` clean, `make docs-contract`
matches, web-ui 209/209 (per prior session). Each agent must re-verify its
own scope; final review re-verifies everything.

## Priority definitions

- P0: blocks the feature or breaks existing behavior.
- P1: required for contract/CLI parity.
- P2: docs/final review hardening.

## Waves

1. Baseline + failing regression (OPENROUTER-01)
2. Type + capabilities (OPENROUTER-02)
3. Descriptor + adapter headers (OPENROUTER-03)
4. Config surface + configure CLI (OPENROUTER-04)
5. Ops/docs contract (OPENROUTER-05)
6. Tests + verification (OPENROUTER-06)
7. Final integration review (OPENROUTER-07)

## Detailed executable tasks

### Task OPENROUTER-01: Baseline and failing regression

Status: completed

Completion evidence:

- Baseline: `go build ./...` exit 0; `go test -count=1 ./internal/config/ ./internal/provider/` both ok.
- Added: `internal/config/openrouter_test.go` → `TestOpenRouterIsKnownProviderType` (fails as required: `ProviderTypes()` lacks `"openrouter"`).
- Verified: only the new test fails; rest of both packages pass; `gofmt` clean; no other files modified; `CHANGELOG.md` untouched.

Priority: P0

Suggested agent: test engineer

Dependencies: none

Primary ownership:

- `internal/config/types.go`
- `internal/provider/provider_test.go` (or nearest provider test)

Finding:

`openrouter` is not a known `ProviderType`; configuring
`provider "openrouter" "x"` fails validation today. No regression test pins
this. The worktree also carries uncommitted messages/effort changes that must
be preserved.

References:

- `internal/config/types.go:195-204`
- `internal/config/capabilities.go:72-117`
- `internal/provider/provider.go:169-210`

Implementation requirements:

1. Record the baseline: `go build ./...`, `go test ./internal/config/ ./internal/provider/` pass before changes.
2. Add a failing regression test asserting `openrouter` resolves (type known,
   descriptor present, or validation accepts it — pick one stable seam and
   keep it).
3. Do not change runtime behavior in this task; the test must fail.

Acceptance criteria:

- New regression test fails on the current tree (`openrouter` unknown).
- Baseline suite passes apart from the new failing test.
- No unrelated files reverted; `git status` shows only additive test changes.

### Task OPENROUTER-02: Provider type constant and capability policy

Status: completed

Completion evidence:

- Changed: `internal/config/types.go` (const), `internal/config/capabilities.go` (order + policy, `requiresBaseURL` false), `internal/config/capabilities_test.go` (openrouter row).
- Verified: `TestOpenRouterIsKnownProviderType` PASS; targeted policy tests PASS; `gofmt` clean.
- Known follow-up: `TestDescribeProviderTypes` expects 8 (now 9) — owned by OPENROUTER-04.

Priority: P0

Suggested agent: config engineer

Dependencies: OPENROUTER-01

Primary ownership:

- `internal/config/types.go`
- `internal/config/capabilities.go`
- `internal/config/capabilities_test.go`

Finding:

`providerTypeOrder` and `providerTypePolicies` have 8 entries ending at
`zenmux`; there is no `openrouter` entry, so defaults/supported resolution
returns nil for it.

References:

- `internal/config/types.go:196-203`
- `internal/config/capabilities.go:72-117`

Implementation requirements:

1. Add `ProviderTypeOpenRouter ProviderType = "openrouter"` alongside the
   existing constants.
2. Append `ProviderTypeOpenRouter` to `providerTypeOrder` (after `zenmux`).
3. Add policy: default `chat, responses, embeddings`; supported adds `images,
audio_transcriptions, audio_speech`; `requiresBaseURL` false (has a
   default URL, like `zenmux`).
4. Extend `capabilities_test.go` with the openrouter row (mirrors the zenmux
   case).

Acceptance criteria:

- `EffectiveCapabilities("openrouter", Model{})` returns
  `chat, responses, embeddings`.
- Explicit `images` capability validates for `openrouter`; `messages` does not.
- `go test ./internal/config/` passes.
- OPENROUTER-01 regression test now passes at the type/policy seam.

### Task OPENROUTER-03: Provider descriptor, default URL, attribution headers

Status: completed

Completion evidence:

- Changed: `internal/provider/provider.go` (default URL + descriptor, `doOpenAI` reuse), `internal/provider/openai.go` (attribution headers scoped to openrouter, JSON + audio paths), `cmd/aiproxy/models_upstream.go` (discovery family + default URL).
- Added: `internal/provider/openrouter_test.go` (passthrough + headers, negative family test, audio headers, `OpMessages` reject, default URL; SSE usage test added later by 06).
- Verified: `go test -count=1 ./internal/provider/ ./cmd/aiproxy/` ok; `gofmt`/`vet` clean.

Priority: P0

Suggested agent: provider engineer

Dependencies: OPENROUTER-02

Primary ownership:

- `internal/provider/provider.go`
- `internal/provider/openai.go`
- `cmd/aiproxy/models_upstream.go`

Finding:

`providerDescriptors` maps 8 types to adapters; `openai`, `openai-compatible`
and `zenmux` share `doOpenAI`. `doOpenAI` sends only `Authorization`,
`Content-Type` and forwarded `Accept` — no OpenRouter attribution headers, so
OpenRouter billing/analytics attribution would be missing even if the type
existed.

References:

- `internal/provider/provider.go:169-210,241-250`
- `internal/provider/openai.go:15-48`
- `cmd/aiproxy/models_upstream.go:58,141-153`

Implementation requirements:

1. Add descriptor: `ProviderTypeOpenRouter` → default
   `https://openrouter.ai/api/v1`, `do: (*adapter).doOpenAI` (zenmux-pattern,
   no new adapter).
2. Send `HTTP-Referer: https://opencode.ai/` and `X-Title: opencode` on
   OpenRouter upstream inference requests (opencode convention). Scope the
   headers to the openrouter type only; `openai`/`zenmux` behavior unchanged.
3. Add the `openrouter` case to `models_upstream.go` upstream discovery
   (`doOpenAI`-family branch + default-URL branch), mirroring `zenmux`.
4. Keep `OpMessages` rejected for openrouter (via shared `doOpenAI` guard).

Acceptance criteria:

- Direct `openrouter` chat request hits `{base}/v1/chat/completions` with
  rewritten `model`, `Bearer` auth, and both attribution headers present.
- `openai` and `zenmux` requests do NOT gain the new headers (regression
  covered by test).
- `OpMessages` via openrouter returns `ErrUnsupportedOperation`.
- `go test ./internal/provider/ ./cmd/aiproxy/` passes.

### Task OPENROUTER-04: Validation, dynamic catalog, and configure CLI

Status: completed

Completion evidence:

- Changed: `cmd/aiproxy/configure.go` (8 openrouter mirror hunks: option, base-URL branches, type list, description, `OPENROUTER_API_KEY` default, capabilities family).
- Tests: `dynamic_test.go` 8→9 + assertions; `openrouter_test.go` +3 load/validation tests (4 total incl. 01's regression test); `configure_test.go` +2 CLI tests; `models_upstream_bounds_test.go` enumeration.
- Verified: `go test -count=1 ./internal/config/ ./internal/dbmerge/ ./cmd/aiproxy/` all ok; `gofmt` clean; validation/dbmerge needed no code changes (generic paths).

Priority: P1

Suggested agent: config/CLI engineer

Dependencies: OPENROUTER-03

Primary ownership:

- `internal/config/validate.go`
- `internal/config/dynamic.go`
- `internal/config/helpers.go`, `internal/config/build.go`
- `internal/dbmerge/dbmerge.go`
- `cmd/aiproxy/configure.go`

Finding:

`validate.go`, `dynamic.go`, `dbmerge.go` and `configure.go` enumerate known
types in multiple switches (credential kind, `base_url` required, healthcheck
eligibility, `extends` surface, interactive/non-interactive prompts). A new
type is rejected or misclassified until every switch mirrors `zenmux`.

References:

- `cmd/aiproxy/configure.go:1996-2031,2078-2097,2238,2286-2300,3248,3270,3774,3802`
- `cmd/aiproxy/models_upstream.go:58`
- `internal/config/validate.go`, `internal/config/dynamic.go` (zenmux-adjacent branches)
- `internal/dbmerge/dbmerge.go` (provider-type branches)

Implementation requirements:

1. `openrouter` uses `api_key`/`api_key_ref` credentials (exactly one, like
   other API-key types); never `credential_ref`.
2. `base_url` optional (defaults to `https://openrouter.ai/api/v1`); transport
   override only.
3. Eligible for `extends` derivation, healthcheck, and database-backed
   providers/aliases/keys merge — mirror `zenmux` in every switch.
4. `configure.go`: add `huh.NewOption("OpenRouter", "openrouter")`, type
   description text, non-interactive `--type openrouter` path, and the
   `openai/openai-compatible/zenmux`-family branch entries for openrouter.
   Non-interactive must NOT require `--base-url` (has default).
5. Add/extend CLI tests for `configure provider --type openrouter`
   (non-interactive + derived provider).

Acceptance criteria:

- `provider "openrouter" "x"` with `api_key` validates without `base_url`;
  missing credential fails validation.
- `configure provider --non-interactive --type openrouter --name x ...`
  succeeds without `--base-url`.
- `go test ./internal/config/ ./internal/dbmerge/ ./cmd/aiproxy/` passes.

### Task OPENROUTER-05: Public matrices and docs contract

Status: completed

Completion evidence:

- Changed: `scripts/check-doc-contracts.sh` (openrouter column + row, zenmux-identical) + mirrors in `AGENTS.md`, `README.md`, `docs/design.md`, `website/docs/api-reference.md`, `website/docs/providers-and-routing.md`, `website/docs/configuration.md` (+ prose type lists/sections).
- Verified: `make docs-contract` → "documentation contract matrices match"; `CHANGELOG.md` untouched.

Priority: P1

Suggested agent: docs engineer

Dependencies: OPENROUTER-04

Primary ownership:

- `scripts/check-doc-contracts.sh`
- `AGENTS.md`, `README.md`, `docs/design.md`
- `website/docs/api-reference.md`, `website/docs/providers-and-routing.md`
- `website/docs/configuration.md`, `website/docs/config-examples.md` (only if they enumerate types)

Finding:

`check-doc-contracts.sh` hardcodes `public_expected` (8 provider columns) and
`capability_expected` (8 rows); `make docs-contract` fails as soon as a 9th
type exists until the contract and all mirrored matrices are updated.

References:

- `scripts/check-doc-contracts.sh:6-33`
- `AGENTS.md`, `README.md`, `docs/design.md`,
  `website/docs/api-reference.md` (both matrices),
  `website/docs/providers-and-routing.md` (capability matrix)

Implementation requirements:

1. Add the `openrouter` column to `public_expected`: chat JSON and SSE;
   messages No; embeddings Yes; responses JSON and SSE; images/audio×3 Yes
   (identical to the `zenmux` column).
2. Add the capability row: default `chat, responses, embeddings`; additional
   `images, audio_transcriptions, audio_speech` (identical to `zenmux`).
3. Mirror both matrices into every file listed in the script's
   `public_matrix_files` / `capability_matrix_files`, plus prose type lists
   (HCL two-label syntax examples, `extends`, `api_key` vs `credential_ref`
   notes) where those files enumerate provider types.
4. Do NOT touch `CHANGELOG.md`.

Acceptance criteria:

- `make docs-contract` passes.
- `grep -c openrouter` is nonzero in every contract-checked file.
- No `CHANGELOG.md` modification (`git status` clean for it).

### Task OPENROUTER-06: Coverage and full verification

Status: completed

Completion evidence:

- Added: `internal/httpapi/openrouter_test.go` (alias route + 5xx failover); extended `internal/provider/openrouter_test.go` (SSE usage). Models listing already covered generically (no duplicate).
- Verified: `gofmt` clean, `make vet` exit 0, `go test ./...` exit 0, race suites ok, `make docs-contract` match; `CHANGELOG.md` untouched.

Priority: P0

Suggested agent: test engineer

Dependencies: OPENROUTER-05

Primary ownership:

- `internal/provider/*_test.go`
- `internal/httpapi/*_test.go` (alias/dispatch coverage if type-gated)
- `internal/config/*_test.go`, `cmd/aiproxy/*_test.go`

Finding:

Per-task tests prove each seam, but cross-path coverage (alias routing to
openrouter, failover on retryable statuses, streaming usage accounting, model
listing via `EffectiveCapabilities`) needs one consolidated pass, plus the
repo's full gates.

References:

- `internal/provider/provider.go:277-285` (operation descriptors)
- `internal/httpapi/operations.go` (ingress gate)
- Prior tasks' tests

Implementation requirements:

1. Add/confirm tests: direct openrouter chat JSON + SSE passthrough (model
   rewrite, headers incl. attribution); alias route to openrouter; retryable
   `5xx` failover across openrouter targets; `OpMessages` rejection end to end;
   streaming usage accounting intact.
2. Run `gofmt -l` (clean), `make vet`, `go test ./...`,
   `go test -race -count=1 ./internal/httpapi/ ./internal/provider/ ./internal/config/`,
   `make docs-contract`.
3. Confirm `CHANGELOG.md` untouched and no files outside this repo modified.

Acceptance criteria:

- All new tests fail before their fix seam and pass after (note which were
  added in earlier tasks vs this one).
- `make vet`, `go test ./...`, race suites, and `make docs-contract` all pass.
- `git status --short CHANGELOG.md` is empty.

### Task OPENROUTER-07: Final integration review

Status: completed

Completion evidence (independent reviewer, read-only + verification only):

- Per-task verdicts: 01 pass, 02 pass, 03 pass, 04 pass, 05 pass, 06 pass (verified against diff + executed tests, not code presence).
- Gates executed by reviewer: spot suites ok; `httpapi`/`accounting`/`payloadlog` ok (messages/effort work intact); `make docs-contract` match; `gofmt` clean; `make vet` exit 0; `go test -count=1 ./...` exit 0 zero failures; race ok for `config` + `provider`; `CHANGELOG.md` empty.
- Review verdict: ship.
- Follow-ups (non-blocking): (1 P2) re-run `go test -race -count=1 ./internal/httpapi/` in CI — reviewer verified race for config+provider only; (2) `X-Title: opencode` vs `aiproxy` maintainer preference stands as recorded; (3) OpenRouter-specific retryable-4xx stays on default alias policy unless upstream demands otherwise.

Priority: P2

Suggested agent: independent reviewer (must NOT be the implementer of 01-06)

Dependencies: OPENROUTER-06

Primary ownership: whole diff

Finding:

Independent verification is required that every acceptance criterion holds
against runtime behavior, docs match implementation, and no unrelated work
was disturbed.

References: this file; full `git diff`; contract script output.

Implementation requirements:

1. Verify each task's acceptance criteria against runtime behavior (not just
   code presence): type known, capabilities, headers, CLI, discovery,
   matrices, cross-path alias/failover/streaming.
2. Check alternate entry paths: direct, alias, `extends`-derived, DB-merged
   openrouter providers behave identically.
3. Confirm public types, docs, and implementation agree; `CHANGELOG.md`
   untouched; uncommitted messages/effort work intact (spot-check its tests
   still pass: `go test ./internal/httpapi/ ./internal/accounting/ ./internal/payloadlog/`).
4. Run and report: `gofmt -l`, `make vet`, `go test ./...`, `make docs-contract`.

Acceptance criteria:

- Review verdict (ship / ship-with-follow-ups / block) with per-task evidence.
- Any follow-ups filed as new `deferred`/`pending` tasks with rationale — not
  hidden in prose.
- This file's statuses + completion evidence are accurate and complete.

## Dependency and parallelization guidance

Strictly sequential: 01 → 02 → 03 → 04 → 05 → 06 → 07. No parallel execution:
shared hotspots (`types.go`, `capabilities.go`, `provider.go`, `configure.go`,
contract script + mirrored docs) overlap materially, and later tasks depend on
earlier behavioral decisions (capability policy → headers → CLI → docs).

## Deferred decisions requiring maintainer input

- `X-Title` value: `opencode` (opencode's own convention) assumed. If you
  prefer `aiproxy`, change in OPENROUTER-03 + tests together.
- Whether OpenRouter-specific retryable `4xx` (e.g. rate-limit shapes) should
  differ from the default alias policy: out of scope; default policy applies.
- `bedrock`/`azure`/`vertex` remain future work; this file covers `openrouter`
  only.

## Definition of done

- `provider "openrouter" "x"` works end to end: validation, direct + alias +
  derived + DB-merged, JSON and SSE, usage accounting, discovery, CLI, docs.
- `make vet`, `go test ./...`, race suites, `make docs-contract` pass;
  `gofmt` clean.
- `CHANGELOG.md` unmodified; no changes outside this repo.
- All tasks `completed` with `Completion evidence`; reviewer verdict recorded.

## Re-review (post-ship)

Two parity gaps found and fixed: `website/docs/intro.md` provider list gained
the `openrouter` bullet; new `examples/openrouter.hcl` mirrors `zenmux.hcl`
(`make validate CONFIG=examples/openrouter.hcl` → valid). Fresh gates:
`go test ./...` exit 0, `make docs-contract` match, `gofmt`/`vet` clean,
`go test -race -count=1 ./internal/httpapi/` ok (closes review follow-up 1).
Verdict stands: ship.

## Re-review 2 (DB-gated count)

Found one real miss: `TestAdminProviderTypesEndpoint`
(`internal/httpapi/admin_validate_test.go:300`) asserted 8 provider types and
was silently skipping locally (`AIPROXY_TEST_DATABASE_URL` unset), so all
prior green runs were false confidence for this seam — it would have failed
in CI. Reproduced against a local postgres container (`provider_types has 9
items, want 8`), then fixed to 9 plus openrouter assertions (`credential:
api_key`, `requires_base_url: false`). Verified: endpoint test passes;
CI-equivalent `go test -race -p 1 ./internal/store ./internal/dbmerge
./internal/httpapi ./internal/app` all ok; repo-wide sweep for sibling
stale-count assertions found none. Container removed. Verdict stands: ship.

## Re-review 3 (prose + CLI mirror pins)

Two more small gaps, both fixed: `website/docs/config-examples.md` had a
`## ZenMux Gateway` section with no OpenRouter counterpart — added
`## OpenRouter Gateway` (env-key example, attribution note, link to
`examples/openrouter.hcl`). `defaultProviderEnvExpression("openrouter")`,
`supportedCapabilities("openrouter")`, and `defaultCapabilities("openrouter")`
had no unit pins — added `TestConfigureOpenRouterCapabilityAndEnvMirror`
(one self-inflicted edit breakage repaired and re-verified). Noted but
deliberately unchanged: CLI interactive `defaultCapabilities` yields
`[chat, responses]` for the whole openai family (server default adds
`embeddings`) — pre-existing family-wide skew, out of scope to diverge.
Verified: `cmd/aiproxy` + `internal/config` suites ok, docs-contract match,
`CHANGELOG.md` untouched. Verdict stands: ship.

## Re-review 4 (live binary smoke)

Booted the built binary against `examples/openrouter.hcl`: `/v1/models`
lists both openrouter models with correct capabilities plus the alias.
Against a local stub upstream (via `base_url` override): direct
`openrouter/gpt-4o-mini` and `alias/openrouter_chat` chat requests return
through the proxy; stub observed `PATH=/v1/chat/completions`,
model rewritten to `openai/gpt-4o-mini`, `Bearer` auth, and
`HTTP-Referer: https://opencode.ai/` + `X-Title: opencode`. One note: an
early probe hit the real upstream with the placeholder key (401, harmless)
before the stub was wired — all asserting traffic stayed hermetic. Both
scratch processes stopped. Verdict stands: ship.

## Re-review 5 (live SSE/responses/embeddings + accounting)

Live stub loop extended beyond JSON chat: SSE chat streams through
(`[DONE]` intact, terminal-chunk usage 4/6/10 captured), `/v1/responses` and
`/v1/embeddings` pass through, and `/v1/billing/usage` records all three
operations with correct token counts — proving stream usage accounting live,
not just in unit tests. Stub observed both attribution headers on all three
upstream paths. Hunk-level diff re-read (`openai.go`, `provider.go`,
`models_upstream.go`) is minimal and correctly scoped; the `OpMessages`
reject is inherited with correct provider identity. No repo junk beyond the
intended new files; `CHANGELOG.md` untouched. Verdict stands: ship.

## Re-review 6 (live media + convert round-trip)

`aiproxy convert` HCL→JSON preserves the openrouter provider and the JSON
re-validates. Live stub loop for the remaining operations: default models
correctly _reject_ images/audio (defaults are chat/responses/embeddings —
supported ≠ default, same as openai/zenmux); with explicit per-model
capabilities, images JSON and multipart audio transcriptions pass through,
stub observing model rewrite _and_ both attribution headers on each,
including inside the multipart body. Scratch processes stopped. Verdict
stands: ship.

## Re-review 7 (live SIGHUP + full gates)

SIGHUP reload proven live: adding `llama-new` to the openrouter provider
surfaced it in `/v1/models` without restart (`config reloaded`); a broken
config was rejected (`config reload failed`) with the old runtime intact;
restored config reloaded and the server stopped cleanly. Fresh gates:
`gofmt` clean, `make vet` clean, `make docs-contract` match,
`go test ./...` exit 0. Verdict stands: ship.

## Re-review 8 (secrets + web-ui + tree)

Secrets: payloadlog redaction is header-name-generic — openrouter's
`Authorization: Bearer` is redacted like every other type; the attribution
headers are public constants, safe to log. Web-ui: one transient `vitest`
exit-1 with zero failure lines (scratch servers were up at the time);
two consecutive re-runs pass 209/209, and the only web-ui diff is 8 lines
from the messages/effort work — openrouter touches no web-ui files. Tree:
46 changed/new paths, all intended; `CHANGELOG.md` untouched. Verdict
stands: ship.

## Re-review 9 (parallel race smoke + test read)

Fired 50 parallel alias chat requests at a `-race` binary against a stub:
zero `DATA RACE` warnings. Response mix reflected harness artifacts
(single-threaded stub + strict per-model check vs round-robin across two
models + race-binary latency), not proxy defects — failover/routing for
those paths is covered deterministically in unit tests. Full read of
`internal/provider/openrouter_test.go`: table-driven negative test,
`errors.As` identity assertions, SSE usage verification — no quality
concerns. Verdict stands: ship.
