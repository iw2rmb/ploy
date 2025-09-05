package distribution

import (
	"os"
	"os/user"
)

// getSystemHostname gets the system hostname
func getSystemHostname() (string, error) {
	return os.Hostname()
}

// getSystemUsername gets the current username
func getSystemUsername() (string, error) {
	currentUser, err := user.Current()
	if err != nil {
		return "", err
	}
	return currentUser.Username, nil
}
