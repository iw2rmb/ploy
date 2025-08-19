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