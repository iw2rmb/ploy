package nodeagent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/stackdetect"
)

const (
	sbomDependencyOutputFileName = "sbom.dependencies.txt"
	sbomJavaClasspathFileName    = "java.classpath"
	sbomShareMountPath           = "/share"
	sbomImageRegistryEnvKey      = "PLOY_CONTAINER_REGISTRY"
	sbomImageRegistryDefault     = "ghcr.io/iw2rmb/ploy"
	sbomScriptDir                = "/usr/local/lib/ploy/sbom"
	sbomGradleCollectorScript    = sbomScriptDir + "/collect-java-classpath-gradle.sh"
	sbomMavenCollectorScript     = sbomScriptDir + "/collect-java-classpath-maven.sh"
	sbomReleaseJDK11             = "11"
	sbomReleaseJDK17             = "17"
	sbomReleaseJDK21             = "21"
	sbomReleaseJDK25             = "25"
)

// --- Shared manifest helpers ---

// resolveImage validates and resolves a JobImage to a concrete image string using the
// given stack. Returns an error if the image is empty after resolution.
func resolveImage(
	img contracts.JobImage,
	stack contracts.MigStack,
	stackExp *contracts.StackExpectation,
	label string,
) (string, error) {
	if img.IsEmpty() {
		return "", fmt.Errorf("%s: image required", label)
	}
	resolved, err := img.ResolveImage(stack)
	if err != nil {
		return "", fmt.Errorf("%s image resolution: %w", label, err)
	}
	expanded, err := contracts.ExpandImageTemplate(resolved, stackExp)
	if err != nil {
		return "", fmt.Errorf("%s image template expansion: %w", label, err)
	}
	resolved = strings.TrimSpace(expanded)
	if resolved == "" {
		return "", fmt.Errorf("%s: image required", label)
	}
	return resolved, nil
}

// injectRepoMetadataEnv adds PLOY_REPO_URL, PLOY_BASE_REF, PLOY_TARGET_REF, and
// PLOY_COMMIT_SHA to env from the request. Only non-empty values are set.
func injectRepoMetadataEnv(env map[string]string, req StartRunRequest) {
	if v := strings.TrimSpace(req.RepoURL.String()); v != "" {
		env["PLOY_REPO_URL"] = v
	}
	if v := strings.TrimSpace(req.BaseRef.String()); v != "" {
		env["PLOY_BASE_REF"] = v
	}
	if v := strings.TrimSpace(req.TargetRef.String()); v != "" {
		env["PLOY_TARGET_REF"] = v
	}
	if v := strings.TrimSpace(req.CommitSHA.String()); v != "" {
		env["PLOY_COMMIT_SHA"] = v
	}
}

func buildSBOMManifest(req StartRunRequest, cycleName string, persistedStack contracts.MigStack) (contracts.StepManifest, error) {
	if req.RunID.IsZero() {
		return contracts.StepManifest{}, errors.New("run_id required")
	}
	if req.JobID.IsZero() {
		return contracts.StepManifest{}, errors.New("job_id required")
	}
	if strings.TrimSpace(req.RepoURL.String()) == "" {
		return contracts.StepManifest{}, errors.New("repo_url required")
	}

	targetRef := strings.TrimSpace(req.TargetRef.String())
	if targetRef == "" && strings.TrimSpace(req.BaseRef.String()) != "" {
		targetRef = strings.TrimSpace(req.BaseRef.String())
	}
	repo := contracts.RepoMaterialization{
		URL:       req.RepoURL,
		BaseRef:   req.BaseRef,
		TargetRef: types.GitRef(targetRef),
		Commit:    req.CommitSHA,
	}

	stack := resolveSBOMStackForCycle(cycleName, persistedStack, req.TypedOptions)
	env := make(map[string]string, len(req.Env)+5)
	for k, v := range req.Env {
		env[k] = v
	}
	injectRepoMetadataEnv(env, req)
	injectStackTupleEnv(env, stackExpectationForRequest(req, stack))
	env["PLOY_SBOM_CYCLE"] = strings.TrimSpace(cycleName)
	env["PLOY_SBOM_STACK"] = string(stack)
	env["PLOY_SBOM_DEPENDENCY_OUTPUT"] = sbomShareMountPath + "/" + sbomDependencyOutputFileName
	env["PLOY_SBOM_JAVA_CLASSPATH_OUTPUT"] = sbomShareMountPath + "/" + sbomJavaClasspathFileName

	manifest := contracts.StepManifest{
		ID:         types.StepID(req.JobID),
		Name:       fmt.Sprintf("SBOM %s for run %s", strings.TrimSpace(cycleName), req.RunID),
		WorkingDir: "/workspace",
		Envs:       env,
		Gate:       &contracts.StepGateSpec{Enabled: false},
		Inputs: []contracts.StepInput{
			{
				Name:      "workspace",
				MountPath: "/workspace",
				Mode:      contracts.StepInputModeReadWrite,
				Hydration: &contracts.StepInputHydration{Repo: &repo},
			},
		},
	}
	if phase := sbomPhaseConfigForCycle(cycleName, req.TypedOptions); phase != nil {
		manifest.CA = append(manifest.CA, phase.CA...)
	}
	runtimeRelease := sbomRuntimeReleaseForRequest(req, stack)
	if err := applySBOMRuntimeForStack(&manifest, stack, runtimeRelease); err != nil {
		return contracts.StepManifest{}, err
	}
	return manifest, nil
}

