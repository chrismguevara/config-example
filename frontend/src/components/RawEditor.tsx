import { useState } from "react";
import { api, type SettingsPatch } from "../api/client";
import type { UserSettings } from "../generated/user-settings";
import { useSettingsMutation } from "../hooks/useSettings";
import { ErrorBanner } from "./ErrorBanner";

/**
 * Whole-document view and a merge-patch box. Demonstrates:
 *  - PATCH with If-Match (stale ETag -> 412 and an automatic refetch);
 *  - 422 with JSON-pointer problems when the patch violates the schema.
 */
export function RawEditor({ settings, etag }: { settings: UserSettings; etag: string | null }) {
  const [patch, setPatch] = useState('{\n  "theme": "dark",\n  "map": { "baseLayer": "satellite" }\n}');
  const [parseError, setParseError] = useState<string | null>(null);
  const m = useSettingsMutation(api.patch);
  const reset = useSettingsMutation(api.reset);

  const apply = () => {
    try {
      const parsed = JSON.parse(patch) as SettingsPatch;
      setParseError(null);
      m.mutate([parsed, etag]);
    } catch (e) {
      setParseError(String(e));
    }
  };

  return (
    <section>
      <h2>Document</h2>
      <p className="hint">
        schemaVersion {settings.schemaVersion}, ETag <code>{etag ?? "—"}</code>
      </p>
      <pre>{JSON.stringify(settings, null, 2)}</pre>
      <h3>Merge patch (RFC 7386)</h3>
      <p className="hint">
        <code>PATCH /api/me/settings</code> with <code>If-Match</code>. Arrays are replaced wholesale; <code>null</code> deletes a key (and fails
        validation if the key is required).
      </p>
      <textarea rows={5} value={patch} onChange={(e) => setPatch(e.target.value)} spellCheck={false} />
      <div className="row">
        <button type="button" onClick={apply} disabled={m.isPending}>
          Apply patch
        </button>
        <button type="button" className="danger" onClick={() => reset.mutate([])} disabled={reset.isPending}>
          Reset to defaults
        </button>
      </div>
      {parseError && <div className="banner error">{parseError}</div>}
      <ErrorBanner error={m.error ?? reset.error} />
    </section>
  );
}
