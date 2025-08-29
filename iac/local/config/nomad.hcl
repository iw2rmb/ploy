# Nomad configuration for local development
datacenter = "local"
data_dir = "/nomad/data"
log_level = "INFO"

# Server configuration
server {
  enabled = true
  bootstrap_expect = 1
  
  # Development mode settings
  encrypt = ""
  
  # Job garbage collection
  job_gc_threshold = "5m"
  eval_gc_threshold = "5m"
  deployment_gc_threshold = "5m"
}

# Client configuration
client {
  enabled = true
  
  # Host networking for local development
  network_interface = "eth0"
  
  # CPU and memory limits for local testing
  reserved {
    cpu = 100
    memory = 256
  }
  
  # Enable host volumes
  host_volume "host_data" {
    path = "/tmp/nomad-host-data"
    read_only = false
  }
}

# Consul integration
consul {
  address = "consul:8500"
  server_service_name = "nomad-server"
  client_service_name = "nomad-client"
  auto_advertise = true
  server_auto_join = true
  client_auto_join = true
}

# Docker driver configuration
plugin "docker" {
  config {
    enabled = true
    
    # Docker daemon configuration
    endpoint = "unix:///var/run/docker.sock"
    
    # Enable privileged containers for testing
    allow_privileged = true
    
    # Volume configuration
    volumes {
      enabled = true
      selinuxlabel = ""
    }
    
    # Network configuration
    allow_caps = [
      "audit_write", "chown", "dac_override", "fowner", "fsetid", "kill", "mknod",
      "net_bind_service", "setfcap", "setgid", "setpcap", "setuid", "sys_chroot"
    ]
  }
}

# Raw exec driver (disabled by default for security)
plugin "raw_exec" {
  config {
    enabled = false
  }
}

# Java driver configuration
plugin "java" {
  config {
    enabled = true
  }
}

# Development mode settings
disable_update_check = true
enable_debug = true

# Performance tuning for local development
limits {
  https_handshake_timeout = "5s"
  rpc_handshake_timeout = "5s"
  rpc_max_conns_per_client = 1000
}

# TLS configuration (disabled for local development)
tls {
  http = false
  rpc = false
}