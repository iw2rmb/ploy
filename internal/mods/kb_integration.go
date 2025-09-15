package mods

import (
	"context"
	"fmt"
	"time"

	"github.com/iw2rmb/ploy/internal/orchestration"
	"github.com/iw2rmb/ploy/internal/storage"
)

// KBConfig contains configuration for KB integration
type KBConfig struct {
	Enabled        bool    `yaml:"enabled"`
	ReadThreshold  float64 `yaml:"read_threshold"` // confidence threshold for using KB suggestions
	WriteEnabled   bool    `yaml:"write_enabled"`  // disable for read-only mode
	RetentionDays  int     `yaml:"retention_days"` // case retention policy
	MaxCasesPerSig int     `yaml:"max_cases_per_signature"`
}

// DefaultKBConfig returns reasonable defaults for KB configuration
func DefaultKBConfig() *KBConfig {
	return &KBConfig{
		Enabled:        true,
		ReadThreshold:  0.7,
		WriteEnabled:   true,
		RetentionDays:  90,
		MaxCasesPerSig: 50,
	}
}

// Extend SelfHealConfig to include KB configuration
type EnhancedSelfHealConfig struct {
	*SelfHealConfig
	KB *KBConfig `yaml:"kb"`
}

// KBContext contains KB data for healing decisions
type KBContext struct {
	Language         string
	Signature        string
	RecommendedFixes []PromotedFix
	HasData          bool
	MatchConfidence  float64
}

// KBIntegrator defines the interface for KB integration functionality
type KBIntegrator interface {
	LoadKBContext(ctx context.Context, lang string, stdout, stderr []byte) (*KBContext, error)
	WriteHealingCase(ctx context.Context, kbCtx *KBContext, attempt *HealingAttempt, outcome *HealingOutcome, stdout, stderr string) error
	ShouldUseKBSuggestions(kbCtx *KBContext) bool
	ConvertKBFixesToBranchSpecs(fixes []PromotedFix) []BranchSpec
}

// KBIntegration provides KB functionality for the Mods healing workflow
type KBIntegration struct {
	storage KBStorage
	lockMgr KBLockManager
	sigGen  SignatureGenerator
	summary *SummaryComputer
	config  *KBConfig
}

// Ensure KBIntegration implements KBIntegrator
var _ KBIntegrator = (*KBIntegration)(nil)

// NewKBIntegration creates a new KB integration with SeaweedFS and Consul backends
func NewKBIntegration(storageBackend storage.Storage, kvStore orchestration.KV, config *KBConfig) *KBIntegration {
	if config == nil {
		config = DefaultKBConfig()
	}

	lockMgr := NewConsulKBLockManager(kvStore)
	kbStorage := NewSeaweedFSKBStorage(storageBackend, lockMgr)
	sigGen := NewDefaultSignatureGenerator()
	summaryComputer := NewSummaryComputer(kbStorage, lockMgr, DefaultSummaryConfig())

	return &KBIntegration{
		storage: kbStorage,
		lockMgr: lockMgr,
		sigGen:  sigGen,
		summary: summaryComputer,
		config:  config,
	}
}

// LoadKBContext retrieves KB suggestions for a given error
func (kb *KBIntegration) LoadKBContext(ctx context.Context, lang string, stdout, stderr []byte) (*KBContext, error) {
	if !kb.config.Enabled {
		return &KBContext{HasData: false}, nil
	}

	// Generate error signature
	signature := kb.sigGen.GenerateSignature(lang, "unknown", stdout, stderr)

	// Load recommended fixes
	fixes, err := kb.summary.GetRecommendedFixes(ctx, lang, signature, 5)
	if err != nil {
		// KB read failure shouldn't break the workflow
		return &KBContext{
			Language:  lang,
			Signature: signature,
			HasData:   false,
		}, nil
	}

	// Determine match confidence based on fix scores
	var maxConfidence float64
	for _, fix := range fixes {
		if fix.Score > maxConfidence {
			maxConfidence = fix.Score
		}
	}

	return &KBContext{
		Language:         lang,
		Signature:        signature,
		RecommendedFixes: fixes,
		HasData:          len(fixes) > 0,
		MatchConfidence:  maxConfidence,
	}, nil
}

