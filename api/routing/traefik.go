package routing

import (
	"fmt"
	"log"
	"strings"
	"time"

	consulapi "github.com/hashicorp/consul/api"
	irouting "github.com/iw2rmb/ploy/internal/routing"
	"github.com/iw2rmb/ploy/internal/utils"
)

// TraefikRouter handles app routing via Traefik and Consul integration
type TraefikRouter struct {
	consul             *consulapi.Client
	platformAppsDomain string
	platformDomain     string
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

	// Get platform apps domain from environment (e.g., dev.ployd.app)
	platformAppsDomain := utils.Getenv("PLOY_APPS_DOMAIN", "")
	if platformAppsDomain == "" {
		platformAppsDomain = "ployd.app" // Default fallback
		log.Printf("PLOY_APPS_DOMAIN not set, using default: %s", platformAppsDomain)
	}

	// Get platform domain from environment (e.g., dev.ployman.app)
	platformDomain := utils.Getenv("PLOY_PLATFORM_DOMAIN", "")
	if platformDomain == "" {
		platformDomain = "ployman.app"
		log.Printf("PLOY_PLATFORM_DOMAIN not set, using default: %s", platformDomain)
	}

	return &TraefikRouter{
		consul:             client,
		platformAppsDomain: platformAppsDomain,
		platformDomain:     platformDomain,
	}, nil
}

// GetConsulClient returns the underlying Consul client for external access
func (tr *TraefikRouter) GetConsulClient() *consulapi.Client {
	return tr.consul
}

// GetPlatformAppsDomain returns the configured platform apps domain
func (tr *TraefikRouter) GetPlatformAppsDomain() string {
	return tr.platformAppsDomain
}

// GenerateAppDomain generates a platform subdomain for an app
func (tr *TraefikRouter) GenerateAppDomain(appName string) string {
	return fmt.Sprintf("%s.%s", appName, tr.platformAppsDomain)
}

// GenerateControllerDomain generates the controller domain
func (tr *TraefikRouter) GenerateControllerDomain() string {
	return fmt.Sprintf("api.%s", tr.platformDomain)
}

// IsPlatformSubdomain checks if a domain is a platform subdomain
func (tr *TraefikRouter) IsPlatformSubdomain(domain string) bool {
	return strings.HasSuffix(domain, "."+tr.platformAppsDomain) &&
		strings.Count(domain, ".") == strings.Count(tr.platformAppsDomain, ".")+1
}

