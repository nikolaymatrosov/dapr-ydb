package ydbconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- Parse(): connection/auth validation ---

func TestParse_DefaultsToAnonymous(t *testing.T) {
	c, err := Parse(map[string]string{"connectionString": "grpc://localhost:2136/local"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.AuthMethod != AuthAnonymous {
		t.Errorf("AuthMethod = %q; want anonymous (default)", c.AuthMethod)
	}
}

func TestParse_MissingConnectionString(t *testing.T) {
	_, err := Parse(map[string]string{})
	if err == nil {
		t.Fatal("expected error for missing connectionString")
	}
	if !strings.Contains(err.Error(), "connectionString") {
		t.Errorf("error %q must name the missing field 'connectionString'", err)
	}
}

func TestParse_StaticRequiresUsername(t *testing.T) {
	_, err := Parse(map[string]string{
		"connectionString": "grpc://localhost:2136/local",
		"authMethod":       "static",
		"password":         "secret",
	})
	if err == nil {
		t.Fatal("expected error for static auth without username")
	}
	if !strings.Contains(err.Error(), "username") {
		t.Errorf("error %q must name the missing field 'username'", err)
	}
}

func TestParse_InvalidBool(t *testing.T) {
	_, err := Parse(map[string]string{
		"connectionString": "grpc://localhost:2136/local",
		"useInternalCA":    "notabool",
	})
	if err == nil {
		t.Fatal("expected error for invalid useInternalCA")
	}
	if !strings.Contains(err.Error(), "useInternalCA") {
		t.Errorf("error %q must name the field 'useInternalCA'", err)
	}
}

func TestParse_InvalidAuthMethod(t *testing.T) {
	_, err := Parse(map[string]string{
		"connectionString": "grpc://localhost:2136/local",
		"authMethod":       "bogus",
	})
	if err == nil {
		t.Fatal("expected error for invalid authMethod")
	}
	if !strings.Contains(err.Error(), "authMethod") {
		t.Errorf("error %q must name the field 'authMethod'", err)
	}
	// The rejection lists the full set of accepted values, including the two
	// Yandex Cloud production methods.
	for _, want := range []string{"anonymous", "static", "token", "serviceAccountKey", "metadata"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q must list accepted value %q", err, want)
		}
	}
}

func TestParse_ServiceAccountKeyRequiresPath(t *testing.T) {
	_, err := Parse(map[string]string{
		"connectionString": "grpcs://ydb.example:2135/db",
		"authMethod":       "serviceAccountKey",
	})
	if err == nil {
		t.Fatal("expected error for serviceAccountKey auth without serviceAccountKeyPath")
	}
	if !strings.Contains(err.Error(), "serviceAccountKeyPath") {
		t.Errorf("error %q must name the missing field 'serviceAccountKeyPath'", err)
	}
}

func TestParse_MetadataNeedsNoSecret(t *testing.T) {
	c, err := Parse(map[string]string{
		"connectionString": "grpcs://ydb.example:2135/db",
		"authMethod":       "metadata",
	})
	if err != nil {
		t.Fatalf("metadata auth requires no secret, but got error: %v", err)
	}
	if c.AuthMethod != AuthMetadata {
		t.Errorf("AuthMethod = %q; want metadata", c.AuthMethod)
	}
}

// --- CredentialOptions(): authMethod → driver-option mapping ---
// These exercise the mapping in isolation; they need no network and assert on
// option count + error wording, since the YC option values are opaque closures.

func TestCredentialOptions_ServiceAccountKey_MissingPath(t *testing.T) {
	_, err := CredentialOptions(Config{AuthMethod: AuthSAKey, ServiceAccountKeyPath: ""})
	if err == nil {
		t.Fatal("expected error for serviceAccountKey with empty path")
	}
	if !strings.Contains(err.Error(), "serviceAccountKeyPath") {
		t.Errorf("error %q must name the field 'serviceAccountKeyPath'", err)
	}
}

func TestCredentialOptions_ServiceAccountKey_UnreadableFile(t *testing.T) {
	_, err := CredentialOptions(Config{AuthMethod: AuthSAKey, ServiceAccountKeyPath: "/no/such/dir/sa-key.json"})
	if err == nil {
		t.Fatal("expected error for unreadable serviceAccountKey file")
	}
	if !strings.Contains(err.Error(), "serviceAccountKeyPath") || !strings.Contains(err.Error(), "cannot read key file") {
		t.Errorf("error %q must name 'serviceAccountKeyPath' and say it cannot read the key file", err)
	}
}

func TestCredentialOptions_ServiceAccountKey_ReadableFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sa-key.json")
	if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	opts, err := CredentialOptions(Config{AuthMethod: AuthSAKey, ServiceAccountKeyPath: path})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(opts) == 0 {
		t.Error("expected a non-empty option slice for a readable serviceAccountKey file")
	}
}

func TestCredentialOptions_Metadata(t *testing.T) {
	opts, err := CredentialOptions(Config{AuthMethod: AuthMetadata})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(opts) == 0 {
		t.Error("expected a non-empty option slice for metadata auth")
	}
}

func TestCredentialOptions_InternalCA_AppendsOption(t *testing.T) {
	for _, method := range []AuthMethod{AuthAnonymous, AuthMetadata} {
		base, err := CredentialOptions(Config{AuthMethod: method, UseInternalCA: false})
		if err != nil {
			t.Fatalf("%s without CA: unexpected error: %v", method, err)
		}
		ca, err := CredentialOptions(Config{AuthMethod: method, UseInternalCA: true})
		if err != nil {
			t.Fatalf("%s with CA: unexpected error: %v", method, err)
		}
		if len(ca) != len(base)+1 {
			t.Errorf("authMethod %q: len(opts) with useInternalCA = %d; want %d (one more than without)", method, len(ca), len(base)+1)
		}
	}
}
