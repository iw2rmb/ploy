package git

import (
	"fmt"
	"net/url"
	"strings"
)

// RedactError replaces any occurrence of the secret in error messages with [REDACTED].
// It handles literal, query-escaped, and path-escaped variants.
func RedactError(err error, secret string) error {
	if err == nil {
		return nil
	}
	if secret == "" {
		return err
	}

	msg := err.Error()

	variants := map[string]struct{}{
		secret: {},
	}
	if q := url.QueryEscape(secret); q != secret {
		variants[q] = struct{}{}
		variants[strings.ReplaceAll(q, "+", "%20")] = struct{}{}
	}
	if p := url.PathEscape(secret); p != secret {
		variants[p] = struct{}{}
	}
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
