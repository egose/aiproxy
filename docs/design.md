# AI Proxy Design

## Overview

This service is a Go-based proxy for multiple AI providers. It exposes an
OpenAI-compatible HTTP API to clients, selects a configured provider-backed
model or alias target, translates requests when needed, forwards them to the
upstream provider, and returns an OpenAI-compatible response.

The service is delivered as:

- a single Go CLI binary
- a container image exposing the service as a public HTTP API

Foreground `aiproxy serve` is cross-platform across the advertised release
targets. Daemon lifecycle commands (`serve -d`, `status`, `stop`, and
`restart`) are Linux-only because their safety checks depend on Linux process
identity primitives.

Configuration is written in HCL with an Alloy-like two-label block style.

## Interactive Dashboard Session Lifetime

The CLI owns signal handling and a single snapshot polling worker. Initial attach
errors return before starting the TUI. After attach, the worker uses a cancelable
session context and one resettable timer; it never launches overlapping snapshot
requests. Transient failures retain the last snapshot and back off from 2 to 30
seconds. Permanent endpoint/auth/config denial suspends automatic retries and gates
new payload/block RPCs until a successful snapshot probe. Credentials are immutable
for the attachment; changed credentials require re-attachment. Explicit Ctrl+R
requests are nonblocking, coalesced, and do not replay pane reads or decisions.

`dashboard.Program.Done()` closes after Bubble Tea returns and terminal cleanup
finishes. `Wait()` is repeatable and returns its result, including initialization
and I/O errors. `Close()` cancels and waits; it is idempotent and safe alongside
refresh/status sends. The CLI cancels and joins its poller on Program completion,
parent cancellation or signal. `RunWithOptions` supplies configured input/output,
a nonblocking `Retry` callback and `SignalsHandled` for callers owning OS signals;
the existing `Run`/`RunWithBlockFetcher` entry points remain available.

The model's `ctx` is the Program-owned child lifetime, canceled on every Program
exit. Every pane command captures that context and derives a ten-second deadline
through `fetchContext`; fetchers must honor it and release response bodies. Optional
parent arguments preserve standalone helper tests, which default to a bounded
background context. New pane commands must pass `m.ctx`, never start an independent
background retry loop, and must not mutate the model from worker goroutines.
`ConnectionStatus` is local session metadata (last successful receipt, reconnect/
denial state, next retry and sanitized guidance), independent of paused snapshot
data and the server clock. Generation/selection rules belong to each pane, and a
denied or canceled mutation is never automatically retried.

## Dashboard Measurement Boundaries

The aggregator keeps global rate counters in a fixed 901-slot second ring, separate
from both the 200-event recent ring and retained billing keys. At snapshot time,
`rates.window_end` is the aggregator clock truncated to a whole second. The one- and
five-minute counters cover `[end-60s,end)` and `[end-300s,end)`. The 15 graph points
are consecutive 60-second intervals, oldest first, covering `[end-900s,end)`.
The ongoing second is admitted but becomes visible only when complete (less than
one second of measurement lag, in addition to polling). Rates divide by the full
60/300 seconds even just after startup. Zero/future event timestamps use admission
clock time; late events count only in their retained second. Older events cannot
overwrite live ring slots. Reading expires idle slots; record and read both remain
bounded independently of traffic volume or identity cardinality. Errors exclude
429, which has its own counter. Tokens are recorded `TotalTokens`, not accumulated
billing tokens. These are completed-request metrics, including unresolved/errors;
they are not upstream attempt or in-flight counters.

`dashrpc.Snapshot` adds optional `rates` and `billing` objects. `billing` contains
`as_of`, `retention_seconds`, `bucket_seconds`, public `usage` and attributed
`upstream` from one locked accounting snapshot. Default billing retention stays at
24 hours with minute buckets: a bucket is retained while its start is at or after
`as_of-24h` (inclusive). This preserves the BOUNDARY billing contract, including
expiry up to almost a minute before an individual event reaches 24 hours.
The legacy `usage` field carries the same public rows; `provider_stats` and legacy
`upstream` remain lifetime counters. The TUI's upstream usage toggle instead groups
retained attributed rows by tenant/client/provider/model/operation/status.

`internal/usagecost` owns billing/TUI estimates at current configured model prices.
Alias attribution must reconcile every count/token field for the exact public
model, tenant, client, operation and status. No configured-target, count-weight or
even-split estimate substitutes for missing attribution. Used target price gaps
make the alias estimate unavailable; unused targets do not affect it. A provider
subtotal includes its direct and attributed alias traffic once and is unavailable
if any of its used models lacks the necessary price. A different provider's price
gap does not invalidate a fully attributed, fully priced subtotal. Incomplete alias
attribution conservatively invalidates provider subtotals, since the missing
provider cannot be inferred. Public billing retains tenant-first filtering, or
tenantless client-only filtering, before cost calculation; exact attribution
matching also preserves empty tenant/client dimensions.

Provider requests/errors/429/tokens are labeled global lifetime, including when a
provider becomes disabled. P95/n is nearest-rank P95 and the provider's positive-
duration sample count among the received recent completions (global cap 200), with
no time window. Idle time does not erase this sample. Global rate/provider metrics
ignore the usage tenant/error filters; usage and its estimates reflect them.
Absent old-RPC rate/billing fields stay unavailable, not zero; the TUI never falls
back to recent events for rates or lifetime upstream totals for retained costs.
Transported measurements are immutable snapshot data; local connection receipt
time and reconnect status remain independent.

## Dashboard Refresh Identity And Pause

Refresh anchors provider and usage top rows, and both selected and top rows for
aliases, payloads and blocks. Provider/alias names, request IDs and block IDs are
stable keys; usage keys include exact tenant/client/model/operation/status, never
counts or token totals. Anchors resolve against the displayed ordering (including
oldest-first payloads). Missing rows fall back to the old index clamped to the new
list. Viewport bounds and keeping selection visible take precedence over the top
anchor if reordering makes both impossible. Tenant selection retains its name
across snapshots; removal selects all tenants. Explicit local usage filter changes
start at the top. Payload order toggles retain selection; existing log order toggles
re-enable follow, while pinned log sequence IDs survive snapshot arrivals.

Pause is a model-loop data boundary. It freezes snapshot data, health, logs,
measurement windows, uptime/display time, pane lists, details and decision results.
Mutable in-process viewers are detached on pause; transported measurements remain
one immutable received unit. There is at most one pending snapshot, one pending
result for each of the five pane request classes, and one latest tick timestamp.
Paused ticks schedule the next tick but do not refresh pane lists. Snapshot polling
and separately labeled connection receipt/denial/reconnect status remain live.
Resume applies the newest snapshot, then accepted pane lists/details/acknowledgments,
and advances display time to the latest tick in one model update. These independent
RPCs have no server-side cross-endpoint transaction guarantee. Local time never
recomputes transported rates or billing windows.

Cached navigation, order and local usage/log filters work during pause. Remote list
refresh/filter, detail fetch and decision keys require resume; they are not queued.
Opening an unvisited pane while paused starts its list read on resume. Already
in-flight commands can finish into bounded buffers. Closing a detail discards its
buffer, cancels its request and invalidates that session, including acknowledgments;
a consumed take-once block capture is never automatically fetched again or replayed.

`requestSlot` starts cancelable child requests under the Program context, tagging
results with a monotonically advancing generation. The bounded helper deadline
still applies. Filter replacement cancels the old read immediately and starts a new
generation. Only the current generation and matching detail ID/action may apply;
completion advances the generation to reject duplicate results. Workers capture
inputs before launch and never read or mutate model state. Following provider/detail
and Requests/search workflows should reuse `anchoredIndex` and `requestSlot` with
their own identity and request classes.

## Dashboard Layout And Input Ownership

The supported minimum remains 80x12. `compactLayout` selects a focused-pane body
below 30 rows; `focusedLayout` also applies to explicit zoom. A normal frame reserves
one header line, two measurement lines and two footer lines before allocating body
space. Focused lists keep a one-line bottom-tab strip, including when Providers or
Usage has focus. Stacked layouts reserve at least twelve stats rows and six bottom
rows; pane resizing cannot violate these budgets. Detail consumes the body directly.
List visible-row helpers use the same geometry as rendering, so a selected record
cannot move outside the rendered rows. Identity reconciliation remains TUI-03's
boundary; resize/zoom/focus only clamp viewports and retain selection.

`paneFrame` receives total outer dimensions, fits content before Lip Gloss can wrap
it, and reserves its borders. Lip Gloss v2 `Height` includes borders. ANSI-aware
cell width/truncation/hard wrapping preserve wide and combining Unicode; fitting
does not slice UTF-8 or style escapes. Optional overflow hints yield to data rows
when the pane is full. Footer controls reserve their own width before contextual
hints, and the second line retains independent connection receipt/retry state.
Payload/block inspection uses the fixed-chrome wrapped detail workflow below.

`input.go` owns dispatch, with derived modes `inputBrowse`, `inputDetail`,
`inputHelp`, `inputSearch` and `inputTooSmall`. Ctrl+C always quits and Ctrl+R remains a nonblocking
connection retry. Help handles only scroll/close/quit plus that reserved retry;
other keys cannot reach hidden pane handlers. Esc unwinds help → detail → zoom →
quit. Enter opens/closes provider and bottom details, never zooms; z toggles list zoom
and is inert inside already-full-body detail. Tab/Shift-Tab changes focus, numbers
select bottom tabs, and brackets move previous/next in numbered order. These
navigation actions first call `closeDetail`, which invalidates/cancels remote
detail/decision slots and clears associated paused buffers. Block decisions have
only a/s/d bindings; numbers cannot mutate. Usage filters and log/payload order
keys require the corresponding focused pane. An undersized warning owns input too.

### Captured Text And Selected-Finding Decisions

`inspection.go` renders payload/block details with fixed ID/finding, scope/status
headers and a wrapped-row/cap footer, leaving at least one content row at 80x12.
All original within-cap text is reachable by line/page/Home/End navigation. Full
metadata is repeated in the body when its fixed header needs cell truncation.
Control bytes and invalid UTF-8 are escaped before wrapping; captured ANSI cannot
alter terminal state. Payload output keeps the existing 64 KiB byte cap and reports
pretty-output truncation and the three existing body truncation flags. Block capture
truncation is explicitly unknown: its existing wire shape has neither flags nor
original lengths. No completeness claim, reconstructed hash or new RPC is needed.

Block n/N cycles one selected finding. a/s/d sends a one-element hash list, requiring
an existing lowercase 64-hex SHA. The scope is persistent GLOBAL future matches,
never replay of the original request. Hash-equivalent findings share the effect.
While a decision is outstanding the selected finding is locked; duplicate actions
are suppressed. The request slot captures the ID/action/hash and lifetime context on
the model loop. Results must match generation, capture, action and hash before they
can change state or enter the bounded paused-result slot. Closing cancels/invalidates
and clears captured text, decision results and paused buffers; cancellation does not
promise rollback of an already committed server mutation.

An open detail retains at most one result per captured hash (bounded by its findings).
An acknowledgment is successful only for ok=true, the requested action and count=1;
success suppresses the same action on that hash even after finding navigation. A
different action deliberately replaces it. Error/invalid acknowledgments leave the
outcome unknown and permit explicit retry. No automatic mutation replay is introduced.
The list explains take-once consumption before Enter, and detail keeps consumption,
pending/error/success, scope and controls visible. Existing operator authorization
and detail-only body confidentiality remain the server boundaries.

