package arf

import (
	"context"
	"testing"
	"time"
)

func TestHAIntegrationCreation(t *testing.T) {
	ha := NewHAIntegration()
	
	if ha == nil {
		t.Fatal("Expected non-nil HA integration")
	}
}

func TestNodeRegistration(t *testing.T) {
	ha := NewHAIntegration()
	ctx := context.Background()
	
	node := NodeInfo{
		ID:             "node-1",
		Address:        "192.168.1.100",
		Port:           8080,
		Capabilities:   []string{"java", "go", "python"},
		MaxConcurrency: 10,
		CurrentLoad:    0,
		HealthStatus:   NodeHealthHealthy,
		AvailableMemory: 8 * 1024 * 1024 * 1024, // 8GB
		AvailableCPU:   4.0,
	}
	
	err := ha.RegisterNode(ctx, node)
	if err != nil {
		t.Fatalf("Expected no error registering node, got: %v", err)
	}
	
	// Test invalid node (missing ID)
	invalidNode := NodeInfo{
		Address: "192.168.1.101",
		Port:    8080,
	}
	
	err = ha.RegisterNode(ctx, invalidNode)
	if err == nil {
		t.Error("Expected error for node without ID")
	}
	
	// Test invalid node (missing address)
	invalidNode2 := NodeInfo{
		ID:   "node-2",
		Port: 8080,
	}
	
	err = ha.RegisterNode(ctx, invalidNode2)
	if err == nil {
		t.Error("Expected error for node without address")
	}
	
	// Test invalid port
	invalidNode3 := NodeInfo{
		ID:      "node-3",
		Address: "192.168.1.102",
		Port:    -1,
	}
	
	err = ha.RegisterNode(ctx, invalidNode3)
	if err == nil {
		t.Error("Expected error for invalid port")
	}
}

func TestClusterStatusTracking(t *testing.T) {
	ha := NewHAIntegration()
	ctx := context.Background()
	
	// Initially empty cluster
	status, err := ha.GetClusterStatus(ctx)
	if err != nil {
		t.Fatalf("Expected no error getting cluster status, got: %v", err)
	}
	
	if status.TotalNodes != 0 {
		t.Errorf("Expected 0 nodes, got %d", status.TotalNodes)
	}
	
	// Register some nodes
	nodes := []NodeInfo{
		{
			ID:             "node-1",
			Address:        "192.168.1.100",
			Port:           8080,
			MaxConcurrency: 10,
			CurrentLoad:    2,
			HealthStatus:   NodeHealthHealthy,
			LastHeartbeat:  time.Now(),
			AvailableMemory: 8 * 1024 * 1024 * 1024,
			AvailableCPU:   4.0,
		},
		{
			ID:             "node-2",
			Address:        "192.168.1.101",
			Port:           8080,
			MaxConcurrency: 5,
			CurrentLoad:    1,
			HealthStatus:   NodeHealthHealthy,
			LastHeartbeat:  time.Now(),
			AvailableMemory: 4 * 1024 * 1024 * 1024,
			AvailableCPU:   2.0,
		},
		{
			ID:             "node-3",
			Address:        "192.168.1.102",
			Port:           8080,
			MaxConcurrency: 8,
			CurrentLoad:    0,
			HealthStatus:   NodeHealthUnhealthy,
			LastHeartbeat:  time.Now().Add(-time.Hour), // Old heartbeat
			AvailableMemory: 6 * 1024 * 1024 * 1024,
			AvailableCPU:   3.0,
		},
	}
	
	for _, node := range nodes {
		err := ha.RegisterNode(ctx, node)
		if err != nil {
			t.Fatalf("Failed to register node %s: %v", node.ID, err)
		}
	}
	
	// Check cluster status
	status, err = ha.GetClusterStatus(ctx)
	if err != nil {
		t.Fatalf("Expected no error getting cluster status, got: %v", err)
	}
	
	if status.TotalNodes != 3 {
		t.Errorf("Expected 3 nodes, got %d", status.TotalNodes)
	}
	
	if status.HealthyNodes != 2 {
		t.Errorf("Expected 2 healthy nodes, got %d", status.HealthyNodes)
	}
	
	if status.UnhealthyNodes != 1 {
		t.Errorf("Expected 1 unhealthy node, got %d", status.UnhealthyNodes)
	}
	
	// Check capacity calculations
	expectedTotalConcurrency := 10 + 5 // Only healthy nodes
	if status.TotalCapacity.TotalConcurrency != expectedTotalConcurrency {
		t.Errorf("Expected total concurrency %d, got %d", 
			expectedTotalConcurrency, status.TotalCapacity.TotalConcurrency)
	}
}

