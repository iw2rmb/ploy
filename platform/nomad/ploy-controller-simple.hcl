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
    
    # Enhanced Consul service registration for testing
    service {
      name = "ploy-controller"
      port = "http"
      tags = [
        "ploy",
        "controller",
        "api",
        "testing",
        "traefik.enable=true",
        "traefik.http.routers.ploy-controller-test.rule=Host(`api-test.ployd.app`) || PathPrefix(`/v1`)",
        "traefik.http.services.ploy-controller-test.loadbalancer.healthcheck.path=/health",
        "service-mesh.connect=true",
        "blue-green.deployment=true",
        "blue-green.weight=50",
        "${NOMAD_ALLOC_ID}"
      ]
      
      # Enhanced metadata for testing environment
      meta {
        version = "1.0.0-test"
        node = "${attr.unique.hostname}"
        datacenter = "${node.datacenter}"
        environment = "testing"
        deployment_id = "${NOMAD_JOB_ID}-${NOMAD_ALLOC_ID}"
        service_type = "service"
      }
      
      # Health check using /health endpoint with service mesh support
      check {
        type = "http"
        path = "/health"
        port = "http"
        interval = "15s"
        timeout = "5s"
        success_before_passing = 1
        failures_before_critical = 2
        check_restart {
          limit = 3
          grace = "10s"
        }
        header {
          X-Service-Mesh = ["ploy-controller-test"]
        }
      }
      
      # Readiness check with auto-deregistration
      check {
        name = "readiness"
        type = "http"
        path = "/ready"
        port = "http"
        interval = "20s"
        timeout = "8s"
        success_before_passing = 1
        failures_before_critical = 2
      }
    }
    
    # Controller task
    task "ploy-controller" {
      driver = "raw_exec"
      
      resources {
        cpu = 200
        memory = 256
      }
      
      # Enhanced environment variables for testing with service mesh
      env {
        PORT = "${NOMAD_PORT_http}"
        CONSUL_HTTP_ADDR = "127.0.0.1:8500"
        NOMAD_ADDR = "http://127.0.0.1:4646"
        PLOY_STORAGE_CONFIG = "/etc/ploy/storage/config.yaml"
        PLOY_USE_CONSUL_ENV = "true"
        PLOY_CLEANUP_AUTO_START = "true"
        
        # Service mesh configuration for testing
        SERVICE_MESH_ENABLED = "true"
        SERVICE_MESH_PROTOCOL = "http"
        CONSUL_CONNECT_ENABLED = "true"
        
        # Blue-green deployment for testing
        BLUE_GREEN_ENABLED = "true"
        DEPLOYMENT_COLOR = "blue"
        DEPLOYMENT_WEIGHT = "50"
        DEPLOYMENT_ID = "${NOMAD_JOB_ID}-${NOMAD_ALLOC_ID}"
        
        # Traefik integration for testing
        TRAEFIK_ENABLED = "true"
        TRAEFIK_DOMAIN = "api-test.ployd.app"
        
        # Service discovery
        SERVICE_NAME = "ploy-controller"
        SERVICE_VERSION = "1.0.0-test"
        
        LOG_LEVEL = "info"
        LOG_SERVICE_MESH = "true"
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