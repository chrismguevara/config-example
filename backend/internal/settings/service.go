package settings

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	jsonpatch "github.com/evanphx/json-patch/v5"
)

// Sentinel errors the HTTP layer maps to status codes.
var (
	ErrPreconditionFailed = errors.New("settings changed since they were read (ETag mismatch)")
	ErrBadRequest         = errors.New("bad request")
)

// Result is a current-version document plus its ETag.
type Result struct {
	Settings json.RawMessage
	ETag     string
}

// Service owns the rules: defaults, lazy upgrade, validation, ETags.
type Service struct {
	store Store
	v     *Validator
}

// NewService wires a Store and a Validator.
func NewService(store Store, v *Validator) *Service { return &Service{store: store, v: v} }

// Get returns the user's settings. A user with no row gets Default() (not
// persisted). A row at an old schema version is upgraded and written back
// (lazy migration) before being returned.
func (s *Service) Get(ctx context.Context, userID string) (Result, error) {
	raw, err := s.store.Get(ctx, userID)
	if errors.Is(err, ErrNotFound) {
		return s.result(Default())
	}
	if err != nil {
		return Result{}, err
	}
	doc, err := decode(raw)
	if err != nil {
		return Result{}, fmt.Errorf("stored settings for %q are not valid JSON: %w", userID, err)
	}
	if Version(doc) == CurrentVersion {
		return s.result(doc)
	}
	// Old version on disk: upgrade under the row lock and persist.
	next, err := s.store.Update(ctx, userID, func(current json.RawMessage) (json.RawMessage, error) {
		return s.loadCurrent(current)
	})
	if err != nil {
		return Result{}, err
	}
	return s.resultRaw(next)
}

// MergePatch applies an RFC 7386 JSON Merge Patch. ifMatch, when non-empty,
// must equal the ETag of the stored document or ErrPreconditionFailed is
// returned. schemaVersion is server-managed and may not appear in the patch.
func (s *Service) MergePatch(ctx context.Context, userID string, patch json.RawMessage, ifMatch string) (Result, error) {
	var probe map[string]any
	if err := json.Unmarshal(patch, &probe); err != nil || probe == nil {
		return Result{}, fmt.Errorf("%w: body must be a JSON object (RFC 7386 merge patch)", ErrBadRequest)
	}
	if _, has := probe["schemaVersion"]; has {
		return Result{}, fmt.Errorf("%w: schemaVersion is managed by the server", ErrBadRequest)
	}
	next, err := s.store.Update(ctx, userID, func(current json.RawMessage) (json.RawMessage, error) {
		base, err := s.loadCurrent(current)
		if err != nil {
			return nil, err
		}
		if ifMatch != "" && !etagMatches(ifMatch, etag(base)) {
			return nil, ErrPreconditionFailed
		}
		merged, err := jsonpatch.MergePatch(base, patch)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrBadRequest, err)
		}
		return s.validateRaw(merged)
	})
	if err != nil {
		return Result{}, err
	}
	return s.resultRaw(next)
}

// Mutate applies a typed change (the per-path endpoints) under the row lock.
func (s *Service) Mutate(ctx context.Context, userID string, fn func(*Settings) error) (Result, error) {
	next, err := s.store.Update(ctx, userID, func(current json.RawMessage) (json.RawMessage, error) {
		raw, err := s.loadCurrent(current)
		if err != nil {
			return nil, err
		}
		var st Settings
		if err := json.Unmarshal(raw, &st); err != nil {
			return nil, err
		}
		if err := fn(&st); err != nil {
			return nil, err
		}
		st.normalize()
		b, err := encode(st)
		if err != nil {
			return nil, err
		}
		return s.validateRaw(b)
	})
	if err != nil {
		return Result{}, err
	}
	return s.resultRaw(next)
}

// Reset deletes the row so the user is back on Default().
func (s *Service) Reset(ctx context.Context, userID string) (Result, error) {
	if err := s.store.Delete(ctx, userID); err != nil {
		return Result{}, err
	}
	return s.result(Default())
}

