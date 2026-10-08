import type { FeatureFlag, UserSettings } from "../generated/user-settings";
import { api } from "../api/client";
import { useEnums } from "../hooks/useSchema";
import { useSettingsMutation } from "../hooks/useSettings";

/**
 * Array-of-enum field. The checkbox list is driven by the schema's enum, so
 * adding a flag to schemas/user-settings.v4.schema.json is enough for it to
 * appear here. Flags not in the array are OFF.
 */
export function FeatureFlags({ settings }: { settings: UserSettings }) {
  const { values } = useEnums();
  const m = useSettingsMutation(api.setFlag);
  return (
    <section>
      <h2>Feature flags</h2>
      <p className="hint">
        Array of enum strings, absent = off. <code>PUT/DELETE /api/me/settings/feature-flags/:flag</code>
      </p>
      <div className="row">
        {values("FeatureFlag").map((flag: FeatureFlag) => {
          const on = settings.featureFlags.includes(flag);
          return (
            <label key={flag}>
              <input type="checkbox" checked={on} disabled={m.isPending} onChange={() => m.mutate([flag, !on])} />
              {flag}
            </label>
          );
        })}
      </div>
    </section>
  );
}
