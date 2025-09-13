package builders

import (
	"errors"
	"fmt"
	"path/filepath"
)

type JavaOSVRequest struct {
	App         string
	MainClass   string
	SrcDir      string // source directory
	JibTar      string // optional
	GitSHA      string
	OutDir      string
	EnvVars     map[string]string // environment variables
	JavaVersion string            // detected Java version (e.g., "21", "17", "11")
}

func BuildOSVJava(req JavaOSVRequest) (string, error) {
	if req.SrcDir == "" && req.JibTar == "" {
		return "", errors.New("either SrcDir or JibTar must be provided")
	}

	// Detect Java version if not provided
	javaVersion := req.JavaVersion
	if javaVersion == "" && req.SrcDir != "" {
		if detected, err := detectJavaVersion(req.SrcDir); err == nil && detected != "" {
			javaVersion = detected
			fmt.Printf("Detected Java version: %s\n", javaVersion)
		} else {
			javaVersion = "21" // Default to Java 21
			fmt.Printf("Java version detection failed, using default: %s\n", javaVersion)
		}
	} else if javaVersion == "" {
		javaVersion = "21" // Default fallback
	}

	jibTar := req.JibTar
	if jibTar == "" {
		var err error
		jibTar, err = runJibBuildTar(req.SrcDir, req.EnvVars)
		if err != nil {
			return "", err
		}
	}

	// Build OSv image using embedded capstan logic
	out := filepath.Join(req.OutDir, fmt.Sprintf("%s-%s.qcow2", req.App, short(req.GitSHA)))
	if err := buildOSvWithCapstan(jibTar, req.MainClass, req.App, req.GitSHA, out, javaVersion); err != nil {
		return "", fmt.Errorf("failed to build OSv image: %w", err)
	}
	return out, nil
}
