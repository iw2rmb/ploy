package routing

import (
    "fmt"
    "strings"
)

// AppRoute represents a routed application configuration
type AppRoute struct {
    App        string
    Domain     string
    Port       int
    AllocID    string
    AllocIP    string
    HealthPath string
    Aliases    []string
}

// RouteConfig holds configuration for app routing
type RouteConfig struct {
    EnableTLS           bool
    CertResolver        string
    HealthPath          string
    HealthCheckInterval string
    HealthCheckTimeout  string
    LoadBalanceMode     string
    StickySession       bool
    Middlewares         []string
    RetryAttempts       int
    RateLimit           int
    SecurityHeaders     bool
}

// BuildTraefikTags constructs Traefik configuration tags (shared helper)
func BuildTraefikTags(route *AppRoute, config *RouteConfig) []string {
    tags := []string{"traefik.enable=true"}

    // Router configuration
    routerName := fmt.Sprintf("%s-router", route.App)
    tags = append(tags, fmt.Sprintf("traefik.http.routers.%s.rule=Host(`%s`)", routerName, route.Domain))
    tags = append(tags, fmt.Sprintf("traefik.http.routers.%s.entrypoints=websecure", routerName))

    // Domain aliases -> merge into a single Host() rule
    if len(route.Aliases) > 0 {
        domains := append([]string{route.Domain}, route.Aliases...)
        hostsRule := fmt.Sprintf("Host(`%s`)", strings.Join(domains, "`,`"))
        tags = append(tags, fmt.Sprintf("traefik.http.routers.%s.rule=%s", routerName, hostsRule))
    }

    // TLS
    if config != nil && config.EnableTLS {
        tags = append(tags, fmt.Sprintf("traefik.http.routers.%s.tls=true", routerName))
        if config.CertResolver != "" {
            tags = append(tags, fmt.Sprintf("traefik.http.routers.%s.tls.certresolver=%s", routerName, config.CertResolver))
        }
    }

    // Service configuration
    serviceName := fmt.Sprintf("%s-service", route.App)
    tags = append(tags, fmt.Sprintf("traefik.http.services.%s.loadbalancer.server.port=%d", serviceName, route.Port))

    // Health checks
    if config != nil && config.HealthPath != "" {
        tags = append(tags, fmt.Sprintf("traefik.http.services.%s.loadbalancer.healthcheck.path=%s", serviceName, config.HealthPath))
        if config.HealthCheckInterval != "" {
            tags = append(tags, fmt.Sprintf("traefik.http.services.%s.loadbalancer.healthcheck.interval=%s", serviceName, config.HealthCheckInterval))
        }
        if config.HealthCheckTimeout != "" {
            tags = append(tags, fmt.Sprintf("traefik.http.services.%s.loadbalancer.healthcheck.timeout=%s", serviceName, config.HealthCheckTimeout))
        }
        tags = append(tags, fmt.Sprintf("traefik.http.services.%s.loadbalancer.healthcheck.scheme=http", serviceName))
        tags = append(tags, fmt.Sprintf("traefik.http.services.%s.loadbalancer.healthcheck.headers.X-Health-Check=traefik", serviceName))
    }

    // Load balancing strategy
    if config != nil && config.LoadBalanceMode != "" {
        tags = append(tags, fmt.Sprintf("traefik.http.services.%s.loadbalancer.strategy=%s", serviceName, config.LoadBalanceMode))
    }

    // Sticky sessions
    if config != nil && config.StickySession {
        tags = append(tags, fmt.Sprintf("traefik.http.services.%s.loadbalancer.sticky.cookie=true", serviceName))
        tags = append(tags, fmt.Sprintf("traefik.http.services.%s.loadbalancer.sticky.cookie.name=%s-session", serviceName, route.App))
    }

    // Middlewares (rate limit + security headers are optional and appended to chain)
    // For now, just materialize their definitions; chaining can be applied by the caller route layer.
    if config != nil && config.RateLimit > 0 {
        rateLimitName := fmt.Sprintf("%s-ratelimit", route.App)
        tags = append(tags, fmt.Sprintf("traefik.http.middlewares.%s.ratelimit.burst=%d", rateLimitName, config.RateLimit*2))
        tags = append(tags, fmt.Sprintf("traefik.http.middlewares.%s.ratelimit.average=%d", rateLimitName, config.RateLimit))
        tags = append(tags, fmt.Sprintf("traefik.http.middlewares.%s.ratelimit.period=1m", rateLimitName))
    }
    if config != nil && config.SecurityHeaders {
        securityName := fmt.Sprintf("%s-security", route.App)
        tags = append(tags, fmt.Sprintf("traefik.http.middlewares.%s.headers.sslredirect=true", securityName))
        tags = append(tags, fmt.Sprintf("traefik.http.middlewares.%s.headers.forcestsheader=true", securityName))
        tags = append(tags, fmt.Sprintf("traefik.http.middlewares.%s.headers.stsincludesubdomains=true", securityName))
        tags = append(tags, fmt.Sprintf("traefik.http.middlewares.%s.headers.stsseconds=63072000", securityName))
    }

    return tags
}

