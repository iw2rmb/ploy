job "{{APP_NAME}}-lane-d" {
  datacenters = ["dc1"]
  type = "service"
  group "edge" {
    count = 2
    network { port "http" { to = 8080 } }
    task "nginx" {
      driver = "exec"
      config {
        command = "/usr/local/sbin/nginx"
        args = ["-c","${NOMAD_TASK_DIR}/nginx.conf","-g","daemon off;"]
      }
      template {
        destination = "local/nginx.conf"
        change_mode = "restart"
        data = <<EOF
{{ file "../../apps/nginx-edge/nginx.conf" }}
EOF
      }
{{ENV_VARS}}
      service { name = "{{APP_NAME}}-lane-d-jail" port = "http" }
      resources { cpu = 200 memory = 128 }
    }
  }
}
