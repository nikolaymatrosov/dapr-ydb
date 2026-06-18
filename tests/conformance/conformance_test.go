//go:build conformance

// Package conformance wires the Dapr state conformance suite to the YDB store.
// It is gated behind the `conformance` build tag and run via `make conformance`,
// which brings up a local YDB instance first.
//
// The store implements Get/Set/Delete (incl. bulk via the default bulk store) and
// optimistic-concurrency ETag semantics. The basic CRUD scenarios always run; the
// "etag" operation enables the optimistic-concurrency scenarios. As later features
// land (TTL, transactions, query), add their operation key to `operations`.
package conformance

import (
	"context"
	"os"
	"testing"

	"github.com/dapr/components-contrib/metadata"
	"github.com/dapr/components-contrib/state"
	conf "github.com/dapr/components-contrib/tests/conformance/state"

	"github.com/nikolaymatrosov/dapr-ydb/internal/ydbstate"
)

func TestYDBStateConformance(t *testing.T) {
	connStr := os.Getenv("YDB_CONNECTION_STRING")
	if connStr == "" {
		connStr = "grpc://localhost:2136/local"
	}
	props := map[string]string{
		"connectionString": connStr,
		"authMethod":       "anonymous",
	}

	store := ydbstate.New()
	if err := store.Init(context.Background(), state.Metadata{Base: metadata.Base{Properties: props}}); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer func() { _ = store.Close() }()

	// Basic CRUD scenarios run unconditionally; "etag" enables the optimistic-
	// concurrency scenarios that gate advertising state.FeatureETag.
	operations := []string{"etag"}
	cfg, err := conf.NewTestConfig("ydb", operations, map[string]interface{}{})
	if err != nil {
		t.Fatalf("NewTestConfig: %v", err)
	}
	conf.ConformanceTests(t, props, store, cfg)
}
