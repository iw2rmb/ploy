package arf

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

// GetTransformationHierarchy returns a hierarchical view of the transformation and its healing attempts
func (h *Handler) GetTransformationHierarchy(c *fiber.Ctx) error {
	transformID := c.Params("id")
	if transformID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "transformation_id is required",
		})
	}

	// Check if consul store is configured
	if h.consulStore == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Consul store not configured",
		})
	}

	// Get transformation status from Consul
	status, err := h.consulStore.GetTransformationStatus(c.Context(), transformID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "Failed to retrieve transformation status",
			"details": err.Error(),
		})
	}

	if status == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Transformation not found",
		})
	}

	// Check requested format
	format := c.Query("format", "json")

	// Build hierarchy visualization
	viz := buildHierarchyVisualization(status)

	// Return based on format
	switch format {
	case "tree":
		c.Set("Content-Type", "text/plain; charset=utf-8")
		return c.SendString(viz.Visualization)
	case "csv":
		csvData := generateCSVFromHierarchy(viz)
		c.Set("Content-Type", "text/csv")
		c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"hierarchy_%s.csv\"", transformID))
		return c.SendString(csvData)
	default: // json
		return c.JSON(viz)
	}
}

// GetActiveHealingAttempts returns currently active healing attempts for a transformation
func (h *Handler) GetActiveHealingAttempts(c *fiber.Ctx) error {
	transformID := c.Params("id")
	if transformID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "transformation_id is required",
		})
	}

	if h.consulStore == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Consul store not configured",
		})
	}

	status, err := h.consulStore.GetTransformationStatus(c.Context(), transformID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "Failed to retrieve transformation status",
			"details": err.Error(),
		})
	}

	if status == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Transformation not found",
		})
	}

	// Extract active attempts
	activeAttempts := extractActiveAttempts(status.Children)

	response := ActiveAttemptsResponse{
		TransformationID: transformID,
		ActiveAttempts:   activeAttempts,
		TotalActive:      len(activeAttempts),
	}

	// Estimate time remaining based on average duration
	if len(activeAttempts) > 0 {
		avgDuration := calculateAverageDuration(status.Children)
		if avgDuration > 0 {
			response.EstimatedTimeRemaining = avgDuration * time.Duration(len(activeAttempts))
		}
	}

	return c.JSON(response)
}

// GetTransformationTimeline returns a chronological timeline of all events
func (h *Handler) GetTransformationTimeline(c *fiber.Ctx) error {
	transformID := c.Params("id")
	if transformID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "transformation_id is required",
		})
	}

	if h.consulStore == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Consul store not configured",
		})
	}

	status, err := h.consulStore.GetTransformationStatus(c.Context(), transformID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "Failed to retrieve transformation status",
			"details": err.Error(),
		})
	}

	if status == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Transformation not found",
		})
	}

	// Build timeline
	timeline := buildTimeline(status)

	return c.JSON(timeline)
}

// GetTransformationAnalysis provides deep analysis of a transformation
func (h *Handler) GetTransformationAnalysis(c *fiber.Ctx) error {
	transformID := c.Params("id")
	if transformID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "transformation_id is required",
		})
	}

	if h.consulStore == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Consul store not configured",
		})
	}

	status, err := h.consulStore.GetTransformationStatus(c.Context(), transformID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "Failed to retrieve transformation status",
			"details": err.Error(),
		})
	}

	if status == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Transformation not found",
		})
	}

	// Perform deep analysis
	analysis := analyzeTransformation(status)

	return c.JSON(analysis)
}

// GetTransformationReport generates a human-readable markdown report
func (h *Handler) GetTransformationReport(c *fiber.Ctx) error {
	transformID := c.Params("id")
	if transformID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "transformation_id is required",
		})
	}

	// Check if consul store is configured
	if h.consulStore == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Consul store not configured",
		})
	}

	// Get transformation status from Consul
	status, err := h.consulStore.GetTransformationStatus(c.Context(), transformID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "Failed to retrieve transformation status",
			"details": err.Error(),
		})
	}

	if status == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Transformation not found",
		})
	}

	// Try to get transformation result for diff data
	var diff string
	if result, exists := globalTransformStore.get(transformID); exists && result != nil {
		diff = result.Diff
	}

	// Generate markdown report with diff if available
	report := generateMarkdownReport(status, diff)
	contentType := "text/markdown; charset=utf-8"

	c.Set("Content-Type", contentType)
	return c.SendString(report)
}

