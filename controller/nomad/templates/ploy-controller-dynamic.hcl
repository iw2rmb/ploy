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
      delay = "30s"
      mode = "delay"
    }
    
    update {
      max_parallel = 1
      min_healthy_time = "60s"
      healthy_deadline = "10m"
      progress_deadline = "15m"
      auto_revert = true
      auto_promote = false
      canary = 0
      stagger = "45s"
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
        "traefik.http.routers.ploy-controller-dynamic.rule=Host(`api.dev.ployd.app`) || Host(`api.ployd.app`)",
        "traefik.http.routers.ploy-controller-dynamic.tls=true",
        "traefik.http.routers.ploy-controller-dynamic.tls.certresolver=dev-wildcard",
        "traefik.http.routers.ploy-controller-dynamic.tls.domains[0].main=dev.ployd.app",
        "traefik.http.routers.ploy-controller-dynamic.tls.domains[0].sans=*.dev.ployd.app",
        "traefik.http.services.ploy-controller-dynamic.loadbalancer.server.scheme=http",
        "traefik.http.services.ploy-controller-dynamic.loadbalancer.healthcheck.path=/health",
        "traefik.http.services.ploy-controller-dynamic.loadbalancer.healthcheck.interval=15s",
        "blue-green.deployment=true",
        "blue-green.weight=100",
        "${NOMAD_ALLOC_ID}"
      ]
      
      meta {
        version = "arf-test-fixes-9929c5f-dirty-20250823-114620"
        git_commit = "9929c5fc9245825ec3f7a496cd4671ff2fdddedc"
        git_branch = "arf-test-fixes"
        build_timestamp = "20250823-114620"
        node = "${attr.unique.hostname}"
        datacenter = "${node.datacenter}"
        deployment_id = "${NOMAD_JOB_ID}-${NOMAD_ALLOC_ID}"
        service_type = "service"
        environment = "development"
      }
      
      check {
        type = "http"
        path = "/health"
        port = "http"
        interval = "15s"
        timeout = "10s"
        success_before_passing = 1
        failures_before_critical = 3
        
        check_restart {
          limit = 3
          grace = "30s"
          ignore_warnings = false
        }
      }
      
      check {
        name = "readiness"
        type = "http"
        path = "/ready"
        port = "http"
        interval = "20s"
        timeout = "15s"
        success_before_passing = 1
        failures_before_critical = 3
      }
      
      check {
        name = "liveness"
        type = "http"
        path = "/live"
        port = "http"
        interval = "30s"
        timeout = "5s"
        success_before_passing = 1
        failures_before_critical = 5
      }
    }
    
    service {
      name = "ploy-controller-metrics"
      port = "metrics"
      tags = [
        "metrics",
        "prometheus",
        "ploy-controller",
        "monitoring.scrape=true",
        "monitoring.path=/health/metrics"
      ]
      
      check {
        type = "http"
        path = "/health/metrics"
        port = "http"
        interval = "30s"
        timeout = "5s"
        success_before_passing = 1
        failures_before_critical = 3
      }
    }
    
    task "ploy-controller" {
      driver = "raw_exec"
      
      resources {
        cpu = 200
        memory = 256
      }
      
      env {
        # Core service configuration
        PORT = "${NOMAD_PORT_http}"
        METRICS_PORT = "${NOMAD_PORT_metrics}"
        
        # Version information (injected at build time)
        PLOY_VERSION = "arf-test-fixes-9929c5f-dirty-20250823-114620"
        GIT_COMMIT = "9929c5fc9245825ec3f7a496cd4671ff2fdddedc"
        GIT_BRANCH = "arf-test-fixes"
        BUILD_TIMESTAMP = "20250823-114620"
        
        # Service discovery
        CONSUL_HTTP_ADDR = "127.0.0.1:8500"
        NOMAD_ADDR = "http://127.0.0.1:4646"
        
        # Configuration paths
        PLOY_STORAGE_CONFIG = "/etc/ploy/storage/config.yaml"
        PLOY_CLEANUP_CONFIG = "/etc/ploy/cleanup/config.yaml"
        
        # Service configuration
        PLOY_USE_CONSUL_ENV = "true"
        PLOY_ENV_STORE_PATH = "/var/lib/ploy/env-store"
        PLOY_CLEANUP_AUTO_START = "true"
        
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
        
        # Platform configuration
        PLOY_APPS_DOMAIN = "ployd.app"
        PLOY_APPS_DOMAIN_PROVIDER = "namecheap"
        
        # ARF Configuration
        ARF_LEARNING_DB_URL = "postgres://ploy:arf_dev_password@localhost/arf_learning?sslmode=disable"
        ARF_TREE_SITTER_PATH = "/usr/local/bin/tree-sitter"
        ARF_LLM_CACHE_DIR = "/tmp/arf-llm-cache"
        ARF_AB_TEST_DIR = "/tmp/arf-ab-tests"
        ARF_SANDBOX_BASE_DIR = "/tmp/arf-sandboxes"
        ARF_CACHE_DIR = "/tmp/arf-cache"
        TREE_SITTER_PARSER_DIR = "/usr/local/lib/node_modules"
        JAVA_HOME = "/usr/lib/jvm/java-17-openjdk-amd64"
        OPENREWRITE_JAR_PATH = "/usr/local/bin/rewrite.jar"
        
        # Logging
        LOG_LEVEL = "info"
        LOG_FORMAT = "json"
        
        # Instance identification
        INSTANCE_ID = "${NOMAD_ALLOC_ID}"
        NODE_NAME = "${attr.unique.hostname}"
        CLUSTER_ID = "${node.unique.id}"
      }
      
      # Configuration template
      template {
        data = <<-EOH
        # Ploy Controller Dynamic Configuration
        # Generated for version: arf-test-fixes-9929c5f-dirty-20250823-114620
        # Build time: 20250823-114620
        # Git commit: 9929c5fc9245825ec3f7a496cd4671ff2fdddedc
        
        instance_id: {{ env "NOMAD_ALLOC_ID" }}
        version: "arf-test-fixes-9929c5f-dirty-20250823-114620"
        git_commit: "9929c5fc9245825ec3f7a496cd4671ff2fdddedc"
        git_branch: "arf-test-fixes"
        build_timestamp: "20250823-114620"
        
        service:
          name: "ploy-controller"
          port: {{ env "NOMAD_PORT_http" }}
          metrics_port: {{ env "NOMAD_PORT_metrics" }}
          
        health:
          check_interval: "10s"
          readiness_interval: "10s"
          service_name: "ploy-controller"
          
        deployment:
          version: "arf-test-fixes-9929c5f-dirty-20250823-114620"
          deployment_id: "{{ env "NOMAD_JOB_ID" }}-{{ env "NOMAD_ALLOC_ID" }}"
          node: "{{ env "attr.unique.hostname" }}"
          datacenter: "{{ env "node.datacenter" }}"
          
        max_concurrent_builds: 3
        build_timeout: "30m"
        storage_timeout: "5m"
        EOH
        
        destination = "local/controller.yaml"
        change_mode = "restart"
      }
      
      # Dynamic binary download from SeaweedFS
      artifact {
        source = "http://45.12.75.241:8888/ploy-artifacts/controller-binaries/arf-test-fixes-9929c5f-dirty-20250823-114620/linux/amd64/controller"
        destination = "local/controller"
        mode = "file"
        
        options {
          checksum = "sha256:6741f5264a30056dbf4f198c8b8543eb3a0af538ef740e2cafdeb462b455a059"
        }
      }
      
      
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
