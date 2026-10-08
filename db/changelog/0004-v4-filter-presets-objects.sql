--liquibase formatted sql

--changeset config-example:0004-v4-filter-presets-objects splitStatements:false
--comment: v3 -> v4. Restructure `filterPresets` from an array of built-in preset ids (enum strings) to an array of user-defined preset objects. Known ids expand to their object form; unknown ids are dropped.
--preconditions onFail:HALT onError:HALT
--precondition-sql-check expectedResult:0 SELECT count(*) FROM user_settings WHERE schema_version <> 3

CREATE FUNCTION pg_temp.settings_v3_to_v4(s jsonb) RETURNS jsonb LANGUAGE sql IMMUTABLE AS $$
    SELECT (s || '{"schemaVersion": 4}'::jsonb) || jsonb_build_object(
        'filterPresets',
        coalesce(
            (
                SELECT jsonb_agg(
                    CASE t.id
                        WHEN 'open-items'     THEN '{"id":"open-items","name":"Open items","filters":{"status":["open"],"assignee":"anyone"}}'::jsonb
                        WHEN 'assigned-to-me' THEN '{"id":"assigned-to-me","name":"Assigned to me","filters":{"status":["open","pending"],"assignee":"me"}}'::jsonb
                        WHEN 'unassigned'     THEN '{"id":"unassigned","name":"Unassigned","filters":{"status":["open"],"assignee":"unassigned"}}'::jsonb
                    END
                    ORDER BY t.ord
                )
                FROM jsonb_array_elements_text(s -> 'filterPresets') WITH ORDINALITY AS t(id, ord)
                WHERE t.id IN ('open-items', 'assigned-to-me', 'unassigned')
            ),
            '[]'::jsonb
        )
    )
$$;

ALTER TABLE user_settings DROP CONSTRAINT user_settings_version;

UPDATE user_settings SET settings = pg_temp.settings_v3_to_v4(settings);

ALTER TABLE user_settings ADD CONSTRAINT user_settings_version CHECK ((settings ->> 'schemaVersion')::integer = 4);

DROP FUNCTION pg_temp.settings_v3_to_v4(jsonb);

-- Reversing this changeset is LOSSY: user-defined presets collapse back to their ids and anything that is not a known built-in id is dropped.
--rollback ALTER TABLE user_settings DROP CONSTRAINT user_settings_version;
--rollback UPDATE user_settings SET settings = (settings || '{"schemaVersion": 3}'::jsonb) || jsonb_build_object('filterPresets', coalesce((SELECT jsonb_agg(p ->> 'id' ORDER BY ord) FROM jsonb_array_elements(settings -> 'filterPresets') WITH ORDINALITY AS t(p, ord) WHERE p ->> 'id' IN ('open-items', 'assigned-to-me', 'unassigned')), '[]'::jsonb));
--rollback ALTER TABLE user_settings ADD CONSTRAINT user_settings_version CHECK ((settings ->> 'schemaVersion')::integer = 3);
