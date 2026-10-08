# JSONB document vs. normalized tables for user settings

The same settings modelled relationally, and how each operation compares.

## The normalized design

```sql
CREATE TABLE user_settings (
    user_id    text PRIMARY KEY,
    theme      text NOT NULL DEFAULT 'system' REFERENCES theme(code),
    base_layer text NOT NULL DEFAULT 'streets' REFERENCES map_layer_base(code),
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- "constraint tables": one row per allowed enum value
CREATE TABLE theme          (code text PRIMARY KEY);
CREATE TABLE feature_flag   (code text PRIMARY KEY, default_on boolean NOT NULL DEFAULT false);
CREATE TABLE map_layer_base (code text PRIMARY KEY);
CREATE TABLE map_layer      (code text PRIMARY KEY, default_on boolean NOT NULL DEFAULT true);
CREATE TABLE item_status    (code text PRIMARY KEY);
CREATE TABLE assignee_filter(code text PRIMARY KEY);

-- arrays of enums become junction tables
CREATE TABLE user_feature_flag (
    user_id text REFERENCES user_settings ON DELETE CASCADE,
    flag    text REFERENCES feature_flag(code) ON DELETE CASCADE,
    PRIMARY KEY (user_id, flag)
);
CREATE TABLE user_map_layer (
    user_id  text REFERENCES user_settings ON DELETE CASCADE,
    layer    text REFERENCES map_layer(code) ON DELETE CASCADE,
    position int  NOT NULL,                       -- arrays are ordered; tables are not
    PRIMARY KEY (user_id, layer)
);

-- arrays of objects become child tables (and their enum arrays, grandchild tables)
CREATE TABLE user_filter_preset (
    user_id   text REFERENCES user_settings ON DELETE CASCADE,
    preset_id text,
    name      text NOT NULL CHECK (length(name) BETWEEN 1 AND 80),
    assignee  text NOT NULL REFERENCES assignee_filter(code),
    position  int  NOT NULL,
    PRIMARY KEY (user_id, preset_id)
);
CREATE TABLE user_filter_preset_status (
    user_id   text,
    preset_id text,
    status    text REFERENCES item_status(code),
    PRIMARY KEY (user_id, preset_id, status),
    FOREIGN KEY (user_id, preset_id) REFERENCES user_filter_preset ON DELETE CASCADE
);
```

Seven tables for what is one JSON object. That is not a criticism; it is the
price of the database knowing the shape.

## Operation by operation

| Operation | JSONB | Tables |
|---|---|---|
| Read a user's settings | 1 row, 1 query | 1 query with 4 joins + `json_agg`, or 4 queries; build the object in Go |
| Replace / merge-patch the whole thing | read-modify-write under `FOR UPDATE`, validate, 1 `UPDATE` | transaction: `UPDATE` + diff-and-sync 3 junction/child tables |
| Toggle one flag | read-modify-write + validate, or `jsonb_set` in place | `INSERT … ON CONFLICT DO NOTHING` / `DELETE`. Atomic, no read needed |
| Add a field with a default (`theme`) | Liquibase: `UPDATE … settings \|\| '{"theme":"system"}'` (rewrites every row) + schema + Go migrator | Liquibase: `ALTER TABLE ADD COLUMN theme … DEFAULT 'system'` (metadata-only on PG11+). Done |
| Remove a field | Liquibase `#-` rewrite + schema + Go | `DROP COLUMN`. Done |
| Add enum value, default off | edit JSON Schema. **No migration** | `INSERT INTO feature_flag VALUES ('betaExport')`. **No DDL** |
| Add enum value, default on | schema + append to every row | `INSERT INTO map_layer` + `INSERT INTO user_map_layer SELECT user_id, 'hydrology' …` |
| Remove enum value | schema + `arr - 'x'` rewrite | `DELETE FROM feature_flag WHERE code='x'` and `ON DELETE CASCADE` cleans up. **Lossy in both**, but the FK makes "where is it referenced" a non-question |
| Rename enum value | schema + rewrite arrays | `UPDATE feature_flag SET code=…` with `ON UPDATE CASCADE`, one statement |
| Restructure (`filterPresets` strings → objects) | one data-rewriting changeset | new tables + `INSERT … SELECT` from the old junction table + drop old. Comparable effort |
| Enforce enum values | app-side (JSON Schema). DB only knows "array" | foreign keys. Impossible to insert an unknown value, from any client |
| Enforce `uniqueItems` | app-side | primary key |
| Enforce `maxItems: 20` presets | app-side | trigger or app-side |
| "Which users have flag X?" | `WHERE settings @> '{"featureFlags":["X"]}'` with the GIN index: fine | `SELECT user_id FROM user_feature_flag WHERE flag='X'`: trivial, and joins to anything |
| "How many users per theme?" | `GROUP BY settings->>'theme'`, sequential scan unless you add an expression index | `GROUP BY theme`, plain B-tree |
| Reporting / BI tools | they see one opaque column | they see tables |
| Schema drift risk | high unless the schema is strict and versioned (this repo's whole point) | the DB is the schema |
| Adding an org-wide layer later (defaults → org → user) | merge three JSONB docs in Go, or `jsonb` `\|\|` in SQL. Natural | every table needs a scope column, or a parallel set of org tables, plus precedence logic per field |
| Concurrency | document-level: ETag/`If-Match` or row lock. Two users editing different fields of the same doc conflict | row-level: two writers to different fields never conflict |
| Rolling deploy with a breaking change | needs expand/contract + lazy migrator (see README) | needs expand/contract too, but additive DDL is cheap and online |
| Liquibase fit | runs the SQL, cannot see inside the JSON; you keep version discipline | native: `addColumn`, `addForeignKeyConstraint`, `diff`, `generateChangeLog` all work |

## When to pick which

Pick **JSONB** when:

- settings are read and written as a unit by their owner, almost never
  queried across users;
- the shape changes often and you want adding a flag to be a one-line schema
  edit (and that discipline is enforced by tests, as here);
- the document is small (kilobytes) and nesting is real but shallow;
- you expect to layer scopes (system / org / user) by merging documents.

Pick **tables** when:

- "which users have X" or per-value analytics are first-class use cases;
- enum values must be enforced for every client, not just your API;
- fields are updated independently by different actors and document-level
  conflicts would be a real problem;
- other systems or BI tools read the data directly.

Pick a **hybrid** when you are not sure, and that is usually the right call:
JSONB document as the source of truth, plus a generated column or a small
denormalized table for the two or three things you actually query. This repo
already does the minimum of that with `schema_version` and the GIN index; a
`theme text GENERATED ALWAYS AS (settings->>'theme') STORED` column is the
next step, and Liquibase adds it with one `addColumn`.

## What stays the same either way

- Liquibase is the migration runner in both designs. The difference is only
  that with tables it understands your changes and with JSONB it runs your SQL.
- The API can look identical. `PATCH` with merge-patch semantics and the
  per-path endpoints are both implementable on tables; the handler would
  translate instead of validate.
- The frontend does not care. It consumes a JSON document and generated
  types in both cases.
