ALTER TABLE db_providers ADD COLUMN IF NOT EXISTS encrypted_reasoning JSONB;
ALTER TABLE db_provider_models ADD COLUMN IF NOT EXISTS encrypted_reasoning JSONB;
