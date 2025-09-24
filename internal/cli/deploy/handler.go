package deploy

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/common"
	utils "github.com/iw2rmb/ploy/internal/cli/utils"
)

func PushCmd(args []string, controllerURL string) {
	fs := flag.NewFlagSet("push", flag.ExitOnError)
	app := fs.String("a", filepath.Base(utils.MustGetwd()), "app name")
	lane := fs.String("lane", "", "lane override (A..G)")
	main := fs.String("main", "", "Java main class for lane C")
	sha := fs.String("sha", "", "git sha to annotate")
	bluegreen := fs.Bool("blue-green", false, "use blue-green deployment")
	_ = fs.Parse(args)

	if *bluegreen {
		fmt.Printf("🔄 Starting blue-green deployment for %s...\n", *app)
		fmt.Println("Blue-green deployments are handled via the bluegreen command")
		fmt.Printf("Use: ploy bluegreen deploy %s\n", *app)
		return
	}

	fmt.Printf("🚀 Deploying %s to %s.ployd.app...\n", *app, *app)

	requestedLane := strings.TrimSpace(*lane)
	if requestedLane != "" {
		fmt.Println("ℹ️ Lane overrides are ignored; Docker lane D is always used")
	}

	metadata := map[string]string{}
	if shouldUseAsyncUploads() {
		metadata["async"] = "true"
	}
	if shouldPropagateAutogen() {
		metadata["autogen_dockerfile"] = "true"
	}
	if len(metadata) == 0 {
		metadata = nil
	}

	var respBuf bytes.Buffer
	config := common.DeployConfig{
		App:           *app,
		MainClass:     *main,
		SHA:           *sha,
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

func shouldUseAsyncUploads() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("PLOY_ASYNC")))
	return v == "" || (v != "0" && v != "false" && v != "off")
}

func shouldPropagateAutogen() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("PLOY_AUTOGEN_DOCKERFILE")))
	return v == "1" || v == "true" || v == "on"
}

func shouldUseMultipart() bool {
	return os.Getenv("PLOY_PUSH_MULTIPART") == "1"
}
func OpenCmd(args []string) {
	if len(args) < 1 {
		fmt.Println("usage: ploy open <app>")
		return
	}
	app := args[0]
	domain := utils.DefaultDomainFor(app)
	fmt.Println("Opening:", domain)
	utils.OpenURL("https://" + domain)
}
