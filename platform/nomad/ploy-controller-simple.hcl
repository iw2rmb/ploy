job "ploy-controller-simple" {
  datacenters = ["dc1"]
  type = "service"  # Use service type for testing, can be changed to system later
  priority = 70
  
  # Constraint to run only on Linux nodes
  constraint {
    attribute = "${attr.kernel.name}"
    value = "linux"
  }
  
  group "controller" {
    count = 2  # Deploy 2 instances for high availability testing
    
    # Restart policy for critical infrastructure
    restart {
      attempts = 3
      interval = "5m"
      delay = "15s"
      mode = "delay"
    }
    
    # Update strategy for rolling updates
    update {
      max_parallel = 1
      min_healthy_time = "15s"
      healthy_deadline = "3m"
      progress_deadline = "5m"
      auto_revert = true
      auto_promote = false
    }
    
    # Network configuration
    network {
      port "http" {
        to = 8081
      }
    }
    
    # Consul service registration
    service {
      name = "ploy-controller"
      port = "http"
      tags = [
        "ploy",
        "controller",
        "api",
        "${NOMAD_ALLOC_ID}"
      ]
      
      # Health check using /health endpoint
      check {
        type = "http"
        path = "/health"
        port = "http"
        interval = "15s"
        timeout = "5s"
        check_restart {
          limit = 3
          grace = "10s"
        }
      }
      
      # Readiness check
      check {
        name = "readiness"
        type = "http"
        path = "/ready"
        port = "http"
        interval = "20s"
        timeout = "8s"
      }
    }
    
    # Controller task
    task "ploy-controller" {
      driver = "raw_exec"
      
      resources {
        cpu = 200
        memory = 256
      }
      
      # Environment variables
      env {
        PORT = "${NOMAD_PORT_http}"
        CONSUL_HTTP_ADDR = "127.0.0.1:8500"
        NOMAD_ADDR = "http://127.0.0.1:4646"
        PLOY_STORAGE_CONFIG = "/etc/ploy/storage/config.yaml"
        PLOY_USE_CONSUL_ENV = "true"
        PLOY_CLEANUP_AUTO_START = "true"
        LOG_LEVEL = "info"
        INSTANCE_ID = "${NOMAD_ALLOC_ID}"
        NODE_NAME = "${attr.unique.hostname}"
      }
      
      # Binary location - use pre-built controller from ploy directory
      config {
        command = "/home/ploy/ploy/build/controller"
        args = []
      }
      
      # Graceful shutdown
      kill_timeout = "30s"
      kill_signal = "SIGTERM"
      
      # Log configuration
      logs {
        max_files = 3
        max_file_size = 25
      }
    }
  }
}