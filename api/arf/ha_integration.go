package arf

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// HAIntegration provides high availability features for distributed ARF workloads
type HAIntegration interface {
	RegisterNode(ctx context.Context, node NodeInfo) error
	GetClusterStatus(ctx context.Context) (*ClusterStatus, error)
	DistributeWorkload(ctx context.Context, workload Workload) (*WorkloadDistribution, error)
	HandleNodeFailure(ctx context.Context, nodeID string) error
	RebalanceWorkload(ctx context.Context) error
	GetNodeMetrics(ctx context.Context, nodeID string) (*NodeMetrics, error)
}

// NodeInfo represents information about a cluster node
type NodeInfo struct {
	ID               string            `json:"id"`
	Address          string            `json:"address"`
	Port             int               `json:"port"`
	Capabilities     []string          `json:"capabilities"`
	MaxConcurrency   int               `json:"max_concurrency"`
	CurrentLoad      int               `json:"current_load"`
	HealthStatus     NodeHealthStatus  `json:"health_status"`
	LastHeartbeat    time.Time         `json:"last_heartbeat"`
	Version          string            `json:"version"`
	Metadata         map[string]string `json:"metadata"`
	RegisteredAt     time.Time         `json:"registered_at"`
	AvailableMemory  int64             `json:"available_memory"`
	AvailableCPU     float64           `json:"available_cpu"`
}

// NodeHealthStatus represents the health status of a node
type NodeHealthStatus string

const (
	NodeHealthHealthy     NodeHealthStatus = "healthy"
	NodeHealthDegraded    NodeHealthStatus = "degraded"
	NodeHealthUnhealthy   NodeHealthStatus = "unhealthy"
	NodeHealthMaintenance NodeHealthStatus = "maintenance"
	NodeHealthUnknown     NodeHealthStatus = "unknown"
)

// ClusterStatus provides an overview of the entire cluster
type ClusterStatus struct {
	TotalNodes       int                     `json:"total_nodes"`
	HealthyNodes     int                     `json:"healthy_nodes"`
	UnhealthyNodes   int                     `json:"unhealthy_nodes"`
	TotalCapacity    ClusterCapacity         `json:"total_capacity"`
	UsedCapacity     ClusterCapacity         `json:"used_capacity"`
	ActiveWorkloads  int                     `json:"active_workloads"`
	QueuedWorkloads  int                     `json:"queued_workloads"`
	LastUpdate       time.Time               `json:"last_update"`
	Nodes            map[string]NodeInfo     `json:"nodes"`
	LoadBalancer     LoadBalancerStatus      `json:"load_balancer"`
}

// ClusterCapacity represents the computational capacity of the cluster
type ClusterCapacity struct {
	TotalConcurrency int     `json:"total_concurrency"`
	AvailableMemory  int64   `json:"available_memory"`
	AvailableCPU     float64 `json:"available_cpu"`
	NetworkBandwidth int64   `json:"network_bandwidth"`
}

// LoadBalancerStatus represents the status of the load balancer
type LoadBalancerStatus struct {
	Algorithm         string            `json:"algorithm"`
	RequestsProcessed int64             `json:"requests_processed"`
	FailedRequests    int64             `json:"failed_requests"`
	AverageLatency    time.Duration     `json:"average_latency"`
	LastRebalance     time.Time         `json:"last_rebalance"`
}

// Workload represents a unit of work to be distributed
type Workload struct {
	ID               string                 `json:"id"`
	Type             WorkloadType           `json:"type"`
	Priority         int                    `json:"priority"`
	EstimatedTime    time.Duration          `json:"estimated_time"`
	RequiredMemory   int64                  `json:"required_memory"`
	RequiredCPU      float64                `json:"required_cpu"`
	Dependencies     []string               `json:"dependencies"`
	Constraints      map[string]interface{} `json:"constraints"`
	Payload          interface{}            `json:"payload"`
	CreatedAt        time.Time              `json:"created_at"`
	DeadlineAt       *time.Time             `json:"deadline_at,omitempty"`
}

// WorkloadType categorizes different types of workloads
type WorkloadType string

