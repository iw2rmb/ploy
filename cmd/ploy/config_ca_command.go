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

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// handleConfigCA routes ca subcommands: list, set, unset.
func handleConfigCA(args []string, stderr io.Writer) error {
	if wantsHelp(args) {
		printConfigCAUsage(stderr)
		return nil
	}
	if len(args) == 0 {
		printConfigCAUsage(stderr)
		return errors.New("ca subcommand required")
	}
	switch args[0] {
	case "list", "ls":
		return handleConfigCAList(args[1:], stderr)
	case "set":
		return handleConfigCASet(args[1:], stderr)
	case "unset":
		return handleConfigCAUnset(args[1:], stderr)
	default:
		printConfigCAUsage(stderr)
		return fmt.Errorf("unknown ca subcommand %q", args[0])
	}
}

func printConfigCAUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy config ca <command>")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Manage global CA certificate entries injected into job sections.")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Commands:")
	_, _ = fmt.Fprintln(w, "  list [--section <SECTION>]       List all CA entries")
	_, _ = fmt.Fprintln(w, "  set --hash <HASH> --section <SECTION>")
	_, _ = fmt.Fprintln(w, "                                  Add a CA entry")
	_, _ = fmt.Fprintln(w, "  unset --hash <HASH> --section <SECTION>")
	_, _ = fmt.Fprintln(w, "                                  Remove a CA entry")
}

// configCAListItem matches the server's list response structure.
type configCAItem struct {
	Hash    string `json:"hash"`
	Section string `json:"section"`
}

// handleConfigCAList retrieves and displays all CA entries.
func handleConfigCAList(args []string, stderr io.Writer) error {
	if wantsHelp(args) {
		printConfigCAListUsage(stderr)
		return nil
	}

	if stderr == nil {
		stderr = io.Discard
	}
	fs := flag.NewFlagSet("config ca list", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var section stringValue
	fs.Var(&section, "section", "Filter by section: pre_gate, re_gate, post_gate, mig, heal")

	if err := parseFlagSet(fs, args, func() { printConfigCAListUsage(stderr) }); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		printConfigCAListUsage(stderr)
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}

	ctx := context.Background()
	baseURL, client, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return fmt.Errorf("resolve control plane: %w", err)
	}

	endpoint := strings.TrimSuffix(baseURL.String(), "/") + "/v1/config/ca"
	if section.set {
		if err := contracts.ValidateHydraSection(section.value); err != nil {
			return err
		}
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

	var items []configCAItem
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	if len(items) == 0 {
		fmt.Println("No CA entries configured.")
		return nil
	}

	fmt.Printf("%-64s %s\n", "HASH", "SECTION")
	fmt.Println(strings.Repeat("-", 80))
	for _, item := range items {
		fmt.Printf("%-64s %s\n", item.Hash, item.Section)
	}
	return nil
}

func printConfigCAListUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy config ca list [--section <SECTION>]")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Options:")
	_, _ = fmt.Fprintln(w, "  --section  Filter by section: pre_gate, re_gate, post_gate, mig, heal")
}

// handleConfigCASet adds a CA entry.
// Supports two modes:
//   - --hash <HASH> --section <SECTION>: register an existing hash
//   - --file <PATH> --section <SECTION> [--section ...]: upload file, compute hash, register
func handleConfigCASet(args []string, stderr io.Writer) error {
	if wantsHelp(args) {
		printConfigCASetUsage(stderr)
		return nil
	}

	if stderr == nil {
		stderr = io.Discard
	}
	fs := flag.NewFlagSet("config ca set", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var (
		hash     stringValue
		file     stringValue
		sections stringsValue
	)
	fs.Var(&hash, "hash", "CA hash (required unless --file is provided)")
	fs.Var(&file, "file", "Path to CA bundle file (mutually exclusive with --hash)")
	fs.Var(&sections, "section", "Target section (repeatable; required)")

	if err := parseFlagSet(fs, args, func() { printConfigCASetUsage(stderr) }); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		printConfigCASetUsage(stderr)
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	if !hash.set && !file.set {
		printConfigCASetUsage(stderr)
		return errors.New("either --hash or --file is required")
	}
	if hash.set && file.set {
		printConfigCASetUsage(stderr)
		return errors.New("--hash and --file are mutually exclusive")
	}
	if len(sections.values) == 0 {
		printConfigCASetUsage(stderr)
		return errors.New("--section is required")
	}
	for _, s := range sections.values {
		if err := contracts.ValidateHydraSection(s); err != nil {
			return err
		}
	}

	ctx := context.Background()
	baseURL, client, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return fmt.Errorf("resolve control plane: %w", err)
	}

	var resolvedHash string
	var resolvedBundleID string
	if file.set {
		// Upload the CA bundle file as a spec bundle and derive the hash.
		archiveBytes, buildErr := buildSourceArchive(expandPath(file.value))
		if buildErr != nil {
			return fmt.Errorf("build archive from %s: %w", file.value, buildErr)
		}
		resolvedHash = computeArchiveShortHash(archiveBytes)
		bundleID, _, _, uploadErr := uploadSpecBundle(ctx, baseURL, client, archiveBytes)
		if uploadErr != nil {
			return fmt.Errorf("upload CA bundle: %w", uploadErr)
		}
		resolvedBundleID = bundleID
	} else {
		normalizedHash, parseErr := contracts.ParseStoredCAEntry(hash.value)
		if parseErr != nil {
			return parseErr
		}
		resolvedHash = normalizedHash
	}

	// Register the hash for each requested section.
	for _, section := range sections.values {
		if regErr := putConfigCAEntry(ctx, baseURL, client, resolvedHash, section, resolvedBundleID); regErr != nil {
			return regErr
		}
	}

	if len(sections.values) == 1 {
		fmt.Printf("CA entry %q added to section %s\n", resolvedHash, sections.values[0])
	} else {
		fmt.Printf("CA entry %q added to sections %s\n", resolvedHash, strings.Join(sections.values, ", "))
	}
	return nil
}

