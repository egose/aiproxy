---
sidebar_position: 3
---

# Configuration

`aiproxy` uses labeled HCL blocks. The core building blocks are:

- `listener "http" "public"`
- `auth "main"`
- `provider "<type>" "<name>"`
- `alias "<name>"`

## Mental Model

Think about the config in four layers:

1. `listener` defines how the proxy accepts traffic.
2. `auth` defines who may call it.
3. `provider` blocks define upstream systems and their models.
4. `alias` blocks define the client-facing virtual models used for routing and failover.

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

Listener address and timeout changes still require a restart, even though some runtime state can reload on `SIGHUP`.

For most deployments, one HTTP listener is enough.

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
- `base_url` for `openai-compatible`
- `api_key`
- `api_key_ref`
- nested `model` blocks

Exactly one of `api_key` or `api_key_ref` must be set for a provider.

Provider names are part of the public model string, so keep them stable and machine-friendly.

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
- `capabilities` narrows the operations exposed through the proxy

Use `upstream_name` when you want a cleaner or more stable public model name than the exact upstream identifier.

Supported capability values:

- `chat`
- `responses`
- `embeddings`
- `images`
- `audio_transcriptions`
- `audio_speech`

## Secrets And Environment Variables

Use `env("VAR")` anywhere a string is allowed. Values are inlined before HCL parsing.

That makes it suitable for API keys, bearer tokens, URLs, and other deployment-specific values.

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

## Naming Rules

`aiproxy` keeps public names intentionally strict:

- provider names are lowercase
- alias names are lowercase
- names must not contain spaces
- names must not contain `/`

These rules keep model parsing simple and unambiguous.

## Validation Rules

Startup fails on invalid configuration. Important checks include:

- duplicate provider or alias names
- invalid provider types or alias algorithms
- names that are not lowercase or contain spaces or `/`
- `openai-compatible` providers missing `base_url`
- providers with both `api_key` and `api_key_ref`
- providers with neither `api_key` nor `api_key_ref`
- providers without any models
- aliases without any targets
- alias targets that reference unknown providers or models
