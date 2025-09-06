package transflow

import (
	"context"
	"os"

	"github.com/iw2rmb/ploy/internal/cli/common"
	"github.com/iw2rmb/ploy/internal/git/provider"
)

// MockGitOperations implements GitOperationsInterface for testing
type MockGitOperations struct {
	CloneError         error
	CreateBranchError  error
	CommitError        error
	PushError          error
	CloneCalled        bool
	CreateBranchCalled bool
	CommitCalled       bool
	PushCalled         bool
	CloneRepo          string
	CloneBranch        string
	ClonePath          string
	BranchName         string
	CommitMessage      string
	PushRemoteURL      string
	PushBranchName     string
}

func NewMockGitOperations() *MockGitOperations {
	return &MockGitOperations{}
}

func (m *MockGitOperations) CloneRepository(ctx context.Context, repoURL, branch, targetPath string) error {
	m.CloneCalled = true
	m.CloneRepo = repoURL
	m.CloneBranch = branch
	m.ClonePath = targetPath

	// Create the directory for testing
	if err := os.MkdirAll(targetPath, 0755); err != nil {
		return err
	}

	return m.CloneError
}

func (m *MockGitOperations) CreateBranchAndCheckout(ctx context.Context, repoPath, branchName string) error {
	m.CreateBranchCalled = true
	m.BranchName = branchName
	return m.CreateBranchError
}

func (m *MockGitOperations) CommitChanges(ctx context.Context, repoPath, message string) error {
	m.CommitCalled = true
	m.CommitMessage = message
	return m.CommitError
}

func (m *MockGitOperations) PushBranch(ctx context.Context, repoPath, remoteURL, branchName string) error {
	m.PushCalled = true
	m.PushRemoteURL = remoteURL
	m.PushBranchName = branchName
	return m.PushError
}

// MockRecipeExecutor implements RecipeExecutorInterface for testing
type MockRecipeExecutor struct {
	ExecuteError  error
	ExecuteCalled bool
	RecipeIDs     []string
	WorkspacePath string
}

func NewMockRecipeExecutor() *MockRecipeExecutor {
	return &MockRecipeExecutor{}
}

func (m *MockRecipeExecutor) ExecuteRecipes(ctx context.Context, workspacePath string, recipeIDs []string) error {
	m.ExecuteCalled = true
	m.RecipeIDs = recipeIDs
	m.WorkspacePath = workspacePath
	return m.ExecuteError
}

// MockBuildChecker implements BuildCheckerInterface for testing
type MockBuildChecker struct {
	BuildError  error
	BuildCalled bool
	BuildConfig common.DeployConfig
	BuildResult *common.DeployResult
}

func NewMockBuildChecker() *MockBuildChecker {
	return &MockBuildChecker{
		BuildResult: &common.DeployResult{
			Success:      true,
			Message:      "Mock build succeeded",
			Version:      "mock-v1.0.0",
			DeploymentID: "mock-deployment-123",
			URL:          "mock://test-image:latest",
		},
	}
}

func (m *MockBuildChecker) CheckBuild(ctx context.Context, config common.DeployConfig) (*common.DeployResult, error) {
	m.BuildCalled = true
	m.BuildConfig = config
	if m.BuildError != nil {
		return nil, m.BuildError
	}
	return m.BuildResult, nil
}

// MockGitProvider implements provider.GitProvider for testing
type MockGitProvider struct {
	MRError          error
	ValidationError  error
	MRCalled         bool
	ValidationCalled bool
	MRConfig         provider.MRConfig
	MRResult         *provider.MRResult
}

func NewMockGitProvider() *MockGitProvider {
	return &MockGitProvider{
		MRResult: &provider.MRResult{
			MRURL:   "https://gitlab.example.com/test/project/-/merge_requests/123",
			MRID:    123,
			Created: true,
		},
	}
}

func (m *MockGitProvider) CreateOrUpdateMR(ctx context.Context, config provider.MRConfig) (*provider.MRResult, error) {
	m.MRCalled = true
	m.MRConfig = config
	if m.MRError != nil {
		return nil, m.MRError
	}
	return m.MRResult, nil
}

func (m *MockGitProvider) ValidateConfiguration() error {
	m.ValidationCalled = true
	return m.ValidationError
}

// MockKBIntegration implements KBIntegrator for testing
type MockKBIntegration struct {
	LoadKBContextCalled       bool
	WriteHealingCaseCalled    bool
	ShouldUseKBSuggestionsVal bool
	LoadKBContextError        error
	WriteHealingCaseError     error
	LoadKBContextResult       *KBContext
	ConvertKBFixesResult      []BranchSpec
}

// Ensure MockKBIntegration implements KBIntegrator
var _ KBIntegrator = (*MockKBIntegration)(nil)

func NewMockKBIntegration() *MockKBIntegration {
	return &MockKBIntegration{
		ShouldUseKBSuggestionsVal: false,
		LoadKBContextResult: &KBContext{
			HasData: false,
		},
		ConvertKBFixesResult: []BranchSpec{},
	}
}

func (m *MockKBIntegration) LoadKBContext(ctx context.Context, lang string, stdout, stderr []byte) (*KBContext, error) {
	m.LoadKBContextCalled = true
	if m.LoadKBContextError != nil {
		return nil, m.LoadKBContextError
	}
	return m.LoadKBContextResult, nil
}

func (m *MockKBIntegration) WriteHealingCase(ctx context.Context, kbCtx *KBContext, attempt *HealingAttempt, outcome *HealingOutcome, stdout, stderr string) error {
	m.WriteHealingCaseCalled = true
	return m.WriteHealingCaseError
}

func (m *MockKBIntegration) ShouldUseKBSuggestions(kbCtx *KBContext) bool {
	return m.ShouldUseKBSuggestionsVal
}

func (m *MockKBIntegration) ConvertKBFixesToBranchSpecs(fixes []PromotedFix) []BranchSpec {
	return m.ConvertKBFixesResult
}
