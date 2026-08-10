# aiproxy

A Go service that proxies multiple AI providers behind a single
OpenAI-compatible HTTP API. It resolves client-supplied model strings to
configured provider-backed models or aliases, forwards requests upstream
(translating when needed for non-OpenAI providers), and returns
OpenAI-compatible responses.

## Current MVP Scope

### Supported Public API

- `GET /v1/models`
- `GET /v1/billing/usage`
- `GET /metrics`
- `POST /v1/chat/completions` (JSON and SSE streaming)
- `POST /v1/embeddings` for `openai`, `openai-compatible`, and `gemini` providers
- `POST /v1/responses` for `openai`, `openai-compatible`, `anthropic`, and `gemini` providers (JSON and SSE streaming)
- `POST /v1/images/generations` for `openai` and `openai-compatible` providers
- `POST /v1/audio/transcriptions` for `openai` and `openai-compatible` providers
- `POST /v1/audio/speech` for `openai` and `openai-compatible` providers

### Auth Modes

- `none` – skip inbound authentication (trusted environments only)
- `bearer_static` – validate inbound `Authorization: Bearer ...` tokens against
  statically configured client credentials
- optional `rate_limit` on the `auth` block applies a local in-memory request
  rate limit; in `bearer_static` mode it is enforced per authenticated client,
  and in `none` mode it is enforced against a shared anonymous bucket
- optional `tenant` and `allowed_models` on `auth.client` let you attach client
  identity metadata and enforce a static allow-list of proxy-visible model
  names
- request accounting is tracked in-process by tenant, client, model, operation,
  and status over a rolling 24-hour window; `/metrics` exposes aggregated usage
  event counters
- optional `logging` config controls structured log level and request lifecycle
  access logging
- `GET /v1/billing/usage` returns aggregated in-process usage summaries. In
  `bearer_static` mode it is scoped to the caller's tenant when present,
  otherwise to the caller's client identity. Summaries use the same rolling
  24-hour in-process window as local accounting.
- optional `provider_health` config can use Redis to share transient provider
  health state across instances; without it, health remains in-process only

### Provider Types

- `openai` – built-in OpenAI adapter (pass-through)
- `openai-compatible` – any OpenAI-compatible endpoint (requires `base_url`)
- `anthropic` – chat and responses translation to Anthropic Messages API
- `gemini` – chat translation to Gemini generateContent API, embeddings translation to Gemini embedContent API, and responses translation through generateContent

Pass-through providers rewrite only the top-level `model` JSON field and preserve other request fields, including unknown extension fields. Translated providers reject unsupported top-level request controls rather than silently dropping them; their supported chat fields are `model`, `messages`, `max_tokens`, `temperature`, `top_p`, and `stream`, their supported responses fields are `model`, `input`, `instructions`, `max_output_tokens`, `temperature`, `top_p`, and `stream`, and Gemini embeddings supports `model`, `input`, and `dimensions`.

### Routing

- Direct model addressing: `<provider-name>/<model-name>`
- Alias addressing: `alias/<alias-name>`
- An alias is a virtual model backed by one or more concrete provider/model targets
- Alias algorithms: `round_robin`, `least_connections`
- Alias failover: retry next target on transport errors and upstream `5xx`
  only; `4xx` client errors are returned verbatim

### Not in MVP

- Quotas, billing, tenancy

The server supports live config reload on `SIGHUP` for runtime request-routing
state such as auth, providers, models, aliases, and metrics-backed inventory.
Listener address, listener timeout, log-level, and dashboard enablement changes
still require a restart. Unchanged rate-limit settings preserve existing buckets;
changed rate-limit settings reset limiter state.

See [docs/design.md](docs/design.md) for the full design document.

## Install

### via asdf

```sh
asdf plugin add aiproxy
# or
asdf plugin add aiproxy https://github.com/egose/aiproxy.git
```

Install and activate a version:

```sh
asdf list all aiproxy
asdf install aiproxy <version>
asdf install aiproxy latest
asdf global aiproxy <version>
```

