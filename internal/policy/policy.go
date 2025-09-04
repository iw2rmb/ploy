package policy

// ArtifactInput captures attributes used for policy enforcement
type ArtifactInput struct {
    Signed      bool
    SBOMPresent bool
    Env         string
    SSHEnabled  bool
    BreakGlass  bool
    App         string
    Lane        string
    Debug       bool
    ImageSizeMB    float64
    ImagePath      string
    DockerImage    string
    VulnScanPassed bool
    SigningMethod  string
    BuildTime      int64
    SourceRepo     string
}

// Enforcer defines policy enforcement behavior
type Enforcer interface {
    Enforce(ArtifactInput) error
}

// DefaultEnforcer is used by callers; can be overridden in tests
var DefaultEnforcer Enforcer = allowAll{}

type allowAll struct{}

func (allowAll) Enforce(ArtifactInput) error { return nil }
