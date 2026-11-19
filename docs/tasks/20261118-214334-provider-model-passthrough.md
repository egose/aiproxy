# Provider model-ID passthrough

Created: 2026-11-18 21:43:34 (local)

Status: completed; MPT-01 through MPT-04 ran sequentially in separate subagent
sessions, with coordinator review and verification between stages.

## Objective

Let a client that already knows an upstream model ID call it as
`<provider>/<upstream-model-id>` without adding a literal `model` block for
each new ID. The provider's operator opts in by declaring one or more named
`model_passthrough` rules. A rule admits a family of IDs; it does not rename
them or establish that the upstream actually offers them.

### Configuration

```hcl
listener "http" "public" {
  address = "127.0.0.1:8080"
}

provider "openai-compatible" "router" {
  base_url = "https://router.example/v1"
  api_key  = env("ROUTER_API_KEY")

  model "qwen/qwen3-32b" {
    upstream_name = "pinned-chat"
    capabilities  = ["chat"]
  }

  model_passthrough "qwen3-chat" {
    match        = "^qwen/qwen3-[0-9]+b(-[a-z0-9-]+)?$"
    capabilities = ["chat"]
  }

  model_passthrough "embeddings" {
    match        = "^openai/text-embedding-3-[a-z0-9-]+$"
    capabilities = ["embeddings"]
  }
}

auth "main" {
  mode = "bearer_static"
  client "chat-app" {
    token                      = env("CHAT_APP_TOKEN")
    allowed_models             = ["router/qwen/qwen3-32b"]
    allowed_model_passthroughs = ["router/qwen3-chat"]
  }
}
```

`router/qwen/qwen3-32b` uses the literal definition and sends `pinned-chat`.
`router/qwen/qwen3-72b` has no literal definition and would send
`qwen/qwen3-72b` unchanged. `qwen3-chat` names the rule for grants and
diagnostics; it does not itself register a callable model. The embedding family
uses the same provider URL and credential but requires a separate grant.
Families with identical policy may share a regex by using alternation.

### v1 scope

- Direct ordinary inference on concrete, file-configured providers sharing the
  OpenAI adapter: `openai`, `openai-compatible`, `openrouter`, and `zenmux`.
  Existing provider capabilities still apply. A provider can have both literal
  models and rules, or rules only. Rules are not inherited or locally declared
  by derived providers.
- `match` is a Go regexp applied to the **entire** suffix after the first `/`.
  `.*` is the regex wildcard; bare `*` is invalid, and `^.+$` is an intentional
  catch-all for valid nonempty IDs. No glob autodetection, typo matching,
  capture-based rewriting, or regex in `model` labels.
- Exact catalog hits always win, even if denied, unsupported for the operation,
  or rejected upstream. On an exact miss, only the named provider's rules
  participate. Zero matches keep the existing miss/authorization behavior;
  multiple matches fail closed before upstream I/O, irrespective of order,
  operation, or which rules the caller is granted.
- A static bearer needs a separate named grant, even when `allowed_models`
  is omitted or empty. Grants do not affect access to literal names:
  `allowed_models` remains an exact-name policy, and omitting it still allows
  all in-scope literal models. A rule on a file-only `auth.mode = "none"`
  instance is an explicit anonymous opt-in. Multi-tenancy keeps its existing
  inference authentication, workspace and freshness rules; v1 has no DB-key
  grants, including for System keys.
- No discovery or automatic registration; `/v1/models`, alias targets,
  inheritance overlays, and peer deferred admission stay literal-only. No
  managed provider/credential/DB-key creation fields, schema migration, or new
  CLI/web authoring controls. Existing CLI edits must preserve declarations.
- V1 uses the current public model segment grammar, including namespaced IDs
  but excluding `:free`-style suffixes and uppercase IDs. Those can still be
  exposed through a literal model with `upstream_name`. Broader ID syntax,
  adapters with per-model protocol selection, discovery, and priced rule
  families are later scope if needed.

## Analysis and verification baseline

