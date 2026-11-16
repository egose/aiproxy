# Caller model-level `deferred_transport`

Created: 2026-11-16 09:03:34 PST

Status: completed; DTC-01 through DTC-06 meet their acceptance criteria.
Implementation and independent reviews ran sequentially in separate subagent
sessions. Compatibility/inheritance decisions were approved before coding.
Optional database-backed durability and live-provider compatibility remain
unverified; required hermetic and shared release checks passed.

## Objective and required configuration boundary

**Only the caller's `deferred_transport` moves to model level.
`deferred_receiver` stays provider-level.**

Allow two models under one caller provider, sharing the same peer endpoint and
credential, to use different transports:

```text
client → caller provider a / model-1 → executor provider b / model-1: ordinary
client → caller provider a / model-2 → executor provider b / model-2: deferred
```

The executor's receiver opt-in remains additive: provider b can accept deferred
jobs for its chat-capable models and still serve ordinary requests for those
same models. It does not force either model to use deferred transport.

### Intended configuration example

These original planned post-change snippets are now supported by the implemented
caller schema. Listener/auth blocks are omitted.

Caller:

```hcl
provider "openai-compatible" "a" {
  base_url = "https://peer.example/v1"
  api_key  = env("PEER_TOKEN")

  model "model-1" {
    upstream_name = "b/model-1"
  }

  model "model-2" {
    upstream_name = "b/model-2"
    deferred_transport {
      max_wait      = "20m"
      poll_interval = "500ms"
    }
  }
}
```

Executor:

```hcl
deferred {}

provider "openai-compatible" "b" {
  base_url = "https://upstream.example/v1"
  api_key  = env("UPSTREAM_KEY")
  deferred_receiver {}

  model "model-1" {}
  model "model-2" {}
}
```

The executor still requires `auth.mode = "bearer_static"` and a peer client
credential. A caller-only instance does not require root `deferred`.

## Scope and preservation requirements

1. A caller model's opt-in determines chat-completions transport for that
   resolved model/alias target, not for every model sharing its provider.
   With no model opt-in and no retained compatibility default, use ordinary
   transport. An empty model block opts in using the documented defaults.
2. Preserve chat JSON and SSE behavior. Non-chat operations, including Responses
   and embeddings on an opted-in model, remain ordinary.
3. Preserve static `openai-compatible` caller restrictions, configured
   `api_key`/`api_key_ref` requirements even when disabled, duration/count bounds,
   base-URL validation, and exclusion of `forward_authorization`.
4. Preserve the prohibition on combining receiver configuration with caller
   deferral on the same provider. Moving caller fields must not enable recursive
   peer execution or bypass receiver target checks.
5. Caller alias pools may still mix ordinary and deferred targets. Selection
   uses each target model's effective transport; accepted/indeterminate attempts,
   committed streams, peer-control errors, health, cooldown, and inference
   failover retain the existing v1 rules.
6. HCL, JSON, inline/env loading, conversion, CLI edit preservation, catalog
   cloning, static inheritance, and reload must agree about effective transport.
   Managed/database provider and credential writes remain static-only boundaries.
7. No model-level `deferred_receiver`, receiver tuning changes, root changes,
   wire-version/schema changes, new operations, SSE peer-control transport,
   persistence, queues, or extra CLI/web creation controls.
8. Existing provider-level caller configs must never be silently ignored.
   Their compatibility/migration policy is a decision in DTC-01, not an
   assumption made by the implementer.

Receiver configuration is unchanged, but defensive code that currently checks
`Provider.DeferredTransport` may need to recognize model-level caller settings.
That is guard alignment, not moving receiver eligibility to model configuration.

## Analysis coverage and related work

Source baseline: `72f97ef`; working tree was clean before task creation.