// GetOrphanedTransformations finds transformations with missing parent references
func (h *Handler) GetOrphanedTransformations(c *fiber.Ctx) error {
	if h.consulStore == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Consul store not configured",
		})
	}

	// This would typically require listing all transformations from Consul
	// For now, return a mock implementation
	response := OrphanedTransformationsResponse{
		OrphanedTransformations: []OrphanedTransformation{},
		TotalOrphaned:           0,
		RecommendedAction:       "Review and clean up orphaned transformations",
	}

	return c.JSON(response)
}

// Helper functions

func buildHierarchyVisualization(status *TransformationStatus) *HierarchyVisualization {
	viz := &HierarchyVisualization{
		TransformationID: status.TransformationID,
		Status:           status.Status,
		WorkflowStage:    status.WorkflowStage,
		StartTime:        status.StartTime,
	}

	if !status.EndTime.IsZero() {
		viz.EndTime = &status.EndTime
	}

	// Build root node
	viz.RootNode = buildHierarchyNode(status, "ROOT", 0)

	// Calculate metrics
	viz.Metrics = calculateHierarchyMetrics(viz.RootNode)

	// Generate ASCII visualization
	viz.Visualization = generateASCIITree(viz)

	return viz
}

func buildHierarchyNode(status *TransformationStatus, path string, depth int) *HierarchyNode {
	node := &HierarchyNode{
		TransformationID: status.TransformationID,
		AttemptPath:      path,
		Status:           status.Status,
		Depth:            depth,
		IsActive:         status.Status == "in_progress",
	}

	if !status.StartTime.IsZero() {
		node.StartTime = &status.StartTime
	}
	if !status.EndTime.IsZero() {
		node.EndTime = &status.EndTime
		duration := status.EndTime.Sub(status.StartTime)
		node.Duration = formatDuration(duration)
	}

	// Add children healing attempts
	for _, attempt := range status.Children {
		childNode := buildHierarchyNodeFromAttempt(&attempt, depth+1)
		node.Children = append(node.Children, childNode)
	}

	return node
}

func buildHierarchyNodeFromAttempt(attempt *HealingAttempt, depth int) *HierarchyNode {
	node := &HierarchyNode{
		TransformationID: attempt.TransformationID,
		AttemptPath:      attempt.AttemptPath,
		Status:           attempt.Status,
		Result:           attempt.Result,
		TriggerReason:    attempt.TriggerReason,
		Depth:            depth,
		IsActive:         attempt.Status == "in_progress",
		LLMAnalysis:      attempt.LLMAnalysis,
	}

	if !attempt.StartTime.IsZero() {
		node.StartTime = &attempt.StartTime
	}
	if !attempt.EndTime.IsZero() {
		node.EndTime = &attempt.EndTime
		duration := attempt.EndTime.Sub(attempt.StartTime)
		node.Duration = formatDuration(duration)
	}

	// Add nested children
	for _, child := range attempt.Children {
		childNode := buildHierarchyNodeFromAttempt(&child, depth+1)
		node.Children = append(node.Children, childNode)
	}

	return node
}

func calculateHierarchyMetrics(root *HierarchyNode) HierarchyMetrics {
	metrics := HierarchyMetrics{}

	// Traverse tree to calculate metrics
	var traverse func(*HierarchyNode, int)
	traverse = func(node *HierarchyNode, depth int) {
		if node.AttemptPath != "ROOT" {
			metrics.TotalAttempts++
		}

		// Track max depth for all nodes including children
		if depth > metrics.MaxDepth {
			metrics.MaxDepth = depth
		}

		if node.AttemptPath != "ROOT" {
			switch node.Status {
			case "completed":
				if node.Result == "success" {
					metrics.SuccessfulHeals++
				} else if node.Result == "failed" {
					metrics.FailedHeals++
				}
			case "in_progress":
				metrics.ActiveHeals++
			case "pending":
				metrics.PendingHeals++
			}
		}

		for _, child := range node.Children {
			traverse(child, depth+1)
		}
	}

	traverse(root, 0)

	// Calculate success rate
	if metrics.TotalAttempts > 0 {
		metrics.SuccessRate = float64(metrics.SuccessfulHeals) / float64(metrics.TotalAttempts)
	}

	// Calculate branching factor
	if metrics.TotalAttempts > 0 && metrics.MaxDepth > 0 {
		metrics.BranchingFactor = float64(metrics.TotalAttempts) / float64(metrics.MaxDepth)
	}

	return metrics
}

