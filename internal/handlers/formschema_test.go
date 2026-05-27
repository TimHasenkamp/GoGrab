package handlers

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateFormSchema_DefaultsWhenEmpty(t *testing.T) {
	cases := [][]byte{nil, []byte(""), []byte("null")}
	for _, in := range cases {
		out, fields, err := ValidateFormSchema(in)
		if err != nil {
			t.Fatalf("unexpected err for input %q: %v", in, err)
		}
		if len(fields) != 1 || fields[0].ID != "secret" || fields[0].Type != "textarea" {
			t.Fatalf("default schema not applied: got %+v", fields)
		}
		if !strings.Contains(string(out), `"id":"secret"`) {
			t.Errorf("re-marshalled default missing id: %s", out)
		}
	}
}

func TestValidateFormSchema_RoundtripsValidSchema(t *testing.T) {
	in := []byte(`[
		{"id":"wifi_name","label":"WLAN-Name","type":"text"},
		{"id":"wifi_pw","label":"Passwort","type":"password","placeholder":"min 8 chars"}
	]`)
	out, fields, err := ValidateFormSchema(in)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if len(fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(fields))
	}
	// Re-parse the canonical output to verify it round-trips.
	var reparsed []FormField
	if err := json.Unmarshal(out, &reparsed); err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	if reparsed[1].Placeholder != "min 8 chars" {
		t.Errorf("placeholder lost: %+v", reparsed[1])
	}
}

func TestValidateFormSchema_Rejects(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantSub string
	}{
		{"not json array", `{"foo":"bar"}`, "JSON array"},
		{"empty array", `[]`, "at least one"},
		{"bad id", `[{"id":"with space","label":"x","type":"text"}]`, "id must match"},
		{"empty label", `[{"id":"x","label":"","type":"text"}]`, "label required"},
		{"bad type", `[{"id":"x","label":"x","type":"checkbox"}]`, "type must be"},
		{"dup id", `[{"id":"a","label":"x","type":"text"},{"id":"a","label":"y","type":"text"}]`, "duplicate id"},
		{
			"too many fields",
			func() string {
				var fields []string
				for i := 0; i < 11; i++ {
					fields = append(fields, `{"id":"f`+string(rune('a'+i))+`","label":"x","type":"text"}`)
				}
				return "[" + strings.Join(fields, ",") + "]"
			}(),
			"at most",
		},
		{
			"label too long",
			`[{"id":"x","label":"` + strings.Repeat("a", maxLabelLen+1) + `","type":"text"}]`,
			"label required",
		},
		{
			"placeholder too long",
			`[{"id":"x","label":"ok","type":"text","placeholder":"` + strings.Repeat("p", maxPlaceholder+1) + `"}]`,
			"placeholder max",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := ValidateFormSchema([]byte(tc.input))
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantSub)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("err = %q, want containing %q", err.Error(), tc.wantSub)
			}
		})
	}
}
