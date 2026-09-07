# OpenCode Zen And Go Providers

Created: 2026-09-06 10:16:16 (local repository environment time)

Status: completed

Completion evidence (2026-09-07, final review): all four items verified
completed — OPCODE-01 (protocol/header contract), OPCODE-02 (config + shared
adapter), OPCODE-03 (CLI + docs/examples), OPCODE-04 (independent hermetic
e2e + full shared checks, all passing, no live keys/spend). Each section
below carries its own Completion evidence. Final re-verification from repo
root: `make docs-contract` → matrices match; `git diff --check` → clean;
`make validate CONFIG=examples/opencode-zen.hcl` → valid;
`make validate CONFIG=examples/opencode-go.hcl` → valid;
`go test` over config/provider/httpapi/configedit/cmd-aiproxy/e2e/app →
all `ok`. `CHANGELOG.md` intentionally untouched per instruction. Note:
working tree also contains unrelated pre-existing changes outside this
objective (`helm/` untracked, `.github/workflows/pre-commit.yml`
`commit-on-push` toggle); they were left alone.

## Objective And Scope

Add first-class `opencode-zen` and `opencode-go` provider types with service-specific
default URLs and shared adapter logic, with correct model-specific upstream protocols,
JSON/SSE handling, configure CLI support, and accurate public documentation.
This document plans implementation; no application code has been changed.

Non-goals: subscription management, upstream billing/quota replication, automatic
paid fallback, a dynamic provider plugin registry, runtime catalog synchronization,
new public Messages/Gemini endpoints, or new general-purpose protocol translators.
Embeddings, images, and audio are out of scope unless separately justified and planned.

## Recommendation And Evidence

Decision update: the initial plan recommended one `opencode` type with separate
instances. Following maintainer discussion, that recommendation is superseded by
**two explicit provider types, `opencode-zen` and `opencode-go`, backed by shared
adapter logic**. Separate types make service selection and endpoint defaults clear
without requiring URLs or an additional service field. Do not introduce a generic
`opencode` type or compatibility alias as part of this feature.

| Provider type  | Default base URL                |
| -------------- | ------------------------------- |
| `opencode-zen` | `https://opencode.ai/zen/v1`    |
| `opencode-go`  | `https://opencode.ai/zen/go/v1` |

Keep `base_url` optional as a transport override for tests and custom gateways.
The provider type, not the URL or credential, selects service-specific behavior,
including Go header requirements. An override must not reclassify the service.
Do not silently switch services or mix billing modes by model. OpenCode's own
client IDs (`opencode` and `opencode-go`) do not dictate aiproxy type names.

Proposed configuration shape, not yet supported by the application:

```hcl
provider "opencode-zen" "zen" {
  api_key = env("OPENCODE_ZEN_API_KEY")

  model "glm-5.3" {
    capabilities = ["chat"]
  }
}

provider "opencode-go" "go" {
  api_key = env("OPENCODE_GO_API_KEY")

  model "glm-5.3" {
    capabilities = ["chat"]
  }
}
```

Public model names are `zen/glm-5.3` and `go/glm-5.3`. These examples illustrate
type/name labels and omitted URLs only. OPCODE-01 must add any required protocol
declarations once that schema is settled; capabilities alone do not select protocol.

Official sources inspected on 2026-09-06:

