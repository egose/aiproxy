# Chat pool spanning translated and pass-through providers with
# tenant-aware auth and a per-client rate limit. For the hardened
# full-stack variant see production.hcl; for allow-list isolation with a
# standby provider see tenant-isolation.hcl.
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
    token          = "test-placeholder-token-a" # pragma: allowlist secret
    tenant         = "team-a"
    allowed_models = ["alias/chat_default", "alias/chat_fallback"]
  }

  client "team-b-app" {
    token          = "test-placeholder-token-b" # pragma: allowlist secret
    tenant         = "team-b"
    allowed_models = ["alias/chat_default"]
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

provider "gemini" "gemini" {
  display_name = "Gemini"
  api_key      = "test-placeholder-key" # pragma: allowlist secret

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