func resolveSBOMStackForCycle(cycleName string, persistedStack contracts.MigStack, typedOpts RunOptions) contracts.MigStack {
	stack := normalizeSBOMStack(persistedStack)
	if stack != contracts.MigStackUnknown {
		return stack
	}
	return sbomStackHintFromPhase(sbomPhaseConfigForCycle(cycleName, typedOpts))
}

func sbomPhaseConfigForCycle(cycleName string, typedOpts RunOptions) *contracts.BuildGatePhaseConfig {
	switch strings.TrimSpace(cycleName) {
	case preGateCycleName:
		return typedOpts.BuildGate.Pre
	case postGateCycleName:
		return typedOpts.BuildGate.Post
	}
	return nil
}

func sbomStackHintFromPhase(phase *contracts.BuildGatePhaseConfig) contracts.MigStack {
	if phase == nil || phase.Stack == nil || !phase.Stack.Enabled {
		return contracts.MigStackUnknown
	}
	switch strings.ToLower(strings.TrimSpace(phase.Stack.Tool)) {
	case "maven":
		return contracts.MigStackJavaMaven
	case "gradle":
		return contracts.MigStackJavaGradle
	}
	if strings.EqualFold(strings.TrimSpace(phase.Stack.Language), "java") {
		return contracts.MigStackJava
	}
	return contracts.MigStackUnknown
}

func detectSBOMStackFromWorkspace(workspace string) (contracts.MigStack, error) {
	trimmedWorkspace := strings.TrimSpace(workspace)
	if trimmedWorkspace == "" {
		return contracts.MigStackUnknown, fmt.Errorf("workspace path is required for sbom stack detection")
	}
	obs, err := stackdetect.DetectTool(context.Background(), trimmedWorkspace)
	if err != nil {
		return contracts.MigStackUnknown, fmt.Errorf("detect sbom tool: %w", err)
	}
	stack := contracts.ToolToMigStack(obs.Tool)
	if stack == contracts.MigStackUnknown {
		return contracts.MigStackUnknown, fmt.Errorf("unsupported sbom tool %q", strings.TrimSpace(obs.Tool))
	}
	return normalizeSBOMStack(stack), nil
}

func applySBOMRuntimeForStack(manifest *contracts.StepManifest, stack contracts.MigStack, release string) error {
	if manifest == nil {
		return errors.New("sbom manifest required")
	}
	runtimeStack := resolveSBOMRuntimeStack(stack)
	runtimeRelease := normalizeSBOMRuntimeRelease(release)
	image, err := resolveImage(
		sbomJobImageSpec(runtimeRelease),
		runtimeStack,
		sbomRuntimeStackExpectation(runtimeStack, runtimeRelease),
		"sbom",
	)
	if err != nil {
		return err
	}
	manifest.Image = image
	manifest.Command = sbomCommandForStack(stack).ToSlice()
	if len(manifest.Command) == 0 {
		return fmt.Errorf("sbom stack %q command required", stack)
	}
	if manifest.Envs == nil {
		manifest.Envs = map[string]string{}
	}
	if strings.TrimSpace(manifest.Envs["PLOY_SBOM_DEPENDENCY_OUTPUT"]) == "" {
		manifest.Envs["PLOY_SBOM_DEPENDENCY_OUTPUT"] = sbomShareMountPath + "/" + sbomDependencyOutputFileName
	}
	if strings.TrimSpace(manifest.Envs["PLOY_SBOM_JAVA_CLASSPATH_OUTPUT"]) == "" {
		manifest.Envs["PLOY_SBOM_JAVA_CLASSPATH_OUTPUT"] = sbomShareMountPath + "/" + sbomJavaClasspathFileName
	}
	injectStackTupleEnv(manifest.Envs, sbomRuntimeStackExpectation(runtimeStack, runtimeRelease))
	manifest.Envs["PLOY_SBOM_STACK"] = string(runtimeStack)
	return nil
}

