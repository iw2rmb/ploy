package main

import (
	"archive/tar"
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

var controllerURL = getenv("PLOY_CONTROLLER", "http://localhost:8081/v1")

func main(){
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "apps": appsCmd(os.Args[2:])
		case "push": pushCmd(os.Args[2:])
		case "open": openCmd(os.Args[2:])
		case "env": envCmd(os.Args[2:])
		case "domains": domainsCmd(os.Args[2:])
		case "certs": certsCmd(os.Args[2:])
		case "debug": debugCmd(os.Args[2:])
		case "rollback": rollbackCmd(os.Args[2:])
		default: usage()
		}
		return
	}
	p := tea.NewProgram(model{})
	if err := p.Start(); err != nil { fmt.Println("error:", err); os.Exit(1) }
}

type model struct{}
func (m model) Init() tea.Cmd { return nil }
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) { return m, nil }
func (m model) View() string { return "Ploy CLI — try `ploy apps new` or `ploy push`\n" }

func usage(){
	fmt.Println(`Ploy CLI
Usage:
  ploy apps new --lang <go|node> --name <app>
  ploy apps destroy --name <app> [--force]
  ploy push -a <app> [-lane A|B|C|D|E|F] [-main com.example.Main] [-sha <sha>]
  ploy open <app>
  ploy env list <app>
  ploy env set <app> <key> <value>
  ploy env get <app> <key>
  ploy env delete <app> <key>
  ploy domains add <app> <domain>
  ploy domains list <app>
  ploy domains remove <app> <domain>
  ploy certs issue <domain>
  ploy certs list
  ploy debug shell <app> [--lane <A-F>]
  ploy rollback <app> <sha>`)
}

func appsCmd(args []string){
	if len(args)>0 && args[0]=="new" {
		fs := flag.NewFlagSet("apps new", flag.ExitOnError)
		lang := fs.String("lang","go","language") ; name := fs.String("name", filepath.Base(mustGetwd()), "name" )
		fs.Parse(args[1:])
		if err := scaffold(*lang, *name); err != nil { fmt.Println("scaffold error:", err); return }
		fmt.Println("Scaffolded app:", *name, "(lang:",*lang,")"); return
	}
	if len(args)>0 && args[0]=="destroy" {
		fs := flag.NewFlagSet("apps destroy", flag.ExitOnError)
		name := fs.String("name", "", "app name to destroy")
		force := fs.Bool("force", false, "skip confirmation prompt")
		fs.Parse(args[1:])
		if *name == "" { fmt.Println("error: --name is required"); return }
		if err := destroyApp(*name, *force); err != nil { fmt.Println("destroy error:", err); return }
		return
	}
	fmt.Println("usage:")
	fmt.Println("  ploy apps new --lang <go|node> --name <app>")
	fmt.Println("  ploy apps destroy --name <app> [--force]")
}

func pushCmd(args []string){
	fs := flag.NewFlagSet("push", flag.ExitOnError)
	app := fs.String("a", filepath.Base(mustGetwd()), "app name" )
	lane := fs.String("lane", "", "lane override (A..F)" )
	main := fs.String("main", "com.ploy.ordersvc.Main", "Java main class for lane C" )
	sha := fs.String("sha", "", "git sha to annotate" )
	fs.Parse(args)

	if *sha == "" { if v := gitSHA(); v != "" { *sha = v } else { *sha = time.Now().Format("20060102-150405") } }

	ign, _ := readGitignore(".")
	pr, pw := io.Pipe()
	go func(){ defer pw.Close(); _ = tarDir(".", pw, ign) }()

	url := fmt.Sprintf("%s/apps/%s/builds?sha=%s&main=%s", controllerURL, *app, *sha, urlQueryEsc(*main))
	if *lane != "" { url += "&lane=" + *lane }
	req, _ := http.NewRequest("POST", url, pr)
	req.Header.Set("Content-Type", "application/x-tar")
	resp, err := http.DefaultClient.Do(req)
	if err != nil { fmt.Println("push error:", err); return }
	defer resp.Body.Close()
	io.Copy(os.Stdout, resp.Body)
}