// --- typed mutations used by the per-path endpoints ------------------------

func SetTheme(theme string) func(*Settings) error {
	return func(s *Settings) error { s.Theme = theme; return nil }
}

func SetFlag(flag string, on bool) func(*Settings) error {
	return func(s *Settings) error {
		if on {
			s.FeatureFlags = addUnique(s.FeatureFlags, flag)
		} else {
			s.FeatureFlags = removeAll(s.FeatureFlags, flag)
		}
		return nil
	}
}

func SetBaseLayer(layer string) func(*Settings) error {
	return func(s *Settings) error { s.Map.BaseLayer = layer; return nil }
}

func SetLayer(layer string, on bool) func(*Settings) error {
	return func(s *Settings) error {
		if on {
			s.Map.Layers = addUnique(s.Map.Layers, layer)
		} else {
			s.Map.Layers = removeAll(s.Map.Layers, layer)
		}
		return nil
	}
}

// UpsertPreset replaces the preset with the same id or appends it.
func UpsertPreset(p FilterPreset) func(*Settings) error {
	return func(s *Settings) error {
		for i := range s.FilterPresets {
			if s.FilterPresets[i].ID == p.ID {
				s.FilterPresets[i] = p
				return nil
			}
		}
		s.FilterPresets = append(s.FilterPresets, p)
		return nil
	}
}

// DeletePreset removes a preset by id (no error if absent).
func DeletePreset(id string) func(*Settings) error {
	return func(s *Settings) error {
		out := s.FilterPresets[:0]
		for _, p := range s.FilterPresets {
			if p.ID != id {
				out = append(out, p)
			}
		}
		s.FilterPresets = out
		return nil
	}
}

// --- internals ---------------------------------------------------------------

// loadCurrent turns a stored document (or nil) into canonical current-version
// bytes: defaults for a missing row, lazy Upgrade for an old row.
func (s *Service) loadCurrent(current json.RawMessage) (json.RawMessage, error) {
	if current == nil {
		return encode(Default())
	}
	doc, err := decode(current)
	if err != nil {
		return nil, fmt.Errorf("stored settings are not valid JSON: %w", err)
	}
	if _, err := Upgrade(doc); err != nil {
		return nil, err
	}
	if err := s.v.Validate(CurrentVersion, doc); err != nil {
		return nil, fmt.Errorf("stored settings failed validation after upgrade: %w", err)
	}
	return encode(doc)
}

// validateRaw decodes, validates against the current schema, re-encodes.
func (s *Service) validateRaw(raw []byte) (json.RawMessage, error) {
	doc, err := decode(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadRequest, err)
	}
	if err := s.v.Validate(CurrentVersion, doc); err != nil {
		return nil, err
	}
	return encode(doc)
}

func (s *Service) result(v any) (Result, error) {
	b, err := encode(v)
	if err != nil {
		return Result{}, err
	}
	return s.resultRaw(b)
}

func (s *Service) resultRaw(raw json.RawMessage) (Result, error) {
	// Re-canonicalise (Postgres may have reordered keys) so the ETag is stable.
	doc, err := decode(raw)
	if err != nil {
		return Result{}, err
	}
	b, err := encode(doc)
	if err != nil {
		return Result{}, err
	}
	return Result{Settings: b, ETag: etag(b)}, nil
}

// etag is a strong ETag over the canonical bytes. No version column needed.
func etag(canonical []byte) string {
	sum := sha256.Sum256(canonical)
	return `"` + hex.EncodeToString(sum[:16]) + `"`
}

func etagMatches(header, current string) bool {
	for _, candidate := range strings.Split(header, ",") {
		c := strings.TrimSpace(candidate)
		c = strings.TrimPrefix(c, "W/")
		if c == "*" || c == current {
			return true
		}
	}
	return false
}
