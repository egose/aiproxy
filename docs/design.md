# AI Proxy Design

## Overview

This service is a Go-based proxy for multiple AI providers. It exposes an
OpenAI-compatible HTTP API to clients, selects a configured provider-backed
model or alias target, translates requests when needed, forwards them to the
upstream provider, and returns an OpenAI-compatible response.

The service is delivered as:

- a single Go CLI binary
- a container image exposing the service as a public HTTP API

Foreground `aiproxy serve` is cross-platform across the advertised release
targets. Daemon lifecycle commands (`serve -d`, `status`, `stop`, and
`restart`) are Linux-only because their safety checks depend on Linux process
identity primitives.

Configuration is written in HCL with an Alloy-like two-label block style.

## Goals

- Accept OpenAI-compatible client requests.
- Proxy requests to multiple upstream AI providers.
- Support direct addressing of configured provider models.
- Support aliases that load-balance across multiple provider/model pairs.
- Keep the external API shape consistent even when upstream providers differ.
- Support streaming responses where listed in the current public API surface.
- Keep configuration static, explicit, and easy to validate.
- Package the service as a single binary and Docker image.

## Non-Goals

- Admin API for provider or alias management
- Provider-specific public APIs exposed directly to clients
- Global cross-instance balancing state
- Persistent request queueing
- External billing, invoicing, and quota systems
- Translated-provider image and audio endpoints

## Core Design Principle

The proxy terminates and rebuilds the request. It is not a blind relay.

Reason:

- upstream providers have different authentication schemes and endpoint shapes
- some providers require translation from OpenAI-compatible requests
- aliases must choose one concrete upstream model per request
- model naming exposed to clients is owned by the proxy, not by any single provider

The proxy must:

1. Parse and validate the inbound OpenAI-compatible request.
2. Authenticate the client if auth is enabled.
3. Resolve the requested model string to either a direct provider model or an alias.
4. Select one effective upstream provider/model target.
5. Translate the request into the provider-native format when required.
6. Send the upstream request using the provider credential.
7. Translate the upstream response back into an OpenAI-compatible response.

## API Surface

The current public API surface is:

<!-- docs-contract:public-matrix:start -->

| Surface                         | `openai`                           | `openai-compatible`                | `anthropic`                        | `gemini`                           |
| ------------------------------- | ---------------------------------- | ---------------------------------- | ---------------------------------- | ---------------------------------- |
| `GET /v1/models`                | Proxy-owned                        | Proxy-owned                        | Proxy-owned                        | Proxy-owned                        |
| `GET /v1/billing/usage`         | Proxy-owned local usage accounting | Proxy-owned local usage accounting | Proxy-owned local usage accounting | Proxy-owned local usage accounting |
| `GET /metrics`                  | Proxy-owned Prometheus metrics     | Proxy-owned Prometheus metrics     | Proxy-owned Prometheus metrics     | Proxy-owned Prometheus metrics     |
| `POST /v1/chat/completions`     | JSON and SSE                       | JSON and SSE                       | JSON and SSE translated            | JSON and SSE translated            |
| `POST /v1/embeddings`           | Yes                                | Yes                                | No                                 | Yes                                |
| `POST /v1/responses`            | JSON and SSE                       | JSON and SSE                       | JSON and SSE translated subset     | JSON and SSE translated subset     |
| `POST /v1/images/generations`   | Yes                                | Yes                                | No                                 | No                                 |
| `POST /v1/audio/transcriptions` | Yes                                | Yes                                | No                                 | No                                 |
| `POST /v1/audio/speech`         | Yes                                | Yes                                | No                                 | No                                 |

<!-- docs-contract:public-matrix:end -->

Chat completions and responses support both standard JSON responses and
OpenAI-compatible Server-Sent Events where the matrix lists SSE support.
`GET /v1/billing/usage` is local in-process usage accounting over the proxy's
rolling window; it is not an external billing, invoicing, or quota system.

Future provider endpoints should reuse the same provider, model, alias,
credential, and adapter concepts rather than defining a separate config model.

## External Naming Model

Clients address models using proxy-owned names.

Supported public model forms:

- `<provider-name>/<model-name>`
- `alias/<alias-name>`

