//go:build integration

// Package integration exercises the YDB output binding against a real YDB
// instance. It is gated behind the `integration` build tag and run via
// `make binding-integration`, which brings up a local YDB first. This is the
// authoritative correctness gate for the binding (constitution Principle II
// analogue for components the state conformance suite does not cover; FR-014).
package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/dapr/components-contrib/bindings"
	"github.com/dapr/components-contrib/metadata"

	"github.com/nikolaymatrosov/dapr-ydb/internal/ydbbinding"
)

const testTable = "dapr_binding_it"

func newBinding(t *testing.T) *ydbbinding.YDBBinding {
	t.Helper()
	connStr := os.Getenv("YDB_CONNECTION_STRING")
	if connStr == "" {
		connStr = "grpc://localhost:2136/local"
	}
	b := ydbbinding.New()
	if err := b.Init(context.Background(), bindings.Metadata{Base: metadata.Base{Properties: map[string]string{
		"connectionString": connStr,
		"authMethod":       "anonymous",
	}}}); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	return b
}

func invoke(t *testing.T, b *ydbbinding.YDBBinding, op bindings.OperationKind, md map[string]string) (*bindings.InvokeResponse, error) {
	t.Helper()
	return b.Invoke(context.Background(), &bindings.InvokeRequest{Operation: op, Metadata: md})
}

func mustExec(t *testing.T, b *ydbbinding.YDBBinding, md map[string]string) {
	t.Helper()
	if _, err := invoke(t, b, "exec", md); err != nil {
		t.Fatalf("exec %v failed: %v", md, err)
	}
}

func TestBinding_EndToEnd(t *testing.T) {
	b := newBinding(t)
	defer func() { _ = b.Close() }()

	// US2: DDL via exec + scheme query mode.
	mustExec(t, b, map[string]string{
		"sql":       fmt.Sprintf("DROP TABLE IF EXISTS %s", testTable),
		"queryMode": "scheme",
	})
	mustExec(t, b, map[string]string{
		"sql":       fmt.Sprintf("CREATE TABLE %s (id Int64, name Utf8, active Bool, PRIMARY KEY (id))", testTable),
		"queryMode": "scheme",
	})
	defer mustExec(t, b, map[string]string{
		"sql":       fmt.Sprintf("DROP TABLE IF EXISTS %s", testTable),
		"queryMode": "scheme",
	})

	// US3: exec with positional params (DML).
	mustExec(t, b, map[string]string{
		"sql":    fmt.Sprintf("UPSERT INTO %s (id, name, active) VALUES (?, ?, ?)", testTable),
		"params": `[1, "alice", true]`,
	})
	mustExec(t, b, map[string]string{
		"sql":    fmt.Sprintf("UPSERT INTO %s (id, name, active) VALUES (?, ?, ?)", testTable),
		"params": `[2, "bob", false]`,
	})

	// US1: query with a positional parameter returns the matching row as JSON.
	t.Run("query positional", func(t *testing.T) {
		resp, err := invoke(t, b, "query", map[string]string{
			"sql":    fmt.Sprintf("SELECT id, name FROM %s WHERE id = ?", testTable),
			"params": `[1]`,
		})
		if err != nil {
			t.Fatalf("query failed: %v", err)
		}
		rows := decodeRows(t, resp.Data)
		if len(rows) != 1 || fmt.Sprint(rows[0]["name"]) != "alice" {
			t.Fatalf("query result = %v; want one row with name=alice", rows)
		}
	})

	// US1: a query matching no rows returns an empty array, not an error (FR-010).
	t.Run("query zero rows", func(t *testing.T) {
		resp, err := invoke(t, b, "query", map[string]string{
			"sql":    fmt.Sprintf("SELECT id FROM %s WHERE id = ?", testTable),
			"params": `[9999]`,
		})
		if err != nil {
			t.Fatalf("query failed: %v", err)
		}
		if string(resp.Data) != "[]" {
			t.Fatalf("zero-row query data = %s; want []", resp.Data)
		}
	})

	// US3: named+typed parameter forces an exact YDB type.
	t.Run("query named typed", func(t *testing.T) {
		resp, err := invoke(t, b, "query", map[string]string{
			"sql":    "SELECT CAST($n AS Uint64) AS n",
			"params": `{"$n": {"type": "Uint64", "value": 42}}`,
		})
		if err != nil {
			t.Fatalf("named-typed query failed: %v", err)
		}
		rows := decodeRows(t, resp.Data)
		if len(rows) != 1 || fmt.Sprint(rows[0]["n"]) != "42" {
			t.Fatalf("named-typed result = %v; want n=42", rows)
		}
	})

	// US3 / SC-005: a value containing YQL-like text is bound as data, not code.
	t.Run("injection treated as data", func(t *testing.T) {
		const evil = "alice'); DROP TABLE " + testTable + "; --"
		resp, err := invoke(t, b, "query", map[string]string{
			"sql":    fmt.Sprintf("SELECT id FROM %s WHERE name = ?", testTable),
			"params": fmt.Sprintf(`[%q]`, evil),
		})
		if err != nil {
			t.Fatalf("query failed: %v", err)
		}
		if string(resp.Data) != "[]" {
			t.Fatalf("injection value matched rows (%s); it must be treated as data", resp.Data)
		}
		// The table must still exist and hold its rows.
		resp2, err := invoke(t, b, "query", map[string]string{"sql": fmt.Sprintf("SELECT id FROM %s", testTable)})
		if err != nil {
			t.Fatalf("post-injection query failed (table dropped?): %v", err)
		}
		if rows := decodeRows(t, resp2.Data); len(rows) != 2 {
			t.Fatalf("table row count = %d; want 2 (injection must not have altered the table)", len(rows))
		}
	})
}

func TestBinding_Errors(t *testing.T) {
	b := newBinding(t)
	defer func() { _ = b.Close() }()

	t.Run("unknown operation", func(t *testing.T) {
		if _, err := invoke(t, b, "get", map[string]string{"sql": "SELECT 1"}); err == nil {
			t.Fatal("expected error for unsupported operation")
		}
	})
	t.Run("missing statement", func(t *testing.T) {
		if _, err := invoke(t, b, "query", map[string]string{}); err == nil {
			t.Fatal("expected error for missing sql")
		}
	})
	t.Run("invalid yql survives", func(t *testing.T) {
		if _, err := invoke(t, b, "query", map[string]string{"sql": "NOT VALID YQL"}); err == nil {
			t.Fatal("expected error for invalid YQL")
		}
		// Component must still serve a valid request afterwards (FR-008).
		if _, err := invoke(t, b, "query", map[string]string{"sql": "SELECT 1 AS one"}); err != nil {
			t.Fatalf("binding unusable after an invalid statement: %v", err)
		}
	})
}

func decodeRows(t *testing.T, data []byte) []map[string]any {
	t.Helper()
	var rows []map[string]any
	if err := json.Unmarshal(data, &rows); err != nil {
		t.Fatalf("response data is not a JSON array of objects: %v (data=%s)", err, data)
	}
	return rows
}
