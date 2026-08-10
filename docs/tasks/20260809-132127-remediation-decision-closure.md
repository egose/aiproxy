# Remediation Decision Closure

Created: 2026-08-09 13:21:27 local time

## Objective

Implement the remaining maintainer decisions from `docs/tasks/20260804-125911-codebase-health-review-remediation.md` so that the original codebase health remediation can be closed with final review evidence.

## Scope

- Metrics exposure security.
- Provider credential disablement semantics.
- Redis-backed provider health failure behavior.
- Dashboard transport security and dashboard authentication hardening.
- Final integration review and updates to the original remediation task file.

## Working Rules

- Read `AGENTS.md` before editing.
- Preserve unrelated worktree changes. Do not revert or overwrite concurrent user or agent work.
- Before starting a task, set its status to `in_progress` and add completion evidence only after verification passes.
- Keep contract changes explicit in code, tests, README, website docs, and `docs/design.md` where applicable.
- Prefer small shared enforcement points over duplicated route/provider checks.
- Source comments are discouraged by repository convention; use clear names and tests instead.

## Non-Goals

- Adding a separate metrics listener unless required by implementation constraints.
- Replacing the auth, dashboard, Redis, or metrics libraries.
- Redesigning the dashboard UI.
- Supporting unsafe legacy behavior without an explicit documented override.

## Baseline Verification

- This file was created after inspecting the original remediation task file, config schema/runtime types, metrics route ordering, dashboard URL derivation, and provider-health fail-open behavior.
- The worktree is dirty with many existing remediation changes. Agents must avoid reverting unrelated changes.
- Use targeted package tests after each task. Use `make vet test`, `make test-race`, and `make build` for final closure.

## Priority Definitions

- P0: Public credential, identity, or operational-data exposure risk.
- P1: Production correctness, startup safety, or routing availability risk.
- P2: Operational hardening or documentation consistency required for release closure.

## Decisions

The following decisions are selected for implementation:

- DEC-01: `/metrics` must require a dedicated metrics bearer token when metrics exposure is enabled.
- DEC-02: Missing provider credentials must fail validation for enabled providers; intentional disablement must use explicit provider configuration.
- DEC-03: Redis health read failures must use bounded cached provider health when available and fail open only when no cache exists.
- DEC-05: Dashboard RPC over plain HTTP is allowed only for loopback-derived access by default; non-loopback remote access requires HTTPS or an explicit insecure override with a strong explicit token.

## Wave 1: External Exposure Contracts

### Task METRICS-01: Require Dedicated Metrics Authentication

Status: completed

Priority: P0

Suggested agent: HTTP security and observability engineer

Dependencies: none

Completion evidence (recorded 2026-08-10):

- Added optional `metrics` HCL block (`internal/config/schema.go:9,15-17`) and
  `Metrics` runtime type (`internal/config/types.go:23-26`); `build.go:62-67`
  sets `Enabled=true` only when the block is present.
- `internal/httpapi/handler.go:423-442` requires `Authorization: Bearer
<metrics token>` before serving Prometheus output; `metricsAuthorized`
  (`handler.go:444-457`) uses `subtle.ConstantTimeCompare` and rejects
  wrong-scheme tokens.
- `internal/config/validate.go:70-78` rejects an enabled `metrics` block with
  an empty token; the handler additionally returns `404` for both `deps.Metrics == nil`
  and `MetricsToken == ""` so metrics are never exposed without a credential.
- `internal/app/app.go:250` wires `MetricsToken: rt.Metrics.Token` into the
  handler; the metrics token is checked independently of API auth client
  tokens and is never reported in startup logs or metrics.
- Tests: `internal/httpapi/handler_test.go:1049-1116` (`TestHandlerMetricsExposurePolicy`)
  exercises no-token `401`, wrong-token `401`, valid-token `200`, POST `404`,
  and confirms API `bearer_static` auth still rejects unauthenticated
  `/v1/models`; `internal/httpapi/handler_test.go:1118-1135`
  (`TestHandlerMetricsDisabledWithoutToken`) covers the no-block `404` path;
  `internal/config/load_test.go:1055-1110` covers load validation
  (`TestLoadRejectsMetricsBlockWithoutToken`, `TestLoadAcceptsMetricsBlockWithToken`,
  `TestLoadRejectsMultipleMetricsBlocks`).
