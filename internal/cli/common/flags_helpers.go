package common

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// StringValue implements flag.Value with a marker for whether it was set.
type StringValue struct {
	IsSet bool
	Value string
}

func (v *StringValue) String() string     { return v.Value }
func (v *StringValue) Set(s string) error { v.Value = s; v.IsSet = true; return nil }

// StringsValue implements flag.Value for accumulating multiple string flag values.
type StringsValue struct {
	Values []string
}

func (v *StringsValue) String() string     { return strings.Join(v.Values, ",") }
func (v *StringsValue) Set(s string) error { v.Values = append(v.Values, s); return nil }

// IntValue implements flag.Value for integers.
type IntValue struct {
	IsSet bool
	Value int
}

func (v *IntValue) String() string { return fmt.Sprintf("%d", v.Value) }
func (v *IntValue) Set(s string) error {
	// Let the FlagSet parse ints instead of re-implementing; keep minimal here.
	var tmp flag.FlagSet
	var parsed int
	tmp.IntVar(&parsed, "v", 0, "")
	if err := tmp.Parse([]string{"-v", s}); err != nil {
		return err
	}
	v.Value = parsed
	v.IsSet = true
	return nil
}

// BoolValue implements flag.Value for booleans.
type BoolValue struct {
	IsSet bool
	Value bool
}

func (v *BoolValue) String() string { return fmt.Sprintf("%t", v.Value) }
func (v *BoolValue) Set(s string) error {
	parsed, err := parseBoolValue(s)
	if err != nil {
		return err
	}
	v.Value = parsed
	v.IsSet = true
	return nil
}
func (v *BoolValue) IsBoolFlag() bool { return true }

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

// ResolveIdentityPath chooses a default SSH identity when not explicitly set.
func ResolveIdentityPath(v StringValue) (string, error) {
	var path string
	if v.IsSet {
		path = ExpandPath(v.Value)
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(home, ".ssh", "id_rsa")
	}
	// Validate the file exists and is readable.
	if err := ValidateFileReadable(path); err != nil {
		return "", fmt.Errorf("identity file: %w", err)
	}
	return path, nil
}

// ResolvePloydBinaryPath locates the ployd binary adjacent to the CLI.
func ResolvePloydBinaryPath(v StringValue) (string, error) {
	if v.IsSet {
		path := ExpandPath(v.Value)
		if err := ValidateFileReadable(path); err != nil {
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

func ExpandPath(path string) string {
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

// ValidateFileReadable checks if a file exists and is readable.
func ValidateFileReadable(path string) error {
	info, err := statFileRooted(path)
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
	f, root, err := openFileRooted(path)
	if err != nil {
		return fmt.Errorf("cannot read file %s: %w", path, err)
	}
	_ = f.Close()
	_ = root.Close()
	return nil
}

// ValidateSSHPort checks if a port number is in a valid range.
func ValidateSSHPort(port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("invalid SSH port %d: must be between 1 and 65535", port)
	}
	return nil
}
