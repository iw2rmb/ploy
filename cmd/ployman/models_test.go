package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	models "github.com/iw2rmb/ploy/internal/llms/models"
)

// setupTestServer creates a test HTTP server for CLI command testing
func setupTestServer() *httptest.Server {
	mux := http.NewServeMux()

	// Mock models storage for testing
	mockModels := make(map[string]models.LLMModel)

	// List models endpoint
	mux.HandleFunc("/v1/llms/models", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			provider := r.URL.Query().Get("provider")
			capability := r.URL.Query().Get("capability")

			filteredModels := make([]models.LLMModel, 0)
			for _, model := range mockModels {
				// Apply filters
				if provider != "" && model.Provider != provider {
					continue
				}
				if capability != "" && !model.HasCapability(capability) {
					continue
				}
				filteredModels = append(filteredModels, model)
			}

			response := map[string]interface{}{
				"models": filteredModels,
				"count":  len(filteredModels),
				"total":  len(mockModels),
			}
			_ = json.NewEncoder(w).Encode(response)

		case "POST":
			var model models.LLMModel
			if err := json.NewDecoder(r.Body).Decode(&model); err != nil {
				http.Error(w, "invalid JSON", http.StatusBadRequest)
				return
			}

			// Check if model already exists
			if _, exists := mockModels[model.ID]; exists {
				http.Error(w, "model already exists", http.StatusConflict)
				return
			}

			// Validate model
			if err := model.Validate(); err != nil {
				http.Error(w, fmt.Sprintf("validation failed: %v", err), http.StatusBadRequest)
				return
			}

			// Set timestamps
			model.SetSystemFields()
			mockModels[model.ID] = model

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"id":      model.ID,
				"message": "model created successfully",
			})
		}
	})

	// Individual model endpoints
	mux.HandleFunc("/v1/llms/models/", func(w http.ResponseWriter, r *http.Request) {
		// Extract model ID from URL
		parts := strings.Split(r.URL.Path, "/")
		if len(parts) < 5 {
			http.Error(w, "invalid URL", http.StatusBadRequest)
			return
		}

		modelID := parts[4]
		if len(parts) > 5 && parts[5] == "stats" {
			// Handle stats endpoint
			if modelID == "" {
				http.Error(w, "model ID is required", http.StatusBadRequest)
				return
			}

			stats := map[string]interface{}{
				"model_id":            modelID,
				"usage_count":         42,
				"last_used":           time.Now().Format(time.RFC3339),
				"total_requests":      100,
				"successful_requests": 95,
				"failed_requests":     5,
				"success_rate":        0.95,
				"cost_metrics": map[string]interface{}{
					"total_cost":   15.50,
					"average_cost": 0.155,
					"cost_per_day": 2.25,
				},
			}
			_ = json.NewEncoder(w).Encode(stats)
			return
		}

		switch r.Method {
		case "GET":
			if modelID == "" {
				http.Error(w, "model ID is required", http.StatusBadRequest)
				return
			}

			model, exists := mockModels[modelID]
			if !exists {
				http.Error(w, "model not found", http.StatusNotFound)
				return
			}

			_ = json.NewEncoder(w).Encode(model)

		case "PUT":
			if modelID == "" {
				http.Error(w, "model ID is required", http.StatusBadRequest)
				return
			}

			var updatedModel models.LLMModel
			if err := json.NewDecoder(r.Body).Decode(&updatedModel); err != nil {
				http.Error(w, "invalid JSON", http.StatusBadRequest)
				return
			}

			if updatedModel.ID != modelID {
				http.Error(w, "model ID mismatch", http.StatusBadRequest)
				return
			}

			existingModel, exists := mockModels[modelID]
			if !exists {
				http.Error(w, "model not found", http.StatusNotFound)
				return
			}

			// Preserve creation time
			updatedModel.Created = existingModel.Created
			updatedModel.SetSystemFields()

			if err := updatedModel.Validate(); err != nil {
				http.Error(w, fmt.Sprintf("validation failed: %v", err), http.StatusBadRequest)
				return
			}

			mockModels[modelID] = updatedModel
			_ = json.NewEncoder(w).Encode(map[string]string{
				"id":      modelID,
				"message": "model updated successfully",
			})

		case "DELETE":
			if modelID == "" {
				http.Error(w, "model ID is required", http.StatusBadRequest)
				return
			}

			if _, exists := mockModels[modelID]; !exists {
				http.Error(w, "model not found", http.StatusNotFound)
				return
			}

			delete(mockModels, modelID)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"id":      modelID,
				"message": "model deleted successfully",
			})
		}
	})

	return httptest.NewServer(mux)
}

