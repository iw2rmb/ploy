package build

import (
	"io"
	"log"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/iw2rmb/ploy/internal/detect/project"
	lanedetect "github.com/iw2rmb/ploy/internal/lane"
	"github.com/iw2rmb/ploy/internal/utils"
)

var persistentArtifactsDir = "/opt/ploy/artifacts"

// readRequestBodyToTar reads the incoming request body (multipart or raw) into dst
func readRequestBodyToTar(c *fiber.Ctx, dst *os.File) (int64, error) {
	ct := strings.ToLower(c.Get("Content-Type"))
	if strings.HasPrefix(ct, "multipart/form-data") {
		var fh *multipart.FileHeader
		for _, key := range []string{"file", "tar", "archive"} {
			if h, err := c.FormFile(key); err == nil && h != nil {
				fh = h
				break
			}
		}
		if fh == nil {
			if form, err := c.MultipartForm(); err == nil && form != nil {
				for _, files := range form.File {
					if len(files) > 0 {
						fh = files[0]
						break
					}
				}
			}
		}
		if fh == nil {
			return 0, fiber.NewError(400, "missing file part in multipart")
		}
		src, err := fh.Open()
		if err != nil {
			return 0, err
		}
		defer func() { _ = src.Close() }()
		n, err := io.Copy(dst, src)
		if err == nil {
			log.Printf("[Build] Received multipart tar %q (%d bytes)", fh.Filename, n)
		}
		return n, err
	}

	// Stream or buffered body
	var written int64
	if reader := c.Context().RequestBodyStream(); reader != nil {
		n, err := io.Copy(dst, reader)
		written = n
		if err != nil {
			return written, err
		}
	} else {
		n, err := dst.Write(c.Body())
		written = int64(n)
		if err != nil {
			return written, err
		}
	}
	return written, nil
}

// untarToDir extracts a tar at tarPath into dstDir
func untarToDir(tarPath, dstDir string) error {
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return err
	}
	return utils.Untar(tarPath, dstDir)
}

// detectBuildContext determines lane, language, main class and facts
func detectBuildContext(srcDir, laneQuery, mainQuery string) (lane, detectedLanguage, detectedJavaVersion, mainClass string, facts project.BuildFacts) {
	_ = laneQuery // ignored but kept for signature stability
	mainClass = mainQuery

	// Lightweight detector avoids spawning external binaries while still surfacing language hints.
	res := lanedetect.Detect(srcDir)
	detectedLanguage = strings.ToLower(strings.TrimSpace(res.Language))
	if detectedLanguage == "unknown" {
		detectedLanguage = ""
	}

	facts = project.ComputeFacts(srcDir, detectedLanguage)
	if detectedLanguage == "" && facts.Language != "" {
		detectedLanguage = facts.Language
	}
	if facts.Versions.Java != "" {
		detectedJavaVersion = facts.Versions.Java
	}
	if mainClass == "" {
		if facts.MainClass != "" {
			mainClass = facts.MainClass
		} else {
			mainClass = "com.ploy.ordersvc.Main"
		}
	}

	lane = "D"
	return
}

// ensurePersistentArtifactCopy copies the artifact and optional sidecar files to persistent dir
func ensurePersistentArtifactCopy(imagePath string) (string, error) {
	if imagePath == "" {
		return "", nil
	}
	if err := os.MkdirAll(persistentArtifactsDir, 0755); err != nil {
		return "", err
	}
	persistentImagePath := filepath.Join(persistentArtifactsDir, filepath.Base(imagePath))
	if err := copyFile(imagePath, persistentImagePath); err != nil {
		return "", err
	}
	if _, err := os.Stat(imagePath + ".sig"); err == nil {
		_ = copyFile(imagePath+".sig", persistentImagePath+".sig")
	}
	if _, err := os.Stat(imagePath + ".sbom.json"); err == nil {
		_ = copyFile(imagePath+".sbom.json", persistentImagePath+".sbom.json")
	}
	return persistentImagePath, nil
}