### Input Contract For Requests/Search

`search.go` routes its derived `inputSearch` mode **after** reserved Ctrl+C/Ctrl+R
handling and **before** q/help/pause/navigation. It retains at most 256 printable
Unicode characters and one applied query per searchable pane. q/p/h/?/digits/brackets
are text; Esc cancels without changing the applied query, Enter applies, Ctrl+U
clears the editor (or the applied query in browse mode). Arrows/Home/End and
Backspace/Delete edit locally. The first footer row holds a cell-bounded, horizontally
scrolled prompt and apply/cancel/quit hints; the live connection line stays independent.
Help is available after leaving the editor; no edit key falls through to pane actions.

Queries AND whitespace-separated case-insensitive substrings. `field:value` limits
a term to one supported field; unknown fields match nothing. Bare terms search all
supported fields. Requests support id/client/tenant/model/resolved/provider/status/op,
Logs support structured id/level, and Payloads support id/model/resolved/provider/
status/method/path. Local queries combine with existing pane status/level filters,
use cached bounded lists and work while paused. Applying/clearing resets that list's
position; refresh anchors the currently displayed ordering. Payload responses still
use the existing generation/cancellation/pause slots; no search worker is added.

## Dashboard Recent Completions And Correlation

Tab 5 is Requests; tabs 1–4 keep their meanings. The pane reads the existing global
200-entry completion ring newest-first, independent of disk payload logging and rate
buckets. `accounting.Event.RequestID` and `PublicModel` are optional additive metadata
from the HTTP handler's deferred completion boundary. `Model` keeps existing billing
categories (including rejection sentinels); `PublicModel` preserves a submitted model
on rejection. Provider/UpstreamModel identify the final result's resolved provider and
**configured** model name, not an invented attempt or the provider's wire model name.
Pre-dispatch/transport failures can have no resolved target. Tokens are reported counts;
zero may mean unreported. HTTP status is the sent status, including for SSE streams.
Only recognized inference operations enter this ring, after response handling finishes.

The shared `ringBuffer.push` boundary stores a detached diagnostic copy **after**
full-event billing, rate and lifetime aggregation. Named byte budgets in
`accounting/recent.go` apply to every string: `RecentIdentityBytes=256` for RequestID,
Tenant, Client and Provider; `RecentModelBytes=512` for PublicModel, Model and
UpstreamModel; `RecentOperationBytes=64` for Operation. Thus
`RecentEntryStringBytes=2624` and the 200-entry ring owns at most 524,800 string
content bytes, plus fixed event/flag/allocator overhead. Even under-budget strings
are cloned, so short substrings cannot pin oversized caller backing allocations.
Over-budget valid UTF-8 retains the longest complete-code-point prefix within its
budget; no ellipsis is inserted into identity text. Under-budget values stay exact.
`RecentEntryJSONBytes=6*2624+1024` is a conservative per-event JSON allowance,
including worst-case string escaping and fixed metadata; the recent array is at
most `2+200*(RecentEntryJSONBytes+1)` (3,353,802) bytes. This is a bound on newly
retained diagnostic recent metadata, **not** total process heap, accounting keys,
catalogs, logs, other snapshot components, old-server data or total RPC size.

Optional capitalized RPC `Truncated` contains boolean field names (RequestID,
PublicModel, Tenant, Client, Model, Operation, Provider, UpstreamModel); absent/false
means no reported truncation, and old snapshots retain their existing fallback.
The fixed boolean struct keeps Event comparable. Optional `RecentSequence` and
`ProviderID` are uint64 decimal **strings** in JSON, avoiding JavaScript rounding.
The ring assigns the sequence to distinguish otherwise identical bounded completions;
it is local to that aggregator, not an external request/correlation ID. ProviderID
joins the existing exact-name `provider_stats[].ProviderID` for P95 grouping without
retaining full provider names in the ring. The aggregate registry is append-only;
snapshot builders read recent before provider summaries so every emitted ID can
resolve even during concurrent recording. The TUI resolves IDs before grouping;
legacy events use EventProvider, but truncated provider/model text without a mapping
is unavailable rather than an exact key. Durations/counts and all full accounting,
billing/filter/routing identities remain unchanged. MemoryRecorder remains an exact
event recorder, not the bounded production diagnostic ring.

Requests marks affected list rows `[truncated]` and detail enumerates shortened
fields; search matches only retained text. A truncated RequestID disables l/v
correlation and its action hints. Other truncated fields do not disable an exact
RequestID. Never use a truncated prefix or RecentSequence as an exact external
correlation key.

At the TUI presentation boundary, `metadataText` escapes data-owned LF/CR/tab/ESC,
all other Unicode control characters, Unicode line/paragraph separators and invalid
UTF-8 bytes before row composition or wrapping. Literal backslashes are doubled
(actual newline displays as `\n`, the two-character backslash-n string as `\\n`);
double quotes are escaped and printable Unicode/combining text/emoji joiners remain.
Application-owned row separators and styling are applied independently. Each Requests
completion occupies one logical summary row, so fitting cannot hide its selected
marker behind data-owned lines. Detail wraps the escaped text and remains scrollable.

`metadataDisplayBytes=512` limits source bytes processed per presentation value at
complete code-point boundaries, with at most 2,048 escaped bytes plus the explicit
`[display clipped at 512 bytes]` suffix when needed. Every newly retained recent
field fits without further clipping; oversized legacy/sibling values receive that
separate presentation notice. The helper also covers shared Usage identity detail,
correlation/filter labels, payload summary fields and payload ID titles. Captured
payload bodies retain their existing bounded multiline inspection contract. Stored
events, full grouping identities, raw search values and exact correlation keys never
use this display encoding; display clipping alone does not disable intact-ID
correlation or fabricate a retention flag.

Request detail copies one selected completion, so refresh/expiry never substitutes
another request. List anchors use completion metadata (including ID and timestamp),
which also distinguishes reused caller IDs; missing rows use the clamped old position.
No body, credentials, additional history or export joins this feed. Old snapshots use
available Model metadata and explicitly lack ID correlation. Usage Enter copies the
top visible exact tenant/client/model/operation/status group; n/N cycles all displayed
groups, including the final rows. Quoted identity text wraps at compact sizes, within
the explicit per-value presentation cap above.

Request-detail l/v enters an exact-ID Logs/Payloads list, retaining a single return
context (request/detail scroll/zoom and target list selection). Target queries and
status/level filters are temporarily bypassed and filter/order keys suppressed;
Esc closes target detail, then restores Requests detail. Explicit tab/focus navigation
abandons the return context. Log IDs come from the logger's pinned structured
`request_id` attribute through optional log JSON `request_id`, never parsed from
unescaped Attrs. Caller-reused IDs may correlate multiple entries; old log snapshots
without the structured field cannot correlate. Payload list reads remain capped at
100; correlation does not page backward or fetch arbitrary missing records. Missing,
disabled, expired/out-of-cap and unavailable metadata have distinct explanations where
the source can distinguish them. Paused correlation browses cached lists only;
remote reads require resume and use existing lifetime/generation guards.

## Dashboard Provider Diagnostics And Metadata

Provider selection is keyed by catalog name across both enabled and disabled lists.
The cursor and top-visible row reconcile independently, then selected-row visibility
wins if their anchors conflict. Detail stores the provider name, resolves against the
displayed snapshot and reports removal rather than silently inspecting another row.
It joins `hasDetail`/`closeDetail`, uses full-body wrapped pagination, and needs no
RPC command or request slot. Pause therefore freezes provider settings, health and
last-check age with the existing snapshot/display clock. Connection receipt stays live.

Additive dashboard RPC fields are optional: `Provider.diagnostics` contains the
header timeout and nullable probe configuration; `ModelPrice.details` contains
display/upstream names, protocol and capabilities; `Alias.session_affinity` contains
an explicit enabled flag and effective header names. Absent/null metadata means
unknown for old snapshots. A present diagnostics object with null probe explicitly
means no configured probe; absent model details do not imply a native protocol.
The remote snapshot keeps availability maps separate from reconstructed config.
These maps and transported data are immutable and catalog-bounded.

`HealthcheckStatus.last_checked` survives source serialization and remote conversion
unchanged. A zero/missing timestamp yields unknown age, and a future timestamp yields
clock-ahead unknown. Probe healthy/unhealthy describes threshold state; latest HTTP
status/reason describes the last attempt, so they need not agree during a threshold
transition. No status record is not proof that a probe is disabled. Provider-wide
lifetime counters are explicitly distinguished from alias/target counts, including
when a provider appears multiple times in one alias.

Diagnostic transport is an allowlist without credentials, credential references,
user-agent values, authorization settings or probe expected bodies. Endpoint/probe
URL sanitization removes userinfo/query/fragment, rejects invalid/opaque/non-HTTP
URLs, and preserves only standard API/health path segments; opaque segments become
`[redacted]`. Error text is reduced to safe status/body-mismatch messages or bounded
transport/timeout/DNS/refused/TLS categories before serialization. The TUI repeats
URL/reason sanitization for older servers and in-process snapshots. HOST displays
the sanitized configured hostname: DNS lookup/cache code was removed entirely, so
View, keyboard navigation and quit have no resolver dependency.

## Goals

- Accept OpenAI-compatible client requests.
- Proxy requests to multiple upstream AI providers.
- Support direct addressing of configured provider models.
- Support aliases that load-balance across multiple provider/model pairs.
- Keep the external API shape consistent even when upstream providers differ.
- Support streaming responses where listed in the current public API surface.
- Keep configuration static, explicit, and easy to validate.
- Package the service as a single binary and Docker image.

## Non-Goals

- Admin API for provider or alias management
- Provider-specific public APIs exposed directly to clients
- Global cross-instance balancing state
- Persistent request queueing
- External billing, invoicing, and quota systems
- Translated-provider image and audio endpoints

## Core Design Principle

The proxy terminates and rebuilds the request. It is not a blind relay.

Reason:

- upstream providers have different authentication schemes and endpoint shapes
- some providers require translation from OpenAI-compatible requests
- aliases must choose one concrete upstream model per request
- model naming exposed to clients is owned by the proxy, not by any single provider

The proxy must:

1. Parse and validate the inbound OpenAI-compatible request.
2. Authenticate the client if auth is enabled.
3. Resolve the requested model string to either a direct provider model or an alias.
4. Select one effective upstream provider/model target.
5. Translate the request into the provider-native format when required.
6. Send the upstream request using the provider credential.
7. Translate the upstream response back into an OpenAI-compatible response.

## API Surface

The current public API surface is:

<!-- docs-contract:public-matrix:start -->