// WriteHealingCase records a healing attempt in the KB
func (kb *KBIntegration) WriteHealingCase(ctx context.Context, kbCtx *KBContext, attempt *HealingAttempt, outcome *HealingOutcome, stdout, stderr string) error {
	if !kb.config.Enabled || !kb.config.WriteEnabled {
		return nil
	}

	// Generate run ID (could be passed in from the runner)
	runID := fmt.Sprintf("run-%d", time.Now().Unix())

	// Create sanitized logs
	sanitizedLogs := CreateSanitizedLogs(stdout, stderr)

	// Create case context (would be populated with more data in real implementation)
	caseContext := &CaseContext{
		Language: kbCtx.Language,
		// Additional context fields would be populated here
	}

	// Create case record
	caseRecord := &CaseRecord{
		RunID:     runID,
		Timestamp: time.Now(),
		Language:  kbCtx.Language,
		Signature: kbCtx.Signature,
		Context:   caseContext,
		Attempt:   attempt,
		Outcome:   outcome,
		BuildLogs: sanitizedLogs,
	}

	// Store the case
	err := kb.storage.WriteCase(ctx, kbCtx.Language, kbCtx.Signature, runID, caseRecord)
	if err != nil {
		// Log the error but don't fail the workflow
		fmt.Printf("Warning: Failed to write KB case: %v\n", err)
		return nil // Non-blocking
	}

	// Try to update summary (non-blocking)
	go func() {
		bgCtx := context.Background()
		_ = kb.summary.UpdateSummaryAfterCase(bgCtx, kbCtx.Language, kbCtx.Signature)
	}()

	return nil
}

// ShouldUseKBSuggestions determines if KB suggestions should be used
func (kb *KBIntegration) ShouldUseKBSuggestions(kbCtx *KBContext) bool {
	if !kb.config.Enabled || !kbCtx.HasData {
		return false
	}

	return kbCtx.MatchConfidence >= kb.config.ReadThreshold
}

// ConvertKBFixesToBranchSpecs converts KB fixes to healing branch specifications
func (kb *KBIntegration) ConvertKBFixesToBranchSpecs(fixes []PromotedFix) []BranchSpec {
	var branches []BranchSpec

	for i, fix := range fixes {
		branchID := fmt.Sprintf("kb-fix-%d", i)

		var branchType string
		var inputs map[string]interface{}

		switch fix.Kind {
		case "orw_recipe":
			branchType = string(StepTypeORWApply)
			inputs = map[string]interface{}{
				"id":     branchID,
				"type":   string(StepTypeORWApply),
				"recipe": fix.Ref,
				"source": "kb",
				"score":  fix.Score,
			}
		case "patch_fingerprint":
			branchType = "patch-apply"
			inputs = map[string]interface{}{
				"id":                branchID,
				"type":              "patch-apply",
				"patch_fingerprint": fix.Ref,
				"source":            "kb",
				"score":             fix.Score,
			}
		}

		branches = append(branches, BranchSpec{
			ID:     branchID,
			Type:   branchType,
			Inputs: inputs,
		})
	}

	return branches
}

// ExtendedJobSubmissionHelper extends the job submission helper with KB capabilities
type ExtendedJobSubmissionHelper struct {
	original JobSubmissionHelper
	kb       KBIntegrator
}

// NewExtendedJobSubmissionHelper creates a job submission helper with KB integration
func NewExtendedJobSubmissionHelper(original JobSubmissionHelper, kb KBIntegrator) *ExtendedJobSubmissionHelper {
	return &ExtendedJobSubmissionHelper{
		original: original,
		kb:       kb,
	}
}

