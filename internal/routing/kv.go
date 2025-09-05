package routing

import (
	"encoding/json"
	"fmt"
	"time"

	consulapi "github.com/hashicorp/consul/api"
)

// DomainRoute represents a persisted domain mapping entry for an app
type DomainRoute struct {
	App        string    `json:"app"`
	Domain     string    `json:"domain"`
	Port       int       `json:"port"`
	AllocID    string    `json:"alloc_id"`
	AllocIP    string    `json:"alloc_ip"`
	HealthPath string    `json:"health_path"`
	Aliases    []string  `json:"aliases,omitempty"`
	TLSEnabled bool      `json:"tls_enabled"`
	CreatedAt  time.Time `json:"created_at"`
}

// SaveAppRoute stores or updates the domain mapping for a given app and domain
func SaveAppRoute(consul *consulapi.Client, route DomainRoute) error {
	if consul == nil {
		return fmt.Errorf("consul client is nil")
	}
	key := fmt.Sprintf("ploy/domains/%s", route.App)
	existing, err := GetAppRoutes(consul, route.App)
	if err != nil {
		existing = make(map[string]DomainRoute)
	}
	existing[route.Domain] = route
	data, err := json.Marshal(existing)
	if err != nil {
		return fmt.Errorf("marshal domain mappings: %w", err)
	}
	pair := &consulapi.KVPair{Key: key, Value: data}
	if _, err := consul.KV().Put(pair, nil); err != nil {
		return fmt.Errorf("put domain mappings: %w", err)
	}
	return nil
}

// GetAppRoutes loads all domain mappings for a given app
func GetAppRoutes(consul *consulapi.Client, app string) (map[string]DomainRoute, error) {
	if consul == nil {
		return nil, fmt.Errorf("consul client is nil")
	}
	key := fmt.Sprintf("ploy/domains/%s", app)
	pair, _, err := consul.KV().Get(key, nil)
	if err != nil {
		return nil, fmt.Errorf("get domain mappings: %w", err)
	}
	if pair == nil {
		return make(map[string]DomainRoute), nil
	}
	var mappings map[string]DomainRoute
	if err := json.Unmarshal(pair.Value, &mappings); err != nil {
		return nil, fmt.Errorf("unmarshal domain mappings: %w", err)
	}
	return mappings, nil
}
