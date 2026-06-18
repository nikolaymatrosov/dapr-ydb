// Package ydbstate implements a Dapr pluggable state store backed by YDB
// (Yandex Database). It satisfies github.com/dapr/components-contrib/state.Store.
//
// The component loads in Dapr, parses and validates its manifest configuration,
// opens a YDB connection, and ensures the state table exists. It implements real
// Get/Set/Delete against the documented KV schema with optimistic-concurrency
// (ETag) semantics. Per constitution Principle I, Features() advertises only
// capabilities that have been conformance-verified.
package ydbstate

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/dapr/components-contrib/state"
	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
)

// YDBStore is the pluggable Dapr state store backed by YDB.
type YDBStore struct {
	// BulkStore delegates BulkGet/BulkSet/BulkDelete to the single-key
	// Get/Set/Delete methods, mirroring the contrib Postgres/Mongo components.
	state.BulkStore

	logger *slog.Logger
	md     storeMetadata
	driver *ydb.Driver
}

// Compile-time assertion that YDBStore satisfies the Dapr state.Store contract.
var _ state.Store = (*YDBStore)(nil)

// New constructs a YDBStore with bulk operations delegated to the default bulk store.
func New() *YDBStore {
	s := &YDBStore{
		logger: slog.New(slog.NewTextHandler(os.Stderr, nil)).With("component", "state.ydb"),
	}
	s.BulkStore = state.NewDefaultBulkStore(s)
	return s
}

// Init parses and validates the manifest metadata, then opens the YDB connection.
// It returns a field-named error on bad configuration and never panics
// (constitution Principle V). The host sidecar is unaffected by a failed Init.
func (s *YDBStore) Init(ctx context.Context, meta state.Metadata) error {
	m, err := parseAndValidateMetadata(meta)
	if err != nil {
		return err
	}
	s.md = m

	opts, err := s.credentialOptions()
	if err != nil {
		return err
	}

	driver, err := ydb.Open(ctx, m.ConnectionString, opts...)
	if err != nil {
		return fmt.Errorf("failed to open YDB connection: %w", err)
	}
	s.driver = driver

	if err := s.ensureTable(ctx); err != nil {
		// Release the just-opened driver so a failed Init leaves no resources.
		_ = s.driver.Close(ctx)
		s.driver = nil
		return err
	}

	s.logger.Info("YDB state store initialized", "database", driver.Name(), "table", m.TableName)
	return nil
}

// credentialOptions maps the configured authMethod to a YDB driver option.
func (s *YDBStore) credentialOptions() ([]ydb.Option, error) {
	switch s.md.AuthMethod {
	case authAnonymous:
		return []ydb.Option{ydb.WithAnonymousCredentials()}, nil
	case authStatic:
		return []ydb.Option{ydb.WithStaticCredentials(s.md.Username, s.md.Password)}, nil
	case authToken:
		return []ydb.Option{ydb.WithAccessTokenCredentials(s.md.AccessToken)}, nil
	case authSAKey:
		return nil, fmt.Errorf("authMethod %q is not yet supported in this build (requires the ydb-go-yc integration, a later feature)", s.md.AuthMethod)
	case authMetadata:
		return nil, fmt.Errorf("authMethod %q is not yet supported in this build (requires the ydb-go-yc-metadata integration, a later feature)", s.md.AuthMethod)
	default:
		return nil, fmt.Errorf("unsupported authMethod %q", s.md.AuthMethod)
	}
}

// Features advertises the capabilities this component implements. ETag optimistic
// concurrency is implemented and verified by the Dapr conformance eTag scenarios
// (which assert both that FeatureETag is advertised and that the behavior is
// correct), satisfying constitution Principles I & II and FR-010/FR-011. TTL,
// transactions, and query are not implemented and therefore never advertised here.
func (s *YDBStore) Features() []state.Feature {
	return []state.Feature{state.FeatureETag}
}

// GetComponentMetadata returns the component's metadata schema map (required by
// the contrib state.Store interface at this version). The scaffold exposes none.
func (s *YDBStore) GetComponentMetadata() map[string]string {
	return map[string]string{}
}

// Get returns the stored value and current etag for a key, or an empty response
// when the key is absent or logically expired.
func (s *YDBStore) Get(ctx context.Context, req *state.GetRequest) (*state.GetResponse, error) {
	return s.get(ctx, req)
}

// Set stores a value under a key (unconditional upsert, or optimistic compare-and-set
// when the request carries an etag).
func (s *YDBStore) Set(ctx context.Context, req *state.SetRequest) error {
	return s.set(ctx, req)
}

// Delete removes a key (idempotent, or optimistic compare-and-delete when the
// request carries an etag).
func (s *YDBStore) Delete(ctx context.Context, req *state.DeleteRequest) error {
	return s.del(ctx, req)
}

// Close releases the YDB driver so the component can be restarted cleanly
// (spec FR-013). It is safe to call when Init never opened a connection.
func (s *YDBStore) Close() error {
	if s.driver == nil {
		return nil
	}
	return s.driver.Close(context.Background())
}
