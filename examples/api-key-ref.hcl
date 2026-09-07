# Secrets kept outside the HCL file via api_key_ref. The default key
# file is $XDG_CONFIG_HOME/aiproxy/keys.json (or ~/.config/aiproxy/keys.json
# when XDG_CONFIG_HOME is unset); `path` overrides it per provider.
# keys.json shape: { "openai": "sk-...", "localai": "secret" }
listener "http" "public" {
  address = ":8080"
}

auth "main" {
  mode = "none"
}

provider "openai" "openai" {
  display_name = "OpenAI"

  api_key_ref {
    key = "openai"
  }

  model "gpt-4.1" {
    display_name = "GPT-4.1"
    capabilities = ["chat", "responses"]
  }
}

provider "openai-compatible" "localai" {
  display_name = "LocalAI"
  base_url     = "https://llm.internal/v1"

  api_key_ref {
    path = "/etc/aiproxy/keys.json"
    key  = "localai"
  }

  model "qwen3-32b" {
    display_name = "Qwen 3 32B"
    capabilities = ["chat", "responses"]
  }
}

alias "chat_default" {
  algorithm = "round_robin"

  target {
    provider = "openai"
    model    = "gpt-4.1"
  }

  target {
    provider = "localai"
    model    = "qwen3-32b"
  }
}
