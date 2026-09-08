---
sidebar_position: 3
---

# Configuration

`aiproxy` uses labeled HCL blocks. The core building blocks are:

- `listener "http" "public"`
- `auth "main"`
- `logging`
- `provider "<type>" "<name>"`
- `alias "<name>"`

## Mental Model

Think about the config in five layers:

1. `listener` defines how the proxy accepts traffic.
2. `auth` defines who may call it.
3. `logging` defines structured log verbosity and request lifecycle access logging.
4. `provider` blocks define upstream systems and their models.
5. `alias` blocks define the client-facing virtual models used for routing and failover.

## Example

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

  rate_limit {
    requests_per_minute = 120
    burst               = 120
  }

  client "internal-app" {
    token          = env("AIPROXY_CLIENT_TOKEN")
    tenant         = "internal"
    allowed_models = ["alias/chat_default", "openai/gpt-4.1"]
  }
}

logging {
  level      = "info"
  access_log = true
}

provider "openai" "openai" {
  display_name = "OpenAI"
  api_key      = env("OPENAI_API_KEY")

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
    model    = "gpt-4.1"
  }

  target {
    provider = "localai"
    model    = "qwen3-32b"
  }
}
```

## Listener

The listener block configures the inbound HTTP server.

- `address` sets the listen address such as `:8080`
- `timeouts` configures read, idle, and write timeouts

Listener address and timeout changes still require a restart, even though runtime state such as auth, providers, models, aliases, upstream header timeouts, access-log enablement, metrics, and provider-health config can reload on `SIGHUP`. Logging level changes and enabling the dashboard after startup also require a restart.

For most deployments, one HTTP listener is enough.

## Logging

The optional `logging` block controls structured application logs.

- `level` accepts `debug`, `info`, `warn`, or `error`
- `access_log` enables or disables request lifecycle logs such as request received, upstream request start and finish, and response sent or stream start and finish

Defaults:

- `level = "info"`
- `access_log = true`

## Auth

Supported inbound auth modes:

- `none`
- `bearer_static`

`none` is only appropriate for trusted environments.

`bearer_static` validates client bearer tokens against statically configured `client` blocks. Each client may also define:

- `tenant`
- `allowed_models`

The optional `rate_limit` block is local and in-memory:

- In `bearer_static` mode, it is keyed per authenticated client
- In `none` mode, it applies to a shared anonymous bucket

Use `allowed_models` when you want a static allow-list at the proxy boundary rather than relying only on application-level policy.

## Providers

Providers always use two labels:

```hcl
provider "<type>" "<name>" {}
```

Common attributes:

- `display_name`
- `base_url` for `openai-compatible` (required), and as an optional transport
  override for `opencode-zen` and `opencode-go`
- `user_agent` as an optional upstream `User-Agent` override for
  `opencode-zen` and `opencode-go` (defaults to `aiproxy/<version>`)
- `extends` for restricted provider inheritance
- `api_key`
- `api_key_ref`
- `credential_ref` for `github-copilot` only (saved device-flow login)
- `upstream_header_timeout`
- `enabled` (optional, default `true`)
- nested `model` blocks (OpenCode models additionally require `protocol`)

Providers normally declare exactly one of `api_key` or `api_key_ref`. Enabled
providers with unresolved, empty, or missing credentials fail validation, with
one exception: `opencode-zen` providers may omit the credential entirely for
keyless upstream access, in which case the proxy sends no `Authorization`
header. To
intentionally disable a provider, declare `enabled = false`; disabled
providers are still validated for structure, URL, models, and capabilities,
but they do not require a usable credential.

`github-copilot` never uses `api_key`/`api_key_ref`. Provision with
`aiproxy login github-copilot --client-id <id> --credential <name>` (your own
public OAuth client ID, no secret), then reference the saved login:

```hcl
provider "github-copilot" "copilot" {
  credential_ref {
    name = "copilot-main"
  }

  model "gpt-5.4-nano" {}
}
```

`credential_ref.path` is optional and defaults to the shared secrets path so
the `copilot-<name>.json` sidecar is found next to `keys.json`. Derived
Copilot providers require their own local `credential_ref`. The token
activates on restart/`SIGHUP`; re-run `login` with the same client ID/name
and reload on `401`/`403`, revocation, or expiry.

```hcl
provider "openai" "backup" {
  enabled = false
  model "gpt-4o-mini" {}
}
```

Provider `base_url` values must be absolute `https` URLs for remote upstreams.
Plain `http` is accepted only for loopback development endpoints such as
`localhost`, `127.0.0.1`, or `::1`. `openai-compatible` requires `base_url`;
`opencode-zen` and `opencode-go` default to their service prefixes
(`https://opencode.ai/zen/v1` and `https://opencode.ai/zen/go/v1`) and accept
`base_url` only as a transport override for tests and custom gateways. An
override never changes service selection, auth, or header behavior.
`github-copilot` defaults to `https://api.githubcopilot.com` with the same
transport-override-only `base_url` rule.

