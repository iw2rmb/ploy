package main

import (
	"os"
	"strings"
)

func asterEnabled() bool {
	value := strings.TrimSpace(os.Getenv("PLOY_ASTER_ENABLE"))
	switch strings.ToLower(value) {
	case "", "0", "false", "no":
		return false
	default:
		return true
	}
}