Once installed, the `aiproxy` binary is available directly on your `PATH`:

```sh
aiproxy serve
aiproxy validate
aiproxy configure
aiproxy configure provider
aiproxy serve --config /etc/aiproxy/config.hcl
aiproxy validate --config /etc/aiproxy/config.hcl
aiproxy version
```

By default, the CLI looks for the config file at `$XDG_CONFIG_HOME/aiproxy/config.hcl`,
falling back to `~/.config/aiproxy/config.hcl` when `XDG_CONFIG_HOME` is unset.
Pass `--config` to use a different file.

## Example Configuration

```hcl
listener "http" "public" {
  address = ":8080"
}

upstream_header_timeout = "120s"

auth "main" {
  mode = "none"
}

logging {
  level      = "info"
  access_log = true
}

provider "openai" "openai" {
  display_name = "OpenAI"
  api_key      = env("OPENAI_API_KEY")

  model "gpt-4o-mini" {
    display_name = "GPT-4o mini"
    capabilities = ["chat"]
  }

  model "gpt-4.1" {
    display_name = "GPT-4.1"
    capabilities = ["chat", "responses"]
  }

  model "text-embedding-3-large" {
    display_name = "text-embedding-3-large"
    capabilities = ["embeddings"]
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
    provider = "localai"
    model    = "qwen3-32b"
  }
}
```

## CLI

```sh
aiproxy paths
aiproxy examples
aiproxy configure
aiproxy configure provider
aiproxy configure provider --config /etc/aiproxy/config.hcl --non-interactive --name backup --type openai-compatible --base-url https://llm.internal/v1 --secrets-key localai --api-key "$LOCALAI_API_KEY" --model qwen3-32b
aiproxy serve --config /etc/aiproxy/config.hcl
aiproxy validate --config /etc/aiproxy/config.hcl
aiproxy version
```

## Config Wizard

Use the built-in configure wizard to create or update the HCL and secrets file
without editing blocks by hand.

Interactive flows:

```sh
aiproxy configure
aiproxy configure provider
aiproxy configure auth
aiproxy configure alias
aiproxy configure upstream
aiproxy configure logging
```

The wizard prompts for the config path first when `--config` is not provided,
defaulting to `$XDG_CONFIG_HOME/aiproxy/config.hcl` and falling back to
`~/.config/aiproxy/config.hcl`.

Supported block workflows:

- `listener`
- `auth`
- `provider`
- `alias`
- `upstream`
- `logging`
- `provider-health`

Block subcommands also support non-interactive scripting with flags:

```sh
aiproxy configure provider \
  --config /etc/aiproxy/config.hcl \
  --non-interactive \
  --name backup \
  --type openai-compatible \
  --display-name "Backup provider" \
  --base-url https://llm.internal/v1 \
  --upstream-header-timeout 180s \
  --secrets-path /etc/aiproxy/keys.json \
  --secrets-key localai \
  --api-key "$LOCALAI_API_KEY" \
  --model qwen3-32b=qwen/qwen3-32b \
  --model-capabilities qwen3-32b=chat,responses

aiproxy configure alias \
  --config /etc/aiproxy/config.hcl \
  --non-interactive \
  --name chat_default \
  --algorithm round_robin \
  --target primary/gpt-4o-mini \
  --target backup/qwen3-32b
```

Use `upstream_header_timeout` to control how long the proxy waits for upstream response headers. Provider values override the root value; otherwise the default is 90 seconds. This timeout does not cap response bodies or SSE streams after headers arrive.

Delete existing blocks with `--delete`:

```sh
aiproxy configure provider --config /etc/aiproxy/config.hcl --delete --name backup
aiproxy configure alias --config /etc/aiproxy/config.hcl --delete --name chat_default
```

## Docker

```sh
docker build -t aiproxy .
docker run --rm \
  -p 8080:8080 \
  -v ./config.hcl:/etc/aiproxy/config.hcl:ro \
  -e OPENAI_API_KEY=... \
  aiproxy
```