func TestModelsCmd_List(t *testing.T) {
	server := setupTestServer()
	defer server.Close()

	// Set the controller URL to our test server
	originalURL := controllerURL
	controllerURL = server.URL
	defer func() { controllerURL = originalURL }()

	// Add a test model first via API
	testModel := models.LLMModel{
		ID:           "test-model@v1",
		Name:         "Test Model",
		Provider:     "openai",
		Capabilities: []string{"code", "analysis"},
		MaxTokens:    8000,
	}
	{
		var buf bytes.Buffer
		_ = json.NewEncoder(&buf).Encode(testModel)
		if resp, err := http.Post(server.URL+"/v1/llms/models", "application/json", &buf); err == nil && resp != nil {
			_ = resp.Body.Close()
		}
	}

	// We can't easily capture stdout in tests, but we can test that the command doesn't panic
	// and that it calls the right endpoint

	tests := []struct {
		name string
		args []string
	}{
		{"list_all", []string{"list"}},
		{"list_short", []string{"ls"}},
		{"list_with_provider", []string{"list", "--provider", "openai"}},
		{"list_with_capability", []string{"list", "--capability", "code"}},
		{"list_json_output", []string{"list", "--output", "json"}},
		{"list_yaml_output", []string{"list", "--output", "yaml"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test that command doesn't panic
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("ModelsCmd panicked: %v", r)
				}
			}()

			ModelsCmd(tt.args)
		})
	}
}

func TestModelsCmd_Get(t *testing.T) {
	server := setupTestServer()
	defer server.Close()

	originalURL := controllerURL
	controllerURL = server.URL
	defer func() { controllerURL = originalURL }()

	tests := []struct {
		name string
		args []string
	}{
		{"get_existing", []string{"get", "test-model@v1"}},
		{"show_existing", []string{"show", "test-model@v1"}},
		{"get_with_json_output", []string{"get", "test-model@v1", "--output", "json"}},
		{"get_non_existent", []string{"get", "non-existent@v1"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("ModelsCmd panicked: %v", r)
				}
			}()

			ModelsCmd(tt.args)
		})
	}
}

func TestModelsCmd_Add(t *testing.T) {
	server := setupTestServer()
	defer server.Close()

	originalURL := controllerURL
	controllerURL = server.URL
	defer func() { controllerURL = originalURL }()

	// Create a temporary model file
	testModel := models.LLMModel{
		ID:           "test-add@v1",
		Name:         "Test Add Model",
		Provider:     "openai",
		Capabilities: []string{"code"},
		MaxTokens:    4000,
	}

	// Test with JSON file
	jsonFile := "/tmp/test-model.json"
	jsonData, _ := json.MarshalIndent(testModel, "", "  ")
	_ = os.WriteFile(jsonFile, jsonData, 0644)
	defer func() { _ = os.Remove(jsonFile) }()

	// Test with YAML-like file (we'll use JSON structure but call it yaml)
	yamlFile := "/tmp/test-model.yaml"
	_ = os.WriteFile(yamlFile, jsonData, 0644) // Using JSON data for simplicity
	defer func() { _ = os.Remove(yamlFile) }()

	tests := []struct {
		name string
		args []string
	}{
		{"add_json_file", []string{"add", "-f", jsonFile}},
		{"create_json_file", []string{"create", "-f", jsonFile}},
		{"add_yaml_file", []string{"add", "--file", yamlFile}},
		{"add_missing_file", []string{"add", "-f", "/nonexistent/file.json"}},
		{"add_no_file", []string{"add"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("ModelsCmd panicked: %v", r)
				}
			}()

			ModelsCmd(tt.args)
		})
	}
}

func TestModelsCmd_Update(t *testing.T) {
	server := setupTestServer()
	defer server.Close()

	originalURL := controllerURL
	controllerURL = server.URL
	defer func() { controllerURL = originalURL }()

	// Create a temporary model file for update
	updateModel := models.LLMModel{
		ID:           "test-update@v1",
		Name:         "Updated Test Model",
		Provider:     "openai",
		Capabilities: []string{"code", "analysis"},
		MaxTokens:    8000,
	}

	jsonFile := "/tmp/test-update-model.json"
	jsonData, _ := json.MarshalIndent(updateModel, "", "  ")
	_ = os.WriteFile(jsonFile, jsonData, 0644)
	defer func() { _ = os.Remove(jsonFile) }()

	tests := []struct {
		name string
		args []string
	}{
		{"update_with_file", []string{"update", "test-update@v1", "-f", jsonFile}},
		{"update_missing_id", []string{"update"}},
		{"update_missing_file", []string{"update", "test-model@v1"}},
		{"update_non_existent", []string{"update", "non-existent@v1", "-f", jsonFile}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("ModelsCmd panicked: %v", r)
				}
			}()

			ModelsCmd(tt.args)
		})
	}
}