Source inspection at `2c8ae0d`; worktree was clean before creating this plan.
Focused searches found no existing task for regex passthrough. Preserve the
[workspace isolation contract](20261117-181649-multi-tenancy-workspace-isolation.md)
and [peer deferred boundaries](20261116-090334-caller-model-level-deferred-transport.md).
Inspected runtime config and conversion, direct/alias resolution, HTTP admission,
OpenAI adapter dispatch, and accounting. No baseline Go tests, live upstreams,
or DB deployment tests were run for this document. Planning checks passed from
the repo root with only this document added: `make docs-contract`, local
references, task dependencies, and whitespace. Commands below are for
implementation, from the repository root with the repo toolchain.

Implementation preflight: Go 1.27.4, Node 26.13.0, and pnpm 11.24.0
match the repo pins. `make check-toolchain`, `pnpm docs:build`,
`make web-build`, `go test -race ./internal/accounting`, and
`go test -race ./internal/identity` passed before their affected stages.
The workspace command `pnpm docs:typecheck` failed at baseline with duplicate
React `Ref` types in two unchanged `website/src/theme` TSX components. A
standalone copy of `website/` using its own frozen lockfile (CI layout)
installed and typechecked successfully. No website dependency changes are
needed for this feature.
An isolated `2c8ae0d` checkout passed `make integration WEB_SKIP=1` with
freshly built web assets, plus `make vet test` and `make test-race` with
`ASDF_HELM_VERSION=4.3.2`. Without the Helm override the asdf shim had no
selected version in that checkout and the unchanged Helm tests failed.
The CI-style serialized race suites for `internal/store`, `internal/dbmerge`,
`internal/httpapi`, and `internal/app` also passed on that baseline against
a disposable loopback PostgreSQL 17 fixture.

P1 means required to ship this improvement; these are not production defects.
Execution order: MPT-01 → MPT-02 → MPT-03 → MPT-04. Completion evidence is
recorded under each task.

## Tasks

### Task MPT-01: Load and preserve named provider rules and client grants

Status: completed

Kind: improvement

Priority: P1 — establishes the config and validation contract

Dependencies: none

Primary ownership: `internal/config`, `internal/dbmerge`, `internal/configedit`,
`cmd/aiproxy/configure.go`; `internal/httpapi` managed write validation only
where necessary.

Finding: `rawProvider` and `rawClient` have no rule or grant fields.
`validateProviders` requires at least one literal model, and
`buildDerivedProvider` and `dbmerge.MaterializeProvider` copy their base.
`jsonBlockLabels` and `existingProviderInput` need to recognize or preserve
new declarations so conversion and unrelated CLI edits do not drop them.

References: `internal/config/schema.go` (`rawClient`, `rawProvider`),
`internal/config/validate.go` (`validateProviders`), `internal/config/catalog.go`
(`cloneProvider`), `internal/config/convert.go` (`jsonBlockLabels`),
`internal/dbmerge/dbmerge.go` (`MaterializeProvider`),
`cmd/aiproxy/configure.go` (`existingProviderInput`).

Requirements:

1. Parse repeatable, provider-local rule labels using the provider name
   grammar. Validate unique labels, static-client `<provider>/<rule>` grant
   references, required nonempty `match`/`capabilities`, and provider
   capability support. Validate disabled declarations without activating
   them; a grant to a declared disabled provider does not prevent disabling it.
2. Bound matching work (32 rules per provider, 512 bytes per
   regex, 256 bytes per input suffix). Compile immutable full-match regexes
   once during candidate validation. Do not store a rule or any matched ID in
   the literal model list/index. Ensure full matching also with alternation
   and inline flags.
3. Support HCL/JSON (including inline config), conversion in both directions,
   snapshot cloning, and in-place provider/client edit preservation. Where an
   existing edit cannot safely preserve declarations, fail without rewriting.
   Reject rule or grant declarations on managed write surfaces. Static and
   managed derivatives must not acquire rules from a base; retain the
   inherited literal inventory and existing credential constraints. A
   derivative of a rule-only base is invalid because it has no literal models.

Acceptance criteria:

- The sample validates, including a rule-only variant; HCL/JSON round trips and
  unrelated CLI edits preserve matching behavior and grant values.
