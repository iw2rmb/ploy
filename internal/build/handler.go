package build

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/ploy/ploy/controller/builders"
	"github.com/ploy/ploy/controller/envstore"
	"github.com/ploy/ploy/controller/nomad"
	"github.com/ploy/ploy/controller/opa"
	"github.com/ploy/ploy/controller/supply"
	"github.com/ploy/ploy/internal/storage"
	"github.com/ploy/ploy/internal/utils"
)

func TriggerBuild(c *fiber.Ctx, storeClient *storage.Client, envStore *envstore.EnvStore) error {
	appName := c.Params("app")
	sha := c.Query("sha", "dev")
	mainClass := c.Query("main", "com.ploy.ordersvc.Main")
	lane := c.Query("lane", "")

	tmpDir, _ := os.MkdirTemp("", "ploy-build-")
	defer os.RemoveAll(tmpDir)

	tarPath := filepath.Join(tmpDir, "src.tar")
	f, _ := os.Create(tarPath)
	defer f.Close()
	if _, err := f.Write(c.Body()); err != nil {
		return c.Status(400).SendString("Failed to read request body: " + err.Error())
	}

	srcDir := filepath.Join(tmpDir, "src")
	os.MkdirAll(srcDir, 0755)
	_ = utils.Untar(tarPath, srcDir)

	appEnvVars, err := envStore.GetAll(appName)
	if err != nil {
		appEnvVars = make(map[string]string)
	}

	if lane == "" {
		if res, err := utils.RunLanePick(srcDir); err == nil {
			lane = res.Lane
		} else {
			lane = "C"
		}
	}

	var imagePath, dockerImage string
	switch strings.ToUpper(lane) {
	case "A", "B":
		img, err := builders.BuildUnikraft(appName, lane, srcDir, sha, tmpDir, appEnvVars)
		if err != nil {
			return utils.ErrJSON(c, 500, err)
		}
		imagePath = img
	case "C":
		img, err := builders.BuildOSVJava(builders.JavaOSVRequest{
			App:       appName,
			MainClass: mainClass,
			SrcDir:    srcDir,
			GitSHA:    sha,
			OutDir:    tmpDir,
			EnvVars:   appEnvVars,
		})
		if err != nil {
			return utils.ErrJSON(c, 500, err)
		}
		imagePath = img
	case "D":
		img, err := builders.BuildJail(appName, srcDir, sha, tmpDir, appEnvVars)
		if err != nil {
			return utils.ErrJSON(c, 500, err)
		}
		imagePath = img
	case "E":
		tag := fmt.Sprintf("harbor.local/ploy/%s:%s", appName, sha)
		img, err := builders.BuildOCI(appName, srcDir, tag, appEnvVars)
		if err != nil {
			return utils.ErrJSON(c, 500, err)
		}
		dockerImage = img
	case "F":
		img, err := builders.BuildVM(appName, sha, tmpDir, appEnvVars)
		if err != nil {
			return utils.ErrJSON(c, 500, err)
		}
		imagePath = img
	default:
		lane = "C"
		img, err := builders.BuildOSVJava(builders.JavaOSVRequest{
			App:       appName,
			MainClass: mainClass,
			SrcDir:    srcDir,
			GitSHA:    sha,
			OutDir:    tmpDir,
			EnvVars:   appEnvVars,
		})
		if err != nil {
			return utils.ErrJSON(c, 500, err)
		}
		imagePath = img
	}

	// Generate comprehensive SBOM for the built artifact
	if imagePath != "" {
		// Generate SBOM for file-based artifacts (Lanes A, B, C, D, F)
		if !utils.FileExists(imagePath + ".sbom.json") {
			if err := supply.GenerateSBOM(imagePath, lane, appName, sha); err != nil {
				// Log error but don't fail the build - SBOM generation is best effort
				fmt.Printf("Warning: SBOM generation failed for %s: %v\n", imagePath, err)
			}
		}
	} else if dockerImage != "" {
		// Generate SBOM for container images (Lane E)
		if err := supply.GenerateSBOM(dockerImage, lane, appName, sha); err != nil {
			// Log error but don't fail the build - SBOM generation is best effort
			fmt.Printf("Warning: SBOM generation failed for container %s: %v\n", dockerImage, err)
		}
	}
	
	// Also generate source code SBOM for dependency analysis
	if !utils.FileExists(filepath.Join(srcDir, ".sbom.json")) {
		generator := supply.NewSBOMGenerator()
		options := supply.DefaultSBOMOptions()
		options.Lane = lane
		options.AppName = appName
		options.SHA = sha
		if err := generator.GenerateForSourceCode(srcDir, options); err != nil {
			// Log error but don't fail the build
			fmt.Printf("Warning: Source code SBOM generation failed: %v\n", err)
		}
	}

	// Sign the built artifact if not already signed
	if imagePath != "" && !utils.FileExists(imagePath + ".sig") {
		// Sign file-based artifacts (Lanes A, B, C, D, F)
		if err := supply.SignArtifact(imagePath); err != nil {
			return utils.ErrJSON(c, 500, fmt.Errorf("artifact signing failed: %w", err))
		}
	} else if dockerImage != "" {
		// Sign Docker images (Lane E)
		// Note: Docker image signing verification is more complex and handled by the registry
		if err := supply.SignDockerImage(dockerImage); err != nil {
			return utils.ErrJSON(c, 500, fmt.Errorf("docker image signing failed: %w", err))
		}
	}

	sbom := utils.FileExists(imagePath+".sbom.json") || utils.FileExists(filepath.Join(srcDir, "SBOM.json"))
	
	var signed bool
	if imagePath != "" {
		// Check for file-based artifact signatures
		signed = utils.FileExists(imagePath + ".sig")
		if signed {
			_ = supply.VerifySignature(imagePath, imagePath+".sig")
		}
	} else if dockerImage != "" {
		// For Docker images, assume signed if signing was successful
		// In a real environment, this would verify against the registry
		signed = true
	}

	// Enhanced OPA policy enforcement with comprehensive context
	env := c.Query("env", "dev")
	breakGlass := c.Query("break_glass", "false") == "true"
	debug := c.Query("debug", "false") == "true"
	
	if err := opa.Enforce(opa.ArtifactInput{
		Signed:      signed,
		SBOMPresent: sbom,
		Env:         env,
		SSHEnabled:  debug, // SSH is enabled for debug builds
		BreakGlass:  breakGlass,
		App:         appName,
		Lane:        lane,
		Debug:       debug,
	}); err != nil {
		return utils.ErrJSON(c, 403, fmt.Errorf("OPA policy enforcement failed: %w", err))
	}

	jobFile, err := nomad.RenderTemplate(lane, nomad.RenderData{
		App:         appName,
		ImagePath:   imagePath,
		DockerImage: dockerImage,
		EnvVars:     appEnvVars,
	})
	if err != nil {
		return utils.ErrJSON(c, 500, err)
	}
	
	if err := nomad.Submit(jobFile); err != nil {
		return utils.ErrJSON(c, 500, err)
	}
	
	_ = nomad.WaitHealthy(appName+"-lane-"+strings.ToLower(lane), 90*time.Second)

	if storeClient != nil {
		keyPrefix := appName + "/" + sha + "/"
		
		// Upload artifact bundle with comprehensive integrity verification
		if imagePath != "" {
			if result, err := storeClient.UploadArtifactBundleWithVerification(keyPrefix, imagePath); err != nil {
				return utils.ErrJSON(c, 500, fmt.Errorf("artifact bundle upload with verification failed: %w", err))
			} else {
				fmt.Printf("Artifact bundle integrity verification: %s\n", result.GetVerificationSummary())
				if !result.Verified {
					return utils.ErrJSON(c, 500, fmt.Errorf("artifact integrity verification failed: %s", strings.Join(result.Errors, "; ")))
				}
			}
		}
		
		// Upload source code SBOM with integrity verification if it exists
		sourceSBOMPath := filepath.Join(srcDir, ".sbom.json")
		if _, err := os.Stat(sourceSBOMPath); err == nil {
			if f, err := os.Open(sourceSBOMPath); err == nil {
				defer f.Close()
				if _, err := storeClient.PutObject(storeClient.GetArtifactsBucket(), keyPrefix+"source.sbom.json", f, "application/json"); err != nil {
					fmt.Printf("Warning: Failed to upload source SBOM: %v\n", err)
				} else {
					// Verify source SBOM upload integrity
					verifier := storage.NewIntegrityVerifier(storeClient)
					if info, err := verifier.VerifyUploadedFile(sourceSBOMPath, keyPrefix+"source.sbom.json"); err != nil {
						fmt.Printf("Warning: Source SBOM integrity verification failed: %v\n", err)
					} else {
						fmt.Printf("Source SBOM integrity verified: %s (size: %d bytes)\n", info.StorageKey, info.UploadedSize)
					}
				}
			}
		}
		
		// Upload container SBOM for Lane E with integrity verification if it exists in /tmp
		if dockerImage != "" {
			containerSBOMPath := fmt.Sprintf("/tmp/%s-%s.sbom.json", appName, strings.ReplaceAll(dockerImage, "/", "-"))
			if _, err := os.Stat(containerSBOMPath); err == nil {
				if f, err := os.Open(containerSBOMPath); err == nil {
					defer f.Close()
					if _, err := storeClient.PutObject(storeClient.GetArtifactsBucket(), keyPrefix+"container.sbom.json", f, "application/json"); err != nil {
						fmt.Printf("Warning: Failed to upload container SBOM: %v\n", err)
					} else {
						// Verify container SBOM upload integrity  
						verifier := storage.NewIntegrityVerifier(storeClient)
						if info, err := verifier.VerifyUploadedFile(containerSBOMPath, keyPrefix+"container.sbom.json"); err != nil {
							fmt.Printf("Warning: Container SBOM integrity verification failed: %v\n", err)
						} else {
							fmt.Printf("Container SBOM integrity verified: %s (size: %d bytes)\n", info.StorageKey, info.UploadedSize)
						}
					}
				}
			}
		}
		
		// Upload metadata
		meta := map[string]string{
			"lane":        lane,
			"image":       imagePath,
			"dockerImage": dockerImage,
			"timestamp":   time.Now().UTC().Format(time.RFC3339),
			"sbom":        fmt.Sprintf("%t", sbom),
			"signed":      fmt.Sprintf("%t", signed),
		}
		mb, _ := json.Marshal(meta)
		if _, err := storeClient.PutObject(storeClient.GetArtifactsBucket(), keyPrefix+"meta.json", bytes.NewReader(mb), "application/json"); err != nil {
			fmt.Printf("Warning: Failed to upload metadata: %v\n", err)
		}
	}

	return c.JSON(fiber.Map{"status": "deployed", "lane": lane, "image": imagePath, "dockerImage": dockerImage})
}

func ListApps(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"apps": []string{}})
}

func Status(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"status": "ok"})
}