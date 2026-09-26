ALTER TABLE team_members ADD COLUMN IF NOT EXISTS role TEXT NOT NULL DEFAULT 'member';

ALTER TABLE inbound_keys ADD COLUMN IF NOT EXISTS owner_user_id UUID REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE inbound_keys ADD COLUMN IF NOT EXISTS owner_team_id UUID REFERENCES organization_teams(id) ON DELETE CASCADE;
ALTER TABLE inbound_keys ADD CONSTRAINT inbound_keys_single_owner
  CHECK (NOT (owner_user_id IS NOT NULL AND owner_team_id IS NOT NULL));
CREATE INDEX IF NOT EXISTS idx_inbound_keys_owner_user ON inbound_keys(owner_user_id);
CREATE INDEX IF NOT EXISTS idx_inbound_keys_owner_team ON inbound_keys(owner_team_id);