- Invalid regex (including bare `*`), duplicate labels, missing/invalid
  capabilities, bad grants, and values beyond the selected bounds fail
  validation. Managed/derived opt-ins fail; a derivative of a mixed base still
  serves only the inherited literal inventory.

Verification: `go test ./internal/config ./internal/configedit ./internal/dbmerge ./internal/httpapi ./cmd/aiproxy`.

Completion evidence:

- Changed: config declarations, full-match compilation, immutable snapshots,
  HCL/JSON conversion, CLI/configedit preservation, managed payload validation,
  and static/managed inheritance.
- Handoff: `config.Provider.MatchModelPassthrough` selects a unique compiled
  rule or returns no match/ambiguity. `config.Client.AllowedModelPassthroughs`
  stores static grants. Exact priority, workspace, and caller checks are MPT-02.
- Verified: the five-package command passed with `-count=1`; config/configedit/
  dbmerge race suites and focused CLI race tests passed. PostgreSQL 17 fixture
  checks passed for managed writes, Copilot setup, and derived inventory. Review
  narrowed validation to endpoints that already accept mutation payloads;
  unrelated bodyless actions still ignore bodies. `git diff --check` passed.

### Task MPT-02: Resolve and authorize fallback only on ordinary exact misses

Status: completed

Kind: improvement

Priority: P1 — delivers the requested routing without widening access

Dependencies: MPT-01

Primary ownership: `internal/modelresolver`, `internal/auth`,
`internal/httpapi` admission and direct dispatch; focused adapter/HTTP tests.

Finding: `Resolve` performs a literal `Catalog.Model` lookup. HTTP admission
checks the literal allowlist and workspace visibility before resolution.
`dispatchDirect` repeats the literal workspace check and passes
`Model.UpstreamName` to the adapter. The same resolution entry point is used by
deferred capabilities and submit, so making all lookups pattern-aware would
extend those surfaces too.

References: `internal/modelresolver/resolve.go` (`Resolve`, `ResolveInWorkspace`),
`internal/httpapi/admission.go` (`checkInferenceModel`, `resolveInferenceModel`),
`internal/httpapi/dispatch.go` (`dispatchDirect`),
`internal/httpapi/deferred_http.go` (`handleDeferred`),
`internal/httpapi/models.go` (`buildModelCatalog`),
`internal/provider/openai.go` (`doOpenAI`).

Requirements:

1. For ordinary direct inference, check the exact catalog first. Only on
   absence, check the enabled provider and its workspace before matching the
   unmodified, bounded, valid suffix. Resolve a single family rule; authorize
   the named grant independently of literal allowlists. Do not let a grant
   supersede a denied exact model, stale key, or workspace restriction.
   If fallback cannot admit the ID, use the existing response for the same
   literal miss (`403` if excluded by the literal allowlist; otherwise `404`).
   Do not disclose hidden rules or change auth/freshness errors.
2. Carry the bounded rule identity separately from the requested suffix.
   Dispatch via the existing direct adapter with the provider's URL,
   credentials, and headers. Preserve ingress guardrails and the effective
   provider/root reasoning policy. Put the suffix unchanged in the upstream
   `model` field. Enforce rule capabilities before I/O. Do not switch
   models/providers or use peer transport for rule matches, even with a
   provider deferred default; preserve ordinary same-target reasoning recovery.
   Keep the existing literal request parsing behavior. For fallback, do not
   trim multipart IDs into valid matches or let a second model field compete
   with the selected suffix at the upstream. Reject the known auxiliary
   selectors `models` and `preset` on rule matches only; arbitrary behavior
   of the trusted upstream (including virtual model IDs) is not a proxy
   authorization or cost guarantee.
3. Keep listing, aliases, and peer deferred checks on literal lookup. Reject
   ambiguous matches rather than using declaration order or authorization to
   break ties. Log the operator diagnostic without including patterns in
   client errors.

Acceptance criteria:

- Mock upstream sees the exact namespaced suffix. A new matching ID works when
  the mock enables it, without a config change; an upstream model-not-found
  before that is returned as-is. A literal override still rewrites `model`.
  JSON and SSE use the same admission; test each exposed operation, including
  multipart audio transcription.
