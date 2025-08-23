package bluegreen

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/hashicorp/consul/api"
	nomadapi "github.com/hashicorp/nomad/api"
)

// updateTraefikWeights updates the traffic weights in Traefik via Consul service tags
func (m *Manager) updateTraefikWeights(ctx context.Context, appName string, blueWeight, greenWeight int) error {
	
	// Update blue service weight
	if blueWeight > 0 {
		blueServiceName := fmt.Sprintf("%s-blue", appName)
		if err := m.updateServiceWeight(ctx, blueServiceName, blueWeight); err != nil {
			return fmt.Errorf("failed to update blue service weight: %w", err)
		}
	}
	
	// Update green service weight  
	if greenWeight > 0 {
		greenServiceName := fmt.Sprintf("%s-green", appName)
		if err := m.updateServiceWeight(ctx, greenServiceName, greenWeight); err != nil {
			return fmt.Errorf("failed to update green service weight: %w", err)
		}
	}
	
	log.Printf("Updated Traefik traffic weights for app %s (blue: %d%%, green: %d%%)", appName, blueWeight, greenWeight)
	
	return nil
}

// updateServiceWeight updates the weight for a specific service
func (m *Manager) updateServiceWeight(ctx context.Context, serviceName string, weight int) error {
	catalog := m.consulClient.Catalog()
	
	// Get current service registration
	services, _, err := catalog.Service(serviceName, "", nil)
	if err != nil {
		return fmt.Errorf("failed to get service %s: %w", serviceName, err)
	}
	
	if len(services) == 0 {
		return fmt.Errorf("service %s not found", serviceName)
	}
	
	// Update each service instance
	for _, service := range services {
		// Parse current tags
		newTags := make([]string, 0, len(service.ServiceTags))
		weightTagFound := false
		
		for _, tag := range service.ServiceTags {
			if strings.HasPrefix(tag, "traefik.http.services.") && strings.Contains(tag, ".loadbalancer.weighted.weight=") {
				// Replace existing weight tag
				parts := strings.Split(tag, "=")
				if len(parts) == 2 {
					newTag := fmt.Sprintf("%s=%d", parts[0], weight)
					newTags = append(newTags, newTag)
					weightTagFound = true
				}
			} else {
				newTags = append(newTags, tag)
			}
		}
		
		// Add weight tag if not found
		if !weightTagFound {
			weightTag := fmt.Sprintf("traefik.http.services.%s.loadbalancer.weighted.weight=%d", serviceName, weight)
			newTags = append(newTags, weightTag)
		}
		
		// Re-register service with updated tags
		agent := m.consulClient.Agent()
		serviceReg := &api.AgentServiceRegistration{
			ID:      service.ServiceID,
			Name:    service.ServiceName,
			Tags:    newTags,
			Port:    service.ServicePort,
			Address: service.ServiceAddress,
			Meta:    service.ServiceMeta,
		}
		
		if err := agent.ServiceRegister(serviceReg); err != nil {
			return fmt.Errorf("failed to update service registration for %s: %w", serviceName, err)
		}
		
		log.Printf("Updated service weight for %s to %d%% on node %s", serviceName, weight, service.Node)
	}
	
	return nil
}

// deployColoredVersion deploys a specific version with blue or green coloring
func (m *Manager) deployColoredVersion(ctx context.Context, appName string, color DeploymentColor, version string) error {
	log.Printf("Deploying %s version %s for app %s", color, version, appName)
	
	// Create job name with color suffix
	jobName := fmt.Sprintf("%s-%s", appName, color)
	
	// Get the base job template for the app
	jobs := m.nomadClient.Jobs()
	job, _, err := jobs.Info(appName, nil)
	if err != nil {
		return fmt.Errorf("failed to get base job template for %s: %w", appName, err)
	}
	
	// Clone and modify job for colored deployment  
	coloredJob := &nomadapi.Job{}
	*coloredJob = *job  // Shallow copy
	coloredJob.ID = &jobName
	coloredJob.Name = &jobName
	
	// Update service names and tags for the colored deployment
	if err := m.updateJobServicesForColor(coloredJob, appName, color, version); err != nil {
		return fmt.Errorf("failed to update job services for color: %w", err)
	}
	
	// Set version environment variable
	if err := m.setVersionEnvironment(coloredJob, version); err != nil {
		return fmt.Errorf("failed to set version environment: %w", err)
	}
	
	// Submit the colored job
	_, _, err = jobs.Register(coloredJob, nil)
	if err != nil {
		return fmt.Errorf("failed to register %s job: %w", color, err)
	}
	
	// Wait for deployment to be healthy
	if err := m.waitForDeploymentHealth(ctx, jobName, 5*time.Minute); err != nil {
		return fmt.Errorf("colored deployment failed health check: %w", err)
	}
	
	return nil
}

