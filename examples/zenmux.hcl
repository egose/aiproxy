# Standalone ZenMux gateway setup (OpenAI pass-through).
# ZenMux speaks OpenAI chat and responses natively; the proxy rewrites only
# the top-level model field and forwards the body (including SSE streams).
listener "http" "public" {
  address = ":8080"
}

auth "main" {
  mode = "none"
}

provider "zenmux" "zenmux" {
  # base_url is omitted: the zenmux type defaults to
  # https://zenmux.ai/api/v1. An optional base_url override is a
  # transport override only.
  # base_url = "http://127.0.0.1:18081"
  api_key = "test-placeholder-key" # pragma: allowlist secret

  model "qwen3-max" {
    upstream_name = "qwen/qwen3-max"
  }

  model "gemini-3.1-pro" {
    upstream_name = "google/gemini-3.1-pro-preview"
    capabilities  = ["chat", "responses"]
  }
}

alias "zenmux_chat" {
  algorithm = "round_robin"

  target {
    provider = "zenmux"
    model    = "qwen3-max"
  }

  target {
    provider = "zenmux"
    model    = "gemini-3.1-pro"
  }
}
