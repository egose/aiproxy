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

GitHub Copilot providers reference a saved device-flow login (no API-key
flags, no OAuth networking here, no token display):

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
- metrics configuration
- provider-health configuration
- metrics-backed inventory state

If rate-limit settings are unchanged, reload preserves existing limiter buckets.
Changing rate-limit settings creates a fresh limiter and resets bucket state.

Alias cooldown deadlines survive reload only for fingerprint-unchanged targets
(resolved `base_url`, credential, upstream model, protocol); removed or changed
targets are dropped, and failed reloads leave state untouched.

These changes still require a restart:

- listener address changes
- listener timeout changes
- logging level changes
- enabling the dashboard after startup

Use reload for routing and auth changes, not for socket-level listener changes.

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