func generateASCIITree(viz *HierarchyVisualization) string {
	var buf bytes.Buffer
	symbols := GetDefaultSymbols()

	// Header
	buf.WriteString(fmt.Sprintf("Transformation Hierarchy: %s\n", viz.TransformationID))
	buf.WriteString(fmt.Sprintf("Status: %s | Total Attempts: %d | Max Depth: %d\n",
		viz.Status, viz.Metrics.TotalAttempts, viz.Metrics.MaxDepth))

	if viz.Metrics.EstimatedCost > 0 {
		buf.WriteString(fmt.Sprintf("Cost: $%.2f | ", viz.Metrics.EstimatedCost))
	}

	if viz.Metrics.TotalDuration > 0 {
		buf.WriteString(fmt.Sprintf("Duration: %s | ", formatDuration(viz.Metrics.TotalDuration)))
	}

	buf.WriteString(fmt.Sprintf("Success Rate: %.0f%%\n\n", viz.Metrics.SuccessRate*100))

	// Tree
	writeNode(&buf, viz.RootNode, "", true, symbols)

	return buf.String()
}

func writeNode(buf *bytes.Buffer, node *HierarchyNode, prefix string, isLast bool, symbols VisualizationSymbols) {
	// Determine status symbol
	statusSymbol := symbols.Pending
	switch node.Status {
	case "completed":
		if node.Result == "success" {
			statusSymbol = symbols.Success
		} else if node.Result == "failed" {
			statusSymbol = symbols.Failure
		}
	case "in_progress":
		statusSymbol = symbols.InProgress
	case "pending":
		statusSymbol = symbols.Pending
	}

	// Write node line
	if node.AttemptPath == "ROOT" {
		idDisplay := node.TransformationID
		if len(idDisplay) > 8 {
			idDisplay = idDisplay[:8]
		}
		buf.WriteString(fmt.Sprintf("└── [ROOT] %s (%s)", idDisplay, node.Status))
	} else {
		branch := symbols.Branch
		if isLast {
			branch = symbols.LastBranch
		}

		idDisplay := node.TransformationID
		if len(idDisplay) > 8 {
			idDisplay = idDisplay[:8]
		}
		info := fmt.Sprintf("[%s] %s %s", node.AttemptPath, idDisplay, statusSymbol)
		if node.TriggerReason != "" {
			info += fmt.Sprintf(" (%s", node.TriggerReason)
			if node.Result != "" {
				info += fmt.Sprintf(" → %s", node.Result)
			}
			info += ")"
		}
		if node.Duration != "" {
			info += " " + node.Duration
		}

		buf.WriteString(prefix + branch + " " + info)
	}
	buf.WriteString("\n")

	// Write children
	for i, child := range node.Children {
		childPrefix := prefix
		if node.AttemptPath != "ROOT" {
			if isLast {
				childPrefix += symbols.Space
			} else {
				childPrefix += symbols.Vertical + "   "
			}
		}
		writeNode(buf, child, childPrefix, i == len(node.Children)-1, symbols)
	}
}

func extractActiveAttempts(attempts []HealingAttempt) []ActiveAttemptDetails {
	var active []ActiveAttemptDetails

	var extract func([]HealingAttempt)
	extract = func(attempts []HealingAttempt) {
		for _, attempt := range attempts {
			if attempt.Status == "in_progress" {
				detail := ActiveAttemptDetails{
					AttemptPath:   attempt.AttemptPath,
					Status:        attempt.Status,
					TriggerReason: attempt.TriggerReason,
					StartTime:     attempt.StartTime,
					ElapsedTime:   time.Since(attempt.StartTime),
					Progress:      attempt.Progress,
				}
				active = append(active, detail)
			}
			// Recurse into children
			extract(attempt.Children)
		}
	}

	extract(attempts)
	return active
}

func calculateAverageDuration(attempts []HealingAttempt) time.Duration {
	var totalDuration time.Duration
	var count int

	for _, attempt := range attempts {
		if !attempt.EndTime.IsZero() {
			totalDuration += attempt.EndTime.Sub(attempt.StartTime)
			count++
		}
	}

	if count > 0 {
		return totalDuration / time.Duration(count)
	}
	return 0
}

func buildTimeline(status *TransformationStatus) *TransformationTimeline {
	timeline := &TransformationTimeline{
		TransformationID: status.TransformationID,
		Timeline:         []TimelineEntry{},
	}

	// Add transformation start
	if !status.StartTime.IsZero() {
		timeline.Timeline = append(timeline.Timeline, TimelineEntry{
			Timestamp:   status.StartTime,
			EventType:   "transformation_start",
			Description: "Transformation started",
			Status:      status.Status,
		})
	}

	// Add healing attempt events
	addAttemptEvents(&timeline.Timeline, status.Children)

	// Add transformation end
	if !status.EndTime.IsZero() {
		timeline.Timeline = append(timeline.Timeline, TimelineEntry{
			Timestamp:   status.EndTime,
			EventType:   "transformation_end",
			Description: "Transformation completed",
			Status:      status.Status,
		})
		timeline.TotalDuration = status.EndTime.Sub(status.StartTime)
	}

	// Sort timeline by timestamp
	sort.Slice(timeline.Timeline, func(i, j int) bool {
		return timeline.Timeline[i].Timestamp.Before(timeline.Timeline[j].Timestamp)
	})

	// Analyze gaps and parallel periods
	timeline.GapAnalysis = analyzeGaps(timeline.Timeline)
	timeline.ParallelPeriods = analyzeParallelPeriods(timeline.Timeline)

	return timeline
}