func TestModelsCmd_Delete(t *testing.T) {
	server := setupTestServer()
	defer server.Close()

	originalURL := controllerURL
	controllerURL = server.URL
	defer func() { controllerURL = originalURL }()

	tests := []struct {
		name string
		args []string
	}{
		{"delete_with_force", []string{"delete", "test-model@v1", "--force"}},
		{"del_with_force", []string{"del", "test-model@v1", "--force"}},
		{"rm_with_force", []string{"rm", "test-model@v1", "--force"}},
		{"delete_missing_id", []string{"delete"}},
		{"delete_non_existent", []string{"delete", "non-existent@v1", "--force"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("ModelsCmd panicked: %v", r)
				}
			}()

			ModelsCmd(tt.args)
		})
	}
}

func TestModelsCmd_Stats(t *testing.T) {
	server := setupTestServer()
	defer server.Close()

	originalURL := controllerURL
	controllerURL = server.URL
	defer func() { controllerURL = originalURL }()

	tests := []struct {
		name string
		args []string
	}{
		{"stats_existing", []string{"stats", "test-model@v1"}},
		{"stats_json_output", []string{"stats", "test-model@v1", "--output", "json"}},
		{"stats_yaml_output", []string{"stats", "test-model@v1", "--output", "yaml"}},
		{"stats_missing_id", []string{"stats"}},
		{"stats_non_existent", []string{"stats", "non-existent@v1"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("ModelsCmd panicked: %v", r)
				}
			}()

			ModelsCmd(tt.args)
		})
	}
}

func TestModelsCmd_Help(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"help_flag", []string{"--help"}},
		{"h_flag", []string{"-h"}},
		{"help_command", []string{"help"}},
		{"no_args", []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("ModelsCmd panicked: %v", r)
				}
			}()

			ModelsCmd(tt.args)
		})
	}
}

func TestModelsCmd_InvalidCommands(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"unknown_action", []string{"unknown"}},
		{"invalid_flag", []string{"list", "--invalid-flag"}},
		{"mixed_args", []string{"get", "model@v1", "extra", "args"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("ModelsCmd panicked: %v", r)
				}
			}()

			ModelsCmd(tt.args)
		})
	}
}

// TestMakeHTTPRequest tests the HTTP request helper function
func TestMakeHTTPRequest(t *testing.T) {
	// Test with valid server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/success":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status": "ok"}`))
		case "/error":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error": "server error"}`))
		case "/bad-json":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`invalid json`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	tests := []struct {
		name           string
		method         string
		url            string
		expectError    bool
		expectedStatus int
	}{
		{
			name:           "successful_request",
			method:         "GET",
			url:            server.URL + "/success",
			expectError:    false,
			expectedStatus: 200,
		},
		{
			name:           "server_error",
			method:         "GET",
			url:            server.URL + "/error",
			expectError:    true,
			expectedStatus: 500,
		},
		{
			name:           "not_found",
			method:         "GET",
			url:            server.URL + "/notfound",
			expectError:    true,
			expectedStatus: 404,
		},
		{
			name:        "invalid_url",
			method:      "GET",
			url:         "invalid-url",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response, err := makeHTTPRequest(tt.method, tt.url, nil)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if response == nil {
					t.Errorf("Expected response but got nil")
				}
			}
		})
	}
}

// BenchmarkModelsCmd_List benchmarks the list command performance
func BenchmarkModelsCmd_List(b *testing.B) {
	server := setupTestServer()
	defer server.Close()

	originalURL := controllerURL
	controllerURL = server.URL
	defer func() { controllerURL = originalURL }()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ModelsCmd([]string{"list"})
	}
}

// BenchmarkMakeHTTPRequest benchmarks the HTTP request helper
func BenchmarkMakeHTTPRequest(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"test": "data"}`))
	}))
	defer server.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = makeHTTPRequest("GET", server.URL, nil)
	}
}
