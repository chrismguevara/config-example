import type { Theme, UserSettings } from "../generated/user-settings";
import { api } from "../api/client";
import { useEnums } from "../hooks/useSchema";
import { useSettingsMutation } from "../hooks/useSettings";

export function ThemePicker({ settings }: { settings: UserSettings }) {
  const { values } = useEnums();
  const m = useSettingsMutation(api.setTheme);
  return (
    <section>
      <h2>Theme</h2>
      <p className="hint">
        Scalar field. <code>PUT /api/me/settings/theme</code>
      </p>
      <div className="row">
        {values("Theme").map((t: Theme) => (
          <label key={t}>
            <input type="radio" name="theme" checked={settings.theme === t} disabled={m.isPending} onChange={() => m.mutate([t])} />
            {t}
          </label>
        ))}
      </div>
    </section>
  );
}
