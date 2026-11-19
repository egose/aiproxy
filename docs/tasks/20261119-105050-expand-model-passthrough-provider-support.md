# Extend model-ID passthrough to the remaining provider types

Created: 2026-11-19 10:50:50 (local)

Status: completed; all four stages ran sequentially in separate subagent
sessions, with coordinator review and verification between stages.

## Objective and scope

Extend the completed [model-ID passthrough feature](20261118-214334-provider-model-passthrough.md)
to `anthropic`, `gemini`, `opencode-zen`, `opencode-go`, and `github-copilot`.
After an exact miss, an eligible client can call a matching upstream ID without
adding a literal model or enabling discovery. Only model selection changes:
the existing adapter still determines the wire format and supported operations.

| Provider                      | Required rule `protocol` | Allowed rule `capabilities`       | Where the matched ID goes                               |
| ----------------------------- | ------------------------ | --------------------------------- | ------------------------------------------------------- |
| `anthropic`                   | Not allowed              | `chat`, `responses`, `messages`   | Native or translated Messages `model` field             |
| `github-copilot`              | Not allowed              | `chat`                            | Chat `model` field through the existing Copilot adapter |
| `opencode-zen`, `opencode-go` | `chat`                   | `chat`                            | Native chat `model` field                               |
| `opencode-zen`, `opencode-go` | `responses`              | `responses`                       | Native Responses `model` field                          |
| `opencode-zen`, `opencode-go` | `messages`               | `chat`, `responses`, `messages`   | Native or translated Messages `model` field             |
| `gemini`                      | Not allowed              | `chat`, `responses`, `embeddings` | One model ID segment in the Gemini API path             |
| `opencode-zen` only           | `gemini`                 | `chat`, `responses`               | One model ID segment in the Zen Gemini path             |

Capabilities remain a required, nonempty, explicit subset of the applicable
row. Chat and Responses translation still use the existing supported subsets.
Keep eligibility explicit; adding a new provider type must not enable rules
on it automatically.
For example, inside an existing `opencode-zen` provider named `zen`:

```hcl
model_passthrough "claude-family" {
  match        = "^claude-[a-z0-9.-]+$"
  protocol     = "messages"
  capabilities = ["chat", "responses", "messages"]
}

model_passthrough "gemini-family" {
  match        = "^gemini-[a-z0-9.-]+$"
  protocol     = "gemini"
  capabilities = ["chat", "responses"]
}
```

A static bearer granted `allowed_model_passthroughs = ["zen/claude-family"]`
could call `zen/claude-sonnet-4-6` on a declared operation if there is no exact
definition and the upstream serves it. It cannot use the Gemini rule without
that rule's grant. A catch-all `match = ".*"` is valid but still picks exactly
one configured protocol; use disjoint patterns for multiple protocols.

### Preserve the existing contract

- Exact literal hits never fall back, including when denied, unsupported for
  the operation, or rejected upstream. On an exact miss, test whole-suffix
  regexes on the named in-scope provider; overlapping patterns fail closed
  before filtering by capability, protocol suitability, or caller grant.
  Unusable fallback keeps the literal miss response (`403` for allowlist
  exclusion, otherwise `404`); do not change auth/freshness errors.
- Rules remain opt-in on concrete static providers only, including rule-only
  configurations. Keep static bearer grants separate from `allowed_models`,
  file-only anonymous opt-in in `none` mode, and System workspace scoping.
  No managed declarations, DB-key grants, rule inheritance, or peer deferred
  admission/dispatch.
- Preserve the limits of 32 rules per provider, 512 bytes per regex, and
  256 bytes per suffix. Body-based adapters retain the existing lowercase
  slash-separated ID grammar. Gemini URL-based rules additionally require a
  single bare ID segment. Do not trim, case-fold, decode, strip a resource prefix,
  or rewrite captures from the regex.
- Keep the existing rate limit, destination and credential policy, canonical
  inbound `model` validation, and guardrail enforcement on covered operations.
  Preserve provider/root reasoning handling and its same-target recovery only
  where already applicable; no stripping/recovery on native Messages. Keep
  ordinary upstream error handling, not cross-model, protocol, or service
  failover.
