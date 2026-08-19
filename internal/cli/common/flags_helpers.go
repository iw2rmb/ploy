package common

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

// StringValue implements flag.Value with a marker for whether it was set.
type StringValue struct {
	IsSet bool
	Value string
}

func (v *StringValue) String() string     { return v.Value }
func (v *StringValue) Set(s string) error { v.Value = s; v.IsSet = true; return nil }

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