func addAttemptEvents(timeline *[]TimelineEntry, attempts []HealingAttempt) {
	for _, attempt := range attempts {
		// Add start event
		if !attempt.StartTime.IsZero() {
			*timeline = append(*timeline, TimelineEntry{
				Timestamp:   attempt.StartTime,
				EventType:   "attempt_start",
				AttemptPath: attempt.AttemptPath,
				Description: fmt.Sprintf("Healing attempt %s started (%s)", attempt.AttemptPath, attempt.TriggerReason),
				Status:      attempt.Status,
			})
		}

		// Add end event
		if !attempt.EndTime.IsZero() {
			duration := attempt.EndTime.Sub(attempt.StartTime)
			*timeline = append(*timeline, TimelineEntry{
				Timestamp:   attempt.EndTime,
				EventType:   "attempt_end",
				AttemptPath: attempt.AttemptPath,
				Description: fmt.Sprintf("Healing attempt %s completed with result: %s", attempt.AttemptPath, attempt.Result),
				Status:      attempt.Status,
				Duration:    &duration,
			})
		}

		// Recurse for children
		addAttemptEvents(timeline, attempt.Children)
	}
}

func analyzeGaps(timeline []TimelineEntry) []GapPeriod {
	var gaps []GapPeriod

	for i := 1; i < len(timeline); i++ {
		gap := timeline[i].Timestamp.Sub(timeline[i-1].Timestamp)
		if gap > 5*time.Minute { // Consider gaps longer than 5 minutes significant
			gaps = append(gaps, GapPeriod{
				Start:    timeline[i-1].Timestamp,
				End:      timeline[i].Timestamp,
				Duration: gap,
				Reason:   "Inactivity detected",
			})
		}
	}

	return gaps
}

func analyzeParallelPeriods(timeline []TimelineEntry) []ParallelPeriod {
	// Simple implementation - would need more sophisticated logic for real parallel analysis
	return []ParallelPeriod{}
}

func analyzeTransformation(status *TransformationStatus) *TransformationAnalysis {
	analysis := &TransformationAnalysis{
		TransformationID: status.TransformationID,
	}

	// Build summary
	totalAttempts := countAttempts(status.Children)
	successCount := countSuccessful(status.Children)

	var totalDuration time.Duration
	if !status.EndTime.IsZero() {
		totalDuration = status.EndTime.Sub(status.StartTime)
	} else {
		totalDuration = time.Since(status.StartTime)
	}

	// Calculate success rate with zero-attempt handling
	var successRate float64
	if totalAttempts > 0 {
		successRate = float64(successCount) / float64(totalAttempts)
	} else if status.Status == "completed" {
		// No healing attempts but transformation completed = 100% success
		successRate = 1.0
	}

	// Calculate max depth
	maxDepth := calculateMaxDepth(status.Children, 0)

	// Find most common error
	mostCommonError := ""
	if len(status.Children) > 0 {
		errorCounts := make(map[string]int)
		for _, attempt := range status.Children {
			if attempt.TriggerReason != "" {
				errorCounts[attempt.TriggerReason]++
			}
		}
		maxCount := 0
		for errorType, count := range errorCounts {
			if count > maxCount {
				maxCount = count
				mostCommonError = errorType
			}
		}
	}

	analysis.Summary = AnalysisSummary{
		TransformationID:   status.TransformationID,
		Status:             status.Status,
		TotalDurationHours: math.Round(totalDuration.Hours()*10) / 10, // Round to 1 decimal place
		TotalAttempts:      totalAttempts,
		SuccessRate:        successRate,
		MaxDepthReached:    maxDepth,
		MostCommonError:    mostCommonError,
		FinalResult:        status.Status,
	}

	// Analyze error patterns
	analysis.ErrorPatterns = analyzeErrorPatterns(status.Children)
	if analysis.ErrorPatterns == nil {
		analysis.ErrorPatterns = []HealingErrorPattern{}
	}

	// Enhanced cost analysis with zero-division protection
	totalCost := estimateCost(status.Children)
	infraCost := estimateInfrastructureCost(totalDuration)

	analysis.CostAnalysis = CostBreakdown{
		TotalCost:          totalCost + infraCost,
		LLMCost:            estimateLLMCost(status.Children),
		InfrastructureCost: infraCost,
		CostPerAttempt:     0.0, // Initialize to zero
		CostPerSuccess:     0.0, // Initialize to zero
		ROI:                calculateROI(totalCost+infraCost, totalDuration),
	}

	// Safe division with zero checks
	if totalAttempts > 0 {
		analysis.CostAnalysis.CostPerAttempt = analysis.CostAnalysis.TotalCost / float64(totalAttempts)
	}
	if successCount > 0 {
		analysis.CostAnalysis.CostPerSuccess = analysis.CostAnalysis.TotalCost / float64(successCount)
	} else if status.Status == "completed" && totalAttempts == 0 {
		// Successful transformation without healing attempts
		analysis.CostAnalysis.CostPerSuccess = analysis.CostAnalysis.TotalCost
	}

	// Performance metrics
	analysis.PerformanceMetrics = analyzePerformance(status.Children)

	// Add time to first success for completed transformations
	if status.Status == "completed" {
		analysis.PerformanceMetrics.TimeToFirstSuccess = totalDuration
	}

	// Generate recommendations
	analysis.Recommendations = generateRecommendations(analysis)

	// Add success factors for well-performing transformations
	analysis.SuccessFactors = identifySuccessFactors(status, totalAttempts, successRate)

	return analysis
}

