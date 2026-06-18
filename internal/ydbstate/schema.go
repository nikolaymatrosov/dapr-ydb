package ydbstate

import (
	"context"
	"fmt"

	"github.com/ydb-platform/ydb-go-sdk/v3/query"
)

// ensureTable creates the state table idempotently (CREATE TABLE IF NOT EXISTS) so
// the component never depends on a pre-existing schema (constitution: explicit,
// idempotent schema management). It is safe under concurrent initializers and
// restarts. DDL runs without an interactive transaction (query.NoTx).
func (s *YDBStore) ensureTable(ctx context.Context) error {
	if err := s.driver.Query().Exec(ctx,
		createTableQuery(s.md.TableName),
		query.WithTxControl(query.NoTx()),
	); err != nil {
		return fmt.Errorf("failed to ensure state table %q exists: %w", s.md.TableName, err)
	}
	return nil
}
