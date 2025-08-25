// Package testutils provides custom assertions for testing Ploy components
package testutils

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// AssertErrorContains checks if error contains expected message
func AssertErrorContains(t *testing.T, err error, expected string) {
	t.Helper()
	if err == nil {
		t.Errorf("expected error containing '%s', got nil", expected)
		return
	}
	if !strings.Contains(err.Error(), expected) {
		t.Errorf("expected error containing '%s', got '%s'", expected, err.Error())
	}
}

// AssertErrorType checks if error is of expected type
func AssertErrorType(t *testing.T, err error, expectedType interface{}) {
	t.Helper()
	if err == nil {
		t.Errorf("expected error of type %T, got nil", expectedType)
		return
	}
	
	expectedTypeValue := reflect.TypeOf(expectedType)
	actualTypeValue := reflect.TypeOf(err)
	
	if !actualTypeValue.AssignableTo(expectedTypeValue) {
		t.Errorf("expected error of type %T, got %T", expectedType, err)
	}
}

// AssertEventually retries an assertion until it passes or times out
func AssertEventually(t *testing.T, condition func() bool, timeout time.Duration, message string) {
	t.Helper()
	
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			t.Fatalf("Assertion failed within timeout %v: %s", timeout, message)
		case <-ticker.C:
			if condition() {
				return
			}
		}
	}
}

// AssertEventuallyWithContext retries an assertion with context support
func AssertEventuallyWithContext(t *testing.T, ctx context.Context, condition func(context.Context) bool, message string) {
	t.Helper()
	
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			t.Fatalf("Assertion failed within context: %s", message)
		case <-ticker.C:
			if condition(ctx) {
				return
			}
		}
	}
}

// AssertJSONEqual compares JSON structures
func AssertJSONEqual(t *testing.T, expected, actual interface{}) {
	t.Helper()
	
	expectedJSON, err := json.MarshalIndent(expected, "", "  ")
	require.NoError(t, err, "Failed to marshal expected JSON")
	
	actualJSON, err := json.MarshalIndent(actual, "", "  ")
	require.NoError(t, err, "Failed to marshal actual JSON")
	
	if !bytes.Equal(expectedJSON, actualJSON) {
		t.Errorf("JSON not equal\nExpected:\n%s\n\nActual:\n%s", 
			string(expectedJSON), string(actualJSON))
	}
}

// AssertJSONContains checks if actual JSON contains all fields from expected
func AssertJSONContains(t *testing.T, expected, actual interface{}) {
	t.Helper()
	
	expectedMap := interfaceToMap(t, expected)
	actualMap := interfaceToMap(t, actual)
	
	for key, expectedValue := range expectedMap {
		actualValue, exists := actualMap[key]
		if !exists {
			t.Errorf("Expected field '%s' not found in actual JSON", key)
			continue
		}
		
		if !reflect.DeepEqual(expectedValue, actualValue) {
			t.Errorf("Field '%s' mismatch\nExpected: %v\nActual: %v", 
				key, expectedValue, actualValue)
		}
	}
}

// AssertSliceContains checks if slice contains expected element
func AssertSliceContains(t *testing.T, slice interface{}, element interface{}) {
	t.Helper()
	
	sliceValue := reflect.ValueOf(slice)
	if sliceValue.Kind() != reflect.Slice {
		t.Fatalf("Expected slice, got %T", slice)
	}
	
	for i := 0; i < sliceValue.Len(); i++ {
		item := sliceValue.Index(i).Interface()
		if reflect.DeepEqual(item, element) {
			return
		}
	}
	
	t.Errorf("Slice does not contain expected element\nSlice: %v\nElement: %v", 
		slice, element)
}

// AssertSliceNotContains checks if slice does not contain element
func AssertSliceNotContains(t *testing.T, slice interface{}, element interface{}) {
	t.Helper()
	
	sliceValue := reflect.ValueOf(slice)
	if sliceValue.Kind() != reflect.Slice {
		t.Fatalf("Expected slice, got %T", slice)
	}
	
	for i := 0; i < sliceValue.Len(); i++ {
		item := sliceValue.Index(i).Interface()
		if reflect.DeepEqual(item, element) {
			t.Errorf("Slice contains unexpected element\nSlice: %v\nElement: %v", 
				slice, element)
			return
		}
	}
}

