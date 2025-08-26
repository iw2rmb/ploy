package arf

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"text/tabwriter"
	"time"

	"github.com/iw2rmb/ploy/api/arf"
)

// Sandbox commands

func handleARFSandboxCommand(args []string) error {
	if len(args) == 0 {
		return listSandboxes()
	}

	action := args[0]
	switch action {
	case "list":
		return listSandboxes()
	case "create":
		return createSandboxInteractive()
	case "destroy":
		if len(args) < 2 {
			fmt.Println("Usage: ploy arf sandbox destroy <sandbox-id>")
			return nil
		}
		return destroySandbox(args[1])
	case "--help":
		printSandboxUsage()
		return nil
	default:
		fmt.Printf("Unknown sandbox action: %s\n", action)
		printSandboxUsage()
		return nil
	}
}

func printSandboxUsage() {
	fmt.Println("Usage: ploy arf sandbox <action> [options]")
	fmt.Println()
	fmt.Println("Available actions:")
	fmt.Println("  list              List active sandboxes")
	fmt.Println("  create            Create new sandbox interactively")
	fmt.Println("  destroy <id>      Destroy sandbox")
}

func listSandboxes() error {
	url := fmt.Sprintf("%s/arf/sandboxes", arfControllerURL)
	response, err := makeAPIRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to list sandboxes: %w", err)
	}

	var data struct {
		Sandboxes []arf.SandboxInfo `json:"sandboxes"`
		Count     int               `json:"count"`
	}

	if err := json.Unmarshal(response, &data); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if data.Count == 0 {
		fmt.Println("No active sandboxes")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tJAIL NAME\tSTATUS\tCREATED\tEXPIRES")
	fmt.Fprintln(w, "--\t---------\t------\t-------\t-------")

	for _, sandbox := range data.Sandboxes {
		created := sandbox.CreatedAt.Format("15:04:05")
		expires := sandbox.ExpiresAt.Format("15:04:05")
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			sandbox.ID, sandbox.JailName, sandbox.Status, created, expires)
	}

	w.Flush()
	fmt.Printf("\nTotal: %d sandboxes\n", data.Count)
	return nil
}

func createSandboxInteractive() error {
	fmt.Println("Creating new sandbox (interactive mode)")

	config := arf.SandboxConfig{}
	config.Repository = promptUser("Repository URL (optional): ")
	config.Branch = promptUser("Branch (optional): ")
	config.Language = promptUser("Language (optional): ")
	config.MemoryLimit = promptUser("Memory limit (e.g., 2G, default: 1G): ")
	if config.MemoryLimit == "" {
		config.MemoryLimit = "1G"
	}

	ttlStr := promptUser("TTL in minutes (default: 60): ")
	ttlMinutes := 60
	if ttlStr != "" {
		if minutes, err := strconv.Atoi(ttlStr); err == nil {
			ttlMinutes = minutes
		}
	}
	config.TTL = time.Duration(ttlMinutes) * time.Minute

	data, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to serialize config: %w", err)
	}

	url := fmt.Sprintf("%s/arf/sandboxes", arfControllerURL)
	response, err := makeAPIRequest("POST", url, data)
	if err != nil {
		return fmt.Errorf("failed to create sandbox: %w", err)
	}

	var sandbox arf.Sandbox
	if err := json.Unmarshal(response, &sandbox); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	fmt.Printf("\nSandbox created successfully!\n")
	fmt.Printf("ID: %s\n", sandbox.ID)
	fmt.Printf("Jail Name: %s\n", sandbox.JailName)
	fmt.Printf("Status: %s\n", sandbox.Status)
	fmt.Printf("Expires: %s\n", sandbox.ExpiresAt.Format(time.RFC3339))

	return nil
}

func destroySandbox(sandboxID string) error {
	url := fmt.Sprintf("%s/arf/sandboxes/%s", arfControllerURL, sandboxID)
	_, err := makeAPIRequest("DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("failed to destroy sandbox: %w", err)
	}

	fmt.Printf("Sandbox %s destroyed successfully\n", sandboxID)
	return nil
}