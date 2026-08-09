# Configurable Upstream Header Timeout

Created: 2026-08-09 13:26:40 PDT

## Objective

Make the upstream response-header timeout configurable globally and per provider, and increase its default from 60 seconds to 90 seconds.

The HCL contract is:

```hcl
upstream_header_timeout = "120s"

provider "openai" "openai" {
  upstream_header_timeout = "180s"
}
```

Effective timeout precedence must be:

1. Provider `upstream_header_timeout` when set.
2. Root `upstream_header_timeout` when set.
3. The 90-second default.

## Scope

- HCL schema, runtime types, parsing, and validation.
- Upstream HTTP transport selection for direct and alias requests.
- Live config reload behavior.
- Interactive and non-interactive configuration editing.
- User-facing configuration documentation and examples.
- Unit and regression tests.

## Working Rules

- Read `AGENTS.md` before editing.
- Preserve unrelated worktree changes; the worktree was already dirty when this task was created.
- Do not set `http.Client.Timeout`; streaming response bodies must remain unrestricted after response headers arrive.
- Do not mutate a shared `http.Transport` timeout per request because concurrent requests may target providers with different values.
- Keep proxy environment behavior and idle-connection cleanup intact.
- Follow the repository convention of avoiding source comments unless surrounding code requires one.

## Non-Goals

- Adding a whole-request, response-body, connect, or TLS-handshake timeout.
- Changing listener timeout semantics.
- Adding model-level or alias-level timeout overrides.
- Changing retry or provider-health policy except where an existing response-header timeout already produces a transport error.

## Baseline

- `internal/app/app.go:29-33` defines `upstreamHeaderTimeout` as 60 seconds.
- `internal/app/app.go:282-289` creates one shared client and assigns that value to `http.Transport.ResponseHeaderTimeout`.
- `internal/app/app.go:93-102` and `internal/app/app.go:243-261` pass that shared client to all provider requests.
- `internal/httpapi/dispatch.go:20-45` and `internal/httpapi/dispatch.go:82-146` use the same client for direct and alias targets.
- `internal/app/app_test.go:643-655` asserts the current 60-second value.
- Root and provider schemas currently have no upstream header timeout field (`internal/config/schema.go:3-11`, `internal/config/schema.go:60-68`).
- Baseline tests were not run while creating this task.

## Task TIMEOUT-01: Add Root And Provider Upstream Header Timeouts

Status: completed

Priority: P1

Suggested agent: Go configuration and HTTP transport engineer

Dependencies: none

Primary ownership:

- `internal/config/schema.go`
- `internal/config/types.go`
- `internal/config/build.go`
- `internal/config/validate.go`
- `internal/config/load_test.go`
- `internal/app/app.go`
- `internal/app/app_test.go`
- `internal/httpapi/dispatch.go`
- focused `internal/httpapi` tests if transport selection is owned there
- `cmd/aiproxy/configure.go`
- `internal/configedit/`
- `README.md`
- `docs/design.md`
- `website/docs/configuration.md`
- relevant examples and operations documentation

Finding:

Every upstream currently uses a hard-coded 60-second `ResponseHeaderTimeout`. Operators cannot increase it globally for slow inference providers or override it for only the providers that need more time.

Implementation requirements:

1. Add optional duration-string field `upstream_header_timeout` to the root HCL object and each `provider` block.
2. Parse values with `time.ParseDuration` and reject malformed, zero, and negative explicit values with errors that identify the root or provider field.
3. Store an effective timeout for each enabled and disabled runtime provider using precedence `provider > root > 90s`. Avoid requiring downstream callers to repeat fallback logic.
4. Replace the hard-coded 60-second default with 90 seconds.
5. Ensure each direct request and each alias attempt uses the selected provider's effective timeout. Concurrent requests to providers with different timeout values must not race or affect one another.
6. Keep `http.Client.Timeout` at zero so JSON and streaming response bodies can continue for any duration after headers are received.
7. Preserve `http.ProxyFromEnvironment`, normal transport connection pooling, and `App.Close` idle-connection cleanup. Reuse clients/transports by effective configuration rather than creating a new transport for every request.
8. Apply root and provider timeout changes after a successful `SIGHUP` reload without requiring a listener restart. Failed reload validation must leave the active configuration and clients unchanged.
9. Support reading and writing both timeout locations through applicable interactive and non-interactive `aiproxy configure` flows.
10. Document the field locations, duration syntax, precedence, 90-second default, header-only scope, and streaming behavior.

Acceptance criteria:

- With neither field configured, every provider has an effective 90-second response-header timeout.
- A root value applies to every provider that omits its own value.
- A provider value overrides the root value only for that provider.
- Direct routing uses the resolved provider's timeout.
- Every alias attempt uses that attempt's provider timeout, including failover between providers with different values.
- Concurrent requests using distinct provider timeout values pass under `go test -race` without shared transport mutation.
- An upstream that delays headers beyond the effective timeout returns through the existing transport-error path, while a response that sends headers before the timeout can stream a body for longer than that timeout.
- Invalid values such as `"later"`, `"0s"`, and `"-1s"` fail configuration loading with field-specific errors at both root and provider levels.
- A successful reload changes effective root and provider timeout behavior for subsequent requests; an invalid reload preserves prior behavior.
- Existing proxy environment behavior remains covered by tests.
- Configuration docs and examples agree with runtime behavior.
- `go test ./internal/config ./internal/app ./internal/httpapi ./cmd/aiproxy` passes.
- `make vet test`, `make test-race`, and `make build` pass before completion.

## Definition Of Done

- `TIMEOUT-01` is marked `completed` with changed-file and verification evidence.
- The public configuration contract, generated or edited configuration, runtime behavior, tests, and documentation all use `upstream_header_timeout` consistently.
- No hard-coded 60-second upstream response-header timeout remains.
- The default is verified as 90 seconds, provider precedence is verified on direct and alias paths, and live reload behavior is verified.

## Completion Evidence

Completed: 2026-08-09

Changed files:

- `internal/config/schema.go`
- `internal/config/types.go`
- `internal/config/helpers.go`
- `internal/config/build.go`
- `internal/config/load_test.go`
- `internal/app/app.go`
- `internal/app/app_test.go`
- `internal/httpapi/handler.go`
- `internal/httpapi/dispatch.go`
- `internal/httpapi/handler_test.go`
- `internal/configedit/configedit.go`
- `cmd/aiproxy/configure.go`
- `cmd/aiproxy/configure_test.go`
- `README.md`
- `docs/design.md`
- `website/docs/configuration.md`
- `website/docs/operations.md`

Verification:

- `go test ./internal/config ./internal/app ./internal/httpapi ./cmd/aiproxy`
- `make vet test`
- `make test-race`
- `make build`
