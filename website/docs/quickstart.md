---
sidebar_position: 2
---

# Quickstart

This quickstart brings up `aiproxy` locally with one OpenAI model and no inbound auth.

## Before You Start

You need:

- an `OPENAI_API_KEY`
- either an installed `aiproxy` binary or a local checkout of the repository

## Install

Install via `asdf`:

```sh
asdf plugin add aiproxy
# or
asdf plugin add aiproxy https://github.com/egose/aiproxy.git

asdf install aiproxy latest
asdf global aiproxy latest
```

Or run directly from source:

```sh
make build
./dist/aiproxy version
```

## Minimal Config

Create `config.hcl`:

```hcl
listener "http" "public" {
  address = ":8080"
}

auth "main" {
  mode = "none"
}

provider "openai" "openai" {
  display_name = "OpenAI"
  api_key      = env("OPENAI_API_KEY")

  model "gpt-4o-mini" {
    display_name = "GPT-4o mini"
    capabilities = ["chat", "responses"]
  }
}
```

## Validate And Run

Export any environment variables referenced by `env("...")` before starting the proxy:

```sh
export OPENAI_API_KEY=sk-...
```

If you keep secrets in a local `.env` file, load them before running the CLI:

```sh
set -a; . ./.env; set +a
```

Validate the configuration:

```sh
aiproxy validate --config ./config.hcl
```

Start the server:

```sh
aiproxy serve --config ./config.hcl
```

From source, the equivalent command is:

```sh
go run ./cmd/aiproxy serve --config ./config.hcl
```

## Send A Request

```sh
curl http://127.0.0.1:8080/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "openai/gpt-4o-mini",
    "messages": [
      {"role": "user", "content": "Say hello in one sentence."}
    ]
  }'
```

You can also inspect the exposed model catalog:

```sh
curl http://127.0.0.1:8080/v1/models
```

## Add Static Bearer Auth

For anything beyond a trusted local environment, switch from `mode = "none"` to `bearer_static`:

```hcl
auth "main" {
  mode = "bearer_static"

  client "local-dev" {
    token = env("AIPROXY_CLIENT_TOKEN")
  }
}
```

Requests then need an `Authorization` header:

```sh
curl http://127.0.0.1:8080/v1/models \
  -H 'Authorization: Bearer your-token'
```

That same token must be sent to every protected endpoint, including chat, responses, embeddings, and model discovery.

## Next Steps

- Add more providers in [Configuration](./configuration.md)
- Expose alias-backed models in [Providers and Routing](./providers-and-routing.md)
- Check the endpoint and provider matrix in [API Reference](./api-reference.md)
