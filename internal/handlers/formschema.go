package handlers

import (
	"encoding/json"
	"fmt"
	"regexp"
)

// FormField is one input on the customer's submit page.
type FormField struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Type        string `json:"type"` // text | password | textarea
	Placeholder string `json:"placeholder,omitempty"`
}

const (
	maxFormFields  = 10
	maxLabelLen    = 80
	maxPlaceholder = 120
	maxFieldIDLen  = 32
)

var (
	fieldIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_\-]{1,32}$`)
	allowedTypes   = map[string]struct{}{
		"text": {}, "password": {}, "textarea": {},
	}
)

// ValidateFormSchema parses + sanitises a JSON-encoded schema. Returns the
// canonical re-serialised bytes (no trailing whitespace, fields in original
// order) or an error suitable for surfacing to the API client.
//
// Accepts an empty/null input and substitutes a default single-textarea
// schema named "secret".
func ValidateFormSchema(raw []byte) ([]byte, []FormField, error) {
	if len(raw) == 0 || string(raw) == "null" {
		def := []FormField{{ID: "secret", Label: "Geheimnis", Type: "textarea"}}
		out, _ := json.Marshal(def)
		return out, def, nil
	}
	var fields []FormField
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, nil, fmt.Errorf("form_schema is not a JSON array of fields: %w", err)
	}
	if len(fields) == 0 {
		return nil, nil, fmt.Errorf("form_schema must contain at least one field")
	}
	if len(fields) > maxFormFields {
		return nil, nil, fmt.Errorf("form_schema must have at most %d fields", maxFormFields)
	}
	seenID := make(map[string]struct{}, len(fields))
	for i, f := range fields {
		if !fieldIDPattern.MatchString(f.ID) {
			return nil, nil, fmt.Errorf("field %d: id must match %s", i, fieldIDPattern)
		}
		if _, dup := seenID[f.ID]; dup {
			return nil, nil, fmt.Errorf("field %d: duplicate id %q", i, f.ID)
		}
		seenID[f.ID] = struct{}{}
		if f.Label == "" || len(f.Label) > maxLabelLen {
			return nil, nil, fmt.Errorf("field %d: label required, max %d chars", i, maxLabelLen)
		}
		if _, ok := allowedTypes[f.Type]; !ok {
			return nil, nil, fmt.Errorf("field %d: type must be text|password|textarea", i)
		}
		if len(f.Placeholder) > maxPlaceholder {
			return nil, nil, fmt.Errorf("field %d: placeholder max %d chars", i, maxPlaceholder)
		}
	}
	out, err := json.Marshal(fields)
	if err != nil {
		return nil, nil, fmt.Errorf("re-marshal: %w", err)
	}
	return out, fields, nil
}
