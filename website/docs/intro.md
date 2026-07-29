---
sidebar_position: 1
---

# aiproxy

`aiproxy` is a Go service that puts multiple AI providers behind one OpenAI-compatible HTTP API.

Clients send standard OpenAI-style requests to the proxy, and the proxy resolves the requested model to a configured provider-backed model or alias target. When needed, it translates the request and response for provider-native APIs such as Anthropic and Gemini.

## Why It Exists

`aiproxy` is useful when you want one stable client integration while your upstream AI inventory is split across providers, self-hosted gateways, or model pools.

It lets the proxy own:

- the public model catalog clients see
- inbound authentication and model allow-lists
- alias-based balancing and failover
- provider credential handling
- observability and health state

## What It Does

- Exposes a single OpenAI-compatible API surface to clients
- Routes requests to multiple upstream providers
- Supports direct model addressing and alias-based routing
- Retries alias targets on transport failures and upstream `5xx` responses
- Preserves streaming responses through OpenAI-compatible SSE framing
- Keeps auth, provider credentials, routing, and observability in one service

## Supported Provider Types

- `openai`
- `openai-compatible`
- `anthropic`
- `gemini`

## Supported Public API

- `GET /v1/models`
- `GET /v1/billing/usage`
- `GET /metrics`
- `POST /v1/chat/completions`
- `POST /v1/embeddings`
- `POST /v1/responses`
- `POST /v1/images/generations`
- `POST /v1/audio/transcriptions`
- `POST /v1/audio/speech`

Provider support varies by operation. See [API Reference](./api-reference.md) for the exact matrix.

## How Clients Address Models

The proxy exposes model names in two forms:

- Direct provider-backed models: `<provider-name>/<model-name>`
- Alias-backed virtual models: `alias/<alias-name>`

Examples:

- `openai/gpt-4o-mini`
- `gemini/gemini-2.5-pro`
- `alias/chat_default`

## Typical Flow

1. A client sends an OpenAI-compatible request to `aiproxy`.
2. The proxy authenticates the caller if auth is enabled.
3. The proxy resolves the requested model string to a direct model or an alias target.
4. The proxy forwards the request upstream, translating it when the provider is not OpenAI-compatible.
5. The proxy returns an OpenAI-compatible response to the client.

## Documentation Map

- [Introduction](./intro.md) for the high-level model and terminology
- [Quickstart](./quickstart.md) for local setup and a first request
- [Configuration](./configuration.md) for HCL structure, auth, and secrets
- [Config Examples](./config-examples.md) for complete deployment-oriented HCL examples
- [Providers and Routing](./providers-and-routing.md) for provider types, capabilities, aliases, and failover
- [Request Examples](./request-examples.md) for common OpenAI-compatible calls through the proxy
- [API Reference](./api-reference.md) for endpoint coverage and behavior
- [Operations](./operations.md) for build, run, reload, Docker, and tests
- [Deployment](./deployment.md) for Docker, Compose, systemd, and rollout guidance

## Design Notes

This site focuses on practical usage. For deeper implementation rationale and design details, see the full design document in the repository:

- [github.com/egose/aiproxy/blob/main/docs/design.md](https://github.com/egose/aiproxy/blob/main/docs/design.md)