// AssertMapContains checks if map contains expected key-value pairs
func AssertMapContains(t *testing.T, actualMap interface{}, expectedPairs map[string]interface{}) {
	t.Helper()
	
	actualMapValue := reflect.ValueOf(actualMap)
	if actualMapValue.Kind() != reflect.Map {
		t.Fatalf("Expected map, got %T", actualMap)
	}
	
	for expectedKey, expectedValue := range expectedPairs {
		actualValue := actualMapValue.MapIndex(reflect.ValueOf(expectedKey))
		
		if !actualValue.IsValid() {
			t.Errorf("Expected key '%s' not found in map", expectedKey)
			continue
		}
		
		if !reflect.DeepEqual(actualValue.Interface(), expectedValue) {
			t.Errorf("Key '%s' value mismatch\nExpected: %v\nActual: %v",
				expectedKey, expectedValue, actualValue.Interface())
		}
	}
}

// AssertStringContains checks if string contains expected substring
func AssertStringContains(t *testing.T, str, substring string) {
	t.Helper()
	if !strings.Contains(str, substring) {
		t.Errorf("String does not contain expected substring\nString: %s\nSubstring: %s",
			str, substring)
	}
}

// AssertStringNotContains checks if string does not contain substring
func AssertStringNotContains(t *testing.T, str, substring string) {
	t.Helper()
	if strings.Contains(str, substring) {
		t.Errorf("String contains unexpected substring\nString: %s\nSubstring: %s",
			str, substring)
	}
}

// AssertStringHasPrefix checks if string has expected prefix
func AssertStringHasPrefix(t *testing.T, str, prefix string) {
	t.Helper()
	if !strings.HasPrefix(str, prefix) {
		t.Errorf("String does not have expected prefix\nString: %s\nPrefix: %s",
			str, prefix)
	}
}

// AssertStringHasSuffix checks if string has expected suffix
func AssertStringHasSuffix(t *testing.T, str, suffix string) {
	t.Helper()
	if !strings.HasSuffix(str, suffix) {
		t.Errorf("String does not have expected suffix\nString: %s\nSuffix: %s",
			str, suffix)
	}
}

// AssertDurationWithin checks if duration is within expected range
func AssertDurationWithin(t *testing.T, actual, expected, tolerance time.Duration) {
	t.Helper()
	
	diff := actual - expected
	if diff < 0 {
		diff = -diff
	}
	
	if diff > tolerance {
		t.Errorf("Duration outside tolerance\nActual: %v\nExpected: %v\nTolerance: %v\nDifference: %v",
			actual, expected, tolerance, diff)
	}
}

// AssertChannelReceives checks if channel receives expected value within timeout
func AssertChannelReceives(t *testing.T, ch <-chan interface{}, expected interface{}, timeout time.Duration) {
	t.Helper()
	
	select {
	case actual := <-ch:
		if !reflect.DeepEqual(actual, expected) {
			t.Errorf("Channel received unexpected value\nExpected: %v\nActual: %v",
				expected, actual)
		}
	case <-time.After(timeout):
		t.Errorf("Channel did not receive expected value within timeout %v", timeout)
	}
}

// AssertChannelEmpty checks if channel is empty (non-blocking)
func AssertChannelEmpty(t *testing.T, ch <-chan interface{}) {
	t.Helper()
	
	select {
	case value := <-ch:
		t.Errorf("Channel was expected to be empty but contained: %v", value)
	default:
		// Channel is empty as expected
	}
}

// AssertFileExists checks if file exists at path
func AssertFileExists(t *testing.T, path string) {
	t.Helper()
	
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("Expected file to exist at path: %s", path)
	} else if err != nil {
		t.Errorf("Error checking file existence at path %s: %v", path, err)
	}
}

