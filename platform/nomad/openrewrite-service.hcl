job "openrewrite-service" {
  datacenters = ["dc1"]
  type        = "service"
  
  group "openrewrite" {
    count = 1  # Start with 1 instance for testing
    
    # Scaling configuration
    scaling {
      enabled = true
      min     = 1  # Start with 1 instance for testing
      max     = 3  # Reduce max for testing
    }
    
    # Network configuration
    network {
      port "http" {
        static = 8092
      }
      port "metrics" {
        static = 8093
      }
    }
    
    # Enhanced restart policy
    restart {
      attempts = 3
      interval = "5m"
      delay    = "15s"
      mode     = "delay"
    }
    
    # Reschedule policy for reliability
    reschedule {
      delay          = "30s"
      delay_function = "exponential"
      max_delay      = "5m"
      unlimited      = true
    }
    
    # OpenRewrite service task
    task "openrewrite" {
      driver = "docker"
      
      config {
        image = "ab311200d170"  # Use Docker image ID directly
        force_pull = false  # Prevent registry pull, use local image
        ports = ["http", "metrics"]
        
        # tmpfs mount for transformations
        mount {
          type   = "tmpfs"
          target = "/tmp/openrewrite"
          tmpfs_options {
            size = 4294967296  # 4GB for transformations
          }
        }
        
        # Volume mount for cache
        mount {
          type   = "bind"
          source = "/var/lib/ploy/cache"
          target = "/app/cache"
        }
      }
      
      # Environment configuration
      env {
        # Port configuration
        PORT = "${NOMAD_PORT_http}"
        METRICS_PORT = "${NOMAD_PORT_metrics}"
        
        # Service discovery
        CONSUL_ADDRESS = "${attr.unique.network.ip-address}:8500"
        SEAWEEDFS_MASTER = "seaweedfs.service.consul:9333"
        
        # Worker configuration
        WORKER_POOL_SIZE = "2"
        MAX_CONCURRENT_JOBS = "3"
        
        # Auto-shutdown configuration
        AUTO_SHUTDOWN_MINUTES = "10"
        ACTIVITY_CHECK_INTERVAL = "30s"
        
        # Storage paths
        OPENREWRITE_WORKSPACE = "/tmp/openrewrite"
        CACHE_DIR = "/app/cache"
        
        # Java configuration
        JAVA_HOME = "/opt/java/openjdk"
        JAVA_OPTS = "-Xmx3g -Xms1g -Dmaven.repo.local=/home/openrewrite/.m2/repository"
        
        # Maven configuration
        MAVEN_OPTS = "-Xmx2g -Xms512m"
        MAVEN_CONFIG = "/home/openrewrite/.m2"
        
        # Service metadata
        SERVICE_NAME = "openrewrite"
        SERVICE_VERSION = "1.0.0"
        INSTANCE_ID = "${NOMAD_ALLOC_ID}"
        NODE_NAME = "${attr.unique.hostname}"
        
        # Logging
        LOG_LEVEL = "info"
        LOG_FORMAT = "json"
      }
      
      # Configuration template for Consul KV integration
      template {
        data = <<-EOH
        # OpenRewrite Service Configuration
        instance_id: {{ env "NOMAD_ALLOC_ID" }}
        node_name: {{ env "attr.unique.hostname" }}
        datacenter: {{ env "node.datacenter" }}
        
        # Service endpoints
        consul_addr: {{ env "attr.unique.network.ip-address" }}:8500
        seaweedfs_master: seaweedfs.service.consul:9333
        
        # Worker pool configuration
        worker_pool_size: {{ env "WORKER_POOL_SIZE" }}
        max_concurrent_jobs: {{ env "MAX_CONCURRENT_JOBS" }}
        
        # Auto-shutdown configuration
        auto_shutdown_minutes: {{ env "AUTO_SHUTDOWN_MINUTES" }}
        activity_check_interval: {{ env "ACTIVITY_CHECK_INTERVAL" }}
        
        # Resource limits
        max_memory_per_job: "2GB"
        transformation_timeout: "15m"
        
        # Storage configuration
        workspace_dir: {{ env "OPENREWRITE_WORKSPACE" }}
        cache_dir: {{ env "CACHE_DIR" }}
        
        # Java configuration
        java_home: {{ env "JAVA_HOME" }}
        java_opts: {{ env "JAVA_OPTS" }}
        EOH
        
        destination = "local/openrewrite.yaml"
        change_mode = "restart"
      }
      
      # Health check script
      template {
        data = <<-EOH
        #!/bin/bash
        # OpenRewrite service health check
        set -e
        
        PORT={{ env "NOMAD_PORT_http" }}
        
        # Check main health endpoint
        echo "Checking OpenRewrite service health..."
        curl -f -s http://localhost:$PORT/v1/openrewrite/health > /dev/null
        
        # Check service info endpoint
        echo "Checking service info endpoint..."
        curl -f -s http://localhost:$PORT/ > /dev/null
        
        # Check transformation endpoint availability
        echo "Checking transformation endpoint..."
        curl -f -s -X POST http://localhost:$PORT/v1/openrewrite/transform \
          -H "Content-Type: application/json" \
          -d '{"test": "availability"}' > /dev/null || echo "Transform endpoint not ready yet"
        
        echo "OpenRewrite service health check passed"
        EOH
        
        destination = "local/health-check.sh"
        perms = "755"
      }
      
      # Resource allocation - increased for Maven operations
      resources {
        cpu    = 2000  # 2 CPU cores for Maven builds
        memory = 4096  # 4GB RAM for Maven operations
        disk   = 4096  # 4GB disk for Maven cache and logs
      }
      
      # Graceful shutdown
      kill_timeout = "60s"
      kill_signal  = "SIGTERM"
      
      # Log configuration
      logs {
        max_files     = 3
        max_file_size = 50  # MB
      }
    }
    
    # Service registration
    service {
      name = "openrewrite"
      port = "http"
      tags = [
        "openrewrite",
        "transformation",
        "java",
        "queue-worker",
        "traefik.enable=true",
        "traefik.http.routers.openrewrite.rule=Host(`openrewrite.service.consul`)",
        "traefik.http.services.openrewrite.loadbalancer.server.scheme=http",
        "traefik.http.services.openrewrite.loadbalancer.healthcheck.path=/health",
        "traefik.http.services.openrewrite.loadbalancer.healthcheck.interval=10s"
      ]
      
      meta {
        version = "1.0.0"
        worker_pool_size = "2"
        max_concurrent_jobs = "3"
        auto_shutdown_minutes = "10"
      }
      
      # Primary health check
      check {
        type     = "http"
        path     = "/health"
        port     = "http"
        interval = "15s"
        timeout  = "10s"
        success_before_passing = 1
        failures_before_critical = 5
        
        check_restart {
          limit = 2
          grace = "20s"
          ignore_warnings = false
        }
        
        header {
          X-Service-Check = ["openrewrite-primary"]
        }
      }
      
      # Readiness check for job processing capability
      check {
        name     = "readiness"
        type     = "http"
        path     = "/ready"
        port     = "http"
        interval = "15s"
        timeout  = "8s"
        success_before_passing = 2
        failures_before_critical = 2
        
        header {
          X-Service-Check = ["openrewrite-readiness"]
        }
      }
      
      # Worker pool status check
      check {
        name     = "worker-status"
        type     = "http"
        path     = "/status"
        port     = "http"
        interval = "30s"
        timeout  = "5s"
        success_before_passing = 1
        failures_before_critical = 5
        
        header {
          X-Service-Check = ["openrewrite-workers"]
        }
      }
    }
    
    # Metrics service
    service {
      name = "openrewrite-metrics"
      port = "metrics"
      tags = [
        "metrics",
        "prometheus",
        "openrewrite"
      ]
      
      meta {
        scrape_interval = "15s"
        metrics_format = "prometheus"
      }
      
      check {
        type     = "http"
        path     = "/metrics"
        port     = "metrics"
        interval = "30s"
        timeout  = "5s"
        success_before_passing = 1
        failures_before_critical = 3
        
        header {
          Accept = ["text/plain; version=0.0.4"]
        }
      }
    }
  }
}