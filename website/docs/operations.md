---
sidebar_position: 6
---

# Operations

This page covers the commands and operational behavior that matter most for local development and production deployment.

## Build And Run

Common commands:

```sh
make build
make run CONFIG=path/to/config.hcl
make validate CONFIG=path/to/config.hcl
```

Direct CLI usage:

```sh
aiproxy serve
aiproxy validate
aiproxy login github-copilot --client-id YOUR_GITHUB_OAUTH_CLIENT_ID --credential copilot-main
aiproxy paths
aiproxy examples
aiproxy configure
aiproxy configure provider
aiproxy serve --config /etc/aiproxy/config.hcl
aiproxy validate --config /etc/aiproxy/config.hcl
aiproxy version
```

Without `--config`, the CLI reads `$XDG_CONFIG_HOME/aiproxy/config.hcl`, falling back to
`~/.config/aiproxy/config.hcl` when `XDG_CONFIG_HOME` is unset. Set `$AIPROXY_CONFIG`
to inline HCL to skip the config file (explicit `--config` overrides it; `serve -d`
and `configure`/`login` file workflows require a file).

Foreground `aiproxy serve` is supported across the advertised release targets.
Linux additionally supports `aiproxy serve -d` and the `aiproxy status`,
`aiproxy stop`, and `aiproxy restart` daemon lifecycle commands. On non-Linux
platforms those daemon lifecycle commands return `daemon lifecycle is
unsupported on this platform`.

When running locally with env-based secrets, load your environment before invoking the binary:

```sh
set -a; . ./.env; set +a
```

## Configure Wizard

`aiproxy` includes an interactive config editor for the top-level HCL blocks and
the provider secrets JSON file.

Interactive entrypoints:

```sh
aiproxy configure
aiproxy configure provider
aiproxy configure auth
aiproxy configure alias
aiproxy configure listener
aiproxy configure upstream
aiproxy configure logging
aiproxy configure provider-health
```

The root `aiproxy configure` command shows a block selector. The block-specific
subcommands can also be used directly.

Supported workflows:

- create or update `listener`, root `upstream_header_timeout`, `auth`, `provider`, `alias`, `logging`, and `provider_health`
- update provider secrets when using `api_key_ref`
- delete existing blocks with `--delete`

For scripted environments, use `--non-interactive` on block subcommands.

Provider example:

```sh
aiproxy configure provider \
  --config /etc/aiproxy/config.hcl \
  --non-interactive \
  --name backup \
  --type openai-compatible \
  --display-name "Backup provider" \
  --base-url https://llm.internal/v1 \
  --upstream-header-timeout 180s \
  --secrets-path /etc/aiproxy/keys.json \
  --secrets-key localai \
  --api-key "$LOCALAI_API_KEY" \
  --model qwen3-32b=qwen/qwen3-32b \
  --model-capabilities qwen3-32b=chat,responses

aiproxy configure provider \
  --config /etc/aiproxy/config.hcl \
  --non-interactive \
  --name backup-2 \
  --type openai-compatible \
  --extends backup \
  --display-name "Backup provider 2" \
  --secrets-key backup-2 \
  --api-key "$BACKUP_2_API_KEY"
```

OpenCode providers use explicit types with per-model protocols (`chat`,
`responses`, `messages`, or `gemini`; `gemini` is Zen-only). Base URLs are
omitted to use the service defaults; `--base-url` remains available as a
transport-only override.

```sh
aiproxy configure provider \
  --config /etc/aiproxy/config.hcl \
  --non-interactive \
  --name zen \
  --type opencode-zen \
  --api-key-env OPENCODE_ZEN_API_KEY \
  --model glm-5.3 \
  --model-protocol glm-5.3=chat

aiproxy configure provider \
  --config /etc/aiproxy/config.hcl \
  --non-interactive \
  --name go \
  --type opencode-go \
  --api-key-env OPENCODE_GO_API_KEY \
  --model minimax-m3 \
  --model-protocol minimax-m3=messages
```

