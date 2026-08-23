---
sidebar_position: 8
---

# Deployment

This page covers practical ways to run `aiproxy` in local, containerized, and service-managed environments.

## Deployment Defaults

The project ships as:

- a single Go binary named `aiproxy`
- a container image built from the repo `Dockerfile`

Foreground `aiproxy serve` is supported across the advertised release targets.
Daemon lifecycle commands (`serve -d`, `status`, `stop`, and `restart`) are
Linux-only; on other platforms they return `daemon lifecycle is unsupported on
this platform`.

The container image:

- exposes port `8080`
- runs as non-root
- starts with `aiproxy serve --config /etc/aiproxy/config.hcl`

## Local Binary

Build the binary:

```sh
make build
```

Run it:

```sh
./dist/aiproxy serve --config /etc/aiproxy/config.hcl
```

Validate config without starting the server:

```sh
./dist/aiproxy validate --config /etc/aiproxy/config.hcl
```

## Docker

Build the image:

```sh
make docker-build
```

Run it with a mounted config file:

```sh
docker run --rm \
  -p 8080:8080 \
  -v ./config.hcl:/etc/aiproxy/config.hcl:ro \
  --env-file .env \
  aiproxy:latest
```

If you use `api_key_ref`, also mount the key file and point the provider config at that path.

## Docker Compose

Example `compose.yaml`:

```yaml
services:
  aiproxy:
    image: aiproxy:latest
    ports:
      - '8080:8080'
    env_file:
      - .env
    volumes:
      - ./config.hcl:/etc/aiproxy/config.hcl:ro
      - ./keys.json:/etc/aiproxy/keys.json:ro
    restart: unless-stopped
```

With this setup, a provider can reference the mounted key file with:

```hcl
api_key_ref {
  path = "/etc/aiproxy/keys.json"
  key  = "openai"
}
```

## systemd

Example unit file:

```ini
[Unit]
Description=aiproxy
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=aiproxy
Group=aiproxy
WorkingDirectory=/etc/aiproxy
EnvironmentFile=/etc/aiproxy/aiproxy.env
ExecStart=/usr/local/bin/aiproxy serve --config /etc/aiproxy/config.hcl
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

Recommended layout:

- binary at `/usr/local/bin/aiproxy`
- config at `/etc/aiproxy/config.hcl`
- env file at `/etc/aiproxy/aiproxy.env`
- optional key file at `/etc/aiproxy/keys.json`

## Reloading Config

`aiproxy` supports runtime reload on `SIGHUP` for auth, providers, models, aliases, root and provider upstream header timeouts, access-log enablement, metrics config, provider-health config, and metrics-backed inventory state.

Reload with:

```sh
kill -HUP <pid>
```

With systemd:

```sh
systemctl kill -s HUP aiproxy
```

For a container:

```sh
docker kill --signal HUP <container>
```

Listener address, listener timeout, logging level, and enabling the dashboard after startup require a full restart.

## Reverse Proxying

`aiproxy` is commonly run behind an external load balancer or reverse proxy.

Typical responsibilities of the outer layer:

- TLS termination
- public DNS and certificates
- network-level access control
- request logging outside the application process

Keep the proxy-visible auth boundary enabled unless the deployment is fully trusted end to end.

## Production Recommendations

- use `bearer_static` auth unless another trusted boundary makes it unnecessary
- keep secrets out of the HCL file when possible
- mount config and key files read-only
- declare `metrics { token = env("AIPROXY_METRICS_TOKEN") }` and scrape
  `GET /metrics` with the configured bearer token (the token is independent of
  API auth client tokens)
- use `aiproxy dashboard` only from the local host; remote dashboard access is
  unsupported until an explicit transport design is added
- explicitly `enabled = false` any provider you want defined but inactive;
  enabled providers with missing credentials fail validation
- use aliases for stable client-facing models and controlled failover
- use `provider_health` with Redis when you need transient health sharing across instances
- validate config before rollout with `aiproxy validate --config ...`

## Rollout Checklist

1. Validate the config before deploy.
2. Confirm required environment variables and key files are present.
3. Start the service and verify `GET /v1/models`.
4. Test one direct model and one alias-backed model.
5. Confirm `GET /metrics` returns `200` with the configured bearer token (and
   `401` without it).
6. If using reloads, test a `SIGHUP` config reload in a non-production environment first.
