import { useEffect, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { getUser, setUser } from "./api/client";
import { CURRENT_SCHEMA_VERSION } from "./generated/user-settings";
import { useEnums } from "./hooks/useSchema";
import { useSettings } from "./hooks/useSettings";
import { ErrorBanner } from "./components/ErrorBanner";
import { FeatureFlags } from "./components/FeatureFlags";
import { FilterPresets } from "./components/FilterPresets";
import { MapSettings } from "./components/MapSettings";
import { RawEditor } from "./components/RawEditor";
import { ThemePicker } from "./components/ThemePicker";

export default function App() {
  const qc = useQueryClient();
  const [user, setUserState] = useState(getUser());
  const settings = useSettings();
  const { serverVersion } = useEnums();

  // The theme setting drives the page theme: settings are consumed, not just edited.
  useEffect(() => {
    document.documentElement.dataset.theme = settings.data?.settings.theme ?? "system";
  }, [settings.data?.settings.theme]);

  const switchUser = (u: string) => {
    setUser(u);
    setUserState(u);
    void qc.invalidateQueries({ queryKey: ["settings"] });
  };

  return (
    <main>
      <header>
        <h1>User settings</h1>
        <label>
          User{" "}
          <select value={user} onChange={(e) => switchUser(e.target.value)}>
            {["alice", "bob", "carol", "new-user"].map((u) => (
              <option key={u}>{u}</option>
            ))}
          </select>
        </label>
        <span className="hint">
          frontend types v{CURRENT_SCHEMA_VERSION}
          {serverVersion !== undefined && serverVersion !== CURRENT_SCHEMA_VERSION && <strong> · server schema v{serverVersion}</strong>}
        </span>
      </header>

      {settings.isLoading && <p>Loading…</p>}
      <ErrorBanner error={settings.error} />
      {settings.data && (
        <div className="grid">
          <ThemePicker settings={settings.data.settings} />
          <FeatureFlags settings={settings.data.settings} />
          <MapSettings settings={settings.data.settings} />
          <FilterPresets settings={settings.data.settings} />
          <RawEditor settings={settings.data.settings} etag={settings.data.etag} />
        </div>
      )}
    </main>
  );
}