Examples:

- `openai/gpt-4o-mini`
- `gemini/gemini-2.5-pro`
- `alias/chat_default`

Name rules:

- provider names must be lowercase
- alias names must be lowercase
- names must not contain spaces
- provider and alias names must not contain `/`
- provider name `alias` is reserved for `alias/<alias-name>` routing
- model names may contain `/` when every slash-separated segment follows the
  same lowercase name rule

Direct model resolution splits on the first `/`, so slash-containing model names
remain unambiguous under `<provider-name>/<model-name>`.

## Request Model

The proxy should normalize inbound requests into an internal request context.

```go
type RequestContext struct {
    Method        string
    Path          string
    Headers       http.Header
    RequestedModel string
    Stream        bool
    Operation     Operation
}
```

Supported operations reuse the same resolver and adapter pipeline for chat
completions, embeddings, responses, image generations, audio transcriptions, and
audio speech.

## Authentication

Authentication is intentionally separate from upstream provider credentials.

Initial auth modes:

- `none`
- `bearer_static`

### `none`

No inbound authentication is performed. This mode is intended only for trusted
deployments.

### `bearer_static`

The proxy validates the inbound `Authorization: Bearer ...` token against
statically configured client credentials from HCL.

Each static client may also define:

- optional `tenant`
- optional `allowed_models`

`allowed_models` applies a static allow-list against the proxy-visible model
name, including both direct `<provider>/<model>` strings and `alias/<name>`.

The proxy also records in-process accounting events keyed by:

- tenant
- client
- model
- operation
- status

These events are also aggregated in-process by the same key dimensions over a
rolling 24-hour window to form the first billing/accounting scaffold. The
window is maintained with bounded one-minute buckets whose start times are in
the rolling window, rather than lifetime totals or idle-key eviction.

`GET /v1/billing/usage` exposes those aggregated summaries. In static bearer
auth mode, responses are scoped to the caller's tenant when present, otherwise
to the caller's client identity.

### Optional Local Rate Limit

The `auth` block may include a `rate_limit` sub-block:

- `requests_per_minute`
- optional `burst`, defaulting to `requests_per_minute`

The current implementation is local to a single proxy instance.

- In `bearer_static` mode, the limiter is keyed by authenticated client name.
- In `none` mode, the limiter applies to a shared anonymous bucket.

Exceeded requests return `429 Too Many Requests` with `Retry-After`.

Deferred auth features:

- token rotation
- external auth integration

## Resolution Model

### Direct Provider Model Resolution

If `model` is in the form `<provider-name>/<model-name>`, the proxy resolves the
request directly to the configured provider and model.

### Alias Resolution

If `model` is in the form `alias/<alias-name>`, the proxy resolves the alias and
selects one target from the alias pool.

Each alias target is a concrete pair:

- provider name
- model name

Alias targets must all be valid configured provider/model pairs.

## Provider Model

Providers are configured with two HCL labels:

- first label: provider type
- second label: provider name

Format:

```hcl
provider "<type>" "<name>" {}
```

Initial provider types:

- `openai`
- `openai-compatible`
- `anthropic`
- `gemini`

Additional types can be added later without changing the external client API.

Provider attributes:

- `display_name`
- `base_url`
- `api_key`
- `api_key_ref`
- `extends`
- `upstream_header_timeout`
- `enabled`
- nested `model` blocks

Provider inheritance is resolved during configuration loading. A provider may
declare `extends = "<base-provider-name>"` to inherit the base provider's type,
`base_url`, effective upstream header timeout, enabled state, and complete model
inventory while using its own provider name and credential.

Derived provider blocks are intentionally restricted:

- the type label remains mandatory and must match the base provider type
- the base must exist, be enabled, and must not itself declare `extends`
- declaration order does not matter
- `display_name` may override the inherited display name
- exactly one local credential form, `api_key` or `api_key_ref`, is required
- local `base_url`, `upstream_header_timeout`, `enabled`, and `model` blocks are rejected

After loading, derived providers are ordinary providers. Direct routing,
health, metrics, billing, dashboard inventory, reloads, and aliases identify
them by their own provider names. Aliases do not expand inherited providers;
each target must still list the concrete provider name and model.

