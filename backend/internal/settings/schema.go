package settings

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"golang.org/x/text/language"
	"golang.org/x/text/message"

	"github.com/chrismguevara/config-example/schemas"
)

// Validator holds the compiled JSON Schema for every document version.
type Validator struct {
	compiled map[int]*jsonschema.Schema
	raw      map[int][]byte
}

// NewValidator compiles schemas/user-settings.v1..v<CurrentVersion>.schema.json.
func NewValidator() (*Validator, error) {
	v := &Validator{compiled: map[int]*jsonschema.Schema{}, raw: map[int][]byte{}}
	c := jsonschema.NewCompiler()
	c.DefaultDraft(jsonschema.Draft2020)
	for version := 1; version <= CurrentVersion; version++ {
		name := schemas.FileName(version)
		b, err := schemas.FS.ReadFile(name)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(b))
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", name, err)
		}
		if err := c.AddResource(name, doc); err != nil {
			return nil, fmt.Errorf("add %s: %w", name, err)
		}
		sch, err := c.Compile(name)
		if err != nil {
			return nil, fmt.Errorf("compile %s: %w", name, err)
		}
		v.compiled[version] = sch
		v.raw[version] = b
	}
	return v, nil
}

// SchemaJSON returns the raw schema document for a version (nil if unknown).
// The HTTP layer serves this so the frontend can read enum lists at runtime.
func (v *Validator) SchemaJSON(version int) []byte { return v.raw[version] }

// Validate checks a decoded JSON value (from encoding/json) against the schema
// for the given version. It returns *ValidationError on schema violations.
func (v *Validator) Validate(version int, doc any) error {
	sch, ok := v.compiled[version]
	if !ok {
		return fmt.Errorf("no schema for settings version %d", version)
	}
	err := sch.Validate(doc)
	if err == nil {
		return nil
	}
	ve, ok := err.(*jsonschema.ValidationError)
	if !ok {
		return err
	}
	return flatten(version, ve)
}

// ValidationError is the API-friendly form of a schema violation.
type ValidationError struct {
	Version  int       `json:"schemaVersion"`
	Problems []Problem `json:"problems"`
}

// Problem is one violation: a JSON Pointer into the document and a message.
type Problem struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

func (e *ValidationError) Error() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "settings do not match schema v%d:", e.Version)
	for _, p := range e.Problems {
		fmt.Fprintf(&sb, " [%s] %s;", p.Path, p.Message)
	}
	return sb.String()
}

func flatten(version int, ve *jsonschema.ValidationError) *ValidationError {
	out := &ValidationError{Version: version}
	p := message.NewPrinter(language.English)
	var walk func(e *jsonschema.ValidationError)
	walk = func(e *jsonschema.ValidationError) {
		if len(e.Causes) == 0 {
			out.Problems = append(out.Problems, Problem{
				Path:    jsonPointer(e.InstanceLocation),
				Message: e.ErrorKind.LocalizedString(p),
			})
			return
		}
		for _, c := range e.Causes {
			walk(c)
		}
	}
	walk(ve)
	return out
}

// jsonPointer renders an instance location as an RFC 6901 pointer.
func jsonPointer(tokens []string) string {
	if len(tokens) == 0 {
		return "/"
	}
	var sb strings.Builder
	for _, t := range tokens {
		sb.WriteByte('/')
		sb.WriteString(strings.NewReplacer("~", "~0", "/", "~1").Replace(t))
	}
	return sb.String()
}

// decode parses raw JSON into the generic form the validator and migrators use.
func decode(raw []byte) (Doc, error) {
	var doc Doc
	dec := json.NewDecoder(bytes.NewReader(raw))
	if err := dec.Decode(&doc); err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, fmt.Errorf("settings document must be a JSON object")
	}
	return doc, nil
}

// encode produces canonical JSON: whatever the input (struct, map, raw bytes
// from Postgres with its own key order), the output has sorted keys and no
// insignificant whitespace, because encoding/json sorts map keys. This is what
// gets stored and what the ETag is computed over.
func encode(v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var generic any
	if err := json.Unmarshal(b, &generic); err != nil {
		return nil, err
	}
	return json.Marshal(generic)
}
