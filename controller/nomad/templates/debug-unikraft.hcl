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
    
    task "debug-unikernel" {
      driver = "qemu"
      
      config {
        image_path = "{{IMAGE_PATH}}"
        accelerator = "kvm"
        args = ["-netdev", "user,id=net0,hostfwd=tcp::8080-:8080,hostfwd=tcp::22-:22", "-device", "virtio-net,netdev=net0"]
        ports = ["http", "ssh"]
      }
      
{{ENV_VARS}}
      
      service {
        name = "debug-{{APP_NAME}}"
        port = "http"
        tags = ["debug", "http", "unikraft"]
        
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
        tags = ["debug", "ssh", "unikraft"]
        
        check {
          type = "tcp"
          interval = "10s"
          timeout = "3s"
        }
      }
      
      resources {
        cpu = 500
        memory = 256
      }
    }
  }
  
  # Debug instances should auto-cleanup after 2 hours
  reschedule {
    attempts = 0
    unlimited = false
  }
}