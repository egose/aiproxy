---
sidebar_position: 5
---

# API Reference

`aiproxy` exposes an OpenAI-compatible HTTP API.

This page focuses on the proxy-facing contract and operation coverage. It does not attempt to restate every upstream provider-specific field or option.

## Endpoint And Provider Support Matrix

<!-- docs-contract:public-matrix:start -->

| Surface                         | `openai`                           | `openai-compatible`                | `anthropic`                        | `gemini`                           | `opencode-zen`                           | `opencode-go`                            | `github-copilot`                   |
| ------------------------------- | ---------------------------------- | ---------------------------------- | ---------------------------------- | ---------------------------------- | ---------------------------------------- | ---------------------------------------- | ---------------------------------- |
| `GET /v1/models`                | Proxy-owned                        | Proxy-owned                        | Proxy-owned                        | Proxy-owned                        | Proxy-owned                              | Proxy-owned                              | Proxy-owned                        |
| `GET /v1/billing/usage`         | Proxy-owned local usage accounting | Proxy-owned local usage accounting | Proxy-owned local usage accounting | Proxy-owned local usage accounting | Proxy-owned local usage accounting       | Proxy-owned local usage accounting       | Proxy-owned local usage accounting |
| `GET /metrics`                  | Proxy-owned Prometheus metrics     | Proxy-owned Prometheus metrics     | Proxy-owned Prometheus metrics     | Proxy-owned Prometheus metrics     | Proxy-owned Prometheus metrics           | Proxy-owned Prometheus metrics           | Proxy-owned Prometheus metrics     |
| `POST /v1/chat/completions`     | JSON and SSE                       | JSON and SSE                       | JSON and SSE translated            | JSON and SSE translated            | JSON and SSE native or translated subset | JSON and SSE native or translated subset | JSON and SSE                       |
| `POST /v1/embeddings`           | Yes                                | Yes                                | No                                 | Yes                                | No                                       | No                                       | No                                 |
| `POST /v1/responses`            | JSON and SSE                       | JSON and SSE                       | JSON and SSE translated subset     | JSON and SSE translated subset     | JSON and SSE native or translated subset | JSON and SSE native or translated subset | No                                 |
| `POST /v1/images/generations`   | Yes                                | Yes                                | No                                 | No                                 | No                                       | No                                       | No                                 |
| `POST /v1/audio/transcriptions` | Yes                                | Yes                                | No                                 | No                                 | No                                       | No                                       | No                                 |
| `POST /v1/audio/speech`         | Yes                                | Yes                                | No                                 | No                                 | No                                       | No                                       | No                                 |

<!-- docs-contract:public-matrix:end -->

## Provider Capability Defaults

<!-- docs-contract:capability-matrix:start -->

| Provider type       | Default capabilities when omitted          | Additional supported capabilities                |
| ------------------- | ------------------------------------------ | ------------------------------------------------ |
| `openai`            | `chat`, `responses`, `embeddings`          | `images`, `audio_transcriptions`, `audio_speech` |
| `openai-compatible` | `chat`, `responses`, `embeddings`          | `images`, `audio_transcriptions`, `audio_speech` |
| `anthropic`         | `chat`, `responses`                        | None                                             |
| `gemini`            | `chat`, `responses`                        | `embeddings`                                     |
| `opencode-zen`      | `chat`, `responses`, or both (by protocol) | None                                             |
| `opencode-go`       | `chat`, `responses`, or both (by protocol) | None                                             |
| `github-copilot`    | `chat`                                     | None                                             |

<!-- docs-contract:capability-matrix:end -->

When a model omits `capabilities`, the provider-type defaults are used. Explicit
capabilities can narrow that default or opt into an additional supported
capability listed above. On `opencode-zen` and `opencode-go` the omitted
default is protocol-aware: `chat` models default to `chat`, `responses` models
to `responses`, and `messages` (both services) plus `gemini` (Zen only) models
to `chat` and `responses`.

## Streaming

Streaming uses OpenAI-compatible Server-Sent Events.

- `POST /v1/chat/completions` supports JSON and SSE streaming
- `POST /v1/responses` supports JSON and SSE streaming where implemented
- translated providers map their upstream streaming format back into OpenAI-compatible SSE chunks

If an upstream stream fails after partial output, the proxy terminates the stream instead of fabricating a full JSON response.

## Authentication

When `bearer_static` auth is enabled, requests must send:

```http
Authorization: Bearer <token>
```

When `none` auth is enabled, no inbound authentication is performed.

In `bearer_static` mode, individual clients may also be restricted with `allowed_models`.

## Usage And Scope

`GET /v1/billing/usage` returns aggregated in-process usage summaries over the
proxy's rolling 24-hour accounting window.

- When the authenticated client has a `tenant`, results are scoped to that tenant
- Otherwise, results are scoped to the caller's client identity
- This endpoint is local accounting only; it is not an external billing,
  invoicing, or quota system

## Error Behavior

- Direct requests never fail over to another provider
- Alias requests retry the next target on transport errors, timeouts, and status
  codes listed in `retry_status_codes`
- The default `retry_status_codes` list is `500`, `502`, `503`, and `504`
- Configured retry statuses may include `4xx` responses such as `429`; retryable
  `4xx` statuses do not mark providers unhealthy
- Other upstream `4xx` responses are returned verbatim
- Unsupported operations return client-visible proxy errors

This behavior is deliberate: direct model requests are explicit, while alias requests are the only place where the proxy is allowed to choose another target.

## Provider Coverage Notes

- `openai` and `openai-compatible` are close to pass-through adapters
- `anthropic` and `gemini` use request and response translation
- translated `/v1/responses` support is intentionally conservative compared with the full upstream provider-native feature set
- `opencode-zen` and `opencode-go` mix native and translated handling per
  model `protocol`: `chat`/`responses` protocols are native pass-through for
  one public operation each, while `messages`/`gemini` use the conservative
  translation subsets. Operations a protocol does not serve, and
  embeddings/images/audio on both OpenCode types, are rejected before upstream
  I/O. See [Providers and Routing](providers-and-routing.md) for the protocol
  contract.
- `github-copilot` is chat-only pass-through: `POST /v1/chat/completions`
  serves JSON and SSE, and every other operation (including `responses`,
  `embeddings`, `images`, and audio) is rejected before upstream I/O.
  `GET /v1/models` and `GET /v1/billing/usage` stay proxy-owned with no
  upstream calls.
