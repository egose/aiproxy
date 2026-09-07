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
`--config` to use a different file.

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
  file is not written and that value is used as-is. The minted token survives
  `SIGHUP` reloads unchanged. Self-hosted reload is still `SIGHUP` for the
  server side.
- `aiproxy paths` to print resolved config and secrets paths
- `aiproxy examples` for boxed command/config examples
- `aiproxy configure` for interactive config editing
- `aiproxy configure <block> --non-interactive ...` for scripted config updates

The HCL config uses `env("VAR")` for secret/placeholder substitution; values
are textually inlined **before** HCL parsing. Run `set -a; . ./.env; set +a`
before invoking the binary locally so env vars resolve.

The server supports `SIGHUP`-triggered live config reload for auth, providers,
models, aliases, root and provider upstream header timeouts, access-log
enablement, metrics config, provider-health config, and metrics-backed inventory
state. Listener address, listener timeout, logging level, and enabling the
dashboard after startup require restart.

## Lint / typecheck / test

| Command              | Effect                                                  |
| -------------------- | ------------------------------------------------------- |
| `make vet`           | `go vet ./...`                                          |
| `make test`          | `go test ./...` (unit tests only)                       |
| `make test-race`     | `go test -race ./...`                                   |
| `make cover`         | Unit tests with coverage profile at `dist/coverage.out` |
| `make docs-contract` | Verify public docs contract matrices stay aligned       |

`make vet test` is the default pre-commit sanity check; run it after any
non-trivial change. There is no separate typecheck target — Go's compiler
is the typecheck, and `make build` exercises it.

Hermetic binary-level integration tests run in normal CI through `make
integration`. Real-provider sandbox tests remain separate and optional.
Markdown-only documentation pull requests run the `Docs Contract` workflow,
which executes `make docs-contract` against the highest-drift public support
matrices.

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
  `api_key_ref.path` defaults to `$XDG_CONFIG_HOME/aiproxy/keys.json`, falling
  back to `~/.config/aiproxy/keys.json`.
- Providers may declare `extends = "<base-provider-name>"` to inherit the base
  provider type, endpoint, timeout, enabled state, and models while using their
  own provider name and local credential. Derived providers may only declare
  `extends`, optional `display_name`, and exactly one local `api_key` or
  `api_key_ref`; the base must be enabled, concrete, and the same type.
- Direct (`<provider>/<model>`) requests never fail over to a different
  target. Alias requests retry the next target on transport errors, timeouts,
  and configured `retry_status_codes` in the `400`-`599` range. The default list
  is `500`, `502`, `503`, and `504`; other upstream `4xx` responses are returned
  verbatim. Retryable `4xx` statuses do not mark providers unhealthy.
- The optional `auth.rate_limit` block applies a local in-memory request rate
  limit. In `bearer_static` mode it is keyed per authenticated client; in
  `none` mode it uses a shared anonymous bucket.
- Static `auth.client` blocks may also define optional `tenant` and
  `allowed_models` fields. `allowed_models` is enforced against proxy-visible
  model names.
- `GET /v1/billing/usage` exposes aggregated in-process usage summaries,
  filtered to the caller's tenant when present, otherwise to the caller's
  client identity.
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
- The openai/openai-compatible adapter is pass-through: it only rewrites the
  `model` field to the configured `upstream_name`, injects the upstream
  `Authorization: Bearer` header, and copies the body (including SSE streams)
  back. `anthropic` and `gemini` use built-in request/response translation for
  chat completions. `gemini` also supports embeddings translation, and both
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
  pass-through serving one public operation each, while `messages`/`gemini`
  serve `chat` and `responses` through the existing conservative translation
  subsets. Unsupported operation/protocol combinations are rejected before
  upstream I/O. `base_url` is an optional transport override only. Every
  upstream request sends `User-Agent: aiproxy/<version>`; `opencode-zen` and
  `opencode-go` additionally send `x-opencode-session` (a caller value is forwarded only
  when valid, falling back to a valid caller `X-Session-Id`, otherwise a fresh per-request ID is generated). No other inbound
  headers or credentials are forwarded. Direct requests never cross services;
  Zen/Go mixing happens only through explicitly configured aliases, and the
  upstream "spend Zen balance past Go limits" console setting never permits
  proxy-side rerouting. Model catalogs are static config; the proxy does no
  runtime catalog sync.

Public endpoint/provider support matrix:

<!-- docs-contract:public-matrix:start -->

| Surface                         | `openai`                           | `openai-compatible`                | `anthropic`                        | `gemini`                           | `opencode-zen`                           | `opencode-go`                            |
| ------------------------------- | ---------------------------------- | ---------------------------------- | ---------------------------------- | ---------------------------------- | ---------------------------------------- | ---------------------------------------- |
| `GET /v1/models`                | Proxy-owned                        | Proxy-owned                        | Proxy-owned                        | Proxy-owned                        | Proxy-owned                              | Proxy-owned                              |
| `GET /v1/billing/usage`         | Proxy-owned local usage accounting | Proxy-owned local usage accounting | Proxy-owned local usage accounting | Proxy-owned local usage accounting | Proxy-owned local usage accounting       | Proxy-owned local usage accounting       |
| `GET /metrics`                  | Proxy-owned Prometheus metrics     | Proxy-owned Prometheus metrics     | Proxy-owned Prometheus metrics     | Proxy-owned Prometheus metrics     | Proxy-owned Prometheus metrics           | Proxy-owned Prometheus metrics           |
| `POST /v1/chat/completions`     | JSON and SSE                       | JSON and SSE                       | JSON and SSE translated            | JSON and SSE translated            | JSON and SSE native or translated subset | JSON and SSE native or translated subset |
| `POST /v1/embeddings`           | Yes                                | Yes                                | No                                 | Yes                                | No                                       | No                                       |
| `POST /v1/responses`            | JSON and SSE                       | JSON and SSE                       | JSON and SSE translated subset     | JSON and SSE translated subset     | JSON and SSE native or translated subset | JSON and SSE native or translated subset |
| `POST /v1/images/generations`   | Yes                                | Yes                                | No                                 | No                                 | No                                       | No                                       |
| `POST /v1/audio/transcriptions` | Yes                                | Yes                                | No                                 | No                                 | No                                       | No                                       |
| `POST /v1/audio/speech`         | Yes                                | Yes                                | No                                 | No                                 | No                                       | No                                       |

<!-- docs-contract:public-matrix:end -->

Provider capability defaults and additional supported capabilities:

<!-- docs-contract:capability-matrix:start -->

| Provider type       | Default capabilities when omitted          | Additional supported capabilities                |
| ------------------- | ------------------------------------------ | ------------------------------------------------ |
| `openai`            | `chat`, `responses`, `embeddings`          | `images`, `audio_transcriptions`, `audio_speech` |
| `openai-compatible` | `chat`, `responses`, `embeddings`          | `images`, `audio_transcriptions`, `audio_speech` |
| `anthropic`         | `chat`, `responses`                        | None                                             |
| `gemini`            | `chat`, `responses`                        | `embeddings`                                     |
| `opencode-zen`      | `chat`, `responses`, or both (by protocol) | None                                             |
| `opencode-go`       | `chat`, `responses`, or both (by protocol) | None                                             |

<!-- docs-contract:capability-matrix:end -->
