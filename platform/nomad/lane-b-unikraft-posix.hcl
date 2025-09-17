job "{{APP_NAME}}-lane-b" {
  datacenters = ["dc1"]
  type = "service"
  priority = 70  # High priority for Node.js unikernels
  
  # Canary deployment strategy for Node.js unikernels
  update {
    max_parallel     = 1
    min_healthy_time = "10s"  # Node.js starts faster than JVM
    healthy_deadline = "2m"
    progress_deadline = "5m"
    auto_revert      = true
    canary           = 1
    stagger          = "15s"
  }
  
  group "app" {
    count = {{INSTANCE_COUNT}}
    
    restart { 
      attempts = 5
      interval = "90s" 
      delay = "8s" 
      mode = "fail" 
    }
    
    reschedule {
      delay          = "10s"
      delay_function = "exponential"
      max_delay      = "20m"
      unlimited      = true
    }
    
    network { 
      port "http" { 
        to = {{HTTP_PORT}} 
      }
      port "metrics" {
        to = 9090
      }
    }
    
    task "unikernel-posix" {
      driver = "qemu"
      
      config {
        image_path = "{{IMAGE_PATH}}"
        args = [
          "-nographic",
          "-netdev", "user,id=net0,hostfwd=tcp::${NOMAD_PORT_http}-:{{HTTP_PORT}}",
          "-device", "virtio-net-pci,netdev=net0"
        ]
        accelerator = "kvm"
        kvm = true
        machine = "q35"
        cpu = "host"
      }
      
      # Node.js environment variables
      env {
        # Node.js runtime configuration
        NODE_ENV = "production"
        PORT = "{{HTTP_PORT}}"
        METRICS_PORT = "9090"
        
        # Application metadata
        APP_NAME = "{{APP_NAME}}"
        VERSION = "{{VERSION}}"
        LANE = "B"
        RUNTIME = "unikraft-posix"
        
        # Unikernel specific
        UK_NAME = "{{APP_NAME}}"
        UK_VERSION = "{{VERSION}}"
        
        # Network configuration
        LISTEN_HOST = "0.0.0.0"
        LISTEN_PORT = "{{HTTP_PORT}}"
        
        # Nomad integration
        NOMAD_ALLOC_ID = "${NOMAD_ALLOC_ID}"
        NOMAD_TASK_NAME = "${NOMAD_TASK_NAME}"
        SERVICE_NAME = "{{APP_NAME}}-lane-b"
        
        # Service registration (Consul service discovery only)
        
        {{CUSTOM_ENV_VARS}}
      }
      
      # Consul KV configuration for Node.js
      {{#if CONSUL_CONFIG_ENABLED}}
      template {
        data = <<EOF
{{range ls "ploy/apps/{{APP_NAME}}/config"}}
export {{.Key}}="{{.Value}}"
{{end}}

# Node.js specific configuration
{{with key "ploy/shared/config/max_memory"}}
export NODE_OPTIONS="--max-old-space-size={{.}}"
{{end}}
{{with key "ploy/shared/config/log_level"}}
export LOG_LEVEL={{.}}
{{end}}
EOF
        destination = "local/node.env"
        env         = true
        change_mode = "restart"
        perms       = "0644"
      }
      {{/if}}
      
      service {
        name = "{{APP_NAME}}-lane-b-unikraft-posix"
        port = "http"
        
        tags = [
          "lane-b",
          "unikraft-posix",
          "nodejs",
          "version-{{VERSION}}",
          "fast-boot",
          "traefik.enable=true",
          "traefik.http.routers.{{APP_NAME}}-b.rule=Host(`{{APP_NAME}}-b.{{DOMAIN_SUFFIX}}`)",
          "traefik.http.routers.{{APP_NAME}}-b.tls.certresolver=letsencrypt",
          "traefik.http.services.{{APP_NAME}}-b.loadbalancer.healthcheck.path=/health",
          "traefik.http.services.{{APP_NAME}}-b.loadbalancer.healthcheck.interval=10s"
        ]
        
        check { 
          type     = "http" 
          path     = "/health" 
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
          interval = "20s"
          timeout  = "5s"
        }
        
        meta {
          version = "{{VERSION}}"
          lane = "B"
          runtime = "unikraft-posix"
          nodejs_version = "18"
          boot_time = "milliseconds"
          memory_footprint = "small"
          build_time = "{{BUILD_TIME}}"
        }
      }
      
      # Metrics service
      service {
        name = "{{APP_NAME}}-nodejs-metrics"
        port = "metrics"
        
        tags = [
          "metrics",
          "prometheus",
          "nodejs"
        ]
        
        check {
          type     = "http"
          path     = "/metrics"
          interval = "30s"
          timeout  = "5s"
        }
      }
      
      # Optimized resources for Node.js unikernels
      resources { 
        cpu = {{CPU_LIMIT}}      # Typically 300-600 MHz for Node.js
        memory = {{MEMORY_LIMIT}} # Typically 128-512 MB for Node.js
      }
      
      logs { 
        max_files = 8
        max_file_size = 25
      }
      
      # Fast shutdown for Node.js
      kill_timeout = "15s"
      kill_signal = "SIGTERM"
    }
    
    # Fast migration for lightweight Node.js unikernels
    migrate {
      max_parallel     = 2
      health_check     = "checks"
      min_healthy_time = "5s"
      healthy_deadline = "45s"
    }
  }
}
