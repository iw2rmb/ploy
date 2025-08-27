#!/bin/bash

# Script to update all test scripts to use HTTPS endpoints instead of localhost:8081

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
TEST_SCRIPTS_DIR="${PROJECT_ROOT}/tests/scripts"

echo "Updating test scripts to use HTTPS endpoints..."
echo "Project root: $PROJECT_ROOT"
echo "Test scripts directory: $TEST_SCRIPTS_DIR"
echo

# Find all test scripts that contain localhost:8081
scripts_to_update=($(grep -l "localhost:8081\|127\.0\.0\.1:8081" "${TEST_SCRIPTS_DIR}"/*.sh 2>/dev/null || true))

if [ ${#scripts_to_update[@]} -eq 0 ]; then
    echo "No test scripts found that need updating."
    exit 0
fi

echo "Found ${#scripts_to_update[@]} test scripts to update:"
for script in "${scripts_to_update[@]}"; do
    echo "  - $(basename "$script")"
done
echo

# Update each script
for script_path in "${scripts_to_update[@]}"; do
    script_name=$(basename "$script_path")
    echo "Updating $script_name..."
    
    # Create backup
    cp "$script_path" "$script_path.backup"
    
    # Apply the transformation
    sed -i.tmp '
        # Pattern 1: PLOY_CONTROLLER=${PLOY_CONTROLLER:-http://localhost:8081/v1}
        s|PLOY_CONTROLLER=\${PLOY_CONTROLLER:-http://localhost:8081/v1}|# Dynamic controller endpoint based on environment\nPLOY_APPS_DOMAIN=\${PLOY_APPS_DOMAIN:-"ployd.app"}\nPLOY_ENVIRONMENT=\${PLOY_ENVIRONMENT:-"dev"}\n\nif [ "$PLOY_ENVIRONMENT" = "dev" ]; then\n    PLOY_CONTROLLER="\${PLOY_CONTROLLER:-https://api.dev.\${PLOY_APPS_DOMAIN}/v1}"\nelse\n    PLOY_CONTROLLER="\${PLOY_CONTROLLER:-https://api.\${PLOY_APPS_DOMAIN}/v1}"\nfi|g
        
        # Pattern 2: BASE_URL="${PLOY_CONTROLLER:-http://localhost:8081/v1}"
        s|BASE_URL="\${PLOY_CONTROLLER:-http://localhost:8081/v1}"|# Dynamic controller endpoint based on environment\nPLOY_APPS_DOMAIN=\${PLOY_APPS_DOMAIN:-"ployd.app"}\nPLOY_ENVIRONMENT=\${PLOY_ENVIRONMENT:-"dev"}\n\nif [ "$PLOY_ENVIRONMENT" = "dev" ]; then\n    BASE_URL="\${PLOY_CONTROLLER:-https://api.dev.\${PLOY_APPS_DOMAIN}/v1}"\nelse\n    BASE_URL="\${PLOY_CONTROLLER:-https://api.\${PLOY_APPS_DOMAIN}/v1}"\nfi|g
        
        # Pattern 3: CONTROLLER_URL=${CONTROLLER_URL:-http://localhost:8081/v1}
        s|CONTROLLER_URL=\${CONTROLLER_URL:-http://localhost:8081/v1}|# Dynamic controller endpoint based on environment\nPLOY_APPS_DOMAIN=\${PLOY_APPS_DOMAIN:-"ployd.app"}\nPLOY_ENVIRONMENT=\${PLOY_ENVIRONMENT:-"dev"}\n\nif [ "$PLOY_ENVIRONMENT" = "dev" ]; then\n    CONTROLLER_URL="\${CONTROLLER_URL:-https://api.dev.\${PLOY_APPS_DOMAIN}/v1}"\nelse\n    CONTROLLER_URL="\${CONTROLLER_URL:-https://api.\${PLOY_APPS_DOMAIN}/v1}"\nfi|g
        
        # Pattern 4: Simple localhost:8081 references (more complex, manual review recommended)
        s|http://localhost:8081|https://api.dev.ployman.app|g
        s|http://127\.0\.0\.1:8081|https://api.dev.ployman.app|g
    ' "$script_path"
    
    # Remove the .tmp file created by sed -i
    rm -f "$script_path.tmp"
    
    echo "  ✅ Updated $script_name"
done

echo
echo "✅ All test scripts updated successfully!"
echo "Backup files created with .backup extension"
echo
echo "To verify the changes:"
echo "  grep -n 'api\.dev\.ployd\.app\|PLOY_ENVIRONMENT' tests/scripts/*.sh"
echo
echo "To remove backup files after verification:"
echo "  rm tests/scripts/*.backup"