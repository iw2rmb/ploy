package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/trinodb/trino-go-client/trino"

	"github.com/iw2rmb/ploy/internal/server/auth"
	"github.com/iw2rmb/ploy/internal/server/deployments"
)

type jvmVersionsLookupFunc func(context.Context, deployments.Request) ([][]string, error)

func (f jvmVersionsLookupFunc) Lookup(ctx context.Context, request deployments.Request) ([][]string, error) {
	return f(ctx, request)
}

type jvmVersionsRecorder struct {
	*httptest.ResponseRecorder
	deadline time.Time
}

func (w *jvmVersionsRecorder) SetWriteDeadline(deadline time.Time) error {
	w.deadline = deadline
	return nil
}

func TestParseJVMVersionsRequest(t *testing.T) {
	// Lists normalize only surrounding item whitespace and identical entries.
	query := url.Values{
		"for":  {" example:client,example:starter, example:client "},
		"with": {"baseline:client@1.0+build, baseline:client@1.0+build ,other:client@2.0"},
	}.Encode()
	got, err := parseJVMVersionsRequest("21", query)
	want := deployments.Request{
		For:  []deployments.Coordinate{{Group: "example", Artifact: "client"}, {Group: "example", Artifact: "starter"}},
		With: []deployments.Constraint{{Group: "baseline", Artifact: "client", Version: "1.0+build"}, {Group: "other", Artifact: "client", Version: "2.0"}},
		JVM:  21,
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("parse = %+v, %v; want %+v", got, err, want)
	}
	// The optional list can be absent and a decimal JVM may have leading zeroes.
	got, err = parseJVMVersionsRequest("021", "for=a:b")
	if err != nil || got.JVM != 21 || len(got.With) != 0 {
		t.Fatalf("optional constraints = %+v, %v", got, err)
	}
}

func TestJVMVersionsInvalidRequestsDoNotQuery(t *testing.T) {
	// Every malformed request fails before invoking the lookup.
	queries := []string{
		"", "with=a:b@1", "for=", "for=a:b&jdk=21", "for=a:b&jvm=21",
		"for=a:b&for=a:b", "for=a:b&with=a:b@1&with=a:b@1", "for=a:b&unknown=x",
		"for=a:b&with=", "for=a:b&with=a:b", "for=a:b&with=a:b@", "for=a:b&with=a:b@1,",
		"for=a:b,", "for=,a:b", "for=a:b,,c:d",
		"for=a", "for=:b", "for=a:", "for=a:b:c", "for=a:b@1", "for=a%20:b", "for=%ZZ", "for=a:b;with=a:b@1",
		"for=a:b&with=a:b@1", "for=a:b,c:d&with=c:d@1",
		"for=a:b&with=c:d@1,c:d@2", "for=a:b&with=c:d@1,%20c:d@2%20",
	}
	for _, query := range queries {
		t.Run(query, func(t *testing.T) {
			called := false
			handler := jvmVersionsHandler(jvmVersionsLookupFunc(func(context.Context, deployments.Request) ([][]string, error) {
				called = true
				return nil, nil
			}))
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request.URL.RawQuery = query
			request.SetPathValue("jvm", "21")
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, request)
			if w.Code != http.StatusBadRequest || called {
				t.Fatalf("status = %d, queried = %v; want 400 without query", w.Code, called)
			}
		})
	}
}

func TestJVMVersionsResponses(t *testing.T) {
	// A failed lookup cannot become an empty or partial successful result.
	tests := []struct {
		name    string
		lookup  JVMVersionsLookup
		expired bool
		status  int
		body    string
	}{
		{"unconfigured", nil, false, 503, "Tech Radar is not configured\n"},
		{"no matches", jvmVersionsLookupFunc(func(context.Context, deployments.Request) ([][]string, error) { return [][]string{}, nil }), false, 200, "{\"versions\":[]}\n"},
		{"observed set", jvmVersionsLookupFunc(func(context.Context, deployments.Request) ([][]string, error) {
			return [][]string{{"a:b@1", "a:b@2"}}, nil
		}), false, 200, "{\"versions\":[[\"a:b@1\",\"a:b@2\"]]}\n"},
		{"upstream failure", jvmVersionsLookupFunc(func(context.Context, deployments.Request) ([][]string, error) {
			return [][]string{{"a:b@1"}}, errors.New("secret upstream detail")
		}), false, 502, "Tech Radar lookup failed\n"},
		{"timeout with cancellation failure", jvmVersionsLookupFunc(func(context.Context, deployments.Request) ([][]string, error) {
			return nil, errors.New("secret cancellation detail")
		}), true, 504, "Tech Radar lookup timed out\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/v1/deployments/jvm/21/versions?for=a:b", nil)
			request.SetPathValue("jvm", "21")
			if tt.expired {
				ctx, cancel := context.WithDeadline(request.Context(), time.Now().Add(-time.Second))
				defer cancel()
				request = request.WithContext(ctx)
			}
			w := &jvmVersionsRecorder{ResponseRecorder: httptest.NewRecorder()}
			jvmVersionsHandler(tt.lookup).ServeHTTP(w, request)
			if w.Code != tt.status || w.Body.String() != tt.body {
				t.Fatalf("response = %d %q; want %d %q", w.Code, w.Body.String(), tt.status, tt.body)
			}
		})
	}
}

