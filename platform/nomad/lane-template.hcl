job "{{APP_NAME}}-{{LANE}}" {
  datacenters = ["dc1"]
  type = "service"
  
  # Priority and constraints
  priority = 50
  constraint {
    attribute = "${attr.kernel.name}"
    value     = "linux"
  }
  
  # Canary deployment strategy
  update {
    max_parallel     = 1
    min_healthy_time = "30s"
    healthy_deadline = "3m"
    progress_deadline = "10m"
    auto_revert      = true
    canary           = 1
    stagger          = "30s"
  }
  
  group "app" {
    count = {{INSTANCE_COUNT}}
    
    # Enhanced restart policy
    restart { 
      attempts = 5
      interval = "2m" 
      delay = "15s" 
      mode = "fail" 
    }
    
    # Enhanced reschedule policy
    reschedule {
      delay          = "30s"
      delay_function = "exponential"
      max_delay      = "1h"
      unlimited      = true
    }
    
    # Network configuration with multiple ports
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
    
    # Persistent volume support
    {{#if VOLUME_ENABLED}}
    volume "app-data" {
      type      = "host"
      source    = "app-data"
      read_only = false
    }
    {{/if}}
    
    # Consul service mesh sidecar
    {{#if CONNECT_ENABLED}}
    service {
      name = "{{APP_NAME}}-connect"
      port = "http"
      
      connect {
        sidecar_service {
          proxy {
            upstream {
              destination_name = "database"
              local_bind_port  = 5432
            }
            {{#if VAULT_ENABLED}}
            upstream {
              destination_name = "vault"
              local_bind_port  = 8200
            }
            {{/if}}
          }
        }
      }
      
      meta {
        version = "{{VERSION}}"
        lane    = "{{LANE}}"
      }
    }
    {{/if}}
    
    task "{{TASK_NAME}}" {
      driver = "{{DRIVER}}"
      
      # Vault integration for secrets
      {{#if VAULT_ENABLED}}
      vault {
        policies = ["{{APP_NAME}}-policy"]
        change_mode = "restart"
      }
      {{/if}}
      
      # Enhanced configuration based on driver
      config {
        {{DRIVER_CONFIG}}
      }
      
      # Volume mounting
      {{#if VOLUME_ENABLED}}
      volume_mount {
        volume      = "app-data"
        destination = "/app/data"
      }
      {{/if}}
      
      # Environment variables with multiple sources
      env {
        # Basic application configuration
        APP_NAME = "{{APP_NAME}}"
        LANE = "{{LANE}}"
        VERSION = "{{VERSION}}"
        NOMAD_ALLOC_ID = "${NOMAD_ALLOC_ID}"
        NOMAD_TASK_NAME = "${NOMAD_TASK_NAME}"
        
        # Network configuration
        PORT = "${NOMAD_PORT_http}"
        METRICS_PORT = "${NOMAD_PORT_metrics}"
        {{#if GRPC_PORT}}
        GRPC_PORT = "${NOMAD_PORT_grpc}"
        {{/if}}
        
        # Consul integration
        CONSUL_HTTP_ADDR = "${attr.unique.network.ip-address}:8500"
        SERVICE_NAME = "{{APP_NAME}}-{{LANE}}"
        
        {{CUSTOM_ENV_VARS}}
      }
      
      # Template-based configuration from Consul KV
      {{#if CONSUL_CONFIG_ENABLED}}
      template {
        data = <<EOF
{{range ls "ploy/apps/{{APP_NAME}}/config"}}
{{.Key}}={{.Value}}
{{end}}
EOF
        destination = "local/app.env"
        env         = true
        change_mode = "restart"
      }
      {{/if}}
      
      # Vault secrets template
      {{#if VAULT_ENABLED}}
      template {
        data = <<EOF
DATABASE_PASSWORD={{with secret "secret/data/{{APP_NAME}}"}}{{.Data.data.database_password}}{{end}}
API_KEY={{with secret "secret/data/{{APP_NAME}}"}}{{.Data.data.api_key}}{{end}}
EOF
        destination = "secrets/app.env"
        env         = true
        change_mode = "restart"
      }
      {{/if}}
      
      # Service registration with enhanced health checks
      service {
        name = "{{APP_NAME}}-{{LANE}}"
        port = "http"
        
        tags = [
          "{{LANE}}",
          "version-{{VERSION}}",
          "traefik.enable=true",
          "traefik.http.routers.{{APP_NAME}}.rule=Host(`{{APP_NAME}}.{{DOMAIN_SUFFIX}}`)",
          "traefik.http.routers.{{APP_NAME}}.tls.certresolver=letsencrypt",
          "traefik.http.services.{{APP_NAME}}.loadbalancer.healthcheck.path=/healthz"
        ]
        
        check { 
          type     = "http" 
          path     = "/healthz" 
          interval = "10s" 
          timeout  = "3s"
          check_restart {
            limit = 3
            grace = "10s"
            ignore_warnings = false
          }
        }
        
        check {
          type     = "http"
          path     = "/ready"
          interval = "30s"
          timeout  = "5s"
        }
        
        {{#if CONNECT_ENABLED}}
        connect {
          sidecar_service {}
        }
        {{/if}}
        
        meta {
          version = "{{VERSION}}"
          lane = "{{LANE}}"
          build_time = "{{BUILD_TIME}}"
        }
      }
      
      # Metrics service
      service {
        name = "{{APP_NAME}}-metrics"
        port = "metrics"
        
        tags = [
          "metrics",
          "prometheus"
        ]
        
        check {
          type     = "http"
          path     = "/metrics"
          interval = "30s"
          timeout  = "5s"
        }
      }
      
      # Resource allocation with scaling
      resources { 
        cpu    = {{CPU_LIMIT}}
        memory = {{MEMORY_LIMIT}}
        {{#if DISK_SIZE}}
        disk   = {{DISK_SIZE}}
        {{/if}}
      }
      
      # Enhanced logging configuration
      logs { 
        max_files = 10
        max_file_size = 50
      }
      
      # Lifecycle hooks
      lifecycle {
        hook    = "prestart"
        sidecar = false
      }
      
      # Kill signal and timeout
      kill_timeout = "30s"
      kill_signal = "SIGTERM"
    }
    
    # Consul Connect sidecar task
    {{#if CONNECT_ENABLED}}
    task "connect-proxy" {
      driver = "docker"
      
      config {
        image = "envoyproxy/envoy:v1.20.0"
      }
      
      lifecycle {
        hook = "prestart"
        sidecar = true
      }
      
      resources {
        cpu    = 200
        memory = 128
      }
    }
    {{/if}}
    
    # Migration strategy
    migrate {
      max_parallel     = 1
      health_check     = "checks"
      min_healthy_time = "10s"
      healthy_deadline = "1m"
    }
  }
}