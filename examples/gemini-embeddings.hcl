# Gemini chat plus embeddings. The embeddings model opts into the
# provider's additional `embeddings` capability and is called directly as
# gemini/gemini-embedding-001; only the chat-capable models sit behind
# alias/gemini_chat, since alias targets must share at least one capability.
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

provider "gemini" "gemini" {
  display_name = "Gemini"
  api_key      = "test-placeholder-key" # pragma: allowlist secret

  model "gemini-2.5-pro" {
    display_name = "Gemini 2.5 Pro"
    capabilities = ["chat", "responses"]
  }

  model "gemini-2.5-flash" {
    display_name = "Gemini 2.5 Flash"
    capabilities = ["chat", "responses"]
  }

  model "gemini-embedding-001" {
    display_name = "Gemini Embedding"
    capabilities = ["embeddings"]
  }
}

alias "gemini_chat" {
  algorithm = "round_robin"

  target {
    provider = "gemini"
    model    = "gemini-2.5-pro"
  }

  target {
    provider = "gemini"
    model    = "gemini-2.5-flash"
  }
}
