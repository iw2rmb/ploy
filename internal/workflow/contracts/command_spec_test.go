package contracts

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestCommandSpecJSONBehavior(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		wire    string
		direct  *CommandSpec
		want    []string
		wantErr string
	}{
		{name: "shell string", wire: `" echo hi "`, want: []string{"/bin/sh", "-c", "echo hi"}},
		{name: "exec array", wire: `["echo","hi"]`, want: []string{"echo", "hi"}},
		{name: "empty", direct: &CommandSpec{}},
		{name: "exec takes precedence", direct: &CommandSpec{Shell: "ignored", Exec: []string{"echo", "used"}}, want: []string{"echo", "used"}},
		{name: "invalid array element", wire: `["echo",1]`, wantErr: "command: expected string or array"},
		{name: "invalid type", wire: `42`, wantErr: "command: expected string or array"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var spec CommandSpec
			if tt.direct != nil {
				spec = *tt.direct
			} else if err := json.Unmarshal([]byte(tt.wire), &spec); err != nil {
				if tt.wantErr == "" || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("json.Unmarshal() error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if tt.wantErr != "" {
				t.Fatalf("json.Unmarshal() succeeded, want error containing %q", tt.wantErr)
			}
			if got := spec.ToSlice(); !slices.Equal(got, tt.want) {
				t.Fatalf("ToSlice() = %v, want %v", got, tt.want)
			}
			wire, err := json.Marshal(spec)
			if err != nil {
				t.Fatalf("json.Marshal() error = %v", err)
			}
			var decoded CommandSpec
			if err := json.Unmarshal(wire, &decoded); err != nil {
				t.Fatalf("json roundtrip error = %v", err)
			}
			if got := decoded.ToSlice(); !slices.Equal(got, tt.want) {
				t.Fatalf("roundtrip ToSlice() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCommandSpecYAMLBehavior(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		wire    string
		want    []string
		wantErr bool
	}{
		{name: "shell string", wire: "command: echo hi\n", want: []string{"/bin/sh", "-c", "echo hi"}},
		{name: "exec array", wire: "command: [echo, hi]\n", want: []string{"echo", "hi"}},
		{name: "YAML scalar coercion", wire: "command: [echo, 1]\n", want: []string{"echo", "1"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var decoded struct {
				Command CommandSpec `yaml:"command"`
			}
			err := yaml.Unmarshal([]byte(tt.wire), &decoded)
			if (err != nil) != tt.wantErr {
				t.Fatalf("yaml.Unmarshal() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && !slices.Equal(decoded.Command.ToSlice(), tt.want) {
				t.Fatalf("ToSlice() = %v, want %v", decoded.Command.ToSlice(), tt.want)
			}
		})
	}
}

func TestCommandSpecDecodingReplacesPreviousState(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		decode func(*CommandSpec, string) error
		first  string
		second string
		want   []string
	}{
		{name: "JSON shell to exec", decode: func(c *CommandSpec, value string) error { return json.Unmarshal([]byte(value), c) }, first: `"echo old"`, second: `["echo","new"]`, want: []string{"echo", "new"}},
		{name: "JSON exec to shell", decode: func(c *CommandSpec, value string) error { return json.Unmarshal([]byte(value), c) }, first: `["echo","old"]`, second: `"echo new"`, want: []string{"/bin/sh", "-c", "echo new"}},
		{name: "JSON null reset", decode: func(c *CommandSpec, value string) error { return json.Unmarshal([]byte(value), c) }, first: `["echo","old"]`, second: `null`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var command CommandSpec
			if err := tt.decode(&command, tt.first); err != nil {
				t.Fatalf("decode first value: %v", err)
			}
			if err := tt.decode(&command, tt.second); err != nil {
				t.Fatalf("decode second value: %v", err)
			}
			if got := command.ToSlice(); !slices.Equal(got, tt.want) {
				t.Fatalf("ToSlice() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMigStepYAMLCommandDecodingReplacesPreviousState(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		first  string
		second string
		want   []string
	}{
		{name: "shell to exec", first: "command: echo old\n", second: "command: [echo, new]\n", want: []string{"echo", "new"}},
		{name: "exec to shell", first: "command: [echo, old]\n", second: "command: echo new\n", want: []string{"/bin/sh", "-c", "echo new"}},
		{name: "null reset", first: "command: [echo, old]\n", second: "command: null\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var step MigStep
			if err := yaml.Unmarshal([]byte(tt.first), &step); err != nil {
				t.Fatalf("decode first value: %v", err)
			}
			if err := yaml.Unmarshal([]byte(tt.second), &step); err != nil {
				t.Fatalf("decode second value: %v", err)
			}
			if got := step.Command.ToSlice(); !slices.Equal(got, tt.want) {
				t.Fatalf("ToSlice() = %v, want %v", got, tt.want)
			}
		})
	}
}
