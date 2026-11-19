CREATE TABLE serving_revisions (
  singleton BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (singleton),
  catalog BIGINT NOT NULL DEFAULT 1 CHECK (catalog > 0),
  keys BIGINT NOT NULL DEFAULT 1 CHECK (keys > 0)
);
INSERT INTO serving_revisions (singleton) VALUES (TRUE);

CREATE FUNCTION advance_serving_revision() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_ARGV[0] = 'catalog' THEN
    UPDATE serving_revisions SET catalog = catalog + 1 WHERE singleton;
  ELSE
    UPDATE serving_revisions SET keys = keys + 1 WHERE singleton;
  END IF;
  RETURN NULL;
END;
$$;

CREATE TRIGGER serving_provider_revision AFTER INSERT OR UPDATE OR DELETE OR TRUNCATE ON db_providers
FOR EACH STATEMENT EXECUTE FUNCTION advance_serving_revision('catalog');
CREATE TRIGGER serving_model_revision AFTER INSERT OR UPDATE OR DELETE OR TRUNCATE ON db_provider_models
FOR EACH STATEMENT EXECUTE FUNCTION advance_serving_revision('catalog');
CREATE TRIGGER serving_alias_revision AFTER INSERT OR UPDATE OR DELETE OR TRUNCATE ON db_aliases
FOR EACH STATEMENT EXECUTE FUNCTION advance_serving_revision('catalog');
CREATE TRIGGER serving_target_revision AFTER INSERT OR UPDATE OR DELETE OR TRUNCATE ON db_alias_targets
FOR EACH STATEMENT EXECUTE FUNCTION advance_serving_revision('catalog');
CREATE TRIGGER serving_key_revision AFTER INSERT OR UPDATE OR DELETE OR TRUNCATE ON inbound_keys
FOR EACH STATEMENT EXECUTE FUNCTION advance_serving_revision('keys');
CREATE TRIGGER serving_key_user_revision AFTER INSERT OR UPDATE OR DELETE OR TRUNCATE ON key_users
FOR EACH STATEMENT EXECUTE FUNCTION advance_serving_revision('keys');
CREATE TRIGGER serving_key_team_revision AFTER INSERT OR UPDATE OR DELETE OR TRUNCATE ON key_teams
FOR EACH STATEMENT EXECUTE FUNCTION advance_serving_revision('keys');
CREATE TRIGGER serving_quota_revision AFTER INSERT OR UPDATE OR DELETE OR TRUNCATE ON scope_quotas
FOR EACH STATEMENT EXECUTE FUNCTION advance_serving_revision('keys');
CREATE TRIGGER serving_workspace_catalog_revision AFTER DELETE OR TRUNCATE ON workspaces
FOR EACH STATEMENT EXECUTE FUNCTION advance_serving_revision('catalog');
CREATE TRIGGER serving_workspace_key_revision AFTER DELETE OR TRUNCATE ON workspaces
FOR EACH STATEMENT EXECUTE FUNCTION advance_serving_revision('keys');
