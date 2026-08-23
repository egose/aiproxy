---
sidebar_position: 4
---

# Providers and Routing

`aiproxy` separates the client-facing model name from the concrete upstream target.

That separation is what allows the proxy to expose a stable public catalog while still changing providers, upstream identifiers, or pool composition over time.

## Public Model Names

Clients use one of two forms:

- `<provider-name>/<model-name>` for direct routing
- `alias/<alias-name>` for proxy-managed routing

Provider and alias names are lowercase and must not contain spaces or `/`.
Model names are lowercase, must not contain spaces, and may contain `/` when
every slash-separated segment follows the same lowercase name rule.

## Direct Routing

Direct requests resolve to one configured provider/model pair and do not fail over.

Use direct routing when the client intentionally wants a specific upstream model.

Examples:

- `openai/gpt-4.1`
- `localai/qwen3-32b`

## Alias Routing

Aliases expose a virtual model name backed by one or more concrete targets.

```hcl
alias "chat_default" {
  algorithm = "round_robin"

  target {
    provider = "openai"
    model    = "gpt-4o-mini"
  }

  target {
    provider = "anthropic"
    model    = "claude-sonnet"
  }
}
```

Aliases are useful when you want:

- simple failover
- pool-based routing
- a stable client-facing model name while changing upstream inventory

Examples:

- `alias/chat_default`
- `alias/chat_fallback`

## Alias Algorithms

- `round_robin`: rotates through targets in process-local order
- `least_connections`: picks the target with the fewest in-flight requests in the current process

`least_connections` is best-effort and not coordinated across instances.

If you run multiple proxy instances, each instance makes its own routing decision locally.

## Failover Rules

Alias requests retry the next target when the selected target fails with:

- transport errors
- timeouts
- upstream responses whose status code is listed in `retry_status_codes`

By default `retry_status_codes` is `["500", "502", "503", "504"]`, so only those common `5xx` responses trigger status-based failover. Add codes like `"429"` to also retry on rate-limited responses.

Alias requests do not fail over on other upstream `4xx` responses. Those are returned to the client as-is.

This avoids masking client-side request problems as routing problems.

Retryable `4xx` statuses are an alias failover policy only. They do not mark the provider unhealthy; provider health is mutated by transport/upstream request errors and upstream `5xx` responses.

## Provider Types

| Provider type       | Behavior                           | Notes                                    |
| ------------------- | ---------------------------------- | ---------------------------------------- |
| `openai`            | Pass-through OpenAI adapter        | Sends OpenAI-style requests upstream     |
| `openai-compatible` | Pass-through compatible adapter    | Requires `base_url`                      |
| `anthropic`         | Translated provider-native adapter | Supports chat and responses              |
| `gemini`            | Translated provider-native adapter | Supports chat, responses, and embeddings |

For `openai` and `openai-compatible`, the proxy stays close to pass-through behavior. For translated providers, the proxy maps between the public OpenAI-style contract and the provider-native request and response shape.

Pass-through providers preserve request JSON values and unknown extension fields, rewriting only the top-level `model` value before forwarding. Malformed JSON, non-object JSON bodies, and duplicate top-level `model` keys are rejected.

Translated providers intentionally support a conservative OpenAI-style request subset. Unsupported top-level controls such as `tools`, `tool_choice`, `response_format`, `logprobs`, `parallel_tool_calls`, and unknown extension fields are rejected with `invalid_request` instead of being silently dropped.

Translated chat completions support these top-level request fields: `model`, `messages`, `max_tokens`, `temperature`, `top_p`, and `stream`. Message roles are limited to `system`, `user`, and `assistant`, and content may be text or arrays of text parts.

Translated responses support these top-level request fields: `model`, `input`, `instructions`, `max_output_tokens`, `temperature`, `top_p`, and `stream`. Input may be a string or an array of message items with text content.

Gemini translated embeddings support these top-level request fields: `model`, `input`, and `dimensions`. Input may be a string or an array of strings.

## Model Capabilities

Capabilities describe which proxy operations a model may serve.

Supported values:

- `chat`
- `responses`
- `embeddings`
- `images`
- `audio_transcriptions`
- `audio_speech`

If `capabilities` is omitted, the proxy derives defaults from the provider type and then enforces operation support at request time.

<!-- docs-contract:capability-matrix:start -->

| Provider type       | Default capabilities when omitted | Additional supported capabilities                |
| ------------------- | --------------------------------- | ------------------------------------------------ |
| `openai`            | `chat`, `responses`, `embeddings` | `images`, `audio_transcriptions`, `audio_speech` |
| `openai-compatible` | `chat`, `responses`, `embeddings` | `images`, `audio_transcriptions`, `audio_speech` |
| `anthropic`         | `chat`, `responses`               | None                                             |
| `gemini`            | `chat`, `responses`               | `embeddings`                                     |

<!-- docs-contract:capability-matrix:end -->

Set explicit capabilities when you want the public catalog to reflect a narrower contract than the provider's default behavior, or to opt into one of the additional supported capabilities for that provider type.

## `GET /v1/models` Metadata

The model catalog includes both direct models and aliases.

For direct models, the response includes metadata such as:

- `display_name`
- `provider_type`
- effective `capabilities`

For aliases, the response includes:

- effective `capabilities`
- `alias_targets` summaries with provider, model, and resolved display name

Alias capabilities are the intersection of every target's capabilities. That means an alias only advertises operations that all of its targets can safely serve.