| Inspected boundary           | Current evidence and implementation implication                                                                                                                                                                                                                                                                                                                     |
| ---------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Schema/types                 | `internal/config/schema.go` (`rawProvider`, `rawModel`) and `internal/config/types.go` (`Provider`): both deferred blocks currently belong to the provider; model transport needs a schema/runtime home.                                                                                                                                                            |
| Declaration/build validation | `internal/config/deferred.go` (`validateDeferredDeclarations`, `validateDeferredConfigBody`, `buildProviderDeferred`): inspection and early-return logic currently only discover provider/root blocks. Nested declarations must not bypass validation.                                                                                                              |
| Inheritance                  | `internal/config/build.go` (`buildDerivedProvider`, `validateDerivedProviderSurface`): derived providers clear opt-ins and cannot declare model blocks. Per-model local opt-ins need an explicit inheritance contract.                                                                                                                                              |
| Catalog ownership            | `internal/config/catalog.go` (`cloneProvider`, `cloneModel`): provider options are deep-copied; any new model pointer must also be copied in both model slices and indexes.                                                                                                                                                                                         |
| Caller dispatch              | `internal/httpapi/dispatch.go` (`dispatchDirect`, `dispatchAlias`) passes provider-level options in both paths; `internal/provider/openai.go` selects deferral from `Request.DeferredTransport` only for chat.                                                                                                                                                      |
| Receiver guards              | `internal/httpapi/deferred_http.go` (`deferredReceiverLimits`) rejects caller-opted targets before receiver selection; preserve this check with the new caller representation.                                                                                                                                                                                      |
| Editing                      | `internal/configedit/configedit.go` (`ProviderModelInput`, `RenderProviderBlock`) and `cmd/aiproxy/configure.go` (`existingProviderInput`) currently preserve provider-level blocks only. Disabled-provider credential preservation also checks the provider caller block.                                                                                          |
| Managed boundary             | `internal/httpapi/admin_providers.go` (`adminProviderUpsert.UnmarshalJSON`, `adminModelUpsert`) detects deferred declarations at provider level; `internal/config/deferred.go` (`ValidateManagedProviderDeferred`) and `internal/dbmerge/dbmerge.go` (`BuildProvider`) inspect provider opt-ins. Extend enforcement to nested model declarations/inherited options. |
| Regression/documentation     | `internal/config/deferred_docs_test.go` checks exact public examples; `internal/e2e/deferred_chain_test.go` and `internal/integration/deferred_binary_test.go` cover two-proxy delivery and caller SIGHUP pinning. Add same-provider mixed-model cases.                                                                                                             |

Related completed phase:
[long-wait upstream resilience](20261114-104925-long-wait-upstream-resilience.md),
especially LWR-09 configuration and LWR-11 caller transport. This is a new
configuration-granularity phase, not a reopening of those completion claims.
Focused task search found no existing model-level caller task.

Coverage is scoped source inspection, not an exhaustive audit or deployment
diagnosis. No runtime baseline, live provider, optional database, or new behavior
tests were run during task creation. Existing task evidence is not fresh
verification of this enhancement.

## Execution rules

- P1 means necessary for a correct release of this requested enhancement, not
  an emergency classification.
- Order: DTC-01 → DTC-02 → DTC-03 → DTC-04 → DTC-05 → DTC-06.
  Shared config/HTTP/CLI files make sequential handoffs preferable.
- Follow `AGENTS.md`; preserve unrelated work and existing task IDs. Update
  status, decisions, remaining criteria, and actual verification evidence here.
- No agents were launched for task creation. If implementation is later
  delegated, keep one coordinator editing this task file and use a final
  reviewer who was not the main implementer.

## Tasks

### Task DTC-01: Set compatibility and local inheritance semantics

Status: completed

Kind: investigation

Priority: P1 — these choices determine parsing, effective options, and migration.

Dependencies: none

Primary ownership: this task's decision record; scoped review of
`internal/config/build.go` and `internal/config/deferred.go`.

Finding:

The requested new placement is clear, but it does not settle whether the
published provider-level caller form remains supported. Derived providers
currently inherit models while forbidding local model blocks, so local caller
opt-in cannot simply be relocated without addressing that rule.

References: `rawModel`, `buildDerivedProvider`,
`validateDerivedProviderSurface`, and `buildProviderDeferred`.

Requirements:

1. Obtain a maintainer decision: retain provider-level `deferred_transport` as
   a compatibility default, or reject it with a clear model-level migration
   diagnostic. Recommended: retain compatibility defaults to avoid breaking
   published configs, while new examples use model opt-ins.
2. If defaults remain, define model/provider precedence and whether a partial
   model block replaces the provider options or inherits unspecified fields.
   Do not introduce an `enabled`/opt-out attribute without separate approval.
3. Define local per-model opt-in for static derivatives without implicitly
   inheriting base caller opt-ins or allowing arbitrary model redefinition.
   A restricted transport-only declaration or a documented concrete-provider-only
   initial scope requires an explicit decision.
4. Confirm provider-level receiver configuration, same-provider caller/receiver
   exclusion, static-only restrictions, and the unchanged peer v1 contract.

Acceptance criteria:

- Decisions record old-form handling, omission/empty/partial-block behavior,
  precedence, and static derivative behavior with examples.
