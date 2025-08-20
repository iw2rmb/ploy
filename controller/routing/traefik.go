package routing

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	consulapi "github.com/hashicorp/consul/api"
)

// TraefikRouter handles app routing via Traefik and Consul integration
type TraefikRouter struct {
	consul *consulapi.Client
}

// NewTraefikRouter creates a new Traefik router instance
func NewTraefikRouter(consulAddr string) (*TraefikRouter, error) {
	config := consulapi.DefaultConfig()
	if consulAddr != "" {
		config.Address = consulAddr
	}
	
	client, err := consulapi.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Consul client: %w", err)
	}
	
	return &TraefikRouter{consul: client}, nil
}

// GetConsulClient returns the underlying Consul client for external access
func (tr *TraefikRouter) GetConsulClient() *consulapi.Client {
	return tr.consul
}

// AppRoute represents a routed application configuration
type AppRoute struct {
	App         string   `json:"app"`
	Domain      string   `json:"domain"`
	Port        int      `json:"port"`
	AllocID     string   `json:"alloc_id"`
	AllocIP     string   `json:"alloc_ip"`
	HealthPath  string   `json:"health_path"`
	Aliases     []string `json:"aliases,omitempty"`
	TLSEnabled  bool     `json:"tls_enabled"`
	CreatedAt   time.Time `json:"created_at"`
}

// RouteConfig holds configuration for app routing
type RouteConfig struct {
	EnableTLS       bool     `json:"enable_tls"`
	CertResolver    string   `json:"cert_resolver"`
	HealthPath      string   `json:"health_path"`
	LoadBalanceMode string   `json:"load_balance_mode"`
	StickySession   bool     `json:"sticky_session"`
	Middlewares     []string `json:"middlewares,omitempty"`
}

// DefaultRouteConfig returns default routing configuration
func DefaultRouteConfig() *RouteConfig {
	return &RouteConfig{
		EnableTLS:       true,
		CertResolver:    "letsencrypt",
		HealthPath:      "/healthz",
		LoadBalanceMode: "round_robin",
		StickySession:   false,
		Middlewares:     []string{},
	}
}

// RegisterApp registers an app with Traefik routing
func (tr *TraefikRouter) RegisterApp(route *AppRoute, config *RouteConfig) error {
	if config == nil {
		config = DefaultRouteConfig()
	}
	
	// Generate unique service ID
	serviceID := fmt.Sprintf("%s-%s", route.App, route.AllocID)
	
	// Validate route before registration
	if err := tr.validateRoute(route); err != nil {
		return fmt.Errorf("invalid route configuration: %w", err)
	}
	
	// Build Traefik tags for routing configuration
	tags := tr.buildTraefikTags(route, config)
	
	// Register service in Consul with Traefik tags
	registration := &consulapi.AgentServiceRegistration{
		ID:   serviceID,
		Name: route.App,
		Port: route.Port,
		Tags: tags,
		Address: route.AllocIP,
		Check: &consulapi.AgentServiceCheck{
			HTTP:     fmt.Sprintf("http://%s:%d%s", route.AllocIP, route.Port, config.HealthPath),
			Interval: "10s",
			Timeout:  "3s",
		},
		Meta: map[string]string{
			"app":         route.App,
			"domain":      route.Domain,
			"alloc_id":    route.AllocID,
			"created_at":  route.CreatedAt.Format(time.RFC3339),
			"managed_by":  "ploy",
		},
	}
	
	log.Printf("Registering Traefik route for app %s: %s -> %s:%d", 
		route.App, route.Domain, route.AllocIP, route.Port)
	
	if err := tr.consul.Agent().ServiceRegister(registration); err != nil {
		return fmt.Errorf("failed to register service with Consul: %w", err)
	}
	
	// Store domain mapping for persistence
	if err := tr.storeDomainMapping(route); err != nil {
		log.Printf("Warning: failed to store domain mapping for %s: %v", route.App, err)
	}
	
	return nil
}

