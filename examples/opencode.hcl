# Combined Zen-plus-Go setup with an alias spanning both services.
# Direct requests never cross services; only the explicit alias below may
# retry across them. Single-service variants: opencode-zen.hcl, opencode-go.hcl.
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
  api_key = "test-placeholder-key" # pragma: allowlist secret

  model "glm-5.3" {
    protocol     = "chat"
    capabilities = ["chat"]
  }

  model "claude-sonnet-5" {
    protocol = "messages"
  }
}

provider "opencode-go" "go" {
  # base_url is omitted: the opencode-go type defaults to
  # https://opencode.ai/zen/go/v1. An optional base_url override is a
  # transport override only and never changes service selection.
  api_key = "test-placeholder-key" # pragma: allowlist secret

  model "minimax-m3" {
    protocol = "messages"
  }

  model "glm-5.3" {
    protocol = "chat"
  }
}

alias "chat_fallback" {
  algorithm = "round_robin"

  target {
    provider = "zen"
    model    = "glm-5.3"
  }

  target {
    provider = "go"
    model    = "glm-5.3"
  }
}