const (
	WorkloadTransformation WorkloadType = "transformation"
	WorkloadAnalysis       WorkloadType = "analysis"
	WorkloadValidation     WorkloadType = "validation"
	WorkloadOrchestration  WorkloadType = "orchestration"
	WorkloadEvolution      WorkloadType = "evolution"
)

// WorkloadDistribution represents how work is distributed across nodes
type WorkloadDistribution struct {
	WorkloadID       string                    `json:"workload_id"`
	AssignedNodes    map[string]WorkloadTask   `json:"assigned_nodes"`
	DistributionTime time.Time                 `json:"distribution_time"`
	EstimatedCompletionTime time.Time          `json:"estimated_completion_time"`
	LoadBalanceScore float64                   `json:"load_balance_score"`
	FailoverNodes    []string                  `json:"failover_nodes"`
}

// WorkloadTask represents a specific task assigned to a node
type WorkloadTask struct {
	NodeID        string        `json:"node_id"`
	TaskID        string        `json:"task_id"`
	StartTime     time.Time     `json:"start_time"`
	EstimatedTime time.Duration `json:"estimated_time"`
	Status        TaskStatus    `json:"status"`
	Progress      float64       `json:"progress"`
	Retries       int           `json:"retries"`
}

// TaskStatus represents the status of a task
type TaskStatus string

const (
	TaskStatusPending    TaskStatus = "pending"
	TaskStatusRunning    TaskStatus = "running"
	TaskStatusCompleted  TaskStatus = "completed"
	TaskStatusFailed     TaskStatus = "failed"
	TaskStatusRetrying   TaskStatus = "retrying"
	TaskStatusCancelled  TaskStatus = "cancelled"
)

// NodeMetrics provides detailed metrics for a specific node
type NodeMetrics struct {
	NodeID              string        `json:"node_id"`
	CPUUsage            float64       `json:"cpu_usage"`
	MemoryUsage         int64         `json:"memory_usage"`
	NetworkThroughput   int64         `json:"network_throughput"`
	TasksCompleted      int64         `json:"tasks_completed"`
	TasksFailed         int64         `json:"tasks_failed"`
	AverageTaskTime     time.Duration `json:"average_task_time"`
	LastTaskCompletion  time.Time     `json:"last_task_completion"`
	Uptime              time.Duration `json:"uptime"`
	ErrorRate           float64       `json:"error_rate"`
}

// DefaultHAIntegration implements the HAIntegration interface
type DefaultHAIntegration struct {
	nodes         map[string]NodeInfo
	workloads     map[string]Workload
	distributions map[string]WorkloadDistribution
	metrics       map[string]NodeMetrics
	mutex         sync.RWMutex
	
	// Configuration
	heartbeatTimeout    time.Duration
	rebalanceInterval   time.Duration
	maxRetries          int
	loadBalanceAlgorithm string
	
	// Internal state
	lastRebalance time.Time
	nodeFailures  map[string]int
}

// NewHAIntegration creates a new high availability integration
func NewHAIntegration() HAIntegration {
	return &DefaultHAIntegration{
		nodes:             make(map[string]NodeInfo),
		workloads:         make(map[string]Workload),
		distributions:     make(map[string]WorkloadDistribution),
		metrics:           make(map[string]NodeMetrics),
		heartbeatTimeout:  30 * time.Second,
		rebalanceInterval: 5 * time.Minute,
		maxRetries:        3,
		loadBalanceAlgorithm: "weighted_round_robin",
		nodeFailures:      make(map[string]int),
	}
}

