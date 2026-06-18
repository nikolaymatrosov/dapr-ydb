package ydbstate

import (
	"context"
	"errors"
	"testing"

	"github.com/dapr/components-contrib/state"
)

// Ensure the scaffold satisfies the Dapr state.Store contract at compile time
// (also asserted in store.go; restated here as a test-visible guarantee).
var _ state.Store = (*YDBStore)(nil)

func TestNew_ReturnsUsableStore(t *testing.T) {
	s := New()
	if s == nil {
		t.Fatal("New() returned nil")
	}
	if s.BulkStore == nil {
		t.Fatal("New() did not wire the default bulk store")
	}
}

func TestFeatures_AdvertisesNothingUnimplemented(t *testing.T) {
	s := New()
	if got := s.Features(); len(got) != 0 {
		t.Fatalf("Features() = %v; scaffold must advertise no capabilities (constitution Principle I)", got)
	}
}

func TestStubOperations_ReturnNotImplemented(t *testing.T) {
	s := New()
	ctx := context.Background()

	if _, err := s.Get(ctx, &state.GetRequest{Key: "k"}); !errors.Is(err, errNotImplemented) {
		t.Errorf("Get error = %v; want errNotImplemented", err)
	}
	if err := s.Set(ctx, &state.SetRequest{Key: "k"}); !errors.Is(err, errNotImplemented) {
		t.Errorf("Set error = %v; want errNotImplemented", err)
	}
	if err := s.Delete(ctx, &state.DeleteRequest{Key: "k"}); !errors.Is(err, errNotImplemented) {
		t.Errorf("Delete error = %v; want errNotImplemented", err)
	}
}

func TestClose_SafeWithoutInit(t *testing.T) {
	if err := New().Close(); err != nil {
		t.Errorf("Close() before Init = %v; want nil", err)
	}
}

func TestGetComponentMetadata_NonNil(t *testing.T) {
	if s := New(); s.GetComponentMetadata() == nil {
		t.Error("GetComponentMetadata() = nil; want non-nil map")
	}
}
