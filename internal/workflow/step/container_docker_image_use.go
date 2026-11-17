package step

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/containerd/errdefs"
	"github.com/distribution/reference"
	"github.com/moby/moby/client"
	"github.com/opencontainers/go-digest"
)

// A session retains identities after nested containers remove themselves, so
// their images remain recent through job finalization.
type dockerImageUseSession struct {
	record    func(string)
	reset     func()
	images    sync.Map
	once      sync.Once
	uncertain atomic.Bool
}

func newDockerImageUseSession(record func(string), reset func()) *dockerImageUseSession {
	if record == nil && reset == nil {
		return nil
	}
	return &dockerImageUseSession{record: record, reset: reset}
}

func (s *dockerImageUseSession) note(id string) {
	if s == nil || id == "" {
		return
	}
	s.images.Store(id, struct{}{})
	if s.record != nil {
		s.record(id)
	}
}

func (s *dockerImageUseSession) invalidate() {
	if s != nil {
		s.uncertain.Store(true)
		if s.reset != nil {
			s.reset()
		}
	}
}

func (s *dockerImageUseSession) close() {
	if s == nil {
		return
	}
	s.once.Do(func() {
		if s.uncertain.Load() && s.reset != nil {
			s.reset()
		}
		if s.record != nil {
			s.images.Range(func(key, _ any) bool {
				s.record(key.(string))
				return true
			})
		}
	})
}

func (s *dockerImageUseSession) observeImage(ctx context.Context, images dockerImageAPI, ref string, allowMissing bool) {
	if s == nil {
		return
	}
	if images == nil || ref == "" {
		s.invalidate()
		return
	}
	inspectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	inspected, err := images.ImageInspect(inspectCtx, ref)
	if allowMissing && errdefs.IsNotFound(err) {
		return
	}
	if err != nil || inspected.ID == "" {
		s.invalidate()
		slog.Warn("Docker image usage identity unavailable", "image", ref, "error", err)
		return
	}
	s.note(inspected.ID)
}

func (s *dockerImageUseSession) observeContainer(ctx context.Context, docker dockerClientAPI, images dockerImageAPI, id string) {
	if s == nil {
		return
	}
	inspectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	inspected, err := docker.ContainerInspect(inspectCtx, id, client.ContainerInspectOptions{})
	if err != nil || inspected.Container.Image == "" {
		s.invalidate()
		slog.Warn("Docker container image usage unavailable", "container_id", id, "error", err)
		return
	}
	// Container.Image can be a platform/config identity. ImageInspect returns
	// the same canonical identity that the node's image listing uses.
	s.observeImage(inspectCtx, images, inspected.Container.Image, false)
}

func (s *dockerImageUseSession) inspectCreateResponse(docker *client.Client) func(*http.Response) error {
	return func(response *http.Response) error {
		if response.Request.Method != http.MethodPost || dockerAPIPath(response.Request.URL.Path) != "containers/create" || response.StatusCode != http.StatusCreated {
			return nil
		}
		body, err := io.ReadAll(response.Body)
		_ = response.Body.Close()
		response.Body = io.NopCloser(bytes.NewReader(body))
		if err != nil {
			s.invalidate()
			return err
		}
		var created struct{ ID string }
		if err := json.Unmarshal(body, &created); err != nil || created.ID == "" {
			s.invalidate()
			return nil
		}
		// Resolve before forwarding: the caller may start an AutoRemove container
		// as soon as it receives this response.
		s.observeContainer(response.Request.Context(), docker, docker, created.ID)
		return nil
	}
}

func (s *dockerImageUseSession) requestHandler(docker *client.Client, next http.Handler) http.Handler {
	if s == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			next.ServeHTTP(w, req)
			return
		}
		p := dockerAPIPath(req.URL.Path)
		switch {
		case p == "containers/create":
			body, err := io.ReadAll(req.Body)
			_ = req.Body.Close()
			req.Body = io.NopCloser(bytes.NewReader(body))
			var config struct{ Image string }
			if err != nil || json.Unmarshal(body, &config) != nil {
				s.invalidate()
			} else {
				s.observeImage(req.Context(), docker, config.Image, true)
			}
		case strings.HasPrefix(p, "containers/") && (strings.HasSuffix(p, "/start") || strings.HasSuffix(p, "/restart")):
			id := strings.TrimSuffix(strings.TrimSuffix(strings.TrimPrefix(p, "containers/"), "/start"), "/restart")
			s.observeContainer(req.Context(), docker, docker, id)
		case p == "images/create" && req.URL.Query().Get("fromImage") != "":
			ref := dockerPullImageReference(req)
			if ref == "" {
				s.invalidate()
				defer s.invalidate()
			} else {
				s.observeImage(req.Context(), docker, ref, true)
				defer s.observeImage(context.WithoutCancel(req.Context()), docker, ref, false)
			}
		case p == "build" || p == "images/load" || p == "images/create" || p == "commit":
			// Build inputs, imports, and all-tag pulls do not identify every used
			// image. Reset both ends so long requests retain a full grace period.
			s.invalidate()
			defer s.invalidate()
		}
		next.ServeHTTP(w, req)
	})
}

func dockerPullImageReference(req *http.Request) string {
	name, err := reference.ParseNormalizedNamed(req.URL.Query().Get("fromImage"))
	if err != nil {
		return ""
	}
	tag := req.URL.Query().Get("tag")
	if tag == "" {
		if _, ok := name.(reference.Tagged); !ok {
			if _, ok := name.(reference.Digested); !ok {
				return ""
			}
		}
	} else if parsed, err := digest.Parse(tag); err == nil {
		name, err = reference.WithDigest(reference.TrimNamed(name), parsed)
		if err != nil {
			return ""
		}
	} else {
		name, err = reference.WithTag(reference.TrimNamed(name), tag)
		if err != nil {
			return ""
		}
	}
	return reference.FamiliarString(name)
}