func TestJVMVersionsDeadlineIncludesDriverCancellation(t *testing.T) {
	// The response budget derives from the effective lookup deadline.
	request := httptest.NewRequest(http.MethodGet, "/v1/deployments/jvm/21/versions?for=a:b", nil)
	request.SetPathValue("jvm", "21")
	w := &jvmVersionsRecorder{ResponseRecorder: httptest.NewRecorder()}
	started := time.Now()
	lookup := jvmVersionsLookupFunc(func(ctx context.Context, _ deployments.Request) ([][]string, error) {
		deadline, ok := ctx.Deadline()
		if !ok || deadline.Before(started.Add(jvmVersionsLookupTimeout)) || deadline.After(time.Now().Add(jvmVersionsLookupTimeout)) {
			t.Errorf("lookup deadline = %v", deadline)
		}
		if want := deadline.Add(2*trino.DefaultCancelQueryTimeout + 5*time.Second); !w.deadline.Equal(want) {
			t.Errorf("write deadline = %v, want %v", w.deadline, want)
		}
		return [][]string{}, nil
	})
	jvmVersionsHandler(lookup).ServeHTTP(w, request)
}

func TestJVMVersionsRouteAuthorization(t *testing.T) {
	// Control-plane and cli-admin roles reach the route; worker access is denied.
	for _, role := range []auth.Role{auth.RoleControlPlane, auth.RoleCLIAdmin, auth.RoleWorker} {
		t.Run(string(role), func(t *testing.T) {
			srv := newTestServerWithRole(t, role)
			w := httptest.NewRecorder()
			srv.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/deployments/jvm/21/versions?for=a:b", nil))
			want := http.StatusServiceUnavailable
			if role == auth.RoleWorker {
				want = http.StatusForbidden
			}
			if w.Code != want {
				t.Fatalf("status = %d, want %d", w.Code, want)
			}
			if strings.Contains(w.Body.String(), "password") {
				t.Fatal("response exposed credentials")
			}
		})
	}
}

func TestJVMVersionsRouteValidatesPathAndQuery(t *testing.T) {
	// Registered routes must extract the JVM before the handler queries Trino.
	srv := newTestServerWithRole(t, auth.RoleControlPlane)
	for _, tt := range []struct {
		name, target string
		status       int
	}{
		{"valid required input", "/v1/deployments/jvm/21/versions?for=a:b", http.StatusServiceUnavailable},
		{"leading zeroes", "/v1/deployments/jvm/021/versions?for=a:b", http.StatusServiceUnavailable},
		{"optional constraints", "/v1/deployments/jvm/21/versions?for=a:b&with=c:d@1", http.StatusServiceUnavailable},
		{"missing for", "/v1/deployments/jvm/21/versions?with=a:b@1", http.StatusBadRequest},
		{"empty for", "/v1/deployments/jvm/21/versions?for=", http.StatusBadRequest},
		{"old query input", "/v1/deployments/jvm/21/versions?for=a:b&jdk=21", http.StatusBadRequest},
		{"duplicate JVM authority", "/v1/deployments/jvm/21/versions?for=a:b&jvm=21", http.StatusBadRequest},
		{"zero JVM", "/v1/deployments/jvm/0/versions?for=a:b", http.StatusBadRequest},
		{"negative JVM", "/v1/deployments/jvm/-1/versions?for=a:b", http.StatusBadRequest},
		{"JVM list", "/v1/deployments/jvm/21,25/versions?for=a:b", http.StatusBadRequest},
		{"decimal JVM", "/v1/deployments/jvm/21.0/versions?for=a:b", http.StatusBadRequest},
		{"signed JVM", "/v1/deployments/jvm/+21/versions?for=a:b", http.StatusBadRequest},
		{"whitespace JVM", "/v1/deployments/jvm/%2021/versions?for=a:b", http.StatusBadRequest},
		{"overflow JVM", "/v1/deployments/jvm/9223372036854775808/versions?for=a:b", http.StatusBadRequest},
		{"nondecimal JVM", "/v1/deployments/jvm/0x15/versions?for=a:b", http.StatusBadRequest},
		{"missing JVM path", "/v1/deployments/jvm/versions?for=a:b", http.StatusNotFound},
		{"retired route", "/v1/deployments/java/versions?for=a:b&jdk=21", http.StatusNotFound},
	} {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			srv.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, tt.target, nil))
			if w.Code != tt.status {
				t.Fatalf("status = %d, want %d", w.Code, tt.status)
			}
		})
	}
}

func TestJVMVersionsMissingPathValueDoesNotQuery(t *testing.T) {
	// Direct callers must supply the path value normally extracted by ServeMux.
	called := false
	handler := jvmVersionsHandler(jvmVersionsLookupFunc(func(context.Context, deployments.Request) ([][]string, error) {
		called = true
		return nil, nil
	}))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/?for=a:b", nil))
	if w.Code != http.StatusBadRequest || called {
		t.Fatalf("status = %d, queried = %v; want 400 without query", w.Code, called)
	}
}
