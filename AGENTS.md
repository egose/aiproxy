# AGENTS.md

Operational guide for AI agents (and humans) working in this repo.

## Build & run

| Command                                                    | Effect                                                    |
| ---------------------------------------------------------- | --------------------------------------------------------- |
| `make build`                                               | Build `dist/aiproxy` for the host platform (CGO disabled) |
| `pnpm release:build -- --version 1.2.3`                    | Build all release archives and `dist/checksums.txt`       |
| `pnpm release:verify -- --version 1.2.3`                   | Verify release archives and checksums                     |
| `pnpm release:verify -- --version 1.2.3 --reproducibility` | Verify two independent release builds are reproducible    |
| `make run CONFIG=path/to/config.hcl`                       | `go run` the server against a config                      |
| `make validate CONFIG=path/to/config.hcl`                  | Load + validate config without serving                    |
| `make docker-build`                                        | Multi-stage container build as `aiproxy:$(VERSION)`       |
| `make docker-run CONFIG=path/to/config.hcl`                | Run the container image with a mounted config             |

The CLI defaults to `$XDG_CONFIG_HOME/aiproxy/config.hcl`, falling back to
`~/.config/aiproxy/config.hcl` when `XDG_CONFIG_HOME` is unset. Pass
`--config` to use a different file. Set `$AIPROXY_CONFIG` to inline HCL to
skip the config file (explicit `--config` overrides it; `serve -d` and
`configure`/`login` file workflows require a file).

The CLI also includes:

- On Linux, `aiproxy serve -d` (`--daemon`) to background `serve`, redirecting logs to
  `$XDG_CONFIG_HOME/aiproxy/aiproxy.log` and writing a per-config daemon state
  record under the same `aiproxy/` subdir of `XDG_CONFIG_HOME` (or
  `~/.config/aiproxy/` when `XDG_CONFIG_HOME` is unset). Daemon state is scoped
  by the canonical config path, so lifecycle commands should use the same
  `--config` value that started the background server.
- On Linux, `aiproxy stop`, `aiproxy status`, `aiproxy restart` for lifecycle control
  of a backgrounded daemon. They verify the recorded executable and process
  start identity before signaling; if no matching live daemon is present, all
  three print `no server running` and exit non-zero. On non-Linux platforms,
  foreground `serve` is supported, but daemon lifecycle commands return
  `daemon lifecycle is unsupported on this platform`.
- `aiproxy dashboard` to attach the interactive TUI to a **running**
  `aiproxy serve`. It requires a `dashboard { ... }` block in the config; the
  dashboard command calls `/_internal/dashboard/snapshot` on the server's
  listener (bearer-token gated) and prints `no server running` if the
  server is unreachable, `dashboard not configured` if the block is
  absent, or `unauthorized: dashboard token mismatch` if the token in the
  config does not match the one the server is using.
  The block's `token` attribute is **optional**: when omitted, `serve` mints a
  random secret at startup and persists it to
  `$XDG_CONFIG_HOME/aiproxy/dashboard.token`, and the `dashboard` command
  reads that file to authenticate. When `token` is declared explicitly, the
  file is not written at startup and that value is used as-is. If a `SIGHUP`
  reload drops a previously declared token, the carried-over secret is
  published to the file before the new runtime activates; a persistence
  failure rejects the reload with the old runtime intact. The minted token survives
  `SIGHUP` reloads unchanged. Self-hosted reload is still `SIGHUP` for the
  server side.
- `aiproxy webui` to print the embedded web dashboard URL (`/`)
  of a **running** `aiproxy serve`, after probing that the server answers
  with the UI. It requires a `web_ui` block in the config and prints
  `no server running` when unreachable; pass `--open` to also launch the
  default browser. The UI's live views still authenticate against the
  `dashboard`-gated `/_internal/dashboard/*` APIs. With multi-tenancy enabled,
  the browser uses the session of a currently stored system administrator,
  ignoring saved dashboard tokens; otherwise it uses the dashboard token.
  These APIs are global operator surfaces; ordinary user and workspace
  administrator sessions receive `403`.
  The UI never claims API surfaces or the reserved `/dashboard` path.
