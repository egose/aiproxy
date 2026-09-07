# Production-oriented stack: tenant-aware auth with a per-client rate
# limit, observability (metrics + dashboard), Redis-shared provider health,
# and layered upstream header timeouts (provider overrides root, root
# overrides the 90s default). Auth, providers, aliases, timeouts, metrics,
# and provider-health reload on SIGHUP; listener, logging level, and
# enabling the dashboard after startup require a restart.
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
    requests_per_minute = 240
    burst               = 240
  }

  client "team-a-app" {
    token          = "test-placeholder-token-a" # pragma: allowlist secret
    tenant         = "team-a"
    allowed_models = ["alias/chat_default", "openai/gpt-4.1"]
  }

  client "team-b-app" {
    token          = "test-placeholder-token-b" # pragma: allowlist secret
    tenant         = "team-b"
    allowed_models = ["alias/chat_default"]
  }
}

logging {
  level      = "info"
  access_log = true
}

metrics {
  token = "test-placeholder-metrics-token" # pragma: allowlist secret
}

dashboard {
  token = "test-placeholder-dashboard-token" # pragma: allowlist secret
}

provider_health {
  redis_url  = "redis://127.0.0.1:6379"
  key_prefix = "aiproxy:prod"
  cooldown   = "45s"
  cache_ttl  = "30s"
}

provider "openai" "openai" {
  display_name            = "OpenAI"
  api_key                 = "test-placeholder-key" # pragma: allowlist secret
  upstream_header_timeout = "180s"

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

alias "chat_default" {
  algorithm          = "round_robin"
  retry_status_codes = ["429", "500", "502", "503", "504"]

  target {
    provider = "openai"
    model    = "gpt-4.1"
  }

  target {
    provider = "anthropic"
    model    = "claude-sonnet"
  }
}
