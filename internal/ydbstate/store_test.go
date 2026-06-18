package ydbstate

import (
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

func TestFeatures_AdvertisesOnlyConformanceVerified(t *testing.T) {
	s := New()
	got := s.Features()
	// ETag is advertised only after passing the conformance eTag scenarios; TTL,
	// transactions, and query remain unadvertised (constitution Principles I & II).
	if len(got) != 1 || got[0] != state.FeatureETag {
		t.Fatalf("Features() = %v; want exactly [%v]", got, state.FeatureETag)
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
