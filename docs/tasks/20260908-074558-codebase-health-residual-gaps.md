# Codebase Health Residual Gaps

Created: 2026-09-08 07:45:58 local time

## Objective

Close concrete residual security, accuracy, resource-safety, configuration, and
lifecycle gaps. Improve readability, encapsulation, reuse, and testability at
the existing enforcement boundaries rather than introducing broad abstractions.
This is an executable plan for sub-agents, not authorization already exercised
to implement the recommendations.

Scope: Go HTTP dispatch and observability, provider translation and usage,
configuration editing, file persistence, dashboard reload, daemon lifecycle,
and catalog lookup performance. Non-goals: new providers, dynamic plugins,
replacement of HCL or the transport stack, cosmetic file splitting, paid/live
provider testing, and wholesale rewrites of previously completed work.

## Review Evidence

- Three non-overlapping static reviews covered server/auth/routing/health,
  adapters/streaming/usage, and config/secrets/CLI/lifecycle. Coordinator review
  additionally inspected catalog cloning, resolver lookup, persistence recovery,
  usage parsing, config mutation, and current verification commands.
- Findings below are confirmed by source control flow unless marked
  `investigation`. Failure scenarios are not newly executed reproductions.
  Referenced tests were inspected; absence of coverage refers to that inspected
  scope, not a proof that no related test exists anywhere.
- Initial `git status --short` showed an unrelated modification to `.gitignore`.
  Preserve it. This review creates only this task document.
- Actually run: `go version` returned `go1.27.1 linux/amd64`, matching the current
  Go selection in `.tool-versions`; `make docs-contract` passed.
- Not run: unit/race tests, vet, builds, binary integration, fuzzing, allocation
  benchmarks, Redis fault experiments, or live-provider/SDK tests. This is a
  planning deliverable; those checks are required during execution as below.
  Historical passing results in older plans are not a fresh baseline.
- Not comprehensively reviewed: deployment/IaC, release supply chain, website,
  dependencies, interactive TUI behavior, or every supported provider payload.
  This document is not a comprehensive security certification.

## Prior Work

These are new residual cases, not duplicate assignments of completed tasks.
Use plan dates to disambiguate reused historical task IDs.

| Existing plan                                                        | Relationship                                                                                                                                                                            |
| -------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `docs/tasks/20260804-125911-codebase-health-review-remediation.md`   | Completed OBS-01 bounded paths, not HTTP methods; STORE-01 did not cover failed recovery; LIFE-01 did not make restart one operation; HEALTH-01 did not report write errors.            |
| `docs/tasks/20260823-112437-codebase-health-follow-up.md`            | Preserve bounded pass-through observation, exactly-once attempts, catalog immutability, and provider policy boundaries. No replacement registry or repeated catalog encapsulation task. |
| `docs/tasks/20260809-132127-remediation-decision-closure.md`         | Preserve Redis read-fallback and local-only dashboard decisions.                                                                                                                        |
| `docs/tasks/20260809-132640-configurable-upstream-header-timeout.md` | CFG-01 below fixes root editing isolation, not timeout precedence.                                                                                                                      |
| `docs/tasks/20260907-104451-alias-upstream-retry-cooldown.md`        | ROUTE-02 below closes an eligible-target exhaustion case left by COOL-02.                                                                                                               |
| `docs/tasks/20260822-160713-provider-inheritance.md`                 | Keep inherited-provider behavior and completed integration evidence.                                                                                                                    |
| `docs/tasks/20260906-101616-opencode-provider-zen-go.md`             | Translator fixes must cover reuse by messages/gemini protocols without expanding service capabilities.                                                                                  |
| `docs/tasks/20260907-022659-github-copilot-device-flow.md`           | Existing live eligibility/token-lifetime gates remain there; mock coverage is not live compatibility evidence.                                                                          |

## Priorities And Coordination

Use the existing health-review rubric: P0 for remotely triggerable unbounded
resource growth or severe credential/process safety; P1 for production
availability, correctness, or secret integrity; P2 for operational contracts or
bounded architectural/performance issues; P3 for low-risk improvements.
Severity describes the demonstrated code path, not measured production incidence.

All tasks start `pending`. An agent must claim a ready task as `in_progress`,
record its owner, and append commands/results before marking it `completed`.
Use `blocked` with a named decision/prerequisite if verification or policy is
unavailable. Investigations may complete with an evidence-backed no-change
decision. Preserve historical findings and append resolutions rather than
rewriting them away. Coordinator alone reconciles this shared task file and
shared documentation; workers provide completion notes.

| Agent lane                   | Execution order                          | Shared hotspots                                                                   |
| ---------------------------- | ---------------------------------------- | --------------------------------------------------------------------------------- |
| HTTP/observability           | SEC-01, ROUTE-02, HEALTH-03              | `internal/httpapi/handler_test.go`, `internal/observability/metrics.go`           |
| Provider protocols           | STREAM-02, STREAM-03, USAGE-01, USAGE-02 | `internal/provider/provider_test.go`, `responses.go`, `anthropic.go`, `gemini.go` |
| Persistence/config/lifecycle | STORE-02, CFG-01, DASH-02, LIFE-02       | `internal/configedit`, `cmd/aiproxy` tests; app and token persistence             |
| Performance investigator     | PERF-01, after runtime correctness lanes | `internal/config/catalog.go`, `internal/modelresolver/resolve.go`                 |
| Independent reviewer         | REVIEW-02 last                           | Acceptance evidence and shared public documentation                               |

The three implementation lanes may run in parallel with disjoint primary
ownership. Dependencies within lanes include deliberate edit serialization,
not just behavioral prerequisites. Do not concurrently edit the same test file
or run builds that replace `dist/` outputs. If a fix needs another lane's files,
coordinate ownership before editing. Put new focused tests in task-specific
files when that is clearer, not simply to avoid integration review.

## Executable Tasks

### Task SEC-01: Bound HTTP Method Metric Labels

Status: completed

Owner: sub-agent ses_f7e0253b2ffe0MBmFvGAHJH4mg (HTTP observability/security engineer)

Completion evidence:

