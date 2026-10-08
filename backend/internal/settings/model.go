// Package settings implements the per-user settings document: its Go shape
// for the current schema version, validation against the versioned JSON
// Schemas, lazy in-app migration of old documents, storage in a Postgres
// JSONB column via GORM, and the Gin HTTP handlers.
package settings

// CurrentVersion is the schema version this build of the backend writes.
// Bumping it means: a new schemas/user-settings.v<N>.schema.json, a Liquibase
// changeset that rewrites stored rows, a migrator in migrate.go, regenerated
// frontend types, and (usually) edits to Default() below.
const CurrentVersion = 4

// Settings is the typed view of the current (v4) document. Enum-like fields
// are plain strings on purpose: the allowed values live in the JSON Schema,
// which is the single source of truth, and every write is validated against it.
// Adding an enum value therefore never requires a Go change.
type Settings struct {
	SchemaVersion int            `json:"schemaVersion"`
	Theme         string         `json:"theme"`
	FeatureFlags  []string       `json:"featureFlags"`
	Map           MapSettings    `json:"map"`
	FilterPresets []FilterPreset `json:"filterPresets"`
}

// MapSettings is the nested `map` object.
type MapSettings struct {
	BaseLayer string   `json:"baseLayer"`
	Layers    []string `json:"layers"`
}

// FilterPreset is one user-defined preset (v4 replaced the v1-v3 enum of
// built-in preset ids with these objects).
type FilterPreset struct {
	ID      string        `json:"id"`
	Name    string        `json:"name"`
	Filters PresetFilters `json:"filters"`
}

// PresetFilters is the filter payload of a preset.
type PresetFilters struct {
	Status   []string `json:"status"`
	Assignee string   `json:"assignee"`
}

// Default is the document a user gets before they have saved anything. It is
// not persisted until the first write. Policy decisions encoded here:
//   - new feature flags default OFF, so they are not listed;
//   - new map layers default ON, so they are listed (hydrology, labels).
//
// TestDefaultValidates guards this against the current schema.
func Default() Settings {
	return Settings{
		SchemaVersion: CurrentVersion,
		Theme:         "system",
		FeatureFlags:  []string{},
		Map: MapSettings{
			BaseLayer: "streets",
			Layers:    []string{"traffic", "hydrology", "labels"},
		},
		FilterPresets: []FilterPreset{
			{ID: "open-items", Name: "Open items", Filters: PresetFilters{Status: []string{"open"}, Assignee: "anyone"}},
			{ID: "assigned-to-me", Name: "Assigned to me", Filters: PresetFilters{Status: []string{"open", "pending"}, Assignee: "me"}},
		},
	}
}

// normalize makes nil slices empty so they encode as [] (the schema says
// "array", and null would fail validation).
func (s *Settings) normalize() {
	s.SchemaVersion = CurrentVersion
	if s.FeatureFlags == nil {
		s.FeatureFlags = []string{}
	}
	if s.Map.Layers == nil {
		s.Map.Layers = []string{}
	}
	if s.FilterPresets == nil {
		s.FilterPresets = []FilterPreset{}
	}
	for i := range s.FilterPresets {
		if s.FilterPresets[i].Filters.Status == nil {
			s.FilterPresets[i].Filters.Status = []string{}
		}
	}
}

// addUnique appends v if it is not already present (uniqueItems semantics).
func addUnique(list []string, v string) []string {
	for _, x := range list {
		if x == v {
			return list
		}
	}
	return append(list, v)
}

// removeAll returns list without any occurrence of v, never nil.
func removeAll(list []string, v string) []string {
	out := make([]string, 0, len(list))
	for _, x := range list {
		if x != v {
			out = append(out, x)
		}
	}
	return out
}
