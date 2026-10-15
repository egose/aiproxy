ALTER TABLE db_providers ADD COLUMN IF NOT EXISTS forward_authorization BOOLEAN NOT NULL DEFAULT false;
