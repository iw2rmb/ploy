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
	"net/url"
	"strings"
)

// handleConfigHome routes home subcommands: list, set, unset.
func handleConfigHome(args []string, stderr io.Writer) error {
	if wantsHelp(args) {
		printConfigHomeUsage(stderr)
		return nil
	}
	if len(args) == 0 {
		printConfigHomeUsage(stderr)
		return errors.New("home subcommand required")
	}
	switch args[0] {
	case "list", "ls":
		return handleConfigHomeList(args[1:], stderr)
	case "set":
		return handleConfigHomeSet(args[1:], stderr)
	case "unset":
		return handleConfigHomeUnset(args[1:], stderr)
	default:
		printConfigHomeUsage(stderr)
		return fmt.Errorf("unknown home subcommand %q", args[0])
	}
}

func printConfigHomeUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy config home <command>")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Manage global home mount entries injected into job sections.")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Commands:")
	_, _ = fmt.Fprintln(w, "  list [--section <SECTION>]       List all home entries")
	_, _ = fmt.Fprintln(w, "  set --entry <ENTRY> --section <SECTION>")
	_, _ = fmt.Fprintln(w, "                                  Add a home mount entry")
	_, _ = fmt.Fprintln(w, "  unset --dst <DST> --section <SECTION>")
	_, _ = fmt.Fprintln(w, "                                  Remove a home mount entry")
}

// configHomeItem matches the server's list response structure.
type configHomeItem struct {
	Entry   string `json:"entry"`
	Dst     string `json:"dst"`
	Section string `json:"section"`
}

// handleConfigHomeList retrieves and displays all home entries.
func handleConfigHomeList(args []string, stderr io.Writer) error {
	if wantsHelp(args) {
		printConfigHomeListUsage(stderr)
		return nil
	}

	if stderr == nil {
		stderr = io.Discard
	}
	fs := flag.NewFlagSet("config home list", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var section stringValue
	fs.Var(&section, "section", "Filter by section: pre_gate, re_gate, post_gate, mig, heal")

	if err := parseFlagSet(fs, args, func() { printConfigHomeListUsage(stderr) }); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		printConfigHomeListUsage(stderr)
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}

	ctx := context.Background()
	baseURL, client, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return fmt.Errorf("resolve control plane: %w", err)
	}

	endpoint := strings.TrimSuffix(baseURL.String(), "/") + "/v1/config/home"
	if section.set {
		endpoint += "/" + section.value
	}
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
	}

	var items []configHomeItem
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	if len(items) == 0 {
		fmt.Println("No home entries configured.")
		return nil
	}

	fmt.Printf("%-40s %-30s %s\n", "ENTRY", "DST", "SECTION")
	fmt.Println(strings.Repeat("-", 80))
	for _, item := range items {
		display := item.Entry
		if len(display) > 40 {
			display = display[:37] + "..."
		}
		fmt.Printf("%-40s %-30s %s\n", display, item.Dst, item.Section)
	}
	return nil
}

func printConfigHomeListUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy config home list [--section <SECTION>]")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Options:")
	_, _ = fmt.Fprintln(w, "  --section  Filter by section: pre_gate, re_gate, post_gate, mig, heal")
}

// handleConfigHomeSet adds a home mount entry.
func handleConfigHomeSet(args []string, stderr io.Writer) error {
	if wantsHelp(args) {
		printConfigHomeSetUsage(stderr)
		return nil
	}

	if stderr == nil {
		stderr = io.Discard
	}
	fs := flag.NewFlagSet("config home set", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var (
		entry   stringValue
		section stringValue
	)
	fs.Var(&entry, "entry", "Canonical home entry: shortHash:dst or shortHash:dst:ro (required)")
	fs.Var(&section, "section", "Target section (required)")

	if err := parseFlagSet(fs, args, func() { printConfigHomeSetUsage(stderr) }); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		printConfigHomeSetUsage(stderr)
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	if !entry.set || strings.TrimSpace(entry.value) == "" {
		printConfigHomeSetUsage(stderr)
		return errors.New("--entry is required")
	}
	if !section.set || strings.TrimSpace(section.value) == "" {
		printConfigHomeSetUsage(stderr)
		return errors.New("--section is required")
	}

	ctx := context.Background()
	baseURL, client, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return fmt.Errorf("resolve control plane: %w", err)
	}

	reqBody := struct {
		Entry   string `json:"entry"`
		Section string `json:"section"`
	}{Entry: entry.value, Section: section.value}
	bodyJSON, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	endpoint := strings.TrimSuffix(baseURL.String(), "/") + "/v1/config/home"
	req, err := http.NewRequestWithContext(ctx, "PUT", endpoint, bytes.NewReader(bodyJSON))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("PUT %s: %w", endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
	}

	fmt.Printf("Home entry %q added to section %s\n", entry.value, section.value)
	return nil
}

func printConfigHomeSetUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy config home set --entry <ENTRY> --section <SECTION>")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Options:")
	_, _ = fmt.Fprintln(w, "  --entry    Canonical home entry: shortHash:dst or shortHash:dst:ro (required)")
	_, _ = fmt.Fprintln(w, "  --section  Target section: pre_gate, re_gate, post_gate, mig, heal (required)")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Examples:")
	_, _ = fmt.Fprintln(w, "  ploy config home set --entry 'abcdef1:.config/app' --section mig")
	_, _ = fmt.Fprintln(w, "  ploy config home set --entry 'abcdef1:.config/readonly:ro' --section pre_gate")
}

// handleConfigHomeUnset removes a home mount entry.
func handleConfigHomeUnset(args []string, stderr io.Writer) error {
	if wantsHelp(args) {
		printConfigHomeUnsetUsage(stderr)
		return nil
	}

	if stderr == nil {
		stderr = io.Discard
	}
	fs := flag.NewFlagSet("config home unset", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var (
		dst     stringValue
		section stringValue
	)
	fs.Var(&dst, "dst", "Home destination path (required)")
	fs.Var(&section, "section", "Target section (required)")

	if err := parseFlagSet(fs, args, func() { printConfigHomeUnsetUsage(stderr) }); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		printConfigHomeUnsetUsage(stderr)
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	if !dst.set || strings.TrimSpace(dst.value) == "" {
		printConfigHomeUnsetUsage(stderr)
		return errors.New("--dst is required")
	}
	if !section.set || strings.TrimSpace(section.value) == "" {
		printConfigHomeUnsetUsage(stderr)
		return errors.New("--section is required")
	}

	ctx := context.Background()
	baseURL, client, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return fmt.Errorf("resolve control plane: %w", err)
	}

	endpoint := strings.TrimSuffix(baseURL.String(), "/") + "/v1/config/home?dst=" + url.QueryEscape(dst.value) + "&section=" + section.value
	req, err := http.NewRequestWithContext(ctx, "DELETE", endpoint, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("DELETE %s: %w", endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
	}

	fmt.Printf("Home entry for destination %q removed from section %s\n", dst.value, section.value)
	return nil
}

func printConfigHomeUnsetUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy config home unset --dst <DST> --section <SECTION>")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Options:")
	_, _ = fmt.Fprintln(w, "  --dst      Home destination path (required)")
	_, _ = fmt.Fprintln(w, "  --section  Target section: pre_gate, re_gate, post_gate, mig, heal (required)")
}
