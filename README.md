# aiproxy

A Go service that proxies multiple AI providers behind a single
OpenAI-compatible HTTP API. It resolves client-supplied model strings to
configured provider-backed models or aliases, forwards requests upstream
(translating when needed for non-OpenAI providers), and returns
OpenAI-compatible responses.

## Current MVP Scope

### Supported Public API

- `GET /v1/models`
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

### Provider Types

- `openai` – built-in OpenAI adapter (pass-through)
- `openai-compatible` – any OpenAI-compatible endpoint (requires `base_url`)
- `anthropic` – chat and responses translation to Anthropic Messages API
- `gemini` – chat translation to Gemini generateContent API, embeddings translation to Gemini embedContent API, and responses translation through generateContent

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
Listener address and timeout changes still require a restart.

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
aiproxy serve --config /etc/aiproxy/config.hcl
aiproxy validate --config /etc/aiproxy/config.hcl
aiproxy version
```

## Example Configuration

```hcl
listener "http" "public" {
  address = ":8080"
}

auth "main" {
  mode = "none"
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
aiproxy serve --config /etc/aiproxy/config.hcl
aiproxy validate --config /etc/aiproxy/config.hcl
aiproxy version
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

## Notes on Behavior

- `upstream_name` (optional on `model` blocks) lets the proxy-visible model name
  differ from the exact string sent upstream; it defaults to the model block
  label.
- `capabilities` (optional on `model` blocks) lets you narrow the effective
  API surface for a model. Supported values are `chat`, `responses`,
  `embeddings`, `images`, `audio_transcriptions`, and `audio_speech`.
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
  coordinated across multiple proxy instances.
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
  health gauges, and upstream request counts / latency / response body size by
  operation and provider.
- API keys and client bearer tokens are never logged.

## Deferred / Planned

See the "Deferred Features" section in [docs/design.md](docs/design.md) for the
full list, including image and audio APIs, rate limiting, and hot config
reload.
