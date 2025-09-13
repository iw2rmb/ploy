package debug

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

func DebugCmd(args []string, controllerURL string) {
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
	_ = fs.Parse(args[2:])

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
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(os.Stdout, resp.Body)
}

func RollbackCmd(args []string, controllerURL string) {
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
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(os.Stdout, resp.Body)
}
