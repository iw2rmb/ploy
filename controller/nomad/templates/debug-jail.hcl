job "debug-{{APP_NAME}}" {
  datacenters = ["dc1"]
  type = "service"
  namespace = "debug"
  
  group "debug" {
    count = 1
    
    network {
      port "http" { to = 8080 }
      port "ssh" { to = 22 }
    }
    
    task "debug-jail" {
      driver = "jail"
      
      config {
        path = "{{IMAGE_PATH}}"
        host_hostname = "debug-{{APP_NAME}}"
        allow_raw_sockets = true
        ports = ["http", "ssh"]
      }
      
{{ENV_VARS}}
      
      service {
        name = "debug-{{APP_NAME}}"
        port = "http"
        tags = ["debug", "http", "jail"]
        
        check {
          type = "http"
          path = "/healthz"
          interval = "10s"
          timeout = "3s"
        }
      }
      
      service {
        name = "debug-{{APP_NAME}}-ssh"
        port = "ssh"
        tags = ["debug", "ssh", "jail"]
        
        check {
          type = "tcp"
          interval = "10s"
          timeout = "3s"
        }
      }
      
      resources {
        cpu = 500
        memory = 512
      }
    }
  }
  
  # Debug instances should auto-cleanup after 2 hours
  reschedule {
    attempts = 0
    unlimited = false
  }
}