- Exact denied/unsupported/upstream-error cases never switch to a rule.
  Unmatched or ambiguous IDs, bad suffixes, out-of-scope and ungranted clients,
  DB keys, disabled providers, unsupported operations, and deferred requests
  for a rule-only ID cannot dispatch upstream via a rule. An ID or rule name
  in `allowed_models` does not substitute for a rule grant.
- Neither a rule label nor a matched ID becomes a model card, a valid alias
  target, or a peer receiver model without its own literal declaration.
- Top-level alternate model selectors are denied before upstream I/O on rule
  matches; literal and alias requests keep their existing payload handling.

Verification: `go test ./internal/auth ./internal/modelresolver ./internal/httpapi ./internal/provider`.

Completion evidence:

- Changed: `internal/auth/auth.go`, `internal/config/catalog.go`,
  `internal/modelresolver/resolve.go`, and HTTP admission/direct dispatch.
  Literal lookups remain separate; a fallback result carries its rule and raw
  upstream suffix separately. The grant shares the runtime authorizer and
  survives key-only refresh without being granted to DB keys.
- Verified: four-package tests and focused race tests passed. Loopback
  upstreams exercised all six operations and four eligible providers, SSE,
  multipart, exact overrides, scope and stale-key denial, guardrails, ordinary
  same-target recovery, and literal-only aliases/listing/deferred paths. A
  follow-up rejected `models` and `preset` on rule dispatch, including after
  guardrail rewrites, without changing literal/alias bodies. `git diff --check`
  passed. The existing workspace inference and serving-reconciliation app
  race regressions also passed against disposable PostgreSQL 17.
- The resolver's literal path now avoids an extra provider clone on hits and
  any catalog clone on misses. `BenchmarkResolveDirectHit/1000` allocations
  dropped from 4024 to 2012 per operation; `BenchmarkResolveUnknownModel/1000`
  from 4024 to 1 in the same local 150 ms benchmark.

### Task MPT-03: Group usage and metrics by configured rule identity

Status: completed

Kind: improvement

Priority: P1 — avoids caller-controlled aggregation and misleading prices

Dependencies: MPT-02

Primary ownership: `internal/httpapi` usage and dispatch attribution;
`internal/accounting` and `internal/observability` only as needed.

Finding: ordinary inference uses the requested name for `AccountingModel` and
the resolved model for upstream aggregation and provider-selection metrics.
Feeding unregistered IDs into either dimension would create a new group for
each caller-supplied ID. Billing prices currently come from literal inventory.

References: `internal/httpapi/admission.go` (`admitInferenceBody`),
`internal/httpapi/handler.go` (`ServeHTTP`, `billingPrices`),
`internal/httpapi/dispatch.go` (`dispatchDirect`),
`internal/accounting/accounting.go` (`Event`, `Aggregator.Record`),
`internal/observability/metrics.go` (`RecordProviderSelection`).

Requirements: Use a stable, non-callable, collision-free rule identity (for
example `router/@passthrough/qwen3-chat`, with `@passthrough/qwen3-chat` as
the provider-local target) for model dimensions in usage, billing, and
metrics. Keep the actual requested ID only in bounded request diagnostics
where appropriate. Preserve credential/workspace filters, rate limiting,
and literal pricing. Report rule usage as unpriced in v1; DB quota behavior
is unchanged because DB keys cannot use rules.

Acceptance criteria: Many distinct matching IDs under a single rule produce
one aggregate model/target identity per existing scope, not one per ID.
Separate rules and literal models remain distinguishable. Billing reports
tokens without a fabricated cost; credential/workspace filters still hold.

Verification: `go test ./internal/accounting ./internal/observability ./internal/httpapi`.

Completion evidence:

- `internal/httpapi/model_passthrough_accounting_test.go` verifies the
  attribution added with routing; no aggregator, metrics, or quota code
  changes were necessary. Sixty-four JSON IDs and a completed SSE request
  share one rule/target identity with 660 tokens and no new labels per ID.
  A second rule stays separate; the literal still prices correctly even when
  it sends an upstream ID also reached by the rule.
