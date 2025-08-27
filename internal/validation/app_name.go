package validation

import (
	"fmt"
	"regexp"
	"strings"
)

// Reserved app names that cannot be used by users
var reservedAppNames = map[string]bool{
	"dev": true, // Reserved for dev environment subdomain
}

// AppNamePattern defines valid app name format
var AppNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,61}[a-z0-9]$`)

// ValidateAppName validates an app name according to platform rules
func ValidateAppName(appName string) error {
	// Convert to lowercase for validation
	appName = strings.ToLower(appName)
	
	// Check if empty
	if appName == "" {
		return fmt.Errorf("app name cannot be empty")
	}
	
	// Check minimum length
	if len(appName) < 2 {
		return fmt.Errorf("app name must be at least 2 characters long")
	}
	
	// Check maximum length
	if len(appName) > 63 {
		return fmt.Errorf("app name cannot exceed 63 characters")
	}
	
	// Check if reserved
	if reservedAppNames[appName] {
		return fmt.Errorf("app name '%s' is reserved for platform use", appName)
	}
	
	// Check pattern (lowercase letters, numbers, hyphens)
	if !AppNamePattern.MatchString(appName) {
		return fmt.Errorf("app name must start with a letter, end with a letter or number, and contain only lowercase letters, numbers, and hyphens")
	}
	
	// Check for double hyphens
	if strings.Contains(appName, "--") {
		return fmt.Errorf("app name cannot contain consecutive hyphens")
	}
	
	return nil
}

// IsReservedAppName checks if an app name is reserved
func IsReservedAppName(appName string) bool {
	return reservedAppNames[strings.ToLower(appName)]
}

// GetReservedAppNames returns list of all reserved app names
func GetReservedAppNames() []string {
	names := make([]string, 0, len(reservedAppNames))
	for name := range reservedAppNames {
		names = append(names, name)
	}
	return names
}