- `aiproxy paths` to print resolved config and secrets paths
- `aiproxy examples` for boxed command/config examples
- `aiproxy configure` for interactive config editing
- `aiproxy configure <block> --non-interactive ...` for scripted config updates
- `aiproxy convert [target-file]` to translate the config between HCL and JSON
  (direction auto-detected; source defaults to the usual config context,
  target defaults to the current working directory; `env()` calls resolve at
  conversion time and output is validated before writing)

The HCL config uses `env("VAR")` for secret/placeholder substitution; values
are textually inlined **before** HCL parsing. The same inlining applies to the
JSON config form, where `env("VAR")` is written bare as the value. Run
`set -a; . ./.env; set +a` before invoking the binary locally so env vars resolve.

The server supports `SIGHUP`-triggered live config reload for auth, providers,
models, static model-ID passthrough rules and grants, aliases, root and provider
upstream header timeouts, provider TLS ClientHello profiles and custom recipes,
root and provider user-agent settings, root/provider/model/alias encrypted-reasoning
policies and fixed upstream headers, access-log enablement, payload-log
configuration, metrics config, provider-health config, ingress-guardrail policy,
web UI enablement, and metrics-backed inventory state. When multi-tenancy is enabled, database-backed
providers, aliases, and inbound keys merge into the serving catalog on the same reload path (and
automatically after admin API mutations and durable-revision polling every 2s);
invalid catalogs retain the old catalog but keys refresh independently. DB-backed
new admission fails closed after 30s without confirmed freshness; admitted work
continues and trusted HCL clients/inference remain available. `/readyz` fails on
expired DB freshness, `/healthz` does not. System-admin `/_internal/admin/serving-status`
and activation headers report local applied revisions, never cluster acknowledgement.
File policies still need per-replica deployment/reload; see
`website/docs/operations.md#bounded-replica-convergence`.
Listener address, listener timeout, logging level, and enabling the
dashboard after startup require restart.

Peer deferred policy also reloads subject to live reservation checks. Removing
root `deferred` and receiver declarations stops new admission but preserves
authenticated controls/replay for draining existing jobs; restart loses the
process-local store. See `website/docs/operations.md#peer-reload-and-finalization`.

## Lint / typecheck / test

| Command              | Effect                                                  |
| -------------------- | ------------------------------------------------------- |
| `make vet`           | `go vet ./...`                                          |
| `make test`          | `go test ./...` (unit tests only)                       |
| `make test-race`     | `go test -race ./...`                                   |
| `make cover`         | Unit tests with coverage profile at `dist/coverage.out` |
| `make docs-contract` | Verify public docs contract matrices stay aligned       |
| `make helm-test`     | Verify Helm NetworkPolicies (requires Helm)             |

`make vet test` is the default pre-commit sanity check; run it after any
non-trivial change. There is no separate typecheck target — Go's compiler
is the typecheck, and `make build` exercises it.

Hermetic binary-level integration tests run in normal CI through `make
integration`. Real-provider sandbox tests remain separate and optional.
Markdown-only documentation pull requests run the `Docs Contract` workflow,
which executes `make docs-contract` against the highest-drift public support
matrices.

`pnpm docs:build` builds the documentation website, not the embedded web UI.
The optional standalone Bun capture tool is documented in
`scripts/tls-clienthello/README.md`; Bun is not a serving or fixture-test dependency.

## Conventions

- **No comments** in source files unless the surrounding code dictates
  otherwise — the design doc at `docs/design.md` holds the rationale.
- Module path: `github.com/egose/aiproxy`.
- All HCL blocks use two-label syntax: `provider "openai" "openai" {}`.
- Provider/alias names are lowercase, no spaces, no `/`; provider name `alias`
  is reserved. Model names are lowercase, no spaces, and may contain `/` when
  every slash-separated segment follows the same lowercase name rule.
