package storage

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewSeaweedFSClient(t *testing.T) {
	tests := []struct {
		name    string
		config  SeaweedFSConfig
		wantErr bool
		errMsg  string
	}{
		{name: "valid configuration", config: SeaweedFSConfig{Master: "localhost:9333", Filer: "localhost:8888"}},
		{name: "missing master address", config: SeaweedFSConfig{Filer: "localhost:8888"}, wantErr: true, errMsg: "seaweedfs master address is required"},
		{name: "missing filer address", config: SeaweedFSConfig{Master: "localhost:9333"}, wantErr: true, errMsg: "seaweedfs filer address is required"},
		{name: "with custom timeout", config: SeaweedFSConfig{Master: "localhost:9333", Filer: "localhost:8888", Timeout: 60}},
		{name: "with custom collection", config: SeaweedFSConfig{Master: "localhost:9333", Filer: "localhost:8888", Collection: "custom-collection"}},
		{name: "with custom replication", config: SeaweedFSConfig{Master: "localhost:9333", Filer: "localhost:8888", Replication: "010"}},
		{name: "adds http scheme if missing", config: SeaweedFSConfig{Master: "master:9333", Filer: "filer:8888"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewSeaweedFSClient(tt.config)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, client)
				return
			}
			assert.NoError(t, err)
			assert.NotNil(t, client)
			if tt.config.Collection == "" {
				assert.Equal(t, "artifacts", client.collection)
			} else {
				assert.Equal(t, tt.config.Collection, client.collection)
			}
			if tt.config.Replication == "" {
				assert.Equal(t, "000", client.replication)
			} else {
				assert.Equal(t, tt.config.Replication, client.replication)
			}
			if tt.config.Timeout == 0 {
				assert.Equal(t, 30*time.Second, client.timeout)
			} else {
				assert.Equal(t, time.Duration(tt.config.Timeout)*time.Second, client.timeout)
			}
			assert.True(t, strings.HasPrefix(client.masterURL, "http://") || strings.HasPrefix(client.masterURL, "https://"))
			assert.True(t, strings.HasPrefix(client.filerURL, "http://") || strings.HasPrefix(client.filerURL, "https://"))
		})
	}
}
