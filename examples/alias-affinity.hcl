# Alias session affinity. The session_affinity block pins a session to a
# stable pool target for upstream prompt-cache locality. The first non-empty
# header in the precedence list selects the session key; affinity is a hint,
# so cooling, unhealthy, or already-tried targets still fall back to normal
# algorithm selection with the usual retry failover. Omitting headers uses the
# built-in defaults covering opencode, Claude Code, and Codex session headers;
# omitting the block disables affinity entirely.
listener "http" "public" {
  address = ":8080"
}

auth "main" {
  mode = "bearer_static"

  client "internal-app" {
    token          = "test-placeholder-token" # pragma: allowlist secret
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

provider "openai-compatible" "localai" {
  display_name = "LocalAI"
  base_url     = "https://llm.internal/v1"
  api_key      = "test-placeholder-key" # pragma: allowlist secret

  model "qwen3-32b" {
    display_name = "Qwen 3 32B"
    capabilities = ["chat", "responses"]
  }
}

alias "chat_default" {
  algorithm = "round_robin"

  session_affinity {
    headers = ["x-opencode-session", "x-claude-code-session-id", "session-id"]
  }

  target {
    provider = "openai"
    model    = "gpt-4.1"
  }

  target {
    provider = "localai"
    model    = "qwen3-32b"
  }
}
