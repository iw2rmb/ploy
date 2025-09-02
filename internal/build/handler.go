// Package build handles build operations and deployment requests
package build

// This file now only serves as the main entry point for the build package.
// The actual implementation is split across multiple files for better organization:
//
// - trigger.go: Contains TriggerBuild function and related upload helpers
// - status.go: Contains Status function and status mapping utilities
// - logs.go: Contains GetLogs function for retrieving application logs
// - apps.go: Contains ListApps function for listing deployed applications
// - signing.go: Contains signing method determination and vulnerability scanning
// - repository.go: Contains source repository extraction logic
// - resources.go: Contains lane-specific resource allocation functions