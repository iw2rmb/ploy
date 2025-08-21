job "ploy-controller-test" {
  datacenters = ["dc1"]
  type = "service"
  priority = 80

  constraint {
    attribute = "${attr.kernel.name}"
    value = "linux"
  }

  group "controller" {
    count = 1

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
      canary = 1
      stagger = "30s"
      health_check = "checks"
    }

    network {
      port "http" {
        to = 8081
      }
    }

    service {
      name = "ploy-controller-test"
      port = "http"
      tags = [
        "ploy",
        "controller",
        "test"
      ]

      check {
        type = "http"
        path = "/health"
        port = "http"
        interval = "15s"
        timeout = "5s"
        success_before_passing = 2
        failures_before_critical = 2
      }
    }

    task "controller" {
      driver = "raw_exec"

      resources {
        cpu = 200
        memory = 256
      }

      env {
        PORT = "${NOMAD_PORT_http}"
        CONSUL_HTTP_ADDR = "127.0.0.1:8500"
        NOMAD_ADDR = "http://127.0.0.1:4646"
        PLOY_STORAGE_CONFIG = "/etc/ploy/storage/config.yaml"
        PLOY_USE_CONSUL_ENV = "true"
        PLOY_CLEANUP_AUTO_START = "true"
        LOG_LEVEL = "info"
      }

      # Use artifact-based deployment for proper binary access
      artifact {
        source = "file:///home/ploy/ploy/build/controller"
        destination = "local/"
        mode = "file"
      }

      config {
        command = "local/controller"
        args = []
      }

      kill_timeout = "30s"
      kill_signal = "SIGTERM"

      logs {
        max_files = 3
        max_file_size = 25
      }
    }
  }
}