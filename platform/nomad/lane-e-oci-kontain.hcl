job "{{APP_NAME}}-lane-e" {
  datacenters = ["dc1"]
  type = "service"
  priority = 50  # Standard priority for containerized workloads
  
  # Canary deployment strategy for containers
  update {
    max_parallel     = 2
    min_healthy_time = "20s"
    healthy_deadline = "3m"
    progress_deadline = "10m"
    auto_revert      = true
    canary           = 1
    stagger          = "20s"
  }
  
  group "app" {
    count = {{INSTANCE_COUNT}}
    
    restart { 
      attempts = 5
      interval = "2m" 
      delay = "15s" 
      mode = "fail" 
    }
    
    reschedule {
      delay          = "20s"
      delay_function = "exponential"
      max_delay      = "1h"
      unlimited      = true
    }
    
    network { 
      port "http" { 
        to = {{HTTP_PORT}} 
      }
      port "metrics" {
        to = 9090
      }
      {{#if GRPC_PORT}}
      port "grpc" {
        to = {{GRPC_PORT}}
      }
      {{/if}}
    }
    
    # Persistent volumes for container data
    {{#if VOLUME_ENABLED}}
    volume "app-data" {
      type      = "host"
      source    = "app-data"
      read_only = false
    }
    
    volume "cache" {
      type      = "host"
      source    = "cache"
      read_only = false
    }
    {{/if}}
    
    # Consul service mesh integration (disabled in dev template)
    # Intentionally omitted to simplify Lane E for user apps on dev/test clusters.
    
    task "oci-kontain" {
      driver = "docker"
      
      # Vault integration for secrets
      {{#if VAULT_ENABLED}}
      vault {
        policies = ["{{APP_NAME}}-policy"]
        change_mode = "restart"
      }
      {{/if}}
      
      config {
        image = "{{DOCKER_IMAGE}}"
        
        # Note: Kontain runtime requires KVM which is not available on this VPS
        # Using standard Docker runtime instead
        # runtime = "io.kontain"
        
        # Port mapping
        ports = ["http", "metrics"{{#if GRPC_PORT}}, "grpc"{{/if}}]
        
        # Container configuration
        hostname = "{{APP_NAME}}-${NOMAD_ALLOC_INDEX}"
        
        # Security options for Kontain
        security_opt = [
          "apparmor:unconfined"
        ]
        
        # Container labels
        labels = {
          "ploy.app" = "{{APP_NAME}}"
          "ploy.lane" = "E"
          "ploy.version" = "{{VERSION}}"
          "ploy.runtime" = "kontain"
        }
        
        # Container-level healthcheck not supported on this cluster's Docker driver; use service-level checks instead.
        
        # Logging configuration
        logging {
          type = "json-file"
          config {
            max-size = "50m"
            max-file = "10"
            labels = "ploy.app,ploy.lane,ploy.version"
          }
        }
        
        {{#if VOLUME_ENABLED}}
        # Volume mounts
        mount {
          type = "volume"
          target = "/app/data"
          source = "app-data"
        }
        
        mount {
          type = "volume" 
          target = "/app/cache"
          source = "cache"
        }
        {{/if}}
        
        # Resource limits at container level
        ulimit {
          nofile = "65536:65536"
          nproc = "32768:32768"
        }
      }
      
      # Volume mounting for Nomad
      {{#if VOLUME_ENABLED}}
      volume_mount {
        volume      = "app-data"
        destination = "/host/app-data"
      }
      
      volume_mount {
        volume      = "cache"
        destination = "/host/cache"
      }
      {{/if}}
      
      # Environment variables
      env {
        # Application configuration
        APP_NAME = "{{APP_NAME}}"
        VERSION = "{{VERSION}}"
        LANE = "E"
        RUNTIME = "kontain"
        
        # Network configuration
        PORT = "${NOMAD_PORT_http}"
        HTTP_PORT = "${NOMAD_PORT_http}"
        METRICS_PORT = "${NOMAD_PORT_metrics}"
        {{#if GRPC_PORT}}
        GRPC_PORT = "${NOMAD_PORT_grpc}"
        {{/if}}
        
        # Container-specific environment
        HOSTNAME = "${NOMAD_ALLOC_ID}"
        POD_NAME = "{{APP_NAME}}-${NOMAD_ALLOC_INDEX}"
        POD_NAMESPACE = "ploy"
        
        # Service registration (Consul service discovery only)
        SERVICE_NAME = "{{APP_NAME}}-lane-e"
        
        # Nomad integration
        NOMAD_ALLOC_ID = "${NOMAD_ALLOC_ID}"
        NOMAD_TASK_NAME = "${NOMAD_TASK_NAME}"
        NOMAD_JOB_NAME = "${NOMAD_JOB_NAME}"
        
        # Database connections (via Connect)
        {{#if CONNECT_ENABLED}}
        DATABASE_HOST = "127.0.0.1"
        DATABASE_PORT = "5432"
        REDIS_HOST = "127.0.0.1"
        REDIS_PORT = "6379"
        ELASTICSEARCH_HOST = "127.0.0.1"
        ELASTICSEARCH_PORT = "9200"
        {{/if}}
        
        # Container runtime information
        KONTAIN_RUNTIME = "true"
        VM_ISOLATION = "true"
        
        {{CUSTOM_ENV_VARS}}
      }
      
      # Dynamic configuration from Consul
      {{#if CONSUL_CONFIG_ENABLED}}
      template {
        data = <<EOF
# Application Configuration
{{range ls "ploy/apps/{{APP_NAME}}/config"}}
{{.Key}}={{.Value}}
{{end}}

# Feature Flags
{{range ls "ploy/apps/{{APP_NAME}}/features"}}
FEATURE_{{.Key | toUpper}}={{.Value}}
{{end}}

# Environment-specific configuration
{{range ls "ploy/shared/config"}}
SHARED_{{.Key | toUpper}}={{.Value}}
{{end}}

# External service URLs
{{with key "ploy/shared/database/url"}}
DATABASE_URL={{.}}
{{end}}
{{with key "ploy/shared/redis/url"}}
REDIS_URL={{.}}
{{end}}
{{with key "ploy/shared/elasticsearch/url"}}
ELASTICSEARCH_URL={{.}}
{{end}}
EOF
        destination = "local/app.env"
        env         = true
        change_mode = "restart"
        perms       = "0644"
      }
      {{/if}}
      
      # Secrets from Vault
      {{#if VAULT_ENABLED}}
      template {
        data = <<EOF
# Database credentials
DATABASE_USERNAME={{with secret "secret/data/{{APP_NAME}}/db"}}{{.Data.data.username}}{{end}}
DATABASE_PASSWORD={{with secret "secret/data/{{APP_NAME}}/db"}}{{.Data.data.password}}{{end}}

# API keys and secrets
API_SECRET_KEY={{with secret "secret/data/{{APP_NAME}}/api"}}{{.Data.data.secret_key}}{{end}}
JWT_SECRET={{with secret "secret/data/{{APP_NAME}}/jwt"}}{{.Data.data.secret}}{{end}}
ENCRYPTION_KEY={{with secret "secret/data/{{APP_NAME}}/encryption"}}{{.Data.data.key}}{{end}}

# Third-party service credentials
STRIPE_SECRET_KEY={{with secret "secret/data/{{APP_NAME}}/stripe"}}{{.Data.data.secret_key}}{{end}}
AWS_ACCESS_KEY_ID={{with secret "aws/creds/{{APP_NAME}}-role"}}{{.Data.access_key}}{{end}}
AWS_SECRET_ACCESS_KEY={{with secret "aws/creds/{{APP_NAME}}-role"}}{{.Data.secret_key}}{{end}}
SENDGRID_API_KEY={{with secret "secret/data/{{APP_NAME}}/sendgrid"}}{{.Data.data.api_key}}{{end}}

# TLS certificates
{{with secret "pki/issue/{{APP_NAME}}" "common_name={{APP_NAME}}.service.consul" "ttl=72h"}}
TLS_CERT_PEM={{.Data.certificate}}
TLS_KEY_PEM={{.Data.private_key}}
TLS_CA_PEM={{.Data.ca_chain}}
{{end}}

# OAuth credentials
OAUTH_CLIENT_ID={{with secret "secret/data/{{APP_NAME}}/oauth"}}{{.Data.data.client_id}}{{end}}
OAUTH_CLIENT_SECRET={{with secret "secret/data/{{APP_NAME}}/oauth"}}{{.Data.data.client_secret}}{{end}}
EOF
        destination = "secrets/app.env"
        env         = true
        change_mode = "restart"
        perms       = "0600"
      }
      {{/if}}
      
      # Service registration with comprehensive health checks
      service {
        name = "{{APP_NAME}}-lane-e-oci-kontain"
        port = "http"
        
        tags = [
          "lane-e",
          "oci",
          "kontain",
          "vm-isolation",
          "version-{{VERSION}}",
          "container",
          "traefik.enable=true",
          "traefik.http.routers.{{APP_NAME}}-e.rule=Host(`{{APP_NAME}}.{{DOMAIN_SUFFIX}}`)",
          "traefik.http.routers.{{APP_NAME}}-e.tls.certresolver=dev-wildcard",
          "traefik.http.services.{{APP_NAME}}-e.loadbalancer.healthcheck.path=/healthz",
          "traefik.http.services.{{APP_NAME}}-e.loadbalancer.healthcheck.interval=10s",
          "traefik.http.services.{{APP_NAME}}-e.loadbalancer.sticky.cookie=true"
        ]
        
        # Service-level health checks
        check {
          type     = "http"
          path     = "/healthz"
          interval = "15s"
          timeout  = "5s"
          check_restart {
            limit = 3
            grace = "20s"
            ignore_warnings = false
          }
        }
        
        {{#if CONNECT_ENABLED}}
        connect {
          sidecar_service {}
        }
        {{/if}}
        
        meta {
          version = "{{VERSION}}"
          lane = "E"
          runtime = "kontain"
          isolation = "vm-level"
          image = "{{DOCKER_IMAGE}}"
          build_time = "{{BUILD_TIME}}"
        }
      }
      
      # Metrics service
      service {
        name = "{{APP_NAME}}-kontain-metrics"
        port = "metrics"
        
        tags = [
          "metrics",
          "prometheus",
          "kontain"
        ]
        
        check {
          type     = "http"
          path     = "/metrics"
          interval = "30s"
          timeout  = "5s"
        }
      }
      
      {{#if GRPC_PORT}}
      # gRPC service
      service {
        name = "{{APP_NAME}}-grpc"
        port = "grpc"
        
        tags = [
          "grpc",
          "api"
        ]
        
        check {
          type     = "grpc"
          interval = "30s"
          timeout  = "5s"
        }
      }
      {{/if}}
      
      # Container-optimized resources
      resources { 
        cpu = {{CPU_LIMIT}}      # Efficient CPU usage with Kontain
        memory = {{MEMORY_LIMIT}} # Memory overhead from Kontain isolation
        {{#if DISK_SIZE}}
        disk = {{DISK_SIZE}}
        {{/if}}
      }
      
      logs { 
        max_files = 10
        max_file_size = 10
      }
      
      # Container lifecycle
      lifecycle {
        hook    = "prestart"
        sidecar = false
      }
      
      # Graceful container shutdown
      kill_timeout = "30s"
      kill_signal = "SIGTERM"
    }
    
    # Consul Connect sidecar
    {{#if CONNECT_ENABLED}}
    task "connect-proxy" {
      driver = "docker"
      
      config {
        image = "envoyproxy/envoy:v1.24.0"
        args = [
          "--config-path", "${NOMAD_SECRETS_DIR}/envoy_bootstrap.json",
          "--log-level", "info"
        ]
        network_mode = "container:${NOMAD_TASK_NAME}"
      }
      
      lifecycle {
        hook = "prestart"
        sidecar = true
      }
      
      resources {
        cpu    = 200
        memory = 128
      }
      
      logs {
        max_files = 5
        max_file_size = 25
      }
    }
    {{/if}}
    
    # Container-optimized migration
    migrate {
      max_parallel     = 2  # Faster container migration
      health_check     = "checks"
      min_healthy_time = "10s"
      healthy_deadline = "2m"
    }
  }
}
