# Standalone OpenRouter gateway setup (OpenAI pass-through).
# OpenRouter speaks OpenAI chat and responses natively; the proxy rewrites only
# the top-level model field and forwards the body (including SSE streams).
# Every upstream request additionally sends OpenRouter attribution headers
# (HTTP-Referer and X-Title).
listener "http" "public" {
  address = ":8080"
}

auth "main" {
  mode = "none"
}

provider "openrouter" "openrouter" {
  # base_url is omitted: the openrouter type defaults to
  # https://openrouter.ai/api/v1. An optional base_url override is a
  # transport override only.
  # base_url = "http://127.0.0.1:18081"
  api_key = "test-placeholder-key" # pragma: allowlist secret

  model "gpt-4o-mini" {
    upstream_name = "openai/gpt-4o-mini"
  }

  model "claude-sonnet" {
    upstream_name = "anthropic/claude-sonnet-4"
    capabilities  = ["chat", "responses"]
  }
}

alias "openrouter_chat" {
  algorithm = "round_robin"

  target {
    provider = "openrouter"
    model    = "gpt-4o-mini"
  }

  target {
    provider = "openrouter"
    model    = "claude-sonnet"
  }
}
