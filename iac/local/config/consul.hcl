# Consul configuration for local development
datacenter = "local"
data_dir = "/consul/data"
log_level = "INFO"
server = true

# UI Configuration
ui_config {
  enabled = true
}

# Client configuration
client_addr = "0.0.0.0"
bind_addr = "0.0.0.0"

# Development mode settings
bootstrap_expect = 1
enable_debug = true

# ACLs disabled for local testing
acl = {
  enabled = false
  default_policy = "allow"
}

# Connect (service mesh) configuration
connect {
  enabled = true
}

# Performance tuning for local development
performance {
  raft_multiplier = 1
}

# Logging configuration
enable_syslog = false
log_rotate_duration = "24h"
log_rotate_max_files = 3