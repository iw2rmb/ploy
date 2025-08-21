job "traefik-system" {
  datacenters = ["dc1"]
  type = "system"  # Runs on every Nomad client node
  priority = 90    # High priority for infrastructure service
  
  constraint {
    attribute = "${attr.kernel.name}"
    value = "linux"
  }
  
  group "traefik" {
    count = 1
    
    # Restart policy for critical infrastructure
    restart {
      attempts = 3
      interval = "5m"
      delay = "15s"
      mode = "fail"
    }
    
    # Update strategy for rolling updates
    update {
      max_parallel = 1
      min_healthy_time = "30s"
      healthy_deadline = "2m"
      progress_deadline = "5m"
      auto_revert = true
    }
    
    network {
      port "http" {
        static = 80
        to = 80
      }
      port "https" {
        static = 443
        to = 443
      }
      port "admin" {
        static = 8080
        to = 8080
      }
      port "metrics" {
        static = 8082
        to = 8082
      }
    }
    
    # Consul service registration for Traefik
    service {
      name = "traefik"
      port = "admin"
      tags = [
        "traefik",
        "load-balancer",
        "ingress",
        "ssl-termination"
      ]
      
      check {
        type = "http"
        path = "/ping"
        port = "admin"
        interval = "10s"
        timeout = "3s"
      }
      
      check {
        type = "http"
        path = "/api/http/routers"
        port = "admin"
        interval = "30s"
        timeout = "5s"
        check_restart {
          limit = 3
          grace = "30s"
        }
      }
    }
    
    # Metrics endpoint for monitoring
    service {
      name = "traefik-metrics"
      port = "metrics"
      tags = [
        "metrics",
        "prometheus"
      ]
      
      check {
        type = "http"
        path = "/metrics"
        port = "metrics"
        interval = "30s"
        timeout = "5s"
      }
    }
    
    task "traefik" {
      driver = "docker"
      
      config {
        image = "traefik:v3.0"
        network_mode = "host"
        
        ports = ["http", "https", "admin", "metrics"]
        
        mount {
          type = "bind"
          source = "local/traefik.yml"
          target = "/etc/traefik/traefik.yml"
          readonly = true
        }
        
        mount {
          type = "bind"
          source = "local/dynamic"
          target = "/etc/traefik/dynamic"
          readonly = true
        }
        
        # Mount for static Traefik configuration files
        mount {
          type = "bind"
          source = "local/static"
          target = "/etc/traefik/static"
          readonly = true
        }
        
        # Volume for Let's Encrypt certificates
        volumes = [
          "traefik-acme:/data"
        ]
      }
      
      # Main Traefik configuration
      template {
        data = <<EOF
# Traefik v3 Configuration for Ploy PaaS
global:
  checkNewVersion: false
  sendAnonymousUsage: false

log:
  level: INFO
  filePath: "/var/log/traefik.log"
  format: json

accessLog:
  filePath: "/var/log/traefik-access.log"
  format: json

# API and Dashboard
api:
  dashboard: true
  debug: false
  insecure: false

# Metrics
metrics:
  prometheus:
    addEntryPointsLabels: true
    addRoutersLabels: true
    addServicesLabels: true
    buckets:
      - 0.1
      - 0.3
      - 1.2
      - 5.0

# Ping endpoint for health checks
ping:
  entryPoint: "admin"

# Entry Points
entryPoints:
  web:
    address: ":80"
    http:
      redirections:
        entrypoint:
          to: websecure
          scheme: https
          permanent: true
  
  websecure:
    address: ":443"
    http:
      tls:
        options: default
  
  admin:
    address: ":8080"
  
  metrics:
    address: ":8082"

# Consul Provider for Service Discovery
providers:
  consul:
    endpoints:
      - "127.0.0.1:8500"
    exposedByDefault: false
    watch: true
    
  consulCatalog:
    endpoints:
      - "127.0.0.1:8500"
    exposedByDefault: false
    prefix: traefik
    watch: true
    connectAware: true
    
  # File provider for dynamic configuration
  file:
    directory: "/etc/traefik/dynamic"
    watch: true

# Certificate Resolvers (Let's Encrypt)
certificatesResolvers:
  letsencrypt:
    acme:
      email: admin@ployd.app
      storage: /data/acme.json
      caServer: https://acme-v02.api.letsencrypt.org/directory
      
      # HTTP Challenge (default)
      httpChallenge:
        entryPoint: web
      
      # DNS Challenge for wildcard certificates
      # Uncomment and configure for wildcard *.ployd.app support
      # dnsChallenge:
      #   provider: cloudflare
      #   delayBeforeCheck: 0
      #   resolvers:
      #     - "1.1.1.1:53"
      #     - "8.8.8.8:53"

# TLS Options
tls:
  options:
    default:
      sslProtocols:
        - "TLSv1.2"
        - "TLSv1.3"
      cipherSuites:
        - "TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384"
        - "TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305"
        - "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256"
      curvePreferences:
        - CurveP521
        - CurveP384
      minVersion: "VersionTLS12"

# Server Transport for backend connections
serversTransport:
  default:
    insecureSkipVerify: false
    maxIdleConnsPerHost: 10
    forwardingTimeouts:
      dialTimeout: 30s
      responseHeaderTimeout: 0s
      idleConnTimeout: 90s
EOF
        destination = "local/traefik.yml"
        perms = "644"
      }
      
      # Controller load balancer configuration - copied from platform/traefik/controller-load-balancer.yml
      template {
        data = <<EOF
# Traefik Dynamic Configuration for Ploy Controller Load Balancing
http:
  middlewares:
    ploy-controller-ratelimit:
      rateLimit:
        burst: 100
        period: "1m"
        average: 50
        sourceCriterion:
          requestHeaderName: "X-Forwarded-For"
          requestHost: true
    
    ploy-controller-security:
      headers:
        sslRedirect: true
        forceSTSHeader: true
        stsIncludeSubdomains: true
        stsPreload: true
        stsSeconds: 63072000
        customRequestHeaders:
          X-Forwarded-Proto: "https"
        customResponseHeaders:
          X-Content-Type-Options: "nosniff"
          X-Frame-Options: "DENY"
          X-XSS-Protection: "1; mode=block"
          Referrer-Policy: "strict-origin-when-cross-origin"
        contentTypeNosniff: true
        browserXssFilter: true
        frameOptions: "DENY"
    
    ploy-controller-circuit-breaker:
      circuitBreaker:
        expression: "NetworkErrorRatio() > 0.30 || ResponseCodeRatio(500, 600, 0, 600) > 0.25"
        checkPeriod: "10s"
        fallbackDuration: "30s"
        recoveryDuration: "60s"
    
    ploy-controller-retry:
      retry:
        attempts: 3
        initialInterval: "100ms"
    
    ploy-controller-compress:
      compress:
        excludedContentTypes:
          - "text/event-stream"
        minResponseBodyBytes: 1024

  routers:
    ploy-controller-api:
      rule: "Host(`api.ployd.app`) && PathPrefix(`/v1`)"
      entryPoints:
        - "websecure"
      service: "ploy-controller@consulcatalog"
      middlewares:
        - "ploy-controller-ratelimit"
        - "ploy-controller-security"
        - "ploy-controller-circuit-breaker"
        - "ploy-controller-retry"
        - "ploy-controller-compress"
      tls:
        certResolver: "letsencrypt"
      priority: 100
    
    ploy-controller-health:
      rule: "Host(`api.ployd.app`) && (PathPrefix(`/health`) || PathPrefix(`/ready`) || PathPrefix(`/live`))"
      entryPoints:
        - "websecure"
      service: "ploy-controller@consulcatalog"
      middlewares:
        - "ploy-controller-security"
        - "ploy-controller-compress"
      tls:
        certResolver: "letsencrypt"
      priority: 200
EOF
        destination = "local/dynamic/controller-load-balancer.yml"
        perms = "644"
      }
      
      # Dynamic configuration directory placeholder
      template {
        data = <<EOF
# Dynamic configuration will be added here by the controller
# when apps register their domains and routing rules
EOF
        destination = "local/dynamic/README.md"
        perms = "644"
      }
      
      # Environment variables for Traefik
      env {
        # Consul configuration
        CONSUL_HTTP_ADDR = "127.0.0.1:8500"
        
        # Let's Encrypt configuration
        LEGO_DISABLE_CNAME_SUPPORT = "true"
        
        # Cloudflare DNS (if using DNS challenge)
        # CF_API_EMAIL = "${CLOUDFLARE_EMAIL}"
        # CF_API_KEY = "${CLOUDFLARE_API_KEY}"
      }
      
      resources {
        cpu    = 200   # 200 MHz
        memory = 128   # 128 MB
      }
      
      # Logging configuration
      logs {
        max_files     = 5
        max_file_size = 50
      }
      
      # Kill timeout
      kill_timeout = "30s"
    }
  }
}