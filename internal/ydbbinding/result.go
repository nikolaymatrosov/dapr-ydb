package ydbbinding

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

// rowsToJSON serializes SQL result rows into a JSON array of row objects, one
// object per row keyed by column name. A result with no rows serializes to `[]`
// (never null), so an empty match is success, not an error (FR-010).
//
// Scan targets are *any, letting the YDB driver pick natural Go types. The JSON
// encoding then follows Go's encoding/json defaults, documented in the contract:
// NULL -> null, integers/floats -> number, bool -> boolean, []byte -> base64
// string, time.Time -> RFC3339 string.
func rowsToJSON(rows *sql.Rows) ([]byte, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("read columns: %w", err)
	}

	var collected [][]any
	for rows.Next() {
		cells := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range cells {
			ptrs[i] = &cells[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		collected = append(collected, cells)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}

	return encodeRows(cols, collected)
}

// encodeRows is the pure serialization step: it maps each row's cells to a
// column-keyed object and marshals the lot to JSON. A zero-length input yields
// `[]` (a non-nil empty slice), never `null`.
func encodeRows(cols []string, rows [][]any) ([]byte, error) {
	out := make([]map[string]any, 0, len(rows))
	for _, cells := range rows {
		row := make(map[string]any, len(cols))
		for i, name := range cols {
			row[name] = cells[i]
		}
		out = append(out, row)
	}

	data, err := json.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("marshal rows: %w", err)
	}
	return data, nil
}
