package mods

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// waitForStepContaining waits until the controller emits an event whose message contains
// the provided substring. Prefers the event SSE stream; falls back to status polling.
// Returns nil when observed, or an error if a failure event is detected or timeout elapses.
func waitForStepContaining(controller, modID, phase, contains string, timeout time.Duration) error {
	if controller == "" || modID == "" || contains == "" {
		return fmt.Errorf("invalid wait parameters")
	}
	// 1) Try SSE stream first for truly event-driven waits
	if err := waitViaSSE(controller, modID, phase, contains, timeout); err == nil {
		return nil
	}
	// 2) Fallback to status polling (previous behavior)
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 5 * time.Second}
	url := strings.TrimRight(controller, "/") + "/mods/" + modID + "/status"
	for time.Now().Before(deadline) {
		req, _ := http.NewRequest(http.MethodGet, url, nil)
		resp, err := client.Do(req)
		if err == nil && resp != nil && resp.Body != nil {
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			s := string(body)
			if strings.Contains(s, contains) {
				return nil
			}
			if phase != "" && strings.Contains(s, "\"phase\":\""+phase+"\"") && strings.Contains(strings.ToLower(s), "job failed") {
				return fmt.Errorf("job in phase %s reported failure", phase)
			}
		}
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("timeout waiting for event: %s", contains)
}

func waitViaSSE(controller, modID, phase, contains string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	url := strings.TrimRight(controller, "/") + "/mods/" + modID + "/logs?follow=1"
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	// Do not set client Timeout; we rely on ctx for streaming
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil || resp == nil || resp.Body == nil || resp.StatusCode/100 != 2 {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		return fmt.Errorf("sse unavailable")
	}
	defer func() { _ = resp.Body.Close() }()
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, ":") { // comment line in SSE
			continue
		}
		// Only examine data lines to reduce noise
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		// Fast substring check
		if phase != "" && !strings.Contains(line, "\"phase\":\""+phase+"\"") {
			// Not our phase; skip
			continue
		}
		if strings.Contains(line, contains) {
			return nil
		}
		if strings.Contains(strings.ToLower(line), "job failed") || strings.Contains(strings.ToLower(line), "level\":\"error\"") {
			if phase == "" || strings.Contains(line, "\"phase\":\""+phase+"\"") {
				return fmt.Errorf("job in phase %s reported failure", phase)
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout")
		default:
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return fmt.Errorf("sse closed before event: %s", contains)
}
