--liquibase formatted sql

--changeset config-example:0003-v3-drop-showlabels-remove-legacyreports-rename-parcels splitStatements:false
--comment: v2 -> v3. Remove field `map.showLabels` (true becomes the `labels` layer); remove flag enum value `legacyReports` (dropped silently); rename layer enum value `parcels` -> `cadastral`. Uses a throwaway SQL function so the transform reads top-to-bottom; the Go lazy migrator (backend/internal/settings/migrate.go) mirrors it.
--preconditions onFail:HALT onError:HALT
--precondition-sql-check expectedResult:0 SELECT count(*) FROM user_settings WHERE schema_version <> 2

CREATE FUNCTION pg_temp.settings_v2_to_v3(s jsonb) RETURNS jsonb LANGUAGE sql IMMUTABLE AS $$
    WITH step AS (
        SELECT
            -- 1. remove field: `showLabels: true` folds into the `labels` layer
            CASE
                WHEN (s #> '{map,showLabels}') = 'true'::jsonb AND NOT (s #> '{map,layers}') @> '["labels"]'::jsonb
                    THEN (s #> '{map,layers}') || '["labels"]'::jsonb
                ELSE s #> '{map,layers}'
            END AS layers
    ),
    renamed AS (
        SELECT
            -- 2. rename enum value: parcels -> cadastral (dedup if both were somehow present)
            CASE
                WHEN layers @> '["parcels"]'::jsonb THEN (layers - 'parcels' - 'cadastral') || '["cadastral"]'::jsonb
                ELSE layers
            END AS layers
        FROM step
    )
    SELECT jsonb_set(
        jsonb_set(
            (s || '{"schemaVersion": 3}'::jsonb) #- '{map,showLabels}',
            -- 3. remove enum value: `jsonb - text` deletes a string element from an array
            '{featureFlags}', (s -> 'featureFlags') - 'legacyReports'
        ),
        '{map,layers}', renamed.layers
    )
    FROM renamed
$$;

ALTER TABLE user_settings DROP CONSTRAINT user_settings_version;

UPDATE user_settings SET settings = pg_temp.settings_v2_to_v3(settings);

ALTER TABLE user_settings ADD CONSTRAINT user_settings_version CHECK ((settings ->> 'schemaVersion')::integer = 3);

DROP FUNCTION pg_temp.settings_v2_to_v3(jsonb);

-- Reversing this changeset is LOSSY: users who had `legacyReports` enabled do not get it back.
--rollback ALTER TABLE user_settings DROP CONSTRAINT user_settings_version;
--rollback UPDATE user_settings SET settings = jsonb_set(jsonb_set(settings || '{"schemaVersion": 2}'::jsonb, '{map,showLabels}', to_jsonb((settings #> '{map,layers}') @> '["labels"]'::jsonb)), '{map,layers}', CASE WHEN (settings #> '{map,layers}') @> '["cadastral"]'::jsonb THEN ((settings #> '{map,layers}') - 'cadastral' - 'labels') || '["parcels"]'::jsonb ELSE (settings #> '{map,layers}') - 'labels' END);
--rollback ALTER TABLE user_settings ADD CONSTRAINT user_settings_version CHECK ((settings ->> 'schemaVersion')::integer = 2);