// buildTraefikTags constructs Traefik configuration tags
func (tr *TraefikRouter) buildTraefikTags(route *AppRoute, config *RouteConfig) []string {
	tags := []string{
		"traefik.enable=true",
	}
	
	// Router configuration
	routerName := fmt.Sprintf("%s-router", route.App)
	tags = append(tags, fmt.Sprintf("traefik.http.routers.%s.rule=Host(`%s`)", routerName, route.Domain))
	tags = append(tags, fmt.Sprintf("traefik.http.routers.%s.entrypoints=websecure", routerName))
	
	// Add domain aliases if provided
	if len(route.Aliases) > 0 {
		domains := append([]string{route.Domain}, route.Aliases...)
		hostsRule := fmt.Sprintf("Host(`%s`)", strings.Join(domains, "`,`"))
		tags = append(tags, fmt.Sprintf("traefik.http.routers.%s.rule=%s", routerName, hostsRule))
	}
	
	// TLS configuration
	if config.EnableTLS {
		tags = append(tags, fmt.Sprintf("traefik.http.routers.%s.tls=true", routerName))
		if config.CertResolver != "" {
			tags = append(tags, fmt.Sprintf("traefik.http.routers.%s.tls.certresolver=%s", routerName, config.CertResolver))
		}
	}
	
	// Service configuration
	serviceName := fmt.Sprintf("%s-service", route.App)
	tags = append(tags, fmt.Sprintf("traefik.http.services.%s.loadbalancer.server.port=%d", serviceName, route.Port))
	
	// Health check configuration
	if config.HealthPath != "" {
		tags = append(tags, fmt.Sprintf("traefik.http.services.%s.loadbalancer.healthcheck.path=%s", serviceName, config.HealthPath))
		tags = append(tags, fmt.Sprintf("traefik.http.services.%s.loadbalancer.healthcheck.interval=10s", serviceName))
	}
	
	// Load balancing configuration
	if config.LoadBalanceMode != "" {
		tags = append(tags, fmt.Sprintf("traefik.http.services.%s.loadbalancer.strategy=%s", serviceName, config.LoadBalanceMode))
	}
	
	// Sticky sessions
	if config.StickySession {
		tags = append(tags, fmt.Sprintf("traefik.http.services.%s.loadbalancer.sticky.cookie=true", serviceName))
		tags = append(tags, fmt.Sprintf("traefik.http.services.%s.loadbalancer.sticky.cookie.name=%s-session", serviceName, route.App))
	}
	
	// Middlewares
	if len(config.Middlewares) > 0 {
		middlewares := strings.Join(config.Middlewares, ",")
		tags = append(tags, fmt.Sprintf("traefik.http.routers.%s.middlewares=%s", routerName, middlewares))
	}
	
	return tags
}

// UnregisterApp removes app routing from Traefik
func (tr *TraefikRouter) UnregisterApp(appName, allocID string) error {
	serviceID := fmt.Sprintf("%s-%s", appName, allocID)
	
	log.Printf("Unregistering Traefik route for app %s (alloc: %s)", appName, allocID)
	
	if err := tr.consul.Agent().ServiceDeregister(serviceID); err != nil {
		return fmt.Errorf("failed to deregister service from Consul: %w", err)
	}
	
	return nil
}

// GetAppRoutes returns all routes for a specific app
func (tr *TraefikRouter) GetAppRoutes(appName string) ([]*AppRoute, error) {
	services, _, err := tr.consul.Catalog().Service(appName, "", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to query Consul catalog: %w", err)
	}
	
	var routes []*AppRoute
	for _, service := range services {
		if managedBy, ok := service.ServiceMeta["managed_by"]; !ok || managedBy != "ploy" {
			continue
		}
		
		route := &AppRoute{
			App:     service.ServiceName,
			Domain:  service.ServiceMeta["domain"],
			Port:    service.ServicePort,
			AllocID: service.ServiceMeta["alloc_id"],
			AllocIP: service.ServiceAddress,
		}
		
		if createdAtStr, ok := service.ServiceMeta["created_at"]; ok {
			if createdAt, err := time.Parse(time.RFC3339, createdAtStr); err == nil {
				route.CreatedAt = createdAt
			}
		}
		
		routes = append(routes, route)
	}
	
	return routes, nil
}

