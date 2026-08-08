# Codebase Health Review Remediation

Created: 2026-08-04 12:59:11 local time

## Objective

Remediate confirmed readability, security, correctness, performance, and architectural-health gaps found during a repository-wide static review. The plan is written for independent sub-agents and prioritizes unsafe lifecycle behavior, request-controlled resource growth, routing correctness, stream ownership, secure persistence, and runtime encapsulation.

## Scope

- Go server, CLI, configuration, provider adapters, routing, health, metrics, accounting, and tests.
- Documentation updates where behavior or external contracts change.
- Small, shared enforcement points rather than duplicated route/provider fixes.

## Working Rules

- Read `AGENTS.md` before starting and preserve unrelated worktree changes. Never revert files outside the assigned task.
- Set a task to `in_progress` before editing and append completion evidence only after its required verification passes.
- Add regression tests that fail against the current implementation before or with each fix.
- Keep externally visible behavior unchanged unless the task explicitly defines a contract correction.
- Do not add compatibility paths for unsafe or contradictory behavior without a concrete consumer requirement.
- Source comments are discouraged by repository convention; prefer clear APIs and names.
- Agents sharing `internal/httpapi/dispatch.go`, `internal/httpapi/handler.go`, `internal/provider/provider.go`, `internal/app/app.go`, or `cmd/aiproxy/configure.go` must run sequentially.

## Non-Goals

- Replacing HCL, Cobra, Prometheus, Redis, or the provider transport stack.
- Adding new providers or broad provider feature support.
- Cosmetic rewrites without an observable correctness, ownership, or testability outcome.
- Treating trusted operator configuration as hostile input unless a task explicitly changes that trust boundary.

## Baseline Verification

Review baseline on 2026-08-04:

- `make vet`: passed.
- `make test`: passed for all packages.
- `docs/tasks/` did not previously exist.
- Worktree contained untracked `.opencode/`; it is unrelated and must not be modified or removed by these tasks.

Use targeted `go test ./path/to/package` after each task. Use `make vet test` after each wave. Run `make test-race` for concurrency and lifecycle tasks and at final integration.

## Priority Definitions

- P0: Can signal unrelated processes, permit unbounded remotely triggered resource growth, or materially expose credentials/data.
- P1: Production correctness, availability, billing, routing, or secret-integrity defect.
- P2: Bounded operational/performance issue, contract inconsistency, or architectural debt with concrete impact.
- P3: Low-risk consistency or documentation correction.

## Wave 1: Lifecycle And Input Safety

### Task LIFE-01: Make Daemon Identity And Startup Safe

Status: pending

Priority: P0

Suggested agent: Unix process lifecycle specialist

Dependencies: none

Primary ownership:

- `cmd/aiproxy/daemon.go`
- `cmd/aiproxy/daemon_test.go`
- `cmd/aiproxy/lifecycle_test.go`
- focused lifecycle documentation

Finding:

The daemon state stores only a PID and considers any live process with that PID to be aiproxy. `stop` can send `SIGTERM` and eventually `SIGKILL` to an unrelated process after PID reuse. Startup also has a check-then-start race and reports success before config loading or listener binding succeeds. The lifecycle `--config` value is accepted by stop/status but ignored when resolving daemon identity.

References:

- `cmd/aiproxy/daemon.go:35-37`
- `cmd/aiproxy/daemon.go:55-81`
- `cmd/aiproxy/daemon.go:85-100`
- `cmd/aiproxy/daemon.go:138-168`
- `cmd/aiproxy/daemon.go:172-180`
- `cmd/aiproxy/daemon.go:189-225`

Implementation requirements:

1. Replace PID-only state with an owned daemon-state record that can verify process start identity and expected executable before any signal is sent.
2. Decide one coherent config contract: scope daemon state by canonical config identity, or remove the misleading stop/status config flags and document one daemon per user.
3. Serialize concurrent starts with an exclusive lifecycle lock held through readiness publication.
4. Add a parent-child startup handshake. The parent may report success and publish final state only after config is loaded and the listener is bound; propagate early child errors and time out safely.
5. Clean up child, lock, state, and file handles on every failure path, including `Process.Release` failure.
6. Preserve detached logging and the existing stop timeout unless tests demonstrate a required change.

