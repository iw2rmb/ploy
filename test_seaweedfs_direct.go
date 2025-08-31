package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"time"
)

func main() {
	// Test direct upload to SeaweedFS without any adapter
	fmt.Println("Testing direct SeaweedFS upload...")

	url := "http://seaweedfs-filer.service.consul:8888/artifacts/test-direct-go/test.txt"
	testData := []byte("This is a test file uploaded directly from Go")

	// Create multipart form
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	fileWriter, err := writer.CreateFormFile("file", "test.txt")
	if err != nil {
		log.Fatalf("Failed to create form file: %v", err)
	}

	_, err = io.Copy(fileWriter, bytes.NewReader(testData))
	if err != nil {
		log.Fatalf("Failed to copy data: %v", err)
	}

	writer.Close()

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Create request
	req, err := http.NewRequest("POST", url, &buf)
	if err != nil {
		log.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	fmt.Printf("Sending request to %s\n", url)
	fmt.Printf("Content-Type: %s\n", writer.FormDataContentType())
	fmt.Printf("Body size: %d bytes\n", buf.Len())

	// Send request
	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("Response Status: %s\n", resp.Status)
	fmt.Printf("Response Body: %s\n", string(body))

	if resp.StatusCode == 201 {
		fmt.Println("SUCCESS: File uploaded!")
	} else {
		fmt.Printf("FAILED: Got status %d\n", resp.StatusCode)
	}
}
