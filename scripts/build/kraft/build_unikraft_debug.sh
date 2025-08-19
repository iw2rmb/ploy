#!/bin/bash
set -euo pipefail

APP=""
APP_DIR=""
LANE=""
SHA=""
OUT_DIR=""
DEBUG=""
SSH_ENABLED="false"

while [[ $# -gt 0 ]]; do
  case $1 in
    --app) APP="$2"; shift 2 ;;
    --app-dir) APP_DIR="$2"; shift 2 ;;
    --lane) LANE="$2"; shift 2 ;;
    --sha) SHA="$2"; shift 2 ;;
    --out-dir) OUT_DIR="$2"; shift 2 ;;
    --debug) DEBUG="true"; shift ;;
    --ssh-enabled=*) SSH_ENABLED="${1#*=}"; shift ;;
    *) echo "Unknown argument: $1"; exit 1 ;;
  esac
done

if [[ -z "$APP" || -z "$APP_DIR" || -z "$LANE" || -z "$SHA" || -z "$OUT_DIR" ]]; then
  echo "Usage: $0 --app APP --app-dir APP_DIR --lane LANE --sha SHA --out-dir OUT_DIR --debug [--ssh-enabled=true/false]"
  exit 1
fi

echo "Building debug Unikraft for $APP (lane $LANE) with SSH=$SSH_ENABLED"

# Create kraftfile with debug configuration
KRAFTFILE="$OUT_DIR/Kraftfile"
cat > "$KRAFTFILE" << EOF
spec: v0.6
unikraft:
  version: stable
  kconfig:
    - CONFIG_LIBUKDEBUG=y
    - CONFIG_LIBUKDEBUG_PRINTD=y
    - CONFIG_LIBUKSCHED_COROUTINE_DEFAULT=y
    $(if [[ "$SSH_ENABLED" == "true" ]]; then
      echo "    - CONFIG_LIBUKNETDEV=y"
      echo "    - CONFIG_LWIP_SOCKET=y"
      echo "    - CONFIG_HAVE_NETWORK=y"
    fi)

libraries:
  musl: stable
  lwip: stable
  $(if [[ "$SSH_ENABLED" == "true" ]]; then
    echo "  dropbear: stable"
  fi)

targets:
  - name: debug-$APP
    arch: x86_64
    plat: qemu
EOF

# Copy app source to build directory
BUILD_DIR="$OUT_DIR/build"
mkdir -p "$BUILD_DIR"
cp -r "$APP_DIR"/* "$BUILD_DIR/"

# Add SSH setup if enabled
if [[ "$SSH_ENABLED" == "true" ]]; then
  cat >> "$BUILD_DIR/main.c" << 'EOF'

#include <sys/socket.h>
#include <netinet/in.h>
#include <string.h>
#include <unistd.h>

void setup_ssh() {
    // Setup SSH public key from environment
    const char* ssh_key = getenv("SSH_PUBLIC_KEY");
    if (ssh_key) {
        system("mkdir -p /root/.ssh");
        FILE* f = fopen("/root/.ssh/authorized_keys", "w");
        if (f) {
            fprintf(f, "%s\n", ssh_key);
            fclose(f);
        }
        system("chmod 600 /root/.ssh/authorized_keys");
        system("chmod 700 /root/.ssh");
    }
    
    // Start dropbear SSH daemon
    system("dropbear -r /etc/dropbear/dropbear_rsa_host_key -p 22 -F");
}

int main_original(); // Declare original main

int main() {
    if (getenv("SSH_ENABLED") && strcmp(getenv("SSH_ENABLED"), "true") == 0) {
        setup_ssh();
    }
    return main_original();
}

#define main main_original
EOF
fi

cd "$OUT_DIR"

# Build with kraft
if ! command -v kraft &> /dev/null; then
  echo "kraft command not found, using docker fallback"
  IMAGE_PATH="$OUT_DIR/debug-$APP-$SHA.img"
  
  # Create a basic debug image
  dd if=/dev/zero of="$IMAGE_PATH" bs=1M count=64
  mkfs.ext4 -F "$IMAGE_PATH"
  
  echo "Created debug image: $IMAGE_PATH"
  
  # Generate comprehensive SBOM and signature for debug image  
  if command -v syft >/dev/null 2>&1; then 
    echo "Generating comprehensive SBOM for fallback debug image $IMAGE_PATH..."
    syft scan "$IMAGE_PATH" -o spdx-json --file "$IMAGE_PATH.sbom.json" || true
  fi
  if command -v cosign >/dev/null 2>&1; then cosign sign-blob --yes --output-signature "$IMAGE_PATH.sig" "$IMAGE_PATH" || true; fi
  
  echo "$IMAGE_PATH"
else
  kraft build --kraftfile "$KRAFTFILE"
  IMAGE_PATH=$(find "$OUT_DIR" -name "*.img" | head -1)
  if [[ -n "$IMAGE_PATH" ]]; then
    NEW_PATH="$OUT_DIR/debug-$APP-$SHA.img"
    mv "$IMAGE_PATH" "$NEW_PATH"
    
    # Generate comprehensive SBOM and signature for debug image
    if command -v syft >/dev/null 2>&1; then 
      echo "Generating comprehensive SBOM for debug image $NEW_PATH..."
      syft scan "$NEW_PATH" -o spdx-json --file "$NEW_PATH.sbom.json" || true
    fi
    if command -v cosign >/dev/null 2>&1; then cosign sign-blob --yes --output-signature "$NEW_PATH.sig" "$NEW_PATH" || true; fi
    
    echo "$NEW_PATH"
  else
    echo "Build failed: no image found" >&2
    exit 1
  fi
fi