| Surface                         | `openai`                           | `openai-compatible`                | `anthropic`                        | `gemini`                           | `opencode-zen`                           | `opencode-go`                            | `github-copilot`                   | `zenmux`                           |
| ------------------------------- | ---------------------------------- | ---------------------------------- | ---------------------------------- | ---------------------------------- | ---------------------------------------- | ---------------------------------------- | ---------------------------------- | ---------------------------------- |
| `GET /v1/models`                | Proxy-owned                        | Proxy-owned                        | Proxy-owned                        | Proxy-owned                        | Proxy-owned                              | Proxy-owned                              | Proxy-owned                        | Proxy-owned                        |
| `GET /v1/billing/usage`         | Proxy-owned local usage accounting | Proxy-owned local usage accounting | Proxy-owned local usage accounting | Proxy-owned local usage accounting | Proxy-owned local usage accounting       | Proxy-owned local usage accounting       | Proxy-owned local usage accounting | Proxy-owned local usage accounting |
| `GET /metrics`                  | Proxy-owned Prometheus metrics     | Proxy-owned Prometheus metrics     | Proxy-owned Prometheus metrics     | Proxy-owned Prometheus metrics     | Proxy-owned Prometheus metrics           | Proxy-owned Prometheus metrics           | Proxy-owned Prometheus metrics     | Proxy-owned Prometheus metrics     |
| `POST /v1/chat/completions`     | JSON and SSE                       | JSON and SSE                       | JSON and SSE translated            | JSON and SSE translated            | JSON and SSE native or translated subset | JSON and SSE native or translated subset | JSON and SSE                       | JSON and SSE                       |
| `POST /v1/embeddings`           | Yes                                | Yes                                | No                                 | Yes                                | No                                       | No                                       | No                                 | Yes                                |
| `POST /v1/responses`            | JSON and SSE                       | JSON and SSE                       | JSON and SSE translated subset     | JSON and SSE translated subset     | JSON and SSE native or translated subset | JSON and SSE native or translated subset | No                                 | JSON and SSE                       |
| `POST /v1/images/generations`   | Yes                                | Yes                                | No                                 | No                                 | No                                       | No                                       | No                                 | Yes                                |
| `POST /v1/audio/transcriptions` | Yes                                | Yes                                | No                                 | No                                 | No                                       | No                                       | No                                 | Yes                                |
| `POST /v1/audio/speech`         | Yes                                | Yes                                | No                                 | No                                 | No                                       | No                                       | No                                 | Yes                                |

<!-- docs-contract:public-matrix:end -->

Chat completions and responses support both standard JSON responses and
OpenAI-compatible Server-Sent Events where the matrix lists SSE support.
`GET /v1/billing/usage` is local in-process usage accounting over the proxy's
rolling window; it is not an external billing, invoicing, or quota system.

Future provider endpoints should reuse the same provider, model, alias,
credential, and adapter concepts rather than defining a separate config model.

## External Naming Model

Clients address models using proxy-owned names.

Supported public model forms:

- `<provider-name>/<model-name>`
- `alias/<alias-name>`

Examples:

- `openai/gpt-4o-mini`
- `gemini/gemini-2.5-pro`
- `alias/chat_default`

Name rules:

- provider names must be lowercase
- alias names must be lowercase
- names must not contain spaces
- provider and alias names must not contain `/`
- provider name `alias` is reserved for `alias/<alias-name>` routing
- model names may contain `/` when every slash-separated segment follows the
  same lowercase name rule

Direct model resolution splits on the first `/`, so slash-containing model names
remain unambiguous under `<provider-name>/<model-name>`.

## Request Model

The proxy should normalize inbound requests into an internal request context.

```go
type RequestContext struct {
    Method        string
    Path          string
    Headers       http.Header
    RequestedModel string
    Stream        bool
    Operation     Operation
}
```

Supported operations reuse the same resolver and adapter pipeline for chat
completions, embeddings, responses, image generations, audio transcriptions, and
audio speech.

## Authentication

Authentication is intentionally separate from upstream provider credentials.

Initial auth modes:

- `none`
- `bearer_static`

### `none`

No inbound authentication is performed. This mode is intended only for trusted
deployments.

### `bearer_static`

The proxy validates the inbound `Authorization: Bearer ...` token against
statically configured client credentials from HCL.

With multi-tenancy enabled, database inbound keys are also matched by token hash.
Catalog merge copies each enabled, not-yet-expired key's deadline into the live
authenticator as a value, alongside its principal. Authentication snapshots own
their credential maps and expiry values; changing a database row or merge input
cannot mutate an already-published snapshot. A zero expiry value represents a
non-expiring key.

Each authentication checks the matching key's deadline against the server clock:
at or after `expires_at`, it returns the standard invalid-client-token error
(`401`, `auth_failed`) without database I/O, a timer, or a catalog reload. The
clock is supplied at construction for deterministic boundary tests and defaults
to `time.Now`. This gate covers model listing, billing usage, and inference,
including requests for SSE. Already-authenticated requests and streams may finish
after expiry. Static HCL clients and database keys without expiry remain valid.
Rotation, disabling/deletion, and edits to expiry still activate through the
existing catalog reload path (automatically after admin API mutations or on
`SIGHUP`); a failed reload preserves the previous snapshot and its deadlines.

Each static client may also define:

- optional `tenant`
- optional `allowed_models`

`allowed_models` applies a static allow-list against the proxy-visible model
name, including both direct `<provider>/<model>` strings and `alias/<name>`.

The proxy also records in-process accounting events keyed by:

- tenant
- client
- model
- operation
- status

These events are also aggregated in-process by the same key dimensions over a
rolling 24-hour window to form the first billing/accounting scaffold. The
window is maintained with bounded one-minute buckets whose start times are in
the rolling window, rather than lifetime totals or idle-key eviction.

`GET /v1/billing/usage` exposes those aggregated summaries. In static bearer
auth mode, responses are scoped to the caller's tenant when present, otherwise
to the caller's tenantless client identity. A same-named client in another tenant
does not enter that scope.

Billing buckets additionally distinguish the requested public model from the
resolved provider and configured target model (not its wire `upstream_name`).
`BillingSummaries` returns public usage and attributed upstream usage from one
locked, pruned snapshot. Both views therefore have identical retention, including
under concurrent recording and at the window boundary. Alias cost sums only the
targets actually recorded for that exact public model, tenant, client, operation,
and status; direct traffic and other aliases sharing a target cannot contribute.
Current alias membership is not used to reconstruct historical routing. A removed
target can still be priced if its provider/model price remains in the current catalog.

The one-minute bucket cutoff is inclusive: a bucket is removed when its start is
strictly older than `now - 24h`. This can expire an event up to one minute before
its exact age reaches 24 hours. Recording and billing reads prune the same buckets;
late expired events do not retain keys and future timestamps are clamped to the
recorder clock for bucket placement. At most 1,441 minute buckets remain at a time;
entries scale with distinct identity/public-model/target/operation/status combinations
within that window, not event count. Idle expired state is reclaimed on the next
record or billing read. The test-only memory recorder retains all its events.
Provider and upstream dashboard counters (`ProviderSummaries`/`UpstreamSummaries`)
remain lifetime counters with their existing aggregation dimensions; the recent
event ring also retains its independent bounded policy.

`estimated_cost_usd` is an optional estimate using **current catalog prices**, not
historical price snapshots or an upstream invoice. For aliases, every retained
request and token counter must reconcile with attributed target usage, and every
contributing target must have pricing. Otherwise the field is omitted for the
entire row, never emitted as a partial sum or an even/request-weighted split.
An unused unpriced alias target does not prevent an estimate. Both direct and alias
estimates require positive rates for consumed token categories; cache-write and
explicit cache-read tokens retain the pricing model's input-rate fallback. Generic
cached tokens require a cached rate. Configuration does not distinguish omitted
rates from explicit zero rates, so a zero applicable rate is conservatively treated
as incomplete for this endpoint. Zero recorded usage with an otherwise priced,
fully attributed target can still produce a zero estimate. This does not establish
that upstream usage reporting was complete. Dashboard estimation and durable quota
ledger pricing retain their separate semantics.

### Database Key Spend Lifetime

Multi-tenant quota spend is persisted separately from the rolling in-process
billing view. Migration `000008_preserve_key_spend.sql` gives each database key a
durable `spend_key_identities` row containing only its UUID and workspace UUID.
Existing keys are backfilled; an insert trigger creates future identities in the
same transaction as the credential. The migration locks key writes while
backfilling and changing the ledger foreign key, so concurrent key creation or
deletion cannot leave a gap. Applied migrations are unchanged.

The ledger references `(key_id, workspace_id)` in this identity table instead of the
live credential. Key deletion therefore preserves both existing entries and the
ability to insert usage from an already-admitted request, including SSE completion.
Unknown keys and cross-workspace key references still fail at the database
boundary. No HTTP-layer fallback, ignored foreign-key error, or join against live
keys is needed. Existing store insertion and aggregation methods retain their
contracts, including per-key attribution after deletion. Legacy ledger rows with
inconsistent key/workspace pairs cause an atomic migration failure rather than
silent reattribution.

Each entry keeps the workspace and user/team owner captured by authentication;
sharing a key does not charge its other users or teams. Replacing a deleted key,
including reuse of its name, creates a new identity without restoring the owner's
budget. The 30-second spend cache and new tracker/store instances read the same
durable scope totals. A workspace administrator's explicit `reset_spend` sets
the quota offset to the sum visible at reset; it deletes no ledger entries or
identities. Admitted requests recorded after that sum count against the reset
budget even if their key has since been deleted.

Identity rows live for the workspace's lifetime, including keys with no spend,
and contain no token hashes or other credentials. Workspace deletion still
cascades to ledger and identities; user/team deletion retains the existing
`SET NULL` ownership behavior. This migration cannot recover spend already erased
by older key deletions. Accounting remains completion-based with the existing
cache lag and possible concurrent budget overshoot; database failures are logged
by the caller and are not covered by a durable retry queue.

### Quota Read Availability

Admission reads budget and public-model TPM policy for each database key owner
before dispatch. Only `sql.ErrNoRows` means absent policy; other read errors
propagate to a controlled JSON `503 quota_unavailable`, including SSE requests.
The server logs the underlying cause without returning it to the client.
Confirmed budget and TPM denials retain `403 budget_exceeded` and
`429 tpm_exceeded`. Static credentials bypass this database-owned quota gate.

Spend is required for admission only when the budget is positive. Cached spend
is usable for the existing 30-second TTL; a failed cold/expired read publishes
no value and never extends the prior cache entry. Policy reads are not cached.
Admin views aggregate spend independently of whether a budget row exists and
propagate read failures rather than presenting zero. Admin updates initialize
absent policy only on `sql.ErrNoRows`; operational errors stop that write.
Storage errors in admin quota routes are logged and returned as controlled
`500` messages, explicitly identifying saved edits if only the response-view
read failed. This does not introduce multi-row quota-update transactions or
change completion-based accounting, spend-cache lag, or ledger-write retries.

### Administrative Membership Transactions

Workspaces own tenancy: tables `workspaces`, `workspace_members`, and
`workspace_teams` joined by `workspace_id`, served at
`/_internal/admin/workspaces` with `X-Workspace-ID` selection. Kinds are
`personal` and `organization` plus the `system` flag. A `personal` workspace is
auto-created exactly once per user at registration and rejects teams and extra
members; `organization` workspaces are created afterwards for collaboration; the
`system` workspace owns global inventory and cannot be deleted.

`internal/store/membership.go` owns insert-only `AddMembership`/`AddTeamMember`,
explicit `SetMembershipRole`/`SetTeamMemberRole`, and membership removals.
`SetMembershipRole` requires the initiating actor ID for the existing workspace
self-demotion rule; callers authorize the actor before invoking the store.
Team self-demotion is permitted with a remaining administrator. Duplicate adds
return `ErrMembershipExists`, never mutate roles, and map to actionable HTTP 409.
Role/removal guard errors map to HTTP 400; missing rows map to 404. Initialization
may add members to an adminless workspace/team; an established admin set cannot be
reduced to zero by a membership transition or user-deletion cascade.

