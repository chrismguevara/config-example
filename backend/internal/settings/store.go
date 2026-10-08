package settings

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrNotFound is returned when a user has no stored settings row.
var ErrNotFound = errors.New("settings not found")

// Store is the persistence boundary. The HTTP tests use an in-memory fake;
// production uses GormStore.
type Store interface {
	// Get returns the stored document or ErrNotFound.
	Get(ctx context.Context, userID string) (json.RawMessage, error)
	// Update runs mutate inside a transaction with the row locked
	// (SELECT ... FOR UPDATE), then upserts the result. mutate receives nil
	// when the user has no row yet. Returning an error from mutate aborts
	// the transaction and is passed through unchanged.
	Update(ctx context.Context, userID string, mutate func(current json.RawMessage) (json.RawMessage, error)) (json.RawMessage, error)
	// Delete removes the row (no error if absent).
	Delete(ctx context.Context, userID string) error
}

// JSONB maps a jsonb column to raw bytes without pulling in gorm.io/datatypes.
type JSONB json.RawMessage

// Value implements driver.Valuer.
func (j JSONB) Value() (driver.Value, error) {
	if len(j) == 0 {
		return nil, nil
	}
	return string(j), nil
}

// Scan implements sql.Scanner.
func (j *JSONB) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		*j = nil
	case []byte:
		*j = append((*j)[:0], v...)
	case string:
		*j = JSONB(v)
	default:
		return fmt.Errorf("cannot scan %T into JSONB", src)
	}
	return nil
}

// userSettingsRow is the GORM model for the user_settings table.
//
// The table is owned by Liquibase (db/changelog). GORM is used for data access
// only: there is deliberately no AutoMigrate call anywhere.
type userSettingsRow struct {
	UserID   string `gorm:"column:user_id;primaryKey"`
	Settings JSONB  `gorm:"column:settings;type:jsonb;not null"`
	// Generated column in Postgres; `->` makes it read-only for GORM.
	SchemaVersion int       `gorm:"column:schema_version;->"`
	CreatedAt     time.Time `gorm:"column:created_at"`
	UpdatedAt     time.Time `gorm:"column:updated_at"`
}

func (userSettingsRow) TableName() string { return "user_settings" }

// GormStore is the Postgres implementation of Store.
type GormStore struct{ db *gorm.DB }

// NewGormStore wraps an open *gorm.DB.
func NewGormStore(db *gorm.DB) *GormStore { return &GormStore{db: db} }

// Get implements Store.
func (s *GormStore) Get(ctx context.Context, userID string) (json.RawMessage, error) {
	var row userSettingsRow
	err := s.db.WithContext(ctx).Where("user_id = ?", userID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return json.RawMessage(row.Settings), nil
}

// Update implements Store. Read-modify-write under a row lock keeps two
// concurrent PATCHes for the same user from clobbering each other; the first
// write for a brand-new user races as a plain upsert (last writer wins),
// which is acceptable for settings.
func (s *GormStore) Update(ctx context.Context, userID string, mutate func(json.RawMessage) (json.RawMessage, error)) (json.RawMessage, error) {
	var result json.RawMessage
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row userSettingsRow
		res := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ?", userID).Limit(1).Find(&row)
		if res.Error != nil {
			return res.Error
		}
		var current json.RawMessage
		if res.RowsAffected == 1 {
			current = json.RawMessage(row.Settings)
		}
		next, err := mutate(current)
		if err != nil {
			return err
		}
		now := time.Now()
		upsert := userSettingsRow{UserID: userID, Settings: JSONB(next), CreatedAt: now, UpdatedAt: now}
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"settings", "updated_at"}),
		}).Create(&upsert).Error; err != nil {
			return err
		}
		result = next
		return nil
	})
	return result, err
}

// Delete implements Store.
func (s *GormStore) Delete(ctx context.Context, userID string) error {
	return s.db.WithContext(ctx).Where("user_id = ?", userID).Delete(&userSettingsRow{}).Error
}