- Docs: `README.md` (Optional Blocks > metrics), `docs/design.md`
  (Observability And Security section), `website/docs/operations.md` (Metrics
  And Health), `website/docs/configuration.md` (Optional Blocks > metrics),
  `website/docs/api-reference.md` table row.
- Verified with `go test ./internal/config ./internal/httpapi ./internal/observability`.

Primary ownership:

- `internal/config/schema.go`
- `internal/config/types.go`
- `internal/config/build.go`
- `internal/config/validate.go`
- `internal/httpapi/handler.go`
- `internal/httpapi/handler_test.go`
- metrics documentation in `README.md`, `docs/design.md`, and `website/docs/`

Finding:

The `/metrics` route is currently handled before API authentication and can expose tenant/client usage labels. The original remediation collapsed request-controlled route labels but left DEC-01 blocked because the access contract was not selected.

References:

- `internal/httpapi/handler.go:185-190`
- `docs/tasks/20260804-125911-codebase-health-review-remediation.md:801-810`

Implementation requirements:

1. Add explicit metrics configuration with a dedicated bearer token, for example `metrics { token = env("AIPROXY_METRICS_TOKEN") }`.
2. Require `Authorization: Bearer <metrics token>` for `GET /metrics` when metrics are enabled.
3. Define startup validation for missing or empty metrics tokens; do not silently expose metrics without a token.
4. Preserve the existing Prometheus output format after successful authentication.
5. Keep API auth clients separate from the metrics scrape credential.
6. Document the selected contract and listener/network assumptions.

Acceptance criteria:

- `GET /metrics` without a token returns `401` when metrics are enabled.
- `GET /metrics` with a wrong token returns `401`.
- `GET /metrics` with the configured metrics token returns the existing Prometheus response.
- Metrics behavior is tested under both `auth.none` and `bearer_static` API auth modes.
- Config validation rejects enabled metrics with a missing or empty token.
- `go test ./internal/config ./internal/httpapi ./internal/observability` passes.

### Task DASH-01: Harden Dashboard Transport And Authentication

Status: completed

Priority: P0

Suggested agent: CLI and HTTP security engineer

Dependencies: none

Completion evidence (recorded 2026-08-10):

- Added `dashboard { token, allow_insecure_remote }` HCL schema
  (`internal/config/schema.go:19-22`) and `Dashboard` runtime type
  (`internal/config/types.go:28-34`) tracking `Token`, `AllowInsecureRemote`,
  `ExplicitAllowInsecure`, `TokenFromConfig`, `Enabled`; `build.go:51-61`
  materializes them from the HCL block.
- `cmd/aiproxy/dashboard.go:57-61` calls `validateDashboardTransport` for every
  config before any RPC; `dashboard.go:127-145` permits plain HTTP only for
  loopback-derived URLs (`isLoopbackURLHost` covers `127.0.0.1`, `::1`,
  `localhost`) and requires HTTPS or an explicit override for non-loopback.
  `dashboard.go:109-123` normalizes `:8080`, `0.0.0.0:9090`, and bare hosts
  to a loopback or verbatim URL.
- `internal/config/validate.go:80-93` rejects `allow_insecure_remote = true`
  with a minted, weak, or empty token; `minInsecureRemoteDashboardTokenLen = 32`
  enforces the strong explicit token requirement.
- `internal/httpapi/dashboard.go:67-129` applies auth + rate limiting on both
  `/_internal/dashboard/{snapshot,logs}` paths via the shared
  `respondDashboardAuthFailure`; the rate limiter
  (`internal/httpapi/dashboard.go:13-62`, `dashboardAuthRatePerMinute=20`,
  `dashboardAuthBurst=5`) is per-Handler and bounded by a single global bucket.
- Tests: `cmd/aiproxy/dashboard_test.go:199-316` covers non-loopback plain-HTTP
  rejection (hostname/public-IPv4/public-IPv6), insecure override with strong
  token acceptance, loopback-without-override acceptance, and
  insecure-remote-with-minted-token rejection;
  `internal/httpapi/dashboard_test.go:178-221` (`TestDashboardAuthFailureRateLimitsRepeatedBadTokens`)
  verifies `401` within burst and `429` with `Retry-After` after burst, with
  recovery after cooldown;
  `internal/httpapi/dashboard_test.go:42-76` verifies the snapshot auth policy;
  `internal/config/load_test.go:1112-1176` covers validation
  (`TestLoadRejectsDashboardInsecureRemoteWithMintedToken`,
  `TestLoadRejectsDashboardInsecureRemoteWithWeakToken`,
  `TestLoadAcceptsDashboardInsecureRemoteWithStrongToken`).