- Public model strings: `<provider-name>/<model-name>` or `alias/<alias-name>`.
- Providers normally declare exactly one of `api_key` or `api_key_ref`;
  enabled providers with unresolved, empty, or missing credentials fail
  validation, except `opencode-zen` providers, which may omit the credential
  for keyless upstream access (no `Authorization` header is sent). To intentionally
  disable a provider, set `enabled = false`.
  Provider-only `allow_insecure_base_url = true` opts a provider into a
  non-loopback plain-HTTP `base_url` (cleartext transport for trusted
  in-cluster networks); `https` is always allowed, other URL rules stay
  enforced, derived providers cannot declare it locally (it is inherited with
  the base URL), and there is no root default, env var, or CLI bypass.
  Provider-only `forward_authorization = true` instead forwards one valid caller
  `Authorization` header unchanged on live inference for `openai`,
  `openai-compatible`, `opencode-zen`, `opencode-go`, `zenmux`, and `openrouter`.
  It cannot coexist with configured credentials; missing or invalid headers fail
  with `400` before upstream I/O. Proxy auth still applies, so its caller token
  is exposed to the trusted upstream. Derived providers must opt in locally.
  `api_key_ref.path` defaults to `$XDG_CONFIG_HOME/aiproxy/keys.json`, falling
  back to `~/.config/aiproxy/keys.json`.
- Non-System database providers/probes default-deny and require exact public HTTPS
  origins in operator config `multi_tenancy.allowed_upstream_origins` (max 128,
  omission/`[]` denies all). Destination workspace policy applies even to global
  admin authors and disabled/old rows, including inherited effective settings and
  credential-only writes, before decryption/file resolution/commit. Tenant own
  encrypted DB API keys and own Copilot DB device credentials remain supported;
  nonempty host API-key/sidecar references, caller Authorization forwarding and
  cleartext exceptions are System-only. Every restricted new connection rejects
  any non-public DNS answer, dials pinned public IPs (max 64 answers, 30s DNS/TCP
  budget), ignores env proxies and retains verified TLS/built-in profiles and
  same-origin redirects. Trusted System/HCL/file-only behavior is unchanged.
  Approval never grants foreign/System resource or credential rights. Run candidate
  `validate --check-db` before upgrade; disabling a row is not a policy repair,
  and a rejected reload retains old approvals. See
  `website/docs/operations.md#tenant-provider-policy-upgrade`.
- Providers may declare `extends = "<base-provider-name>"` to inherit the base
  provider type, endpoint, timeout, enabled state, and models while using their
  own provider name and local credential. Derived providers normally declare
  `extends`, optional `display_name`, and at most one local `api_key` or
  `api_key_ref` (exactly one, except `opencode-zen` derivatives which may omit
  it for keyless access, or derivatives explicitly setting
  `forward_authorization = true` which must omit it); the base must be enabled,
  concrete, and the same type.
- Optional root/provider/model/alias `encrypted_reasoning` blocks replace
  wholesale. Direct precedence is model > provider > root > built-ins; explicit
  alias policy wins over the selected target's policy. Missing inherits; `{}`
  resets to passthrough true, mismatch `fail`, built-in patterns; omitted members
  never merge lower-layer values. Patterns normalize/trim to 4–256 Go bytes,
  at most 16, unique case-insensitively; `[]` selects built-ins.
  Declared base provider/model policies copy by value; inherited model policy
  still outranks a child provider override. Local existing-name model policy
  overlays can reset/override it, without model redefinition or credential/chain
  relaxation. Applies only to chat completions/Responses, never native messages.
  Recovery requires ordinary non-streaming upstream 400, raw opaque request
  markers, bounded decoded pattern matching, and a changed stripping rewrite:
  one same-identity/context extra attempt (direct max 2, alias max N+1 once
  total). Immediate 400s to streaming requests may qualify; established streams,
  peer controls, cancellation and exhausted budgets never authorize recovery.
  Explicit aliases keep stripped bodies through normal failover; absent alias
  policy uses each target's policy against the immutable admitted/guardrailed
  original, sharing the recovery allowance. Pre-stripping adds no recovery call
  or allowance use. Ordinary text/tool history survives, but typed unsigned
  thinking/reasoning and chat reasoning fields can be stripped too; no universal
  plaintext-reasoning preservation or output decryption.
  Managed provider/model storage keeps nullable sparse local intent; PUT omitted
  provider/alias policy preserves, null removes, `{}` resets, object replaces.
  Omitted/null `models` preserves the collection; arrays replace local rows/
  overlays, with derivative `[]` retaining inherited inventory. Managed overlays
  accept name+policy only (no deferred/definition keys, even empty/null).
  Credential PUT rejects policy/model edits. GET exposes local nullable intent,
  full inherited inventory and read-only effective/source/inherited metadata;
  undeclared alias effective policy is null. Root has no admin edit surface or
  RBAC expansion. Provider/model/alias web controls expose inherit/reset/custom,
  preserve untouched local intent by omission, and show read-only saved context,
  not unsaved resolution or proof of runtime activation. Static rows stay read-only.
  See
  `website/docs/configuration.md#scoped-encrypted-reasoning` and
  `website/docs/api-reference.md#encrypted-reasoning-policy-intent`.
  Exact doc/example fixtures run with
  `go test ./internal/config -run DocumentedEncryptedReasoning -count=1`.
