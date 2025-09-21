package routing

import "time"

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