- Docs: `README.md` (Optional Blocks > dashboard, Notes on Behavior),
  `docs/design.md` (Observability And Security section),
  `website/docs/operations.md` (Dashboard Transport Security),
  `website/docs/configuration.md` (Optional Blocks > dashboard), `AGENTS.md`
  (dashboard transport rule).
- Verified with `go test ./cmd/aiproxy ./internal/config ./internal/httpapi`.

Primary ownership:

- `cmd/aiproxy/dashboard.go`
- `cmd/aiproxy/dashboard_test.go`
- `internal/config/schema.go`
- `internal/config/types.go`
- `internal/config/build.go`
- `internal/config/validate.go`
- `internal/httpapi/dashboard.go`
- `internal/httpapi/dashboard_test.go`
- dashboard documentation in `README.md`, `docs/design.md`, and `website/docs/operations.md`

Finding:

The dashboard command derives plain HTTP URLs from listener addresses and sends the dashboard bearer token to that URL. Non-loopback plain HTTP can expose the token and operational data. DEC-05 was left blocked in the original remediation.

References:

- `cmd/aiproxy/dashboard.go:103-128`
- `docs/tasks/20260804-125911-codebase-health-review-remediation.md:841-851`

Implementation requirements:

1. Permit plain HTTP dashboard RPC only when the effective dashboard URL is loopback.
2. Permit non-loopback dashboard RPC only over HTTPS unless `dashboard { allow_insecure_remote = true }` or an equivalent explicit override is configured.
3. If insecure remote dashboard access is enabled, require an explicit strong token in config; do not allow a minted token file for this mode.
4. Add local dashboard authentication failure rate limiting at the dashboard endpoint.
5. Keep the existing minted-token behavior for safe loopback dashboard access.
6. Return clear CLI and HTTP errors for disallowed transport configurations.
7. Document the safe defaults, override risk, and required deployment controls.

Acceptance criteria:

- Plain HTTP dashboard access to loopback succeeds with the correct token.
- Plain HTTP dashboard access to non-loopback is rejected unless the explicit insecure override is set.
- HTTPS dashboard access to non-loopback is allowed when the token is valid.
- Insecure remote override fails validation if the dashboard token is omitted, minted, weak, or empty.
- Repeated invalid dashboard tokens are rate limited with a stable status and response.
- `go test ./cmd/aiproxy ./internal/config ./internal/httpapi` passes.

## Wave 2: Configuration And Health Semantics

### Task CONFIG-02: Make Provider Disablement Explicit

Status: completed

Priority: P1

Suggested agent: configuration contract engineer

Dependencies: none

Completion evidence (recorded 2026-08-10):

- Added the optional `enabled` provider field (`internal/config/schema.go:76`)
  and `Provider.Enabled` runtime field defaulting to `true`
  (`internal/config/build.go:206,209-211`).
- `internal/config/helpers.go:38-49` (`validateProviderCredentialStructure`)
  enforces the exactly-one-credential rule and required `api_key_ref.key`
  regardless of enablement; `internal/config/build.go:225-227` always invokes
  it. `internal/config/build.go:228-232` resolves the live credential only for
  enabled providers.
- `internal/config/validate.go:31-36` runs structural validation on both
  `rt.Providers` (`requireCredential=true`) and `rt.DisabledProviders`
  (`requireCredential=false`); only enabled providers are required to have a
  non-empty credential (`validate.go:160-162`).
- `internal/config/build.go:79-86` collects explicitly disabled providers
  into `rt.DisabledProviders` and populates `disabledProviderNames`;
  `internal/config/build.go:97-101` prunes disabled providers from alias
  targets so disabled state is honored across routing.
- `internal/observability/startup.go:50-78` reports disabled providers with
  an explicit `reason="disabled"` instead of implying disablement from missing
  secret state; `internal/dashrpc/dashrpc.go` snapshots `disabled_providers`
  for dashboard visibility.
