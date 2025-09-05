package env

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

func EnvCmd(args []string, controllerURL string) {
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
