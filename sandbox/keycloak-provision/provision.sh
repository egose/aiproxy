#!/bin/sh
# Idempotent local-only Keycloak provisioning for future OIDC use.
# Creates the realm, confidential client, and disposable test users.
set -eu

: "${KEYCLOAK_URL:?missing KEYCLOAK_URL}"
: "${MASTER_ADMIN:?missing MASTER_ADMIN}"
: "${MASTER_ADMIN_PASSWORD:?missing MASTER_ADMIN_PASSWORD}"
: "${AUTH_REALM_NAME:?missing AUTH_REALM_NAME}"
: "${AUTH_CLIENT_ID:?missing AUTH_CLIENT_ID}"
: "${AUTH_CLIENT_SECRET:?missing AUTH_CLIENT_SECRET}"

deadline=$(($(date +%s) + ${KEYCLOAK_READINESS_TIMEOUT_MS:-120000} / 1000))
while ! curl -sf -m 5 "$KEYCLOAK_URL/realms/master/.well-known/openid-configuration" >/dev/null; do
  if [ "$(date +%s)" -gt "$deadline" ]; then
    echo "keycloak not ready" >&2
    exit 1
  fi
  sleep 2
done

admin_token() {
  curl -sf -m 10 -X POST "$KEYCLOAK_URL/realms/master/protocol/openid-connect/token" \
    -d "username=$MASTER_ADMIN" -d "password=$MASTER_ADMIN_PASSWORD" \
    -d 'grant_type=password' -d 'client_id=admin-cli' | python3 -c 'import sys,json; print(json.load(sys.stdin)["access_token"])'
}

TOKEN=$(admin_token)
auth="Authorization: Bearer $TOKEN"

if ! curl -sf -m 10 -H "$auth" "$KEYCLOAK_URL/admin/realms/$AUTH_REALM_NAME" >/dev/null; then
  curl -sf -m 10 -X POST -H "$auth" -H 'Content-Type: application/json' \
    "$KEYCLOAK_URL/admin/realms" \
    -d "{\"realm\":\"$AUTH_REALM_NAME\",\"enabled\":true}" >/dev/null
  echo "created realm $AUTH_REALM_NAME"
fi

CLIENT_UUID=$(curl -s -m 10 -H "$auth" \
  "$KEYCLOAK_URL/admin/realms/$AUTH_REALM_NAME/clients?clientId=$(python3 -c "import urllib.parse,os; print(urllib.parse.quote(os.environ['AUTH_CLIENT_ID']))")" |
  python3 -c 'import sys,json; rows=json.load(sys.stdin); print(rows[0]["id"] if rows else "")')

CLIENT_JSON=$(python3 -c 'import json,os; print(json.dumps({
  "clientId": os.environ["AUTH_CLIENT_ID"],
  "secret": os.environ["AUTH_CLIENT_SECRET"],
  "standardFlowEnabled": True,
  "directAccessGrantsEnabled": True,
  "redirectUris": [os.environ.get("BACKEND_CALLBACK_URI", "")],
  "webOrigins": [os.environ.get("FRONTEND_WEB_ORIGIN", "")],
  "attributes": {
    "post.logout.redirect.uris": os.environ.get("POST_LOGOUT_REDIRECT_URI", ""),
    "backchannel.logout.url": os.environ.get("BACKCHANNEL_LOGOUT_URI", ""),
    "backchannel.logout.session.required": "true",
    "backchannel.logout.revoke.offline.tokens": "true",
  },
}))')

if [ -z "$CLIENT_UUID" ]; then
  curl -sf -m 10 -X POST -H "$auth" -H 'Content-Type: application/json' \
    "$KEYCLOAK_URL/admin/realms/$AUTH_REALM_NAME/clients" -d "$CLIENT_JSON" >/dev/null
  echo "created client $AUTH_CLIENT_ID"
else
  curl -sf -m 10 -X PUT -H "$auth" -H 'Content-Type: application/json' \
    "$KEYCLOAK_URL/admin/realms/$AUTH_REALM_NAME/clients/$CLIENT_UUID" -d "$CLIENT_JSON" >/dev/null
  echo "updated client $AUTH_CLIENT_ID"
fi

# Development-only disposable identities with predictable credentials.
for identity in "admin@example.com:Admin:User" "user1@example.com:User:One"; do
  email=${identity%%:*}
  rest=${identity#*:}
  first=${rest%%:*}
  last=${rest##*:}

  USER_ID=$(curl -s -m 10 -H "$auth" \
    "$KEYCLOAK_URL/admin/realms/$AUTH_REALM_NAME/users?email=$(python3 -c "import urllib.parse,sys; print(urllib.parse.quote(sys.argv[1]))" "$email")" |
    python3 -c 'import sys,json; rows=json.load(sys.stdin); print(rows[0]["id"] if rows else "")')

  if [ -z "$USER_ID" ]; then
    USER_JSON=$(python3 -c 'import json,sys; print(json.dumps({
      "username": sys.argv[1], "email": sys.argv[1], "firstName": sys.argv[2],
      "lastName": sys.argv[3], "enabled": True, "emailVerified": True,
    }))' "$email" "$first" "$last")
    curl -sf -m 10 -X POST -H "$auth" -H 'Content-Type: application/json' \
      "$KEYCLOAK_URL/admin/realms/$AUTH_REALM_NAME/users" -d "$USER_JSON" >/dev/null
    USER_ID=$(curl -s -m 10 -H "$auth" \
      "$KEYCLOAK_URL/admin/realms/$AUTH_REALM_NAME/users?email=$(python3 -c "import urllib.parse,sys; print(urllib.parse.quote(sys.argv[1]))" "$email")" |
      python3 -c 'import sys,json; rows=json.load(sys.stdin); print(rows[0]["id"] if rows else "")')
    echo "created user $email"
  fi

  curl -sf -m 10 -X PUT -H "$auth" -H 'Content-Type: application/json' \
    "$KEYCLOAK_URL/admin/realms/$AUTH_REALM_NAME/users/$USER_ID/reset-password" \
    -d "{\"type\":\"password\",\"value\":\"$email\",\"temporary\":false}" >/dev/null
done

echo "Local sandbox provisioning complete."