// RegisterNode registers a new node in the cluster
func (ha *DefaultHAIntegration) RegisterNode(ctx context.Context, node NodeInfo) error {
	ha.mutex.Lock()
	defer ha.mutex.Unlock()
	
	// Validate node information
	if node.ID == "" {
		return fmt.Errorf("node ID is required")
	}
	
	if node.Address == "" {
		return fmt.Errorf("node address is required")
	}
	
	if node.Port <= 0 || node.Port > 65535 {
		return fmt.Errorf("invalid port number: %d", node.Port)
	}
	
	// Set defaults
	if node.RegisteredAt.IsZero() {
		node.RegisteredAt = time.Now()
	}
	
	if node.LastHeartbeat.IsZero() {
		node.LastHeartbeat = time.Now()
	}
	
	if node.HealthStatus == "" {
		node.HealthStatus = NodeHealthHealthy
	}
	
	// Register the node
	ha.nodes[node.ID] = node
	
	// Initialize metrics for new node
	ha.metrics[node.ID] = NodeMetrics{
		NodeID:            node.ID,
		CPUUsage:          0.0,
		MemoryUsage:       0,
		NetworkThroughput: 0,
		TasksCompleted:    0,
		TasksFailed:       0,
		Uptime:            time.Since(node.RegisteredAt),
		ErrorRate:         0.0,
	}
	
	// Reset failure count
	ha.nodeFailures[node.ID] = 0
	
	return nil
}

// GetClusterStatus returns the current status of the cluster
func (ha *DefaultHAIntegration) GetClusterStatus(ctx context.Context) (*ClusterStatus, error) {
	ha.mutex.RLock()
	defer ha.mutex.RUnlock()
	
	status := &ClusterStatus{
		TotalNodes:      len(ha.nodes),
		HealthyNodes:    0,
		UnhealthyNodes:  0,
		ActiveWorkloads: 0,
		QueuedWorkloads: 0,
		LastUpdate:      time.Now(),
		Nodes:          make(map[string]NodeInfo),
		LoadBalancer: LoadBalancerStatus{
			Algorithm:      ha.loadBalanceAlgorithm,
			LastRebalance:  ha.lastRebalance,
		},
	}
	
	// Calculate cluster capacity and health
	totalCapacity := ClusterCapacity{}
	usedCapacity := ClusterCapacity{}
	
	for nodeID, node := range ha.nodes {
		status.Nodes[nodeID] = node
		
		// Check if node is healthy (recent heartbeat)
		if time.Since(node.LastHeartbeat) <= ha.heartbeatTimeout {
			if node.HealthStatus == NodeHealthHealthy || node.HealthStatus == NodeHealthDegraded {
				status.HealthyNodes++
				
				// Add to cluster capacity
				totalCapacity.TotalConcurrency += node.MaxConcurrency
				totalCapacity.AvailableMemory += node.AvailableMemory
				totalCapacity.AvailableCPU += node.AvailableCPU
				
				// Add current usage
				usedCapacity.TotalConcurrency += node.CurrentLoad
				if metrics, exists := ha.metrics[nodeID]; exists {
					usedCapacity.AvailableMemory += metrics.MemoryUsage
					usedCapacity.AvailableCPU += metrics.CPUUsage
				}
			} else {
				status.UnhealthyNodes++
			}
		} else {
			status.UnhealthyNodes++
		}
	}
	
	// Count active and queued workloads
	for _, workload := range ha.workloads {
		if distribution, exists := ha.distributions[workload.ID]; exists {
			hasRunningTask := false
			for _, task := range distribution.AssignedNodes {
				if task.Status == TaskStatusRunning {
					hasRunningTask = true
					break
				}
			}
			if hasRunningTask {
				status.ActiveWorkloads++
			} else {
				status.QueuedWorkloads++
			}
		} else {
			status.QueuedWorkloads++
		}
	}
	
	status.TotalCapacity = totalCapacity
	status.UsedCapacity = usedCapacity
	
	return status, nil
}

