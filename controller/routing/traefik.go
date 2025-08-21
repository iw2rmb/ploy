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
	EnableTLS           bool     `json:"enable_tls"`
	CertResolver        string   `json:"cert_resolver"`
	HealthPath          string   `json:"health_path"`
	LoadBalanceMode     string   `json:"load_balance_mode"`
	StickySession       bool     `json:"sticky_session"`
	Middlewares         []string `json:"middlewares,omitempty"`
	HealthCheckInterval string   `json:"health_check_interval"`
	HealthCheckTimeout  string   `json:"health_check_timeout"`
	HealthCheckRetries  int      `json:"health_check_retries"`
	CircuitBreaker      bool     `json:"circuit_breaker"`
	RetryAttempts       int      `json:"retry_attempts"`
	RateLimit           int      `json:"rate_limit"`
	SecurityHeaders     bool     `json:"security_headers"`
}

// DefaultRouteConfig returns default routing configuration
func DefaultRouteConfig() *RouteConfig {
	return &RouteConfig{
		EnableTLS:           true,
		CertResolver:        "letsencrypt",
		HealthPath:          "/healthz",
		LoadBalanceMode:     "weighted_round_robin",
		StickySession:       false,
		Middlewares:         []string{},
		HealthCheckInterval: "10s",
		HealthCheckTimeout:  "5s",
		HealthCheckRetries:  3,
		CircuitBreaker:      true,
		RetryAttempts:       3,
		RateLimit:           50,
		SecurityHeaders:     true,
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

// ControllerRouteConfig returns optimized configuration for Ploy Controller
func ControllerRouteConfig() *RouteConfig {
	return &RouteConfig{
		EnableTLS:           true,
		CertResolver:        "letsencrypt",
		HealthPath:          "/health",
		LoadBalanceMode:     "weighted_round_robin",
		StickySession:       false,
		Middlewares:         []string{"ploy-controller-cors"}, // Use global middleware
		HealthCheckInterval: "10s",
		HealthCheckTimeout:  "5s",
		HealthCheckRetries:  3,
		CircuitBreaker:      true,
		RetryAttempts:       3,
		RateLimit:           100, // Higher rate limit for controller
		SecurityHeaders:     true,
	}
}

// RegisterController registers the Ploy Controller with enhanced load balancing
func (tr *TraefikRouter) RegisterController(allocID, allocIP string, port int) error {
	route := &AppRoute{
		App:        "ploy-controller",
		Domain:     "api.ployd.app",
		Port:       port,
		AllocID:    allocID,
		AllocIP:    allocIP,
		HealthPath: "/health",
		TLSEnabled: true,
		CreatedAt:  time.Now(),
	}
	
	config := ControllerRouteConfig()
	
	return tr.RegisterApp(route, config)
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
	
	// Advanced health check configuration
	if config.HealthPath != "" {
		tags = append(tags, fmt.Sprintf("traefik.http.services.%s.loadbalancer.healthcheck.path=%s", serviceName, config.HealthPath))
		tags = append(tags, fmt.Sprintf("traefik.http.services.%s.loadbalancer.healthcheck.interval=%s", serviceName, config.HealthCheckInterval))
		tags = append(tags, fmt.Sprintf("traefik.http.services.%s.loadbalancer.healthcheck.timeout=%s", serviceName, config.HealthCheckTimeout))
		tags = append(tags, fmt.Sprintf("traefik.http.services.%s.loadbalancer.healthcheck.retries=%d", serviceName, config.HealthCheckRetries))
		tags = append(tags, fmt.Sprintf("traefik.http.services.%s.loadbalancer.healthcheck.scheme=http", serviceName))
		tags = append(tags, fmt.Sprintf("traefik.http.services.%s.loadbalancer.healthcheck.headers.X-Health-Check=traefik", serviceName))
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
	
	// Build dynamic middleware chain
	middlewares := make([]string, 0, len(config.Middlewares)+5)
	middlewares = append(middlewares, config.Middlewares...)
	
	// Add rate limiting if configured
	if config.RateLimit > 0 {
		rateLimitName := fmt.Sprintf("%s-ratelimit", route.App)
		tags = append(tags, fmt.Sprintf("traefik.http.middlewares.%s.ratelimit.burst=%d", rateLimitName, config.RateLimit*2))
		tags = append(tags, fmt.Sprintf("traefik.http.middlewares.%s.ratelimit.average=%d", rateLimitName, config.RateLimit))
		tags = append(tags, fmt.Sprintf("traefik.http.middlewares.%s.ratelimit.period=1m", rateLimitName))
		middlewares = append(middlewares, rateLimitName)
	}
	
	// Add security headers if enabled
	if config.SecurityHeaders {
		securityName := fmt.Sprintf("%s-security", route.App)
		tags = append(tags, fmt.Sprintf("traefik.http.middlewares.%s.headers.sslredirect=true", securityName))
		tags = append(tags, fmt.Sprintf("traefik.http.middlewares.%s.headers.forcestsheader=true", securityName))
		tags = append(tags, fmt.Sprintf("traefik.http.middlewares.%s.headers.stsincludesubdomains=true", securityName))
		tags = append(tags, fmt.Sprintf("traefik.http.middlewares.%s.headers.stsseconds=63072000", securityName))
		tags = append(tags, fmt.Sprintf("traefik.http.middlewares.%s.headers.customresponseheaders.X-Content-Type-Options=nosniff", securityName))
		tags = append(tags, fmt.Sprintf("traefik.http.middlewares.%s.headers.customresponseheaders.X-Frame-Options=DENY", securityName))
		middlewares = append(middlewares, securityName)
	}
	
	// Add circuit breaker if enabled
	if config.CircuitBreaker {
		circuitBreakerName := fmt.Sprintf("%s-circuitbreaker", route.App)
		tags = append(tags, fmt.Sprintf("traefik.http.middlewares.%s.circuitbreaker.expression=NetworkErrorRatio() > 0.30", circuitBreakerName))
		tags = append(tags, fmt.Sprintf("traefik.http.middlewares.%s.circuitbreaker.checkperiod=10s", circuitBreakerName))
		tags = append(tags, fmt.Sprintf("traefik.http.middlewares.%s.circuitbreaker.fallbackduration=30s", circuitBreakerName))
		middlewares = append(middlewares, circuitBreakerName)
	}
	
	// Add retry middleware if configured
	if config.RetryAttempts > 0 {
		retryName := fmt.Sprintf("%s-retry", route.App)
		tags = append(tags, fmt.Sprintf("traefik.http.middlewares.%s.retry.attempts=%d", retryName, config.RetryAttempts))
		tags = append(tags, fmt.Sprintf("traefik.http.middlewares.%s.retry.initialinterval=100ms", retryName))
		middlewares = append(middlewares, retryName)
	}
	
	// Apply middleware chain to router
	if len(middlewares) > 0 {
		middlewareChain := strings.Join(middlewares, ",")
		tags = append(tags, fmt.Sprintf("traefik.http.routers.%s.middlewares=%s", routerName, middlewareChain))
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