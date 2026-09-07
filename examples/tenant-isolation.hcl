# Tenant isolation with a standby provider. Each client is pinned to its
# own tenant and model allow-list, and billing usage summaries scope to
# the caller tenant. The disabled backup provider needs no credential and
# stays out of alias targets until it is re-enabled; aliases can only
# reference enabled providers.
listener "http" "public" {
  address = ":8080"
}

auth "main" {
  mode = "bearer_static"

  rate_limit {
    requests_per_minute = 120
    burst               = 120
  }

  client "team-a-app" {
    token          = "test-placeholder-token-a" # pragma: allowlist secret
    tenant         = "team-a"
    allowed_models = ["alias/chat_default"]
  }

  client "team-b-app" {
    token          = "test-placeholder-token-b" # pragma: allowlist secret
    tenant         = "team-b"
    allowed_models = ["openai/gpt-4.1"]
  }
}

provider "openai" "openai" {
  display_name = "OpenAI"
  api_key      = "test-placeholder-key" # pragma: allowlist secret

  model "gpt-4.1" {
    display_name = "GPT-4.1"
    capabilities = ["chat", "responses"]
  }
}

provider "anthropic" "anthropic" {
  display_name = "Anthropic"
  api_key      = "test-placeholder-key" # pragma: allowlist secret

  model "claude-sonnet" {
    display_name  = "Claude Sonnet"
    upstream_name = "claude-sonnet-4-20250514"
    capabilities  = ["chat", "responses"]
  }
}

provider "openai-compatible" "backup" {
  enabled = false

  display_name = "Standby backup"
  base_url     = "https://llm.internal/v1"

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
    provider = "anthropic"
    model    = "claude-sonnet"
  }
}
