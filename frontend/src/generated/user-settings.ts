/* eslint-disable */
/**
 * GENERATED FILE - do not edit.
 * Source: schemas/user-settings.v4.schema.json
 * Regenerate with: npm run gen:types
 */

export type Theme = "light" | "dark" | "system";
export type FeatureFlag = "newDashboard" | "betaExport";
export type BaseLayer = "streets" | "satellite";
export type MapLayer = "traffic" | "cadastral" | "zoning" | "hydrology" | "labels";
export type ItemStatus = "open" | "pending" | "closed";
export type AssigneeFilter = "me" | "unassigned" | "anyone";

/**
 * Version 4 (current). Changes from v3: `filterPresets` becomes an array of user-defined preset objects instead of an array of built-in preset ids. Existing ids are expanded into their object form.
 */
export interface UserSettings {
  schemaVersion: 4;
  theme: Theme;
  /**
   * Enabled feature flags. A flag that is absent is off.
   */
  featureFlags: FeatureFlag[];
  map: {
    baseLayer: BaseLayer;
    /**
     * Visible overlay layers.
     */
    layers: MapLayer[];
  };
  /**
   * User-defined filter presets. `id` is unique within the document.
   *
   * @maxItems 20
   */
  filterPresets: FilterPreset[];
}
export interface FilterPreset {
  id: string;
  name: string;
  filters: {
    status: ItemStatus[];
    assignee: AssigneeFilter;
  };
}

export const CURRENT_SCHEMA_VERSION = 4 as const;

/** Enum values from the schema's $defs, in schema order. */
export const ENUMS = {
  Theme: ["light","dark","system"] as const,
  FeatureFlag: ["newDashboard","betaExport"] as const,
  BaseLayer: ["streets","satellite"] as const,
  MapLayer: ["traffic","cadastral","zoning","hydrology","labels"] as const,
  ItemStatus: ["open","pending","closed"] as const,
  AssigneeFilter: ["me","unassigned","anyone"] as const,
} as const;
