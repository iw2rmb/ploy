job "{{APP_NAME}}-lane-c" {
  datacenters = ["dc1"]
  type = "service"
  priority = 60  # High priority for JVM workloads
  
  # Template: Lane C - Java/JVM Applications
  # Runtime: OSv JVM with Java-specific configurations
  
  # Canary deployment strategy optimized for JVM
  update {
    max_parallel     = 1
    min_healthy_time = "45s"  # JVM needs longer warmup
    healthy_deadline = "5m"
    progress_deadline = "15m"
    auto_revert      = true
    canary           = 1
    stagger          = "60s"
  }
  
  group "app" {
    count = {{INSTANCE_COUNT}}
    
    restart { 
      attempts = 3
      interval = "3m" 
      delay = "30s"  # Longer delay for JVM startup
      mode = "fail" 
    }
    
    reschedule {
      delay          = "30s"
      delay_function = "exponential"
      max_delay      = "2h"
      unlimited      = true
    }
    
    network { 
      mode = "bridge"
      port "http" { 
        to = {{HTTP_PORT}} 
      }
      port "metrics" {
        to = 9090
      }
      port "jmx" {
        to = 9999
      }
    }
    
    # Persistent volume for JVM heap dumps and logs
    volume "jvm-data" {
      type      = "host"
      source    = "jvm-data"
      read_only = false
    }
    
    # Consul service mesh integration (optional)
    {{#if CONNECT_ENABLED}}
    service {
      name = "{{APP_NAME}}-connect"
      port = "http"
      
      connect {
        sidecar_service {
          proxy {
            upstreams {
              destination_name = "database"
              local_bind_port  = 5432
            }
            upstreams {
              destination_name = "redis"
              local_bind_port  = 6379
            }
            upstreams {
              destination_name = "vault"
              local_bind_port  = 8200
            }
          }
        }
      }
      
      meta {
        version = "{{VERSION}}"
        lane    = "C"
        runtime = "osv-jvm"
      }
    }
    {{/if}}
    
    task "osv-jvm" {
      driver = "qemu"
      
      # Vault integration for JVM applications (optional)
      {{#if VAULT_ENABLED}}
      vault {
        policies = ["{{APP_NAME}}-policy"]
        change_mode = "restart"
      }
      {{/if}}
      
      config {
        image_path = "{{IMAGE_PATH}}"
        args = [
          "-nographic",
          "-smp", "{{JVM_CPUS}}",
          "-m", "{{JVM_MEMORY}}M",
          "-netdev", "user,id=net0,hostfwd=tcp::${NOMAD_PORT_http}-:{{HTTP_PORT}}",
          "-device", "virtio-net-pci,netdev=net0"
        ]
        # Dev VPS: disable KVM/accelerator to avoid host dependency
        kvm = false
        machine = "q35"
        cpu = "max"
      }
      
      # Volume mounting for JVM data
      volume_mount {
        volume      = "jvm-data"
        destination = "/app/data"
      }
      
      # Comprehensive environment variables for JVM
      env {
        # JVM Configuration
        JAVA_OPTS = "{{JVM_OPTS}}"
        JVM_MEMORY = "{{JVM_MEMORY}}"
        JVM_CPUS = "{{JVM_CPUS}}"
        
        # Application configuration  
        APP_NAME = "{{APP_NAME}}"
        VERSION = "{{VERSION}}"
        LANE = "C"
        MAIN_CLASS = "{{MAIN_CLASS}}"
        
        # Network configuration
        SERVER_PORT = "{{HTTP_PORT}}"
        METRICS_PORT = "9090"
        JMX_PORT = "9999"
        
        # Spring Boot / Micronaut configuration
        MANAGEMENT_ENDPOINTS_WEB_EXPOSURE_INCLUDE = "health,info,metrics,prometheus"
        MANAGEMENT_ENDPOINT_HEALTH_SHOW_DETAILS = "always"
        MANAGEMENT_METRICS_EXPORT_PROMETHEUS_ENABLED = "true"
        
        # Database configuration (if using Connect)
        DATABASE_HOST = "127.0.0.1"
        DATABASE_PORT = "5432"
        REDIS_HOST = "127.0.0.1"  
        REDIS_PORT = "6379"
        
        # Service registration (Consul service discovery only)
        SERVICE_NAME = "{{APP_NAME}}-lane-c"
        
        # Nomad integration
        NOMAD_ALLOC_ID = "${NOMAD_ALLOC_ID}"
        NOMAD_TASK_NAME = "${NOMAD_TASK_NAME}"
        
        {{CUSTOM_ENV_VARS}}
      }
      
      # Application configuration from Consul KV
      template {
        data = <<EOF
# Application Configuration from Consul KV
{{range ls "ploy/apps/{{APP_NAME}}/config"}}
{{.Key}}={{.Value}}
{{end}}

# Database Configuration
{{with key "ploy/shared/database/url"}}
DATABASE_URL={{.}}
{{end}}
{{with key "ploy/shared/redis/url"}}  
REDIS_URL={{.}}
{{end}}
EOF
        destination = "local/application.properties"
        change_mode = "restart"
        perms       = "0644"
      }
      
      # Secrets from Vault
      template {
        data = <<EOF
# Database Credentials
DATABASE_USERNAME={{with secret "secret/data/{{APP_NAME}}/db"}}{{.Data.data.username}}{{end}}
DATABASE_PASSWORD={{with secret "secret/data/{{APP_NAME}}/db"}}{{.Data.data.password}}{{end}}

# API Keys and Tokens
API_SECRET_KEY={{with secret "secret/data/{{APP_NAME}}/api"}}{{.Data.data.secret_key}}{{end}}
JWT_SECRET={{with secret "secret/data/{{APP_NAME}}/jwt"}}{{.Data.data.secret}}{{end}}

# Third-party integrations
STRIPE_SECRET_KEY={{with secret "secret/data/{{APP_NAME}}/stripe"}}{{.Data.data.secret_key}}{{end}}
SENDGRID_API_KEY={{with secret "secret/data/{{APP_NAME}}/sendgrid"}}{{.Data.data.api_key}}{{end}}

# TLS Configuration
{{with secret "pki/issue/{{APP_NAME}}" "common_name={{APP_NAME}}.service.consul" "ttl=72h"}}
TLS_CERTIFICATE={{.Data.certificate}}
TLS_PRIVATE_KEY={{.Data.private_key}}
TLS_CA_CHAIN={{.Data.ca_chain}}
{{end}}
EOF
        destination = "secrets/application-secrets.properties"
        change_mode = "restart"
        perms       = "0600"
      }
      
      # Enhanced service registration
      service {
        name = "{{APP_NAME}}-lane-c-osv"
        port = "http"
        
        tags = [
          "lane-c",
          "osv",
          "jvm", 
          "version-{{VERSION}}",
          "java-{{JAVA_VERSION}}",
          "traefik.enable=true",
          "traefik.http.routers.{{APP_NAME}}-c.rule=Host(`{{APP_NAME}}-c.{{DOMAIN_SUFFIX}}`)",
          "traefik.http.routers.{{APP_NAME}}-c.tls.certresolver=letsencrypt",
          "traefik.http.services.{{APP_NAME}}-c.loadbalancer.healthcheck.path=/actuator/health",
          "traefik.http.services.{{APP_NAME}}-c.loadbalancer.healthcheck.interval=10s"
        ]
        
        check { 
          type     = "http" 
          path     = "/actuator/health" 
          interval = "15s" 
          timeout  = "5s"
          check_restart {
            limit = 2
            grace = "30s"
            ignore_warnings = false
          }
        }
        
        check {
          type     = "http"
          path     = "/actuator/health/readiness"
          interval = "30s"
          timeout  = "10s"
        }
        
        meta {
          version = "{{VERSION}}"
          lane = "C"
          runtime = "osv-jvm"
          java_version = "{{JAVA_VERSION}}"
          main_class = "{{MAIN_CLASS}}"
          build_time = "{{BUILD_TIME}}"
        }
      }
      
      # JMX monitoring service
      service {
        name = "{{APP_NAME}}-jmx"
        port = "jmx"
        
        tags = [
          "jmx",
          "monitoring",
          "java"
        ]
        
        check {
          type     = "tcp"
          interval = "30s"
          timeout  = "5s"
        }
      }
      
      # Metrics service
      service {
        name = "{{APP_NAME}}-osv-metrics"
        port = "metrics"
        
        tags = [
          "metrics",
          "prometheus",
          "jvm"
        ]
        
        check {
          type     = "http"
          path     = "/actuator/prometheus"
          interval = "30s"
          timeout  = "5s"
        }
      }
      
      # JVM-optimized resources
      resources { 
        cpu = {{CPU_LIMIT}}      # Typically 1000-2000 MHz for JVM
        memory = {{MEMORY_LIMIT}} # Typically 512-2048 MB for JVM
      }
      
      logs { 
        max_files = 5
        max_file_size = 20  # Keep under default ephemeral disk
      }
      
      # JVM lifecycle management
      lifecycle {
        hook    = "prestart"
        sidecar = false
      }
      
      # Graceful JVM shutdown
      kill_timeout = "60s"  # Longer for JVM cleanup
      kill_signal = "SIGTERM"
    }
    
    
    # JVM-optimized migration
    migrate {
      max_parallel     = 1  # Slower migration for JVM warmup
      health_check     = "checks"
      min_healthy_time = "30s"
      healthy_deadline = "3m"
    }
  }
}
