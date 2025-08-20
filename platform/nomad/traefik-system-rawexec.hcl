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
      driver = "raw_exec"
      
      config {
        command = "/usr/local/bin/traefik"
        args = ["--configfile=/opt/ploy/traefik/traefik.yml"]
      }
      
      # Download and extract Traefik binary
      artifact {
        source = "https://github.com/traefik/traefik/releases/download/v3.0.4/traefik_v3.0.4_linux_amd64.tar.gz"
        destination = "local/"
        options {
          checksum = "sha256:e8ad5e5cfaacad1c35a5e1c60a04fbd3b1e3b2b8a3b7b8b7b8b7b8b7b8b7b8b7"
        }
      }
      
      # Copy binary to system location
      template {
        data = <<EOF
#!/bin/bash
set -e
cp local/traefik /usr/local/bin/traefik
chmod +x /usr/local/bin/traefik
mkdir -p /opt/ploy/traefik
mkdir -p /opt/ploy/traefik/dynamic
EOF
        destination = "local/setup.sh"
        perms = "755"
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
  filePath: "/opt/ploy/traefik/traefik.log"
  format: json

accessLog:
  filePath: "/opt/ploy/traefik/traefik-access.log"
  format: json

# API and Dashboard
api:
  dashboard: true
  debug: false
  insecure: true

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
    directory: "/opt/ploy/traefik/dynamic"
    watch: true

# Certificate Resolvers (Let's Encrypt) - disabled for now
# certificatesResolvers:
#   letsencrypt:
#     acme:
#       email: admin@ployd.app
#       storage: /opt/ploy/traefik/acme.json
#       caServer: https://acme-v02.api.letsencrypt.org/directory
#       
#       # HTTP Challenge (default)
#       httpChallenge:
#         entryPoint: web

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
        destination = "/opt/ploy/traefik/traefik.yml"
        perms = "644"
      }
      
      # Dynamic configuration directory placeholder
      template {
        data = <<EOF
# Dynamic configuration will be added here by the controller
# when apps register their domains and routing rules
EOF
        destination = "/opt/ploy/traefik/dynamic/README.md"
        perms = "644"
      }
      
      # Pre-start setup
      lifecycle {
        hook = "prestart"
        sidecar = false
      }
      
      resources {
        cpu    = 200   # 200 MHz
        memory = 128   # 128 MB
      }
      
      # Environment variables for Traefik
      env {
        # Consul configuration
        CONSUL_HTTP_ADDR = "127.0.0.1:8500"
        
        # Let's Encrypt configuration
        LEGO_DISABLE_CNAME_SUPPORT = "true"
      }
      
      # Kill timeout
      kill_timeout = "30s"
    }
  }
}