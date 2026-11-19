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

With multi-tenancy, discovery and new admissions use the bearer credential's
workspace. Accepted replays and job controls require the same admitted
credential identity and workspace, and still check the current allowlist for
the admitted public model. They do not use a replacement catalog to re-resolve
accepted work. No workspace fields or headers are added to the peer protocol.

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
| `404 model_not_found`                                       | Model or alias unavailable to the caller, including missing names and out-of-workspace inventory         |
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

When `none` auth is enabled and multi-tenancy is disabled, inference does not
authenticate incoming requests. With multi-tenancy enabled, `none` is
management-only. Model discovery, inference, billing usage, and the private
deferred API return `401` without a valid inference bearer with workspace scope.
Browser login JWTs and workspace-selection headers are not inference credentials.

In `bearer_static` mode, configured clients and database keys each have their
own `allowed_models` policy and request rate-limit bucket (if configured).
Matching names or `tenant` labels do not share policy or capacity. No bearer
header or token format changes are required.

### Model Access With Multi-tenancy

`GET /v1/models` lists only the models and aliases the key can use in its own
workspace. An empty workspace returns `{"object":"list","data":[]}`. HCL
providers, models and aliases are System-only, and HCL client tokens are scoped
to System. A database key is scoped to its owning workspace regardless of the
issuer's roles.

`allowed_models` narrows the workspace by public name; omitting it or setting
`[]` never grants access to another workspace. A name excluded by the allowlist
returns `403 forbidden` regardless of whether it exists. Otherwise missing
and out-of-scope names return the same `404 model_not_found`. An alias with a
missing or out-of-workspace target is entirely unavailable: it is not listed,
and no part of its pool is tried. A derived provider whose base is missing or
belongs to another workspace is likewise unavailable. Allowing an alias does
not require listing its target names in `allowed_models`, but cannot widen
workspace scope.

### Invitation Onboarding

