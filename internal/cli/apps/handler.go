package apps

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/utils"
)

func AppsCmd(args []string, controllerURL string) {
	if len(args) > 0 && args[0] == "new" {
		fs := flag.NewFlagSet("apps new", flag.ExitOnError)
		lang := fs.String("lang", "go", "language")
		name := fs.String("name", filepath.Base(utils.MustGetwd()), "name")
		fs.Parse(args[1:])
		if err := scaffold(*lang, *name); err != nil {
			fmt.Println("scaffold error:", err)
			return
		}
		fmt.Println("Scaffolded app:", *name, "(lang:", *lang, ")")
		return
	}
	if len(args) > 0 && args[0] == "destroy" {
		fs := flag.NewFlagSet("apps destroy", flag.ExitOnError)
		name := fs.String("name", "", "app name to destroy")
		force := fs.Bool("force", false, "skip confirmation prompt")
		fs.Parse(args[1:])
		if *name == "" {
			fmt.Println("error: --name is required")
			return
		}
		if err := DestroyApp(*name, *force, controllerURL); err != nil {
			fmt.Println("destroy error:", err)
			return
		}
		return
	}
	fmt.Println("usage:")
	fmt.Println("  ploy apps new --lang <go|node> --name <app>")
	fmt.Println("  ploy apps destroy --name <app> [--force]")
}

func scaffold(lang, name string) error {
	appDir := filepath.Join("apps", name)
	if err := os.MkdirAll(appDir, 0755); err != nil {
		return err
	}
	switch lang {
	case "go":
		return os.WriteFile(filepath.Join(appDir, "main.go"), []byte("package main\nimport (\n\t\"net/http\"\n\t\"log\"\n)\nfunc main(){http.HandleFunc(\"/healthz\", func(w http.ResponseWriter,_ *http.Request){w.Write([]byte(\"ok\"))}); log.Fatal(http.ListenAndServe(\":8080\", nil))}"), 0644)
	case "node":
		_ = os.WriteFile(filepath.Join(appDir, "package.json"), []byte("{\"name\":\""+name+"\",\"version\":\"0.1.0\",\"main\":\"server.js\",\"dependencies\":{\"express\":\"^4\"}}"), 0644)
		return os.WriteFile(filepath.Join(appDir, "server.js"), []byte("const e=require('express')(); e.get('/healthz',(a,b)=>b.send('ok')); e.listen(8080);"), 0644)
	default:
		return fmt.Errorf("unsupported lang: %s", lang)
	}
}

func DestroyApp(appName string, force bool, controllerURL string) error {
	if !force {
		fmt.Printf("⚠️  WARNING: This will permanently destroy the app '%s' and ALL associated resources:\n", appName)
		fmt.Println("   • All running services and deployments")
		fmt.Println("   • Environment variables")
		fmt.Println("   • Domain registrations")
		fmt.Println("   • SSL certificates")
		fmt.Println("   • Container images and storage artifacts")
		fmt.Println("   • Debug instances and SSH keys")
		fmt.Println("   • Build artifacts and temporary files")
		fmt.Println()
		fmt.Print("Are you sure you want to proceed? (type 'yes' to confirm): ")

		reader := bufio.NewReader(os.Stdin)
		confirmation, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read confirmation: %v", err)
		}

		confirmation = strings.TrimSpace(strings.ToLower(confirmation))
		if confirmation != "yes" {
			fmt.Println("❌ Destroy operation cancelled")
			return nil
		}
	}

	fmt.Printf("🗑️  Destroying app '%s'...\n", appName)

	url := fmt.Sprintf("%s/apps/%s", controllerURL, appName)
	if force {
		url += "?force=true"
	}

	client := &http.Client{Timeout: 60 * time.Second}
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create destroy request: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to destroy app: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read destroy response: %v", err)
	}

	if resp.StatusCode != 200 {
		return fmt.Errorf("destroy failed: HTTP %d - %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		fmt.Printf("✅ App '%s' destroyed (raw response: %s)\n", appName, string(body))
		return nil
	}

	status, _ := result["status"].(string)
	message, _ := result["message"].(string)
	operations, _ := result["operations"].(map[string]interface{})
	errors, _ := result["errors"].([]interface{})

	fmt.Printf("📊 Destroy Status: %s\n", status)
	if message != "" {
		fmt.Printf("📝 Message: %s\n", message)
	}

	if operations != nil && len(operations) > 0 {
		fmt.Println("\n🔧 Operations performed:")
		for op, result := range operations {
			fmt.Printf("   • %s: %s\n", op, result)
		}
	}

	if errors != nil && len(errors) > 0 {
		fmt.Println("\n⚠️  Errors encountered:")
		for _, err := range errors {
			fmt.Printf("   • %s\n", err)
		}
	}

	if status == "destroyed" {
		fmt.Printf("\n✅ App '%s' successfully destroyed!\n", appName)
	} else if status == "partially_destroyed" {
		fmt.Printf("\n⚠️  App '%s' partially destroyed - some operations failed\n", appName)
	} else {
		fmt.Printf("\n❌ App '%s' destruction incomplete\n", appName)
	}

	return nil
}