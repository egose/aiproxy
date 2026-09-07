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

Status: completed

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

Decisions (maintainer-proxy approved, 2026-09-07):

1. Cooldown identity: key `(alias-name, provider, model)`, alias-local, shared across that alias's operations, process-local. Direct requests neither consult nor populate cooldown state. Rationale: matches per-alias `Selector` ownership (`modelresolver.Resolver.selectors` map keyed by alias name, `resolve.go:43-53`); prevents cross-alias poisoning and preserves explicit Zen/Go service boundaries (no proxy-side rerouting); `dispatchDirect` (`dispatch.go:21-85`) has no selector/exclusion path and direct requests must never fail over per contract; operation sharing reflects that quota is per upstream account/model, not per proxy operation, and avoids per-operation stampedes.
2. Trigger statuses: any alias-target HTTP response carrying valid advice creates/extends a deadline, regardless of status code. Success (2xx) is still returned normally; advice affects future selection only. Transport errors with no response, pre-I/O `ErrInvalidRequest`/`ErrUnsupportedOperation`, and client-canceled contexts record nothing. Capture advice before serialization for all content types (JSON and `writeUpstreamError` non-JSON path in `response.go:33-83`) and at streaming-header time without inspecting bodies or replaying committed responses. Rationale: literal request scope ("any response with valid advice"); avoids inventing a status allowlist that would miss custom quota codes; preserves failover/health separation (`recordProviderHealth`, `dispatch.go:236-263`, only marks 5xx/transport failure unhealthy, never cooldown).
3. Precedence/parsing: valid positive-integer `retry-after-ms` wins; otherwise parse standard `Retry-After` (delay-seconds, then HTTP-date). Header names case-insensitive via canonicalization; scan all field values in wire order, first valid value wins per header; comma-joined `Retry-After` uses the first token. Numeric format: optional surrounding OWS, ASCII digits only, no sign, no decimals, non-empty; `0`/negative/past-date/unparseable means no cooldown from that header (fall through to the next source, else no deadline). `retry-after-ms` malformed or absent falls back to `Retry-After`. No silent maximum duration: the only cap is overflow protection — a delay that overflows `time.Duration` or is otherwise unrepresentable yields no cooldown rather than an indefinite one. Rationale: satisfies contract items 2-3 (no indefinite cooldown, no overflow); deterministic first-valid-wins avoids amplifying conflicting repeated fields; ceiling conversion happens only at synthetic-response time, not at parse time.
4. Terminal-response precedence: if the response that newly cools the final eligible target is an uncommitted retryable failure (status in that alias's `retry_status_codes`, default 500/502/503/504) and all pool targets are now actively cooling, discard its body (`closeResult`, release lease exactly once) and return synthetic 429 immediately. A terminal success or non-retryable error (including unconfigured 429/4xx) is always returned verbatim even if its advice completes all-cooling coverage; only subsequent requests observe synthetic 429. Never synthesize after a response is committed (streaming headers flushed via `writeResult` / `attachStreamFinalizers`). Rationale: implements the all-targets rule for the retryable path without swallowing real successes/errors; respects "no response committed" boundary and existing `retryCodes`/`i+1 < len(targets)` failover semantics (`dispatch.go:208-221`).
5. Synthetic response: HTTP 429 JSON `{"error":{"type":"upstream_rate_limited","message":"all alias targets cooling, retry after <N>ms"}}`, `Content-Type: application/json`. Type reuses existing `upstreamErrorType(429)` (`response.go:93-108`). Emit both `Retry-After: <ceil seconds, min 1>` and `retry-after-ms: <ceil ms, min 1>` computed from the same earliest remaining deadline evaluated at response time (`deadline - now`, rounded up so clients never retry early). Do not copy stale upstream headers. A distinct `429` path is required because `handler.go:290-303` currently maps generic dispatch errors to 502. Zero upstream calls, zero upstream-attempt/selection/retry/usage attribution for skipped targets; client-facing status stays visible in HTTP accounting/metrics.
6. Reload/translation failures: cooldown store is runtime-owned (parallel to `Resolver`/`Health`), consulted by `dispatchAlias` combined with `tried` exclusions and rechecked before dispatch without holding locks during I/O. Effective target fingerprint = `(alias, provider, model, resolved base_url, credential identity, upstream model, protocol)`; algorithm and `retry_status_codes` changes do not invalidate deadlines (fairness/exhaustion logic reads current config). Retain deadlines for fingerprint-unchanged targets across `NewWithPrevious`/`App.Reload`; drop entries for removed/changed-fingerprint targets; failed reload leaves state untouched (reload returns before `UpdateDependencies`, `app.go:212-263`); in-flight completions from the old snapshot carrying the old fingerprint must not write to replacement identities. Capture advice at the lowest point where raw `*http.Response` headers exist (inside `executeUpstream`, `provider.go:330-365`, before `OnError`/`OnSuccess`/`OnStream` branching) so translation failures (`translate response: %w`), error-body read failures, and non-JSON errors still record advice; streaming advice is recorded at `OnStream` entry from headers only. `Result.Header` alone is insufficient because anthropic/gemini/opencode translated paths rebuild it as Content-Type-only (`anthropic.go:88-96,134-143`, `gemini.go:142-150`, `opencode.go:111-119`); avoid forwarding unrelated raw upstream headers just to retain metadata — plumb parsed deadline/expiry instead.
7. Maintainer input/blockers: none blocking. Acting as maintainer proxy, all six edges above are decided with rationale recorded; COOL-02 may proceed. Residual note for COOL-02: clock must be injectable for deterministic tests; concurrent writes must take max deadline (never shorten); expired/removed entries need cleanup to bound state.

Truth Table (selection + response; cooldown = active unexpired deadline):

| #   | Pool state at request start                                                | Last-response event                              | Selection behavior                                                                           | Upstream calls      | Client response                                                                                                                                                                               |
| --- | -------------------------------------------------------------------------- | ------------------------------------------------ | -------------------------------------------------------------------------------------------- | ------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1   | A cooling, B free                                                          | n/a                                              | Exclude A (combined with `tried` map); `Selector.Acquire` picks B under either algorithm     | B only, A untouched | B's real response                                                                                                                                                                             |
| 2   | A + B both actively cooling                                                | n/a (pre-dispatch check)                         | No eligible target; recheck expiry at decision boundary                                      | 0                   | Synthetic 429 with `min(remaining)` headers (ceil s + ceil ms)                                                                                                                                |
| 3   | A cooling, B unhealthy (health-marked, not cooling)                        | n/a                                              | A excluded by cooldown, B skipped by health (`IsHealthyContext`); not mislabeled all-cooling | 0                   | Existing exhaustion path (`alias has no healthy targets` → 502), not synthetic 429                                                                                                            |
| 4   | A cooling, B free; B returns retryable failure + valid advice that cools B | Newly cooling last target, uncommitted retryable | Record B deadline, discard B body, release lease once                                        | 1 (B only)          | Synthetic 429 immediately with `min(remaining A, remaining B)`                                                                                                                                |
| 5   | A cooling, B free; B returns success + valid advice                        | Success with advice                              | Record B deadline for future; return original                                                | 1 (B)               | B's 2xx verbatim (future requests see all-cooling until expiry)                                                                                                                               |
| 6   | A cooling, B free; B returns non-retryable error (e.g. 400) + valid advice | Non-retryable with advice                        | Record B deadline for future; return original                                                | 1 (B)               | B's error verbatim via existing path (downstream `Retry-After` still copied on JSON path per `TestHandlerPreservesJSONUpstreamErrors`); subsequent requests get synthetic 429 while both cool |

Verification (COOL-01, investigation only, no runtime changes): re-read `internal/httpapi/dispatch.go:87-227` (loop-index exhaustion, health skip, lease release, retryCodes), `:236-263` (health separation); `internal/alias/alias.go:17-59,67-99` (Acquire exclusions, both algorithms); `internal/provider/provider.go:41-50,330-365` (Result.Header boundary, executeUpstream branches); `internal/provider/anthropic.go:88-96,134-143`, `internal/provider/gemini.go:142-150`, `internal/provider/opencode.go:111-119,191-192` (Content-Type-only translated errors); `internal/httpapi/response.go:33-83,93-108` (non-JSON wrap drops headers, 429 type string); `internal/httpapi/handler.go:285-308` (502 mapping, 429 path needed) and `handler_test.go:1361-1385` (preservation vs interpretation); `internal/modelresolver/resolve.go:43-76` (selector reuse ignores credentials/endpoints); `internal/app/app.go:212-263` (reload snapshot/failed-reload semantics). No tests run (planning-only per file scope); no code, changelog, or COOL-02/COOL-03/DoD edits made.

### Task COOL-02: Implement Target Deadlines And Alias Exclusion

Status: completed

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

Completion (2026-09-07): implemented per COOL-01 decisions with no policy changes.

Commands and results (repository root):

- `go build ./...` — clean.
- `go vet ./internal/provider/ ./internal/modelresolver/ ./internal/httpapi/` — clean.
- `go test ./internal/provider/ -run 'TestParseRetry|TestCooldown|TestExecuteUpstream|TestSynthetic'` — pass (parser edges, capture on JSON/non-JSON/translated/translate-failure/streaming paths, synthetic headers).
- `go test ./internal/modelresolver/ -run TestCooldown` — pass (10/10: fingerprint identity, max-wins/expiry/cleanup, alias isolation, retain unchanged, drop rotated/removed/expired, algorithm+retry-code retention, clock carryover, concurrency, nil safety).
- `go test ./internal/httpapi/ -run 'TestAliasCooldown|TestDirect'` — pass (18/18: both-algorithm exclusion, 10s+30s/4s-later 6s-remaining synthetic with zero upstream calls, sub-second rounding, terminal/success/non-retryable final-target semantics, expiry, mixed health/cooldown 502, header-free failover, direct isolation + no failover, cross-alias isolation, cross-operation sharing, streaming, translate-error advice, canceled-context discard + lease release, HTTP-metrics visibility with no skipped-target attribution and no health mutation, concurrency, reload retain + old-after-reload isolation).
- `go test ./internal/alias ./internal/modelresolver ./internal/provider ./internal/httpapi ./internal/app` — all ok.
- `go test -race ./internal/alias ./internal/modelresolver ./internal/provider ./internal/httpapi ./internal/app` — all ok.
- `make vet test` — vet clean, all packages ok including `internal/e2e`.

Files changed:

- `internal/provider/retrycooldown.go` (new): `ParseRetryCooldown` (positive-integer `retry-after-ms` wins, else `Retry-After` delay-seconds then HTTP-date; case-insensitive, first-valid-wins, strict digits-only OWS-trimmed, 0/past/unparseable/overflow yields no cooldown from that header with ms→Retry-After fallback, saturation guard), `CooldownError` + `CooldownDelayFromError` for translate/body-read failure paths, `SyntheticCooldownResult` (429 JSON `upstream_rate_limited`, ceil `Retry-After` min 1 + `retry-after-ms` min 1 from same remaining deadline).
- `internal/provider/provider.go`: `Result.RetryDelay`/`HasRetryDelay` plumbed parsed delay; `executeUpstream` parses raw `*http.Response` headers once before `OnError`/`OnSuccess`/`OnStream` branching and attaches to every result or wraps post-response errors; `EffectiveBaseURL` helper for fingerprint resolution.
- `internal/modelresolver/cooldown.go` (new): alias-local `CooldownFingerprint` (alias, provider, model, resolved base_url, credential identity, upstream model, protocol), concurrency-safe `CooldownStore` with max-wins `Observe`, lazy-expiry `Remaining`, injectable clock, catalog-scoped `cloneForCatalog` (retain unchanged incl. algorithm/retry-code changes; drop removed/changed/expired).
- `internal/modelresolver/resolve.go`: `Resolver` owns the store; `NewWithPrevious` carries it over (failed reload never reaches it; old snapshots stay isolated); `Cooldowns()` accessor.
- `internal/httpapi/dispatch.go`: `dispatchAlias` combines cooldown exclusions with `tried` for both algorithms, rechecks before dispatch without holding locks during I/O, loop restructured off the loop-index assumption (break on empty acquisition), approved terminal policy (retryable failure completing all-cooling coverage → discard body, exactly-once lease release, synthetic 429; success/non-retryable verbatim), canceled contexts and `ErrInvalidRequest`/`ErrUnsupportedOperation` record nothing, no health mutation, skipped/synthetic targets unattributed. `dispatchDirect` untouched (never consults nor populates).
- Tests: `internal/provider/retrycooldown_test.go`, `internal/modelresolver/cooldown_test.go`, `internal/httpapi/cooldown_test.go` (all new files; no existing tests modified).

No `handler.go`, `response.go`, `app.go`, or `CHANGELOG.md` changes were needed: the synthetic 429 returns as a normal `*provider.Result` through `writeResult`, and reload preservation rides on the existing `NewWithPrevious`/`App.Reload` snapshot flow.

### Task COOL-03: Verify The Public Contract And Document It

Status: completed

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

Completion (2026-09-07, COOL-03 only; no COOL-01/COOL-02/DoD edits; no CHANGELOG.md):

Integration coverage: `internal/integration/binary_test.go`
(`TestBinaryAliasCooldownExclusionAndSynthetic`, plus `upstreamResponse.Headers`
support and a `postChatFull` status/header/body helper; no existing tests
modified). Two hermetic upstream stubs behind a `round_robin` alias with
`retry_status_codes = ["429"]`: stub A always answers `429` with
`retry-after-ms: 120000`; stub B answers `200` then `200` with
`retry-after-ms: 120000`. Request 1 returns `200 b-first` with A=1/B=1 calls
(retryable failover, A now cooling). Request 2 returns `200 b-second` with A
still 1 and B=2 (advice learned on request 1 excluded A on a later request).
Request 3 returns `429` with A and B call counts unchanged (zero upstream
calls) and asserts exact synthetic semantics: `Content-Type:
application/json`, both `Retry-After` and `Retry-After-Ms` present with
`Retry-After == ceil(retry-after-ms/1000)` (both >= 1, derived from the same
earliest remaining deadline), and body containing `"type":"upstream_rate_limited"`
with the matching `retry after <N>ms` message.

Docs (routing/operations sections only; no endpoint capability matrix
broadened): `README.md` (Routing bullets), `docs/design.md` (new `Upstream
Retry Cooldown` under Failure Handling), `website/docs/providers-and-routing.md`
(new `Upstream Retry Cooldown` section), `website/docs/api-reference.md` (Error
Behavior bullets), `AGENTS.md` (direct/alias routing + cooldown bullets),
`website/docs/operations.md` (Metrics And Health + Reload Behavior notes). All
six document identity scope, parsing/precedence, process-local lifetime, reload
policy, direct-request behavior, status-based failover interaction, and
original-vs-remaining delay, and explicitly note that another process and
previously admitted in-flight requests are not coordinated.

Release notes: the repository has no release-notes surface other than
`CHANGELOG.md`, which is banned from editing per user constraint, so no
release-notes file was changed. Proposed note for wherever release notes are
kept: "Alias targets now honor upstream `retry-after-ms`/`Retry-After` as a
process-local cross-request cooldown keyed by `(alias, provider, model)`.
Cooling targets are skipped until expiry; when all pool targets cool, the
proxy returns a generated JSON `429` (`upstream_rate_limited`) with
`Retry-After`/`retry-after-ms` from the earliest remaining delay and zero
upstream calls. Direct requests neither consult nor populate this state."

Independent review (fresh sub-agent, not the COOL-02 implementer): checked all
seven COOL-01 decisions against code and tests. (1) Identity: fingerprint is
`(alias, provider, model, base_url, credential, upstream model, protocol)`
(`modelresolver/cooldown.go:11-35`); `dispatchDirect` (`dispatch.go:21-85`)
has no cooldown path; cross-operation sharing and alias isolation covered by
`TestAliasCooldownSharedAcrossOperations`/`TestAliasCooldownIsolationAcrossAliases`.
(2) Triggers: `executeUpstream` (`provider.go:343-383`) parses raw headers
before every branch and wraps post-response errors via `CooldownError`;
dispatch records on all alias results, skips canceled/invalid/unsupported,
covered by streaming/translate-error/cancel tests. (3) Parsing: ms-wins with
ms→Retry-After fallback on zero/malformed, first-valid-wins, strict digits,
overflow guards (`retrycooldown.go:55-140`); synthetic uses ceiling min-1 from
remaining (`retrycooldown.go:175-188`). (4) Terminal policy matches code
(`dispatch.go:346-365`): retryable-last-target → discard + synthetic;
success/non-retryable verbatim; no synthesis after commit. (5) Synthetic type
reuses `upstreamErrorType(429)`; returns as a normal `Result` through
`writeResult`, which copies the headers — confirmed end-to-end by the new
binary test. (6) Reload rides `NewWithPrevious`/`App.Reload`; algorithm and
retry-code changes retained, failed reloads untouched, old in-flight isolated
(`resolve.go:44-62`, `cooldown.go:103-137`). (7) No blockers. Lease lifecycle
verified: `closeResult` invokes `OnClose` (`response.go:144-159`), so the
discard paths release every lease exactly once via `releaseOnce`; inflight
gauges asserted zero in cooldown tests. No findings blocking acceptance.

Residual limitations and follow-ups (not blocking): (a) `headerValuesFold`
iterates the Go header map, so two distinct map keys that compare equal
case-insensitively have nondeterministic order — unreachable for real wire
headers (net/http canonicalizes) and only observable in hand-built maps. (b)
A translate/body-read error that newly cools the last target returns the
mapped `502`, not synthetic `429` — consistent with decision 4's
retryable-status scope, but undocumented in the truth table. (c) No
cross-process, Redis, dashboard, or config surface by design (contract item 8).

Commands and results (repository root, run serially):

- `make vet test` — vet clean; all packages ok (integration shows `[no test
files]` without the tag, as before).
- `make test-race` — all packages ok with `-race`.
- `make integration` — ok (`2.062s`), including the new
  `TestBinaryAliasCooldownExclusionAndSynthetic` (also run standalone:
  `AIPROXY_BINARY="$(pwd)/dist/aiproxy" go test -tags=integration
./internal/integration/ -run TestBinaryAliasCooldownExclusionAndSynthetic -v`
  — PASS `0.24s`).
- `make docs-contract` — `documentation contract matrices match`.
- No real-provider credentials or external requests used in any check.

## Definition Of Done

- COOL-01 decisions are recorded and COOL-02/COOL-03 acceptance criteria pass with command/result evidence appended here.
- No cooling target is intentionally selected after its advice is visible to dispatch; all-cooling responses expose the earliest remaining deadline without false upstream attribution.
- No provider-wide health regression, cross-service fallback, lease leak, unbounded state growth, or stale reload update is introduced.
- Unrelated worktree changes remain intact. No implementation task is complete merely because this plan exists.
- Final Review (2026-09-07): verified COOL-01/02/03 Status completed with Decisions + Truth Table + Verification / commands+results + files changed / integration + docs + independent review + commands+results; `git status --short` shows only expected COOL-02/03 + docs edits plus pre-existing unrelated `helm/main/Makefile` intact; `git diff --stat` shows no `CHANGELOG.md` change; `retrycooldown.go`, `cooldown.go`, new tests, binary test, and docs edits all present; `go vet ./internal/provider/ ./internal/modelresolver/ ./internal/httpapi/` clean.
