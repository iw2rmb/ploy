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
  
  # Note: System jobs cannot use affinity or spread blocks
  # Placement handled by system job scheduling across all nodes
  
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
    
    # Note: System jobs do not support reschedule policies
    # System jobs automatically reschedule on node failures
    
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
    
    # Enhanced Consul service registration for load balancing and service mesh
    service {
      name = "ploy-controller"
      port = "http"
      tags = [
        "ploy",
        "controller",
        "api",
        "http",
        "traefik.enable=true",
        "traefik.http.routers.ploy-controller.rule=Host(`api.ployd.app`) || PathPrefix(`/v1`)",
        "traefik.http.routers.ploy-controller.tls=true",
        "traefik.http.routers.ploy-controller.tls.certresolver=letsencrypt",
        "traefik.http.services.ploy-controller.loadbalancer.server.scheme=http",
        "traefik.http.services.ploy-controller.loadbalancer.healthcheck.path=/health",
        "traefik.http.services.ploy-controller.loadbalancer.healthcheck.interval=10s",
        "traefik.http.middlewares.ploy-controller-auth.basicauth.usersfile=/etc/traefik/users",
        "traefik.http.middlewares.ploy-controller-ratelimit.ratelimit.burst=100",
        "traefik.http.middlewares.ploy-controller-secure.headers.sslredirect=true",
        "traefik.http.routers.ploy-controller.middlewares=ploy-controller-ratelimit,ploy-controller-secure",
        "service-mesh.connect=true",
        "service-mesh.protocol=http",
        "blue-green.deployment=true",
        "blue-green.weight=100",
        "${NOMAD_ALLOC_ID}"  # Include allocation ID for identification
      ]
      
      # Enhanced metadata for service discovery and deployment management
      meta {
        version = "1.0.0"
        node = "${attr.unique.hostname}"
        datacenter = "${node.datacenter}"
        region = "${node.region}"
        deployment_id = "${NOMAD_JOB_ID}-${NOMAD_ALLOC_ID}"
        service_type = "system"
        load_balancer = "traefik"
        health_endpoint = "/health"
        readiness_endpoint = "/ready"
        metrics_endpoint = "/health/metrics"
        api_version = "v1"
        environment = "production"
      }
      
      # Primary health check using the /health endpoint
      check {
        type = "http"
        path = "/health"
        port = "http"
        interval = "10s"
        timeout = "5s"
        success_before_passing = 2  # Require 2 consecutive successes before passing
        failures_before_critical = 3  # Allow 3 failures before marking critical
        check_restart {
          limit = 3
          grace = "10s"
          ignore_warnings = false
        }
      }
      
      # Readiness check using the /ready endpoint with service mesh awareness
      check {
        name = "readiness"
        type = "http"
        path = "/ready"
        port = "http"
        interval = "15s"
        timeout = "8s"
        success_before_passing = 2
        failures_before_critical = 2
        check_restart {
          limit = 2
          grace = "15s"
        }
        # Service mesh connectivity check
        header {
          X-Service-Mesh = ["ploy-controller"]
        }
      }
      
      # Liveness check for basic connectivity and automatic deregistration
      check {
        name = "liveness"
        type = "http"
        path = "/live"
        port = "http"
        interval = "30s"
        timeout = "3s"
        success_before_passing = 1
        failures_before_critical = 5  # Allow more failures for liveness
      }
      
      # Blue-green deployment health check
      check {
        name = "deployment-status"
        type = "http"
        path = "/health/deployment"
        port = "http"
        interval = "20s"
        timeout = "5s"
        success_before_passing = 1
        failures_before_critical = 3
        # Custom header for blue-green deployment tracking
        header {
          X-Deployment-Color = ["blue"]
          X-Deployment-Weight = ["100"]
        }
      }
    }
    
    # Enhanced metrics service for monitoring integration with service mesh
    service {
      name = "ploy-controller-metrics"
      port = "metrics"
      tags = [
        "metrics",
        "prometheus",
        "ploy-controller",
        "traefik.enable=true",
        "traefik.http.routers.ploy-metrics.rule=Host(`metrics.ployd.app`) || PathPrefix(`/metrics`)",
        "traefik.http.routers.ploy-metrics.tls=true",
        "traefik.http.routers.ploy-metrics.tls.certresolver=letsencrypt",
        "traefik.http.services.ploy-metrics.loadbalancer.server.scheme=http",
        "traefik.http.middlewares.ploy-metrics-auth.basicauth.usersfile=/etc/traefik/metrics-users",
        "traefik.http.routers.ploy-metrics.middlewares=ploy-metrics-auth",
        "service-mesh.connect=true",
        "service-mesh.protocol=http",
        "monitoring.scrape=true",
        "monitoring.path=/health/metrics"
      ]
      
      # Metrics service metadata
      meta {
        service_type = "metrics"
        scrape_interval = "15s"
        metrics_format = "prometheus"
        version = "1.0.0"
        environment = "production"
      }
      
      # Metrics endpoint health check with service mesh awareness
      check {
        type = "http"
        path = "/health/metrics"
        port = "http"  # Use main HTTP port as metrics are served there
        interval = "30s"
        timeout = "5s"
        success_before_passing = 1
        failures_before_critical = 3
        header {
          X-Service-Mesh = ["ploy-controller-metrics"]
          Accept = ["text/plain; version=0.0.4"]
        }
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
        
        # Service mesh configuration
        SERVICE_MESH_ENABLED = "true"
        SERVICE_MESH_PROTOCOL = "http"
        SERVICE_MESH_CONNECT = "true"
        CONSUL_CONNECT_ENABLED = "true"
        
        # Blue-green deployment configuration
        BLUE_GREEN_ENABLED = "true"
        DEPLOYMENT_COLOR = "blue"
        DEPLOYMENT_WEIGHT = "100"
        DEPLOYMENT_ID = "${NOMAD_JOB_ID}-${NOMAD_ALLOC_ID}"
        
        # Traefik integration
        TRAEFIK_ENABLED = "true"
        TRAEFIK_DOMAIN = "api.ployd.app"
        TRAEFIK_TLS_ENABLED = "true"
        TRAEFIK_CERT_RESOLVER = "letsencrypt"
        
        # Service discovery and health checks
        SERVICE_NAME = "ploy-controller"
        SERVICE_VERSION = "1.0.0"
        HEALTH_CHECK_INTERVAL = "10s"
        READINESS_CHECK_INTERVAL = "15s"
        
        # Logging configuration
        LOG_LEVEL = "info"
        LOG_FORMAT = "json"
        LOG_SERVICE_MESH = "true"
        
        # Nomad integration
        NOMAD_NODE_ID = "${attr.unique.hostname}"
        NOMAD_DATACENTER = "${node.datacenter}"
        NOMAD_REGION = "${node.region}"
        
        # Instance identification
        INSTANCE_ID = "${NOMAD_ALLOC_ID}"
        NODE_NAME = "${attr.unique.hostname}"
        CLUSTER_ID = "${node.unique.id}"
      }
      
      # Enhanced configuration files with service mesh and blue-green deployment
      template {
        data = <<-EOH
        # Ploy Controller Instance Configuration
        # Generated automatically by Nomad with service mesh integration
        instance_id: {{ env "NOMAD_ALLOC_ID" }}
        node_name: {{ env "attr.unique.hostname" }}
        datacenter: {{ env "node.datacenter" }}
        region: {{ env "node.region" }}
        cluster_id: {{ env "node.unique.id" }}
        
        # Service endpoints
        consul_addr: {{ env "attr.unique.network.ip-address" }}:8500
        nomad_addr: http://{{ env "attr.unique.network.ip-address" }}:4646
        
        # Service mesh configuration
        service_mesh:
          enabled: {{ env "SERVICE_MESH_ENABLED" }}
          protocol: {{ env "SERVICE_MESH_PROTOCOL" }}
          connect: {{ env "SERVICE_MESH_CONNECT" }}
          consul_connect: {{ env "CONSUL_CONNECT_ENABLED" }}
        
        # Blue-green deployment configuration
        deployment:
          enabled: {{ env "BLUE_GREEN_ENABLED" }}
          color: {{ env "DEPLOYMENT_COLOR" }}
          weight: {{ env "DEPLOYMENT_WEIGHT" }}
          deployment_id: {{ env "DEPLOYMENT_ID" }}
          version: {{ env "SERVICE_VERSION" }}
        
        # Traefik load balancer configuration
        traefik:
          enabled: {{ env "TRAEFIK_ENABLED" }}
          domain: {{ env "TRAEFIK_DOMAIN" }}
          tls_enabled: {{ env "TRAEFIK_TLS_ENABLED" }}
          cert_resolver: {{ env "TRAEFIK_CERT_RESOLVER" }}
        
        # Health check configuration
        health:
          check_interval: {{ env "HEALTH_CHECK_INTERVAL" }}
          readiness_interval: {{ env "READINESS_CHECK_INTERVAL" }}
          service_name: {{ env "SERVICE_NAME" }}
        
        # Resource limits
        max_concurrent_builds: 3
        build_timeout: "30m"
        storage_timeout: "5m"
        
        # Service discovery settings
        service_discovery:
          auto_deregister: true
          deregister_after: "60s"
          health_endpoint: "/health"
          readiness_endpoint: "/ready"
          metrics_endpoint: "/health/metrics"
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
      
      # Use pre-built binary from ploy directory for now
      # In production, this would be replaced with artifact from SeaweedFS
      
      # Controller startup configuration
      config {
        command = "/home/ploy/ploy/build/controller"
        args = []
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
      
      # Note: Volume mounts commented out - requires host volume configuration first
      # These would be enabled in production with proper volume setup
    }
    
    # Note: Host volumes commented out - would need to be configured in Nomad client first
    # These would be enabled in production deployment
    
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
  
  # Note: Vault integration and parameterized jobs removed for system job compatibility
  # Vault can be enabled when needed, parameterized jobs only work with batch jobs
}