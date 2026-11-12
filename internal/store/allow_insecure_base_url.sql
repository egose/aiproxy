ALTER TABLE db_providers ADD COLUMN IF NOT EXISTS allow_insecure_base_url BOOLEAN NOT NULL DEFAULT false;
