package seaweedfs

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSeaweedFSStorage(t *testing.T) {
	tests := []struct {
		name      string
		masterURL string
		wantErr   bool
	}{
		{
			name:      "valid master URL",
			masterURL: "http://localhost:9333",
			wantErr:   false,
		},
		{
			name:      "empty master URL returns error",
			masterURL: "",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storage, err := NewSeaweedFSStorage(tt.masterURL)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, storage)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, storage)
				assert.Equal(t, tt.masterURL, storage.masterURL)
				assert.NotNil(t, storage.httpClient)
			}
		})
	}
}

func TestStoreDiff(t *testing.T) {
	// Mock SeaweedFS master server
	masterServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/dir/assign":
			// Return volume assignment
			response := AssignResponse{
				FID:   "3,01234567890123",
				URL:   "127.0.0.1:8080",
				Count: 1,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer masterServer.Close()

	// Mock SeaweedFS volume server
	volumeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify multipart upload
		assert.Equal(t, "POST", r.Method)
		assert.Contains(t, r.Header.Get("Content-Type"), "multipart/form-data")
		
		// Parse multipart form
		err := r.ParseMultipartForm(32 << 20)
		require.NoError(t, err)
		
		// Check file upload
		file, header, err := r.FormFile("file")
		require.NoError(t, err)
		defer file.Close()
		
		assert.Equal(t, "job-123.diff", header.Filename)
		
		// Read uploaded content
		content, err := io.ReadAll(file)
		require.NoError(t, err)
		assert.Equal(t, "diff content here", string(content))
		
		// Return success
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"size": 17}`))
	}))
	defer volumeServer.Close()

	// Extract port from volume server URL
	parts := strings.Split(volumeServer.URL, ":")
	port := parts[len(parts)-1]
	
	storage := &SeaweedFSStorage{
		masterURL:  masterServer.URL,
		httpClient: &http.Client{},
		volumeURLOverride: "127.0.0.1:" + port, // Override for testing
	}

	jobID := "job-123"
	diff := []byte("diff content here")
	
	fileID, err := storage.StoreDiff(jobID, diff)
	assert.NoError(t, err)
	assert.Equal(t, "3,01234567890123", fileID)
}

func TestStoreDiffWithAssignError(t *testing.T) {
	// Mock server that returns error for assign
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/dir/assign" {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("assign error"))
		}
	}))
	defer server.Close()

	storage := &SeaweedFSStorage{
		masterURL:  server.URL,
		httpClient: &http.Client{},
	}

	jobID := "job-456"
	diff := []byte("diff content")
	
	fileID, err := storage.StoreDiff(jobID, diff)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to assign volume")
	assert.Empty(t, fileID)
}

func TestRetrieveDiff(t *testing.T) {
	expectedDiff := "retrieved diff content"
	
	// Mock SeaweedFS master server for lookup
	masterServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/dir/lookup") {
			// Check volumeId parameter
			volumeID := r.URL.Query().Get("volumeId")
			assert.Equal(t, "3", volumeID)
			
			// Return lookup response
			response := LookupResponse{
				VolumeID: "3",
				Locations: []Location{
					{
						URL:       "127.0.0.1:8080",
						PublicURL: "127.0.0.1:8080",
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		} else {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer masterServer.Close()

	// Mock SeaweedFS volume server for download
	volumeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify download request
		assert.Equal(t, "GET", r.Method)
		assert.Equal(t, "/3,01234567890123", r.URL.Path)
		
		// Return diff content
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(expectedDiff))
	}))
	defer volumeServer.Close()

	// Extract port from volume server URL
	parts := strings.Split(volumeServer.URL, ":")
	port := parts[len(parts)-1]
	
	storage := &SeaweedFSStorage{
		masterURL:  masterServer.URL,
		httpClient: &http.Client{},
		volumeURLOverride: "127.0.0.1:" + port, // Override for testing
	}

	fileID := "3,01234567890123"
	diff, err := storage.RetrieveDiff(fileID)
	assert.NoError(t, err)
	assert.Equal(t, []byte(expectedDiff), diff)
}

func TestRetrieveDiffWithLookupError(t *testing.T) {
	// Mock server that returns error for lookup
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/dir/lookup") {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("volume not found"))
		}
	}))
	defer server.Close()

	storage := &SeaweedFSStorage{
		masterURL:  server.URL,
		httpClient: &http.Client{},
	}

	fileID := "3,01234567890123"
	diff, err := storage.RetrieveDiff(fileID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to lookup volume")
	assert.Nil(t, diff)
}

func TestRetrieveDiffNoLocations(t *testing.T) {
	// Mock server that returns empty locations
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/dir/lookup") {
			response := LookupResponse{
				VolumeID:  "3",
				Locations: []Location{}, // Empty locations
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}
	}))
	defer server.Close()

	storage := &SeaweedFSStorage{
		masterURL:  server.URL,
		httpClient: &http.Client{},
	}

	fileID := "3,01234567890123"
	diff, err := storage.RetrieveDiff(fileID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no locations found")
	assert.Nil(t, diff)
}

func TestExtractVolumeID(t *testing.T) {
	tests := []struct {
		name     string
		fileID   string
		expected string
		wantErr  bool
	}{
		{
			name:     "valid file ID",
			fileID:   "3,01234567890123",
			expected: "3",
			wantErr:  false,
		},
		{
			name:     "valid file ID with larger volume",
			fileID:   "123,abcdef",
			expected: "123",
			wantErr:  false,
		},
		{
			name:     "invalid file ID without comma",
			fileID:   "invalid",
			expected: "",
			wantErr:  true,
		},
		{
			name:     "empty file ID",
			fileID:   "",
			expected: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storage := &SeaweedFSStorage{}
			volumeID, err := storage.extractVolumeID(tt.fileID)
			
			if tt.wantErr {
				assert.Error(t, err)
				assert.Empty(t, volumeID)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, volumeID)
			}
		})
	}
}

func TestDiffLifecycle(t *testing.T) {
	// Test complete upload and download cycle
	expectedDiff := []byte("complete diff content for testing")
	
	// Shared state for the test
	var storedFID string
	var storedContent []byte
	
	// Mock master server
	masterServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/dir/assign":
			response := AssignResponse{
				FID:   "5,fedcba9876543210",
				URL:   "127.0.0.1:8080",
				Count: 1,
			}
			storedFID = response.FID
			json.NewEncoder(w).Encode(response)
			
		case strings.HasPrefix(r.URL.Path, "/dir/lookup"):
			response := LookupResponse{
				VolumeID: "5",
				Locations: []Location{
					{URL: "127.0.0.1:8080"},
				},
			}
			json.NewEncoder(w).Encode(response)
		}
	}))
	defer masterServer.Close()

	// Mock volume server
	volumeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "POST":
			// Store upload
			r.ParseMultipartForm(32 << 20)
			file, _, _ := r.FormFile("file")
			storedContent, _ = io.ReadAll(file)
			w.WriteHeader(http.StatusCreated)
			
		case "GET":
			// Return stored content
			w.Write(storedContent)
		}
	}))
	defer volumeServer.Close()

	parts := strings.Split(volumeServer.URL, ":")
	port := parts[len(parts)-1]
	
	storage := &SeaweedFSStorage{
		masterURL:         masterServer.URL,
		httpClient:        &http.Client{},
		volumeURLOverride: "127.0.0.1:" + port,
	}

	// Store diff
	jobID := "job-lifecycle"
	fileID, err := storage.StoreDiff(jobID, expectedDiff)
	require.NoError(t, err)
	assert.Equal(t, storedFID, fileID)
	assert.Equal(t, expectedDiff, storedContent)
	
	// Retrieve diff
	retrievedDiff, err := storage.RetrieveDiff(fileID)
	require.NoError(t, err)
	assert.Equal(t, expectedDiff, retrievedDiff)
}

func TestDeleteDiff(t *testing.T) {
	// Mock servers for delete operation
	masterServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/dir/lookup") {
			response := LookupResponse{
				VolumeID: "7",
				Locations: []Location{
					{URL: "127.0.0.1:8080"},
				},
			}
			json.NewEncoder(w).Encode(response)
		}
	}))
	defer masterServer.Close()

	volumeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "DELETE", r.Method)
		assert.Equal(t, "/7,abcdef123456", r.URL.Path)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer volumeServer.Close()

	parts := strings.Split(volumeServer.URL, ":")
	port := parts[len(parts)-1]
	
	storage := &SeaweedFSStorage{
		masterURL:         masterServer.URL,
		httpClient:        &http.Client{},
		volumeURLOverride: "127.0.0.1:" + port,
	}

	err := storage.DeleteDiff("7,abcdef123456")
	assert.NoError(t, err)
}

func TestDeleteDiffWithError(t *testing.T) {
	storage := &SeaweedFSStorage{
		masterURL:  "http://invalid-server",
		httpClient: &http.Client{},
	}

	err := storage.DeleteDiff("invalid")
	assert.Error(t, err)
}