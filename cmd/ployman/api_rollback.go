package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

func runApiRollback(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: ployman api rollback <version>")
		return
	}

	targetVersion := args[0]
	controllerURL := getControllerURL()

	// Call rollback endpoint
	rollbackURL := fmt.Sprintf("%s/rollback", controllerURL)

	payload := map[string]string{
		"version": targetVersion,
		"reason":  "Manual rollback via ployman CLI",
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		fmt.Printf("Error: failed to create request: %v\n", err)
		return
	}

	req, err := http.NewRequest("POST", rollbackURL, bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Printf("Error creating request: %v\n", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Rollback failed: %v\n", err)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Error reading response: %v\n", err)
		return
	}

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("Rollback failed with status %d: %s\n", resp.StatusCode, string(body))
		return
	}

	fmt.Printf("Successfully rolled back to version %s\n", targetVersion)
}
