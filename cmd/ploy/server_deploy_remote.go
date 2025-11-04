package main

import (
	"github.com/iw2rmb/ploy/internal/deploy"
)

// rolloutServerHost is defined elsewhere; this file holds server deploy indirections.

// provisionHost indirection allows tests to stub remote provisioning to avoid
// real scp/ssh timeouts. Default is deploy.ProvisionHost.
var provisionHost = deploy.ProvisionHost

// detectRunner allows tests to inject a mock runner for cluster detection.
// Default is nil, which causes DetectExisting to use systemRunner.
var detectRunner deploy.Runner
