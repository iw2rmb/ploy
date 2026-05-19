package daemonlog

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	defaultEnv    = "prod"
	defaultSystem = "ploy-server"
	defaultInst   = "ploy.t-tech.team"
	timeFormat    = "2006-01-02T15:04:05.000Z"
)

// Identity is the fixed daemon log envelope identity.
type Identity struct {
	Env    string
	System string
	Inst   string
}

// FromEnv returns the daemon log identity, allowing deployment-specific overrides.
func FromEnv() Identity {
	return Identity{
		Env:    valueOrDefault(os.Getenv("PLOY_LOG_ENV"), defaultEnv),
		System: valueOrDefault(os.Getenv("PLOY_LOG_SYSTEM"), defaultSystem),
		Inst:   valueOrDefault(os.Getenv("PLOY_LOG_INST"), defaultInst),
	}
}

// ConfigureDefault installs the daemon JSON logger as the global slog and log logger.
func ConfigureDefault(stdout, stderr io.Writer, level slog.Leveler, identity Identity) {
	logger := slog.New(NewHandler(stdout, stderr, &slog.HandlerOptions{Level: level}, identity))
	slog.SetDefault(logger)
	log.SetOutput(slog.NewLogLogger(logger.Handler(), slog.LevelInfo).Writer())
	log.SetFlags(0)
}

// NewHandler returns a slog handler that writes daemon JSON lines to stdout/stderr.
func NewHandler(stdout, stderr io.Writer, opts *slog.HandlerOptions, identity Identity) slog.Handler {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	if opts == nil {
		opts = &slog.HandlerOptions{}
	}
	identity = normalizeIdentity(identity)
	return &handler{
		stdout:   stdout,
		stderr:   stderr,
		level:    opts.Level,
		identity: identity,
		mu:       &sync.Mutex{},
	}
}

type handler struct {
	stdout io.Writer
	stderr io.Writer
	level  slog.Leveler

	identity Identity
	attrs    []slog.Attr
	groups   []string

	mu *sync.Mutex
}

func (h *handler) Enabled(_ context.Context, level slog.Level) bool {
	min := slog.LevelInfo
	if h.level != nil {
		min = h.level.Level()
	}
	return level >= min
}

func (h *handler) Handle(_ context.Context, r slog.Record) error {
	fields := map[string]any{
		"env":        h.identity.Env,
		"system":     h.identity.System,
		"inst":       h.identity.Inst,
		"@timestamp": r.Time.UTC().Format(timeFormat),
		"level":      streamLevel(r.Level),
		"msg":        r.Message,
	}

	for _, attr := range h.attrs {
		addAttr(fields, h.groups, attr)
	}
	r.Attrs(func(attr slog.Attr) bool {
		addAttr(fields, h.groups, attr)
		return true
	})

	line, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	line = append(line, '\n')

	h.lock()
	defer h.unlock()
	if r.Level >= slog.LevelError {
		_, err = h.stderr.Write(line)
	} else {
		_, err = h.stdout.Write(line)
	}
	return err
}

func (h *handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	cp := h.clone()
	cp.attrs = append(cp.attrs, attrs...)
	return cp
}

func (h *handler) WithGroup(name string) slog.Handler {
	if strings.TrimSpace(name) == "" {
		return h
	}
	cp := h.clone()
	cp.groups = append(cp.groups, name)
	return cp
}

func (h *handler) clone() *handler {
	cp := *h
	cp.attrs = append([]slog.Attr(nil), h.attrs...)
	cp.groups = append([]string(nil), h.groups...)
	if cp.mu == nil {
		cp.mu = &sync.Mutex{}
	}
	return &cp
}

func (h *handler) lock() {
	h.mu.Lock()
}

func (h *handler) unlock() {
	h.mu.Unlock()
}

func valueOrDefault(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func normalizeIdentity(identity Identity) Identity {
	return Identity{
		Env:    valueOrDefault(identity.Env, defaultEnv),
		System: valueOrDefault(identity.System, defaultSystem),
		Inst:   valueOrDefault(identity.Inst, defaultInst),
	}
}

func streamLevel(level slog.Level) string {
	if level >= slog.LevelError {
		return "ERROR"
	}
	return "INFO"
}

func addAttr(fields map[string]any, groups []string, attr slog.Attr) {
	attr.Value = attr.Value.Resolve()
	if attr.Key == "" {
		return
	}

	if len(groups) == 0 {
		fields[attr.Key] = valueAny(attr.Value)
		return
	}

	dst := fields
	for _, group := range groups {
		next, _ := dst[group].(map[string]any)
		if next == nil {
			next = map[string]any{}
			dst[group] = next
		}
		dst = next
	}
	dst[attr.Key] = valueAny(attr.Value)
}

func valueAny(value slog.Value) any {
	switch value.Kind() {
	case slog.KindAny:
		if err, ok := value.Any().(error); ok {
			return err.Error()
		}
		return value.Any()
	case slog.KindBool:
		return value.Bool()
	case slog.KindDuration:
		return value.Duration().String()
	case slog.KindFloat64:
		return value.Float64()
	case slog.KindInt64:
		return value.Int64()
	case slog.KindString:
		return value.String()
	case slog.KindTime:
		return value.Time().UTC().Format(time.RFC3339Nano)
	case slog.KindUint64:
		return value.Uint64()
	case slog.KindGroup:
		group := map[string]any{}
		for _, attr := range value.Group() {
			addAttr(group, nil, attr)
		}
		return group
	case slog.KindLogValuer:
		return valueAny(value.Resolve())
	default:
		return value.String()
	}
}
