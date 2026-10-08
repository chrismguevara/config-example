# User settings in a Postgres JSONB column, managed with Liquibase

A worked example of storing a per-user settings document (feature flags, map
layers, filter presets) as **one JSONB column**, evolving its shape over four
schema versions with **Liquibase**, validating it with **versioned JSON
Schemas**, serving it from a **Go + Gin** API through **GORM**, and consuming it
from a **React + TypeScript** frontend whose types are generated from the same
schema.

```
┌───────────────────────────┐   go:embed   ┌──────────────────────┐
│ schemas/                  │─────────────▶│ backend (Go/Gin/GORM)│
│  user-settings.v1..v4.json│              │  validate every write│
│  (single source of truth) │  gen:types   │  lazy-upgrade on read│
│                           │─────────────▶│ frontend (React/TS)  │
└───────────────────────────┘              └──────────┬───────────┘
            │ hand-written to match                   │ GET/PATCH/PUT
            ▼                                         ▼
┌───────────────────────────┐              ┌──────────────────────┐
│ db/changelog/*.sql        │  liquibase   │ Postgres             │
│  v1→v2→v3→v4 rewrites     │─────────────▶│  user_settings.settings jsonb
│  + coarse CHECKs          │              │  schema_version (generated)
└───────────────────────────┘              └──────────────────────┘
```

## Quick start

```bash
docker compose up --build      # postgres → liquibase (seeds 3 v1 users, migrates to v4) → api → web
open http://localhost:5173     # pick alice / bob / carol to see migrated docs, new-user for defaults
```

Without Docker: run Postgres, `cd db && liquibase update --contexts=dev`,
`make api`, `make web`. See the `Makefile` for the rest.

Tests:

```bash
go test ./...                                                         # unit (no DB)
TEST_DATABASE_URL=postgres://settings:settings@localhost:5432/settings?sslmode=disable go test ./...   # + integration
```

`TestEagerMatchesLazy` is the important one: it asserts that what Liquibase
did to the seeded rows is byte-for-byte what the Go migrators produce from the
same v1 input.

## Layout

| Path | What |
|---|---|
| `schemas/user-settings.v{1..4}.schema.json` | One JSON Schema per document version. Enum-like values are `$defs` with `enum`. |
| `schemas/embed.go` | Embeds the schemas into the Go binary. |
| `db/changelog/changelog-master.yaml` | Liquibase master changelog. |
| `db/changelog/0001-create-user-settings.sql` | Table, generated `schema_version` column, GIN index, coarse CHECKs. |
| `db/changelog/0001b-dev-seed-v1.sql` | `context:dev` seed of three v1 documents (placed before the migrations on purpose). |
| `db/changelog/0002..0004-*.sql` | One data migration per version, each with a precondition and a `--rollback`. |
| `backend/internal/settings/model.go` | Go struct for the current version, `Default()`. |
| `backend/internal/settings/schema.go` | Compiles the schemas, validates, flattens errors to `{path, message}`. |
| `backend/internal/settings/migrate.go` | Lazy in-app migrators that mirror the SQL. |
| `backend/internal/settings/store.go` | GORM model and read-modify-write under `SELECT … FOR UPDATE`. |
| `backend/internal/settings/service.go` | Defaults, lazy upgrade, merge patch, ETags, typed mutations. |
| `backend/internal/settings/handler.go` | Gin routes. |
| `frontend/scripts/gen-types.mjs` | JSON Schema → `src/generated/user-settings.ts` (types + `ENUMS`). |
| `frontend/src/hooks/useSchema.ts` | Enum option lists: generated at build time, overridden by the live schema at runtime. |
| `docs/json-vs-tables.md` | The comparison with a normalized design. |

## The document

Current (v4) shape:

```json
{
  "schemaVersion": 4,
  "theme": "dark",
  "featureFlags": ["newDashboard"],
  "map": { "baseLayer": "satellite", "layers": ["traffic", "hydrology", "labels", "cadastral"] },
  "filterPresets": [
    { "id": "open-items", "name": "Open items", "filters": { "status": ["open"], "assignee": "anyone" } }
  ]
}
```

Rules that make the rest work:

- **The document is always complete.** Every required key is present; defaults
  are applied when a document is created (`Default()` in Go) or migrated, not
  merged in at read time. A user with no row gets `Default()` without a write.
