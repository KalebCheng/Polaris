-- 021 回滚：把 config / secrets / field_mapping 还原为独立列
ALTER TABLE syncers ADD COLUMN host          TEXT    NOT NULL DEFAULT '';
ALTER TABLE syncers ADD COLUMN port          INTEGER NOT NULL DEFAULT 0;
ALTER TABLE syncers ADD COLUMN "user"        TEXT    NOT NULL DEFAULT '';
ALTER TABLE syncers ADD COLUMN password      TEXT    NOT NULL DEFAULT '';
ALTER TABLE syncers ADD COLUMN database      TEXT    NOT NULL DEFAULT '';
ALTER TABLE syncers ADD COLUMN "table"       TEXT    NOT NULL DEFAULT '';
ALTER TABLE syncers ADD COLUMN database_type TEXT    NOT NULL DEFAULT '';
ALTER TABLE syncers ADD COLUMN ssl_mode      TEXT    NOT NULL DEFAULT '';
ALTER TABLE syncers ADD COLUMN app_id        TEXT    NOT NULL DEFAULT '';
ALTER TABLE syncers ADD COLUMN app_secret    TEXT    NOT NULL DEFAULT '';
ALTER TABLE syncers ADD COLUMN agent_id      TEXT    NOT NULL DEFAULT '';
ALTER TABLE syncers ADD COLUMN corp_id       TEXT    NOT NULL DEFAULT '';
ALTER TABLE syncers ADD COLUMN user_mapping  JSONB   NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE syncers ADD COLUMN group_mapping JSONB   NOT NULL DEFAULT '{}'::jsonb;

UPDATE syncers SET
    host          = COALESCE(config->>'host', ''),
    port          = COALESCE((config->>'port')::int, 0),
    "user"        = COALESCE(config->>'user', ''),
    database      = COALESCE(config->>'database', ''),
    "table"       = COALESCE(config->>'table', ''),
    database_type = COALESCE(config->>'database_type', ''),
    ssl_mode      = COALESCE(config->>'ssl_mode', ''),
    app_id        = COALESCE(config->>'app_id', ''),
    corp_id       = COALESCE(config->>'corp_id', ''),
    password      = COALESCE(secrets->>'password', ''),
    app_secret    = COALESCE(secrets->>'app_secret', ''),
    user_mapping  = COALESCE(field_mapping->'account', '{}'::jsonb);

ALTER TABLE syncers DROP COLUMN config, DROP COLUMN secrets, DROP COLUMN field_mapping;
