# Long-wait upstream resilience for chained aiproxy deployments

Created: 2026-11-14 10:49:25 PST

Status: completed for the selected generic v1 feature. LWR-08, LWR-09, LWR-10,
LWR-06, LWR-11, LWR-12, and LWR-07 meet their acceptance criteria. Remaining
implementation ran through sequential subagents, with separate remediation and
independent review. Final independent acceptance passed after R1/R2 resolution.

**Verification boundary:** hermetic unit/race, two-app and binary integration,
configuration/documentation checks, and independent review passed. Optional
deployment investigations LWR-01/LWR-02 remain blocked on operator evidence;
LWR-03/LWR-04/LWR-05 remain deferred. No live-provider/deployment, database-backed
optional-test, or total heap/RSS validation is claimed. No commits were made.

## Objective

Provide an opt-in, per-provider deferred-request transport between aiproxy
instances, so long periods of model-provider silence do not require a silent,
long-lived HTTP request across an intermediary with an idle timeout.

Preserve the interactive client's ordinary response semantics, including the
original `stream` choice. Keep accepted work and retained results in bounded memory
on one executing instance. Define delivery, cancellation, expiry, and retry
behavior before implementation.

LWR-01 through LWR-05 retain the deployment-diagnosis and alternative-mechanism
work from the first draft. They are not a requirement to implement every possible
mitigation.

## Problem shape and motivating evidence

```text
interactive client → caller proxy → intermediary → executor proxy → model provider
```

Roles used throughout this document:

- **Caller proxy**: the downstream aiproxy facing the interactive client; its
  configured provider points to another aiproxy.
- **Executor proxy**: the upstream aiproxy accepting the deferred job and making
  the ordinary inference request. Its provider connection can have its own
  intermediaries and deadlines.
- **Idle window**: the inactivity limit on the peer leg, determined from the
  deployment or a test fixture; no fixed duration is assumed.

The motivating symptom is an upstream transport failure before the caller proxy
obtains a response, while an equivalent direct request can succeed. This suggests
a path-dependent failure but does not identify a particular appliance or prove
that the provider connection remains alive.

## Scope, non-goals, and working rules

- Initial proposed scope: `POST /v1/chat/completions`, JSON and SSE, with a caller
  `openai-compatible` provider targeting an executor. The executor can use any
  existing adapter that supports that operation. LWR-08 must approve an explicit
  operation/provider matrix; provider-agnostic does not mean every protocol is
  automatically supported.
- In scope: versioned peer protocol, configuration, bounded in-memory execution,
  retrieval, caller integration, authorization, lifecycle, observability,
  documentation, and hermetic tests.
- Ordinary non-opted-in requests retain existing behavior. Opted-in clients still
  receive the original API's response shape; peer `202` acknowledgments, status
  objects, cursors, and protocol headers are consumed by the caller proxy.
- This adds an authenticated network extension. An `/_internal` prefix is a
  routing convention, not an access-control boundary. Document the extension
  separately from provider capability support.
- Non-goals: bulk/file-based Batch APIs, public Responses background-mode
  compatibility, queues or distributed persistence, crash-durable or exactly-once
  inference guarantees, automatic infrastructure changes, and protocol expansion
  beyond the approved matrix.
- The single-executor assumption includes deployments and rolling updates:
  subsequent requests must reach the same process. Load balancing across replicas
  and recovery after process replacement require a future design.
- Existing provider header timeouts, client cancellation, listener write
  deadlines, and limits elsewhere in the chain still apply. Deferred transport
  supports bounded long waits, not arbitrarily long execution.
- Follow `AGENTS.md`, including source-comment conventions. Rationale belongs in
  `docs/design.md`. Preserve unrelated changes.
- Shared code hotspots are `internal/httpapi`, `internal/provider`,
  `internal/config`, and `internal/app`. Sequence dependent changes. This task file
  alone does not authorize implementation; the user subsequently explicitly
  authorized implementation via subagents.

## Analysis coverage and limitations

The pre-implementation review inspected these boundaries:

| Boundary                | Evidence and implication                                                                                                                                                                                                                                                       |
| ----------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Request snapshots       | `internal/provider/provider.go` (`executeUpstream`) attaches snapshots only to a non-nil `Result`; `internal/httpapi/handler.go` (`payloadUpstreamRequest`) omits them otherwise. An empty snapshot is not proof of zero network bytes or a missing outgoing request.          |
| Response timing         | `internal/httpapi/handler.go` dispatches before `writeResult`. `internal/httpapi/response.go` flushes streaming headers and each read buffer, not each SSE event. Unary responses and error bodies are read before returning a result.                                         |
| SSE handling            | `internal/provider/sse.go` ignores comments and bounds parsing. Its event type contains `Type` and `Data`, not a resumable sequence number. `TestSSEDecoderHandlesFramingBoundaries` already covers comments/multiline input.                                                  |
| Admission and ownership | `Handler.ServeHTTP` authenticates, rate-limits, authorizes the model, checks quotas/capabilities, and applies guardrails. `internal/auth/auth.go` has static identities, dynamic key/workspace identities, and a nil principal for `auth` mode `none`.                         |
| Retry and completion    | `internal/httpapi/dispatch.go` retries generic alias errors, releases target leases via `OnClose`, and finalizes streams separately. A peer-control error cannot safely become an ordinary retryable inference error.                                                          |
| Timeouts and transport  | `internal/config/types.go` defines a 90s header-timeout default. `internal/app/app.go` applies provider header timeouts and optional listener write timeouts. `internal/upstreamhttp/redirect.go` enforces same-origin redirects.                                              |
| Runtime ownership       | `App.Reload` replaces dependencies and retires/closes old resources; `App.Close` closes app-owned resources. Jobs outliving their submit handler need explicit ownership and shutdown behavior.                                                                                |
| Configuration surfaces  | `internal/config/schema.go`, `build.go`, `convert.go`, `dynamic.go`, `internal/configedit/configedit.go`, `internal/httpapi/admin_providers.go`, and `internal/dbmerge/dbmerge.go` each carry provider contracts. A new block needs round-trip and managed-provider decisions. |
| Route namespace         | `internal/webui/webui.go` reserves `/_internal`, not `/internal`. New job endpoints must use the reserved namespace and explicit API routing.                                                                                                                                  |

Related existing work: [configurable header timeouts](20260809-132640-configurable-upstream-header-timeout.md)
and [alias cooldowns](20260907-104451-alias-upstream-retry-cooldown.md).
These are preservation constraints, not duplicate feature tasks. Focused backlog
search found no other peer-deferred implementation plan.

This is a source/design review, not a cluster diagnosis or exhaustive audit.
Actual intermediary settings, packet-level timings, live providers, and new
protocol behavior have not been tested. Runtime baseline tests were not run for
this task-document change; review verification is recorded at the end.

## Review corrections to the first draft

The earlier draft's conclusions are superseded as follows:

1. EOF timing, missing finish logs, and configured keepalive do not convict a
   particular load balancer or exonerate every other layer. A bypass experiment
   narrows the changed path; it does not identify one appliance by itself.
2. Time to first byte (TTFB), first body data, and first generated token (TTFT)
   differ. A line-timestamping pipeline alone cannot measure headers or raw idle
   gaps. Streaming does not guarantee prompt bytes.
3. Poll-then-live-attach can recreate the original idle failure during a later
   pause. Incremental delivery needs bounded polling or an independently
   keepalive-protected, resumable peer stream.
4. Early `200` commits lose ordinary HTTP error/status retry semantics. Heartbeat
   and bridge mechanisms are alternatives with explicit tradeoffs, not mandatory
   steps or generally lossless conversions.
5. A memory-only server cannot reliably distinguish unknown, expired, deleted,
   and pre-restart jobs without additional metadata. Cancellation also cannot
   guarantee that an external provider stops computing or billing.
6. Version downgrade, ambiguous submission, result-fetch retry, growing response
   buffers, ownership, and reload were underspecified. LWR-08 through LWR-12 add
   the missing design and implementation boundaries.

External references:

