package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"
	"time"
)

// handleToken routes token subcommands.
func handleToken(args []string, stderr io.Writer) error {
	// Handle --help and -h flags to print usage and exit cleanly.
	if wantsHelp(args) {
		printTokenUsage(stderr)
		return nil
	}
	if len(args) == 0 {
		printTokenUsage(stderr)
		return errors.New("token subcommand required")
	}
	switch args[0] {
	case "create":
		return handleTokenCreate(args[1:], stderr)
	case "list":
		return handleTokenList(args[1:], stderr)
	case "revoke":
		return handleTokenRevoke(args[1:], stderr)
	default:
		printTokenUsage(stderr)
		return fmt.Errorf("unknown token subcommand %q", args[0])
	}
}

// printTokenUsage prints the token command usage information.
// Token operations are accessible via `ploy cluster token`.
func printTokenUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy cluster token <command>")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Commands:")
	_, _ = fmt.Fprintln(w, "  create    Create a new API token")
	_, _ = fmt.Fprintln(w, "  list      List all API tokens")
	_, _ = fmt.Fprintln(w, "  revoke    Revoke an API token")
}

// handleTokenCreate creates a new API token.
func handleTokenCreate(args []string, stderr io.Writer) error {
	if wantsHelp(args) {
		printTokenCreateUsage(stderr)
		return nil
	}

	if stderr == nil {
		stderr = io.Discard
	}

	fs := flag.NewFlagSet("token create", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var (
		role          stringValue
		description   stringValue
		expiresInDays intValue
	)
	fs.Var(&role, "role", "Token role: cli-admin, control-plane, or worker (required)")
	fs.Var(&description, "description", "Human-readable description of the token")
	fs.Var(&expiresInDays, "expires", "Expiration in days (default: 365)")

	if err := parseFlagSet(fs, args, func() { printTokenCreateUsage(stderr) }); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		printTokenCreateUsage(stderr)
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	if !role.set || strings.TrimSpace(role.value) == "" {
		printTokenCreateUsage(stderr)
		return errors.New("--role is required")
	}

	ctx := context.Background()
	baseURL, client, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return fmt.Errorf("resolve control plane: %w", err)
	}

	// Prepare request
	reqBody := map[string]interface{}{
		"role": role.value,
	}
	if description.set && strings.TrimSpace(description.value) != "" {
		reqBody["description"] = description.value
	}
	if expiresInDays.set && expiresInDays.value > 0 {
		reqBody["expires_in_days"] = expiresInDays.value
	} else {
		reqBody["expires_in_days"] = 365 // Default
	}

	bodyJSON, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	endpoint := strings.TrimSuffix(baseURL.String(), "/") + "/v1/tokens"
	req, err := makeAuthenticatedRequest(ctx, "POST", endpoint, bytes.NewReader(bodyJSON))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("POST %s: %w", endpoint, err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return controlPlaneHTTPError(resp)
	}

	var result struct {
		Token     string    `json:"token"`
		TokenID   string    `json:"token_id"`
		Role      string    `json:"role"`
		ExpiresAt time.Time `json:"expires_at"`
		Warning   string    `json:"warning"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	// Display the token (only shown once!)
	fmt.Println("=================================================================")
	fmt.Println("API Token Created Successfully")
	fmt.Println("=================================================================")
	fmt.Printf("Token ID:    %s\n", result.TokenID)
	fmt.Printf("Role:        %s\n", result.Role)
	fmt.Printf("Expires:     %s\n", result.ExpiresAt.Format(time.RFC3339))
	fmt.Println()
	fmt.Println("TOKEN (save this securely - it will not be shown again):")
	fmt.Println(result.Token)
	fmt.Println()
	if result.Warning != "" {
		fmt.Println("WARNING:", result.Warning)
	}
	fmt.Println("=================================================================")

	return nil
}

// printTokenCreateUsage prints the token create subcommand usage information.
func printTokenCreateUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy cluster token create --role <role> [--description <desc>] [--expires <days>]")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Flags:")
	_, _ = fmt.Fprintln(w, "  --role          Token role: cli-admin, control-plane, or worker (required)")
	_, _ = fmt.Fprintln(w, "  --description   Human-readable description")
	_, _ = fmt.Fprintln(w, "  --expires       Expiration in days (default: 365)")
}

// handleTokenList lists all API tokens.
func handleTokenList(args []string, stderr io.Writer) error {
	if wantsHelp(args) {
		printTokenListUsage(stderr)
		return nil
	}

	if stderr == nil {
		stderr = io.Discard
	}

	fs := flag.NewFlagSet("token list", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	if err := parseFlagSet(fs, args, func() { printTokenListUsage(stderr) }); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		printTokenListUsage(stderr)
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}

	ctx := context.Background()
	baseURL, client, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return fmt.Errorf("resolve control plane: %w", err)
	}

	endpoint := strings.TrimSuffix(baseURL.String(), "/") + "/v1/tokens"
	req, err := makeAuthenticatedRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", endpoint, err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return controlPlaneHTTPError(resp)
	}

	var result struct {
		Tokens []struct {
			TokenID     string     `json:"token_id"`
			Role        string     `json:"role"`
			Description *string    `json:"description"`
			IssuedAt    time.Time  `json:"issued_at"`
			ExpiresAt   time.Time  `json:"expires_at"`
			LastUsedAt  *time.Time `json:"last_used_at"`
			RevokedAt   *time.Time `json:"revoked_at"`
		} `json:"tokens"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	if len(result.Tokens) == 0 {
		fmt.Println("No API tokens found")
		return nil
	}

	// Display as table
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "TOKEN ID\tROLE\tDESCRIPTION\tEXPIRES\tLAST USED\tSTATUS")
	for _, t := range result.Tokens {
		desc := "-"
		if t.Description != nil {
			desc = *t.Description
			if len(desc) > 40 {
				desc = desc[:37] + "..."
			}
		}

		expires := t.ExpiresAt.Format("2006-01-02")

		lastUsed := "never"
		if t.LastUsedAt != nil {
			lastUsed = t.LastUsedAt.Format("2006-01-02")
		}

		status := "active"
		if t.RevokedAt != nil {
			status = "REVOKED"
		} else if time.Now().After(t.ExpiresAt) {
			status = "EXPIRED"
		}

		tokenIDDisplay := t.TokenID
		if len(tokenIDDisplay) > 12 {
			tokenIDDisplay = tokenIDDisplay[:12] + "..."
		}

		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			tokenIDDisplay, t.Role, desc, expires, lastUsed, status)
	}
	_ = w.Flush()

	return nil
}

