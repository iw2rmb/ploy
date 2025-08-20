#!/usr/bin/env bash
set -euo pipefail
usage(){ echo "usage: $0 --lane A|B --app-dir <dir> --port 8080"; exit 1; }
LANE="A"; APPDIR=""; PORT="8080"
while [[ $# -gt 0 ]]; do
  case "$1" in
    --lane) LANE="$2"; shift 2;;
    --app-dir) APPDIR="$2"; shift 2;;
    --port) PORT="$2"; shift 2;;
    *) usage;;
  esac
done
[[ -z "$APPDIR" ]] && usage

# Node.js application detection
detect_nodejs() {
  local app_dir="$1"
  [[ -f "$app_dir/package.json" ]]
}

# Extract Node.js version from package.json engines field
get_nodejs_version_requirement() {
  local app_dir="$1"
  local package_json="$app_dir/package.json"
  
  if [[ ! -f "$package_json" ]]; then
    echo "18"  # Default version
    return 0
  fi
  
  # Extract engines.node field using node to parse JSON safely
  local node_version
  # Convert to absolute path for require()
  local abs_package_json
  abs_package_json=$(realpath "$package_json" 2>/dev/null || echo "$package_json")
  
  node_version=$(node -p "
    try {
      const pkg = require('$abs_package_json');
      const engines = pkg.engines || {};
      const nodeVersion = engines.node || '';
      // Handle version ranges like '^18.0.0', '>=16.0.0', '18.x'
      // Extract major version number
      const match = nodeVersion.match(/(\d+)/);
      match ? match[1] : '18';
    } catch (e) {
      '18';
    }
  " 2>/dev/null || echo "18")
  
  echo "$node_version"
}

# Select appropriate template based on lane and application type
select_template() {
  local lane="$1"
  local app_dir="$2"
  
  case "$lane" in
    "A")
      echo "lanes/A-unikraft-minimal/kraft.yaml"
      ;;
    "B")
      if detect_nodejs "$app_dir"; then
        echo "lanes/B-unikraft-nodejs/kraft.yaml"
      else
        echo "lanes/B-unikraft-posix/kraft.yaml" 
      fi
      ;;
    *)
      echo "lanes/A-unikraft-minimal/kraft.yaml"
      ;;
  esac
}

# Generate Node.js-specific configuration
configure_nodejs_template() {
  local template_file="$1"
  local app_dir="$2"
  local port="$3"
  
  if detect_nodejs "$app_dir" && [[ -f "$app_dir/package.json" ]]; then
    # Extract Node.js application metadata
    local app_name="nodejs-app"
    local main_file="index.js"
    local node_version
    
    node_version=$(get_nodejs_version_requirement "$app_dir")
    
    if command -v node >/dev/null 2>&1; then
      app_name=$(node -p "try { require('$app_dir/package.json').name || 'nodejs-app' } catch(e) { 'nodejs-app' }" 2>/dev/null || echo "nodejs-app")
      main_file=$(node -p "try { require('$app_dir/package.json').main || 'index.js' } catch(e) { 'index.js' }" 2>/dev/null || echo "index.js")
    fi
    
    # Customize template for Node.js application
    sed -i.bak \
      -e "s/^name:.*/name: $app_name/" \
      -e "s/http_port:.*/http_port: $port/" \
      -e "s|sources:.*|sources: ./|" \
      "$template_file" 2>/dev/null || true
    
    # Add Node.js version comment to kraft.yaml
    echo "# Node.js version requirement: $node_version" >> "$template_file"
    
    echo "Generated Node.js-optimized configuration for $app_name (main: $main_file, node: v$node_version)"
  else
    # Standard template customization
    sed -i.bak "s/http_port:.*/http_port: $port/" "$template_file" 2>/dev/null || true
  fi
}

# Main generation logic
TPL=$(select_template "$LANE" "$APPDIR")
OUT="$APPDIR/kraft.yaml"

# Ensure output directory exists
mkdir -p "$(dirname "$OUT")"

# Copy template
if [[ -f "$TPL" ]]; then
  cp "$TPL" "$OUT" || {
    echo "Warning: Could not copy template $TPL, using basic template"
    echo "spec_version: '0.6'
name: basic-app
unikraft:
  version: stable
targets:
  - architecture: x86_64
    platform: qemu
application:
  sources: ./
options:
  http_port: $PORT" > "$OUT"
  }
else
  echo "Warning: Template $TPL not found, creating basic configuration"
  echo "spec_version: '0.6'
name: basic-app  
unikraft:
  version: stable
targets:
  - architecture: x86_64
    platform: qemu
application:
  sources: ./
options:
  http_port: $PORT" > "$OUT"
fi

# Apply application-specific configuration
configure_nodejs_template "$OUT" "$APPDIR" "$PORT"

echo "$OUT"