Provider names are part of the public model string, so keep them stable and machine-friendly.

### OpenCode Zen And Go

`opencode-zen` and `opencode-go` share one adapter behind two explicit types;
the type selects the service, never the URL or credential. Every model
declares a required `protocol` (`chat`, `responses`, `messages`, or `gemini`;
`gemini` is Zen-only):

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

Public model names are `zen/glm-5.3` and `go/minimax-m3`. `chat` and
`responses` protocols are native pass-through serving one public operation
each; `messages` and `gemini` serve `chat` and `responses` through the
existing conservative translation subsets. Anything else, including
`embeddings`, `images`, and audio on both OpenCode types, is rejected before
upstream I/O. `opencode-zen` and `opencode-go` send `x-opencode-session` on every upstream
request (caller values are forwarded only when valid, falling back to a valid
caller `X-Session-Id`, otherwise a fresh
per-request ID is generated); direct requests never cross services, and only
explicitly configured aliases retry another target. See
[Providers and Routing](providers-and-routing.md) for the full contract and
`examples/opencode-zen.hcl` / `examples/opencode-go.hcl` for complete
validated configs.

### Provider Inheritance

Use `extends` when several accounts share the same provider type, endpoint, timeout, and model inventory but need separate credentials:

```hcl
provider "openai-compatible" "nvidia-1" {
  display_name = "Nvidia - j.dev"
  base_url     = "https://integrate.api.nvidia.com/v1"

  api_key_ref {
    key = "nvidia-1"
  }

  model "z-ai/glm-5.2" {
    display_name = "GLM 5.2"
    capabilities = ["chat", "responses"]
  }
}

provider "openai-compatible" "nvidia-2" {
  extends      = "nvidia-1"
  display_name = "Nvidia - corean"

  api_key_ref {
    key = "nvidia-2"
  }
}
```

A derived provider may be declared before or after its base. It may declare only `extends`, optional `display_name`, and exactly one local credential, either `api_key` or `api_key_ref`. It inherits the base provider type, `base_url`, effective upstream header timeout, enabled state, and all model blocks.

`github-copilot` derivatives instead require a local `credential_ref` and stay compact (no `base_url`/models); `credential_ref` is rejected on all other types, and `api_key`/`api_key_ref` are rejected on Copilot blocks.

The type label remains required and must match the base. The base must exist, be enabled, and must not itself use `extends`; inheritance chains are rejected. Local `base_url`, `upstream_header_timeout`, `enabled`, and `model` declarations are rejected instead of ignored.

Derived providers are flattened during config loading and reload. After a successful load, direct model strings, health, metrics, billing, and dashboard inventory use the derived provider's own name. Aliases still list each provider target explicitly.

## Upstream Header Timeout

`upstream_header_timeout` controls how long the proxy waits for upstream response headers. It accepts Go duration strings such as `30s`, `2m`, or `1h`.

You can set it globally at the root or override it per provider:

```hcl
upstream_header_timeout = "120s"

provider "openai" "openai" {
  upstream_header_timeout = "180s"
}
```

Precedence is provider value, then root value, then the 90-second default. The timeout applies only until response headers arrive; JSON and streaming response bodies can continue for any duration after headers are received. Root and provider timeout changes apply on a successful `SIGHUP` reload.

## Models

Each provider contains one or more `model` blocks:

```hcl
model "gpt-4.1" {
  display_name = "GPT-4.1"
  upstream_name = "gpt-4.1"
  capabilities  = ["chat", "responses"]
}
```

- The block label is the proxy-visible model name
- `display_name` is optional metadata
- `upstream_name` lets the upstream identifier differ from the public name
- `protocol` is required on `opencode-zen` and `opencode-go` models (`chat`,
  `responses`, `messages`, or `gemini`; `gemini` is Zen-only) and rejected on
  other provider types
- `capabilities` narrows the operations exposed through the proxy

Use `upstream_name` when you want a cleaner or more stable public model name than the exact upstream identifier.

Supported capability values:

- `chat`
- `responses`
- `embeddings`
- `images`
- `audio_transcriptions`
- `audio_speech`

Omitted `capabilities` default to the provider-type defaults, except on
OpenCode providers where the default is protocol-aware (`chat` serves `chat`,
`responses` serves `responses`, `messages` and `gemini` serve both).

## Secrets And Environment Variables

