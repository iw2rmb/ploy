package ploy.admission

default allow = false

# Require signatures & SBOM
allow {
  input.artifact.signed == true
  input.artifact.sbom == true
}

# SSH gating
deny[msg] {
  input.env == "prod"
  input.security.ssh.enabled == true
  not input.security.break_glass_approval
  msg := "SSH enabled in prod without approval"
}