- Any intentionally unsupported case has a controlled error and migration
  guidance; no silent ordinary fallback or implicit inherited caller opt-in.
- Dependent tasks are reconciled with the approved decisions before coding.

Verification: maintainer decision/evidence review, not a runtime test.

Approved decision record:

- The user selected all three recommended options during implementation kickoff:
  retain provider-level compatibility defaults, replace rather than merge model
  blocks, and allow restricted transport-only static derivative overlays.
- Effective caller options are the model's local block when present, otherwise
  the provider's local compatibility default, otherwise no caller deferral.
  Omission without a provider default is ordinary transport. An empty model
  block opts in with the existing documented defaults; a partial model block
  uses those defaults for unspecified fields, not the provider's values.
- There is no opt-out or `enabled` attribute. To mix ordinary and deferred chat
  models, remove the provider-wide default and declare caller blocks only on the
  intended models. Existing provider-level caller configs remain supported.
- Static derivatives do not inherit base provider or base model caller opt-ins.
  A derivative may declare an existing inherited model name with only a local
  `deferred_transport` block; new names, model redefinition fields, empty
  overlays, and duplicate overlays must produce controlled diagnostics.
  A local provider-level compatibility default remains supported on derivatives.
- `deferred_receiver` stays provider-only. Receiver configuration cannot coexist
  with any local model caller opt-in or provider caller default on that provider.
  Existing provider-type, credential, forwarding, static-only, and peer v1
  restrictions remain unchanged.

Approved derivative example (partial configuration):

```hcl
provider "openai-compatible" "derived" {
  extends = "base"
  api_key = env("DERIVED_PEER_TOKEN")
  model "model-2" {
    deferred_transport {}
  }
}
```

Here `model-2` must already exist on `base`; its model definition is inherited,
but its caller transport is selected locally. `model-1` remains ordinary unless
the derivative declares its own provider default or model opt-in.

Completion evidence:

- DTC-01 investigation session reviewed schema/build/declaration validation,
  inheritance, cloning, conversion, and CLI preservation constraints read-only
  at `72f97ef`. No additional maintainer decision is required.
- Existing caller defaults are `control_timeout = "5s"`, `max_wait = "15m"`,
  `poll_interval = "250ms"`, `max_control_calls = 4096`, and
  `max_control_retries = 8`; approved replacement semantics use these defaults.
- Implementation handoff requires presence-based derivative validation, clearing
  caller options from both inherited model views, and guard inspection of both
  `Models` and `ModelByName`. Provider compatibility defaults retain the existing
  at-least-one-chat-model rule; direct model opt-ins require that model to support
  chat.
- Verified: user approval plus scoped source/test evidence review (repo root).
  This investigation made no code changes and required no runtime test.

### Task DTC-02: Load, validate, and clone caller model opt-ins

Status: completed

Kind: improvement

Priority: P1 — model-level configuration must be valid and isolated per model.

Dependencies: DTC-01

Primary ownership: `internal/config/schema.go`, `internal/config/types.go`,
`internal/config/load.go`, `internal/config/build.go`, `internal/config/deferred.go`,
`internal/config/catalog.go`, `internal/config/convert.go`, and focused config tests.

Finding:

Only `rawProvider` currently accepts the caller block; validation discovery and
catalog cloning also assume provider storage.

References: `rawModel`, `validateDeferredDeclarations`,
`validateDeferredConfigBody`, `buildProviderDeferred`, `cloneModel`.

Requirements:

1. Add model-level caller parsing/runtime options and one consistent effective
   option lookup following DTC-01. Keep receiver fields provider-only.
2. Validate nested HCL/JSON declarations, including duplicate/labeled blocks,
   null/wrong-shape objects, unknown attributes, and range boundaries.
3. A directly opted-in model must be chat-capable and satisfy existing provider
   type, credential, auth-forwarding, and URL rules; validate disabled providers.
4. Preserve provider defaults if selected in DTC-01, without making unrelated
   models deferred when no compatibility default exists.
5. Handle static derivatives as approved; deep-copy model options and maintain
   consistent `Models`/`ModelByName` views.
6. Cover HCL ↔ JSON conversion and inline/env loading without losing placement
   or changing effective options.

Acceptance criteria:

- The planned caller example loads; model-1 has no effective caller opt-in and
  model-2 has the expected options.
- Model-level `deferred_receiver` remains rejected.
- Positive, negative, boundary, compatibility, and inheritance cases are tested.
- Mutating input/returned model options does not mutate the catalog or siblings.

Verification: `go test -count=1 ./internal/config` from repo root.

