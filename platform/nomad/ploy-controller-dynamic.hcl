job "ploy-controller" {
  datacenters = ["dc1"]
  type = "service"
  priority = 80
  
  constraint {
    attribute = "${attr.kernel.name}"
    value = "linux"
  }
  
  group "controller" {
    count = 3
    
    restart {
      attempts = 3
      interval = "5m"
      delay = "15s"
      mode = "delay"
    }
    
    update {
      max_parallel = 1
      min_healthy_time = "30s"
      healthy_deadline = "5m"
      progress_deadline = "10m"
      auto_revert = true
      auto_promote = false
      canary = 0
      stagger = "30s"
      health_check = "checks"
    }
    
    network {
      port "http" {}
      port "metrics" {}
    }
    
    service {
      name = "ploy-controller"
      port = "http"
      tags = [
        "ploy",
        "controller",
        "api",
        "http",
        "traefik.enable=true",
        "traefik.http.routers.ploy-controller.rule=Host(`api.ployd.app`) || Host(`api.dev.ployd.app`) || PathPrefix(`/v1`)",
        "traefik.http.routers.ploy-controller.tls=true",
        "traefik.http.routers.ploy-controller.tls.certresolver=letsencrypt",
        "traefik.http.services.ploy-controller.loadbalancer.server.scheme=http",
        "traefik.http.services.ploy-controller.loadbalancer.healthcheck.path=/health",
        "traefik.http.services.ploy-controller.loadbalancer.healthcheck.interval=10s",
        "traefik.http.middlewares.ploy-controller-ratelimit.ratelimit.burst=100",
        "traefik.http.middlewares.ploy-controller-secure.headers.sslredirect=true",
        "traefik.http.routers.ploy-controller.middlewares=ploy-controller-ratelimit,ploy-controller-secure",
        "service-mesh.connect=true",
        "service-mesh.protocol=http",
        "blue-green.deployment=true",
        "blue-green.weight=100",
        "${NOMAD_ALLOC_ID}"
      ]
      
      meta {
        version = "{{ env "CONTROLLER_VERSION" }}"
        git_commit = "{{ env "GIT_COMMIT" }}"
        git_branch = "{{ env "GIT_BRANCH" }}"
        build_time = "{{ env "BUILD_TIME" }}"
        node = "${attr.unique.hostname}"
        datacenter = "${node.datacenter}"
        region = "${node.region}"
        deployment_id = "${NOMAD_JOB_ID}-${NOMAD_ALLOC_ID}"
        service_type = "service"
        load_balancer = "traefik"
        health_endpoint = "/health"
        readiness_endpoint = "/ready"
        metrics_endpoint = "/health/metrics"
        api_version = "v1"
        environment = "production"
      }
      
      check {
        type = "http"
        path = "/health"
        port = "http"
        interval = "10s"
        timeout = "5s"
        success_before_passing = 3
        failures_before_critical = 2
        
        check_restart {
          limit = 2
          grace = "20s"
          ignore_warnings = false
        }
        
        header {
          X-Health-Check = ["rolling-update"]
          X-Update-Strategy = ["canary-enabled"]
          X-Service-Mesh = ["ploy-controller"]
        }
      }
      
      check {
        name = "readiness"
        type = "http"
        path = "/ready"
        port = "http"
        interval = "10s"
        timeout = "10s"
        success_before_passing = 3
        failures_before_critical = 2
        
        check_restart {
          limit = 1
          grace = "30s"
        }
        
        header {
          X-Service-Mesh = ["ploy-controller"]
          X-Update-Phase = ["canary-validation"]
          X-Dependency-Check = ["consul,nomad,seaweedfs,vault"]
        }
      }
      
      check {
        name = "liveness"
        type = "http"
        path = "/live"
        port = "http"
        interval = "30s"
        timeout = "3s"
        success_before_passing = 1
        failures_before_critical = 5
      }
      
      check {
        name = "update-progress"
        type = "http"
        path = "/health/update"
        port = "http"
        interval = "15s"
        timeout = "8s"
        success_before_passing = 2
        failures_before_critical = 3
        
        header {
          X-Update-Strategy = ["canary-rollout"]
          X-Update-Phase = ["${META_UPDATE_PHASE}"]
          X-Canary-Status = ["${META_CANARY_STATUS}"]
          X-Rollback-Capability = ["enabled"]
        }
      }
    }
    
    service {
      name = "ploy-controller-metrics"
      port = "metrics"
      tags = [
        "metrics",
        "prometheus",
        "ploy-controller",
        "traefik.enable=true",
        "traefik.http.routers.ploy-metrics.rule=Host(`metrics.ployd.app`) || Host(`metrics.dev.ployd.app`) || PathPrefix(`/metrics`)",
        "traefik.http.routers.ploy-metrics.tls=true",
        "traefik.http.routers.ploy-metrics.tls.certresolver=letsencrypt",
        "traefik.http.services.ploy-metrics.loadbalancer.server.scheme=http",
        "service-mesh.connect=true",
        "service-mesh.protocol=http",
        "monitoring.scrape=true",
        "monitoring.path=/health/metrics"
      ]
      
      meta {
        service_type = "metrics"
        scrape_interval = "15s"
        metrics_format = "prometheus"
        version = "{{ env "CONTROLLER_VERSION" }}"
        environment = "production"
      }
      
      check {
        type = "http"
        path = "/health/metrics"
        port = "http"
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
    
    task "ploy-controller" {
      driver = "raw_exec"
      
      resources {
        cpu = 200
        memory = 256
      }
      
      env {
        # Controller configuration
        PORT = "${NOMAD_PORT_http}"
        METRICS_PORT = "${NOMAD_PORT_metrics}"
        
        # Service discovery addresses
        CONSUL_HTTP_ADDR = "127.0.0.1:8500"
        NOMAD_ADDR = "http://127.0.0.1:4646"
        
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
        HEALTH_CHECK_INTERVAL = "10s"
        READINESS_CHECK_INTERVAL = "10s"
        UPDATE_HEALTH_CHECK_INTERVAL = "15s"
        
        # Rolling update configuration
        ROLLING_UPDATE_ENABLED = "true"
        CANARY_DEPLOYMENT_ENABLED = "true"
        AUTO_ROLLBACK_ENABLED = "true"
        UPDATE_STRATEGY = "rolling-canary"
        
        # Update monitoring
        UPDATE_MONITORING_ENABLED = "true"
        UPDATE_PROGRESS_REPORTING = "true"
        ROLLBACK_THRESHOLD_FAILURES = "2"
        
        # Update timing
        CANARY_PROMOTION_DELAY = "5m"
        UPDATE_STAGGER_DELAY = "30s"
        HEALTH_VALIDATION_TIMEOUT = "5m"
        
        # Binary distribution configuration (templated)
        CONTROLLER_VERSION = "{{ env "CONTROLLER_VERSION" }}"
        GIT_COMMIT = "{{ env "GIT_COMMIT" }}"
        GIT_BRANCH = "{{ env "GIT_BRANCH" }}"
        BUILD_TIME = "{{ env "BUILD_TIME" }}"
        CONTROLLER_BINARY_SOURCE = "seaweedfs"
        BINARY_CACHE_DIR = "/var/lib/ploy/cache/binaries"
        BINARY_INTEGRITY_CHECK = "true"
        
        # DNS configuration
        PLOY_DNS_PROVIDER = "namecheap"
        PLOY_DNS_DOMAIN = "ployd.app"
        PLOY_DNS_TARGET_IP = "45.12.75.241"
        PLOY_DNS_CONFIG_PATH = "/etc/ploy/dns/config.json"
        
        # Namecheap DNS provider configuration
        NAMECHEAP_API_KEY = "c8615d72b5794eb0a52cbf1cf22fc42f"
        NAMECHEAP_SANDBOX_API_KEY = "4ecde47766444cc4b464d017c9dc3749"
        NAMECHEAP_API_USER = "iw2rmb"
        NAMECHEAP_USERNAME = "iw2rmb"
        NAMECHEAP_CLIENT_IP = "45.12.75.241"
        NAMECHEAP_SANDBOX = "false"
        
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
        
        # Platform configuration
        PLOY_APPS_DOMAIN = "ployd.app"
        PLOY_APPS_DOMAIN_PROVIDER = "namecheap"
        
        # ARF Phase 3 - LLM Integration & Learning System
        ARF_LEARNING_DB_URL = "postgres://ploy:arf_dev_password@localhost/arf_learning?sslmode=disable"
        ARF_TREE_SITTER_PATH = "/usr/local/bin/tree-sitter"
        ARF_LLM_CACHE_DIR = "/tmp/arf-llm-cache"
        ARF_AB_TEST_DIR = "/tmp/arf-ab-tests"
        ARF_SANDBOX_BASE_DIR = "/tmp/arf-sandboxes"
        ARF_CACHE_DIR = "/tmp/arf-cache"
        TREE_SITTER_PARSER_DIR = "/usr/local/lib/node_modules"
        JAVA_HOME = "/usr/lib/jvm/java-17-openjdk-amd64"
        OPENREWRITE_JAR_PATH = "/usr/local/bin/rewrite.jar"
      }
      
      template {
        data = <<-EOH
        # Ploy Controller Instance Configuration (Dynamic)
        # Generated with Git-based versioning: {{ env "CONTROLLER_VERSION" }}
        instance_id: {{ env "NOMAD_ALLOC_ID" }}
        node_name: {{ env "attr.unique.hostname" }}
        datacenter: {{ env "node.datacenter" }}
        region: {{ env "node.region" }}
        cluster_id: {{ env "node.unique.id" }}
        
        # Version information from Git
        version:
          full: {{ env "CONTROLLER_VERSION" }}
          git_commit: {{ env "GIT_COMMIT" }}
          git_branch: {{ env "GIT_BRANCH" }}
          build_time: {{ env "BUILD_TIME" }}
        
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
          version: {{ env "CONTROLLER_VERSION" }}
        
        # Traefik configuration
        traefik:
          enabled: {{ env "TRAEFIK_ENABLED" }}
          domain: {{ env "TRAEFIK_DOMAIN" }}
          tls_enabled: {{ env "TRAEFIK_TLS_ENABLED" }}
          cert_resolver: {{ env "TRAEFIK_CERT_RESOLVER" }}
        
        # Health check configuration
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
          
        # Update monitoring
        monitoring:
          enabled: {{ env "UPDATE_MONITORING_ENABLED" }}
          progress_reporting: {{ env "UPDATE_PROGRESS_REPORTING" }}
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
      
      # Binary artifact with templated download URL
      artifact {
        source = "{{ env "CONTROLLER_ARTIFACT_URL" }}"
        destination = "local/controller"
        mode = "file"
        
        options {
          checksum = "{{ env "CONTROLLER_CHECKSUM" }}"
        }
      }
      
      # Binary execution
      config {
        command = "local/controller"
        args = []
      }
      
      lifecycle {
        hook = "prestart"
        sidecar = false
      }
      
      kill_timeout = "60s"
      kill_signal = "SIGTERM"
      
      logs {
        max_files = 5
        max_file_size = 50
      }
    }
  }
}