// DistributeWorkload distributes a workload across available nodes
func (ha *DefaultHAIntegration) DistributeWorkload(ctx context.Context, workload Workload) (*WorkloadDistribution, error) {
	ha.mutex.Lock()
	defer ha.mutex.Unlock()
	
	// Store the workload
	ha.workloads[workload.ID] = workload
	
	// Find suitable nodes
	suitableNodes := ha.findSuitableNodes(workload)
	if len(suitableNodes) == 0 {
		return nil, fmt.Errorf("no suitable nodes available for workload %s", workload.ID)
	}
	
	// Create distribution plan
	distribution := WorkloadDistribution{
		WorkloadID:       workload.ID,
		AssignedNodes:    make(map[string]WorkloadTask),
		DistributionTime: time.Now(),
		FailoverNodes:    make([]string, 0),
	}
	
	// Select the best node based on load balancing algorithm
	selectedNode := ha.selectBestNode(suitableNodes, workload)
	
	// Create task for selected node
	task := WorkloadTask{
		NodeID:        selectedNode.ID,
		TaskID:        fmt.Sprintf("%s-task-1", workload.ID),
		StartTime:     time.Now(),
		EstimatedTime: workload.EstimatedTime,
		Status:        TaskStatusPending,
		Progress:      0.0,
		Retries:       0,
	}
	
	distribution.AssignedNodes[selectedNode.ID] = task
	distribution.EstimatedCompletionTime = time.Now().Add(workload.EstimatedTime)
	
	// Select failover nodes
	for _, node := range suitableNodes {
		if node.ID != selectedNode.ID && len(distribution.FailoverNodes) < 2 {
			distribution.FailoverNodes = append(distribution.FailoverNodes, node.ID)
		}
	}
	
	// Calculate load balance score
	distribution.LoadBalanceScore = ha.calculateLoadBalanceScore()
	
	// Store distribution
	ha.distributions[workload.ID] = distribution
	
	return &distribution, nil
}

// findSuitableNodes finds nodes that can handle the given workload
func (ha *DefaultHAIntegration) findSuitableNodes(workload Workload) []NodeInfo {
	suitable := make([]NodeInfo, 0)
	
	for _, node := range ha.nodes {
		// Check if node is healthy
		if node.HealthStatus != NodeHealthHealthy && node.HealthStatus != NodeHealthDegraded {
			continue
		}
		
		// Check heartbeat
		if time.Since(node.LastHeartbeat) > ha.heartbeatTimeout {
			continue
		}
		
		// Check capacity constraints
		if node.CurrentLoad >= node.MaxConcurrency {
			continue
		}
		
		if node.AvailableMemory < workload.RequiredMemory {
			continue
		}
		
		if node.AvailableCPU < workload.RequiredCPU {
			continue
		}
		
		// Check capabilities if specified
		if capabilities, ok := workload.Constraints["required_capabilities"].([]string); ok {
			if !ha.hasRequiredCapabilities(node, capabilities) {
				continue
			}
		}
		
		suitable = append(suitable, node)
	}
	
	return suitable
}

// hasRequiredCapabilities checks if a node has all required capabilities
func (ha *DefaultHAIntegration) hasRequiredCapabilities(node NodeInfo, required []string) bool {
	nodeCapabilities := make(map[string]bool)
	for _, cap := range node.Capabilities {
		nodeCapabilities[cap] = true
	}
	
	for _, req := range required {
		if !nodeCapabilities[req] {
			return false
		}
	}
	
	return true
}

// selectBestNode selects the best node for a workload based on the load balancing algorithm
func (ha *DefaultHAIntegration) selectBestNode(nodes []NodeInfo, workload Workload) NodeInfo {
	if len(nodes) == 0 {
		return NodeInfo{}
	}
	
	switch ha.loadBalanceAlgorithm {
	case "least_loaded":
		return ha.selectLeastLoadedNode(nodes)
	case "weighted_round_robin":
		return ha.selectWeightedRoundRobinNode(nodes)
	case "resource_based":
		return ha.selectResourceBasedNode(nodes, workload)
	default:
		// Default to least loaded
		return ha.selectLeastLoadedNode(nodes)
	}
}

// selectLeastLoadedNode selects the node with the lowest current load
func (ha *DefaultHAIntegration) selectLeastLoadedNode(nodes []NodeInfo) NodeInfo {
	bestNode := nodes[0]
	bestLoadRatio := float64(bestNode.CurrentLoad) / float64(bestNode.MaxConcurrency)
	
	for _, node := range nodes[1:] {
		loadRatio := float64(node.CurrentLoad) / float64(node.MaxConcurrency)
		if loadRatio < bestLoadRatio {
			bestNode = node
			bestLoadRatio = loadRatio
		}
	}
	
	return bestNode
}

