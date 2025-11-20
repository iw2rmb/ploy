package nodeagent

import "os"

const defaultBearerTokenPath = "/etc/ploy/bearer-token"

// bearerTokenPath returns the path to the worker bearer token file,
// overridable for tests via PLOY_NODE_BEARER_TOKEN_PATH.
func bearerTokenPath() string {
	if v := os.Getenv("PLOY_NODE_BEARER_TOKEN_PATH"); v != "" {
		return v
	}
	return defaultBearerTokenPath
}