- Rule calls remain direct only. No new endpoint, native Gemini public API,
  translator feature, OAuth flow, CLI/web authoring, discovery, `/v1/models`
  entries, or alias targets. Keep usage unpriced and grouped under
  `<provider>/@passthrough/<rule>`; responses must not substitute this synthetic
  identity for the adapter's normal client-facing model value. The original
  four providers' rules and existing literal/alias behavior are unchanged.

## Analysis and verification baseline

Inspected clean `2533c40`: rule validation and matching, the literal protocol
capability checks, common admission/dispatch, the five adapters, relevant
fixture tests, and the prior task files. The allowlist in
`internal/config/model_passthrough.go` currently excludes all five types.
This is an expansion, not a claim that existing literal routes are broken.
No duplicate expansion task was found. The mock-only
[Copilot live verification deferral](20260927-102347-copilot-live-compatibility.md)
remains in force and is not an implementation dependency.

From the repo root, this focused baseline passed:

```sh
go test -count=1 ./internal/config ./internal/provider -run '^(TestModelPassthroughProviderScope|TestOpenCodeDefaultBaseURLs|TestGeminiAdapterEmbeddingsBatch|TestGitHubCopilotChatPathBearerAndHeaders)$'
```

No full suite or live inference was run for task creation. API schema evidence
below describes documented wire requirements, not deployed behavior.
Implementation preflight: `make check-toolchain` passed from the repo root;
local Helm 4.3.2 is available.

P1 means required for this enhancement. Sequence MPTX-01 → MPTX-02 → MPTX-03
→ MPTX-04 because validation, direct dispatch and HTTP admission are shared.
Each stage should enable only the provider/protocol combinations it verifies.

## Tasks

### Task MPTX-01: Enable Anthropic and GitHub Copilot rules on their existing adapters

Status: completed

Kind: improvement

Priority: P1 — enables the body-based providers that do not need rule protocol metadata

