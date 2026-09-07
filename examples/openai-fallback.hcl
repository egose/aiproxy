# Starter alias across hosted OpenAI and a self-hosted OpenAI-compatible
# backend. Builds on this topology: alias-retry.hcl (failover tuning),
# env-secrets.hcl (env-substituted secrets), api-key-ref.hcl (key file).
listener "http" "public" {
  address = ":8080"
}

auth "main" {
  mode = "bearer_static"

  client "internal-app" {
    token          = "test-placeholder-token" # pragma: allowlist secret
    allowed_models = ["alias/chat_default", "openai/gpt-4.1"]
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

provider "openai-compatible" "localai" {
  display_name = "LocalAI"
  base_url     = "https://llm.internal/v1"
  api_key      = "test-placeholder-key" # pragma: allowlist secret

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
