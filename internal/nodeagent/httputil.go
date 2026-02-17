// Package nodeagent httputil.go contains shared HTTP utilities for the nodeagent package.
package nodeagent

import (
	"fmt"
	"net/url"
)

// BuildURL resolves a base URL and a path-only reference, preserving scheme/host.
// This is used by all HTTP uploaders/fetchers to construct API endpoints.
//
// Example:
//
//	BuildURL("https://api.example.com", "/v1/runs/123/jobs/456/diff")
//	// => "https://api.example.com/v1/runs/123/jobs/456/diff"
func BuildURL(base, p string) (string, error) {
	bu, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("parse base url: %w", err)
	}
	pu, err := url.Parse(p)
	if err != nil {
		return "", fmt.Errorf("parse path: %w", err)
	}
	if pu.IsAbs() || pu.Scheme != "" || pu.Host != "" || pu.User != nil {
		return "", fmt.Errorf("path must not include scheme or host")
	}
	return bu.ResolveReference(pu).String(), nil
}

// MustBuildURL is like BuildURL but panics on error.
// Use this only when the inputs are known to be valid (e.g., hardcoded paths).
func MustBuildURL(base, p string) string {
	u, err := BuildURL(base, p)
	if err != nil {
		panic(fmt.Sprintf("MustBuildURL: %v", err))
	}
	return u
}
