job "vm-bhyve" {
  datacenters = ["dc1"]
  group "g" {
    network { port "http" { to = 8080 } }
    task "vm" {
      driver = "qemu"
      config { image_path = "local/vm.img" args = ["-nographic"] }
      resources { cpu = 1000 memory = 1024 }
    }
  }
}