Execution: assigned to the DTC-02 implementation session after DTC-01 completion.

Handoff review:

- Initial configuration and race tests plus `make vet test` passed.
- Coordinator review found that unconditional indexed-option replacement in
  `cloneProvider` can erase a slice-level caller opt-in when the index is stale,
  or retain stale options after a slice update. The task remains in progress.
- Required correction: preserve caller presence from either representation,
  keep ordered model definitions authoritative, and make normalized policy views
  consistent and independently copied. Add resolved-lookup regressions for stale
  nil/non-nil indexes before handing off again.

Completion evidence:

- Changed schema/runtime model storage, nested declaration validation,
  credential-I/O-free standalone validation, static derivative overlays, and
  catalog cloning in the owned configuration files; added
  `internal/config/deferred_model_test.go`.
- Shared APIs are `EffectiveDeferredTransport(Provider, Model)` (copied effective
  options) and `HasCallerDeferredTransport(Provider)` (presence in provider and
  both model collections). Conversion's existing recursive traversal needed no
  production change.
- Coverage includes HCL/JSON/file/inline/env and conversion, omission/empty/
  partial precedence, declaration shapes and exact numeric bounds, disabled
  credentials, receiver exclusion, local derivative overlays and rejection by
  presence, and mutation isolation across both model views.
- Handoff correction delivered and reviewed: ordered non-nil policy wins
  wholesale; otherwise indexed caller presence is retained in both normalized
  views. Views use independent copies. Stale nil/non-nil index and resolved
  lookup regressions failed before the correction and pass afterward.
- Verified (repo root): `go test -count=1 ./internal/config`,
  `go test -race -count=1 ./internal/config`, `make vet test`, and
  `git diff --check` passed after the correction. Coordinator reviewed the
  delivered code/tests and reran `git diff --check`.
- Remaining dispatch, edit/write-boundary, documentation, and integration
  acceptance criteria are owned by DTC-03 through DTC-06.

### Task DTC-03: Select transport by resolved target model

Status: completed

Kind: improvement

Priority: P1 — direct and alias requests must honor the new granularity safely.

Dependencies: DTC-02

Primary ownership: `internal/httpapi/dispatch.go`,
`internal/httpapi/deferred_http.go`, focused HTTP/provider tests, and
`internal/app` reload tests where needed.

Finding:

Both dispatch paths currently pass provider options. Receiver guards also
identify caller deferral through the provider field, so merely changing parsing
would either fail to route model opt-ins or leave a defensive enforcement gap.

References: `dispatchDirect`, `dispatchAlias`, `deferredReceiverLimits`,
`internal/provider/openai.go` (`doOpenAI`), and
`TestBinaryDeferredCallerSIGHUPKeepsAdmittedPeer`.

Requirements:

1. Use the effective options of the resolved model for direct dispatch and
   each selected alias target; avoid mutable provider-wide overrides.
2. Keep non-chat operations ordinary and preserve prepared model rewriting,
   configured peer credentials, and all existing caller-control budgets.
3. Preserve accepted/indeterminate pinning, same-cursor recovery, terminal
   inference retry rules, stream commitment, and health/cooldown isolation.
4. Align defensive receiver checks with effective caller options; do not add
   model receiver fields or relax receiver-only executor alias eligibility.
5. New requests after reload use the changed model policy; admitted jobs/streams
   retain their original peer, credential, options, and resource ownership.

Acceptance criteria:

- One provider's ordinary model makes zero private peer calls; its deferred
  model negotiates/submits/reads without synchronous fallback.
- Both direct and alias-target paths obey the same rule for JSON and SSE.
- Non-chat requests on an opted-in model remain ordinary.
- Protocol/admission/retrieval failures cannot dispatch another inference;
  receiver guard and reload/pinning regressions pass.

Verification: `go test -count=1 ./internal/httpapi ./internal/provider ./internal/app`
and affected race checks from repo root.

Execution: assigned to the DTC-03 implementation session after DTC-02 completion.

Completion evidence:

- Direct dispatch and every resolved alias attempt now call
  `EffectiveDeferredTransport`; receiver checks reject any provider/model caller
  presence, including a separately supplied resolved model. Production changes
  are confined to `dispatch.go` and `deferred_http.go`; peer wire/store/adapter
  semantics are unchanged.
- Added `internal/httpapi/deferred_model_test.go` and
  `internal/app/deferred_model_reload_test.go`; existing broad caller regressions
  now use model opt-ins. Coverage includes same-provider mixed direct/alias
  JSON/SSE, non-chat ordinary routing, compatibility and replacement budgets,
  qualified submit/read recovery, fail-closed controls, per-target cooldown,
  receiver rejection across both representations, and admitted snapshot/resource
  ownership across failed/successful reload.
