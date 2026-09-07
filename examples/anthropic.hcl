# Standalone Anthropic setup through the translated provider-native adapter.
# Chat and responses are translated; embeddings/images/audio are unsupported
# on this provider type. Public models: anthropic/claude-sonnet,
# anthropic/claude-haiku, alias/anthropic_chat.
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

provider "anthropic" "anthropic" {
  display_name = "Anthropic"
  api_key      = "test-placeholder-key" # pragma: allowlist secret

  model "claude-sonnet" {
    display_name  = "Claude Sonnet"
    upstream_name = "claude-sonnet-4-20250514"
    capabilities  = ["chat", "responses"]
  }

  model "claude-haiku" {
    display_name  = "Claude Haiku"
    upstream_name = "claude-haiku-4-20250514"
    capabilities  = ["chat", "responses"]
  }
}

alias "anthropic_chat" {
  algorithm = "round_robin"

  target {
    provider = "anthropic"
    model    = "claude-sonnet"
  }

  target {
    provider = "anthropic"
    model    = "claude-haiku"
  }
}
