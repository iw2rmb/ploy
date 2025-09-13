package utils

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

type LanePickResult struct {
	Lane     string   `json:"lane"`
	Language string   `json:"language"`
	Reasons  []string `json:"reasons"`
}

func Getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

// ParseIntEnv parses an environment variable as an integer with a default value
func ParseIntEnv(k string, d int) int {
	if v := os.Getenv(k); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return d
}

func FileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func ErrJSON(c *fiber.Ctx, code int, err error) error {
	return c.Status(code).JSON(fiber.Map{"error": err.Error()})
}

func IsHealthy(url string) bool {
	client := &http.Client{Timeout: 1 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return false
	}
	if resp != nil && resp.Body != nil {
		defer func() { _ = resp.Body.Close() }()
	}
	return resp.StatusCode == 200
}

func Untar(tarPath, dst string) error {
	f, err := os.Open(tarPath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	var r io.Reader = f
	if strings.HasSuffix(tarPath, ".gz") {
		gzr, err := gzip.NewReader(f)
		if err != nil {
			return err
		}
		defer func() { _ = gzr.Close() }()
		r = gzr
	}

	tr := tar.NewReader(r)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		p := filepath.Join(dst, h.Name)
		if h.FileInfo().IsDir() {
			if err := os.MkdirAll(p, 0755); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			return err
		}
		out, err := os.Create(p)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, tr); err != nil {
			_ = out.Close()
			return err
		}
		if err := out.Close(); err != nil {
			return err
		}
	}
	return nil
}

func RunLanePick(path string) (LanePickResult, error) {
	// Try to run pre-built lane-pick binary first
	lanePickBinary := "/home/ploy/ploy/bin/lane-pick"

	var cmd *exec.Cmd
	if _, err := os.Stat(lanePickBinary); err == nil {
		// Use pre-built binary if it exists
		cmd = exec.Command(lanePickBinary, "--path", path)
	} else {
		// Fall back to go run with absolute path
		lanePickPath := "/home/ploy/ploy/tools/lane-pick"
		if _, err := os.Stat(lanePickPath); os.IsNotExist(err) {
			// Fall back to relative path for local development
			lanePickPath = "./tools/lane-pick"
		}
		cmd = exec.Command("go", "run", lanePickPath, "--path", path)
	}
	b, err := cmd.Output()
	if err != nil {
		return LanePickResult{}, err
	}

	var res LanePickResult
	if err := json.Unmarshal(b, &res); err != nil {
		return LanePickResult{}, err
	}
	return res, nil
}