Acceptance criteria:

- Stop/status refuse a live PID whose recorded process identity no longer matches, without signaling it.
- Two concurrent starts produce exactly one running server and one controlled error.
- Invalid config and occupied-listener starts return non-zero without a stale live state file.
- Malformed/truncated state and stale processes produce `no server running` without deleting state owned by a verified different process.
- Focused lifecycle tests and `go test -race ./cmd/aiproxy` pass.

### Task OBS-01: Bound HTTP Metric Labels And Clarify Metrics Exposure

Status: pending

Priority: P0

Suggested agent: observability and HTTP security engineer

Dependencies: none

Primary ownership:

- `internal/httpapi/handler.go`
- `internal/httpapi/handler_test.go`
- `internal/observability/metrics.go`
- `internal/observability/metrics_test.go`
- metrics documentation

Finding:

Unknown paths beginning `/_internal/dashboard` are copied verbatim into the Prometheus `path` label, allowing unauthenticated callers to create unbounded time-series cardinality. The `/metrics` route is also handled before API authentication and exports tenant/client usage labels; whether this endpoint is public is not documented as an explicit security contract.

References:

- `internal/httpapi/handler.go:185-203`
- `internal/httpapi/handler.go:333-344`
- `internal/observability/metrics.go:83-86`
- `internal/observability/metrics.go:284-330`

Implementation requirements:

1. Map all HTTP metrics to a closed route-name set; unknown dashboard suffixes must map to one constant label.
2. Add a cardinality regression test that sends many unique unknown paths and gathers a constant number of series.
3. Resolve decision DEC-01 before changing `/metrics` authentication or removing identity labels.
4. Whichever exposure policy is selected, test it under both `auth.none` and `bearer_static` and document listener/network assumptions.

Acceptance criteria:

- Request-controlled path values cannot create new Prometheus label values.
- Known route and operation labels remain useful and stable.
- The selected metrics access policy has positive and negative HTTP tests.
- `go test ./internal/httpapi ./internal/observability` passes.

### Task STREAM-01: Add A Shared Bounded SSE Decoder

Status: pending

Priority: P1

Suggested agent: streaming protocol engineer

Dependencies: none

Primary ownership:

- `internal/provider/anthropic.go`
- `internal/provider/gemini.go`
- new focused decoder file under `internal/provider/`
- `internal/provider/provider_test.go`

Finding:

Anthropic and Gemini stream parsing uses `bufio.Reader.ReadString('\n')` and appends event data without line or event limits. A malformed or hostile upstream can force memory growth beyond the 32 MiB limit used only by non-streaming responses. The implementations duplicate SSE framing logic.

References:

- `internal/provider/anthropic.go:319-371`
- `internal/provider/gemini.go:499-586`
- `internal/provider/provider.go:104-110`
- `internal/provider/provider.go:147-156`

Implementation requirements:

1. Introduce one internal SSE decoder used by both translated providers.
2. Enforce explicit maximum line and assembled-event sizes and return typed overflow errors.
3. Define behavior for CRLF, comments, multiple `data:` lines, EOF without a blank separator, and malformed fields.
4. Ensure closing the downstream stream cancels or closes upstream processing without leaking goroutines.
5. Preserve valid translated SSE output.

Acceptance criteria:

- Oversized lines and events fail within a bounded allocation and close upstream resources.
- Valid fragmented events and EOF boundaries remain correctly translated for Anthropic and Gemini.
- Cancellation/leak tests pass under `go test -race ./internal/provider`.

## Wave 2: Routing And Stream Correctness

### Task ROUTE-01: Make Alias Selection Lease-Based And Health-Consistent

Status: pending

Priority: P1

Suggested agent: concurrent routing engineer

Dependencies: STREAM-01 may run in parallel; serialize with STREAM-02 at `internal/httpapi/dispatch.go`

Primary ownership:

- `internal/alias/alias.go`
- `internal/alias/alias_test.go`
- `internal/httpapi/dispatch.go`
- alias routing tests in `internal/httpapi/handler_test.go`

Finding:

Retry dispatch can manually substitute an untried target after selector acquisition, then release a target that was never acquired. Least-connections counts therefore omit active fallback requests. If all targets are unhealthy, dispatch intentionally calls the first unhealthy target despite the design's no-healthy-target error. Configured retry statuses are also conflated with provider health, so even successful or client statuses can poison provider health.

References:

- `internal/alias/alias.go:59-94`
- `internal/httpapi/dispatch.go:74-135`
- `internal/httpapi/dispatch.go:173-219`
- `internal/httpapi/dispatch.go:298-319`
- `docs/design.md:511-514`

Implementation requirements:

1. Change selector acquisition to accept exclusions and return a lease/release closure tied to the acquired target.
2. Remove manual target substitution and unhealthy fallback dispatch.
3. Release each lease exactly once, including streaming close, retry, validation error, and cancellation paths.
4. Separate alias retry policy from health mutation. Mark health failure only for transport errors attributable to upstream and defined transient server failures.
5. Do not mark providers unhealthy for inbound `context.Canceled` or inbound deadline expiration.
6. Reject retry status configuration that can retry informational or successful responses; define and test the accepted status set.

Acceptance criteria:

- Concurrent least-connections retries account for every active target and release exactly once.
- An alias with all targets unhealthy returns a controlled no-healthy-target result without an upstream call.
- Inbound cancellation, 4xx retry policy, and 429 do not globally mark a provider unhealthy; transport and selected 5xx failures do.
- Existing direct-request no-failover behavior remains unchanged.
- `go test -race ./internal/alias ./internal/httpapi` passes.

### Task STREAM-02: Finalize Streaming Health, Metrics, And Usage

Status: pending

Priority: P1

Suggested agent: provider/HTTP lifecycle engineer

Dependencies: STREAM-01; ROUTE-01 because both modify dispatch and close ownership

Primary ownership:

- `internal/provider/provider.go`
- `internal/provider/openai.go`
- `internal/provider/anthropic.go`
- `internal/provider/gemini.go`
- `internal/httpapi/response.go`
- `internal/httpapi/handler.go`
- `internal/httpapi/dispatch.go`
- focused provider, HTTP, and accounting tests

Finding:

Provider health and upstream metrics are finalized when stream headers arrive, before stream consumption. Mid-stream read/write/translation failures are discarded, so broken providers remain healthy and requests retain status 200. Streaming usage remains private to some translators or is never parsed for OpenAI, while accounting reads the initial zero-valued `Result.Usage` after write completion.

References:

- `internal/httpapi/dispatch.go:35-70`
- `internal/httpapi/dispatch.go:138-174`
- `internal/httpapi/response.go:41-48`
- `internal/httpapi/response.go:146-168`
- `internal/httpapi/handler.go:128-145`
- `internal/httpapi/handler.go:292-301`
- `internal/provider/openai.go:48-56`
- `internal/provider/anthropic.go:513-563`
- `internal/provider/gemini.go:662-688`

Implementation requirements:

1. Define an explicit, concurrency-safe stream completion result carrying clean completion, read/translation error, downstream cancellation, and final usage.
2. Finalize health, stream metrics, logs, and accounting at stream completion rather than header receipt.
3. Distinguish upstream failure from downstream/client cancellation so client disconnects do not poison health.
4. Extract usage from translated streams and OpenAI-compatible SSE when the upstream supplies it; do not invent usage when absent.
5. Preserve close callbacks and alias lease release exactly once.

Acceptance criteria:

- Clean OpenAI, Anthropic, and Gemini streams record final available token usage.
- Injected mid-stream upstream errors mark the correct target unhealthy and are observable in metrics/log outcome.
- Client disconnect releases resources without marking provider failure.
- Non-streaming behavior and usage remain unchanged.
- `go test -race ./internal/provider ./internal/httpapi ./internal/accounting` passes.

### Task PROVIDER-01: Preserve Pass-Through JSON And Reject Silent Feature Loss

Status: pending

Priority: P1

Suggested agent: provider protocol compatibility engineer

Dependencies: STREAM-01; avoid overlapping STREAM-02 provider edits

Primary ownership:

- `internal/provider/openai.go`
- `internal/provider/openai_types.go`
- `internal/provider/anthropic.go`
- `internal/provider/gemini.go`
- `internal/provider/responses.go`
- `internal/provider/provider_test.go`
- provider support documentation

Finding:

OpenAI pass-through rewrites the full payload through `map[string]any`, converting JSON numbers to `float64` and potentially changing integers above 2^53. Translated adapters decode narrow request structs while silently ignoring unknown fields, so unsupported controls such as tools or response formatting can disappear without an error.

References:

- `internal/provider/openai.go:186-193`
- `internal/provider/openai_types.go:5-12`
- `internal/provider/anthropic.go:153-191`
- `internal/provider/gemini.go:246-291`
- `internal/provider/responses.go:10-30`
- `README.md:44-49`

Implementation requirements:

1. Rewrite only the top-level OpenAI `model` value while preserving all other values as raw JSON, including large integers and unknown extension fields.
2. Define duplicate `model` key behavior and reject malformed/non-object payloads consistently.
3. For translated endpoints, explicitly reject unsupported top-level fields rather than silently dropping them, unless that field is implemented in the same task.
4. Document the exact conservative supported subset for translated providers.

Acceptance criteria:

- Pass-through tests preserve integers above 2^53, nested numbers, and unknown fields byte-semantically except for the model field and insignificant object formatting.
- Unsupported translated request controls return a stable `invalid_request` response before any upstream call.
- Existing supported translated requests and streams continue to pass.
- `go test ./internal/provider ./internal/httpapi ./internal/e2e` passes.

## Wave 3: Persistence, Configuration, And Runtime Ownership

### Task STORE-01: Centralize Secure Atomic File Persistence

Status: pending

Priority: P1

Suggested agent: secure filesystem engineer

Dependencies: LIFE-01, so daemon state uses the same primitive rather than competing helpers

Primary ownership:

- new internal file-store package
- `internal/dashrpc/dashrpc.go`
- persistence helpers in `cmd/aiproxy/configure.go`
- related tests

Finding:

Configuration, keys, dashboard token, and daemon-related files are written directly with `os.WriteFile`. Existing permissive modes are not tightened, writes are truncation-prone, and pre-existing symlinks are followed. Configure creates parent directories as `0755`, including secret-bearing paths. Provider configuration updates secrets before config, leaving partial state if the second write fails.

References:

- `internal/dashrpc/dashrpc.go:184-194`
- `cmd/aiproxy/configure.go:605-608`
- `cmd/aiproxy/configure.go:1196-1210`
- `cmd/aiproxy/configure.go:3421-3435`
- `cmd/aiproxy/daemon.go:66-69`

Implementation requirements:

1. Introduce one small secure atomic-write utility using a same-directory temporary regular file, exact mode enforcement, write/sync/close, atomic rename, and directory sync where supported.
2. Reject unsafe symlink/non-regular destinations for secret-bearing files and use `0700` secret directories.
3. Make dashboard-token persistence atomic and enforce `0600` even when replacing an existing permissive file.
4. Stage and validate config and secrets before mutation; implement recoverable two-file update semantics so failure does not leave a mixed version.
5. Keep interactive prompting outside the file-store package.

Acceptance criteria:

- Existing `0644` secret/token files become `0600`; symlink targets are not overwritten.
- Injected failures at write, sync, and rename boundaries leave either the complete old state or complete new state.
- Concurrent writers do not expose partial file contents.
- Configure and dashboard token tests pass under `go test -race ./cmd/aiproxy ./internal/dashrpc`.

### Task CONFIG-01: Validate Declared Configuration Before Activation

Status: pending

Priority: P1

Suggested agent: configuration contract engineer

Dependencies: none

Primary ownership:

- `internal/config/build.go`
- `internal/config/validate.go`
- `internal/config/helpers.go`
- `internal/modelresolver/resolve.go`
- config/resolver tests and configuration docs

Finding:

Providers with unresolved empty credentials are moved to `DisabledProviders` before validation, allowing invalid names, types, URLs, models, and capabilities to pass startup. A provider named `alias` validates but its direct namespace is unreachable because resolution reserves `alias/`. Negative in-memory health cooldown also validates because cooldown checks return early when Redis is absent.

References:

- `internal/config/build.go:50-67`
- `internal/config/validate.go:39-49`
- `internal/config/validate.go:108-149`
- `internal/config/helpers.go:114-133`
- `internal/modelresolver/resolve.go:74-81`

Implementation requirements:

1. Validate every declared provider's structure independently of credential activation.
2. Reserve provider name `alias` unless the model grammar is deliberately redesigned.
3. Validate cooldown regardless of Redis configuration.
4. Parse and validate provider base URLs. Require an explicit policy for non-HTTPS remote URLs rather than silently forwarding bearer credentials over cleartext.
5. Resolve DEC-02 before changing missing-secret startup behavior; align tests and docs with the selected contract.
6. Resolve the existing slash-containing model-name documentation contradiction and align validation, examples, and design text.

Acceptance criteria:

- Invalid disabled providers fail validation with the same structural errors as active providers.
- `provider "..." "alias"` is rejected with a namespace-specific error.
- Negative cooldown and malformed base URLs are rejected.
- Missing-secret behavior is explicit, tested, and documented.
- `go test ./internal/config ./internal/modelresolver` passes.

### Task APP-01: Separate Pure Validation From Runtime Side Effects

Status: pending

Priority: P1

Suggested agent: application assembly architect

Dependencies: STORE-01, CONFIG-01

Primary ownership:

- `internal/app/app.go`
- `cmd/aiproxy/main.go`
- `internal/observability/logger.go`
- app/CLI tests

Finding:

The `validate` command calls full `app.Build`, which can mint/persist a dashboard token, initialize runtime services, emit startup logs, and replace the process-global default logger. Runtime assembly is therefore not a pure validation boundary and multiple app instances interfere through `slog.SetDefault`.

References:

- `cmd/aiproxy/main.go:82`
- `internal/app/app.go:56-85`
- `internal/app/app.go:97`
- `internal/dashrpc/dashrpc.go:184-194`

Implementation requirements:

1. Make CLI validation perform load plus structural/semantic validation only, with no persistent or global side effects.
2. Move token persistence and runtime resource creation to the serve lifecycle.
3. Inject loggers into owned components and avoid changing `slog.Default` during app construction.
4. Preserve startup logging for actual serve operations.

Acceptance criteria:

- `aiproxy validate` succeeds against a read-only config directory when its inputs are readable.
- Validation creates no dashboard token or other file and does not change `slog.Default`.
- Two app instances can be built in one process with isolated loggers.
- `go test ./cmd/aiproxy ./internal/app ./internal/observability` passes.

### Task APP-02: Give Reloaded Resources Explicit Ownership

Status: pending

Priority: P1

Suggested agent: concurrent runtime lifecycle engineer

Dependencies: APP-01

Primary ownership:

- `internal/app/app.go`
- `internal/providerhealth/providerhealth.go`
- `internal/providerhealth/redis.go`
- `internal/ratelimit/ratelimit.go`
- `internal/observability/metrics.go`
- focused reload/shutdown tests

Finding:

Changing provider-health configuration replaces the tracker without closing its Redis client; shutdown does not close health or idle HTTP transport resources. Reload recreates rate limiters and resets all client buckets. Logging-level changes are ignored, and enabling dashboard after startup cannot install a log buffer. Metrics retain stale provider/alias label series across configuration changes.

References:

- `internal/app/app.go:137-162`
- `internal/app/app.go:165-191`
- `internal/app/app.go:205-218`
- `internal/providerhealth/redis.go:12-25`
- `internal/observability/metrics.go:196-251`
- `internal/ratelimit/ratelimit.go:16-25`

Implementation requirements:

1. Add explicit close ownership for health backends and close replaced resources only after published dependencies no longer reference them.
2. Close the active health backend and idle HTTP transport connections on shutdown.
3. Preserve limiter state when rate-limit configuration is unchanged; explicitly define reset behavior when it changes.
4. Support dynamic log-level updates and dashboard log-buffer enablement, or reject those reload changes with a clear restart-required error.
5. Reconcile metric inventory by removing retired provider/alias/static-label series and preserving actual retained health state.
6. Keep reload publication atomic from request handlers' perspective.

Acceptance criteria:

- Repeated Redis config reloads and shutdown do not leak clients/goroutines under race tests.
- Unchanged limiter configuration preserves existing bucket exhaustion across reload.
- Logging/dashboard reload behavior is observable and documented.
- Removed providers and aliases no longer retain stale inventory series.
- `go test -race ./internal/app ./internal/providerhealth ./internal/ratelimit ./internal/observability` passes.

## Wave 4: Performance And Encapsulation

### Task HEALTH-01: Bound Remote Health Snapshot Latency

Status: pending

Priority: P2

Suggested agent: Redis and concurrency performance engineer

Dependencies: APP-02

Primary ownership:

- `internal/providerhealth/providerhealth.go`
- `internal/providerhealth/redis.go`
- `internal/providerhealth/providerhealth_test.go`
- dashboard/readiness timing tests

Finding:

Health snapshots and readiness checks issue sequential Redis calls. Snapshot holds `Tracker.mu` during remote I/O, so latency can approach provider count times the two-second Redis timeout and block provider-set updates. Redis URL parse errors are silently reinterpreted as raw addresses and backend errors fail open without explicit policy or instrumentation.

References:

- `internal/providerhealth/providerhealth.go:67-81`
- `internal/providerhealth/providerhealth.go:116-138`
- `internal/providerhealth/redis.go:10-25`
- `internal/providerhealth/redis.go:44-61`
- `internal/dashrpc/dashrpc.go:85-90`

Implementation requirements:

1. Copy provider identity under lock and perform no remote I/O while holding tracker mutexes.
2. Pipeline or bounded-parallelize reads under one overall operation deadline.
3. Reject malformed Redis URLs and expose backend failures in metrics/logs.
4. Resolve DEC-03 and implement the selected fail-open/fail-closed readiness and routing policy consistently.

Acceptance criteria:

- Snapshot latency is bounded by one documented deadline rather than provider count times backend timeout.
- Concurrent `SetProviders` is not blocked by slow backend reads.
- Cancellation stops outstanding work promptly.
- Malformed Redis configuration fails validation; unavailable-backend behavior matches documented policy.
- `go test -race ./internal/providerhealth ./internal/dashrpc ./internal/httpapi` passes.

### Task ACCOUNT-01: Make Usage Retention Semantics Accurate And Efficient

Status: pending

Priority: P2

Suggested agent: accounting/data-structure engineer

Dependencies: STREAM-02

Primary ownership:

- `internal/accounting/accounting.go`
- `internal/accounting/accounting_test.go`
- billing documentation

Finding:

Every recorded request scans the full summary map under a mutex, making record cost O(cardinality). Retention is inactivity-based because cumulative totals retain old events as long as a key is used frequently, which does not represent a rolling retention window.

References:

- `internal/accounting/accounting.go:143-205`
- `internal/accounting/accounting.go:262-271`

Implementation requirements:

1. Decide and document whether usage is rolling-window or lifetime-until-idle; use DEC-04.
2. For rolling retention, use bounded time buckets and expire old buckets without a full-map scan on each request.
3. Keep tenant/client filtering and current summary dimensions stable.
4. Add high-cardinality concurrent benchmarks or deterministic complexity tests.

Acceptance criteria:

- Continuously active keys do not include events older than the documented window if rolling retention is selected.
- Record does not perform a complete cardinality scan per request.
- Race tests and benchmarks show bounded lock contention at representative cardinality.
- `go test -race ./internal/accounting ./internal/httpapi` passes.

### Task HTTP-01: Correct Bounded HTTP Edge Cases And Opaque Responses

Status: pending

Priority: P2

Suggested agent: HTTP protocol engineer

Dependencies: STREAM-02

Primary ownership:

- `internal/httpapi/handler.go`
- `internal/httpapi/response.go`
- `internal/provider/provider.go`
- `internal/provider/openai.go`
- focused HTTP/provider tests

Finding:

Requests above the 8 MiB body limit are reported as malformed `400` rather than `413`. Health endpoints accept all methods. Successful opaque image/audio bodies are fully buffered and capped at 32 MiB, causing valid large responses to fail and multiplying memory use under concurrency.

References:

- `internal/httpapi/handler.go:228-232`
- `internal/httpapi/handler.go:347-377`
- `internal/provider/provider.go:104-110`
- `internal/provider/provider.go:182-213`
- `internal/provider/openai.go:46-61`

Implementation requirements:

1. Map `*http.MaxBytesError` to `413` with a stable error type.
2. Restrict health endpoints to GET/HEAD and return 405 with `Allow` otherwise.
3. Stream successful opaque binary responses; retain bounded buffering where translation or error normalization requires it.
4. Preserve upstream close and downstream cancellation behavior.

Acceptance criteria:

- Oversized requests return 413; malformed in-limit JSON remains 400.
- Unsupported health methods return 405.
- Binary responses larger than 32 MiB stream without full buffering or proxy rejection.
- Cancellation closes upstream and does not leak goroutines.
- `go test -race ./internal/httpapi ./internal/provider` passes.

### Task ARCH-01: Extract Configuration Editing And Dashboard Snapshot Boundaries

Status: pending

Priority: P2

Suggested agent: Go package-boundary refactoring engineer

Dependencies: STORE-01, APP-02

Primary ownership:

- `cmd/aiproxy/configure.go`
- new `internal/configedit/` package
- `internal/httpapi/handler.go`
- `internal/httpapi/dashboard.go`
- `internal/dashrpc/`
- package-level tests

Finding:

`cmd/aiproxy/configure.go` is approximately 3,800 lines and combines Cobra wiring, prompts, HCL mutation/rendering, validation, secrets, and filesystem persistence. `httpapi.Dependencies` carries many dashboard assembly fields, coupling proxy transport to app state representation. These boundaries increase merge conflicts and make pure mutation/snapshot behavior harder to test.

References:

- `cmd/aiproxy/configure.go:1`
- `cmd/aiproxy/configure.go:1190-1210`
- `cmd/aiproxy/configure.go:3421-3449`
- `internal/httpapi/handler.go:26-53`
- `internal/app/app.go:165-191`

Implementation requirements:

1. First add characterization tests for HCL mutation/render round trips and current CLI output.
2. Extract non-interactive document parsing, mutation, rendering, validation, and persistence orchestration into `internal/configedit`.
3. Leave Cobra and interactive prompt adaptation in `cmd/aiproxy`; do not redesign prompts in this task.
4. Replace dashboard-specific dependency fields with one immutable snapshot source/interface owned by the dashboard boundary.
5. Keep proxy routing dependencies explicit and avoid a general-purpose service locator.

Acceptance criteria:

- Configuration edits can be tested without Cobra, terminal prompts, or real filesystem mutation.
- Existing configure command behavior and formatting remain covered and unchanged.
- HTTP handler construction needs one dashboard dependency rather than app-internal slices and metadata fields.
- No import cycle or mutable shared snapshot is introduced.
- `make vet test` passes.

## Deferred Decisions Requiring Maintainer Input

### DEC-01: Metrics Security Boundary

Status: blocked

Owner: maintainer/security

Decision needed:

Choose one contract for `/metrics`: API authentication, a dedicated metrics token/listener, or intentionally public metrics with tenant/client labels removed. Current pre-auth exposure includes identity and usage dimensions at `internal/httpapi/handler.go:195-197` and `internal/observability/metrics.go:314-330`.

### DEC-02: Missing Provider Secret Contract

Status: blocked

Owner: maintainer/product

Decision needed:

Choose whether an unresolved credential must fail startup or intentionally disable a provider. If disabling remains supported, add an explicit configuration mechanism such as `enabled = false`; do not use a missing required secret as the disable signal. Current behavior is in `internal/config/build.go:56-67` and conflicts with the documented exactly-one-credential contract.

### DEC-03: Shared Health Backend Failure Policy

Status: blocked

Owner: maintainer/operations

Decision needed:

Define whether Redis health read failures are fail-open, fail-closed, or use bounded cached state for routing and readiness. The current implementation silently treats errors as healthy at `internal/providerhealth/providerhealth.go:116-127`.

### DEC-04: Accounting Retention Meaning

Status: blocked

Owner: maintainer/product

Decision needed:

Define whether the configured retention means a rolling usage window or idle-key eviction. Current cumulative summaries implement idle-key eviction while presenting a nominal retention duration.

