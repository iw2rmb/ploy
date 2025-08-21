package domains

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

func DomainsCmd(args []string, controllerURL string) {
	if len(args) < 1 {
		printUsage()
		return
	}

	action := args[0]
	switch action {
	case "add":
		handleAdd(args[1:], controllerURL)
	case "list":
		handleList(args[1:], controllerURL)
	case "remove":
		handleRemove(args[1:], controllerURL)
	case "certificates":
		handleCertificates(args[1:], controllerURL)
	default:
		printUsage()
	}
}

func printUsage() {
	fmt.Println("usage: ploy domains <command> [options]")
	fmt.Println("")
	fmt.Println("Commands:")
	fmt.Println("  add <app> <domain> [--cert=auto|manual|none]  Add domain to app")
	fmt.Println("  list <app>                                     List domains for app")
	fmt.Println("  remove <app> <domain>                          Remove domain from app")
	fmt.Println("  certificates <app> [list|get|provision|remove] <domain>  Manage certificates")
	fmt.Println("")
	fmt.Println("Certificate options:")
	fmt.Println("  --cert=auto     Automatically provision Let's Encrypt certificate (default)")
	fmt.Println("  --cert=manual   Manual certificate management")
	fmt.Println("  --cert=none     No certificate provisioning")
	fmt.Println("")
	fmt.Println("Examples:")
	fmt.Println("  ploy domains add myapp example.com")
	fmt.Println("  ploy domains add myapp example.com --cert=auto")
	fmt.Println("  ploy domains add myapp example.com --cert=none")
	fmt.Println("  ploy domains certificates myapp list")
	fmt.Println("  ploy domains certificates myapp provision example.com")
}

func handleAdd(args []string, controllerURL string) {
	if len(args) < 2 {
		fmt.Println("usage: ploy domains add <app> <domain> [--cert=auto|manual|none]")
		return
	}
	
	app, domain := args[0], args[1]
	certMode := "auto" // default
	
	// Parse certificate option
	for i := 2; i < len(args); i++ {
		if strings.HasPrefix(args[i], "--cert=") {
			certMode = strings.TrimPrefix(args[i], "--cert=")
		}
	}
	
	// Validate certificate mode
	if certMode != "auto" && certMode != "manual" && certMode != "none" {
		fmt.Printf("Invalid certificate mode: %s. Use auto, manual, or none.\n", certMode)
		return
	}
	
	url := fmt.Sprintf("%s/v1/apps/%s/domains", controllerURL, app)
	payload := fmt.Sprintf(`{"domain":"%s","certificate":"%s"}`, domain, certMode)
	
	resp, err := http.Post(url, "application/json", strings.NewReader(payload))
	if err != nil {
		fmt.Printf("Error adding domain: %v\n", err)
		return
	}
	defer resp.Body.Close()
	
	// Print full response for better feedback
	io.Copy(os.Stdout, resp.Body)
	fmt.Println() // Add newline
}

func handleList(args []string, controllerURL string) {
	if len(args) < 1 {
		fmt.Println("usage: ploy domains list <app>")
		return
	}
	
	app := args[0]
	url := fmt.Sprintf("%s/v1/apps/%s/domains", controllerURL, app)
	
	resp, err := http.Get(url)
	if err != nil {
		fmt.Printf("Error listing domains: %v\n", err)
		return
	}
	defer resp.Body.Close()
	
	io.Copy(os.Stdout, resp.Body)
	fmt.Println() // Add newline
}

func handleRemove(args []string, controllerURL string) {
	if len(args) < 2 {
		fmt.Println("usage: ploy domains remove <app> <domain>")
		return
	}
	
	app, domain := args[0], args[1]
	url := fmt.Sprintf("%s/v1/apps/%s/domains/%s", controllerURL, app, domain)
	
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		fmt.Printf("Error creating request: %v\n", err)
		return
	}
	
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("Error removing domain: %v\n", err)
		return
	}
	defer resp.Body.Close()
	
	io.Copy(os.Stdout, resp.Body)
	fmt.Println() // Add newline
}

func handleCertificates(args []string, controllerURL string) {
	if len(args) < 2 {
		fmt.Println("usage: ploy domains certificates <app> <list|get|provision|remove> [domain]")
		return
	}
	
	app := args[0]
	action := args[1]
	
	switch action {
	case "list":
		url := fmt.Sprintf("%s/v1/apps/%s/certificates", controllerURL, app)
		resp, err := http.Get(url)
		if err != nil {
			fmt.Printf("Error listing certificates: %v\n", err)
			return
		}
		defer resp.Body.Close()
		io.Copy(os.Stdout, resp.Body)
		fmt.Println() // Add newline
		
	case "get":
		if len(args) < 3 {
			fmt.Println("usage: ploy domains certificates <app> get <domain>")
			return
		}
		domain := args[2]
		url := fmt.Sprintf("%s/v1/apps/%s/certificates/%s", controllerURL, app, domain)
		resp, err := http.Get(url)
		if err != nil {
			fmt.Printf("Error getting certificate: %v\n", err)
			return
		}
		defer resp.Body.Close()
		io.Copy(os.Stdout, resp.Body)
		fmt.Println() // Add newline
		
	case "provision":
		if len(args) < 3 {
			fmt.Println("usage: ploy domains certificates <app> provision <domain>")
			return
		}
		domain := args[2]
		url := fmt.Sprintf("%s/v1/apps/%s/certificates/%s/provision", controllerURL, app, domain)
		resp, err := http.Post(url, "application/json", strings.NewReader("{}"))
		if err != nil {
			fmt.Printf("Error provisioning certificate: %v\n", err)
			return
		}
		defer resp.Body.Close()
		io.Copy(os.Stdout, resp.Body)
		fmt.Println() // Add newline
		
	case "remove":
		if len(args) < 3 {
			fmt.Println("usage: ploy domains certificates <app> remove <domain>")
			return
		}
		domain := args[2]
		url := fmt.Sprintf("%s/v1/apps/%s/certificates/%s", controllerURL, app, domain)
		req, err := http.NewRequest("DELETE", url, nil)
		if err != nil {
			fmt.Printf("Error creating request: %v\n", err)
			return
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			fmt.Printf("Error removing certificate: %v\n", err)
			return
		}
		defer resp.Body.Close()
		io.Copy(os.Stdout, resp.Body)
		fmt.Println() // Add newline
		
	default:
		fmt.Println("usage: ploy domains certificates <app> <list|get|provision|remove> [domain]")
	}
}