- [Zen endpoints](https://opencode.ai/docs/zen/#endpoints): service prefix
  `https://opencode.ai/zen/v1`; models use Responses, Chat Completions, Anthropic
  Messages, or Gemini model endpoints depending on the model.
- [Go endpoints](https://opencode.ai/docs/go/#endpoints): service prefix
  `https://opencode.ai/zen/go/v1`; models use Responses, Chat Completions, or
  Anthropic Messages. The documented MiniMax M3 endpoint is Chat Completions on
  Zen but Messages on Go. Model-name-only routing is therefore insufficient.
- Both pages publish service-specific `/models` endpoints. Catalogs change;
  model examples in this plan are evidence, not a permanent allowlist.
- [Go client requirements](https://opencode.ai/docs/go/#where-can-i-use-it):
  callers must identify themselves with a non-generic user agent and include
  `x-opencode-session` for prompt caching, while avoiding abusive traffic.
- [Go usage beyond limits](https://opencode.ai/docs/go/#usage-beyond-limits):
  the upstream console can enable spending Zen balance after Go limits. That is
  an upstream account setting, not permission for aiproxy to reroute requests.

Consequently, simply registering both types as `doOpenAI` would cover only a
subset. One shared adapter needs a deliberate per-model protocol contract, or
the release must explicitly restrict which model/protocol combinations it supports.
Capabilities describe public operations; they cannot alone select an upstream
protocol because multiple protocols can serve public chat/responses operations.

## Repository Analysis

- `internal/config/types.go:91-143` and `schema.go:74-97`: four provider types;
  provider-level URL/credential and model-level upstream name/capabilities, but no
  service or protocol selector.
- `internal/config/capabilities.go:37-68` and `validate.go:141-211`: provider policy
  registry controls accepted types, capabilities, and URL requirements/safety.
- `internal/provider/provider.go:134-155,195-209`: descriptors choose one adapter
  and default URL per provider type. `internal/httpapi/dispatch.go` builds adapter
  requests for both direct and alias targets.
- `internal/provider/openai.go:15-60,235-240`: model rewrite, bearer authentication,
  JSON/SSE pass-through, and special handling for a base URL ending in `/v1`.
  It copies `Accept`, not the Go session header or caller user agent.
- `internal/provider/anthropic.go:59-79,101-125`: translation appends `/v1/messages`
  and uses `x-api-key` plus `anthropic-version`.
- `internal/provider/gemini.go:120-132,203-215`: translation appends `/v1beta/models/`
  paths and uses `x-goog-api-key`. Zen documents `/zen/v1/models/`, so changing
  only the base URL is not sufficient to reuse this transport unchanged.
- `internal/config/build.go:136-191`: derived providers cannot override endpoints
  or models. Zen and Go should be separate concrete blocks, not an `extends`
  relationship that violates the existing contract.
- `cmd/aiproxy/configure.go` (`promptProviderInput`, `defaultProviderEnvExpression`,
  `defaultCapabilities`, `supportedCapabilities`): separate provider lists and
  policy; interactive flows clear URLs for types other than `openai-compatible`.
- `scripts/check-doc-contracts.sh`: hard-coded matrices must change together with
  their copies in `AGENTS.md`, `README.md`, `docs/design.md`, and website docs.

Related work, not duplicated here:

- `20260823-112437-codebase-health-follow-up.md`, completed PROVIDER-POLICY-01:
  intentionally keeps config policy and adapter registration separate, with parity
  tests. Preserve that boundary rather than reopening registry consolidation.
- `20260804-125911-codebase-health-review-remediation.md`, PROVIDER-01:
  preserve pass-through fields and reject unsupported translated features.
- `20260822-160713-provider-inheritance.md`, PROVIDER-INHERIT-01:
  preserve restricted inheritance semantics.

Coverage: focused source/test/contract inspection and a search of existing task
documents found no dedicated Zen/Go provider task. Working tree was clean before
task creation. No paid/authenticated requests, upstream source-level auth audit,
or full catalog validation were performed. Documentation establishes protocol
diversity but does not establish every accepted header or alternate endpoint.
Tests/builds were not run during analysis because this is a planning-only change.

## Execution

Priority P1 means a correctness prerequisite for this feature, not an existing
production incident. P2 means required delivery/integration work after the contract
is settled. Execute in order to avoid overlapping config, adapter, and docs edits.

### Task OPCODE-01: Settle Protocol And Header Contracts

Status: completed

Kind: investigation

Priority: P1, prevents wrong protocol routing, credential handling, and billing mode.

Dependencies: none

Primary ownership: this document's decision record; relevant official OpenCode
documentation/source and focused disposable protocol probes if necessary.

Finding: Zen and Go will share adapter logic behind two explicit types, but the current schema cannot
describe heterogeneous upstream protocols and official docs leave transport details
that must be checked before safely reusing existing adapters.

References: official sources and provider/config paths in the sections above.

Requirements:

1. Record a table of service, protocol, URL construction, authentication headers,
   public operations, and JSON/SSE behavior. Inspect official upstream source at a
   recorded revision where docs are insufficient; do not infer auth solely from
   AI SDK package names. Live requests require an authorized key and spending approval.
2. Retain the decided two-type service selection and default URLs above. Specify
   override validation, version/path joining, and inheritance/reload behavior.
   Service identity must remain type-driven even with a loopback/custom URL;
   cross-type inheritance remains invalid under the existing same-type rule.
3. Choose the smallest deterministic per-model protocol representation. Prefer
   explicit configuration over a hard-coded changing catalog or name prefixes.
   Define omitted/unknown protocol behavior and model capability validation/defaults.
4. Bound initial support: native chat and Responses pass-through, existing Messages
   and Gemini translation subsets where verified. Do not promise chat-to-Responses
   or Responses-to-chat conversion without an existing suitable implementation.
5. Define Go user-agent and session-header handling, including missing/invalid values,
   stable conversation identity, size bounds, and tenant isolation. Do not invent
   a global session ID or impersonate the OpenCode client. Avoid forwarding arbitrary
   inbound credentials/headers to satisfy these requirements.

Acceptance criteria:

- This document contains an actionable schema, protocol/support matrix, header
  policy, and two proposed HCL examples, clearly labeled until implemented.
- Every unresolved auth/path/header question has evidence or a specifically scoped
  deferral. No deferred protocol is advertised as supported.
- Maintainer decisions, if needed, are recorded as blockers rather than guessed.

Verification: evidence review against the cited docs and recorded upstream source
revision; no live-provider access is required if source evidence resolves the contract.

Completion evidence:

Recorded 2026-09-07. No application code was changed. No live authenticated
requests were made. Official pages `https://opencode.ai/docs/zen/` and
`https://opencode.ai/docs/go/` were fetched on 2026-09-07; both footers read
"Last updated: Sep 6, 2026". Model IDs below are evidence samples from those
fetches, not a permanent allowlist; catalogs change and per-model routing comes
from explicit configuration, never from a hard-coded catalog or name prefix.

1. Upstream endpoint table (condensed from the fetched `#endpoints` tables):

| Service | Base prefix                     | Upstream path (relative to base)                                                                         | SDK package shown           | Auth evidence                                                                                                                                                         | Serves public                                                                                      | JSON / SSE                                                  |
| ------- | ------------------------------- | -------------------------------------------------------------------------------------------------------- | --------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------- | ----------------------------------------------------------- |
| Zen     | `https://opencode.ai/zen/v1`    | `/responses` (e.g. `gpt-5.5`, `grok-4.6`, `muse-spark-1.3`)                                              | `@ai-sdk/openai`            | `Authorization: Bearer <key>` presumed (OpenAI-SDK `apiKey` pattern; Bearer curl against `/models` attested only by third-party examples, not by the official docs)   | Responses                                                                                          | JSON presumed; SSE presumed via AI SDK, unverified per path |
| Zen     | `https://opencode.ai/zen/v1`    | `/chat/completions` (e.g. `glm-5.3`, `deepseek-v4-flash`, `kimi-k3`, `minimax-m3`)                       | `@ai-sdk/openai-compatible` | Same Bearer presumption as above                                                                                                                                      | Chat                                                                                               | JSON presumed; SSE presumed, unverified per path            |
| Zen     | `https://opencode.ai/zen/v1`    | `/messages` (e.g. `claude-sonnet-5`, `qwen3.7-plus`)                                                     | `@ai-sdk/anthropic`         | Bearer presumed; must be verified against upstream source (must NOT assume `x-api-key`; aiproxy's existing `x-api-key` injection must not be reused without evidence) | Chat via existing Messages translation subset; Responses via existing Responses translation subset | Translated subset, JSON and SSE, as implemented today       |
| Zen     | `https://opencode.ai/zen/v1`    | `/models/<id>` (e.g. `/models/gemini-3.8-flash`; native Gemini REST shape, SDK appends operation suffix) | `@ai-sdk/google`            | DEFERRED: Bearer vs `x-goog-api-key` unresolved (cf. the `ACCESS_TOKEN_TYPE_UNSUPPORTED` failure pattern when a Bearer header reaches a Google backend)               | Chat + Responses via existing Gemini translation subset only                                       | Translated subset, JSON and SSE, as implemented today       |
| Zen     | `https://opencode.ai/zen/v1`    | `GET /models` service catalog                                                                            | n/a                         | Same Bearer presumption; proxy keeps `GET /v1/models` proxy-owned, no runtime sync (non-goal)                                                                         | Inventory only                                                                                     | JSON                                                        |
| Go      | `https://opencode.ai/zen/go/v1` | `/responses` (e.g. `grok-4.6`, `gpt-5.6-luna`, `muse-spark-1.3-contributor`)                             | `@ai-sdk/openai`            | Same Bearer presumption as Zen                                                                                                                                        | Responses                                                                                          | Same caveats as Zen                                         |
| Go      | `https://opencode.ai/zen/go/v1` | `/chat/completions` (e.g. `glm-5.3`, `kimi-k3`, `deepseek-v4-pro`, `hy3`, `omen-alpha`)                  | `@ai-sdk/openai-compatible` | Same Bearer presumption as Zen                                                                                                                                        | Chat                                                                                               | Same caveats as Zen                                         |
| Go      | `https://opencode.ai/zen/go/v1` | `/messages` (e.g. `minimax-m3`, `minimax-m2.7`, `qwen3.8-max`, `qwen3.7-plus`)                           | `@ai-sdk/anthropic`         | Bearer presumed; same `x-api-key` deferral as Zen                                                                                                                     | Chat + Responses via existing translation subsets                                                  | Translated subset, JSON and SSE, as implemented today       |
| Go      | `https://opencode.ai/zen/go/v1` | `GET /models` service catalog                                                                            | n/a                         | Same as Zen catalog row                                                                                                                                               | Inventory only                                                                                     | JSON                                                        |
| Go      | n/a                             | No Gemini-native `/models/<id>` rows documented on Go                                                    | n/a                         | n/a                                                                                                                                                                   | None                                                                                               | Not advertised                                              |

Divergence proof that model-name-only routing is insufficient: `minimax-m3`
uses `/chat/completions` on Zen but `/messages` on Go. Auth was not inferred
from SDK package names alone: the SDK column is recorded as a hint only, and
every per-path auth mapping above is either a stated presumption or an explicit
deferral to upstream-source inspection.

2. Service selection (retained decision): two explicit types, `opencode-zen`
   (default `https://opencode.ai/zen/v1`) and `opencode-go` (default
   `https://opencode.ai/zen/go/v1`); no generic `opencode` type or alias.
   `base_url` stays optional and is a transport override only: it must pass the
   existing `validateProviderBaseURL` rules (absolute http/https URL, non-HTTPS
   loopback only, no userinfo), trailing `/` is trimmed, a base ending in `/v1`
   must not produce duplicated version segments, and query strings on constructed
   targets (e.g. Gemini streaming `?alt=sse`) are preserved. An override never
   reclassifies the service: auth, header, and protocol behavior stay type-driven
   under loopback/custom URLs. Cross-type `extends` stays invalid under the
   existing same-type rule; derived blocks cannot declare `base_url`, models,
   timeouts, or the new per-model protocol field. The protocol field is model-level
   data and participates in the existing `SIGHUP` reload path like other
   provider/model content; no restart-only fields are introduced.

3. Per-model protocol schema (smallest deterministic representation; explicit
   config over hard-coded catalog):

```hcl
model "glm-5.3" {
  upstream_name = "glm-5.3"
  protocol      = "chat"
  capabilities  = ["chat"]
}
```

`protocol` is a required model-level string with exactly four values, `chat` |
`responses` | `messages` | `gemini`. Omitted or unknown `protocol` fails
validation (no silent default: a wrong default would route across billing
modes, e.g. `minimax-m3`). Protocol-to-upstream mapping:

| `protocol`  | Upstream request                                                                                                                          | Serves public operations                                                                              |
| ----------- | ----------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------- |
| `chat`      | `POST <base>/chat/completions`, model rewrite, JSON/SSE pass-through                                                                      | `chat` only; public Responses is rejected before upstream I/O                                         |
| `responses` | `POST <base>/responses`, model rewrite, JSON/SSE pass-through                                                                             | `responses` only; public chat is rejected before upstream I/O                                         |
| `messages`  | `POST <base>/messages`, existing Messages/Responses translation subsets                                                                   | `chat` and `responses` (translated subset only)                                                       |
| `gemini`    | `POST <base>/models/<upstream>:generateContent` (JSON) / `:streamGenerateContent?alt=sse` (SSE); exact suffix and auth per deferral below | `chat` and `responses` (translated subset only); Zen only, rejected on `opencode-go` until documented |

No chat-to-Responses or Responses-to-chat conversion is promised; unsupported
operation/protocol combinations return an unsupported-operation error with zero
upstream calls. Capabilities remain public-operation gating and are orthogonal
to protocol: allowed capabilities per protocol are `chat` -> {`chat`},
`responses` -> {`responses`}, `messages`/`gemini` -> {`chat`, `responses`}.
`embeddings`, `images`, `audio_transcriptions`, `audio_speech` are rejected on
both new types (out of scope). Omitted `capabilities` default to the full
protocol-served set (so `EffectiveCapabilities` must become protocol-aware for
these types; a provider-type-only default would over-advertise `responses` on
`chat`-protocol models). Any capability outside the protocol-served set, and
`gemini` on `opencode-go`, fail validation. Provider policy registers both
types with supported capabilities [`chat`, `responses`] and no `base_url`
requirement.

4. Initial support bound: native chat and Responses pass-through plus the
   existing Messages and Gemini translation subsets where verified above. No new
   protocol translators. Embeddings, images, and audio are out of scope. No
   deferred protocol is advertised as supported.

5. Header policy: the shared adapter sets `User-Agent: aiproxy/<version>` on
   every `opencode-zen`/`opencode-go` upstream request (non-generic tool
   identification; never forwards the inbound `User-Agent`; never impersonates the
   `opencode`/`opencode-go` client IDs). Only `opencode-go` additionally sends
   `x-opencode-session` on every supported protocol and public path: a caller
   supplied inbound value is forwarded as-is iff it is 1-128 characters of
   `[A-Za-z0-9_-]`; otherwise (missing or invalid) the proxy generates a fresh
   per-request random `ses_` + 128-bit hex ID. Missing/invalid never fails the
   request (the header is a caching hint, not auth) and never creates a global or
   cross-client session: generated IDs are never persisted, logged, or shared, so
   each authenticated client/tenant stays isolated. No other inbound
   headers or credentials (`Authorization`, `x-api-key`, cookies) are ever
   forwarded upstream. `opencode-zen` sends no session header (none documented).

PROPOSED HCL examples (labeled PROPOSED until OPCODE-02 implements them; the
`minimax-m3` contrast is the point — same model name, different protocol per
service):

```hcl
# PROPOSED — not yet implemented.
provider "opencode-zen" "zen" {
  api_key = env("OPENCODE_ZEN_API_KEY")

  model "glm-5.3" {
    protocol     = "chat"
    capabilities = ["chat"]
  }

  model "claude-sonnet-5" {
    protocol = "messages"
  }
}
```

```hcl
# PROPOSED — not yet implemented.
provider "opencode-go" "go" {
  api_key = env("OPENCODE_GO_API_KEY")

  model "minimax-m3" {
    protocol = "messages"
  }

  model "glm-5.3" {
    protocol = "chat"
  }
}
```

Evidence and scoped deferrals for OPCODE-02 (verify against official upstream
source at a recorded revision, e.g. the opencode provider definitions; no live
authenticated requests): (a) exact per-path auth header — Bearer vs `x-api-key`
on `/messages`, Bearer vs `x-goog-api-key` on Gemini-native paths, and whether
`/responses` and `/chat/completions` both accept Bearer on both services;
(b) exact Gemini suffix/query construction behind the documented
`/models/<id>` endpoint; (c) SSE support per path; (d) Go quota/limit error
shape for alias-retry classification (no Zen/Go crossing except explicitly
configured aliases; upstream "use balance" setting never permits proxy-side
rerouting). Decisions: none required from the maintainer; no blockers. Two-type
selection, defaults, required-`protocol` schema, capability rules, and the Go
header policy above are settled.

### Task OPCODE-02: Implement Config And Shared OpenCode Adapter

Status: completed

Kind: improvement

Priority: P1, core feature and transport correctness.

Dependencies: OPCODE-01

Primary ownership: `internal/config/`, `internal/provider/`,
`internal/httpapi/dispatch.go`, operation guards, and focused tests.

Finding: both types are currently rejected and adapter dispatch lacks the approved
OpenCode model/service metadata. Existing translators embed vendor paths and auth.

References: `providerDescriptors`, `providerTypePolicies`, `adapter.Do`,
`doOpenAI`, `doAnthropicChat`, `doAnthropicResponses`, and `doGemini` above;
`internal/httpapi/operations.go` (`ensureOperationSupported`).

Requirements:

1. Register `opencode-zen` and `opencode-go` in config policy and adapter descriptors
   with their default URLs and shared implementation. Implement OPCODE-01's schema,
   loading, validation, and runtime metadata. Preserve remote HTTPS validation, credentials/api_key_ref rules,
   disabled-provider behavior, restricted inheritance, and live reload consistency.
2. Reuse existing pass-through/translation logic with minimal transport customization;
   do not fork whole adapters. Preserve native provider URLs/authentication.
3. Route using the selected service and declared model protocol. Test exact prefix,
   version, suffix, trailing slash, and query construction with loopback upstreams.
4. Apply the approved Go header policy on every supported protocol and public path.
   Keep secrets out of errors/logs and avoid cross-client session mixing.
5. Preserve timeouts, cancellation, streaming cleanup, bounded usage observation,
   provider health, and existing error/alias retry policy. Direct requests never
   switch target; crossing Zen/Go is possible only through explicitly configured aliases.
6. Reject unsupported operation/protocol/capability combinations before upstream I/O.
   Keep `/v1/models` proxy-owned and capability/alias intersections truthful.

Acceptance criteria:

- Config tests cover both explicit types, omitted URLs, invalid protocols, missing
  credentials, unsupported capabilities, overrides, same-type inheritance, rejection
  of cross-type inheritance, and reload. A generic `opencode` type is not accepted.
- Registration parity tests include both OpenCode types without changing other provider policies.
- Default URL tests prove each type targets its documented service prefix. Override
  tests prove custom URLs do not change type-driven service/header behavior.
- Hermetic tests cover each supported service/protocol combination for JSON and SSE,
  model rewriting, auth/session headers, tool calls, usage, errors, and cancellation.
- The same model configured differently on Zen and Go reaches the correct protocol;
  unsupported requests make zero upstream calls and no implicit paid fallback occurs.
- Existing provider tests continue to pass, including translated feature rejection
  and byte-preserving pass-through behavior.

Verification: targeted config/provider/httpapi tests below, then `make vet test`.

Completion evidence:

Recorded 2026-09-07. Implemented exactly the OPCODE-01 contract; no
OPCODE-03 (CLI/docs) or OPCODE-04 work, no `CHANGELOG.md` change.

Files changed (implementation):

- `internal/config/types.go`: `ProviderTypeOpenCodeZen` (`opencode-zen`),
  `ProviderTypeOpenCodeGo` (`opencode-go`), `ModelProtocol`
  (`chat`|`responses`|`messages`|`gemini`), `Model.Protocol`.
- `internal/config/schema.go`: optional model-level `protocol` HCL attribute.
- `internal/config/capabilities.go`: both types registered in
  `providerTypeOrder`/`providerTypePolicies` with supported capabilities
  [`chat`, `responses`] and no `base_url` requirement;
  `EffectiveCapabilities` is protocol-aware for both types via
  `OpenCodeProtocolCapabilities` (`chat`->{`chat`},
  `responses`->{`responses`}, `messages`/`gemini`->{`chat`,`responses`}).
- `internal/config/validate.go`: required known protocol on both types,
  `gemini` rejected on `opencode-go`, capability-vs-protocol orthogonality
  check after the existing provider-support check, `protocol` rejected on
  non-OpenCode types. Existing URL, credential, disabled-provider, and
  same-type inheritance rules unchanged.
- `internal/config/build.go`: `Model.Protocol` populated from HCL; derived
  providers inherit it via the existing model clone, and derived blocks still
  cannot declare models/`base_url`.
- `internal/provider/provider.go`: `Request.ModelProtocol` and
  `Request.Version` fields; descriptors for both types with defaults
  `https://opencode.ai/zen/v1` and `https://opencode.ai/zen/go/v1` sharing
  `doOpenCode`.
- `internal/provider/opencode.go` (new, shared adapter): service+protocol
  routing only — `chat`=>`POST <base>/chat/completions`,
  `responses`=>`POST <base>/responses` (via `joinBaseURLAndPath`, so a base
  ending in `/v1` never duplicates the version segment),
  `messages`=>`POST <base>/messages`,
  `gemini`=>`POST <base>/models/<upstream>:generateContent` (JSON) /
  `:streamGenerateContent?alt=sse` (SSE), Zen only. Pass-through reuses the
  extracted `openAIPassthroughHandlers`; Messages/Gemini reuse the existing
  translate primitives with Bearer auth — no forked adapters, native provider
  behavior untouched. Unsupported operation/protocol combos return
  `ErrUnsupportedOperation` (unknown protocol returns `ErrInvalidRequest`)
  before any upstream I/O. Headers on every request: `Authorization: Bearer`,
  `Content-Type: application/json`, `User-Agent: aiproxy/<version>` (from
  `Request.Version`, `dev` fallback; inbound UA never forwarded);
  `opencode-go` additionally sends `x-opencode-session` (valid inbound
  `1-128` chars `[A-Za-z0-9_-]` forwarded as-is, otherwise fresh per-request
  `ses_`+128-bit hex from `crypto/rand`); Zen sends none; no other inbound
  headers/credentials forwarded; secrets never in errors/logs.
- `internal/provider/openai.go`: pass-through handler set extracted to
  `openAIPassthroughHandlers` (behavior unchanged, covered by existing tests).
- `internal/httpapi/dispatch.go` + `handler.go`, `internal/app/app.go`:
  `Dependencies.Version` threaded from app build version into every
  `provider.Request` alongside per-target `ModelProtocol` (direct and alias);
  timeouts, cancellation, streaming cleanup, usage, health, and alias retry
  paths unchanged; direct requests never switch target.

Files changed (tests):

- `internal/config/capabilities_test.go`: parity table extended with both
  OpenCode types; other provider policies unchanged.
- `internal/config/opencode_test.go` (new): both types with omitted URLs,
  omitted-capability protocol defaults, gemini-on-Zen, invalid/omitted
  protocols, missing credentials, unsupported capabilities, protocol/capability
  mismatches, generic `opencode` rejection, `protocol` on `openai` rejection,
  loopback override acceptance, remote-HTTP override rejection, same-type
  inheritance, cross-type inheritance rejection, derived-model prohibition,
  reload preservation, `EffectiveCapabilities` protocol matrix.
- `internal/provider/opencode_test.go` (new): descriptor defaults, empty
  `BaseURL` targeting documented prefixes via stub transport, hermetic
  loopback coverage per supported service/protocol for JSON and SSE (model
  rewrite, Bearer auth, UA/session policy, tools preservation, usage, error
  passthrough, cancellation), `/v1`-suffix dedup per protocol,
  same-model-different-protocol (`minimax-m3` chat on Zen vs messages on Go),
  session forward/generate policy incl. uniqueness, UA policy incl. `dev`
  fallback, unsupported-combo zero-upstream-call matrix.
- `internal/httpapi/opencode_test.go` (new): end-to-end direct routing by
  service+protocol with header assertions, unsupported direct request returns
  `400 unsupported_operation` with zero upstream calls, explicit alias
  failover Zen->Go on retryable `502`, `/v1/models` proxy-owned with zero
  upstream calls.

Test results (all from repository root, serial):

- `go test ./internal/config ./internal/provider ./internal/httpapi -count=1`:
  all three packages `ok`.
- `make vet test`: exit 0, all packages `ok` (including `cmd/aiproxy`,
  `internal/app`, `internal/e2e`); no failures introduced or fixed elsewhere.
- `make docs-contract`: `documentation contract matrices match`.
- `git diff --check`: clean.
- `make test-race`, `make integration`, and e2e beyond `go test ./...` were
  not run in this task; OPCODE-04 owns integrated verification.

### Task OPCODE-03: Expose Configuration And Document Support

Status: completed

Kind: improvement

Priority: P2, makes the implemented feature usable without misleading support claims.

Dependencies: OPCODE-02

Primary ownership: `cmd/aiproxy/configure.go` and tests, `internal/configedit/`,
`AGENTS.md`, `README.md`, `docs/design.md`, `website/docs/`, and
`scripts/check-doc-contracts.sh`.

Finding: CLI selection/defaults currently enumerate four types and can erase custom
endpoints. Public matrices cannot accurately describe heterogeneous model support
by copying the unrestricted OpenAI row.

References: configure symbols and documentation contract checker above;
`internal/configedit/configedit.go` (`RenderProviderBlock`).

Requirements:

1. Expose separate `opencode-zen` and `opencode-go` choices in interactive and
   scripted configuration. Support approved protocol fields; preserve optional
   URL overrides and all newly introduced fields on round trips.
2. Provide appropriate credential prompts/env suggestions and capability choices.
   Keep existing provider editing behavior intact.
3. Publish validated two-label HCL examples using the explicit types with omitted
   base URLs, demonstrating public model names and any required per-model protocol
   declaration. Document optional overrides without implying they select the service.
4. Update provider lists, endpoint guidance, capability and public support matrices,
   checker expectations, and configure examples together. Explain native versus
   translated subsets and rejected operations rather than claiming universal support.
5. Document session/client requirements, Go quota errors, explicit alias failover,
   upstream-controlled balance fallback, and static model configuration. Record the
   new public contract in the repository's release notes surface if one exists.

Acceptance criteria:

- CLI tests prove both types can be created with default URLs and edited without
  dropping metadata or explicit URL overrides.
- Examples load/validate with placeholder test credentials; published choices match
  runtime policy, including protocol-specific capability restrictions.
- `make docs-contract` passes with accurate OpenCode entries and subset caveats.

Verification: targeted CLI/configedit/config tests, example validation, and
`make docs-contract` from the repository root.

Completion evidence:

Recorded 2026-09-07. Implemented OPCODE-03 only; no OPCODE-01/02/04 changes,
no `CHANGELOG.md` change (release-notes update explicitly out of scope per
task instruction).

Files changed (CLI):

- `cmd/aiproxy/configure.go`: `opencode-zen`/`opencode-go` exposed in TUI and
  non-TUI provider-type choices; new `--model-protocol name=protocol` flag;
  required-protocol validation in scripted flows (missing/invalid protocol,
  `gemini` on `opencode-go`, and capability-outside-protocol rejections mirror
  runtime policy); protocol-aware capability defaults
  (`chat`->`chat`, `responses`->`responses`, `messages`/`gemini`->both);
  `OPENCODE_ZEN_API_KEY`/`OPENCODE_GO_API_KEY` env suggestions;
  optional `base_url` override prompt for OpenCode types (previously cleared
  for every non-`openai-compatible` type); protocol round-tripped through
  `existingProviderInput` and model prompts (interactive re-entry forced when
  existing models lack a valid protocol); `configure provider` example text
  extended with an opencode-zen command.
- `internal/configedit/configedit.go`: `ProviderModelInput.Protocol` added and
  rendered as `protocol = "..."` in `RenderProviderBlock`, so URL overrides
  and all new fields survive round trips.

Files changed (tests):

- `cmd/aiproxy/configure_opencode_test.go` (new): scripted creation of both
  types with default (omitted) URLs, Go explicit loopback override, missing
  protocol rejection, `gemini`-on-Go rejection, capability/protocol mismatch
  rejection, scripted edit preserving protocol plus URL override plus
  credential, interactive creation proving the `opencode-zen` choice and the
  `OPENCODE_ZEN_API_KEY` default, env/capability/protocol helper unit tests.
- `internal/configedit/configedit_test.go`: `TestRenderProviderBlockOpenCodeProtocol`
  proves protocol plus `base_url` override render and survive an upsert round
  trip.

Files changed (docs/examples/checker):

- `scripts/check-doc-contracts.sh`: public matrix gains `opencode-zen` and
  `opencode-go` columns (`chat`/`responses` are "JSON and SSE native or
  translated subset", `embeddings`/`images`/audio are "No"); capability matrix
  gains both types (protocol-dependent default, no additional capabilities).
- `AGENTS.md`, `README.md`, `docs/design.md`, `website/docs/api-reference.md`,
  `website/docs/providers-and-routing.md`: matrices updated together with
  provider lists, endpoint guidance (`base_url` transport-override-only),
  native-vs-translated protocol table, rejected operations, session/client
  requirements, Go quota/alias/balance-fallback/static-catalog notes.
- `website/docs/configuration.md`, `website/docs/config-examples.md`,
  `website/docs/operations.md`, `website/docs/intro.md`: two-label HCL
  examples with explicit types, omitted base URLs, public model names
  (`zen/glm-5.3`, `go/minimax-m3`), required per-model protocol, and
  scripted `configure provider --model-protocol` examples.
- `examples/opencode-zen.hcl` (new), `examples/opencode-go.hcl` (new):
  complete two-label configs with omitted base URLs, per-model protocols,
  explicit same-service aliases, placeholder test credentials.

Example validation (`make validate CONFIG=<path>`, repository root):

- `examples/opencode-zen.hcl`: `config is valid`.
- `examples/opencode-go.hcl`: `config is valid`.
- Composite docs snippet (Zen + Go providers plus cross-service
  `chat_fallback` alias, placeholder creds): `config is valid`.

Test results (repository root):

- `go test ./internal/configedit ./cmd/aiproxy -count=1`: both packages `ok`.
- `make vet test`: exit 0, all packages `ok`; no failures introduced.
- `make docs-contract`: `documentation contract matrices match`.
- `make test-race`, `make integration`, and OPCODE-04 e2e verification were
  not run in this task; OPCODE-04 owns integrated verification.

### Task OPCODE-04: Independently Verify Integrated Behavior

Status: completed

Kind: improvement

Priority: P2, release gate for a cross-cutting provider addition.

Suggested agent: reviewer other than the primary implementer.

Dependencies: OPCODE-02, OPCODE-03

Primary ownership: `internal/e2e/`, `internal/integration/`, and this task record.

Finding: isolated adapter tests cannot establish configuration-to-routing correctness
or prove that direct/alias paths preserve service selection and security boundaries.

References: `internal/e2e/e2e_test.go`, `internal/integration/binary_test.go`,
`internal/httpapi/dispatch.go`, and acceptance criteria above.

Requirements:

1. Add/extend hermetic integrated coverage for Zen/Go configuration through direct
   and explicit alias requests, capability gating, JSON/SSE, usage, and header isolation.
2. Review every prior acceptance criterion and verify no service switching occurs on
   quota/auth errors except existing explicitly configured alias retry behavior.
3. Run shared checks, record exact results, and triage new findings into this objective
   or a separate linked task. Do not mark unverified tasks completed.

Acceptance criteria:

- Independent review has no unresolved correctness findings for advertised support.
- Required checks pass; any optional live-provider evidence is clearly distinguished
  from hermetic verification and does not expose keys or spend without approval.

Verification: all shared checks below.

Completion evidence:

Recorded 2026-09-07. Independent reviewer; no OPCODE-01/02/03 sections changed,
no `CHANGELOG.md` change. No live provider keys used; all coverage is hermetic
over loopback httptest upstreams unless stated.

Integrated coverage added (`internal/e2e/opencode_test.go`, new; full-stack via
`app.Build`, loopback upstreams only):

- `TestE2EOpenCodeDirectChatAndResponses`: direct `zen/glm-5.3` chat JSON and
  `go/grok-4.6` responses JSON; asserts upstream paths `/v1/chat/completions`
  and `/v1/responses`, model rewrite, `Authorization: Bearer` per-provider key,
  `User-Agent: aiproxy/test`, Zen sends no session header, Go forwards a valid
  inbound `x-opencode-session`, inbound `Authorization`/`Cookie`/`X-Custom`
  never forwarded, and no cross-service upstream calls.
- `TestE2EOpenCodeTranslatedProtocols`: Go `messages`-protocol chat and Zen
  `gemini`-protocol chat through the translation subsets; asserts `/messages`
  and `/models/gemini-3.8-flash:generateContent` targets, Bearer auth on both
  (no `x-api-key` reuse on Messages, no `x-goog-api-key` on Gemini), Go session
  present, Zen session absent.
- `TestE2EOpenCodeStreamingChatSSE`: Zen chat streaming returns
  `text/event-stream` with content and `data: [DONE]`.
- `TestE2EOpenCodeCapabilityGating`: `/v1/responses` and `/v1/embeddings`
  against a chat-only model return `400 unsupported_operation` with zero
  upstream calls.
- `TestE2EOpenCodeDirectErrorsDoNotSwitchService`: direct Zen 401 and direct
  Zen 429 are returned verbatim with zero calls to the Go stub.
- `TestE2EOpenCodeExplicitAliasRetry`: explicit Zen->Go alias retries default
  retryable 502 to Go and succeeds; Go failover carries Bearer plus session,
  Zen carries no session.
- `TestE2EOpenCodeExplicitAliasRetryOnConfiguredQuotaStatus`: explicit alias
  with `retry_status_codes = ["429"]` retries Zen 429 to Go and succeeds.
- `TestE2EOpenCodeModelsProxyOwnedAndUsageRecorded`: `GET /v1/models` lists
  `zen/glm-5.3` and `go/minimax-m3` with zero upstream calls;
  `GET /v1/billing/usage` records `zen/glm-5.3` after a successful chat.

Review findings per prior criterion:

- OPCODE-01: contract table, required-`protocol` schema
  (`chat`|`responses`|`messages`|`gemini`), Zen-only `gemini`,
  protocol-aware capability defaults, and Go header policy in the record match
  the implementation (`internal/config/`, `internal/provider/opencode.go`);
  the `minimax-m3` Zen/chat vs Go/messages divergence is covered by
  `internal/provider/opencode_test.go` and the e2e direct tests above. No
  unresolved auth/path/header question is advertised as supported beyond the
  verified subsets. No findings.
- OPCODE-02: config/registration/adapter acceptance points verified —
  `go test ./internal/config ./internal/provider ./internal/httpapi` all `ok`;
  descriptor defaults target the documented prefixes; generic `opencode` type
  rejected (`internal/config/opencode_test.go` "generic opencode type");
  overrides stay transport-only; unsupported combos return before upstream I/O
  (e2e gating tests confirm zero upstream calls). No findings.
- OPCODE-03: CLI coverage (`go test ./internal/configedit ./cmd/aiproxy`
  both `ok`), both examples validate (see below), `make docs-contract`
  passes. No findings.
- Service-switching rule: `dispatch.go` direct path never retries another
  target; the alias path retries only on transport error or configured
  `retry_status_codes`; `recordProviderHealth` marks failure only on error or
  5xx, so retryable 4xx neither switch service directly nor poison health.
  The e2e tests above prove verbatim 401/429 on direct requests and failover
  only through explicitly configured aliases. No implicit paid fallback
  exists: no code path selects Zen vs Go except provider type and explicit
  alias targets. No findings; nothing triaged to a separate task.

Exact check outputs (all from repository root, serial):

- `go test ./internal/config ./internal/provider ./internal/httpapi -count=1`:
  `ok internal/config 0.039s`, `ok internal/provider 1.166s`,
  `ok internal/httpapi 0.405s`.
- `go test ./internal/configedit ./cmd/aiproxy -count=1`:
  `ok internal/configedit 0.015s`, `ok cmd/aiproxy 3.885s`.
- `go test ./internal/e2e ./internal/app -count=1`:
  `ok internal/e2e 0.813s`, `ok internal/app 0.377s`
  (includes the 8 new `TestE2EOpenCode*` tests, all PASS).
- `make vet test`: exit 0, all packages `ok`.
- `make test-race`: exit 0, all packages `ok` (incl. `internal/e2e 2.660s`).
- `make docs-contract`: `documentation contract matrices match`, exit 0.
- `make integration`: rebuilt `dist/aiproxy`,
  `ok internal/integration 1.262s`, exit 0.
- `git diff --check`: clean, exit 0.
- `make validate CONFIG=examples/opencode-zen.hcl`: `config is valid`.
- `make validate CONFIG=examples/opencode-go.hcl`: `config is valid`.
  (Examples declare literal `test-placeholder-key` credentials; placeholder
  `OPENCODE_ZEN_API_KEY`/`OPENCODE_GO_API_KEY` env values were also exported
  and are unused by these files.)

Live-provider distinction: no live-provider evidence was gathered and none is
claimed. No upstream credentials were used, no traffic left loopback, no spend
occurred. SSE per-path behavior beyond the hermetic stubs remains covered only
to the extent of the existing translation subsets, as scoped in OPCODE-01.

## Shared Verification And Definition Of Done

Prerequisites: repository-supported Go toolchain and Make; loopback sockets for hermetic upstreams.
No external provider credentials are required for automated tests. Serialize build/integration commands.

```sh
go test ./internal/config ./internal/provider ./internal/httpapi
go test ./internal/configedit ./cmd/aiproxy
go test ./internal/e2e ./internal/app
make vet test
make test-race
make docs-contract
make integration
git diff --check
```

Validate each implementation-owned example with `make validate CONFIG=<example-path>`
and non-secret test env values. Exact paths must be recorded when examples are added.

Done means OPCODE-01 decisions are recorded, implementation and CLI/docs agree,
advertised combinations pass hermetic coverage, independent review and required
checks pass, and each task has completion evidence. If verification is blocked,
record the missing prerequisite and leave the affected task blocked.

Current execution gate: provider types and default URLs are decided. OPCODE-01
must settle model protocol schema, transport/auth/session details
before implementation starts. No maintainer decision is needed to begin that
investigation. No implementation or test baseline is claimed by this task document.
