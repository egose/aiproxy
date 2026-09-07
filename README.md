# aiproxy

A Go service that proxies multiple AI providers behind a single
OpenAI-compatible HTTP API. It resolves client-supplied model strings to
configured provider-backed models or aliases, forwards requests upstream
(translating when needed for non-OpenAI providers), and returns
OpenAI-compatible responses.

## Current Supported Scope

### Supported Public API

<!-- docs-contract:public-matrix:start -->

| Surface                         | `openai`                           | `openai-compatible`                | `anthropic`                        | `gemini`                           | `opencode-zen`                           | `opencode-go`                            |
| ------------------------------- | ---------------------------------- | ---------------------------------- | ---------------------------------- | ---------------------------------- | ---------------------------------------- | ---------------------------------------- |
| `GET /v1/models`                | Proxy-owned                        | Proxy-owned                        | Proxy-owned                        | Proxy-owned                        | Proxy-owned                              | Proxy-owned                              |
| `GET /v1/billing/usage`         | Proxy-owned local usage accounting | Proxy-owned local usage accounting | Proxy-owned local usage accounting | Proxy-owned local usage accounting | Proxy-owned local usage accounting       | Proxy-owned local usage accounting       |
| `GET /metrics`                  | Proxy-owned Prometheus metrics     | Proxy-owned Prometheus metrics     | Proxy-owned Prometheus metrics     | Proxy-owned Prometheus metrics     | Proxy-owned Prometheus metrics           | Proxy-owned Prometheus metrics           |
| `POST /v1/chat/completions`     | JSON and SSE                       | JSON and SSE                       | JSON and SSE translated            | JSON and SSE translated            | JSON and SSE native or translated subset | JSON and SSE native or translated subset |
| `POST /v1/embeddings`           | Yes                                | Yes                                | No                                 | Yes                                | No                                       | No                                       |
| `POST /v1/responses`            | JSON and SSE                       | JSON and SSE                       | JSON and SSE translated subset     | JSON and SSE translated subset     | JSON and SSE native or translated subset | JSON and SSE native or translated subset |
| `POST /v1/images/generations`   | Yes                                | Yes                                | No                                 | No                                 | No                                       | No                                       |
| `POST /v1/audio/transcriptions` | Yes                                | Yes                                | No                                 | No                                 | No                                       | No                                       |
| `POST /v1/audio/speech`         | Yes                                | Yes                                | No                                 | No                                 | No                                       | No                                       |

<!-- docs-contract:public-matrix:end -->

<!-- docs-contract:capability-matrix:start -->

| Provider type       | Default capabilities when omitted          | Additional supported capabilities                |
| ------------------- | ------------------------------------------ | ------------------------------------------------ |
| `openai`            | `chat`, `responses`, `embeddings`          | `images`, `audio_transcriptions`, `audio_speech` |
| `openai-compatible` | `chat`, `responses`, `embeddings`          | `images`, `audio_transcriptions`, `audio_speech` |
| `anthropic`         | `chat`, `responses`                        | None                                             |
| `gemini`            | `chat`, `responses`                        | `embeddings`                                     |
| `opencode-zen`      | `chat`, `responses`, or both (by protocol) | None                                             |
| `opencode-go`       | `chat`, `responses`, or both (by protocol) | None                                             |

<!-- docs-contract:capability-matrix:end -->

Capabilities are enforced per configured model. If `capabilities` is omitted,
the provider-type defaults above are used; explicit values can narrow or, where
listed as additionally supported, opt a model into more operations. For
`opencode-zen` and `opencode-go` the omitted default comes from the model's
required `protocol`: `chat` defaults to `chat`, `responses` defaults to
`responses`, and `messages` (both services) plus `gemini` (Zen only) default to
`chat` and `responses`.

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
- `opencode-zen` – OpenCode Zen service (defaults to `https://opencode.ai/zen/v1`); every model declares a required `protocol`
- `opencode-go` – OpenCode Go service (defaults to `https://opencode.ai/zen/go/v1`); every model declares a required `protocol`

Pass-through providers rewrite only the top-level `model` JSON field and preserve other request fields, including unknown extension fields. Translated providers reject unsupported top-level request controls rather than silently dropping them; their supported chat fields are `model`, `messages`, `max_tokens`, `temperature`, `top_p`, and `stream`, their supported responses fields are `model`, `input`, `instructions`, `max_output_tokens`, `temperature`, `top_p`, and `stream`, and Gemini embeddings supports `model`, `input`, and `dimensions`.