Use `env("VAR")` anywhere a string is allowed. Values are inlined before HCL parsing.

That makes it suitable for API keys, bearer tokens, URLs, and other deployment-specific values.

Set `$AIPROXY_CONFIG` to inline the whole HCL document and skip the config file:

```sh
export AIPROXY_CONFIG='listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "openai" "openai" {
  api_key = env("OPENAI_API_KEY")
  model "gpt-4o-mini" {}
}'
aiproxy validate
aiproxy serve
```

An explicit `--config` overrides `$AIPROXY_CONFIG`. `serve -d` and the
`configure`/`login` file workflows require a file. `SIGHUP` re-reads
`$AIPROXY_CONFIG` when the server was started from it.

For local runs, if your config depends on variables in `.env`, load them first:

```sh
set -a; . ./.env; set +a
```

## `api_key_ref`

When a provider uses `api_key_ref`, `aiproxy` reads the secret from a JSON file:

```json
{
  "openai": "sk-...",
  "localai": "secret"
}
```

The default path is:

- `$XDG_CONFIG_HOME/aiproxy/keys.json`
- or `~/.config/aiproxy/keys.json` when `XDG_CONFIG_HOME` is unset

You can override the file path per provider:

```hcl
api_key_ref {
  path = "/etc/aiproxy/keys.json"
  key  = "localai"
}
```

Use `api_key_ref` when you want provider secrets stored outside the main HCL file.

### `credential_ref` (GitHub Copilot)

`credential_ref` references a structured sidecar written by `aiproxy login github-copilot`:

```hcl
credential_ref {
  name = "copilot-main"
}
```

The sidecar lives at `<secrets-dir>/copilot-<name>.json` (`0600`). `path` is
optional and defaults to the shared secrets path. `configure provider
--credential/--credential-path` writes this block without OAuth networking or
token display.

## Naming Rules

`aiproxy` keeps public names intentionally strict:

- provider names are lowercase
- alias names are lowercase
- names must not contain spaces
- provider and alias names must not contain `/`
- provider name `alias` is reserved for `alias/<alias-name>` routing
- model names may contain `/` when every slash-separated segment follows the
  same lowercase name rule

These rules keep model parsing simple and unambiguous; direct model resolution
splits on the first `/`, so `<provider-name>/<model-name>` still works when the
model name contains additional slashes.

## Validation Rules

Startup fails on invalid configuration. Important checks include:

- duplicate provider or alias names
- invalid provider types or alias algorithms
- provider or alias names that are not lowercase or contain spaces or `/`
- provider name `alias`
- model names with invalid slash-separated segments
- `openai-compatible` providers missing `base_url`
- malformed provider `base_url` values, and non-loopback `http` base URLs
- `opencode-zen` or `opencode-go` models missing `protocol`, using an unknown
  protocol, using `gemini` on `opencode-go`, or declaring a capability the
  protocol does not serve; `protocol` on any other provider type
- `user_agent` on any non-OpenCode provider type (including `github-copilot`)
- providers with both `api_key` and `api_key_ref`
- `github-copilot` providers with `api_key`/`api_key_ref`, or `credential_ref`
  on any other provider type; enabled Copilot providers without a resolvable
  sidecar credential
- enabled providers with no resolved credential, including an empty
  `api_key = env("...")`; missing or empty credentials fail validation unless
  `enabled = false` is declared explicitly or the provider type is
  `opencode-zen`
- providers without any models
- aliases without any targets
- alias targets that reference unknown providers or models
- a `metrics` block with an empty or missing token
- listener addresses that are URLs instead of TCP `host:port` bind addresses
- a `dashboard` block with `allow_insecure_remote = true`; the dashboard command
  is local-only

## Optional Blocks

### `metrics`

```hcl
metrics {
  token = env("AIPROXY_METRICS_TOKEN")
}
```

When present, `GET /metrics` requires `Authorization: Bearer <token>` with the
configured value. The metrics token is checked independently of API auth
client tokens; API clients cannot scrape `/metrics` with their own credentials.

### `dashboard`

```hcl
dashboard {
  token = env("AIPROXY_DASHBOARD_TOKEN")
}
```

When `token` is omitted, `aiproxy serve` mints a random secret at startup and
persists it to `$XDG_CONFIG_HOME/aiproxy/dashboard.token`; the `dashboard`
command reads that file to authenticate. If a reload drops a previously
declared token, the carried-over secret is published to the file before the
new runtime activates, so tokenless discovery keeps working; a persistence
failure rejects the reload and keeps the old runtime unchanged. The dashboard command is local-only: it
connects over loopback plain HTTP and refuses concrete non-loopback listener
hosts. HTTPS and remote dashboard URLs are not supported by the current
configuration model.