- Changed: `internal/observability/metrics.go` (NormalizeHTTPMethod, applied in RecordHTTP/RecordHTTPSize/RecordHTTPStream/RecordHTTPError; Help text documents UNKNOWN bucket); new `internal/observability/metrics_method_test.go`; new `internal/httpapi/http_method_cardinality_test.go` (300 distinct methods x unknown/health/dashboard surfaces, both auth modes, no scrape token; asserts bounded method values <=4, UNKNOWN present, no raw CUSTOMMETHOD labels in counters or histogram buckets; GET/HEAD preserved).
- Method set: GET, HEAD, POST, PUT, PATCH, DELETE, CONNECT, OPTIONS, TRACE + UNKNOWN.
- Verification: `go vet ./internal/httpapi/ ./internal/observability/` pass; `go test ./internal/observability/ ./internal/httpapi/ -count=1` ok; `go test -race ./internal/httpapi/ ./internal/observability/ -count=1` ok (httpapi 1.884s, observability 1.112s); fail-before check (stashed fix, kept tests) FAILs on raw labels, passes after; `gofmt -l` clean.

Kind: defect

Priority: P0; unauthenticated extension methods can create persistent unbounded
metric series whenever the listener is reachable.

Suggested agent: HTTP observability/security engineer

Dependencies: none

Primary ownership: `internal/observability/metrics.go`, focused metric tests,
and method-cardinality regressions in `internal/httpapi`.

Finding: the handler records raw `r.Method`, including for unknown routes before
authentication. The shared metric writers use it unchanged in counters and
histograms. Paths are normalized, but distinct valid methods such as `CUSTOM1`
and `CUSTOM2` each allocate new series. Metrics collection is attached even
without a scrape token; protecting `/metrics` does not bound collection.

References:

- `internal/httpapi/handler.go:198-225` (`ServeHTTP`).
- `internal/httpapi/response.go:137-140` (HTTP error metrics).
- `internal/observability/metrics.go:345-377` (`RecordHTTP`, size/error writers).
- `internal/app/app.go:88-101` (metrics assembly).
- `internal/httpapi/handler_test.go:980-1046`: existing closed-route/cardinality tests vary paths, not extension methods.

Requirements:

1. Normalize methods at the shared metric boundary to a finite standard-method
   set plus one unknown label, consistently across all HTTP metric families.
2. Preserve routing, authentication, useful standard labels, and path bounds.
   Document the label change; do not reject requests solely to fix observation.

Acceptance criteria:

- Hundreds of distinct extension methods do not grow series counts after the
  first unknown-method request, under both auth modes and without a scrape token.
- Unknown paths and health/dashboard method rejection contain no raw extension
  method labels; standard GET/POST/HEAD and other selected standard labels remain.
- A regression fails before normalization and passes afterward; metric family
  gathering checks counters and histograms, not just one writer.

Verification: `go test -race ./internal/httpapi ./internal/observability`.

### Task STORE-02: Preserve Recovery Copies When Rollback Fails

Status: completed

Owner: sub-agent ses_f7dfe3af6ffe1HpdpuBOXYvKYZ (filesystem persistence engineer)

Completion evidence:

- Changed: `internal/filestore/filestore.go` (ReplaceError/RecoveryFailure with Unwrap []error, atomic restore via rename without pre-delete, injectable remove/rename/syncDir boundary, retention-aware publish/rollback/sync/cleanup); narrowed existing rename-failure hook in `filestore_test.go` to publish-only; new `internal/filestore/filestore_recovery_test.go` (publish+restore failure, earlier-committed restore, successful rollback, absent originals, cleanup/sync failure retention; asserts retained paths without contents, modes preserved).
- Verification: `go test ./internal/filestore/ -count=1` ok; `go test -race ./internal/filestore ./internal/configedit -count=1` ok (1.276s/1.208s); `go vet` pass; `gofmt -l` clean.

Kind: defect

Priority: P1; failed filesystem recovery can destroy the only remaining copy
of secrets or configuration.

Suggested agent: filesystem persistence engineer

Dependencies: none

Primary ownership: `internal/filestore/filestore.go`, its tests, and focused
`internal/configedit` persistence integration tests.

Finding: `ReplaceFiles` ignores restoration errors; rollback deletes a published
replacement before restoring its backup, and deferred cleanup then deletes
backups even when restoration failed. If secrets publication succeeds, config
publication fails, and secrets restoration fails, neither secrets destination
nor its old backup need remain. This is error-path data loss, distinct from the
documented absence of crash-atomic multi-file transactions.

References:

- `internal/filestore/filestore.go:56-112,212-228` (`ReplaceFiles`, cleanup, rollback).
- `internal/configedit/configedit.go:425-444` (`WriteProviderFiles`).
- `internal/filestore/filestore_test.go:48-74` (`TestReplaceFilesRollsBackRenameFailure`) injects publication failure but not failed restoration.

Requirements:

1. Surface recovery errors with the original failure and preserve recoverable
   copies until restoration/publication and required durability steps succeed.
2. Avoid deleting a valid replacement before atomic restoration where supported.
   Use the same injectable filesystem boundary for publish and restore paths.
3. Describe retained recovery paths in errors without exposing file contents;
   preserve restrictive modes, symlink defenses, and existing lock ownership.

Acceptance criteria:

- Deterministic publication-plus-restoration failure leaves a complete old or
  new copy recoverable, with retained backups not removed by deferred cleanup.
- Tests cover restoration of both the currently failing file and an earlier
  committed file, successful rollback, absent original files, and cleanup success.
- Errors identify primary and recovery failures without secret contents.

Verification: `go test -race ./internal/filestore ./internal/configedit`.

### Task STREAM-02: Propagate Translated Stream Errors And Premature EOF

Status: completed

Owner: sub-agent ses_f7df97378ffelWYbQ5PhMGRL1r (streaming protocol engineer)

Completion evidence:

- Changed: `internal/provider/anthropic.go` (anthropicStreamError, processAnthropicEvent returns done+err, tracks message_stop, premature EOF → truncated error, no synthetic response.completed); `internal/provider/gemini.go` (geminiStreamError, checked first, premature EOF → truncated, no synthetic DONE/completed); updated `TestAnthropicStreamTranslatesFragmentedEOFEvent` to expect truncated error; new `internal/provider/stream_error_test.go` (17-case table chat/Responses Anthropic/Gemini errors before/after content, premature EOF, valid+EOF-delimited terminals exactly once, ping ignored, upstream closure; cancellation-distinct test; OpenCode Zen messages/Gemini reuse).
- Verification: targeted run PASS (17 table + cancellation + 4 OpenCode); `go test -race ./internal/provider/ -count=1` ok (9.447s); `go vet` pass; `gofmt` clean. HTTP-outcome assembly deferred to REVIEW-02.

Kind: defect

Priority: P1; upstream failures can become empty or partial successful responses.

Suggested agent: streaming protocol engineer

Dependencies: none

Primary ownership: `internal/provider/anthropic.go`, `gemini.go`, shared stream
error/completion types only as needed, and translated-stream tests.

Finding: Anthropic event processors ignore `error` events; Gemini error envelopes
decode as empty successful candidates. EOF can then manufacture successful
Responses completion or chat termination. An Anthropic `event: error` with
`{"type":"error","error":{"type":"overloaded_error","message":"busy"}}`
is ignored instead of failing the translated stream. OpenCode translated paths
reuse these processors.

References:

- `internal/provider/anthropic.go:349-359,374-514` (stream EOF and processors).
- `internal/provider/gemini.go:504-549,566-574,632-647` (EOF and response decoding).
- `internal/provider/provider_test.go:1168-1259,1375-1439,1482-1495`: happy/fragmented streams and pass-through error coverage; no equivalent translated error coverage in these tests.
- `internal/provider/opencode_test.go:319-356,423-456`: translated happy paths.

Requirements:

1. Recognize protocol error envelopes and propagate them through the existing
   pipe/completion boundary; do not attempt to replace already-sent HTTP headers.
2. Track protocol success separately from transport EOF. Do not synthesize
   success for truncated streams. Preserve valid EOF-delimited final events and
   ignorable non-error extensions.
3. Update the existing Anthropic fragmented-EOF expectation deliberately where
   it currently accepts a missing terminal message. Document/release-note this
   externally observable tightening of translated stream semantics.

Acceptance criteria:

- Errors before and after content cause a non-nil stream read error, upstream
  closure, and no later successful terminal event.
- Premature EOF does not emit `response.completed`; valid terminal streams
  complete exactly once and valid EOF-delimited terminal events still work.
- Table-driven chat/Responses coverage includes Anthropic, Gemini, and relevant
  OpenCode reuse; cancellation remains distinguishable from upstream failure.

Verification: `go test -race ./internal/provider`; assembled HTTP outcome
verification is also required at REVIEW-02.

### Task ROUTE-02: Preserve Terminal Responses When Fallbacks Are Cooling

Status: completed

Owner: sub-agent ses_f7df437f6ffexwBNDxGiRX7Wp4 (alias routing/concurrency engineer)

Completion evidence:

- Changed: `internal/httpapi/dispatch.go` (pending retryable-result retention, commit-based RecordAliasRetry only on actual subsequent attempt, synthetic all-cooling precedence preserved, transport-error deferral, ErrInvalidRequest closes pending); new `internal/httpapi/route02_pending_test.go` (6 terminal + 2 failover + 2 synthetic cases, both round_robin and least_connections).
- Cases: 429 no advice / zero advice / retryable 5xx with cooling B → returns A's original status/body, 1 call, no retry metric; eligible B failover 2 calls + 1 retry; valid advice all-cooling → synthetic 429 + Retry-After + 1 call. Gauges zero, leases released.
- Verification: `go vet` pass; `go test ./internal/httpapi/ -count=1` ok; fail-before (stashed dispatch) FAILs with generic 502, passes after; `go test -race ./internal/alias ./internal/modelresolver ./internal/httpapi -count=1` all ok (1.035s/1.076s/1.656s); gofmt/diff-check clean.

Kind: defect

Priority: P1; an upstream retryable response becomes generic 502 without an
actual fallback attempt, losing the original status/body and overstating retries.

Suggested agent: alias routing/concurrency engineer

Dependencies: SEC-01

Primary ownership: `internal/httpapi/dispatch.go`, `internal/httpapi/cooldown_test.go`,
and focused attempt-accounting tests.

Finding: with alias target B already cooling, A returning retryable 429 without
valid retry advice leaves A non-cooling. `hasUntriedTarget` sees B, closes A's
response, and records a retry. Acquisition then excludes tried A and cooling B;
the handler returns generic 502 despite only one upstream call. All-cooling
synthetic 429 logic does not apply because A never entered cooldown.

References:

- `internal/httpapi/dispatch.go:142-165,212-215,346-371` (eligibility, retry, exhaustion).
- `internal/httpapi/handler.go:290-303` (dispatch error mapping).
- `internal/httpapi/cooldown_test.go:250-279,351-383,580-597`: all-cooling, mixed-health/no-call, and usable-fallback tests do not cover this combination.

Requirements:

1. Retain a pending retryable result until another eligible attempt is committed
   or exhaustion is resolved. Return the terminal upstream response when only
   cooling alternatives remain; preserve synthetic all-cooling precedence.
2. Increment retry metrics for actual subsequent attempts, not merely configured
   untried targets. Preserve direct routing and no-response/all-unhealthy behavior.

Acceptance criteria:

- For both algorithms, cooling B plus A's retryable 429 without advice returns
  A's original status/body, calls only A, and records no retry. Include malformed
  and zero advice and a retryable 5xx case.
- Eligible B still receives failover; valid A advice completing all-cooling
  coverage still yields the specified synthetic 429 and retry headers.
- Bodies close exactly once and leases/in-flight gauges return to zero.

Verification: `go test -race ./internal/alias ./internal/modelresolver ./internal/httpapi`.

### Task STREAM-03: Bound Retained Responses Output Across Events

Status: completed

Owner: sub-agent ses_f7ded4c24ffe3y3rui9VuXTWQq (provider resource-safety engineer)

Completion evidence:

- Changed: `internal/provider/responses.go` (1 MiB retained-text budget doc, ErrResponsesOutputOverflow, appendText + pre-framing check in writeResponsesDelta); new `internal/provider/responses_overflow_test.go` (accumulator unit, Anthropic/Gemini overflow + at-limit, OpenCode-Zen reuse, pass-through non-regression, BenchmarkResponsesRetainedText).
- Budget: 1 MiB per stream (order of one max SSE line; ~250k tokens; peak ~3 MiB transient vs 32 MiB body cap). Fixed internal, no new config. Overflow delta neither retained nor emitted; typed error via STREAM-02 pipe boundary, upstream closed, no completed/DONE. Pass-through uncapped.
- Benchmarks `go test ./internal/provider -run '^$' -bench Responses -benchmem -count=5`: AtLimit ~16-26ms/17.0MB/4790 allocs; Overflow2x ~11-12ms/13.7MB/4749 allocs; Overflow4x ~10-13ms/13.7MB/4729 allocs — Overflow4x ≈ Overflow2x despite 2x upstream, retained bounded.
- Verification: targeted 7 tests ok; `go test -race ./internal/provider/ -count=1` ok 9.408s; vet/gofmt/build clean.

Kind: defect

Priority: P1; individually bounded upstream events can still exhaust memory
through aggregate translated Responses text retention.

Suggested agent: provider resource-safety engineer

Dependencies: STREAM-02

Primary ownership: `internal/provider/responses.go`, translator error propagation
only as needed, and focused aggregate-output tests/benchmarks.

Finding: `responsesStreamState.Text` appends all generated text to a
`strings.Builder` without an aggregate bound. Completion serializes that text
again in multiple events. Per-line/event SSE limits reset between events, so
many small valid deltas can grow retained state indefinitely. This is not the
previously fixed pass-through observer or per-event decoder issue.

References:

- `internal/provider/responses.go:93-102,137-143,235-285` (state, append, completion).
- `internal/provider/sse.go:12-15,137-159` (per-event bounds).
- `internal/provider/provider_test.go:360-375,1587-1603`: existing overflow tests cover individual framing bounds, not aggregate output.

Requirements:

1. Record a justified finite retained-text budget and its user-visible overflow
   contract before implementation. A fixed internal cap is the minimal option;
   new configuration requires a concrete operator need, not speculation.
2. Enforce before append at the shared accumulator; propagate a typed overflow
   through both translators without success completion. Do not cap total opaque
   pass-through stream length as a side effect.
3. Document/release-note the translated-output limit and account for peak
   completion serialization allocations when choosing the budget.

Acceptance criteria:

- Many individually valid sub-limit events deterministically exceed a small
  test budget; at-limit text succeeds and the overflowing delta is not retained.
- Overflow closes upstream and terminates translation without a leaked goroutine
  or successful terminal event across native and reused translators.
- Tests use a draining/discarding consumer; an allocation benchmark or bounded
  heap experiment records retained-state behavior as total upstream text grows.

Verification: `go test -race ./internal/provider`; run the new focused benchmark
with `go test ./internal/provider -run '^$' -bench Responses -benchmem -count=5`
and record its actual benchmark names/results. No speedup is presumed.

### Task CFG-01: Edit Only Root HCL Timeout Attributes

Status: completed

Owner: sub-agent ses_f7ddd08c8ffeeSlGBJsxXdeUz3 (configuration editing engineer)

Completion evidence:

- Changed: `internal/configedit/configedit.go` (Upsert/TopLevel now (string,error) via hclsyntax parse + expression ranges; replace root-only, insert at top when absent, error on parse failure, never touch nested/comments); `cmd/aiproxy/configure.go` (handles errors, aborts before write, preview says provider overrides preserved); new `internal/configedit/root_attribute_test.go` (7-case table + Load proofs + invalid-source no-publish via WriteProviderFiles); new `cmd/aiproxy/upstream_root_test.go` (CLI preserve/insert/fail-no-modify with Load checks).
- Verification: `go test ./internal/configedit/ -count=1` ok; CLI upstream tests 4 pass; `go test -race ./internal/configedit ./internal/config ./cmd/aiproxy -count=1` all ok (1.266s/1.285s/6.275s); vet/gofmt/diff-check clean. Fail-before: old regex rewrote every matching line (verified), new exact-count tests fail under old behavior.

Kind: defect

Priority: P2; a successful root timeout edit silently changes provider overrides.

Suggested agent: configuration editing engineer

Dependencies: STORE-02

Primary ownership: `internal/configedit/configedit.go`, `cmd/aiproxy/configure.go`,
and focused source-mutation/CLI tests.

Finding: `UpsertTopLevelStringAttribute` uses a multiline regex accepting any
leading whitespace and replaces every matching line. It rewrites nested provider
timeouts too; if only a provider override exists, it changes that instead of
inserting a root attribute. `TopLevelStringAttribute` can likewise read a nested
value as the root default. CLI preview describes only the intended root change.

References:

- `internal/configedit/configedit.go:560-583` (root attribute helpers).
- `cmd/aiproxy/configure.go:418-457` (`runConfigureUpstream`).
- `cmd/aiproxy/configure_test.go:602-636` (`TestConfigureUpstreamNonInteractiveSetsRootTimeout`) covers insertion without existing overrides.

Requirements:

1. Locate root attributes using HCL syntax/source ranges, preserving nested
   overrides, comments, and unrelated text. Keep scope-aware mutation in
   `configedit`, not duplicated in the Cobra command.
2. Surface parse/mutation errors explicitly; do not silently fall back to
   indiscriminate line replacement. Ensure preview reflects actual scope.

Acceptance criteria:

- Table tests cover root absent/present, zero/one/multiple provider overrides,
  provider-before-root order, and commented examples.
- Loading the edited config proves only the root effective value changes;
  provider-specific and derived inherited overrides stay unchanged.
- Invalid source fails without publishing partial changes.

Verification: `go test -race ./internal/configedit ./internal/config ./cmd/aiproxy`.

### Task USAGE-01: Correct Translated Responses Usage Schema And Totals

Status: completed

Owner: sub-agent ses_f7dda0137ffeUAV3S7u2DeqCgg (protocol contract/usage engineer)

Completion evidence:

- Changed: `openai_types.go` (new openAIResponsesUsage input/output/total; Usage \*pointer so zero omits); `responses.go` (Usage type, reconcileResponsesUsage like SetUsage, only set when non-zero); `anthropic.go`/`gemini.go` translated Responses use new wire type (authoritative total preserved via max, missing→sum); new `responses_usage_test.go` (JSON Anthropic 9/6/15, Gemini authoritative/missing/zero, SSE split 7/11→18 wire+internal no Chat keys, zero omission, Gemini stream authoritative, chat unchanged, OpenCode messages/gemini reuse).
- Verification: `go test ./internal/provider/ -run TestResponses -count=1` ok; `go test -race ./internal/provider/ -count=1` ok 9.3s (x2); vet/gofmt clean. STREAM-02/03 preserved.

Kind: defect

Priority: P2; supported Responses payloads expose Chat usage keys and can disagree
with internal accounting when Anthropic usage arrives in separate events.

Suggested agent: protocol contract/usage engineer

Dependencies: STREAM-03

Primary ownership: `internal/provider/openai_types.go`, `responses.go`, relevant
Anthropic/Gemini usage mapping, and translated Responses tests.

Finding: `openAIResponsesResponse.Usage` reuses Chat's `openAIUsage` with
`prompt_tokens`/`completion_tokens`. Responses requires `input_tokens` and
`output_tokens`. Separately, input 7 at Anthropic message start followed by
output 11 at message delta leaves retained input/output 7/11 but total 11:
`setUsage` overwrites the total with the partial event's total. Internal
`StreamCompletion.SetUsage` already reconciles split usage differently.

References:

- `internal/provider/openai_types.go:39-43,74-80` (wire types).
- `internal/provider/responses.go:122-135` (`setUsage`).
- `internal/provider/anthropic.go:475-480,503-508`; `internal/provider/provider.go:82-98` (split usage handling).
- `internal/provider/provider_test.go:824-840,1241-1259`: Responses fields are decoded but not asserted; realistic split usage is tested for chat accounting, not Responses wire output.

Requirements:

1. Separate endpoint-specific wire usage from shared internal accounting. Preserve
   Chat keys and authoritative provider totals where they contain additional
   token categories; do not blindly replace all totals with input plus output.
2. Reconcile split Anthropic observations consistently across wire and internal
   usage. Correct tests, docs, and release notes together for the wire schema fix.

Acceptance criteria:

- JSON and final Responses SSE contain and assert input/output/total field names
  and values, without incorrectly emitting Chat-only keys.
- Split Anthropic 7/11 yields total 18 on the wire and internally. Include zero,
  missing, and provider-authoritative-total cases.
- Chat serialization is unchanged; OpenCode translated reuse is covered.

Verification: `go test -race ./internal/provider`.

### Task HEALTH-03: Expose Provider Health Backend Write Failures

Status: completed

Owner: sub-agent ses_f7dd31537ffegOBRKCZv2Jjxtd (health backend/observability engineer)

Completion evidence:

- Changed: `internal/providerhealth/providerhealth.go:159-183` (MarkSuccess/Failure record mark_success/mark_failure in existing backend-error counter; cache+gauge unchanged); new `internal/providerhealth/providerhealth_mark_errors_test.go` (tracker + /metrics scrape assertions).
- Verification: `go test ./internal/providerhealth/ -count=1` ok 0.090s; `go test -race ./internal/providerhealth ./internal/observability ./internal/httpapi -count=1` all ok (1.108s/1.053s/1.528s); vet/gofmt clean. Each injected failed mark increments exactly once; success emits nothing; write-failing/readable backend observable without read failure.

Kind: defect

Priority: P2; failed cross-instance health propagation can be invisible even
while the backend remains readable.

Suggested agent: health backend/observability engineer

Dependencies: ROUTE-02

Primary ownership: `internal/providerhealth/providerhealth.go`, tracker tests,
and metric assertions; avoid changing routing fallback policy.

Finding: `MarkSuccessContext` and `MarkFailureContext` discard backend errors,
then update the local cache/gauge. Redis `DEL`/`SET` failures are consequently
absent from backend-error metrics, unlike read failures. A readable Redis with
write-denying ACLs can fail propagation without producing any read error.

References:

- `internal/providerhealth/providerhealth.go:159-200,248-251` (mark/read reporting).
- `internal/providerhealth/redis.go:41-50` (write errors).
- `internal/observability/metrics.go:122-125,322-329` (operation-labelled error counter).
- `internal/providerhealth/providerhealth_test.go:23-43,105-141,413-442`: injectable mark errors exist, but inspected tracker tests cover successful cache updates and backend-level cancellation, not reporting failed writes.

Requirements:

1. Count failed mark operations using fixed `mark_success`/`mark_failure` labels
   in the existing metric. Keep local cache behavior and read-fallback policy.
2. Do not replace upstream client responses with persistence errors, expose Redis
   details to clients, or add speculative backend retries/configuration.

Acceptance criteria:

- Each injected failed mark operation increments its counter exactly once;
  successful writes do not increment it.
- A readable/write-failing backend is observable without a later read failure;
  existing cache and upstream response semantics remain unchanged.

Verification: `go test -race ./internal/providerhealth ./internal/observability ./internal/httpapi`.

### Task DASH-02: Make Token Source Changes Coherent Across Reload And CLI

Status: completed

Owner: sub-agent ses_f7dd14f43ffeNYbaEwfa0KYevn (application credential-lifecycle engineer)

Completion evidence:

- Policy: publish-on-transition (no maintainer in session). Carried-over in-memory secret published to discoverable file (0600) before new runtime activates; persistence failure rejects reload with old runtime intact. Automatic-token reload preserved. Provenance via TokenFromConfig.
- Changed: `internal/app/app.go` (ensureDashboardToken takes full Dashboard + provenance, returns minted/published/err, injectable persistDashboardToken, Reload tracks written, nil-guard); updated `dashboard_token_test.go` (expects publication, 0600, omitted provenance + publish/failure helper tests); new `dashboard_transition_test.go` (assembled Build→Reload: absent-file publication + live snapshot via CLI-discovered token, stale overwrite new-200/stale-401, injected failure leaves runtime/snapshot/stale-file unchanged, retry succeeds); doc updates in `dashrpc.go`, `README.md`, `website/docs/configuration.md`, `AGENTS.md` (one sentence each).
- Verification: build/vet/gofmt clean; targeted 12 tests pass; `go test -race ./internal/app ./cmd/aiproxy -count=1` ok (1.520s/7.085s); `make docs-contract` pass; diff-check clean. No secret contents in errors/files evidence.

