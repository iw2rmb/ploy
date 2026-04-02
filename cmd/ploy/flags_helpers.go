package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// stringValue implements flag.Value with a marker for whether it was set.
type stringValue struct {
	set   bool
	value string
}

func (v *stringValue) String() string     { return v.value }
func (v *stringValue) Set(s string) error { v.value = s; v.set = true; return nil }

// stringsValue implements flag.Value for accumulating multiple string flag values.
type stringsValue struct {
	values []string
}

func (v *stringsValue) String() string     { return strings.Join(v.values, ",") }
func (v *stringsValue) Set(s string) error { v.values = append(v.values, s); return nil }

// intValue implements flag.Value for integers.
type intValue struct {
	set   bool
	value int
}

func (v *intValue) String() string { return fmt.Sprintf("%d", v.value) }
func (v *intValue) Set(s string) error {
	// Let the FlagSet parse ints instead of re-implementing; keep minimal here.
	var tmp flag.FlagSet
	var parsed int
	tmp.IntVar(&parsed, "v", 0, "")
	if err := tmp.Parse([]string{"-v", s}); err != nil {
		return err
	}
	v.value = parsed
	v.set = true
	return nil
}

// boolValue implements flag.Value for booleans.
type boolValue struct {
	set   bool
	value bool
}

func (v *boolValue) String() string { return fmt.Sprintf("%t", v.value) }
func (v *boolValue) Set(s string) error {
	parsed, err := parseBoolValue(s)
	if err != nil {
		return err
	}
	v.value = parsed
	v.set = true
	return nil
}
func (v *boolValue) IsBoolFlag() bool { return true }

// parseBoolValue parses a boolean string value (1, t, T, TRUE, true, True, 0, f, F, FALSE, false, False).
func parseBoolValue(s string) (bool, error) {
	switch s {
	case "1", "t", "T", "true", "TRUE", "True":
		return true, nil
	case "0", "f", "F", "false", "FALSE", "False":
		return false, nil
	default:
		return false, fmt.Errorf("invalid boolean value: %q", s)
	}
}

// resolveIdentityPath chooses a default SSH identity when not explicitly set.
func resolveIdentityPath(v stringValue) (string, error) {
	var path string
	if v.set {
		path = expandPath(v.value)
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(home, ".ssh", "id_rsa")
	}
	// Validate the file exists and is readable.
	if err := validateFileReadable(path); err != nil {
		return "", fmt.Errorf("identity file: %w", err)
	}
	return path, nil
}

// resolvePloydBinaryPath locates the ployd binary adjacent to the CLI.
func resolvePloydBinaryPath(v stringValue) (string, error) {
	if v.set {
		path := expandPath(v.value)
		if err := validateFileReadable(path); err != nil {
			return "", fmt.Errorf("ployd binary: %w", err)
		}
		return path, nil
	}
	execPath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate ploy executable: %w", err)
	}
	dir := filepath.Dir(execPath)
	osName := runtime.GOOS
	candidates := make([]string, 0, 3)
	if osName != "linux" {
		candidates = append(candidates, filepath.Join(dir, "ployd-linux"))
	}
	if osName == "windows" {
		candidates = append(candidates, filepath.Join(dir, "ployd.exe"))
	}
	candidates = append(candidates, filepath.Join(dir, "ployd"))
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && !info.IsDir() {
			return c, nil
		}
	}
	return "", errors.New("ployd binary not found alongside CLI; provide --binary")
}

func expandPath(path string) string {
	if path == "" {
		return ""
	}
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
		return path
	}
	if len(path) > 2 && path[:2] == "~/" {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

// validateFileReadable checks if a file exists and is readable.
func validateFileReadable(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("file does not exist: %s", path)
		}
		return fmt.Errorf("cannot access file %s: %w", path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("path is a directory, not a file: %s", path)
	}
	// Try to open for reading to verify permissions.
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("cannot read file %s: %w", path, err)
	}
	_ = f.Close()
	return nil
}

// validateSSHPort checks if a port number is in a valid range.
func validateSSHPort(port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("invalid SSH port %d: must be between 1 and 65535", port)
	}
	return nil
}
