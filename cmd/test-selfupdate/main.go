package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
)

type UpdateRequest struct {
	TargetVersion string `json:"target_version"`
	Strategy      string `json:"strategy"`
}

func main() {
	// Test basic self-update status endpoint
	fmt.Println("Testing self-update status endpoint...")
	resp, err := http.Get("https://api.dev.ployman.app/v1/api/update/status")
	if err != nil {
		log.Fatalf("failed to get status: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("failed to read response: %v", err)
	}

	fmt.Printf("Status response: %s\n", body)

	// Test submitting a self-update request
	fmt.Println("Testing self-update submission...")

	updateReq := UpdateRequest{
		TargetVersion: "20250922-test",
		Strategy:      "rolling",
	}

	reqBody, err := json.Marshal(updateReq)
	if err != nil {
		log.Fatalf("failed to marshal request: %v", err)
	}

	resp, err = http.Post("https://api.dev.ployman.app/v1/api/update", "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		log.Fatalf("failed to submit update: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err = io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("failed to read response: %v", err)
	}

	fmt.Printf("Update submission response: %s\n", body)
	fmt.Printf("Status code: %d\n", resp.StatusCode)
}
