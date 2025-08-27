package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCreatePylintConfig(t *testing.T) {
	config := createPylintConfig()
	
	assert.NotEmpty(t, config)
	assert.Contains(t, config, "pylint-chttp")
	assert.Contains(t, config, "pylint")
	assert.Contains(t, config, ".py")
	assert.Contains(t, config, ".pyw")
	assert.Contains(t, config, "pylint_json")
}

func TestPylintServiceInfo(t *testing.T) {
	serviceName, port := getPylintServiceInfo()
	
	assert.Equal(t, "pylint-chttp", serviceName)
	assert.Equal(t, 8080, port)
}

func TestValidateEnvironment(t *testing.T) {
	// Test that environment validation doesn't panic
	err := validatePylintEnvironment()
	
	// Environment validation can succeed or fail depending on system
	// The important thing is it doesn't crash
	t.Logf("Environment validation result: %v", err)
}