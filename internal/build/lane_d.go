package build

import (
	ibuilders "github.com/iw2rmb/ploy/internal/builders"
)

var jailBuilder = ibuilders.BuildJail

// buildLaneD handles lane D via FreeBSD jail builder and returns imagePath.
func buildLaneD(appName, srcDir, sha, tmpDir string, appEnvVars map[string]string) (string, error) {
	img, err := jailBuilder(appName, srcDir, sha, tmpDir, appEnvVars)
	if err != nil {
		return "", err
	}
	return img, nil
}
