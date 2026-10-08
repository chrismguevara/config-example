import { useState } from "react";
import type { AssigneeFilter, FilterPreset, ItemStatus, UserSettings } from "../generated/user-settings";
import { api } from "../api/client";
import { useEnums } from "../hooks/useSchema";
import { useSettingsMutation } from "../hooks/useSettings";
import { ErrorBanner } from "./ErrorBanner";

const slug = (s: string) =>
  s
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 64);

/** Array of objects (v4 shape). Each object has its own enum-typed fields. */
export function FilterPresets({ settings }: { settings: UserSettings }) {
  const { values } = useEnums();
  const upsert = useSettingsMutation(api.upsertPreset);
  const remove = useSettingsMutation(api.deletePreset);

  const [name, setName] = useState("");
  const [status, setStatus] = useState<ItemStatus[]>(["open"]);
  const [assignee, setAssignee] = useState<AssigneeFilter>("anyone");

  const submit = (e: React.FormEvent) => {
    e.preventDefault();
    const preset: FilterPreset = { id: slug(name), name: name.trim(), filters: { status, assignee } };
    upsert.mutate([preset], { onSuccess: () => setName("") });
  };

  return (
    <section>
      <h2>Filter presets</h2>
      <p className="hint">
        Array of objects. <code>PUT/DELETE /filter-presets/:id</code> (PUT upserts by id)
      </p>
      <ul className="presets">
        {settings.filterPresets.map((p) => (
          <li key={p.id}>
            <span>
              <strong>{p.name}</strong> <code>{p.id}</code> — status {p.filters.status.join(", ") || "any"}, assignee {p.filters.assignee}
            </span>
            <button type="button" onClick={() => remove.mutate([p.id])} disabled={remove.isPending}>
              remove
            </button>
          </li>
        ))}
        {settings.filterPresets.length === 0 && <li className="hint">No presets.</li>}
      </ul>
      <form onSubmit={submit} className="preset-form">
        <input placeholder="Preset name" value={name} onChange={(e) => setName(e.target.value)} required />
        <fieldset>
          <legend>Status</legend>
          {values("ItemStatus").map((s: ItemStatus) => (
            <label key={s}>
              <input
                type="checkbox"
                checked={status.includes(s)}
                onChange={(e) => setStatus(e.target.checked ? [...status, s] : status.filter((x) => x !== s))}
              />
              {s}
            </label>
          ))}
        </fieldset>
        <select value={assignee} onChange={(e) => setAssignee(e.target.value as AssigneeFilter)}>
          {values("AssigneeFilter").map((a: AssigneeFilter) => (
            <option key={a} value={a}>
              assignee: {a}
            </option>
          ))}
        </select>
        <button type="submit" disabled={upsert.isPending || !name.trim()}>
          Save preset
        </button>
      </form>
      <ErrorBanner error={upsert.error ?? remove.error} />
    </section>
  );
}