- Peer Deferred Transport v1 uses unlabeled root `deferred`, static
  `openai-compatible` model-level caller `deferred_transport`, and provider-only
  static chat `deferred_receiver` blocks. A local provider caller block remains
  a compatibility default: a model block replaces it wholesale, with standard
  caller defaults for omitted fields; without a model caller block, use the local
  provider default or ordinary transport when neither exists. There is no opt-out flag;
  remove the provider default to mix ordinary and deferred models. Only chat
  JSON/SSE uses peer transport; other operations remain ordinary. Receiver
  eligibility is additive and does not defer ordinary calls, but a receiver
  cannot coexist with any caller setting on the same provider. Static derivatives
  inherit model definitions, never base caller/receiver opt-ins; local existing-name
  model overlays may contain `deferred_transport`, `encrypted_reasoning`, and/or
  `upstream_headers`, with no model definition attributes. Deferred opt-ins are
  incompatible with `forward_authorization` and rejected on managed provider/credential writes
  (including nested models) and managed inheritance. Callers require configured
  `api_key`/`api_key_ref` even when disabled. Peer control failures never authorize
  fresh inference. There are no CLI/web creation controls. Full configurations
  in `website/docs/configuration.md#peer-deferred-transport-v1` are checked by
  `go test ./internal/config -run DocumentedDeferred -count=1`; API and limits are
  in `website/docs/api-reference.md` and `website/docs/operations.md`.
- Any provider may declare an optional `user_agent` override (1-256 printable
  ASCII characters, no newlines). It replaces the default `aiproxy/<version>`
  upstream `User-Agent` on every proxy-initiated request for that provider
  (inference, upstream model listing, and health probes); the inbound caller's
  `User-Agent` is never forwarded. A provider may instead set
  `forward_user_agent = true` to forward the inbound caller `User-Agent`
  upstream on live inference requests (explicit `user_agent` still wins;
  missing or invalid inbound values fall back to the default).
  Both attributes also exist at root scope as defaults for all providers:
  a provider-level `user_agent` overrides the root value, while
  `forward_user_agent` is effective when set at either level.