### Provider Type Semantics

#### `openai`

Well-known built-in adapter for OpenAI's API.

#### `openai-compatible`

Adapter for providers that already expose an OpenAI-compatible API surface.

This type requires:

- `base_url`

The proxy can mostly pass through OpenAI-compatible request and response bodies
for this provider type, while still applying model resolution, auth, metrics,
and error normalization.

#### `anthropic` and `gemini`

These provider types require explicit request and response translation between
the public OpenAI-compatible API and the provider-native API.

In the current implementation:

- `anthropic` supports translated chat completions and responses
- `gemini` supports translated chat completions, responses, and embeddings

## Model Model

Each provider contains one or more nested `model` blocks:

```hcl
model "<name>" {}
```

Model attributes:

- `display_name`
- `upstream_name`
- `capabilities`

Semantics:

- the model block label is the proxy-visible model name
- `display_name` is optional metadata for humans
- `upstream_name` is optional and defaults to the model block label
- `capabilities` is optional; when omitted, the proxy derives default
  capabilities from the provider type

Using `upstream_name` avoids coupling the proxy-visible model name to the exact
string sent to the upstream provider.

Capability values:

- `chat`
- `responses`
- `embeddings`
- `images`
- `audio_transcriptions`
- `audio_speech`

Default capability behavior:

<!-- docs-contract:capability-matrix:start -->

| Provider type       | Default capabilities when omitted | Additional supported capabilities                |
| ------------------- | --------------------------------- | ------------------------------------------------ |
| `openai`            | `chat`, `responses`, `embeddings` | `images`, `audio_transcriptions`, `audio_speech` |
| `openai-compatible` | `chat`, `responses`, `embeddings` | `images`, `audio_transcriptions`, `audio_speech` |
| `anthropic`         | `chat`, `responses`               | None                                             |
| `gemini`            | `chat`, `responses`               | `embeddings`                                     |

<!-- docs-contract:capability-matrix:end -->

If `capabilities` is set on a model, it replaces the default capability set for
that model. The config validator rejects capability values that the provider
type cannot actually serve.

## Alias Model

Aliases are configured with one HCL label.

Intent:

- an alias is a virtual model exposed by the proxy
- it is not just a rename
- it can represent a load-balanced or failover-backed pool of concrete provider/model targets

Aliases are configured with one HCL label:

```hcl
alias "<name>" {}
```

Alias attributes:

- `algorithm`
- nested `target` blocks

Each `target` block contains:

- `provider`
- `model`

Example:

```hcl
alias "chat_default" {
  algorithm = "round_robin"

  target {
    provider = "openai"
    model    = "gpt-4o-mini"
  }

  target {
    provider = "localai"
    model    = "qwen3-32b"
  }
}
```

Aliases can serve any operation included in the intersection of their targets'
effective capabilities. Operators should only combine targets that are safe to
use interchangeably for the operations exposed through that alias.

When the proxy renders `GET /v1/models`, alias capability metadata is the
intersection of the effective capabilities of every target in the alias pool.
This avoids advertising `responses` or `embeddings` on an alias unless every
target behind it can actually serve that operation.

The `GET /v1/models` response should also expose human- and operator-friendly
metadata:

- direct provider-backed models include:
  - `display_name`
  - `provider_type`
  - effective `capabilities`
- aliases include:
  - effective `capabilities`
  - `alias_targets` summaries containing provider name, model name, and
    resolved display name for each target

## Load Balancing And Failure Policy

### Algorithms

Initial alias algorithms:

- `round_robin`
- `least_connections`

Algorithm values are lowercase machine-friendly enums.

### `round_robin`

Requests rotate across alias targets in process-local order.

### `least_connections`

The proxy selects the target with the fewest currently active in-flight
requests.

This is:

- per-process
- best-effort
- not coordinated across multiple proxy instances

### Failure Handling

If the chosen alias target fails, the retry policy is:

- do retry another alias target on transport errors and timeouts
- do retry another alias target on upstream statuses listed in
  `retry_status_codes`; the default is `500`, `502`, `503`, and `504`
- configured retry statuses may be any status in the `400`-`599` range, so
  deployments can opt into retrying responses such as `429`
