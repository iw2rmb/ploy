package builders

// DebugBuildResult minimal result for debug build
type DebugBuildResult struct {
	SSHCommand   string
	SSHPublicKey string
	ImagePath    string
	DockerImage  string
}

// DebugBuilder is the interface for creating debug builds
type DebugBuilder interface {
	BuildDebugInstance(app, lane, srcDir, sha, outDir string, envVars map[string]string, sshEnabled bool) (*DebugBuildResult, error)
}

// DefaultDebugBuilder can be replaced in tests
var DefaultDebugBuilder DebugBuilder = nopDebugBuilder{}

type nopDebugBuilder struct{}

func (nopDebugBuilder) BuildDebugInstance(app, lane, srcDir, sha, outDir string, envVars map[string]string, sshEnabled bool) (*DebugBuildResult, error) {
	return &DebugBuildResult{
		SSHCommand:   "ssh user@debug-host",
		SSHPublicKey: "ssh-rsa AAA...",
		ImagePath:    "/tmp/out/final.img",
		DockerImage:  "",
	}, nil
}
