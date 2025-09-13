package bluegreen

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/hashicorp/consul/api"
	nomadapi "github.com/hashicorp/nomad/api"
)

// DeploymentColor represents blue or green deployment
type DeploymentColor string

const (
	Blue  DeploymentColor = "blue"
	Green DeploymentColor = "green"
)

// DeploymentState represents the current state of a blue-green deployment
type DeploymentState struct {
	AppName       string          `json:"app_name"`
	BlueVersion   string          `json:"blue_version,omitempty"`
	GreenVersion  string          `json:"green_version,omitempty"`
	ActiveColor   DeploymentColor `json:"active_color"`
	BlueWeight    int             `json:"blue_weight"`
	GreenWeight   int             `json:"green_weight"`
	Status        string          `json:"status"`
	LastShiftTime time.Time       `json:"last_shift_time"`
	TargetWeight  int             `json:"target_weight,omitempty"`
}

// TrafficShiftStep represents a step in the traffic shifting process
type TrafficShiftStep struct {
	BlueWeight  int           `json:"blue_weight"`
	GreenWeight int           `json:"green_weight"`
	Duration    time.Duration `json:"duration"`
	Description string        `json:"description"`
}

// DefaultShiftingStrategy defines the default traffic shifting pattern
var DefaultShiftingStrategy = []TrafficShiftStep{
	{BlueWeight: 100, GreenWeight: 0, Duration: time.Minute * 2, Description: "Initial validation"},
	{BlueWeight: 90, GreenWeight: 10, Duration: time.Minute * 5, Description: "Canary traffic"},
	{BlueWeight: 75, GreenWeight: 25, Duration: time.Minute * 10, Description: "Quarter traffic"},
	{BlueWeight: 50, GreenWeight: 50, Duration: time.Minute * 15, Description: "Split traffic"},
	{BlueWeight: 25, GreenWeight: 75, Duration: time.Minute * 10, Description: "Majority traffic"},
	{BlueWeight: 0, GreenWeight: 100, Duration: 0, Description: "Full cutover"},
}

// Manager handles blue-green deployment operations
type Manager struct {
	consulClient *api.Client
	nomadClient  *nomadapi.Client
}

// NewManager creates a new blue-green deployment manager
func NewManager(consulClient *api.Client, nomadClient *nomadapi.Client) *Manager {
	return &Manager{
		consulClient: consulClient,
		nomadClient:  nomadClient,
	}
}

// GetDeploymentState retrieves the current blue-green deployment state for an app
func (m *Manager) GetDeploymentState(ctx context.Context, appName string) (*DeploymentState, error) {
	kv := m.consulClient.KV()

	// Get deployment state from Consul KV
	kvPair, _, err := kv.Get(fmt.Sprintf("ploy/apps/%s/bluegreen/state", appName), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment state: %w", err)
	}

	if kvPair == nil {
		// No blue-green deployment active, check for standard deployment
		return m.detectStandardDeployment(ctx, appName)
	}

	var state DeploymentState
	if err := json.Unmarshal(kvPair.Value, &state); err != nil {
		return nil, fmt.Errorf("failed to unmarshal deployment state: %w", err)
	}

	return &state, nil
}

// StartBlueGreenDeployment initiates a new blue-green deployment
func (m *Manager) StartBlueGreenDeployment(ctx context.Context, appName string, newVersion string) (*DeploymentState, error) {
	log.Printf("Starting blue-green deployment for app %s version %s", appName, newVersion)

	// Get current deployment state
	currentState, err := m.GetDeploymentState(ctx, appName)
	if err != nil {
		return nil, fmt.Errorf("failed to get current deployment state: %w", err)
	}

	// Determine colors for blue-green deployment
	var blueVersion, greenVersion string
	var activeColor, inactiveColor DeploymentColor

	if currentState.ActiveColor == Blue || currentState.ActiveColor == "" {
		// Current is blue, deploy green
		blueVersion = currentState.BlueVersion
		greenVersion = newVersion
		activeColor = Blue
		inactiveColor = Green
	} else {
		// Current is green, deploy blue
		blueVersion = newVersion
		greenVersion = currentState.GreenVersion
		activeColor = Green
		inactiveColor = Blue
	}

	// Create new deployment state
	newState := &DeploymentState{
		AppName:       appName,
		BlueVersion:   blueVersion,
		GreenVersion:  greenVersion,
		ActiveColor:   activeColor,
		BlueWeight:    100,
		GreenWeight:   0,
		Status:        "deploying",
		LastShiftTime: time.Now(),
		TargetWeight:  0,
	}

	// Deploy the inactive color version
	if err := m.deployColoredVersion(ctx, appName, inactiveColor, newVersion); err != nil {
		return nil, fmt.Errorf("failed to deploy %s version: %w", inactiveColor, err)
	}

	// Save state to Consul
	if err := m.saveDeploymentState(ctx, newState); err != nil {
		return nil, fmt.Errorf("failed to save deployment state: %w", err)
	}

	return newState, nil
}

