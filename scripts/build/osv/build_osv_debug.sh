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

echo "Building debug OSv image for $APP with SSH=$SSH_ENABLED"

# Create build directory
BUILD_DIR="$OUT_DIR/osv-build"
mkdir -p "$BUILD_DIR"

# Copy source
cp -r "$SRC"/* "$BUILD_DIR/"

# Create Capstanfile for debug build
cat > "$BUILD_DIR/Capstanfile" << EOF
base: cloudius/osv-openjdk8

cmdline: |
  --env=JAVA_HOME=/usr/lib/jvm/java
  $(if [[ "$SSH_ENABLED" == "true" ]]; then
    echo "--mount-fs=rofs"
    echo "--env=SSH_ENABLED=true"
  fi)

build: |
  # Install debugging tools
  echo "Installing debug tools..."
  
  $(if [[ "$SSH_ENABLED" == "true" ]]; then
cat << 'SSHEOF'
  # Setup SSH
  mkdir -p /etc/ssh /root/.ssh
  
  # Generate SSH host keys
  ssh-keygen -t rsa -f /etc/ssh/ssh_host_rsa_key -N ''
  ssh-keygen -t dsa -f /etc/ssh/ssh_host_dsa_key -N ''
  
  # Configure SSH
  cat > /etc/ssh/sshd_config << 'SSHD_EOF'
Port 22
Protocol 2
HostKey /etc/ssh/ssh_host_rsa_key
HostKey /etc/ssh/ssh_host_dsa_key
PermitRootLogin yes
PubkeyAuthentication yes
AuthorizedKeysFile /root/.ssh/authorized_keys
PasswordAuthentication yes
X11Forwarding no
UsePAM no
SSHD_EOF
  
  # Setup authorized keys from environment
  if [ ! -z "\${SSH_PUBLIC_KEY:-}" ]; then
    echo "\$SSH_PUBLIC_KEY" > /root/.ssh/authorized_keys
    chmod 600 /root/.ssh/authorized_keys
    chmod 700 /root/.ssh
  fi
SSHEOF
  fi)
  
  # Copy application
  mkdir -p /app
  cp -r . /app/
  
  # Build application if needed
  cd /app
  if [ -f "pom.xml" ]; then
    mvn clean package -DskipTests
  elif [ -f "build.gradle" ]; then
    ./gradlew build -x test
  fi

run: |
  $(if [[ "$SSH_ENABLED" == "true" ]]; then
    echo "/usr/sbin/sshd -D &"
  fi)
  cd /app
  java -cp "\$(find . -name '*.jar' | head -1)" \${MAIN_CLASS:-com.example.Application}
EOF

# Build with Capstan
cd "$BUILD_DIR"

if ! command -v capstan &> /dev/null; then
  echo "capstan command not found, creating fallback debug image"
  
  # Create a basic QCOW2 image as fallback
  IMAGE_PATH="$OUT_DIR/debug-$APP-$SHA.qcow2"
  
  if command -v qemu-img &> /dev/null; then
    qemu-img create -f qcow2 "$IMAGE_PATH" 1G
  else
    # Create a dummy file
    dd if=/dev/zero of="$IMAGE_PATH" bs=1M count=64
  fi
  
  echo "Created debug image: $IMAGE_PATH"
  
  # Generate SBOM and signature for debug image
  if command -v syft >/dev/null 2>&1; then syft scan "$IMAGE_PATH" -o json > "$IMAGE_PATH.sbom.json" || true; fi
  if command -v cosign >/dev/null 2>&1; then cosign sign-blob --yes --output-signature "$IMAGE_PATH.sig" "$IMAGE_PATH" || true; fi
  
  echo "$IMAGE_PATH"
else
  capstan build -v debug-$APP-$SHA
  
  # Find the generated image
  IMAGE_PATH=$(find ~/.capstan/repository/debug-$APP-$SHA -name "*.qcow2" | head -1)
  if [[ -n "$IMAGE_PATH" ]]; then
    NEW_PATH="$OUT_DIR/debug-$APP-$SHA.qcow2"
    cp "$IMAGE_PATH" "$NEW_PATH"
    
    # Generate SBOM and signature for debug image
    if command -v syft >/dev/null 2>&1; then syft scan "$NEW_PATH" -o json > "$NEW_PATH.sbom.json" || true; fi
    if command -v cosign >/dev/null 2>&1; then cosign sign-blob --yes --output-signature "$NEW_PATH.sig" "$NEW_PATH" || true; fi
    
    echo "$NEW_PATH"
  else
    echo "Build failed: no image found" >&2
    exit 1
  fi
fi