func openCmd(args []string){
	if len(args)<1 { fmt.Println("usage: ploy open <app>"); return }
	app := args[0]; domain := defaultDomainFor(app)
	fmt.Println("Opening:", domain); openURL("https://"+domain)
}

func defaultDomainFor(app string) string {
	b, err := os.ReadFile(filepath.Join("manifests", app+".yaml"))
	if err == nil {
		lines := strings.Split(string(b), "\n")
		for _, ln := range lines {
			ln = strings.TrimSpace(ln)
			if strings.HasPrefix(ln, "- host:") {
				parts := strings.SplitN(ln, ":", 2); if len(parts)==2 { return strings.TrimSpace(parts[1]) }
			}
		}
	}
	return app+".ployd.app"
}

func openURL(u string) {
	switch runtime.GOOS {
	case "darwin": exec.Command("open", u).Start()
	case "windows": exec.Command("rundll32", "url.dll,FileProtocolHandler", u).Start()
	default: exec.Command("xdg-open", u).Start()
	}
}

func getenv(k, d string) string { if v:=os.Getenv(k); v!="" { return v }; return d }
func mustGetwd() string { wd, _ := os.Getwd(); return wd }
func urlQueryEsc(s string) string { return strings.NewReplacer(" ", "%20", "\n", "%0A", "\t", "%09").Replace(s) }
func gitSHA() string { b, _ := exec.Command("git","rev-parse","--short=12","HEAD").Output(); return strings.TrimSpace(string(b)) }

type ignore struct{ patterns []string; neg []string }
func readGitignore(dir string) (ignore, error) {
	f, err := os.Open(filepath.Join(dir, ".gitignore")); if err != nil { return ignore{}, nil }
	defer f.Close()
	s := bufio.NewScanner(f); var pats, neg []string
	for s.Scan() {
		line := strings.TrimSpace(s.Text()); if line == "" || strings.HasPrefix(line, "#") { continue }
		if strings.HasPrefix(line, "!") { neg = append(neg, strings.TrimPrefix(line, "!")); continue }
		pats = append(pats, line)
	}
	return ignore{patterns:pats, neg:neg}, nil
}

func matchAny(rel string, globs []string) bool {
	for _, g := range globs {
		g = strings.TrimSpace(g)
		if strings.HasSuffix(g, "/") { if strings.HasPrefix(rel, strings.TrimSuffix(g, "/")) { return true }; continue }
		ok, _ := filepath.Match(g, rel); if ok { return true }
		ok, _ = filepath.Match(g, filepath.Base(rel)); if ok { return true }
	}
	return false
}

func tarDir(dir string, w io.Writer, ign ignore) error {
	tw := tar.NewWriter(w); defer tw.Close()
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil { return err }
		rel, _ := filepath.Rel(dir, path)
		if rel == "." { return nil }
		if strings.HasPrefix(rel, ".git") { if info.IsDir() { return filepath.SkipDir }; return nil }
		skipped := matchAny(rel, ign.patterns); if matchAny(rel, ign.neg) { skipped = false }
		if skipped { if info.IsDir() { return filepath.SkipDir }; return nil }
		if info.IsDir() { return nil }
		hdr, _ := tar.FileInfoHeader(info, ""); hdr.Name = rel
		if err := tw.WriteHeader(hdr); err != nil { return err }
		f, _ := os.Open(path); defer f.Close()
		_, err = io.Copy(tw, f); return err
	})
}

