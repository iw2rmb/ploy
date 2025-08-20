job "traefik-system" {
  datacenters = ["dc1"]
  type = "service"
  priority = 90
  
  constraint {
    attribute = "${attr.kernel.name}"
    value = "linux"
  }
  
  group "traefik" {
    count = 1
    
    restart {
      attempts = 3
      interval = "5m"
      delay = "15s"
      mode = "fail"
    }
    
    network {
      port "http" {
        static = 80
      }
      port "https" {
        static = 443  
      }
      port "admin" {
        static = 8080
      }
    }
    
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
    
    task "traefik" {
      driver = "raw_exec"
      
      config {
        command = "/usr/local/bin/traefik"
        args = ["--configfile=local/traefik.yml"]
      }
      
      template {
        data = <<EOF
# Traefik Basic Configuration
global:
  checkNewVersion: false
  sendAnonymousUsage: false

log:
  level: INFO

# API and Dashboard
api:
  dashboard: true
  debug: false
  insecure: true

# Ping endpoint
ping: {}

# Entry Points
entryPoints:
  web:
    address: ":80"
  
  websecure:
    address: ":443"
  
  traefik:
    address: ":8080"

# Simple file provider for now (we'll add Consul later)
providers:
  file:
    directory: /opt/ploy/traefik/dynamic
    watch: true
EOF
        destination = "local/traefik.yml"
        perms = "644"
      }
      
      resources {
        cpu    = 200
        memory = 128
      }
      
      env {
        CONSUL_HTTP_ADDR = "127.0.0.1:8500"
      }
    }
  }
}