### DEC-05: Dashboard Transport Security

Status: blocked

Owner: maintainer/security

Decision needed:

Choose whether dashboard RPC must use a loopback/Unix socket, HTTPS for non-loopback listeners, or an explicitly acknowledged insecure override. The CLI currently derives plain HTTP from listener addresses and sends the bearer token at `cmd/aiproxy/dashboard.go:103-128`.

After the decision, create a focused implementation task requiring non-loopback cleartext rejection or a separate protected transport, plus tests and operator documentation. Also apply rate limiting to dashboard authentication and define a minimum explicit-token strength if remote access remains supported.

## Parallelization Guidance

| Agent | Tasks                               | Sequencing                                                        |
| ----- | ----------------------------------- | ----------------------------------------------------------------- |
| A     | LIFE-01, then STORE-01              | Own daemon and shared persistence integration.                    |
| B     | OBS-01                              | Independent except final handler merge.                           |
| C     | STREAM-01, then PROVIDER-01         | Coordinate provider files with STREAM-02.                         |
| D     | ROUTE-01, then STREAM-02            | Exclusive ownership of `dispatch.go` during both tasks.           |
| E     | CONFIG-01, then APP-01, then APP-02 | Runtime/config dependency chain.                                  |
| F     | HEALTH-01                           | Starts after APP-02 lifecycle API settles.                        |
| G     | ACCOUNT-01 and HTTP-01              | Can run in parallel after STREAM-02 if separate agents own files. |
| H     | ARCH-01                             | Last refactor before integration to avoid moving active code.     |

Shared hotspots:

- Sequence ROUTE-01 before STREAM-02 for `internal/httpapi/dispatch.go`.
- Sequence STREAM-01 and PROVIDER-01 before STREAM-02, or assign all provider stream work to one agent.
- Sequence STORE-01 before ARCH-01 because configure persistence moves packages.
- Sequence APP-01 before APP-02 and HEALTH-01 because resource ownership APIs build on pure assembly.
- Do not run repository-wide formatting or shared-output builds concurrently.

## Final Integration Task

### Task REVIEW-01: Independently Verify Remediation

Status: pending

Priority: P1

Suggested agent: independent senior security/reliability reviewer

Dependencies: all implemented tasks and resolved decisions

Primary ownership:

- review only; focused corrective edits require maintainer approval or a new task

Finding:

The remediation spans process lifecycle, security boundaries, concurrent stream completion, resource ownership, and package extraction. Independent integration review is required because package-local passing tests cannot establish that cross-path contracts and all task acceptance criteria remain coherent.

References:

- this task document and completion evidence for every implemented task
- `AGENTS.md`
- `Makefile:71-87`

Implementation requirements:

1. Verify every completed acceptance criterion against tests and runtime behavior, not completion notes alone.
2. Re-test daemon identity, concurrent startup, symlink/file-mode handling, metric cardinality, SSE size bounds, stream cancellation, and alias lease release.
3. Verify alternate entry paths enforce the same auth, routing, and health contracts.
4. Confirm public documentation, config validation, and implementation agree.
5. Inspect all response and snapshot serializers for accidental secret/internal-data exposure.
6. Confirm request-controlled recursive, collection, line, event, body, header, and metric-label inputs have explicit bounds.
7. Review deferred decisions for rationale and residual risk; no blocked security policy may be silently marked completed.

Acceptance criteria:

- `make vet test` passes.
- `make test-race` passes.
- `make build` passes.
- All completed task criteria have traceable tests or documented runtime evidence.
- No unrelated worktree changes were reverted or folded into remediation.
- Remaining gaps are recorded as new uniquely numbered tasks with ownership, priority, references, and acceptance criteria.

## Definition Of Done

- All P0 and P1 tasks are completed, or explicitly deferred by a maintainer with residual risk recorded.
- Maintainer decisions are resolved and reflected in tests and operator documentation.
- P2 tasks selected for the remediation release are completed; remaining P2/P3 work is deliberately deferred with rationale.
- Targeted, full, race, and build verification pass.
- The independent final review is completed with evidence.
- Task statuses and completion evidence in this file reflect the actual repository state.
