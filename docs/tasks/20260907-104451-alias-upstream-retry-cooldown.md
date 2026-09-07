# Alias Upstream Retry Cooldown

Created: 2026-09-07 10:44:51 local time

## Objective

Honor upstream `retry-after-ms` and `Retry-After` as cross-request cooldown advice for alias targets. Skip a target until its deadline expires instead of repeatedly calling a throttled upstream. When every target in an alias pool is actively cooling, return a proxy-generated JSON `429` with retry advice based on the minimum **remaining** cooldown, not the minimum original header value.

This is a requested routing improvement, not a defect against the existing documented status-based failover contract. This document plans implementation only; no runtime changes are included.

## Scope And Evidence

- `internal/httpapi/dispatch.go:86-224` (`dispatchAlias`) tracks attempted targets only within a request and retries immediately based on status. The loop-index test for another attempt must be revisited when targets can be excluded by cooldown.
- `internal/alias/alias.go` (`Selector.Acquire`) already supports target exclusions and lease-based round-robin/least-connections selection.
- `internal/httpapi/dispatch.go:234-260` (`recordProviderHealth`) treats provider health separately from configured retryable 4xx. Keep that separation.
- `internal/provider/provider.go` (`Result`, `executeUpstream`) is a shared boundary for retaining upstream advice. OpenAI preserves response headers, but error-result construction in `internal/provider/anthropic.go`, `internal/provider/gemini.go`, and translated paths in `internal/provider/opencode.go` retains only Content-Type. Reading current `Result.Header` alone would miss those protocols.
- `internal/httpapi/response.go:33-83` copies headers on the normal JSON result path but wraps non-JSON errors without copying headers. Capture cooldown advice before serialization, regardless of error content type.
- `internal/httpapi/handler.go` maps generic dispatch errors to 502. A synthetic cooldown response needs an explicit 429 path.
- `internal/modelresolver/resolve.go` (`NewWithPrevious`, `selectorForAlias`, `aliasesShareSelectorState`) preserves selector state for unchanged alias target lists; this comparison does not establish credential/endpoint identity for cooldown reuse.
- `internal/app/app.go` (`App.Reload`) publishes new dependency snapshots while old requests may still complete.
- `internal/httpapi/handler_test.go` (`TestHandlerPreservesJSONUpstreamErrors`) tests downstream Retry-After preservation, not interpreting it for routing.

Focused source and backlog inspection found no existing task for upstream-advised target cooldown. Related completed work: lease-based selection and health separation in [codebase health remediation](20260804-125911-codebase-health-review-remediation.md), unchanged selector preservation across reload in [health follow-up](20260823-112437-codebase-health-follow-up.md), and explicit configured quota-status failover in [Zen/Go support](20260906-101616-opencode-provider-zen-go.md). Preserve those contracts except where this plan explicitly changes them.

Coverage is focused, not an exhaustive provider/backlog audit. Baseline tests were not run because this deliverable is planning only. Existing unrelated login/config-edit/filestore worktree changes must remain untouched.

## Contract And Boundaries

1. Store a deadline derived from the time advice is observed. Expired advice no longer excludes a target. Historical presence of a header alone is never sufficient to return 429.
2. Support standard Retry-After delay-seconds and HTTP-date, plus millisecond advice. Malformed, non-positive, past, and unrepresentable values must not create indefinite cooldowns or cause arithmetic overflow.
3. Record valid advice before discarding retry responses or returning terminal responses, including translated protocols and streaming response headers. Never inspect streaming bodies for retry advice or replay a committed response.
4. Cooldown eligibility is distinct from current-request failover policy. Do not silently add 429 to `retry_status_codes`; direct requests must never fail over.
5. When all configured pool targets have active cooldowns and no response has been committed, synthesize 429 using the earliest deadline. Recalculate remaining time when responding and round up when converting to header units so clients are not instructed to retry too early.
6. Do not mislabel a mixed pool of cooling and otherwise unavailable targets as all-cooling. Preserve existing health/error behavior outside the explicitly agreed cooldown cases.
7. Do not sleep inside dispatch. Preserve selection fairness, exactly-once lease release, cancellation behavior, authentication, allowed-model enforcement, and explicit Zen/Go service boundaries.
8. Keep the first implementation process-local with no Redis sharing, persistence, new dashboard, or new configuration surface. Distributed coordination and proactive probing are non-goals. In-flight requests admitted before advice arrives cannot be retroactively prevented.