// putConfigCAEntry registers a CA hash for a single section via PUT /v1/config/ca/{hash}.
func putConfigCAEntry(ctx context.Context, baseURL *url.URL, client *http.Client, hash, section, bundleID string) error {
	reqBody := struct {
		Section  string `json:"section"`
		BundleID string `json:"bundle_id,omitempty"`
	}{Section: section, BundleID: strings.TrimSpace(bundleID)}
	bodyJSON, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	endpoint := baseURL.JoinPath("v1", "config", "ca", hash)
	req, err := http.NewRequestWithContext(ctx, "PUT", endpoint.String(), bytes.NewReader(bodyJSON))
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
		return fmt.Errorf("server returned %d for section %s: %s", resp.StatusCode, section, string(body))
	}
	return nil
}

func printConfigCASetUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy config ca set (--hash <HASH> | --file <PATH>) --section <SECTION> [--section ...]")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Options:")
	_, _ = fmt.Fprintln(w, "  --hash     CA content hash (7-64 hex chars; mutually exclusive with --file)")
	_, _ = fmt.Fprintln(w, "  --file     Path to CA bundle file (uploads content, derives hash; mutually exclusive with --hash)")
	_, _ = fmt.Fprintln(w, "  --section  Target section: pre_gate, re_gate, post_gate, mig, heal (repeatable, required)")
}

// handleConfigCAUnset removes a CA entry.
func handleConfigCAUnset(args []string, stderr io.Writer) error {
	if wantsHelp(args) {
		printConfigCAUnsetUsage(stderr)
		return nil
	}

	if stderr == nil {
		stderr = io.Discard
	}
	fs := flag.NewFlagSet("config ca unset", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var (
		hash    stringValue
		section stringValue
	)
	fs.Var(&hash, "hash", "CA hash (required)")
	fs.Var(&section, "section", "Target section (required)")

	if err := parseFlagSet(fs, args, func() { printConfigCAUnsetUsage(stderr) }); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		printConfigCAUnsetUsage(stderr)
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	if !hash.set || strings.TrimSpace(hash.value) == "" {
		printConfigCAUnsetUsage(stderr)
		return errors.New("--hash is required")
	}
	if !section.set || strings.TrimSpace(section.value) == "" {
		printConfigCAUnsetUsage(stderr)
		return errors.New("--section is required")
	}
	if err := contracts.ValidateHydraSection(section.value); err != nil {
		return err
	}

	// Validate and normalize hash using Hydra parser rules.
	normalizedHash, err := contracts.ParseStoredCAEntry(hash.value)
	if err != nil {
		return err
	}
	hash.value = normalizedHash

	ctx := context.Background()
	baseURL, client, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return fmt.Errorf("resolve control plane: %w", err)
	}

	endpoint := strings.TrimSuffix(baseURL.String(), "/") + "/v1/config/ca/" + hash.value + "?section=" + section.value
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

	fmt.Printf("CA entry %q removed from section %s\n", hash.value, section.value)
	return nil
}

func printConfigCAUnsetUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy config ca unset --hash <HASH> --section <SECTION>")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Options:")
	_, _ = fmt.Fprintln(w, "  --hash     CA content hash (required)")
	_, _ = fmt.Fprintln(w, "  --section  Target section: pre_gate, re_gate, post_gate, mig, heal (required)")
}
