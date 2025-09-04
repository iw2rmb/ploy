package utils

import (
	"archive/tar"
	"bufio"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func Getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func MustGetwd() string {
	wd, _ := os.Getwd()
	return wd
}

func URLQueryEsc(s string) string {
	return strings.NewReplacer(" ", "%20", "\n", "%0A", "\t", "%09").Replace(s)
}

func GitSHA() string {
	b, _ := exec.Command("git", "rev-parse", "--short=12", "HEAD").Output()
	return strings.TrimSpace(string(b))
}

func OpenURL(u string) {
	switch runtime.GOOS {
	case "darwin":
		exec.Command("open", u).Start()
	case "windows":
		exec.Command("rundll32", "url.dll,FileProtocolHandler", u).Start()
	default:
		exec.Command("xdg-open", u).Start()
	}
}

func DefaultDomainFor(app string) string {
	return app + ".ployd.app"
}

type Ignore struct {
	Patterns []string
	Neg      []string
}

func ReadGitignore(dir string) (Ignore, error) {
	f, err := os.Open(filepath.Join(dir, ".gitignore"))
	if err != nil {
		return Ignore{}, nil
	}
	defer f.Close()
	
	s := bufio.NewScanner(f)
	var pats, neg []string
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "!") {
			neg = append(neg, strings.TrimPrefix(line, "!"))
			continue
		}
		pats = append(pats, line)
	}
	return Ignore{Patterns: pats, Neg: neg}, nil
}

func MatchAny(rel string, globs []string) bool {
	for _, g := range globs {
		g = strings.TrimSpace(g)
		if strings.HasSuffix(g, "/") {
			if strings.HasPrefix(rel, strings.TrimSuffix(g, "/")) {
				return true
			}
			continue
		}
		ok, _ := filepath.Match(g, rel)
		if ok {
			return true
		}
		ok, _ = filepath.Match(g, filepath.Base(rel))
		if ok {
			return true
		}
	}
	return false
}

func TarDir(dir string, w io.Writer, ign Ignore) error {
	tw := tar.NewWriter(w)
	defer tw.Close()
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		if rel == "." {
			return nil
		}
		if strings.HasPrefix(rel, ".git") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		skipped := MatchAny(rel, ign.Patterns)
		if MatchAny(rel, ign.Neg) {
			skipped = false
		}
		if skipped {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info.IsDir() {
			return nil
		}
		hdr, _ := tar.FileInfoHeader(info, "")
		hdr.Name = rel
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		f, _ := os.Open(path)
		defer f.Close()
		_, err = io.Copy(tw, f)
		return err
	})
}