- Verified (repo root):
  `go test -count=1 ./internal/httpapi ./internal/provider ./internal/app`,
  the same packages with `-race -count=1`, `make vet test`, and
  `git diff --check` passed.
- Coordinator reviewed the delivered code/tests and independently reran the
  mixed-model routing, same-provider cooldown, and receiver-presence regressions
  with `go test -count=1 ./internal/httpapi -run 'TestDeferredCallerMixedModelsSelectDistinctTransports|TestDeferredCallerMixedModelAliasFailoverAndCooldownStayPerTarget|TestDeferredReceiverRejectsCallerModelPresenceAcrossRepresentations'`;
  passed. Binary/SIGHUP coverage remains assigned to DTC-05.

### Task DTC-04: Preserve model blocks during edits and enforce static-only writes

Status: completed

Kind: improvement

Priority: P1 — editing must not drop opt-ins or admit managed equivalents.

Dependencies: DTC-02, DTC-03

Primary ownership: `internal/configedit/configedit.go`,
`cmd/aiproxy/configure.go`, `cmd/aiproxy/configure_deferred.go`,
`internal/httpapi/admin_providers.go`, `internal/config/deferred.go`,
`internal/dbmerge/dbmerge.go`, and focused tests.

Finding:

Model editor inputs/rendering do not carry caller blocks. Existing managed
declaration detection and inheritance rejection only inspect provider opt-ins.

References: `ProviderModelInput`, `RenderProviderBlock`, `existingProviderInput`,
`adminProviderUpsert.UnmarshalJSON`, `adminModelUpsert`,
`ValidateManagedProviderDeferred`, and `BuildProvider`.

Requirements:

1. Preserve nested model blocks and supported `env()` spelling during
   unrelated interactive/non-interactive provider/model edits.
2. Disabled caller providers retain configured credentials when any effective
   model caller opt-in requires them; preserve existing CLI unsupported-workflow
   behavior rather than adding creation flags.
3. Reject managed model declarations, including empty/null/case variants,
   before persistence on applicable provider/credential update paths.
4. Reject managed inheritance of a static base with model caller opt-ins;
   inspect model collections/indexes as needed to avoid representation bypasses.
5. Do not add database columns, managed writable fields, or web controls.

Acceptance criteria:

- Editing a credential or sibling model preserves the original model opt-in,
  timing expressions, ordinary sibling, and provider-level receiver blocks.
- Nested managed declarations and opted-in managed inheritance fail before
  mutation; ordinary managed providers retain existing behavior.
- Required hermetic pre-mutation tests run without a database; optional
  database-backed skips/results are explicitly recorded.

Verification: `go test -count=1 ./internal/configedit ./cmd/aiproxy ./internal/config ./internal/httpapi ./internal/dbmerge`
from repo root; database-backed checks require `AIPROXY_TEST_DATABASE_URL`.

Execution: assigned to the DTC-04 implementation session after DTC-03 completion.
The optional database environment variable is unset; hermetic pre-mutation
checks remain required, and database-backed skips must be reported explicitly.

Completion evidence:

- Editors now carry model caller-block source, preserve `env()` timing spelling
  through credential/sibling/model edits, and retain disabled caller credentials.
  Derivative render/prompt paths preserve transport-only overlays without
  enabling inherited-field flags; explicit model deletion still works.
- Managed request inspection rejects nested model declarations by presence,
  including null/empty/case/Unicode-case/prefix variants and duplicate parent/
  model fields. Inspection is fixed-level, preserves unrelated unknown-field
  behavior, and retains existing request-byte/JSON-depth bounds.
- `ValidateManagedProviderDeferred` checks both model representations. Hermetic
  validation/admin/view/credential/dbmerge regressions reject zero, conflicting,
  and indexed-only caller options before candidate mutation, store access, or
  credential handling. No database schema or web controls changed.
- Verified (repo root): the five owned packages with `go test -count=1` and
  `go test -race -count=1`, `make vet test`, and `git diff --check` passed.
  Coordinator reviewed the code/tests and independently reran nested/duplicate
  rejection, managed model presence, dbmerge-before-credential, and CLI
  credential/derivative-preservation selections; passed.
- Optional database selection was independently rerun with `-v -count=1`:
  three `TestAdminDeferred*` persistence/role tests and two
  `TestMergeCatalogDeferred*` tests skipped because `AIPROXY_TEST_DATABASE_URL`
  is unset. Database-backed durability is unverified; required database-free
  pre-mutation coverage passed. No database was provisioned.

