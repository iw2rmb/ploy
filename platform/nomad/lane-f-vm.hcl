job "lane-f-vm" {
  datacenters = ["dc1"]
  type = "service"
  group "db" {
    count = 1
    network { port "db" { to = 5432 } }
    task "vm" {
      driver = "qemu"
      config {
        image_path = "local/${NOMAD_TASK_DIR}/postgres.img"
        args = ["-nographic"]
      }
      resources { cpu = 2000 memory = 4096 }
    }
  }
}