Every membership transaction uses PostgreSQL READ COMMITTED. The shared mutex is
the containing workspace row, acquired by `lockMembershipWorkspace` with
`FOR NO KEY UPDATE` and held through commit/rollback. Current role and admin count
are read **after** the lock; counts from route prechecks are not used. One workspace lock
also serializes its teams, avoiding team-first lock ordering and allowing atomic
multi-team offboarding to use the same boundary. `lockTeamMembershipWorkspace` resolves
the immutable team workspace, takes the workspace lock, then rechecks that the team exists in
that workspace. No team-reparenting API is supported.

Membership additions first acquire `lockMembershipUser` (`FOR KEY SHARE`), then
the workspace lock; team additions check current workspace membership inside that critical
section. `DeleteUser` takes the user's `FOR UPDATE` lock before discovering and
locking affected workspaces in ascending UUID order. This prevents an addition
from slipping into its cascade after discovery. All workspace/team administrator checks
and the user delete share one transaction, so a failed handoff preserves the whole
account. Do not acquire a user lock after a workspace membership mutex, and do not call a public
transaction-opening membership method from an existing membership transaction.

Invitation acceptance reads the current scope and takes a KEY SHARE lock on its
workspace before locking the invite, preventing inversion with workspace
deletion's parent-to-invite cascade. It rejects an intervening scope change before
writing and checks expiry using database wall time after the invite lock. The
parent lock permits the later membership NO KEY UPDATE mutex and does not block
other membership transactions. Its new user is private to the acceptance transaction;
existing-user additions retain the user-before-workspace-mutex order.
Acceptance calls transaction-local `insertMembership`
after locking the current invite and creating its new user. Registration uses it after inserting
a new user and new workspace. Neither can change an existing membership role. Bootstrap
uses `AddMembership` and ignores only the explicit duplicate error, preserving
its idempotent insert-only semantics; personal-workspace/OIDC initialization uses the
same add boundary. Whole-workspace/team deletion destroys that scope rather than
transitioning a surviving membership set.

`DeleteMembership` takes the workspace lock and checks every affected team membership
with `protectTeamAdmin` before deleting anything. It atomically deletes that user's
team memberships in the workspace, direct user shares on the workspace's keys, and workspace
membership. A sole-team-admin refusal requires an explicit handoff and leaves all
memberships/shares intact; a storage failure rolls back the entire cleanup. Team
additions, promotions, demotions and removals contend on the same workspace lock. Acquire
multiple workspace locks in UUID order if needed; no separate team locks are required.
Raw SQL fixture teardown may bypass these product invariants only in tests.

Non-system callers need current containing-workspace membership at shared key management
and team-admin gates, key detail visibility, and team quota GET/PUT admission.
Historical surviving team/share rows alone cannot authorize access. The admission
check does not retroactively cancel an operation already admitted before removal.
Offboarding preserves other workspaces, key ownership, team-wide key sharing, quotas and
historical spend. Ordinary re-add restores no team membership/admin role or direct
share; preserved personal ownership becomes usable again with current membership.
API bearer credentials are separate from account management access: offboarding
does not rotate, revoke or disable keys, including personal keys. Operators must
explicitly rotate/revoke previously distributed credentials when required.

### Atomic Key Policy Updates

`UpdateInboundKeyPolicy` accepts a field patch plus optional replacement of both
sharing sets. PUT parses/validates the complete request before invoking it, and
the store rechecks grant eligibility and the actor's current authority. Missing
policy fields stay untouched; null has the same omission semantics. Either
non-null binding list replaces both sets (an omitted counterpart is empty), while
omitting both preserves sharing. `SetExpiry` distinguishes preserving expiry from
clearing it. The key's identity, owner, and credential are never policy columns.
Rotation and revocation write only their credential/enabled columns so stale
route snapshots cannot overwrite a concurrent policy commit in either order.

The READ COMMITTED transaction locks referenced users plus the actor with KEY
SHARE in UUID order **before** the containing workspace's NO KEY UPDATE lock. This
matches user deletion's user-before-workspace order. After the workspace lock it revalidates
user membership and locks proposed teams with KEY SHARE in UUID order, before
locking/re-reading the current key with NO KEY UPDATE. Team-before-key avoids
inverting team deletion's cascade into key ownership/shares. It then checks the
current active account, system/workspace admin status, personal ownership or team-admin
membership; non-system actors must still belong to the workspace. Membership role edits
and offboarding share the workspace lock, so they cannot invalidate a checked grant or
manager before commit. Standalone `SetKeyBindings` uses the same grant validation
and lock path, including the creation caller. Raw store callers of that method
remain responsible for actor authorization.

All policy writes and both sharing deletes/inserts use this transaction; any
failure rolls back fields and grants together. The HTTP handler activates once
only after commit, before querying response views. Rejected mutations never
activate and cannot reappear on a later reload. A valid commit followed by reload
failure keeps the saved state and returns the existing `saved but activation
failed` response while the old runtime remains active. Key creation's broader
multi-step persistence/activation flow is outside this PUT transaction contract.

### Catalog Aggregate Writes And Publication

Provider creation commits metadata and models in one store transaction. Provider
updates commit metadata and an optional model replacement together; omitted/null
models preserve the existing model rows, including their identities. Credential
PUT uses the same provider write boundary without replacing models. Alias parent
and target writes remain transactional. Failed writes do not publish generated
IDs/timestamps into caller objects or mutate caller model/target slices.

Provider and alias writes compare the previously read `updated_at` revision in
their SQL UPDATE. A mismatch or concurrently deleted row returns a conflict rather
than overwriting newer state. Revisions advance by at least one PostgreSQL
microsecond. Standalone provider-model replacement updates/locks the parent first
and advances its revision in the same transaction. Thus a provider's metadata and
model reads used for validation cannot be committed over a concurrent aggregate
or credential edit. A conflict requires a fresh read and validation; this also
preserves omitted fields in alias updates. Direct SQL writers must maintain this
revision protocol. These locks do not change the LIFE membership/key lock order.

Catalog create/update handlers separate persistence from fallible presentation:
after commit they request activation exactly once, then query the response view.
Validation returns 400, concurrent edits return 409, and storage errors return a
controlled 500 (`could not save catalog edit`); none requests activation. Activation
failure returns `saved but activation failed`, retaining the complete DB edit and
old runtime. A subsequent view failure returns `saved but response view unavailable;
read current state before retrying`. Storage/activation/view causes are logged
server-side rather than included in public failure responses. Runtime publication
and database persistence are separate operations, not a distributed transaction:
dependency edits may still cause activation failure, and later valid reloads load
the latest saved catalog. A response view may reflect a subsequent committed edit.

Database catalog merging receives the runtime's root defaults on startup, reload
and CLI database validation. Concrete rows inherit an omitted timeout/user-agent,
OR root/local user-agent forwarding and union root-first forwarded headers using
the static provider rules. Derived rows inherit the already-resolved base settings;
their enabled state and credentials remain local. Defaults are never persisted into
the database row or its admin response view, so clearing a local override and later
changing a root setting remain effective without rewriting saved providers.

### Optional Local Rate Limit

The `auth` block may include a `rate_limit` sub-block:

- `requests_per_minute`
- optional `burst`, defaulting to `requests_per_minute`

The current implementation is local to a single proxy instance.

- In `bearer_static` mode, the limiter is keyed by authenticated client name.
- In `none` mode, the limiter applies to a shared anonymous bucket.

Exceeded requests return `429 Too Many Requests` with `Retry-After`.

Deferred auth features:

- token rotation
- external auth integration

## Resolution Model

### Direct Provider Model Resolution

If `model` is in the form `<provider-name>/<model-name>`, the proxy resolves the
request directly to the configured provider and model.

### Alias Resolution

If `model` is in the form `alias/<alias-name>`, the proxy resolves the alias and
selects one target from the alias pool.

Each alias target is a concrete pair:

- provider name
- model name

Alias targets must all be valid configured provider/model pairs.

## Provider Model

Providers are configured with two HCL labels:

- first label: provider type
- second label: provider name

Format:

```hcl
provider "<type>" "<name>" {}
```

Initial provider types:

- `openai`
- `openai-compatible`
- `anthropic`
- `gemini`
- `opencode-zen`
- `opencode-go`
- `github-copilot`

Additional types can be added later without changing the external client API.

Provider attributes:

- `display_name`
- `base_url`
- `api_key`
- `api_key_ref`
- `credential_ref` (`github-copilot` only)
- `extends`
- `upstream_header_timeout`
- `enabled`
- nested `model` blocks

Provider inheritance is resolved during configuration loading. A provider may
declare `extends = "<base-provider-name>"` to inherit the base provider's type,
`base_url`, effective upstream header timeout, enabled state, and complete model
inventory while using its own provider name and credential.

Derived provider blocks are intentionally restricted:

- the type label remains mandatory and must match the base provider type
- the base must exist, be enabled, and must not itself declare `extends`
- declaration order does not matter
- `display_name` may override the inherited display name
- exactly one local credential form, `api_key` or `api_key_ref`, is required
  (`github-copilot` derivatives require a local `credential_ref` instead and
  reject `api_key`/`api_key_ref`; `credential_ref` is rejected on all other types) # pragma: allowlist secret
- local `base_url`, `upstream_header_timeout`, `enabled`, and `model` blocks are rejected

After loading, derived providers are ordinary providers. Direct routing,
health, metrics, billing, dashboard inventory, reloads, and aliases identify
them by their own provider names. Aliases do not expand inherited providers;
each target must still list the concrete provider name and model.

### Provider Type Semantics

#### `openai`

Well-known built-in adapter for OpenAI's API.

#### `openai-compatible`

Adapter for providers that already expose an OpenAI-compatible API surface.

This type requires:

- `base_url`

The proxy can mostly pass through OpenAI-compatible request and response bodies
for this provider type, while still applying model resolution, auth, metrics,
and error normalization.

#### `anthropic` and `gemini`

These provider types require explicit request and response translation between
the public OpenAI-compatible API and the provider-native API.

In the current implementation:

- `anthropic` supports translated chat completions and responses
- `gemini` supports translated chat completions, responses, and embeddings

#### `opencode-zen` and `opencode-go`

These provider types share one adapter behind two explicit types. The type
selects the service, never the URL or credential:

- `opencode-zen` defaults to `https://opencode.ai/zen/v1`
- `opencode-go` defaults to `https://opencode.ai/zen/go/v1`

`base_url` is an optional transport override only (same absolute-URL and
loopback rules as other providers); an override never reclassifies the
service, so auth, header, and protocol behavior stay type-driven.

Every model declares a required `protocol` of `chat`, `responses`,
`messages`, or `gemini` (`gemini` is Zen-only and rejected on `opencode-go`):

| `protocol`  | Upstream request                                                                                          | Serves public operations |
| ----------- | --------------------------------------------------------------------------------------------------------- | ------------------------ |
| `chat`      | `POST <base>/chat/completions`, model rewrite, JSON/SSE pass-through                                      | `chat` only              |
| `responses` | `POST <base>/responses`, model rewrite, JSON/SSE pass-through                                             | `responses` only         |
| `messages`  | `POST <base>/messages`, existing Messages translation subset                                              | `chat` and `responses`   |
| `gemini`    | `POST <base>/models/<upstream>:generateContent` (JSON) / `:streamGenerateContent?alt=sse` (SSE), Zen only | `chat` and `responses`   |

