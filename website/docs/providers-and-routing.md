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

Names are lowercase and must not contain spaces or `/`.

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

Alias requests retry the next target only when the selected target fails with:

- transport errors
- timeouts
- upstream `5xx` responses

Alias requests do not fail over on upstream `4xx` responses. Those are returned to the client as-is.

This avoids masking client-side request problems as routing problems.

## Provider Types

| Provider type       | Behavior                           | Notes                                    |
| ------------------- | ---------------------------------- | ---------------------------------------- |
| `openai`            | Pass-through OpenAI adapter        | Sends OpenAI-style requests upstream     |
| `openai-compatible` | Pass-through compatible adapter    | Requires `base_url`                      |
| `anthropic`         | Translated provider-native adapter | Supports chat and responses              |
| `gemini`            | Translated provider-native adapter | Supports chat, responses, and embeddings |

For `openai` and `openai-compatible`, the proxy stays close to pass-through behavior. For translated providers, the proxy maps between the public OpenAI-style contract and the provider-native request and response shape.

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

Set explicit capabilities when you want the public catalog to reflect a narrower, safer contract than the provider's default behavior.

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