- do not retry on other upstream `4xx` request validation errors
- stop after each target in the alias pool has been tried at most once

This avoids hiding client request mistakes while still allowing basic failover
for transient upstream failures.

Retryable `4xx` statuses are an alias-routing decision only. They do not mark a
provider unhealthy; provider-health mutation remains tied to transport/upstream
request errors and upstream `5xx` responses.

Direct provider model requests do not fail over to a different provider or
model, because the client selected a specific target explicitly.

## Provider Adapter Model

The proxy uses provider adapters behind the OpenAI-compatible frontend.

Each adapter is responsible for:

- building provider-specific HTTP requests
- injecting auth headers
- mapping the proxy model selection to the upstream model identifier
- translating provider-specific success payloads
- translating provider-specific error payloads
- translating streaming event formats when needed

Adapter categories:

- near pass-through adapters for `openai` and `openai-compatible`
- translation adapters for provider-native APIs such as `anthropic` and `gemini`

## Streaming Behavior

The current API supports streaming chat completions and streaming responses.

Streaming rules:

- the public API uses OpenAI-compatible SSE framing
- upstream provider streaming formats are translated into OpenAI-compatible event streams when needed
- the proxy should flush chunks promptly and avoid buffering the full stream in memory
- if an upstream stream fails after partial output, the client receives a terminated stream rather than a synthetic full JSON response

## Credential Resolution

Provider credentials are configured per provider.

Exactly one of these must be set:

- `api_key`
- `api_key_ref`

### `api_key`

Inline string value, typically sourced from environment substitution:

```hcl
api_key = env("OPENAI_API_KEY")
```

### `api_key_ref`

Nested block with:

- `path`
- `key`

Example:

```hcl
api_key_ref {
  path = "/home/user/.config/aiproxy/keys.json"
  key  = "openai"
}
```

`path` defaults to a secure user-scoped location:

1. `$XDG_CONFIG_HOME/aiproxy/keys.json` when `XDG_CONFIG_HOME` is set
2. `~/.config/aiproxy/keys.json` otherwise

The JSON file is expected to be a flat object mapping string keys to string API
keys.

Example:

```json
{
  "openai": "sk-...",
  "anthropic": "sk-ant-...",
  "localai": "secret"
}
```

Credential lookup should happen during config load so invalid references fail
startup rather than failing on first request.

## Error Handling

The proxy should normalize errors into OpenAI-compatible error responses where
possible.

Typical cases:

- unknown model name
- unknown alias name
- alias with no healthy targets
- invalid client auth
- invalid config
- unsupported provider type
- upstream provider auth failure
- upstream validation failure
- upstream transport timeout
- translation failure

Rules:

- config errors fail startup
- unknown model or alias returns a client-visible `4xx` error
- provider auth failures are surfaced as upstream errors, not rewritten as local auth failures
- transient alias target failures may trigger retry to another alias target
- proxy-generated errors should include a request ID for debugging

## Config Model

Configuration uses Alloy-like labeled HCL blocks.

Recommended block types:

- `listener "http" "public"`
- `auth "main"`
- `provider "<type>" "<name>"`
- `alias "<name>"`

### Example Config

```hcl
listener "http" "public" {
  address = ":8080"

  timeouts {
    read_header = "10s"
    idle        = "60s"
    write       = "0s"
  }
}

upstream_header_timeout = "120s"

auth "main" {
  mode = "bearer_static"

  client "local-dev" {
    token = env("AIPROXY_CLIENT_LOCAL_DEV_TOKEN")
  }
}

provider "openai" "openai" {
  display_name = "OpenAI"
  api_key      = env("OPENAI_API_KEY")

  model "gpt-4o-mini" {
    display_name = "GPT-4o mini"
  }

  model "gpt-4.1" {
    display_name = "GPT-4.1"
  }
}

provider "anthropic" "anthropic" {
  display_name = "Anthropic"
  api_key_ref {
    key = "anthropic"
  }

  model "claude-sonnet" {
    display_name = "Claude Sonnet"
    upstream_name = "claude-sonnet-4-20250514"
  }
}

provider "openai-compatible" "localai" {
  display_name = "LocalAI"
  base_url     = "https://llm.internal/v1"
  upstream_header_timeout = "180s"

  api_key_ref {
    key = "localai"
  }

  model "qwen3-32b" {
    display_name = "Qwen 3 32B"
  }
}

alias "chat_default" {
  algorithm = "round_robin"

  target {
    provider = "openai"
    model    = "gpt-4o-mini"
  }

  target {
    provider = "anthropic"
    model    = "claude-sonnet"
  }
}

alias "chat_fallback" {
  algorithm = "least_connections"

  target {
    provider = "openai"
    model    = "gpt-4.1"
  }

  target {
    provider = "localai"
    model    = "qwen3-32b"
  }
}
```

