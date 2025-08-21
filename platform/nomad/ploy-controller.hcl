job "ploy-controller" {
  datacenters = ["dc1"]
  type = "system"  # Runs on every Nomad client node for high availability
  priority = 80    # High priority for core infrastructure service
  
  # Constraint to run only on Linux nodes
  constraint {
    attribute = "${attr.kernel.name}"
    value = "linux"
  }
  
  # Optional constraint to run only on nodes with sufficient resources
  constraint {
    attribute = "${attr.memory.totalbytes}"
    operator = ">="
    value = "1073741824"  # 1GB minimum memory
  }
  
  # Affinity rule for optimal placement - prefer nodes with more CPU
  affinity {
    attribute = "${attr.cpu.numcores}"
    operator = ">="
    value = "2"
    weight = 50
  }
  
  # Spread across different availability zones if labeled
  spread {
    attribute = "${meta.zone}"
    weight = 100
  }
  
  group "controller" {
    # Run 1 instance per node (system job behavior)
    count = 1
    
    # Restart policy for critical infrastructure
    restart {
      attempts = 5           # Allow more restart attempts for critical service
      interval = "10m"       # Reset attempt counter every 10 minutes
      delay = "15s"          # Wait 15 seconds between restarts
      mode = "delay"         # Continue trying to restart with exponential backoff
    }
    
    # Reschedule policy for node failures
    reschedule {
      attempts = 3           # Try to reschedule up to 3 times
      interval = "24h"       # Reset attempt counter daily
      delay = "30s"          # Initial delay before rescheduling
      delay_function = "exponential"  # Exponential backoff
      max_delay = "10m"      # Maximum delay between attempts
      unlimited = false      # Don't reschedule indefinitely
    }
    
    # Update strategy for rolling updates with zero downtime
    update {
      max_parallel = 1       # Update one node at a time for system jobs
      min_healthy_time = "15s"   # Wait for service to be healthy
      healthy_deadline = "3m"    # Give up if not healthy within 3 minutes
      progress_deadline = "10m"  # Overall update timeout
      auto_revert = true         # Automatically rollback failed updates
      auto_promote = false       # Require manual promotion for safety
      canary = 0                 # No canary deployment for system jobs
    }
    
    # Network configuration
    network {
      port "http" {
        to = 8081          # Controller HTTP port
      }
      port "metrics" {
        to = 9090          # Metrics port for monitoring
      }
    }
    
    # Consul service registration for load balancing
    service {
      name = "ploy-controller"
      port = "http"
      tags = [
        "ploy",
        "controller",
        "api",
        "http",
        "${NOMAD_ALLOC_ID}"  # Include allocation ID for identification
      ]
      
      # Add metadata for service discovery
      meta {
        version = "${meta.ploy_version}"
        node = "${attr.unique.hostname}"
        datacenter = "${attr.consul.datacenter}"
      }
      
      # Primary health check using the /health endpoint
      check {
        type = "http"
        path = "/health"
        port = "http"
        interval = "10s"
        timeout = "5s"
        check_restart {
          limit = 3
          grace = "10s"
          ignore_warnings = false
        }
      }
      
      # Readiness check using the /ready endpoint
      check {
        name = "readiness"
        type = "http"
        path = "/ready"
        port = "http"
        interval = "15s"
        timeout = "8s"
        check_restart {
          limit = 2
          grace = "15s"
        }
      }
      
      # Liveness check for basic connectivity
      check {
        name = "liveness"
        type = "http"
        path = "/live"
        port = "http"
        interval = "30s"
        timeout = "3s"
      }
    }
    
    # Metrics service for monitoring integration
    service {
      name = "ploy-controller-metrics"
      port = "metrics"
      tags = [
        "metrics",
        "prometheus",
        "ploy-controller"
      ]
      
      check {
        type = "http"
        path = "/health/metrics"
        port = "http"  # Use main HTTP port as metrics are served there
        interval = "30s"
        timeout = "5s"
      }
    }
    
    # Main controller task
    task "ploy-controller" {
      driver = "raw_exec"
      
      # Resource allocation
      resources {
        cpu = 200      # 200 MHz (0.2 CPU cores)
        memory = 256   # 256 MB RAM
        
        # Reserve additional resources for burst workloads
        memory_max = 512  # Allow burst up to 512 MB
      }
      
      # Environment variables for configuration
      env {
        # Controller configuration
        PORT = "${NOMAD_PORT_http}"
        
        # Service discovery addresses
        CONSUL_HTTP_ADDR = "${attr.unique.network.ip-address}:8500"
        NOMAD_ADDR = "http://${attr.unique.network.ip-address}:4646"
        
        # External configuration paths
        PLOY_STORAGE_CONFIG = "/etc/ploy/storage/config.yaml"
        PLOY_CLEANUP_CONFIG = "/etc/ploy/cleanup/config.yaml"
        
        # Service configuration
        PLOY_USE_CONSUL_ENV = "true"
        PLOY_ENV_STORE_PATH = "/var/lib/ploy/env-store"
        PLOY_CLEANUP_AUTO_START = "true"
        
        # Logging configuration
        LOG_LEVEL = "info"
        LOG_FORMAT = "json"
        
        # Nomad integration
        NOMAD_NODE_ID = "${attr.unique.hostname}"
        NOMAD_DATACENTER = "${node.datacenter}"
        NOMAD_REGION = "${node.region}"
        
        # Instance identification
        INSTANCE_ID = "${NOMAD_ALLOC_ID}"
        NODE_NAME = "${attr.unique.hostname}"
      }
      
      # Configuration files
      template {
        data = <<-EOH
        # Ploy Controller Instance Configuration
        # Generated automatically by Nomad
        instance_id: {{ env "NOMAD_ALLOC_ID" }}
        node_name: {{ env "attr.unique.hostname" }}
        datacenter: {{ env "node.datacenter" }}
        region: {{ env "node.region" }}
        
        # Service endpoints
        consul_addr: {{ env "attr.unique.network.ip-address" }}:8500
        nomad_addr: http://{{ env "attr.unique.network.ip-address" }}:4646
        
        # Resource limits
        max_concurrent_builds: 3
        build_timeout: "30m"
        storage_timeout: "5m"
        EOH
        
        destination = "local/controller.yaml"
        change_mode = "restart"
      }
      
      # Health check script template
      template {
        data = <<-EOH
        #!/bin/bash
        # Health check script for Ploy Controller
        set -e
        
        # Check if controller is responding
        curl -f -s http://localhost:{{ env "NOMAD_PORT_http" }}/health > /dev/null
        
        # Check if ready endpoint is healthy
        curl -f -s http://localhost:{{ env "NOMAD_PORT_http" }}/ready > /dev/null
        
        echo "Controller health check passed"
        EOH
        
        destination = "local/health-check.sh"
        perms = "755"
      }
      
      # Binary artifact configuration
      artifact {
        source = "file:///usr/local/bin/ploy-controller"
        destination = "local/ploy-controller"
        mode = "file"
        options {
          checksum = "file:///usr/local/bin/ploy-controller.sha256"
        }
      }
      
      # Controller startup configuration
      config {
        command = "local/ploy-controller"
        args = []
        
        # Process configuration
        pid_mode = "private"
        ipc_mode = "private"
      }
      
      # Lifecycle hooks
      lifecycle {
        hook = "prestart"
        sidecar = false
      }
      
      # Service registration delay to ensure readiness
      service {
        name = "ploy-controller-prestart"
        check {
          type = "script"
          command = "local/health-check.sh"
          interval = "10s"
          timeout = "5s"
        }
      }
      
      # Graceful shutdown configuration
      kill_timeout = "30s"
      kill_signal = "SIGTERM"
      
      # Log configuration
      logs {
        max_files = 5
        max_file_size = 50  # MB
      }
      
      # Volume mounts for persistent data
      volume_mount {
        volume = "ploy-data"
        destination = "/var/lib/ploy"
        read_only = false
      }
      
      volume_mount {
        volume = "ploy-config"
        destination = "/etc/ploy"
        read_only = true
      }
      
      volume_mount {
        volume = "ploy-logs"
        destination = "/var/log/ploy"
        read_only = false
      }
    }
    
    # Shared data volume for application state
    volume "ploy-data" {
      type = "host"
      source = "ploy-data"
      read_only = false
    }
    
    # Configuration volume (read-only)
    volume "ploy-config" {
      type = "host"
      source = "ploy-config"
      read_only = true
    }
    
    # Logs volume for centralized logging
    volume "ploy-logs" {
      type = "host"
      source = "ploy-logs"
      read_only = false
    }
    
    # Ephemeral disk for temporary build artifacts
    ephemeral_disk {
      size = 1000     # 1GB for temporary build files
      migrate = false # Don't migrate on updates
      sticky = false  # Don't preserve across restarts
    }
  }
  
  # Job-level metadata for operational tracking
  meta {
    service = "ploy-controller"
    version = "1.0.0"
    environment = "production"
    contact = "ploy-team@organization.com"
    documentation = "https://docs.ploy.dev/controller"
  }
  
  # Vault integration for secrets management
  vault {
    policies = ["ploy-controller"]
    change_mode = "restart"
  }
  
  # Parameterized job configuration for different environments
  parameterized {
    meta_optional = ["environment", "log_level", "storage_backend"]
    meta_required = []
  }
}