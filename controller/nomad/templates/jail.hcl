job "nginx-edge" {
  datacenters = ["dc1"]
  group "g" {
    network { port "http" { to = 8080 } }
    task "nginx" {
      driver = "exec"
      config { command = "/usr/local/sbin/nginx" args = ["-c","${NOMAD_TASK_DIR}/nginx.conf","-g","'daemon off;'"] }
      template { destination = "local/nginx.conf" data = file("../../apps/nginx-edge/nginx.conf") }
      service { name = "nginx-edge" port = "http" }
      resources { cpu = 200 memory = 64 }
    }
  }
}