// SubmitPlannerJob submits a planner job with KB context
func (e *ExtendedJobSubmissionHelper) SubmitPlannerJob(ctx context.Context, config *ModConfig, buildError string, workspace string) (*PlanResult, error) {
	// Extract language from config or detect from build error
	language := DetectLanguageFromBuildError(buildError)

	// Load KB context
	kbCtx, err := e.kb.LoadKBContext(ctx, language, []byte(""), []byte(buildError))
	if err != nil {
		// Continue without KB if loading fails
		return e.original.SubmitPlannerJob(ctx, config, buildError, workspace)
	}

	// If we have high-confidence KB suggestions, use them directly
	if e.kb.ShouldUseKBSuggestions(kbCtx) {
		// Convert KB fixes to options
		branches := e.kb.ConvertKBFixesToBranchSpecs(kbCtx.RecommendedFixes)

		// Convert to planner result format
		var options []map[string]interface{}
		for _, branch := range branches {
			options = append(options, branch.Inputs)
		}

		return &PlanResult{
			PlanID:  fmt.Sprintf("kb-plan-%d", time.Now().Unix()),
			Options: options,
		}, nil
	}

	// Otherwise, submit normal planner job
	// In the future, we could pass KB context to the planner for informed suggestions
	return e.original.SubmitPlannerJob(ctx, config, buildError, workspace)
}

// SubmitReducerJob submits a reducer job (unchanged for now)
func (e *ExtendedJobSubmissionHelper) SubmitReducerJob(ctx context.Context, planID string, results []BranchResult, winner *BranchResult, workspace string) (*NextAction, error) {
	return e.original.SubmitReducerJob(ctx, planID, results, winner, workspace)
}

// DetectLanguageFromBuildError attempts to detect the programming language from build error
func DetectLanguageFromBuildError(buildError string) string {
	// Simple heuristic-based language detection
	if containsAny(buildError, "javac", "Java", "java.lang", "org.java", "maven", "gradle") {
		return "java"
	}
	if containsAny(buildError, "go build", "go.mod", "package main", "func main") {
		return "go"
	}
	if containsAny(buildError, "tsc", "typescript", "node_modules", "npm", "yarn") {
		return "typescript"
	}
	if containsAny(buildError, "python", "pip", "pytest", "import ") {
		return "python"
	}
	if containsAny(buildError, "rustc", "cargo", "Cargo.toml") {
		return "rust"
	}

	return "unknown" // Default fallback
}

// containsAny checks if text contains any of the given substrings
func containsAny(text string, substrings ...string) bool {
	for _, substring := range substrings {
		if len(substring) > 0 && len(text) >= len(substring) {
			for i := 0; i <= len(text)-len(substring); i++ {
				if text[i:i+len(substring)] == substring {
					return true
				}
			}
		}
	}
	return false
}

// KBModRunner wraps the standard ModRunner with KB capabilities
type KBModRunner struct {
	*ModRunner
	kb KBIntegrator
}

// NewKBModRunner creates a Mod runner with KB integration
func NewKBModRunner(config *ModConfig, workspaceDir string, kb KBIntegrator) (*KBModRunner, error) {
	runner, err := NewModRunner(config, workspaceDir)
	if err != nil {
		return nil, err
	}

	return &KBModRunner{
		ModRunner: runner,
		kb:        kb,
	}, nil
}

// SetJobSubmitter extends the base implementation with KB-aware job submission
func (kr *KBModRunner) SetJobSubmitter(submitter JobSubmitter) {
	// Set the original submitter
	kr.ModRunner.SetJobSubmitter(submitter)

	// The attemptHealing method will need to be overridden or extended
	// to use the KB-enhanced job submission helper
}

// attemptHealing overrides the base implementation to use KB-enhanced healing
func (kr *KBModRunner) attemptHealing(ctx context.Context, repoPath string, buildError string) (*ModHealingSummary, error) {
	return kr.attemptHealingWithKB(ctx, repoPath, buildError)
}

