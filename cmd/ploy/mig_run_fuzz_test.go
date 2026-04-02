package main

import (
	"context"
	"encoding/json"
	"testing"
)

// FuzzBuildSpecPayload_NoPanic fuzzes CLI override inputs to ensure
// buildSpecPayload never panics and always returns valid JSON when non-nil.
func FuzzBuildSpecPayload_NoPanic(f *testing.F) {
	// Seeds cover command-as-array, plain string, and empty values.
	f.Add("[/bin/sh,-c,echo]", "", "gitlab.example.com", true, true, false)
	f.Add("echo hello", "glpat-123", "", false, false, true)
	f.Add("", "", "", false, false, false)

	f.Fuzz(func(t *testing.T, migCommand, gitlabPAT, gitlabDomain string, retain, mrSuccess, mrFail bool) {
		// Provide a small variety of env shapes, including malformed entries.
		migEnvs := []string{"KEY=VALUE", "A=B=C", "EMPTY=", "ONLYKEY"}

		payload, err := buildSpecPayload(
			context.Background(), // ctx
			nil,                  // no base URL
			nil,                  // no http client
			"",                   // no spec file
			migEnvs,              // env overrides
			"",                   // no image override
			retain,               // retain flag
			migCommand,           // command (may be JSON array or string)
			gitlabPAT,            // gitlab PAT (may be empty)
			gitlabDomain,         // gitlab domain (may be empty)
			mrSuccess,            // MR on success
			mrFail,               // MR on fail
		)
		if err != nil {
			// On parse/merge error just return; fuzzer will expand inputs.
			return
		}
		if payload != nil {
			var out map[string]any
			if err := json.Unmarshal(payload, &out); err != nil {
				t.Fatalf("payload must unmarshal as JSON: %v", err)
			}
		}
	})
}
