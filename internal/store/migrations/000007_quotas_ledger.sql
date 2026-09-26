CREATE TABLE IF NOT EXISTS scope_quotas (
  org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  scope_type TEXT NOT NULL,
  scope_id UUID NOT NULL,
  model TEXT NOT NULL DEFAULT '',
  budget_micros BIGINT NOT NULL DEFAULT 0,
  tpm_ceiling BIGINT NOT NULL DEFAULT 0,
  tpm_effective BIGINT NOT NULL DEFAULT 0,
  spent_offset_micros BIGINT NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (org_id, scope_type, scope_id, model),
  CONSTRAINT scope_quotas_type_check CHECK (scope_type IN ('user', 'team'))
);
CREATE INDEX IF NOT EXISTS idx_scope_quotas_scope ON scope_quotas(scope_type, scope_id);

CREATE TABLE IF NOT EXISTS spend_ledger (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  key_id UUID NOT NULL REFERENCES inbound_keys(id) ON DELETE CASCADE,
  user_id UUID REFERENCES users(id) ON DELETE SET NULL,
  team_id UUID REFERENCES organization_teams(id) ON DELETE SET NULL,
  model TEXT NOT NULL DEFAULT '',
  tokens BIGINT NOT NULL DEFAULT 0,
  cost_micros BIGINT NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_spend_ledger_scope ON spend_ledger(org_id, user_id, team_id, model, created_at);
CREATE INDEX IF NOT EXISTS idx_spend_ledger_key ON spend_ledger(key_id, created_at);