### Config Semantics

- `display_name` is descriptive only
- `base_url` is required only for `openai-compatible`
- `base_url` must be an absolute `https` URL for remote upstreams; `http` is
  allowed only for loopback hosts such as `localhost`, `127.0.0.1`, or `::1`
- `api_key_ref.path` is optional because it has a secure default
- `upstream_header_timeout` accepts a positive duration at root or provider scope; provider values override root values, and the default is 90 seconds
- the upstream header timeout limits only the wait for response headers, not JSON or streaming response bodies after headers arrive
- aliases reference provider and model names without extra ref prefixes

## Validation Rules

The config loader should validate:

- duplicate provider names
- duplicate alias names
- invalid provider type values
- invalid alias algorithm values
- provider names that are not lowercase
- alias names that are not lowercase
- provider or alias names containing spaces or `/`
- provider name `alias`, which is reserved for `alias/<alias-name>` routing
- model names that are not lowercase, contain spaces, or contain empty `/`
  segments; slash-containing model names are valid when each segment is valid
- `openai-compatible` providers missing `base_url`
- malformed provider `base_url` values, and non-loopback `http` base URLs
- providers with both `api_key` and `api_key_ref`
- malformed, zero, or negative `upstream_header_timeout` values
- active providers with no resolved credential, including an empty
  `api_key = env("...")`; missing or empty credentials fail validation unless
  `enabled = false` is declared explicitly
- `api_key_ref` blocks missing `key`
- `api_key_ref` JSON files that do not exist or do not contain the requested key
- providers without any models
- duplicate model names within a provider
- aliases without any targets
- alias targets pointing to unknown providers
- alias targets pointing to unknown models

The service should fail startup on invalid config.

Providers default to enabled. To intentionally disable a provider, declare
`enabled = false`; disabled providers are still structurally validated (name,
type, base URL, models, capabilities) but do not require a usable credential.
The disabled state is reported explicitly in startup logs and dashboard
snapshots rather than inferred from missing secret state.

## Observability And Security

The proxy should provide logs, metrics, and traces, but default to protecting
prompt and credential data.

Defaults:

- never log API keys or client bearer tokens
- redact or omit prompt and response bodies from standard logs
- emit request IDs for correlation
- record per-provider latency and error-rate metrics
- record alias target selection counts
- record active in-flight request counts for `least_connections`

Initial `/metrics` coverage includes:

- inbound HTTP request counts by method/path/status
- inbound HTTP request latency by method/path/status
- inbound HTTP request body size histograms by method/path
- outbound HTTP response body size histograms by method/path/status
- streaming response counts by method/path/status
- streaming response duration by method/path/status
- proxy-generated HTTP error counts by method/path/status/error_type
- provider selection counts
- alias retry counts
- alias in-flight request gauges by target
- auth mode startup state
- build version info
- provider counts by type and active/disabled state
- alias counts by algorithm
- skipped-provider state
- provider health state
- readiness state
- readiness reason state
- upstream response body size histograms by operation/provider/outcome
- upstream request counts by operation/provider/outcome
- upstream request latency by operation/provider/outcome
- provider health backend error counts
- provider health fallback counts by operation and reason

`GET /metrics` is served on the same listener as the proxy API but is gated by
a dedicated metrics bearer token declared in a `metrics { token = ... }` block.
The token is checked independently of API auth client tokens; an empty or
missing token is rejected at config validation so metrics are never exposed
without a dedicated credential. Metric output can include tenant, client,
provider, model, and alias labels, so the token must be shared only with
trusted scrapers. HTTP route labels are a closed set of stable endpoint names,
with unknown dashboard-internal paths reported as
`/_internal/dashboard/unknown`.

