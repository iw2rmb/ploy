job "{{APP_NAME}}-lane-f" {
  datacenters = ["dc1"]
  type = "service"
  priority = 40  # Lower priority due to resource intensity
  
  # Conservative canary deployment for VMs
  update {
    max_parallel     = 1
    min_healthy_time = "60s"  # VMs take longer to boot and stabilize
    healthy_deadline = "8m"
    progress_deadline = "20m"
    auto_revert      = true
    canary           = 1
    stagger          = "90s"
  }
  
  group "app" {
    count = {{INSTANCE_COUNT}}
    
    restart { 
      attempts = 2
      interval = "5m" 
      delay = "60s" 
      mode = "fail" 
    }
    
    reschedule {
      delay          = "60s"
      delay_function = "exponential"
      max_delay      = "4h"
      unlimited      = false
      max_unlimited  = 3
    }
    
    network { 
      port "http" { 
        to = {{HTTP_PORT}} 
      }
      port "metrics" {
        to = 9090
      }
      port "ssh" {
        to = 22
      }
      {{#if DEBUG_ENABLED}}
      port "debug" {
        to = 2222
      }
      {{/if}}
    }
    
    # Persistent volumes for VM storage
    volume "vm-data" {
      type      = "host"
      source    = "vm-data"
      read_only = false
    }
    
    volume "vm-logs" {
      type      = "host"
      source    = "vm-logs"
      read_only = false
    }
    
    task "vm" {
      driver = "qemu"
      
      # Vault integration for VM secrets
      {{#if VAULT_ENABLED}}
      vault {
        policies = ["{{APP_NAME}}-policy"]
        change_mode = "restart"
      }
      {{/if}}
      
      config {
        image_path = "{{IMAGE_PATH}}"
        
        # VM configuration with generous resources
        args = [
          "-nographic",
          "-m", "{{MEMORY_LIMIT}}M",
          "-smp", "{{JVM_CPUS}}",
          "-netdev", "user,id=net0,hostfwd=tcp::${NOMAD_PORT_http}-:{{HTTP_PORT}},hostfwd=tcp::${NOMAD_PORT_ssh}-:22",
          "-device", "virtio-net-pci,netdev=net0",
          "-drive", "file={{IMAGE_PATH}},format=qcow2,if=virtio",
          "-device", "virtio-balloon"
        ]
        accelerator = "kvm"
        kvm = true
        machine = "q35"
        cpu = "host"
        
        # VM-specific options
        vnc = ":1"  # VNC display for debugging
        monitor = "stdio"
        
        # Resource allocation
        port_map {
          http = 8080
          ssh = 22
          metrics = 9090
        }
      }
      
      # Volume mounting for VM storage
      volume_mount {
        volume      = "vm-data"
        destination = "/host/vm-data"
      }
      
      volume_mount {
        volume      = "vm-logs"
        destination = "/host/vm-logs"
      }
      
      # VM environment configuration
      env {
        # VM identification
        VM_NAME = "{{APP_NAME}}-${NOMAD_ALLOC_INDEX}"
        VM_ID = "${NOMAD_ALLOC_ID}"
        
        # Application configuration
        APP_NAME = "{{APP_NAME}}"
        VERSION = "{{VERSION}}"
        LANE = "F"
        RUNTIME = "full-vm"
        
        # Network configuration
        PORT = "{{HTTP_PORT}}"
        HTTP_PORT = "{{HTTP_PORT}}"
        METRICS_PORT = "9090"
        SSH_PORT = "22"
        {{#if DEBUG_ENABLED}}
        DEBUG_PORT = "2222"
        {{/if}}
        
        # VM resource information
        VM_MEMORY = "{{MEMORY_LIMIT}}"
        VM_CPUS = "{{JVM_CPUS}}"
        
        # Nomad integration
        NOMAD_ALLOC_ID = "${NOMAD_ALLOC_ID}"
        NOMAD_TASK_NAME = "${NOMAD_TASK_NAME}"
        SERVICE_NAME = "{{APP_NAME}}-lane-f"
        
        # Consul integration
        CONSUL_HTTP_ADDR = "${attr.unique.network.ip-address}:8500"
        
        # System configuration
        HOSTNAME = "{{APP_NAME}}-${NOMAD_ALLOC_INDEX}"
        
        {{CUSTOM_ENV_VARS}}
      }
      
      # VM configuration from Consul KV
      {{#if CONSUL_CONFIG_ENABLED}}
      template {
        data = <<EOF
# Full VM application configuration
{{range ls "ploy/apps/{{APP_NAME}}/config"}}
{{.Key}}={{.Value}}
{{end}}

# VM system configuration
{{with key "ploy/shared/config/timezone"}}
TIMEZONE={{.}}
{{end}}
{{with key "ploy/shared/config/locale"}}
LOCALE={{.}}
{{end}}

# Infrastructure services
{{with key "ploy/shared/database/url"}}
DATABASE_URL={{.}}
{{end}}
{{with key "ploy/shared/redis/url"}}
REDIS_URL={{.}}
{{end}}
{{with key "ploy/shared/elasticsearch/url"}}
ELASTICSEARCH_URL={{.}}
{{end}}
{{with key "ploy/shared/kafka/brokers"}}
KAFKA_BROKERS={{.}}
{{end}}

# Monitoring configuration
{{with key "ploy/shared/monitoring/prometheus_url"}}
PROMETHEUS_URL={{.}}
{{end}}
{{with key "ploy/shared/monitoring/grafana_url"}}
GRAFANA_URL={{.}}
{{end}}
EOF
        destination = "local/vm.env"
        env         = true
        change_mode = "restart"
        perms       = "0644"
      }
      {{/if}}
      
      # Comprehensive Vault secrets for VM
      {{#if VAULT_ENABLED}}
      template {
        data = <<EOF
# Database credentials
DATABASE_USERNAME={{with secret "secret/data/{{APP_NAME}}/db"}}{{.Data.data.username}}{{end}}
DATABASE_PASSWORD={{with secret "secret/data/{{APP_NAME}}/db"}}{{.Data.data.password}}{{end}}
DB_ROOT_PASSWORD={{with secret "secret/data/{{APP_NAME}}/db"}}{{.Data.data.root_password}}{{end}}

# Application secrets
API_SECRET_KEY={{with secret "secret/data/{{APP_NAME}}/api"}}{{.Data.data.secret_key}}{{end}}
JWT_SECRET={{with secret "secret/data/{{APP_NAME}}/jwt"}}{{.Data.data.secret}}{{end}}
SESSION_SECRET={{with secret "secret/data/{{APP_NAME}}/session"}}{{.Data.data.secret}}{{end}}
ENCRYPTION_KEY={{with secret "secret/data/{{APP_NAME}}/encryption"}}{{.Data.data.key}}{{end}}

# External services
STRIPE_SECRET_KEY={{with secret "secret/data/{{APP_NAME}}/stripe"}}{{.Data.data.secret_key}}{{end}}
AWS_ACCESS_KEY_ID={{with secret "aws/creds/{{APP_NAME}}-role"}}{{.Data.access_key}}{{end}}
AWS_SECRET_ACCESS_KEY={{with secret "aws/creds/{{APP_NAME}}-role"}}{{.Data.secret_key}}{{end}}
SENDGRID_API_KEY={{with secret "secret/data/{{APP_NAME}}/sendgrid"}}{{.Data.data.api_key}}{{end}}

# OAuth and third-party integrations
GOOGLE_CLIENT_ID={{with secret "secret/data/{{APP_NAME}}/google"}}{{.Data.data.client_id}}{{end}}
GOOGLE_CLIENT_SECRET={{with secret "secret/data/{{APP_NAME}}/google"}}{{.Data.data.client_secret}}{{end}}
GITHUB_CLIENT_ID={{with secret "secret/data/{{APP_NAME}}/github"}}{{.Data.data.client_id}}{{end}}
GITHUB_CLIENT_SECRET={{with secret "secret/data/{{APP_NAME}}/github"}}{{.Data.data.client_secret}}{{end}}

# TLS and PKI
{{with secret "pki/issue/{{APP_NAME}}" "common_name={{APP_NAME}}.service.consul" "ttl=168h"}}
TLS_CERTIFICATE={{.Data.certificate}}
TLS_PRIVATE_KEY={{.Data.private_key}}
TLS_CA_CHAIN={{.Data.ca_chain}}
{{end}}

# SSH keys for VM access
{{with secret "secret/data/{{APP_NAME}}/ssh"}}
SSH_PRIVATE_KEY={{.Data.data.private_key}}
SSH_PUBLIC_KEY={{.Data.data.public_key}}
{{end}}
EOF
        destination = "secrets/vm.env"
        env         = true
        change_mode = "restart"
        perms       = "0600"
      }
      {{/if}}
      
      service {
        name = "{{APP_NAME}}-lane-f-vm"
        port = "http"
        
        tags = [
          "lane-f",
          "full-vm",
          "stateful",
          "version-{{VERSION}}",
          "high-resources",
          "traefik.enable=true",
          "traefik.http.routers.{{APP_NAME}}-f.rule=Host(`{{APP_NAME}}-f.{{DOMAIN_SUFFIX}}`)",
          "traefik.http.routers.{{APP_NAME}}-f.tls.certresolver=letsencrypt",
          "traefik.http.services.{{APP_NAME}}-f.loadbalancer.healthcheck.path=/health",
          "traefik.http.services.{{APP_NAME}}-f.loadbalancer.healthcheck.interval=30s",
          "traefik.http.services.{{APP_NAME}}-f.loadbalancer.sticky.cookie=true"
        ]
        
        check { 
          type     = "http" 
          path     = "/health" 
          interval = "30s" 
          timeout  = "10s"
          check_restart {
            limit = 2
            grace = "60s"
            ignore_warnings = false
          }
        }
        
        check {
          type     = "http"
          path     = "/ready"
          interval = "60s"
          timeout  = "15s"
        }
        
        check {
          type     = "tcp"
          port     = "ssh"
          interval = "60s"
          timeout  = "10s"
        }
        
        meta {
          version = "{{VERSION}}"
          lane = "F"
          runtime = "full-vm"
          isolation = "vm-level"
          persistence = "high"
          resources = "high"
          build_time = "{{BUILD_TIME}}"
        }
      }
      
      # SSH service for VM management
      service {
        name = "{{APP_NAME}}-vm-ssh"
        port = "ssh"
        
        tags = [
          "ssh",
          "management",
          "vm"
        ]
        
        check {
          type     = "tcp"
          interval = "60s"
          timeout  = "10s"
        }
      }
      
      # Metrics service for VM monitoring
      service {
        name = "{{APP_NAME}}-vm-metrics"
        port = "metrics"
        
        tags = [
          "metrics",
          "prometheus",
          "vm"
        ]
        
        check {
          type     = "http"
          path     = "/metrics"
          interval = "30s"
          timeout  = "10s"
        }
      }
      
      # High resource allocation for full VMs
      resources { 
        cpu = {{CPU_LIMIT}}      # High CPU allocation
        memory = {{MEMORY_LIMIT}} # High memory allocation
        disk = {{DISK_SIZE}}     # Significant disk space
      }
      
      logs { 
        max_files = 20
        max_file_size = 100
      }
      
      # VM lifecycle management
      lifecycle {
        hook    = "prestart"
        sidecar = false
      }
      
      # Extended shutdown time for VM cleanup
      kill_timeout = "120s"
      kill_signal = "SIGTERM"
    }
    
    # Conservative migration for resource-intensive VMs
    migrate {
      max_parallel     = 1
      health_check     = "checks"
      min_healthy_time = "45s"
      healthy_deadline = "5m"
    }
  }
}