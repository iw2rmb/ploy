package handlers

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	bsmock "github.com/iw2rmb/ploy/internal/blobstore/mock"
	"github.com/iw2rmb/ploy/internal/gitauth"
	"github.com/iw2rmb/ploy/internal/server/auth"
	"github.com/iw2rmb/ploy/internal/server/blobpersist"
	"github.com/iw2rmb/ploy/internal/server/events"
)

type recordedRoute struct {
	method string
	path   string
}

func (r recordedRoute) String() string {
	return r.method + " " + r.path
}

type recordingRegistrar struct {
	patterns []string
}

func (r *recordingRegistrar) RegisterRouteFunc(pattern string, _ http.HandlerFunc, _ ...auth.Role) {
	r.patterns = append(r.patterns, pattern)
}

func (r *recordingRegistrar) RegisterRouteFuncAllowQueryToken(pattern string, _ http.HandlerFunc, _ ...auth.Role) {
	r.patterns = append(r.patterns, pattern)
}

func TestRegisterRoutesMatchesOpenAPI(t *testing.T) {
	registrar := &recordingRegistrar{}
	st := &handlerStore{}
	bs := bsmock.New()
	bp := blobpersist.New(st, bs)
	eventsService, err := events.NewService(events.Options{})
	if err != nil {
		t.Fatalf("events service: %v", err)
	}
	RegisterRoutes(registrar, st, bs, bp, eventsService, NewConfigHolder(nil), "test-secret", gitauth.Options{}, nil, nil, nil)

	registered, err := registeredRouteSet(registrar.patterns)
	if err != nil {
		t.Fatalf("registered routes: %v", err)
	}
	documented, err := loadOpenAPIRouteSet(filepath.Join("..", "..", "..", "docs", "api", "OpenAPI.yaml"))
	if err != nil {
		t.Fatalf("OpenAPI routes: %v", err)
	}

	undocumented, unregistered := routeSetDifferences(registered, documented)
	if len(undocumented) != 0 || len(unregistered) != 0 {
		t.Fatalf("route contract mismatch\nregistered but undocumented: %v\ndocumented but unregistered: %v", undocumented, unregistered)
	}

	t.Run("detects both drift directions", func(t *testing.T) {
		withExtraRegistration := cloneRouteSet(registered)
		extra := recordedRoute{method: http.MethodPost, path: "/v1/undocumented"}
		withExtraRegistration[extra] = struct{}{}
		gotUndocumented, gotUnregistered := routeSetDifferences(withExtraRegistration, documented)
		if !slices.Equal(gotUndocumented, []recordedRoute{extra}) || len(gotUnregistered) != 0 {
			t.Fatalf("undocumented registration drift = (%v, %v), want ([%s], [])", gotUndocumented, gotUnregistered, extra)
		}

		withoutDocumentedRoute := cloneRouteSet(registered)
		missing := recordedRoute{method: http.MethodGet, path: "/v1/runs"}
		delete(withoutDocumentedRoute, missing)
		gotUndocumented, gotUnregistered = routeSetDifferences(withoutDocumentedRoute, documented)
		if len(gotUndocumented) != 0 || !slices.Equal(gotUnregistered, []recordedRoute{missing}) {
			t.Fatalf("unregistered documentation drift = (%v, %v), want ([], [%s])", gotUndocumented, gotUnregistered, missing)
		}
	})

	t.Run("detects path parameter name drift", func(t *testing.T) {
		runtimeRoute := recordedRoute{method: http.MethodDelete, path: "/v1/migs/{mig_ref}"}
		documentedRoute := recordedRoute{method: http.MethodDelete, path: "/v1/migs/{mig_id}"}
		gotUndocumented, gotUnregistered := routeSetDifferences(
			map[recordedRoute]struct{}{runtimeRoute: {}},
			map[recordedRoute]struct{}{documentedRoute: {}},
		)
		if !slices.Equal(gotUndocumented, []recordedRoute{runtimeRoute}) || !slices.Equal(gotUnregistered, []recordedRoute{documentedRoute}) {
			t.Fatalf("parameter drift = (%v, %v), want ([%s], [%s])", gotUndocumented, gotUnregistered, runtimeRoute, documentedRoute)
		}
	})
}

func registeredRouteSet(patterns []string) (map[recordedRoute]struct{}, error) {
	routes := make(map[recordedRoute]struct{}, len(patterns))
	for _, pattern := range patterns {
		fields := strings.Fields(pattern)
		var method, path string
		switch len(fields) {
		case 1:
			path = fields[0]
			if path != "/health" {
				return nil, &routePatternError{pattern: pattern}
			}
			// /health intentionally remains methodless at runtime for legacy probes.
			method = http.MethodGet
		case 2:
			method, path = strings.ToUpper(fields[0]), fields[1]
		default:
			return nil, &routePatternError{pattern: pattern}
		}
		route := recordedRoute{method: method, path: path}
		if _, exists := routes[route]; exists {
			return nil, fmt.Errorf("duplicate registered route %s", route)
		}
		routes[route] = struct{}{}
	}
	return routes, nil
}

func TestRegisteredRouteSetRejectsDuplicatePatterns(t *testing.T) {
	if _, err := registeredRouteSet([]string{"GET /v1/runs", "GET /v1/runs"}); err == nil {
		t.Fatal("registeredRouteSet() error = nil, want duplicate route error")
	}
}

type routePatternError struct {
	pattern string
}

func (e *routePatternError) Error() string {
	return "unsupported route pattern " + e.pattern
}

func loadOpenAPIRouteSet(specPath string) (map[recordedRoute]struct{}, error) {
	data, err := os.ReadFile(specPath)
	if err != nil {
		return nil, err
	}
	var spec struct {
		Paths map[string]map[string]any `yaml:"paths"`
	}
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, err
	}

	routes := make(map[recordedRoute]struct{})
	for path, pathItem := range spec.Paths {
		methods := pathItem
		if ref, ok := pathItem["$ref"].(string); ok {
			refPath := strings.SplitN(ref, "#", 2)[0]
			refData, err := os.ReadFile(filepath.Join(filepath.Dir(specPath), refPath))
			if err != nil {
				return nil, err
			}
			if err := yaml.Unmarshal(refData, &methods); err != nil {
				return nil, err
			}
		}
		for method := range methods {
			method = strings.ToUpper(method)
			if isHTTPMethod(method) {
				routes[recordedRoute{method: method, path: path}] = struct{}{}
			}
		}
	}
	return routes, nil
}

func isHTTPMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodConnect, http.MethodOptions, http.MethodTrace:
		return true
	default:
		return false
	}
}

func routeSetDifferences(registered, documented map[recordedRoute]struct{}) (undocumented, unregistered []recordedRoute) {
	for route := range registered {
		if _, ok := documented[route]; !ok {
			undocumented = append(undocumented, route)
		}
	}
	for route := range documented {
		if _, ok := registered[route]; !ok {
			unregistered = append(unregistered, route)
		}
	}
	slices.SortFunc(undocumented, compareRecordedRoutes)
	slices.SortFunc(unregistered, compareRecordedRoutes)
	return undocumented, unregistered
}

func compareRecordedRoutes(a, b recordedRoute) int {
	return strings.Compare(a.String(), b.String())
}

func cloneRouteSet(routes map[recordedRoute]struct{}) map[recordedRoute]struct{} {
	clone := make(map[recordedRoute]struct{}, len(routes))
	for route := range routes {
		clone[route] = struct{}{}
	}
	return clone
}
