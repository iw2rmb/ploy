job "{{APP_NAME}}-lane-c" {
  datacenters = ["dc1"]
  type = "service"
  priority = 60  # High priority for Node.js workloads
  
  # Template: Lane C - Node.js Applications
  # Runtime: OSv Node.js with Node-specific configurations
  
  # Canary deployment strategy optimized for Node.js
  update {
    max_parallel     = 1
    min_healthy_time = "30s"  # Node.js needs moderate warmup
    healthy_deadline = "3m"
    progress_deadline = "10m"
    auto_revert      = true
    canary           = 1
    stagger          = "30s"
  }
  
  group "app" {
    count = {{INSTANCE_COUNT}}
    
    restart { 
      attempts = 3
      interval = "2m" 
      delay = "15s"  # Faster restart for Node.js
      mode = "fail" 
    }
    
    reschedule {
      delay          = "15s"
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
      {{#if DEBUG_ENABLED}}
      port "debug" {
        to = 9229
      }
      {{/if}}
    }
    
    {{#if VOLUME_ENABLED}}
    # Persistent volume for Node.js application data (platform services only)
    volume "node-data" {
      type      = "host"
      source    = "node-data"
      read_only = false
    }
    {{/if}}
    
    {{#if CONNECT_ENABLED}}
    # Consul service mesh integration
    service {
      name = "{{APP_NAME}}-connect"
      port = "http"
      
      connect {
        sidecar_service {
          proxy {}
        }
      }
      
      meta {
        version = "{{VERSION}}"
        lane    = "C"
        runtime = "osv-node"
      }
    }
    {{/if}}
    
    task "osv-node" {
      driver = "qemu"
      
      config {
        image_path = "{{IMAGE_PATH}}"
        args = [
          "-nographic",
          "-smp", "{{JVM_CPUS}}",
          "-m", "{{MEMORY_LIMIT}}M",
          "-netdev", "user,id=net0,hostfwd=tcp::${NOMAD_PORT_http}-:{{HTTP_PORT}},hostfwd=tcp::${NOMAD_PORT_debug}-:9229",
          "-device", "virtio-net-pci,netdev=net0"
        ]
      }
      
      {{#if VOLUME_ENABLED}}
      # Volume mounting for Node.js data
      volume_mount {
        volume      = "node-data"
        destination = "/app/data"
      }
      {{/if}}
      
      # Comprehensive environment variables for Node.js
      env {
        # Node.js Configuration
        NODE_ENV = "production"
        NODE_OPTIONS = "--max-old-space-size={{MEMORY_LIMIT}}"
        
        # Application configuration  
        APP_NAME = "{{APP_NAME}}"
        VERSION = "{{VERSION}}"
        LANE = "C"
        
        # Network configuration
        PORT = "{{HTTP_PORT}}"
        METRICS_PORT = "9090"
        {{#if DEBUG_ENABLED}}
        DEBUG_PORT = "9229"
        NODE_OPTIONS = "--inspect=0.0.0.0:9229 --max-old-space-size={{MEMORY_LIMIT}}"
        {{/if}}
        
        # Express/Fastify configuration
        TRUST_PROXY = "true"
        
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

EOF
        destination = "local/application.properties"
        change_mode = "restart"
        perms       = "0644"
      }
      
      {{#if CONSUL_CONFIG_ENABLED}}
      # Enhanced service registration
      service {
        name = "{{APP_NAME}}-lane-c-osv"
        port = "http"
        
        tags = [
          "lane-c",
          "osv",
          "node", 
          "version-{{VERSION}}",
          "node-{{NODE_VERSION}}",
          "traefik.enable=true",
          "traefik.http.routers.{{APP_NAME}}-c.rule=Host(`{{APP_NAME}}-c.{{DOMAIN_SUFFIX}}`)",
          "traefik.http.routers.{{APP_NAME}}-c.tls.certresolver=letsencrypt",
          "traefik.http.services.{{APP_NAME}}-c.loadbalancer.healthcheck.path=/health",
          "traefik.http.services.{{APP_NAME}}-c.loadbalancer.healthcheck.interval=10s"
        ]
        
        check { 
          type     = "http" 
          path     = "/health" 
          interval = "10s" 
          timeout  = "3s"
          check_restart {
            limit = 2
            grace = "15s"
            ignore_warnings = false
          }
        }
        
        check {
          type     = "http"
          path     = "/ready"
          interval = "15s"
          timeout  = "5s"
        }
        
        connect {
          sidecar_service {}
        }
        
        meta {
          version = "{{VERSION}}"
          lane = "C"
          runtime = "osv-node"
          node_version = "{{NODE_VERSION}}"
          build_time = "{{BUILD_TIME}}"
        }
      }
      
      # Node.js metrics service
      service {
        name = "{{APP_NAME}}-osv-metrics"
        port = "metrics"
        
        tags = [
          "metrics",
          "prometheus",
          "node"
        ]
        
        check {
          type     = "http"
          path     = "/metrics"
          interval = "30s"
          timeout  = "5s"
        }
      }
      {{/if}}
      
      # Node.js-optimized resources
      resources { 
        cpu = {{CPU_LIMIT}}      # Typically 500-1000 MHz for Node.js
        memory = {{MEMORY_LIMIT}} # Typically 256-1024 MB for Node.js
        {{#if DISK_SIZE}}
        disk = {{DISK_SIZE}}     # For Node modules and logs
        {{/if}}
      }
      
      logs { 
        max_files = 10
        max_file_size = 50  # Moderate logs for Node.js applications
      }
      
      # Node.js lifecycle management
      lifecycle {
        hook    = "prestart"
        sidecar = false
      }
      
      # Graceful Node.js shutdown
      kill_timeout = "30s"  # Faster shutdown for Node.js
      kill_signal = "SIGTERM"
    }
    
    {{#if CONNECT_ENABLED}}
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
        max_files = 5
        max_file_size = 25
      }
    }
    {{/if}}
    
    # Node.js-optimized migration
    migrate {
      max_parallel     = 2  # Faster migration for Node.js
      health_check     = "checks"
      min_healthy_time = "15s"
      healthy_deadline = "2m"
    }
  }
}