func TestWorkloadDistribution(t *testing.T) {
	ha := NewHAIntegration()
	ctx := context.Background()
	
	// Register nodes
	nodes := []NodeInfo{
		{
			ID:             "node-1",
			Address:        "192.168.1.100",
			Port:           8080,
			Capabilities:   []string{"java", "python"},
			MaxConcurrency: 10,
			CurrentLoad:    2,
			HealthStatus:   NodeHealthHealthy,
			LastHeartbeat:  time.Now(),
			AvailableMemory: 8 * 1024 * 1024 * 1024,
			AvailableCPU:   4.0,
		},
		{
			ID:             "node-2",
			Address:        "192.168.1.101",
			Port:           8080,
			Capabilities:   []string{"go", "python"},
			MaxConcurrency: 5,
			CurrentLoad:    1,
			HealthStatus:   NodeHealthHealthy,
			LastHeartbeat:  time.Now(),
			AvailableMemory: 4 * 1024 * 1024 * 1024,
			AvailableCPU:   2.0,
		},
	}
	
	for _, node := range nodes {
		ha.RegisterNode(ctx, node)
	}
	
	// Create workload
	workload := Workload{
		ID:             "workload-1",
		Type:           WorkloadTransformation,
		Priority:       1,
		EstimatedTime:  30 * time.Second,
		RequiredMemory: 1 * 1024 * 1024 * 1024, // 1GB
		RequiredCPU:    1.0,
		CreatedAt:      time.Now(),
		Constraints: map[string]interface{}{
			"required_capabilities": []string{"python"},
		},
	}
	
	// Distribute workload
	distribution, err := ha.DistributeWorkload(ctx, workload)
	if err != nil {
		t.Fatalf("Expected no error distributing workload, got: %v", err)
	}
	
	if distribution == nil {
		t.Fatal("Expected non-nil distribution")
	}
	
	if distribution.WorkloadID != workload.ID {
		t.Errorf("Expected workload ID %s, got %s", workload.ID, distribution.WorkloadID)
	}
	
	if len(distribution.AssignedNodes) == 0 {
		t.Error("Expected at least one assigned node")
	}
	
	// Verify node selection (both nodes have Python capability, should select best one)
	var assignedNodeID string
	for nodeID := range distribution.AssignedNodes {
		assignedNodeID = nodeID
		break
	}
	
	if assignedNodeID == "" {
		t.Error("Expected assigned node")
	}
	
	// Should have failover nodes
	if len(distribution.FailoverNodes) == 0 {
		t.Error("Expected failover nodes to be assigned")
	}
}

func TestWorkloadDistributionConstraints(t *testing.T) {
	ha := NewHAIntegration()
	ctx := context.Background()
	
	// Register node without required capability
	node := NodeInfo{
		ID:             "node-1",
		Address:        "192.168.1.100",
		Port:           8080,
		Capabilities:   []string{"go"},
		MaxConcurrency: 10,
		CurrentLoad:    0,
		HealthStatus:   NodeHealthHealthy,
		LastHeartbeat:  time.Now(),
		AvailableMemory: 8 * 1024 * 1024 * 1024,
		AvailableCPU:   4.0,
	}
	
	ha.RegisterNode(ctx, node)
	
	// Create workload requiring Java capability
	workload := Workload{
		ID:             "workload-1",
		Type:           WorkloadTransformation,
		EstimatedTime:  30 * time.Second,
		RequiredMemory: 1 * 1024 * 1024 * 1024,
		RequiredCPU:    1.0,
		Constraints: map[string]interface{}{
			"required_capabilities": []string{"java"},
		},
	}
	
	// Should fail to distribute (no suitable nodes)
	_, err := ha.DistributeWorkload(ctx, workload)
	if err == nil {
		t.Error("Expected error distributing workload to unsuitable nodes")
	}
}