func countAttempts(attempts []HealingAttempt) int {
	count := len(attempts)
	for _, attempt := range attempts {
		count += countAttempts(attempt.Children)
	}
	return count
}

func countSuccessful(attempts []HealingAttempt) int {
	count := 0
	for _, attempt := range attempts {
		if attempt.Result == "success" {
			count++
		}
		count += countSuccessful(attempt.Children)
	}
	return count
}

func estimateCost(attempts []HealingAttempt) float64 {
	// Simplified cost estimation - $0.10 per attempt + LLM costs
	baseCost := float64(countAttempts(attempts)) * 0.10

	// Add estimated LLM costs
	llmCost := estimateLLMCost(attempts)

	return baseCost + llmCost
}

func estimateLLMCost(attempts []HealingAttempt) float64 {
	llmCalls := 0
	var estimateLLMCalls func([]HealingAttempt)
	estimateLLMCalls = func(attempts []HealingAttempt) {
		for _, attempt := range attempts {
			if attempt.LLMAnalysis != nil {
				llmCalls++
			}
			// Recurse into children
			estimateLLMCalls(attempt.Children)
		}
	}

	estimateLLMCalls(attempts)
	return float64(llmCalls) * 0.05 // $0.05 per LLM call estimate
}

func estimateInfrastructureCost(duration time.Duration) float64 {
	// Estimate based on compute time: $0.10 per hour for container execution
	hours := duration.Hours()
	if hours < 0.1 {
		hours = 0.1 // Minimum billing
	}
	return hours * 0.10
}

func calculateROI(totalCost float64, duration time.Duration) float64 {
	// Estimate ROI based on developer time saved
	// Assume manual migration would take 4 hours at $50/hour = $200
	manualCost := 200.0
	if totalCost > 0 {
		return ((manualCost - totalCost) / totalCost) * 100
	}
	return 0.0
}

func identifySuccessFactors(status *TransformationStatus, totalAttempts int, successRate float64) []string {
	var factors []string

	// Check for immediate success
	if totalAttempts == 0 && status.Status == "completed" {
		factors = append(factors, "Clean transformation requiring no healing attempts")
		factors = append(factors, "Well-structured codebase with compatible patterns")
		factors = append(factors, "Appropriate recipe selection for target transformation")
	}

	// Check for high success rate
	if successRate >= 0.8 {
		factors = append(factors, "High healing success rate indicates robust error recovery")
	}

	// Check for fast execution
	var duration time.Duration
	if !status.EndTime.IsZero() {
		duration = status.EndTime.Sub(status.StartTime)
	} else {
		duration = time.Since(status.StartTime)
	}

	if duration < 1*time.Minute {
		factors = append(factors, "Fast execution time indicates efficient processing")
	} else if duration < 5*time.Minute {
		factors = append(factors, "Reasonable execution time for transformation complexity")
	}

	return factors
}

