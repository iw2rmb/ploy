package lifecycle

import (
	"fmt"
	"log"
	"os/exec"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/ploy/ploy/controller/envstore"
	"github.com/ploy/ploy/internal/storage"
)

func DestroyApp(c *fiber.Ctx, storeClient *storage.StorageClient, envStore *envstore.EnvStore) error {
	app := c.Params("app")
	force := c.Query("force") == "true"
	
	log.Printf("Destroying app %s (force: %v)", app, force)
	
	destroyStatus := map[string]interface{}{
		"app":        app,
		"status":     "destroying",
		"operations": map[string]string{},
		"errors":     []string{},
	}
	
	if err := destroyNomadJobs(app, destroyStatus); err != nil {
		destroyStatus["errors"] = append(destroyStatus["errors"].([]string), fmt.Sprintf("Nomad cleanup failed: %v", err))
	}
	
	if err := destroyEnvironmentVariables(app, destroyStatus, envStore); err != nil {
		destroyStatus["errors"] = append(destroyStatus["errors"].([]string), fmt.Sprintf("Environment cleanup failed: %v", err))
	}
	
	if err := destroyDomains(app, destroyStatus); err != nil {
		destroyStatus["errors"] = append(destroyStatus["errors"].([]string), fmt.Sprintf("Domain cleanup failed: %v", err))
	}
	
	if err := destroyCertificates(app, destroyStatus); err != nil {
		destroyStatus["errors"] = append(destroyStatus["errors"].([]string), fmt.Sprintf("Certificate cleanup failed: %v", err))
	}
	
	if err := destroyStorageArtifacts(app, destroyStatus, storeClient); err != nil {
		destroyStatus["errors"] = append(destroyStatus["errors"].([]string), fmt.Sprintf("Storage cleanup failed: %v", err))
	}
	
	if err := destroyContainerImages(app, destroyStatus); err != nil {
		destroyStatus["errors"] = append(destroyStatus["errors"].([]string), fmt.Sprintf("Container cleanup failed: %v", err))
	}
	
	if err := destroyTemporaryFiles(app, destroyStatus); err != nil {
		destroyStatus["errors"] = append(destroyStatus["errors"].([]string), fmt.Sprintf("Temporary files cleanup failed: %v", err))
	}
	
	errors := destroyStatus["errors"].([]string)
	if len(errors) == 0 {
		destroyStatus["status"] = "destroyed"
		destroyStatus["message"] = "Application and all associated resources destroyed successfully"
	} else {
		destroyStatus["status"] = "partially_destroyed"
		destroyStatus["message"] = fmt.Sprintf("Application destroyed with %d errors", len(errors))
	}
	
	return c.JSON(destroyStatus)
}

func destroyNomadJobs(app string, status map[string]interface{}) error {
	log.Printf("Destroying Nomad jobs for app: %s", app)
	operations := status["operations"].(map[string]string)
	
	jobPatterns := []string{
		app,
		fmt.Sprintf("%s-*", app),
		fmt.Sprintf("debug-%s-*", app),
	}
	
	for _, pattern := range jobPatterns {
		cmd := exec.Command("nomad", "job", "stop", "-purge", pattern)
		if output, err := cmd.CombinedOutput(); err != nil {
			if !strings.Contains(string(output), "not found") {
				log.Printf("Failed to stop Nomad job %s: %v", pattern, err)
				return fmt.Errorf("failed to stop job %s: %v", pattern, err)
			}
		}
		operations[fmt.Sprintf("nomad_%s", pattern)] = "stopped"
	}
	
	log.Printf("Nomad jobs destroyed for app: %s", app)
	return nil
}