Kind: defect

Priority: P2; an accepted explicit-to-automatic token reload can prevent a new
dashboard command from authenticating. This is not an unauthorized-access claim.

Suggested agent: application credential-lifecycle engineer

Dependencies: CFG-01

Primary ownership: `internal/app/app.go`, `internal/app/dashboard_token_test.go`,
and `cmd/aiproxy/dashboard_test.go`; shared persistence only by coordination.

Finding: startup with an explicit token does not write a token file. Removing
the token and reloading reuses the in-memory token without persisting it. The
CLI reading the now-tokenless config requires the file, so attachment fails
when the file is absent or stale. Separate tests currently enshrine both halves
of this incompatible behavior.

References:

- `internal/app/app.go:234-256,412-433` (`Reload`, `ensureDashboardToken`).
- `cmd/aiproxy/dashboard.go:48-54` (token discovery).
- `internal/app/dashboard_token_test.go:120-180` (`TestReloadReusesConfigSuppliedToken`) explicitly expects no file after transition.
- `cmd/aiproxy/dashboard_test.go:61-135` (tokenless config reads persisted token).

Requirements:

1. Resolve the policy before implementation: securely publish the discoverable
   token before activation, or reject this transition as restart-required while
   retaining the old runtime. Record maintainer approval; if unavailable, mark
   blocked with this decision as the prerequisite rather than guessing.
2. Use explicit provenance (`TokenFromConfig`) and preserve unchanged automatic
   token reload behavior. Update conflicting tests, docs, and release notes.

Acceptance criteria:

- An assembled explicit-to-omitted transition with absent and stale token files
  either permits a subsequent CLI-authenticated snapshot or rejects reload
  clearly with unchanged active runtime.
- If publication is selected, injected persistence failure leaves active runtime
  unchanged and never publishes a misleading credential; files retain secure modes.
- No token contents appear in logs/errors or completion evidence.

Verification: `go test -race ./internal/app ./cmd/aiproxy`; include assembled
transition evidence, not only separate helper tests.

### Task USAGE-02: Extract Native Responses JSON And SSE Usage Correctly

Status: completed

Owner: sub-agent ses_f7dc81bb0ffeV4kYQCE5EikbUw (provider usage observation engineer)

Completion evidence:

- Changed: `internal/provider/usage.go` (shape-aware usageFromBody via top-level usage key inspection, Gemini distinct-key first, usageFromSSEData top-level then nested response.usage, bounded unmarshal, no rewrite); `openai.go` observe uses usageFromSSEData; fixed `opencode_test.go` fixture to native input/output/total keys; new `usage_native_test.go` (native JSON 4/6/10 openai/compatible/zen/go byte-for-byte, absent/zero/empty, Chat unchanged, fragmented nested SSE 4/6/10 + preservation); new `httpapi/usage02_accounting_test.go` (assembled /v1/responses body preserved + exactly one 4/6/10 accounting event).
- Verification: targeted native tests ok; fail-before (stashed fix) FAILs {0 0 10} vs 4/6/10, passes after; `go test ./internal/httpapi/ -run TestUsage02` PASS; `go test -race ./internal/provider ./internal/httpapi -count=1` ok (10.978s/1.577s); vet/gofmt/diff-check clean.

Kind: defect

Priority: P2; native Responses bytes pass through correctly but token breakdowns
or all stream usage are lost at the provider accounting boundary.

Suggested agent: provider usage observation engineer

Dependencies: USAGE-01

Primary ownership: `internal/provider/usage.go`, `internal/provider/openai.go`,
and native/OpenCode Responses observation tests.

Finding: `usageFromBody` returns the first parser with any positive count. Chat
parsing sees `total_tokens:10` and returns before Responses parsing can see
`input_tokens:4, output_tokens:6`. Native SSE `response.completed` nests usage
under `response.usage`, while existing extractors look only at top-level usage.

References:

- `internal/provider/usage.go:7-55,59-70,97-114` (shape precedence and extraction).
- `internal/provider/openai.go:89-102` (whole-event observer input).
- `internal/provider/provider_test.go:524-567`: native Responses fixture lacks usage.
- `internal/provider/opencode_test.go:276-295`: native Responses fixture uses Chat-style keys, masking the gap.

Requirements:

1. Select/normalize the actual usage shape rather than accepting a partial total
   from the first decoder. Handle Responses terminal envelopes through the
   existing bounded observer, without rewriting pass-through bytes.
2. Keep absent usage absent and preserve Chat, Anthropic, Gemini, and observer
   overflow behavior. Avoid new protocol-dispatch registries for this local fix.

Acceptance criteria:

- Native JSON with input/output/total 4/6/10 yields all three counts internally;
  fragmented nested `response.completed` SSE yields the same counts.
- Tests cover native OpenAI-compatible and OpenCode responses paths, absent/zero
  usage, and assert byte-for-byte body/stream preservation.
- An assembled usage-summary regression demonstrates that the corrected counts
  reach accounting without double counting.

Verification: `go test -race ./internal/provider ./internal/httpapi`.

### Task LIFE-02: Make Restart A Serialized Stop-And-Start Operation

Status: completed

Owner: sub-agent ses_f7dc2c246ffejqLyTCAn4JgRIp (Linux process lifecycle engineer)

Completion evidence:

- Changed: `cmd/aiproxy/daemon.go` (spawnDaemonLocked/stopDaemonLocked helpers, restartServer holds one lifecycle lock, restartStopImpl/restartSpawnImpl vars for tests, error propagation never spawns after failed stop, preserves identity/canonical/non-Linux); new `cmd/aiproxy/restart_test.go` linux-only 7 tests (absent/malformed/mismatch no-spawn, injected stop zero spawns, single-lock serialization+ordering, failed-replacement cleanup+unlock, successful restart readiness e2e).
- Verification: `go vet` pass; `go test -run TestRestart` 7 PASS 0.959s; `go test -race ./cmd/aiproxy/ -count=1` ok 9.206s; gofmt/diff-check clean.

