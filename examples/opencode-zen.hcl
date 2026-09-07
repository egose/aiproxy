# Standalone opencode-zen setup with explicit per-model protocols.
# Combined Zen-plus-Go setup: opencode.hcl.
listener "http" "public" {
  address = ":8080"
}

auth "main" {
  mode = "none"
}

provider "opencode-zen" "zen" {
  # base_url is omitted: the opencode-zen type defaults to
  # https://opencode.ai/zen/v1. An optional base_url override is a
  # transport override only and never changes service selection.
  # base_url = "http://127.0.0.1:18081"
  api_key = "test-placeholder-key" # pragma: allowlist secret

  model "glm-5.3" {
    protocol     = "chat"
    capabilities = ["chat"]
  }

  model "claude-sonnet-5" {
    protocol = "messages"
  }

  model "gemini-3.8-flash" {
    protocol = "gemini"
  }
}

alias "zen_chat" {
  algorithm = "round_robin"

  target {
    provider = "zen"
    model    = "glm-5.3"
  }

  target {
    provider = "zen"
    model    = "claude-sonnet-5"
  }
}