func sbomJobImageSpec(release string) contracts.JobImage {
	prefix := strings.TrimRight(strings.TrimSpace(os.Getenv(sbomImageRegistryEnvKey)), "/")
	if prefix == "" {
		prefix = sbomImageRegistryDefault
	}
	gateGradleTag := sbomRuntimeTagForRelease(release)
	mavenTag := "3-eclipse-temurin-17"
	switch normalizeSBOMRuntimeRelease(release) {
	case sbomReleaseJDK11:
		mavenTag = "3-eclipse-temurin-11"
	case sbomReleaseJDK21:
		mavenTag = "3-eclipse-temurin-21"
	case sbomReleaseJDK25:
		mavenTag = "3-eclipse-temurin-25"
	}
	return contracts.JobImage{
		ByStack: map[contracts.MigStack]string{
			contracts.MigStackJavaMaven:  prefix + "/maven:" + mavenTag,
			contracts.MigStackJavaGradle: prefix + "/gate-gradle:" + gateGradleTag,
			contracts.MigStackDefault:    prefix + "/maven:" + mavenTag,
		},
	}
}

func sbomRuntimeReleaseForRequest(req StartRunRequest, fallback contracts.MigStack) string {
	exp := stackExpectationForRequest(req, fallback)
	if exp == nil {
		return sbomReleaseJDK17
	}
	return normalizeSBOMRuntimeRelease(exp.Release)
}

func normalizeSBOMRuntimeRelease(release string) string {
	switch strings.TrimSpace(release) {
	case sbomReleaseJDK11:
		return sbomReleaseJDK11
	case sbomReleaseJDK17:
		return sbomReleaseJDK17
	case sbomReleaseJDK21:
		return sbomReleaseJDK21
	case sbomReleaseJDK25:
		return sbomReleaseJDK25
	default:
		return sbomReleaseJDK17
	}
}

func sbomRuntimeTagForRelease(release string) string {
	switch normalizeSBOMRuntimeRelease(release) {
	case sbomReleaseJDK11:
		return "jdk11"
	case sbomReleaseJDK21:
		return "jdk21"
	case sbomReleaseJDK25:
		return "jdk25"
	default:
		return "jdk17"
	}
}

func sbomRuntimeStackExpectation(stack contracts.MigStack, release string) *contracts.StackExpectation {
	tool := "maven"
	if stack == contracts.MigStackJavaGradle {
		tool = "gradle"
	}
	return &contracts.StackExpectation{
		Language: "java",
		Tool:     tool,
		Release:  normalizeSBOMRuntimeRelease(release),
	}
}

func sbomCommandForStack(stack contracts.MigStack) contracts.CommandSpec {
	switch normalizeSBOMStack(stack) {
	case contracts.MigStackJavaGradle:
		return contracts.CommandSpec{
			Shell: "set -eu; if [ -x /workspace/gradlew ]; then PLOY_SBOM_GRADLE_CMD=\"/workspace/gradlew\" " + sbomGradleCollectorScript +
				"; else PLOY_SBOM_GRADLE_CMD=\"gradle\" " + sbomGradleCollectorScript + "; fi",
		}
	case contracts.MigStackJavaMaven:
		return contracts.CommandSpec{
			Shell: "set -eu; if [ ! -f /workspace/pom.xml ]; then echo \"missing /workspace/pom.xml\" >&2; exit 1; fi; " +
				sbomMavenCollectorScript,
		}
	default:
		return contracts.CommandSpec{
			Shell: "set -eu; if [ -f /workspace/pom.xml ]; then " +
				sbomMavenCollectorScript +
				"; exit 0; fi; if [ -x /workspace/gradlew ]; then PLOY_SBOM_GRADLE_CMD=\"/workspace/gradlew\" " +
				sbomGradleCollectorScript +
				"; exit 0; fi; if [ -f /workspace/build.gradle ] || [ -f /workspace/build.gradle.kts ] || [ -f /workspace/settings.gradle ] || [ -f /workspace/settings.gradle.kts ]; then if command -v gradle >/dev/null 2>&1; then " +
				"PLOY_SBOM_GRADLE_CMD=\"gradle\" " + sbomGradleCollectorScript +
				"; exit 0; fi; echo \"gradle build detected but no gradle wrapper and no gradle binary available\" >&2; exit 1; fi; echo \"unable to resolve sbom collector: expected pom.xml or gradle markers\" >&2; exit 1",
		}
	}
}

