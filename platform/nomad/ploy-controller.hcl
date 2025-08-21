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
    
    # Enhanced update strategy for rolling updates with zero downtime and canary deployment
    update {
      max_parallel = 2           # Update 2 nodes simultaneously for faster rollout
      min_healthy_time = "30s"   # Extended time to ensure service stability
      healthy_deadline = "5m"    # Increased deadline for complex health checks
      progress_deadline = "15m"  # Extended overall timeout for system jobs
      auto_revert = true         # Automatically rollback failed updates
      auto_promote = false       # Require manual promotion for safety
      canary = 1                 # Enable canary deployment with 1 instance
      
      # Stagger updates to prevent simultaneous node failures
      stagger = "30s"            # 30 second delay between parallel updates
      
      # Health checks must pass before promoting canary
      health_check = "checks"    # Use Consul health checks for validation
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
      
      # Enhanced primary health check for rolling update validation
      check {
        type = "http"
        path = "/health"
        port = "http"
        interval = "10s"
        timeout = "5s"
        success_before_passing = 3  # Require 3 consecutive successes for update validation
        failures_before_critical = 2  # Stricter failure tolerance during updates
        
        # Enhanced check restart for rolling updates
        check_restart {
          limit = 2                # Reduced restart limit during updates
          grace = "20s"            # Extended grace period for clean shutdown
          ignore_warnings = false  # Fail fast on warnings during updates
        }
        
        # Custom headers for update validation
        header {
          X-Health-Check = ["rolling-update"]
          X-Update-Strategy = ["canary-enabled"]
          X-Service-Mesh = ["ploy-controller"]
        }
      }
      
      # Enhanced readiness check for rolling update dependency validation
      check {
        name = "readiness"
        type = "http"
        path = "/ready"
        port = "http"
        interval = "10s"           # More frequent checks during updates
        timeout = "10s"            # Extended timeout for dependency checks
        success_before_passing = 3 # Stricter requirement for readiness
        failures_before_critical = 2  # Fail faster on readiness issues
        
        check_restart {
          limit = 1              # Single restart attempt for readiness failures
          grace = "30s"          # Extended grace for dependency cleanup
        }
        
        # Enhanced headers for update validation
        header {
          X-Service-Mesh = ["ploy-controller"]
          X-Update-Phase = ["canary-validation"]
          X-Dependency-Check = ["consul,nomad,seaweedfs,vault"]
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
      
      # Rolling update progress monitoring health check
      check {
        name = "update-progress"
        type = "http"
        path = "/health/update"
        port = "http"
        interval = "15s"
        timeout = "8s"
        success_before_passing = 2
        failures_before_critical = 3
        
        # Custom headers for update progress tracking
        header {
          X-Update-Strategy = ["canary-rollout"]
          X-Update-Phase = ["${META_UPDATE_PHASE}"]  # Will be set during updates
          X-Canary-Status = ["${META_CANARY_STATUS}"]
          X-Rollback-Capability = ["enabled"]
        }
      }
      
      # Enhanced deployment status check with rollback monitoring
      check {
        name = "deployment-status"
        type = "http"
        path = "/health/deployment"
        port = "http"
        interval = "20s"
        timeout = "5s"
        success_before_passing = 1
        failures_before_critical = 2  # Stricter for deployment issues
        
        # Enhanced headers for deployment and rollback tracking
        header {
          X-Deployment-Color = ["blue"]
          X-Deployment-Weight = ["100"]
          X-Auto-Revert = ["enabled"]
          X-Update-Strategy = ["rolling-canary"]
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
        
        # Service discovery and enhanced health checks
        SERVICE_NAME = "ploy-controller"
        SERVICE_VERSION = "1.0.0"
        HEALTH_CHECK_INTERVAL = "10s"
        READINESS_CHECK_INTERVAL = "10s"
        UPDATE_HEALTH_CHECK_INTERVAL = "15s"
        
        # Rolling update configuration
        ROLLING_UPDATE_ENABLED = "true"
        CANARY_DEPLOYMENT_ENABLED = "true"
        AUTO_ROLLBACK_ENABLED = "true"
        UPDATE_STRATEGY = "rolling-canary"
        
        # Update monitoring and alerting
        UPDATE_MONITORING_ENABLED = "true"
        UPDATE_ALERT_WEBHOOK = "https://hooks.slack.com/services/ploy-updates"
        UPDATE_PROGRESS_REPORTING = "true"
        ROLLBACK_THRESHOLD_FAILURES = "2"
        
        # Update timing configuration
        CANARY_PROMOTION_DELAY = "5m"
        UPDATE_STAGGER_DELAY = "30s"
        HEALTH_VALIDATION_TIMEOUT = "5m"
        
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
        
        # Enhanced health check configuration for rolling updates
        health:
          check_interval: {{ env "HEALTH_CHECK_INTERVAL" }}
          readiness_interval: {{ env "READINESS_CHECK_INTERVAL" }}
          update_check_interval: {{ env "UPDATE_HEALTH_CHECK_INTERVAL" }}
          service_name: {{ env "SERVICE_NAME" }}
          
        # Rolling update configuration
        rolling_update:
          enabled: {{ env "ROLLING_UPDATE_ENABLED" }}
          strategy: {{ env "UPDATE_STRATEGY" }}
          canary_enabled: {{ env "CANARY_DEPLOYMENT_ENABLED" }}
          auto_rollback: {{ env "AUTO_ROLLBACK_ENABLED" }}
          promotion_delay: {{ env "CANARY_PROMOTION_DELAY" }}
          stagger_delay: {{ env "UPDATE_STAGGER_DELAY" }}
          health_timeout: {{ env "HEALTH_VALIDATION_TIMEOUT" }}
          rollback_threshold: {{ env "ROLLBACK_THRESHOLD_FAILURES" }}
          
        # Update monitoring and alerting
        monitoring:
          enabled: {{ env "UPDATE_MONITORING_ENABLED" }}
          progress_reporting: {{ env "UPDATE_PROGRESS_REPORTING" }}
          alert_webhook: {{ env "UPDATE_ALERT_WEBHOOK" }}
          metrics_enabled: true
        
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
      
      # Enhanced health check script template
      template {
        data = <<-EOH
        #!/bin/bash
        # Enhanced health check script for Ploy Controller with rolling update support
        set -e
        
        PORT={{ env "NOMAD_PORT_http" }}
        
        # Check if controller is responding
        echo "Checking controller health..."
        curl -f -s -H "X-Health-Check: rolling-update" \
             -H "X-Update-Strategy: canary-enabled" \
             http://localhost:$PORT/health > /dev/null
        
        # Check if ready endpoint is healthy with dependency validation
        echo "Checking controller readiness..."
        curl -f -s -H "X-Update-Phase: canary-validation" \
             -H "X-Dependency-Check: consul,nomad,seaweedfs,vault" \
             http://localhost:$PORT/ready > /dev/null
        
        # Check update progress if monitoring is enabled
        if [ "{{ env "UPDATE_MONITORING_ENABLED" }}" = "true" ]; then
            echo "Checking update progress..."
            curl -f -s -H "X-Update-Strategy: canary-rollout" \
                 http://localhost:$PORT/health/update > /dev/null || echo "Update endpoint not available (normal during steady state)"
        fi
        
        echo "Controller health check passed"
        EOH
        
        destination = "local/health-check.sh"
        perms = "755"
      }
      
      # Rolling update monitoring script template
      template {
        data = <<-EOH
        #!/bin/bash
        # Rolling update monitoring and alerting script
        set -e
        
        PORT={{ env "NOMAD_PORT_http" }}
        WEBHOOK_URL="{{ env "UPDATE_ALERT_WEBHOOK" }}"
        SERVICE_NAME="{{ env "SERVICE_NAME" }}"
        INSTANCE_ID="{{ env "NOMAD_ALLOC_ID" }}"
        
        # Function to send alert
        send_alert() {
            local message="$1"
            local severity="$2"
            local timestamp=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
            
            if [ -n "$WEBHOOK_URL" ]; then
                curl -X POST "$WEBHOOK_URL" \
                     -H "Content-Type: application/json" \
                     -d "{
                         \"text\": \"$severity: $SERVICE_NAME Update Alert\",
                         \"attachments\": [{
                             \"color\": \"$([ \"$severity\" = \"ERROR\" ] && echo \"danger\" || echo \"warning\")\",
                             \"fields\": [
                                 {\"title\": \"Service\", \"value\": \"$SERVICE_NAME\", \"short\": true},
                                 {\"title\": \"Instance\", \"value\": \"$INSTANCE_ID\", \"short\": true},
                                 {\"title\": \"Message\", \"value\": \"$message\", \"short\": false},
                                 {\"title\": \"Timestamp\", \"value\": \"$timestamp\", \"short\": true}
                             ]
                         }]
                     }" 2>/dev/null || echo "Failed to send alert"
            fi
        }
        
        # Check update progress
        if curl -f -s "http://localhost:$PORT/health/update" > /dev/null 2>&1; then
            echo "Update progress monitoring: OK"
        else
            echo "Update endpoint not available (normal during steady state)"
        fi
        
        # Check deployment status
        if ! curl -f -s "http://localhost:$PORT/health/deployment" > /dev/null 2>&1; then
            send_alert "Deployment health check failed for instance $INSTANCE_ID" "ERROR"
            exit 1
        fi
        
        echo "Update monitoring check passed"
        EOH
        
        destination = "local/update-monitor.sh"
        perms = "755"
      }
      
      # Use pre-built binary from ploy directory for now
      # In production, this would be replaced with artifact from SeaweedFS
      
      # Controller startup configuration
      config {
        command = "/home/ploy/ploy/build/controller"
        args = []
        work_dir = "/home/ploy/ploy"
      }
      
      # Enhanced lifecycle hooks for rolling updates
      lifecycle {
        hook = "prestart"
        sidecar = false
      }
      
      # Enhanced service registration with update monitoring
      service {
        name = "ploy-controller-prestart"
        
        # Primary health check
        check {
          name = "startup-health"
          type = "script"
          command = "local/health-check.sh"
          interval = "10s"
          timeout = "8s"
        }
        
        # Update monitoring check
        check {
          name = "update-monitoring"
          type = "script"
          command = "local/update-monitor.sh"
          interval = "30s"
          timeout = "10s"
          success_before_passing = 1
          failures_before_critical = 3
        }
      }
      
      # Enhanced graceful shutdown configuration for rolling updates
      kill_timeout = "60s"      # Extended timeout for rolling updates
      kill_signal = "SIGTERM"   # Standard graceful shutdown signal
      
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