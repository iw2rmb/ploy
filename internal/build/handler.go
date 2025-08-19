package build

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
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
	io.Copy(f, c.Context().RequestBodyStream())

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

	sbom := utils.FileExists(imagePath+".sbom.json") || utils.FileExists(filepath.Join(srcDir, "SBOM.json"))
	signed := utils.FileExists(imagePath + ".sig")
	if signed && imagePath != "" {
		_ = supply.VerifySignature(imagePath, imagePath+".sig")
	}

	if err := opa.Enforce(opa.ArtifactInput{
		Signed:     signed,
		SBOMPresent: sbom,
		Env:        c.Query("env", "dev"),
		SSHEnabled: false,
	}); err != nil {
		return utils.ErrJSON(c, 403, fmt.Errorf("policy denied: %w", err))
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
		if imagePath != "" {
			if f, err := os.Open(imagePath); err == nil {
				defer f.Close()
				storeClient.PutObject(storeClient.Artifacts, keyPrefix+filepath.Base(imagePath), f, "application/octet-stream")
			}
			if f, err := os.Open(imagePath + ".sbom.json"); err == nil {
				defer f.Close()
				storeClient.PutObject(storeClient.Artifacts, keyPrefix+filepath.Base(imagePath+".sbom.json"), f, "application/json")
			}
			if f, err := os.Open(imagePath + ".sig"); err == nil {
				defer f.Close()
				storeClient.PutObject(storeClient.Artifacts, keyPrefix+filepath.Base(imagePath+".sig"), f, "application/octet-stream")
			}
		}
		meta := map[string]string{"lane": lane, "image": imagePath, "dockerImage": dockerImage}
		mb, _ := json.Marshal(meta)
		storeClient.PutObject(storeClient.Artifacts, keyPrefix+"meta.json", bytes.NewReader(mb), "application/json")
	}

	return c.JSON(fiber.Map{"status": "deployed", "lane": lane, "image": imagePath, "dockerImage": dockerImage})
}

func ListApps(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"apps": []string{}})
}

func Status(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"status": "ok"})
}