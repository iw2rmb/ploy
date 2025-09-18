package orchestration

import (
	"fmt"

	consulapi "github.com/hashicorp/consul/api"
)

// ConsulTemplateClient wraps Consul client for template KV operations
type ConsulTemplateClient struct {
	client *consulapi.Client
}

// NewConsulTemplateClient creates a new Consul template client
func NewConsulTemplateClient() (*ConsulTemplateClient, error) {
	config := consulapi.DefaultConfig()
	client, err := consulapi.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create consul client: %w", err)
	}
	return &ConsulTemplateClient{client: client}, nil
}

// GetTemplate retrieves a template from Consul KV (single key bucket ploy/templates)
func (c *ConsulTemplateClient) GetTemplate(templatePath string) ([]byte, error) {
	keyPath := fmt.Sprintf("ploy/templates/%s", templatePath)
	pair, _, err := c.client.KV().Get(keyPath, nil)
	if err == nil && pair != nil && len(pair.Value) > 0 {
		return pair.Value, nil
	}
	return nil, fmt.Errorf("template not found in Consul KV: %s", keyPath)
}
