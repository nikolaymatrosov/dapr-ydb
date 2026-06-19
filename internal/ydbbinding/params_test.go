package ydbbinding

import (
	"database/sql"
	"strings"
	"testing"
)

func TestBuildParams_Empty(t *testing.T) {
	args, err := buildParams("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if args != nil {
		t.Errorf("buildParams(\"\") = %v; want nil", args)
	}
}

// Positional: integral JSON numbers infer to int64 (so auto-declare picks Int64),
// fractional numbers stay float64 (Double), strings/bools pass through (FR-006a).
func TestBuildParams_PositionalInference(t *testing.T) {
	args, err := buildParams(`[42, 3.14, "alice", true, null]`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(args) != 5 {
		t.Fatalf("len(args) = %d; want 5", len(args))
	}
	if v, ok := args[0].(int64); !ok || v != 42 {
		t.Errorf("args[0] = %T(%v); want int64(42)", args[0], args[0])
	}
	if v, ok := args[1].(float64); !ok || v != 3.14 {
		t.Errorf("args[1] = %T(%v); want float64(3.14)", args[1], args[1])
	}
	if v, ok := args[2].(string); !ok || v != "alice" {
		t.Errorf("args[2] = %T(%v); want string(alice)", args[2], args[2])
	}
	if v, ok := args[3].(bool); !ok || v != true {
		t.Errorf("args[3] = %T(%v); want bool(true)", args[3], args[3])
	}
	if args[4] != nil {
		t.Errorf("args[4] = %v; want nil", args[4])
	}
}

// Named+typed: each entry becomes a sql.NamedArg whose name has the leading `$`
// stripped (the driver binds sql.Named("x") to `$x`).
func TestBuildParams_NamedTyped(t *testing.T) {
	args, err := buildParams(`{"$id": {"type": "Uint64", "value": 42}, "$name": {"type": "Utf8", "value": "bob"}}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(args) != 2 {
		t.Fatalf("len(args) = %d; want 2", len(args))
	}
	names := map[string]bool{}
	for _, a := range args {
		na, ok := a.(sql.NamedArg)
		if !ok {
			t.Fatalf("arg %T is not a sql.NamedArg", a)
		}
		if strings.HasPrefix(na.Name, "$") {
			t.Errorf("named arg %q must not retain the leading '$'", na.Name)
		}
		if na.Value == nil {
			t.Errorf("named arg %q has nil value", na.Name)
		}
		names[na.Name] = true
	}
	if !names["id"] || !names["name"] {
		t.Errorf("names = %v; want id and name", names)
	}
}

func TestBuildParams_UnknownType(t *testing.T) {
	_, err := buildParams(`{"$x": {"type": "Frobnicate", "value": 1}}`)
	if err == nil {
		t.Fatal("expected error for unknown type")
	}
	if !strings.Contains(err.Error(), "x") || !strings.Contains(err.Error(), "Frobnicate") {
		t.Errorf("error %q must name the parameter and the unknown type", err)
	}
}

func TestBuildParams_BadValueForType(t *testing.T) {
	_, err := buildParams(`{"$id": {"type": "Int64", "value": "not-a-number"}}`)
	if err == nil {
		t.Fatal("expected error for uncoercible value")
	}
	if !strings.Contains(err.Error(), "id") {
		t.Errorf("error %q must name the offending parameter 'id'", err)
	}
}

func TestBuildParams_MalformedJSON(t *testing.T) {
	if _, err := buildParams(`[1, 2`); err == nil {
		t.Error("expected error for malformed positional JSON")
	}
	if _, err := buildParams(`{"x": }`); err == nil {
		t.Error("expected error for malformed named JSON")
	}
}

func TestBuildParams_NotArrayOrObject(t *testing.T) {
	if _, err := buildParams(`"just a string"`); err == nil {
		t.Error("expected error for params that is neither array nor object")
	}
}

func TestBuildParams_TimestampAndBytes(t *testing.T) {
	// Timestamp parses RFC3339; String decodes base64 — both must succeed.
	if _, err := buildParams(`{"$t": {"type": "Timestamp", "value": "2026-06-18T00:00:00Z"}}`); err != nil {
		t.Errorf("Timestamp param: unexpected error: %v", err)
	}
	if _, err := buildParams(`{"$b": {"type": "String", "value": "aGk="}}`); err != nil {
		t.Errorf("String(base64) param: unexpected error: %v", err)
	}
	if _, err := buildParams(`{"$t": {"type": "Timestamp", "value": "not-a-time"}}`); err == nil {
		t.Error("expected error for invalid RFC3339 timestamp")
	}
}
