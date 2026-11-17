---
sidebar_position: 5
---

# API Reference

`aiproxy` exposes an OpenAI-compatible HTTP API.

This page focuses on the proxy-facing contract and operation coverage. It does not attempt to restate every upstream provider-specific field or option.

## Endpoint And Provider Support Matrix

<!-- docs-contract:public-matrix:start -->

| Surface                         | `openai`                           | `openai-compatible`                | `anthropic`                        | `gemini`                           | `opencode-zen`                               | `opencode-go`                                | `github-copilot`                   | `zenmux`                           | `openrouter`                       |
| ------------------------------- | ---------------------------------- | ---------------------------------- | ---------------------------------- | ---------------------------------- | -------------------------------------------- | -------------------------------------------- | ---------------------------------- | ---------------------------------- | ---------------------------------- |
| `GET /v1/models`                | Proxy-owned                        | Proxy-owned                        | Proxy-owned                        | Proxy-owned                        | Proxy-owned                                  | Proxy-owned                                  | Proxy-owned                        | Proxy-owned                        | Proxy-owned                        |
| `GET /v1/billing/usage`         | Proxy-owned local usage accounting | Proxy-owned local usage accounting | Proxy-owned local usage accounting | Proxy-owned local usage accounting | Proxy-owned local usage accounting           | Proxy-owned local usage accounting           | Proxy-owned local usage accounting | Proxy-owned local usage accounting | Proxy-owned local usage accounting |
| `GET /metrics`                  | Proxy-owned Prometheus metrics     | Proxy-owned Prometheus metrics     | Proxy-owned Prometheus metrics     | Proxy-owned Prometheus metrics     | Proxy-owned Prometheus metrics               | Proxy-owned Prometheus metrics               | Proxy-owned Prometheus metrics     | Proxy-owned Prometheus metrics     | Proxy-owned Prometheus metrics     |
| `POST /v1/chat/completions`     | JSON and SSE                       | JSON and SSE                       | JSON and SSE translated            | JSON and SSE translated            | JSON and SSE native or translated subset     | JSON and SSE native or translated subset     | JSON and SSE                       | JSON and SSE                       | JSON and SSE                       |
| `POST /v1/messages`             | No                                 | No                                 | JSON and SSE                       | No                                 | JSON and SSE native (messages protocol only) | JSON and SSE native (messages protocol only) | No                                 | No                                 | No                                 |
| `POST /v1/embeddings`           | Yes                                | Yes                                | No                                 | Yes                                | No                                           | No                                           | No                                 | Yes                                | Yes                                |
| `POST /v1/responses`            | JSON and SSE                       | JSON and SSE                       | JSON and SSE translated subset     | JSON and SSE translated subset     | JSON and SSE native or translated subset     | JSON and SSE native or translated subset     | No                                 | JSON and SSE                       | JSON and SSE                       |
| `POST /v1/images/generations`   | Yes                                | Yes                                | No                                 | No                                 | No                                           | No                                           | No                                 | Yes                                | Yes                                |
| `POST /v1/audio/transcriptions` | Yes                                | Yes                                | No                                 | No                                 | No                                           | No                                           | No                                 | Yes                                | Yes                                |
| `POST /v1/audio/speech`         | Yes                                | Yes                                | No                                 | No                                 | No                                           | No                                           | No                                 | Yes                                | Yes                                |

<!-- docs-contract:public-matrix:end -->

## Provider Capability Defaults

<!-- docs-contract:capability-matrix:start -->

| Provider type       | Default capabilities when omitted                      | Additional supported capabilities                |
| ------------------- | ------------------------------------------------------ | ------------------------------------------------ |
| `openai`            | `chat`, `responses`, `embeddings`                      | `images`, `audio_transcriptions`, `audio_speech` |
| `openai-compatible` | `chat`, `responses`, `embeddings`                      | `images`, `audio_transcriptions`, `audio_speech` |
| `anthropic`         | `chat`, `responses`, `messages`                        | None                                             |
| `gemini`            | `chat`, `responses`                                    | `embeddings`                                     |
| `opencode-zen`      | `chat`, `responses`, `messages`, or more (by protocol) | None                                             |
| `opencode-go`       | `chat`, `responses`, `messages`, or more (by protocol) | None                                             |
| `github-copilot`    | `chat`                                                 | None                                             |
| `zenmux`            | `chat`, `responses`, `embeddings`                      | `images`, `audio_transcriptions`, `audio_speech` |
| `openrouter`        | `chat`, `responses`, `embeddings`                      | `images`, `audio_transcriptions`, `audio_speech` |

<!-- docs-contract:capability-matrix:end -->

When a model omits `capabilities`, the provider-type defaults are used. Explicit
capabilities can narrow that default or opt into an additional supported
capability listed above. On `opencode-zen` and `opencode-go` the omitted
default is protocol-aware: `chat` models default to `chat`, `responses` models
to `responses`, `messages` models to `chat`, `responses` and `messages`, and
`gemini` (Zen only) models to `chat` and `responses`.

## Streaming

Streaming uses OpenAI-compatible Server-Sent Events, except `POST /v1/messages`
which passes Anthropic SSE through verbatim.

- `POST /v1/chat/completions` supports JSON and SSE streaming
- `POST /v1/responses` supports JSON and SSE streaming where implemented
- `POST /v1/messages` supports JSON and SSE (Anthropic shape, Anthropic-family targets only)
- translated providers map their upstream streaming format back into OpenAI-compatible SSE chunks

If an upstream stream fails after partial output, the proxy terminates the stream instead of fabricating a full JSON response.

## Peer Deferred Transport v1

