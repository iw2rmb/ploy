job "{{APP_NAME}}-lane-c" {
  datacenters = ["dc1"]
  type = "service"
  priority = 60

  update {
    max_parallel     = 1
    min_healthy_time = "45s"
    healthy_deadline = "5m"
    progress_deadline = "15m"
    auto_revert      = true
    canary           = 1
    stagger          = "60s"
  }

  group "app" {
    count = {{INSTANCE_COUNT}}

    restart {
      attempts = 3
      interval = "3m"
      delay = "30s"
      mode = "fail"
    }

    reschedule {
      delay          = "30s"
      delay_function = "exponential"
      max_delay      = "2h"
      unlimited      = true
    }

    network {
      port "http" { to = {{HTTP_PORT}} }
      port "metrics" { to = 9090 }
      port "jmx" { to = 9999 }
    }

    # host volume removed for dev simplicity

    task "osv-jvm" {
      driver = "qemu"

      config {
        image_path = "{{IMAGE_PATH}}"
        args = [
          "-nographic",
          "-smp", "{{JVM_CPUS}}",
          "-m", "{{JVM_MEMORY}}M",
          "-netdev", "user,id=net0,hostfwd=tcp::${NOMAD_PORT_http}-:{{HTTP_PORT}},hostfwd=tcp::${NOMAD_PORT_jmx}-:9999",
          "-device", "virtio-net-pci,netdev=net0"
        ]
        accelerator = "kvm"
        kvm = true
        machine = "q35"
        cpu = "host"
      }

      # volume mount removed

      env {
        JAVA_OPTS = "{{JVM_OPTS}}"
        JVM_MEMORY = "{{JVM_MEMORY}}"
        JVM_CPUS = "{{JVM_CPUS}}"
        APP_NAME = "{{APP_NAME}}"
        VERSION = "{{VERSION}}"
        LANE = "C"
        MAIN_CLASS = "{{MAIN_CLASS}}"
        SERVER_PORT = "{{HTTP_PORT}}"
        METRICS_PORT = "9090"
        JMX_PORT = "9999"
        SERVICE_NAME = "{{APP_NAME}}-lane-c"
        NOMAD_ALLOC_ID = "${NOMAD_ALLOC_ID}"
        NOMAD_TASK_NAME = "${NOMAD_TASK_NAME}"
        {{CUSTOM_ENV_VARS}}
      }

      # No Consul service registration in dev-minimal template
      {{#if CONSUL_CONFIG_ENABLED}}
      service {
        name = "{{APP_NAME}}-lane-c"
        port = "http"
        tags = [
          "lane-c",
          "app={{APP_NAME}}",
          "version={{VERSION}}",
          "runtime=jvm"
        ]
        check {
          name     = "http-health"
          type     = "http"
          path     = "/health"
          interval = "10s"
          timeout  = "2s"
        }
      }

      # Optional metrics service (HTTP)
      service {
        name = "{{APP_NAME}}-lane-c-metrics"
        port = "metrics"
        tags = [
          "lane-c",
          "app={{APP_NAME}}",
          "metrics",
          "runtime=jvm"
        ]
        check {
          name     = "http-metrics"
          type     = "http"
          path     = "/metrics"
          interval = "15s"
          timeout  = "2s"
        }
      }

      # Optional JMX service (TCP) for JVM tooling
      service {
        name = "{{APP_NAME}}-lane-c-jmx"
        port = "jmx"
        tags = [
          "lane-c",
          "app={{APP_NAME}}",
          "jmx",
          "runtime=jvm"
        ]
        check {
          name     = "jmx-tcp"
          type     = "tcp"
          interval = "20s"
          timeout  = "2s"
        }
      }
      {{/if}}

      resources { cpu = {{CPU_LIMIT}}; memory = {{MEMORY_LIMIT}} }
      logs { max_files = 5; max_file_size = 20 }
      lifecycle { hook = "prestart"; sidecar = false }
      kill_timeout = "60s"; kill_signal = "SIGTERM"
    }

    # Minimal migration settings
    migrate { max_parallel = 1; health_check = "checks"; min_healthy_time = "30s"; healthy_deadline = "3m" }
  }
}
