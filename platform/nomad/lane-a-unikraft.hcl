job "{{APP_NAME}}-lane-a" {
  datacenters = ["dc1"]
  type = "service"
  priority = 75  # Higher priority for unikernels
  
  # Canary deployment strategy
  update {
    max_parallel     = 1
    min_healthy_time = "15s"  # Faster startup for unikernels
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
      delay = "10s" 
      mode = "fail" 
    }
    
    reschedule {
      delay          = "15s"
      delay_function = "exponential"
      max_delay      = "30m"
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
    
    # Consul service mesh (limited support for unikernels)
    {{#if CONNECT_ENABLED}}
    service {
      name = "{{APP_NAME}}-connect"
      port = "http"
      
      meta {
        version = "{{VERSION}}"
        lane    = "A"
        runtime = "unikraft"
      }
    }
    {{/if}}
    
    task "unikernel" {
      driver = "qemu"
      
      # Vault integration for configuration secrets
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
          "-netdev", "user,id=net0,hostfwd=tcp::${NOMAD_PORT_http}-:{{HTTP_PORT}}",
          "-device", "virtio-net-pci,netdev=net0"
        ]
        accelerator = "kvm"
        kvm = true
        machine = "q35"
        cpu = "host"
      }
      
      # Environment variables for unikernel configuration
      env {
        # Basic unikernel environment
        UK_NAME = "{{APP_NAME}}"
        UK_VERSION = "{{VERSION}}"
        UK_LANE = "A"
        
        # Network configuration
        LISTEN_PORT = "{{HTTP_PORT}}"
        METRICS_PORT = "9090"
        
        # Nomad integration
        NOMAD_ALLOC_ID = "${NOMAD_ALLOC_ID}"
        NOMAD_TASK_NAME = "${NOMAD_TASK_NAME}"
        SERVICE_NAME = "{{APP_NAME}}-lane-a"
        
        # Consul integration (if supported by unikernel)
        CONSUL_HTTP_ADDR = "${attr.unique.network.ip-address}:8500"
        
        {{CUSTOM_ENV_VARS}}
      }
      
      # Consul KV configuration template
      {{#if CONSUL_CONFIG_ENABLED}}
      template {
        data = <<EOF
{{range ls "ploy/apps/{{APP_NAME}}/config"}}
export {{.Key}}="{{.Value}}"
{{end}}
EOF
        destination = "local/unikernel.env"
        env         = true
        change_mode = "restart"
        perms       = "0644"
      }
      {{/if}}
      
      # Vault secrets for unikernels
      {{#if VAULT_ENABLED}}
      template {
        data = <<EOF
DATABASE_PASSWORD={{with secret "secret/data/{{APP_NAME}}"}}{{.Data.data.database_password}}{{end}}
API_KEY={{with secret "secret/data/{{APP_NAME}}"}}{{.Data.data.api_key}}{{end}}
TLS_CERT={{with secret "pki/issue/{{APP_NAME}}" "common_name={{APP_NAME}}.service.consul"}}{{.Data.certificate}}{{end}}
TLS_KEY={{with secret "pki/issue/{{APP_NAME}}" "common_name={{APP_NAME}}.service.consul"}}{{.Data.private_key}}{{end}}
EOF
        destination = "secrets/unikernel.env"
        env         = true
        change_mode = "restart"
        perms       = "0600"
      }
      {{/if}}
      
      service {
        name = "{{APP_NAME}}-lane-a-unikraft"
        port = "http"
        
        tags = [
          "lane-a",
          "unikraft",
          "version-{{VERSION}}",
          "microsecond-boot",
          "traefik.enable=true",
          "traefik.http.routers.{{APP_NAME}}-a.rule=Host(`{{APP_NAME}}-a.{{DOMAIN_SUFFIX}}`)",
          "traefik.http.routers.{{APP_NAME}}-a.tls.certresolver=letsencrypt",
          "traefik.http.services.{{APP_NAME}}-a.loadbalancer.healthcheck.path=/healthz",
          "traefik.http.services.{{APP_NAME}}-a.loadbalancer.healthcheck.interval=5s"
        ]
        
        check { 
          type     = "http" 
          path     = "/healthz" 
          interval = "5s" 
          timeout  = "2s"
          check_restart {
            limit = 2
            grace = "5s"
            ignore_warnings = false
          }
        }
        
        check {
          type     = "http"
          path     = "/ready"
          interval = "15s"
          timeout  = "3s"
        }
        
        meta {
          version = "{{VERSION}}"
          lane = "A"
          runtime = "unikraft"
          boot_time = "microseconds"
          memory_footprint = "minimal"
          build_time = "{{BUILD_TIME}}"
        }
      }
      
      # Metrics service for monitoring
      service {
        name = "{{APP_NAME}}-unikraft-metrics"
        port = "metrics"
        
        tags = [
          "metrics",
          "prometheus",
          "unikraft"
        ]
        
        check {
          type     = "http"
          path     = "/metrics"
          interval = "30s"
          timeout  = "5s"
        }
      }
      
      # Optimized resources for unikernels
      resources { 
        cpu = {{CPU_LIMIT}}      # Typically 200-500 MHz
        memory = {{MEMORY_LIMIT}} # Typically 64-256 MB
      }
      
      logs { 
        max_files = 5
        max_file_size = 10  # Smaller logs for unikernels
      }
      
      # Fast shutdown for unikernels
      kill_timeout = "10s"
      kill_signal = "SIGTERM"
    }
    
    # Migration optimized for fast-booting unikernels
    migrate {
      max_parallel     = 2  # Can migrate faster due to quick boot
      health_check     = "checks"
      min_healthy_time = "5s"
      healthy_deadline = "30s"
    }
  }
}