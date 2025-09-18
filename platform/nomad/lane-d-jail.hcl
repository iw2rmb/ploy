job "{{APP_NAME}}-lane-d" {
  datacenters = ["dc1"]
  type = "service"
  priority = 55  # Good priority for FreeBSD jails
  
  # Canary deployment strategy for FreeBSD jails
  update {
    max_parallel     = 1
    min_healthy_time = "20s"
    healthy_deadline = "3m"
    progress_deadline = "8m"
    auto_revert      = true
    canary           = 1
    stagger          = "25s"
  }
  
  group "app" {
    count = {{INSTANCE_COUNT}}
    
    restart { 
      attempts = 4
      interval = "2m" 
      delay = "20s" 
      mode = "fail" 
    }
    
    reschedule {
      delay          = "25s"
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
        to = 2222  # SSH for debugging
      }
      {{/if}}
    }
    
    # Persistent volumes for jail data
    {{#if VOLUME_ENABLED}}
    volume "jail-data" {
      type      = "host"
      source    = "jail-data"
      read_only = false
    }
    {{/if}}
    
    task "jail" {
      driver = "jail"
      
      config {
        path = "{{IMAGE_PATH}}"
        
        # Jail configuration
        hostname = "{{APP_NAME}}-${NOMAD_ALLOC_INDEX}"
        allow_raw_exec = true
        exec_timeout = "60s"
        
        # Network configuration for jail
        ip4 = "inherit"
        ip6 = "inherit"
        
        # Security and resource limits
        allow_chflags = false
        allow_mount = false
        allow_quotas = false
        allow_raw_sockets = false
        
        # FreeBSD-specific options
        devfs_ruleset = 4  # Standard devfs ruleset
        enforce_statfs = 2
        
        {{#if VOLUME_ENABLED}}
        # Mount points for persistent data
        mount_devfs = true
        mount_fdescfs = true
        {{/if}}
      }
      
      # Volume mounting for jails
      {{#if VOLUME_ENABLED}}
      volume_mount {
        volume      = "jail-data"
        destination = "/app/data"
      }
      {{/if}}
      
      # FreeBSD jail environment variables
      env {
        # Application configuration
        APP_NAME = "{{APP_NAME}}"
        VERSION = "{{VERSION}}"
        LANE = "D"
        RUNTIME = "freebsd-jail"
        
        # Network configuration
        PORT = "${NOMAD_PORT_http}"
        HTTP_PORT = "${NOMAD_PORT_http}"
        METRICS_PORT = "${NOMAD_PORT_metrics}"
        {{#if DEBUG_ENABLED}}
        SSH_PORT = "${NOMAD_PORT_debug}"
        {{/if}}
        
        # FreeBSD specific
        JAIL_NAME = "{{APP_NAME}}-${NOMAD_ALLOC_INDEX}"
        JAIL_IP = "${attr.unique.network.ip-address}"
        
        # System paths and configuration
        PATH = "/sbin:/bin:/usr/sbin:/usr/bin:/usr/local/sbin:/usr/local/bin"
        HOME = "/app"
        USER = "app"
        
        # Nomad integration
        NOMAD_ALLOC_ID = "${NOMAD_ALLOC_ID}"
        NOMAD_TASK_NAME = "${NOMAD_TASK_NAME}"
        SERVICE_NAME = "{{APP_NAME}}-lane-d"
        
        # Service registration (Consul service discovery only)
        
        # FreeBSD jail resource limits
        RLIMIT_DATA = "{{MEMORY_LIMIT}}M"
        RLIMIT_STACK = "8M"
        
        {{CUSTOM_ENV_VARS}}
      }
      
      # Application configuration from Consul KV
      {{#if CONSUL_CONFIG_ENABLED}}
      template {
        data = <<EOF
# FreeBSD jail application configuration
{{range ls "ploy/apps/{{APP_NAME}}/config"}}
export {{.Key}}="{{.Value}}"
{{end}}

# System configuration
{{with key "ploy/shared/config/timezone"}}
export TZ={{.}}
{{end}}
{{with key "ploy/shared/config/locale"}}
export LANG={{.}}
{{end}}
EOF
        destination = "local/jail.env"
        env         = true
        change_mode = "restart"
        perms       = "0644"
      }
      {{/if}}
      
      service {
        name = "{{APP_NAME}}-lane-d-jail"
        port = "http"
        
        tags = [
          "lane-d",
          "freebsd-jail",
          "native-performance",
          "version-{{VERSION}}",
          "secure-isolation",
          "traefik.enable=true",
          "traefik.http.routers.{{APP_NAME}}-d.rule=Host(`{{APP_NAME}}-d.{{DOMAIN_SUFFIX}}`)",
          "traefik.http.routers.{{APP_NAME}}-d.tls.certresolver=letsencrypt",
          "traefik.http.services.{{APP_NAME}}-d.loadbalancer.healthcheck.path=/health",
          "traefik.http.services.{{APP_NAME}}-d.loadbalancer.healthcheck.interval=15s"
        ]
        
        check { 
          type     = "http" 
          path     = "/health" 
          interval = "15s" 
          timeout  = "5s"
          check_restart {
            limit = 3
            grace = "20s"
            ignore_warnings = false
          }
        }
        
        check {
          type     = "http"
          path     = "/ready"
          interval = "30s"
          timeout  = "8s"
        }
        
        meta {
          version = "{{VERSION}}"
          lane = "D"
          runtime = "freebsd-jail"
          isolation = "os-level"
          performance = "native"
          build_time = "{{BUILD_TIME}}"
        }
      }
      
      # Metrics service
      service {
        name = "{{APP_NAME}}-jail-metrics"
        port = "metrics"
        
        tags = [
          "metrics",
          "prometheus",
          "freebsd"
        ]
        
        check {
          type     = "http"
          path     = "/metrics"
          interval = "30s"
          timeout  = "5s"
        }
      }
      
      # Resource allocation for FreeBSD jails
      resources { 
        cpu = {{CPU_LIMIT}}      # Native performance with minimal overhead
        memory = {{MEMORY_LIMIT}} # Efficient memory usage
        {{#if DISK_SIZE}}
        disk = {{DISK_SIZE}}     # Local storage for jail
        {{/if}}
      }
      
      logs { 
        max_files = 10
        max_file_size = 50
      }
      
      # Jail lifecycle management
      lifecycle {
        hook    = "prestart"
        sidecar = false
      }
      
      # FreeBSD jail shutdown
      kill_timeout = "45s"
      kill_signal = "SIGTERM"
    }
    
    # Migration optimized for jail portability
    migrate {
      max_parallel     = 1
      health_check     = "checks"
      min_healthy_time = "15s"
      healthy_deadline = "2m"
    }
  }
}
