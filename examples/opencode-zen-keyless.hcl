# Keyless opencode-zen access. This provider declares no api_key or
# api_key_ref, so the proxy sends no Authorization header upstream.
# Useful for free-tier models served without a credential; add api_key or
# api_key_ref when the model requires one. Public model:
# zen/mimo-v2.5-free.
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

  model "mimo-v2.5-free" {
    protocol     = "chat"
    capabilities = ["chat"]
  }
}
