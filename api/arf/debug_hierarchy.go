package arf

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"time"
)

// buildHierarchyVisualization creates a hierarchical view of the transformation and its healing attempts
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

// buildHierarchyNode creates a hierarchy node from transformation status
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

// buildHierarchyNodeFromAttempt creates a hierarchy node from a healing attempt
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

// calculateHierarchyMetrics calculates metrics for the hierarchy tree
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

// generateASCIITree generates an ASCII tree visualization of the hierarchy
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

// writeNode writes a single node and its children to the buffer
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

// generateCSVFromHierarchy generates CSV data from hierarchy visualization
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

// formatDuration formats a duration for display
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.0fs", d.Seconds())
	} else if d < time.Hour {
		return fmt.Sprintf("%.0fm", d.Minutes())
	} else {
		return fmt.Sprintf("%.1fh", d.Hours())
	}
}