// selectWeightedRoundRobinNode selects node using weighted round robin
func (ha *DefaultHAIntegration) selectWeightedRoundRobinNode(nodes []NodeInfo) NodeInfo {
	// Simplified weighted round robin - weight based on available capacity
	bestNode := nodes[0]
	bestWeight := bestNode.MaxConcurrency - bestNode.CurrentLoad
	
	for _, node := range nodes[1:] {
		weight := node.MaxConcurrency - node.CurrentLoad
		if weight > bestWeight {
			bestNode = node
			bestWeight = weight
		}
	}
	
	return bestNode
}

// selectResourceBasedNode selects node based on resource requirements
func (ha *DefaultHAIntegration) selectResourceBasedNode(nodes []NodeInfo, workload Workload) NodeInfo {
	bestNode := nodes[0]
	bestScore := ha.calculateResourceScore(bestNode, workload)
	
	for _, node := range nodes[1:] {
		score := ha.calculateResourceScore(node, workload)
		if score > bestScore {
			bestNode = node
			bestScore = score
		}
	}
	
	return bestNode
}

// calculateResourceScore calculates a resource fitness score for a node and workload
func (ha *DefaultHAIntegration) calculateResourceScore(node NodeInfo, workload Workload) float64 {
	// Score based on available resources vs. required resources
	memoryScore := float64(node.AvailableMemory) / float64(workload.RequiredMemory)
	cpuScore := node.AvailableCPU / workload.RequiredCPU
	loadScore := float64(node.MaxConcurrency-node.CurrentLoad) / float64(node.MaxConcurrency)
	
	// Weighted average (memory and CPU more important than load)
	return (memoryScore*0.4 + cpuScore*0.4 + loadScore*0.2)
}

// calculateLoadBalanceScore calculates how well-balanced the cluster load is
func (ha *DefaultHAIntegration) calculateLoadBalanceScore() float64 {
	if len(ha.nodes) == 0 {
		return 1.0
	}
	
	// Calculate variance in load ratios
	var loadRatios []float64
	var sum float64
	
	for _, node := range ha.nodes {
		if node.MaxConcurrency > 0 {
			ratio := float64(node.CurrentLoad) / float64(node.MaxConcurrency)
			loadRatios = append(loadRatios, ratio)
			sum += ratio
		}
	}
	
	if len(loadRatios) == 0 {
		return 1.0
	}
	
	mean := sum / float64(len(loadRatios))
	var variance float64
	
	for _, ratio := range loadRatios {
		diff := ratio - mean
		variance += diff * diff
	}
	
	variance /= float64(len(loadRatios))
	
	// Convert variance to score (lower variance = higher score)
	// Score is between 0 and 1, where 1 is perfectly balanced
	return 1.0 / (1.0 + variance)
}

// HandleNodeFailure handles the failure of a node
func (ha *DefaultHAIntegration) HandleNodeFailure(ctx context.Context, nodeID string) error {
	ha.mutex.Lock()
	defer ha.mutex.Unlock()
	
	// Check if node exists
	node, exists := ha.nodes[nodeID]
	if !exists {
		return fmt.Errorf("node %s not found", nodeID)
	}
	
	// Mark node as unhealthy
	node.HealthStatus = NodeHealthUnhealthy
	ha.nodes[nodeID] = node
	
	// Increment failure count
	ha.nodeFailures[nodeID]++
	
	// Find tasks assigned to failed node
	affectedWorkloads := make([]string, 0)
	
	for workloadID, distribution := range ha.distributions {
		if task, exists := distribution.AssignedNodes[nodeID]; exists {
			if task.Status == TaskStatusRunning || task.Status == TaskStatusPending {
				affectedWorkloads = append(affectedWorkloads, workloadID)
			}
		}
	}
	
	// Reassign affected workloads to failover nodes
	for _, workloadID := range affectedWorkloads {
		err := ha.reassignWorkload(workloadID, nodeID)
		if err != nil {
			return fmt.Errorf("failed to reassign workload %s: %w", workloadID, err)
		}
	}
	
	return nil
}

