#!/bin/bash

# Helper script to get the correct api URL based on environment
# This script detects the appropriate api endpoint for different environments

set -e

# Default values
PLOY_APPS_DOMAIN=${PLOY_APPS_DOMAIN:-"ployd.app"}
PLOY_ENVIRONMENT=${PLOY_ENVIRONMENT:-"dev"}

# Function to get api URL
get_api_url() {
    local environment="${1:-$PLOY_ENVIRONMENT}"
    local domain="${2:-$PLOY_APPS_DOMAIN}"
    
    case "$environment" in
        "dev")
            echo "https://api.dev.${domain}/v1"
            ;;
        "staging")
            echo "https://api.staging.${domain}/v1"
            ;;
        "prod"|"production")
            echo "https://api.${domain}/v1"
            ;;
        *)
            echo "https://api.dev.${domain}/v1"
            ;;
    esac
}

# If script is sourced, make function available
if [[ "${BASH_SOURCE[0]}" != "${0}" ]]; then
    # Script is being sourced
    export -f get_api_url
else
    # Script is being executed directly
    if [[ $# -eq 0 ]]; then
        # No arguments, use defaults
        get_api_url
    elif [[ $# -eq 1 ]]; then
        # One argument (environment)
        get_api_url "$1"
    elif [[ $# -eq 2 ]]; then
        # Two arguments (environment and domain)
        get_api_url "$1" "$2"
    else
        echo "Usage: $0 [environment] [domain]"
        echo "  environment: dev, staging, prod (default: dev)"
        echo "  domain: base domain (default: ployd.app)"
        echo ""
        echo "Examples:"
        echo "  $0                    # https://api.dev.ployman.app/v1"
        echo "  $0 prod               # https://api.ployd.app/v1"
        echo "  $0 dev example.com    # https://api.dev.example.com/v1"
        exit 1
    fi
fi