// storeDomainMapping stores domain mapping in Consul KV for persistence
func (tr *TraefikRouter) storeDomainMapping(route *AppRoute) error {
	key := fmt.Sprintf("ploy/domains/%s", route.App)
	
	// Get existing mappings
	existing, err := tr.getDomainMappings(route.App)
	if err != nil {
		existing = make(map[string]*AppRoute)
	}
	
	// Add/update current route
	existing[route.Domain] = route
	
	// Store updated mappings
	data, err := json.Marshal(existing)
	if err != nil {
		return fmt.Errorf("failed to marshal domain mappings: %w", err)
	}
	
	pair := &consulapi.KVPair{
		Key:   key,
		Value: data,
	}
	
	_, err = tr.consul.KV().Put(pair, nil)
	if err != nil {
		return fmt.Errorf("failed to store domain mapping in Consul KV: %w", err)
	}
	
	return nil
}

// getDomainMappings retrieves domain mappings from Consul KV
func (tr *TraefikRouter) getDomainMappings(appName string) (map[string]*AppRoute, error) {
	key := fmt.Sprintf("ploy/domains/%s", appName)
	
	pair, _, err := tr.consul.KV().Get(key, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get domain mappings: %w", err)
	}
	
	if pair == nil {
		return make(map[string]*AppRoute), nil
	}
	
	var mappings map[string]*AppRoute
	if err := json.Unmarshal(pair.Value, &mappings); err != nil {
		return nil, fmt.Errorf("failed to unmarshal domain mappings: %w", err)
	}
	
	return mappings, nil
}

// HealthCheck verifies Traefik service is running and accessible
func (tr *TraefikRouter) HealthCheck() error {
	// Check if Traefik service is registered in Consul
	services, _, err := tr.consul.Catalog().Service("traefik", "", nil)
	if err != nil {
		return fmt.Errorf("failed to query Traefik service: %w", err)
	}
	
	if len(services) == 0 {
		return fmt.Errorf("no Traefik service found in Consul catalog")
	}
	
	// Check service health
	health, _, err := tr.consul.Health().Service("traefik", "", true, nil)
	if err != nil {
		return fmt.Errorf("failed to check Traefik health: %w", err)
	}
	
	if len(health) == 0 {
		return fmt.Errorf("no healthy Traefik instances found")
	}
	
	log.Printf("Traefik health check passed: %d healthy instances", len(health))
	return nil
}

// validateRoute validates an AppRoute configuration
func (tr *TraefikRouter) validateRoute(route *AppRoute) error {
	if route.App == "" {
		return fmt.Errorf("app name cannot be empty")
	}
	
	if route.Domain == "" {
		return fmt.Errorf("domain cannot be empty")
	}
	
	if route.Port <= 0 || route.Port > 65535 {
		return fmt.Errorf("invalid port: %d", route.Port)
	}
	
	if route.AllocID == "" {
		return fmt.Errorf("allocation ID cannot be empty")
	}
	
	if route.AllocIP == "" {
		return fmt.Errorf("allocation IP cannot be empty")
	}
	
	// Validate domain format
	if err := validateDomainName(route.Domain); err != nil {
		return fmt.Errorf("invalid domain %s: %w", route.Domain, err)
	}
	
	// Validate aliases
	for _, alias := range route.Aliases {
		if err := validateDomainName(alias); err != nil {
			return fmt.Errorf("invalid alias domain %s: %w", alias, err)
		}
	}
	
	return nil
}

// validateDomainName validates a domain name format
func validateDomainName(domain string) error {
	if domain == "" {
		return fmt.Errorf("domain cannot be empty")
	}
	
	if strings.Contains(domain, " ") {
		return fmt.Errorf("domain cannot contain spaces")
	}
	
	if !strings.Contains(domain, ".") {
		return fmt.Errorf("domain must contain at least one dot")
	}
	
	if strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") {
		return fmt.Errorf("domain cannot start or end with a dot")
	}
	
	if len(domain) > 253 {
		return fmt.Errorf("domain too long (max 253 characters)")
	}
	
	return nil
}