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
aiproxy paths
aiproxy examples
aiproxy configure
aiproxy configure provider
aiproxy serve --config /etc/aiproxy/config.hcl
aiproxy validate --config /etc/aiproxy/config.hcl
aiproxy version
```

Without `--config`, the CLI reads `$XDG_CONFIG_HOME/aiproxy/config.hcl`, falling back to
`~/.config/aiproxy/config.hcl` when `XDG_CONFIG_HOME` is unset.

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
aiproxy configure logging
aiproxy configure provider-health
```

The root `aiproxy configure` command shows a block selector. The block-specific
subcommands can also be used directly.

Supported workflows:

- create or update `listener`, `auth`, `provider`, `alias`, `logging`, and `provider_health`
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
  --secrets-path /etc/aiproxy/keys.json \
  --secrets-key localai \
  --api-key "$LOCALAI_API_KEY" \
  --model qwen3-32b=qwen/qwen3-32b \
  --model-capabilities qwen3-32b=chat,responses
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
make cover
```

The standard local sanity check is:

```sh
make vet test
```

There is no separate typecheck target. A successful Go build is the typecheck.

## Reload Behavior

`aiproxy` supports live config reload on `SIGHUP` for runtime state such as:

- auth configuration
- provider and model inventory
- alias routing state
- metrics-backed inventory state

These changes still require a restart:

- listener address changes
- listener timeout changes

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

Transient transport failures and upstream `5xx` responses can mark a provider unhealthy for routing and readiness decisions.

This health state is shared across requests within the same process.

## Shared Provider Health

Without extra config, provider health is in-process only.

You can optionally configure Redis-backed shared health state with `provider_health` so multiple instances can observe the same transient provider status.

Without Redis-backed sharing, each instance tracks transient health independently.

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

## Production Checklist

- enable `bearer_static` auth unless the deployment is fully trusted
- keep provider secrets out of the HCL file when possible
- mount config and key files read-only
- scrape `GET /metrics`
- use aliases for controlled failover instead of relying on direct model requests
