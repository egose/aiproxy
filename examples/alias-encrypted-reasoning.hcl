# Alias encrypted-reasoning handling. Clients that replay prior-turn
# thinking blocks (extended thinking / reasoning passthrough) send
# caller-bound opaque blobs (encrypted_content, signatures, redacted
# thinking) that the issuing model rejects when a pool routes the next
# turn to a different credential ("not issued to this caller",
# "invalid_encrypted_content", "Invalid signature").
# The encrypted_reasoning block controls this per alias. Plaintext
# reasoning summaries without opaque blobs are never stripped or matched.
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
  base_url     = "https://spark.internal/v1"
  api_key      = "test-placeholder-key" # pragma: allowlist secret

  model "muse-spark" {
    display_name = "Muse Spark"
    capabilities = ["chat", "responses"]
  }
}

provider "openai-compatible" "spark-b" {
  display_name = "Spark B"
  base_url     = "https://spark.internal/v1"
  api_key      = "test-placeholder-key" # pragma: allowlist secret

  model "muse-spark" {
    display_name = "Muse Spark"
    capabilities = ["chat", "responses"]
  }
}

# Optimistic: try with reasoning intact, strip opaque blocks and retry
# the same target once when the upstream 400 matches match_messages.
# Omitting match_messages uses the built-in defaults covering Console,
# OpenAI, and Anthropic phrasings; a non-empty list replaces them.
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

# Pessimistic: always strip opaque reasoning blocks before fan-out.
# One upstream call, deterministic, but loses reasoning continuity even
# when the pool would have hit the issuing caller.
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