func destroyEnvironmentVariables(app string, status map[string]interface{}, envStore *envstore.EnvStore) error {
	log.Printf("Destroying environment variables for app: %s", app)
	operations := status["operations"].(map[string]string)
	
	envVars, err := envStore.GetAll(app)
	if err != nil {
		log.Printf("No environment variables found for app %s: %v", app, err)
		operations["env_vars"] = "none_found"
		return nil
	}
	
	for key := range envVars {
		if err := envStore.Delete(app, key); err != nil {
			return fmt.Errorf("failed to delete environment variable %s: %v", key, err)
		}
	}
	
	operations["env_vars"] = fmt.Sprintf("deleted_%d_variables", len(envVars))
	log.Printf("Environment variables destroyed for app: %s", app)
	return nil
}

func destroyDomains(app string, status map[string]interface{}) error {
	log.Printf("Destroying domains for app: %s", app)
	operations := status["operations"].(map[string]string)
	
	operations["domains"] = "not_implemented"
	log.Printf("Domain destruction not implemented for app: %s", app)
	return nil
}

func destroyCertificates(app string, status map[string]interface{}) error {
	log.Printf("Destroying certificates for app: %s", app)
	operations := status["operations"].(map[string]string)
	
	operations["certificates"] = "not_implemented"
	log.Printf("Certificate destruction not implemented for app: %s", app)
	return nil
}

func destroyStorageArtifacts(app string, status map[string]interface{}, storeClient *storage.StorageClient) error {
	log.Printf("Destroying storage artifacts for app: %s", app)
	operations := status["operations"].(map[string]string)
	
	if storeClient == nil {
		operations["storage"] = "no_client"
		return nil
	}
	
	operations["storage"] = "not_implemented"
	log.Printf("Storage artifact destruction not implemented for app: %s", app)
	return nil
}

func destroyContainerImages(app string, status map[string]interface{}) error {
	log.Printf("Destroying container images for app: %s", app)
	operations := status["operations"].(map[string]string)
	
	imagePattern := fmt.Sprintf("harbor.local/ploy/%s", app)
	
	cmd := exec.Command("docker", "images", "--format", "{{.Repository}}:{{.Tag}}", imagePattern)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("Failed to list images for %s: %v", app, err)
		operations["container_images"] = "list_failed"
		return nil
	}
	
	images := strings.Split(strings.TrimSpace(string(output)), "\n")
	deletedCount := 0
	
	for _, image := range images {
		if image != "" && strings.Contains(image, app) {
			rmCmd := exec.Command("docker", "rmi", "-f", image)
			if rmErr := rmCmd.Run(); rmErr != nil {
				log.Printf("Failed to remove image %s: %v", image, rmErr)
			} else {
				deletedCount++
			}
		}
	}
	
	operations["container_images"] = fmt.Sprintf("deleted_%d_images", deletedCount)
	log.Printf("Container images destroyed for app: %s (deleted: %d)", app, deletedCount)
	return nil
}

func destroyTemporaryFiles(app string, status map[string]interface{}) error {
	log.Printf("Destroying temporary files for app: %s", app)
	operations := status["operations"].(map[string]string)
	
	tempDirs := []string{
		fmt.Sprintf("/tmp/*%s*", app),
		fmt.Sprintf("/tmp/debug-*%s*", app),
		fmt.Sprintf("/tmp/build-*%s*", app),
	}
	
	deletedDirs := 0
	for _, pattern := range tempDirs {
		cmd := exec.Command("sh", "-c", fmt.Sprintf("rm -rf %s", pattern))
		if err := cmd.Run(); err != nil {
			log.Printf("Failed to remove temp files %s: %v", pattern, err)
		} else {
			deletedDirs++
		}
	}
	
	sshKeyPattern := fmt.Sprintf("/tmp/debug-%s-*.key", app)
	cmd := exec.Command("sh", "-c", fmt.Sprintf("rm -f %s", sshKeyPattern))
	if err := cmd.Run(); err != nil {
		log.Printf("Failed to remove SSH keys for %s: %v", app, err)
	}
	
	operations["temp_files"] = fmt.Sprintf("cleaned_%d_patterns", deletedDirs)
	log.Printf("Temporary files destroyed for app: %s", app)
	return nil
}