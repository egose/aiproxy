---
sidebar_position: 5
---

# Config Examples

This page collects complete configuration examples for common `aiproxy` setups.

Use these as starting points, then adapt provider names, model names, auth settings, and secret sources for your environment.

## Local Development With One Provider

This is the smallest useful config. It exposes one OpenAI-backed model and skips inbound auth.

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
    capabilities = ["chat", "responses"]
  }
}
```

Use this when:

- you are testing locally
- the proxy sits behind another trusted boundary
- you want the shortest path to a working setup

## OpenAI Plus OpenAI-Compatible Fallback

This setup exposes a stable alias while routing across a hosted model and a self-hosted OpenAI-compatible backend.

```hcl
listener "http" "public" {
  address = ":8080"
}

auth "main" {
  mode = "bearer_static"

  client "internal-app" {
    token          = env("AIPROXY_CLIENT_TOKEN")
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
}

provider "openai-compatible" "localai" {
  display_name = "LocalAI"
  base_url     = "https://llm.internal/v1"

  api_key_ref {
    key = "localai"
  }

  model "qwen3-32b" {
    display_name = "Qwen 3 32B"
    capabilities = ["chat", "responses"]
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

Use this when:

- clients should call one stable virtual model
- you want simple balancing across two backends
- you want alias retry behavior on transport failures and upstream `5xx`

## Multi-Provider Chat Pool With Tenant-Aware Auth

This example mixes translated and pass-through providers and adds tenant metadata plus a local rate limit.

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
    requests_per_minute = 240
    burst               = 240
  }

  client "team-a-app" {
    token          = env("TEAM_A_TOKEN")
    tenant         = "team-a"
    allowed_models = ["alias/chat_default", "alias/chat_fallback"]
  }

  client "team-b-app" {
    token          = env("TEAM_B_TOKEN")
    tenant         = "team-b"
    allowed_models = ["alias/chat_default"]
  }
}

provider "openai" "openai" {
  display_name = "OpenAI"
  api_key      = env("OPENAI_API_KEY")

  model "gpt-4.1" {
    display_name = "GPT-4.1"
    capabilities = ["chat", "responses"]
  }
}

provider "anthropic" "anthropic" {
  display_name = "Anthropic"

  api_key_ref {
    key = "anthropic"
  }

  model "claude-sonnet" {
    display_name  = "Claude Sonnet"
    upstream_name = "claude-sonnet-4-20250514"
    capabilities  = ["chat", "responses"]
  }
}

provider "gemini" "gemini" {
  display_name = "Gemini"
  api_key      = env("GEMINI_API_KEY")

  model "gemini-2.5-pro" {
    display_name = "Gemini 2.5 Pro"
    capabilities = ["chat", "responses"]
  }
}

alias "chat_default" {
  algorithm = "round_robin"

  target {
    provider = "openai"
    model    = "gpt-4.1"
  }

  target {
    provider = "anthropic"
    model    = "claude-sonnet"
  }

  target {
    provider = "gemini"
    model    = "gemini-2.5-pro"
  }
}

alias "chat_fallback" {
  algorithm = "least_connections"

  target {
    provider = "openai"
    model    = "gpt-4.1"
  }

  target {
    provider = "gemini"
    model    = "gemini-2.5-pro"
  }
}
```

Use this when:

- different teams or clients need separate identities
- you want a stable chat pool spanning multiple providers
- you want usage summaries scoped by tenant where present

## Shared Provider Health Across Instances

Add `provider_health` when you want transient provider health state shared across instances through Redis.

```hcl
provider_health {
  redis_url  = "redis://127.0.0.1:6379"
  key_prefix = "aiproxy:prod"
  cooldown   = "45s"
}
```

Use this when:

- you run more than one proxy instance
- you want transient provider failure state shared between them

Without this block, provider health is tracked in-process only.

## Key File Example

When you use `api_key_ref`, the key file is a JSON object mapping key names to secret strings:

```json
{
  "openai": "sk-...",
  "anthropic": "sk-ant-...",
  "localai": "secret"
}
```

Point a provider at one of these entries with:

```hcl
api_key_ref {
  path = "/etc/aiproxy/keys.json"
  key  = "openai"
}
```

## Tips

- keep provider names stable because they appear in public model strings
- use aliases when clients should not depend on a single concrete upstream model
- use `capabilities` to narrow the public contract to the operations you actually want to expose
- keep secrets in environment variables or a key file instead of inline literals