// printTokenListUsage prints the token list subcommand usage information.
func printTokenListUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy cluster token list")
}

// handleTokenRevoke revokes an API token.
func handleTokenRevoke(args []string, stderr io.Writer) error {
	if wantsHelp(args) {
		printTokenRevokeUsage(stderr)
		return nil
	}

	if stderr == nil {
		stderr = io.Discard
	}

	fs := flag.NewFlagSet("token revoke", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	if err := parseFlagSet(fs, args, func() { printTokenRevokeUsage(stderr) }); err != nil {
		return err
	}

	if fs.NArg() == 0 {
		printTokenRevokeUsage(stderr)
		return errors.New("token ID is required")
	}
	if fs.NArg() > 1 {
		printTokenRevokeUsage(stderr)
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args()[1:], " "))
	}

	tokenID := fs.Arg(0)

	ctx := context.Background()
	baseURL, client, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return fmt.Errorf("resolve control plane: %w", err)
	}

	endpoint := strings.TrimSuffix(baseURL.String(), "/") + "/v1/tokens/" + tokenID
	req, err := makeAuthenticatedRequest(ctx, "DELETE", endpoint, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("DELETE %s: %w", endpoint, err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return controlPlaneHTTPError(resp)
	}

	fmt.Printf("Token %s revoked successfully\n", tokenID)
	return nil
}

// printTokenRevokeUsage prints the token revoke subcommand usage information.
func printTokenRevokeUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy cluster token revoke <token-id>")
}