A public operation the model's protocol does not serve, and any
`embeddings`, `images`, or audio operation on both OpenCode types, is rejected
before upstream I/O; no chat-to-Responses or Responses-to-chat conversion is
performed. The same model name may use different protocols on each service
(for example `minimax-m3`), so routing comes from explicit configuration,
never from the model name.

Every upstream request sends `User-Agent: aiproxy/<version>` unless the
provider declares a `user_agent` override (or `forward_user_agent = true`,
which forwards the inbound caller `User-Agent` on live inference requests),
and
`Authorization: Bearer <key>` (omitted for keyless Zen providers). Both services additionally send
`x-opencode-session` for prompt caching: a caller-supplied inbound value is
forwarded as-is when it is 1-128 characters of `[A-Za-z0-9_-]`; otherwise a
valid caller `X-Session-Id` is adopted when present, and only then does the
proxy generate a fresh per-request `ses_` + 128-bit hex ID. A caller-supplied
`x-opencode-client` is forwarded under the same validity rule. Missing or
invalid values never fail the request and never create shared cross-client
state. No other inbound headers or credentials are forwarded, and secrets
never appear in errors or logs.

Direct requests never cross services. Upstream Go quota/limit errors are
returned like any other upstream error; only explicitly configured aliases
retry another target, and the proxy never reroutes between services on its
own. The upstream console setting that spends Zen balance past Go limits is an
account setting, not proxy routing permission. Model catalogs are static
configuration; the proxy performs no runtime catalog sync.

#### `github-copilot`