func resolveSBOMRuntimeStack(stack contracts.MigStack) contracts.MigStack {
	switch normalizeSBOMStack(stack) {
	case contracts.MigStackJavaGradle:
		return contracts.MigStackJavaGradle
	case contracts.MigStackJavaMaven:
		return contracts.MigStackJavaMaven
	default:
		return contracts.MigStackJavaMaven
	}
}

func normalizeSBOMStack(stack contracts.MigStack) contracts.MigStack {
	switch stack {
	case contracts.MigStackJavaMaven, contracts.MigStackJavaGradle, contracts.MigStackJava:
		return stack
	default:
		return contracts.MigStackUnknown
	}
}

// --- Main manifest builders ---

// buildManifestFromRequest converts a StartRunRequest into a StepManifest.
// For multi-step runs, stepIndex selects the step; for single-step runs it is ignored.
// The stack parameter resolves stack-specific images (pass MigStackUnknown if unknown).
func buildManifestFromRequest(req StartRunRequest, typedOpts RunOptions, stepIndex int, stack contracts.MigStack) (contracts.StepManifest, error) {
	if req.RunID.IsZero() {
		return contracts.StepManifest{}, errors.New("run_id required")
	}
	if req.JobID.IsZero() {
		return contracts.StepManifest{}, errors.New("job_id required")
	}
	if strings.TrimSpace(req.RepoURL.String()) == "" {
		return contracts.StepManifest{}, errors.New("repo_url required")
	}

	const defaultImage = "ubuntu:latest"
	image := defaultImage
	command := []string(nil)
	env := make(map[string]string, len(req.Env))
	stackExp := stackExpectationForRequest(req, stack)

	var hydraCA, hydraIn, hydraOut, hydraHome []string
	if len(typedOpts.Steps) > 0 {
		// Multi-step run.
		if stepIndex < 0 || stepIndex >= len(typedOpts.Steps) {
			return contracts.StepManifest{}, fmt.Errorf("step index %d out of range (0-%d)", stepIndex, len(typedOpts.Steps)-1)
		}
		stepMig := typedOpts.Steps[stepIndex]

		if !stepMig.Image.IsEmpty() {
			resolved, err := resolveImage(stepMig.Image, stack, stackExp, fmt.Sprintf("step[%d]", stepIndex))
			if err != nil {
				return contracts.StepManifest{}, err
			}
			image = resolved
		}
		command = stepMig.Command.ToSlice()
		hydraCA = stepMig.CA
		hydraIn = stepMig.In
		hydraOut = stepMig.Out
		hydraHome = stepMig.Home

		for k, v := range req.Env {
			env[k] = v
		}
		for k, v := range stepMig.Env {
			env[k] = v
		}
	} else {
		// Single-step run.
		if !typedOpts.Execution.Image.IsEmpty() {
			resolved, err := resolveImage(typedOpts.Execution.Image, stack, stackExp, "execution")
			if err != nil {
				return contracts.StepManifest{}, err
			}
			image = resolved
		}
		command = typedOpts.Execution.Command.ToSlice()
		hydraCA = typedOpts.Execution.CA
		hydraIn = typedOpts.Execution.In
		hydraOut = typedOpts.Execution.Out
		hydraHome = typedOpts.Execution.Home

		for k, v := range req.Env {
			env[k] = v
		}
	}

	injectStackTupleEnv(env, stackExp)

	// Inject placeholder command only for default ubuntu image.
	if len(command) == 0 && image == defaultImage {
		command = []string{"/bin/sh", "-c", "echo 'Build gate placeholder'"}
	}

	targetRef := strings.TrimSpace(req.TargetRef.String())
	if targetRef == "" && strings.TrimSpace(req.BaseRef.String()) != "" {
		targetRef = strings.TrimSpace(req.BaseRef.String())
	}

	repo := contracts.RepoMaterialization{
		URL:       req.RepoURL,
		BaseRef:   req.BaseRef,
		TargetRef: types.GitRef(targetRef),
		Commit:    req.CommitSHA,
	}

	// Build manifest options from typed accessors.
	mergedOpts := make(map[string]any)
	if pat := strings.TrimSpace(typedOpts.MRWiring.GitLabPAT); pat != "" {
		mergedOpts["gitlab_pat"] = pat
	}
	if domain := strings.TrimSpace(typedOpts.MRWiring.GitLabDomain); domain != "" {
		mergedOpts["gitlab_domain"] = domain
	}
	if typedOpts.MRFlagsPresent.MROnSuccessSet {
		mergedOpts["mr_on_success"] = typedOpts.MRWiring.MROnSuccess
	}
	if typedOpts.MRFlagsPresent.MROnFailSet {
		mergedOpts["mr_on_fail"] = typedOpts.MRWiring.MROnFail
	}
	if !typedOpts.ServerMetadata.JobID.IsZero() {
		mergedOpts["job_id"] = typedOpts.ServerMetadata.JobID.String()
	}
	if aname := strings.TrimSpace(typedOpts.Artifacts.Name); aname != "" {
		mergedOpts["artifact_name"] = aname
	}

	// Derive gate ref: CommitSHA > TargetRef > BaseRef.
	gateRef := ""
	if sha := strings.TrimSpace(req.CommitSHA.String()); sha != "" {
		gateRef = sha
	} else if tr := strings.TrimSpace(req.TargetRef.String()); tr != "" {
		gateRef = tr
	} else if br := strings.TrimSpace(req.BaseRef.String()); br != "" {
		gateRef = br
	}

	stepID := types.StepID(req.JobID)

	// Gate env mirrors the job env.
	gateEnv := make(map[string]string, len(env))
	for k, v := range env {
		gateEnv[k] = v
	}

	manifest := contracts.StepManifest{
		ID:         stepID,
		Name:       fmt.Sprintf("Run %s", req.RunID),
		Image:      image,
		Command:    command,
		WorkingDir: "/workspace",
		Envs:       env,
		CA:         hydraCA,
		In:         hydraIn,
		Out:        hydraOut,
		Home:       hydraHome,
		BundleMap:  typedOpts.BundleMap,
		Gate: &contracts.StepGateSpec{
			Enabled:        true,
			Env:            gateEnv,
			ImageOverrides: nil,
			RepoID:         req.RepoID,
			RepoURL:        types.RepoURL(strings.TrimSpace(req.RepoURL.String())),
			Ref:            types.GitRef(strings.TrimSpace(gateRef)),
		},
		Inputs: []contracts.StepInput{
			{
				Name:      "workspace",
				MountPath: "/workspace",
				Mode:      contracts.StepInputModeReadWrite,
				Hydration: &contracts.StepInputHydration{
					Repo: &repo,
				},
			},
		},
		Options: mergedOpts,
	}

	manifest.Gate.Enabled = typedOpts.BuildGate.Enabled
	manifest.Gate.ImageOverrides = typedOpts.BuildGate.Images

	return manifest, nil
}

