package platform

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/common"
	utils "github.com/iw2rmb/ploy/internal/cli/utils"
)

// PushCmd handles platform service deployment to ployman.app domain
func PushCmd(args []string, controllerURL string) {
	fs := flag.NewFlagSet("push", flag.ExitOnError)
	service := fs.String("a", "", "platform service name")
	lane := fs.String("lane", "E", "lane override (default: E for containers)")
	sha := fs.String("sha", "", "git sha to annotate")
	env := fs.String("env", "dev", "target environment (dev, staging, prod)")
	_ = fs.Parse(args)

	if *service == "" {
		fmt.Println("Error: platform service name required (-a flag)")
		fmt.Println("Example: ployman push -a ploy-api")
		return
	}

	baseDomain := platformDomainForEnv(*env)
	fmt.Printf("🚀 Deploying platform service %s to %s.%s...\n", *service, *service, baseDomain)

	metadata := map[string]string{}
	if shouldPropagateAutogen() {
		metadata["autogen_dockerfile"] = "true"
	}
	if len(metadata) == 0 {
		metadata = nil
	}

	var respBuf bytes.Buffer
	config := common.DeployConfig{
		App:           *service,
		Lane:          strings.TrimSpace(*lane),
		SHA:           *sha,
		IsPlatform:    true,
		Environment:   strings.ToLower(*env),
		ControllerURL: controllerURL,
		Metadata:      metadata,
		Timeout:       3 * time.Minute,
		UseMultipart:  shouldUseMultipart(),
		Deps: &common.SharedPushDeps{
			Stdout: &respBuf,
		},
	}

	result, err := common.SharedPush(config)
	if err != nil {
		fmt.Printf("❌ Deployment failed: %v\n", err)
		flushControllerResponse(&respBuf)
		return
	}

	if result.Success {
		fmt.Printf("✅ Successfully deployed to %s\n", result.URL)
		if result.DeploymentID != "" {
			fmt.Printf("📋 Deployment ID: %s\n", result.DeploymentID)
		}
	} else {
		fmt.Println("❌ Deployment failed")
	}

	flushControllerResponse(&respBuf)
}

func flushControllerResponse(buf *bytes.Buffer) {
	if buf == nil || buf.Len() == 0 {
		return
	}
	output := buf.String()
	fmt.Print(output)
	if output[len(output)-1] != '\n' {
		fmt.Println()
	}
}
func shouldUseMultipart() bool {
	return os.Getenv("PLOY_PUSH_MULTIPART") == "1"
}

func shouldPropagateAutogen() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("PLOY_AUTOGEN_DOCKERFILE")))
	return v == "1" || v == "true" || v == "on"
}

// OpenCmd opens a platform service in the browser
func OpenCmd(args []string) {
	if len(args) < 1 {
		fmt.Println("usage: ployman open <service>")
		return
	}
	service := args[0]
	domain := getPlatformDomain(service)
	fmt.Println("Opening platform service:", domain)
	utils.OpenURL("https://" + domain)
}

// getPlatformDomain returns the platform domain for a service
func getPlatformDomain(service string) string {
	baseDomain := platformDomainForEnv(os.Getenv("PLOY_ENVIRONMENT"))
	return fmt.Sprintf("%s.%s", service, baseDomain)
}

func platformDomainForEnv(env string) string {
	base := os.Getenv("PLOY_PLATFORM_DOMAIN")
	if base == "" {
		base = "dev.ployman.app"
	}
	env = strings.ToLower(env)
	if env == "" {
		env = "dev"
	}
	if env == "prod" {
		trimmed := strings.TrimPrefix(base, "dev.")
		trimmed = strings.TrimPrefix(trimmed, ".")
		if trimmed == "" {
			trimmed = "ployman.app"
		}
		return trimmed
	}
	return base
}
