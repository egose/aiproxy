# Minimal observability/health fragment: metrics, dashboard, and
# Redis-shared provider health on top of a single provider. For the full
# hardened stack (auth, timeouts, aliasing) see production.hcl.
listener "http" "public" {
  address = ":8080"
}

auth "main" {
  mode = "none"
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
}

provider "openai" "openai" {
  display_name = "OpenAI"
  api_key      = "test-placeholder-key" # pragma: allowlist secret

  model "gpt-4.1" {
    display_name = "GPT-4.1"
    capabilities = ["chat", "responses"]
  }
}