func domainsCmd(args []string) {
	if len(args) < 1 {
		fmt.Println("usage: ploy domains <add|list|remove> <app> [domain]")
		return
	}

	action := args[0]
	switch action {
	case "add":
		if len(args) < 3 {
			fmt.Println("usage: ploy domains add <app> <domain>")
			return
		}
		app, domain := args[1], args[2]
		url := fmt.Sprintf("%s/apps/%s/domains", controllerURL, app)
		payload := fmt.Sprintf(`{"domain":"%s"}`, domain)
		resp, err := http.Post(url, "application/json", strings.NewReader(payload))
		if err != nil {
			fmt.Println("domains add error:", err)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
			fmt.Printf("Domain %s added to app %s\n", domain, app)
		} else {
			fmt.Printf("Failed to add domain: HTTP %d\n", resp.StatusCode)
		}

	case "list":
		if len(args) < 2 {
			fmt.Println("usage: ploy domains list <app>")
			return
		}
		app := args[1]
		url := fmt.Sprintf("%s/apps/%s/domains", controllerURL, app)
		resp, err := http.Get(url)
		if err != nil {
			fmt.Println("domains list error:", err)
			return
		}
		defer resp.Body.Close()
		io.Copy(os.Stdout, resp.Body)

	case "remove":
		if len(args) < 3 {
			fmt.Println("usage: ploy domains remove <app> <domain>")
			return
		}
		app, domain := args[1], args[2]
		url := fmt.Sprintf("%s/apps/%s/domains/%s", controllerURL, app, domain)
		req, _ := http.NewRequest("DELETE", url, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			fmt.Println("domains remove error:", err)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
			fmt.Printf("Domain %s removed from app %s\n", domain, app)
		} else {
			fmt.Printf("Failed to remove domain: HTTP %d\n", resp.StatusCode)
		}

	default:
		fmt.Println("usage: ploy domains <add|list|remove> <app> [domain]")
	}
}

func certsCmd(args []string) {
	if len(args) < 1 {
		fmt.Println("usage: ploy certs <issue|list>")
		return
	}

	action := args[0]
	switch action {
	case "issue":
		if len(args) < 2 {
			fmt.Println("usage: ploy certs issue <domain>")
			return
		}
		domain := args[1]
		url := fmt.Sprintf("%s/certs/issue", controllerURL)
		payload := fmt.Sprintf(`{"domain":"%s"}`, domain)
		resp, err := http.Post(url, "application/json", strings.NewReader(payload))
		if err != nil {
			fmt.Println("certs issue error:", err)
			return
		}
		defer resp.Body.Close()
		io.Copy(os.Stdout, resp.Body)

	case "list":
		url := fmt.Sprintf("%s/certs", controllerURL)
		resp, err := http.Get(url)
		if err != nil {
			fmt.Println("certs list error:", err)
			return
		}
		defer resp.Body.Close()
		io.Copy(os.Stdout, resp.Body)

	default:
		fmt.Println("usage: ploy certs <issue|list>")
	}
}

func debugCmd(args []string) {
	if len(args) < 1 {
		fmt.Println("usage: ploy debug shell <app> [--lane <A-F>]")
		return
	}

	if args[0] != "shell" {
		fmt.Println("usage: ploy debug shell <app> [--lane <A-F>]")
		return
	}

	if len(args) < 2 {
		fmt.Println("usage: ploy debug shell <app> [--lane <A-F>]")
		return
	}

	fs := flag.NewFlagSet("debug shell", flag.ExitOnError)
	lane := fs.String("lane", "", "lane override for debug build")
	fs.Parse(args[2:])

	app := args[1]
	url := fmt.Sprintf("%s/apps/%s/debug", controllerURL, app)
	if *lane != "" {
		url += "?lane=" + *lane
	}

	resp, err := http.Post(url, "application/json", strings.NewReader(`{"ssh_enabled":true}`))
	if err != nil {
		fmt.Println("debug shell error:", err)
		return
	}
	defer resp.Body.Close()
	io.Copy(os.Stdout, resp.Body)
}

func rollbackCmd(args []string) {
	if len(args) < 2 {
		fmt.Println("usage: ploy rollback <app> <sha>")
		return
	}

	app, sha := args[0], args[1]
	url := fmt.Sprintf("%s/apps/%s/rollback", controllerURL, app)
	payload := fmt.Sprintf(`{"sha":"%s"}`, sha)
	resp, err := http.Post(url, "application/json", strings.NewReader(payload))
	if err != nil {
		fmt.Println("rollback error:", err)
		return
	}
	defer resp.Body.Close()
	io.Copy(os.Stdout, resp.Body)
}