### Task DTC-05: Document and exercise the mixed-model two-proxy configuration

Status: completed

Kind: improvement

Priority: P1 — public examples and binary behavior must demonstrate the use case.

Dependencies: DTC-02, DTC-03, DTC-04

Primary ownership: `docs/design.md`, `website/docs/configuration.md`,
`website/docs/operations.md`, `website/docs/api-reference.md`, `AGENTS.md`,
`internal/config/deferred_docs_test.go`, `internal/e2e/deferred_chain_test.go`,
`internal/integration/deferred_binary_test.go`, and relevant fixtures/release notes.

Finding:

Current examples/tests establish provider-wide caller opt-ins, not two models
sharing one provider with different chat transports.

References: `TestDocumentedDeferredConfigurations`,
`TestBinaryDeferredCallerSIGHUPKeepsAdmittedPeer`, and the v1 configuration
section in `website/docs/configuration.md`.

Requirements:

1. Publish validated two-role examples: one caller provider with ordinary
   model-1 and deferred model-2, one executor provider with receiver opt-in.
2. Document the approved legacy/precedence/derivative rules and migration
   implications. State explicitly that receiver eligibility does not force
   ordinary requests into deferred transport.
3. Add hermetic two-app and binary regressions for direct/alias JSON/SSE mixed
   models; assert peer endpoint usage, exact delivered bytes, one inference,
   non-chat ordinary routing, fail-closed behavior, and reload pinning.
4. Keep peer v1 schema, receiver configuration, and public operation matrices
   unchanged; amend narrative references only as needed.

Acceptance criteria:

- Exact documented examples pass load/conversion/inline checks.
- Two actual proxies using the same caller provider and executor provider
  demonstrably route the models differently without leaking peer envelopes.
- Compatibility/migration and receiver-placement documentation agrees with
  implementation; release notes mention any externally visible change.

Verification: documented-config checks, focused two-app/binary tests, and
`make docs-contract` from repo root; see shared final checks.

Execution: assigned to the DTC-05 documentation/integration session after DTC-04
completion. Build/integration commands must remain serialized.

Completion evidence:

- Updated `AGENTS.md`, `CHANGELOG.md`, design/configuration/operations/API
  narratives without changing public support/capability matrices or wire/schema
  tables. Receiver placement and ordinary eligibility remain explicit.
- Exact-tested full mixed pair: `peer-model-caller.hcl` /
  `peer-model-executor.hcl`, one provider per role and two models. Added tested
  compatibility/default and local derivative snippets; migrated the existing
  full caller's block into its `chat` model while retaining the executor alias
  minimum-limit and receiver-derivative example.
- Added 14 two-app and 20 two-binary cases covering direct/alias JSON/SSE,
  exact bytes, private-header/envelope isolation, credentials/model rewriting,
  non-chat ordinary routing, fail-closed controls, and reload/SIGHUP pinning with
  old-option same-cursor recovery. Shared existing caller fixtures now use model
  placement without changing signatures/budgets.
- Verified (repo root): documented-config checks, focused two-app checks,
  config/e2e race tests, fresh `make build`, focused binary checks and binary race
  checks, `make docs-contract`, `make vet test`, `make integration`, and
  `pnpm docs:build` passed. Builds were sequential; full integration freshly
  rebuilt embedded assets/binary and passed in 103.410s.
- The documentation build emitted non-blocking update/Node warnings.
  `git diff --check` and formatting checks passed. Database/live-provider
  verification was not performed and is not claimed.
- Coordinator reviewed delivered documentation and both proxy test layers, then
  independently reran `go test -count=1 ./internal/config -run DocumentedDeferred`
  and `make docs-contract`; both passed. Final acceptance remains DTC-06.

### Task DTC-06: Independently review the caller-only scope and release readiness

Status: completed

Kind: investigation

Priority: P1 — confirm no receiver migration or peer retry regression was introduced.

Dependencies: DTC-01, DTC-02, DTC-03, DTC-04, DTC-05

Primary ownership: independent acceptance review, shared verification, and this
task's completion evidence; fixes return to the owning implementation task.

Finding:

The change spans parsing, target selection, cloning, edits, and managed guards.
A schema-only success does not prove safe runtime granularity or compatibility.

References: all acceptance criteria above and the completed v1 task's
LWR-09/LWR-11 preservation contracts.

Requirements:

1. A reviewer other than the main implementer checks every criterion against
   delivered code, public docs, and observed hermetic behavior.
2. Review nested declarations, inherited/indexed models, disabled credentials,
   receiver no-caller guards, direct/alias routing, and admitted reload state.
3. Run shared final checks; record actual results, prerequisites, skips, and
   defects. Do not mark complete on a code handoff or repeated rerun alone.

Acceptance criteria:

- No unresolved release-blocking finding; any required fix is independently
  rechecked with regression evidence.
- Receiver remains provider-only; peer wire/store semantics are unchanged.
- Shared final checks pass, or missing prerequisites keep this task blocked.

Verification: independent evidence review plus the shared checks below.

Execution: independent review follows DTC-01 through DTC-05 completion.
Reviewers do not implement fixes or edit this task file; findings return to the
owning task and require independent recheck.

Review execution note:

- A provider token-rate limit interrupted the initial DTC-06 review and its
  resume before an independent handoff. No completion or shared-check result is
  inferred from either interruption.
- To bound review context, DTC-06 now uses fresh, sequential independent scopes:
  configuration acceptance, edit/managed-write boundaries, then runtime/docs/
  release readiness. The final scope reconciles preceding independent evidence
  and executes all shared checks. All review sessions are separate from the
  implementation sessions.
- Configuration acceptance was assigned first, but the fresh bounded session
  also failed with a provider token-rate limit before delivering evidence.
- Automatic retries are stopped. DTC-06 is blocked until independent review
  capacity becomes available or the user authorizes a specific alternate model.
  All implementation changes and prior verification remain preserved, but
  independent release sign-off is not claimed.
- Alternate-model authorization was not obtained; no model switch is made.
  Review remains paused, with configuration acceptance, edit/write-boundary
  review, runtime/docs review, and independent shared-check evidence outstanding.

Previous blocker: model-provider capacity interrupted independent review.
Resolved by the successful sequential review sessions recorded below.

Resume record:

- The user requested continuation after the provider-capacity pause. No alternate
  model was selected; fresh independent review resumes with the default model,
  one bounded scope at a time. Earlier incomplete attempts are not evidence.
- At resume, the next action was independent configuration acceptance, followed
  sequentially by edit/managed-write review and runtime/docs/release readiness.

Independent configuration acceptance:

- A fresh non-implementation review session accepted DTC-01/DTC-02 against
  actual config source/tests: compatibility/replacement defaults, local static
  overlays and forbidden declaration presence, nested declaration shapes/exact
  bounds, caller eligibility/disabled credentials, provider-only receiver
  exclusion, both model representations, copy isolation, and file/inline/env/
  conversion agreement. No confirmed defect or reopening was recommended.
- Verified independently (repo root):
  `go test -count=1 ./internal/config` passed (0.810s),
  `go test -race -count=1 ./internal/config` passed (4.633s), a focused
  bounds/receiver/inheritance/JSON/conversion regression selection passed
  (0.037s), and scoped config `git diff --check` passed.
- The reviewer was read-only and left git status unchanged. Remaining review:
  DTC-04 edit/managed-write boundaries, then DTC-03/DTC-05 runtime/docs and all
  shared release checks. These remain required before DTC-06 completion.

Independent edit/managed-write acceptance:

- A second fresh non-implementation review session accepted DTC-04 against
  actual code/tests: raw block/`env()` preservation across prompt/bulk/edit
  paths, explicit deletion, disabled credentials, protected derivative overlays,
  atomic file preflight, nested/duplicate managed declarations, both model
  collections in inheritance guards, bounded inspection, and metadata
  allowlists. No confirmed defect or reopening was recommended.
- Verified independently (repo root): the five owned packages with
  `go test -count=1` and `go test -race -count=1`, focused editor/managed
  regression selections, and scoped `git diff --check` passed.
- The explicit optional database selection again skipped all five named tests
  because `AIPROXY_TEST_DATABASE_URL` is unset. Required hermetic before-store/
  before-mutation tests passed; database-backed durability remains unverified.
- The reviewer was read-only and left git status unchanged. Remaining review:
  DTC-03/DTC-05 runtime/docs and shared final release checks.

Independent runtime/documentation acceptance and completion evidence:

- A third fresh non-implementation review session accepted DTC-03/DTC-05 against
  delivered runtime code, documentation and observed hermetic behavior, and
  reconciled the preceding DTC-01/DTC-02 and DTC-04 independent reviews.
  No confirmed defect, reopening, or release-blocking finding remained.
