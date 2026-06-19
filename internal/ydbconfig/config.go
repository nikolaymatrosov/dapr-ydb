// Package ydbconfig holds the YDB connection and authentication configuration
// shared by every component in this plugin — the state store and the output
// binding. Centralizing it keeps the credential logic (including the Yandex
// Cloud auth methods) in one place so both components parse the same manifest
// fields identically and stay in lockstep (constitution Principle V).
//
// Component-specific manifest fields (for example the state store's tableName)
// are parsed by the owning component, not here.
package ydbconfig

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	yc "github.com/ydb-platform/ydb-go-yc"
)

// AuthMethod enumerates the supported authentication modes for connecting to YDB.
type AuthMethod string

// Supported authentication modes, selected by the manifest's authMethod field.
const (
	AuthAnonymous AuthMethod = "anonymous"
	AuthStatic    AuthMethod = "static"
	AuthToken     AuthMethod = "token"
	AuthSAKey     AuthMethod = "serviceAccountKey"
	AuthMetadata  AuthMethod = "metadata"
)

// Config is the parsed, validated connection/auth configuration. It is populated
// exclusively from declared component-manifest properties.
type Config struct {
	ConnectionString      string
	AuthMethod            AuthMethod
	Username              string
	Password              string
	AccessToken           string
	ServiceAccountKeyPath string
	UseInternalCA         bool
}

// Parse reads the shared connection/auth properties from a component manifest and
// validates them, returning a field-named error on any missing or invalid value.
// It never panics (constitution Principle V).
func Parse(props map[string]string) (Config, error) {
	c := Config{
		ConnectionString:      strings.TrimSpace(props["connectionString"]),
		AuthMethod:            AuthAnonymous,
		Username:              props["username"],
		Password:              props["password"],
		AccessToken:           props["accessToken"],
		ServiceAccountKeyPath: strings.TrimSpace(props["serviceAccountKeyPath"]),
	}

	if v := strings.TrimSpace(props["authMethod"]); v != "" {
		c.AuthMethod = AuthMethod(v)
	}
	if v := strings.TrimSpace(props["useInternalCA"]); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return Config{}, fmt.Errorf("invalid value for metadata field 'useInternalCA': %q (expected 'true' or 'false')", v)
		}
		c.UseInternalCA = b
	}

	if c.ConnectionString == "" {
		return Config{}, fmt.Errorf("required metadata field 'connectionString' is missing")
	}

	switch c.AuthMethod {
	case AuthAnonymous, AuthStatic, AuthToken, AuthSAKey, AuthMetadata:
		// recognized
	default:
		return Config{}, fmt.Errorf("invalid metadata field 'authMethod': %q (expected one of: anonymous, static, token, serviceAccountKey, metadata)", c.AuthMethod)
	}

	switch c.AuthMethod {
	case AuthStatic:
		if c.Username == "" {
			return Config{}, fmt.Errorf("metadata field 'username' is required when authMethod=static")
		}
		if c.Password == "" {
			return Config{}, fmt.Errorf("metadata field 'password' is required when authMethod=static")
		}
	case AuthToken:
		if c.AccessToken == "" {
			return Config{}, fmt.Errorf("metadata field 'accessToken' is required when authMethod=token")
		}
	case AuthSAKey:
		if c.ServiceAccountKeyPath == "" {
			return Config{}, fmt.Errorf("metadata field 'serviceAccountKeyPath' is required when authMethod=serviceAccountKey")
		}
	}

	return c, nil
}

// CredentialOptions maps the configured auth method to a base YDB credential
// option, then layers on the internal-CA trust option when requested. The Yandex
// Cloud production paths (serviceAccountKey, metadata) are wired via the
// ydb-go-yc helper module, which acquires and auto-refreshes IAM tokens.
func CredentialOptions(c Config) ([]ydb.Option, error) {
	var base ydb.Option
	switch c.AuthMethod {
	case AuthAnonymous:
		base = ydb.WithAnonymousCredentials()
	case AuthStatic:
		base = ydb.WithStaticCredentials(c.Username, c.Password)
	case AuthToken:
		base = ydb.WithAccessTokenCredentials(c.AccessToken)
	case AuthSAKey:
		// Pre-flight the key file so a missing or unreadable path fails with a
		// field-named error before any network call. The key itself is parsed and
		// exchanged for tokens lazily by the YC SDK at connect time.
		if _, err := os.ReadFile(c.ServiceAccountKeyPath); err != nil {
			return nil, fmt.Errorf("metadata field 'serviceAccountKeyPath': cannot read key file %q: %w", c.ServiceAccountKeyPath, err)
		}
		base = yc.WithServiceAccountKeyFileCredentials(c.ServiceAccountKeyPath)
	case AuthMetadata:
		// Secret-less: credentials come from the instance metadata service of the
		// Yandex Cloud workload this process runs on.
		base = yc.WithMetadataCredentials()
	default:
		return nil, fmt.Errorf("unsupported authMethod %q", c.AuthMethod)
	}

	opts := []ydb.Option{base}
	// useInternalCA is orthogonal to the auth method: managed YDB endpoints present
	// certificates that chain to the Yandex Cloud internal CA.
	if c.UseInternalCA {
		opts = append(opts, yc.WithInternalCA())
	}
	return opts, nil
}

// Open opens a YDB driver for the given config using its credential options.
// Callers own the returned driver and must Close it.
func Open(ctx context.Context, c Config) (*ydb.Driver, error) {
	opts, err := CredentialOptions(c)
	if err != nil {
		return nil, err
	}
	driver, err := ydb.Open(ctx, c.ConnectionString, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to open YDB connection: %w", err)
	}
	return driver, nil
}
