# Standalone opencode-go setup with explicit per-model protocols.
# Combined Zen-plus-Go setup: opencode.hcl.
listener "http" "public" {
  address = ":8080"
}

auth "main" {
  mode = "none"
}

provider "opencode-go" "go" {
  # base_url is omitted: the opencode-go type defaults to
  # https://opencode.ai/zen/go/v1. An optional base_url override is a
  # transport override only and never changes service selection.
  # base_url = "http://127.0.0.1:18082"
  api_key = "test-placeholder-key" # pragma: allowlist secret

  model "minimax-m3" {
    protocol = "messages"
  }

  model "glm-5.3" {
    protocol     = "chat"
    capabilities = ["chat"]
  }
}

alias "go_chat" {
  algorithm = "round_robin"

  target {
    provider = "go"
    model    = "minimax-m3"
  }

  target {
    provider = "go"
    model    = "glm-5.3"
  }
}