The signed-out browser route `/invite` works with `allow_public_registration =
false`. Open the clean `/invite` page on your trusted deployment origin and
paste the one-time code delivered separately by your inviter. Invitation codes
are not read from URLs, including query strings and fragments. Treat previously
shared code-bearing links as exposed: [revoke and reissue pending invitations](operations.md#dashboard-web-ui-and-multi-tenancy).
Opening the form does not erase URLs that were already visited.

The app keeps redemption form values in component memory, not in browser
storage or React Query mutation/query state. The UI uses a `no-referrer` policy.
Reloading or leaving the form discards the app's copy of the code. After
acceptance, sign in with the invited email and new password. The portal selects
the authorized invitation workspace, not System or a stale workspace saved by a
different session.

`POST /_internal/admin/invites` returns the one-time code as `token`. Deliver it
separately over a trusted channel. With multi-tenancy enabled,
`POST /_internal/admin/invites/accept` accepts the code only as `token` in the
JSON body, with `password` (at least eight characters). A successful `201`
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
Invitation creation and acceptance responses use `Cache-Control: no-store`.

### SSO Admission

OIDC is **existing accounts only**, including invitees after redemption creates
their accounts. A pending invitation alone is not admission.
Verified IdP identities without an existing local account receive an error
redirect, never a user, workspace or refresh session. This rule also applies
when public password registration is enabled. A user lookup database error is
distinct from a missing account and fails closed without database details.
Disabled accounts cannot sign in. Returning accounts keep their memberships;
SSO does not create personal workspaces or enroll tenant users into System.
Existing configured admin-claim mapping still applies to admitted accounts;
it is not authority to provision an unknown identity.

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
- `system`: the built-in workspace (`is_system`) that owns config-backed
  inventory. Global administrators and explicit System members can read that
  inventory through the management API. This does not grant System
  administration, and System cannot be deleted.

The provider, alias, and key management lists and detail reads apply workspace
access before returning metadata. `workspace_id` takes precedence over
`X-Workspace-ID`; either can narrow access, not grant it. An inaccessible
workspace or resource returns `404`. Workspace and invitation lists cover the
caller's authorized workspaces; only global administrators see invitations
without a workspace. A caller with no memberships and no global authority gets
empty unfiltered lists. If a required membership lookup fails, these reads
return `500` without inventory or database error details.

### Workspace Catalog References

Provider inheritance and every alias target must belong to the **destination
workspace**, not merely a workspace the author can administer. This applies to
provider/alias POST and PUT, including alias `providers`/`model` shorthand and
omitted targets retained on update. A global administrator creating a tenant
resource cannot use a System or another tenant's base or models. Config-backed
providers, models and aliases belong only to System; a managed System resource
may reference a compatible System base, subject to the ordinary inheritance
restrictions.

Missing and foreign references return the same controlled `400`, without
foreign provider types, models, credentials or enabled-state metadata. Rejected
edits persist nothing and request no activation.

Provider and alias names remain globally unique, including disabled providers.
Startup, reload and `validate --check-db` reject cross-workspace graphs and
static/managed name collisions rather than overwriting a resource or changing
its owner. Invalid legacy graph views fail without disclosing referenced
inventory. See [upgrade and recovery](operations.md#workspace-isolation-upgrade-and-recovery).

The portal honors a saved choice only when it is authorized. Otherwise global
administrators start in System; tenant users prefer an authorized non-System
workspace. Account and workspace resolution must complete before inventory
queries run. No memberships means no tenant write target. Explicit System
membership permits reads, not System administration; tenant membership roles
are retained. Forms, reference pickers, key/credential requests and mutations
use the selected workspace, and stale forms cannot retarget themselves after a
workspace or account change.

### Direct Catalog Writes

This applies to `PUT` and `DELETE` on `/_internal/admin/providers/{name}` and
`/_internal/admin/aliases/{name}`, and to
`PUT /_internal/admin/providers/{name}/credential`. It also applies to
[starting a Copilot device flow](#github-copilot-device-authorization) with
`provider_name`.

- Each supplied `workspace_id` query parameter, `X-Workspace-ID` header, and
  `workspace_id` body field (provider/alias PUT and named Copilot starts) must
  match the resource's workspace. Even global administrators cannot override a
  conflicting selector. Omit unused selectors; empty or invalid IDs return
  `400` (`invalid workspace id`). These operations cannot move a resource.
- For valid requests, missing names, names outside the caller's visibility
  (including hidden System HCL resources), and valid selector mismatches all
  return `404`. The plain-text body is `provider not found` or `alias not found`,
  followed by a newline. These denials save nothing, request no reload, and
  include no save or activation headers.
- A required membership lookup failure returns `500` (`could not resolve
workspace visibility`) without database details. For visible targets in the
  selected scope, write-role requirements are unchanged. Visible HCL resources
  remain read-only (`400` for PUT and named Copilot starts, `403` for DELETE).

Scoped GET selection, graph-reference validation and global name uniqueness on
creation are unchanged.

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
an existing login token. After removal, key detail, [direct key writes](#direct-key-writes),
and team quota GET/PUT return `404`. System administrators retain access. Personal
ownership is preserved and becomes usable again if the owner is re-added to the
workspace. Already-admitted operations are not retroactively canceled.

**API bearer credentials are separate:** removing management access does not
automatically revoke, rotate or disable any key, including a personal key. To stop
use of previously distributed API tokens, explicitly rotate or revoke the key.

### Account Deletion And Personal Bearers

`DELETE /_internal/admin/users/{user_id}` is a global-administrator action.
Self-deletion and deleting the last enabled global administrator are rejected;
workspace/team last-administrator checks remain transactional. Rejected deletion
requests no activation and preserves the account and its keys.

After confirmed deletion, the response sets `X-Aiproxy-User-Deleted: true`,
`X-Aiproxy-User-ID: <user_id>` and `Cache-Control: no-store`, then requests local
activation once. A `200` means new requests using personally owned keys removed
by the account cascade now receive `401` on that instance. Team-owned and other
users' keys retain their identity and workspace; historical spend remains, with
the deleted user's ledger reference set to null. Already-admitted work can finish.
Workspace membership removal still does not revoke bearers.

A `500 saved but activation failed` with the deletion headers means the account
and personal keys are deleted in PostgreSQL, but the previous runtime remains
active and may still accept those bearers. Do not repeat account deletion to
activate it: resolve the logged failure and reload/restart the receiving instance.
DB changes also reconcile independently on other replicas, without cluster acknowledgement.

`GET /_internal/admin/serving-status` requires live system-administrator authorization.
It returns local `applied_revisions` (`catalog`, `keys`), `keys_confirmed_at`,
`catalog_confirmed_at`, `keys_fresh`, `catalog_fresh`,
`freshness_deadline_seconds` (30) and `cluster_acknowledged` (false).
Mutation activation headers distinguish `X-Aiproxy-Local-Active` from
`X-Aiproxy-Cluster-Acknowledged` (always false), alongside
`X-Aiproxy-Applied-Catalog-Revision` and `X-Aiproxy-Applied-Key-Revision`.
They do not replace durable-save headers or acknowledge other replicas.
Expired DB-serving freshness returns `503 serving_stale` on new DB inference,
listing/billing and applicable peer controls; `/readyz` becomes unavailable.
Independent key refresh may revoke keys even when catalog activation fails.
See [bounded replica convergence](operations.md#bounded-replica-convergence).
After response loss, reconcile the account's presence through the authorized user
list; missing headers do not establish rollback. This is not a cluster-wide
revocation acknowledgement.

### Atomic Key Issuance And Bearer Recovery

`POST /_internal/admin/keys` commits the new key, its intended user/team owner,
and both sharing lists in one authorized transaction. The owner's share is included
automatically; repeated binding IDs are deduplicated. Current actor authority,
owner eligibility and user/team workspace membership are rechecked under the
membership locks, including concurrent offboarding, demotion and deletion. A
non-administrator can issue only their own personal key or a key for a team they
currently administer in that workspace; unowned keys require workspace or global
administrator authority.

Validation/authorization failures and key/grant persistence failures roll back
the entire issuance and request no activation. Lost authority returns `403`,
invalid bindings `400`, and storage failures a controlled `500` without database
details. An existing key name returns `409`: read that key rather than issuing
under a new name to recover a missing bearer.

After confirmed commit, creation and rotation responses set
`X-Aiproxy-Key-Saved: true`, `X-Aiproxy-Key-ID: <key_id>`,
`Location: /_internal/admin/keys/<key_id>`, and `Cache-Control: no-store`.
Activation is requested exactly once after commit, before creating the response
view. Creation returns `201` with `key` and the one-time `token`; rotation returns
`200` with a new one-time `token`. Only the bearer hash and display prefix are
stored, never a recoverable plaintext bearer.

Post-commit activation or creation-view failures return `500` with those saved
headers and an explicit key ID and rotation recovery instruction. The complete
issuance remains saved; an activation failure retains the previous runtime.
Read the authorized key detail/list, resolve any activation problem, then use
`POST /_internal/admin/keys/<key_id>/rotate` to obtain a new bearer. Do not repeat
creation. Rotation preserves immutable credential identity, workspace, ownership
and grants, and invalidates the prior bearer after successful local activation.
If rotation itself commits but activation fails or its response is lost, rotate
the same key again after resolving the problem; plaintext tokens are not replayed.

A lost connection/response (including an indeterminate database commit result)
does not prove rollback, and absence of saved headers alone is not proof either.
Discover the key by its globally unique name through the authorized list before
retrying. A duplicate-name rejection does not overwrite or rotate it. These are
single-instance activation outcomes, not cluster-wide acknowledgements.

### Direct Key Writes

`PUT` and `DELETE` on `/_internal/admin/keys/{key_id}` and `POST` on its
`/rotate` and `/revoke` subpaths require current visibility and key manager
authority. Each supplied `workspace_id` query parameter and `X-Workspace-ID`
header must match the key's workspace, even for global administrators. Omitting
selectors preserves authorized access by ID. An empty or malformed selector
returns `400` (`invalid workspace id`) before the key is looked up.

Missing keys, keys no longer visible to the caller after offboarding, other
hidden keys, and valid selector mismatches all return `404` with the plain-text
body `key not found` followed by a newline. A visible read-only share still
returns `403` (`key manager required`). Required membership lookups that fail
return `500` (`could not resolve workspace visibility`); key or sharing lookup
failures return `500` (`could not load key`). These denials save nothing, request
no activation, and include no save or activation headers. Scoped GET continues
to give the query selector precedence over the header.

### Key Policy And Sharing Updates

`PUT /_internal/admin/keys/{key_id}` accepts `description`, `tenant`,
`allowed_models`, `expires_at`, `user_ids`, and `team_ids`. Policy fields and an
optional sharing replacement commit in one database transaction. The update keeps
the key's name, workspace, ownership, bearer credential, and enabled state.

- Omitted or `null` fields preserve their current values. An empty `description`
  or `tenant` clears it; `allowed_models: []` removes the explicit list
  restriction, not the key's workspace restriction.
- `expires_at` accepts RFC3339; an empty string clears expiry. A past timestamp
  is valid and makes the key stop authenticating after successful activation.
- Supplying either non-null binding list replaces **both** user and team shares.
  The other omitted list is treated as empty. Supplying `user_ids: []` alone,
  for example, clears both sets. Omitting both lists preserves both sets.

Malformed UUIDs, nonmember users, and teams from another workspace return
`400`; missing keys/teams return `404`. Current manager authority and grant
eligibility are rechecked inside the transaction, serialized with workspace
offboarding and membership role changes. If manager authority is lost after the
initial access check, the transactional recheck still returns `403`.
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

Hosted requests require a workspace-scoped inference bearer.

- A DB bearer sees only its own immutable key/workspace usage, even if its labels
  overlap another key or HCL client, or its creator is a global administrator.
  Neither a `tenant` label nor an allowed-model list grants workspace-wide reporting.
- Label edits do not hide that key's earlier usage. Token rotation preserves its
  identity; deleting and recreating a same-named key gives it a new identity and
  no access to the deleted key's usage.
- Trusted HCL clients with a `tenant` see same-tenant **static** clients in System
  only, never DB keys with matching labels. Tenantless HCL clients see their own
  tenantless static usage. These file-only reporting contracts remain supported.
- File-only `auth.mode = "none"` retains unfiltered anonymous reporting. Hosted
  inference never uses this anonymous view.
- Workspace-wide or global reads require separate explicit management/operator
  authority. This endpoint does not accept scope-broadening query/header selectors
  or inherit browser roles from a key's creator.

Rows are grouped first by internal credential identity and explicit workspace,
then by the recorded tenant/client labels, requested public model, operation and
HTTP status. Earlier and later labels can therefore appear as separate own-key
rows. Credential/workspace identifiers are not emitted in these JSON summaries.
Ordinary and deferred completions use the identity, scope and labels captured at
admission, including after a reload, edit, rotation or deletion.

The optional `estimated_cost_usd` uses current configured model prices.
For an alias, it sums the recorded token usage of the targets actually used for
that alias's identity-scoped row. Other credentials/workspaces, another alias or
a direct model sharing those targets are excluded. Changing alias membership does
not redistribute past traffic; changing model prices reprices the retained estimate.
Hosted estimates use only current prices from the caller's workspace. If a
retained provider/model name is recreated in another workspace, its foreign
price is not used; the retained own-key row remains visible without an estimate.

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
This is a rolling usage/cost **estimate, not an invoice**, cross-instance total,
external billing service or quota system.
The dashboard's lifetime provider/upstream counters and the durable quota spend
ledger have separate lifetimes and pricing semantics.

Database user/team quotas are completion-based admission thresholds, **not hard
spending limits or funded workspace caps**. Ownerless DB keys and static/HCL or
anonymous traffic have no DB owner quota. Workspace reporting does not assign
a payer or grant workspace budget authority. See the
[spending contract](operations.md#spending-contract-owner-quotas-not-funded-workspace-caps)
for quota permissions, cache/stream overshoot, missing data and deletion gaps.

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

Provider PUT supports explicit `true`/`false` for `enabled` and
`forward_user_agent` (also `forward_authorization` and `allow_insecure_base_url`
in System), `[]` to clear local `forward_headers`, and `""` to clear
`display_name`, `base_url`, `upstream_header_timeout`, `user_agent` or `extends`,
subject to normal validation. Clearing an override restores applicable defaults;
it does not disable root-level forwarding. Omit untouched `api_key`, `api_key_ref`
and `credential_ref` fields to retain them. Supplied reference objects replace
their path and key/name together; an empty path selects the default secrets path.

### Tenant Provider Credential And Destination Policy

For non-System database providers, create, update and credential-only PUT
validate the **destination workspace's** effective configuration before
credential decryption/file resolution or persistence. Global administrators
authoring tenant rows do not bypass this policy. The operator-controlled
`multi_tenancy.allowed_upstream_origins` config attribute approves exact public
HTTPS upstream/probe origins; omission or `[]` denies all. It is not a writable
provider/API field and does not grant access to a System or foreign resource or
credential. See [configuration](configuration.md#tenant-managed-upstreams).

- Tenant `api_key` values remain write-only and encrypted in the database.
  Own Copilot database credentials still use the actor/workspace-bound device
  authorization flow, never a raw-token or foreign-credential input.
- Nonempty `api_key_ref` and sidecar `credential_ref` references are
  System-only, including the flat reference fields on credential PUT.
  `forward_authorization: true` and `allow_insecure_base_url: true` are likewise
  forbidden in tenant workspaces, even with an approved origin.
- Both effective upstream and health destinations must be approved; provider
  defaults and inherited endpoints/healthchecks cannot bypass the check.
  Disabled rows and old stored rows are checked too.
- A forbidden edit returns `400`, has no `X-Aiproxy-Catalog-Saved` header,
  requests no activation and leaves provider/models/credentials/revision
  unchanged. Correcting legacy intent may explicitly clear both members of a
  reference object and set reserved flags to `false`. Omission does not repair
  a forbidden old setting.

At connection time, tenant transports reject any non-public DNS answer and
pin dialing to the validated IPs, ignore environment proxies and keep TLS
verification and same-origin redirect limits. A syntactically approved
hostname can still fail at runtime because DNS or certificate verification
fails; successful save/preflight is not an upstream compatibility check.
Trusted System/HCL behavior is unchanged. Run the candidate binary's
[`validate --check-db` preflight](operations.md#tenant-provider-policy-upgrade)
before upgrading.

`allow_insecure_base_url` is a provider-only opt-in that accepts a non-loopback
plain-HTTP `base_url` (see
[plain-HTTP upstreams](configuration.md#allowing-plain-http-upstreams)).
The opt-in is **System-only** for database providers. Omitting the flag preserves
its current value on concrete providers. Enabling
it does not relax any other URL validation. Derived provider POST/PUT rejects
explicit `true` or `false`; setting a non-empty `extends` without declaring the
flag clears any previously stored local opt-in so the base's value is inherited.

`forward_authorization` is a System-only database provider opt-in for supported
bearer-header
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

Catalog POST/PUT/DELETE outcomes:

| Status                                                                                         | Meaning and next step                                                                                                                                        |
| ---------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `400`                                                                                          | Validation rejected the edit; correct the input.                                                                                                             |
| `400`, `catalog dependency conflict; retarget or remove dependent aliases and providers first` | The saved dependency graph would become invalid; retarget/delete referencers first. No rows changed and no activation was requested.                         |
| `409`                                                                                          | The edited aggregate changed concurrently; read current state and retry.                                                                                     |
| `500`, `could not save catalog edit`                                                           | Storage failed before a successful commit; activation was not requested.                                                                                     |
| `500`, `saved but activation failed`                                                           | The complete edit is saved; the previous runtime remains active. Consult server logs, correct the activation problem, then reload.                           |
| `500`, `saved but response view unavailable; read current state before retrying`               | The edit is saved and activation was requested successfully, but its response view could not be read. Read current state rather than blindly repeating POST. |
| `201` (POST), `200` (PUT/DELETE)                                                               | Saved, activation requested successfully, and any response view is available.                                                                                |

After each valid commit, activation is requested once before response-view reads.
Operational failure responses omit storage and credential details. Persistence and
runtime activation are separate: later reloads can activate a complete saved edit
after an activation failure. Aggregate revision checks reject stale edits; the
workspace database lock also serializes graph-affecting provider/model/alias and
Copilot aggregate writes. Dependency validation uses current saved definitions and
effective static System bases, never stale managed runtime inventory.
Successful provider POST/PUT/DELETE and credential PUT responses carry
`X-Aiproxy-Catalog-Saved: true` once the database commit is durable, including
when activation itself fails.

### Managed Catalog Dependency Edits

Provider deletion, disabling, model removal, concrete/derived transitions and
type/capability/protocol changes cannot invalidate an alias or derivative.
Every alias target must remain defined and enabled in the alias's destination
workspace, with at least one capability shared by all targets. Derivatives must
retain an enabled, concrete, same-type base; disabled derivatives still require
that base and inherited models named by local policy overlays. Aliases targeting
inherited models are checked when their base changes.

Retarget or delete referencers before removing the referenced provider/model.
There is no force-delete/cascade or automatic retargeting. Dependency errors are
generic and reveal no dependent names or counts. Rejection preserves all aggregate
rows, revisions and credentials, requests no activation, has no saved header, and
does not consume a ready Copilot device flow.

Only affected dependencies are validated during a save. Unrelated corrupted rows
remain repairable; they can still cause full runtime activation to fail **after a
valid commit**, which retains the saved-but-activation-failed contract above.
Static file edits and direct SQL are not covered by managed-write serialization.
This does not acknowledge activation on any other instance. See
[cleanup and upgrade guidance](operations.md#managed-catalog-dependencies-and-cleanup).

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
available for trusted System/HCL workflows, not tenant-managed providers.

- `POST /_internal/admin/copilot-device-flows` starts a flow with
  `{client_id, workspace_id?, provider_name?}`. With `provider_name`, the
  [direct catalog write rules](#direct-catalog-writes) check visibility, every
  supplied workspace selector, and write authority before loading the full
  provider or checking its type and policy. Missing, hidden, and selector-mismatched
  names return the same `404` (`provider not found`). Visible HCL providers cannot
  be edited here. A visible tenant provider requires workspace admin authority;
  an explicit non-global System member cannot start a flow for a System DB
  provider (`404`, `workspace not found`). A provider-bound tenant edit must
  also pass effective provider policy. These checks run before saving a flow,
  recording its start for throttling, or contacting the issuer. Standalone
  authorization (without `provider_name`) keeps the normal provider write-workspace
  selection and uses the same fixed issuer. Provider destination approval is
  checked before its credential can be saved. Successful starts return `201`
  flow-status bodies.
- `GET /_internal/admin/copilot-device-flows/{id}` returns the sanitized flow
  status without any upstream calls.
- `POST /_internal/admin/copilot-device-flows/{id}/poll` performs at most one
  upstream token request when the stored schedule and lease permit; early or
  competing calls return the pending status and delay with no extra upstream call.
  Provider-bound tenant policy is checked before claiming/decrypting the flow.
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
an explicit reread and retry. The ready flow's workspace and create/edit provider
binding are checked before decrypting its credential; an approved tenant
destination cannot consume a System or foreign-workspace flow, even if a global
author initiated it. Provider views expose
`copilot_credential_source` (`database`, `sidecar`, or `none` alongside the
existing `has_credential`), and `GET /_internal/admin/provider-types` advertises
`supports_device_authorization` for `github-copilot`. The legacy credential
endpoint switches sources in System the same way (a nonempty sidecar reference clears the
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
