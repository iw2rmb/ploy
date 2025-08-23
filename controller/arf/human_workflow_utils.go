package arf

import "time"

// determineStakeholders determines stakeholders based on vulnerabilities and recipe
func (h *HumanWorkflowEngine) determineStakeholders(vulns []VulnerabilityInfo, recipe RemediationRecipe) []Stakeholder {
	stakeholders := []Stakeholder{
		{
			ID:   "security-team",
			Name: "Security Team",
			Role: RoleSecurityOfficer,
			Permissions: []Permission{PermissionApprove, PermissionReject},
			Required: true,
		},
	}
	
	// Add architect for high-impact changes
	if h.isHighImpactChange(recipe) {
		stakeholders = append(stakeholders, Stakeholder{
			ID:   "tech-architect",
			Name: "Technical Architect", 
			Role: RoleArchitect,
			Permissions: []Permission{PermissionApprove, PermissionReject, PermissionReview},
			Required: true,
		})
	}
	
	// Add product owner for business-critical applications
	if h.isBusinessCritical(recipe) {
		stakeholders = append(stakeholders, Stakeholder{
			ID:   "product-owner",
			Name: "Product Owner",
			Role: RoleProductOwner,
			Permissions: []Permission{PermissionApprove, PermissionReject},
			Required: false,
		})
	}
	
	return stakeholders
}

// calculatePriority calculates priority based on vulnerability severity
func (h *HumanWorkflowEngine) calculatePriority(vulns []VulnerabilityInfo) Priority {
	maxSeverity := "low"
	for _, vuln := range vulns {
		if vuln.Severity == "critical" {
			return PriorityCritical
		}
		if vuln.Severity == "high" && maxSeverity != "critical" {
			maxSeverity = "high"
		}
		if vuln.Severity == "medium" && maxSeverity != "high" && maxSeverity != "critical" {
			maxSeverity = "medium"
		}
	}
	
	switch maxSeverity {
	case "critical":
		return PriorityCritical
	case "high":
		return PriorityHigh
	case "medium":
		return PriorityMedium
	default:
		return PriorityLow
	}
}

// assessImpact assesses the impact of a remediation
func (h *HumanWorkflowEngine) assessImpact(recipe RemediationRecipe, vulns []VulnerabilityInfo) ImpactAssessment {
	// Simplified impact assessment - in practice, this would be more sophisticated
	securityImpact := ImpactHigh // Remediation always has security impact
	
	performanceImpact := ImpactLow
	if len(recipe.Recipe.Operations) > 5 {
		performanceImpact = ImpactMedium
	}
	
	stabilityImpact := ImpactMedium
	if recipe.Recipe.Type == "code_transformation" {
		stabilityImpact = ImpactHigh
	}
	
	return ImpactAssessment{
		SecurityImpact:    securityImpact,
		PerformanceImpact: performanceImpact,
		StabilityImpact:   stabilityImpact,
		BusinessImpact:    ImpactMedium,
		UserImpact:        ImpactLow,
		ComplianceImpact:  ImpactMedium,
	}
}

// createRemediationSteps creates workflow steps for remediation
func (h *HumanWorkflowEngine) createRemediationSteps(recipe RemediationRecipe, stakeholders []Stakeholder, priority Priority) []WorkflowStep {
	var steps []WorkflowStep
	
	// Step 1: Security approval
	steps = append(steps, WorkflowStep{
		ID:                "security-approval",
		Type:              StepApproval,
		Name:              "Security Team Approval",
		Description:       "Security team must approve the remediation approach",
		AssignedTo:        h.getStakeholderIDs(stakeholders, RoleSecurityOfficer),
		Status:            StepPending,
		RequiredApprovals: 1,
		Timeout:           h.getStepTimeout(priority),
	})
	
	// Step 2: Architecture review (for high-impact changes)
	if h.isHighImpactChange(recipe) {
		steps = append(steps, WorkflowStep{
			ID:                "architecture-review",
			Type:              StepReview,
			Name:              "Architecture Review",
			Description:       "Technical architecture review of the remediation",
			AssignedTo:        h.getStakeholderIDs(stakeholders, RoleArchitect),
			Status:            StepPending,
			RequiredApprovals: 1,
			Timeout:           h.getStepTimeout(priority),
			Dependencies:      []string{"security-approval"},
		})
	}
	
	// Step 3: Final approval
	steps = append(steps, WorkflowStep{
		ID:                "final-approval",
		Type:              StepApproval,
		Name:              "Final Approval",
		Description:       "Final approval to proceed with remediation",
		AssignedTo:        h.getStakeholderIDs(stakeholders, RoleApprover),
		Status:            StepPending,
		RequiredApprovals: 1,
		Timeout:           h.getStepTimeout(priority),
	})
	
	return steps
}

