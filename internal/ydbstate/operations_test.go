package ydbstate

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/dapr/components-contrib/metadata"
	"github.com/dapr/components-contrib/state"
	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"
)

// marshalValue is pure and needs no database: []byte is stored verbatim, everything
// else is JSON-encoded (matches the Dapr conformance assertDataEquals expectations).
func TestMarshalValue(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"raw bytes verbatim", []byte("hello\x00\x01"), "hello\x00\x01"},
		{"string is json-encoded", "hello world", `"hello world"`},
		{"empty string is json-encoded", "", `""`},
		{"int is json-encoded", 123, "123"},
		{"bool is json-encoded", true, "true"},
		{"struct is json-encoded", struct {
			A int `json:"a"`
		}{A: 5}, `{"a":5}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := marshalValue(tc.in)
			if err != nil {
				t.Fatalf("marshalValue(%v) error: %v", tc.in, err)
			}
			if string(got) != tc.want {
				t.Errorf("marshalValue(%v) = %q; want %q", tc.in, got, tc.want)
			}
		})
	}
}

// newTestStore initializes a store against a local/CI YDB, skipping when none is
// reachable so the pure unit suite (`make test`) stays green without a database.
// The full behavioral matrix is also covered by the conformance suite.
func newTestStore(t *testing.T) *YDBStore {
	t.Helper()
	connStr := os.Getenv("YDB_CONNECTION_STRING")
	if connStr == "" {
		connStr = "grpc://localhost:2136/local"
	}
	s := New()
	err := s.Init(context.Background(), state.Metadata{Base: metadata.Base{Properties: map[string]string{
		"connectionString": connStr,
		"authMethod":       "anonymous",
	}}})
	if err != nil {
		t.Skipf("skipping YDB-backed test; no reachable YDB at %s: %v", connStr, err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// uniqueKey derives a collision-free key from the test name.
func uniqueKey(t *testing.T, suffix string) string {
	return "ydbstate-test/" + t.Name() + "/" + suffix
}

func TestIntegration_RoundTripAndDelete(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	key := uniqueKey(t, "rt")
	payload := []byte{0x00, 0x01, 0xFF, 0x10} // arbitrary binary (FR-005, SC-001)

	if err := s.Set(ctx, &state.SetRequest{Key: key, Value: payload}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := s.Get(ctx, &state.GetRequest{Key: key})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got.Data) != string(payload) {
		t.Errorf("Get Data = %v; want byte-identical %v", got.Data, payload)
	}
	if got.ETag == nil || *got.ETag == "" {
		t.Errorf("Get ETag = %v; want a non-empty token", got.ETag)
	}

	if err := s.Delete(ctx, &state.DeleteRequest{Key: key}); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	got, err = s.Get(ctx, &state.GetRequest{Key: key})
	if err != nil {
		t.Fatalf("Get after delete: %v", err)
	}
	if len(got.Data) != 0 || got.ETag != nil {
		t.Errorf("Get after delete = %+v; want empty not-found response", got)
	}
}

func TestIntegration_GetAbsentAndDeleteIdempotent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	key := uniqueKey(t, "absent")

	got, err := s.Get(ctx, &state.GetRequest{Key: key})
	if err != nil {
		t.Fatalf("Get absent: %v", err) // FR-003: absent is not an error
	}
	if len(got.Data) != 0 || got.ETag != nil {
		t.Errorf("Get absent = %+v; want empty", got)
	}
	// FR-004: deleting an absent key with no etag succeeds.
	if err := s.Delete(ctx, &state.DeleteRequest{Key: key}); err != nil {
		t.Errorf("Delete absent = %v; want nil (idempotent)", err)
	}
}

func TestIntegration_SetOverwritesAndRotatesETag(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	key := uniqueKey(t, "overwrite")
	t.Cleanup(func() { _ = s.Delete(ctx, &state.DeleteRequest{Key: key}) })

	if err := s.Set(ctx, &state.SetRequest{Key: key, Value: []byte("v1")}); err != nil {
		t.Fatalf("Set v1: %v", err)
	}
	first, _ := s.Get(ctx, &state.GetRequest{Key: key})

	if err := s.Set(ctx, &state.SetRequest{Key: key, Value: []byte("v2")}); err != nil {
		t.Fatalf("Set v2 (unconditional overwrite): %v", err)
	}
	second, _ := s.Get(ctx, &state.GetRequest{Key: key})

	if string(second.Data) != "v2" {
		t.Errorf("after overwrite Data = %q; want v2", second.Data)
	}
	if *first.ETag == *second.ETag {
		t.Errorf("etag did not rotate on write: %q", *second.ETag) // FR-006
	}
}

func TestIntegration_ETagMismatchAndBadEtag(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	key := uniqueKey(t, "etag")
	t.Cleanup(func() { _ = s.Delete(ctx, &state.DeleteRequest{Key: key}) })

	if err := s.Set(ctx, &state.SetRequest{Key: key, Value: []byte("v1")}); err != nil {
		t.Fatalf("Set v1: %v", err)
	}

	stale := "00000000-0000-0000-0000-000000000000"
	bad := "bad-etag" // not a UUID — conformance expects ETagMismatch, not Invalid

	for _, etag := range []string{stale, bad} {
		err := s.Set(ctx, &state.SetRequest{Key: key, Value: []byte("v2"), ETag: &etag})
		var etagErr *state.ETagError
		if !errors.As(err, &etagErr) || etagErr.Kind() != state.ETagMismatch {
			t.Errorf("Set with etag %q error = %v; want ETagMismatch", etag, err)
		}
	}

	// Value must be unchanged after the rejected writes (FR-007).
	got, _ := s.Get(ctx, &state.GetRequest{Key: key})
	if string(got.Data) != "v1" {
		t.Errorf("value mutated by rejected write: %q", got.Data)
	}

	// A correct etag succeeds and advances the token.
	if err := s.Set(ctx, &state.SetRequest{Key: key, Value: []byte("v2"), ETag: got.ETag}); err != nil {
		t.Fatalf("Set with matching etag: %v", err)
	}
}

func TestIntegration_ConcurrentCASAtMostOneWinner(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	key := uniqueKey(t, "race")
	t.Cleanup(func() { _ = s.Delete(ctx, &state.DeleteRequest{Key: key}) })

	if err := s.Set(ctx, &state.SetRequest{Key: key, Value: []byte("seed")}); err != nil {
		t.Fatalf("seed Set: %v", err)
	}
	seed, _ := s.Get(ctx, &state.GetRequest{Key: key})

	const writers = 8
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		wins     int
		mismatch int
	)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			etag := *seed.ETag
			err := s.Set(ctx, &state.SetRequest{Key: key, Value: []byte("contended"), ETag: &etag})
			mu.Lock()
			defer mu.Unlock()
			var etagErr *state.ETagError
			if err == nil {
				wins++
			} else if errors.As(err, &etagErr) && etagErr.Kind() == state.ETagMismatch {
				mismatch++
			} else {
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()

	if wins != 1 || mismatch != writers-1 { // SC-004
		t.Errorf("concurrent CAS: wins=%d mismatch=%d; want wins=1 mismatch=%d", wins, mismatch, writers-1)
	}
}

// seedWithExpiry inserts a row with an explicit expires_at (the public Set API never
// writes expiry — this feature only reads/filters it), so the read filter can be
// exercised directly.
func (s *YDBStore) seedWithExpiry(ctx context.Context, t *testing.T, key string, exp time.Time) {
	t.Helper()
	params := ydb.ParamsBuilder().
		Param("$key").Text(key).
		Param("$value").Bytes([]byte("v")).
		Param("$etag").Text(newETag()).
		Param("$exp").Timestamp(exp).
		Build()
	sql := "DECLARE $key AS Utf8;\nDECLARE $value AS String;\nDECLARE $etag AS Utf8;\nDECLARE $exp AS Timestamp;\n" +
		"UPSERT INTO `" + s.md.TableName + "` (key, value, etag, expires_at) VALUES ($key, $value, $etag, $exp);"
	if err := s.driver.Query().Exec(ctx, sql, query.WithParameters(params)); err != nil {
		t.Fatalf("seed insert: %v", err)
	}
}

func TestIntegration_ExpiryFilteredOnRead(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	expiredKey := uniqueKey(t, "expired")
	futureKey := uniqueKey(t, "future")
	t.Cleanup(func() {
		_ = s.Delete(ctx, &state.DeleteRequest{Key: expiredKey})
		_ = s.Delete(ctx, &state.DeleteRequest{Key: futureKey})
	})

	s.seedWithExpiry(ctx, t, expiredKey, time.Now().Add(-time.Hour)) // logically expired
	s.seedWithExpiry(ctx, t, futureKey, time.Now().Add(time.Hour))   // not yet expired

	// FR-009 / SC-005: an expired row is reported not-found even before purge.
	got, err := s.Get(ctx, &state.GetRequest{Key: expiredKey})
	if err != nil {
		t.Fatalf("Get expired: %v", err)
	}
	if len(got.Data) != 0 || got.ETag != nil {
		t.Errorf("expired row returned %+v; want not-found", got)
	}

	// Sanity: a non-expired row IS returned (the filter isn't dropping everything).
	got, err = s.Get(ctx, &state.GetRequest{Key: futureKey})
	if err != nil {
		t.Fatalf("Get future: %v", err)
	}
	if string(got.Data) != "v" {
		t.Errorf("non-expired row not returned: %+v", got)
	}
}

func TestIntegration_ContextCancellationReturnsError(t *testing.T) {
	s := newTestStore(t)
	key := uniqueKey(t, "cancel")
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancelled

	// FR-012 / SC-006: operations surface the cancellation as an error, never panic.
	if _, err := s.Get(ctx, &state.GetRequest{Key: key}); err == nil {
		t.Error("Get with cancelled ctx returned nil error; want error")
	}
	if err := s.Set(ctx, &state.SetRequest{Key: key, Value: []byte("v")}); err == nil {
		t.Error("Set with cancelled ctx returned nil error; want error")
	}
	if err := s.Delete(ctx, &state.DeleteRequest{Key: key}); err == nil {
		t.Error("Delete with cancelled ctx returned nil error; want error")
	}
}

func TestIntegration_OperationsAfterCloseReturnError(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// FR-012 / SC-006: with the backend connection gone, operations degrade to an
	// error rather than panicking or leaking.
	if _, err := s.Get(ctx, &state.GetRequest{Key: uniqueKey(t, "closed")}); err == nil {
		t.Error("Get after Close returned nil error; want error")
	}
	s.driver = nil // prevent a double-close in newTestStore's cleanup
}