- Verified resolved-model direct/alias dispatch, ordinary non-chat routing,
  whole-pool receiver guards, configured credentials/model rewriting, isolated
  caller budgets, accepted/indeterminate pinning, same-cursor recovery, stream
  commitment, inference retries, health/cooldown rules, and admitted reload/
  resource ownership. Receivers remain provider-only/additive; peer wire/store
  semantics and public operation/capability/API schema tables are unchanged.
- Documentation, exact-tested examples and release notes agree with the approved
  compatibility/replacement/local-overlay contract. No peer envelopes/headers,
  extra CLI/web creation controls or database schema changes were found.
- All shared checks ran independently from repo root:
  - The affected eight-package `go test -count=1` command passed uncached.
  - The affected packages plus `internal/deferred` with
    `go test -race -count=1` passed uncached.
  - `go test -count=1 ./internal/config -run DocumentedDeferred` passed uncached.
  - `make vet test` passed; its unit-test results were cached, in addition to the
    explicit uncached package and race runs above.
  - `make docs-contract` passed.
  - `make integration` freshly built embedded web assets and the binary, then
    executed the full binary suite uncached: passed in 133.041s.
  - `git diff --check` passed.
- Builds were serialized. Go `1.27.4`, pnpm `11.24.0`, Node `26.13.0`,
  dependencies and unset `WEB_SKIP` were verified; integration emitted no build
  warning or failure. Reviewers made no source/task edits, commits or delegated
  work, and preserved the original change inventory including the LWR task.
- Limitations remain explicit: all five selected optional database tests skipped
  for unset `AIPROXY_TEST_DATABASE_URL`; database-backed durability and live
  provider/deployment behavior are not verified. Required hermetic pre-mutation
  checks and all shared release checks passed, so these optional checks do not
  block the approved scope.
- Coordinator reconciled all three independent handoffs with the delivered
  changes and verification evidence before completing DTC-06.
- Final coordinator document audit (repo root) passed: six completed stable task
  IDs, ordered dependencies, required fields, local links, balanced fences,
  clean whitespace and no machine paths/private session IDs. After recording
  completion, `make docs-contract` and `git diff --check` also passed; the
  original four-line LWR backlink change remains preserved.

## Shared verification and definition of done

Run commands from repo root (`<repo-root>` means the checkout, not a literal
path). Use the repository's declared Go toolchain; build/integration also need
pnpm dependencies and the embedded web build prerequisites.

```bash
go test -count=1 ./internal/config ./internal/configedit ./cmd/aiproxy ./internal/provider ./internal/httpapi ./internal/app ./internal/dbmerge ./internal/e2e
go test -race -count=1 ./internal/config ./internal/configedit ./cmd/aiproxy ./internal/provider ./internal/httpapi ./internal/app ./internal/dbmerge ./internal/deferred ./internal/e2e
go test -count=1 ./internal/config -run DocumentedDeferred
make vet test
make docs-contract
make integration
git diff --check
```

`make integration` builds the binary and embedded web assets. Do not run it
concurrently with another build targeting the same outputs. Focused binary
checks can use a freshly built binary and the existing `integration` build tag.
Live providers/deployments are not required and must not be claimed as verified.

Done means all six tasks meet their criteria, DTC-01 decisions are settled,
ordinary/deferred models coexist on one caller provider as demonstrated,
receiver placement and v1 delivery guarantees are preserved, and final evidence
is recorded. This document's creation does not complete implementation tasks.

### Task-creation verification

- Runtime baseline and enhancement checks: not run; documentation-only request.
- Inline document validation (repo root): passed; six unique task IDs, required
  fields, valid statuses, acyclic dependencies, local links/backlink, no
  machine-specific paths/session IDs, and balanced fences/clean whitespace.
- `git diff --check` (repo root): passed.
- `make docs-contract` (repo root): passed; documentation contract matrices match.

### Implementation kickoff verification

- Source revision: `72f97ef`; no source-code changes before baseline execution.
  Existing changes to the related LWR task and this untracked task document are
  preserved.
- Toolchain (repo root): Go `1.27.4`, pnpm `11.24.0`, Node `v26.13.0`, matching
  the declared prerequisites.
- Baseline (repo root):
  `go test -count=1 ./internal/config ./internal/configedit ./cmd/aiproxy ./internal/provider ./internal/httpapi ./internal/app ./internal/dbmerge ./internal/e2e`
  passed before implementation.
- Sequential delegation is authorized. DTC-01 uses a separate investigation
  session; a failed initial session returned no handoff, so its replacement
  performs the readiness review. Full session identifiers are not recorded.
