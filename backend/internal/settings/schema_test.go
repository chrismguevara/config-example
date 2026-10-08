package settings

import (
	"encoding/json"
	"errors"
	"os"
	"testing"
)

func mustValidator(t *testing.T) *Validator {
	t.Helper()
	v, err := NewValidator()
	if err != nil {
		t.Fatalf("NewValidator: %v", err)
	}
	return v
}

func loadFixture(t *testing.T, name string) map[string]Doc {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]Doc
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestDefaultValidates(t *testing.T) {
	v := mustValidator(t)
	b, _ := json.Marshal(Default())
	doc, _ := decode(b)
	if err := v.Validate(CurrentVersion, doc); err != nil {
		t.Fatalf("Default() does not satisfy schema v%d: %v", CurrentVersion, err)
	}
}

func TestFixturesValidateAgainstTheirVersion(t *testing.T) {
	v := mustValidator(t)
	for user, doc := range loadFixture(t, "seed-v1.json") {
		if err := v.Validate(1, doc); err != nil {
			t.Errorf("seed %s vs v1: %v", user, err)
		}
	}
	for user, doc := range loadFixture(t, "expected-v4.json") {
		if err := v.Validate(4, doc); err != nil {
			t.Errorf("expected %s vs v4: %v", user, err)
		}
	}
}

func TestValidationErrorsPointAtTheProblem(t *testing.T) {
	v := mustValidator(t)
	cases := map[string]struct {
		mutate   func(Doc)
		wantPath string
	}{
		"unknown feature flag":   {func(d Doc) { d["featureFlags"] = []any{"notAFlag"} }, "/featureFlags/0"},
		"unknown map layer":      {func(d Doc) { d["map"].(Doc)["layers"] = []any{"traffic", "bogus"} }, "/map/layers/1"},
		"bad theme":              {func(d Doc) { d["theme"] = "sepia" }, "/theme"},
		"extra top-level field":  {func(d Doc) { d["unknown"] = 1 }, "/"},
		"wrong version const":    {func(d Doc) { d["schemaVersion"] = 3 }, "/schemaVersion"},
		"duplicate flag":         {func(d Doc) { d["featureFlags"] = []any{"betaExport", "betaExport"} }, "/featureFlags"},
		"bad preset id pattern":  {func(d Doc) { d["filterPresets"].([]any)[0].(Doc)["id"] = "Not Valid!" }, "/filterPresets/0/id"},
		"bad preset status enum": {func(d Doc) { d["filterPresets"].([]any)[0].(Doc)["filters"].(Doc)["status"] = []any{"weird"} }, "/filterPresets/0/filters/status/0"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			b, _ := json.Marshal(Default())
			doc, _ := decode(b)
			tc.mutate(doc)
			err := v.Validate(CurrentVersion, doc)
			var ve *ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("expected *ValidationError, got %v", err)
			}
			found := false
			for _, p := range ve.Problems {
				if p.Path == tc.wantPath {
					found = true
				}
			}
			if !found {
				t.Errorf("want a problem at %s, got %+v", tc.wantPath, ve.Problems)
			}
		})
	}
}

func TestEveryVersionHasASchemaAndAMigrator(t *testing.T) {
	v := mustValidator(t)
	for n := 1; n <= CurrentVersion; n++ {
		if v.SchemaJSON(n) == nil {
			t.Errorf("missing schema for v%d", n)
		}
		if n < CurrentVersion {
			if _, ok := migrators[n]; !ok {
				t.Errorf("missing migrator v%d -> v%d", n, n+1)
			}
		}
	}
}