func analyzeErrorPatterns(attempts []HealingAttempt) []HealingErrorPattern {
	errorCounts := make(map[string]*HealingErrorPattern)

	var analyze func([]HealingAttempt)
	analyze = func(attempts []HealingAttempt) {
		for _, attempt := range attempts {
			if attempt.TriggerReason != "" {
				pattern, exists := errorCounts[attempt.TriggerReason]
				if !exists {
					pattern = &HealingErrorPattern{
						ErrorType:    attempt.TriggerReason,
						AttemptPaths: []string{},
					}
					errorCounts[attempt.TriggerReason] = pattern
				}
				pattern.Frequency++
				pattern.AttemptPaths = append(pattern.AttemptPaths, attempt.AttemptPath)

				if attempt.Result == "success" {
					pattern.SuccessRate = (pattern.SuccessRate*float64(pattern.Frequency-1) + 1) / float64(pattern.Frequency)
				} else {
					pattern.SuccessRate = (pattern.SuccessRate * float64(pattern.Frequency-1)) / float64(pattern.Frequency)
				}
			}
			analyze(attempt.Children)
		}
	}

	analyze(attempts)

	// Convert map to slice
	var patterns []HealingErrorPattern
	for _, pattern := range errorCounts {
		patterns = append(patterns, *pattern)
	}

	// Sort by frequency
	sort.Slice(patterns, func(i, j int) bool {
		return patterns[i].Frequency > patterns[j].Frequency
	})

	return patterns
}

func analyzePerformance(attempts []HealingAttempt) PerformanceAnalysis {
	var durations []time.Duration

	var collect func([]HealingAttempt)
	collect = func(attempts []HealingAttempt) {
		for _, attempt := range attempts {
			if !attempt.EndTime.IsZero() {
				duration := attempt.EndTime.Sub(attempt.StartTime)
				durations = append(durations, duration)
			}
			collect(attempt.Children)
		}
	}

	collect(attempts)

	perf := PerformanceAnalysis{
		DepthPerformance: make(map[int]time.Duration),
	}

	if len(durations) > 0 {
		// Calculate average
		var total time.Duration
		for _, d := range durations {
			total += d
		}
		perf.AverageAttemptDuration = total / time.Duration(len(durations))

		// Calculate median
		sort.Slice(durations, func(i, j int) bool {
			return durations[i] < durations[j]
		})
		perf.MedianAttemptDuration = durations[len(durations)/2]

		// Calculate P95
		p95Index := int(float64(len(durations)) * 0.95)
		if p95Index < len(durations) {
			perf.P95AttemptDuration = durations[p95Index]
		}
	}

	return perf
}

func generateRecommendations(analysis *TransformationAnalysis) []string {
	var recommendations []string

	// Handle successful transformations with no healing attempts
	if analysis.Summary.TotalAttempts == 0 && analysis.Summary.Status == "completed" {
		recommendations = append(recommendations, "Transformation performing within normal parameters.")
		recommendations = append(recommendations, "Excellent success rate with no healing required.")

		if analysis.Summary.TotalDurationHours < 0.5 {
			recommendations = append(recommendations, "Fast execution time - consider using this pattern as a template.")
		}

		if analysis.CostAnalysis.ROI > 50 {
			recommendations = append(recommendations, "High ROI transformation - significant cost savings achieved.")
		}

		return recommendations
	}

	// Check success rate for transformations with healing attempts
	if analysis.Summary.SuccessRate < 0.5 {
		recommendations = append(recommendations,
			"Low success rate detected. Consider reviewing healing strategies and error patterns.")
	} else if analysis.Summary.SuccessRate >= 0.8 {
		recommendations = append(recommendations,
			"High success rate achieved - healing strategies are working effectively.")
	}

	// Check for recurring errors
	if len(analysis.ErrorPatterns) > 0 && analysis.ErrorPatterns[0].Frequency > 3 {
		recommendations = append(recommendations,
			fmt.Sprintf("Frequent error pattern '%s' detected. Consider creating specific healing recipe.",
				analysis.ErrorPatterns[0].ErrorType))
	}

	// Check cost efficiency
	if analysis.CostAnalysis.CostPerSuccess > 10.0 {
		recommendations = append(recommendations,
			"High cost per successful heal. Consider optimizing LLM usage or healing strategies.")
	} else if analysis.CostAnalysis.CostPerSuccess < 1.0 {
		recommendations = append(recommendations,
			"Very cost-effective transformation. Current approach is optimal.")
	}

	// Check performance
	if analysis.PerformanceMetrics.P95AttemptDuration > 30*time.Minute {
		recommendations = append(recommendations,
			"Long-running healing attempts detected. Consider adding timeouts or parallel processing.")
	}

	// Check transformation duration
	if analysis.Summary.TotalDurationHours > 2.0 {
		recommendations = append(recommendations,
			"Long transformation duration. Consider breaking into smaller chunks or optimizing recipe selection.")
	}

	// Check healing depth
	if analysis.Summary.MaxDepthReached > 5 {
		recommendations = append(recommendations,
			"Deep healing tree detected. Consider reviewing initial recipe selection to reduce complexity.")
	}

	// ROI recommendations
	if analysis.CostAnalysis.ROI > 100 {
		recommendations = append(recommendations,
			"Excellent ROI achieved. This transformation approach is highly cost-effective.")
	} else if analysis.CostAnalysis.ROI < 0 {
		recommendations = append(recommendations,
			"Negative ROI detected. Consider optimizing costs or reviewing transformation necessity.")
	}

	if len(recommendations) == 0 {
		recommendations = append(recommendations, "Transformation performing within normal parameters.")
	}

	return recommendations
}