## Local Run

Create `config.hcl` then:

```sh
go run ./cmd/aiproxy serve --config config.hcl
```

## Tests

```sh
go test ./...                # unit tests
make vet test               # vet + unit tests
make test-race              # unit tests with the race detector
```

The repo also includes stub-backed end-to-end tests that run as part of the
normal Go test suite. These use in-process HTTP test servers as upstream
providers so the full request path can be exercised without external services.

Integration tests are intentionally skipped for now. The repo does not yet
ship sandbox services for stable end-to-end provider testing. Reintroduce
integration coverage once the sandbox stack is added.

## Environment Variables

Use `env("VAR")` in any string attribute in the HCL config to inline an
environment variable. This is necessary for secrets — do not commit secret
values into the config file.

## API Key Reference Files

When a provider uses `api_key_ref`, the proxy reads the key from a JSON file
mapping string keys to string API keys:

```json
{
  "openai": "sk-...",
  "localai": "secret"
}
```

The file path defaults to `$XDG_CONFIG_HOME/aiproxy/keys.json`, falling back to
`~/.config/aiproxy/keys.json` when `XDG_CONFIG_HOME` is unset. Override it per
provider with `api_key_ref { path = "..." key = "..." }`.

## Optional Configuration Blocks

### `metrics`

The optional `metrics` block governs Prometheus metric exposure. When present,
`GET /metrics` requires `Authorization: Bearer <token>` with the configured
token; the token is checked independently from API auth client tokens.

```hcl
metrics {
  token = env("AIPROXY_METRICS_TOKEN")
}
```

- The block is optional. When absent, `/metrics` is not exposed.
- An empty token is rejected at config validation; metrics are never exposed
  without a dedicated credential.
- API auth clients (`auth.client` blocks) cannot scrape `/metrics` with their
  own tokens — they are checked against the metrics token separately.

### `dashboard`

The optional `dashboard` block enables the `aiproxy dashboard` TUI and the
`/_internal/dashboard/*` HTTP endpoints on the proxy listener.

```hcl
dashboard {
  token                 = env("AIPROXY_DASHBOARD_TOKEN")
  allow_insecure_remote = false
}
```

- `token` is optional. When omitted, `aiproxy serve` mints a random secret at
  startup and persists it to `$XDG_CONFIG_HOME/aiproxy/dashboard.token`; the
  `dashboard` command reads that file to authenticate. Declared tokens are
  used as-is and the file is not written.
- `allow_insecure_remote` (optional, default `false`) authorizes the dashboard
  command to talk to a non-loopback plain-HTTP listener. When `true`, `token`
  must be declared explicitly in config and be at least 32 characters long; a
  minted or weak token is rejected at validation. HTTPS listeners always
  satisfy the transport check.

### `provider { enabled = false }`

Use the optional `enabled` field on a `provider` block to intentionally disable
a provider. Disabled providers are structurally validated (name, type, base
URL, models, capabilities) but do not require a usable `api_key` or
`api_key_ref`. Enabled providers with an unresolved, empty, or missing
credential fail validation.

```hcl
provider "openai" "backup" {
  enabled = false
  model "gpt-4o-mini" {}
}
```

## Notes on Behavior

- `upstream_name` (optional on `model` blocks) lets the proxy-visible model name
  differ from the exact string sent upstream; it defaults to the model block
  label.
- `capabilities` (optional on `model` blocks) lets you narrow the effective
  API surface for a model. Supported values are `chat`, `responses`,
  `embeddings`, `images`, `audio_transcriptions`, and `audio_speech`.
- Provider and alias names are lowercase and must not contain spaces or `/`;
  provider name `alias` is reserved. Model names follow the same lowercase rule
  and may contain `/` when every slash-separated segment is valid.
- Provider `base_url` values must be absolute `https` URLs for remote upstreams.
  Plain `http` is accepted only for loopback development endpoints.
