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

    {{#if CONNECT_ENABLED}}
    service {
      name = "{{APP_NAME}}-connect"
      port = "http"
      connect { sidecar_service {} }
      meta {
        version = "{{VERSION}}"
        lane    = "C"
        runtime = "osv-jvm"
      }
    }
    {{/if}}

    task "osv-jvm" {
      driver = "qemu"

      {{#if VAULT_ENABLED}}
      vault { policies = ["{{APP_NAME}}-policy"]; change_mode = "restart" }
      {{/if}}

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

      service {
        name = "{{APP_NAME}}-lane-c-osv"
        port = "http"
        tags = ["lane-c","osv","jvm","version-{{VERSION}}"]
        # Single health check to avoid duplicate default names
        check {
          type     = "http"
          path     = "/actuator/health"
          interval = "15s"
          timeout  = "5s"
        }
        {{#if CONNECT_ENABLED}}
        connect { sidecar_service {} }
        {{/if}}
        meta {
          version      = "{{VERSION}}"
          lane         = "C"
          runtime      = "osv-jvm"
          java_version = "{{JAVA_VERSION}}"
          main_class   = "{{MAIN_CLASS}}"
          build_time   = "{{BUILD_TIME}}"
        }
      }

      service {
        name = "{{APP_NAME}}-jmx"
        port = "jmx"
        check { type = "tcp"; interval = "30s"; timeout = "5s" }
      }
      service {
        name = "{{APP_NAME}}-osv-metrics"
        port = "metrics"
        check { type = "http"; path = "/actuator/prometheus"; interval = "30s"; timeout = "5s" }
      }

      resources { cpu = {{CPU_LIMIT}}; memory = {{MEMORY_LIMIT}} }
      logs { max_files = 5; max_file_size = 20 }
      lifecycle { hook = "prestart"; sidecar = false }
      kill_timeout = "60s"; kill_signal = "SIGTERM"
    }

    migrate { max_parallel = 1; health_check = "checks"; min_healthy_time = "30s"; healthy_deadline = "3m" }
  }
}