GitHub Copilot is **hermetically verified; live GitHub compatibility unverified**.
See [mock-only verification](#mock-only-copilot-verification) for local checks
requiring no real client ID/account. Production `login` below contacts GitHub;
`configure provider` only references its saved credential (no API-key flags,
OAuth networking or token display in configure). The model name is illustrative:

```sh
aiproxy login github-copilot --client-id YOUR_GITHUB_OAUTH_CLIENT_ID --credential copilot-main

aiproxy configure provider \
  --config /etc/aiproxy/config.hcl \
  --non-interactive \
  --name copilot \
  --type github-copilot \
  --credential copilot-main \
  --model gpt-5.4-nano
```

`login` prints the verification URI and user code, then writes
`<secrets-dir>/copilot-<name>.json` (`0600`). It never edits HCL or signals a
server: restart or `SIGHUP` to activate, and re-run the same `login` + reload
on upstream `401`/`403`, revocation, or expiry. Use your own public OAuth
client ID; never reuse another application's client ID.

The web provider form offers the same authorization as Connect GitHub: enter
the public OAuth client ID, approve at the shown URL with the shown code, then
save the provider to apply. The browser never receives tokens; the server
stores the credential encrypted in the database. CLI sidecar and Connect GitHub
are alternatives; Copilot Rotate in the web UI opens reauthorization.

Connect GitHub details: the server performs the device challenge over the
fixed GitHub issuer, fixed `read:user` scope, and the fixed verification page,
then stores the result as an AES-GCM-encrypted database credential (never a
sidecar, never a raw-token API input). The same database encryption key must
be configured on every instance or the saved credential fails to decrypt.
Unfinished device sessions expire (pending is bounded by the issuer expiry and
15 minutes, ready lasts 10 minutes) and are reaped by a bounded per-minute
cleanup (100 rows per batch); provider saves set
`X-Aiproxy-Catalog-Saved: true` once the database commit is durable, including
saved-but-activation-failed outcomes where the old runtime stays active.
Activation applies to the receiving instance only; there is no cluster
broadcast, so reload or mutate each instance (or let its next mutation pick
the catalog up). Hermetically verified; live GitHub compatibility unverified
(see below).

Inference preserves upstream JSON `401`/`403` errors without implicit re-login;
`models --upstream` adds a re-login hint. A new listing invocation reads the new
sidecar immediately after login, while the running server still requires reload.

Root upstream timeout example:

```sh
aiproxy configure upstream \
  --config /etc/aiproxy/config.hcl \
  --non-interactive \
  --upstream-header-timeout 120s
```

Alias example:

```sh
aiproxy configure alias \
  --config /etc/aiproxy/config.hcl \
  --non-interactive \
  --name chat_default \
  --algorithm round_robin \
  --target primary/gpt-4o-mini \
  --target backup/qwen3-32b
```

Auth example:

```sh
aiproxy configure auth \
  --config /etc/aiproxy/config.hcl \
  --non-interactive \
  --name main \
  --mode bearer_static \
  --rate-limit-rpm 120 \
  --rate-limit-burst 120 \
  --client internal-app \
  --client-token-env internal-app=AIPROXY_CLIENT_TOKEN \
  --client-tenant internal-app=internal \
  --client-allowed-models internal-app=alias/chat_default,openai/gpt-4o-mini
```

Delete examples:

```sh
aiproxy configure provider --config /etc/aiproxy/config.hcl --delete --name backup
aiproxy configure alias --config /etc/aiproxy/config.hcl --delete --name chat_default
```

## Docker

```sh
make docker-build
make docker-run CONFIG=path/to/config.hcl
```

The image mounts the config file and runs the same CLI entrypoint.

In containerized deployments, mount the config file read-only and inject secrets through environment variables or the key file used by `api_key_ref`.

## Tests

```sh
make vet
make test
make test-race
make docs-contract
make cover
```

The standard local sanity check is:

```sh
make vet test
```

There is no separate typecheck target. A successful Go build is the typecheck.

Documentation-only pull requests run `make docs-contract` through the `Docs
Contract` workflow. Website pull requests also run `pnpm typecheck` and `pnpm
build` from the `website` directory.

## Reload Behavior

`aiproxy` supports live config reload on `SIGHUP` for runtime state such as:

- auth configuration
- provider and model inventory
- root and provider upstream header timeouts
- alias routing state
- access-log enablement
- payload-log configuration
- metrics configuration
- provider-health configuration
- metrics-backed inventory state

If rate-limit settings are unchanged, reload preserves existing limiter buckets.
Changing rate-limit settings creates a fresh limiter and resets bucket state.

Alias cooldown deadlines survive reload only for fingerprint-unchanged targets
(resolved `base_url`, credential, upstream model, protocol); removed or changed
targets are dropped, and failed reloads leave state untouched.

When multi-tenancy is enabled, database-backed providers, aliases, and
inbound keys merge into the serving catalog on the same reload path (and
automatically after admin API mutations). An invalid row — for example a
provider whose encrypted credential no longer decrypts under the current
encryption key, or an alias pointing at a removed target — fails the reload
with the old runtime intact, and blocks server startup the same way an
invalid config file does. Use `aiproxy validate --check-db` to identify the
offending row without running the server, then fix it (rotate the credential,
retarget or disable the entry) or delete it via the admin UI/CLI; the next
mutation or `SIGHUP` picks the catalog back up.

Database inbound keys with `expires_at` stop authenticating at that timestamp,
using the server clock, even if no reload occurs. Model listing, billing usage,
and new inference requests (including SSE) return the standard JSON `401`
`auth_failed` error (`invalid client token`) at or after expiry, before upstream
I/O. Requests and streams already authenticated may finish. Static HCL clients
and database keys without expiry do not expire. Changing a key's expiry,
rotating it, or disabling/deleting it still uses the catalog reload path above;
the stored deadline in an active runtime requires no database access per request.

These changes still require a restart:

- listener address changes
- listener timeout changes
- logging level changes
- enabling the dashboard after startup

Provider and alias administrative POST/PUT requests commit their complete aggregate
before requesting runtime activation, then read the response view. Provider model
write failures roll back metadata too. A concurrent edit returns `409`; read the
current provider/alias and retry. `500 could not save catalog edit` means no
activation was requested. `500 saved but activation failed` means the complete
edit is in the database while the old runtime remains active: inspect server logs,
resolve the failure and reload. `500 saved but response view unavailable` means the
edit was saved and activation succeeded, but the response view failed; read current
state before retrying, especially before repeating a create. These outcomes do not
promise a distributed transaction between the database and running proxies.

Use reload for routing and auth changes, not for socket-level listener changes.

### Editing Database Providers In The Web UI

The provider editor sends changed fields as a partial update. Both enabled
transitions, unchecked forwarding/healthcheck authorization, cleared strings and
cleared local header lists are saved explicitly. Unchanged models and credential
references are omitted. Leave the write-only API-key field blank to keep its stored
secret; entering a new value replaces it. Changing a reference path retains the
displayed key/name, and clearing the path selects the default secrets file.

Blank URL, user-agent and header-timeout overrides restore the applicable provider
or root defaults; provider types without a default URL still require one. Turning
off local user-agent forwarding or clearing local forwarded headers cannot opt out
of independently enabled root forwarding. The form displays local database values,
not the merged effective root settings.
These defaults apply to database providers on startup and every successful reload;
changing a root setting does not rewrite the saved local fields.

For inherited database providers, the editor uses the base provider's models,
transport and healthcheck rather than asking for local model rows. Type must match
the static base; display name and credentials remain local, with a blank display
name falling back to the base. The database provider's Enabled flag remains local.
Clearing Extends requires valid local settings and models before saving.

An existing custom healthcheck remains selected because the current API cannot
remove its block. Its path must stay nonempty. Clearing other scalar fields resets
them to the defaults shown in the form (GET, status 200, body `*`, interval 30s,
timeout 5s, failure threshold 2, success threshold 1); unchecking authorization
explicitly saves false. Saving and reopening shows the API-normalized values.

### Editing Database Aliases In The Web UI

Enter one `provider/model` target per line. Only the first slash separates the
provider from the model: `gateway/z-ai/glm-5.2` keeps the complete model name
`z-ai/glm-5.2` when creating, saving or reopening an alias. Deeper names such as
`gateway/group/family/model` are supported too. Blank lines and surrounding provider/
model whitespace are ignored; whitespace inside a name is invalid.

Each provider name and slash-separated model segment must start with a lowercase
letter or digit, followed by lowercase letters, digits, dots, underscores or
hyphens. Provider name `alias` is reserved. Missing names, empty segments, repeated
or trailing slashes and other separators produce a field error before submission.
Target errors identify the input line to correct.

Alternatively, supply comma-separated provider names and one full model name in
the shorthand fields. The same naming rules apply; list each provider once with
no empty comma entries. Clear explicit targets before using shorthand, and supply
both shorthand fields. Saved shorthand reopens as the equivalent full target list.

## Database Key Spend And Quota Resets

With multi-tenancy enabled, deleting an inbound key preserves its recorded spend
and user/team owner attribution. Requests already admitted with that key still
record their usage when they finish, including streamed responses. Creating a
replacement key, even with the same name, does not restore the owner's budget;
scope totals survive spend-cache refresh and server restart. Sharing a key does
not transfer its spend to the other users or teams with access.

A workspace administrator can explicitly reset a user/team budget's spend
with `reset_spend: true` on its quota endpoint. Resetting records an offset against
the current total; it does not delete historical usage. A request whose usage is
recorded after the reset counts against the new budget, even if it started before
the reset or its key has been deleted. Budget checks retain their existing
30-second cache and completion-based accounting; already-admitted requests may
exceed the budget.

Quota policy reads distinguish missing rows (unlimited) from storage errors.
When required budget/TPM rows cannot be read, or spend cannot be read for a
positive budget with a cold or expired cache, new inference requests return JSON
`503` `quota_unavailable` before any upstream call. This also applies to SSE
requests, which receive JSON rather than a started stream. Storage causes are
logged server-side; the response contains no database details. Retry after
storage recovers. Actual budget exhaustion remains `403` `budget_exceeded`, and
actual TPM exhaustion remains `429` `tpm_exceeded` with `Retry-After`.

A fresh spend-cache entry remains usable within its existing 30-second TTL,
including for budget denial. Failed refreshes neither cache zero nor extend an
expired entry. Policy rows are still read on each request, even with fresh
cached spend. A genuinely missing or zero budget needs no spend read for
admission; static HCL credentials do not acquire database quota dependencies.
Admin quota views read current spend even without a budget and fail with a
controlled `500` if the data is unavailable. Admin edits stop on failed policy
reads; if an edit saves successfully but its response view cannot be read, the
error explicitly says the quota was saved. Inspect the view after recovery
before repeating that edit.

The forward database migration retains key and workspace UUIDs for accounting,
without retaining deleted credentials. These identities and the ledger remain
until the workspace is deleted. Previously erased spend cannot be recovered
by the migration.

## Metrics And Health

The proxy exposes Prometheus metrics at `GET /metrics`.

Coverage includes:

- inbound request counts and latency
- streaming counts and duration
- provider selection counts
- alias retry counts
- alias in-flight gauges
- provider health state
- readiness state and reason
- upstream request counts, latency, and response sizes
- provider health backend error counts
- provider health fallback counts by operation and reason

`/metrics` requires a dedicated bearer token declared in a `metrics` block:

```hcl
metrics {
  token = env("AIPROXY_METRICS_TOKEN")
}
```

`GET /metrics` without a valid `Authorization: Bearer <metrics token>` header
returns `401`. The metrics token is checked independently of API auth client
tokens; API clients cannot scrape `/metrics` with their own credentials. An
empty or missing token is rejected at config validation.

Transient transport failures, upstream request errors, and upstream `5xx` responses can mark a provider unhealthy for routing and readiness decisions. Configured retryable `4xx` statuses can trigger alias failover but do not mark providers unhealthy.

Alias upstream retry advice (`retry-after-ms`, else `Retry-After`) is tracked
separately from provider health as process-local per-target cooldown deadlines.
It is never shared across processes or via Redis, never marks providers
unhealthy, and leaves skipped targets out of upstream attribution while the
client-facing `429` stays visible in HTTP accounting and metrics.

This health state is shared across requests within the same process.

## Shared Provider Health

Without extra config, provider health is in-process only.

You can optionally configure Redis-backed shared health state with `provider_health` so multiple instances can observe the same transient provider status.

```hcl
provider_health {
  redis_url  = env("AIPROXY_REDIS_URL")
  key_prefix = "aiproxy:provider-health"
  cooldown   = "30s"
  cache_ttl  = "30s"
}
```

`cache_ttl` (default 30s) bounds how long a stale in-process cache entry is
reused for routing and readiness when the Redis backend becomes unreadable.
When a Redis health read fails, routing, readiness, and dashboard snapshots fall
back to the bounded in-process cache and fail open only when no fresh cache
entry exists; both the backend error and the fallback reason are recorded as
Prometheus metrics so degraded mode is observable.

Without Redis-backed sharing, each instance tracks transient health independently.

## Interactive Dashboard Lifecycle

`aiproxy dashboard` attaches to a running local server. `q` (outside search) or `Ctrl+C` exits the
command and stops its polling and in-flight requests. `Esc` closes help, then a
detail, then a zoomed pane; at the main view it exits. Terminal initialization/I/O errors
are returned by the command, and the TUI uses the command's configured input/output.

Initial attachment still fails clearly for an unreachable server, missing dashboard
configuration, or invalid token. After successful attachment, transient transport,
timeout, malformed-snapshot, HTTP 408/429 and server errors preserve the last view.
The connection line shows **RECONNECTING**, the local time of the last successful
snapshot, and the next retry time. Retries wait 2, 4, 8, 16, then at most 30 seconds
after failed attempts; successful snapshots resume the normal two-second interval.
Each snapshot request has a two-second timeout. Connection state remains live while
display updates are paused.

`Ctrl+R` requests a retry now in any pane; repeated presses during an active snapshot
request coalesce into that request. Existing pane-local `r` refresh actions remain
available. HTTP 401/403/404, redirects and other permanent endpoint denials show
**DENIED** and stop automatic snapshot retries and new pane RPCs. Fix the server and
press `Ctrl+R` to probe again. The token/config are read only at attachment: quit and
re-run `aiproxy dashboard` after changing local credentials or listener configuration.
A successful probe reopens pane access; use pane-local refresh to retry a failed
pane list. Block decisions and take-once detail reads are never automatically replayed.

## Dashboard Layout And Keyboard Controls

The minimum supported terminal is **80 columns × 12 rows**. Below 30 rows (including
80×24), a compact layout shows only the focused pane. At 30 rows and above, providers,
usage and the active bottom tab are stacked. **Tab / Shift-Tab** cycles forward/back
through Providers → Usage → bottom pane, even in compact or zoomed layouts. Headers,
rows and errors are fitted in terminal cells, including wide Unicode characters.
The footer keeps help/back/quit controls visible (apply/cancel/quit while editing), with a separate live
connection line. Narrow views abbreviate measurement legends; help explains them.

| Key                                            | Action                                                                            |
| ---------------------------------------------- | --------------------------------------------------------------------------------- |
| `1` / `2` / `3` / `4` / `5`                    | Select Aliases / Logs / Payloads / Blocks / Requests and focus it                 |
| `[` / `]`                                      | Previous / next bottom tab in that order, wrapping at either end                  |
| `Enter`                                        | Open selected provider/bottom-row or Usage top-group detail; close an open detail |
| `z`                                            | Toggle focused-pane zoom from a list; details already occupy the body             |
| `Esc`                                          | Close help, else detail, else zoom; otherwise quit                                |
| `q` / `Ctrl+C`                                 | Quit, including from help/detail (`q` is text in search)                          |
| `?` / `h`                                      | Open help; `?`, `h`, `Esc` or `Enter` closes it                                   |
| `j/k`, arrows, `PgUp/PgDn`, `Home/End` (`g/G`) | Move/scroll the focused view; scroll help while help is open                      |
| `+/-`, `J/K`                                   | Resize the bottom pane in stacked layout                                          |
| `t/e/u`                                        | Tenant / errors-only / public-upstream filters in focused Usage                   |
| `l`, `o`                                       | Level filter / order in focused Logs                                              |
| `s` (or `e`), `o`, `r`                         | Status filter / order / refresh in focused Payloads list                          |
| `r`                                            | Refresh focused Blocks list                                                       |
| `p`                                            | Pause/resume data in a pane or detail                                             |
| `Ctrl+R`                                       | Retry the connection, including from help                                         |
| `/`, `Ctrl+U`                                  | Search Requests/Logs/Payloads metadata / clear applied search                     |
| `l` / `v` in Request detail                    | Exact-ID Logs / Payloads; Esc returns to Requests                                 |
| `n` / `N` in Usage detail                      | Next / previous exact identity group                                              |

Changing focus or tabs closes an open detail first and preserves list selection.
For remote details this cancels the read and discards late replies, including paused
results. Resizing and opening/closing help preserve the detail and selected record.
Enter on Usage shows the top group's full tenant/client/model/operation/status;
`n/N` cycles every group, including the final rows. `z` zooms its list.
Help owns input: pane filters, pause, tab navigation and decisions cannot run behind
it. An undersized-terminal warning also suppresses pane actions until resized.

Block-detail **n/N** selects the next/previous finding. **a/s/d** records allow
(non-secret), redact (placeholder), or deny (keep blocking) for **only that finding's
exact hash**. The selected index, scope and status stay visible while scrolling.
Numbered keys 1–5 always navigate tabs outside search and never record a decision.

## Dashboard Payload And Block Inspection

Payload and block text wraps to terminal cells, including long JSON strings, CJK,
combining characters and emoji. Use **j/k**, **PgUp/PgDn**, **Home/End** to inspect
all retained text. The fixed header identifies the record and selected finding;
wrapped row position and cap notice stay visible. Full IDs, hashes and metadata also
appear in the scrollable body. Control bytes and invalid UTF-8 appear as escapes
instead of being interpreted as terminal commands. Resizing rewraps/clamps the
position without changing the selected record or finding.

Payload pretty output retains the existing **64 KiB** cap. The view explicitly
labels pretty-output truncation and server request/upstream-request/response body
truncation flags. An unavailable flag is not proof of completeness. Block snippets
are already server-capped (scanner default 512 bytes, further limited by configured
quarantine snippet size); the existing response does not carry original lengths or
truncation flags, so its fixed notice says **truncation unknown**. Wrapping cannot
recover uncaptured text.

The Blocks list warns before Enter: a successful detail read **consumes the
take-once capture**; closing it does not make it available again. Another operator,
expiry, or a canceled/failed read may also leave it unavailable. A decision persists
**global future-match behavior for that hash**, including identical hashes in other
findings/requests. It **never replays or resumes the blocked request**. There is no
bulk decision shortcut.

While recording, finding selection is locked and further decisions are ignored;
scroll/help/back/quit remain available. A successful acknowledgment suppresses the
same action for that hash during the open detail; another action may deliberately
replace it. Failures show that the outcome may be unknown (the server could have
persisted before a connection failure); pressing a/s/d again deliberately retries.
Missing/invalid hashes disable decisions; the TUI never hashes a possibly truncated
snippet as a substitute. Pause defers acknowledgments and prevents new decisions.
Closing/navigation cancels outstanding work and discards late results; it cannot
undo a decision already persisted by the server. No read or decision is automatically
replayed on reconnect/resume. Full text remains confined to authenticated operator
detail fetches, outside Requests metadata and search indexes.

## Dashboard Requests And Search

**5:Requests** shows the last **up to 200 completed inference operations**, newest
first, even with disk payload logging disabled. This is a process-local count cap,
not a time window or complete request history. It contains no prompts or credentials,
and does not claim attempt or in-flight statistics. Enter inspects request ID,
tenant/client, submitted public model, resolved provider/configured model, operation,
sent HTTP status, duration and reported input/output/cache tokens. A zero token count
may mean unreported usage; unresolved targets and old-snapshot IDs are unavailable.
Detail remains on the selected completion even if its list entry expires.

Recent diagnostic text is bounded before storage: request ID, tenant, client and
provider each retain up to **256 bytes**; submitted/accounting/resolved model fields
each up to **512 bytes**; operation up to **64 bytes**. UTF-8 truncation preserves
complete characters. The detached strings total at most **2,624 bytes per entry**
(524,800 for 200 entries, excluding fixed overhead). This does not bound total
process memory or the rest of the dashboard snapshot. Full routing, billing,
rate and provider/P95 grouping retain their original semantics.

Affected Requests rows show **[truncated]**; detail lists the shortened fields.
Search covers their retained prefixes only. If the **request ID** was truncated,
exact log/payload correlation is unavailable: **l/v** stay in detail with an
explanation. Other shortened fields do not prevent correlation by an intact ID.
Old snapshots without truncation metadata keep their existing behavior.

Request metadata is displayed literally: actual newlines, carriage returns, tabs
and terminal controls appear as escapes such as `\n`, `\r`, `\t` and `\x1b`.
Literal backslashes are doubled, so a submitted backslash-n displays as `\\n`.
Printable Unicode is retained. Each completion stays on one summary row; Enter
opens that selected completion, with wrapped, scrollable escaped detail.
Request/Usage identity detail, correlation labels and payload summary/ID labels
use a **512-source-byte per-value display cap**. This preserves every newly retained
recent field; larger legacy/sibling values explicitly show
`[display clipped at 512 bytes]`. This is separate from server **[truncated]** flags.
Search and exact-ID correlation still use original retained values, not the display
escapes: typing `\n` searches for a literal backslash-n, not an actual newline.

Press **/** in Requests, Logs or Payloads. Search ANDs space-separated, case-insensitive
substrings; use `field:value` for one metadata field. Examples:
`client:ci tenant:team-a status:500` or `id:request-123`. Supported fields:

- Requests: `id`, `client`, `tenant`, `model`, `resolved`, `provider`, `status`, `op`.
- Logs: `id`, `level` (structured metadata, not flattened attributes or body text).
- Payloads: `id`, `model`, `resolved`, `provider`, `status`, `method`, `path`.

Bare terms search all supported fields. Unknown fields match nothing. Search combines
with Requests `e` errors-only, Logs level, and Payloads status filters. The applied
filter and no-match state are visible; **Ctrl+U** clears search in the focused list.
Payload status changes preserve a matching selected identity or clamp to the nearest
remaining row immediately. While the replacement list is loading, Enter waits for
the visible list before opening detail.
The editor holds at most **256 printable characters**. Arrows, Home/End,
Backspace/Delete edit; **Enter** applies, **Esc** cancels to the prior filter,
and **Ctrl+U** clears the edit. q/p/h/?/numbers/brackets are text while editing;
Ctrl+C still quits and Ctrl+R retries. Leave the editor to open help or pause.
Search only filters already bounded metadata; it never searches payload bodies or
fetches more history. Cached search and request/usage detail work while paused.

From Request detail, **l** opens exact-ID logs and **v** opens exact-ID payloads.
Target filters are temporarily bypassed; their settings and selected row return
afterward. Enter inspects a matching row; Esc closes it, then another Esc restores
Request detail/scroll/zoom. Ordinary tab/focus navigation exits this return workflow.
Logs may be disabled, expired or missing structured IDs on older servers. Payloads
may be disabled, never captured, expired or outside the latest **100** entries;
the pane explains what is known, without expanding retention. Duplicate caller IDs
can match several records. Resume before a remote payload read; late replies after
closing correlation are discarded.

## Dashboard Provider Diagnostics

Focus **Providers**, move with arrows or `j/k` (`PgUp/PgDn`, `Home/End` also work),
and press **Enter**. The `>` cursor follows the provider name across refreshes and
enabled/disabled group changes. If a provider disappears, its open detail reports
removal; returning to the list selects the nearest surviving row. `z` still zooms
the list. Details wrap and scroll even at 80×12; pause freezes their data and clock.

Provider detail includes name/display/type, sanitized effective endpoint, enabled
and routing-health state, upstream header timeout, configured probe method/path,
expected HTTP status, interval/timeout and failure/success thresholds. Probe state
is the stored threshold state; the last HTTP status and reason describe the latest
check and can differ while a threshold is being reached. Last-check age comes from
the stored check timestamp, never snapshot receipt time. Pending, absent probe,
disabled provider and unavailable old-server metadata are distinguished. No aliases
are needed to inspect a provider. Models show public-to-upstream mappings, protocol
and capabilities from the active catalog; an empty native-provider protocol means
it is not separately configured.

The list's **HOST** is the configured hostname, with no DNS/network work in rendering
or navigation. Endpoint/probe URLs omit userinfo, query strings and fragments. Only
standard API/health path segments are retained; other segments show URL-encoded
`[redacted]` to avoid exposing path credentials. Invalid URLs are redacted wholesale.
Raw probe error text becomes a bounded category (timeout, DNS, connection refused,
TLS, transport error) or a safe status/body-mismatch reason. Credentials, credential
references, probe expected bodies and authorization settings are not diagnostic
transport fields.

Alias detail carries the actual session-affinity configuration and effective header
names, including defaults; absent old-server metadata is **unknown**, not disabled.
Its counters remain **provider-wide lifetime**, repeated when targets share a
provider; they are not target-specific or alias-specific counts.

## Dashboard Stable Inspection And Pause

Refresh keeps the top provider/usage row and selected alias/payload/block by identity.
It also keeps the top cursor-list row when that row and the selection can still fit
in the viewport. If a row disappears, its old position is clamped to the remaining
list; selection visibility takes priority. Usage identity includes tenant, client,
model, operation and status. A selected tenant remains selected when other tenants
arrive or reorder; if it disappears, the filter returns to **all tenants**.
Payload order changes preserve the selected request. Logs follow new arrivals until
you move away; pinned logs stay selected by sequence ID. Changing log order resumes
follow in that order.

Press **p** in a pane or detail to freeze incoming data and the display clock,
including uptime, health, logs, measurement windows, payload/block rows, details and
decision acknowledgments. Connection status remains live and separately labeled.
Only the latest snapshot and latest accepted result per pane request type are held;
resume applies them together in one UI update and advances the clock to the latest
tick. Independent pane RPCs do not promise the same server measurement instant.

While paused, browse cached rows and use local usage/log filters or ordering. Resume
before requesting a remote refresh, payload status filter, new payload/block detail,
or block decision; these keys do not queue work while paused. A previously unvisited
pane begins loading on resume. In-flight results wait for resume, and closing a
detail discards its buffered result. Closing/reopening an ID or changing a remote
filter rejects obsolete replies. Take-once block reads and decisions are never
automatically repeated; an already consumed capture can return unavailable on a
deliberate reopen.

## Dashboard Metrics And Cost Estimates

- **GLOBAL rates:** requests/second over the last 60 and 300 complete seconds,
  plus one-minute error, 429 and token totals. The 15-minute graph contains 15
  consecutive one-minute request totals, oldest on the left. Counters use one-second
  buckets, include completed requests (including failures/unresolved models), and
  do not depend on the 200 recent-request cap. The ongoing second appears when it
  completes; normal snapshot polling adds up to approximately two seconds. Idle
  minute tokens become zero. Errors exclude 429; throttling is shown separately.
- **Provider counters:** global process-lifetime requests/errors/429/tokens. The
  same provider-wide lifetime scope applies to counters shown under alias targets.
  These do not reconcile to rolling usage/cost after old billing buckets expire.
- **P95/n:** nearest-rank P95 latency and the number of positive-duration samples
  for that provider among up to 200 received recent completions across all providers.
  This is a capped sample with **no time window**, not a minute percentile or a
  lifetime percentile. `n/a/0` means no positive-duration sample; idle samples remain.
- **USAGE and EST$:** retained rolling 24-hour usage and estimated USD at current
  configured prices. Retention uses minute buckets: a bucket remains while its
  start is at or after `snapshot time - 24h`. Individual events can therefore expire
  almost a minute early. `t`/`e` select tenant/error usage; global rates and provider
  counters ignore those filters. `u` switches between public and resolved upstream
  usage over the same retention window, preserving tenant/client dimensions.
- Alias estimates use the actual retained public-model/tenant/client/operation/status
  attribution. Direct traffic and other aliases sharing a target are not charged
  again. Unused targets do not affect the estimate. `-` means unavailable (missing
  attribution, a needed price, or no retained usage), not free. A provider estimate
  is its complete retained subtotal, not a sum of only priced models; another
  provider's price gap does not hide its own fully attributed/priced subtotal.

Older servers without the additive rate/billing snapshot data display unavailable
measurements. The TUI does not estimate rates from recent requests or costs from
lifetime totals. Paused/stale views retain the received measurement window; the
connection line separately reports local refresh/reconnect activity.

## Dashboard Transport Security

The `aiproxy dashboard` command and the `/_internal/dashboard/*` HTTP endpoints
share the proxy listener.

- Listener addresses are TCP bind addresses in `host:port` form, not URLs.
- The dashboard command is local-only. It connects over loopback plain HTTP with
  bearer authentication and refuses concrete non-loopback listener hosts.
- HTTPS and remote dashboard URLs are not supported by the current configuration
  model. Remote dashboard access requires a future explicit transport design.
- Repeated invalid dashboard tokens are rate limited with `429` and a
  `Retry-After` header.

The embedded web dashboard is served at `/` when a `web_ui` block
is present. Use `aiproxy webui` to print that URL after probing that a
running server answers with the UI (`--open` also launches the default
browser). The UI's live views authenticate against the same
`dashboard`-gated `/_internal/dashboard/*` APIs with the dashboard token or an
active stored system-administrator account. Ordinary accounts and workspace
administrators cannot access these global operator surfaces.

Within a loaded browser tab, changing accounts, changing/removing the dashboard
token, or selecting another workspace clears sensitive cached data and open
detail/dialog state. Delayed responses from the previous context are discarded.
Account replacement also resets the selected workspace. A successful normal
token refresh preserves the current account's cache and workspace; a failed
refresh clears the account session. Sign out clears the local account, workspace
selection, and dashboard token immediately, even if server-side session revocation
is delayed or fails. Dashboard tokens are installed only after successful validation.
These transitions are local to the current tab; other already-open tabs do not
automatically synchronize their in-memory sessions.

## Security Defaults

- API keys and client bearer tokens are never logged
- prompt and response bodies should be redacted or omitted from standard logs
- request IDs are emitted for correlation

## Logging

Use the optional `logging` block to control structured log verbosity and request lifecycle access logging.

```hcl
logging {
  level      = "info"
  access_log = true
}
```

When `access_log = true`, request logs include events for request receipt, upstream provider/model selection and completion, and the final response or streaming start and end.

### Payload logging

The optional nested `payload_log` block records request/response headers
and bodies as JSONL (one JSON object per line per inference request). It is
disabled by default. Each entry carries three sides: `request` (inbound
headers and body as received), `upstream_request` (headers and body actually
sent upstream, after model rewrites and provider translation; present only
when an upstream call was made, so alias retries log the final attempt), and
`response` (the upstream reply, or the proxy's own error on early rejections).
Credential headers (`Authorization`, `x-api-key`, `x-goog-api-key`, cookies)
are redacted on every side, and bodies covered by an ingress-guardrail policy
are omitted from both request sides.

```hcl
logging {
  level      = "info"
  access_log = true

  payload_log {
    enabled        = true
    dir            = "/var/log/aiproxy/payloads"
    rotation       = "daily"
    retention      = "168h"
    max_body_bytes = 1048576

    mongodb {
      uri        = env("AIPROXY_PAYLOAD_MONGO_URI")
      database   = "aiproxy"
      collection = "payloads"
      timeout    = "5s"
    }
  }
}
```

The disk backend (`dir`) and the MongoDB backend (`mongodb`) are enabled
independently: set `dir` for JSONL files, `uri` for one MongoDB document per
request with the same JSON field names, or both to fan out to both. When
enabled, at least one backend is required. Recording is best-effort and never
fails a request; a bad MongoDB URI fails startup fast instead.

The dashboard and TUI payload viewer read from the disk backend only: with
MongoDB-only logging (no `dir`), the viewer reports payload logging as
disabled.

Files are split by datetime so no single file grows without bound: `daily`
rotation writes `payload-YYYYMMDD.jsonl` and `hourly` writes
`payload-YYYYMMDD-HH.jsonl` (UTC) under `dir` (`0600` files, `0700`
directory). `retention` (default 7 days, `"168h"`) controls how long files are
kept: files older than the retention window are removed on rotation and by an
hourly sweep; `"0s"` disables expiry. Bodies larger than `max_body_bytes` per
side are truncated with `"truncated": true` (`0` stores full bodies), and
sensitive headers (`authorization`, `proxy-authorization`, `cookie`,
`set-cookie`, `x-api-key`) are stored as `[REDACTED]`.

Only enable payload logging when you can protect the output directory:
bodies contain prompts and completions. Changes apply on `SIGHUP` reload.

## Secret Handling

When `api_key_ref` is used, the default key file path is:

- `$XDG_CONFIG_HOME/aiproxy/keys.json`
- or `~/.config/aiproxy/keys.json`

Mount this file read-only in production deployments.

GitHub Copilot logins live beside that file as `copilot-<name>.json`
sidecars (`0600`, restrictive parent directory). `credential_ref.path`
defaults to the same secrets path; mount the secrets directory (not just
`keys.json`) when Copilot providers are configured, and reload with `SIGHUP`
or a restart after every `login` or sidecar rotation.

## Mock-only Copilot Verification

**Hermetically verified; live GitHub compatibility unverified.** From the repo
root, with the repository Go/pnpm toolchain and dependencies installed, run these
commands serially. Finish the UI build before Go checks read embedded assets:

```sh
make web-build
go test -race ./internal/copilotlogin ./cmd/aiproxy ./internal/config ./internal/app ./internal/provider -count=1
make integration
AIPROXY_BINARY="$PWD/dist/aiproxy" go test -tags=integration ./internal/integration -run GitHubCopilot -count=1 -v
```

No real client ID, account, browser authorization, or PostgreSQL fixture is needed
for Copilot scenarios. Binary integration runs on Linux. The final command forces
the Copilot binary cases to execute uncached and shows their names. Optional
database-backed tests in broader suites may skip without their separate fixture.

| Layer                                                                              | What the checks establish                                                                                                                                                                                                                                                                                                                                                                     |
| ---------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Protocol/persistence unit tests (`internal/copilotlogin`; command `login_test.go`) | Success, pending/slowdown, denial/expiry, cancellation, malformed/oversized responses, redirect/network errors, secret redaction, file permissions, unsafe destinations and independent writers.                                                                                                                                                                                              |
| Composed in-process (`cmd/aiproxy`, `TestCopilotMockProvisioningAndRecovery`)      | Real `runLoginCopilot` command orchestration with injected local issuer/time, persisted sidecar, scripted configure/validate and models commands, real App handler over a local HTTP server, configured/upstream model distinction, JSON/SSE, simulated 401/403 revocation, failed/cancelled re-login preservation, successful re-login, failed-reload rollback and explicit reload recovery. |
| Real binary (`internal/integration`, `TestBinaryGitHubCopilot*`)                   | CLI flag constraints, real `serve`, `models --upstream`, static inventory, JSON/SSE, unsupported-operation rejection, 401/403 and sidecar rotation, rejected/successful SIGHUP reloads. Binary tests provision synthetic sidecars directly through `Save`; they do **not** run device authorization in the child process.                                                                     |

The composed test uses the existing in-process issuer seam, not a public mock
flag. Its transport refuses non-loopback dials. The new binary recovery fixture
routes unexpected non-loopback HTTP(S) through a rejecting local proxy; all
intended service URLs are loopback. Tests close local servers, application
resources, response bodies and child processes, and use automatically removed
temporary config/credential directories. They do not modify your saved login.

These checks simulate the upstream contract. They cannot establish direct-Bearer
acceptance for your application, entitlement, actual model availability, required
GitHub headers, token lifetime, or exchange/refresh needs. Live compatibility is
deferred separately in repository task **COPILOT-LIVE-01**
(`docs/tasks/20260927-102347-copilot-live-compatibility.md`). Production login still
requires an explicitly supplied client ID and uses GitHub's fixed issuer URLs.

## Production Checklist

- enable `bearer_static` auth unless the deployment is fully trusted
- keep provider secrets out of the HCL file when possible
- mount config and key files read-only
- declare `metrics { token = env("AIPROXY_METRICS_TOKEN") }` and scrape
  `GET /metrics` with the configured bearer token
- use `aiproxy dashboard` only from the local host; remote dashboard access is
  unsupported until an explicit transport design is added
- explicitly `enabled = false` any provider you want to keep defined but
  inactive; missing credentials on enabled providers fail validation
- use aliases for controlled failover instead of relying on direct model requests