- Denials used the existing fixed reason groups with zero target series.
  Bounded request diagnostics retained correlation. The credential rate
  limiter grouped different IDs, billing isolated same-name/same-tenant
  static and DB identities, and a DB-key miss did not enter quota/ledger
  paths. The real quota tracker with a fake store still priced and limited
  the literal. The three-package verification passed with `-count=1`;
  focused race tests and whitespace checks passed. After routing/accounting,
  the serialized race suites for `internal/store`, `internal/dbmerge`, and
  `internal/httpapi` passed with disposable PostgreSQL 17 as well.

### Task MPT-04: Document and verify the integrated feature

Status: completed

Kind: improvement

Priority: P1 — closes the public contract and proves reload safety

Dependencies: MPT-03

Primary ownership: `website/docs/configuration.md`,
`website/docs/providers-and-routing.md`, `website/docs/api-reference.md`,
`website/docs/operations.md`, `docs/design.md`, `AGENTS.md`, `CHANGELOG.md`,
and config/app and binary integration fixtures.

Finding: the public docs describe concrete direct resolution, exact
allowlists, and catalog-only discovery. Passthrough needs a distinct documented
contract, including the effect of a broad regex on future upstream IDs and costs.

References: `website/docs/providers-and-routing.md` (Direct Routing),
`website/docs/api-reference.md` (Model Access With Multi-tenancy),
`internal/integration/encrypted_reasoning_binary_test.go`
(`TestBinaryEncryptedReasoningSIGHUPUpdateAndRollback`).

Requirements: Publish validated HCL/JSON examples, eligibility, regex syntax and
bounds, grant and miss/overlap semantics, discovery and pricing limitations,
and prepare a release note without executing a release. Verify a valid SIGHUP
activates rules and grants together, an invalid candidate retains the old
snapshot, and an already-admitted stream continues on its pinned runtime.

Acceptance criteria: A hermetic binary scenario proves two IDs can route via
one rule without editing config or calling model discovery, then verifies
rule/grant updates and invalid-regex rollback while an admitted stream
finishes under its original runtime. Literal and alias scenarios retain
existing behavior. Published examples parse and match the verified contract;
no live provider account is required. With local PostgreSQL available, verify
the merged System catalog serves a static client's grant but does not grant
that rule to DB keys.

Verification: `go test ./internal/config ./internal/app`, `make integration`
(Go and pnpm workspace dependencies), then `make vet test`,
`make docs-contract`, `pnpm docs:build`, standalone website `pnpm typecheck`
with its frozen lockfile, and `git diff --check`.

Completion evidence:

- Published HCL/JSON examples in `website/docs/configuration.md`; the
  `TestDocumentedModelPassthroughPublishedExamples` fixture loads the
  exact snippets. Updated routing, API, operations, design, and agent docs,
  plus the Unreleased feature note. No release or commit was made.
- `TestBinaryModelPassthroughSIGHUPUpdateAndRollback` verified a new ID
  becoming available with no edits/discovery, literal and alias priority,
  non-enumeration, valid rule/grant activation, invalid regex rollback with
  zero rejected-upstream calls, and completion of the original SSE stream.
- `TestAppModelPassthroughWithMergedWorkspaceKeys` passed under the race
  detector against disposable loopback PostgreSQL 17; the merged static
  System grant worked, System/tenant DB keys could not use it, and allowed
  literal calls from both keys still worked.
- From the repo root, config/app tests, `make integration`, `make vet test`,
  `make test-race` (Helm 4.3.2), `make docs-contract`, `pnpm docs:build`,
  Go formatting, and diff whitespace checks passed. The coordinator
  repeated the binary test 10 times under `go test -race -tags=integration`
  and reran the PostgreSQL app test. Standalone website typecheck/build
  passed; workspace `pnpm docs:typecheck` still shows the baseline React
  type conflicts. Live upstream provider compatibility was not tested.

## Definition of done

All four tasks met their acceptance criteria and recorded verification.
Regressions cover exact priority, authorization, workspace isolation,
non-enumeration, bounded aggregation, and reload. Scope exclusions are
documented.

Follow-up: [extend model-ID passthrough to the remaining provider types](20261119-105050-expand-model-passthrough-provider-support.md).
