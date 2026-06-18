package ydbstate

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/dapr/components-contrib/state"
)

// authMethod enumerates the supported authentication modes for connecting to YDB.
type authMethod string

const (
	authAnonymous authMethod = "anonymous"
	authStatic    authMethod = "static"
	authToken     authMethod = "token"
	authSAKey     authMethod = "serviceAccountKey"
	authMetadata  authMethod = "metadata"
)

const defaultTableName = "dapr_state"

// storeMetadata is the parsed, validated configuration for the YDB state store.
// It is populated exclusively from the declared component manifest
// (constitution Principle V: manifest-only configuration).
type storeMetadata struct {
	ConnectionString      string
	TableName             string
	AuthMethod            authMethod
	Username              string
	Password              string
	AccessToken           string
	ServiceAccountKeyPath string
	UseInternalCA         bool
}

// parseAndValidateMetadata reads the component manifest properties into a
// storeMetadata and validates them, returning a field-named error on any
// missing or invalid value. It never panics.
func parseAndValidateMetadata(meta state.Metadata) (storeMetadata, error) {
	props := meta.Properties

	m := storeMetadata{
		ConnectionString:      strings.TrimSpace(props["connectionString"]),
		TableName:             defaultTableName,
		AuthMethod:            authAnonymous,
		Username:              props["username"],
		Password:              props["password"],
		AccessToken:           props["accessToken"],
		ServiceAccountKeyPath: strings.TrimSpace(props["serviceAccountKeyPath"]),
	}

	if v := strings.TrimSpace(props["tableName"]); v != "" {
		m.TableName = v
	}
	if v := strings.TrimSpace(props["authMethod"]); v != "" {
		m.AuthMethod = authMethod(v)
	}
	if v := strings.TrimSpace(props["useInternalCA"]); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return storeMetadata{}, fmt.Errorf("invalid value for metadata field 'useInternalCA': %q (expected 'true' or 'false')", v)
		}
		m.UseInternalCA = b
	}

	if m.ConnectionString == "" {
		return storeMetadata{}, fmt.Errorf("required metadata field 'connectionString' is missing")
	}

	switch m.AuthMethod {
	case authAnonymous, authStatic, authToken, authSAKey, authMetadata:
		// recognized
	default:
		return storeMetadata{}, fmt.Errorf("invalid metadata field 'authMethod': %q (expected one of: anonymous, static, token, serviceAccountKey, metadata)", m.AuthMethod)
	}

	switch m.AuthMethod {
	case authStatic:
		if m.Username == "" {
			return storeMetadata{}, fmt.Errorf("metadata field 'username' is required when authMethod=static")
		}
		if m.Password == "" {
			return storeMetadata{}, fmt.Errorf("metadata field 'password' is required when authMethod=static")
		}
	case authToken:
		if m.AccessToken == "" {
			return storeMetadata{}, fmt.Errorf("metadata field 'accessToken' is required when authMethod=token")
		}
	case authSAKey:
		if m.ServiceAccountKeyPath == "" {
			return storeMetadata{}, fmt.Errorf("metadata field 'serviceAccountKeyPath' is required when authMethod=serviceAccountKey")
		}
	}

	return m, nil
}
