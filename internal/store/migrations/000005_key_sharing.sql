ALTER TABLE inbound_keys ADD COLUMN IF NOT EXISTS description TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS key_users (
  key_id UUID NOT NULL REFERENCES inbound_keys(id) ON DELETE CASCADE,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (key_id, user_id)
);
CREATE INDEX IF NOT EXISTS idx_key_users_user ON key_users(user_id);

CREATE TABLE IF NOT EXISTS key_teams (
  key_id UUID NOT NULL REFERENCES inbound_keys(id) ON DELETE CASCADE,
  team_id UUID NOT NULL REFERENCES organization_teams(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (key_id, team_id)
);
CREATE INDEX IF NOT EXISTS idx_key_teams_team ON key_teams(team_id);
