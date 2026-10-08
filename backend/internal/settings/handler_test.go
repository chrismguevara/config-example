package settings

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
)

// memStore is an in-memory Store so the HTTP layer is testable without Postgres.
type memStore struct {
	mu   sync.Mutex
	rows map[string]json.RawMessage
}

func newMemStore() *memStore { return &memStore{rows: map[string]json.RawMessage{}} }

func (m *memStore) Get(_ context.Context, id string) (json.RawMessage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r, ok := m.rows[id]; ok {
		return r, nil
	}
	return nil, ErrNotFound
}

func (m *memStore) Update(_ context.Context, id string, fn func(json.RawMessage) (json.RawMessage, error)) (json.RawMessage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cur, ok := m.rows[id]
	if !ok {
		cur = nil
	}
	next, err := fn(cur)
	if err != nil {
		return nil, err
	}
	m.rows[id] = next
	return next, nil
}

func (m *memStore) Delete(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.rows, id)
	return nil
}

type env struct {
	t     *testing.T
	r     *gin.Engine
	store *memStore
}

func newEnv(t *testing.T) *env {
	t.Helper()
	gin.SetMode(gin.TestMode)
	v := mustValidator(t)
	store := newMemStore()
	r := gin.New()
	RegisterRoutes(r, NewService(store, v), v)
	return &env{t: t, r: r, store: store}
}

func (e *env) do(method, path string, body any, headers map[string]string) *httptest.ResponseRecorder {
	e.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		switch b := body.(type) {
		case string:
			buf.WriteString(b)
		default:
			_ = json.NewEncoder(&buf).Encode(b)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("X-User-ID", "u1")
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	e.r.ServeHTTP(w, req)
	return w
}

func settingsOf(t *testing.T, w *httptest.ResponseRecorder) Settings {
	t.Helper()
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var s Settings
	if err := json.Unmarshal(w.Body.Bytes(), &s); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestGetReturnsDefaultsWithoutPersisting(t *testing.T) {
	e := newEnv(t)
	w := e.do(http.MethodGet, "/api/me/settings", nil, nil)
	s := settingsOf(t, w)
	if s.Theme != "system" || s.SchemaVersion != CurrentVersion {
		t.Errorf("unexpected defaults: %+v", s)
	}
	if w.Header().Get("ETag") == "" {
		t.Error("missing ETag")
	}
	if len(e.store.rows) != 0 {
		t.Error("GET must not persist defaults")
	}
}

func TestMissingUserHeaderIs401(t *testing.T) {
	e := newEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/api/me/settings", nil)
	w := httptest.NewRecorder()
	e.r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status %d", w.Code)
	}
}

func TestPerPathEndpoints(t *testing.T) {
	e := newEnv(t)

	s := settingsOf(t, e.do(http.MethodPut, "/api/me/settings/feature-flags/betaExport", nil, nil))
	if !contains(s.FeatureFlags, "betaExport") {
		t.Errorf("flag not enabled: %v", s.FeatureFlags)
	}
	s = settingsOf(t, e.do(http.MethodPut, "/api/me/settings/feature-flags/betaExport", nil, nil))
	if n := len(removeAll(s.FeatureFlags, "betaExport")); len(s.FeatureFlags)-n != 1 {
		t.Errorf("flag enabled twice: %v", s.FeatureFlags)
	}
	s = settingsOf(t, e.do(http.MethodDelete, "/api/me/settings/feature-flags/betaExport", nil, nil))
	if contains(s.FeatureFlags, "betaExport") {
		t.Errorf("flag not disabled: %v", s.FeatureFlags)
	}

	w := e.do(http.MethodPut, "/api/me/settings/feature-flags/legacyReports", nil, nil)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("removed enum value accepted: %d %s", w.Code, w.Body.String())
	}
	var eb errorBody
	_ = json.Unmarshal(w.Body.Bytes(), &eb)
	if len(eb.Problems) == 0 || eb.Problems[0].Path != "/featureFlags/0" {
		t.Errorf("problem path = %+v", eb.Problems)
	}

	s = settingsOf(t, e.do(http.MethodDelete, "/api/me/settings/map/layers/hydrology", nil, nil))
	if contains(s.Map.Layers, "hydrology") {
		t.Errorf("layer not hidden: %v", s.Map.Layers)
	}
	s = settingsOf(t, e.do(http.MethodPut, "/api/me/settings/map/base-layer", `{"baseLayer":"satellite"}`, nil))
	if s.Map.BaseLayer != "satellite" {
		t.Errorf("baseLayer = %s", s.Map.BaseLayer)
	}
	s = settingsOf(t, e.do(http.MethodPut, "/api/me/settings/theme", `{"theme":"dark"}`, nil))
	if s.Theme != "dark" {
		t.Errorf("theme = %s", s.Theme)
	}

	preset := `{"name":"Closed by me","filters":{"status":["closed"],"assignee":"me"}}`
	s = settingsOf(t, e.do(http.MethodPut, "/api/me/settings/filter-presets/closed-by-me", preset, nil))
	if len(s.FilterPresets) != 3 || s.FilterPresets[2].ID != "closed-by-me" {
		t.Errorf("presets = %+v", s.FilterPresets)
	}
	s = settingsOf(t, e.do(http.MethodPut, "/api/me/settings/filter-presets/closed-by-me", `{"name":"Renamed","filters":{"status":["closed"],"assignee":"me"}}`, nil))
	if len(s.FilterPresets) != 3 || s.FilterPresets[2].Name != "Renamed" {
		t.Errorf("upsert did not replace: %+v", s.FilterPresets)
	}
	s = settingsOf(t, e.do(http.MethodDelete, "/api/me/settings/filter-presets/open-items", nil, nil))
	if len(s.FilterPresets) != 2 {
		t.Errorf("delete failed: %+v", s.FilterPresets)
	}

	s = settingsOf(t, e.do(http.MethodDelete, "/api/me/settings", nil, nil))
	if s.Theme != "system" || len(e.store.rows) != 0 {
		t.Errorf("reset failed: %+v rows=%d", s, len(e.store.rows))
	}
}