## Ordered Tasks

P1 means required for this feature's correctness and release, not an emergency security fix. Execute these tasks sequentially because the behavioral contract and routing/runtime ownership are shared.

### Task COOL-01: Resolve Cooldown Policy Edges

Status: pending

Kind: investigation

Priority: P1; prevents implementing ambiguous quota and response semantics.

Dependencies: none

Primary ownership: this document; focused inspection of alias dispatch, provider result creation, and reload lifecycle.

Finding: The core requested behavior is clear, but target-sharing scope, header conflicts, and terminal-response precedence affect observable routing. Existing provider-health and selector reuse rules do not answer these questions.

References: `dispatchAlias`, `executeUpstream`, `aliasesShareSelectorState`, and `App.Reload` cited above.

Requirements:

1. Record a decision for cooldown identity: alias-local provider/model versus provider/model shared across aliases; whether operations share state; and whether direct responses populate cooldown state. Recommended initial scope is alias-local provider/model across that alias's operations, with direct requests neither consulting nor populating it. These are recommendations, not approved requirements.
2. Confirm trigger statuses. The request describes any response with valid advice; retain that literal scope unless the maintainer chooses only throttling/error statuses. Successful responses with advice should still be returned normally, with advice affecting future selection only.
3. Resolve precedence when both headers are valid, malformed millisecond fallback to Retry-After, repeated/conflicting header fields, numeric format, and excessive durations. Recommended starting policy: valid positive integer `retry-after-ms` takes precedence, otherwise parse standard Retry-After. Do not silently invent a maximum duration; record any cap and its rationale.
4. Resolve whether the response that newly puts the final target into cooldown becomes synthetic 429 immediately, or whether its original response is preserved and only subsequent requests get synthetic 429. The user's all-targets rule favors immediate synthesis for an uncommitted retryable failure; non-retryable and successful responses need an explicit precedence decision.
5. Specify synthetic JSON error type and output headers. Recommend both `Retry-After` (ceiling seconds) and `retry-after-ms` (ceiling milliseconds) from the same earliest deadline.
6. Specify reload preservation/invalidation for target removal, credential, endpoint, upstream model/protocol, algorithm, and retry-policy changes. Unchanged effective targets should retain advice; obsolete in-flight responses must not throttle replacement identities. Specify behavior when raw headers arrive but response translation/body reading fails.
7. Seek maintainer input for unresolved behavioral choices, mark the investigation blocked if needed, and update this document with approved decisions before implementation.

Acceptance criteria:

- Each question has a recorded decision with rationale, or a named maintainer blocker; COOL-02 cannot start while a material choice remains unresolved.
- A compact response/selection truth table covers partial cooldown, all cooldown, mixed unhealthy/cooling, and a response newly cooling the last target.

Verification: evidence review against the referenced code and maintainer decisions; no runtime tests required for this investigation.

### Task COOL-02: Implement Target Deadlines And Alias Exclusion

Status: pending

Kind: improvement

Priority: P1; implements the requested protection against repeated throttled calls.

Dependencies: COOL-01

Primary ownership:

- `internal/provider/provider.go` and necessary adapter tests/header metadata plumbing.
- `internal/alias/`, `internal/modelresolver/resolve.go`, and `internal/app/app.go` for narrowly scoped runtime state and reload ownership.
- `internal/httpapi/dispatch.go`, `internal/httpapi/response.go`, and `internal/httpapi/handler.go` only as needed for selection and synthetic response handling.
- Focused tests alongside these packages.

Finding: Current request-local exclusion has no persistent cooldown, translated adapters can discard advice, and generic exhaustion errors become 502 rather than the requested 429.

References: scope evidence above, especially `dispatchAlias`, `Result`, `executeUpstream`, `Selector.Acquire`, and `App.Reload`.

Requirements:

1. Implement the approved parser and retain advice at the smallest shared upstream boundary. Avoid forwarding unrelated raw upstream headers through translated adapters just to retain retry metadata.
2. Add concurrency-safe, runtime-owned state bounded by configured target identities, with deterministic clock-driven tests and expired/removed entry cleanup. Concurrent advice must not shorten an existing later deadline; unrelated successful completions must not erase newer advice.
3. Combine cooldown exclusions with existing attempted-target exclusions for both algorithms, and recheck eligibility before upstream dispatch. Do not hold state locks during network I/O.
4. Replace loop-index assumptions about remaining eligible targets with correct exhaustion handling. Apply the approved terminal-response policy, close discarded bodies, and release every lease exactly once.
5. Return synthetic JSON 429 with the approved minimum-remaining-time headers when the all-cooling condition holds. Handle expiry at the decision boundary without returning a stale all-cooling verdict.
6. Keep synthetic results and skipped targets out of upstream-attempt, selection, retry, and usage attribution where no upstream I/O occurred; ensure the client-facing status remains visible in existing HTTP accounting/metrics. Do not mark providers unhealthy merely because of cooldown.
7. Preserve state safely across approved reload cases, leave state untouched on failed reload, and isolate old in-flight completions from replacement target identities.

Acceptance criteria:

- Deterministic tests cover both headers individually, seconds/date formats, precedence/fallback, casing, repeated fields, zero/negative/malformed/past/overflow values, and approved duration bounds.
- After target A advertises cooldown, subsequent alias requests call B without calling A until expiry; test both routing algorithms and target/alias/operation isolation under the approved identity policy.
- With deadlines 10 and 30 seconds from observation, a request four seconds later receives synthetic 429 with six seconds remaining, not ten; include sub-second rounding and differing observation times.
- An all-cooling request causes zero upstream calls. Header-free pools, expired advice, mixed health/cooldown pools, and approved final-target response semantics behave as documented.
- Advice survives native and translated provider result paths, including non-JSON errors and streaming headers, without waiting for stream completion.
- Race tests cover concurrent deadline updates, cancellation/stream lease cleanup, unchanged/changed target reloads, failed reloads, and old requests completing after reload.
- Direct requests retain the approved behavior and never fail over; cooldown alone never mutates provider health.

Verification: run focused parser/routing tests first, then `go test ./internal/alias ./internal/modelresolver ./internal/provider ./internal/httpapi ./internal/app` and the same package list with `go test -race`, from the repository root. Record exact commands and results when implemented.

### Task COOL-03: Verify The Public Contract And Document It

Status: pending

Kind: improvement

Priority: P1; prevents shipping an undocumented or partially supported routing change.

Dependencies: COOL-02

Primary ownership: `internal/integration/binary_test.go`, relevant routing/operations sections in `README.md`, `docs/design.md`, `website/docs/providers-and-routing.md`, `website/docs/api-reference.md`, `AGENTS.md`, release notes, and this document.

Finding: Public documentation currently describes status-based immediate failover, not cross-request target exclusion or proxy-generated all-cooling responses.

References: related completed routing tasks above and `internal/integration/binary_test.go` configured retryable 4xx coverage.

Requirements:

1. Add hermetic binary-level coverage proving advice learned on one request excludes that target on later requests and an all-cooling request returns the documented JSON 429/headers without upstream calls.
2. Document identity scope, parsing/precedence, process-local lifetime, reload policy, direct-request behavior, status-based failover interaction, and the distinction between original and remaining delay. Explicitly note that another process and previously admitted in-flight requests are not coordinated.
3. Update externally visible contract descriptions and release notes together; do not broaden endpoint capability matrices for a routing-only feature.
4. Have a reviewer other than the main implementer check the approved COOL-01 decisions against code, tests, lease lifecycle, and docs. Record residual limitations and any actionable follow-up separately.

Acceptance criteria:

- Hermetic integration coverage observes actual upstream call counts and exact synthetic response semantics.
- Documentation, release notes, and parser/routing tests agree on every approved policy edge.
- Required checks pass and independent review findings are resolved or explicitly blocked, not silently treated as completed.

Verification: run `make vet test`, `make test-race`, `make integration`, and `make docs-contract`. Standard repository Go/build tooling is required; no real-provider credentials or external provider requests are needed. Run final checks serially where build outputs may overlap.

## Definition Of Done

- COOL-01 decisions are recorded and COOL-02/COOL-03 acceptance criteria pass with command/result evidence appended here.
- No cooling target is intentionally selected after its advice is visible to dispatch; all-cooling responses expose the earliest remaining deadline without false upstream attribution.
- No provider-wide health regression, cross-service fallback, lease leak, unbounded state growth, or stale reload update is introduced.
- Unrelated worktree changes remain intact. No implementation task is complete merely because this plan exists.