**Hermetically verified; live GitHub compatibility unverified.** The implemented
direct-Bearer/path/header contract is exercised against synthetic local fixtures.
It does not establish application eligibility, GitHub model availability or
exchange/refresh requirements. [COPILOT-LIVE-01](tasks/20260927-102347-copilot-live-compatibility.md)
retains those deferred questions. See the [mock-only workflow](../website/docs/operations.md#mock-only-copilot-verification)
for protocol-unit, composed command/App and real-binary coverage boundaries.

Chat-only device-flow provider (GitHub.com release scope). Defaults to
`https://api.githubcopilot.com`; `base_url` is an optional transport override
only (same absolute-URL and loopback rules as other providers) that never
changes auth or header behavior. Models declare no `protocol` (rejected);
default and supported capabilities are `chat` only.

Provisioning is explicit and headless-friendly:
`aiproxy login github-copilot --client-id <id> --credential <name>` uses the
GitHub device authorization grant with the operator-supplied public client ID
(no secret, no endpoint-override flags), polls with server `interval` /
`slow_down` handling, then persists a structured sidecar
`<secrets-dir>/copilot-<name>.json` (`0600`, restrictive directory, atomic
rename) holding `{client_id, domain, access_token, refresh_token, expires_at,
obtained_at}`. Login never edits HCL, never signals a server, and never prints
tokens; it reports persistence separately from activation.

Providers reference the saved login with `credential_ref { path?, name }`
(`path` defaults to the shared secrets path). The token resolves at load into
a dedicated field (never `api_key`), activates on restart/`SIGHUP`, and
sidecar-only changes leave a running server untouched until reload; failed
reload candidates keep the old runtime. Upstream `401`/`403` requires operator
re-login with the same client ID/name, then reload. Inference preserves upstream
JSON auth errors; `models --upstream` adds an explicit re-login hint. Listing loads
the current sidecar each invocation, independently of the running server's runtime.

Database catalogs additionally support an internal structured credential in
`db_providers.copilot_credential_encrypted` (migration `000009`). Its plaintext
envelope is `{version: 1, kind: "github-copilot", credential: Credential}`; the
nested credential has the same fields as the CLI sidecar. Store helpers
`EncryptCopilotCredential(credential, now)` and
`DecryptCopilotCredential(ciphertext, now)` use existing `EncryptSecret` /
`DecryptSecret` AES-GCM and the existing encryption-key/JWT-secret fallback.
Both validate fixed `github.com` domain, client ID, nonempty printable tokens,
timestamps and expiry; decryption also rejects unknown envelope fields, unsupported
versions/kinds, malformed data and incorrect keys. Failures return the controlled
`store.ErrCopilotCredential`, never payload data or raw crypto/JSON errors.
The caller supplies the validation time (runtime uses current time; transactional
callers should use DB time). These helpers do not write rows or consume sessions.

The nullable dedicated column is exclusive with sidecar and API-key sources and
valid only for `github-copilot`; SQL enforces this for new encrypted credentials,
and runtime validates source combinations before decrypting. Existing rows remain
unchanged with NULL in the new column. BuildProvider decrypts and validates even
disabled stored DB credentials, placing only the access token in `CopilotToken`.
Disabled providers may still omit credentials; disabled sidecars are not read.
Derived providers clear all base credential fields before attaching their own
local DB/file source. Resolved sidecar ref plus token remains legitimate. Root
defaults and local enabled/display-name behavior are unchanged. Static HCL/JSON
still accepts only sidecar references, not raw tokens or encrypted blobs. Admin
views omit secret material. Public device sessions and provisioning handlers are
subsequent WEB-COPILOT tasks, not part of this storage/runtime foundation.

Inference is `POST {base}/chat/completions` with model rewrite and shared
JSON/SSE pass-through handling. Upstream headers are an allowlist only:
`Authorization` from the stored login, proxy `User-Agent`,
`X-GitHub-Api-Version`, `Openai-Intent`, derived `x-initiator: user`, and
`Copilot-Vision-Request` only when the request body structurally contains
image parts. Inbound authorization, cookies, `x-api-key`, and caller-supplied
Copilot metadata are stripped, never trusted. Unsupported operations are
rejected before auth/inference I/O; direct failures never change targets and
aliases follow the standard configured status policy.

#### `zenmux`

OpenAI pass-through gateway. Defaults to `https://zenmux.ai/api/v1`;
`base_url` is an optional transport override only (same absolute-URL and
loopback rules as other providers). Models declare no `protocol` (rejected);
default capabilities are `chat`, `responses`, `embeddings`, with additional
support for `images`, `audio_transcriptions`, and `audio_speech`.

## Model Model

Each provider contains one or more nested `model` blocks:

```hcl
model "<name>" {}
```

Model attributes:

- `display_name`
- `upstream_name`
- `protocol` (required on `opencode-zen` and `opencode-go`, rejected elsewhere)
- `capabilities`

Semantics:

- the model block label is the proxy-visible model name
- `display_name` is optional metadata for humans
- `upstream_name` is optional and defaults to the model block label
- `capabilities` is optional; when omitted, the proxy derives default
  capabilities from the provider type

Using `upstream_name` avoids coupling the proxy-visible model name to the exact
string sent to the upstream provider.

Capability values:

- `chat`
- `responses`
- `embeddings`
- `images`
- `audio_transcriptions`
- `audio_speech`

Default capability behavior:

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
| `zenmux`            | `chat`, `responses`, `embeddings`          | `images`, `audio_transcriptions`, `audio_speech` |

<!-- docs-contract:capability-matrix:end -->

If `capabilities` is set on a model, it replaces the default capability set for
that model. The config validator rejects capability values that the provider
type cannot actually serve. On `opencode-zen` and `opencode-go` the omitted
default is protocol-aware (`chat` serves `chat`, `responses` serves
`responses`, `messages` and `gemini` serve both), and any capability outside
the protocol-served set fails validation.

## Alias Model

Aliases are configured with one HCL label.

Intent:

- an alias is a virtual model exposed by the proxy
- it is not just a rename
- it can represent a load-balanced or failover-backed pool of concrete provider/model targets

Aliases are configured with one HCL label:

```hcl
alias "<name>" {}
```

Alias attributes:

- `algorithm`
- optional nested `session_affinity` block
- nested `target` blocks

Each `target` block contains:

- `provider`
- `model`

Example:

```hcl
alias "chat_default" {
  algorithm = "round_robin"

  session_affinity {
    headers = ["x-opencode-session", "x-claude-code-session-id"]
  }

  target {
    provider = "openai"
    model    = "gpt-4o-mini"
  }

  target {
    provider = "localai"
    model    = "qwen3-32b"
  }
}
```

The `session_affinity` block is optional; omitting it disables affinity and
keeps pure `algorithm` routing. When present with no `headers`, the proxy uses
a built-in default list covering opencode (`x-opencode-session`,
`x-session-affinity`, `x-session-id`), Claude Code
(`x-claude-code-session-id`), and Codex (`session-id`, `thread-id`,
`x-codex-window-id`, `x-client-request-id`) session headers. A custom
`headers` list overrides the defaults.

Aliases can serve any operation included in the intersection of their targets'
effective capabilities. Operators should only combine targets that are safe to
use interchangeably for the operations exposed through that alias.

When the proxy renders `GET /v1/models`, alias capability metadata is the
intersection of the effective capabilities of every target in the alias pool.
This avoids advertising `responses` or `embeddings` on an alias unless every
target behind it can actually serve that operation.

The `GET /v1/models` response should also expose human- and operator-friendly
metadata:

- direct provider-backed models include:
  - `display_name`
  - `provider_type`
  - effective `capabilities`
- aliases include:
  - effective `capabilities`
  - `alias_targets` summaries containing provider name, model name, and
    resolved display name for each target

## Load Balancing And Failure Policy

### Algorithms

Initial alias algorithms:

- `round_robin`
- `least_connections`

Algorithm values are lowercase machine-friendly enums.

### `round_robin`

Requests rotate across alias targets in process-local order.

### `least_connections`

The proxy selects the target with the fewest currently active in-flight
requests.

This is:

- per-process
- best-effort
- not coordinated across multiple proxy instances

### Session Affinity

An alias pool may pin sessions to a stable target for upstream prompt-cache
locality. The first non-empty header in the alias's configured precedence
selects the session key; the key hashes to a preferred pool target. Affinity
is a hint, never a guarantee: a cooling, unhealthy, or already-tried preferred
target falls back to normal `algorithm` selection, and retry failover proceeds
across the remaining pool exactly as without affinity. Requests without any
affinity header use normal selection directly.

This is:

- per-process and stateless (no session table, nothing to expire)
- best-effort
- not coordinated across multiple proxy instances

### Failure Handling

If the chosen alias target fails, the retry policy is:

- do retry another alias target on transport errors and timeouts
- do retry another alias target on upstream statuses listed in
  `retry_status_codes`; the default is `500`, `502`, `503`, and `504`
- configured retry statuses may be any status in the `400`-`599` range, so
  deployments can opt into retrying responses such as `429`
- do not retry on other upstream `4xx` request validation errors
- stop after each target in the alias pool has been tried at most once

This avoids hiding client request mistakes while still allowing basic failover
for transient upstream failures.

Retryable `4xx` statuses are an alias-routing decision only. They do not mark a
provider unhealthy; provider-health mutation remains tied to transport/upstream
request errors and upstream `5xx` responses.

### Upstream Retry Cooldown

Alias targets honor upstream retry advice (`retry-after-ms`, else
`Retry-After`) as cross-request cooldown advice. Any alias-target response
carrying valid advice records a deadline stored at observation time; later
alias requests exclude cooling targets (combined with already-tried targets
for both algorithms) until expiry, rechecked before dispatch without holding
state locks during I/O. Successes and non-retryable errors are returned
verbatim while recording advice for future selection; an uncommitted retryable
failure (per that alias's `retry_status_codes`) that newly cools the last
eligible target is discarded (body closed, lease released exactly once) and
becomes a proxy-generated JSON `429` immediately.

Identity is `(alias, provider, model)`, alias-local and shared across that
alias's operations; direct requests neither consult nor populate it. Effective
reload identity additionally includes resolved `base_url`, credential,
upstream model, and protocol. Parsing: valid positive-integer
`retry-after-ms` wins, otherwise `Retry-After` delay-seconds then HTTP-date;
case-insensitive names, first valid value wins, strict ASCII-digits format,
and zero/malformed/past/overflow values record nothing (no silent maximum,
only overflow protection). The synthetic response reuses the `429`
`upstream_rate_limited` error type with `Retry-After` (ceiling seconds, min 1)
and `retry-after-ms` (ceiling milliseconds, min 1) computed from the same
earliest remaining deadline at response time — the stored deadline uses the
original delay, the response uses `deadline - now` rounded up. It performs
zero upstream calls, leaves skipped targets out of upstream-attempt, selection,
retry, and usage attribution (client-facing status stays visible in HTTP
accounting/metrics), and never mutates provider health. A mixed pool of
cooling plus otherwise-unhealthy targets keeps the existing exhaustion path,
not synthetic `429`.

Cooldown state is runtime-owned and process-local: no Redis sharing,
persistence, new dashboard, or configuration surface. It is retained across
`SIGHUP` for fingerprint-unchanged targets (algorithm and `retry_status_codes`
changes do not invalidate), dropped for removed/changed/expired entries,
untouched by failed reloads, and isolated so old in-flight completions cannot
write to replacement identities. Another proxy process is never coordinated,
and requests admitted before advice arrives cannot be retroactively prevented.
Dispatch never sleeps.

Direct provider model requests do not fail over to a different provider or
model, because the client selected a specific target explicitly.

### Bounded Encrypted-Reasoning Inspection

Alias `encrypted_reasoning.on_caller_mismatch = "strip_and_retry"` inspects only
non-streaming HTTP 400 responses to applicable chat/Responses requests carrying
opaque reasoning markers. Status, request applicability, configured patterns and
the single stripped-attempt budget are checked before decoding. Successful small
mismatches retry the same target with opaque blocks removed and text retained.

Optional inspection supports gzip (including x-gzip), raw deflate, Brotli and zstd,
decoding Content-Encoding layers in reverse order. Repeated header fields are
combined in order, with a 256-byte budget including joining commas and at most
four comma-separated entries (including identity/empty entries). Metadata and
unsupported encodings are rejected before decoder construction. Each decoded
layer must fit in 1 MiB. Reader-based codecs read at most limit-plus-one bytes,
reject overflow and propagate decoder/trailer errors rather than inspecting a
truncated prefix.

Zstd uses one low-memory decoder, a 1 MiB maximum window, a 1 MiB maximum decoded
size and `WithDecodeAllCapLimit` with a fixed 1 MiB output buffer. These options
bound allocation before expansion for advertised sizes, unknown-size frames and
concatenated frames; post-decode slicing is insufficient. The memory option is
not a total-heap ceiling: bounded decoder/block bookkeeping adds overhead, and
each of the at-most-four layers may allocate its own bounded buffer. The fixed
output reservation also applies to small valid zstd responses. This deliberately
favors predictable per-inspection allocation over minimizing small-response cost.

Failed inspection returns the original body plus an explicit failure flag, so
literal mismatch text in rejected compressed bytes cannot trigger a retry. Neither
successful inspection nor failure mutates the upstream body or headers. A rejected
inspection skips only this optional mismatch retry; configured alias status-code
failover and cooldown behavior still apply. Uncompressed response scanning retains
its existing 64 KiB prefix limit.

## Provider Adapter Model

The proxy uses provider adapters behind the OpenAI-compatible frontend.

Each adapter is responsible for:

- building provider-specific HTTP requests
- injecting auth headers
- mapping the proxy model selection to the upstream model identifier
- translating provider-specific success payloads
- translating provider-specific error payloads
- translating streaming event formats when needed

Adapter categories:

- near pass-through adapters for `openai` and `openai-compatible`
- translation adapters for provider-native APIs such as `anthropic` and `gemini`

## Streaming Behavior

The current API supports streaming chat completions and streaming responses.

Streaming rules:

- the public API uses OpenAI-compatible SSE framing
- upstream provider streaming formats are translated into OpenAI-compatible event streams when needed
- the proxy should flush chunks promptly and avoid buffering the full stream in memory
- if an upstream stream fails after partial output, the client receives a terminated stream rather than a synthetic full JSON response

## Credential Resolution

Provider credentials are configured per provider.

Exactly one of these must be set:

- `api_key`
- `api_key_ref`

`github-copilot` is the exception: it declares `credential_ref { path?, name }`
instead and rejects `api_key`/`api_key_ref` (`credential_ref` is rejected on
all other types). `credential_ref` points at a sidecar written by
`aiproxy login github-copilot` (`<secrets-dir>/copilot-<name>.json`); `path`
defaults to the shared secrets path. The token resolves at config load, so a
missing/expired credential fails startup/reload with a re-login hint rather
than failing on first request.

### `api_key`

Inline string value, typically sourced from environment substitution:

```hcl
api_key = env("OPENAI_API_KEY")
```

### `api_key_ref`

Nested block with:

- `path`
- `key`

Example:

```hcl
api_key_ref {
  path = "~/.config/aiproxy/keys.json"
  key  = "openai"
}
```

`path` defaults to a secure user-scoped location:

1. `$XDG_CONFIG_HOME/aiproxy/keys.json` when `XDG_CONFIG_HOME` is set
2. `~/.config/aiproxy/keys.json` otherwise

The JSON file is expected to be a flat object mapping string keys to string API
keys.

Example:

```json
{
  "openai": "sk-...",
  "anthropic": "sk-ant-...",
  "localai": "secret"
}
```

Credential lookup should happen during config load so invalid references fail
startup rather than failing on first request.

## Error Handling

The proxy should normalize errors into OpenAI-compatible error responses where
possible.

Typical cases:

- unknown model name
- unknown alias name
- alias with no healthy targets
- invalid client auth
- invalid config
- unsupported provider type
- upstream provider auth failure
- upstream validation failure
- upstream transport timeout
- translation failure

Rules:

- config errors fail startup
- unknown model or alias returns a client-visible `4xx` error
- provider auth failures are surfaced as upstream errors, not rewritten as local auth failures
- transient alias target failures may trigger retry to another alias target
- proxy-generated errors should include a request ID for debugging

## Config Model

Configuration uses Alloy-like labeled HCL blocks.

Recommended block types:

- `listener "http" "public"`
- `auth "main"`
- `provider "<type>" "<name>"`
- `alias "<name>"`

### Example Config

```hcl
listener "http" "public" {
  address = ":8080"

  timeouts {
    read_header = "10s"
    idle        = "60s"
    write       = "0s"
  }
}

upstream_header_timeout = "120s"

auth "main" {
  mode = "bearer_static"

  client "local-dev" {
    token = env("AIPROXY_CLIENT_LOCAL_DEV_TOKEN")
  }
}

provider "openai" "openai" {
  display_name = "OpenAI"
  api_key      = env("OPENAI_API_KEY")

  model "gpt-4o-mini" {
    display_name = "GPT-4o mini"
  }

  model "gpt-4.1" {
    display_name = "GPT-4.1"
  }
}

provider "anthropic" "anthropic" {
  display_name = "Anthropic"
  api_key_ref {
    key = "anthropic"
  }

  model "claude-sonnet" {
    display_name = "Claude Sonnet"
    upstream_name = "claude-sonnet-4-20250514"
  }
}

provider "openai-compatible" "localai" {
  display_name = "LocalAI"
  base_url     = "https://llm.internal/v1"
  upstream_header_timeout = "180s"

  api_key_ref {
    key = "localai"
  }

  model "qwen3-32b" {
    display_name = "Qwen 3 32B"
  }
}

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

alias "chat_fallback" {
  algorithm = "least_connections"

  target {
    provider = "openai"
    model    = "gpt-4.1"
  }

  target {
    provider = "localai"
    model    = "qwen3-32b"
  }
}
```

### Config Semantics

- `display_name` is descriptive only
- `base_url` is required only for `openai-compatible`; `opencode-zen`,
  `opencode-go`, `github-copilot`, and `zenmux` default to their service prefixes and accept `base_url` only
  as a transport override that never changes service selection
- `base_url` must be an absolute `https` URL for remote upstreams; `http` is
  allowed only for loopback hosts such as `localhost`, `127.0.0.1`, or `::1`
- `api_key_ref.path` is optional because it has a secure default
- `upstream_header_timeout` accepts a positive duration at root or provider scope; provider values override root values, and the default is 90 seconds
- `user_agent` accepts a 1-256 printable ASCII override at root or provider scope; provider values override the root value, and the default is `aiproxy/<version>`
- `forward_user_agent` enables inbound `User-Agent` forwarding at root or provider scope; it is effective when set at either level, with no per-provider opt-out when the root enables it
- the upstream header timeout limits only the wait for response headers, not JSON or streaming response bodies after headers arrive
- aliases reference provider and model names without extra ref prefixes

### Upstream Redirect Policy

Inference client construction and CLI upstream model discovery share
`internal/upstreamhttp.CheckRedirect`. It compares each redirect with the original
request's origin: scheme, case-insensitive hostname, and effective port (HTTP 80,
HTTPS 443 when omitted). It permits at most 10 same-origin follow-up requests;
cross-origin redirects, including downgrades, fail before destination I/O. This
boundary protects all headers, including `x-api-key`, `x-goog-api-key`, bearer
tokens and configured custom forwarded headers, as well as replayable request
bodies. Go's default sensitive-header stripping does not protect custom headers
or port-only origin changes.

Provider execution and health probes additionally use `upstreamhttp.Do`, which
copies the HTTP client before installing this policy, including on fallback and
injected clients. The transport, connection pool, timeout and cookie jar are
retained; a caller's stricter redirect callback still runs for permitted origins.
Shared clients are never mutated. Probe origin means the initial resolved health
URL, including an explicitly configured absolute health URL.

Policy violations return errors rather than `http.ErrUseLastResponse`, so a
blocked 3xx cannot enter a translated success handler. Existing transport-error
handling returns a generic `502 upstream_error` for direct inference, permits
configured alias failover, and reports a discovery error in the CLI. Operators
must configure the final `base_url` instead of relying on cross-origin redirects.
Same-origin redirects retain Go's method/body semantics (301/302/303 can become
GET; 307/308 preserve replayable bodies). The policy changes neither pooled
transports nor header timeouts or post-header stream lifetimes. OAuth device
login retains its separate, stricter refusal of every redirect.

## Validation Rules

The config loader should validate:

- duplicate provider names
- duplicate alias names
- invalid provider type values
- invalid alias algorithm values
- provider names that are not lowercase
- alias names that are not lowercase
- provider or alias names containing spaces or `/`
- provider name `alias`, which is reserved for `alias/<alias-name>` routing
- model names that are not lowercase, contain spaces, or contain empty `/`
  segments; slash-containing model names are valid when each segment is valid