func TestMergePatch(t *testing.T) {
	e := newEnv(t)
	get := e.do(http.MethodGet, "/api/me/settings", nil, nil)
	etag := get.Header().Get("ETag")

	// Nested merge: only baseLayer changes, layers untouched; top-level theme changes.
	w := e.do(http.MethodPatch, "/api/me/settings", `{"theme":"light","map":{"baseLayer":"satellite"}}`, map[string]string{"If-Match": etag})
	s := settingsOf(t, w)
	if s.Theme != "light" || s.Map.BaseLayer != "satellite" || len(s.Map.Layers) != 3 {
		t.Errorf("merge result: %+v", s)
	}
	newTag := w.Header().Get("ETag")
	if newTag == etag {
		t.Error("ETag should change after a write")
	}

	// Stale If-Match -> 412.
	w = e.do(http.MethodPatch, "/api/me/settings", `{"theme":"dark"}`, map[string]string{"If-Match": etag})
	if w.Code != http.StatusPreconditionFailed {
		t.Fatalf("stale If-Match accepted: %d", w.Code)
	}

	// Arrays are replaced wholesale by merge patch (RFC 7386).
	s = settingsOf(t, e.do(http.MethodPatch, "/api/me/settings", `{"map":{"layers":["zoning"]}}`, map[string]string{"If-Match": newTag}))
	if len(s.Map.Layers) != 1 || s.Map.Layers[0] != "zoning" {
		t.Errorf("layers = %v", s.Map.Layers)
	}

	// Schema violation -> 422 with pointer.
	w = e.do(http.MethodPatch, "/api/me/settings", `{"featureFlags":["nope"]}`, nil)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid patch accepted: %d %s", w.Code, w.Body.String())
	}
	// Removing a required field with null -> 422.
	w = e.do(http.MethodPatch, "/api/me/settings", `{"theme":null}`, nil)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("null theme accepted: %d", w.Code)
	}
	// schemaVersion is server-managed -> 400.
	w = e.do(http.MethodPatch, "/api/me/settings", `{"schemaVersion":9}`, nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("schemaVersion patch accepted: %d", w.Code)
	}
	// Not an object -> 400.
	w = e.do(http.MethodPatch, "/api/me/settings", `[1,2]`, nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("array patch accepted: %d", w.Code)
	}
}

func TestLazyUpgradeOnRead(t *testing.T) {
	e := newEnv(t)
	seed := loadFixture(t, "seed-v1.json")
	raw, _ := json.Marshal(seed["alice"])
	e.store.rows["u1"] = raw

	s := settingsOf(t, e.do(http.MethodGet, "/api/me/settings", nil, nil))
	if s.SchemaVersion != CurrentVersion {
		t.Fatalf("not upgraded: %+v", s)
	}
	if !contains(s.Map.Layers, "cadastral") || contains(s.FeatureFlags, "legacyReports") {
		t.Errorf("migration content wrong: %+v", s)
	}
	var persisted Doc
	_ = json.Unmarshal(e.store.rows["u1"], &persisted)
	if Version(persisted) != CurrentVersion {
		t.Errorf("upgraded doc not written back: v%d", Version(persisted))
	}
}

func TestLazyUpgradeOnWrite(t *testing.T) {
	e := newEnv(t)
	seed := loadFixture(t, "seed-v1.json")
	raw, _ := json.Marshal(seed["bob"])
	e.store.rows["u1"] = raw

	s := settingsOf(t, e.do(http.MethodPut, "/api/me/settings/feature-flags/betaExport", nil, nil))
	if s.SchemaVersion != CurrentVersion || !contains(s.FeatureFlags, "betaExport") || len(s.FilterPresets) != 1 {
		t.Errorf("write on old row: %+v", s)
	}
}

func TestSchemaEndpoint(t *testing.T) {
	e := newEnv(t)
	w := e.do(http.MethodGet, "/api/settings/schema", nil, nil)
	if w.Code != http.StatusOK {
		t.Fatal(w.Code)
	}
	var sch map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &sch)
	if sch["title"] != "UserSettings" {
		t.Errorf("title = %v", sch["title"])
	}
	if w := e.do(http.MethodGet, "/api/settings/schema/1", nil, nil); w.Code != http.StatusOK {
		t.Errorf("v1 schema: %d", w.Code)
	}
	if w := e.do(http.MethodGet, "/api/settings/schema/99", nil, nil); w.Code != http.StatusNotFound {
		t.Errorf("v99 schema: %d", w.Code)
	}
}
