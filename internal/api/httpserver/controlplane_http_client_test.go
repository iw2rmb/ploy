package httpserver_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func postJSON(t *testing.T, endpoint string, payload map[string]any) map[string]any {
	status, out := postJSONStatus(t, endpoint, payload)
	if status >= 400 {
		t.Fatalf("post %s -> http %d: %v", endpoint, status, out)
	}
	return out
}

func putJSON(t *testing.T, endpoint string, payload map[string]any) map[string]any {
	status, out := putJSONStatus(t, endpoint, payload)
	if status >= 400 {
		t.Fatalf("put %s -> http %d: %v", endpoint, status, out)
	}
	return out
}

func getJSON(t *testing.T, endpoint string) map[string]any {
	status, out := getJSONStatus(t, endpoint)
	if status >= 400 {
		t.Fatalf("get %s -> http %d: %v", endpoint, status, out)
	}
	return out
}

func postJSONStatus(t *testing.T, endpoint string, payload map[string]any) (int, map[string]any) {
	return sendJSONStatus(t, http.MethodPost, endpoint, payload)
}

func patchJSON(t *testing.T, endpoint string, payload map[string]any) map[string]any {
	status, out := patchJSONStatus(t, endpoint, payload)
	if status >= 400 {
		t.Fatalf("patch %s -> http %d: %v", endpoint, status, out)
	}
	return out
}

func patchJSONStatus(t *testing.T, endpoint string, payload map[string]any) (int, map[string]any) {
	return sendJSONStatus(t, http.MethodPatch, endpoint, payload)
}

func putJSONStatus(t *testing.T, endpoint string, payload map[string]any) (int, map[string]any) {
	return sendJSONStatus(t, http.MethodPut, endpoint, payload)
}

func deleteJSONStatus(t *testing.T, endpoint string, payload map[string]any) (int, map[string]any) {
	return sendJSONStatus(t, http.MethodDelete, endpoint, payload)
}

func getJSONStatus(t *testing.T, endpoint string) (int, map[string]any) {
	t.Helper()
	resp, err := http.Get(endpoint)
	if err != nil {
		t.Fatalf("get %s: %v", endpoint, err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	status := resp.StatusCode
	data, _ := io.ReadAll(resp.Body)
	if len(data) == 0 {
		return status, nil
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		out = map[string]any{"error": strings.TrimSpace(string(data))}
	}
	return status, out
}

func sendJSONStatus(t *testing.T, method, endpoint string, payload map[string]any) (int, map[string]any) {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	req, err := http.NewRequest(method, endpoint, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, endpoint, err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	status := resp.StatusCode
	data, _ := io.ReadAll(resp.Body)
	if status == http.StatusNoContent {
		return status, nil
	}
	if len(data) == 0 {
		return status, nil
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		out = map[string]any{"error": strings.TrimSpace(string(data))}
	}
	return status, out
}
