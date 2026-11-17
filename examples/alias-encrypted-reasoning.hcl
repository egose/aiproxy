# Alias encrypted-reasoning handling. Clients that replay prior-turn
# thinking blocks (extended thinking / reasoning passthrough) send
# caller-bound opaque blobs (encrypted_content, signatures, redacted
# thinking) that the issuing model rejects when a pool routes the next
# turn to a different credential ("not issued to this caller",
# "invalid_encrypted_content", "Invalid signature").
# An explicit alias encrypted_reasoning block overrides model/provider/root
# policies. Ordinary text and tool history are preserved, but the stripping
# helper also removes typed unsigned thinking/reasoning and chat reasoning
# fields. It does not decrypt outputs; stripping can lose reasoning continuity.
# These loopback URLs are placeholders for local OpenAI-compatible stubs.
listener "http" "public" {
  address = ":8080"
}

auth "main" {
  mode = "bearer_static"

  client "internal-app" {
    token          = "test-placeholder-token" # pragma: allowlist secret
    allowed_models = ["alias/chat_default", "alias/chat_strict"]
  }
}

provider "openai-compatible" "spark-a" {
  display_name = "Spark A"
  base_url     = "http://127.0.0.1:18081/v1"
  api_key      = "test-placeholder-key" # pragma: allowlist secret

  model "muse-spark" {
    display_name = "Muse Spark"
    capabilities = ["chat", "responses"]
  }
}

provider "openai-compatible" "spark-b" {
  display_name = "Spark B"
  base_url     = "http://127.0.0.1:18082/v1"
  api_key      = "test-placeholder-key" # pragma: allowlist secret

  model "muse-spark" {
    display_name = "Muse Spark"
    capabilities = ["chat", "responses"]
  }
}

# Optimistic: try with reasoning intact. Recovery requires an ordinary
# non-streaming upstream 400, raw opaque request markers, a bounded decoded
# match, and a changed rewrite. Retry the same identity once, with one extra
# recovery call total across the alias (not one per target).
# Omitting match_messages uses the built-in defaults covering Console,
# OpenAI, and Anthropic phrasings; a non-empty list replaces them.
# Once stripped, an explicit alias keeps that body through normal failover.
alias "chat_default" {
  algorithm = "round_robin"

  encrypted_reasoning {
    passthrough        = true
    on_caller_mismatch = "strip_and_retry"
  }

  target {
    provider = "spark-a"
    model    = "muse-spark"
  }

  target {
    provider = "spark-b"
    model    = "muse-spark"
  }
}

# Pessimistic: strip supported reasoning shapes before the first attempt.
# No extra recovery call is added; ordinary alias failover still applies.
# This loses reasoning continuity even with the issuing credential.
alias "chat_strict" {
  algorithm = "round_robin"

  encrypted_reasoning {
    passthrough = false
  }

  target {
    provider = "spark-a"
    model    = "muse-spark"
  }

  target {
    provider = "spark-b"
    model    = "muse-spark"
  }
}
