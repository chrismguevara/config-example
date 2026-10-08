import type { BaseLayer, MapLayer, UserSettings } from "../generated/user-settings";
import { api } from "../api/client";
import { useEnums } from "../hooks/useSchema";
import { useSettingsMutation } from "../hooks/useSettings";

/** Nested object with a scalar enum and an array of enums. */
export function MapSettings({ settings }: { settings: UserSettings }) {
  const { values } = useEnums();
  const base = useSettingsMutation(api.setBaseLayer);
  const layer = useSettingsMutation(api.setLayer);
  return (
    <section>
      <h2>Map</h2>
      <p className="hint">
        Nested object. <code>PUT /map/base-layer</code>, <code>PUT/DELETE /map/layers/:layer</code>
      </p>
      <h3>Base layer</h3>
      <div className="row">
        {values("BaseLayer").map((b: BaseLayer) => (
          <label key={b}>
            <input type="radio" name="baseLayer" checked={settings.map.baseLayer === b} disabled={base.isPending} onChange={() => base.mutate([b])} />
            {b}
          </label>
        ))}
      </div>
      <h3>Layers</h3>
      <div className="row">
        {values("MapLayer").map((l: MapLayer) => {
          const on = settings.map.layers.includes(l);
          return (
            <label key={l}>
              <input type="checkbox" checked={on} disabled={layer.isPending} onChange={() => layer.mutate([l, !on])} />
              {l}
            </label>
          );
        })}
      </div>
    </section>
  );
}
