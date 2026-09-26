CREATE TABLE IF NOT EXISTS oidc_configs (
  id TEXT PRIMARY KEY DEFAULT 'default',
  enabled BOOLEAN NOT NULL DEFAULT FALSE,
  issuer_url TEXT NOT NULL DEFAULT '',
  client_id TEXT NOT NULL DEFAULT '',
  client_secret_encrypted BYTEA,
  scopes TEXT NOT NULL DEFAULT 'openid email profile',
  username_claim TEXT NOT NULL DEFAULT 'email',
  admin_claim TEXT NOT NULL DEFAULT '',
  admin_value TEXT NOT NULL DEFAULT '',
  display_name TEXT NOT NULL DEFAULT 'Single Sign-On',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
