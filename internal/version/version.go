package version

// Version metadata injected at build time; defaults suit local development.
var (
	// Version is the semantic version or channel for the build.
	Version = "dev"
	// Commit is the git revision associated with the build.
	Commit = "unknown"
	// BuiltAt records the UTC timestamp when the binary was produced.
	BuiltAt = ""
)