- `openai-compatible` providers missing `base_url`
- malformed provider `base_url` values, and non-loopback `http` base URLs
- `opencode-zen` or `opencode-go` models missing `protocol`, using an unknown
  protocol, using `gemini` on `opencode-go`, or declaring a capability the
  protocol does not serve; `protocol` on any other provider type
- providers with both `api_key` and `api_key_ref`
- `github-copilot` providers with `api_key`/`api_key_ref`, missing
  `credential_ref.name`, or `credential_ref` on any other provider type
- `protocol` on any non-OpenCode provider type
- malformed, zero, or negative `upstream_header_timeout` values
- active providers with no resolved credential, including an empty
  `api_key = env("...")`; missing or empty credentials fail validation unless
  `enabled = false` is declared explicitly
- `api_key_ref` blocks missing `key`
- `api_key_ref` JSON files that do not exist or do not contain the requested key
- providers without any models
- duplicate model names within a provider
- aliases without any targets
- alias targets pointing to unknown providers
- alias targets pointing to unknown models
- `ingress_guardrails` with an unknown mode, out-of-range `max_text_bytes`
  or `max_strings`, or duplicate blocks

The service should fail startup on invalid config.

Providers default to enabled. To intentionally disable a provider, declare
`enabled = false`; disabled providers are still structurally validated (name,
type, base URL, models, capabilities) but do not require a usable credential.
The disabled state is reported explicitly in startup logs and dashboard
snapshots rather than inferred from missing secret state.

## Ingress Secret Guardrails

Inbound `POST /v1/chat/completions` and `POST /v1/responses` requests can be
scanned for suspected secrets before any provider call when the optional
`ingress_guardrails` block is enabled. The scanner reuses the embedded
Gitleaks default rule set as a pinned Go module behind the small
`internal/guardrails` interface (complete-clean, findings, and
incomplete/error outcomes); only bounded safe metadata (outcome, rule IDs,
counts, reason) leaves the wrapper.

- Coverage is decoded text: chat message content (string and text parts),
  tool-call arguments (plus one JSON-decoded level for stringified
  arguments), tool results, and responses instructions/input items. JSON keys,
  unknown fields, images, audio, embeddings, attachments, encoded blobs,
  multipart bodies, response bodies, and SSE streams are out of scope and are
  never asserted clean for those bytes.
- `mode = "block"` (default) rejects flagged scans with
  `400 secret_blocked` and unscannable required scans with
  `400 scan_incomplete`, with zero upstream I/O (no adapter call, retries,
  cooldowns, health updates, or upstream usage). `mode = "audit"` forwards
  unchanged and records the outcome. `gitleaks:allow` never suppresses a
  finding; cancellation and bound overruns are visible `incomplete`
  outcomes, never clean scans.
- `max_text_bytes` (default 65536, 1024..32MiB) and `max_strings` (default 512, 1..16384) bound the
  per-request work; clean 64 KiB scans average about 11 ms on the reference
  host while full 32 MiB bodies take tens of seconds, so the cap is the latency
  budget. Scanning runs synchronously in the request goroutine against one
  immutable shared detector per policy generation.
- While enabled, covered-operation request bodies are omitted from
  payload-log entries on success, blocked, and early-rejected paths.
  Response capture is unchanged (response scanning is deferred). Findings
  never appear in logs, metrics, or client errors.
- Policy compiles at startup (failure fails startup) and rebuilds before
  publication on `SIGHUP` (failure rejects the reload with the active policy
  intact). Custom rules, rule subsets, and operator allowlists are not v1
  scope; use the full default rule set or leave the block disabled.

## Observability And Security

The proxy should provide logs, metrics, and traces, but default to protecting
prompt and credential data.

Defaults:

- never log API keys or client bearer tokens
- redact or omit prompt and response bodies from standard logs
- emit request IDs for correlation
- record per-provider latency and error-rate metrics
- record alias target selection counts
- record active in-flight request counts for `least_connections`

Initial `/metrics` coverage includes:

- inbound HTTP request counts by method/path/status
- inbound HTTP request latency by method/path/status
- inbound HTTP request body size histograms by method/path
- outbound HTTP response body size histograms by method/path/status
- streaming response counts by method/path/status
- streaming response duration by method/path/status
- proxy-generated HTTP error counts by method/path/status/error_type
- provider selection counts
- alias retry counts
- alias in-flight request gauges by target
- auth mode startup state
- build version info
- provider counts by type and active/disabled state
- alias counts by algorithm
- skipped-provider state
- provider health state
- readiness state
- readiness reason state
- upstream response body size histograms by operation/provider/outcome
- ingress guardrail scan counts by operation/mode/outcome
  (`clean`, `flagged`, `incomplete`; labels stay bounded)
- upstream request counts by operation/provider/outcome
- upstream request latency by operation/provider/outcome
- provider health backend error counts
- provider health fallback counts by operation and reason

`GET /metrics` is served on the same listener as the proxy API but is gated by
a dedicated metrics bearer token declared in a `metrics { token = ... }` block.
The token is checked independently of API auth client tokens; an empty or
missing token is rejected at config validation so metrics are never exposed
without a dedicated credential. Metric output can include tenant, client,
provider, model, and alias labels, so the token must be shared only with
trusted scrapers. HTTP route labels are a closed set of stable endpoint names,
with unknown dashboard-internal paths reported as
`/_internal/dashboard/unknown`.

The interactive `aiproxy dashboard` command and the
`/_internal/dashboard/{snapshot,logs}` endpoints share the metrics-token-less
listener. Listener addresses are TCP bind addresses in `host:port` form, not
URLs. The dashboard command is local-only: it derives a loopback plain-HTTP URL
from wildcard or loopback binds, refuses concrete non-loopback hosts, and uses a
dashboard bearer token for every RPC. Remote dashboard support requires a future
explicit transport design. Repeated invalid dashboard tokens are rate limited
with `429` and a `Retry-After` header so the bearer surface cannot be
brute-forced from the listener.

Every dashboard route (snapshot, logs, payload list/detail, block list/take-once
capture, and exception decision) uses one operator gate before reading data or
writing decisions. The dashboard secret grants global operator access. With an
admin store, a verified JWT must resolve to an active stored system administrator;
workspace membership and the selected workspace do not scope or authorize
these global surfaces. The shared admin claims boundary resolves the JWT subject
by user ID and replaces role/email claims with current stored values, so admin
routes and `/me` agree with dashboard authorization after promotion, demotion,
disablement, deletion, or email changes. Valid non-operators receive `403`;
unverifiable identities use the existing throttled authentication-failure path.
The browser checks `/me` before enabling global queries and navigation, and
renders an operator-access explanation for direct links or server denials.

### Browser authentication lifetime

The browser has a process-local, monotonically increasing authentication generation.
Explicit account installation (password login, OIDC callback, or direct replacement),
session clearing, dashboard-token changes, and workspace selection advance it.
All sensitive query hooks append this non-secret generation to their existing keys;
tokens never appear in query keys. The shared query boundary synchronously cancels
and removes sensitive queries, including inactive cached results, on a transition.
The public admin status query is independent. Query clients are weakly referenced,
and obsolete mutation-cache entries are removed as well.

Admin and dashboard HTTP clients capture the generation synchronously with the
request's credentials and share a generation-scoped abort signal. They reject stale
successes and errors before parsing or retrying; query completion also checks the
generation, even for transports that ignore cancellation. The generation-keyed app
and dialog provider discard local detail views, forms, and one-time credential
results. Workspace creation callbacks recheck the generation after asynchronous
dialog/refetch work before selecting a workspace. Account replacement clears
the persisted workspace selection before the new account's catalog is loaded.

A separate account-session epoch owns refresh work. Concurrent `401` responses share
one refresh, and late `401`s for an already-replaced access token retry with the
current token. Only refresh of the still-current session can rotate credentials
without changing the data generation, preserving its cached data and workspace.
Refresh failure clears that session; late success/failure from a previous session
cannot install credentials, clear a replacement account, or retry as that account.
Login results use the same session-ownership check. The OIDC callback consumes its
fragment once, including under React Strict Mode.

Logout clears local account/workspace/dashboard credentials immediately and
revokes the captured refresh token separately; a delayed revocation result cannot
clear a later login. Dashboard-token validation uses the candidate credential
without installing it first, committing only a valid response for the current
generation. Failure never restores an earlier token, and sign-out invalidates
validation even when the stored token is already empty.

These lifetimes are per loaded browser tab; cross-tab storage-event synchronization
is not implemented. Cancellation prevents obsolete results from being published in
the UI; it cannot undo a mutation already admitted by the server. Refresh/login/token
verification responses remain guarded even when their network work finishes after
a transition. Browser authorization is presentation logic; server-side stored
authority remains the access-control boundary.

## CLI Design

The service is a single binary named `aiproxy`.

Recommended commands:

- `aiproxy serve --config /etc/aiproxy/config.hcl`
- `aiproxy validate --config /etc/aiproxy/config.hcl`
- `aiproxy login github-copilot --client-id <id> --credential <name>`
- `aiproxy models --provider <name> --upstream`
- `aiproxy version`

Linux also supports `aiproxy serve -d`, `aiproxy status`, `aiproxy stop`, and
`aiproxy restart` for daemon lifecycle management. On non-Linux platforms,
foreground `serve` remains supported but daemon lifecycle commands return
`daemon lifecycle is unsupported on this platform`.

Optional future commands:

- `aiproxy print-example-config`

### Upstream Model Discovery

`aiproxy models --provider <name> --upstream` retrieves a complete listing before
printing display names and configured/not-in-config annotations. All provider
listing readers share these fixed per-invocation limits:

| Budget                     | Inclusive maximum |
| -------------------------- | ----------------: |
| Response pages             |               100 |
| Model entries across pages |            10,000 |
| Each response body         |             8 MiB |
| Aggregate response bodies  |            32 MiB |
| Whole listing duration     |         2 minutes |

The existing 8 MiB page allowance accommodates rich model metadata; 32 MiB bounds
aggregate metadata processing, and 10,000 entries/100 pages allow substantial
catalog growth while bounding retained output, cursor history, and network calls.
Entries count before filtering blank IDs and include duplicates. Body bytes count
JSON, whitespace, and metadata as delivered by Go's HTTP response reader (after
automatic HTTP gzip decompression when applicable). These are resource budgets,
not a process-wide heap limit. Reads stop at the smaller of the page allowance and
remaining aggregate allowance, plus one byte to distinguish an exact boundary
from overflow. Oversized success and error bodies fail explicitly.

The two-minute context covers the entire traversal, including body reads; it is
not reset per page. An earlier caller deadline/cancellation or the existing
per-request timeout (provider `upstream_header_timeout`, default 30 seconds for CLI
listing) can stop retrieval sooner. Fixed internal constants avoid adding public
configuration for malformed or impractically large catalogs. Operators encountering
a limit should check the provider endpoint/catalog or its pagination implementation.

