// Package redact provides utilities for redacting sensitive information from error messages.
package redact

import (
	"fmt"
	"net/url"
	"strings"
)

// Error replaces any occurrence of the secret in error messages with [REDACTED].
// It handles both literal secret and URL-encoded variants (query-escaped, path-escaped).
//
// This is commonly used to redact Personal Access Tokens (PATs) from git error messages
// before logging or returning errors to callers.
func Error(err error, secret string) error {
	if err == nil {
		return nil
	}
	if secret == "" {
		return err
	}

	msg := err.Error()

	// Build a set of variants to redact: literal, query-escaped, path-escaped,
	// and a minimal legacy replacement used in early code paths.
	variants := map[string]struct{}{
		secret: {},
	}
	if q := url.QueryEscape(secret); q != secret {
		variants[q] = struct{}{}
		// Some logs render spaces as %20 not "+"; include that form.
		variants[strings.ReplaceAll(q, "+", "%20")] = struct{}{}
	}
	if p := url.PathEscape(secret); p != secret {
		variants[p] = struct{}{}
	}
	// Legacy minimal encoding coverage.
	variants[strings.ReplaceAll(strings.ReplaceAll(secret, " ", "%20"), "@", "%40")] = struct{}{}

	modified := false
	for v := range variants {
		if v == "" || v == msg {
			continue
		}
		if strings.Contains(msg, v) {
			msg = strings.ReplaceAll(msg, v, "[REDACTED]")
			modified = true
		}
	}

	if modified {
		return fmt.Errorf("%s", msg)
	}
	return err
}
