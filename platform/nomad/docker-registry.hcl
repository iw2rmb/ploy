job "docker-registry" {
  datacenters = ["dc1"]
  type = "service"
  priority = 75  # High priority for container registry service
  
  # Constraint to run only on Linux nodes
  constraint {
    attribute = "${attr.kernel.name}"
    value = "linux"
  }
  
  group "registry" {
    count = 1  # Single instance for simplicity initially
    
    # Restart policy for registry service
    restart {
      attempts = 3
      interval = "5m"
      delay = "15s"
      mode = "delay"
    }
    
    # Update strategy for rolling updates
    update {
      max_parallel = 1
      min_healthy_time = "30s"
      healthy_deadline = "3m"
      progress_deadline = "5m"
      auto_revert = true
      stagger = "30s"
      health_check = "checks"
    }
    
    # Network configuration
    network {
      port "http" {}  # Dynamic port allocation for multiple instances
    }
    
    # Consul service registration
    service {
      name = "docker-registry"
      port = "http"
      tags = [
        "registry",
        "docker",
        "container-registry",
        "traefik.enable=true",
        "traefik.http.routers.docker-registry.rule=Host(`registry.dev.ployman.app`)",
        "traefik.http.routers.docker-registry.tls=true",
        "traefik.http.routers.docker-registry.tls.certresolver=default-acme",
        "traefik.http.routers.docker-registry.tls.domains[0].main=dev.ployman.app",
        "traefik.http.routers.docker-registry.tls.domains[0].sans=*.dev.ployman.app",
        "traefik.http.services.docker-registry.loadbalancer.server.scheme=http",
        "traefik.http.services.docker-registry.loadbalancer.healthcheck.path=/",
        "traefik.http.services.docker-registry.loadbalancer.healthcheck.interval=10s",
      ]
      
      meta {
        version = "2.8.3"
        storage_backend = "seaweedfs"
        environment = "dev"
      }
      
      # Health check for Docker Registry
      check {
        type = "http"
        path = "/"
        port = "http"
        interval = "10s"
        timeout = "3s"
        success_before_passing = 2
        failures_before_critical = 3
      }
      
      # Registry API health check
      check {
        name = "registry-api"
        type = "http"
        path = "/v2/"
        port = "http"
        interval = "30s"
        timeout = "5s"
        success_before_passing = 1
        failures_before_critical = 2
      }
    }
    
    # Docker Registry task
    task "registry" {
      driver = "docker"
      
      resources {
        cpu = 200
        memory = 256
      }
      
      
      # Environment variables for Docker Registry
      env {
        # Registry configuration
        REGISTRY_VERSION = "0.1"
        REGISTRY_LOG_LEVEL = "info"
        REGISTRY_LOG_FORMATTER = "json"
        
        # Storage configuration - Filesystem with SeaweedFS mount
        REGISTRY_STORAGE = "filesystem"
        REGISTRY_STORAGE_FILESYSTEM_ROOTDIRECTORY = "/var/lib/registry"
        
        # HTTP configuration
        REGISTRY_HTTP_ADDR = "0.0.0.0:${NOMAD_PORT_http}"
        REGISTRY_HTTP_SECRET = "registry-http-secret-key"
        
        # Health check configuration - disable storage driver health check
        REGISTRY_HEALTH_STORAGEDRIVER_ENABLED = "false"
        REGISTRY_HEALTH_STORAGEDRIVER_INTERVAL = "10s"
        REGISTRY_HEALTH_STORAGEDRIVER_THRESHOLD = "3"
        
        # Delete support (for image cleanup)
        REGISTRY_STORAGE_DELETE_ENABLED = "true"
        
        # Validation and compatibility
        REGISTRY_VALIDATION_MANIFESTS_URLS_ALLOW = "[\"^https?://\"]"
        REGISTRY_COMPATIBILITY_SCHEMA1_ENABLED = "true"
      }
      
      # Registry configuration template
      template {
        data = <<-EOH
        version: 0.1
        log:
          level: {{ env "REGISTRY_LOG_LEVEL" }}
          formatter: {{ env "REGISTRY_LOG_FORMATTER" }}
        
        storage:
          filesystem:
            rootdirectory: {{ env "REGISTRY_STORAGE_FILESYSTEM_ROOTDIRECTORY" }}
          
          delete:
            enabled: {{ env "REGISTRY_STORAGE_DELETE_ENABLED" }}
        
        http:
          addr: 0.0.0.0:{{ env "NOMAD_PORT_http" }}
          secret: {{ env "REGISTRY_HTTP_SECRET" }}
          
        health:
          storagedriver:
            enabled: {{ env "REGISTRY_HEALTH_STORAGEDRIVER_ENABLED" }}
            interval: {{ env "REGISTRY_HEALTH_STORAGEDRIVER_INTERVAL" }}
            threshold: {{ env "REGISTRY_HEALTH_STORAGEDRIVER_THRESHOLD" }}
        
        validation:
          manifests:
            urls:
              allow: {{ env "REGISTRY_VALIDATION_MANIFESTS_URLS_ALLOW" }}
        
        compatibility:
          schema1:
            enabled: {{ env "REGISTRY_COMPATIBILITY_SCHEMA1_ENABLED" }}
        EOH
        
        destination = "local/config.yml"
        change_mode = "restart"
      }
      
      # Docker configuration
      config {
        image = "registry:2.8.3"
        ports = ["http"]

        volumes = [
          "local/config.yml:/etc/docker/registry/config.yml:ro",
          "/opt/ploy/registry:/var/lib/registry"
        ]
        
        logging {
          type = "json-file"
          config {
            max-size = "10m"
            max-file = "3"
          }
        }
      }
      
      # Lifecycle hooks
      lifecycle {
        hook = "prestart"
        sidecar = false
      }
      
      # Graceful shutdown
      kill_timeout = "30s"
      kill_signal = "SIGTERM"
      
      # Log configuration
      logs {
        max_files = 3
        max_file_size = 10  # MB
      }
    }
  }
}