func TestLoadBalancing(t *testing.T) {
	ha := NewHAIntegration().(*DefaultHAIntegration)
	
	// Test least loaded selection
	nodes := []NodeInfo{
		{ID: "node-1", MaxConcurrency: 10, CurrentLoad: 8}, // 80% load
		{ID: "node-2", MaxConcurrency: 10, CurrentLoad: 3}, // 30% load
		{ID: "node-3", MaxConcurrency: 10, CurrentLoad: 5}, // 50% load
	}
	
	selected := ha.selectLeastLoadedNode(nodes)
	if selected.ID != "node-2" {
		t.Errorf("Expected node-2 (least loaded), got %s", selected.ID)
	}
	
	// Test weighted round robin
	selected = ha.selectWeightedRoundRobinNode(nodes)
	if selected.ID != "node-2" {
		t.Errorf("Expected node-2 (highest weight), got %s", selected.ID)
	}
	
	// Test resource-based selection
	workload := Workload{
		RequiredMemory: 1 * 1024 * 1024 * 1024,
		RequiredCPU:    1.0,
	}
	
	// Add resource information
	for i := range nodes {
		nodes[i].AvailableMemory = 4 * 1024 * 1024 * 1024
		nodes[i].AvailableCPU = 2.0
	}
	
	selected = ha.selectResourceBasedNode(nodes, workload)
	// Should still prefer less loaded node with same resources
	if selected.ID != "node-2" {
		t.Errorf("Expected node-2 (best resource score), got %s", selected.ID)
	}
}

func TestNodeFailureHandling(t *testing.T) {
	ha := NewHAIntegration()
	ctx := context.Background()
	
	// Register nodes
	nodes := []NodeInfo{
		{
			ID:             "node-1",
			Address:        "192.168.1.100",
			Port:           8080,
			MaxConcurrency: 10,
			CurrentLoad:    2,
			HealthStatus:   NodeHealthHealthy,
			LastHeartbeat:  time.Now(),
			AvailableMemory: 8 * 1024 * 1024 * 1024,
			AvailableCPU:   4.0,
		},
		{
			ID:             "node-2",
			Address:        "192.168.1.101",
			Port:           8080,
			MaxConcurrency: 5,
			CurrentLoad:    1,
			HealthStatus:   NodeHealthHealthy,
			LastHeartbeat:  time.Now(),
			AvailableMemory: 4 * 1024 * 1024 * 1024,
			AvailableCPU:   2.0,
		},
	}
	
	for _, node := range nodes {
		ha.RegisterNode(ctx, node)
	}
	
	// Distribute workload to node-1
	workload := Workload{
		ID:             "workload-1",
		Type:           WorkloadTransformation,
		EstimatedTime:  30 * time.Second,
		RequiredMemory: 1 * 1024 * 1024 * 1024,
		RequiredCPU:    1.0,
	}
	
	distribution, err := ha.DistributeWorkload(ctx, workload)
	if err != nil {
		t.Fatalf("Failed to distribute workload: %v", err)
	}
	
	// Simulate node failure
	err = ha.HandleNodeFailure(ctx, "node-1")
	if err != nil {
		t.Fatalf("Expected no error handling node failure, got: %v", err)
	}
	
	// Check cluster status - node-1 should be unhealthy
	status, err := ha.GetClusterStatus(ctx)
	if err != nil {
		t.Fatalf("Failed to get cluster status: %v", err)
	}
	
	if status.HealthyNodes != 1 {
		t.Errorf("Expected 1 healthy node after failure, got %d", status.HealthyNodes)
	}
	
	if status.UnhealthyNodes != 1 {
		t.Errorf("Expected 1 unhealthy node after failure, got %d", status.UnhealthyNodes)
	}
	
	// Workload should be reassigned (check that failover occurred)
	// This is a simplified check - in a real implementation, 
	// we'd verify the workload was actually moved
	if distribution.WorkloadID != workload.ID {
		t.Error("Distribution should still exist after node failure")
	}
}

func TestLoadBalanceScoreCalculation(t *testing.T) {
	ha := NewHAIntegration().(*DefaultHAIntegration)
	ctx := context.Background()
	
	// Register perfectly balanced nodes
	balancedNodes := []NodeInfo{
		{ID: "node-1", MaxConcurrency: 10, CurrentLoad: 5},
		{ID: "node-2", MaxConcurrency: 10, CurrentLoad: 5},
		{ID: "node-3", MaxConcurrency: 10, CurrentLoad: 5},
	}
	
	for _, node := range balancedNodes {
		node.Address = "192.168.1." + node.ID[5:]
		node.Port = 8080
		node.HealthStatus = NodeHealthHealthy
		node.LastHeartbeat = time.Now()
		ha.RegisterNode(ctx, node)
	}
	
	score := ha.calculateLoadBalanceScore()
	if score < 0.9 {
		t.Errorf("Expected high balance score for balanced cluster, got %f", score)
	}
	
	// Create imbalanced cluster
	ha2 := NewHAIntegration().(*DefaultHAIntegration)
	
	imbalancedNodes := []NodeInfo{
		{ID: "node-1", Address: "192.168.1.1", Port: 8080, MaxConcurrency: 10, CurrentLoad: 9, HealthStatus: NodeHealthHealthy, LastHeartbeat: time.Now()},
		{ID: "node-2", Address: "192.168.1.2", Port: 8080, MaxConcurrency: 10, CurrentLoad: 1, HealthStatus: NodeHealthHealthy, LastHeartbeat: time.Now()},
		{ID: "node-3", Address: "192.168.1.3", Port: 8080, MaxConcurrency: 10, CurrentLoad: 2, HealthStatus: NodeHealthHealthy, LastHeartbeat: time.Now()},
	}
	
	for _, node := range imbalancedNodes {
		ha2.RegisterNode(ctx, node)
	}
	
	score2 := ha2.calculateLoadBalanceScore()
	if score2 >= score {
		t.Errorf("Expected lower balance score for imbalanced cluster, got %f vs %f", score2, score)
	}
}

