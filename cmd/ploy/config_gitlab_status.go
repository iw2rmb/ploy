package main

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

func printGitlabSignerStatus(w io.Writer, status gitlabSignerStatus, limit int) {
	if limit == 0 {
		limit = -1
	}
	_, _ = fmt.Fprintln(w, "GitLab signer status")
	if status.FeedRevision > 0 {
		_, _ = fmt.Fprintf(w, "Audit feed revision: %d\n", status.FeedRevision)
	}
	if len(status.Secrets) == 0 {
		_, _ = fmt.Fprintln(w, "No GitLab secrets managed by the signer.")
		return
	}

	secrets := append([]gitlabSignerSecretStatus(nil), status.Secrets...)
	sort.Slice(secrets, func(i, j int) bool {
		return strings.ToLower(secrets[i].Name) < strings.ToLower(secrets[j].Name)
	})

	for _, secret := range secrets {
		_, _ = fmt.Fprintf(w, "\nSecret: %s\n", secret.Name)
		if secret.Revision > 0 {
			_, _ = fmt.Fprintf(w, "  Revision: %d\n", secret.Revision)
		}
		if !secret.RotatedAt.IsZero() {
			_, _ = fmt.Fprintf(w, "  Rotated at: %s\n", secret.RotatedAt.UTC().Format(time.RFC3339))
		}
		if len(secret.Scopes) > 0 {
			_, _ = fmt.Fprintf(w, "  Scopes: %s\n", strings.Join(secret.Scopes, ", "))
		}
		printSignerAudit(w, secret.Audit, limit)
	}
}

func printSignerAudit(w io.Writer, audit gitlabSignerAudit, limit int) {
	_, _ = fmt.Fprintln(w, "  Audit:")
	if !audit.LastRotation.IsZero() {
		_, _ = fmt.Fprintf(w, "    Last rotation: %s\n", audit.LastRotation.UTC().Format(time.RFC3339))
	}

	revoked := limitEntries(audit.Revocations, limit)
	if len(revoked) == 0 {
		_, _ = fmt.Fprintln(w, "    Revoked nodes: none recorded")
	} else {
		_, _ = fmt.Fprintln(w, "    Revoked nodes:")
		for _, entry := range revoked {
			ts := ""
			if !entry.Timestamp.IsZero() {
				ts = entry.Timestamp.UTC().Format(time.RFC3339)
			}
			_, _ = fmt.Fprintf(w, "      - %s (token=%s%s)\n", entry.NodeID, entry.TokenID, formatTimestampSuffix(ts))
		}
	}

	failures := limitEntries(audit.Failures, limit)
	if len(failures) == 0 {
		_, _ = fmt.Fprintln(w, "    Revocation failures: none recorded")
	} else {
		_, _ = fmt.Fprintln(w, "    Revocation failures:")
		for _, entry := range failures {
			ts := ""
			if !entry.Timestamp.IsZero() {
				ts = entry.Timestamp.UTC().Format(time.RFC3339)
			}
			errMsg := strings.TrimSpace(entry.Error)
			if errMsg == "" {
				errMsg = "unknown error"
			}
			_, _ = fmt.Fprintf(w, "      - %s (token=%s, error=%s%s)\n", entry.NodeID, entry.TokenID, errMsg, formatTimestampSuffix(ts))
		}
	}
}

func formatTimestampSuffix(ts string) string {
	if ts == "" {
		return ""
	}
	return ", ts=" + ts
}

func limitEntries[T any](items []T, limit int) []T {
	if limit < 0 || limit >= len(items) {
		return items
	}
	return items[:limit]
}