// reassignWorkload reassigns a workload from a failed node to a failover node
func (ha *DefaultHAIntegration) reassignWorkload(workloadID, failedNodeID string) error {
	distribution, exists := ha.distributions[workloadID]
	if !exists {
		return fmt.Errorf("distribution not found for workload %s", workloadID)
	}
	
	workload, exists := ha.workloads[workloadID]
	if !exists {
		return fmt.Errorf("workload %s not found", workloadID)
	}
	
	// Find a suitable failover node
	var failoverNodeID string
	for _, nodeID := range distribution.FailoverNodes {
		if node, exists := ha.nodes[nodeID]; exists {
			if node.HealthStatus == NodeHealthHealthy || node.HealthStatus == NodeHealthDegraded {
				failoverNodeID = nodeID
				break
			}
		}
	}
	
	// If no failover node available, find any suitable node
	if failoverNodeID == "" {
		suitableNodes := ha.findSuitableNodes(workload)
		if len(suitableNodes) > 0 {
			failoverNode := ha.selectBestNode(suitableNodes, workload)
			failoverNodeID = failoverNode.ID
		}
	}
	
	if failoverNodeID == "" {
		return fmt.Errorf("no suitable failover node found for workload %s", workloadID)
	}
	
	// Remove task from failed node
	delete(distribution.AssignedNodes, failedNodeID)
	
	// Create new task for failover node
	newTask := WorkloadTask{
		NodeID:        failoverNodeID,
		TaskID:        fmt.Sprintf("%s-failover-%d", workloadID, time.Now().Unix()),
		StartTime:     time.Now(),
		EstimatedTime: workload.EstimatedTime,
		Status:        TaskStatusPending,
		Progress:      0.0,
		Retries:       ha.nodeFailures[failedNodeID],
	}
	
	distribution.AssignedNodes[failoverNodeID] = newTask
	ha.distributions[workloadID] = distribution
	
	return nil
}

// RebalanceWorkload rebalances workloads across healthy nodes
func (ha *DefaultHAIntegration) RebalanceWorkload(ctx context.Context) error {
	ha.mutex.Lock()
	defer ha.mutex.Unlock()
	
	// Check if rebalancing is needed
	if time.Since(ha.lastRebalance) < ha.rebalanceInterval {
		return nil
	}
	
	// Calculate current load balance score
	currentScore := ha.calculateLoadBalanceScore()
	
	// Only rebalance if score is below threshold
	if currentScore > 0.8 {
		ha.lastRebalance = time.Now()
		return nil
	}
	
	// Find overloaded and underloaded nodes
	var overloadedNodes, underloadedNodes []NodeInfo
	
	for _, node := range ha.nodes {
		if node.HealthStatus != NodeHealthHealthy && node.HealthStatus != NodeHealthDegraded {
			continue
		}
		
		loadRatio := float64(node.CurrentLoad) / float64(node.MaxConcurrency)
		if loadRatio > 0.8 {
			overloadedNodes = append(overloadedNodes, node)
		} else if loadRatio < 0.4 {
			underloadedNodes = append(underloadedNodes, node)
		}
	}
	
	// Move workloads from overloaded to underloaded nodes
	if len(overloadedNodes) > 0 && len(underloadedNodes) > 0 {
		// For simplicity, just mark that rebalancing occurred
		// In a real implementation, this would involve more complex logic
		ha.lastRebalance = time.Now()
	}
	
	return nil
}

// GetNodeMetrics returns detailed metrics for a specific node
func (ha *DefaultHAIntegration) GetNodeMetrics(ctx context.Context, nodeID string) (*NodeMetrics, error) {
	ha.mutex.RLock()
	defer ha.mutex.RUnlock()
	
	metrics, exists := ha.metrics[nodeID]
	if !exists {
		return nil, fmt.Errorf("metrics not found for node %s", nodeID)
	}
	
	// Update uptime if node exists
	if node, exists := ha.nodes[nodeID]; exists {
		metrics.Uptime = time.Since(node.RegisteredAt)
	}
	
	return &metrics, nil
}