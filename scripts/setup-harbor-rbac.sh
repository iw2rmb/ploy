#!/bin/bash
# Harbor RBAC Setup Script
# 
# This script retrieves Harbor robot account credentials from the Harbor API
# and sets up environment variables for Ploy's Harbor RBAC integration

set -euo pipefail

HARBOR_ENDPOINT="${HARBOR_ENDPOINT:-harbor.dev.ployman.app}"
HARBOR_ADMIN_USER="${HARBOR_ADMIN_USER:-admin}"
HARBOR_ADMIN_PASSWORD="${HARBOR_ADMIN_PASSWORD:-Harbor12345}"

echo "🔧 Setting up Harbor RBAC for Ploy integration..."
echo "Harbor endpoint: ${HARBOR_ENDPOINT}"

# Function to get robot account token from Harbor API
get_robot_token() {
    local robot_name="$1"
    local project="$2"
    
    echo "Retrieving $robot_name robot account credentials..."
    
    # Get robot account details
    response=$(curl -s -k \
        -u "${HARBOR_ADMIN_USER}:${HARBOR_ADMIN_PASSWORD}" \
        "https://${HARBOR_ENDPOINT}/api/v2.0/robots" \
        | jq -r '.[] | select(.name=="'$robot_name'") | {id: .id, name: .name, token: .token}')
    
    if [[ -z "$response" || "$response" == "null" ]]; then
        echo "❌ Robot account $robot_name not found. Creating new robot account..."
        
        # Create robot account
        create_payload=$(cat <<EOF
{
  "name": "$robot_name",
  "level": "project",
  "permissions": [
    {
      "kind": "project",
      "namespace": "$project",
      "access": [
        {"resource": "repository", "action": "push"},
        {"resource": "repository", "action": "pull"},
        {"resource": "artifact", "action": "read"},
        {"resource": "scan", "action": "create"}
      ]
    }
  ],
  "duration": -1
}
EOF
        )
        
        create_response=$(curl -s -k \
            -u "${HARBOR_ADMIN_USER}:${HARBOR_ADMIN_PASSWORD}" \
            -H "Content-Type: application/json" \
            -X POST \
            "https://${HARBOR_ENDPOINT}/api/v2.0/robots" \
            -d "$create_payload")
        
        robot_id=$(echo "$create_response" | jq -r '.id')
        robot_name_full=$(echo "$create_response" | jq -r '.name')
        robot_secret=$(echo "$create_response" | jq -r '.secret')
        
        echo "✅ Created robot account: $robot_name_full"
        echo "Username: $robot_name_full"
        echo "Secret: $robot_secret"
        
        echo "$robot_name_full:$robot_secret"
    else
        echo "❌ Robot account $robot_name exists but secret is not accessible via API"
        echo "Please manually retrieve credentials from Harbor UI or recreate the robot account"
        return 1
    fi
}

echo
echo "🤖 Setting up Platform robot account..."
platform_creds=$(get_robot_token "platform-pusher" "platform")
platform_username=$(echo "$platform_creds" | cut -d':' -f1)
platform_password=$(echo "$platform_creds" | cut -d':' -f2)

echo
echo "🤖 Setting up Apps robot account..."
apps_creds=$(get_robot_token "apps-pusher" "apps")
apps_username=$(echo "$apps_creds" | cut -d':' -f1)
apps_password=$(echo "$apps_creds" | cut -d':' -f2)

echo
echo "📝 Harbor RBAC Environment Configuration:"
echo "=========================================="
echo
echo "# Add these environment variables to your shell profile (.bashrc, .zshrc, etc.)"
echo "# or export them before running Ploy commands"
echo
echo "export HARBOR_ENDPOINT=\"${HARBOR_ENDPOINT}\""
echo "export HARBOR_USERNAME=\"${HARBOR_ADMIN_USER}\""
echo "export HARBOR_PASSWORD=\"${HARBOR_ADMIN_PASSWORD}\""
echo
echo "# Platform service account (for platform services)"
echo "export HARBOR_PLATFORM_USERNAME=\"${platform_username}\""
echo "export HARBOR_PLATFORM_PASSWORD=\"${platform_password}\""
echo
echo "# Apps service account (for user applications)"  
echo "export HARBOR_APPS_USERNAME=\"${apps_username}\""
echo "export HARBOR_APPS_PASSWORD=\"${apps_password}\""
echo
echo "# Harbor project configuration"
echo "export HARBOR_PLATFORM_PROJECT=\"platform\""
echo "export HARBOR_USER_PROJECT=\"apps\""
echo "export HARBOR_INSECURE=\"false\""
echo
echo "✅ Harbor RBAC setup completed!"
echo
echo "🔍 Next Steps:"
echo "1. Export the environment variables above"
echo "2. Test Docker login: docker login ${HARBOR_ENDPOINT} -u ${platform_username}"
echo "3. Deploy a test application to verify RBAC permissions"
echo "4. Check Harbor UI for project access and permissions"
echo
echo "📖 Documentation: https://goharbor.io/docs/2.11.0/administration/robot-accounts/"