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
aiproxy serve --config /etc/aiproxy/config.hcl
aiproxy validate --config /etc/aiproxy/config.hcl
aiproxy version
```

When running locally with env-based secrets, load your environment before invoking the binary:

```sh
set -a; . ./.env; set +a
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