Anthropic and Gemini cursors are query-escaped opaque values. Any repeated cursor,
including a multi-page cycle, is an error. Anthropic `has_more = true` requires a
nonempty page and a nonempty `last_id` matching the final returned entry; a terminal
page may still contain `last_id`. Gemini permits empty pages with a next-page token,
which still consume page/byte/time budgets; an absent or empty token is terminal.
At an exact page/aggregate limit, a terminal page succeeds but further continuation
fails before another request. Every response body is closed before the next page
or return. Any failure discards accumulated models and reports incomplete discovery;
the CLI never prints a partial model list. Provider authentication, User-Agent,
OpenCode session headers, and the shared same-origin redirect policy still apply
on every request. These limits do not change the proxy-owned `GET /v1/models`.
Copilot accepts both a `data` object and a raw model array, including empty
catalogs, with identical byte and entry limits.

### Configuration Conversion Publication

`aiproxy convert [target-file]` resolves `env()` values into literals, including
secrets, then validates the converted HCL/JSON before any destination write.
`-` emits the validated payload to stdout and propagates write errors.

On Linux and macOS, file output uses `internal/filestore` with `Secret: true`, file mode `0600`, and
parent-directory mode `0700` (subject to umask, existing directories unchanged).
The shared staging path writes a same-directory temporary regular file, enforces
the exact file mode, syncs, and closes it before publication. Destination checks
use `Lstat` to reject live/dangling symlinks and non-regular files. With `--force`,
`WriteFile` publishes via rename, replacing the old inode instead of truncating
it. Without force, `CreateFile` publishes via a hard link to the staged inode,
then removes the temporary name and syncs the directory. The link is the atomic
create-if-absent boundary: a competing destination created after validation is
never overwritten. No stat-then-write, placeholder file, process-local lock, or
non-atomic fallback is used; filesystems without hard-link support return errors.

File conversion on Windows (and other unsupported OS targets) fails before
staging or creating parent directories. Go's Windows chmod only toggles the
read-only attribute, not owner-only ACLs, and its rename API does not guarantee
atomic replacement. The CLI therefore does not claim the POSIX publication
contract there. Validated stdout conversion remains available on every platform;
users can save that output through an external tool with suitable permissions.

Validation, staging, and failed rename/link operations preserve the previous
destination. After successful publication, cleanup or directory-sync errors may
leave the complete new file present; these are not rollback transactions.
Cleanup, directory-sync and confirmation-output failures explicitly report that
publication succeeded.
Temporary-file cleanup is attempted on failure; a cleanup failure may retain a
private staged file. As with existing filestore consumers, these operations use
normal parent-directory traversal and do not establish a filesystem sandbox.
The existing replacement and multi-file recovery contracts remain unchanged.

## Deployment Model

The service is packaged as a Docker image that runs the CLI.

Recommended container behavior:

- expose the proxy on `:8080`
- mount config at `/etc/aiproxy/config.hcl`
- pass client tokens and provider secrets via environment variables or the key file
- mount the key file read-only when `api_key_ref` is used

Recommended image approach:

- multi-stage Docker build
- static or near-static Go binary
- minimal runtime image with CA certificates

## Recommended Package Layout

```text
cmd/aiproxy/
internal/app/
internal/config/
internal/auth/
internal/httpapi/
internal/requestctx/
internal/modelresolver/
internal/alias/
internal/provider/
internal/provider/openai/
internal/provider/openaicompat/
internal/provider/anthropic/
internal/provider/gemini/
internal/stream/
internal/observability/
```

## Historical Implementation Plan

The milestone list below is historical and no longer defines the current public
contract.

### Milestone 1

- CLI scaffold
- HCL config parsing and validation
- inbound auth modes:
  - `none`
  - `bearer_static`
- direct provider/model resolution
- alias resolution with `round_robin`
- `POST /v1/chat/completions`
- non-streaming chat responses
- streaming chat responses
- OpenAI and `openai-compatible` adapters
- logs, health, readiness, basic metrics

### Milestone 2

- `least_connections` alias selection
- transient-failure retry across alias targets
- stronger streaming robustness
- more metrics and integration tests

### Later Phase

- anthropic embeddings if a viable provider-native mapping exists
- translated-provider image and audio APIs
- external billing/invoicing and quota systems
- per-client policy
- broader provider catalog

## Testing Strategy

### Unit Tests

- config parsing and validation
- provider/model name parsing
- alias target selection
- retry policy
- credential resolution from env and key file
- adapter request translation
- adapter response translation
- streaming event translation

### Stub-Backed End-To-End Tests

Run against in-process provider stubs using `httptest` servers:

- health and readiness endpoints
- direct `openai/<model>` chat completion
- `alias/<name>` routing through configured upstream targets
- `POST /v1/embeddings`
- `POST /v1/responses`

These tests exercise the full proxy request path without depending on external
provider accounts or sandbox containers.

### Hermetic Binary Integration Tests

The normal CI suite includes hermetic binary-level integration tests that build
`dist/aiproxy`, start the binary with temporary local configuration and upstream
stubs, and exercise listener, readiness, reload, auth, metrics, streaming,
derived-provider routing, and alias retry behavior without paid provider
credentials.

Real-provider sandbox tests remain separate and optional.

### Documentation Contract Checks

`make docs-contract` checks the marked public endpoint/provider and provider
capability tables in README, this design document, the website docs, and
AGENTS.md. Markdown-only pull requests run the same check through the `Docs
Contract` workflow so high-drift contract tables cannot change in only one
location.

## Deferred Features

The following are intentionally out of scope for the current public contract:

- anthropic embeddings
- translated-provider image endpoints
- translated-provider audio endpoints
- external billing / invoicing systems and quotas

## Provider Health

The proxy maintains dynamic provider health state in-process and shares it
across requests and aliases.

Transient transport failures, upstream request errors, and upstream `5xx`
responses mark a provider temporarily unhealthy for alias routing and readiness
decisions. Configured retryable `4xx` statuses can cause an alias to try another
target, but they do not mutate provider health.

Provider health state is not coordinated across multiple proxy instances.

An optional `provider_health` block may configure Redis-backed transient health
state sharing across instances:

- `redis_url`
- optional `key_prefix`
- optional `cooldown`
- optional `cache_ttl` (default 30s), bounding how long a stale local cache
  entry is reused for routing and readiness when the Redis backend becomes
  unreadable

When `redis_url` is configured and a Redis health read fails, routing,
readiness, and dashboard snapshots fall back to the bounded in-process cache
and fail open only when no fresh cache entry exists. Both the backend error
and the fallback reason are recorded as Prometheus metrics so degraded mode is
observable.

## Reload Behavior

The server supports in-process config reload on `SIGHUP`.

Reload currently rebuilds and swaps:

- inbound auth configuration
- provider/model catalog
- alias routing state
- root and provider upstream header timeouts
- access-log enablement
- payload-log configuration
- metrics configuration
- provider-health configuration
- readiness and startup inventory metrics

Reload does not replace the active listener socket.

Runtime construction and reload separate resource preparation from activation.
Preparation loads and validates the catalog, opens candidate payload sinks,
compiles guardrails, loads the exception file, and validates quarantine policy.
Reload publishes a required dashboard-token file only after these steps succeed,
as the final fallible preparation step. Until then, the active health catalog,
probe manager, routing dependencies, and config inventory remain in use. A corrupt
or unreadable exception file, sink initialization failure, or token persistence
failure rejects the candidate without starting its probes or stopping live probes.

Each preparation attempt owns only newly acquired resources. Failure cleanup
closes candidate MongoDB and disk payload sinks and health backends; failed initial
Build also closes its database store. Cleanup preserves the preparation error and
joins any close errors. Reused active sinks and health trackers are borrowed, so
rollback never closes them. Sink constructors retain responsibility for partial
acquisition (for example, a disk sink opened before MongoDB initialization fails).
Preparation may create directories or perform sink initialization/retention work;
these external side effects are not filesystem/database transactions.

After preparation succeeds, activation updates health/probe membership and
publishes handler dependencies and runtime fields, then closes replaced resources.
Build starts probes only after construction has no remaining fallible steps.
Unchanged routing state, health trackers, payload sinks and probe schedules retain
their existing reuse rules. A canceled probe's result is discarded so a retired
probe cannot publish its cancellation as a failure over the replacement's health.
Probe status, metrics and tracker publication hold the manager lock through the
complete update, serializing replacement with already-started backend writes.
Consequently replacement and status reads may wait for a health-backend write;
they cannot overtake it and then be overwritten by the retired probe.
Successful publication still uses the existing request-snapshot lifecycle; it
does not drain already-admitted requests or make all shared metrics and probe
observations switch in one atomic operation. Normal app shutdown owns resources
transferred by successful construction or reload.

The following config changes still require a full restart:

- listener address changes
- listener timeout changes
- logging level changes
- enabling the dashboard after startup

## Appendix: Open Questions And Rejected Alternatives

### Open Questions

- `GET /v1/models` now returns both direct provider-backed models and aliases in one list, including capability metadata, display names, provider types, and alias target summaries. Should a later revision also expose raw upstream model identifiers in that response?
- Should future capability declarations remain an optional narrowing mechanism, or eventually become required for every configured model?
- Should later auth work remain simple static bearer tokens, or grow into tenant-aware policy and quotas?
- Should provider key files be re-read on each request for easy secret rotation, or only at startup for predictability?

### Rejected Or Deferred Alternatives

#### Free-form object arrays for models and alias pools

Rejected.

Reason:

- nested HCL blocks fit the existing repo style better
- nested blocks validate more cleanly
- two-label blocks keep provider type and provider name explicit

#### Public provider-native endpoint passthrough

Rejected for the current public contract.

Reason:

- it weakens the value of a consistent OpenAI-compatible frontend
- it complicates auth, logging, and routing behavior
- it encourages provider-specific client coupling

#### Global least-connections balancing across all instances

Rejected for the current public contract.

Reason:

- it requires shared state or a control plane
- it adds operational complexity that is not necessary for the first release

The chosen design keeps `least_connections` process-local.

#### Silent fallback for direct provider/model requests

Rejected.

Reason:

- if a client asks for `openai/gpt-4.1`, it should either get that target or a clear error
- failing over to a different provider or model would be surprising and hard to debug

The chosen design allows fallback only for alias-based requests.

## Final Current Decisions

- the public API is OpenAI-compatible
- the current endpoint/provider support matrix is the one listed in API Surface
- direct model names use `<provider-name>/<model-name>`
- alias names use `alias/<alias-name>`
- provider name `alias` is reserved, and direct model resolution uses the first
  `/`, so provider model names may contain `/` when each segment is valid
- HCL uses two-label `provider "<type>" "<name>"` blocks
- `openai-compatible` requires `base_url`; `opencode-zen` and `opencode-go`
  default to their service prefixes with `base_url` as an optional transport
  override, and every OpenCode model declares a required `protocol`
- providers normally declare exactly one of `api_key` or `api_key_ref`;
  missing or empty credentials fail validation unless `enabled = false` is
  declared explicitly or the provider type is `opencode-zen` (keyless upstream
  access sends no `Authorization` header); `github-copilot` declares
  `credential_ref` instead and resolves its device-flow sidecar at load
- `api_key_ref.path` defaults to `$XDG_CONFIG_HOME/aiproxy/keys.json` and falls back to `~/.config/aiproxy/keys.json`
- aliases support `round_robin` and `least_connections`
- alias retry happens for transport errors, timeouts, and configured
  `retry_status_codes` in the `400`-`599` range
- direct provider/model requests do not fall back to different targets
- streaming chat completions and responses are part of the current supported API