Dependencies: [MPT-04](20261118-214334-provider-model-passthrough.md#task-mpt-04-document-and-verify-the-integrated-feature) (completed)

Primary ownership: `internal/config/model_passthrough.go`,
`internal/httpapi/model_passthrough.go`, and config/HTTP/provider fixture tests.

Finding: `supportsModelPassthrough` rejects both types today, although
`doAnthropicChat`, `doAnthropicResponses`, `doAnthropicMessages`, and
`doGitHubCopilot` already use `Request.UpstreamModel`. Native Messages
preserves fields besides `model`; some Anthropic-compatible endpoints can
interpret the top-level `fallbacks` field as another model selector.

References: `internal/provider/anthropic.go` (`doAnthropicMessages`),
`internal/provider/githubcopilot.go` (`doGitHubCopilot`),
`internal/copilotlogin/auth.go` (`ApplyHeaders`),
`internal/config/helpers.go` (`resolveCopilotCredential`),
`internal/httpapi/admission.go` (`admitInferenceBody`), and the documented
[compatible Messages extension](https://openrouter.ai/docs/guides/routing/model-fallbacks#using-with-the-anthropic-messages-api).

Requirements:

1. Enable static mixed and rule-only configurations with the capabilities in
   the table. Reuse existing adapters, required credentials and outbound
   header policy. Copilot uses the existing loaded `credential_ref`, not
   `api_key` or caller authorization, with no new login/refresh behavior.
2. Keep the common canonical model checks and `models`/`preset` rejection;
   also reject case-insensitive top-level `fallbacks` on rule calls to
   native Messages, including on the post-guardrail check. Do not modify
   literal or alias body handling. This constrains known selectors the
   proxy sends, not arbitrary behavior of a trusted upstream.

Acceptance criteria:

- Loopback captures show the matched ID unchanged in Anthropic's native
  Messages and translated chat/Responses calls and Copilot chat, with
  existing JSON/SSE behavior and credentials. Native thinking/tool fields,
  normal response model fields and usage accounting are preserved.
- Disabled declarations validate but cannot dispatch. Invalid capabilities,
  absent or invalid local Copilot credential declarations, excluded operations,
  denied exact hits, and competing model fields never dispatch via a rule.
  An upstream Copilot authorization rejection retains its ordinary behavior
  and never triggers credential refresh or fallback.

Verification: `go test ./internal/config ./internal/httpapi ./internal/provider ./internal/copilotlogin`.

Completion evidence:

- Enabled the two types in `internal/config/model_passthrough.go`, and reused
  the existing provider adapters. `internal/httpapi/model_passthrough.go`
  rejects top-level `fallbacks` on native Messages rule calls, even after
  guardrail rewriting. Literal and alias handling was not changed.
- Config and loopback HTTP tests cover all four routes with JSON/SSE,
  native thinking/tools and scoped usage, Copilot credential validation and
  ordinary upstream 401/403 without refresh, exact priority, invalid selectors,
  disabled providers, and guardrail block/redaction. Coordinator reran the
  four-package suite with `-count=1`; it passed. Focused passthrough race
  tests, `make vet test`, and whitespace checks passed. No live or DB-backed
  checks ran in this stage.

### Task MPTX-02: Pin OpenCode body-based rules to an explicit protocol

Status: completed

Kind: improvement

Priority: P1 — prevents choosing the service or protocol from caller input

Dependencies: MPTX-01

Primary ownership: `internal/config/schema.go`, `internal/config/model_passthrough.go`,
`internal/httpapi/dispatch.go`, and protocol, conversion, and CLI preservation tests.

Finding: rules store a regex and capabilities but no protocol.
`dispatchDirect` uses `r.Model.Protocol`, which is unset for rule matches;
`doOpenCode` rejects an unset protocol. Literal validation already checks the
provider/protocol/operation combination.

References: `internal/config/model_capabilities.go` (`ValidateModelCapabilities`),
`internal/config/validate.go` (`validateModelProtocol`),
`internal/config/capabilities.go` (`OpenCodeProtocolCapabilities`),
`internal/provider/opencode.go` (`doOpenCode`),
`cmd/aiproxy/configure.go` (`existingProviderInput`).

Requirements:

1. Add required `protocol` to Zen/Go rules. Enable `chat`, `responses`, and
   `messages` in this stage; keep Zen `gemini` gated until MPTX-03. Reject
   any rule `protocol` declaration on non-OpenCode types, even empty/null.
   Reuse the literal capability/protocol policy without creating a literal
   model entry or inheriting protocol defaults from a model.
2. Carry the selected rule's declared protocol from the admitted snapshot
   into the existing adapter. No inference from the model ID, operation,
   credential, upstream URL, or caller metadata. Preserve Zen/Go endpoints,
   header/session behavior and configured auth. MPTX-01's native selector
   guard also applies to OpenCode Messages rule calls.
3. Preserve explicit declarations through HCL/JSON/inline loading, `convert`,
   and unrelated CLI edits (or fail without writing when preservation is
   impossible).

Acceptance criteria:

- Tests cover both services and each of the three body-based protocols,
  their supported JSON/SSE routes and rejection of unsupported operations
  before I/O. The same ID can route differently on different providers; a
  Go quota response never tries Zen or another protocol. A literal with
  a different protocol still wins.
- Missing/empty/null/unknown protocol, `gemini` on Go, and incompatible
  capabilities fail candidate validation. Conversion and CLI tests retain
  protocol intent and grants, including on a rule-only provider.

Verification: `go test ./internal/config ./internal/configedit ./internal/modelresolver ./internal/httpapi ./internal/provider ./cmd/aiproxy`.

Completion evidence:

- Added immutable `ModelPassthroughRule.Protocol()` and shared the literal
  provider/protocol capability checks. Direct rule dispatch uses the
  rule's protocol; literal dispatch is unchanged. Zen Gemini remains gated
  until the next stage. Non-OpenCode configurations reject rule `protocol`
  even when empty or null.
- Loopback tests covered both services and all five body-based
  operation/protocol pairs in JSON/SSE, literal priority, selector denial,
  and a Go quota response with no fallback. Config and CLI tests covered
  HCL/JSON/inline, conversion, preservation and unsafe-edit rejection.
- The six-package suite, focused race tests, `make vet test`, and formatting
  and whitespace checks passed. The coordinator reviewed dispatch and
  validation and independently reran the focused race suites. No live-provider,
  DB-backed or binary checks ran at this stage.

### Task MPTX-03: Bind Gemini-style rules to a single path segment

Status: completed

Kind: improvement

Priority: P1 — admits Gemini rules without letting IDs select a different URL or action

Dependencies: MPTX-02

Primary ownership: `internal/config/model_passthrough.go`, `internal/httpapi/model_passthrough.go`,
`internal/provider/gemini.go`, `internal/provider/opencode.go`, and wire-path tests.

Finding: Gemini and Zen `gemini` use the model to construct the request path,
not just a JSON field. The current matcher accepts slash-separated suffixes.
The current Gemini batch translator generates `requests[]` without a `model`
member; the public schema documents each nested model as matching the outer
model. Existing loopback tests assert the path and content, not that member.

References: `internal/config/helpers.go` (`IsLowercaseName`),
`internal/provider/gemini.go` (`doGeminiChat`, `doGeminiResponses`,
`doGeminiEmbeddings`, `translateOpenAIEmbeddingsToGemini`),
`internal/provider/opencode.go` (`newOpenCodeGeminiRequest`),
`internal/provider/provider_test.go` (`TestGeminiAdapterEmbeddingsBatch`), and
the [Gemini v1beta REST schema](https://generativelanguage.googleapis.com/$discovery/rest?version=v1beta)
(`BatchEmbedContentsRequest`, `EmbedContentRequest`).

Requirements:

1. Enable `gemini` and Zen's explicit `protocol = "gemini"` only with a
   bounded bare ID (`[a-z0-9][a-z0-9._-]*`). Enforce that after identifying the
   unique regex winner and before any upstream I/O. A broad regex cannot allow
   path separators, full resource names, encoded separators, dot segments,
   query/fragment or method delimiters. Do not strip `models/` or let a
   different rule win when the unique winner is unusable.
2. Use the configured destination and fixed adapter path/action. Permit
   configured base path prefixes, but reject a base override with a query or
   fragment delimiter (even with an empty value) when it enables path-based
   rules. Leave literal-only base URL validation and transport policy
   unchanged. Only the adapter adds `alt=sse`; the model cannot affect the
   destination or other URL components.
3. Reuse the existing single and batch embeddings translation for direct
   Gemini. For rule batches, fill every generated `requests[].model` with
   `models/<admitted-id>` from the same value used in the path, never a
   caller-provided subrequest override. Keep literal/alias payload behavior;
   do not add async batching, Zen embeddings, or other protocols.

Acceptance criteria:

- Mock HTTP asserts Gemini and Zen path prefixes, host, fixed action and
  query for translated chat/Responses JSON and SSE; Gemini embeddings
  additionally assert single/batch paths and the exact batch item model.
  A strict fixture verifies the nested wire requirement, without inferring
  live provider compatibility.
- With `match = ".*"`, invalid or multi-segment Gemini IDs make zero
  upstream calls and keep literal miss behavior. Body-based rules still
  accept valid slash-separated IDs. Exact overrides and ambiguous matches
  across body and path protocols preserve the shared routing contract.

Verification: `go test ./internal/config ./internal/modelresolver ./internal/httpapi ./internal/provider`.

Completion evidence:

- Enabled direct Gemini and Zen `gemini` rule matching. The shared matcher
  counts overlapping regexes before validating the selected rule's bare ID;
  the adapter checks the same rule-specific path constraints before building
  a request. Candidate, conversion, and edit validation reject query/fragment
  delimiters only for providers with path-based rules.
- Loopback fixtures verified fixed paths/actions and SSE query, early
  rejection under broad regexes, mixed Zen overlaps, single and batch
  embeddings, and stable usage labels. Rule batches set each item to
  `models/<id>`; literal and alias batches retain their original body.
- The four-package suite, targeted race tests, `make vet test`, and
  whitespace checks passed. The coordinator reviewed the path and payload
  changes and reran the four-package suite with `-count=1`. No live or
  DB-backed checks ran in this stage.

### Task MPTX-04: Verify cross-provider behavior and publish the configuration contract

Status: completed

Kind: improvement

Priority: P1 — validates the integrated feature and removes the old provider-only limitation from docs

Dependencies: MPTX-03

Primary ownership: config docs fixtures, `internal/integration`, and
`website/docs/configuration.md`, `website/docs/providers-and-routing.md`,
`website/docs/api-reference.md`, `website/docs/operations.md`, `docs/design.md`,
`AGENTS.md`, and `CHANGELOG.md`.

Finding: the current docs restrict the feature to the four OpenAI-adapter
types. Existing binary verification pins regex and grant reloads for an
OpenAI-compatible provider, not a change to a rule's protocol.

References: `internal/config/model_passthrough_docs_test.go`
(`TestDocumentedModelPassthroughPublishedExamples`),
`internal/integration/model_passthrough_binary_test.go`
(`TestBinaryModelPassthroughSIGHUPUpdateAndRollback`),
`internal/app/model_passthrough_test.go` (`TestAppModelPassthroughWithMergedWorkspaceKeys`).

Requirements:

1. Add exact-tested HCL/JSON examples, including OpenCode rule protocol and
   Gemini bare IDs, and an Unreleased note. Document every provider's allowed
   combinations, no new operations/support-matrix rows, cost and discovery
   limits, and mock-only Copilot verification. Do not run a release.
2. Extend hermetic verification for a newly accepted ID without config edits
   or discovery. Exercise successful reload of rule protocol/regex/grant
   together, invalid-candidate rollback, and completion of an admitted
   stream with the old protocol. Reuse existing handler and binary fixtures.

Acceptance criteria:

- Published examples parse and match their documented behavior. No new
  model cards, alias/deferred targets or managed grants. Existing v1, static
  versus DB key, workspace and bounded-usage regressions remain green; add
  a representative new-adapter regression where existing coverage is generic.
- At least one binary fixture observes a changed protocol on new requests
  without changing the protocol of work already admitted; an invalid
  protocol/rule/grant candidate makes no switch and no rejected-upstream call.
  No live credentials are required or used.

Verification: shared final checks below and `go test ./internal/config ./internal/app`.

Completion evidence:

- Added tested HCL/JSON examples and updated the public configuration, API,
  routing and operations docs, `docs/design.md`, `AGENTS.md`, and the
  Unreleased changelog. No public capability or endpoint matrix changed.
- `TestBinaryOpenCodePassthroughProtocolReload` observed a new ID without
  edits or discovery, a Chat-to-Gemini wire-path switch with protocol, regex
  and grants published together, and rollback after an invalid protocol
  candidate. The admitted Chat stream finished normally. Literal/alias
  inventory and permissions were unchanged, and two Gemini IDs shared one
  unpriced usage row. The coordinator reran this binary scenario five times
  with the race detector and reran the exact documentation fixtures.
- Root config/app tests, `make vet test`, `make test-race`, `make integration`,
  `make docs-contract`, `pnpm docs:build`, and Go formatting/whitespace checks
  passed on Go 1.27.4. An initial overlapping race and integration run hit a
  loopback port collision; both suites passed when run serially. The existing
  merged-workspace regression also passed on disposable PostgreSQL 17.11.
- No live provider or issuer was contacted; Copilot remains mock-only. No
  release or commit was made.

## Shared verification and definition of done

Run the staged package checks from the repo root with the pinned Go toolchain.
Final integration: `make vet test`, `make test-race`, `make integration`,
`make docs-contract`, `pnpm docs:build`, and `git diff --check`. Use Helm for
NetworkPolicy tests and the pinned Node/pnpm with workspace dependencies for
binary/website builds. Serialize binary builds that write `dist/` or embedded
web assets. Use disposable loopback PostgreSQL only for any DB-backed checks;
record explicitly when those optional checks are not run.

All five provider types work through their existing adapters and policy
limits, with captures for both body and URL targets. Record each task's
verification before completion. If implementation is delegated, the final
reviewer should be separate from the routing implementer. Live account/model
availability, larger resource names, and changes to existing literal
translation are outside this plan.
