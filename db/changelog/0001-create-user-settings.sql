--liquibase formatted sql

--changeset config-example:0001-create-user-settings
--comment: One row per user. The whole settings object lives in `settings`; `schema_version` is a generated column for cheap filtering/indexing.
CREATE TABLE user_settings (
    user_id        text        PRIMARY KEY,
    settings       jsonb       NOT NULL,
    schema_version integer     GENERATED ALWAYS AS ((settings ->> 'schemaVersion')::integer) STORED,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),

    -- Coarse structural guard rails. Full validation (enums, nesting, required
    -- keys) is done by the application against schemas/user-settings.v<N>.schema.json.
    -- See README "Validation in the database" for the pg_jsonschema alternative.
    CONSTRAINT user_settings_shape CHECK (
        jsonb_typeof(settings)                   = 'object'
        AND jsonb_typeof(settings -> 'schemaVersion') = 'number'
        AND jsonb_typeof(settings -> 'featureFlags')  = 'array'
        AND jsonb_typeof(settings -> 'map')           = 'object'
        AND jsonb_typeof(settings -> 'filterPresets') = 'array'
    ),
    -- Only the current version may be written. Each migration moves this window.
    CONSTRAINT user_settings_version CHECK ((settings ->> 'schemaVersion')::integer = 1)
);

CREATE INDEX user_settings_schema_version_idx ON user_settings (schema_version);

-- jsonb_path_ops supports @> containment, e.g. "which users have flag X on":
--   SELECT user_id FROM user_settings WHERE settings @> '{"featureFlags":["betaExport"]}';
CREATE INDEX user_settings_settings_gin_idx ON user_settings USING gin (settings jsonb_path_ops);

COMMENT ON COLUMN user_settings.settings IS 'Validated by the app against schemas/user-settings.v<schemaVersion>.schema.json';

--rollback DROP TABLE user_settings;
