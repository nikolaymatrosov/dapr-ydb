package ydbstate

import "testing"

// marshalValue is pure and needs no database: []byte is stored verbatim, everything
// else is JSON-encoded (matches the Dapr conformance assertDataEquals expectations).
// YDB-backed behavior lives in integration_test.go (build tag `integration`).
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