This private opt-in extension serves chat completions between a statically
configured caller and executor. It adds **no provider/model capability** and is
not Batch or Responses background mode. Ordinary clients keep
`POST /v1/chat/completions`, JSON/SSE and their original `stream` choice; they do
not receive peer receipts, cursors or private headers. Other operations keep
ordinary transport. See [configuration](configuration.md#peer-deferred-transport-v1)
and [operational limits](operations.md#peer-deferred-transport-v1).

Caller chat transport follows the resolved model's local `deferred_transport`,
otherwise its provider's local compatibility default, otherwise ordinary
transport. A model block replaces the whole default, including when empty or
partial. Direct and alias-target requests use the same precedence. The executor's
`deferred_receiver` is provider-only and additive: it permits private admission
without forcing ordinary calls for the same models into jobs. Model placement
changes neither these v1 routes/schema nor the public support/capability matrices.

### Peer Routes And Recognition

The executor serves the unmounted prefix `/_internal/deferred/v1`. Private paths
are intercepted before web UI fallback. All controls require inference bearer
authentication and `X-Aiproxy-Deferred-Version: 1`; job controls additionally
require one `X-Aiproxy-Deferred-Generation` header matching their receipt.
Submit carries the generation in its envelope.

| Method / path under `/_internal/deferred/v1`        | Success status / body                                                                                    | `Allow` on a wrong method |
| --------------------------------------------------- | -------------------------------------------------------------------------------------------------------- | ------------------------- |
| `GET /capabilities?model=<public-model>`            | `200 Capability`; current model/receiver eligibility, no inference quota charge                          | `GET`                     |
| `POST /submit`                                      | `202 Receipt` after bounded admission/scheduling, or accepted dedup replay                               | `POST`                    |
| `GET /jobs/{id}`                                    | `200 JobInfo`                                                                                            | `GET, DELETE`             |
| `GET /jobs/{id}/result`                             | `202 JobInfo` while active; `200 ResultManifest` when terminal; no body bytes here                       | `GET`                     |
| `GET /jobs/{id}/read?cursor=<offset>&limit=<bytes>` | `200 ReadFrame`, immediate finite snapshot; required cursor, optional limit 1–65536, default 65536 bytes | `GET`                     |
| `DELETE /jobs/{id}`                                 | Empty `204`; cancel-if-running and release, including repeated known release                             | `GET, DELETE`             |

Only submit has a request body, with
`Content-Type: application/vnd.aiproxy.deferred+json`. Query fields must be
recognized and unique; submit has no query. Every recognized response carries:

```http
Content-Type: application/vnd.aiproxy.deferred+json
X-Aiproxy-Deferred-Version: 1
Cache-Control: private, no-store
Pragma: no-cache
Vary: Authorization
```

The empty `204` still carries these headers. Recognition requires the dedicated
path, exact media/version markers, matching envelope and expected identity,
**not just HTTP `202`**. The caller validates a positive capability for its
configured executor public model before any submit. Missing/wrong-version
capabilities cause zero submits. An unrecognized submit response is
indeterminate, never permission to use synchronous fallback or a new attempt.

For a caller `base_url`, remove one terminal `/v1` segment and append the private
prefix: `https://executor.example/mount/v1` becomes
`https://executor.example/mount/_internal/deferred/v1`. An external mount must
strip `/mount` before forwarding to the executor. Query strings, fragments, dot
segments, backslashes and encoded path separators are invalid in a deferred
caller base URL. Control URLs are derived locally, never from arbitrary peer
links. Submit's `Location` must match the expected origin, mount and job path
(the exact absolute URL or path). **All control redirects are forbidden.**

For the [full configuration examples](configuration.md#full-caller-configuration),
capability discovery is:

```http
GET /_internal/deferred/v1/capabilities?model=alias%2Fpeer_chat HTTP/1.1
Host: executor.example
Authorization: Bearer <peer-token>
Accept: application/vnd.aiproxy.deferred+json
X-Aiproxy-Deferred-Version: 1
```

Substitute the configured peer token; use the returned generation/ticket and
receipt ID, not invented identifiers, for subsequent controls. A read request is:

```http
GET /_internal/deferred/v1/jobs/<receipt-id>/read?cursor=0&limit=65536 HTTP/1.1
Host: executor.example
Authorization: Bearer <peer-token>
Accept: application/vnd.aiproxy.deferred+json
X-Aiproxy-Deferred-Version: 1
X-Aiproxy-Deferred-Generation: <receipt-generation>
```

### Peer JSON Schema

Objects below use flattened fields, not wrapper objects named `Receipt` or
`JobInfo`. Every envelope has `protocol: "aiproxy-deferred"` and numeric
`version: 1`. Unknown/duplicate fields and numeric overflow are rejected.
Timestamps are integer Unix milliseconds; durations are integer milliseconds.
Optional fields are omitted when absent.

| Object           | Required fields in addition to the common envelope                                                                                                                |
| ---------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `Capability`     | `generation`, `public_model`, `submit_token`, `submit_until_ms`, `limits`, `poll_after_ms`                                                                        |
| `SubmitRequest`  | `generation`, `submit_token`, `attempt_key`, `headers`, `body`                                                                                                    |
| `Receipt`        | `generation`, `id`, `public_model`, `attempt_key`, `scope_digest`, `fingerprint`, `state`, `execution_deadline_ms`, `dedup_until_ms`, `poll_after_ms`, `replayed` |
| `JobInfo`        | All receipt fields plus `available_bytes`; optional `response`, `outcome`, `retain_until_ms`                                                                      |
| `ResultManifest` | All job-info fields plus `body_sha256`; terminal only                                                                                                             |
| `ReadFrame`      | All job-info fields plus `cursor`, `next_cursor`, `data`, `done`                                                                                                  |
| `ControlError`   | `error`, `acceptance`; optional `poll_after_ms`                                                                                                                   |

| Nested object | Fields / meaning                                                                                                                                                 |
| ------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `limits`      | `request_bytes`, `result_bytes`, `execution_timeout_ms`, `retention_ms`, `control_timeout_ms`, `finalization_timeout_ms`, `read_bytes`                           |
| `response`    | `status_code` (200–599), `headers` (name → string array), `streaming` (boolean); optional `retry`                                                                |
| `retry`       | `observed_at_ms`, `deadline_ms`, `remaining_ms`; original inference retry timing, not control poll advice                                                        |
| `outcome`     | `kind`, `usage`; optional `error`                                                                                                                                |
| `usage`       | Nonnegative observed integers: `prompt_tokens`, `completion_tokens`, `total_tokens`, `cached_tokens`, `cache_creation_tokens`, `cache_read_tokens`; no estimates |
| `error`       | `type`, `message`; optional `block_id`                                                                                                                           |

`body` and `data` are non-null standard JSON base64 strings; empty bytes are
`""`. The decoded submit body is a JSON object with one required `model` string
and zero/one boolean `stream`; other inference fields are retained. It contains
the caller adapter's model rewrite, then undergoes executor admission and
existing adapter processing. The bounded `headers` bag has JSON content type
and safe prepared inference metadata, never credentials, cookies, hop-by-hop or
private deferred headers.

`cursor`, `next_cursor` and `available_bytes` are **strings** containing canonical
unsigned decimal byte offsets/counts, with no sign or leading zeros except `"0"`.
Generation and job IDs are canonical unpadded base64url encodings of 16 random
bytes. Attempt keys are 22–64 base64url characters, generated with at least
128 random bits. `scope_digest`, `fingerprint` and `body_sha256` are lowercase
SHA-256 hex strings. `public_model` is at most 512 bytes.

Header bags allow at most 64 case-insensitively distinct names, 128 values total
and 4096 bytes/value. Non-data serialized metadata is at most 16 KiB; error detail
is at most 1 KiB. Finite control responses are at most 128 KiB, reads at most
64 KiB decoded data, and submission tickets at most 512 bytes. Submit input has
its separate decoded request ceiling plus bounded base64/metadata scratch;
the 128 KiB response ceiling is not a submit input ceiling.

### Peer State And Byte Delivery

| State           | Terminal `outcome.kind`                                                      | Meaning                                                                              |
| --------------- | ---------------------------------------------------------------------------- | ------------------------------------------------------------------------------------ |
| `running`       | Absent                                                                       | No selected response; reads can have empty `data`                                    |
| `headers_ready` | Absent                                                                       | Final selected adapter head and incremental bytes, not an intermediate alias attempt |
| `completed`     | `response`                                                                   | Complete normalized adapter result, including provider HTTP errors                   |
| `failed`        | `upstream_error`, `invalid_request`, `execution_timeout` or `limit_exceeded` | Known failed execution; an exact streamed prefix may be retained                     |
| `cancelled`     | `cancelled`                                                                  | Local cancellation, not proof external execution/billing stopped                     |

Released/expired tombstones are not result states and disclose no retained head
or body. `response` is the original selected status and filtered headers after
existing adapter/non-JSON-error normalization, with private headers stripped.
A terminal provider `500` is a `completed` / `response` outcome, not a peer
control `500`.

Each read atomically captures state/head/outcome/available tail and one finite
byte slice. `next_cursor = cursor + len(decoded_data)`. A cursor beyond that
captured tail is `400 invalid_cursor`. `done` is true only when terminal and
the frame reaches the final byte; an empty pending frame or HTTP EOF is not
completion. Decode and validate the **whole transfer** before delivering bytes
and advancing the cursor; discard partial transfers and retry the unchanged
cursor.

The terminal manifest hashes all final retained body bytes, including an empty
body or failed prefix. Unary callers wait for that manifest. Streaming callers
deliver the existing adapter/native bytes incrementally without synthetic SSE
events, IDs, heartbeats or `[DONE]`; terminal validation cannot retract already
delivered bytes. After downstream commitment, errors terminate the stream rather
than selecting another inference. This is peer byte recovery, not client-session
resume.

### Acceptance And Deduplication

Capabilities issue a stateless signed **30-second** ticket binding protocol,
process generation, owner/model digests, nonce, expiry and advertised limits.
Qualified dedup identity is `(generation, SHA256(ticket), owner, attempt_key)`.
The fingerprint covers fixed operation/model, exact decoded body and canonical
header bag, not mutable credentials or post-guardrail output.

The same fingerprint replays an accepted receipt without repeating inference
admission, rate/quota charges or guardrails. A conflict returns `409`; a still
preparing duplicate returns `409 submission_in_progress`. Preparation rejections
are cached through the ticket window. Accepted records/tombstones remain protected
through `dedup_until_ms = submit_until_ms + execution_timeout_ms + retention_ms +
finalization_timeout_ms`. Existing accepted entries can replay after ticket
expiry; absent expired scopes or old generations dispatch nothing.

Once submit begins, the caller freezes generation/ticket/key/prepared input.
Lost acknowledgments retry only that qualified submit, never a fresh capability
or key. `acceptance` is `not_accepted`, `accepted` or `indeterminate`; later
auth/control failures never downgrade an accepted/indeterminate logical attempt.
Missing restart evidence means unavailable/indeterminate, not invented proof of
expiry, cancellation or exactly-once external work.

### Peer Control Errors

Protocol errors use `ControlError`. Existing ordinary inference admission
rejections instead preserve their ordinary `{"error": ...}` shape, status/type
and optional `block_id`, without protocol fields. Neither is an inference
success or provider outcome.

| Status / type                                               | Meaning                                                                                                  |
| ----------------------------------------------------------- | -------------------------------------------------------------------------------------------------------- |
| `400`                                                       | Invalid envelope, body, headers, model/receiver eligibility or query; `invalid_cursor` for invalid reads |
| `401 auth_failed` / `403 forbidden`                         | Current inference authentication / admitted-model authorization failed; no result disclosure             |
| `404 job_unavailable`                                       | Unknown or different-owner job, including unknown DELETE                                                 |
| `404 deferred_disabled`                                     | New deferred admission is disabled                                                                       |
| `405 method_not_allowed`                                    | Recognized route, unsupported method; carries `Allow`                                                    |
| `409 attempt_conflict` / `409 submission_in_progress`       | Conflicting qualified request / preparation still active                                                 |
| `410 job_released` / `410 job_expired`                      | Protected release / retention expiry                                                                     |
| `410 generation_mismatch` / `410 submit_scope_expired`      | Old generation / absent expired submission scope; zero dispatch                                          |
| `413 request_too_large`                                     | Oversized decoded/wire submit input                                                                      |
| `426 unsupported_deferred_version`                          | Wrong path version or missing/wrong version header                                                       |
| `429 deferred_owner_limit` / `429 deferred_control_limited` | Owner admission / control pressure                                                                       |
| `503 deferred_saturated`                                    | Global admission/resource pressure                                                                       |

The ordinary caller sees standard JSON errors before downstream commitment:

| Caller status / error type                               | Meaning                                                                      |
| -------------------------------------------------------- | ---------------------------------------------------------------------------- |
| `502 deferred_protocol_error`                            | Capability/version/marker/schema/identity or terminal-hash validation failed |
| `502 deferred_submit_indeterminate`                      | Unacknowledged submission uncertainty; no fresh inference                    |
| `502 deferred_request_lost`                              | Accepted work became unavailable; no fresh inference                         |
| `502 deferred_control_error`                             | Control failure or recovery budgets exhausted                                |
| `504 deferred_wait_timeout`                              | Overall caller wait expired                                                  |
| `504 deferred_execution_timeout`                         | Fixed executor job deadline expired                                          |
| `502 deferred_limit_exceeded` / `502 deferred_cancelled` | Bounded execution failed / observable local cancellation                     |

Definite terminal provider HTTP/transport outcomes **before downstream
commitment** still use ordinary inference retry rules. Peer errors, polling
`Retry-After`, admission pressure and accepted/indeterminate retrieval failures
do not trigger alias/encrypted-reasoning inference retries, provider-health
mutation or model cooldown. After commitment, failure only terminates the stream.
See [ownership and cleanup](operations.md#ownership-cancellation-and-release).

## Authentication

When `bearer_static` auth is enabled, requests must send:

```http
Authorization: Bearer <token>
```

When `none` auth is enabled, no inbound authentication is performed.

In `bearer_static` mode, individual clients may also be restricted with `allowed_models`.

### Invitation Onboarding

With multi-tenancy enabled, `POST /_internal/admin/invites/accept` accepts a JSON
body with `token` and `password` (at least eight characters). A successful `201`
creates the account, adds the invitation's workspace membership when present,
and consumes the invitation in one database transaction. Email, system-admin
authority, workspace, and workspace role come from the current stored
invitation; an omitted or unrecognized workspace role defaults to `member`.

Acceptance is complete-or-no-change. Revocation, expiry, and prior acceptance are
checked again at consumption, including after waiting for another acceptance.
Only one competing request can consume an eligible invitation. Missing, revoked,
or replaced tokens return `404`; expired/already accepted invitations and an
already registered email return `400`. Database failures return a controlled
`500` without database details. An account or membership write failure rolls back
the whole acceptance, leaving an otherwise eligible invitation available to retry
after the cause is resolved. Successful onboarding supports the normal login flow.
Acceptance also serializes with workspace deletion. If the stored invitation's
workspace changes while acceptance is acquiring locks, it returns `400` with
retry guidance and saves nothing; retry uses the current invitation scope.

### Workspace Model

Tenancy is built on workspaces. Tables are `workspaces`, `workspace_members`,
and `workspace_teams`; owning rows carry `workspace_id`. Admin routes live under
`/_internal/admin/workspaces`, resource selection accepts `workspace_id` or the
`X-Workspace-ID` header, and mutable fields use `workspace_id`, `workspace_role`,
and `workspace_name`.

Kinds are `personal` and `organization`, plus a `system` flag:

- `personal`: exactly one per user, auto-created at registration. It holds only
  its owner, so teams and additional members are rejected.
- `organization` (kind): created afterwards via `POST /_internal/admin/workspaces`
  for collaboration with members and teams.
- `system`: the built-in workspace (`is_system`) that owns global config-backed
  inventory. It is hidden from membership listings and cannot be deleted.

### Workspace And Team Membership Changes

With multi-tenancy enabled, Add member is insert-only on both
`POST /_internal/admin/workspaces/{workspace_id}/members` and
`POST /_internal/admin/workspaces/{workspace_id}/teams/{team_id}/members`. Supply `user_id`
or `email`, plus optional `role` (`admin` or `member`, default `member`). A new
membership returns `201`; an existing membership returns `409` and keeps its role,
even if a different role was supplied. In the web UI, use the existing member's
role control; API clients should use `PUT` on the same path with `/{user_id}`
appended and an explicit `role`.

Role changes and removals check the current administrator set in a serialized
database transaction. Once a workspace or team has an administrator, these
operations cannot remove or demote its last one, including competing requests.
Assign another administrator before retrying a rejected handoff (`400`). A
workspace administrator cannot demote their own workspace role; another
administrator must make that edit. Team administrators may demote themselves
when another team administrator remains. These guards also apply when deleting
a user account would cascade away their administrator memberships. New teams
can still start without a team administrator and receive their first one later.

Team additions require current membership in the containing workspace. A
successful role edit or removal returns `200`; a missing membership returns
`404`. Unexpected storage failures return a controlled `500` with no committed
membership change. The UI displays the server's conflict/handoff guidance.

Workspace offboarding (`DELETE /_internal/admin/workspaces/{workspace_id}/members/{user_id}`)
also removes that user's team memberships and direct key-sharing grants in the
workspace, in the same transaction. If they are the sole admin of any affected
team, the request returns `400` with handoff guidance and changes nothing. Assign
another team administrator, then retry. Other workspaces, key ownership,
team-wide key shares, quota records and historical spend are preserved. Re-adding
an ordinary member does not restore prior team roles or direct sharing grants.

Current workspace membership is required for non-system key owners/team admins
to read or manage keys and for team quota access, even with a known resource ID or
an existing login token. After removal, key detail and team quota GET/PUT return
`404`; key update/rotate/revoke/delete return `403`. System administrators retain
access. Personal ownership is preserved and becomes usable again if the owner is
re-added to the workspace. Already-admitted operations are not retroactively
canceled.

**API bearer credentials are separate:** removing management access does not
automatically revoke, rotate or disable any key, including a personal key. To stop
use of previously distributed API tokens, explicitly rotate or revoke the key.

### Key Policy And Sharing Updates

`PUT /_internal/admin/keys/{key_id}` accepts `description`, `tenant`,
`allowed_models`, `expires_at`, `user_ids`, and `team_ids`. Policy fields and an
optional sharing replacement commit in one database transaction. The update keeps
the key's name, workspace, ownership, bearer credential, and enabled state.

- Omitted or `null` fields preserve their current values. An empty `description`
  or `tenant` clears it; `allowed_models: []` removes the model restriction.
- `expires_at` accepts RFC3339; an empty string clears expiry. A past timestamp
  is valid and makes the key stop authenticating after successful activation.
- Supplying either non-null binding list replaces **both** user and team shares.
  The other omitted list is treated as empty. Supplying `user_ids: []` alone,
  for example, clears both sets. Omitting both lists preserves both sets.

Malformed UUIDs, nonmember users, and teams from another workspace return
`400`; missing keys/teams return `404`. Current manager authority and grant
eligibility are rechecked inside the transaction, serialized with workspace
offboarding and membership role changes. Lost manager authority returns `403`.
Rejected validation and binding-storage failures leave all fields and both sharing
sets unchanged and do not request runtime activation. Storage failures return
`500` (`could not update key`) without database details. A later reload cannot
activate a rejected edit.

After a valid commit the server requests runtime activation once, before building
the success response. Successful activation applies authentication expiry, tenant,
and allowed-model policy to subsequent requests. If activation fails, the response
is `500` with `saved but activation failed: ...`: **the complete edit is saved**,
the previous runtime remains active, and a later successful reload applies it.
Resolve the reported activation problem and reload; this differs from a mutation
failure that saved nothing.

## Usage And Scope

`GET /v1/billing/usage` returns aggregated in-process usage summaries over the
proxy's rolling 24-hour accounting window.

- When the authenticated client has a `tenant`, results are scoped to that tenant
- Otherwise, results are scoped to the caller's tenantless client identity; a
  same-named client in another tenant is excluded
- This endpoint is local accounting only; it is not an external billing,
  invoicing, or quota system

Rows are grouped by tenant, client, requested public model, operation, and HTTP
status. The optional `estimated_cost_usd` uses current configured model prices.
For an alias, it sums the recorded token usage of the targets actually used for
that alias's row. Traffic through another alias or a direct model sharing those
targets is excluded. Changing alias membership does not redistribute past traffic;
changing model prices reprices the retained estimate.

The estimate is **omitted**, rather than returned as a partial sum, when any
contributing target lacks pricing or when target attribution does not account for
all the row's recorded requests and tokens. There is no even-split or request-count
fallback. Unused targets with missing prices do not affect the estimate. Direct and
alias estimates also require positive rates for the token categories consumed:
input, output, and generic cached tokens. Explicit cache-read and cache-write usage
can use the configured input-rate fallback. An omitted rate and an explicit zero
rate cannot be distinguished, so either makes the estimate unavailable when needed.
A numeric zero means the fully priced recorded usage costs zero; it does not prove
that the upstream reported every billable token.

Usage and cost share one rolling snapshot, using one-minute buckets retained while
their start is at or after `now - 24h`. Consequently, events can expire up to one
minute before their exact 24-hour age. Data is process-local and resets on restart.
The dashboard's lifetime provider/upstream counters and the durable quota spend
ledger have separate lifetimes and pricing semantics.

## Error Behavior

- Direct requests never fail over to another provider
- Alias requests retry the next target on transport errors, timeouts, and status
  codes listed in `retry_status_codes`
- The default `retry_status_codes` list is `500`, `502`, `503`, and `504`
- Configured retry statuses may include `4xx` responses such as `429`; retryable
  `4xx` statuses do not mark providers unhealthy
- Other upstream `4xx` responses are returned verbatim, except for the narrowly
  gated same-target reasoning recovery below
- Chat completions/Responses may make one conditional same-identity
  encrypted-reasoning recovery attempt: direct requests make at most two
  inference attempts; aliases share one extra attempt across all targets, not
  one per target. An ordinary immediate 400 to `stream: true` may qualify;
  established streams and peer control failures never do. See
  [all gates and stripping limits](providers-and-routing.md#scoped-encrypted-reasoning)
- Unsupported operations return client-visible proxy errors
- Peer deferred control failures are not provider outcomes and do not authorize
  alias failover; definite terminal inference outcomes retain the rules above
  before downstream commitment (see [peer errors](#peer-control-errors))
- For database-owned user/team keys, unavailable required quota policy or spend
  data returns JSON `503`
  (`{"error":{"type":"quota_unavailable","message":"quota data temporarily unavailable"}}`)
  before upstream dispatch, including direct/alias and streaming requests. This
  is a temporary failure that can be retried after storage recovers. Confirmed
  budget exhaustion instead returns `403` `budget_exceeded`; confirmed TPM
  exhaustion returns `429` `tpm_exceeded` with `Retry-After`.
- Alias targets carrying valid upstream retry advice (`retry-after-ms`, else
  `Retry-After`, else exhausted Meta quota headers) cool down across requests:
  later alias requests skip cooling
  targets until expiry. When every pool target is actively cooling, the proxy
  returns a generated JSON `429`
  (`{"error":{"type":"upstream_rate_limited","message":"all alias targets cooling, retry after <N>ms"}}`)
  with `Retry-After` (ceiling seconds, min 1) and `retry-after-ms` (ceiling
  milliseconds, min 1) from the same earliest remaining delay, without upstream
  calls. Direct requests never consult or populate this state.

This behavior is deliberate: direct model requests are explicit, while alias requests are the only place where the proxy is allowed to choose another target.

## Administrative Provider And Alias Saves

`POST /_internal/admin/providers` and `PUT /_internal/admin/providers/{name}`
save provider metadata and supplied models atomically. A rejected model write
leaves no new provider on POST and preserves the exact previous provider and models
on PUT. Omitting `models` (or sending null) preserves models; a supplied array is a
replacement subject to normal provider validation. Omitted fields and write-only
credentials are preserved on update. Policy values have the explicit null-removal
exception below. Credential-only PUT uses the same concurrency
protection and preserves models/policies; it rejects policy/model edits rather
than silently accepting them. Alias POST/PUT likewise save metadata and targets
in one transaction, preserving omitted update fields.

Provider PUT supports explicit `true`/`false` for `enabled`,
`forward_user_agent`, `forward_authorization` and `allow_insecure_base_url`, `[]` to clear local `forward_headers`, and `""` to clear
`display_name`, `base_url`, `upstream_header_timeout`, `user_agent` or `extends`,
subject to normal validation. Clearing an override restores applicable defaults;
it does not disable root-level forwarding. Omit untouched `api_key`, `api_key_ref`
and `credential_ref` fields to retain them. Supplied reference objects replace
their path and key/name together; an empty path selects the default secrets path.

`allow_insecure_base_url` is a provider-only opt-in that accepts a non-loopback
plain-HTTP `base_url` (see
[plain-HTTP upstreams](configuration.md#allowing-plain-http-upstreams)).
Omitting the flag preserves its current value on concrete providers. Enabling
it does not relax any other URL validation. Derived provider POST/PUT rejects
explicit `true` or `false`; setting a non-empty `extends` without declaring the
flag clears any previously stored local opt-in so the base's value is inherited.

`forward_authorization` is a provider-only opt-in for supported bearer-header
upstreams, independent of inherited defaults. It cannot coexist with stored
credentials. To switch an existing keyed provider, explicitly send
`"api_key": ""` and `"api_key_ref": {"path": "", "key": ""}` along with
`"forward_authorization": true`. Omitting the flag preserves its current value.
See [client authorization forwarding](configuration.md#forwarding-client-authorization)
for supported types and credential-exposure warnings.

`deferred_transport` and `deferred_receiver` are static-config-only.
Administrative provider POST/PUT and credential PUT reject either field before
commit, including nested model declarations, null/empty objects and case variants,
for every admin role. Managed derivatives cannot extend a static base carrying
any provider/model caller setting or receiver opt-in.
There are no writable managed/web transport fields; see
[peer configuration](configuration.md#peer-deferred-transport-v1).

Within an existing `healthcheck`, omitted scalar fields are preserved and
`send_authorization: false` disables probe authorization. Empty `method`,
`expected_body`, `interval` and `timeout` reset to `GET`, `*`, `30s` and `5s`;
zero `expected_status`, `failure_threshold` and `success_threshold` reset to
`200`, `2` and `1`. A nonempty `path` replaces the path. **Removing the entire
healthcheck block is unsupported**: omission, null, an empty object or an empty
path does not delete it.

Catalog POST/PUT outcomes:

| Status                                                                           | Meaning and next step                                                                                                                                        |
| -------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `400`                                                                            | Validation rejected the edit; correct the input.                                                                                                             |
| `409`                                                                            | The edited aggregate changed concurrently; read current state and retry.                                                                                     |
| `500`, `could not save catalog edit`                                             | Storage failed before a successful commit; activation was not requested.                                                                                     |
| `500`, `saved but activation failed`                                             | The complete edit is saved; the previous runtime remains active. Consult server logs, correct the activation problem, then reload.                           |
| `500`, `saved but response view unavailable; read current state before retrying` | The edit is saved and activation was requested successfully, but its response view could not be read. Read current state rather than blindly repeating POST. |
| `201` (POST), `200` (PUT)                                                        | Saved, activation requested successfully, and the response view is available.                                                                                |

After each valid commit, activation is requested once before response-view reads.
Operational failure responses omit storage and credential details. Persistence and
runtime activation are separate: later reloads can activate a complete saved edit
after an activation failure. The concurrency check protects the edited aggregate,
not an atomic snapshot of every catalog dependency or another proxy's runtime.
Successful provider POST/PUT and credential PUT responses carry
`X-Aiproxy-Catalog-Saved: true` once the database commit is durable, including
when activation itself fails.

### Encrypted Reasoning Policy Intent

Provider POST/PUT, nested `models` entries, and alias POST/PUT accept an optional
`encrypted_reasoning` object. Members are `passthrough` (boolean),
`on_caller_mismatch` (`"fail"`, `"strip_and_retry"`, or `""` for fail) and
`match_messages` (string array). These are **whole-block declarations**, not
effective values or member patches; omitted members select built-ins. Pattern
normalization/bounds and runtime gates are the same as
[static configuration](configuration.md#scoped-encrypted-reasoning).
Policy members themselves cannot be null.

| Policy input    | Create         | Provider / alias PUT                                                  | Entry in a supplied replacement `models` array        |
| --------------- | -------------- | --------------------------------------------------------------------- | ----------------------------------------------------- |
| Omitted         | No local block | Preserve the exact saved local block                                  | No local policy; do not preserve the old row's policy |
| `null`          | No local block | Remove the local block; resume lower-layer resolution                 | No local policy/overlay                               |
| `{}`            | Safe reset     | Replace with passthrough `true`, mismatch `"fail"`, built-in patterns | Local reset, not inheritance                          |
| Nonempty object | Local override | Replace the entire block, not just supplied members                   | Local override                                        |

**Alias null removal is intentional:** omission keeps an explicit alias policy
and its sticky stripped-body behavior; explicit null removes it, switching to
per-selected-target policies and original-body-per-target behavior. Existing
non-null alias overrides keep their semantics.

Policy null and collection null are different. On provider PUT, omitted **or
null `models` preserves the complete local collection**. An array replaces it
transactionally. A derivative's `models: []` clears only local overlays, not
inherited inventory. A concrete provider still needs at least one valid model.
Within a replacement array, omitted/null policy removes that row's local policy;
retain every other intended local override when submitting the array.

Managed derivatives inherit declared base provider/model policies by value,
with local provider overrides and existing-name model overlays. Their nested
model entries accept **only `name` and `encrypted_reasoning`**:

```json
{
  "extends": "primary",
  "encrypted_reasoning": { "passthrough": false },
  "models": [{ "name": "recover", "encrypted_reasoning": {} }]
}
```

This is a provider PUT fragment, not a complete create. The model reset wins
over both provider policies. Remove the overlay with `models: []` or a replacement
entry with omitted/null policy to restore the base model policy. Policy-free
name entries are validated against the base inventory, then canonicalized away.
New names, duplicate overlays, model definition keys and all deferred keys are
rejected, **even empty/null** (`upstream_name: ""`, `capabilities: []`,
`pricing: null`, or `deferred_transport: null` are not allowed). Static overlays
may combine reasoning and deferred transport; managed overlays cannot.

The base must be enabled, concrete, same-type and eligible for managed
inheritance; credentials stay local, chains remain unsupported. Switching
concrete ↔ derived with any stored model policy/overlay requires an explicit
replacement `models` **array** in that update; omission or null is insufficient.
Changing bases validates preserved overlays against the new inventory. No
provider or credential relaxation is implied.

Writes reject duplicate JSON keys (including case-variant duplicates),
noncanonical reasoning/overlay keys, unknown policy members, wrong
types, invalid patterns and read-only metadata. Do not PUT a GET view unchanged:
inherited model definitions and effective metadata are not local declarations.
`PUT /_internal/admin/providers/{name}/credential` preserves policy/model intent
and rejects supplied policy or model edits before commit.

Provider/model GET views (including static entries under existing visibility
rules) expose:

| Field                              | Meaning                                                                                                                                                                               |
| ---------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `encrypted_reasoning`              | Nullable **owner-local sparse declaration**: null for no local block, `{}` for reset, or only declared members, including explicit false.                                             |
| `encrypted_reasoning_effective`    | Fully normalized policy; provider views resolve provider → root → built-ins, model views resolve model → provider → root → built-ins. A provider value does not override every model. |
| `encrypted_reasoning_source`       | Winning scope: `model`, `provider`, `root`, or `builtin`; provider views never report `model`.                                                                                        |
| `encrypted_reasoning_inherited`    | True only if the winning declaration was copied from a base provider/model through `extends`; root/built-in fallback is false.                                                        |
| `models_inherited` (provider only) | Model definitions come from `extends`, independent of local policy overlays.                                                                                                          |

Derivative GET `models` is the **full inherited inventory**, with local nullable
intent and effective metadata per model; PUT sends only sparse local overlays.
For an inherited model policy, local policy is null, source is `model`, and
inherited is true. A local `{}` overlay changes inherited to false without
changing `models_inherited`.

Alias views expose local `encrypted_reasoning` and normalized
`encrypted_reasoning_effective`. Without a local alias block, effective is
**null**: there is no single pool-wide effective policy.

All effective/source/inherited fields are read-only. Reads describe **saved
intent** resolved against the relevant base/root snapshot, not proof that a
candidate activated. Existing RBAC, static-edit restrictions, durable-save
outcomes/headers and receiving-instance-only activation still apply. Root has
no new admin edit endpoint. Provider/model/alias web policy controls use these
same local-intent semantics: unchanged policies are omitted, inherit removes a
local block, safe reset writes `{}`, and custom writes only declared members.
Inherited model definitions and static rows remain read-only; the forms do not
turn effective context into local declarations.

### Provider TLS ClientHello Settings

Provider POST/PUT accepts `tls_client_hello` for **built-ins only**:
`native`, `chrome`, `chrome-133`, `safari`, `safari-16.0`. Browser aliases resolve
to the frozen bundled Chrome 133/Safari 16.0 presets in uTLS v1.8.2, not live/latest
browser profiles. This adds no inference capability or new authorization surface.

| Input on a concrete provider                             | Behavior                                                    |
| -------------------------------------------------------- | ----------------------------------------------------------- |
| Field omitted on create                                  | Native Go TLS                                               |
| Field omitted on update                                  | Preserve the saved selection                                |
| `"tls_client_hello": "native"`                           | Explicitly reset to native                                  |
| Empty, null, wrong-type, wrong-case or unknown selection | `400`; no commit or activation                              |
| `custom`, any spec-file or inline-spec declaration       | `400`; static-config-only, no commit or profile-file access |

Derived providers inherit the effective profile with their base transport. Every
local TLS declaration, including `native`, is rejected. Changing a concrete
provider to a derived one clears its previous local selection **only if the incoming
TLS field is omitted**. A managed derivative cannot extend a static custom-profile
base. Invalid stored selectors or inheritance also reject catalog startup/reload.
Existing databases require `aiproxy migrate up`; the additive selector column's
empty default means omitted, preserving existing native/inheritance behavior.

`tls_client_hello_spec_file`, `tls_client_hello_spec` and other custom TLS-prefixed
fields are forbidden, even empty or null, for both workspace and system
administrators. `PUT /_internal/admin/providers/{name}/credential` rejects **all**
TLS declarations, including built-in selection, without saving a replacement
credential. Use provider PUT to change a concrete provider's built-in profile.
Static custom configuration requires a file reference; `tls_client_hello_spec`
is not a supported inline recipe or upload option on either surface.

Provider views expose these metadata fields, never profile paths or recipe contents:

```json
{
  "tls_client_hello": "chrome",
  "tls_client_hello_effective": "chrome-133",
  "tls_client_hello_inherited": false,
  "tls_client_hello_static_only": false
}
```

`tls_client_hello` describes the selected or inherited base selector (omitted native
is presented as `native`); `tls_client_hello_effective` is canonical. The booleans
identify inheritance and static-only custom profiles. Effective/inherited/static-only
fields are view metadata, not writable configuration. Each provider-type entry from
`GET /_internal/admin/provider-types` supplies `tls_client_hello_selectors` with the
approved built-in list. The UI uses that list, shows inheritance and displays static
custom profiles read-only. Do not send a derived view's inherited selector back as
a local update. See [configuration](configuration.md#upstream-tls-clienthello) and
[reload behavior](operations.md#reload-behavior); the saved-versus-activated outcomes
above and receiving-instance-only activation remain unchanged.

## GitHub Copilot Device Authorization

`github-copilot` providers can be authorized from the admin API without handling
OAuth tokens in the browser. The server performs the GitHub device challenge over
a fixed issuer (`https://github.com/login/device/code`,
`https://github.com/login/oauth/access_token`), fixed `read:user` scope, and the
fixed device-verification page, then stores the result as an encrypted database
credential. Saving the provider consumes the ready authorization exactly once and
activates it through the normal catalog reload path. The CLI sidecar login remains
available as an alternative.

- `POST /_internal/admin/copilot-device-flows` starts a flow with
  `{client_id, workspace_id?, provider_name?}`. The workspace is resolved with the
  normal provider write-workspace policy; `provider_name` pins an edit target and must
  belong to the same workspace. Responses are `201` flow-status bodies.
- `GET /_internal/admin/copilot-device-flows/{id}` returns the sanitized flow
  status without any upstream calls.
- `POST /_internal/admin/copilot-device-flows/{id}/poll` performs at most one
  upstream token request when the stored schedule and lease permit; early or
  competing calls return the pending status and delay with no extra upstream call.
- `DELETE /_internal/admin/copilot-device-flows/{id}` cancels a live flow and is
  idempotent for terminal flows.

Flow-status bodies carry `id`, `workspace_id`, `status`
(`starting`, `pending`, `ready`, `consumed`, `denied`, `expired`, `failed`,
`cancelled`), pending `user_code`/`verification_uri`/`expires_at`/`poll_after_ms`,
`ready_expires_at`, bound/consumed provider references, and a safe `error_code`.
Device codes, access tokens, refresh tokens, and ciphertext are never returned,
and responses use `Cache-Control: no-store`. Every operation requires a current
app-user JWT with workspace or system administrator authority, plus
initiating-user ownership; another administrator cannot adopt a flow. Unknown or
foreign IDs return `404`, invalid logins `401`, insufficient rights `403`.

Provider `POST`/`PUT` accepts `copilot_device_flow_id` (flow ID only, never a raw
credential) instead of a credential reference. Edit saves that consume a flow
require `expected_updated_at`, the opaque revision from the provider view
(`updated_at`). A stale revision returns `409` and preserves the ready flow for
an explicit reread and retry. Provider views expose
`copilot_credential_source` (`database`, `sidecar`, or `none` alongside the
existing `has_credential`), and `GET /_internal/admin/provider-types` advertises
`supports_device_authorization` for `github-copilot`. The legacy credential
endpoint switches sources the same way (a nonempty sidecar reference clears the
database ciphertext) but never consumes flows. **Hermetically verified; live
GitHub compatibility unverified.**

## Provider Coverage Notes

- `openai`, `openai-compatible`, `zenmux`, and `openrouter` are close to pass-through adapters (`zenmux` defaults to `https://zenmux.ai/api/v1`; `openrouter` defaults to `https://openrouter.ai/api/v1` and adds `HTTP-Referer`/`X-Title` attribution headers)
- `anthropic` and `gemini` use request and response translation
- translated `/v1/responses` support is intentionally conservative compared with the full upstream provider-native feature set
- `opencode-zen` and `opencode-go` mix native and translated handling per
  model `protocol`: `chat`/`responses` protocols are native pass-through for
  one public operation each, `messages` is native Anthropic passthrough for
  `POST /v1/messages` (plus conservative translation for chat/responses),
  and `gemini` uses the conservative translation subsets. Operations a protocol does not serve, and
  embeddings/images/audio on both OpenCode types, are rejected before upstream
  I/O. See [Providers and Routing](providers-and-routing.md) for the protocol
  contract.
- `github-copilot` is chat-only pass-through: `POST /v1/chat/completions`
  serves JSON and SSE, and every other operation (including `responses`,
  `embeddings`, `images`, and audio) is rejected before upstream I/O.
  `GET /v1/models` and `GET /v1/billing/usage` stay proxy-owned with no
  upstream calls. **Hermetically verified; live GitHub compatibility unverified.**
  These capability matrices describe implemented behavior, not verified GitHub
  access. See [mock-only verification](operations.md#mock-only-copilot-verification).
