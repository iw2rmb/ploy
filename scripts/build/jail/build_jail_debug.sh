#!/bin/bash
set -euo pipefail

APP=""
SRC=""
SHA=""
OUT_DIR=""
DEBUG=""
SSH_ENABLED="false"

while [[ $# -gt 0 ]]; do
  case $1 in
    --app) APP="$2"; shift 2 ;;
    --src) SRC="$2"; shift 2 ;;
    --sha) SHA="$2"; shift 2 ;;
    --out-dir) OUT_DIR="$2"; shift 2 ;;
    --debug) DEBUG="true"; shift ;;
    --ssh-enabled=*) SSH_ENABLED="${1#*=}"; shift ;;
    *) echo "Unknown argument: $1"; exit 1 ;;
  esac
done

if [[ -z "$APP" || -z "$SRC" || -z "$SHA" || -z "$OUT_DIR" ]]; then
  echo "Usage: $0 --app APP --src SRC --sha SHA --out-dir OUT_DIR --debug [--ssh-enabled=true/false]"
  exit 1
fi

echo "Building debug FreeBSD jail for $APP with SSH=$SSH_ENABLED"

# Create jail directory structure
JAIL_DIR="$OUT_DIR/debug-$APP-$SHA-jail"
mkdir -p "$JAIL_DIR"/{bin,sbin,lib,libexec,usr,var,tmp,dev,proc,root,home}

# Install base FreeBSD userland (simplified for demo)
echo "Setting up debug jail environment..."

# Copy essential binaries
BINS="sh ls cat echo mkdir rm cp mv chmod chown ps top kill mount umount"
for bin in $BINS; do
  if command -v $bin >/dev/null 2>&1; then
    cp "$(which $bin)" "$JAIL_DIR/bin/" 2>/dev/null || true
  fi
done

# Copy application source
cp -r "$SRC"/* "$JAIL_DIR/home/"

# Set up SSH if enabled
if [[ "$SSH_ENABLED" == "true" ]]; then
  echo "Configuring SSH for debug jail..."
  
  # Install SSH daemon
  mkdir -p "$JAIL_DIR/usr/sbin"
  if command -v sshd >/dev/null 2>&1; then
    cp "$(which sshd)" "$JAIL_DIR/usr/sbin/" 2>/dev/null || true
  fi
  
  # Create SSH configuration
  mkdir -p "$JAIL_DIR/etc/ssh"
  cat > "$JAIL_DIR/etc/ssh/sshd_config" << 'EOF'
Port 22
Protocol 2
HostKey /etc/ssh/ssh_host_rsa_key
HostKey /etc/ssh/ssh_host_dsa_key
HostKey /etc/ssh/ssh_host_ecdsa_key
HostKey /etc/ssh/ssh_host_ed25519_key
UsePrivilegeSeparation yes
KeyRegenerationInterval 3600
ServerKeyBits 1024
SyslogFacility AUTH
LogLevel INFO
LoginGraceTime 120
PermitRootLogin yes
PubkeyAuthentication yes
AuthorizedKeysFile %h/.ssh/authorized_keys
StrictModes yes
RSAAuthentication yes
IgnoreRhosts yes
RhostsRSAAuthentication no
HostbasedAuthentication no
PermitEmptyPasswords no
ChallengeResponseAuthentication no
PasswordAuthentication yes
X11Forwarding yes
X11DisplayOffset 10
PrintMotd no
PrintLastLog yes
TCPKeepAlive yes
AcceptEnv LANG LC_*
Subsystem sftp /usr/libexec/sftp-server
UsePAM yes
EOF

  # Create startup script with SSH
  cat > "$JAIL_DIR/startup.sh" << 'EOF'
#!/bin/sh

# Setup SSH keys if provided
if [ ! -z "$SSH_PUBLIC_KEY" ]; then
  mkdir -p /root/.ssh
  echo "$SSH_PUBLIC_KEY" > /root/.ssh/authorized_keys
  chmod 600 /root/.ssh/authorized_keys
  chmod 700 /root/.ssh
fi

# Generate host keys if they don't exist
if [ ! -f /etc/ssh/ssh_host_rsa_key ]; then
  ssh-keygen -t rsa -f /etc/ssh/ssh_host_rsa_key -N ''
  ssh-keygen -t dsa -f /etc/ssh/ssh_host_dsa_key -N ''
  ssh-keygen -t ecdsa -f /etc/ssh/ssh_host_ecdsa_key -N ''
  ssh-keygen -t ed25519 -f /etc/ssh/ssh_host_ed25519_key -N ''
fi

# Start SSH daemon
/usr/sbin/sshd -D &

# Start application
cd /home
exec "$@"
EOF
  chmod +x "$JAIL_DIR/startup.sh"
else
  # Create startup script without SSH
  cat > "$JAIL_DIR/startup.sh" << 'EOF'
#!/bin/sh
cd /home
exec "$@"
EOF
  chmod +x "$JAIL_DIR/startup.sh"
fi

# Create jail configuration
cat > "$JAIL_DIR/jail.conf" << EOF
debug-$APP {
  host.hostname = "debug-$APP.debug.ployd.app";
  path = "$JAIL_DIR";
  mount.devfs;
  ip4.addr = "lo1|127.0.1.1/32";
  interface = "lo1";
  exec.start = "/startup.sh";
  exec.stop = "/bin/sh /etc/rc.shutdown";
  allow.raw_sockets = true;
  devfs_ruleset = 4;
}
EOF

# Create tarball for jail
cd "$OUT_DIR"
tar -czf "debug-$APP-$SHA-jail.tar.gz" -C "$JAIL_DIR" .

JAIL_TAR="$OUT_DIR/debug-$APP-$SHA-jail.tar.gz"

# Generate SBOM and signature for debug jail
if command -v syft >/dev/null 2>&1; then syft packages "$JAIL_TAR" -o json > "$JAIL_TAR.sbom.json" || true; fi
if command -v cosign >/dev/null 2>&1; then cosign sign-blob --yes --output-signature "$JAIL_TAR.sig" "$JAIL_TAR" || true; fi

echo "Built debug jail: $JAIL_TAR"
echo "$JAIL_TAR"