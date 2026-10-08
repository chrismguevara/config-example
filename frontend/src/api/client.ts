// Thin typed client for the settings API. Every write returns the whole
// document plus its ETag, so the UI always converges on server truth.
import type { UserSettings, FilterPreset, Theme, FeatureFlag, BaseLayer, MapLayer } from "../generated/user-settings";

export interface SettingsResponse {
  settings: UserSettings;
  etag: string | null;
}

export interface Problem {
  path: string;
  message: string;
}

/** Error envelope the backend returns for 4xx/5xx. */
export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
    public problems: Problem[] = [],
  ) {
    super(message);
  }
  get isConflict() {
    return this.status === 412;
  }
  get isValidation() {
    return this.status === 422;
  }
}

/** JSON Merge Patch (RFC 7386) shape: any subset of the document, null deletes. */
export type SettingsPatch = {
  theme?: Theme;
  featureFlags?: FeatureFlag[];
  map?: { baseLayer?: BaseLayer; layers?: MapLayer[] };
  filterPresets?: FilterPreset[];
};

let currentUser = "alice";
export const getUser = () => currentUser;
export const setUser = (u: string) => {
  currentUser = u;
};

async function call(path: string, init: RequestInit = {}, extraHeaders: Record<string, string> = {}): Promise<SettingsResponse> {
  const res = await fetch(`/api/me/settings${path}`, {
    ...init,
    headers: {
      "Content-Type": "application/json",
      "X-User-ID": currentUser, // auth is out of scope; see README
      ...extraHeaders,
      ...(init.headers ?? {}),
    },
  });
  if (!res.ok) {
    let body: { error?: string; problems?: Problem[] } = {};
    try {
      body = await res.json();
    } catch {
      /* non-JSON error body */
    }
    throw new ApiError(res.status, body.error ?? res.statusText, body.problems ?? []);
  }
  return { settings: (await res.json()) as UserSettings, etag: res.headers.get("ETag") };
}

export const api = {
  get: () => call(""),
  /** Whole-document merge patch with optimistic concurrency via If-Match. */
  patch: (patch: SettingsPatch, etag?: string | null) =>
    call("", { method: "PATCH", body: JSON.stringify(patch) }, etag ? { "If-Match": etag } : {}),
  reset: () => call("", { method: "DELETE" }),

  // Per-path endpoints: small, idempotent, no ETag needed (server serialises them).
  setTheme: (theme: Theme) => call("/theme", { method: "PUT", body: JSON.stringify({ theme }) }),
  setFlag: (flag: FeatureFlag, on: boolean) => call(`/feature-flags/${flag}`, { method: on ? "PUT" : "DELETE" }),
  setBaseLayer: (baseLayer: BaseLayer) => call("/map/base-layer", { method: "PUT", body: JSON.stringify({ baseLayer }) }),
  setLayer: (layer: MapLayer, on: boolean) => call(`/map/layers/${layer}`, { method: on ? "PUT" : "DELETE" }),
  upsertPreset: (preset: FilterPreset) => call(`/filter-presets/${preset.id}`, { method: "PUT", body: JSON.stringify(preset) }),
  deletePreset: (id: string) => call(`/filter-presets/${id}`, { method: "DELETE" }),
};

/** The live JSON Schema the backend validates against (for enum lists). */
export async function fetchSchema(): Promise<JsonSchema> {
  const res = await fetch("/api/settings/schema");
  if (!res.ok) throw new ApiError(res.status, "cannot load settings schema");
  return res.json();
}

export interface JsonSchema {
  title?: string;
  properties?: { schemaVersion?: { const?: number } };
  $defs?: Record<string, { enum?: string[]; title?: string }>;
}