// ShiftTraffic gradually shifts traffic between blue and green deployments
func (m *Manager) ShiftTraffic(ctx context.Context, appName string, targetWeight int) error {
	log.Printf("Shifting traffic for app %s to target weight %d", appName, targetWeight)

	// Get current state
	state, err := m.GetDeploymentState(ctx, appName)
	if err != nil {
		return fmt.Errorf("failed to get deployment state: %w", err)
	}

	if state.Status != "ready" && state.Status != "shifting" {
		return fmt.Errorf("deployment not ready for traffic shifting, status: %s", state.Status)
	}

	// Validate green deployment health before shifting
	if err := m.validateDeploymentHealth(ctx, appName, Green); err != nil {
		log.Printf("Green deployment health check failed for app %s, aborting traffic shift: %v", appName, err)
		return fmt.Errorf("green deployment health check failed: %w", err)
	}

	// Calculate new weights
	var blueWeight, greenWeight int
	if state.ActiveColor == Blue {
		blueWeight = 100 - targetWeight
		greenWeight = targetWeight
	} else {
		blueWeight = targetWeight
		greenWeight = 100 - targetWeight
	}

	// Update Consul service weights
	if err := m.updateTraefikWeights(ctx, appName, blueWeight, greenWeight); err != nil {
		return fmt.Errorf("failed to update traffic weights: %w", err)
	}

	// Update state
	state.BlueWeight = blueWeight
	state.GreenWeight = greenWeight
	state.Status = "shifting"
	state.LastShiftTime = time.Now()
	state.TargetWeight = targetWeight

	// Save updated state
	if err := m.saveDeploymentState(ctx, state); err != nil {
		return fmt.Errorf("failed to save updated state: %w", err)
	}

	return nil
}

// CompleteDeployment completes the blue-green deployment by making green active
func (m *Manager) CompleteDeployment(ctx context.Context, appName string) error {
	log.Printf("Completing blue-green deployment for app %s", appName)

	// Get current state
	state, err := m.GetDeploymentState(ctx, appName)
	if err != nil {
		return fmt.Errorf("failed to get deployment state: %w", err)
	}

	// Shift all traffic to green
	if err := m.ShiftTraffic(ctx, appName, 100); err != nil {
		return fmt.Errorf("failed to complete traffic shift: %w", err)
	}

	// Update state to mark green as active
	if state.ActiveColor == Blue {
		state.ActiveColor = Green
		state.BlueWeight = 0
		state.GreenWeight = 100
	} else {
		state.ActiveColor = Blue
		state.BlueWeight = 100
		state.GreenWeight = 0
	}

	state.Status = "completed"
	state.LastShiftTime = time.Now()

	// Save final state
	if err := m.saveDeploymentState(ctx, state); err != nil {
		return fmt.Errorf("failed to save final state: %w", err)
	}

	// Clean up old deployment after completion
	go m.cleanupOldDeployment(context.Background(), appName, state)

	return nil
}

// RollbackDeployment rolls back to the previous version
func (m *Manager) RollbackDeployment(ctx context.Context, appName string) error {
	log.Printf("Rolling back blue-green deployment for app %s", appName)

	// Get current state
	state, err := m.GetDeploymentState(ctx, appName)
	if err != nil {
		return fmt.Errorf("failed to get deployment state: %w", err)
	}

	// Shift all traffic back to the original active color
	if state.ActiveColor == Blue {
		if err := m.updateTraefikWeights(ctx, appName, 100, 0); err != nil {
			return fmt.Errorf("failed to rollback traffic: %w", err)
		}
		state.BlueWeight = 100
		state.GreenWeight = 0
	} else {
		if err := m.updateTraefikWeights(ctx, appName, 0, 100); err != nil {
			return fmt.Errorf("failed to rollback traffic: %w", err)
		}
		state.BlueWeight = 0
		state.GreenWeight = 100
	}

	state.Status = "rolled_back"
	state.LastShiftTime = time.Now()

	// Save rollback state
	if err := m.saveDeploymentState(ctx, state); err != nil {
		return fmt.Errorf("failed to save rollback state: %w", err)
	}

	// Clean up failed deployment
	go m.cleanupFailedDeployment(context.Background(), appName, state)

	return nil
}

// AutoShiftTraffic automatically shifts traffic using the default strategy
func (m *Manager) AutoShiftTraffic(ctx context.Context, appName string) error {
	log.Printf("Starting automatic traffic shifting for app %s", appName)

	for i, step := range DefaultShiftingStrategy {
		log.Printf("Executing traffic shift step %d for app %s: %s (blue: %d%%, green: %d%%)", i+1, appName, step.Description, step.BlueWeight, step.GreenWeight)

		// Get current state to determine target weight
		state, err := m.GetDeploymentState(ctx, appName)
		if err != nil {
			return fmt.Errorf("failed to get deployment state for step %d: %w", i+1, err)
		}

		// Determine target weight based on active color
		var targetWeight int
		if state.ActiveColor == Blue {
			targetWeight = step.GreenWeight
		} else {
			targetWeight = step.BlueWeight
		}

		// Shift traffic
		if err := m.ShiftTraffic(ctx, appName, targetWeight); err != nil {
			log.Printf("Traffic shift failed for app %s, initiating rollback: %v", appName, err)
			_ = m.RollbackDeployment(ctx, appName)
			return fmt.Errorf("traffic shift step %d failed: %w", i+1, err)
		}

		// Wait for the specified duration before next step
		if step.Duration > 0 {
			log.Printf("Waiting %v before next traffic shift step for app %s", step.Duration, appName)

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(step.Duration):
				// Continue to next step
			}
		}

		// Health check after each step
		if err := m.validateDeploymentHealth(ctx, appName, Green); err != nil {
			log.Printf("Health check failed during traffic shift for app %s, rolling back: %v", appName, err)
			_ = m.RollbackDeployment(ctx, appName)
			return fmt.Errorf("health check failed at step %d: %w", i+1, err)
		}
	}

	log.Printf("Automatic traffic shifting completed successfully for app %s", appName)
	return nil
}