- **`schemaVersion` is inside the document** and exposed as a generated column
  so you can `SELECT count(*) … GROUP BY schema_version`.
- **Enum-like arrays are arrays of enum strings** with `uniqueItems: true`.
  Absent flag = off. Policy choices are explicit: new flags default **off**
  (schema-only change), new map layers default **on** (schema + data backfill).
- **The JSON Schema is the only place enum values live.** Go uses plain
  strings and validates every write against the schema; the frontend
  generates union types and reads the live enum lists from `/api/settings/schema`.

## Change playbook

What each kind of change touches. "Schema" = edit `schemas/user-settings.v<N+1>.schema.json`
(copy the previous file, bump `const`), "SQL" = new Liquibase changeset,
"Go" = new entry in `migrators` + possibly `Default()`, "TS" = `npm run gen:types`.

| Change | Schema | SQL (Liquibase) | Go migrator | `Default()` | Frontend |
|---|---|---|---|---|---|
| Add scalar field (`theme`, v2) | add property + `required` | `settings \|\| '{"theme":"system"}'` | `doc["theme"]="system"` | add | regen; UI picks it up |
| Remove field (`map.showLabels`, v3) | drop property | `settings #- '{map,showLabels}'` | `delete(m,"showLabels")` | remove | regen; compile error where it was used |
| Add enum value, default off (`betaExport`, v2) | add to `enum` | **none** | **none** | none | regen; checkbox appears |
| Add enum value, default on (`hydrology`, v2) | add to `enum` | append to every row's array | `addUnique` | add | regen; checkbox appears |
| Remove enum value (`legacyReports`, v3) | remove from `enum` | `arr - 'legacyReports'` | `removeAll` | — | regen; compile error where it was referenced |
| Rename enum value (`parcels`→`cadastral`, v3) | rename in `enum` | `(arr - 'parcels') \|\| '["cadastral"]'` | same | update | regen |
| Restructure (`filterPresets` strings→objects, v4) | new `$defs` + items | throwaway `pg_temp` function mapping old→new | same mapping | update | regen; components rewritten |
| Add a nested field with default | add property | `jsonb_set(settings,'{map,newKey}','…')` | set key | add | regen |

Every data migration follows the same template (`db/changelog/0002-*.sql`):

```sql
--changeset config-example:000N-vN-what-changed
--preconditions onFail:HALT onError:HALT
--precondition-sql-check expectedResult:0 SELECT count(*) FROM user_settings WHERE schema_version <> N-1
ALTER TABLE user_settings DROP CONSTRAINT user_settings_version;
UPDATE user_settings SET settings = <jsonb expression producing version N>;
ALTER TABLE user_settings ADD CONSTRAINT user_settings_version CHECK ((settings->>'schemaVersion')::integer = N);
--rollback <reverse, with a comment on what is lossy>
```

The JSONB operators you need for all of the above: `||` (add/overwrite
top-level keys, append to arrays), `-` (remove key, or remove a string element
from an array), `#-` (remove nested key), `jsonb_set` (set nested path), `@>`
(contains, for "only append if absent"), `jsonb_array_elements[_text] WITH
ORDINALITY` + `jsonb_agg(… ORDER BY ord)` (map/filter arrays preserving order).
When the expression stops reading top-to-bottom, define a `pg_temp` SQL
function in the changeset and drop it at the end (`0003`, `0004`).

## How the four versions evolved

| v | Change | Lossless rollback? |
|---|---|---|
| 1 | `featureFlags[]`, `map{baseLayer, layers[], showLabels}`, `filterPresets[]` of preset ids | — |
| 2 | + `theme`; + flag `betaExport` (off, no data change); + layer `hydrology` (on, backfilled) | yes |
| 3 | − `map.showLabels` (true → `labels` layer); − flag `legacyReports` (dropped); layer `parcels` → `cadastral` | **no** (`legacyReports` is gone) |
| 4 | `filterPresets` becomes user-defined objects; known ids expand, unknown ids drop | **no** (custom presets collapse) |

`docker compose up` seeds alice/bob/carol at v1 before 0002–0004 run, so the
database you get is a migrated one. Try `make rollback-one` and look at the rows.

## Backend API

