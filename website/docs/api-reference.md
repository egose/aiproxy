---
sidebar_position: 5
---

# API Reference

`aiproxy` exposes an OpenAI-compatible HTTP API.

This page focuses on the proxy-facing contract and operation coverage. It does not attempt to restate every upstream provider-specific field or option.

## Endpoints

| Endpoint                   | Method | Notes                                                                  |
| -------------------------- | ------ | ---------------------------------------------------------------------- |
| `/v1/models`               | `GET`  | Lists direct models and aliases                                        |
| `/v1/billing/usage`        | `GET`  | Returns aggregated in-process usage summaries                          |
| `/metrics`                 | `GET`  | Prometheus metrics                                                     |
| `/v1/chat/completions`     | `POST` | JSON and SSE streaming                                                 |
| `/v1/embeddings`           | `POST` | Supported for `openai`, `openai-compatible`, and `gemini`              |
| `/v1/responses`            | `POST` | Supported for `openai`, `openai-compatible`, `anthropic`, and `gemini` |
| `/v1/images/generations`   | `POST` | Supported for `openai` and `openai-compatible`                         |
| `/v1/audio/transcriptions` | `POST` | Supported for `openai` and `openai-compatible`                         |
| `/v1/audio/speech`         | `POST` | Supported for `openai` and `openai-compatible`                         |

## Provider Support Matrix

| Operation                       | `openai` | `openai-compatible` | `anthropic` | `gemini` |
| ------------------------------- | -------- | ------------------- | ----------- | -------- |
| `GET /v1/models`                | Yes      | Yes                 | Yes         | Yes      |
| `GET /v1/billing/usage`         | Yes      | Yes                 | Yes         | Yes      |
| `POST /v1/chat/completions`     | Yes      | Yes                 | Yes         | Yes      |
| `POST /v1/embeddings`           | Yes      | Yes                 | No          | Yes      |
| `POST /v1/responses`            | Yes      | Yes                 | Yes         | Yes      |
| `POST /v1/images/generations`   | Yes      | Yes                 | No          | No       |
| `POST /v1/audio/transcriptions` | Yes      | Yes                 | No          | No       |
| `POST /v1/audio/speech`         | Yes      | Yes                 | No          | No       |

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

`GET /v1/billing/usage` returns aggregated in-process usage summaries.

- When the authenticated client has a `tenant`, results are scoped to that tenant
- Otherwise, results are scoped to the caller's client identity

## Error Behavior

- Direct requests never fail over to another provider
- Alias requests retry the next target on transport errors and upstream `5xx`
- Alias requests also retry on upstream `4xx` responses listed in `retry_status_codes` (default: `5xx` only)
- Other upstream `4xx` responses are returned verbatim
- Unsupported operations return client-visible proxy errors

This behavior is deliberate: direct model requests are explicit, while alias requests are the only place where the proxy is allowed to choose another target.

## Provider Coverage Notes

- `openai` and `openai-compatible` are close to pass-through adapters
- `anthropic` and `gemini` use request and response translation
- translated `/v1/responses` support is intentionally conservative compared with the full upstream provider-native feature set
