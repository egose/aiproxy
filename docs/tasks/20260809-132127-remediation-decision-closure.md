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

Status: pending

Priority: P0

Suggested agent: HTTP security and observability engineer

Dependencies: none

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

Status: pending

Priority: P0

Suggested agent: CLI and HTTP security engineer

Dependencies: none

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

Status: pending

Priority: P1

Suggested agent: configuration contract engineer

Dependencies: none

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

Status: pending

Priority: P1

Suggested agent: provider-health reliability engineer

Dependencies: none

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

Status: pending

Priority: P1

Suggested agent: independent senior security/reliability reviewer

Dependencies: METRICS-01, DASH-01, CONFIG-02, HEALTH-02

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
