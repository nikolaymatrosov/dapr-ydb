package ydbstate

import "fmt"

// This file builds the YQL statements for the state operations. The table name is
// interpolated (YQL cannot bind identifiers as parameters); it comes from validated
// manifest configuration and is wrapped in backticks. All row data — key, value,
// etag — is always passed as bound parameters, never interpolated (research D6).
//
// Reads filter logically-expired rows regardless of native-TTL purge timing so an
// expired row is never returned before the engine reclaims it (research D5, FR-009).

// expiryFilter excludes rows whose expires_at is in the past. Rows this feature
// writes always have a NULL expires_at, but the filter is correct and future-proof.
const expiryFilter = "(expires_at IS NULL OR expires_at > CurrentUtcTimestamp())"

// getQuery selects the value and etag for a key, skipping logically-expired rows.
func getQuery(table string) string {
	return fmt.Sprintf(
		"DECLARE $key AS Utf8;\n"+
			"SELECT value, etag FROM `%s` WHERE key = $key AND %s;",
		table, expiryFilter,
	)
}

// etagQuery selects only the current etag for a key, skipping expired rows. Used
// inside the compare-and-set transaction (research D3).
func etagQuery(table string) string {
	return fmt.Sprintf(
		"DECLARE $key AS Utf8;\n"+
			"SELECT etag FROM `%s` WHERE key = $key AND %s;",
		table, expiryFilter,
	)
}

// upsertQuery creates or overwrites a row with a fresh value and etag.
func upsertQuery(table string) string {
	return fmt.Sprintf(
		"DECLARE $key AS Utf8;\n"+
			"DECLARE $value AS String;\n"+
			"DECLARE $etag AS Utf8;\n"+
			"UPSERT INTO `%s` (key, value, etag) VALUES ($key, $value, $etag);",
		table,
	)
}

// deleteQuery removes a row by key.
func deleteQuery(table string) string {
	return fmt.Sprintf(
		"DECLARE $key AS Utf8;\n"+
			"DELETE FROM `%s` WHERE key = $key;",
		table,
	)
}

// createTableQuery is the idempotent DDL matching the documented KV schema
// (specs/001-project-scaffold/data-model.md). Native row TTL on expires_at lets the
// engine reclaim expired rows; reads filter independently (research D2/D5).
func createTableQuery(table string) string {
	return fmt.Sprintf(
		"CREATE TABLE IF NOT EXISTS `%s` (\n"+
			"    key        Utf8,\n"+
			"    value      String,\n"+
			"    etag       Utf8,\n"+
			"    expires_at Timestamp,\n"+
			"    PRIMARY KEY (key)\n"+
			") WITH (\n"+
			"    TTL = Interval(\"PT0S\") ON expires_at\n"+
			");",
		table,
	)
}
