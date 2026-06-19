// Package ydbbinding implements a Dapr pluggable output binding backed by YDB
// (Yandex Database). It exposes two operations over raw YQL — `query` (returns
// result rows as JSON) and `exec` (runs DML/DDL and reports a summary) — mirroring
// the contract of the reference dapr/components-contrib bindings/postgres binding.
//
// The binding is served by the same plugin binary as the state.ydb state store
// and reuses the shared connection/auth configuration (internal/ydbconfig). It
// executes statements through the YDB database/sql layer with positional-argument
// rewriting and automatic type declaration, so postgres-style `?` parameters work
// with inferred types; a named+typed parameter form is the escape hatch for exact
// YDB types. Per the constitution, Operations() advertises only what is implemented.
package ydbbinding

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/dapr/components-contrib/bindings"
	ydb "github.com/ydb-platform/ydb-go-sdk/v3"

	"github.com/nikolaymatrosov/dapr-ydb/internal/ydbconfig"
)

// Operation kinds this binding implements.
const (
	queryOperation bindings.OperationKind = "query"
	execOperation  bindings.OperationKind = "exec"
)

// Request metadata field names (aligned with the contrib postgres binding).
const (
	metaSQL       = "sql"
	metaParams    = "params"
	metaQueryMode = "queryMode"
)

// YDBBinding is the pluggable Dapr output binding backed by YDB.
type YDBBinding struct {
	logger *slog.Logger
	cfg    ydbconfig.Config
	driver *ydb.Driver
	db     *sql.DB
}

// Compile-time assertion that YDBBinding satisfies the Dapr output-binding contract.
var _ bindings.OutputBinding = (*YDBBinding)(nil)

// New constructs an uninitialized YDBBinding.
func New() *YDBBinding {
	return &YDBBinding{
		logger: slog.New(slog.NewTextHandler(os.Stderr, nil)).With("component", "bindings.ydb"),
	}
}

// Init parses and validates the manifest metadata (shared with the state store),
// opens the YDB driver, and derives a database/sql handle whose connector rewrites
// positional `?` placeholders into typed YQL parameters (auto-declared). It returns
// a field-named error on bad configuration and never panics (constitution Principle V).
func (b *YDBBinding) Init(ctx context.Context, meta bindings.Metadata) error {
	cfg, err := ydbconfig.Parse(meta.Properties)
	if err != nil {
		return err
	}
	b.cfg = cfg

	driver, err := ydbconfig.Open(ctx, cfg)
	if err != nil {
		return err
	}
	b.driver = driver

	connector, err := ydb.Connector(driver,
		ydb.WithAutoDeclare(),
		ydb.WithPositionalArgs(),
	)
	if err != nil {
		_ = b.driver.Close(ctx)
		b.driver = nil
		return fmt.Errorf("failed to create YDB database/sql connector: %w", err)
	}
	b.db = sql.OpenDB(connector)

	b.logger.Info("YDB output binding initialized", "database", driver.Name())
	return nil
}

// Operations advertises exactly the operations this binding implements
// (constitution Principle I analogue: never advertise unimplemented behavior).
func (b *YDBBinding) Operations() []bindings.OperationKind {
	return []bindings.OperationKind{queryOperation, execOperation}
}

// Invoke dispatches by operation kind. It validates the request before any database
// call and maps every database/input error to a returned error — it never panics or
// takes down the host process or the co-resident state store (FR-008, FR-011).
func (b *YDBBinding) Invoke(ctx context.Context, req *bindings.InvokeRequest) (*bindings.InvokeResponse, error) {
	switch req.Operation {
	case queryOperation:
		return b.query(ctx, req)
	case execOperation:
		return b.exec(ctx, req)
	default:
		return nil, fmt.Errorf("operation %q not supported by the YDB binding (supported: %q, %q)",
			req.Operation, queryOperation, execOperation)
	}
}

