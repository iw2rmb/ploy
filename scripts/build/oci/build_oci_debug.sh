#!/bin/bash
set -euo pipefail

APP=""
SRC=""
TAG=""
DEBUG=""
SSH_ENABLED="false"

while [[ $# -gt 0 ]]; do
  case $1 in
    --app) APP="$2"; shift 2 ;;
    --src) SRC="$2"; shift 2 ;;
    --tag) TAG="$2"; shift 2 ;;
    --debug) DEBUG="true"; shift ;;
    --ssh-enabled=*) SSH_ENABLED="${1#*=}"; shift ;;
    *) echo "Unknown argument: $1"; exit 1 ;;
  esac
done

if [[ -z "$APP" || -z "$SRC" || -z "$TAG" ]]; then
  echo "Usage: $0 --app APP --src SRC --tag TAG --debug [--ssh-enabled=true/false]"
  exit 1
fi

echo "Building debug OCI image for $APP with SSH=$SSH_ENABLED"

# Create temporary directory for debug build
BUILD_DIR=$(mktemp -d)
trap "rm -rf $BUILD_DIR" EXIT

# Copy source
cp -r "$SRC"/* "$BUILD_DIR/"

# Create debug Dockerfile
cat > "$BUILD_DIR/Dockerfile.debug" << EOF
# Use base image with debugging tools
FROM ubuntu:22.04

# Install debugging tools and SSH
RUN apt-get update && apt-get install -y \\
    gdb \\
    strace \\
    tcpdump \\
    netcat \\
    curl \\
    wget \\
    htop \\
    $(if [[ "$SSH_ENABLED" == "true" ]]; then
      echo "openssh-server"
    fi) \\
    && rm -rf /var/lib/apt/lists/*

$(if [[ "$SSH_ENABLED" == "true" ]]; then
cat << 'SSHEOF'
# Configure SSH
RUN mkdir /var/run/sshd
RUN echo 'root:debug' | chpasswd
RUN sed -i 's/#PermitRootLogin prohibit-password/PermitRootLogin yes/' /etc/ssh/sshd_config
RUN sed -i 's/#PubkeyAuthentication yes/PubkeyAuthentication yes/' /etc/ssh/sshd_config

# SSH login fix for older versions of Ubuntu
RUN sed 's@session\\s*required\\s*pam_loginuid.so@session optional pam_loginuid.so@g' -i /etc/pam.d/sshd

# Create SSH directory
RUN mkdir -p /root/.ssh
SSHEOF
fi)

# Copy application
COPY . /app
WORKDIR /app

# Set up SSH key if provided
$(if [[ "$SSH_ENABLED" == "true" ]]; then
cat << 'SSHEOF'
RUN if [ ! -z "\${SSH_PUBLIC_KEY}" ]; then \\
      echo "\${SSH_PUBLIC_KEY}" >> /root/.ssh/authorized_keys && \\
      chmod 600 /root/.ssh/authorized_keys && \\
      chmod 700 /root/.ssh; \\
    fi
SSHEOF
fi)

# Install app dependencies and build
RUN if [ -f "package.json" ]; then npm install; fi
RUN if [ -f "requirements.txt" ]; then pip3 install -r requirements.txt; fi
RUN if [ -f "go.mod" ]; then go build -o app; fi

EXPOSE 8080
$(if [[ "$SSH_ENABLED" == "true" ]]; then
  echo "EXPOSE 22"
fi)

# Create startup script
$(if [[ "$SSH_ENABLED" == "true" ]]; then
cat << 'SSHEOF'
RUN echo '#!/bin/bash' > /start.sh && \\
    echo 'service ssh start' >> /start.sh && \\
    echo 'exec "\$@"' >> /start.sh && \\
    chmod +x /start.sh
ENTRYPOINT ["/start.sh"]
SSHEOF
fi)

# Default command (will be overridden by app-specific logic)
CMD ["./app"]
EOF

# Detect app type and adjust build
if [[ -f "$BUILD_DIR/package.json" ]]; then
  echo "CMD [\"npm\", \"start\"]" >> "$BUILD_DIR/Dockerfile.debug"
elif [[ -f "$BUILD_DIR/requirements.txt" ]]; then
  echo "CMD [\"python3\", \"app.py\"]" >> "$BUILD_DIR/Dockerfile.debug"
elif [[ -f "$BUILD_DIR/go.mod" ]]; then
  echo "CMD [\"./app\"]" >> "$BUILD_DIR/Dockerfile.debug"
fi

# Build debug image
cd "$BUILD_DIR"
docker build -f Dockerfile.debug -t "$TAG" . \
  $(if [[ "$SSH_ENABLED" == "true" && -n "${SSH_PUBLIC_KEY:-}" ]]; then
    echo "--build-arg SSH_PUBLIC_KEY=\"$SSH_PUBLIC_KEY\""
  fi)

# Generate SBOM and signature for debug image
if command -v syft >/dev/null 2>&1; then syft scan "$TAG" -o json > "/tmp/$APP-$TAG.sbom.json" || true; fi
if command -v cosign >/dev/null 2>&1; then cosign sign --yes "$TAG" || true; fi

echo "Built debug image: $TAG"
echo "$TAG"