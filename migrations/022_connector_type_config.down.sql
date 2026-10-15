-- 022 回滚
DROP INDEX IF EXISTS connectors_app_unique;

ALTER TABLE connectors ADD COLUMN provider_url TEXT     NOT NULL DEFAULT '';
ALTER TABLE connectors ADD COLUMN principal    TEXT     NOT NULL DEFAULT '';
ALTER TABLE connectors ADD COLUMN credentials  TEXT     NOT NULL DEFAULT '';
ALTER TABLE connectors ADD COLUMN filters      TEXT[]   NOT NULL DEFAULT '{}';
ALTER TABLE connectors ADD COLUMN schedule     TEXT     NOT NULL DEFAULT '';

UPDATE connectors SET
    provider_url = COALESCE(config->>'provider_url', ''),
    principal    = COALESCE(config->>'principal', ''),
    credentials  = COALESCE(secrets->>'credentials', '');

ALTER TABLE connectors DROP COLUMN type, DROP COLUMN config, DROP COLUMN secrets;
