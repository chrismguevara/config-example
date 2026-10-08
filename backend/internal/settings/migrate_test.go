package settings

import (
	"encoding/json"
	"reflect"
	"testing"
)

// canon round-trips through JSON so ints/float64s and []any/[]string compare equal.
func canon(t *testing.T, v any) any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestUpgradeV1ToCurrentMatchesExpected(t *testing.T) {
	seed := loadFixture(t, "seed-v1.json")
	want := loadFixture(t, "expected-v4.json")
	v := mustValidator(t)

	for user, doc := range seed {
		changed, err := Upgrade(doc)
		if err != nil {
			t.Fatalf("%s: %v", user, err)
		}
		if !changed {
			t.Errorf("%s: expected Upgrade to report a change", user)
		}
		if err := v.Validate(CurrentVersion, doc); err != nil {
			t.Errorf("%s: upgraded doc invalid: %v", user, err)
		}
		if got, exp := canon(t, doc), canon(t, want[user]); !reflect.DeepEqual(got, exp) {
			gb, _ := json.MarshalIndent(got, "", "  ")
			eb, _ := json.MarshalIndent(exp, "", "  ")
			t.Errorf("%s:\n got %s\nwant %s", user, gb, eb)
		}
	}
}

func TestEachStepProducesAValidIntermediateVersion(t *testing.T) {
	v := mustValidator(t)
	for user, doc := range loadFixture(t, "seed-v1.json") {
		for from := 1; from < CurrentVersion; from++ {
			if err := migrators[from](doc); err != nil {
				t.Fatalf("%s v%d->v%d: %v", user, from, from+1, err)
			}
			doc["schemaVersion"] = from + 1
			if err := v.Validate(from+1, doc); err != nil {
				t.Errorf("%s after v%d->v%d: %v", user, from, from+1, err)
			}
		}
	}
}

func TestUpgradeIsIdempotentOnCurrentVersion(t *testing.T) {
	b, _ := json.Marshal(Default())
	doc, _ := decode(b)
	changed, err := Upgrade(doc)
	if err != nil || changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
}

func TestUpgradeRejectsFutureVersion(t *testing.T) {
	doc := Doc{"schemaVersion": float64(CurrentVersion + 1)}
	if _, err := Upgrade(doc); err == nil {
		t.Fatal("expected error for a newer-than-supported document")
	}
}

func TestV2ToV3EdgeCases(t *testing.T) {
	// labels already present and parcels+cadastral both present: no duplicates.
	doc := Doc{
		"schemaVersion": float64(2), "theme": "dark", "featureFlags": []any{"legacyReports", "newDashboard"},
		"map":           Doc{"baseLayer": "streets", "layers": []any{"labels", "parcels", "cadastral", "traffic"}, "showLabels": true},
		"filterPresets": []any{},
	}
	if err := migrateV2toV3(doc); err != nil {
		t.Fatal(err)
	}
	got := stringList(doc["map"].(Doc)["layers"])
	want := []string{"labels", "traffic", "cadastral"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("layers = %v, want %v", got, want)
	}
	if _, has := doc["map"].(Doc)["showLabels"]; has {
		t.Error("showLabels should be removed")
	}
	if flags := stringList(doc["featureFlags"]); !reflect.DeepEqual(flags, []string{"newDashboard"}) {
		t.Errorf("flags = %v", flags)
	}
}

func TestV3ToV4DropsUnknownPresetIds(t *testing.T) {
	doc := Doc{"schemaVersion": float64(3), "filterPresets": []any{"unassigned", "something-old", "open-items"}}
	if err := migrateV3toV4(doc); err != nil {
		t.Fatal(err)
	}
	presets := doc["filterPresets"].([]any)
	if len(presets) != 2 {
		t.Fatalf("got %d presets, want 2", len(presets))
	}
	if presets[0].(map[string]any)["id"] != "unassigned" || presets[1].(map[string]any)["id"] != "open-items" {
		t.Errorf("order not preserved: %v", presets)
	}
}
