CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS users (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  email TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  is_admin BOOLEAN NOT NULL DEFAULT FALSE,
  disabled BOOLEAN NOT NULL DEFAULT FALSE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS refresh_tokens (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token_hash TEXT NOT NULL UNIQUE,
  expires_at TIMESTAMPTZ NOT NULL,
  revoked_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user ON refresh_tokens(user_id);

CREATE TABLE IF NOT EXISTS db_providers (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL UNIQUE,
  type TEXT NOT NULL,
  display_name TEXT NOT NULL DEFAULT '',
  base_url TEXT NOT NULL DEFAULT '',
  upstream_header_timeout_ms BIGINT,
  user_agent TEXT NOT NULL DEFAULT '',
  forward_user_agent BOOLEAN NOT NULL DEFAULT FALSE,
  forward_headers JSONB NOT NULL DEFAULT '[]',
  api_key_encrypted BYTEA,
  api_key_ref_path TEXT NOT NULL DEFAULT '',
  api_key_ref_key TEXT NOT NULL DEFAULT '',
  copilot_credential_path TEXT NOT NULL DEFAULT '',
  copilot_credential_name TEXT NOT NULL DEFAULT '',
  extends TEXT NOT NULL DEFAULT '',
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  healthcheck JSONB,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS db_provider_models (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  provider_id UUID NOT NULL REFERENCES db_providers(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  display_name TEXT NOT NULL DEFAULT '',
  upstream_name TEXT NOT NULL DEFAULT '',
  protocol TEXT NOT NULL DEFAULT '',
  capabilities JSONB NOT NULL DEFAULT '[]',
  pricing JSONB,
  UNIQUE (provider_id, name)
);
CREATE INDEX IF NOT EXISTS idx_db_provider_models_provider ON db_provider_models(provider_id);

CREATE TABLE IF NOT EXISTS db_aliases (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL UNIQUE,
  algorithm TEXT NOT NULL,
  retry_status_codes JSONB NOT NULL DEFAULT '[]',
  session_affinity JSONB,
  encrypted_reasoning JSONB,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS db_alias_targets (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  alias_id UUID NOT NULL REFERENCES db_aliases(id) ON DELETE CASCADE,
  provider TEXT NOT NULL,
  model TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_db_alias_targets_alias ON db_alias_targets(alias_id);

CREATE TABLE IF NOT EXISTS inbound_keys (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL UNIQUE,
  token_hash TEXT NOT NULL UNIQUE,
  token_prefix TEXT NOT NULL,
  tenant TEXT NOT NULL DEFAULT '',
  allowed_models JSONB NOT NULL DEFAULT '[]',
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  expires_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
