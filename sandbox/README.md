# Sandbox (local Postgres + Keycloak)

Local-only development infrastructure. Never use these credentials outside this sandbox.

## Start

```sh
make sandbox-up            # postgres on 127.0.0.1:55432
make sandbox-provision     # no-op unless the keycloak profile is up
```

With Keycloak (for OIDC work):

```sh
docker compose -f sandbox/docker-compose.yml --profile keycloak up -d --wait postgres keycloak
docker compose -f sandbox/docker-compose.yml --profile keycloak up --build keycloak-provision
```

This creates realm `aiproxy`, confidential client `aiproxy-web` (`testsecret`),
and disposable users `admin@example.com` / `user1@example.com` (password equals
the email). Registered callback: `http://localhost:8080/_internal/admin/oidc/callback`.

## App wiring

```sh
set -a; . sandbox/.env.example; set +a
go run ./cmd/aiproxy migrate up --config examples/multi-tenancy.hcl
go run ./cmd/aiproxy serve --config examples/multi-tenancy.hcl
```

Then sign in with the seeded admin and open Admin → Single sign-on to point the
instance at the sandbox issuer `http://localhost:18081/realms/aiproxy`.

## Known limitation

The sandbox Keycloak serves plain HTTP but marks session cookies `Secure`, so
real browser logins against it fail at the Keycloak login form
("Restart login cookie not found"). This affects only the local sandbox:
point the admin OIDC settings at an HTTPS issuer for browser SSO, or exercise
the code flow headlessly (see `TestAdminOIDCFullCodeFlow`, gated by
`AIPROXY_TEST_DATABASE_URL`).

```sh
make sandbox-destroy CONFIRM_DESTROY=1   # wipe sandbox volumes
```