The interactive `aiproxy dashboard` command and the
`/_internal/dashboard/{snapshot,logs}` endpoints share the metrics-token-less
listener. Listener addresses are TCP bind addresses in `host:port` form, not
URLs. The dashboard command is local-only: it derives a loopback plain-HTTP URL
from wildcard or loopback binds, refuses concrete non-loopback hosts, and uses a
dashboard bearer token for every RPC. Remote dashboard support requires a future
explicit transport design. Repeated invalid dashboard tokens are rate limited
with `429` and a `Retry-After` header so the bearer surface cannot be
brute-forced from the listener.

## CLI Design

The service is a single binary named `aiproxy`.

Recommended commands:

- `aiproxy serve --config /etc/aiproxy/config.hcl`
- `aiproxy validate --config /etc/aiproxy/config.hcl`
- `aiproxy version`

Linux also supports `aiproxy serve -d`, `aiproxy status`, `aiproxy stop`, and
`aiproxy restart` for daemon lifecycle management. On non-Linux platforms,
foreground `serve` remains supported but daemon lifecycle commands return
`daemon lifecycle is unsupported on this platform`.

Optional future commands:

- `aiproxy print-example-config`
- `aiproxy models --config ...`

## Deployment Model

The service is packaged as a Docker image that runs the CLI.

Recommended container behavior:

- expose the proxy on `:8080`
- mount config at `/etc/aiproxy/config.hcl`
- pass client tokens and provider secrets via environment variables or the key file
- mount the key file read-only when `api_key_ref` is used

Recommended image approach:

- multi-stage Docker build
- static or near-static Go binary
- minimal runtime image with CA certificates

## Recommended Package Layout

```text
cmd/aiproxy/
internal/app/
internal/config/
internal/auth/
internal/httpapi/
internal/requestctx/
internal/modelresolver/
internal/alias/
internal/provider/
internal/provider/openai/
internal/provider/openaicompat/
internal/provider/anthropic/
internal/provider/gemini/
internal/stream/
internal/observability/
```

## Historical Implementation Plan

The milestone list below is historical and no longer defines the current public
contract.

### Milestone 1

- CLI scaffold
- HCL config parsing and validation
- inbound auth modes:
  - `none`
  - `bearer_static`
- direct provider/model resolution
- alias resolution with `round_robin`
- `POST /v1/chat/completions`
- non-streaming chat responses
- streaming chat responses
- OpenAI and `openai-compatible` adapters
- logs, health, readiness, basic metrics

### Milestone 2

- `least_connections` alias selection
- transient-failure retry across alias targets
- stronger streaming robustness
- more metrics and integration tests

### Later Phase

- anthropic embeddings if a viable provider-native mapping exists
- translated-provider image and audio APIs
- external billing/invoicing and quota systems
- per-client policy
- broader provider catalog

## Testing Strategy

### Unit Tests

- config parsing and validation
- provider/model name parsing
- alias target selection
- retry policy
- credential resolution from env and key file
- adapter request translation
- adapter response translation
- streaming event translation

### Stub-Backed End-To-End Tests

Run against in-process provider stubs using `httptest` servers:

- health and readiness endpoints
- direct `openai/<model>` chat completion
- `alias/<name>` routing through configured upstream targets
- `POST /v1/embeddings`
- `POST /v1/responses`

These tests exercise the full proxy request path without depending on external
provider accounts or sandbox containers.

### Hermetic Binary Integration Tests

The normal CI suite includes hermetic binary-level integration tests that build
`dist/aiproxy`, start the binary with temporary local configuration and upstream
stubs, and exercise listener, readiness, reload, auth, metrics, streaming,
derived-provider routing, and alias retry behavior without paid provider
credentials.

Real-provider sandbox tests remain separate and optional.

### Documentation Contract Checks

`make docs-contract` checks the marked public endpoint/provider and provider
capability tables in README, this design document, the website docs, and
AGENTS.md. Markdown-only pull requests run the same check through the `Docs
Contract` workflow so high-drift contract tables cannot change in only one
location.

