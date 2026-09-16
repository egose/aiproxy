listener "http" "public" {
  address = ":8080"
}

auth "main" {
  mode = "bearer_static"

  client "local-dev" {
    token = env("AIPROXY_CLIENT_TOKEN")
  }
}

metrics {
  token = env("AIPROXY_METRICS_TOKEN")
}

ingress_guardrails {
  enabled        = true
  mode           = "block"
  max_text_bytes = 65536
  max_strings    = 512

  # Durable per-secret triage decisions (survives restart and quarantine
  # TTL). Only SHA-256 fingerprints and actions are stored, never secrets.
  # Defaults to $XDG_CONFIG_HOME/aiproxy/guardrail-exceptions.json.
  # exceptions_file    = "/etc/aiproxy/guardrail-exceptions.json"
  # redact_placeholder = "REDACTED"

  # Optional in-memory triage for blocked requests. Disabled by default;
  # when enabled, matched snippets are kept in process memory (never on
  # disk) and readable once per block_id through the dashboard-gated
  # blocks endpoints / `4:Blocks` TUI tab.
  # quarantine {
  #   enabled           = true
  #   max_entries       = 128
  #   ttl               = "15m"
  #   max_snippet_bytes = 512
  # }
}

provider "openai" "openai" {
  api_key = env("OPENAI_API_KEY")

  model "gpt-4o-mini" {}
}

alias "default" {
  algorithm = "round_robin"

  target {
    provider = "openai"
    model    = "gpt-4o-mini"
  }
}
