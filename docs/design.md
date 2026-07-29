# AI Proxy Design

## Overview

This service is a Go-based proxy for multiple AI providers. It exposes an
OpenAI-compatible HTTP API to clients, selects a configured provider-backed
model or alias target, translates requests when needed, forwards them to the
upstream provider, and returns an OpenAI-compatible response.

The service is delivered as:

- a single Go CLI binary
- a container image exposing the service as a public HTTP API

Configuration is written in HCL with an Alloy-like two-label block style.

## Goals

- Accept OpenAI-compatible client requests.
- Proxy requests to multiple upstream AI providers.
- Support direct addressing of configured provider models.
- Support aliases that load-balance across multiple provider/model pairs.
- Keep the external API shape consistent even when upstream providers differ.
- Support streaming chat responses for the MVP API surface.
- Keep configuration static, explicit, and easy to validate.
- Package the service as a single binary and Docker image.

## Non-Goals For MVP

- Admin API for provider or alias management
- Provider-specific public APIs exposed directly to clients
- Global cross-instance balancing state
- Persistent request queueing
- Rate limiting and quota accounting
- Billing and tenant management
- Embeddings, images, audio, and response-style APIs in the first release

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

### MVP

The initial public API surface is:

- `GET /v1/models`
- `GET /metrics`
- `POST /v1/chat/completions`
- `POST /v1/embeddings` for `openai`, `openai-compatible`, and `gemini`
- `POST /v1/responses` for `openai`, `openai-compatible`, `anthropic`, and `gemini`

The MVP supports both:

- standard JSON responses
- streaming responses via Server-Sent Events when `stream = true`

### Future API Surface

The design should leave room for later support of:

- translated-provider embeddings
- image generation endpoints
- audio transcription and speech endpoints

These later endpoints should reuse the same provider, model, alias, credential,
and adapter concepts rather than defining a separate config model.

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
- names must not contain `/`

This keeps parsing trivial and prevents ambiguity in public model strings.

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

Initial operation support for MVP:

- `ChatCompletions`

Future operations can reuse the same resolver and adapter pipeline.

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

Deferred auth features:

- token rotation
- tenant-scoped policy
- per-client rate limits
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
- nested `model` blocks

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

- `anthropic` supports translated chat completions
- `gemini` supports translated chat completions and embeddings

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

Default capability behavior:

- `openai` and `openai-compatible` default to `chat`, `responses`, and `embeddings`
- `anthropic` default to `chat` and `responses`
- `gemini` default to `chat` and `responses`

If `capabilities` is set on a model, it narrows the effective capability set.
The config validator rejects capability values that the provider type cannot
actually serve.

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

For MVP, aliases are chat-only and should only contain models that are safe to
use interchangeably for chat requests.

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

- do not retry on upstream `4xx` request validation errors
- do retry another alias target on transport errors, timeouts, and upstream `5xx`
- stop after each target in the alias pool has been tried at most once

This avoids hiding client request mistakes while still allowing basic failover
for transient upstream failures.

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

The MVP should support streaming chat completions.

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
- `api_key_ref.path` is optional because it has a secure default
- aliases reference provider and model names without extra ref prefixes

## Validation Rules

The config loader should validate:

- duplicate provider names
- duplicate alias names
- invalid provider type values
- invalid alias algorithm values
- provider names that are not lowercase
- alias names that are not lowercase
- names containing spaces or `/`
- `openai-compatible` providers missing `base_url`
- providers with both `api_key` and `api_key_ref`
- providers with neither `api_key` nor `api_key_ref`
- `api_key_ref` blocks missing `key`
- `api_key_ref` JSON files that do not exist or do not contain the requested key
- providers without any models
- duplicate model names within a provider
- aliases without any targets
- alias targets pointing to unknown providers
- alias targets pointing to unknown models

The service should fail startup on invalid config.

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
- readiness state
- readiness reason state
- upstream response body size histograms by operation/provider/outcome
- upstream request counts by operation/provider/outcome
- upstream request latency by operation/provider/outcome

## CLI Design

The service is a single binary named `aiproxy`.

Recommended commands:

- `aiproxy serve --config /etc/aiproxy/config.hcl`
- `aiproxy validate --config /etc/aiproxy/config.hcl`
- `aiproxy version`

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

## Implementation Plan

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
- image and audio APIs
- rate limiting and quotas
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

### Integration Tests

Integration tests are intentionally skipped for now.

Add them back once the repo has sandbox services or stable provider stubs for
repeatable end-to-end coverage.

Planned coverage once the sandbox exists:

- direct `openai/<model>` chat completion
- `alias/<name>` resolution
- round-robin balancing across alias targets
- least-connections selection behavior
- retry to another alias target on transient failure
- no retry on upstream `4xx` validation errors
- streaming chat responses
- invalid auth rejection
- provider auth failure surfacing

## Deferred Features

The following are intentionally out of scope for the MVP:

- anthropic embeddings
- image and audio endpoints
- rate limiting
- billing and tenancy
- dynamic provider health state shared across instances

## Reload Behavior

The server supports in-process config reload on `SIGHUP`.

Reload currently rebuilds and swaps:

- inbound auth configuration
- provider/model catalog
- alias routing state
- readiness and startup inventory metrics

Reload does not replace the active listener socket.

The following config changes still require a full restart:

- listener address changes
- listener timeout changes

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

Rejected for MVP.

Reason:

- it weakens the value of a consistent OpenAI-compatible frontend
- it complicates auth, logging, and routing behavior
- it encourages provider-specific client coupling

#### Global least-connections balancing across all instances

Rejected for MVP.

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

## Final MVP Decisions

- the public API is OpenAI-compatible
- the MVP endpoint is `POST /v1/chat/completions`
- direct model names use `<provider-name>/<model-name>`
- alias names use `alias/<alias-name>`
- HCL uses two-label `provider "<type>" "<name>"` blocks
- `openai-compatible` requires `base_url`
- exactly one of `api_key` or `api_key_ref` must be set per provider
- `api_key_ref.path` defaults to `$XDG_CONFIG_HOME/aiproxy/keys.json` and falls back to `~/.config/aiproxy/keys.json`
- aliases support `round_robin` and `least_connections`
- alias retry only happens for transient upstream failures
- direct provider/model requests do not fall back to different targets
- streaming chat responses are part of the MVP