// attemptHealingWithKB is an enhanced version of attemptHealing that uses KB
func (kr *KBModRunner) attemptHealingWithKB(ctx context.Context, repoPath string, buildError string) (*ModHealingSummary, error) {
	summary := &ModHealingSummary{
		Enabled:       true,
		AttemptsCount: 1,
	}

	// Detect language from build error
	language := DetectLanguageFromBuildError(buildError)

	// Load KB context
	kbCtx, err := kr.kb.LoadKBContext(ctx, language, []byte(""), []byte(buildError))
	if err != nil {
		// Continue without KB if loading fails
		kbCtx = &KBContext{HasData: false}
	}

	// Create KB-enhanced job helper (use production runner to enable Nomad submit)
	originalHelper := NewJobSubmissionHelperWithRunner(kr.jobSubmitter, kr.ModRunner)
	jobHelper := NewExtendedJobSubmissionHelper(originalHelper, kr.kb)

	// Submit planner job (may use KB suggestions directly)
	planResult, err := jobHelper.SubmitPlannerJob(ctx, kr.config, buildError, kr.workspaceDir)
	if err != nil {
		return summary, fmt.Errorf("planner job failed: %w", err)
	}

	summary.PlanID = planResult.PlanID

	// Convert planner options to branch specs
	var branches []BranchSpec
	for i, option := range planResult.Options {
		branchID := fmt.Sprintf("option-%d", i)
		if id, ok := option["id"].(string); ok {
			branchID = id
		}

		// Default and normalize planner types to canonical values (e.g., human -> human-step)
		branchType := string(StepTypeLLMExec)
		if t, ok := option["type"].(string); ok {
			branchType = string(NormalizeStepType(t))
		}

		branches = append(branches, BranchSpec{
			ID:     branchID,
			Type:   branchType,
			Inputs: option,
		})
	}

	// Execute fanout orchestration using production runner (Nomad HCL submit)
	orchestrator := NewFanoutOrchestratorWithRunner(kr.jobSubmitter, kr.ModRunner)
	maxParallel := 3
	if kr.config.SelfHeal.MaxRetries > 0 {
		maxParallel = kr.config.SelfHeal.MaxRetries
	}

	winner, allResults, err := orchestrator.RunHealingFanout(ctx, nil, branches, maxParallel)
	summary.AllResults = allResults

	if err != nil {
		summary.Winner = nil
	} else {
		summary.Winner = &winner
	}

	// Submit reducer job
	nextAction, reducerErr := jobHelper.SubmitReducerJob(ctx, planResult.PlanID, allResults, summary.Winner, kr.workspaceDir)
	if reducerErr != nil {
		return summary, fmt.Errorf("reducer job failed: %w", reducerErr)
	}

	// Write KB cases for all attempts
	for _, result := range allResults {
		attempt := kr.convertBranchResultToAttempt(result)
		outcome := kr.convertBranchResultToOutcome(result)

		// Write the case (non-blocking)
		go func(a *HealingAttempt, o *HealingOutcome) {
			_ = kr.kb.WriteHealingCase(context.Background(), kbCtx, a, o, buildError, "")
		}(attempt, outcome)
	}

	// Check final result
	if nextAction.Action == "stop" && summary.Winner != nil {
		summary.SetFinalResult(true)
		return summary, nil
	}

	return summary, fmt.Errorf("healing failed: %s", nextAction.Notes)
}

// Helper methods to convert between data structures
func (kr *KBModRunner) convertBranchResultToAttempt(result BranchResult) *HealingAttempt {
	// This would extract the actual healing attempt from the branch result
	// For now, return a basic structure
	return &HealingAttempt{
		Type: "llm_patch", // Would be determined from the branch type
	}
}

func (kr *KBModRunner) convertBranchResultToOutcome(result BranchResult) *HealingOutcome {
	return &HealingOutcome{
		Success:     result.Status == "success",
		BuildStatus: result.Status,
		Duration:    int64(result.Duration.Milliseconds()),
		CompletedAt: result.FinishedAt,
	}
}
