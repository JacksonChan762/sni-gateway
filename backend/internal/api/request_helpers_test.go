package api

import (
	"encoding/json"
	"testing"
)

func TestParseOptionalUintField(t *testing.T) {
	tests := []struct {
		name      string
		fields    map[string]json.RawMessage
		present   bool
		value     *uint
		wantError bool
	}{
		{
			name:    "missing field",
			fields:  map[string]json.RawMessage{},
			present: false,
		},
		{
			name:    "null field clears binding",
			fields:  map[string]json.RawMessage{"exit_id": json.RawMessage("null")},
			present: true,
			value:   nil,
		},
		{
			name:    "valid id",
			fields:  map[string]json.RawMessage{"exit_id": json.RawMessage("12")},
			present: true,
			value:   uintPtr(12),
		},
		{
			name:      "zero rejected",
			fields:    map[string]json.RawMessage{"exit_id": json.RawMessage("0")},
			present:   true,
			wantError: true,
		},
		{
			name:      "invalid type rejected",
			fields:    map[string]json.RawMessage{"exit_id": json.RawMessage(`"abc"`)},
			present:   true,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			present, value, err := parseOptionalUintField(tt.fields, "exit_id")
			if (err != nil) != tt.wantError {
				t.Fatalf("unexpected error state: %v", err)
			}
			if present != tt.present {
				t.Fatalf("present = %v, want %v", present, tt.present)
			}
			if !sameUintPtr(value, tt.value) {
				t.Fatalf("value = %v, want %v", value, tt.value)
			}
		})
	}
}

func uintPtr(v uint) *uint {
	return &v
}

func sameUintPtr(a, b *uint) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
