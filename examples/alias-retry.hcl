# Alias failover tuning. Alias requests retry the next target on
# transport errors, timeouts, and statuses in retry_status_codes
# (400-599; default 500/502/503/504). The "429" entry below also retries
# rate-limited responses. Retryable 4xx codes trigger failover only and
# never mark a provider unhealthy; other 4xx responses return verbatim.
# Direct openai/gpt-4.1 requests never fail over.
listener "http" "public" {
  address = ":8080"
}

auth "main" {
  mode = "bearer_static"

  client "internal-app" {
    token          = "test-placeholder-token" # pragma: allowlist secret
    allowed_models = ["alias/chat_default", "alias/chat_fallback", "openai/gpt-4.1"]
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
  algorithm          = "round_robin"
  retry_status_codes = ["429", "500", "502", "503", "504"]

  target {
    provider = "openai"
    model    = "gpt-4.1"
  }

  target {
    provider = "localai"
    model    = "qwen3-32b"
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
