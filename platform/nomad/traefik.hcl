job "traefik-system" {
  datacenters = ["dc1"]
  type = "system"  # Runs on every Nomad client node
  priority = 90    # High priority for infrastructure service
  
  constraint {
    attribute = "${attr.kernel.name}"
    value = "linux"
  }
  
  # Run only on edge/gateway nodes (Nomad client config: client.meta.role = "gateway")
  constraint {
    attribute = "${meta.role}"
    value     = "gateway"
  }
  
  group "traefik" {
    count = 1
    
    # Restart policy for critical infrastructure
    restart {
      attempts = 3
      interval = "5m"
      delay = "15s"
      mode = "fail"
    }
    
    # Update strategy for rolling updates
    update {
      max_parallel = 1
      min_healthy_time = "30s"
      healthy_deadline = "2m"
      progress_deadline = "5m"
      auto_revert = true
    }
    
    network {
      port "http" {
        static = 80
        to = 80
      }
      port "https" {
        static = 443
        to = 443
      }
      port "admin" {
        static = 8090
        to = 8090
      }
      port "metrics" {
        static = 8091
        to = 8091
      }
      port "traefik" {
        static = 8095
        to = 8095
      }
      port "nats" {
        static = 4222
        to = 4222
      }
    }
    
    # Consul service registration for Traefik
    service {
      name = "traefik"
      port = "admin"
      tags = [
        "traefik",
        "load-balancer",
        "ingress"
      ]
      
      check {
        type = "http"
        path = "/ping"
        port = "admin"
        interval = "10s"
        timeout = "3s"
      }
    }
    
    # Metrics endpoint for monitoring
    service {
      name = "traefik-metrics"
      port = "metrics"
      tags = [
        "metrics",
        "prometheus"
      ]
      
      check {
        type = "http"
        path = "/metrics"
        port = "metrics"
        interval = "30s"
        timeout = "5s"
    }
  }

    task "traefik" {
      driver = "docker"
      
      config {
        image = "traefik:v3.5.0"
        network_mode = "host"

        ports = ["http", "https", "admin", "metrics", "traefik", "nats"]
        
        args = [
          "--global.checkNewVersion=false",
          "--global.sendAnonymousUsage=false",
          "--log.level=INFO",
          "--api.dashboard=true",
          "--api.insecure=true",
          "--ping=true",
          "--ping.entryPoint=admin",
          "--entrypoints.web.address=:80",
          "--entrypoints.websecure.address=:443", 
          "--entrypoints.admin.address=:8090",
          "--entrypoints.metrics.address=:8091",
          "--entrypoints.traefik.address=:8095",
          "--entrypoints.nats.address=:4222",
          "--providers.consulcatalog=true",
          "--providers.consulcatalog.prefix=traefik",
          "--providers.consulcatalog.exposedByDefault=false",
          "--providers.consulcatalog.endpoint.address=127.0.0.1:8500",
          "--providers.consulcatalog.endpoint.scheme=http",
          # Token is provided via CONSUL_HTTP_TOKEN environment variable when Consul ACLs are enabled
          "--providers.file.filename=/etc/traefik/dynamic-configs/dynamic-config.yml",
          "--providers.file.watch=true",
          "--metrics.prometheus=true",
          "--metrics.prometheus.addEntryPointsLabels=true",
          "--metrics.prometheus.addRoutersLabels=true",
          "--metrics.prometheus.addServicesLabels=true",
          # Default ACME resolver uses HTTP-01 (port 80) with TLS-ALPN fallback (port 443)
          "--entrypoints.websecure.http.tls.certresolver=default-acme",
          "--certificatesresolvers.default-acme.acme.email=admin@ployman.app",
          "--certificatesresolvers.default-acme.acme.storage=/data/default-acme.json",
          "--certificatesresolvers.default-acme.acme.httpchallenge=true",
          "--certificatesresolvers.default-acme.acme.httpchallenge.entrypoint=web",
          "--certificatesresolvers.default-acme.acme.tlschallenge=true",
          "--certificatesresolvers.default-acme.acme.caserver=https://acme-v02.api.letsencrypt.org/directory",
          # HTTP to HTTPS redirect
          "--entrypoints.web.http.redirections.entrypoint.to=websecure",
          "--entrypoints.web.http.redirections.entrypoint.scheme=https"
        ]
        
        # Host mount for Let's Encrypt certificates
        mount {
          type = "bind"
          source = "/opt/ploy/traefik-data"
          target = "/data"
        }
        
        # Host mount for dynamic configuration directory
        mount {
          type = "bind"
          source = "/opt/ploy/traefik-data"
          target = "/etc/traefik/dynamic-configs"
          readonly = true
        }
      }
      
      # Configuration now loaded from external file provider at /etc/traefik/dynamic-configs/dynamic-config.yml
      
      # Environment variables for Traefik
      env {
        # Consul configuration
        CONSUL_HTTP_ADDR = "127.0.0.1:8500"
        
      }
      
      resources {
        cpu    = 200   # 200 MHz
        memory = 128   # 128 MB
      }
      
      # Logging configuration
      logs {
        max_files     = 5
        max_file_size = 50
      }
      
      # Kill timeout
      kill_timeout = "30s"
    }

    task "routing-sync" {
      driver = "docker"

      config {
        image        = "registry.dev.ployman.app/ploy/traefik-sync:latest"
        network_mode = "host"

        mount {
          type   = "bind"
          source = "/opt/ploy/traefik-data"
          target = "/data"
        }
      }

      env {
        PLOY_ROUTING_JETSTREAM_URL          = "nats://nats.ploy.local:4222"
        PLOY_ROUTING_OBJECT_BUCKET          = "routing_maps"
        PLOY_ROUTING_EVENT_STREAM           = "routing_events"
        PLOY_ROUTING_EVENT_SUBJECT_PREFIX   = "routing.app"
        PLOY_TRAEFIK_DYNAMIC_CONFIG         = "/data/dynamic-config.yml"
        PLOY_TRAEFIK_ROUTING_DURABLE        = "traefik-routing-sync"
      }

      resources {
        cpu    = 100
        memory = 64
      }
    }
  }
}
