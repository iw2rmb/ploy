package sbom

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

// SBOMCmd is the entrypoint: ploy sbom <subcommand>
func SBOMCmd(args []string, controllerURL string) {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		usage()
		return
	}
	switch args[0] {
	case "generate":
		generate(args[1:], controllerURL)
	case "analyze":
		analyze(args[1:], controllerURL)
	case "compliance":
		compliance(args[1:], controllerURL)
	case "report":
		report(args[1:], controllerURL)
	default:
		usage()
	}
}

func usage() {
	fmt.Println(`Usage: ploy sbom <command> [options]

Commands:
  generate   Generate SBOM for an artifact or source
  analyze    Analyze an SBOM for security issues
  compliance Check SBOM policy compliance
  report     Get SBOM report

Examples:
  ploy sbom generate --target /path/to/app --format spdx-json
  ploy sbom analyze --sbom app.sbom.json
  ploy sbom compliance --sbom app.sbom.json --policy corporate
  ploy sbom report --id sbom-12345`)
}

func generate(args []string, base string) {
	target := ""
	format := "spdx-json"
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--target":
			if i+1 < len(args) {
				target = args[i+1]
				i++
			}
		case "--format":
			if i+1 < len(args) {
				format = args[i+1]
				i++
			}
		}
	}
	if target == "" {
		fmt.Println("--target required")
		os.Exit(1)
	}
	body := map[string]interface{}{
		"artifact": target,
		"format":   format,
	}
	postJSON(base+"/sbom/generate", body)
}

func analyze(args []string, base string) {
	path := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--sbom", "--sbom-file":
			if i+1 < len(args) {
				path = args[i+1]
				i++
			}
		}
	}
	if path == "" {
		fmt.Println("--sbom required")
		os.Exit(1)
	}
	body := map[string]interface{}{"sbom_path": path}
	postJSON(base+"/sbom/analyze", body)
}

func compliance(args []string, base string) {
	sbom := ""
	policy := "default"
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--sbom", "--sbom-file":
			if i+1 < len(args) {
				sbom = args[i+1]
				i++
			}
		case "--policy":
			if i+1 < len(args) {
				policy = args[i+1]
				i++
			}
		}
	}
	if sbom == "" {
		fmt.Println("--sbom required")
		os.Exit(1)
	}
	url := fmt.Sprintf("%s/sbom/compliance?sbom_id=%s&policy=%s", base, sbom, policy)
	doGet(url)
}

func report(args []string, base string) {
	id := ""
	for i := 0; i < len(args); i++ {
		if args[i] == "--id" && i+1 < len(args) {
			id = args[i+1]
			i++
		}
	}
	url := base + "/sbom/report"
	if id != "" {
		url = fmt.Sprintf("%s/sbom/%s", base, id)
	}
	doGet(url)
}

func postJSON(url string, payload map[string]interface{}) {
	b, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("request error:", err)
		os.Exit(1)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(os.Stdout, resp.Body)
	if resp.StatusCode >= 400 {
		os.Exit(1)
	}
}

func doGet(url string) {
	resp, err := http.Get(url)
	if err != nil {
		fmt.Println("request error:", err)
		os.Exit(1)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(os.Stdout, resp.Body)
	if resp.StatusCode >= 400 {
		os.Exit(1)
	}
}