// query runs a row-returning statement and serializes the rows as a JSON array of
// column-keyed objects (FR-003, FR-010).
func (b *YDBBinding) query(ctx context.Context, req *bindings.InvokeRequest) (*bindings.InvokeResponse, error) {
	stmt, err := requireSQL(req)
	if err != nil {
		return nil, err
	}
	args, err := buildParams(req.Metadata[metaParams])
	if err != nil {
		return nil, err
	}

	start := time.Now()
	rows, err := b.db.QueryContext(ctx, stmt, args...)
	if err != nil {
		return nil, fmt.Errorf("ydbbinding: query failed: %w", err)
	}
	defer func() { _ = rows.Close() }()

	data, err := rowsToJSON(rows)
	if err != nil {
		return nil, fmt.Errorf("ydbbinding: serialize query result: %w", err)
	}
	end := time.Now()

	return &bindings.InvokeResponse{
		Data:        data,
		ContentType: contentTypeJSON(),
		Metadata:    summaryMetadata(queryOperation, stmt, start, end, ""),
	}, nil
}

// exec runs a non-row-returning statement (DML or DDL) and reports an execution
// summary. DDL requires scheme query mode, selectable via the queryMode metadata
// field (FR-004).
func (b *YDBBinding) exec(ctx context.Context, req *bindings.InvokeRequest) (*bindings.InvokeResponse, error) {
	stmt, err := requireSQL(req)
	if err != nil {
		return nil, err
	}
	args, err := buildParams(req.Metadata[metaParams])
	if err != nil {
		return nil, err
	}
	execCtx, err := withQueryMode(ctx, req.Metadata[metaQueryMode])
	if err != nil {
		return nil, err
	}

	start := time.Now()
	res, err := b.db.ExecContext(execCtx, stmt, args...)
	if err != nil {
		return nil, fmt.Errorf("ydbbinding: exec failed: %w", err)
	}
	end := time.Now()

	rowsAffected := "unknown"
	if n, raErr := res.RowsAffected(); raErr == nil {
		rowsAffected = fmt.Sprintf("%d", n)
	}

	return &bindings.InvokeResponse{
		Metadata: summaryMetadata(execOperation, stmt, start, end, rowsAffected),
	}, nil
}

// Close releases the database/sql handle and the underlying YDB driver. It is safe
// to call when Init never opened a connection.
func (b *YDBBinding) Close() error {
	if b.db != nil {
		_ = b.db.Close()
		b.db = nil
	}
	if b.driver != nil {
		err := b.driver.Close(context.Background())
		b.driver = nil
		return err
	}
	return nil
}

// Ping verifies connectivity for health checking (optional health.Pinger).
func (b *YDBBinding) Ping(ctx context.Context) error {
	if b.db == nil {
		return fmt.Errorf("ydbbinding: not initialized")
	}
	return b.db.PingContext(ctx)
}

// GetComponentMetadata satisfies the contrib metadata.ComponentWithMetadata
// interface required by the output-binding contract.
func (b *YDBBinding) GetComponentMetadata() map[string]string {
	return map[string]string{}
}

// requireSQL returns the trimmed statement from request metadata, or a descriptive
// error (with no database call) when it is missing (FR-011).
func requireSQL(req *bindings.InvokeRequest) (string, error) {
	stmt := strings.TrimSpace(req.Metadata[metaSQL])
	if stmt == "" {
		return "", fmt.Errorf("ydbbinding: required metadata field %q (the YQL statement) is missing", metaSQL)
	}
	return stmt, nil
}

// withQueryMode returns a context carrying the requested YDB query mode for exec.
// An empty value keeps the default (data) mode; DDL needs "scheme".
func withQueryMode(ctx context.Context, mode string) (context.Context, error) {
	switch strings.TrimSpace(mode) {
	case "", "data":
		return ctx, nil
	case "scheme":
		return ydb.WithQueryMode(ctx, ydb.SchemeQueryMode), nil
	case "scripting":
		return ydb.WithQueryMode(ctx, ydb.ScriptingQueryMode), nil
	default:
		return nil, fmt.Errorf("ydbbinding: invalid %q %q (expected one of: data, scheme, scripting)", metaQueryMode, mode)
	}
}

// summaryMetadata builds the response metadata common to both operations. The
// rowsAffected entry is included only for exec (empty string => omitted).
func summaryMetadata(op bindings.OperationKind, stmt string, start, end time.Time, rowsAffected string) map[string]string {
	m := map[string]string{
		"operation":  string(op),
		"sql":        stmt,
		"start-time": start.Format(time.RFC3339Nano),
		"end-time":   end.Format(time.RFC3339Nano),
		"duration":   end.Sub(start).String(),
	}
	if rowsAffected != "" {
		m["rows-affected"] = rowsAffected
	}
	return m
}

func contentTypeJSON() *string {
	ct := "application/json"
	return &ct
}
