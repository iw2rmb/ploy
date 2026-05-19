package daemonlog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"regexp"
	"testing"
	"time"
)

func TestHandlerRoutesRecordsAndBuildsEnvelope(t *testing.T) {
	tests := []struct {
		name       string
		level      slog.Level
		wantStdout bool
		wantLevel  string
	}{
		{name: "debug", level: slog.LevelDebug, wantStdout: true, wantLevel: "INFO"},
		{name: "info", level: slog.LevelInfo, wantStdout: true, wantLevel: "INFO"},
		{name: "warn", level: slog.LevelWarn, wantStdout: true, wantLevel: "INFO"},
		{name: "error", level: slog.LevelError, wantStdout: false, wantLevel: "ERROR"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			h := NewHandler(&stdout, &stderr, &slog.HandlerOptions{Level: slog.LevelDebug}, Identity{
				Env:    "stage",
				System: "ploy-test",
				Inst:   "ploy.example",
			})
			rec := slog.NewRecord(time.Date(2026, 5, 19, 12, 34, 56, 123456789, time.FixedZone("MSK", 3*60*60)), tt.level, "hello", 0)
			rec.AddAttrs(slog.String("repo", "demo"), slog.Any("err", errors.New("boom")))

			if err := h.Handle(context.Background(), rec); err != nil {
				t.Fatalf("Handle() error: %v", err)
			}

			gotStdout := stdout.Len() > 0
			if gotStdout != tt.wantStdout {
				t.Fatalf("stdout written = %t, want %t; stdout=%q stderr=%q", gotStdout, tt.wantStdout, stdout.String(), stderr.String())
			}
			line := stdout.String()
			if !tt.wantStdout {
				line = stderr.String()
			}
			frame := decodeFrame(t, line)
			if frame["env"] != "stage" || frame["system"] != "ploy-test" || frame["inst"] != "ploy.example" {
				t.Fatalf("identity fields = env:%v system:%v inst:%v", frame["env"], frame["system"], frame["inst"])
			}
			if frame["@timestamp"] != "2026-05-19T09:34:56.123Z" {
				t.Fatalf("@timestamp = %v", frame["@timestamp"])
			}
			if frame["level"] != tt.wantLevel {
				t.Fatalf("level = %v, want %s", frame["level"], tt.wantLevel)
			}
			if frame["msg"] != "hello" || frame["repo"] != "demo" || frame["err"] != "boom" {
				t.Fatalf("message attrs not preserved: %#v", frame)
			}
		})
	}
}

func TestHandlerAppliesDefaultsAndEnvOverrides(t *testing.T) {
	t.Setenv("PLOY_LOG_ENV", "dev")
	t.Setenv("PLOY_LOG_SYSTEM", "ploy-node")
	t.Setenv("PLOY_LOG_INST", "node-1")

	var stdout, stderr bytes.Buffer
	logger := slog.New(NewHandler(&stdout, &stderr, nil, FromEnv()))
	logger.Info("ready")

	frame := decodeFrame(t, stdout.String())
	if frame["env"] != "dev" || frame["system"] != "ploy-node" || frame["inst"] != "node-1" {
		t.Fatalf("env override identity = %#v", frame)
	}

	stdout.Reset()
	t.Setenv("PLOY_LOG_ENV", "")
	t.Setenv("PLOY_LOG_SYSTEM", "")
	t.Setenv("PLOY_LOG_INST", "")
	logger = slog.New(NewHandler(&stdout, &stderr, nil, FromEnv()))
	logger.Info("ready")

	frame = decodeFrame(t, stdout.String())
	if frame["env"] != defaultEnv || frame["system"] != defaultSystem || frame["inst"] != defaultInst {
		t.Fatalf("default identity = %#v", frame)
	}
}

func TestHandlerFiltersByConfiguredLevel(t *testing.T) {
	var stdout, stderr bytes.Buffer
	logger := slog.New(NewHandler(&stdout, &stderr, &slog.HandlerOptions{Level: slog.LevelWarn}, Identity{}))

	logger.Info("hidden")
	logger.Warn("visible")

	if stdout.String() == "" {
		t.Fatal("expected warn record on stdout")
	}
	if bytes.Contains(stdout.Bytes(), []byte("hidden")) {
		t.Fatalf("info record was not filtered: %q", stdout.String())
	}
}

func TestHandlerTimestampHasMillisecondUTCFormat(t *testing.T) {
	var stdout, stderr bytes.Buffer
	h := NewHandler(&stdout, &stderr, nil, Identity{})
	rec := slog.NewRecord(time.Now(), slog.LevelInfo, "hello", 0)
	if err := h.Handle(context.Background(), rec); err != nil {
		t.Fatalf("Handle() error: %v", err)
	}

	frame := decodeFrame(t, stdout.String())
	matched := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z$`).MatchString(frame["@timestamp"].(string))
	if !matched {
		t.Fatalf("@timestamp has wrong format: %v", frame["@timestamp"])
	}
}

func decodeFrame(t *testing.T, line string) map[string]any {
	t.Helper()
	var frame map[string]any
	if err := json.Unmarshal([]byte(line), &frame); err != nil {
		t.Fatalf("decode frame %q: %v", line, err)
	}
	return frame
}
