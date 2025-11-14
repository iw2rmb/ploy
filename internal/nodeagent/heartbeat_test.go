package nodeagent

// This file previously contained heartbeat tests (578 LOC).
// Tests have been split into:
// - heartbeat_connection_test.go: connection/retry tests (URL building, TLS, HTTP client, request/response)
// - heartbeat_timing_test.go: timing tests (timeout, backoff, intervals)
