package main

import (
	"fmt"
	"os"
	"time"

	"github.com/iw2rmb/ploy/internal/server/auth"
)

func main() {
	if len(os.Args) != 5 {
		fmt.Fprintf(os.Stderr, "Usage: %s <secret> <cluster_id> <role> <expires_days>\n", os.Args[0])
		os.Exit(1)
	}

	secret := os.Args[1]
	clusterID := os.Args[2]
	role := os.Args[3]
	expiresDays := os.Args[4]

	// Parse expires days
	var days int
	if _, err := fmt.Sscanf(expiresDays, "%d", &days); err != nil {
		fmt.Fprintf(os.Stderr, "Invalid expires_days: %v\n", err)
		os.Exit(1)
	}

	expiresAt := time.Now().AddDate(0, 0, days)
	token, err := auth.GenerateAPIToken(secret, clusterID, role, expiresAt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to generate token: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(token)
}
