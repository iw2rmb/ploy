package storage

import (
    "errors"
    "testing"
    "time"
)

func TestRetryConfig_CalculateDelay_Caps(t *testing.T) {
    rc := &RetryConfig{InitialDelay: 100 * time.Millisecond, MaxDelay: 300 * time.Millisecond, BackoffMultiplier: 2.0}
    if d := rc.CalculateDelay(0); d != 100*time.Millisecond {
        t.Fatalf("attempt 0 delay = %v, want %v", d, 100*time.Millisecond)
    }
    if d := rc.CalculateDelay(1); d != 200*time.Millisecond {
        t.Fatalf("attempt 1 delay = %v, want 200ms", d)
    }
    // Should cap at MaxDelay
    if d := rc.CalculateDelay(5); d != 300*time.Millisecond {
        t.Fatalf("attempt 5 delay = %v, want 300ms (capped)", d)
    }
}

func TestRetryConfig_ShouldRetry_Edges(t *testing.T) {
    rc := &RetryConfig{MaxAttempts: 2, RetryableErrors: []ErrorType{ErrorTypeNetwork, ErrorTypeInternal}}
    se := &StorageError{ErrorType: ErrorTypeNetwork, Retryable: true}
    if !rc.ShouldRetry(se, 0) {
        t.Fatal("expected retry for retryable network error on first attempt")
    }
    if rc.ShouldRetry(se, 2) { // attempt >= MaxAttempts
        t.Fatal("did not expect retry beyond max attempts")
    }
    if rc.ShouldRetry(&StorageError{ErrorType: ErrorTypeAuthorization, Retryable: false}, 0) {
        t.Fatal("did not expect retry for non-retryable error type")
    }
    if rc.ShouldRetry(&StorageError{ErrorType: ErrorTypeTimeout, Retryable: true}, 0) {
        t.Fatal("did not expect retry when type not in RetryableErrors list")
    }
}

func TestNewStorageError_HTTPClassification_Extras(t *testing.T) {
    // 429 with Retry-After should be rate_limit and retryable
    se := NewStorageError("op", errors.New("too many requests"), ErrorContext{HTTPStatus: 429, Headers: map[string]string{"Retry-After": "5"}})
    if se.ErrorType != ErrorTypeRateLimit || !se.Retryable || se.GetRetryDelay() < 5*time.Second {
        t.Fatalf("expected rate limit retryable with >=5s delay, got %+v", se)
    }
    // 503 should map to service_unavailable and be retryable
    se2 := NewStorageError("op", errors.New("service unavailable"), ErrorContext{HTTPStatus: 503})
    if se2.ErrorType != ErrorTypeServiceUnavailable || !se2.Retryable {
        t.Fatalf("expected service_unavailable retryable, got %+v", se2)
    }
}