// updateJobServicesForColor modifies job services for blue-green deployment
func (m *Manager) updateJobServicesForColor(job *nomadapi.Job, appName string, color DeploymentColor, version string) error {
	coloredServiceName := fmt.Sprintf("%s-%s", appName, color)
	
	// Update all task groups
	for _, group := range job.TaskGroups {
		// Update services in the task group
		for _, service := range group.Services {
			// Update service name
			service.Name = coloredServiceName
			
			// Update tags for blue-green deployment
			newTags := make([]string, 0, len(service.Tags))
			for _, tag := range service.Tags {
				if strings.Contains(tag, "traefik.http.routers.") {
					// Update router name in tags
					newTag := strings.ReplaceAll(tag, appName, coloredServiceName)
					newTags = append(newTags, newTag)
				} else {
					newTags = append(newTags, tag)
				}
			}
			
			// Add blue-green specific tags
			newTags = append(newTags, 
				fmt.Sprintf("blue-green.app=%s", appName),
				fmt.Sprintf("blue-green.color=%s", color),
				fmt.Sprintf("blue-green.version=%s", version),
				fmt.Sprintf("traefik.http.services.%s.loadbalancer.weighted.weight=0", coloredServiceName),
			)
			
			service.Tags = newTags
			
			// Update service meta with deployment info
			if service.Meta == nil {
				service.Meta = make(map[string]string)
			}
			service.Meta["deployment_color"] = string(color)
			service.Meta["deployment_version"] = version
			service.Meta["deployment_type"] = "blue-green"
		}
		
		// Update tasks in the group
		for _, task := range group.Tasks {
			// Add deployment metadata to task environment
			if task.Env == nil {
				task.Env = make(map[string]string)
			}
			task.Env["DEPLOYMENT_COLOR"] = string(color)
			task.Env["DEPLOYMENT_VERSION"] = version
			task.Env["DEPLOYMENT_TYPE"] = "blue-green"
		}
	}
	
	return nil
}

// setVersionEnvironment sets the application version in all job tasks
func (m *Manager) setVersionEnvironment(job *nomadapi.Job, version string) error {
	for _, group := range job.TaskGroups {
		for _, task := range group.Tasks {
			if task.Env == nil {
				task.Env = make(map[string]string)
			}
			task.Env["APP_VERSION"] = version
		}
	}
	return nil
}

// waitForDeploymentHealth waits for a deployment to become healthy
func (m *Manager) waitForDeploymentHealth(ctx context.Context, jobName string, timeout time.Duration) error {
	jobs := m.nomadClient.Jobs()
	
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for deployment health: %w", ctx.Err())
		case <-ticker.C:
			// Check job status
			status, _, err := jobs.Summary(jobName, nil)
			if err != nil {
				log.Printf("Failed to get job status during health check: %v", err)
				continue
			}
			
			// Check if all task groups are healthy
			allHealthy := true
			for groupName, summary := range status.Summary {
				if summary.Running == 0 || summary.Failed > 0 {
					log.Printf("Task group %s not yet healthy for job %s (running: %d, failed: %d)", groupName, jobName, summary.Running, summary.Failed)
					allHealthy = false
					break
				}
			}
			
			if allHealthy {
				log.Printf("Deployment is healthy for job %s", jobName)
				return nil
			}
		}
	}
}