Kind: defect

Priority: P2; restart ignores stop errors and can launch an absent daemon contrary
to the documented lifecycle contract.

Suggested agent: Linux process lifecycle engineer

Dependencies: DASH-02

Primary ownership: `cmd/aiproxy/daemon.go`, platform lock helpers only if needed,
and `cmd/aiproxy/lifecycle_test.go`.

Finding: `restartServer` discards `stopServer`'s error then calls `spawnDaemon`.
An absent daemon with valid config is therefore started rather than returning
`no server running` and nonzero as documented. Stop and spawn separately lock,
so restart also lacks one serialized lifecycle boundary.

References:

- `cmd/aiproxy/daemon.go:337-340` (`restartServer`).
- `cmd/aiproxy/lifecycle_test.go`: inspected absent stop/status, identity, malformed state, failed startup, and concurrent-start cases contain no restart regression.
- `AGENTS.md`, Build & run: absent-daemon lifecycle contract and Linux-only safety guarantees.

Requirements:

1. Propagate stop errors and never spawn after a failed stop. Follow the current
   documented absent-daemon contract, not a new start-if-absent compatibility mode.
2. Hold one lifecycle lock through stop and replacement readiness, factoring
   minimal lock-held helpers. Preserve process start/executable identity checks,
   canonical config scoping, and non-Linux unsupported behavior.

Acceptance criteria:

- Absent, malformed, and identity-mismatched state return nonzero without a new
  child; injected stop failure causes zero spawn attempts.
- A deterministic concurrent lifecycle test proves no interleaving between
  verified stop and replacement startup; successful restart waits for readiness.
- Failed replacement startup reports failure without stale live state or leaked
  locks, and existing identity/portability tests remain valid.

Verification: `go test -race ./cmd/aiproxy` on Linux; binary lifecycle coverage
under `make integration` at final review.

### Task PERF-01: Measure Catalog Clone Cost Before Changing Lookup APIs

Status: completed

Owner: sub-agent ses_f7dbecad5ffe754NynFU0454dP (performance/investigation engineer)

Completion evidence (investigation only, no API changes):

- Added benchmarks only: `internal/config/catalog_perf_test.go`, `internal/modelresolver/resolve_perf_test.go`.
- Toolchain go1.27.1 linux/amd64 i9-13950HX; fixture 1 provider x N models (2 caps each). Medians count=5: config Provider hit ~760ns/1104B/5 (N=1), ~26µs/43KB/205 (N=100), ~365-402µs/613KB/2011 (N=1000); Model hit same +1 alloc; modelresolver Resolve hit ~1.7µs/2272B/12 (N=1), ~53-64µs/86.5KB/412 (N=100), ~732-810µs/1.2MB/4024 (N=1000). Unknown-provider/alias size-independent (~90ns). Spread ≤15%.
- Attribution: cloneProvider ~2N+3 allocs (~430B/model); Resolve does exactly 2 full clones (412=2x206, B/op 2x). Clones dominate hot path.
- Recommendation: IMPLEMENT via follow-up (not this task): narrow Catalog.Direct(provider,model) returning header copy + single cloned model with ErrUnknownProvider vs ErrUnknownModel distinction; rewire Model+Resolve onto it. Removing existence-check clone alone halves but leaves O(N) (~375µs at N=1000 vs 0.6µs narrow prototype, size-independent 1008B/4 allocs). Follow-up acceptance: private storage unexported, mutation-isolation extended, error distinction table test, reload unchanged, N=1000 Resolve ≤3µs/3KB/12 allocs flat 1→1000.
- Verification: `go test ./internal/config/ ./internal/modelresolver/ -count=1` ok (0.107s/0.014s); bench Catalog count=5 pass (139s), Resolve count=5 pass (68s); vet/gofmt clean.

Kind: investigation

Priority: P2; request lookup copies whole model collections, but production
latency/GC impact has not been measured and does not justify an assumed rewrite.

Suggested agent: Go performance and immutable-data-boundary engineer

Dependencies: ROUTE-02, HEALTH-03, USAGE-02, LIFE-02

Primary ownership: focused benchmarks in `internal/config` and
`internal/modelresolver`, with a decision/evidence note in this task.
Do not change catalog APIs as part of the investigation.

Finding: `Catalog.Provider` deep-clones all models, capability slices, and a
model map. `Catalog.Model` calls it before a single model lookup. Direct
`Resolver.Resolve` calls `Provider` for existence and then `Model`, causing two
full-provider clones per direct lookup. Defensive copying is intentional;
whether a narrower immutable lookup boundary is worthwhile is the question.

References:

- `internal/config/catalog.go:52-78,122-161` (lookup and deep cloning).
- `internal/modelresolver/resolve.go:109-133` (`Resolve`).
- `internal/config/catalog_test.go:5-71`: lookup and mutation-isolation tests establish behavior to preserve, not allocation performance.
- Completed CATALOG-01 in the 2026-08-23 plan: do not reopen exposed mutable maps.

Requirements:

1. Benchmark direct hit, unknown provider/model, and alias target lookup across
   1, 100, and 1000 models per provider. Record ns/op, B/op, allocs/op, toolchain,
   fixture size, and repeated-run variability; inspect allocation attribution.
2. Answer whether repeated defensive clones materially dominate these local
   operations and whether removing only the duplicate existence-check clone is
   sufficient. Compare a bounded local prototype if needed, without publishing
   a mutable catalog or claiming end-to-end throughput improvements.
3. Conclude implement/defer/no-action with evidence. If implementation is useful,
   create a separately owned follow-up with a concrete target and immutability,
   reload, and error-distinction acceptance criteria before code changes.

Acceptance criteria:

- Reproducible benchmark evidence and a justified recommendation are recorded;
  no speculative optimization is required for investigation completion.
- Any proposed API preserves private immutable storage, returned-value mutation
  isolation, and unknown-provider versus unknown-model errors.

