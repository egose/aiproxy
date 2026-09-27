CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS schema_migrations (
  name TEXT PRIMARY KEY,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS users (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  email TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  is_admin BOOLEAN NOT NULL DEFAULT FALSE,
  disabled BOOLEAN NOT NULL DEFAULT FALSE,
  copilot_flow_started_at TIMESTAMPTZ,
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

CREATE TABLE IF NOT EXISTS workspaces (
  id UUID PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  display_name TEXT NOT NULL DEFAULT '',
  is_system BOOLEAN NOT NULL DEFAULT FALSE,
  kind TEXT NOT NULL DEFAULT 'organization' CONSTRAINT workspaces_kind_check CHECK (kind IN ('personal', 'organization')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO workspaces (id, name, display_name, is_system, kind)
VALUES ('00000000-0000-0000-0000-000000000000', 'system', 'System', TRUE, 'organization')
ON CONFLICT (id) DO NOTHING;

CREATE TABLE IF NOT EXISTS workspace_members (
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  role TEXT NOT NULL DEFAULT 'member',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, workspace_id)
);
CREATE INDEX IF NOT EXISTS idx_workspace_members_workspace ON workspace_members(workspace_id);
CREATE INDEX IF NOT EXISTS idx_workspace_members_user ON workspace_members(user_id);

CREATE TABLE IF NOT EXISTS workspace_teams (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (workspace_id, name)
);
CREATE INDEX IF NOT EXISTS idx_workspace_teams_workspace ON workspace_teams(workspace_id);

CREATE TABLE IF NOT EXISTS team_members (
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  team_id UUID NOT NULL REFERENCES workspace_teams(id) ON DELETE CASCADE,
  role TEXT NOT NULL DEFAULT 'member',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, team_id)
);
CREATE INDEX IF NOT EXISTS idx_team_members_team ON team_members(team_id);

CREATE TABLE IF NOT EXISTS invites (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  email TEXT NOT NULL UNIQUE,
  is_admin BOOLEAN NOT NULL DEFAULT FALSE,
  token_hash TEXT NOT NULL UNIQUE,
  expires_at TIMESTAMPTZ NOT NULL,
  accepted_at TIMESTAMPTZ,
  created_by UUID REFERENCES users(id) ON DELETE SET NULL,
  workspace_id UUID REFERENCES workspaces(id) ON DELETE CASCADE,
  workspace_role TEXT NOT NULL DEFAULT 'member',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_invites_token ON invites(token_hash);

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
  copilot_credential_encrypted BYTEA,
  extends TEXT NOT NULL DEFAULT '',
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  workspace_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000' REFERENCES workspaces(id),
  healthcheck JSONB,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT db_providers_copilot_credential_source CHECK (
    copilot_credential_encrypted IS NULL OR (
      octet_length(copilot_credential_encrypted) > 0
      AND type = 'github-copilot'
      AND COALESCE(octet_length(api_key_encrypted), 0) = 0
      AND api_key_ref_path = '' AND api_key_ref_key = ''
      AND copilot_credential_path = '' AND copilot_credential_name = ''
    )
  )
);
CREATE INDEX IF NOT EXISTS idx_db_providers_workspace ON db_providers(workspace_id);

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
  workspace_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000' REFERENCES workspaces(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_db_aliases_workspace ON db_aliases(workspace_id);

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
  workspace_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000' REFERENCES workspaces(id),
  description TEXT NOT NULL DEFAULT '',
  expires_at TIMESTAMPTZ,
  owner_user_id UUID REFERENCES users(id) ON DELETE CASCADE,
  owner_team_id UUID REFERENCES workspace_teams(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT inbound_keys_single_owner CHECK (NOT (owner_user_id IS NOT NULL AND owner_team_id IS NOT NULL))
);
CREATE INDEX IF NOT EXISTS idx_inbound_keys_workspace ON inbound_keys(workspace_id);
CREATE INDEX IF NOT EXISTS idx_inbound_keys_owner_user ON inbound_keys(owner_user_id);
CREATE INDEX IF NOT EXISTS idx_inbound_keys_owner_team ON inbound_keys(owner_team_id);

CREATE TABLE IF NOT EXISTS key_users (
  key_id UUID NOT NULL REFERENCES inbound_keys(id) ON DELETE CASCADE,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (key_id, user_id)
);
CREATE INDEX IF NOT EXISTS idx_key_users_user ON key_users(user_id);

CREATE TABLE IF NOT EXISTS key_teams (
  key_id UUID NOT NULL REFERENCES inbound_keys(id) ON DELETE CASCADE,
  team_id UUID NOT NULL REFERENCES workspace_teams(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (key_id, team_id)
);
CREATE INDEX IF NOT EXISTS idx_key_teams_team ON key_teams(team_id);

CREATE TABLE IF NOT EXISTS scope_quotas (
  workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  scope_type TEXT NOT NULL,
  scope_id UUID NOT NULL,
  model TEXT NOT NULL DEFAULT '',
  budget_micros BIGINT NOT NULL DEFAULT 0,
  tpm_ceiling BIGINT NOT NULL DEFAULT 0,
  tpm_effective BIGINT NOT NULL DEFAULT 0,
  spent_offset_micros BIGINT NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (workspace_id, scope_type, scope_id, model),
  CONSTRAINT scope_quotas_type_check CHECK (scope_type IN ('user', 'team'))
);
CREATE INDEX IF NOT EXISTS idx_scope_quotas_scope ON scope_quotas(scope_type, scope_id);

CREATE TABLE IF NOT EXISTS spend_key_identities (
  id UUID PRIMARY KEY,
  workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  UNIQUE (id, workspace_id)
);

CREATE TABLE IF NOT EXISTS spend_ledger (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  key_id UUID NOT NULL,
  user_id UUID REFERENCES users(id) ON DELETE SET NULL,
  team_id UUID REFERENCES workspace_teams(id) ON DELETE SET NULL,
  model TEXT NOT NULL DEFAULT '',
  tokens BIGINT NOT NULL DEFAULT 0,
  cost_micros BIGINT NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT spend_ledger_key_identity_fkey FOREIGN KEY (key_id, workspace_id) REFERENCES spend_key_identities(id, workspace_id)
);
CREATE INDEX IF NOT EXISTS idx_spend_ledger_scope ON spend_ledger(workspace_id, user_id, team_id, model, created_at);
CREATE INDEX IF NOT EXISTS idx_spend_ledger_key ON spend_ledger(key_id, created_at);

CREATE OR REPLACE FUNCTION retain_spend_key_identity() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  INSERT INTO spend_key_identities (id, workspace_id) VALUES (NEW.id, NEW.workspace_id) ON CONFLICT DO NOTHING;
  RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS inbound_key_spend_identity ON inbound_keys;
CREATE TRIGGER inbound_key_spend_identity
AFTER INSERT ON inbound_keys
FOR EACH ROW EXECUTE FUNCTION retain_spend_key_identity();

CREATE TABLE IF NOT EXISTS copilot_device_flows (
  id UUID PRIMARY KEY,
  actor_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  purpose TEXT NOT NULL CHECK (purpose IN ('create','edit')),
  provider_id UUID REFERENCES db_providers(id) ON DELETE CASCADE,
  provider_name TEXT NOT NULL DEFAULT '',
  client_id TEXT NOT NULL CHECK (length(client_id) BETWEEN 1 AND 256),
  state TEXT NOT NULL CHECK (state IN ('starting','pending','ready','consumed','denied','expired','failed','cancelled')),
  challenge_encrypted BYTEA,
  credential_encrypted BYTEA,
  created_at TIMESTAMPTZ NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  poll_at TIMESTAMPTZ NOT NULL,
  interval_ns BIGINT NOT NULL DEFAULT 5000000000 CHECK (interval_ns > 0),
  lease_id UUID,
  lease_until TIMESTAMPTZ,
  revision BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
  retain_until TIMESTAMPTZ,
  error_code TEXT NOT NULL DEFAULT '' CHECK (error_code IN ('','access_denied','expired','authorization_failed','lease_abandoned','cancelled','authority_lost')),
  consumed_provider_id UUID,
  consumed_updated_at TIMESTAMPTZ,
  CHECK ((purpose = 'create' AND provider_id IS NULL) OR (purpose = 'edit' AND provider_id IS NOT NULL)),
  CHECK ((lease_id IS NULL) = (lease_until IS NULL)),
  CHECK (challenge_encrypted IS NULL OR octet_length(challenge_encrypted) BETWEEN 29 AND 32768),
  CHECK (credential_encrypted IS NULL OR octet_length(credential_encrypted) BETWEEN 29 AND 32768),
  CHECK (
    (state = 'starting' AND challenge_encrypted IS NULL AND credential_encrypted IS NULL AND lease_id IS NOT NULL AND retain_until IS NULL) OR
    (state = 'pending' AND challenge_encrypted IS NOT NULL AND credential_encrypted IS NULL AND retain_until IS NULL) OR
    (state = 'ready' AND challenge_encrypted IS NULL AND credential_encrypted IS NOT NULL AND lease_id IS NULL AND retain_until IS NULL) OR
    (state IN ('consumed','denied','expired','failed','cancelled') AND challenge_encrypted IS NULL AND credential_encrypted IS NULL AND lease_id IS NULL AND retain_until IS NOT NULL)
  )
);
CREATE UNIQUE INDEX IF NOT EXISTS copilot_device_flows_live_actor_workspace ON copilot_device_flows(actor_id, workspace_id) WHERE state IN ('starting','pending','ready');
CREATE INDEX IF NOT EXISTS copilot_device_flows_actor_start ON copilot_device_flows(actor_id, created_at);
CREATE INDEX IF NOT EXISTS copilot_device_flows_workspace ON copilot_device_flows(workspace_id);
CREATE INDEX IF NOT EXISTS copilot_device_flows_provider ON copilot_device_flows(provider_id);
CREATE INDEX IF NOT EXISTS copilot_device_flows_expiry ON copilot_device_flows(expires_at) WHERE state IN ('starting','pending','ready');
CREATE INDEX IF NOT EXISTS copilot_device_flows_retention ON copilot_device_flows(retain_until);
