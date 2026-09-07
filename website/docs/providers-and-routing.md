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

| Provider type       | Behavior                           | Notes                                                   |
| ------------------- | ---------------------------------- | ------------------------------------------------------- |
| `openai`            | Pass-through OpenAI adapter        | Sends OpenAI-style requests upstream                    |
| `openai-compatible` | Pass-through compatible adapter    | Requires `base_url`                                     |
| `anthropic`         | Translated provider-native adapter | Supports chat and responses                             |
| `gemini`            | Translated provider-native adapter | Supports chat, responses, and embeddings                |
| `opencode-zen`      | Native or translated, per protocol | Requires per-model `protocol`; Zen service              |
| `opencode-go`       | Native or translated, per protocol | Requires per-model `protocol`; Go service               |
| `github-copilot`    | Pass-through chat-only adapter     | Device-flow login; `credential_ref`; chat JSON/SSE only |

For `openai` and `openai-compatible`, the proxy stays close to pass-through behavior. For translated providers, the proxy maps between the public OpenAI-style contract and the provider-native request and response shape.

Pass-through providers preserve request JSON values and unknown extension fields, rewriting only the top-level `model` value before forwarding. Malformed JSON, non-object JSON bodies, and duplicate top-level `model` keys are rejected.

Translated providers intentionally support a conservative OpenAI-style request subset. Unsupported top-level controls such as `tools`, `tool_choice`, `response_format`, `logprobs`, `parallel_tool_calls`, and unknown extension fields are rejected with `invalid_request` instead of being silently dropped.

Translated chat completions support these top-level request fields: `model`, `messages`, `max_tokens`, `temperature`, `top_p`, and `stream`. Message roles are limited to `system`, `user`, and `assistant`, and content may be text or arrays of text parts.

Translated responses support these top-level request fields: `model`, `input`, `instructions`, `max_output_tokens`, `temperature`, `top_p`, and `stream`. Input may be a string or an array of message items with text content.

Gemini translated embeddings support these top-level request fields: `model`, `input`, and `dimensions`. Input may be a string or an array of strings.

## OpenCode Zen And Go

`opencode-zen` and `opencode-go` share one adapter behind two explicit types.
The type selects the service, never the URL or credential:

- `opencode-zen` defaults to `https://opencode.ai/zen/v1`
- `opencode-go` defaults to `https://opencode.ai/zen/go/v1`

`base_url` is an optional transport override only (same absolute-URL and
loopback rules as other providers). An override never reclassifies the
service: auth, header, and protocol behavior stay type-driven.

Every model declares a required `protocol`:

| `protocol`  | Upstream request                                                                                          | Serves public operations |
| ----------- | --------------------------------------------------------------------------------------------------------- | ------------------------ |
| `chat`      | `POST <base>/chat/completions`, model rewrite, JSON/SSE pass-through                                      | `chat` only              |
| `responses` | `POST <base>/responses`, model rewrite, JSON/SSE pass-through                                             | `responses` only         |
| `messages`  | `POST <base>/messages`, existing Messages translation subset                                              | `chat` and `responses`   |
| `gemini`    | `POST <base>/models/<upstream>:generateContent` (JSON) / `:streamGenerateContent?alt=sse` (SSE); Zen only | `chat` and `responses`   |

A public operation the model's protocol does not serve is rejected before any
upstream I/O, as are `embeddings`, `images`, and audio operations on both
OpenCode types. The same model name may use different protocols per service
(for example `minimax-m3`, which is chat-protocol on Zen but
messages-protocol on Go), so routing comes from explicit configuration, never
from the model name. There is no generic `opencode` type.

### Session And Client Requirements

Every upstream request sends `User-Agent: aiproxy/<version>` unless the
provider declares a `user_agent` override (supported on `opencode-zen` and
`opencode-go` only, validated as 1-256 printable ASCII characters); the
inbound `User-Agent` is never forwarded implicitly, so matching a first-party
client fingerprint is always an explicit operator choice. Both
`opencode-zen` and `opencode-go` additionally send `x-opencode-session` for
prompt caching: a caller-supplied `x-opencode-session` is forwarded as-is when it is 1-128
characters of `[A-Za-z0-9_-]`; otherwise the caller's `X-Session-Id` is adopted
when valid, and only then does the proxy generate a fresh
per-request `ses_` + 128-bit hex ID. A caller-supplied `x-opencode-client`
is forwarded under the same validity rule and omitted otherwise. Missing or invalid values never fail the
request and never create shared cross-client state. No other inbound headers
or credentials are forwarded.

### Quota Errors, Failover, And Static Catalogs

Direct `<provider>/<model>` requests never cross services: a request for
`zen/glm-5.3` either reaches Zen or fails with a clear error. Upstream Go
quota/limit errors are returned to the client like any other upstream error;
only explicitly configured aliases retry another target, for example an alias
spanning `zen` and `go` targets. The upstream console setting that spends Zen
balance past Go limits is an account setting, not permission for the proxy to
reroute requests. Model catalogs are static configuration validated at load;
the proxy performs no runtime catalog sync and advertises no universal model
support beyond what is configured.

## GitHub Copilot

`github-copilot` is a chat-only provider backed by a device-flow login. It
serves `POST /v1/chat/completions` (JSON and SSE); `responses`, `embeddings`,
`images`, and audio are rejected before upstream I/O. The default origin is
`https://api.githubcopilot.com`; `base_url` is an optional transport override
only and never changes auth or header behavior.

Provisioning uses your own public OAuth client ID (no secret):

```sh
aiproxy login github-copilot --client-id YOUR_GITHUB_OAUTH_CLIENT_ID --credential copilot-main
```

The command prints the verification URI and user code, waits for
authorization, then writes `<secrets-dir>/copilot-<name>.json` (`0600`)
without editing HCL or signaling a server. It is headless-friendly over SSH:
copy the URI/code to a browser, authorize, and return. Never reuse another
application's client ID.

Reference the saved login from config:

```hcl
provider "github-copilot" "copilot" {
  credential_ref {
    name = "copilot-main"
  }

  model "gpt-5.4-nano" {}
}
```

`credential_ref.path` defaults to the shared secrets path. Derived Copilot
providers require their own local `credential_ref`. The token activates on
restart/`SIGHUP`; sidecar-only changes are inert until reload, failed reloads
keep the old runtime, and upstream `401`/`403` means re-run `login` with the
same client ID/name and reload. Upstream headers are an allowlist only
(`Authorization` from the stored login, proxy `User-Agent`,
`X-GitHub-Api-Version`, `Openai-Intent`, derived `x-initiator: user`, vision
only on image bodies); inbound auth/cookies/`x-api-key`/caller Copilot
metadata are stripped. `GET {base}/models` listing shares the same auth
without changing the static proxy inventory. See `examples/github-copilot.hcl`
for a complete config.

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

Set explicit capabilities when you want the public catalog to reflect a narrower contract than the provider's default behavior, or to opt into one of the additional supported capabilities for that provider type. On `opencode-zen` and `opencode-go` the omitted default is protocol-aware (`chat` serves `chat`, `responses` serves `responses`, `messages` and `gemini` serve both), and capabilities outside the protocol-served set fail validation.

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
