package ydbstate

import (
	"strings"

	"github.com/dapr/components-contrib/state"

	"github.com/nikolaymatrosov/dapr-ydb/internal/ydbconfig"
)

const defaultTableName = "dapr_state"

// storeMetadata is the parsed, validated configuration for the YDB state store:
// the shared connection/auth config (constitution Principle V: manifest-only)
// plus the state-store-specific table name.
type storeMetadata struct {
	ydbconfig.Config
	TableName string
}

// parseAndValidateMetadata delegates connection/auth parsing to the shared
// ydbconfig package, then layers on the state-store-specific tableName. It
// returns a field-named error on any missing or invalid value and never panics.
func parseAndValidateMetadata(meta state.Metadata) (storeMetadata, error) {
	cfg, err := ydbconfig.Parse(meta.Properties)
	if err != nil {
		return storeMetadata{}, err
	}

	m := storeMetadata{Config: cfg, TableName: defaultTableName}
	if v := strings.TrimSpace(meta.Properties["tableName"]); v != "" {
		m.TableName = v
	}
	return m, nil
}