## Deferred Features

The following are intentionally out of scope for the current public contract:

- anthropic embeddings
- translated-provider image endpoints
- translated-provider audio endpoints
- external billing / invoicing systems and quotas

## Provider Health

The proxy maintains dynamic provider health state in-process and shares it
across requests and aliases.

Transient transport failures, upstream request errors, and upstream `5xx`
responses mark a provider temporarily unhealthy for alias routing and readiness
decisions. Configured retryable `4xx` statuses can cause an alias to try another
target, but they do not mutate provider health.

Provider health state is not coordinated across multiple proxy instances.

An optional `provider_health` block may configure Redis-backed transient health
state sharing across instances:

- `redis_url`
- optional `key_prefix`
- optional `cooldown`
- optional `cache_ttl` (default 30s), bounding how long a stale local cache
  entry is reused for routing and readiness when the Redis backend becomes
  unreadable

When `redis_url` is configured and a Redis health read fails, routing,
readiness, and dashboard snapshots fall back to the bounded in-process cache
and fail open only when no fresh cache entry exists. Both the backend error
and the fallback reason are recorded as Prometheus metrics so degraded mode is
observable.

## Reload Behavior

The server supports in-process config reload on `SIGHUP`.

Reload currently rebuilds and swaps:

- inbound auth configuration
- provider/model catalog
- alias routing state
- root and provider upstream header timeouts
- access-log enablement
- metrics configuration
- provider-health configuration
- readiness and startup inventory metrics

Reload does not replace the active listener socket.

The following config changes still require a full restart:

- listener address changes
- listener timeout changes
- logging level changes
- enabling the dashboard after startup

## Appendix: Open Questions And Rejected Alternatives

### Open Questions

- `GET /v1/models` now returns both direct provider-backed models and aliases in one list, including capability metadata, display names, provider types, and alias target summaries. Should a later revision also expose raw upstream model identifiers in that response?
- Should future capability declarations remain an optional narrowing mechanism, or eventually become required for every configured model?
- Should later auth work remain simple static bearer tokens, or grow into tenant-aware policy and quotas?
- Should provider key files be re-read on each request for easy secret rotation, or only at startup for predictability?

### Rejected Or Deferred Alternatives

#### Free-form object arrays for models and alias pools

Rejected.

Reason:

- nested HCL blocks fit the existing repo style better
- nested blocks validate more cleanly
- two-label blocks keep provider type and provider name explicit

#### Public provider-native endpoint passthrough

Rejected for the current public contract.

Reason:

- it weakens the value of a consistent OpenAI-compatible frontend
- it complicates auth, logging, and routing behavior
- it encourages provider-specific client coupling

#### Global least-connections balancing across all instances

Rejected for the current public contract.

Reason:

- it requires shared state or a control plane
- it adds operational complexity that is not necessary for the first release

The chosen design keeps `least_connections` process-local.

#### Silent fallback for direct provider/model requests

Rejected.

Reason:

- if a client asks for `openai/gpt-4.1`, it should either get that target or a clear error
- failing over to a different provider or model would be surprising and hard to debug

The chosen design allows fallback only for alias-based requests.

## Final Current Decisions

- the public API is OpenAI-compatible
- the current endpoint/provider support matrix is the one listed in API Surface
- direct model names use `<provider-name>/<model-name>`
- alias names use `alias/<alias-name>`
- provider name `alias` is reserved, and direct model resolution uses the first
  `/`, so provider model names may contain `/` when each segment is valid
- HCL uses two-label `provider "<type>" "<name>"` blocks
- `openai-compatible` requires `base_url`
- providers normally declare exactly one of `api_key` or `api_key_ref`;
  missing or empty credentials fail validation unless `enabled = false` is
  declared explicitly
- `api_key_ref.path` defaults to `$XDG_CONFIG_HOME/aiproxy/keys.json` and falls back to `~/.config/aiproxy/keys.json`
- aliases support `round_robin` and `least_connections`
- alias retry happens for transport errors, timeouts, and configured
  `retry_status_codes` in the `400`-`599` range
- direct provider/model requests do not fall back to different targets
- streaming chat completions and responses are part of the current supported API
