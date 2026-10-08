package settings

import (
	"encoding/json"
	"fmt"
)

// Doc is the untyped JSON object form used while a document is being migrated
// or merge-patched, i.e. while we cannot assume the current Go struct shape.
type Doc = map[string]any

// migrators upgrade a document from version N to N+1, in place. Each one
// MIRRORS the Liquibase changeset for the same step (db/changelog/000N-*.sql):
// the SQL is the eager path that rewrites every row at deploy time, this is
// the lazy path that fixes up any row the SQL did not reach (a row written by
// an old app instance during a rolling deploy, a restored backup, a dev DB
// that skipped a migration). TestEagerMatchesLazy asserts they agree.
var migrators = map[int]func(Doc) error{
	1: migrateV1toV2,
	2: migrateV2toV3,
	3: migrateV3toV4,
}

// Upgrade brings doc to CurrentVersion. It reports whether anything changed.
func Upgrade(doc Doc) (bool, error) {
	from := Version(doc)
	if from > CurrentVersion {
		return false, fmt.Errorf("settings version %d is newer than this backend supports (%d)", from, CurrentVersion)
	}
	for v := from; v < CurrentVersion; v++ {
		m, ok := migrators[v]
		if !ok {
			return false, fmt.Errorf("no migrator from settings version %d", v)
		}
		if err := m(doc); err != nil {
			return false, fmt.Errorf("migrate v%d -> v%d: %w", v, v+1, err)
		}
		doc["schemaVersion"] = v + 1
	}
	return from != CurrentVersion, nil
}

// Version reads schemaVersion from an untyped document (0 if missing/invalid).
func Version(doc Doc) int {
	switch n := doc["schemaVersion"].(type) {
	case float64:
		return int(n)
	case int:
		return n
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	}
	return 0
}

// --- v1 -> v2 ---------------------------------------------------------------
// Add `theme` (default "system"). Add layer `hydrology` which defaults ON, so
// it is appended to every row. Flag `betaExport` defaults OFF: nothing to do.
func migrateV1toV2(doc Doc) error {
	doc["theme"] = "system"
	m, err := object(doc, "map")
	if err != nil {
		return err
	}
	layers := stringList(m["layers"])
	m["layers"] = toAny(addUnique(layers, "hydrology"))
	return nil
}

// --- v2 -> v3 ---------------------------------------------------------------
// Remove field `map.showLabels` (true -> `labels` layer). Remove flag
// `legacyReports`. Rename layer `parcels` -> `cadastral`.
func migrateV2toV3(doc Doc) error {
	m, err := object(doc, "map")
	if err != nil {
		return err
	}
	layers := stringList(m["layers"])
	if show, _ := m["showLabels"].(bool); show {
		layers = addUnique(layers, "labels")
	}
	delete(m, "showLabels")

	if contains(layers, "parcels") {
		layers = removeAll(removeAll(layers, "parcels"), "cadastral")
		layers = append(layers, "cadastral")
	}
	m["layers"] = toAny(layers)

	doc["featureFlags"] = toAny(removeAll(stringList(doc["featureFlags"]), "legacyReports"))
	return nil
}

// --- v3 -> v4 ---------------------------------------------------------------
// `filterPresets`: array of built-in ids -> array of preset objects. Known ids
// expand to their definition, unknown ids are dropped.
var builtinPresets = map[string]FilterPreset{
	"open-items":     {ID: "open-items", Name: "Open items", Filters: PresetFilters{Status: []string{"open"}, Assignee: "anyone"}},
	"assigned-to-me": {ID: "assigned-to-me", Name: "Assigned to me", Filters: PresetFilters{Status: []string{"open", "pending"}, Assignee: "me"}},
	"unassigned":     {ID: "unassigned", Name: "Unassigned", Filters: PresetFilters{Status: []string{"open"}, Assignee: "unassigned"}},
}

func migrateV3toV4(doc Doc) error {
	ids := stringList(doc["filterPresets"])
	out := make([]any, 0, len(ids))
	for _, id := range ids {
		p, ok := builtinPresets[id]
		if !ok {
			continue
		}
		// Round-trip through JSON so the element has the same untyped shape
		// (map[string]any) as everything else in the document.
		b, _ := json.Marshal(p)
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		out = append(out, m)
	}
	doc["filterPresets"] = out
	return nil
}

// --- helpers ---------------------------------------------------------------

func object(doc Doc, key string) (map[string]any, error) {
	m, ok := doc[key].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%q is not an object", key)
	}
	return m, nil
}

func stringList(v any) []string {
	items, _ := v.([]any)
	out := make([]string, 0, len(items))
	for _, it := range items {
		if s, ok := it.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func toAny(list []string) []any {
	out := make([]any, len(list))
	for i, s := range list {
		out[i] = s
	}
	return out
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
