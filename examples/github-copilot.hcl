# GitHub Copilot chat-only setup backed by a device-flow login.
# Provision first with your own public OAuth client ID (no secret):
#   aiproxy login github-copilot --client-id YOUR_GITHUB_OAUTH_CLIENT_ID --credential copilot-main
# Never reuse another application's client ID. The login writes
# <secrets-dir>/copilot-<name>.json (0600) without editing this file.
# Restart or SIGHUP after login; this config validates only after the sidecar exists.
# Re-run login + reload on upstream 401/403, revocation, or expiry.
listener "http" "public" {
  address = ":8080"
}

auth "main" {
  mode = "none"
}

provider "github-copilot" "copilot" {
  # base_url is omitted: github-copilot defaults to
  # https://api.githubcopilot.com. An optional base_url override is a
  # transport override only and never changes auth or header behavior.
  # base_url = "http://127.0.0.1:18081"
  credential_ref {
    name = "copilot-main"
  }

  model "gpt-5.4-nano" {
    display_name = "Copilot Nano"
  }
}

alias "copilot_chat" {
  algorithm = "round_robin"

  target {
    provider = "copilot"
    model    = "gpt-5.4-nano"
  }
}