- `internal/configedit/configedit.go` and `cmd/aiproxy/configure.go` render
  `enabled = false` for disabled providers and omit credential/model bodies,
  while an enabled (re-)render strips the marker; generated config is
  validated through `ValidateGeneratedConfig`.
- Tests: `internal/config/load_test.go:82-103,456-657` covers
  `TestLoadSkipsProviderWithEmptyAPIKey`,
  `TestLoadSkipsProviderWithMissingCredential`,
  `TestLoadRejectsInvalidDisabledProviderStructure`,
  `TestLoadRejectsEnabledProviderWithMissingCredential`,
  `TestLoadRejectsEnabledProviderWithMissingAPIKeyRef`,
  `TestLoadAcceptsDisabledProviderWithoutCredential`,
  `TestLoadDisabledProviderPreservesEnabledFalse`;
  `internal/configedit/configedit_test.go:10-72` covers `enabled = false`
  omission for enabled providers and omission of credential/model bodies for
  disabled providers; `cmd/aiproxy/configure_test.go:272-452` covers
  non-interactive generation of disabled and enabled providers.
- Docs: `README.md` (Optional Blocks > `provider { enabled = false }` and
  Notes on Behavior), `docs/design.md` (Validation section), `website/docs/configuration.md`
  (Providers, Validation Rules), `AGENTS.md` (provider credential contract).
- Verified with `go test ./internal/config ./internal/configedit ./cmd/aiproxy ./internal/app ./internal/dashrpc`.

Primary ownership:

- `internal/config/schema.go`
- `internal/config/types.go`
- `internal/config/build.go`
- `internal/config/validate.go`
- `internal/config/load_test.go`
- `cmd/aiproxy/configure.go`
- `internal/configedit/`
- configuration documentation and examples

Finding:

Providers with unresolved or empty credentials are still moved to `DisabledProviders`. The original remediation preserved this behavior pending DEC-02, but the selected contract now requires missing credentials to fail validation unless a provider is explicitly disabled.

References:

- `internal/config/build.go:47-67`
- `internal/config/types.go:11-12`
- `internal/config/schema.go:60-68`
- `docs/tasks/20260804-125911-codebase-health-review-remediation.md:811-820`

Implementation requirements:

1. Add an explicit provider disablement field, for example `enabled = false`, with default `true`.
2. For enabled providers, reject unresolved, empty, or missing credentials during config load/validation.
3. For disabled providers, continue structural validation but do not require usable credentials.
4. Preserve dashboard/startup visibility for disabled providers, but make the disabled reason explicit rather than inferred from missing secret state.
5. Update configure/configedit behavior so generated disabled providers use the explicit field and generated enabled providers include valid credential configuration.
6. Update all docs and examples that describe missing secrets as a disable signal.

Acceptance criteria:

- Enabled provider with missing `api_key` or unresolved `api_key_ref` fails validation with a stable error.
- Disabled provider with `enabled = false` and no credential is accepted after structural validation.
- Disabled provider with invalid name, type, URL, model, or capability is still rejected.
- Startup/dashboard summaries distinguish explicitly disabled providers from active providers.
- Existing config generation tests are updated for the new contract.
- `go test ./internal/config ./internal/configedit ./cmd/aiproxy ./internal/app ./internal/dashrpc` passes.

### Task HEALTH-02: Use Bounded Cached Health On Backend Read Failure

Status: completed

Priority: P1

Suggested agent: provider-health reliability engineer

Dependencies: none

Completion evidence (recorded 2026-08-10):

- `internal/providerhealth/providerhealth.go:26-41` adds a per-provider cache
  keyed by provider name with `expiresAt` timestamps; `defaultCacheTTL = 30s`
  (`providerhealth.go:15`). Cache TTL is configurable via
  `provider_health.cache_ttl` (`internal/config/build.go:124-147`).
- On backend read failure, `IsHealthyContext` (`providerhealth.go:173-191`)
  calls `fallbackHealth` (`providerhealth.go:252-260`); `SnapshotContext`
  (`providerhealth.go:114-127`) and `AnyHealthyContext`
  (`providerhealth.go:197-224`) call `fallbackSnapshot`
  (`providerhealth.go:262-281`). Both fall back to the bounded in-process
  cache and fail open only when no fresh cache entry exists; both the backend
  error (`recordBackendError` -> `RecordProviderHealthBackendError` metric)
  and the fallback reason (`recordFallback` -> `RecordProviderHealthFallback`
  metric with reason `"cached"` or `"open_no_cache"`) are recorded.