Verification: after adding benchmarks, `go test ./internal/config ./internal/modelresolver`
and `go test ./internal/config ./internal/modelresolver -run '^$' -bench 'Catalog|Resolve' -benchmem -count=5`.

### Task REVIEW-02: Independently Verify Integrated Contracts And Bounds

Status: completed

Owner: sub-agent ses_f7db94951ffewBL1AFA4VxOxnG (independent reviewer)

Completion evidence:

- Per-task verdicts all pass (SEC-01, STORE-02, STREAM-02, ROUTE-02, STREAM-03, CFG-01, USAGE-01, HEALTH-03, DASH-02, USAGE-02, LIFE-02, PERF-01-investigation).
- New `internal/httpapi/review02_stream_usage_test.go` only (no existing-file edits): TestReview02TranslatedStreamErrorHTTPOutcome (Anthropic error mid-stream → 200 committed headers unchanged, partial Hello preserved, no DONE/completed fabricated, provider unhealthy, exactly one zero-usage accounting event) PASS; TestReview02NativeResponsesUsageReachesBilling (native 4/6/10 → GET /v1/billing/usage one entry 4/6/10) PASS. Note: billing JSON uses Go-default capitalized keys, asserted as-is.
- Shared gates serialized from root: `make vet test` pass; `make test-race` pass; `make integration` pass (2.8s, rebuilt dist/aiproxy); `make docs-contract` pass; `git diff --check` clean; gofmt clean.
- Hygiene: CHANGELOG.md untouched (0 diff); .gitignore pre-existing +.kamal preserved; synthetic fixtures only, no real secrets; public matrices + Responses subset + limitations still documented; deferrals preserved.

Kind: improvement

Priority: P1; independently validate interacting fixes before declaring the
delegated plan complete, rather than relying on worker summaries alone.

Suggested agent: reviewer who did not implement the primary fixes

Dependencies: SEC-01, STORE-02, STREAM-02, ROUTE-02, STREAM-03, CFG-01,
USAGE-01, HEALTH-03, DASH-02, USAGE-02, LIFE-02, PERF-01

Primary ownership: this execution record, focused assembled regressions only
where missing, and coordinated public contract documentation/release notes.

Finding: the reviewed defects cross individually tested boundaries, including
wire versus internal usage, stream EOF versus success, configured versus
eligible fallback, and server token state versus CLI discovery. Unit helper
success alone is insufficient evidence of the integrated outcome.

References: task-specific source/tests above; `Makefile:42-60` defines docs,
vet, unit, race, and hermetic binary integration gates.

Requirements:

1. Check every acceptance criterion against regression or bounded investigation
   evidence, including negative, boundary, cancellation, and reused-provider paths.
2. Verify translated stream failures reach HTTP completion/health/attempt
   accounting exactly once without fabricating success or changing committed
   headers. Check corrected native usage reaches public usage summaries.
3. Recheck unauthenticated metric bounds, recovery confidentiality, token-source
   transition rollback, no unintended config edits, and restart identity checks.
4. Reconcile public types/docs/implementation, the documented Responses subset,
   overflow/error semantics, and release notes for external corrections. Keep
   explicit deferrals and existing product limitations visible.

Acceptance criteria:

- All required checks below pass with commands and results recorded; any blocked
  task or unresolved contract prevents marking the entire plan complete.
- Fixes have failing-before/passing-after regressions where feasible; an
  alternative proof is recorded where not feasible. No secrets enter fixtures,
  logs, public errors, or this file.
- No unrelated worktree changes are reverted and all new document paths remain
  repository-relative or generic system paths.

Verification: shared final gates below, plus evidence review of PERF-01's
decision and the chosen DASH-02/STREAM-03 contracts.

## Shared Verification And Done

All commands run from the repository root. Use the current toolchain declared
in `.tool-versions` and build instructions in `AGENTS.md`; do not copy stale
version overrides or removed build targets from historical completion notes.
Tests should be hermetic, with generated credentials and isolated temporary
directories. Linux is required for daemon runtime tests; no real provider or
production Redis credentials are required.

Run task-specific package/race tests after each task and affected package checks
after each lane merges. Run the following once during independent final review,
with builds serialized because integration replaces `dist/aiproxy`:

```sh
make vet test
make test-race
make integration
make docs-contract
git diff --check
```

`make integration` includes the host build. Add new focused benchmark names and
measured results to the responsible tasks; do not treat benchmark commands that
matched no benchmarks as verification. If a required check cannot run, keep the
task blocked and name the missing prerequisite and unverified criterion.

Definition of done: each fix is minimal, has observable regression evidence,
preserves stated boundaries, and passes its checks; policy choices are recorded;
PERF-01 has an evidence-backed recommendation; independent review and shared
gates pass. Append changed files, exact commands/results, and any separately
tracked follow-up to each completed task. Saving this plan does not complete
any implementation task.

## Decisions And Deferrals

- STREAM-03 must justify the finite retained-output budget before coding. This
  is a contract choice, not evidence that an unbounded accumulator is acceptable.
- DASH-02 requires approval of publish-on-transition versus restart-required.
  Other independent lanes can proceed while that decision is pending.
- Late cooldown advice after an unchanged reload is deliberately not called a
  defect here: `internal/modelresolver/cooldown.go:103-122` clones deadlines,
  and `internal/httpapi/cooldown_test.go:631-671` explicitly tests old/new resolver
  isolation. Reversing that policy needs a separate requirement; residual risk
  is that late old-request advice does not reach the new snapshot.
- Tools/vision expansion in conservative translations, Anthropic embeddings,
  non-chat Copilot operations, and remote dashboard support remain documented
  product limitations, not newly confirmed defects. Do not broaden the matrix
  merely to fill a missing-features category.
- Further Responses lifecycle fidelity, including token-limit completion status,
  needs separate protocol evidence; it was not developed into a speculative fix.
- Redis write-failure precedence versus a later successful read is not changed
  by HEALTH-03. That task exposes degraded propagation; it does not guarantee
  cross-instance consistency during backend faults.
- Architectural improvements are intentionally local: testable restoration,
  scope-aware config mutation, endpoint-specific wire types, shared stream/error
  bounds, and lock-held lifecycle composition. No benefit was established for a
  general provider rewrite or cosmetic package splitting.
