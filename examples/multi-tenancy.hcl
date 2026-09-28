# Multi-tenancy with database-backed dynamic resources. Static providers and
# aliases below remain read-only in the admin UI/CLI (badge: config); resources
# created via the admin UI/CLI are stored in Postgres (badge: database).
#
# Provision locally with:
#   make sandbox-up
#   set -a; . sandbox/.env.example; set +a
#   go run ./cmd/aiproxy migrate up --config examples/multi-tenancy.hcl
#   go run ./cmd/aiproxy serve --config examples/multi-tenancy.hcl

listener "http" "srv" {
  address = "127.0.0.1:9090"
}

auth "main" {
  mode = "bearer_static"
  client "ci" {
    token = env("AIPROXY_CLIENT_CI_TOKEN")
  }
}

database {
  url = env("AIPROXY_DATABASE_URL")
}

multi_tenancy {
  enabled = true
}

web_ui {
  enabled = true
}

dashboard {
}

provider "openai" "openai" {
  api_key = env("OPENAI_API_KEY")
  model "gpt-4o-mini" {
  }
}

alias "fast" {
  algorithm = "round_robin"
  target {
    provider = "openai"
    model    = "gpt-4o-mini"
  }
}