All under `/api/me/settings`, user from `X-User-ID` (auth is out of scope).
Every write returns the whole document and an `ETag`.

| Method | Path | Body | Notes |
|---|---|---|---|
| GET | `` | | 200 + `ETag`. Upgrades and persists an old-version row on the way out. |
| PATCH | `` | RFC 7386 merge patch | Optional `If-Match` → 412 on mismatch. Arrays replace wholesale; `null` deletes. `schemaVersion` is rejected (400). |
| DELETE | `` | | Reset to `Default()` (deletes the row). |
| PUT | `/theme` | `{"theme":"dark"}` | |
| PUT / DELETE | `/feature-flags/:flag` | | Enable / disable. Idempotent. |
| PUT | `/map/base-layer` | `{"baseLayer":"satellite"}` | |
| PUT / DELETE | `/map/layers/:layer` | | Show / hide. |
| PUT / DELETE | `/filter-presets/:id` | preset object | Upsert by id / remove. |
| GET | `/api/settings/schema[/:version]` | | The JSON Schema, for runtime enum lists. |

Error envelope, always the same:

```json
{ "error": "settings failed schema validation", "schemaVersion": 4,
  "problems": [ { "path": "/map/layers/4", "message": "value must be one of 'traffic', 'cadastral', 'zoning', 'hydrology', 'labels'" } ] }
```

Write path (`service.go`): `SELECT … FOR UPDATE` → nil→`Default()` or
decode + lazy `Upgrade` → apply change (merge patch on raw JSON, or typed
mutation on the `Settings` struct) → **validate against the current JSON
Schema** → canonical encode → upsert. The ETag is a hash of the canonical
bytes, so no version column is needed and `If-Match` works across the
PATCH endpoint; per-path endpoints skip it because they are serialized by
the row lock and semantically idempotent.

GORM is used for data access only. **There is no `AutoMigrate`**; Liquibase
owns the DDL. The `schema_version` generated column is tagged `->` (read-only).

## Frontend consumption

1. `npm run gen:types` turns the highest `schemas/user-settings.v*.schema.json`
   into `src/generated/user-settings.ts`: the `UserSettings` interface, one
   union type per `$defs` enum (`type MapLayer = "traffic" | …`), and an
   `ENUMS` constant with the value lists. `npm run build` runs it.
2. `useEnums()` returns option lists for checkboxes/radios: the generated
   `ENUMS` by default, overridden by the live schema from
   `/api/settings/schema`. A backend that added a layer shows it before the
   frontend is rebuilt; a frontend that is ahead gets a 422 it can display.
3. `useSettings()` is a TanStack Query over `GET`; `useSettingsMutation(api.x)`
   wraps any write, replaces the cache with the server's document on success,
   and refetches on 412 so the user sees what changed under them.
4. Components never hard-code enum values. Removing a value from the schema
   makes the generated union narrower, so stale references fail `tsc`.

## Your questions

### Can Liquibase manage all these changes?

Yes, with one caveat about *what* it manages. Liquibase is a changeset
runner with bookkeeping (`databasechangelog`, locks, contexts, preconditions,
rollbacks, `status`/`diff` tooling). Every change in the playbook above is
plain SQL against JSONB, so formatted-SQL changesets handle all of it, including
data rewrites, constraint swaps and rollbacks. What Liquibase does **not** know
is the shape of the document: its `diff`/`generateChangeLog` features see a
`jsonb` column and nothing else, and its XML/YAML change types (`addColumn`,
`addForeignKeyConstraint`, …) do not apply inside the JSON. The document's
"schema" therefore lives in the JSON Schema files, and the discipline of
"one schema version = one changeset = one Go migrator" is yours to keep,
enforced here by `TestEveryVersionHasASchemaAndAMigrator` and
`TestEagerMatchesLazy`.

Things worth knowing:

- Preconditions (`--precondition-sql-check expectedResult:0 … WHERE schema_version <> N-1`)
  make a migration refuse to run on a database in an unexpected state.
- `--rollback` is free-form SQL, so you decide what is reversible. Say so in a
  comment when it is lossy (v3, v4). Note the parser gotcha: a comment line
  starting with `-- rollback …` (any case) is treated as rollback SQL.
- Use `splitStatements:false` on changesets that contain `$$`-quoted function
  bodies.