// buildGateManifestFromRequest builds a StepManifest for gate jobs (pre_gate,
// post_gate). Gate jobs use the default image since stack detection
// happens inside the Build Gate itself.
func buildGateManifestFromRequest(req StartRunRequest, typedOpts RunOptions) (contracts.StepManifest, error) {
	sanitized := typedOpts
	sanitized.Steps = nil
	sanitized.Execution.Image = contracts.JobImage{}
	sanitized.Execution.Command = contracts.CommandSpec{}

	manifest, err := buildManifestFromRequest(req, sanitized, 0, contracts.MigStackUnknown)
	if err != nil {
		return manifest, err
	}

	if typedOpts.StackGate != nil {
		manifest.Gate.StackGate = typedOpts.StackGate
	}

	return manifest, nil
}

// isAmataHealingImage returns true if the image name indicates an Amata-based healing container.
func isAmataHealingImage(image string) bool {
	return strings.Contains(strings.ToLower(image), "amata")
}

// buildHealingManifest constructs a StepManifest from a typed MigContainerSpec.
// The healing mig runs with /workspace (RW), /out (RW), and /in (RO) mounts.
// When codexSession is non-empty and the image is Amata-based, CODEX_RESUME=1 is injected.
func buildHealingManifest(req StartRunRequest, mig MigContainerSpec, index int, codexSession string, stack contracts.MigStack) (contracts.StepManifest, error) {
	if req.JobID.IsZero() {
		return contracts.StepManifest{}, errors.New("job_id required")
	}

	stackExp := stackExpectationForRequest(req, stack)
	image, err := resolveImage(mig.Image, stack, stackExp, fmt.Sprintf("healing mig[%d]", index))
	if err != nil {
		return contracts.StepManifest{}, err
	}

	command := mig.Command.ToSlice()

	env := make(map[string]string, len(req.Env)+len(mig.Env)+4)
	for k, v := range req.Env {
		env[k] = v
	}
	for k, v := range mig.Env {
		env[k] = v
	}
	injectRepoMetadataEnv(env, req)
	injectStackTupleEnv(env, stackExpectationForRequest(req, stack))

	if codexSession != "" && isAmataHealingImage(image) {
		env["CODEX_RESUME"] = "1"
	}

	healingStepID := types.StepID(fmt.Sprintf("%s-heal-%d", req.JobID, index))

	manifest := contracts.StepManifest{
		ID:         healingStepID,
		Name:       fmt.Sprintf("Healing mig %d for run %s", index, req.RunID),
		Image:      image,
		Command:    command,
		WorkingDir: "/workspace",
		Envs:       env,
		CA:         mig.CA,
		In:         mig.In,
		Out:        mig.Out,
		Home:       mig.Home,
		Gate:       &contracts.StepGateSpec{Enabled: false},
		Inputs: []contracts.StepInput{
			{
				Name:      "workspace",
				MountPath: "/workspace",
				Mode:      contracts.StepInputModeReadWrite,
			},
		},
		Options: map[string]any{
			"mount_docker_socket": true,
		},
	}

	return manifest, nil
}

