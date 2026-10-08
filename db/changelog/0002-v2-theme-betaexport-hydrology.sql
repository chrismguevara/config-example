--liquibase formatted sql

--changeset config-example:0002-v2-theme-betaexport-hydrology
--comment: v1 -> v2. Add scalar field `theme` (default "system"); add flag enum value `betaExport` (default OFF: schema-only, no data change); add layer enum value `hydrology` (default ON: backfilled into every row).
--preconditions onFail:HALT onError:HALT
--precondition-sql-check expectedResult:0 SELECT count(*) FROM user_settings WHERE schema_version <> 1

ALTER TABLE user_settings DROP CONSTRAINT user_settings_version;

UPDATE user_settings
SET settings = jsonb_set(
        -- `||` on the top level adds/overwrites top-level keys.
        settings || jsonb_build_object('schemaVersion', 2, 'theme', 'system'),
        -- jsonb_set reaches into the nested path; append only if absent (uniqueItems).
        '{map,layers}',
        CASE
            WHEN (settings #> '{map,layers}') @> '["hydrology"]'::jsonb THEN settings #> '{map,layers}'
            ELSE (settings #> '{map,layers}') || '["hydrology"]'::jsonb
        END
    );

ALTER TABLE user_settings DROP CONSTRAINT user_settings_shape;
ALTER TABLE user_settings ADD CONSTRAINT user_settings_shape CHECK (
    jsonb_typeof(settings)                   = 'object'
    AND jsonb_typeof(settings -> 'schemaVersion') = 'number'
    AND jsonb_typeof(settings -> 'theme')         = 'string'
    AND jsonb_typeof(settings -> 'featureFlags')  = 'array'
    AND jsonb_typeof(settings -> 'map')           = 'object'
    AND jsonb_typeof(settings -> 'filterPresets') = 'array'
);
ALTER TABLE user_settings ADD CONSTRAINT user_settings_version CHECK ((settings ->> 'schemaVersion')::integer = 2);

-- Reversing this changeset is lossless: theme did not exist in v1 and hydrology was not a v1 value.
--rollback ALTER TABLE user_settings DROP CONSTRAINT user_settings_version;
--rollback ALTER TABLE user_settings DROP CONSTRAINT user_settings_shape;
--rollback UPDATE user_settings SET settings = jsonb_set((settings - 'theme') || '{"schemaVersion": 1}'::jsonb, '{map,layers}', (settings #> '{map,layers}') - 'hydrology');
--rollback ALTER TABLE user_settings ADD CONSTRAINT user_settings_shape CHECK (jsonb_typeof(settings) = 'object' AND jsonb_typeof(settings -> 'schemaVersion') = 'number' AND jsonb_typeof(settings -> 'featureFlags') = 'array' AND jsonb_typeof(settings -> 'map') = 'object' AND jsonb_typeof(settings -> 'filterPresets') = 'array');
--rollback ALTER TABLE user_settings ADD CONSTRAINT user_settings_version CHECK ((settings ->> 'schemaVersion')::integer = 1);