- A provider with no resolved credential, including an empty `api_key = env("...")`,
  fails validation when the provider is enabled. To intentionally disable a
  provider, set `enabled = false`; disabled providers are still structurally
  validated (name, type, base URL, models, capabilities) but do not require a
  usable credential.
- `/v1/models` returns effective capabilities for both direct models and
  aliases. Alias capabilities are the safe intersection of their target models.
- `/v1/models` also includes richer metadata:
  - direct models include `display_name` and `provider_type`
  - aliases include `alias_targets` summaries with provider, model, and
    resolved display name
- Alias `least_connections` selection is per-process and best-effort; it is not
  coordinated across multiple proxy instances.
- Provider health state is shared in-process across requests and aliases.
  Transient transport failures and upstream `5xx` responses temporarily mark a
  provider unhealthy for routing and readiness decisions, but this state is not
  coordinated across multiple proxy instances unless `provider_health.redis_url`
  is configured. When `provider_health.redis_url` is configured and a Redis read
  fails, routing and readiness fall back to a bounded in-process cache
  (`provider_health.cache_ttl`, default 30s) and fail open only when no fresh
  cache entry exists; the fallback and the underlying backend error are
  recorded as Prometheus metrics.
- Direct `<provider>/<model>` requests do not fail over to other targets.
- Alias requests retry the next target only on transport errors, timeouts, and
  upstream `5xx`; upstream `4xx` responses are returned to the client verbatim.
- Anthropic providers are translated through the Messages API for both JSON and
  SSE streaming chat completions.
- Gemini providers are translated through `generateContent` and
  `streamGenerateContent?alt=sse` for JSON and SSE streaming chat completions.
- `POST /v1/embeddings` is currently implemented for `openai`,
  `openai-compatible`, and `gemini` providers. Requests targeting `anthropic`
  models return a client-visible unsupported-operation error.
- `POST /v1/images/generations` is currently implemented for `openai` and
  `openai-compatible` providers. Requests targeting translated providers return
  a client-visible unsupported-operation error.
- `POST /v1/audio/transcriptions` is currently implemented for `openai` and
  `openai-compatible` providers. Requests targeting translated providers return
  a client-visible unsupported-operation error.
- `POST /v1/audio/speech` is currently implemented for `openai` and
  `openai-compatible` providers. Requests targeting translated providers return
  a client-visible unsupported-operation error.
- `POST /v1/responses` is currently implemented for `openai`,
  `openai-compatible`, `anthropic`, and `gemini` providers. The translated
  provider path supports a conservative request subset for both JSON and
  streaming responses.
- `/metrics` exposes Prometheus-format metrics for provider selection, alias
  retries, skipped providers, readiness state, startup inventory gauges for
  build version / auth mode / provider types / alias algorithms, explicit
  readiness reason gauges, inbound HTTP request counts / latency by method and
  path, request / response body size histograms,
  streaming response counts / duration, proxy-generated HTTP error counts by
  endpoint and error type, alias in-flight request gauges by target, provider
  health gauges, upstream request counts / latency / response body size by
  operation and provider, provider health backend error counts, and provider
  health fallback counts by operation and reason. `/metrics` requires a
  dedicated bearer token declared in a `metrics { token = env("...") }` block;
  `GET /metrics` without a valid `Authorization: Bearer <token>` header returns
  `401`. The metrics token is independent of API auth client tokens.
- API keys and client bearer tokens are never logged.
- The interactive `aiproxy dashboard` command calls `/_internal/dashboard/*` on
  the same listener as the proxy API. Plain HTTP dashboard RPC is allowed only
  when the effective listener address is loopback; non-loopback plain HTTP is
  rejected unless `dashboard { allow_insecure_remote = true }` is set with a
  strong explicit `token` (at least 32 characters) declared in config. HTTPS
  listeners are always allowed. Repeated invalid dashboard tokens are rate
  limited with `429` and a `Retry-After` header.

## Deferred / Planned

See the "Deferred Features" section in [docs/design.md](docs/design.md) for the
full list, including image and audio APIs, rate limiting, and hot config
reload.