OpenCode providers share one adapter behind the two explicit types: the type
selects the service, never the URL or credential. `protocol` is `chat`,
`responses`, `messages`, or `gemini` (`gemini` is Zen-only). `chat` and
`responses` protocols are native pass-through and each serve one public
operation; `messages` and `gemini` serve `chat` and `responses` through the
existing conservative translation subsets. A public operation the model's
protocol does not serve is rejected before any upstream I/O. `base_url` is an
optional transport override only and never reclassifies the service. Every
upstream request sends `User-Agent: aiproxy/<version>`; `opencode-zen` and
`opencode-go` additionally send `x-opencode-session` (a caller-supplied value is forwarded
only when it is 1-128 `[A-Za-z0-9_-]` characters, otherwise a fresh per-request
ID is generated). No other inbound headers or credentials are forwarded.
Public model names look like `zen/glm-5.3` and `go/minimax-m3`:

```hcl
provider "opencode-zen" "zen" {
  api_key = env("OPENCODE_ZEN_API_KEY")

  model "glm-5.3" {
    protocol     = "chat"
    capabilities = ["chat"]
  }

  model "claude-sonnet-5" {
    protocol = "messages"
  }
}

provider "opencode-go" "go" {
  api_key = env("OPENCODE_GO_API_KEY")

  model "minimax-m3" {
    protocol = "messages"
  }

  model "glm-5.3" {
    protocol = "chat"
  }
}
```

See `examples/opencode-zen.hcl` and `examples/opencode-go.hcl` for complete
validated configs, including explicit alias targets.

### Routing

- Direct model addressing: `<provider-name>/<model-name>`
- Alias addressing: `alias/<alias-name>`
- An alias is a virtual model backed by one or more concrete provider/model targets
- Alias algorithms: `round_robin`, `least_connections`
- Alias failover: retry the next target on transport errors, timeouts, and
  upstream statuses listed in `retry_status_codes`. The default is `500`, `502`,
  `503`, and `504`; configured `4xx` statuses such as `429` can be retried.
  Other upstream `4xx` responses are returned verbatim.

### Not Implemented

- External billing, invoicing, and quota systems. The implemented
  `/v1/billing/usage` endpoint is local in-process usage accounting, not an
  external billing or quota authority.

The server supports live config reload on `SIGHUP` for auth, providers, models,
aliases, root and provider upstream header timeouts, access-log enablement,
metrics config, provider-health config, and metrics-backed inventory state.
Listener address, listener timeout, logging level, and enabling the dashboard
after startup require a restart. Unchanged rate-limit settings preserve existing
buckets; changed rate-limit settings reset limiter state.

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

Foreground `aiproxy serve` is supported across the advertised release targets.
Linux additionally supports `aiproxy serve -d` and the `aiproxy status`,
`aiproxy stop`, and `aiproxy restart` daemon lifecycle commands. On non-Linux
platforms those daemon lifecycle commands return `daemon lifecycle is
unsupported on this platform`.

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
aiproxy configure provider --config /etc/aiproxy/config.hcl --non-interactive --name backup-2 --type openai-compatible --extends backup --secrets-key backup-2 --api-key "$BACKUP_2_API_KEY"
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

aiproxy configure provider \
  --config /etc/aiproxy/config.hcl \
  --non-interactive \
  --name zen \
  --type opencode-zen \
  --api-key-env OPENCODE_ZEN_API_KEY \
  --model glm-5.3 \
  --model-protocol glm-5.3=chat

aiproxy configure alias \
  --config /etc/aiproxy/config.hcl \
  --non-interactive \
  --name chat_default \
  --algorithm round_robin \
  --target primary/gpt-4o-mini \
  --target backup/qwen3-32b
```

Use `upstream_header_timeout` to control how long the proxy waits for upstream response headers. Provider values override the root value; otherwise the default is 90 seconds. This timeout does not cap response bodies or SSE streams after headers arrive.

Use `extends` when several credentials share one provider type, endpoint, timeout, and model inventory. A derived provider keeps the two-label provider form and may declare only `extends`, optional `display_name`, and exactly one local credential (`api_key` or `api_key_ref`):

```hcl
provider "openai-compatible" "nvidia-1" {
  display_name = "Nvidia - j.dev"
  base_url     = "https://integrate.api.nvidia.com/v1"

  api_key_ref { key = "nvidia-1" }

  model "z-ai/glm-5.2" {
    display_name = "GLM 5.2"
    capabilities = ["chat", "responses"]
  }
}