func generateCSVFromHierarchy(viz *HierarchyVisualization) string {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)

	// Write headers
	writer.Write([]string{"Path", "Status", "Result", "Trigger", "Duration", "Depth"})

	// Write nodes
	var writeNodeCSV func(*HierarchyNode)
	writeNodeCSV = func(node *HierarchyNode) {
		writer.Write([]string{
			node.AttemptPath,
			node.Status,
			node.Result,
			node.TriggerReason,
			node.Duration,
			fmt.Sprintf("%d", node.Depth),
		})

		for _, child := range node.Children {
			writeNodeCSV(child)
		}
	}

	writeNodeCSV(viz.RootNode)
	writer.Flush()

	return buf.String()
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.0fs", d.Seconds())
	} else if d < time.Hour {
		return fmt.Sprintf("%.0fm", d.Minutes())
	} else {
		return fmt.Sprintf("%.1fh", d.Hours())
	}
}

// generateMarkdownReport creates a comprehensive markdown report
func generateMarkdownReport(status *TransformationStatus, diff string) string {
	var report strings.Builder

	// Header
	report.WriteString(fmt.Sprintf("# Transformation Report: %s\n", status.TransformationID))
	report.WriteString(fmt.Sprintf("Generated: %s\n\n", time.Now().Format("2006-01-02 15:04:05")))

	// Summary section
	report.WriteString("## 📊 Summary\n")
	report.WriteString(fmt.Sprintf("- **Status**: %s\n", status.Status))

	if !status.StartTime.IsZero() && !status.EndTime.IsZero() {
		duration := status.EndTime.Sub(status.StartTime)
		report.WriteString(fmt.Sprintf("- **Duration**: %s\n", formatDuration(duration)))
	}

	report.WriteString(fmt.Sprintf("- **Workflow Stage**: %s\n", status.WorkflowStage))
	report.WriteString(fmt.Sprintf("- **Healing Attempts**: %d\n", len(status.Children)))
	report.WriteString("\n")

	// Timeline section
	report.WriteString("## ⏱️ Timeline\n")
	timeline := buildTimeline(status)
	report.WriteString(formatTimelineMarkdown(timeline))
	report.WriteString("\n")

	// Healing Attempts section
	if len(status.Children) > 0 {
		report.WriteString("## 🔄 Healing Attempts\n")
		report.WriteString(formatHealingAttemptsMarkdown(status.Children))
		report.WriteString("\n")
	}

	// Code Changes section
	report.WriteString("## 📝 Code Changes\n")
	report.WriteString(formatCodeChangesMarkdown(status.Children, diff))
	report.WriteString("\n")

	// Cost Analysis section
	if status.CoordinatorMetrics != nil {
		report.WriteString("## 💰 Cost Analysis\n")
		report.WriteString(formatCostAnalysisMarkdown(status.CoordinatorMetrics))
		report.WriteString("\n")
	}

	return report.String()
}

// formatTimelineMarkdown formats timeline data as markdown
func formatTimelineMarkdown(timeline *TransformationTimeline) string {
	var md strings.Builder

	md.WriteString("### Step-by-Step Execution\n")

	for _, entry := range timeline.Timeline {
		duration := ""
		if entry.Duration != nil && *entry.Duration > 0 {
			duration = fmt.Sprintf(" [%s]", formatDuration(*entry.Duration))
		}

		md.WriteString(fmt.Sprintf("- **%s**%s **%s** - %s\n",
			entry.Timestamp.Format("15:04:05"), duration, entry.EventType, entry.Status))

		if entry.Description != "" {
			md.WriteString(fmt.Sprintf("  - %s\n", entry.Description))
		}
	}

	return md.String()
}

