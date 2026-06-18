package ydbstate

import (
	"os"
	"path/filepath"
	"strings"
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

// --- credentialOptions(): authMethod → driver-option mapping (feature 003) ---
// These exercise the mapping in isolation by constructing the parsed metadata
// directly; they need no network and assert on option count + error wording,
// since the YC option values are opaque closures (research D5).

// US1: serviceAccountKey — a missing path fails with a field-named error before
// any network call (FR-005).
func TestCredentialOptions_ServiceAccountKey_MissingPath(t *testing.T) {
	s := &YDBStore{md: storeMetadata{AuthMethod: authSAKey, ServiceAccountKeyPath: ""}}
	_, err := s.credentialOptions()
	if err == nil {
		t.Fatal("expected error for serviceAccountKey with empty path")
	}
	if !strings.Contains(err.Error(), "serviceAccountKeyPath") {
		t.Errorf("error %q must name the field 'serviceAccountKeyPath'", err)
	}
}

// US1: serviceAccountKey — an unreadable/non-existent file fails with a
// field-named "cannot read key file" error (FR-005, SC-003).
func TestCredentialOptions_ServiceAccountKey_UnreadableFile(t *testing.T) {
	s := &YDBStore{md: storeMetadata{AuthMethod: authSAKey, ServiceAccountKeyPath: "/no/such/dir/sa-key.json"}}
	_, err := s.credentialOptions()
	if err == nil {
		t.Fatal("expected error for unreadable serviceAccountKey file")
	}
	if !strings.Contains(err.Error(), "serviceAccountKeyPath") || !strings.Contains(err.Error(), "cannot read key file") {
		t.Errorf("error %q must name 'serviceAccountKeyPath' and say it cannot read the key file", err)
	}
}

// US1: serviceAccountKey — a readable file yields options with no error. The key
// is parsed lazily by the YC SDK at connect time, so a placeholder file suffices
// to exercise the pre-flight here.
func TestCredentialOptions_ServiceAccountKey_ReadableFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sa-key.json")
	if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	s := &YDBStore{md: storeMetadata{AuthMethod: authSAKey, ServiceAccountKeyPath: path}}
	opts, err := s.credentialOptions()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(opts) == 0 {
		t.Error("expected a non-empty option slice for a readable serviceAccountKey file")
	}
}

// US2: metadata — needs no secret/config and yields options with no error (FR-002).
func TestCredentialOptions_Metadata(t *testing.T) {
	s := &YDBStore{md: storeMetadata{AuthMethod: authMetadata}}
	opts, err := s.credentialOptions()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(opts) == 0 {
		t.Error("expected a non-empty option slice for metadata auth")
	}
}

// US3: useInternalCA is orthogonal to the auth method and appends exactly one
// extra option, for any method (FR-007).
func TestCredentialOptions_InternalCA_AppendsOption(t *testing.T) {
	for _, method := range []authMethod{authAnonymous, authMetadata} {
		without := &YDBStore{md: storeMetadata{AuthMethod: method, UseInternalCA: false}}
		with := &YDBStore{md: storeMetadata{AuthMethod: method, UseInternalCA: true}}

		base, err := without.credentialOptions()
		if err != nil {
			t.Fatalf("%s without CA: unexpected error: %v", method, err)
		}
		ca, err := with.credentialOptions()
		if err != nil {
			t.Fatalf("%s with CA: unexpected error: %v", method, err)
		}
		if len(ca) != len(base)+1 {
			t.Errorf("authMethod %q: len(opts) with useInternalCA = %d; want %d (one more than without)", method, len(ca), len(base)+1)
		}
	}
}
