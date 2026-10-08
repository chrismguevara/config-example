package settings

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Integration tests. They need a database that Liquibase has already migrated:
//
//	TEST_DATABASE_URL=postgres://settings:settings@localhost:5432/settings?sslmode=disable go test ./...
//
// TestEagerMatchesLazy additionally needs the dev seed (`liquibase update --contexts=dev`).
func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestGormStoreRoundTrip(t *testing.T) {
	db := openTestDB(t)
	store := NewGormStore(db)
	ctx := context.Background()
	const user = "__store_test__"
	t.Cleanup(func() { _ = store.Delete(ctx, user) })
	_ = store.Delete(ctx, user)

	if _, err := store.Get(ctx, user); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}

	def, _ := json.Marshal(Default())
	var sawNil bool
	if _, err := store.Update(ctx, user, func(cur json.RawMessage) (json.RawMessage, error) {
		sawNil = cur == nil
		return def, nil
	}); err != nil {
		t.Fatal(err)
	}
	if !sawNil {
		t.Error("first Update should see nil current")
	}

	// Second update sees the stored doc and can change it.
	_, err := store.Update(ctx, user, func(cur json.RawMessage) (json.RawMessage, error) {
		var d Doc
		if err := json.Unmarshal(cur, &d); err != nil {
			return nil, err
		}
		d["theme"] = "dark"
		return json.Marshal(d)
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := store.Get(ctx, user)
	if err != nil {
		t.Fatal(err)
	}
	var got Settings
	_ = json.Unmarshal(raw, &got)
	if got.Theme != "dark" {
		t.Errorf("theme = %s", got.Theme)
	}

	// The CHECK constraints in Postgres reject a wrong version even if the app misbehaves.
	_, err = store.Update(ctx, user, func(json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`{"schemaVersion":1,"featureFlags":[],"map":{},"filterPresets":[]}`), nil
	})
	if err == nil {
		t.Error("expected user_settings_version CHECK to reject schemaVersion 1")
	}

	// Errors from mutate abort the transaction and surface unchanged.
	sentinel := errors.New("boom")
	if _, err := store.Update(ctx, user, func(json.RawMessage) (json.RawMessage, error) { return nil, sentinel }); !errors.Is(err, sentinel) {
		t.Errorf("want sentinel, got %v", err)
	}
}

// TestEagerMatchesLazy is the contract between db/changelog/*.sql and
// migrate.go: the rows Liquibase seeded at v1 and migrated to the current
// version must be byte-for-byte (after canonicalisation) what the Go
// migrators produce from the same v1 input.
func TestEagerMatchesLazy(t *testing.T) {
	db := openTestDB(t)
	store := NewGormStore(db)
	ctx := context.Background()
	seed := loadFixture(t, "seed-v1.json")

	for user, v1 := range seed {
		raw, err := store.Get(ctx, user)
		if errors.Is(err, ErrNotFound) {
			t.Skipf("seed user %q not present; run liquibase update --contexts=dev", user)
		}
		if err != nil {
			t.Fatal(err)
		}
		var eager any
		_ = json.Unmarshal(raw, &eager)

		if _, err := Upgrade(v1); err != nil {
			t.Fatal(err)
		}
		lazy := canon(t, v1)
		if !reflect.DeepEqual(canon(t, eager), lazy) {
			eb, _ := json.MarshalIndent(eager, "", "  ")
			lb, _ := json.MarshalIndent(lazy, "", "  ")
			t.Errorf("%s: SQL migration and Go migration disagree\n sql: %s\n  go: %s", user, eb, lb)
		}
	}
}