// AssertFileNotExists checks if file does not exist at path
func AssertFileNotExists(t *testing.T, path string) {
	t.Helper()
	
	if _, err := os.Stat(path); err == nil {
		t.Errorf("Expected file to not exist at path: %s", path)
	} else if !os.IsNotExist(err) {
		t.Errorf("Error checking file non-existence at path %s: %v", path, err)
	}
}

// AssertFileContains checks if file contains expected content
func AssertFileContains(t *testing.T, path, expected string) {
	t.Helper()
	
	content, err := os.ReadFile(path)
	require.NoError(t, err, "Failed to read file: %s", path)
	
	if !strings.Contains(string(content), expected) {
		t.Errorf("File does not contain expected content\nFile: %s\nExpected: %s\nActual content:\n%s",
			path, expected, string(content))
	}
}

// AssertNoGoroutineLeaks checks for goroutine leaks
func AssertNoGoroutineLeaks(t *testing.T, before int, tolerance int) {
	t.Helper()
	
	// Give goroutines time to finish
	time.Sleep(100 * time.Millisecond)
	runtime.GC()
	runtime.GC() // Run GC twice to ensure finalizers run
	
	after := runtime.NumGoroutine()
	diff := after - before
	
	if diff > tolerance {
		buf := make([]byte, 1<<16)
		stackLen := runtime.Stack(buf, true)
		
		t.Errorf("Potential goroutine leak detected\nBefore: %d goroutines\nAfter: %d goroutines\nDifference: %d (tolerance: %d)\n\nStack trace:\n%s",
			before, after, diff, tolerance, string(buf[:stackLen]))
	}
}

// interfaceToMap converts interface{} to map[string]interface{} via JSON
func interfaceToMap(t *testing.T, obj interface{}) map[string]interface{} {
	t.Helper()
	
	jsonData, err := json.Marshal(obj)
	require.NoError(t, err, "Failed to marshal object to JSON")
	
	var result map[string]interface{}
	err = json.Unmarshal(jsonData, &result)
	require.NoError(t, err, "Failed to unmarshal JSON to map")
	
	return result
}

// TestingLogger provides a logger that writes to testing.T
type TestingLogger struct {
	t *testing.T
}

// NewTestingLogger creates a new testing logger
func NewTestingLogger(t *testing.T) *TestingLogger {
	return &TestingLogger{t: t}
}

// Log writes a log message to the test output
func (l *TestingLogger) Log(level, message string) {
	l.t.Helper()
	l.t.Logf("[%s] %s", level, message)
}

// Info writes an info log message
func (l *TestingLogger) Info(message string) {
	l.Log("INFO", message)
}

// Warn writes a warning log message
func (l *TestingLogger) Warn(message string) {
	l.Log("WARN", message)
}

// Error writes an error log message
func (l *TestingLogger) Error(message string) {
	l.Log("ERROR", message)
}

// Debug writes a debug log message
func (l *TestingLogger) Debug(message string) {
	l.Log("DEBUG", message)
}

// Benchmark helpers for performance testing

// BenchmarkHelper provides utilities for benchmark testing
type BenchmarkHelper struct {
	b *testing.B
}

// NewBenchmarkHelper creates a new benchmark helper
func NewBenchmarkHelper(b *testing.B) *BenchmarkHelper {
	return &BenchmarkHelper{b: b}
}

// MeasureOperation measures the execution time of an operation
func (h *BenchmarkHelper) MeasureOperation(name string, op func()) time.Duration {
	h.b.Helper()
	
	start := time.Now()
	op()
	duration := time.Since(start)
	
	h.b.Logf("%s took %v", name, duration)
	return duration
}

// ResetTimer resets the benchmark timer
func (h *BenchmarkHelper) ResetTimer() {
	h.b.ResetTimer()
}

// StopTimer stops the benchmark timer
func (h *BenchmarkHelper) StopTimer() {
	h.b.StopTimer()
}

// StartTimer starts the benchmark timer
func (h *BenchmarkHelper) StartTimer() {
	h.b.StartTimer()
}