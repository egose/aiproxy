# Environment-substituted variant of the fallback setup. env("VAR") values
# are inlined before HCL parsing, so load them first:
# set -a; . ./.env; set +a
listener "http" "public" {
  address = ":8080"
}

auth "main" {
  mode = "bearer_static"

  client "internal-app" {
    token          = env("AIPROXY_CLIENT_TOKEN")
    allowed_models = ["alias/chat_default", "openai/gpt-4.1"]
  }
}

provider "openai" "openai" {
  display_name = "OpenAI"
  api_key      = env("OPENAI_API_KEY")

  model "gpt-4.1" {
    display_name = "GPT-4.1"
    capabilities = ["chat", "responses"]
  }
}

provider "openai-compatible" "localai" {
  display_name = "LocalAI"
  base_url     = "https://llm.internal/v1"

  api_key_ref {
    key = "localai"
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