- Static `upstream_headers` maps at root/provider/model/alias scope merge names
  case-insensitively, with alias > selected model > selected provider > root >
  permitted caller value. Static derivatives never inherit base maps. Managed
  destinations, probes, discovery and outgoing deferred control/submit metadata
  get no fixed values; receiver inference uses local static scopes. The proxy
  masks applied values in upstream-request captures, not upstream logs or echoed
  responses. HCL edits preserve maps and hide values in previews; conversion
  materializes `env()` values. See
  [fixed upstream headers](website/docs/configuration.md#fixed-upstream-headers).
- Provider-only `tls_client_hello` accepts `native` (omitted default), `chrome`,
  `chrome-133`, `safari`, `safari-16.0`, or `custom`. Pinned uTLS v1.8.2 maps aliases
  to bundled Chrome 133/Safari 16.0, never a live/latest browser catalog. Empty,
  wrong-case and unknown selectors fail; Firefox/Bun/BoringSSL/randomized aliases
  are not supported. `custom` requires `tls_client_hello_spec_file`; any file
  declaration on non-custom is invalid, even empty. There is no root default or
  bypass knob. Static derived providers inherit both fields with the endpoint and
  reject local declarations; disabled profiles are validated too.
  Custom recipes are strict native-shaped JSON (64 KiB, depth 8, 4096 tokens,
  64 suites, 32 extensions) loaded into immutable cached profiles during candidate
  validation. Runtime validation and requests never reopen files; each connection
  gets fresh SNI/random/key material. Relative paths use the config file's lexical
  directory; inline/source-less config requires absolute paths. In-place CLI edits
  preserve references and `env()` spelling; conversion emits resolved absolute
  host-local references without copying files. See
  `website/docs/configuration.md#upstream-tls-clienthello` and `docs/design.md`.
  Profiles apply to inference, health and model discovery HTTPS, including absolute
  HTTPS health URLs for HTTP-based providers. Plain HTTP remains unchanged. Go 1.27
  HTTP/1.1/HTTP/2 follows ALPN; HTTP/1.1 CONNECT through HTTP/HTTPS proxies is
  supported, with separate verified standard Go proxy TLS. Non-native SOCKS,
  HTTP/2 CONNECT, QUIC, resumption and selected ALPS (even empty) fail explicitly.
  Never disable certificate/hostname verification, enable weak negotiation, rewrite
  fingerprints to hide unsupported behavior, or fall back/cycle profiles. Retiring
  successful-reload transports let admitted requests/alias attempts/streams finish;
  failed reloads retain old cached bytes even after same-file replacement.
  Managed providers support built-ins only, including effective inheritance; reject
  custom/file/inline data before commit for every admin role and credential PUT.
  A managed derivative cannot extend a static custom base. API views expose selector,
  effective/inherited/static-only metadata, never profile paths or contents. This
  adds no provider capability or global authorization surface and does not configure
  OAuth/device/dashboard clients or the upstream `User-Agent`.
- Direct (`<provider>/<model>`) requests never fail over to a different
  target and never consult or populate alias cooldown state. Alias requests retry the next target on transport errors, timeouts,
  and configured `retry_status_codes` in the `400`-`599` range. The default list
  is `500`, `502`, `503`, and `504`; outside the conditional same-target reasoning
  recovery above, other upstream `4xx` responses are returned verbatim.
  Retryable `4xx` statuses do not mark providers unhealthy.
- Alias targets honor upstream retry advice (`retry-after-ms` wins, else
  `Retry-After` delay-seconds then HTTP-date, else exhausted Meta quota
  (`x-ratelimit-remaining-tokens` / `x-ratelimit-remaining-requests` of `0`
  cools for 60s); first valid value wins; zero, malformed, past, or overflow
  values record nothing) as a process-local
  cross-request cooldown keyed by `(alias, provider, model)` and shared across
  that alias's operations. Cooling targets are excluded until expiry; when all
  pool targets actively cool, dispatch returns a generated JSON `429`
  (`upstream_rate_limited`) with ceiling `Retry-After` (min 1) and
  `retry-after-ms` (min 1) from the same earliest remaining delay and zero
  upstream calls. A retryable failure newly cooling the last target becomes
  synthetic `429` immediately; successes and non-retryable errors return
  verbatim and cool future requests only. Deadlines survive `SIGHUP` for
  fingerprint-unchanged targets (`base_url`, credential, upstream model,
  protocol) and are dropped otherwise; failed reloads leave state untouched.
  Another process and previously admitted in-flight requests are not
  coordinated, and cooldown never mutates provider health.
- The optional `auth.rate_limit` block applies a local in-memory request rate
  limit. In `bearer_static` mode it is keyed per authenticated client; in
  `none` mode it uses a shared anonymous bucket.
- Static `auth.client` blocks may also define optional `tenant` and
  `allowed_models` fields. `allowed_models` is enforced against literal public
  model and alias names.
- Static concrete `openai`, `openai-compatible`, `openrouter`, `zenmux`,
  `anthropic`, `gemini`, `opencode-zen`, `opencode-go`, and `github-copilot`
  providers may declare `model_passthrough "<rule>"` alongside or instead of
  literal models. Exact hits always win. On a miss, require a unique full Go
  regexp match (32 rules/provider, 512 bytes/pattern, 256 bytes/suffix),
  explicit supported capability, and a separate System static client
  `allowed_model_passthroughs = ["<provider>/<rule>"]` grant. File-only `none`
  explicitly allows anonymous use of configured rules; DB keys, including System,
  cannot use them. Zero/overlapping/unusable rule matches keep the literal
  `403`/`404`; protocol suitability never breaks a tie. Zen/Go rules require an
  explicit `protocol` (`chat`, `responses`, `messages`, or Zen-only `gemini`);
  other providers must omit it. Rule calls use the configured adapter,
  destination and credential policy (Copilot uses its loaded `credential_ref`,
  mock-only verification). Body adapters keep the lowercase slash-separated
  grammar and send the suffix in `model`; Gemini and Zen `gemini` use one bare
  ID in a fixed URL path. Path rules permit base path prefixes but no query or
  fragment delimiters. Gemini rule embedding batches use `models/<id>` in every
  generated `requests[].model`; literal and alias batches do not change.
  Require a canonical JSON/multipart `model`; reject `models` and `preset`
  on all rule calls and top-level `fallbacks` only on native Messages rule
  calls, including after guardrail rewrites. No discovery, model cards, alias
  targets, inheritance, peer transport, managed creation or new operations.
  Use trusted upstreams and narrow regexes: rule usage is grouped under
  `<provider>/@passthrough/<rule>` and unpriced. CLI edits preserve declarations;
  `SIGHUP` publishes rules (including protocols) and grants together or retains
  the prior snapshot. Admitted streams are unchanged. See the exact-tested
  [HCL/JSON configurations](website/docs/configuration.md#model-id-passthrough).
- `GET /v1/billing/usage` is a rolling, process-local usage/cost estimate, not an
  invoice. DB bearers see only their immutable credential/workspace pair, including
  earlier label snapshots; rotation preserves history and deletion/recreation does
  not inherit it. HCL tenant reporting includes only same-tenant static clients in
  System; tenantless HCL clients see only their own tenantless static usage.
  File-only anonymous reporting remains unfiltered. Workspace-wide/operator reads
  require separate explicit authority; billing query/header labels cannot grant it.
  Events, grouping and alias-cost joins retain internal credential/workspace identity
  before filtering; these IDs are not JSON fields or new metrics labels.
- Provider health state is shared in-process across requests and alias routing.
  Transient transport failures, upstream request errors, and upstream `5xx`
  responses temporarily mark a provider unhealthy. Configured retryable `4xx`
  statuses can trigger alias failover without mutating provider health. An
  optional `provider_health` Redis config can share transient health state across
  instances; on Redis read failure routing and readiness use a bounded
  in-process cache (`cache_ttl`, default 30s) and fail open only when no fresh
  cache entry exists, recording both the backend error and the fallback reason as
  Prometheus metrics.
- `/metrics` requires a dedicated bearer token declared in a
  `metrics { token = env("...") }` block; the token is checked independently of
  API auth client tokens. An empty or missing token is rejected at validation.
- Listener addresses are TCP bind addresses in `host:port` form, not URLs. The
  `aiproxy dashboard` command is local-only: it connects over loopback plain
  HTTP using bearer authentication and refuses non-loopback listener hosts.
  Remote dashboard support requires a future explicit transport design.
  Repeated invalid dashboard tokens are rate limited with `429`.
- Apart from explicit ingress guardrails and reasoning stripping, the
  openai/openai-compatible adapter is pass-through: it only rewrites the
  `model` field to the configured `upstream_name`, injects the upstream
  `Authorization: Bearer` header, and copies the body (including SSE streams)
  back. `anthropic` and `gemini` use built-in request/response translation for
  chat completions. `anthropic` additionally serves native `POST /v1/messages`
  passthrough (model rewrite only, thinking/tools preserved). `gemini` also supports embeddings translation, and both
  translated providers support a conservative `/v1/responses` subset for both
  JSON and SSE streaming. `POST /v1/images/generations` and
  `POST /v1/audio/transcriptions` and `POST /v1/audio/speech` are currently
  supported only for `openai` and `openai-compatible`. Anthropic embeddings are
  still deferred.
- `opencode-zen` (default `https://opencode.ai/zen/v1`) and `opencode-go`
  (default `https://opencode.ai/zen/go/v1`) share one adapter behind two
  explicit types; the type selects the service, never the URL or credential.
  Every model declares a required `protocol` (`chat`, `responses`, `messages`,
  or `gemini`; `gemini` is Zen-only): `chat`/`responses` are native
  pass-through serving one public operation each, `messages` is native
  Anthropic passthrough for `POST /v1/messages` (plus conservative translation
  for `chat` and `responses`), and `gemini` serves `chat` and `responses`
  through the existing conservative translation
  subsets. Unsupported operation/protocol combinations are rejected before
  upstream I/O. `base_url` is an optional transport override only. Every
  upstream request sends `User-Agent: aiproxy/<version>` unless the provider
  declares a `user_agent` override; `opencode-zen` and
  `opencode-go` additionally send `x-opencode-session` (a caller value is forwarded only
  when valid, falling back to a valid caller `X-Session-Id`, otherwise a fresh per-request ID is generated)
  and forward a caller-supplied `x-opencode-client` under the same validity
  rule. No other inbound
  headers or credentials are forwarded. Direct requests never cross services;
  Zen/Go mixing happens only through explicitly configured aliases, and the
  upstream "spend Zen balance past Go limits" console setting never permits
  proxy-side rerouting. Model catalogs are static config; the proxy does no
  runtime catalog sync.
- `github-copilot` (default `https://api.githubcopilot.com`) is chat-only
  (`POST /v1/chat/completions` JSON and SSE; every other operation is rejected
  before upstream I/O). Provision with
  `aiproxy login github-copilot --client-id <id> --credential <name>` using an
  explicitly supplied public OAuth client ID (no secret, no endpoint-override
  flags); the command writes a structured sidecar
  `<secrets-dir>/copilot-<name>.json` (`0600`) and never edits HCL or signals
  a server. Providers declare `credential_ref { name = ... }` (`path`
  optional, defaults to the shared secrets path); the token resolves at load
  into a dedicated field (never `api_key`), activates on restart/`SIGHUP`,
  and requires re-login + reload on `401`/`403`/revocation/expiry. Derived
  Copilot providers require their own local `credential_ref`. Upstream
  inference is `POST {base}/chat/completions` with adapter-managed headers
  (`Authorization`, proxy `User-Agent`, `X-GitHub-Api-Version`,
  `Openai-Intent`, derived `x-initiator: user`, vision only on image bodies);
  static inference can add non-reserved fixed headers. Inbound auth/cookies/
  `x-api-key`/caller Copilot metadata are stripped.
  `aiproxy configure provider --credential/--credential-path` references the
  saved login without OAuth networking or token display, and upstream model
  listing (`GET {base}/models`) shares the same auth without changing the
  static inventory. With multi-tenancy enabled, the web provider form offers
  Connect GitHub as a database alternative: enter the same explicitly supplied
  public OAuth client ID, approve at the shown URL with the shown code, then
  save the provider to apply. The server performs the device challenge over
  the fixed issuer/scope/verification page and stores the result as an
  AES-GCM-encrypted database credential (never a sidecar, never a raw-token
  API input); the browser receives only the authorization URL/code and status.
  Database credentials require the shared database encryption key on every
  instance. Abandoned device sessions expire and are reaped by a bounded
  per-minute cleanup (100/batch); provider saves set
  `X-Aiproxy-Catalog-Saved: true` once the commit is durable, including
  saved-but-activation-failed outcomes, and activation applies to the receiving
  instance immediately (no cluster broadcast), with other replicas reconciling durable
  DB revisions independently. Hermetically verified; live GitHub compatibility unverified.
  Mock-only checks and their coverage layers are documented in
  `website/docs/operations.md#mock-only-copilot-verification`; live compatibility
  remains deferred in task COPILOT-LIVE-01.

Public endpoint/provider support matrix:

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

Provider capability defaults and additional supported capabilities:

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
