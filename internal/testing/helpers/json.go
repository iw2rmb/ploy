package helpers

import (
	"bytes"
	"encoding/json"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

// MarshalJSON marshals an object to JSON bytes
func MarshalJSON(t testing.TB, v interface{}) []byte {
	t.Helper()

	data, err := json.Marshal(v)
	require.NoError(t, err)

	return data
}

// MarshalJSONString marshals an object to JSON string
func MarshalJSONString(t testing.TB, v interface{}) string {
	t.Helper()
	return string(MarshalJSON(t, v))
}

// UnmarshalJSON unmarshals JSON bytes to an object
func UnmarshalJSON(t testing.TB, data []byte, target interface{}) {
	t.Helper()

	err := json.Unmarshal(data, target)
	require.NoError(t, err)
}

// UnmarshalJSONString unmarshals JSON string to an object
func UnmarshalJSONString(t testing.TB, jsonStr string, target interface{}) {
	t.Helper()
	UnmarshalJSON(t, []byte(jsonStr), target)
}

// CreateJSONReader creates an io.Reader from a JSON-serializable object
func CreateJSONReader(t testing.TB, v interface{}) io.Reader {
	t.Helper()
	return bytes.NewReader(MarshalJSON(t, v))
}