provider "openai-compatible" "nvidia-2" {
  extends      = "nvidia-1"
  display_name = "Nvidia - corean"

  api_key_ref { key = "nvidia-2" }
}
```

Derived providers may appear before or after the base. The base must be enabled, concrete, and the same provider type; inheritance chains are rejected. Aliases still enumerate each provider target explicitly, for example `nvidia-1/z-ai/glm-5.2` and `nvidia-2/z-ai/glm-5.2`.

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
make integration             # hermetic binary-level integration tests
make docs-contract           # public docs contract matrix check
```

The repo also includes stub-backed end-to-end tests that run as part of the
normal Go test suite. These use in-process HTTP test servers as upstream
providers so the full request path can be exercised without external services.

Hermetic binary-level integration tests run with local upstream stubs and are
part of normal CI. Real-provider sandbox tests remain separate and optional so
normal test runs do not require paid credentials or external services.

Documentation-only changes run the `Docs Contract` workflow, which executes
`make docs-contract` to keep the public endpoint/provider and capability
matrices synchronized across README, design, website, and agent guidance.

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
  token = env("AIPROXY_DASHBOARD_TOKEN")
}
```

- `token` is optional. When omitted, `aiproxy serve` mints a random secret at
  startup and persists it to `$XDG_CONFIG_HOME/aiproxy/dashboard.token`; the
  `dashboard` command reads that file to authenticate. Declared tokens are
  used as-is and the file is not written.
- The `dashboard` command is local-only. It connects to the configured listener
  over loopback plain HTTP with bearer authentication and refuses non-loopback
  listener hosts. Remote dashboard access requires a future explicit transport
  design.

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
- Listener `address` values must be TCP bind addresses in `host:port` form, not
  URLs. Native TLS and remote dashboard URLs are not configured on the listener.
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
  Transient transport failures, upstream request errors, and upstream `5xx`
  responses temporarily mark a provider unhealthy for routing and readiness
  decisions. Configured retryable `4xx` statuses can trigger alias failover but
  do not mark a provider unhealthy. This state is not coordinated across
  multiple proxy instances unless `provider_health.redis_url` is configured.
  When `provider_health.redis_url` is configured and a Redis read fails,
  routing and readiness fall back to a bounded in-process cache
  (`provider_health.cache_ttl`, default 30s) and fail open only when no fresh
  cache entry exists; the fallback and the underlying backend error are
  recorded as Prometheus metrics.
- Direct `<provider>/<model>` requests do not fail over to other targets.
- Alias requests retry the next target on transport errors, timeouts, and
  configured `retry_status_codes` in the `400`-`599` range. The default list is
  `500`, `502`, `503`, and `504`; other upstream `4xx` responses are returned to
  the client verbatim.
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
- `opencode-zen` and `opencode-go` serve `POST /v1/chat/completions` and
  `POST /v1/responses` per model protocol: `chat` serves chat only,
  `responses` serves responses only, and `messages` (both services) plus
  `gemini` (Zen only) serve both through the existing conservative translation
  subsets. Any other operation/protocol combination, including `embeddings`,
  `images`, and audio on both OpenCode types, is rejected before upstream I/O.
- `opencode-zen` and `opencode-go` send `x-opencode-session` on every upstream request for prompt
  caching. Callers may supply their own session value (1-128 characters of
  `[A-Za-z0-9_-]`); missing or invalid values get a fresh per-request ID, never
  a shared global session.
- Direct `<provider>/<model>` requests never cross OpenCode services. Upstream
  Go quota/limit errors are returned to the client like any other upstream
  error; only explicitly configured aliases retry another target, and the proxy
  never reroutes to the other service on its own. The upstream console setting
  that spends Zen balance past Go limits is an account setting, not proxy
  routing. Model catalogs are static configuration; the proxy performs no
  runtime catalog sync.
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
  the same listener as the proxy API. It derives a local plain-HTTP URL from the
  TCP bind address, maps wildcard binds such as `:8080` and `0.0.0.0:8080` to
  loopback, and refuses concrete non-loopback hosts. HTTPS and remote dashboard
  transports are not supported by the current configuration model. Repeated
  invalid dashboard tokens are rate limited with `429` and a `Retry-After`
  header.

## Deferred / Planned

See the "Deferred Features" section in [docs/design.md](docs/design.md) for the
full list, including external billing/invoicing, quotas, translated-provider
image and audio endpoints, and Anthropic embeddings.
