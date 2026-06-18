package ydbstate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/dapr/components-contrib/state"
	stateutils "github.com/dapr/components-contrib/state/utils"
	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"
)

// marshalValue converts a Dapr value to the raw bytes persisted in the value
// column. A []byte is stored verbatim (never parsed); any other type is JSON-
// encoded, matching the Dapr state contract. The stored payload is opaque and is
// never interpreted on read (constitution Principle III).
func marshalValue(v any) ([]byte, error) {
	return stateutils.Marshal(v, json.Marshal)
}

// get returns the value and current etag for a key, or an empty response (no
// error) when the key is absent or logically expired (FR-003, FR-006, FR-009).
func (s *YDBStore) get(ctx context.Context, req *state.GetRequest) (*state.GetResponse, error) {
	params := ydb.ParamsBuilder().Param("$key").Text(req.Key).Build()

	row, err := s.driver.Query().QueryRow(ctx, getQuery(s.md.TableName),
		query.WithParameters(params))
	if errors.Is(err, query.ErrNoRows) {
		return &state.GetResponse{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("ydbstate: get %q: %w", req.Key, err)
	}

	var (
		value []byte
		etag  string
	)
	if err := row.Scan(&value, &etag); err != nil {
		return nil, fmt.Errorf("ydbstate: get %q: scan: %w", req.Key, err)
	}
	return &state.GetResponse{Data: value, ETag: &etag}, nil
}

// set performs an unconditional upsert when no etag is supplied, or an optimistic
// compare-and-set when one is. Every successful write assigns a fresh opaque etag
// (FR-002, FR-005, FR-006, FR-007).
func (s *YDBStore) set(ctx context.Context, req *state.SetRequest) error {
	value, err := marshalValue(req.Value)
	if err != nil {
		return fmt.Errorf("ydbstate: set %q: marshal value: %w", req.Key, err)
	}

	writeParams := ydb.ParamsBuilder().
		Param("$key").Text(req.Key).
		Param("$value").Bytes(value).
		Param("$etag").Text(newETag()).
		Build()
	write := func(ctx context.Context, e query.Executor) error {
		return e.Exec(ctx, upsertQuery(s.md.TableName), query.WithParameters(writeParams))
	}

	if !req.HasETag() {
		if err := write(ctx, s.driver.Query()); err != nil {
			return fmt.Errorf("ydbstate: set %q: %w", req.Key, err)
		}
		return nil
	}
	return s.compareAndWrite(ctx, req.Key, *req.ETag, write)
}

// del performs an unconditional, idempotent delete when no etag is supplied, or an
// optimistic compare-and-delete when one is (FR-004, FR-007).
func (s *YDBStore) del(ctx context.Context, req *state.DeleteRequest) error {
	params := ydb.ParamsBuilder().Param("$key").Text(req.Key).Build()
	write := func(ctx context.Context, e query.Executor) error {
		return e.Exec(ctx, deleteQuery(s.md.TableName), query.WithParameters(params))
	}

	if !req.HasETag() {
		if err := write(ctx, s.driver.Query()); err != nil {
			return fmt.Errorf("ydbstate: delete %q: %w", req.Key, err)
		}
		return nil
	}
	return s.compareAndWrite(ctx, req.Key, *req.ETag, write)
}

// compareAndWrite runs the optimistic-concurrency path in a single serializable
// transaction: read the current etag, reject with ETagMismatch if the row is absent
// or the stored etag differs, otherwise apply the write. A non-matching or malformed
// caller etag yields ETagMismatch — mirroring contrib Postgres v2 and what the Dapr
// conformance suite asserts for a bad etag (research D3/D4).
func (s *YDBStore) compareAndWrite(ctx context.Context, key, wantETag string, write func(context.Context, query.Executor) error) error {
	readParams := ydb.ParamsBuilder().Param("$key").Text(key).Build()
	err := s.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		row, err := tx.QueryRow(ctx, etagQuery(s.md.TableName), query.WithParameters(readParams))
		if errors.Is(err, query.ErrNoRows) {
			return state.NewETagError(state.ETagMismatch, nil)
		}
		if err != nil {
			return err
		}
		var current string
		if err := row.Scan(&current); err != nil {
			return err
		}
		if current != wantETag {
			return state.NewETagError(state.ETagMismatch, nil)
		}
		return write(ctx, tx)
	})
	if err != nil {
		// Surface ETag errors unwrapped so the runtime classifies them correctly.
		var etagErr *state.ETagError
		if errors.As(err, &etagErr) {
			return etagErr
		}
		return fmt.Errorf("ydbstate: conditional write %q: %w", key, err)
	}
	return nil
}
