# ClientHello profiles only: HTTP headers/stack and WAF acceptance are not matched.
# Endpoints and model names are illustrative; validation makes no provider calls.
# The custom reference is relative to this config's directory (examples/).
# Copy the recipe alongside your config when relocating this example.
listener "http" "public" {
  address = "127.0.0.1:8080"
}

auth "main" {
  mode = "none"
}

upstream_header_timeout = "120s"

provider "openai-compatible" "browser" {
  base_url         = "https://api.example.test/v1"
  api_key          = env("UPSTREAM_API_KEY")
  tls_client_hello = "chrome-133"

  model "example-model" {
    capabilities = ["chat", "responses"]
  }
}

# Measured Bun 1.4.4 fetch, Linux x64, fresh h1-only ClientHello; not auto-detected.
# Metadata stays separate and is never loaded as part of the recipe.
provider "openai-compatible" "measured-bun" {
  base_url                  = "https://api.example.test/v1"
  api_key                   = env("UPSTREAM_API_KEY")
  tls_client_hello           = "custom"
  tls_client_hello_spec_file = "tls/bun-1.4.4-fetch.json"

  model "example-model" {
    capabilities = ["chat", "responses"]
  }
}

# Static derivatives inherit the endpoint and both TLS fields, never local TLS flags.
provider "openai-compatible" "measured-bun-backup" {
  extends = "measured-bun"
  api_key = env("BACKUP_UPSTREAM_API_KEY")
}

# Only explicit alias routing can select another provider; profiles never cycle.
alias "profile-example" {
  algorithm = "round_robin"

  target {
    provider = "browser"
    model    = "example-model"
  }

  target {
    provider = "measured-bun-backup"
    model    = "example-model"
  }
}
