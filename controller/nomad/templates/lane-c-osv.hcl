job "{{APP_NAME}}" {
  datacenters = ["dc1"]
  type = "service"
  priority = 60  # High priority for JVM workloads
  
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
    count = 1
    
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
        to = 8080 
      }
      port "metrics" {
        to = 9090
      }
      port "jmx" {
        to = 9999
      }
      {{#if DEBUG_ENABLED}}
      port "debug" {
        to = 5005
      }
      {{/if}}
    }
    
    # Persistent volume for JVM heap dumps and logs
    volume "jvm-data" {
      type      = "host"
      source    = "jvm-data"
      read_only = false
    }
    
    # Consul service mesh integration
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
    
    task "osv-jvm" {
      driver = "qemu"
      
      # Vault integration for JVM applications
      vault {
        policies = ["default"]
        change_mode = "restart"
        namespace = "default"
      }
      
      artifact {
        source      = "{{IMAGE_PATH}}"
        destination = "local/"
        mode        = "file"
      }
      
      config {
        image_path = "local/{{APP_NAME}}.qcow2"
        args = [
          "-nographic",
          "-smp", "2",
          "-m", "512M",
          "-netdev", "user,id=net0,hostfwd=tcp::${NOMAD_PORT_http}-:8080,hostfwd=tcp::${NOMAD_PORT_jmx}-:9999",
          "-device", "virtio-net-pci,netdev=net0"
        ]
        accelerator = "kvm"
        kvm = true
        machine = "q35"
        cpu = "host"
      }
      
      # Volume mounting for JVM data
      volume_mount {
        volume      = "jvm-data"
        destination = "/app/data"
        read_only   = false
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
        SERVER_PORT = "8080"
        METRICS_PORT = "9090"
        JMX_PORT = "9999"
        {{#if DEBUG_ENABLED}}
        DEBUG_PORT = "5005"
        JAVA_TOOL_OPTIONS = "-agentlib:jdwp=transport=dt_socket,server=y,suspend=n,address=*:5005"
        {{/if}}
        
        # Spring Boot / Micronaut configuration
        MANAGEMENT_ENDPOINTS_WEB_EXPOSURE_INCLUDE = "health,info,metrics,prometheus"
        MANAGEMENT_ENDPOINT_HEALTH_SHOW_DETAILS = "always"
        MANAGEMENT_METRICS_EXPORT_PROMETHEUS_ENABLED = "true"
        
        # Database configuration (if using Connect)
        DATABASE_HOST = "127.0.0.1"
        DATABASE_PORT = "5432"
        REDIS_HOST = "127.0.0.1"  
        REDIS_PORT = "6379"
        
        # Consul integration
        CONSUL_HTTP_ADDR = "${attr.unique.network.ip-address}:8500"
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
          "traefik.http.routers.{{APP_NAME}}.rule=Host(`{{APP_NAME}}.{{DOMAIN_SUFFIX}}`)",
          "traefik.http.services.{{APP_NAME}}.loadbalancer.server.port=${NOMAD_PORT_http}"
        ]
        
        check { 
          type     = "http" 
          path     = "/health" 
          interval = "15s" 
          timeout  = "5s"
          check_restart {
            limit = 2
            grace = "30s"
            ignore_warnings = false
          }
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
        cpu = 1000      # 1000 MHz for JVM
        memory = 512    # 512 MB for JVM
        disk = 512      # 512 MB disk allocation
      }
      
      logs { 
        max_files = 3
        max_file_size = 10  # Reduced for efficient disk usage
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
    
    # Consul Connect sidecar for service mesh
    task "connect-proxy" {
      driver = "docker"
      
      config {
        image = "envoyproxy/envoy:v1.24.0"
        args = [
          "--config-path", "${NOMAD_SECRETS_DIR}/envoy_bootstrap.json",
          "--log-level", "info",
          "--component-log-level", "upstream:debug,connection:trace"
        ]
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
        max_files = 2
        max_file_size = 10
      }
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