func envCmd(args []string) {
	if len(args) < 1 {
		fmt.Println("usage: ploy env <list|set|get|delete> <app> [key] [value]")
		return
	}

	action := args[0]
	switch action {
	case "list":
		if len(args) < 2 {
			fmt.Println("usage: ploy env list <app>")
			return
		}
		app := args[1]
		url := fmt.Sprintf("%s/apps/%s/env", controllerURL, app)
		resp, err := http.Get(url)
		if err != nil {
			fmt.Println("env list error:", err)
			return
		}
		defer resp.Body.Close()
		
		if resp.StatusCode != 200 {
			fmt.Printf("Failed to list environment variables: HTTP %d\n", resp.StatusCode)
			return
		}
		
		var result struct {
			App string            `json:"app"`
			Env map[string]string `json:"env"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			fmt.Println("error parsing response:", err)
			return
		}
		
		fmt.Printf("Environment variables for app %s:\n", result.App)
		if len(result.Env) == 0 {
			fmt.Println("  (none)")
		} else {
			for key, value := range result.Env {
				fmt.Printf("  %s=%s\n", key, value)
			}
		}

	case "set":
		if len(args) < 4 {
			fmt.Println("usage: ploy env set <app> <key> <value>")
			return
		}
		app, key, value := args[1], args[2], args[3]
		url := fmt.Sprintf("%s/apps/%s/env/%s", controllerURL, app, key)
		payload := fmt.Sprintf(`{"value":"%s"}`, value)
		req, _ := http.NewRequest("PUT", url, strings.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			fmt.Println("env set error:", err)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
			fmt.Printf("Environment variable %s set for app %s\n", key, app)
		} else {
			fmt.Printf("Failed to set environment variable: HTTP %d\n", resp.StatusCode)
		}

	case "get":
		if len(args) < 3 {
			fmt.Println("usage: ploy env get <app> <key>")
			return
		}
		app, key := args[1], args[2]
		url := fmt.Sprintf("%s/apps/%s/env", controllerURL, app)
		resp, err := http.Get(url)
		if err != nil {
			fmt.Println("env get error:", err)
			return
		}
		defer resp.Body.Close()
		
		var result struct {
			App string            `json:"app"`
			Env map[string]string `json:"env"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			fmt.Println("error parsing response:", err)
			return
		}
		
		if value, exists := result.Env[key]; exists {
			fmt.Printf("%s=%s\n", key, value)
		} else {
			fmt.Printf("Environment variable %s not found for app %s\n", key, app)
		}

	case "delete":
		if len(args) < 3 {
			fmt.Println("usage: ploy env delete <app> <key>")
			return
		}
		app, key := args[1], args[2]
		url := fmt.Sprintf("%s/apps/%s/env/%s", controllerURL, app, key)
		req, _ := http.NewRequest("DELETE", url, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			fmt.Println("env delete error:", err)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
			fmt.Printf("Environment variable %s deleted from app %s\n", key, app)
		} else {
			fmt.Printf("Failed to delete environment variable: HTTP %d\n", resp.StatusCode)
		}

	default:
		fmt.Println("usage: ploy env <list|set|get|delete> <app> [key] [value]")
	}
}

func destroyApp(appName string, force bool) error {
	// Show confirmation prompt unless --force is used
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
	
	// Make API call to destroy the app
	url := fmt.Sprintf("%s/apps/%s", controllerURL, appName)
	if force {
		url += "?force=true"
	}
	
	client := &http.Client{Timeout: 60 * time.Second} // Longer timeout for destroy operations
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
	
	// Parse and display the destroy status
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		fmt.Printf("✅ App '%s' destroyed (raw response: %s)\n", appName, string(body))
		return nil
	}
	
	status, _ := result["status"].(string)
	message, _ := result["message"].(string)
	operations, _ := result["operations"].(map[string]interface{})
	errors, _ := result["errors"].([]interface{})
	
	// Display detailed results
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