- [OpenAI Batch](https://developers.openai.com/api/docs/guides/batch) describes
  file-based asynchronous groups, not this single-request transport.
- [OpenAI background mode](https://developers.openai.com/api/docs/guides/background)
  is useful precedent for polling and resume vocabulary, not a universal standard
  or an existing Chat Completions contract.
- [RFC 7240, section 4.1](https://www.rfc-editor.org/rfc/rfc7240#section-4.1)
  standardizes `Prefer: respond-async`; it permits ignoring preferences and leaves
  result retrieval implementation-specific. It is not mandatory negotiation.
- [AWS NLB idle-timeout documentation](https://docs.aws.amazon.com/elasticloadbalancing/latest/network/update-idle-timeout.html)
  corrects the earlier blanket “fixed 350s” claim: TCP listeners default to 350s
  but allow 60–6000s; TLS listeners remain fixed at 350s. Deployment tasks must
  inspect actual listener types, versions, and supported operator settings.

## Execution order and contract decisions

P1 means required for a correct feature release; P2 means optional operational
work or an alternative, not an emergency classification.

Recommended deferred-feature sequence:

```text
LWR-08 → LWR-09 → LWR-10 → LWR-06 → LWR-11 → LWR-12 → LWR-07
```

LWR-01/LWR-02 are optional deployment investigations; LWR-03 follows LWR-01 if an
operator selects a configuration change. LWR-04/LWR-05 remain deferred alternatives.
If LWR-08 selects a different scope, explicitly revise affected dependencies and
the definition of done. A deferred task does not satisfy a dependency.

LWR-08 must settle these material decisions:

- **Streaming delivery:** completion-then-replay, preserving response bytes but
  delaying all tokens; incremental finite cursor-based polls; or live attachment
  with peer heartbeats, bounded framing, and reconnection. Select one v1 contract.
  For incremental UX, finite chunk polls are the simplest way to keep generation
  silence off peer connections.
- **Negotiation:** header-opt-in submission versus a dedicated versioned submit
  endpoint, recognition before inference, and mixed-version behavior. Deferred
  mode must not silently fall back to synchronous inference or resubmit after an
  ambiguous acknowledgment.
- **Scope and ownership:** supported operations, per-provider caller opt-in,
  executor enablement, aliases, authenticated job ownership, and anonymous mode.
  A shared peer credential identifies the caller proxy at the executor; it does
  not authenticate each end user behind that proxy.
- **Lifetimes and limits:** execution deadline, terminal-result retention,
  abandoned-job cleanup, per-control-request timeout, overall caller wait, memory
  limits, concurrency, and polling cadence. Execution deadline and retention TTL
  are distinct; ordinary polling must not retain jobs indefinitely.
- **Configuration reach:** inheritance and reload, static versus managed provider
  support, CLI editing/conversion, and interaction with existing transport/auth
  settings. Prefer a documented, explicitly validated initial subset over silently
  ignoring new fields.

The following is a design sketch, not an implemented or finalized API:

```text
POST   /v1/chat/completions                 + X-Aiproxy-Deferred: v1
       → 202 Accepted, Location: /_internal/deferred/<id>
         {"id":"<opaque-job-id>","status":"queued"}
GET    /_internal/deferred/<id>             status and result metadata
GET    /_internal/deferred/<id>/result      retained completed response
DELETE /_internal/deferred/<id>             cancel if running, release if terminal
```

Add a cursor-read/stream endpoint only if incremental delivery is selected.
Specify all response schemas, statuses, errors, version markers, expiry metadata,
and polling advice in LWR-08. `202` means admitted after bounded validation and
capacity checks, not inference success; it does not promise instant acknowledgment
before the request body or authorization checks finish.

### Task LWR-01: Investigate a deployment's constrained connection path

Status: blocked; optional investigation requires deployment access and sanitized
operator evidence, which have not been supplied.

Kind: investigation

Priority: P2 — informs an operator remedy without blocking generic feature work.

Dependencies: none

Primary ownership:

- Operator-run inspection and a sanitized evidence summary in this document.

Finding:

Path-dependent disconnects warrant investigation, but current application logs
and an empty request snapshot cannot identify the exact closing component.

References: `executeUpstream`, `payloadUpstreamRequest`, `dispatchDirect`,
`dispatchAlias`, and `applyServerConfig` in the analysis table.

Requirements:

1. Inventory intermediaries, configured application deadlines, listener type,
   HTTP version, environment proxies, and effective infrastructure idle/request
   timeouts. Record observed values separately from documented defaults.
2. Inspect the actual route/backend and every serving router replica when
   applicable. A generic timeout grep or missing service annotation does not
   establish the route's effective configuration or the cloud resource value.
3. Compare the same representative request through the public path and a
   controlled bypass, preserving hostname/TLS verification, model/body, and
   recording differences in protocol or serving instance.
4. Correlate response headers, transport failures, proxy/intermediary logs, and
   cancellation. State confirmed facts, remaining hypotheses, and the next
   observation needed if the component remains unidentified.

Acceptance criteria:

- A sanitized topology/timing report names the proven failure boundary, or
  explicitly records what remains uncertain and a bounded next experiment.
- An operator-supported remediation path is identified where evidence permits.

Verification: evidence review; deployment access is required. Record a missing
access prerequisite as blocked, without blocking LWR-08.

### Task LWR-02: Measure whether streaming keeps bytes flowing

Status: blocked; original request evidence and deployment measurements have not
been supplied. Local silence fixtures will be covered by LWR-07.

Kind: investigation

Priority: P2 — measures the usefulness and limitations of streaming in a deployment.

Dependencies: none

Primary ownership:

- Operator-run measurements or a local mock, with a sanitized summary here.

Finding:

Streaming requests can remain silent before headers, before the first body data,
or between later chunks. These require different observations.

References: `internal/httpapi/response.go` (`writeResult`, `copyAndFlush`),
`internal/provider/openai.go` (`openAIPassthroughHandlers`), and
`TestHandlerStreamingPassthroughFlushesIncrementally`.

Requirements:

1. Inspect the original incoming request's `stream` flag. Use the request side of
   a payload entry when its upstream snapshot is absent; account for configured
   omission/truncation rather than inferring the flag.
2. Record request start, response-header arrival, first body data, first token
   where identifiable, later data gaps, HTTP status, and process/transport exit
   status. Use a timestamped receive trace or instrumented client; line timestamps
   alone omit headers and partial-line traffic. Label TTFB and TTFT separately.
3. Compare peer-route and direct paths using equivalent workloads. Sanitize traces
   before recording evidence because they can contain credentials and content.
4. Include initial silence, early headers followed by silence, and a later pause.
   Report measurements as observations, not guarantees for all future requests.
5. Recommend an operator fix, streaming where adequate, or a selected resilience
   mechanism. Heartbeats alone only cover streaming; a bridge depends on upstream
   activity unless paired with an independently specified heartbeat mechanism.

Acceptance criteria:

- Recorded header/body/token timings, longest observed gap, failure outcome, and
  stream-flag finding, with measurement limitations.
- A recommendation distinguishes bounded silent-upstream protection from an
  improvement that merely happens to work on the measured workload.

Verification: evidence review; no live model traffic is required for CI.

### Task LWR-03: Apply an evidence-backed intermediary configuration remedy

Status: deferred; no proven deployment cause or operator-authorized infrastructure
change. LWR-01 remains its prerequisite.

Kind: improvement

Priority: P2 — can resolve a deployment issue without a proxy feature.

Dependencies: LWR-01

Primary ownership:

- Operator-owned configuration change or support request; sanitized summary here.

Finding:

When LWR-01 confirms an intermediary timeout, its owning configuration surface is
the place to adjust that limit. This finding is conditional, not a presumed cause.

References: LWR-01 evidence and the documentation for the observed intermediary
and deployed version.

Requirements:

1. Identify the supported setting and its controller/owner. Select a timeout
   consistent with application deadlines, or an approved connectivity change.
2. Track an operator-authorized change or support request. An internal/private
   endpoint is not assumed to eliminate all load balancers or timeouts.
3. Repeat the reproducer and record effective post-change settings and results.
   Preserve uncertainty if no change or successful retest is available.

Acceptance criteria:

- The remedy is applied and the reproduction passes beyond the former limit.
- A pending support case remains blocked, not completed. If no remedy is selected,
  mark the task deferred with rationale and residual constraints.

Verification: operator evidence and a post-change reproduction; no automatic
infrastructure mutation is part of repository implementation.

### Task LWR-04: Define and implement an optional early-SSE heartbeat mode

Status: deferred; alternative to the primary deferred-request transport

Kind: improvement

Priority: P2 — potentially useful for streaming, but changes HTTP error semantics.

Dependencies: LWR-08, LWR-09; requires explicit selection in LWR-08

Primary ownership:

- `internal/httpapi/handler.go`, `response.go`, `dispatch.go`,
  `internal/provider/openai.go`, `sse.go`, and focused tests.

Finding:

`writeResult` runs after dispatch. Editing only that function cannot emit bytes
while waiting for initial upstream headers. Committing `200` earlier also prevents
later HTTP status replacement.

References: `Handler.ServeHTTP`, `writeResult`, `dispatchAlias`,
`observeOpenAIStreamEvent`, and `TestSSEDecoderHandlesFramingBoundaries`.

Requirements:

1. Before activation, document accepted error/usage/retry tradeoffs and supported
   operations. No-opt-in behavior remains unchanged; both provider configuration
   and a recognized peer request must opt in.
2. Complete authentication, model authorization, quota/guardrail/capability checks,
   and relevant request validation before any speculative response commitment.
3. If selected, emit heartbeats during initial silence and later pauses using one
   serialized writer. Never inject a comment into a partial SSE line/event or race
   heartbeat writes with upstream copying. Bound any framing buffer.
4. Specify in-stream errors after commitment. Ordinary status-based alias failover
   cannot be claimed unchanged; never splice a replacement inference into an
   already committed client stream. A private envelope that delays client
   commitment is a separate transport design, not a free property of SSE comments.
5. Stop heartbeat/read goroutines on completion, cancellation, write failure, or
   shutdown. Exclude synthetic traffic from model usage and result cursors.

Acceptance criteria:

- The approved error contract is documented and reflected in accounting and tests.
- A hermetic idle-cutoff intermediary survives initial and later silence; fragmented
  SSE stays valid; concurrent completion/cancellation leaves no writer races.
- Authentication/validation failures precede commitment, and no-opt-in paths match
  existing tests.

Verification: focused tests in `internal/httpapi` and `internal/provider`, including
race checks, then shared final checks if this alternative is selected.

Deferral rationale: this changes the external streaming error contract and is not
needed merely to implement the private deferred protocol.

### Task LWR-05: Define and implement an optional bounded streaming bridge

Status: deferred; alternative for a specifically supported response subset

Kind: improvement

Priority: P2 — can help non-streaming callers when upstream streaming is timely.

Dependencies: LWR-08, LWR-09; requires explicit selection in LWR-08

Primary ownership:

- `internal/provider/openai.go`, `sse.go`, related response types,
  `internal/httpapi/handler.go`, and focused tests.

Finding:

Upconverting to streaming may produce earlier bytes, but cannot create bytes
during upstream silence. Arbitrary provider-specific SSE cannot be assumed to
reconstruct an equivalent non-streaming JSON response.

References: `doOpenAI`, `openAIPassthroughHandlers`, `observeOpenAIStreamEvent`,
`newSSEDecoder`, and the caller-visible accounting boundary in `Handler.ServeHTTP`.

Requirements:

1. Specify a supported operation/schema subset before implementation. Cover
   multiple choices, tool-call argument deltas, finish reasons, refusal/reasoning
   fields, and the treatment of provider extensions; reject unsupported input
   before inference instead of silently dropping fields.
2. Define bounded accumulation and total wait. Recognize malformed/truncated
   streams and in-stream errors; partial output is not a successful unary result.
3. Request usage where the upstream supports it and define missing-usage behavior.
   Do not assume a final usage chunk always exists or invent token counts.
4. Preserve ordinary pre-stream HTTP errors. If paired with LWR-04, explicitly
   resolve its speculative-200 error semantics instead of promising lossless
   status preservation.
5. Keep the incoming body and actual transformed upstream request distinguishable
   in payload logging. Record one caller completion and observed usage; preserve
   existing behavior for unselected providers and already-streaming callers.

Acceptance criteria:

- Fixtures cover the approved subset, unknown fields, tools, missing usage,
  fragmented data, errors, overflow, and cancellation.
- A silent-upstream test demonstrates the bridge's limitation unless separately
  protected; documentation does not advertise generic lossless conversion.

Verification: focused provider/HTTP tests and shared final checks if selected.

Deferral rationale: the deferred transport can retain the original `stream` flag
and avoid reconstructing provider-specific JSON.

### Task LWR-08: Finalize the deferred protocol and release scope

Status: completed

Assigned agent: LWR-08 contract agent; coordinator owns this task document.

Kind: investigation

Priority: P1 — resolve behavioral ambiguities before implementing either peer.

Dependencies: none

Primary ownership:

- This document and a bounded design update in `docs/design.md` when executed.

Finding:

The first draft mixed delayed replay with live streaming and omitted negotiation,
admission ambiguity, timeout budgets, and resource ownership. These choices affect
correctness, not just endpoint naming.

References: the analysis table, `executeUpstream`, `dispatchAlias`,
`StreamCompletion`, `App.Reload`, and the external protocol references.

Requirements:

1. Resolve every material decision in “Execution order and contract decisions,”
   including whether v1 preserves incremental streaming or buffers until terminal
   completion. Record any latency tradeoff explicitly.
2. Publish a method/status/schema table: admission, pending, headers-ready,
   completion, provider HTTP errors, job execution failures, cancellation, expiry,
   missing job, saturation, invalid cursor, and unsupported protocol. Define which
   states actually exist; keep job/control status separate from inference status.
3. Specify positive protocol recognition, accepted version signaling, base URL
   prefix handling, and old/new peer behavior. A regular provider `202` must not
   be mistaken for a deferred acknowledgment. Unknown preference/header behavior
   cannot safely be the negotiation strategy by itself.
4. Define submission deduplication using a caller-generated logical-attempt key,
   an ownership/request fingerprint, retention, and process-generation behavior.
   Select how to handle a lost submit reply, including a restart between attempts.
   Never promise exactly-once external inference.
5. Define receiver policy before target selection: app-owned storage, per-provider
   eligibility/budgets, mixed alias targets, and accidental recursive deferral.
   Preserve model authorization and existing alias selection rules.
6. Specify ownership for static and dynamic credentials, current authorization on
   control requests, credential rotation/revocation, and `auth` mode `none`.
   Recommended v1: require authenticated peer identities for deferred admission.
7. Specify execution, caller-wait, control-request, abandoned-job, and retention
   deadlines; finite defaults and limits; cancellation/release races; and reload,
   disablement, capacity reduction, and shutdown behavior.
8. If incremental, define transport framing and exclusive cursors independently of
   native SSE. Decide finite polls versus heartbeat-protected attach, incomplete
   transfer replay, and end-of-stream/error markers. If completion-only, retain
   the original SSE bytes and document delayed delivery.
9. Resolve terminal retry/cooldown metadata timing and typed no-retry outcomes for
   accepted/indeterminate work. Choose configuration exposure and state whether
   LWR-04/LWR-05 remain deferred; update dependent tasks if scope changes.

Acceptance criteria:

- A state diagram, method/error table, configuration table, ownership model, and
  failure/retry matrix answer the questions above with rationale.
- No material design choice remains hidden in an implementation task. Unresolved
  maintainer choices are recorded as blockers; downstream tasks cannot start.

Verification: design/evidence review; no full runtime suite is required for this
investigation.

Completion evidence:

- LWR-08 contract agent added the finalized
  [Peer Deferred Transport v1 contract](../design.md#peer-deferred-transport-v1):
  state/method/error/configuration tables, neutral wire schemas, signed
  submission scope, ownership, retry matrix, memory and lifecycle obligations.
- Coordinator reviewed the delivered section against the recorded user choices.
  It resolves the gate; LWR-04/LWR-05 remain unselected alternatives.
- Verified by the contract agent: `make docs-contract`, `git diff --check`,
  declaration syntax, local link, and contract-boundary checks.
- The finalized API supersedes the earlier header-based sketch in this file:
  `/_internal/deferred/v1/{capabilities,submit,jobs/...}`, terminal manifests,
  and immediate bounded byte-cursor reads for both unary and SSE responses.

### Task LWR-09: Add validated configuration and preserve editing contracts

Status: completed

Assigned agent: LWR-09 configuration agent (resumed sequentially after the
preparation rate-limit error).

Kind: improvement

Priority: P1 — opt-in settings must survive every supported configuration path.

Dependencies: LWR-08

Primary ownership:

- `internal/config/`, `internal/configedit/`, `cmd/aiproxy/`,
  `internal/dbmerge/`, and relevant admin provider validation/view code.

Finding:

The feature needs both caller selection and executor resource policy. The earlier
sketch mixed client wait with server retention and implied unselected mechanisms
could all be enabled together.

References: `rawProvider`, `config.Provider`, `jsonBlockLabels`,
`RenderProviderBlock`, `adminProviderUpsert`, and `dbmerge.BuildProvider`.

Requirements:

1. Implement only the LWR-08 configuration table, disabled by default. Validate
   duration/size/concurrency ranges, overflow, incompatible modes, provider and
   operation eligibility, and authenticated receiver policy.
2. Implement HCL/JSON parsing and conversion, inline configuration, defaults,
   disabled-provider validation, and explicit derived-provider inheritance/local
   declaration rules.
3. Preserve settings and supported `env()` expressions during CLI edits. Add
   scripted/interactive configuration support as selected in LWR-08, with clear
   errors for unsupported workflows.
4. For managed providers, implement the approved fields through persistence,
   merge, admin views, and forms, or explicitly enforce the approved static-only
   boundary. Cover derivatives of static bases and credential updates; do not
   silently accept and discard new fields.
5. Reserve private protocol headers against generic forwarding as needed, while
   preserving existing credential, user-agent, TLS-profile, proxy, and redirect
   contracts. Provider discovery and health probes do not submit inference jobs.

Acceptance criteria:

- Positive/negative configuration fixtures cover the approved surfaces, bounds,
  inheritance, and mode combinations.
- Editing and HCL/JSON round trips preserve effective settings.
- Unsupported managed/API inputs are rejected before commit; supported inputs
  survive save, reload, and view round trips.

Verification: focused tests in `internal/config`, `internal/configedit`,
`cmd/aiproxy`, `internal/dbmerge`, and `internal/httpapi`; include `internal/store`
and web type/build checks if their schemas or forms change.

Completion evidence:

- Configuration agent implemented types/defaults, HCL/JSON/inline/env parsing and
  validation, conversion, source-preserving CLI edits, local derivative opt-ins,
  ambiguous URL/private-header rejection, and managed static-only enforcement.
- Exported helpers include `DefaultDeferred`, `DefaultDeferredTransport`,
  `ValidateDeferred`, `ValidateDeferredConfig`,
  `ValidateManagedProviderDeferred`, and `IsDeferredHeader`.
- Passed from repository root:
  `go test -count=1 ./internal/config ./internal/configedit ./cmd/aiproxy ./internal/dbmerge ./internal/httpapi`
  and `git diff --check`.
- Coordinator inspected config/default/boundary code and hermetic tests proving
  rejected declarations return before row mutation or any store access, including
  credential PUT and opted-base merge/validation.
- Optional database-backed admin tests are present but skipped without
  `AIPROXY_TEST_DATABASE_URL`. No persistence schema was added; the static-only
  boundary is covered before persistence by hermetic tests. Database-backed
  end-to-end evidence is not claimed.
- LWR-07 integration found a reserved-header isolation gap on ordinary outbound
  responses. The shared response-header filter now strips the case-insensitive
  private namespace even without opt-in. The regression and post-fix checks are
  recorded under LWR-07; configuration forwarding restrictions remain intact.

### Task LWR-10: Implement bounded job storage and app-owned execution lifetimes

Status: completed; R2 test-synchronization correction accepted by independent
final re-review.

Prior owners: LWR-10 storage/lifecycle agent and LWR-10 reconciliation agent;
both stopped on provider encrypted-response verification errors. No agent remains
running from those interrupted sessions. Inspect existing work, not rebuild it blindly.

Completion owner: fresh LWR-10 completion/reconciliation agent, following the
allocation agent's provider token-rate-limit interruption.

Review owner: original independent final reviewer; the separate LWR-10
cleanup-verification investigation/remediation agent delivered its correction.

Kind: improvement

Priority: P1 — detached execution must have predictable memory and cleanup bounds.

Dependencies: LWR-08, LWR-09

Primary ownership:

- New `internal/deferred/` package (proposed), `internal/app/app.go`,
  lifecycle integration, and focused concurrency tests.

Finding:

Current inference lifetimes follow a request context and response body. A `202`
handler return must not cancel an accepted job, and a submit disconnect must not
become its only cleanup mechanism.

References: `Handler.ServeHTTP`, `cloneRequestWithBody`, `Result.OnClose`,
`StreamCompletion`, `App.Reload`, and `App.Close`.

Requirements:

1. Own jobs under the application lifecycle with a separate bounded execution
   context. Copy only needed request/body/identity data before handler return;
   never retain its response writer or a body the HTTP server will close.
2. Bound admitted/queued/running jobs, retained input/output bytes, per-job output,
   protocol overhead, readers, and dedup/tombstone entries. Use process-wide
   accounting even if policy is configured per provider or owner. Enforce limits
   during reads/appends, including temporary copies and encoded/decoded frames,
   rather than counting only completed entries.
3. Atomically reserve admission capacity before starting inference. Reject
   saturation before work; handle response growth after acceptance by the approved
   bounded failure/cancellation policy. Never silently evict active work or
   truncate output into a successful result.
4. Make state transitions, admission deduplication, reads, completion, delete, and
   expiry race-safe. Same key/same fingerprint reuses a job; conflicting reuse is
   rejected. Release must not erase the dedup protection while replay is allowed.
   Generate job IDs with at least 128 bits of cryptographic randomness, separately
   from caller-supplied correlation IDs. Bound identifier/key/cursor inputs and
   reject numeric overflow according to the protocol schema.
5. Apply distinct execution and terminal retention limits. Bound abandoned jobs if
   the caller disappears without `DELETE`; polls cannot extend absolute limits.
   Cleanup uses bounded work and releases actual buffers, readers, and workers.
6. Support retryable, non-destructive result reads until release/expiry. Define
   active-reader behavior during delete/expiry so retained memory is still counted.
7. Implement the selected process-generation/tombstone scheme. Without retained
   evidence, report an unavailable/unknown job; the caller may classify its
   previously acknowledged missing job as `request_lost`. Do not fabricate precise
   lost-versus-expired knowledge after restart.
8. Preserve admitted work across successful reload according to LWR-08, including
   its endpoint/model/credential snapshot and resources needed to finish.
   Failed reload leaves jobs intact. Disablement, smaller limits, and shutdown
   must have explicit drain/cancel behavior and bounded cleanup.

Acceptance criteria:

- Deterministic tests cover concurrent deduplication, conflicting keys, admission
  saturation, growing output, expiry, deletion, active readers, and restart.
- Slow consumers and abandoned callers cannot exceed configured memory/work bounds.
- Reload success/failure and shutdown tests show no orphaned goroutines, leaked
  provider bodies/leases, or use of prematurely closed runtime resources.

Verification: focused tests for the new package and `internal/app`, including
`go test -race` for those packages; use injectable clocks/barriers for lifecycle
boundaries.

Prior partial delivery / handoff evidence:

- Added `internal/deferred/` wire/validation/schema/ticket/budget/control/store/writer
  code and tests. It includes signed 30-second scopes, protected dedup/tombstones,
  bounded reservations/slab output/read scratch, control/delete lanes, two
  finalization workers, deadline/expiry, and transactional option updates.
- Added `internal/app/runtime_resources.go`,
  `internal/app/deferred_lifecycle_test.go`, and app Build/Reload/Close changes;
  the store is lazy-created on first receiver enablement and retained across
  removal/re-enablement. Added handler dependency leases and resource tests in
  `internal/httpapi/runtime_resources*.go`.
- The ordinary handler now acquires/releases runtime resources; it does **not**
  expose deferred job HTTP routes or execute deferred caller transport.
- Coordinator verified the partial snapshot with
  `go test -count=1 ./internal/deferred ./internal/app ./internal/httpapi` (passed).
  The interrupted agents did not deliver a trustworthy completion report;
  coordinator checkpoint checks and the remaining scope are recorded below.
- Still required: reconcile store/resource code against the finalized contract;
  implement deferred-specific provider input/output/JSON/SSE/snapshot preallocation
  limits and `provider.ExecutionLimits` wiring; prove these limits before buffered
  allocation and rerun affected tests/race checks. Store append limits alone do
  not satisfy the memory acceptance criteria.
- Full executor integration, remote deadline-aware completion sinks, HTTP
  metrics, and caller retry/outcome handling belong to LWR-06/LWR-11 and are not
  claimed delivered by this scaffold.
- The resumed allocation agent added provider execution limits, bounded JSON/SSE
  helpers, adapter hooks, and allocation/lifecycle regression tests before a
  provider token-rate-limit interruption. It did not return a completion report.
  Coordinator preserved the changes and verified
  `go test -count=1 ./internal/deferred ./internal/provider ./internal/app ./internal/httpapi`,
  the same package command with `-race`, and `git diff --check` (all passed).
  A fresh completion/reconciliation agent must establish the remaining acceptance
  evidence before this task is completed.

Completion evidence:

- Fresh completion/reconciliation agent inspected the preserved storage,
  allocation, adapter, and lifecycle work against the v1 contract and found no
  remaining LWR-10 gap. It replaced expanding response `Connection` token
  collections with bounded scanning in `internal/deferred/validation.go`.
- Regression `TestResponseConnectionTokensDoNotExpandMetadataAllocations` in
  `internal/deferred/schema_test.go` failed before the fix (1,591 versus 35
  allocations for long/short lists) and passed afterward.
- Delivered `provider.ExecutionLimits`, typed allocation errors, bounded
  JSON/SSE/encoding/read helpers, capture-prefix snapshots, and hooks across
  chat-capable adapters. Nil limits retain ordinary behavior. Allocation and
  lifecycle tests cover late HTTP-body ownership, finalization credit, frozen
  endpoint/credential snapshots, failed resource adoption, and bounded shutdown.
- Completion agent independently passed, from repository root:
  `go test -count=1 ./internal/deferred ./internal/provider ./internal/app ./internal/httpapi`,
  the same command with `-race`, `make vet test`, and `git diff --check`.
  Coordinator inspected the bounded header fix and reran
  `go test -count=1 ./internal/deferred` (passed).
- LWR-06 must reserve its own rewrite/redaction/payload/completion copies before
  allocation, pass frozen limits with `Writer.WorkBudget()`, and transfer the
  already-acquired runtime lease only on successful commit. HTTP/caller behavior
  and remote completion sinks remain downstream acceptance work.

Reopened verification R2:

- During post-R1 uncached `GOFLAGS=-count=1 make test-race`,
  `TestExecutionLimitsAllChatAdaptersPreserveJSONSSEAndNilPaths/gemini/stream=true`
  failed at `internal/provider/execution_limits_test.go:243` with
  `result close leaked 65738 reserved bytes`. A focused race `-count=10` rerun
  and one uncached full race rerun passed. The failure is not established as
  baseline, caused by R1, a true leak, or a faulty synchronization assertion.
- Investigate `executionCloseResult`, translated-stream pipe/source cleanup, and
  `executionState` retained ownership. Determine whether closing the result is
  being confused with asynchronous translation/source drain; never fix a test by
  falsely releasing live ownership or adding arbitrary sleeps.
- Required outcome: evidence-backed disposition and deterministic regression/
  synchronization correction if needed, followed by affected and uncached full
  checks. Independent final review must accept this disposition before closure.

R2 resolution evidence (accepted by independent final re-review):

- Investigation established faulty test synchronization, not a demonstrated
  runtime leak: the translator can close its pipe before releasing its retained
  execution lease. EOF/`OnClose` is not proof of completed source/worker drain.
  The reported 65,738 bytes match 64 KiB metadata plus a retained 202-byte request.
- Only `internal/provider/execution_limits_test.go` and
  `internal/provider/execution_sse_test.go` changed. Test budgets now use
  release-notification-based bounded drain checks instead of immediate assertions
  or polling. Production ownership and R1 behavior are unchanged.
- A deterministic translator cleanup barrier failed with the old assertion for
  all five translated adapter cases. New tests prove result Close stays
  nonblocking, live reservations remain counted until actual cleanup, ownership
  then drains exactly to zero, and the source closes once. Fake-clock coverage
  confirms partial/unreleased reservations fail the deadline without freeing
  accounting; no arbitrary sleep or success-by-rerun disposition is used.
- Remediation agent passed focused/original affected tests with `-race -count=20`,
  uncached normal/race provider/deferred/app/httpapi/e2e checks,
  `GOFLAGS=-count=1 make vet test`, fresh
  `GOFLAGS=-count=1 make test-race`, formatting checks, and `git diff --check`.
  Coordinator inspected the barrier/notification tests and reran their targeted
  race checks plus the original adapter matrix (passed).
- Integration/docs builds were not repeated for these test-only changes; the
  post-R1 production snapshot's verified integration/docs artifacts are unchanged.
- Original independent reviewer accepted R2's cause, deterministic regression,
  bounded drain verification, and unchanged production ownership. Its repeated
  race checks passed; LWR-07 records final acceptance evidence.

### Task LWR-06: Integrate authenticated deferred admission and result endpoints

Status: completed

Completion owner: fresh LWR-06 readiness/reconciliation agent after two executor
agents' provider token-rate-limit interruptions.

Kind: improvement

Priority: P1 — reuse inference policy while separating HTTP control from execution.

Dependencies: LWR-08, LWR-09, LWR-10

Primary ownership:

- `internal/httpapi/handler.go`, `dispatch.go`, `response.go`, new deferred
  handlers/protocol types, accounting/payload integration, and focused tests.

Finding:

The original LWR-06 combined server storage, protocol, caller transport, and
lifecycle into one task. Storage and caller work are now LWR-10 and LWR-11; this
task owns the executor's HTTP and inference integration.

References: `Handler.ServeHTTP`, `allowRequest`, `allowQuota`, `auth.Principal`,
`payloadUpstreamRequest`, `copyResponseHeaders`, and `webui.reservedPaths`.

Requirements:

1. Implement the approved versioned endpoints under `/_internal/deferred` and
   the selected submit surface. Define methods and content types; no web UI
   fallback may answer protocol requests. Disabled, malformed, or unsupported
   negotiation must have the LWR-08 pre-inference behavior.
2. Reuse the existing admission pipeline once: authentication, rate admission,
   body limits, allowed-model checks, resolution, quota, operation capability, and
   guardrails. Preserve any body rewrites/redactions and resolved identity when
   dispatching the job. Return `202` only after admission and scheduling succeed.
3. Authenticate every control/result request, bind it to the admitted owner and
   public model, and enforce the selected current-policy/revocation rules.
   Unguessable IDs supplement ownership checks; dashboard/admin credentials do not
   automatically grant access. Treat an end-user identity header as untrusted.
4. Apply a bounded control-plane rate/concurrency policy without charging each
   poll as new inference quota. Cancellation/release must remain usable under
   polling pressure. Do not allow a status/read request to dispatch a new job.
5. Serialize the original inference outcome independently of the control response:
   allowed status/headers, exact retained body bytes after existing adapter
   processing, transport failure, and stream outcome. Preserve existing response
   normalization and hop-by-hop filtering instead of promising raw HTTP identity.
   Use non-cacheable private control/result responses.
6. If incremental delivery is selected, implement its bounded transport frames,
   cursor rules, completion/error signal, and keepalive/short-poll behavior.
   Native SSE event IDs, comments, multiline data, and vendor extensions must not
   be replaced by invented Responses `sequence_number` fields.
7. Keep retained results readable after a dropped transfer. `DELETE` is idempotent
   cancel-if-running plus release; return promptly according to the selected
   cancellation semantics while cleanup completes. Local cancellation closes the
   provider request/body; it cannot guarantee cessation of external billing.
8. Record inference completion, known usage, and payload logs once per logical
   request at the existing serving/accounting boundary. Submit acknowledgments and
   replay/poll requests are control traffic, not successful inference events.
   Preserve guardrail redaction and capture limits; never log private credentials.
9. Add bounded-cardinality metrics for admission, active/retained jobs and bytes,
   expiration, cancellation, control retries, and delivery failures. Correlate job
   IDs in logs rather than metric labels; normalize dynamic endpoint paths.

Acceptance criteria:

- Tests cover accepted/pending/completed/failed/cancelled outcomes, retained
  provider errors, repeated reads, disabled/wrong versions, methods, and web UI
  coexistence.
- Admission failures make zero provider calls. Cross-owner/model-denied controls
  cannot reveal or delete a job; static, dynamic, and anonymous policies match
  LWR-08.
- Poll storms, replay, and `DELETE` races do not duplicate inference accounting,
  quota consumption, dispatch, or finalizers.
- Both unary and selected streaming paths survive the adversarial silence fixtures.

Verification: focused `internal/httpapi`, `internal/provider`, `internal/auth`,
and new deferred-package tests; affected race checks and shared final checks.

Partial delivery / continuation evidence:

- Executor integration agent delivered shared admission helpers, private HTTP
  routes, detached execution/completion integration, source tracking, bounded
  metrics, and context-aware quota/health/payload sinks before a provider
  token-rate-limit interruption. No completion report was returned.
- Coordinator inspected `internal/httpapi/{admission,deferred_http,deferred_execution}.go`
  and the preserved tests. From repository root,
  `go test -count=1 ./internal/httpapi ./internal/provider ./internal/auth ./internal/deferred ./internal/app ./internal/observability ./internal/providerhealth ./internal/payloadlog ./internal/mongolog`
  failed only in `internal/httpapi`:
  `TestDeferredUnaryInitialAndHeaderOnlySilenceRetryObservationAndExactBytes`
  reported changed unary inference metadata. Other listed packages passed;
  `git diff --check` passed. This is unresolved, not a claimed baseline failure.
- Fresh completion/reconciliation must fix the regression, reconcile the remaining
  LWR-06 criteria (especially finalization, control I/O, poll/delete and reload
  boundaries), and pass focused/race and mandatory repository checks before
  LWR-11 starts.
- The completion/reconciliation agent fixed whitespace-prefixed JSON usage
  observation without changing body bytes, expanded unary diagnostics, and added
  socket I/O, concurrent controls, remote/local finalization, HTTP app reload,
  metrics, and sink deadline regressions before another provider token-rate-limit
  interruption. It returned no completion report.
- Coordinator inspected those changes and independently passed the focused
  command above, the same package command with `-race`, and `make vet test` from
  repository root. A fresh readiness/reconciliation agent will review the current
  delivered criteria rather than repeat the implementation.

Completion evidence:

- Fresh executor readiness agent reconciled the current code and tests against
  the v1 contract with no material unmet LWR-06 criterion or blocker.
- Delivered shared inference admission, authenticated versioned control routes,
  finite byte-cursor reads/manifests, dedup replay and current model/owner checks,
  detached source/lease ownership, normalized final response heads, bounded
  metrics, and once-only local completion with deadline-aware remote sinks.
- Inspected tests cover static rotation/dynamic key/workspace ownership,
  anonymous/admin exclusion, guardrail/quota parity, whole-alias eligibility,
  exact unary/native SSE bytes, retained provider errors, delayed retry advice,
  concurrent replay/poll/delete, real socket deadlines and unchanged-cursor
  recovery, finalizer queue expiry/backpressure, and HTTP reload/draining.
- Coordinator passed the nine-package uncached/race commands recorded above and
  `make vet test`. Readiness agent additionally passed
  `go test -count=1 -run '^(TestDeferred|TestExecution)' ./internal/httpapi ./internal/app ./internal/provider ./internal/observability`
  and `git diff --check`, from repository root.
- Full caller transport and two-proxy/intermediary evidence remain LWR-11/LWR-07;
  executor completion does not claim those downstream criteria.
- LWR-07 corrected ordinary JSON/SSE response filtering to exclude reserved
  deferred headers. Its focused regression failed before the fix and passed
  afterward; status/body and non-reserved headers are preserved. Full post-fix
  release checks passed as recorded under LWR-07.

### Task LWR-11: Implement the caller transport and explicit retry boundaries

Status: completed; independent final re-review accepted the R1 correction.

Prior completion owner: fresh LWR-11 readiness/reconciliation agent after the caller
transport agent's provider token-rate-limit interruption.

Review owner: original independent final reviewer after separate R1 remediation
and reconciliation/verification agents delivered the correction and checks.

Kind: improvement

Priority: P1 — the private protocol must not leak or duplicate accepted inference.

Dependencies: LWR-06

Primary ownership:

- `internal/provider/`, `internal/httpapi/dispatch.go`,
  `internal/upstreamhttp/`, caller runtime wiring, and peer-chain tests.

Finding:

The current OpenAI-compatible adapter forwards a single request and returns its
result. `dispatchAlias` retries generic errors. Treating a lost poll as another
upstream inference attempt could create duplicate accepted work.

References: `doOpenAI`, `executeUpstream`, `dispatchDirect`, `dispatchAlias`,
`ParseRetryCooldown`, `upstreamhttp.Do`, and `ClientForProvider`.

Requirements:

1. Enable peer behavior only for the configured provider and approved operations.
   Preserve normal request/model/header handling for the actual inference payload;
   private negotiation/dedup headers must not reach the ultimate model provider,
   even through generic forwarded-header settings.
2. Validate the agreed version and acknowledgment. Pin accepted work to its peer,
   job ID, execution generation, credential/transport context, and logical attempt.
   Validate `Location`/result URLs against the configured origin and allowed
   protocol path, including base path prefixes; never follow arbitrary URLs with
   provider credentials.
3. Bound each control call's header and body wait, overall job wait, poll size,
   backoff, jitter, and retry count. Honor bounded peer polling advice separately
   from model cooldown advice. Do not mutate shared transports or overload
   `upstream_header_timeout` to mean result retention. Set finite poll/heartbeat
   budgets compatible with the declared peer idle window; heartbeats cannot
   override a total request deadline.
4. Implement lost-ack handling with the approved dedup contract. Automatic submit
   retries, including HTTP transport replays, must be safe within a proven
   generation/retention scope. The executor must reject expired-generation replay
   before dispatch; client-side observation alone cannot close that race.
5. Once work is accepted or acceptance is indeterminate, transient control/fetch
   failures retry retrieval of that work, not inference or another alias target.
   Add typed no-retry/indeterminate outcomes where generic alias errors currently
   retry. Restart, expiry, and exhausted recovery produce explicit unavailable/lost
   outcomes without claiming whether external usage occurred.
6. Preserve configured alias failover for definite inference outcomes before
   client commitment, and direct requests' no-failover rule. After commitment,
   resume only the same retained output; never splice output from a fresh job.
   Distinguish peer-control pressure/failures from model health and rate limits.
7. Capture model retry advice with its observation time/deadline and apply only
   remaining cooldown; repeated/delayed retrieval must not restart the original
   duration. Finalize provider metrics, health, usage, and alias leases once at
   the appropriate logical completion boundary.
8. For unary results, return the existing caller-facing status/body semantics.
   For streaming, implement exactly the delivery mode approved in LWR-08.
   If resumable, acknowledge/advance only fully received and delivered private
   frames; discard partial transport frames on reconnect and avoid duplicate bytes.
   Keepalives do not advance data cursors. Bound replay buffers and transfer time.
9. Map client cancellation/deadline to a bounded best-effort `DELETE` using a
   cleanup context independent of the already-cancelled request. Release after
   successful delivery; retain server-side deadline/TTL cleanup when the caller
   crashes or cannot authenticate/reach the peer.

Acceptance criteria:

- For a direct target, duplicate submissions and interrupted acknowledgments cause
  one provider call within the accepted generation's dedup window; alias attempts
  match explicit policy rather than poll count.
- Mixed versions, ordinary upstream `202`, invalid URLs, and expired generations
  produce the approved outcome without credential leakage or hidden resubmission.
- Interrupted retrieval is recoverable within retention; streaming replay has no
  duplicates/gaps, and premature EOF is not reported as successful delivery.
- Cancellation, TTL loss, reload, and alias error paths release resources and
  classify health/cooldown/accounting according to the contract.

Verification: focused provider/HTTP/upstream transport tests, new chain tests in
`internal/e2e` or `internal/integration`, and affected race checks.

Partial delivery / continuation evidence:

- Caller transport agent delivered `internal/provider/deferred.go`, typed
  control/origin retry boundaries, context-bound redirect/cookie isolation,
  HTTP dispatch/error mapping, and caller/two-app chain regression tests before
  a provider token-rate-limit interruption. No completion report was returned.
- Coordinator inspected capability/submit recognition, qualified replay,
  safe local control URLs, call/retry budgets, and error classification.
  From repository root, these passed:
  `go test -count=1 ./internal/provider ./internal/httpapi ./internal/upstreamhttp ./internal/deferred ./internal/app ./internal/e2e`,
  the same command with `-race`, `make vet test`, and `git diff --check`.
- `internal/e2e/deferred_chain_test.go` now covers a response-inactivity cutter,
  dropped submit/read transfers with one job, and caller reload preserving the
  accepted transport/credential snapshot. Fresh readiness reconciliation must
  establish the full LWR-11 disposition before documentation starts.

Completion evidence:

- Fresh caller readiness agent reviewed the delivered transport, retry boundaries,
  and tests against the v1 contract without finding a material unmet LWR-11
  criterion. No corrective edits were needed.
- Delivered `Request.DeferredTransport`, `Result.Origin`, retry observation/deadline
  fields, typed `DeferredControlError`, and `upstreamhttp.ControlContext`; executor
  receipts/frames remain private. Qualified lost-ack replay, finite authenticated
  retrieval, strict origin/mount/version/schema validation, once-only cleanup,
  and explicit inference-versus-control outcomes are wired through HTTP dispatch.
- Acceptance coverage includes same-job interrupted transfer recovery, exact
  unary/SSE delivery and terminal hashes, restart/expiry loss, cancellation,
  unconsumed streaming heads, non-opted/non-chat compatibility, alias retry and
  health/cooldown boundaries, and two-app inactivity-cutter resilience.
- Coordinator six-package focused/race suites and `make vet test` passed as
  recorded above. Readiness agent independently passed
  `go test -count=1 -run 'TestDeferredCaller|TestDeferredPeerChain|TestControlContext' ./internal/provider ./internal/httpapi ./internal/upstreamhttp ./internal/e2e`,
  the same command with `-race`, and `git diff --check` from repository root.
- Binary lifecycle, HTTP/2, and broader adversarial release evidence remain LWR-07,
  not implied by this task's completion.

Reopened acceptance / independent review finding R1:

- P1 confirmed correctness defect: after a deferred SSE prefix has committed, a
  definite terminal `upstream_error` becomes an ordinary stream error in
  `internal/provider/deferred.go` (`inferenceOutcome`, `deferredReadCloser.Read`).
  `internal/httpapi/handler.go` aborts delivery only for typed peer-control
  failures. The ordinary return can therefore close HTTP 200 cleanly with a
  partial body and no client read error.
- Independent reviewer reproduced this 3/3 under race instrumentation using real
  upstream/executor/caller HTTP handlers and actual adapters. The upstream aborts
  its chunked response only after the client has read the prefix; client
  `io.ReadAll` then incorrectly returns a nil error. No backup inference occurred.
- Required correction: permanent socket-level regression and explicit failed
  delivery after deferred commitment, without new inference or changing a
  definite upstream failure into a peer-control classification. Preserve known
  usage, provider health semantics, and once-only finalizers/cleanup. Ordinary
  non-opted-in behavior must remain unchanged.
- Prior completion evidence is retained above but does not satisfy the missing
  premature-EOF criterion. Remediation and independent re-review are required
  before this task and dependent LWR-07 can close.

R1 remediation / continuation evidence:

- Remediation agent saved a separate `Result.AbortOnStreamError` delivery marker,
  enabled only for deferred streaming results, and made the HTTP handler abort
  failed marked delivery after normal stream finalization. Definite inference
  errors remain inference errors, not peer-control failures.
- Added `internal/httpapi/deferred_delivery_test.go` with real-socket direct/alias
  cases for upstream transport failure and native error events, plus ordinary
  non-opted-in compatibility. It checks exact prefixes, known usage, health
  classification, no replacement inference, and once-only resource/accounting/
  payload/cleanup behavior.
- The agent stopped on a usage limit without a completion report. Coordinator
  preserved and inspected the changes and passed, from repository root,
  `go test -race -count=3 -run '^TestDeferredCallerCommittedInferenceFailureDelivery$' ./internal/httpapi -timeout=3m`;
  the independent review's original temporary-overlay reproducer with `-race`
  and `-count=3`; formatting checks on the four remediation files; and
  `git diff --check`.
- These focused checks do not claim the interrupted agent's unreported release
  verification. Fresh reconciliation must establish post-fix shared check
  evidence before independent re-review.
- Fresh R1 reconciliation agent found no material gap in the fix and made no
  edits. It passed `make vet test`, `make test-race` (cached),
  `go test -race -count=1 -timeout=10m ./internal/httpapi ./internal/provider ./internal/e2e`,
  `make integration` (UI/binary rebuilt), `make docs-contract`,
  `pnpm docs:build`, `git diff --check`, and `GOFLAGS=-count=1 make test`.
- An uncached `GOFLAGS=-count=1 make test-race` initially failed the Gemini
  execution-limit cleanup assertion recorded as R2 under LWR-10. A focused
  ten-run race check and one full uncached rerun passed, but the cause remains
  unexplained. This failure is retained as unresolved evidence, not waived by
  reruns. R1 independent re-review follows after R2 disposition.
- Final disposition supersedes the earlier pending notes: R2 was diagnosed and
  corrected as recorded under LWR-10. Original independent reviewer accepted
  R1's separate delivery marker, post-finalization HTTP abort, inference health/
  usage classification, no replacement inference, once-only cleanup, and ordinary
  streaming compatibility. Its original reproducer and permanent socket tests
  passed three times with race instrumentation.

### Task LWR-12: Document the delivered protocol and operational limits

Status: completed

Completion owner: fresh LWR-12 documentation agent after a provider token-rate-limit
interruption before delivery.

Kind: improvement

Priority: P1 — operators must be able to configure and reason about the feature.

Dependencies: LWR-09, LWR-06, LWR-11

Primary ownership:

- `docs/design.md`, `website/docs/configuration.md`,
  `website/docs/operations.md`, relevant API documentation, and release notes.

Finding:

Deferred execution introduces resource retention and a new peer API even though
ordinary callers retain their existing API shape.

References: approved LWR-08 contract, implemented config/handlers/client, and the
repository's public docs contract matrices.

Requirements:

1. Document supported operations, actual configuration/defaults, both peer roles,
   namespace/version negotiation, timeouts, retry ownership, and mixed-version
   deployment behavior with generic examples.
2. Explain the chosen streaming latency/resume contract, memory bounds, single
   process assumption, rolling replacement/restart loss, cancellation limits,
   retention/release, and end-client deadlines.
3. Describe diagnostics and metrics, distinguish control failures from provider
   outcomes, and document the separate payload-log/redaction policy.
4. Keep provider capability matrices accurate without claiming Batch/Responses
   background compatibility. Add opt-in protocol documentation and release notes.

Acceptance criteria:

- Configuration examples validate against the implementation; API examples match
  the method/status table and have no deployment-specific credentials or hosts.
- Documentation exposes the real limits and selected streaming behavior.
- Docs contract and website build checks pass.

Verification: `make docs-contract`, `pnpm docs:build`, configuration validation
fixtures, and implementation-to-document review from the repository root.

Continuation evidence:

- Initial documentation agent stopped on a provider token-rate limit before
  delivering documentation or validation fixtures. Existing runtime changes were
  preserved; LWR-12 remains incomplete.
- Coordinator passed `make docs-contract` and a baseline `pnpm docs:build` from
  repository root. These baseline checks do not validate still-unwritten feature
  documentation. A fresh, narrowly scoped documentation agent follows.

Completion evidence:

- Documentation agent delivered configuration/default/range tables, full generic
  caller/executor examples, versioned API schemas/statuses, retry/ownership/deadline
  rules, resource and logging bounds, metrics, and reload/draining guidance in
  `website/docs/{configuration,operations,api-reference}.md`.
- Updated `docs/design.md`, `CHANGELOG.md` Unreleased, `README.md`, and `AGENTS.md`
  with the delivered extension and operational limits. Public support matrices
  remain unchanged; no Batch/background compatibility or live-provider guarantee
  is claimed.
- `internal/config/deferred_docs_test.go` extracts the actual documented examples,
  validates both roles with fixture secrets, and checks HCL/JSON/inline round
  trips, alias eligibility/minimum limits, and separate credentials without
  networking or a database.
- Agent passed focused fixture/config tests, `make vet test`,
  `make docs-contract`, `pnpm docs:build`, `git diff --check`, and link/anchor,
  metrics, formatting, and implementation-to-document review from repository root.
  Coordinator inspected the exact-example test and release note and reran
  `go test -count=1 -run 'DocumentedDeferred|Deferred' ./internal/config` plus
  `git diff --check` (passed).

### Task LWR-07: Verify end-to-end behavior and independently review the release

Status: completed

Review owner: LWR-07 independent release reviewer, separate from all implementation
and remediation agents. Initial review found R1; final re-review accepted R1/R2
resolution and the selected v1 release.

Kind: improvement

Priority: P1 — close only after the selected contract has integrated evidence.

Dependencies: LWR-10, LWR-06, LWR-11, LWR-12

Primary ownership:

- Hermetic peer-chain verification and acceptance review. The final reviewer
  is independent of the runtime implementation and integration agent; final
  acceptance and findings disposition are recorded below.

Finding:

Package-level correctness alone does not prove resilience across two proxies and
an idle-cutting intermediary.

References: all selected implementation tasks, `internal/e2e`,
`internal/integration`, and the repository `Makefile`.

Requirements:

1. Use a local mock model provider, two real proxy instances, and an intermediary
   fixture with a short controlled idle cutoff. Cover both JSON and the selected
   SSE mode, including silence before headers and a later pause. Demonstrate the
   baseline failure and the opted-in behavior without a live provider.
2. Exercise dropped submit replies, dropped result/stream transfers, restart and
   expiry, duplicate keys, response growth, slow/disappearing consumers,
   cancellation loss, reload, and shutdown. Include HTTP/1.1 and HTTP/2 where the
   approved transport supports them.
3. Verify policy parity: auth/model/guardrail/quota rejection, mixed aliases,
   non-opted-in requests, no duplicated usage or attempts, no private envelope
   leakage, bounded memory, and original response normalization.
4. Reconcile every acceptance criterion, status, and evidence entry. Any scope
   change must revise dependencies explicitly. A blocked check or missing reviewer
   is recorded rather than claimed complete.

Acceptance criteria:

- Hermetic integration proves the chosen contract through the intermediary,
  including later silence and transfer interruption.
- Required checks pass and an independent review records disposition of all
  material findings. Optional deployment work is separately pending/deferred.

Verification: shared final checks below plus criterion-by-criterion review.

Integration delivery and verification evidence:

- Added `internal/integration/deferred_binary_test.go` and
  `internal/integration/deferred_fixtures_test.go`. Two actual binaries and a
  local mock model demonstrate ordinary-path failures versus exact opted-in
  JSON/SSE delivery through a 150ms response-inactivity cutter with 450ms
  initial/later silence.
- Binary cases cover interrupted submit/read/terminal-manifest transfers without
  duplicate inference/accounting, actual crash/restart and old-generation
  rejection, SIGHUP success/failure/removal/drain/re-enable, frozen execution and
  caller credentials/transports, 1s execution and 10s terminal expiry,
  duplicate/conflicting keys, response growth, disappearing consumers, lost
  cancellation, shutdown, released ownership, and feature memory reservations.
- TLS/ALPN fixtures cover HTTP/1.1 and HTTP/2 peer controls, including slow partial
  HTTP/2 read-transfer deadline recovery. HTTP/2 terminates at the trusted
  intermediary; the executor's listener remains HTTP/1.1. This is not a claim
  of native TLS/HTTP/2 listener support.
- Binary policy coverage includes guardrail/auth/owner/no-opt-in/error parity;
  focused unit/race layers cover quota/current-model denial, mixed aliases,
  injected-clock expired signed scopes, blocked socket consumers, and
  allocation/finalizer bounds. Not every criterion is claimed as a binary test.
- Integration found and fixed reserved deferred headers leaking from ordinary
  responses in `internal/httpapi/response.go`. Added
  `TestDeferredReservedResponseHeadersStrippedWithoutOptIn` in
  `internal/httpapi/deferred_response_test.go`; agent demonstrated pre-fix failure
  and post-fix success. Coordinator inspected the correction and independently
  reran the focused regression and `git diff --check` (passed).
- Integration agent passed from repository root, with shared-output rebuilds
  serialized: `make build`;
  `go test ./internal/httpapi -run '^TestDeferredReservedResponseHeadersStrippedWithoutOptIn$' -count=1`;
  `go test -tags=integration ./internal/integration -run '^TestBinaryDeferred' -count=1 -v -timeout=5m`;
  `go build -race -o dist/aiproxy-lwr07-race ./cmd/aiproxy`;
  `AIPROXY_BINARY="$PWD/dist/aiproxy-lwr07-race" go test -race -tags=integration ./internal/integration -run '^TestBinaryDeferred' -count=1 -v -timeout=8m`.
  Here `$PWD` is the repository checkout; both fixtures and child binaries were
  race-instrumented.
- Required post-fix shared checks passed in the integration session:
  `make vet test`, `make test-race`, `make integration`, `make docs-contract`,
  `pnpm docs:build`, and `git diff --check`.
- Optional database-backed tests skipped with `AIPROXY_TEST_DATABASE_URL` unset.
  No live-provider or deployment evidence is claimed; memory checks concern
  feature-owned reservations, not total Go heap/RSS.
- Independent final acceptance/security review initially remained the only selected
  gate; its finding and current disposition are recorded below. Integration
  delivery alone does not complete LWR-07.

Initial independent review evidence and disposition (historical):

- Independent reviewer confirmed R1 (P1), failed deferred SSE ending as clean HTTP
  EOF after commitment; see reopened LWR-11. A temporary overlay regression failed
  3/3 with the race detector. No repository files were edited by the reviewer.
- No additional material findings were identified in the inspected negotiation,
  dedup, ownership/current authorization, static-only, alias eligibility,
  allocation, finalization, reload/drain, and resource-lease paths. The reserved
  response-header correction and documented HTTP/2 termination boundary were
  accepted. This is a scoped review, not an exhaustive audit.
- Reviewer passed from repository root:
  `go test -race ./internal/deferred ./internal/provider ./internal/httpapi ./internal/app ./internal/config ./internal/configedit ./internal/dbmerge ./cmd/aiproxy -run 'Deferred|Execution|RuntimeResource|Context' -count=1 -timeout=5m`;
  `go test -race ./internal/deferred ./internal/payloadlog ./internal/mongolog ./internal/providerhealth ./internal/upstreamhttp ./internal/observability -count=1 -timeout=3m`;
  a race build and race-instrumented binary/fixture subset covering inactivity,
  dropped transfers, crash generations, reload/drain, and guardrail/normalization;
  `make docs-contract`; and `git diff --check`. Temporary machine-specific binary
  and overlay paths are intentionally omitted.
- Blocker owner: separate LWR-11 committed-stream remediation agent. Closure
  requires the regression to pass, affected release checks after correction,
  and independent re-review accepting the final disposition.
- R1 correction and post-fix verification were delivered by separate agents;
  one uncached race-suite cleanup assertion failed and is tracked as R2 under
  LWR-10. Rerun success alone is not its disposition. Final acceptance remains
  blocked until R2 investigation/correction and original-reviewer re-review.

Final independent acceptance and completion evidence:

- **PASS:** original independent reviewer accepted R1 as fixed and R2 as resolved;
  no material unmet criterion remains for the selected v1 release. Previously
  accepted negotiation, ownership, bounds, lifecycle, documentation, and binary/
  intermediary gates remain accepted. No scope or dependency change was needed.
- R1's marked deferred streams abort failed wire delivery after normal stream
  finalization. Definite inference errors retain health/known-usage/accounting
  semantics, ordinary streaming is unchanged, and no replacement inference occurs.
- R2's deterministic release notifications distinguish pipe EOF/result Close from
  actual retained translator/source drain, without weakening live accounting or
  changing production ownership.
- Re-review independently passed the original temporary-overlay reproducer plus
  permanent committed-inference/control-loss/private-header socket regressions
  with `-race -count=3`; overlay paths are intentionally not recorded here.
- Re-review independently passed, from repository root:
  `go test -race ./internal/provider -run '^(TestExecutionBudgetDrainWaitRequiresEveryReservation|TestExecutionSSECloseKeepsBudgetUntilTranslatorCleanup|TestExecutionLimitsAllChatAdaptersPreserveJSONSSEAndNilPaths|TestExecutionSSECloseDuringBlockedReadKeepsBudgetUntilReaderAndWorkerExit|TestExecutionSSEMinimumWorkBudgetAcceptsNormalStreams)$' -count=20 -timeout=3m`;
  `go test -race -count=1 -timeout=3m ./internal/provider ./internal/httpapi ./internal/deferred ./internal/app ./internal/e2e`;
  `git diff --check`; and formatting checks on the six R1/R2 correction files.
- Shared post-R1 production verification passed `make integration` (fresh
  UI/binary build), `make docs-contract`, and `pnpm docs:build`; post-R2 test-only
  verification passed fresh `GOFLAGS=-count=1 make vet test` and
  `GOFLAGS=-count=1 make test-race`. Detailed initial failures and corrective
  evidence are retained above; they are not hidden by the final passing result.
- Coordinator reconciled these delivered changes, checks, and independent
  disposition, then completed LWR-10/LWR-11/LWR-07. All selected definition-of-done
  items are satisfied. Optional deployment work and unselected alternatives retain
  their separate blocked/deferred statuses.
- Documented single-process/restart loss, cancellation/billing limits, bounded
  local-filesystem blocking, optional database checks, and no live-provider or
  total heap/RSS claims remain unchanged.
- Final coordinator validation passed for all 12 stable task IDs, required fields,
  completed prerequisites, acyclic dependencies, local links, and private-path
  constraints. Final `make docs-contract` and `git diff --check` passed.

## Shared verification

Run commands from the repository root. Use the Go toolchain declared in `go.mod`;
build/website checks require the Node/pnpm versions and installed dependencies
declared by the repository. Race checks require a supported toolchain/platform.
Replace any documentation placeholders before executing examples.

- Each implementation task runs the focused package tests listed above, including
  race tests where concurrency/lifecycle changes.
- Final integration runs `make vet test`, `make test-race`, `make integration`,
  `make docs-contract`, and `pnpm docs:build`.
- `make integration` already depends on the binary/web build; serialize checks
  that rebuild shared artifacts rather than rebuilding them concurrently.
- Use injectable clocks/barriers for state-machine tests and short bounded
  network fixtures. Do not require real idle windows, live providers, or cluster
  credentials in CI.
- Record executed commands and outcomes separately from planned checks. Do not
  mark an implementation complete while its required verification is blocked.

## Definition of done

- LWR-08 has a complete, recorded contract and explicit selected scope.
- Selected configuration, store, executor, caller, and documentation tasks meet
  their acceptance criteria; ordinary requests retain existing behavior.
- Long silent waits and interrupted deliveries are covered by hermetic chain
  tests, with finite execution/memory limits and explicit loss/retry semantics.
- Shared final checks pass and LWR-07 records the acceptance review.
- Optional investigations/remedies and unselected alternatives have truthful
  statuses; their completion is not a prerequisite for the generic feature.

## Pre-implementation review evidence

- Reviewed the source/test boundaries and external references listed above.
- Preserved LWR-01 through LWR-07 identifiers. Added LWR-08 through LWR-12 to
  separate contract, configuration, storage/lifecycle, caller transport, and docs.
- Only this task document was edited. No feature implementation or runtime
  verification has been performed during this review.
- Verified from the repository root: `make docs-contract` passed
  (`documentation contract matrices match`).
- A read-only document check passed for all 12 task IDs, required fields,
  references, acyclic dependencies, local Markdown links, generic/private-path
  constraints, and whitespace.

## Implementation execution record

- User explicitly authorized implementation via subagents.
- User selected incremental finite polls for streaming, a dedicated versioned
  peer API, authenticated single-process storage, and static-config-only
  provider opt-ins. Unsupported managed-provider declarations/inheritance must
  fail explicitly; no public Batch/Responses background compatibility.
- User approved the remaining conservative v1 rules: exclude
  `forward_authorization` from deferred opt-ins; require a local opt-in on static
  derivatives; keep non-chat operations on ordinary transport; use configurable
  1 MiB request/8 MiB result defaults; retain the existing local-filesystem logging
  assumption while bounding remote finalization calls.
- Contract recommendation selected: signed 30-second submission scopes and a
  process generation, byte-offset finite reads for JSON/SSE, terminal result
  manifests, no execution queue, protected dedup/tombstones, and separate
  inference/control outcomes. Local filesystem payload I/O has no hard
  cancellation guarantee; no goroutine-per-timeout workaround is permitted.
- LWR-08 contract agent assigned before runtime implementation. Deployment
  investigation/remedy work is blocked/deferred independently of the feature.
- Baseline `make vet test` passed from the repository root before runtime edits
  (Go 1.27.4; all unit packages passed).
- LWR-07 fixture exploration agent assigned read-only to the existing integration
  harness, independently of contract design. This is preparation, not completion
  of LWR-07.
- Fixture exploration completed: reuse the binary integration launch/config
  helpers; add a per-request response-inactivity cutter (not server idle timeout),
  scripted context-aware model gates, and authenticated incremental consumption.
  Binary tests are needed for actual restart/SIGHUP; supplementary two-app tests
  must retain and close each App. No runtime tests were run by the exploration
  agent.
- Baseline `pnpm docs:build` passed from the repository root before documentation
  implementation (Node 26.13.0, pnpm 11.24.0).

Planned allocation (assignment alone is not completion evidence):

| Role                      | Primary task | Boundary                                                                             |
| ------------------------- | ------------ | ------------------------------------------------------------------------------------ |
| Contract agent            | LWR-08       | Design analysis and `docs/design.md`; coordinator records the selected contract here |
| Configuration agent       | LWR-09       | Config/parser/editor/managed-boundary validation and focused tests                   |
| Storage/lifecycle agent   | LWR-10       | Bounded deferred package and app lifecycle                                           |
| Executor agent            | LWR-06       | HTTP admission, policy parity, detached finalization, control endpoints              |
| Caller agent              | LWR-11       | Peer transport, retrieval, typed retry boundaries                                    |
| Documentation agent       | LWR-12       | Public docs and release notes                                                        |
| Integration/review agents | LWR-07       | Hermetic chain fixtures and independent acceptance/security review                   |

Runtime agents will be assigned only after their prerequisites are reconciled.
Shared runtime files and build outputs are serialized; the coordinator alone
updates task statuses and completion evidence.

- Configuration read-only preparation encountered a provider token-rate-limit
  error before edit authorization. LWR-08 is now verified; resume LWR-09
  sequentially, using the finalized design section as its contract.
- The first LWR-10 storage/lifecycle agent stopped on an encrypted-response
  verification error after delivering partial store/resource code and tests.
  Coordinator preserved and inspected the changes; the current
  `go test -count=1 ./internal/deferred ./internal/app ./internal/httpapi` passed.
  A fresh reconciliation agent was launched to review/finish these changes before
  the deferred adapter-allocation handoff. At that point LWR-10 remained in progress.
- The fresh LWR-10 reconciliation agent also stopped on the provider's
  `invalid_encrypted_content` error. The user requested a checkpoint/status update
  rather than further delegation. Partial changes are preserved; no alternate
  subagent model was selected and no replacement agent was launched afterward.
- User subsequently requested completion of the remaining feature tasks by
  sequential subagents. Resumed with a fresh LWR-10 reconciliation/allocation
  agent; all existing uncommitted work is preserved. This continuation follows
  LWR-10 → LWR-06 → LWR-11 → LWR-12 → LWR-07, with coordinator reconciliation
  before each handoff and an independent final reviewer.
- The resumed LWR-10 allocation agent stopped on a provider token-rate limit after
  delivering additional code/tests. Coordinator focused and race checks passed
  for the preserved snapshot; a fresh LWR-10 completion/reconciliation agent was
  assigned next, without starting any dependent task.
- The fresh LWR-10 completion/reconciliation agent completed its review and bounded
  header-token fix; focused/race checks and `make vet test` passed. Coordinator
  reconciled the report, completed LWR-10, and assigned LWR-06 next.
- LWR-06 executor integration agent stopped on a provider token-rate limit after
  partial implementation. Coordinator verified the delivered snapshot and recorded
  its failing unary-metadata regression; fresh LWR-06 completion/reconciliation
  proceeds sequentially before any caller assignment.
- The LWR-06 completion/reconciliation agent delivered the unary fix and expanded
  boundary tests before another provider token-rate-limit interruption. Coordinator
  focused/race checks and `make vet test` passed for this snapshot. Fresh executor
  readiness reconciliation follows; LWR-11 is still unassigned.
- Fresh executor readiness agent found no material remaining LWR-06 criterion;
  its focused checks and whitespace validation passed. Coordinator reconciled
  that evidence, completed LWR-06, and assigned LWR-11 next.
- LWR-11 caller agent delivered transport and chain tests before a provider
  token-rate-limit interruption. Coordinator focused/race checks, `make vet test`,
  and whitespace validation passed; fresh caller readiness reconciliation is next,
  without assigning documentation or final review prematurely.
- Fresh caller readiness agent found no material LWR-11 gap and independently
  passed targeted caller/chain/transport tests and race checks. Coordinator
  reconciled completion and assigned LWR-12 documentation next.
- Initial LWR-12 agent stopped on a provider token-rate limit without delivery.
  Coordinator docs-contract/website baseline checks passed; a fresh documentation
  agent is assigned with focused reads. No dependent integration agent is running.
- Fresh LWR-12 documentation agent delivered the feature docs and exact-example
  validation fixtures; required builds/tests and coordinator fixture checks passed.
  Coordinator completed LWR-12 and assigned LWR-07 integration verification next.
  Independent final release review will run in a separate subsequent session.
- LWR-07 integration agent delivered binary/TLS/HTTP2/adversarial evidence and all
  shared checks passed after its reserved-response-header correction. Coordinator
  reconciled the correction with LWR-06/LWR-09 and verified its focused regression.
  Following a server restart, saved changes and completion evidence were inspected
  without repeating implementation; a fresh independent final reviewer is next.
- Independent final reviewer completed a read-only acceptance/security review and
  identified R1, a confirmed failed committed-stream delivery bug. Coordinator
  reopened LWR-11, blocked LWR-07 on that prerequisite, and assigned a separate
  remediation agent. Prior passing checks remain historical evidence, not proof
  the newly discovered branch meets acceptance.
- The R1 remediation agent stopped on a usage limit after saving a focused
  delivery-marker correction and permanent regression tests. Coordinator passed
  the new regression and original independent reproducer three times each with
  race instrumentation, preserved all work, and assigned fresh remediation
  reconciliation/verification before the original independent reviewer returns.
- Fresh R1 reconciliation completed without edits and passed the required release
  checks, but reported an intermittent uncached Gemini allocation cleanup
  assertion. Coordinator preserved the failure as R2, reopened LWR-10 verification
  for bounded investigation/remediation, and kept LWR-07 blocked rather than
  treating passing reruns as evidence of a resolved cause.
- R2 investigation isolated faulty test synchronization and delivered
  deterministic cleanup/drain tests without production changes. Fresh uncached
  checks passed; coordinator inspected and verified the new race regressions.
  The original independent reviewer returns next to recheck R1/R2 and decide
  final acceptance without repeating already accepted implementation.
- Original independent reviewer completed re-review with **PASS**, accepted R1/R2
  disposition, and found no material unmet selected criterion. Independent
  repeated socket/cleanup race tests passed. Coordinator reconciled the final
  evidence and closed all selected feature tasks; no commits or infrastructure
  mutations were performed.

## Original Fresh-Session Continuation Checkpoint

Historical checkpoint from the earlier pause; task statuses and continuation
evidence above supersede its allocation and next-action notes.

All paths/commands below are repository-relative and commands run from the
repository root. Read `AGENTS.md`, this checkpoint, and
`docs/design.md#peer-deferred-transport-v1` first. Preserve the current uncommitted
tracked changes and untracked new files; no commit/reset/revert was requested.

| Task                              | Checkpoint                                                                                              |
| --------------------------------- | ------------------------------------------------------------------------------------------------------- |
| LWR-01 / LWR-02                   | Blocked on deployment/original-request evidence; not generic feature gates                              |
| LWR-03                            | Deferred operator remedy; LWR-01 prerequisite is unsatisfied                                            |
| LWR-04 / LWR-05                   | Deferred, unselected heartbeat/bridge alternatives                                                      |
| LWR-08                            | Completed; user choices and authoritative v1 contract recorded                                          |
| LWR-09                            | Completed; focused config/editor/managed-boundary checks passed                                         |
| LWR-10                            | Blocked/partial; store and resource scaffolding exists, adapter allocation limits/reconciliation remain |
| LWR-06 / LWR-11 / LWR-12 / LWR-07 | Pending; do not skip their prerequisites or claim release completion                                    |

### Next concrete action

1. In a fresh implementation session, inspect current changes with
   `git status --short` and `git diff`; include the untracked
   `internal/deferred/` and tests when reviewing (ordinary `git diff` omits them).
2. Assign a fresh LWR-10 reconciliation/allocation-boundary agent. Do not reuse
   interrupted child-session context. Keep shared app/handler code serialized.
3. Reconcile existing store/lifecycle invariants, then implement only missing
   deferred adapter bounds in `internal/provider/`: `ExecutionLimits`/request
   plumbing, bounded reads and work reservations, structural JSON/SSE limits,
   and capture-prefix snapshots. Preserve ordinary adapter paths.
4. Run focused tests/race checks for `internal/deferred`, `internal/provider`,
   `internal/app`, and `internal/httpapi`. Mark LWR-10 completed only when its
   actual acceptance criteria pass; record any new gap/blocker.
5. Then delegate LWR-06 → LWR-11 → LWR-12 → LWR-07. Final independent review and
   shared release verification have not happened.

### Existing interfaces for the handoff

- Config: `config.Deferred`, `DeferredTransport`, `DeferredReceiver`,
  `DefaultDeferred()`, `DefaultDeferredTransport()`, `ValidateDeferred()`,
  `ValidateDeferredConfig()`, `ValidateManagedProviderDeferred()`,
  `IsDeferredHeader()`.
- Store setup: `deferred.New(ctx, options, hooks...)`, `OptionsFromConfig`,
  `DefaultOptions`, `Store.Generation/Options/Stats`, injectable clock/random hooks.
- Control/input capacity: `Store.AcquireControl(owner, deleting)`,
  `Store.AcquireIngress(owner, contentLength)`, `Control.Encode/Release`,
  `Ingress.ReadSubmit/Release`. Keep scratch leases through actual I/O completion.
- Admission: `Store.Capability(control, model, receiverLimits)`,
  `Store.Begin(ingress, model, submit)` returns a preparation or replayed receipt.
  `Preparation.Input()` returns borrowed input plus a release callback;
  `WorkBudget`, `Reject`, `Abort`, and `Commit(work, releaseRuntime)` exist.
- Worker: `Writer.Input/WorkBudget/SetSource/SetResponse/Write/Complete/SetFinalizer`.
  Transfer an already acquired runtime lease to the accepted job; do not reacquire
  an unleased retired dependency snapshot or double-release the handler's lease.
  Finalization credit/work reservations remain owned until the actual callback
  and source cleanup finish.
- Retrieval: `Store.Model/Status/Result/Read/Delete(control, generation, id, ...)`.
  Authenticate and authorize the admitted model before exposing status/body or
  deleting; `Store.Model` supports lookup without target reselection.
- Reload/shutdown: `Store.PrepareUpdate(...).Commit(activation)/Cancel`,
  `BeginShutdown`, `Close(ctx)`; app `DeferredStore()` accessor and lazy store
  activation exist. Handler `SnapshotDependenciesWithLease` and
  `ActivateDependencies`/`UpdateDependenciesWithActivation` exist.
- Wire/schema: authoritative types/constants in `internal/deferred/types.go`,
  `DecodeSubmit`, `DecodeControl`, structural/header/identity validation.
  Internal `ProtocolError{Status, Wire}` wraps the neutral control-error DTO.

### Checkpoint verification

- Passed before the pause: focused configuration/editor/CLI/dbmerge/HTTP tests;
  partial `go test -count=1 ./internal/deferred ./internal/app ./internal/httpapi`;
  `make docs-contract`; `git diff --check`.
- Checkpoint `make vet test` passed (all unit packages, including the new deferred
  package). Focused
  `go test -race -count=1 ./internal/deferred ./internal/app ./internal/httpapi`
  also passed. These verify the partial code snapshot, not the still-unimplemented
  deferred HTTP/caller feature.
- Baseline `pnpm docs:build` passed before runtime changes. No feature-complete
  `make integration`, full `make test-race`, final website build, or independent
  runtime acceptance review has been performed.
- Database-backed admin tests remain optional/unrun without
  `AIPROXY_TEST_DATABASE_URL`; hermetic pre-mutation/static-only checks passed.
- Checkpoint task-document validation passed: 12 stable IDs, required fields,
  current status assertions, acyclic dependencies, local links, private/generic
  path constraints, and whitespace. `git diff --check` passed.
- No live provider or deployment diagnosis was performed. No commits were made.