- A single `UPDATE` rewrites every row in one transaction. Beyond a few
  million rows, batch by `user_id` ranges inside the changeset, or run the
  data part with `runInTransaction:false` and a loop. The schema-version
  `CHECK` must then be relaxed to a range (`BETWEEN N-1 AND N`) for the
  duration.

### Can we use JSON Schemas between versions?

Yes, and it is the piece that makes JSONB disciplined. One file per version,
`schemaVersion` as a `const` inside each, and three consumers of the same
file: the Go validator (embedded), the TypeScript generator, and you when
writing the changeset. Versioned schemas also let the backend keep validating
*old* documents (`GET /api/settings/schema/1`) and let tests check that each
migration step produces a document valid for the *next* version
(`TestEachStepProducesAValidIntermediateVersion`).

Keep the schema strict: `additionalProperties: false`, `required` for every
key, `uniqueItems` on enum arrays, `const` for `schemaVersion`. Strictness
is what turns "someone typo'd a flag name" into a 422 with a JSON pointer
instead of silently dead data.

### Validation in the database

Vanilla Postgres cannot validate against JSON Schema, so this example keeps
database-side guards coarse: `jsonb_typeof` checks on the top-level keys, and
a `CHECK` that pins `schemaVersion` to the current version so an old app
build cannot write an old shape after the migration. The full validation
runs in Go on every write.

If you can install the `pg_jsonschema` extension (available on Supabase,
Neon, and self-hosted; not on RDS as of this writing), you can get
full validation in the database and it handles multiple versions fine,
because the constraint is just an expression:

```sql
ALTER TABLE user_settings ADD CONSTRAINT user_settings_valid CHECK (
  CASE (settings->>'schemaVersion')::int
    WHEN 4 THEN jsonb_matches_schema('<contents of v4 schema>'::json, settings)
    ELSE false
  END);
```

Each Liquibase changeset would then also replace this constraint with the new
schema text. It costs a schema parse per write; keep schemas small or cache
them in a table and wrap the check in a SQL function.

### Eager (Liquibase) vs lazy (Go) migration

Both are implemented; eager is primary. The lazy path (`migrate.go`) exists
because the eager path has a gap: during a rolling deploy, an old app
instance can read a v3 row, the migration rewrites it to v4, and the old
instance writes v3 back (blocked here by the `CHECK`, which then surfaces as
a 500 in the old instance). The lazy path also covers restored backups and
developer databases that skipped a step. If you only want one, keep the
eager one and accept the expand/contract discipline below; if you cannot
afford a full-table rewrite at deploy time, keep only the lazy one and run a
background job that touches every row.

### Zero-downtime deploys

A version bump that removes or renames something is a breaking change for the
app build that is still running. The safe order is expand/contract: ship a
backend that accepts both N and N+1 (the lazy migrator does this on read),
run the Liquibase changeset, then ship the build that only knows N+1.
Additive changes (a new field with a default, a new enum value) do not need
that: an old build simply ignores or never emits the new value, and the
schema `CHECK` only pins the version, not the content.

### How this compares to normalized tables

See [docs/json-vs-tables.md](docs/json-vs-tables.md). Short version: JSONB
wins when the settings are read and written as a unit by one user, change
shape often, and are rarely queried across users. Tables win when you need
to ask "which users have flag X" at scale, enforce enum values with foreign
keys rather than app code, or patch one field without a read-modify-write.
A hybrid (JSONB document + a few generated or denormalized columns for the
things you query) is often the right answer and is what the
`schema_version` column and the GIN index already hint at.

## What was verified

- Liquibase 4.29 `update --contexts=dev` from empty to v4, `rollback-count 3`
  back to v1 (rows and constraints match v1), `update` again, `rollback-count 5`
  to an empty database, and a deliberate precondition failure (a row left at
  v2 makes the v4 changeset halt).
- `go test ./...` with `TEST_DATABASE_URL` set: schema, migrator, handler
  (in-memory store), GORM store, and SQL-vs-Go migration equivalence tests.
- `curl` against the running API: GET/PATCH/If-Match/412/422, per-path endpoints,
  defaults for an unknown user.
- Headless Chromium against the Vite dev server: migrated rows render,
  toggles round-trip, invalid patch shows the 422 banner, user switch shows
  defaults.
