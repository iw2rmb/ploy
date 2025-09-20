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
	"time"
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
		_ = exec.Command("open", u).Start()
	case "windows":
		_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", u).Start()
	default:
		_ = exec.Command("xdg-open", u).Start()
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
	defer func() { _ = f.Close() }()

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

type TarExtra struct {
	Data []byte
	Mode os.FileMode
}

type TarOptions struct {
	Extras map[string]TarExtra
}

func TarDir(dir string, w io.Writer, ign Ignore) error {
	return TarDirWithOptions(dir, w, ign, TarOptions{})
}

func TarDirWithOptions(dir string, w io.Writer, ign Ignore, opts TarOptions) error {
	tw := tar.NewWriter(w)
	defer func() { _ = tw.Close() }()
	if err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
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
		hdr.Name = filepath.ToSlash(rel)
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		f, _ := os.Open(path)
		defer func() { _ = f.Close() }()
		_, err = io.Copy(tw, f)
		return err
	}); err != nil {
		return err
	}
	if len(opts.Extras) == 0 {
		return nil
	}
	for name, extra := range opts.Extras {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			continue
		}
		mode := extra.Mode
		if mode == 0 {
			mode = 0o644
		}
		data := extra.Data
		hdr := &tar.Header{
			Name:    filepath.ToSlash(trimmed),
			Mode:    int64(mode),
			Size:    int64(len(data)),
			ModTime: time.Now(),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if len(data) > 0 {
			if _, err := tw.Write(data); err != nil {
				return err
			}
		}
	}
	return nil
}
