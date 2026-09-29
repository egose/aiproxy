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
- Other upstream `4xx` responses are returned verbatim
- Unsupported operations return client-visible proxy errors
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
credentials are preserved on update. Credential-only PUT uses the same concurrency
protection and preserves models. Alias POST/PUT likewise save metadata and targets
in one transaction, preserving omitted update fields.

Provider PUT supports explicit `true`/`false` for `enabled` and
`forward_user_agent`, `[]` to clear local `forward_headers`, and `""` to clear
`display_name`, `base_url`, `upstream_header_timeout`, `user_agent` or `extends`,
subject to normal validation. Clearing an override restores applicable defaults;
it does not disable root-level forwarding. Omit untouched `api_key`, `api_key_ref`
and `credential_ref` fields to retain them. Supplied reference objects replace
their path and key/name together; an empty path selects the default secrets path.

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
