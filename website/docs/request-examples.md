---
sidebar_position: 6
---

# Request Examples

These examples show how clients typically call `aiproxy` through its OpenAI-compatible API.

All request bodies use proxy-visible model names such as `openai/gpt-4.1` or `alias/chat_default`.

## Common Headers

With `bearer_static` auth enabled:

```http
Authorization: Bearer your-token
Content-Type: application/json
```

With `none` auth enabled, omit the `Authorization` header.

## List Models

```sh
curl http://127.0.0.1:8080/v1/models \
  -H 'Authorization: Bearer your-token'
```

Representative response shape:

```json
{
  "object": "list",
  "data": [
    {
      "id": "openai/gpt-4.1",
      "object": "model",
      "display_name": "GPT-4.1",
      "provider_type": "openai",
      "capabilities": ["chat", "responses"]
    },
    {
      "id": "alias/chat_default",
      "object": "model",
      "capabilities": ["chat", "responses"],
      "alias_targets": [
        { "provider": "openai", "model": "gpt-4.1", "display_name": "GPT-4.1" },
        { "provider": "localai", "model": "qwen3-32b", "display_name": "Qwen 3 32B" }
      ]
    }
  ]
}
```

## Chat Completions

Request:

```sh
curl http://127.0.0.1:8080/v1/chat/completions \
  -H 'Authorization: Bearer your-token' \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "alias/chat_default",
    "messages": [
      {"role": "system", "content": "Be concise."},
      {"role": "user", "content": "Give me three rollout checks for a new proxy deployment."}
    ]
  }'
```

Representative response shape:

```json
{
  "id": "chatcmpl-123",
  "object": "chat.completion",
  "model": "alias/chat_default",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "1. Verify /v1/models and /metrics respond. 2. Test one direct model and one alias. 3. Confirm upstream failover and auth behavior."
      },
      "finish_reason": "stop"
    }
  ]
}
```

## Streaming Chat Completions

```sh
curl http://127.0.0.1:8080/v1/chat/completions \
  -H 'Authorization: Bearer your-token' \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "alias/chat_default",
    "stream": true,
    "messages": [
      {"role": "user", "content": "Write a short status update."}
    ]
  }'
```

The response is an OpenAI-compatible SSE stream. Translated providers are converted back into the same public streaming format.

## Responses API

```sh
curl http://127.0.0.1:8080/v1/responses \
  -H 'Authorization: Bearer your-token' \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "openai/gpt-4.1",
    "input": "Summarize the purpose of an alias-backed model in one paragraph."
  }'
```

Use this endpoint only with provider/model combinations that support `responses` through the proxy.

## Embeddings

Supported for `openai`, `openai-compatible`, and `gemini` providers.

```sh
curl http://127.0.0.1:8080/v1/embeddings \
  -H 'Authorization: Bearer your-token' \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "openai/text-embedding-3-large",
    "input": [
      "first document",
      "second document"
    ]
  }'
```

## Image Generation

Supported only for `openai` and `openai-compatible` providers.

```sh
curl http://127.0.0.1:8080/v1/images/generations \
  -H 'Authorization: Bearer your-token' \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "openai/gpt-image-1",
    "prompt": "A blueprint-style illustration of a service mesh"
  }'
```

Only expose image-capable models through the proxy if you actually want clients to use that operation.

## Audio Endpoints

`/v1/audio/transcriptions` and `/v1/audio/speech` are supported only for `openai` and `openai-compatible` providers.

These endpoints usually involve multipart upload or binary output handling on the client side, but the proxy-visible model naming and auth behavior stay the same.

## Billing Usage

```sh
curl http://127.0.0.1:8080/v1/billing/usage \
  -H 'Authorization: Bearer your-token'
```

In `bearer_static` mode, results are scoped to the caller's tenant when present, otherwise to the caller's client identity.

## Error Behavior Examples

- direct request to `openai/gpt-4.1`: never rerouted to another provider
- alias request to `alias/chat_default`: may retry the next target on timeout or upstream `5xx`
- alias request returning upstream `4xx`: returned to the client without failover
- request to an unsupported operation: returned as a client-visible proxy error
