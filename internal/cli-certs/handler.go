package clicerts

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

func CertsCmd(args []string, controllerURL string) {
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