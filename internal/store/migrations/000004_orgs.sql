CREATE TABLE IF NOT EXISTS organizations (
  id UUID PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  display_name TEXT NOT NULL DEFAULT '',
  is_system BOOLEAN NOT NULL DEFAULT FALSE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO organizations (id, name, display_name, is_system)
VALUES ('00000000-0000-0000-0000-000000000000', 'system', 'System', TRUE)
ON CONFLICT (id) DO NOTHING;

CREATE TABLE IF NOT EXISTS organization_members (
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  role TEXT NOT NULL DEFAULT 'member',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, org_id)
);
CREATE INDEX IF NOT EXISTS idx_organization_members_org ON organization_members(org_id);
CREATE INDEX IF NOT EXISTS idx_organization_members_user ON organization_members(user_id);

CREATE TABLE IF NOT EXISTS organization_teams (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (org_id, name)
);
CREATE INDEX IF NOT EXISTS idx_organization_teams_org ON organization_teams(org_id);

CREATE TABLE IF NOT EXISTS team_members (
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  team_id UUID NOT NULL REFERENCES organization_teams(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, team_id)
);
CREATE INDEX IF NOT EXISTS idx_team_members_team ON team_members(team_id);

ALTER TABLE db_providers ADD COLUMN IF NOT EXISTS org_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000' REFERENCES organizations(id);
ALTER TABLE db_aliases ADD COLUMN IF NOT EXISTS org_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000' REFERENCES organizations(id);
ALTER TABLE inbound_keys ADD COLUMN IF NOT EXISTS org_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000' REFERENCES organizations(id);
CREATE INDEX IF NOT EXISTS idx_db_providers_org ON db_providers(org_id);
CREATE INDEX IF NOT EXISTS idx_db_aliases_org ON db_aliases(org_id);
CREATE INDEX IF NOT EXISTS idx_inbound_keys_org ON inbound_keys(org_id);

ALTER TABLE invites ADD COLUMN IF NOT EXISTS org_id UUID REFERENCES organizations(id) ON DELETE CASCADE;
ALTER TABLE invites ADD COLUMN IF NOT EXISTS org_role TEXT NOT NULL DEFAULT 'member';
