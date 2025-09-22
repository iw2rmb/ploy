job "jetstream-cluster" {
  datacenters = ["dc1"]
  type        = "service"
  priority    = 85

  constraint {
    attribute = "${attr.kernel.name}"
    value     = "linux"
  }

  group "jetstream" {
    count = 1

    restart {
      attempts = 5
      interval = "10m"
      delay    = "30s"
      mode     = "fail"
    }

    reschedule {
      attempts    = 6
      interval    = "30m"
      delay       = "60s"
      max_delay   = "10m"
      unlimited   = false
      delay_function = "exponential"
    }

    update {
      max_parallel      = 1
      min_healthy_time  = "30s"
      healthy_deadline  = "2m"
      progress_deadline = "10m"
      auto_revert       = true
      canary            = 1
      stagger           = "45s"
      health_check      = "checks"
    }

    spread {
      attribute = "${node.unique.id}"
      weight    = 100
    }

    network {
      mode = "host"

      port "client" {
        to = 4222
      }

      port "cluster" {
        to = 6222
      }

      port "monitoring" {
        to = 8222
      }
    }

    volume "jetstream-data" {
      type      = "host"
      read_only = false
      source    = "jetstream-data"
    }

    service {
      name = "nats"
      port = "client"
      tags = [
        "nats",
        "jetstream",
        "traefik.enable=true",
        "traefik.tcp.routers.nats.rule=HostSNI(`*`)",
        "traefik.tcp.routers.nats.entrypoints=nats",
        "traefik.tcp.routers.nats.tls=false",
        "traefik.tcp.routers.nats.service=nats"
      ]

      check {
        name     = "tcp"
        type     = "tcp"
        port     = "client"
        interval = "10s"
        timeout  = "3s"
      }
    }

    service {
      name = "nats-cluster"
      port = "cluster"

      check {
        name     = "cluster-tcp"
        type     = "tcp"
        port     = "cluster"
        interval = "15s"
        timeout  = "5s"
      }
    }

    service {
      name = "nats-monitoring"
      port = "monitoring"
      tags = ["metrics", "nats"]

      check {
        name     = "http"
        type     = "http"
        path     = "/healthz"
        port     = "monitoring"
        interval = "15s"
        timeout  = "5s"
      }
    }

    task "nats-server" {
      driver = "docker"

      env {
        NATS_LOG_LEVEL     = "info"
        NATS_CLUSTER_NAME  = "ploy-jetstream"
        NATS_DOMAIN        = "nats.ploy.local"
        NATS_STORE_DIR     = "/data/jetstream"
        NATS_RESOLVER_DIR  = "/data/accounts"
        NATS_CONNECT_RETRY = "30"
      }

      config {
        image = "nats:2.10.18-alpine"
        ports = ["client", "cluster", "monitoring"]

        entrypoint = ["/bin/sh", "-ec"]
        args = [
          "mkdir -p /data/jetstream /data/accounts && exec nats-server -c /local/nats.conf"
        ]

        logging {
          type = "json-file"
          config {
            max-size = "10m"
            max-file = "5"
          }
        }
      }

      resources {
        cpu    = 800
        memory = 1024
      }

      logs {
        max_files     = 5
        max_file_size = 20
      }

      kill_timeout = "45s"
      shutdown_delay = "10s"

      volume_mount {
        volume      = "jetstream-data"
        destination = "/data"
      }

      template {
        destination = "local/nats.conf"
        change_mode = "restart"
        data = <<-EOT
        server_name: "jetstream-{{ env "NOMAD_ALLOC_INDEX" }}"
        port: {{ env "NOMAD_PORT_client" }}
        http: {{ env "NOMAD_PORT_monitoring" }}
        trace: false
        debug: false
        
        jetstream {
          store_dir: "{{ env "NATS_STORE_DIR" }}"
          max_mem: 1024Mb
          max_file: 64Gb
        }

        cluster {
          name: "{{ env "NATS_CLUSTER_NAME" }}"
          port: {{ env "NOMAD_PORT_cluster" }}
          routes = [
            "nats-route://nats.ploy.local:6222"
          ]
          connect_retries: {{ env "NATS_CONNECT_RETRY" }}
          advertise: "{{ env "NOMAD_ADDR_cluster" }}"
        }

        EOT
      }
    }
  }
}
