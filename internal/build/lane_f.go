package build

import (
	ibuilders "github.com/iw2rmb/ploy/internal/builders"
)

var vmBuilder = ibuilders.BuildVM

// buildLaneF handles lane F (full VM) and returns imagePath.
func buildLaneF(appName, sha, tmpDir string, appEnvVars map[string]string) (string, error) {
	img, err := vmBuilder(appName, sha, tmpDir, appEnvVars)
	if err != nil {
		return "", err
	}
	return img, nil
}
