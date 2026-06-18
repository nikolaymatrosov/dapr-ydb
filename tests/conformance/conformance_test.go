//go:build conformance

// Package conformance wires the Dapr state conformance suite to the YDB store.
// It is gated behind the `conformance` build tag and run via `make conformance`,
// which brings up a local YDB instance first.
//
// The scaffold implements no persistence operations and advertises no Features(),
// so this harness compiles and is fully wired but skips the assertions. As each
// later feature lands (Get/Set/Delete -> bulk -> ETag -> TTL -> transactions),
// add its operation to `operations` and remove the skip.
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

	// Scaffold baseline: no operations implemented yet, so there is nothing for
	// the suite to assert. Later features populate `operations` and drop this skip.
	t.Skip("scaffold: no state operations implemented yet; conformance coverage grows per feature")

	store := ydbstate.New()
	if err := store.Init(context.Background(), state.Metadata{Base: metadata.Base{Properties: props}}); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer func() { _ = store.Close() }()

	var operations []string // grows as features are implemented and advertised
	cfg, err := conf.NewTestConfig("ydb", operations, map[string]interface{}{})
	if err != nil {
		t.Fatalf("NewTestConfig: %v", err)
	}
	conf.ConformanceTests(t, props, store, cfg)
}