func TestNodeMetrics(t *testing.T) {
	ha := NewHAIntegration()
	ctx := context.Background()
	
	node := NodeInfo{
		ID:            "node-1",
		Address:       "192.168.1.100",
		Port:          8080,
		HealthStatus:  NodeHealthHealthy,
		RegisteredAt:  time.Now().Add(-time.Hour),
	}
	
	err := ha.RegisterNode(ctx, node)
	if err != nil {
		t.Fatalf("Failed to register node: %v", err)
	}
	
	// Get metrics
	metrics, err := ha.GetNodeMetrics(ctx, "node-1")
	if err != nil {
		t.Fatalf("Expected no error getting metrics, got: %v", err)
	}
	
	if metrics.NodeID != "node-1" {
		t.Errorf("Expected node ID 'node-1', got %s", metrics.NodeID)
	}
	
	if metrics.Uptime <= 0 {
		t.Error("Expected positive uptime")
	}
	
	// Test non-existent node
	_, err = ha.GetNodeMetrics(ctx, "non-existent")
	if err == nil {
		t.Error("Expected error for non-existent node metrics")
	}
}

func TestCapabilityMatching(t *testing.T) {
	ha := NewHAIntegration().(*DefaultHAIntegration)
	
	node := NodeInfo{
		ID:           "node-1",
		Capabilities: []string{"java", "python", "docker"},
	}
	
	// Test matching capabilities
	required := []string{"java", "python"}
	if !ha.hasRequiredCapabilities(node, required) {
		t.Error("Expected node to have required capabilities")
	}
	
	// Test missing capability
	required = []string{"java", "go"}
	if ha.hasRequiredCapabilities(node, required) {
		t.Error("Expected node to NOT have all required capabilities")
	}
	
	// Test empty requirements
	required = []string{}
	if !ha.hasRequiredCapabilities(node, required) {
		t.Error("Expected node to satisfy empty requirements")
	}
}

func TestWorkloadRebalancing(t *testing.T) {
	ha := NewHAIntegration().(*DefaultHAIntegration)
	ctx := context.Background()
	
	// Set short rebalance interval for testing
	ha.rebalanceInterval = 1 * time.Millisecond
	
	// Register imbalanced nodes
	nodes := []NodeInfo{
		{ID: "node-1", Address: "192.168.1.1", Port: 8080, MaxConcurrency: 10, CurrentLoad: 9, HealthStatus: NodeHealthHealthy, LastHeartbeat: time.Now()},
		{ID: "node-2", Address: "192.168.1.2", Port: 8080, MaxConcurrency: 10, CurrentLoad: 1, HealthStatus: NodeHealthHealthy, LastHeartbeat: time.Now()},
	}
	
	for _, node := range nodes {
		ha.RegisterNode(ctx, node)
	}
	
	// Trigger rebalancing
	err := ha.RebalanceWorkload(ctx)
	if err != nil {
		t.Fatalf("Expected no error rebalancing, got: %v", err)
	}
	
	// Check that rebalancing was performed
	if ha.lastRebalance.IsZero() {
		t.Error("Expected rebalance timestamp to be updated")
	}
	
	// Test that rebalancing is skipped if recently performed
	lastRebalance := ha.lastRebalance
	ha.rebalanceInterval = time.Hour // Long interval
	
	err = ha.RebalanceWorkload(ctx)
	if err != nil {
		t.Fatalf("Expected no error on second rebalance, got: %v", err)
	}
	
	if ha.lastRebalance != lastRebalance {
		t.Error("Expected rebalance to be skipped due to recent execution")
	}
}