# Smallest useful config: one OpenAI-backed model, no inbound auth.
# Use for local testing behind another trusted boundary.
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

provider "openai" "openai" {
  display_name = "OpenAI"
  api_key      = "test-placeholder-key" # pragma: allowlist secret

  model "gpt-4o-mini" {
    display_name = "GPT-4o mini"
    capabilities = ["chat", "responses"]
  }
}
