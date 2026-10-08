// Generates TypeScript types from the CURRENT settings JSON Schema.
//
// The schema file is the single source of truth shared with the Go backend
// (which embeds the same directory) and with the Liquibase migrations. Run
// `npm run gen:types` after editing a schema; `npm run build` runs it too.
import { compile } from "json-schema-to-typescript";
import { readFile, writeFile, readdir } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
const schemasDir = path.resolve(here, "../../schemas");
const outFile = path.resolve(here, "../src/generated/user-settings.ts");

// Pick the highest user-settings.v<N>.schema.json.
const files = (await readdir(schemasDir)).filter((f) => /^user-settings\.v\d+\.schema\.json$/.test(f));
const versions = files.map((f) => Number(f.match(/v(\d+)/)[1])).sort((a, b) => a - b);
const current = versions.at(-1);
const schemaPath = path.join(schemasDir, `user-settings.v${current}.schema.json`);
const schema = JSON.parse(await readFile(schemaPath, "utf8"));

let ts = await compile(schema, "UserSettings", {
  bannerComment: `/* eslint-disable */
/**
 * GENERATED FILE - do not edit.
 * Source: schemas/user-settings.v${current}.schema.json
 * Regenerate with: npm run gen:types
 */`,
  additionalProperties: false,
  strictIndexSignatures: true,
  maxItems: -1, // emit FilterPreset[] instead of a 20-way tuple union for maxItems: 20
  style: { singleQuote: false, semi: true },
});

// Also export the enum VALUE lists so UI code can iterate them without a
// runtime schema fetch. (The app fetches the live schema as well, see
// src/api/schema.ts, so a backend that is ahead of this build still works.)
const enumDefs = Object.entries(schema.$defs ?? {}).filter(([, d]) => Array.isArray(d.enum));
ts += `\nexport const CURRENT_SCHEMA_VERSION = ${current} as const;\n`;
ts += `\n/** Enum values from the schema's $defs, in schema order. */\nexport const ENUMS = {\n`;
for (const [name, def] of enumDefs) {
  ts += `  ${name}: ${JSON.stringify(def.enum)} as const,\n`;
}
ts += `} as const;\n`;

await writeFile(outFile, ts);
console.log(`wrote ${path.relative(process.cwd(), outFile)} from v${current}`);
