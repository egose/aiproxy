# Self-hosted provider with an active upstream healthcheck. The proxy polls
# `path` relative to `base_url` and feeds the result into the same provider
# health state used for alias routing and /readyz. Providers without a
# `healthcheck` block keep the default passive-only behavior. For the
# Redis-shared variant of passive health see operations.hcl.
listener "http" "public" {
  address = ":8080"
}

auth "main" {
  mode = "none"
}

provider "openai-compatible" "local" {
  base_url = "http://127.0.0.1:11434/v1"
  api_key  = "test-placeholder-key" # pragma: allowlist secret

  healthcheck {
    path                  = "/health"
    method                = "GET"
    expected_status       = 200
    expected_body         = "*"
    interval              = "30s"
    timeout               = "5s"
    failure_threshold     = 2
    success_threshold     = 1
    send_authorization    = false
  }

  model "qwen3-32b" {
    capabilities = ["chat"]
  }
}

alias "chat_default" {
  algorithm = "round_robin"

  target {
    provider = "local"
    model    = "qwen3-32b"
  }
}
