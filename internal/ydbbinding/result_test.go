package ydbbinding

import (
	"testing"
	"time"
)

// An empty result set serializes to `[]`, not `null` or an error (FR-010).
func TestEncodeRows_EmptyIsEmptyArray(t *testing.T) {
	data, err := encodeRows([]string{"id", "name"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "[]" {
		t.Errorf("encodeRows(empty) = %q; want %q", data, "[]")
	}
}

func TestEncodeRows_ColumnKeyedObjects(t *testing.T) {
	data, err := encodeRows(
		[]string{"id", "name", "active"},
		[][]any{
			{int64(1), "alice", true},
			{int64(2), "bob", false},
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := `[{"active":true,"id":1,"name":"alice"},{"active":false,"id":2,"name":"bob"}]`
	if string(data) != want {
		t.Errorf("encodeRows = %s; want %s", data, want)
	}
}

func TestEncodeRows_NullAndTypes(t *testing.T) {
	ts := time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)
	data, err := encodeRows(
		[]string{"n", "f", "b", "bytes", "ts", "nilcol"},
		[][]any{
			{int64(7), 3.5, true, []byte("hi"), ts, nil},
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// []byte marshals to base64 ("hi" => "aGk="); time.Time to RFC3339; nil to null.
	want := `[{"b":true,"bytes":"aGk=","f":3.5,"n":7,"nilcol":null,"ts":"2026-06-18T12:00:00Z"}]`
	if string(data) != want {
		t.Errorf("encodeRows = %s;\n want %s", data, want)
	}
}