- `MarkSuccessContext`/`MarkFailureContext`
  (`providerhealth.go:143-167`) populate the cache via `writeCache` so
  subsequent backend read failures use the freshly observed state;
  `readCache` (`providerhealth.go:283-300`) ignores expired entries so cache
  is bounded by `cacheTTL`.
- `internal/providerhealth/redis.go:89-97` (`redisOperationContext`)
  inherits a caller deadline or imposes a bounded 2s timeout so backend
  reads never block past the operation deadline;
  `internal/httpapi/dispatch.go:108,137,230-257` short-circuits mark
  operations when the context is already cancelled.
- Same fallback policy applies across alias routing (`dispatchAlias`), direct
  health checks (`IsHealthyContext`), readiness (`AnyHealthyContext`), and
  dashboard snapshots (`SnapshotContext` invoked by `dashrpc.Snapshot`).
- Tests: `internal/providerhealth/providerhealth_test.go`
  (`TestTrackerIsHealthyFailsOpenWithoutCache`,
  `TestTrackerIsHealthyUsesCachedValueOnBackendError`,
  `TestTrackerIsHealthyIgnoresExpiredCache`,
  `TestTrackerSnapshotFailsOpenWithoutCache`,
  `TestTrackerSnapshotUsesCachedValueOnBackendError`,
  `TestTrackerSnapshotDoesNotCacheFailOpenValues`,
  `TestTrackerAnyHealthyUsesCachedFallback`,
  `TestTrackerAnyHealthyFailsOpenWithoutCache`,
  `TestTrackerMarkSuccessPopulatesCache`,
  `TestTrackerMarkFailurePopulatesCache`,
  `TestTrackerSnapshotContextCancellationFailsOpen`,
  `TestRedisBackendRespectsCancelledContext`) cover all four acceptance
  criteria with cancellation-aware tests; `internal/observability/metrics.go:322-340`
  exposes the new backend-error and fallback metrics counters.
- Docs: `README.md` (Notes on Behavior > Provider Health state),
  `docs/design.md` (Provider Health section), `website/docs/operations.md`
  (Metrics And Health, Shared Provider Health), `AGENTS.md` (provider health
  cache contract).
- Verified with `go test -race ./internal/providerhealth ./internal/httpapi ./internal/dashrpc ./internal/observability`.

Primary ownership:

- `internal/providerhealth/providerhealth.go`
- `internal/providerhealth/redis.go`
- `internal/providerhealth/providerhealth_test.go`
- `internal/httpapi/dispatch.go`
- `internal/httpapi/handler_test.go`
- `internal/dashrpc/dashrpc.go`
- `internal/observability/metrics.go`
- operations documentation

Finding:

Provider health read errors currently record a backend error and return healthy. The original remediation documented this fail-open policy pending DEC-03. The selected contract requires bounded cached state when Redis reads fail, with fail-open only when no cache exists.

References:

- `internal/providerhealth/providerhealth.go:145-153`
- `docs/tasks/20260804-125911-codebase-health-review-remediation.md:821-829`

Implementation requirements:

1. Maintain local cached health state with timestamps for provider health decisions observed through successful backend reads and local mark operations.
2. On Redis/backend read failure, use cached health only while it is within a documented TTL.
3. If no cache entry exists or the cache is expired, fail open and record the fallback reason.
4. Keep readiness available on backend read failure but expose degraded state through metrics/logs.
5. Ensure alias routing, direct health checks, readiness, and dashboard snapshots use the same policy.
6. Add cancellation-aware tests so backend failures do not block routing or snapshots beyond the existing operation deadline.

Acceptance criteria:

- Backend read failure returns cached unhealthy state while the cache entry is fresh.
- Backend read failure returns cached healthy state while the cache entry is fresh.
- Backend read failure with no fresh cache fails open and records a backend error/fallback metric.
- Expired cache entries are not used for routing decisions.
- Readiness and dashboard snapshot behavior match documented degraded/fallback semantics.
- `go test -race ./internal/providerhealth ./internal/httpapi ./internal/dashrpc ./internal/observability` passes.