// determineRiskLevel determines risk level from vulnerabilities
func (h *HumanWorkflowEngine) determineRiskLevel(vulns []VulnerabilityInfo) string {
	for _, vuln := range vulns {
		if vuln.Severity == "critical" {
			return "critical"
		}
		if vuln.Severity == "high" {
			return "high"
		}
	}
	return "medium"
}

// extractBusinessContext extracts business context from recipe
func (h *HumanWorkflowEngine) extractBusinessContext(recipe RemediationRecipe) BusinessContext {
	// This would extract business context from recipe metadata
	return BusinessContext{
		ApplicationName: "Unknown",
		Environment:     "production",
		ServiceTier:     "tier2",
	}
}

// extractTechnicalContext extracts technical context from recipe
func (h *HumanWorkflowEngine) extractTechnicalContext(recipe RemediationRecipe) TechnicalContext {
	// This would extract technical context from recipe metadata
	return TechnicalContext{
		Language:     "java",
		Framework:    "spring",
		Architecture: "microservices",
	}
}

// calculateTimeout calculates timeout based on priority
func (h *HumanWorkflowEngine) calculateTimeout(priority Priority) time.Duration {
	switch priority {
	case PriorityCritical:
		return 4 * time.Hour
	case PriorityHigh:
		return 8 * time.Hour
	case PriorityMedium:
		return 24 * time.Hour
	default:
		return 48 * time.Hour
	}
}

// isHighImpactChange checks if the change is high impact
func (h *HumanWorkflowEngine) isHighImpactChange(recipe RemediationRecipe) bool {
	return recipe.Recipe.Type == "code_transformation" || len(recipe.Recipe.Operations) > 3
}

// isBusinessCritical checks if the application is business critical
func (h *HumanWorkflowEngine) isBusinessCritical(recipe RemediationRecipe) bool {
	// This would check business criticality from metadata
	return false
}

// determineSecurityReviewers determines security reviewers for a recipe
func (h *HumanWorkflowEngine) determineSecurityReviewers(recipe RemediationRecipe) []Stakeholder {
	return []Stakeholder{
		{
			ID:   "security-reviewer",
			Name: "Security Reviewer",
			Role: RoleReviewer,
			Permissions: []Permission{PermissionReview, PermissionComment},
			Required: true,
		},
	}
}

// calculateReviewPriority calculates review priority
func (h *HumanWorkflowEngine) calculateReviewPriority(recipe RemediationRecipe) Priority {
	if recipe.Recipe.Type == "code_transformation" {
		return PriorityHigh
	}
	return PriorityMedium
}

// createSecurityReviewSteps creates security review workflow steps
func (h *HumanWorkflowEngine) createSecurityReviewSteps(recipe RemediationRecipe, stakeholders []Stakeholder) []WorkflowStep {
	return []WorkflowStep{
		{
			ID:                "security-review",
			Type:              StepReview,
			Name:              "Security Review",
			Description:       "Review remediation for security implications",
			AssignedTo:        h.getStakeholderIDs(stakeholders, RoleReviewer),
			Status:            StepPending,
			RequiredApprovals: 1,
			Timeout:           24 * time.Hour,
		},
	}
}

// getStakeholderIDs gets IDs of stakeholders with a specific role
func (h *HumanWorkflowEngine) getStakeholderIDs(stakeholders []Stakeholder, role StakeholderRole) []string {
	var ids []string
	for _, s := range stakeholders {
		if s.Role == role {
			ids = append(ids, s.ID)
		}
	}
	return ids
}

// getStepTimeout gets timeout for a step based on priority
func (h *HumanWorkflowEngine) getStepTimeout(priority Priority) time.Duration {
	switch priority {
	case PriorityCritical:
		return 2 * time.Hour
	case PriorityHigh:
		return 4 * time.Hour
	case PriorityMedium:
		return 8 * time.Hour
	default:
		return 24 * time.Hour
	}
}