// AppRoute represents a routed application configuration
type AppRoute struct {
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
		CertResolver:        "default-acme", // Let Traefik issue certificates automatically per domain
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

// PlatformAppRouteConfig returns optimized configuration for platform subdomain apps
func PlatformAppRouteConfig() *RouteConfig {
	return &RouteConfig{
		EnableTLS:           true,
		CertResolver:        "default-acme", // Shared ACME resolver handles platform domains
		HealthPath:          "/healthz",
		LoadBalanceMode:     "weighted_round_robin",
		StickySession:       false,
		Middlewares:         []string{"platform-security-headers"},
		HealthCheckInterval: "10s",
		HealthCheckTimeout:  "3s", // Faster health checks for platform apps
		HealthCheckRetries:  3,
		CircuitBreaker:      true,
		RetryAttempts:       2,   // Fewer retries for platform apps
		RateLimit:           100, // Higher rate limit for platform apps
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
		ID:      serviceID,
		Name:    route.App,
		Port:    route.Port,
		Tags:    tags,
		Address: route.AllocIP,
		Check: &consulapi.AgentServiceCheck{
			HTTP:     fmt.Sprintf("http://%s:%d%s", route.AllocIP, route.Port, config.HealthPath),
			Interval: "10s",
			Timeout:  "3s",
		},
		Meta: map[string]string{
			"app":        route.App,
			"domain":     route.Domain,
			"alloc_id":   route.AllocID,
			"created_at": route.CreatedAt.Format(time.RFC3339),
			"managed_by": "ploy",
		},
	}

	log.Printf("Registering Traefik route for app %s: %s -> %s:%d",
		route.App, route.Domain, route.AllocIP, route.Port)

	if err := tr.consul.Agent().ServiceRegister(registration); err != nil {
		return fmt.Errorf("failed to register service with Consul: %w", err)
	}

	// Store domain mapping for persistence
	if err := irouting.SaveAppRoute(tr.consul, irouting.DomainRoute{
		App:        route.App,
		Domain:     route.Domain,
		Port:       route.Port,
		AllocID:    route.AllocID,
		AllocIP:    route.AllocIP,
		HealthPath: route.HealthPath,
		Aliases:    route.Aliases,
		TLSEnabled: route.TLSEnabled,
		CreatedAt:  route.CreatedAt,
	}); err != nil {
		log.Printf("Warning: failed to store domain mapping for %s: %v", route.App, err)
	}

	return nil
}

// ControllerRouteConfig returns optimized configuration for Ploy Controller
func ControllerRouteConfig() *RouteConfig {
	return &RouteConfig{
		EnableTLS:           true,
		CertResolver:        "default-acme",
		HealthPath:          "/health",
		LoadBalanceMode:     "weighted_round_robin",
		StickySession:       false,
		Middlewares:         []string{"ploy-api-cors"}, // Use global middleware
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
	controllerDomain := tr.GenerateControllerDomain()

	route := &AppRoute{
		App:        "ploy-api",
		Domain:     controllerDomain,
		Port:       port,
		AllocID:    allocID,
		AllocIP:    allocIP,
		HealthPath: "/health",
		TLSEnabled: true,
		CreatedAt:  time.Now(),
	}

	config := ControllerRouteConfig()

	log.Printf("Registering Ploy Controller at: %s (platform domain: %s)", controllerDomain, tr.platformAppsDomain)

	return tr.RegisterApp(route, config)
}

// RegisterAppWithPlatformDomain registers an app with automatically generated platform subdomain
func (tr *TraefikRouter) RegisterAppWithPlatformDomain(appName, allocID, allocIP string, port int, config *RouteConfig) error {
	appDomain := tr.GenerateAppDomain(appName)

	route := &AppRoute{
		App:        appName,
		Domain:     appDomain,
		Port:       port,
		AllocID:    allocID,
		AllocIP:    allocIP,
		HealthPath: "/healthz", // Standard health check path for apps
		TLSEnabled: true,       // Always enable TLS for platform subdomains
		CreatedAt:  time.Now(),
	}

	if config == nil {
		config = PlatformAppRouteConfig() // Use optimized config for platform apps
	}

	log.Printf("Registering app %s at platform subdomain: %s", appName, appDomain)

	return tr.RegisterApp(route, config)
}

// buildTraefikTags constructs Traefik configuration tags
func (tr *TraefikRouter) buildTraefikTags(route *AppRoute, config *RouteConfig) []string {
	base := irouting.BuildTraefikTags(
		&irouting.AppRoute{
			App:        route.App,
			Domain:     route.Domain,
			Port:       route.Port,
			AllocID:    route.AllocID,
			AllocIP:    route.AllocIP,
			HealthPath: route.HealthPath,
			Aliases:    route.Aliases,
		},
		&irouting.RouteConfig{
			EnableTLS:           config.EnableTLS,
			CertResolver:        config.CertResolver,
			HealthPath:          config.HealthPath,
			HealthCheckInterval: config.HealthCheckInterval,
			HealthCheckTimeout:  config.HealthCheckTimeout,
			LoadBalanceMode:     config.LoadBalanceMode,
			StickySession:       config.StickySession,
			Middlewares:         config.Middlewares,
			RetryAttempts:       config.RetryAttempts,
			RateLimit:           config.RateLimit,
			SecurityHeaders:     config.SecurityHeaders,
		},
	)

	// Compose router middleware chain tag (if any)
	routerName := fmt.Sprintf("%s-router", route.App)
	middlewares := make([]string, 0, len(config.Middlewares)+5)
	middlewares = append(middlewares, config.Middlewares...)
	if config.RateLimit > 0 {
		middlewares = append(middlewares, fmt.Sprintf("%s-ratelimit", route.App))
	}
	if config.SecurityHeaders {
		middlewares = append(middlewares, fmt.Sprintf("%s-security", route.App))
	}
	if config.CircuitBreaker {
		middlewares = append(middlewares, fmt.Sprintf("%s-circuitbreaker", route.App))
		// circuit breaker definitions remain local to API build for now
		base = append(base, fmt.Sprintf("traefik.http.middlewares.%s.circuitbreaker.expression=NetworkErrorRatio() > 0.30", route.App+"-circuitbreaker"))
		base = append(base, fmt.Sprintf("traefik.http.middlewares.%s.circuitbreaker.checkperiod=10s", route.App+"-circuitbreaker"))
		base = append(base, fmt.Sprintf("traefik.http.middlewares.%s.circuitbreaker.fallbackduration=30s", route.App+"-circuitbreaker"))
	}
	if config.RetryAttempts > 0 {
		middlewares = append(middlewares, fmt.Sprintf("%s-retry", route.App))
		base = append(base, fmt.Sprintf("traefik.http.middlewares.%s.retry.attempts=%d", route.App+"-retry", config.RetryAttempts))
		base = append(base, fmt.Sprintf("traefik.http.middlewares.%s.retry.initialinterval=100ms", route.App+"-retry"))
	}
	if len(middlewares) > 0 {
		base = append(base, fmt.Sprintf("traefik.http.routers.%s.middlewares=%s", routerName, strings.Join(middlewares, ",")))
	}
	return base
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
// Domain mapping storage moved to internal/routing helpers (SaveAppRoute/GetAppRoutes)

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
