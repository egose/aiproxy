# AGENTS.md

Operational guide for AI agents (and humans) working in this repo.

## Build & run

| Command                                     | Effect                                                    |
| ------------------------------------------- | --------------------------------------------------------- |
| `make build`                                | Build `dist/aiproxy` for the host platform (CGO disabled) |
| `make build-all`                            | Cross-compile for every `OS_ARCH_PAIRS`                   |
| `make run CONFIG=path/to/config.hcl`        | `go run` the server against a config                      |
| `make validate CONFIG=path/to/config.hcl`   | Load + validate config without serving                    |
| `make docker-build`                         | Multi-stage container build as `aiproxy:$(VERSION)`       |
| `make docker-run CONFIG=path/to/config.hcl` | Run the container image with a mounted config             |

The CLI defaults to `$XDG_CONFIG_HOME/aiproxy/config.hcl`, falling back to
`~/.config/aiproxy/config.hcl` when `XDG_CONFIG_HOME` is unset. Pass
`--config` to use a different file.

The HCL config uses `env("VAR")` for secret/placeholder substitution; values
are textually inlined **before** HCL parsing. Run `set -a; . ./.env; set +a`
before invoking the binary locally so env vars resolve.

The server supports `SIGHUP`-triggered live config reload for auth, provider,
model, alias, and metrics-backed inventory state. Listener address or timeout
changes still require restart.

## Lint / typecheck / test

| Command          | Effect                                                  |
| ---------------- | ------------------------------------------------------- |
| `make vet`       | `go vet ./...`                                          |
| `make test`      | `go test ./...` (unit tests only)                       |
| `make test-race` | `go test -race ./...`                                   |
| `make cover`     | Unit tests with coverage profile at `dist/coverage.out` |

`make vet test` is the default pre-commit sanity check; run it after any
non-trivial change. There is no separate typecheck target — Go's compiler
is the typecheck, and `make build` exercises it.

(Integration tests are intentionally skipped for now. Add them back once the
repo has sandbox services for stable end-to-end provider coverage.)

## Conventions

- **No comments** in source files unless the surrounding code dictates
  otherwise — the design doc at `docs/design.md` holds the rationale.
- Module path: `github.com/egose/aiproxy`.
- All HCL blocks use two-label syntax: `provider "openai" "openai" {}`.
- Provider/alias/model names are lowercase, no spaces, no `/`.
- Public model strings: `<provider-name>/<model-name>` or `alias/<alias-name>`.
- Exactly one of `api_key` or `api_key_ref` per provider; `api_key_ref.path`
  defaults to `$XDG_CONFIG_HOME/aiproxy/keys.json`, falling back to
  `~/.config/aiproxy/keys.json`.
- Direct (`<provider>/<model>`) requests never fail over to a different
  target. Alias requests retry the next target on transport / 5xx only;
  client 4xx errors are returned verbatim.
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
  Transient transport failures and upstream `5xx` responses temporarily mark a
  provider unhealthy. An optional `provider_health` Redis config can share that
  transient state across instances.
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