// --- Stack Gate chaining ---

// validateAndDeriveStackGateChaining validates and derives Stack Gate chaining for multi-step runs.
// For steps after the first, it derives inbound expectations from the previous step's outbound
// when omitted, and rejects mismatched explicit inbound. Migifies steps in place.
func validateAndDeriveStackGateChaining(steps []StepMig) error {
	if len(steps) <= 1 {
		return nil
	}

	for i := 1; i < len(steps); i++ {
		prev := steps[i-1]
		curr := &steps[i]

		if prev.Stack == nil || prev.Stack.Outbound == nil || !prev.Stack.Outbound.Enabled {
			continue
		}
		prevOutbound := prev.Stack.Outbound

		if curr.Stack == nil {
			curr.Stack = &contracts.StackGateSpec{
				Inbound: &contracts.StackGatePhaseSpec{
					Enabled: prevOutbound.Enabled,
					Expect:  prevOutbound.Expect,
				},
			}
			continue
		}

		if curr.Stack.Inbound == nil {
			curr.Stack.Inbound = &contracts.StackGatePhaseSpec{
				Enabled: prevOutbound.Enabled,
				Expect:  prevOutbound.Expect,
			}
			continue
		}

		currInbound := curr.Stack.Inbound
		if currInbound.Enabled && prevOutbound.Enabled {
			if currInbound.Expect != nil && prevOutbound.Expect != nil {
				if !currInbound.Expect.Equal(*prevOutbound.Expect) {
					return fmt.Errorf(
						"steps[%d].stack.inbound: mismatch with steps[%d].stack.outbound "+
							"(inbound: language=%q tool=%q release=%q, outbound: language=%q tool=%q release=%q)",
						i, i-1,
						currInbound.Expect.Language, currInbound.Expect.Tool, currInbound.Expect.Release,
						prevOutbound.Expect.Language, prevOutbound.Expect.Tool, prevOutbound.Expect.Release,
					)
				}
			}
		}
	}

	return nil
}

// stackGatePhaseSpecToStepGate converts a StackGatePhaseSpec to StepGateStackSpec.
// Returns nil if the input is nil or disabled.
func stackGatePhaseSpecToStepGate(phase *contracts.StackGatePhaseSpec, _ []contracts.BuildGateImageRule) *contracts.StepGateStackSpec {
	if phase == nil || !phase.Enabled {
		return nil
	}
	return &contracts.StepGateStackSpec{
		Enabled: phase.Enabled,
		Expect:  phase.Expect,
	}
}
