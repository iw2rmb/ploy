job "traefik-system" {
  datacenters = ["dc1"]
  type = "system"  # Runs on every Nomad client node
  priority = 90    # High priority for infrastructure service
  
  constraint {
    attribute = "${attr.kernel.name}"
    value = "linux"
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
        
        ports = ["http", "https", "admin", "metrics", "traefik"]
        
        args = [
          "--global.checkNewVersion=false",
          "--global.sendAnonymousUsage=false",
          "--log.level=INFO",
          "--api.dashboard=true",
          "--api.insecure=true",
          "--ping=true",
          "--entrypoints.web.address=:80",
          "--entrypoints.websecure.address=:443", 
          "--entrypoints.admin.address=:8090",
          "--entrypoints.metrics.address=:8091",
          "--entrypoints.traefik.address=:8095",
          "--providers.consulcatalog=true",
          "--providers.consulcatalog.prefix=traefik",
          "--providers.consulcatalog.exposedByDefault=false",
          "--providers.consulcatalog.endpoint.address=127.0.0.1:8500",
          "--providers.consulcatalog.endpoint.scheme=http",
          "--providers.file.directory=/etc/traefik/dynamic",
          "--metrics.prometheus=true",
          "--metrics.prometheus.addEntryPointsLabels=true",
          "--metrics.prometheus.addRoutersLabels=true",
          "--metrics.prometheus.addServicesLabels=true"
        ]
        
        mount {
          type = "bind"
          source = "local/dynamic"
          target = "/etc/traefik/dynamic"
          readonly = true
        }
        
        # Host mount for Let's Encrypt certificates
        mount {
          type = "bind"
          source = "/opt/ploy/traefik-data"
          target = "/data"
        }
      }
      
      # Basic dynamic configuration directory
      template {
        data = <<EOF
# Basic dynamic configuration placeholder
# This file will be populated by the Ploy controller
# when applications register their routing rules
http:
  routers: {}
  services: {}
  middlewares: {}
EOF
        destination = "local/dynamic/apps.yml"
        perms = "644"
      }
      
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
  }
}