package certs

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
)

func CertsCmd(args []string, controllerURL string) {
	if len(args) < 1 {
		printUsage()
		return
	}

	action := args[0]
	switch action {
	case "issue":
		handleIssue(args[1:], controllerURL)
	case "issue-wildcard":
		handleIssueWildcard(args[1:], controllerURL)
	case "list":
		handleList(args[1:], controllerURL)
	case "get":
		handleGet(args[1:], controllerURL)
	case "delete":
		handleDelete(args[1:], controllerURL)
	case "renew":
		handleRenew(args[1:], controllerURL)
	case "check":
		handleCheck(args[1:], controllerURL)
	case "status":
		handleStatus(args[1:], controllerURL)
	default:
		printUsage()
	}
}

func printUsage() {
	fmt.Println("usage: ploy certs <command> [options]")
	fmt.Println("")
	fmt.Println("Commands:")
	fmt.Println("  issue <domain1> [domain2...]  Issue certificate for domain(s)")
	fmt.Println("  issue-wildcard <domain>       Issue wildcard certificate")
	fmt.Println("  list                          List all certificates")
	fmt.Println("  get <domain>                  Get certificate details")
	fmt.Println("  delete <domain>               Delete certificate")
	fmt.Println("  renew <domain>                Renew specific certificate")
	fmt.Println("  renew all                     Renew all expiring certificates")
	fmt.Println("  check [--days=30]             Check which certificates need renewal")
	fmt.Println("  status                        Get renewal service status")
	fmt.Println("")
	fmt.Println("Examples:")
	fmt.Println("  ploy certs issue example.com")
	fmt.Println("  ploy certs issue example.com www.example.com")
	fmt.Println("  ploy certs issue-wildcard ployd.app")
	fmt.Println("  ploy certs check --days=7")
}

func handleIssue(args []string, controllerURL string) {
	if len(args) < 1 {
		fmt.Println("usage: ploy certs issue <domain1> [domain2...]")
		return
	}

	domains := args
	domainsJSON := `["` + strings.Join(domains, `","`) + `"]`
	payload := fmt.Sprintf(`{"domains":%s}`, domainsJSON)
	
	url := fmt.Sprintf("%s/v1/certs/issue", controllerURL)
	makeRequest("POST", url, payload)
}

func handleIssueWildcard(args []string, controllerURL string) {
	if len(args) < 1 {
		fmt.Println("usage: ploy certs issue-wildcard <domain>")
		return
	}

	domain := args[0]
	payload := fmt.Sprintf(`{"domain":"%s"}`, domain)
	
	url := fmt.Sprintf("%s/v1/certs/issue/wildcard", controllerURL)
	makeRequest("POST", url, payload)
}

func handleList(args []string, controllerURL string) {
	url := fmt.Sprintf("%s/v1/certs", controllerURL)
	makeRequest("GET", url, "")
}

func handleGet(args []string, controllerURL string) {
	if len(args) < 1 {
		fmt.Println("usage: ploy certs get <domain>")
		return
	}

	domain := args[0]
	url := fmt.Sprintf("%s/v1/certs/%s", controllerURL, domain)
	makeRequest("GET", url, "")
}

func handleDelete(args []string, controllerURL string) {
	if len(args) < 1 {
		fmt.Println("usage: ploy certs delete <domain>")
		return
	}

	domain := args[0]
	url := fmt.Sprintf("%s/v1/certs/%s", controllerURL, domain)
	makeRequest("DELETE", url, "")
}

func handleRenew(args []string, controllerURL string) {
	if len(args) < 1 {
		fmt.Println("usage: ploy certs renew <domain|all>")
		return
	}

	if args[0] == "all" {
		url := fmt.Sprintf("%s/v1/certs/renew/all", controllerURL)
		makeRequest("POST", url, "")
	} else {
		domain := args[0]
		url := fmt.Sprintf("%s/v1/certs/renew/%s", controllerURL, domain)
		makeRequest("POST", url, "")
	}
}

func handleCheck(args []string, controllerURL string) {
	days := "30" // default
	
	// Parse --days flag
	for _, arg := range args {
		if strings.HasPrefix(arg, "--days=") {
			days = strings.TrimPrefix(arg, "--days=")
			if _, err := strconv.Atoi(days); err != nil {
				fmt.Printf("Invalid days value: %s\n", days)
				return
			}
		}
	}

	url := fmt.Sprintf("%s/v1/certs/renew/check?days=%s", controllerURL, days)
	makeRequest("GET", url, "")
}

func handleStatus(args []string, controllerURL string) {
	url := fmt.Sprintf("%s/v1/certs/renewal/status", controllerURL)
	makeRequest("GET", url, "")
}

func makeRequest(method, url, payload string) {
	var resp *http.Response
	var err error

	switch method {
	case "GET":
		resp, err = http.Get(url)
	case "POST":
		resp, err = http.Post(url, "application/json", strings.NewReader(payload))
	case "DELETE":
		req, reqErr := http.NewRequest("DELETE", url, nil)
		if reqErr != nil {
			fmt.Printf("Error creating request: %v\n", reqErr)
			return
		}
		client := &http.Client{}
		resp, err = client.Do(req)
	default:
		fmt.Printf("Unsupported method: %s\n", method)
		return
	}

	if err != nil {
		fmt.Printf("Request error: %v\n", err)
		return
	}
	defer resp.Body.Close()

	// Print response
	io.Copy(os.Stdout, resp.Body)
	fmt.Println() // Add newline for better formatting
}