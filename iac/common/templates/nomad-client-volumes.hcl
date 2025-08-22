# Nomad Client Configuration for Ploy Controller Host Volumes
# This configuration should be added to the Nomad client configuration

client {
  enabled = true
  
  # Host volumes for Ploy Controller
  host_volume "ploy-data" {
    path = "/var/lib/ploy"
    read_only = false
  }
  
  host_volume "ploy-config" {
    path = "/etc/ploy"
    read_only = true
  }
  
  host_volume "ploy-logs" {
    path = "/var/log/ploy"
    read_only = false
  }
  
  # Additional host volumes for build artifacts and temporary storage
  host_volume "ploy-build-cache" {
    path = "/var/cache/ploy/builds"
    read_only = false
  }
  
  host_volume "ploy-artifact-store" {
    path = "/var/lib/ploy/artifacts"
    read_only = false
  }
}