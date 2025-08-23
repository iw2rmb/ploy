package deploy

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	utils "github.com/iw2rmb/ploy/internal/cli/utils"
)

func PushCmd(args []string, controllerURL string) {
	fs := flag.NewFlagSet("push", flag.ExitOnError)
	app := fs.String("a", filepath.Base(utils.MustGetwd()), "app name")
	lane := fs.String("lane", "", "lane override (A..F)")
	main := fs.String("main", "com.ploy.ordersvc.Main", "Java main class for lane C")
	sha := fs.String("sha", "", "git sha to annotate")
	bluegreen := fs.Bool("blue-green", false, "use blue-green deployment")
	fs.Parse(args)

	if *sha == "" {
		if v := utils.GitSHA(); v != "" {
			*sha = v
		} else {
			*sha = time.Now().Format("20060102-150405")
		}
	}

	// Check if blue-green deployment is requested
	if *bluegreen {
		fmt.Printf("🔄 Starting blue-green deployment for %s...\n", *app)
		fmt.Println("Blue-green deployments are handled via the bluegreen command")
		fmt.Printf("Use: ploy bluegreen deploy %s %s\n", *app, *sha)
		return
	}

	ign, _ := utils.ReadGitignore(".")
	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		_ = utils.TarDir(".", pw, ign)
	}()

	url := fmt.Sprintf("%s/apps/%s/builds?sha=%s&main=%s", controllerURL, *app, *sha, utils.URLQueryEsc(*main))
	if *lane != "" {
		url += "&lane=" + *lane
	}
	req, _ := http.NewRequest("POST", url, pr)
	req.Header.Set("Content-Type", "application/x-tar")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Println("push error:", err)
		return
	}
	defer resp.Body.Close()
	io.Copy(os.Stdout, resp.Body)
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