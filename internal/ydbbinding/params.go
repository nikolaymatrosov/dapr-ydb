package ydbbinding

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/ydb-platform/ydb-go-sdk/v3/table/types"
)

// buildParams turns the request's `params` metadata (a JSON string) into the
// argument slice passed to the database/sql query. The JSON kind selects the form
// (FR-006): a JSON array => positional (bound to `?`, types inferred); a JSON
// object => named+typed (bound to `$name`, with an explicit YDB type). An absent
// or empty value means no parameters.
func buildParams(raw string) ([]any, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	switch raw[0] {
	case '[':
		return positionalParams(raw)
	case '{':
		return namedParams(raw)
	default:
		return nil, fmt.Errorf("ydbbinding: metadata %q must be a JSON array (positional) or object (named); got %q", metaParams, raw[0:1])
	}
}

// positionalParams decodes a JSON array into positional arguments. Types are
// inferred from the JSON value (FR-006a): an integral number becomes Int64, a
// fractional number Double, a string Utf8, a bool Bool, null a NULL. Integral
// numbers are converted to int64 so auto-declaration infers Int64 rather than
// Double (a JSON number otherwise decodes to Go float64).
func positionalParams(raw string) ([]any, error) {
	var vals []any
	if err := json.Unmarshal([]byte(raw), &vals); err != nil {
		return nil, fmt.Errorf("ydbbinding: parse positional %q: %w", metaParams, err)
	}
	args := make([]any, len(vals))
	for i, v := range vals {
		if f, ok := v.(float64); ok && f == math.Trunc(f) && !math.IsInf(f, 0) {
			args[i] = int64(f)
			continue
		}
		args[i] = v
	}
	return args, nil
}

// typedParam is one entry of the named+typed parameter object.
type typedParam struct {
	Type  string          `json:"type"`
	Value json.RawMessage `json:"value"`
}

// namedParams decodes a JSON object into named, explicitly-typed arguments. Each
// value is an object {"type": "<YdbType>", "value": <json>}; the name is the key,
// with an optional leading `$` (the YDB driver binds sql.Named("x") to `$x`).
func namedParams(raw string) ([]any, error) {
	var obj map[string]typedParam
	dec := json.NewDecoder(strings.NewReader(raw))
	if err := dec.Decode(&obj); err != nil {
		return nil, fmt.Errorf("ydbbinding: parse named %q: %w (each entry must be {\"type\":..,\"value\":..})", metaParams, err)
	}
	args := make([]any, 0, len(obj))
	for name, p := range obj {
		val, err := typedValue(p.Type, p.Value)
		if err != nil {
			return nil, fmt.Errorf("ydbbinding: parameter %q: %w", name, err)
		}
		args = append(args, sql.Named(strings.TrimPrefix(name, "$"), val))
	}
	return args, nil
}

// typedValue converts a JSON value to the named YDB type. An unknown type name or
// a value that cannot be coerced returns an error so the caller can name the
// offending parameter (FR-007).
func typedValue(typeName string, raw json.RawMessage) (types.Value, error) {
	switch strings.ToLower(strings.TrimSpace(typeName)) {
	case "bool":
		var v bool
		if err := json.Unmarshal(raw, &v); err != nil {
			return nil, fmt.Errorf("expected a boolean for Bool: %w", err)
		}
		return types.BoolValue(v), nil
	case "int32":
		n, err := jsonInt(raw)
		if err != nil {
			return nil, fmt.Errorf("Int32: %w", err)
		}
		return types.Int32Value(int32(n)), nil
	case "uint32":
		n, err := jsonUint(raw)
		if err != nil {
			return nil, fmt.Errorf("Uint32: %w", err)
		}
		return types.Uint32Value(uint32(n)), nil
	case "int64":
		n, err := jsonInt(raw)
		if err != nil {
			return nil, fmt.Errorf("Int64: %w", err)
		}
		return types.Int64Value(n), nil
	case "uint64":
		n, err := jsonUint(raw)
		if err != nil {
			return nil, fmt.Errorf("Uint64: %w", err)
		}
		return types.Uint64Value(n), nil
	case "float":
		f, err := jsonFloat(raw)
		if err != nil {
			return nil, fmt.Errorf("as Float: %w", err)
		}
		return types.FloatValue(float32(f)), nil
	case "double":
		f, err := jsonFloat(raw)
		if err != nil {
			return nil, fmt.Errorf("as Double: %w", err)
		}
		return types.DoubleValue(f), nil
	case "utf8", "text":
		s, err := jsonString(raw)
		if err != nil {
			return nil, fmt.Errorf("Utf8: %w", err)
		}
		return types.TextValue(s), nil
	case "string", "bytes":
		s, err := jsonString(raw)
		if err != nil {
			return nil, fmt.Errorf("as String: %w", err)
		}
		b, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			return nil, fmt.Errorf("as String (expects base64-encoded bytes): %w", err)
		}
		return types.BytesValue(b), nil
	case "json":
		s, err := jsonString(raw)
		if err != nil {
			return nil, fmt.Errorf("as Json: %w", err)
		}
		return types.JSONValue(s), nil
	case "timestamp":
		t, err := jsonTime(raw)
		if err != nil {
			return nil, fmt.Errorf("as Timestamp: %w", err)
		}
		return types.TimestampValueFromTime(t), nil
	case "datetime":
		t, err := jsonTime(raw)
		if err != nil {
			return nil, fmt.Errorf("as Datetime: %w", err)
		}
		return types.DatetimeValueFromTime(t), nil
	case "date":
		t, err := jsonTime(raw)
		if err != nil {
			return nil, fmt.Errorf("as Date: %w", err)
		}
		return types.DateValueFromTime(t), nil
	default:
		return nil, fmt.Errorf("unknown YDB type %q", typeName)
	}
}

// jsonInt accepts a JSON number or numeric string and returns an int64.
func jsonInt(raw json.RawMessage) (int64, error) {
	var n json.Number
	if err := json.Unmarshal(raw, &n); err == nil {
		return n.Int64()
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return strconv.ParseInt(s, 10, 64)
	}
	return 0, fmt.Errorf("expected an integer or numeric string")
}

// jsonUint accepts a JSON number or numeric string and returns a uint64.
func jsonUint(raw json.RawMessage) (uint64, error) {
	var n json.Number
	if err := json.Unmarshal(raw, &n); err == nil {
		return strconv.ParseUint(n.String(), 10, 64)
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return strconv.ParseUint(s, 10, 64)
	}
	return 0, fmt.Errorf("expected an unsigned integer or numeric string")
}

func jsonFloat(raw json.RawMessage) (float64, error) {
	var f float64
	if err := json.Unmarshal(raw, &f); err != nil {
		return 0, fmt.Errorf("expected a number")
	}
	return f, nil
}

func jsonString(raw json.RawMessage) (string, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", fmt.Errorf("expected a string")
	}
	return s, nil
}

// jsonTime parses an RFC3339 timestamp from a JSON string.
func jsonTime(raw json.RawMessage) (time.Time, error) {
	s, err := jsonString(raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("expected an RFC3339 string")
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("expected an RFC3339 timestamp: %w", err)
	}
	return t, nil
}
