package ydbstate

import (
	"strings"
	"testing"

	"github.com/dapr/components-contrib/metadata"
	"github.com/dapr/components-contrib/state"

	"github.com/nikolaymatrosov/dapr-ydb/internal/ydbconfig"
)

func newMeta(props map[string]string) state.Metadata {
	return state.Metadata{Base: metadata.Base{Properties: props}}
}

func TestParseMetadata_ValidAnonymous(t *testing.T) {
	m, err := parseAndValidateMetadata(newMeta(map[string]string{
		"connectionString": "grpc://localhost:2136/local",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.AuthMethod != ydbconfig.AuthAnonymous {
		t.Errorf("AuthMethod = %q; want anonymous (default)", m.AuthMethod)
	}
	if m.TableName != defaultTableName {
		t.Errorf("TableName = %q; want default %q", m.TableName, defaultTableName)
	}
}

func TestParseMetadata_AppliesOverrides(t *testing.T) {
	m, err := parseAndValidateMetadata(newMeta(map[string]string{
		"connectionString": "grpcs://ydb.example:2135/db",
		"tableName":        "my_state",
		"useInternalCA":    "true",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.TableName != "my_state" {
		t.Errorf("TableName = %q; want my_state", m.TableName)
	}
	if !m.UseInternalCA {
		t.Error("UseInternalCA = false; want true")
	}
}

func TestParseMetadata_MissingConnectionString(t *testing.T) {
	_, err := parseAndValidateMetadata(newMeta(map[string]string{}))
	if err == nil {
		t.Fatal("expected error for missing connectionString")
	}
	if !strings.Contains(err.Error(), "connectionString") {
		t.Errorf("error %q must name the missing field 'connectionString'", err)
	}
}

func TestParseMetadata_StaticRequiresUsername(t *testing.T) {
	_, err := parseAndValidateMetadata(newMeta(map[string]string{
		"connectionString": "grpc://localhost:2136/local",
		"authMethod":       "static",
		"password":         "secret",
	}))
	if err == nil {
		t.Fatal("expected error for static auth without username")
	}
	if !strings.Contains(err.Error(), "username") {
		t.Errorf("error %q must name the missing field 'username'", err)
	}
}

func TestParseMetadata_InvalidBool(t *testing.T) {
	_, err := parseAndValidateMetadata(newMeta(map[string]string{
		"connectionString": "grpc://localhost:2136/local",
		"useInternalCA":    "notabool",
	}))
	if err == nil {
		t.Fatal("expected error for invalid useInternalCA")
	}
	if !strings.Contains(err.Error(), "useInternalCA") {
		t.Errorf("error %q must name the field 'useInternalCA'", err)
	}
}

func TestParseMetadata_InvalidAuthMethod(t *testing.T) {
	_, err := parseAndValidateMetadata(newMeta(map[string]string{
		"connectionString": "grpc://localhost:2136/local",
		"authMethod":       "bogus",
	}))
	if err == nil {
		t.Fatal("expected error for invalid authMethod")
	}
	if !strings.Contains(err.Error(), "authMethod") {
		t.Errorf("error %q must name the field 'authMethod'", err)
	}
	// FR-010: the rejection lists the full set of accepted values, including the
	// two Yandex Cloud production methods.
	for _, want := range []string{"anonymous", "static", "token", "serviceAccountKey", "metadata"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q must list accepted value %q", err, want)
		}
	}
}

func TestParseMetadata_ServiceAccountKeyRequiresPath(t *testing.T) {
	_, err := parseAndValidateMetadata(newMeta(map[string]string{
		"connectionString": "grpcs://ydb.example:2135/db",
		"authMethod":       "serviceAccountKey",
	}))
	if err == nil {
		t.Fatal("expected error for serviceAccountKey auth without serviceAccountKeyPath")
	}
	if !strings.Contains(err.Error(), "serviceAccountKeyPath") {
		t.Errorf("error %q must name the missing field 'serviceAccountKeyPath'", err)
	}
}

func TestParseMetadata_MetadataNeedsNoSecret(t *testing.T) {
	m, err := parseAndValidateMetadata(newMeta(map[string]string{
		"connectionString": "grpcs://ydb.example:2135/db",
		"authMethod":       "metadata",
	}))
	if err != nil {
		t.Fatalf("metadata auth requires no secret, but got error: %v", err)
	}
	if m.AuthMethod != ydbconfig.AuthMetadata {
		t.Errorf("AuthMethod = %q; want metadata", m.AuthMethod)
	}
}
