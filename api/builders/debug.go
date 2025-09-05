package builders

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"golang.org/x/crypto/ssh"
)

type DebugBuildResult struct {
	ImagePath    string
	DockerImage  string
	SSHPublicKey string
	SSHCommand   string
}

func BuildDebugInstance(app, lane, srcDir, sha, outDir string, envVars map[string]string, sshEnabled bool) (*DebugBuildResult, error) {
	result := &DebugBuildResult{}

	// Generate SSH key pair if SSH is enabled
	if sshEnabled {
		privateKey, publicKey, err := generateSSHKeyPair()
		if err != nil {
			return nil, fmt.Errorf("failed to generate SSH key pair: %v", err)
		}

		// Store private key for user access
		privateKeyPath := filepath.Join(outDir, fmt.Sprintf("debug-%s-%s.key", app, sha))
		if err := os.WriteFile(privateKeyPath, privateKey, 0600); err != nil {
			return nil, fmt.Errorf("failed to write private key: %v", err)
		}

		result.SSHPublicKey = string(publicKey)
		result.SSHCommand = fmt.Sprintf("ssh -i %s debug@debug-%s-%s.debug.ployd.app", privateKeyPath, app, sha)

		// Add SSH public key to environment variables for injection into build
		envVars["SSH_PUBLIC_KEY"] = string(publicKey)
		envVars["SSH_ENABLED"] = "true"
	}

	// Build debug variant based on lane
	switch lane {
	case "A", "B":
		// For Unikraft lanes, build with SSH support
		imagePath, err := buildUnikraftDebug(app, lane, srcDir, sha, outDir, envVars, sshEnabled)
		if err != nil {
			return nil, err
		}
		result.ImagePath = imagePath

	case "C":
		// For OSv lane, build with SSH daemon
		imagePath, err := buildOSVDebug(app, srcDir, sha, outDir, envVars, sshEnabled)
		if err != nil {
			return nil, err
		}
		result.ImagePath = imagePath

	case "D":
		// For jail lane, build with SSH in jail
		imagePath, err := buildJailDebug(app, srcDir, sha, outDir, envVars, sshEnabled)
		if err != nil {
			return nil, err
		}
		result.ImagePath = imagePath

	case "E", "F":
		// For OCI and VM lanes, build debug container/VM with SSH
		dockerImage, err := buildOCIDebug(app, srcDir, sha, envVars, sshEnabled)
		if err != nil {
			return nil, err
		}
		result.DockerImage = dockerImage

	default:
		return nil, fmt.Errorf("unsupported lane for debug: %s", lane)
	}

	return result, nil
}

func buildUnikraftDebug(app, lane, srcDir, sha, outDir string, envVars map[string]string, sshEnabled bool) (string, error) {
	// Use debug-enabled Unikraft build script
	args := []string{"--app", app, "--app-dir", srcDir, "--lane", lane, "--sha", sha, "--out-dir", outDir, "--debug", "--ssh-enabled=" + fmt.Sprintf("%t", sshEnabled)}
	// Use absolute path to the build script in the ploy repository
	scriptPath := "/home/ploy/ploy/scripts/build/kraft/build_unikraft_debug.sh"

	// Fall back to relative path if absolute doesn't exist (for local development)
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		scriptPath = "./scripts/build/kraft/build_unikraft_debug.sh"
	}

	cmd := exec.Command(scriptPath, args...)

	env := os.Environ()
	for k, v := range envVars {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Env = env

	b, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("unikraft debug build failed: %v: %s", err, string(b))
	}
	return bytesTrimSpace(b), nil
}

func buildOSVDebug(app, srcDir, sha, outDir string, envVars map[string]string, sshEnabled bool) (string, error) {
	args := []string{"--app", app, "--src", srcDir, "--sha", sha, "--out-dir", outDir, "--debug", "--ssh-enabled=" + fmt.Sprintf("%t", sshEnabled)}
	// Use absolute path to the build script in the ploy repository
	scriptPath := "/home/ploy/ploy/scripts/build/osv/build_osv_debug.sh"

	// Fall back to relative path if absolute doesn't exist (for local development)
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		scriptPath = "./scripts/build/osv/build_osv_debug.sh"
	}

	cmd := exec.Command(scriptPath, args...)

	env := os.Environ()
	for k, v := range envVars {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Env = env

	b, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("osv debug build failed: %v: %s", err, string(b))
	}
	return bytesTrimSpace(b), nil
}

func buildJailDebug(app, srcDir, sha, outDir string, envVars map[string]string, sshEnabled bool) (string, error) {
	args := []string{"--app", app, "--src", srcDir, "--sha", sha, "--out-dir", outDir, "--debug", "--ssh-enabled=" + fmt.Sprintf("%t", sshEnabled)}
	// Use absolute path to the build script in the ploy repository
	scriptPath := "/home/ploy/ploy/scripts/build/jail/build_jail_debug.sh"

	// Fall back to relative path if absolute doesn't exist (for local development)
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		scriptPath = "./scripts/build/jail/build_jail_debug.sh"
	}

	cmd := exec.Command(scriptPath, args...)

	env := os.Environ()
	for k, v := range envVars {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Env = env

	b, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("jail debug build failed: %v: %s", err, string(b))
	}
	return bytesTrimSpace(b), nil
}

func buildOCIDebug(app, srcDir, sha string, envVars map[string]string, sshEnabled bool) (string, error) {
	debugTag := fmt.Sprintf("harbor.local/ploy/%s-debug:%s", app, sha)
	args := []string{"--app", app, "--src", srcDir, "--tag", debugTag, "--debug", "--ssh-enabled=" + fmt.Sprintf("%t", sshEnabled)}
	// Use absolute path to the build script in the ploy repository
	scriptPath := "/home/ploy/ploy/scripts/build/oci/build_oci_debug.sh"

	// Fall back to relative path if absolute doesn't exist (for local development)
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		scriptPath = "./scripts/build/oci/build_oci_debug.sh"
	}

	cmd := exec.Command(scriptPath, args...)

	env := os.Environ()
	for k, v := range envVars {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Env = env

	b, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("oci debug build failed: %v: %s", err, string(b))
	}
	return debugTag, nil
}

func generateSSHKeyPair() (privateKey, publicKey []byte, err error) {
	// Generate RSA private key
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}

	// Encode private key
	privateKeyPEM := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}
	privateKey = pem.EncodeToMemory(privateKeyPEM)

	// Generate public key
	pub, err := ssh.NewPublicKey(&key.PublicKey)
	if err != nil {
		return nil, nil, err
	}

	publicKey = ssh.MarshalAuthorizedKey(pub)
	return privateKey, publicKey, nil
}