// formatHealingAttemptsMarkdown formats healing attempts as markdown
func formatHealingAttemptsMarkdown(attempts []HealingAttempt) string {
	var md strings.Builder

	var formatAttempt func([]HealingAttempt, int)
	formatAttempt = func(attempts []HealingAttempt, depth int) {
		for _, attempt := range attempts {
			indent := strings.Repeat("  ", depth)

			md.WriteString(fmt.Sprintf("%s- **Path**: %s\n", indent, attempt.AttemptPath))
			md.WriteString(fmt.Sprintf("%s  - **Trigger**: %s\n", indent, attempt.TriggerReason))
			md.WriteString(fmt.Sprintf("%s  - **Status**: %s\n", indent, attempt.Status))

			if attempt.LLMAnalysis != nil {
				md.WriteString(fmt.Sprintf("%s  - **LLM Analysis**: %.0f%% confidence - %s\n",
					indent, attempt.LLMAnalysis.Confidence*100, attempt.LLMAnalysis.SuggestedFix))
			}

			if len(attempt.TargetErrors) > 0 {
				md.WriteString(fmt.Sprintf("%s  - **Target Errors**: %s\n", indent,
					strings.Join(attempt.TargetErrors, ", ")))
			}

			if len(attempt.Children) > 0 {
				formatAttempt(attempt.Children, depth+1)
			}

			md.WriteString("\n")
		}
	}

	formatAttempt(attempts, 0)
	return md.String()
}

// formatCodeChangesMarkdown formats code changes as markdown
func formatCodeChangesMarkdown(attempts []HealingAttempt, diff string) string {
	var md strings.Builder

	// If we have a diff from OpenRewrite, display it
	if diff != "" {
		md.WriteString("### Transformation Diff\n")
		md.WriteString("```diff\n")

		// Limit diff output to reasonable size for report
		lines := strings.Split(diff, "\n")
		maxLines := 100
		if len(lines) > maxLines {
			md.WriteString(strings.Join(lines[:maxLines], "\n"))
			md.WriteString("\n... (truncated, showing first 100 lines)\n")
		} else {
			md.WriteString(diff)
		}

		md.WriteString("\n```\n\n")

		// Extract file list from diff
		md.WriteString("### Files Modified\n")
		filesModified := extractFilesFromDiff(diff)
		if len(filesModified) > 0 {
			for _, file := range filesModified {
				md.WriteString(fmt.Sprintf("- %s\n", file))
			}
		} else {
			md.WriteString("- Changes detected in transformation\n")
		}
	} else {
		// Fall back to healing attempt changes if no diff
		md.WriteString("### Files Modified\n")

		var hasChanges bool
		var collectChanges func([]HealingAttempt)
		collectChanges = func(attempts []HealingAttempt) {
			for _, attempt := range attempts {
				if attempt.LLMAnalysis != nil && attempt.LLMAnalysis.SuggestedFix != "" {
					md.WriteString(fmt.Sprintf("- **Change**: %s\n", attempt.LLMAnalysis.SuggestedFix))
					hasChanges = true
				}
				collectChanges(attempt.Children)
			}
		}

		collectChanges(attempts)

		if !hasChanges {
			md.WriteString("- No detailed file changes recorded\n")
		}
	}

	return md.String()
}

// extractFilesFromDiff parses a unified diff to extract modified file names
func extractFilesFromDiff(diff string) []string {
	var files []string
	seenFiles := make(map[string]bool)

	lines := strings.Split(diff, "\n")
	for _, line := range lines {
		// Look for diff headers like "diff --git a/file.java b/file.java"
		if strings.HasPrefix(line, "diff --git") {
			parts := strings.Fields(line)
			if len(parts) >= 4 {
				// Extract filename from a/filename format
				file := strings.TrimPrefix(parts[2], "a/")
				if !seenFiles[file] {
					files = append(files, file)
					seenFiles[file] = true
				}
			}
		}
		// Also look for +++ and --- lines
		if strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---") {
			parts := strings.Fields(line)
			if len(parts) >= 2 && parts[1] != "/dev/null" {
				file := strings.TrimPrefix(parts[1], "b/")
				file = strings.TrimPrefix(file, "a/")
				if !seenFiles[file] && file != "" {
					files = append(files, file)
					seenFiles[file] = true
				}
			}
		}
	}

	return files
}

// formatCostAnalysisMarkdown formats cost analysis as markdown
func formatCostAnalysisMarkdown(metrics *HealingCoordinatorMetrics) string {
	var md strings.Builder

	md.WriteString(fmt.Sprintf("- **Total LLM calls**: %d\n", metrics.TotalLLMCalls))
	md.WriteString(fmt.Sprintf("- **Total tokens**: %d\n", metrics.TotalLLMTokens))
	md.WriteString(fmt.Sprintf("- **Estimated cost**: $%.2f\n", metrics.TotalLLMCost))

	if metrics.LLMCacheHitRate > 0 {
		md.WriteString(fmt.Sprintf("- **Cache hit rate**: %.1f%%\n", metrics.LLMCacheHitRate*100))
	}

	return md.String()
}
