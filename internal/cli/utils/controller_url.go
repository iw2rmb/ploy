package utils

import (
    "fmt"
    "os"
)

// ResolveControllerURLFromEnv computes the controller URL from environment with sensible defaults.
// Priority:
// 1) PLOY_CONTROLLER if explicitly set
// 2) PLOY_APPS_DOMAIN (ploy CLI): https://api.dev.<domain>/v1 for dev env, else https://api.<domain>/v1
// 3) PLOY_PLATFORM_DOMAIN (ployman CLI): https://api.<domain>/v1
// 4) Default: https://api.dev.ployman.app/v1
func ResolveControllerURLFromEnv() string {
    if url := os.Getenv("PLOY_CONTROLLER"); url != "" {
        return url
    }
    if domain := os.Getenv("PLOY_APPS_DOMAIN"); domain != "" {
        if env := os.Getenv("PLOY_ENVIRONMENT"); env == "dev" {
            return fmt.Sprintf("https://api.dev.%s/v1", domain)
        }
        return fmt.Sprintf("https://api.%s/v1", domain)
    }
    if domain := os.Getenv("PLOY_PLATFORM_DOMAIN"); domain != "" {
        return fmt.Sprintf("https://api.%s/v1", domain)
    }
    return "https://api.dev.ployman.app/v1"
}