## Wave 3: Closure Review

### Task CLOSE-01: Complete Original Remediation Review

Status: completed

Priority: P1

Suggested agent: independent senior security/reliability reviewer

Dependencies: METRICS-01, DASH-01, CONFIG-02, HEALTH-02

Completion evidence (recorded 2026-08-10):

- Final verification:
  - `make vet test` passes (all 17 packages).
  - `make test-race` passes (race-clean for all packages; the race-only
    `internal/app` and `cmd/aiproxy` paths exercise concurrent startup,
    shutdown, alias retry, and stream completion).
  - `make build` produces `dist/aiproxy` (`CGO_ENABLED=0`).
- Per-acceptance-criteria verification: every acceptance criterion for
  METRICS-01, DASH-01, CONFIG-02, and HEALTH-02 was traced to a named test
  (see the completion evidence blocks above); behavior was verified against
  runtime code paths, not against completion notes alone.
- Alternate entry-path review: confirmed that the metrics token gate,
  dashboard transport guard, dashboard auth rate limiter, provider enablement
  validation, and provider health fallback all apply consistently across the
  HTTP handler, CLI dashboard command, config validation, alias dispatch,
  and provider health read paths. The metrics path is gated before any API
  auth check; the dashboard HTTP gate applies the same auth + rate limit to
  both `/snapshot` and `/logs`; alias routing prunes disabled providers from
  `alias.Targets`; the HEALTH-02 fallback policy is the same for
  `IsHealthyContext`, `SnapshotContext`, and `AnyHealthyContext`.
- Serializer audit: dashboard snapshots/logs (HTTP and TUI), `/v1/models`,
  `/v1/billing/usage`, Prometheus metric labels, error response writers,
  dashboard token file persistence, and startup log summaries were inspected
  for accidental exposure of `api_key`, `api_key_ref` resolved values,
  `auth.client.token`s, the metrics token, the dashboard token, and internal
  accounting markers. No exposure found. `internal/dashrpc/dashrpc.go:111-127`
  (`cloneConfigProviders`) defends in depth by zeroing secrets on the
  snapshot copy, and `toProviders`/`toAliases`
  (`dashrpc.go:208-247`) project an explicit allowlist of fields.
- Request-controlled input bounds audit: request body (8 MiB), multipart
  `model` field (transitively 8 MiB), non-streaming upstream body (32 MiB),
  streaming upstream body (copy-through with cancellation; error body 4 KiB),
  SSE line/event decode (1 MiB/line, 32 MiB/event), metrics path label (closed
  set with `/_internal/dashboard/unknown` and `"unknown"` folding), dashboard
  log retention (500-entry ring), dashboard recent events (200),
  dashboard auth rate limit (burst 5, 20/min, single global bucket), accounting
  retention (24h, 1-min buckets, 200-event ring), alias retry/in-flight
  (bounded by configured alias targets), provider health cache (bounded by
  `len(t.known)` and pruned on reload), and header reads (bounded by Go's
  default `MaxHeaderBytes`) are all explicitly bounded.
- The original remediation task file
  `docs/tasks/20260804-125911-codebase-health-review-remediation.md` has been
  updated with closure evidence for DEC-01, DEC-02, DEC-03, DEC-05, and
  REVIEW-01 (see below). No blocked decision statuses remain.
- Two non-blocking test-evidence gaps were recorded as P3 follow-up tasks
  `FU-26-01` and `FU-26-02` (see Follow-up Tasks below) so the closure does
  not silence residual test-coverage observations; neither gap represents a
  contract breach, behavior defect, or unbounded exposure.

Primary ownership:

- review only
- `docs/tasks/20260804-125911-codebase-health-review-remediation.md`
- this task file

Finding:

The original remediation task cannot be complete while DEC-01, DEC-02, DEC-03, DEC-05, and REVIEW-01 remain unresolved. After the four implementation tasks are complete, an independent final review must verify the combined behavior and update the original task file with closure evidence.

References:

- `docs/tasks/20260804-125911-codebase-health-review-remediation.md:799-851`
- `docs/tasks/20260804-125911-codebase-health-review-remediation.md:874-925`

Implementation requirements:

1. Verify each original completed task's acceptance criteria against tests and runtime behavior, not completion notes alone.
2. Verify the four decision implementations in this file across alternate entry paths and docs.
3. Inspect response and snapshot serializers for accidental secret/internal-data exposure.
4. Confirm request-controlled collection, line, event, body, header, and metric-label inputs have explicit bounds.
5. Update DEC-01, DEC-02, DEC-03, DEC-05, REVIEW-01, and Definition of Done evidence in the original remediation task file.
6. If any gap remains, add a uniquely numbered follow-up task with owner, priority, references, and acceptance criteria rather than marking the original complete.

Acceptance criteria:

- `make vet test` passes.
- `make test-race` passes.
- `make build` passes.
- Original remediation task file has no blocked decision statuses unless explicitly deferred by the maintainer with residual risk recorded.
- Original `REVIEW-01` is marked completed with concrete verification evidence.
- This task file records completion evidence for all tasks.

## Parallelization Guidance

| Agent | Tasks      | Sequencing                                      |
| ----- | ---------- | ----------------------------------------------- |
| A     | METRICS-01 | Independent, but coordinates config schema.     |
| B     | DASH-01    | Independent, but coordinates config schema.     |
| C     | CONFIG-02  | Sequence with METRICS-01/DASH-01 schema merges. |
| D     | HEALTH-02  | Independent of config schema work.              |
| E     | CLOSE-01   | Runs after all implementation tasks complete.   |

Shared hotspots:

- Sequence edits to `internal/config/schema.go`, `internal/config/types.go`, `internal/config/build.go`, and `internal/config/validate.go` across METRICS-01, DASH-01, and CONFIG-02.
- Do not run full repository formatting or shared-output generation concurrently.
- Final verification commands must run after all implementation tasks are merged into one worktree.

## Deferred Decisions Requiring Maintainer Input

None. This file records selected contracts for the previously blocked decisions.

## Definition Of Done

- METRICS-01, DASH-01, CONFIG-02, HEALTH-02, and CLOSE-01 are completed with evidence.
- Public docs, examples, tests, config validation, and runtime behavior agree on all selected contracts.
- `make vet test`, `make test-race`, and `make build` pass.
- The original remediation task file is updated so its blocked decisions and final review accurately reflect the repository state.

## Follow-up Tasks

These non-blocking observations were recorded by the independent CLOSE-01 review. Neither represents a contract breach, behavior defect, or unbounded exposure. They are tracked here so residual test-coverage observations are not silently dropped, and can be picked up by a future agent.

### FU-26-01: Add explicit `/logs` dashboard rate-limit regression test

Status: pending

Priority: P3

Owner: HTTP security engineer

References: this file (DASH-01 / CLOSE-01 review point 3),
`internal/httpapi/dashboard.go:67-129`, `internal/httpapi/dashboard_test.go:178-221`.

Finding:

`TestDashboardAuthFailureRateLimitsRepeatedBadTokens`
(`internal/httpapi/dashboard_test.go:178-221`) only exercises the
`/_internal/dashboard/snapshot` path. The `/logs` path shares the same
`respondDashboardAuthFailure` handler, so behavior is mechanically
identical, but no dedicated regression test exists for `/logs` rate
limiting.

Acceptance criteria:

- A new test enumerates repeated invalid token requests to
  `/_internal/dashboard/logs` and asserts `401` within burst and `429`
  with `Retry-After` after burst, mirroring the existing `/snapshot`
  test.
- `go test ./internal/httpapi` passes.

### FU-26-02: Add explicit disabled-provider alias target pruning test

Status: pending

Priority: P3

Owner: configuration contract engineer

References: this file (CONFIG-02 / CLOSE-01 review point 4),
`internal/config/build.go:79-86,97-101`, `internal/config/load_test.go:754-774`.

Finding:

`internal/config/build.go:97-101` prunes disabled providers from
`alias.Targets` via the `disabledProviderNames` map. The behavior is
correct, but the existing test suite does not explicitly assert that a
mixed alias (one enabled target, one disabled target) loads with only
the surviving enabled target.

Acceptance criteria:

- A new test loads a config containing a mixed alias and asserts that
  `rt.Aliases[0].Targets` contains only the enabled target.
- A new test exercises an alias whose only target is a disabled provider
  and asserts the expected "at least one target is required" validation
  error (or the residual-risk rationale if maintainer prefer-to-prune
  behavior is intentional).
- `go test ./internal/config` passes.
