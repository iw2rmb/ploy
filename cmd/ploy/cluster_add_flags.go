package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/iw2rmb/ploy/internal/deploy"
)

func cloneLabelMap(labels labelMap) map[string]string {
	if len(labels) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(labels))
	for k, v := range labels {
		cloned[k] = v
	}
	return cloned
}

type labelMap map[string]string

func (m labelMap) Set(value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return errors.New("label cannot be empty")
	}
	parts := strings.SplitN(trimmed, "=", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid label %q, expected key=value", value)
	}
	key := strings.TrimSpace(parts[0])
	if key == "" {
		return fmt.Errorf("invalid label %q, key required", value)
	}
	val := strings.TrimSpace(parts[1])
	if m == nil {
		return errors.New("label map not initialised")
	}
	m[key] = val
	return nil
}

func (m labelMap) String() string {
	if len(m) == 0 {
		return ""
	}
	pairs := make([]string, 0, len(m))
	for k, v := range m {
		pairs = append(pairs, fmt.Sprintf("%s=%s", k, v))
	}
	return strings.Join(pairs, ",")
}

type probeList []deploy.WorkerHealthProbe

func (p *probeList) Set(value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return errors.New("health probe cannot be empty")
	}
	name := ""
	endpoint := trimmed
	if strings.Contains(trimmed, "=") {
		parts := strings.SplitN(trimmed, "=", 2)
		name = strings.TrimSpace(parts[0])
		endpoint = strings.TrimSpace(parts[1])
	}
	if endpoint == "" {
		return fmt.Errorf("invalid health probe %q, endpoint required", value)
	}
	if name == "" {
		name = fmt.Sprintf("probe-%d", len(*p)+1)
	}
	*p = append(*p, deploy.WorkerHealthProbe{Name: name, Endpoint: endpoint})
	return nil
}

func (p *probeList) String() string {
	if p == nil {
		return ""
	}
	items := make([]string, 0, len(*p))
	for _, probe := range *p {
		items = append(items, fmt.Sprintf("%s=%s", probe.Name, probe.Endpoint))
	}
	return strings.Join(items, ",")
}

type stringValue struct {
	value string
	set   bool
}

func (s *stringValue) Set(value string) error {
	s.value = value
	s.set = true
	return nil
}

func (s *stringValue) String() string {
	return s.value
}

type intValue struct {
	value int
	set   bool
}

func (i *intValue) Set(value string) error {
	v, err := strconv.Atoi(value)
	if err != nil {
		return fmt.Errorf("parse int flag: %w", err)
	}
	i.value = v
	i.set = true
	return nil
}

func (i *intValue) String() string {
	if !i.set {
		return ""
	}
	return strconv.Itoa(i.value)
}
