package helpers

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ReadJSONResponse reads and unmarshals a JSON response
func ReadJSONResponse(t testing.TB, resp *http.Response, target interface{}) {
	t.Helper()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	err = json.Unmarshal(body, target)
	require.NoError(t, err)
}

// ReadStringResponse reads a response body as string
func ReadStringResponse(t testing.TB, resp *http.Response) string {
	t.Helper()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	return string(body)
}

// AssertJSONResponse asserts that a response contains expected JSON
func AssertJSONResponse(t testing.TB, resp *http.Response, expected interface{}) {
	t.Helper()

	var actual interface{}
	ReadJSONResponse(t, resp, &actual)

	assert.Equal(t, expected, actual)
}

// AssertJSONResponseContains asserts that a JSON response contains specific fields
func AssertJSONResponseContains(t testing.TB, resp *http.Response, expectedFields map[string]interface{}) {
	t.Helper()

	var responseMap map[string]interface{}
	ReadJSONResponse(t, resp, &responseMap)

	for key, expectedValue := range expectedFields {
		assert.Equal(t, expectedValue, responseMap[key], "Field %s should have expected value", key)
	}
}

// AssertResponseStatus asserts the HTTP response status
func AssertResponseStatus(t testing.TB, resp *http.Response, expectedStatus int) {
	t.Helper()
	assert.Equal(t, expectedStatus, resp.StatusCode, "Response status should match")
}

// AssertResponseHeader asserts a specific response header
func AssertResponseHeader(t testing.TB, resp *http.Response, headerName, expectedValue string) {
	t.Helper()
	assert.Equal(t, expectedValue, resp.Header.Get(headerName), "Header %s should have expected value", headerName)
}