// validateDeploymentHealth validates that a colored deployment is healthy
func (m *Manager) validateDeploymentHealth(ctx context.Context, appName string, color DeploymentColor) error {
	serviceName := fmt.Sprintf("%s-%s", appName, color)
	
	// Check Consul service health
	health := m.consulClient.Health()
	services, _, err := health.Service(serviceName, "", true, nil) // passing=true for healthy services only
	if err != nil {
		return fmt.Errorf("failed to check service health: %w", err)
	}
	
	if len(services) == 0 {
		return fmt.Errorf("no healthy instances found for service %s", serviceName)
	}
	
	// Verify all instances are healthy
	healthyCount := 0
	for _, service := range services {
		allPassing := true
		for _, check := range service.Checks {
			if check.Status != "passing" {
				allPassing = false
				break
			}
		}
		if allPassing {
			healthyCount++
		}
	}
	
	if healthyCount == 0 {
		return fmt.Errorf("no fully healthy instances for service %s", serviceName)
	}
	
	log.Printf("Deployment health validated for service %s (%d/%d healthy instances)", serviceName, healthyCount, len(services))
	
	return nil
}

// cleanupOldDeployment removes the old deployment after successful blue-green switch
func (m *Manager) cleanupOldDeployment(ctx context.Context, appName string, state *DeploymentState) {
	// Wait before cleanup to ensure stability
	time.Sleep(5 * time.Minute)
	
	var oldColor DeploymentColor
	if state.ActiveColor == Blue {
		oldColor = Green
	} else {
		oldColor = Blue
	}
	
	oldJobName := fmt.Sprintf("%s-%s", appName, oldColor)
	
	log.Printf("Cleaning up old deployment %s for app %s", oldJobName, appName)
	
	// Stop and remove old job
	jobs := m.nomadClient.Jobs()
	_, _, err := jobs.Deregister(oldJobName, true, nil) // purge=true to remove completely
	if err != nil {
		log.Printf("Failed to cleanup old deployment: %v", err)
	}
}

// cleanupFailedDeployment removes a failed deployment
func (m *Manager) cleanupFailedDeployment(ctx context.Context, appName string, state *DeploymentState) {
	var failedColor DeploymentColor
	if state.ActiveColor == Blue {
		failedColor = Green
	} else {
		failedColor = Blue
	}
	
	failedJobName := fmt.Sprintf("%s-%s", appName, failedColor)
	
	log.Printf("Cleaning up failed deployment %s for app %s", failedJobName, appName)
	
	// Stop and remove failed job
	jobs := m.nomadClient.Jobs()
	_, _, err := jobs.Deregister(failedJobName, true, nil)
	if err != nil {
		log.Printf("Failed to cleanup failed deployment: %v", err)
	}
}

// saveDeploymentState saves the current deployment state to Consul KV
func (m *Manager) saveDeploymentState(ctx context.Context, state *DeploymentState) error {
	kv := m.consulClient.KV()
	
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal deployment state: %w", err)
	}
	
	kvPair := &api.KVPair{
		Key:   fmt.Sprintf("ploy/apps/%s/bluegreen/state", state.AppName),
		Value: data,
	}
	
	_, err = kv.Put(kvPair, nil)
	if err != nil {
		return fmt.Errorf("failed to save deployment state to Consul: %w", err)
	}
	
	return nil
}

// detectStandardDeployment detects if there's a standard (non-blue-green) deployment
func (m *Manager) detectStandardDeployment(ctx context.Context, appName string) (*DeploymentState, error) {
	// Check if standard job exists
	jobs := m.nomadClient.Jobs()
	_, _, err := jobs.Info(appName, nil)
	if err != nil {
		// No deployment exists
		return &DeploymentState{
			AppName:     appName,
			ActiveColor: "",
			Status:      "none",
		}, nil
	}
	
	// Standard deployment exists, treat as blue
	return &DeploymentState{
		AppName:     appName,
		BlueVersion: "unknown",
		ActiveColor: Blue,
		BlueWeight:  100,
		GreenWeight: 0,
		Status:      "standard",
	}, nil
}