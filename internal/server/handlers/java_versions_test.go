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
	"github.com/iw2rmb/ploy/internal/server/techradar"
)

type javaVersionsLookupFunc func(context.Context, techradar.Request) ([][]string, error)

func (f javaVersionsLookupFunc) Lookup(ctx context.Context, request techradar.Request) ([][]string, error) {
	return f(ctx, request)
}

type javaVersionsRecorder struct {
	*httptest.ResponseRecorder
	deadline time.Time
}

func (w *javaVersionsRecorder) SetWriteDeadline(deadline time.Time) error {
	w.deadline = deadline
	return nil
}

func TestParseJavaVersionsRequest(t *testing.T) {
	// Lists normalize only surrounding item whitespace and identical entries.
	query := url.Values{
		"for":  {" example:client,example:starter, example:client "},
		"with": {"example:client@1.0+build,example:client@1.0+build,example:client@2.0"},
		"jdk":  {"21"},
	}.Encode()
	got, err := parseJavaVersionsRequest(query)
	want := techradar.Request{
		For:  []techradar.Coordinate{{Group: "example", Artifact: "client"}, {Group: "example", Artifact: "starter"}},
		With: []techradar.Constraint{{Group: "example", Artifact: "client", Version: "1.0+build"}, {Group: "example", Artifact: "client", Version: "2.0"}},
		JDK:  21,
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("parse = %+v, %v; want %+v", got, err, want)
	}
	// The optional list can be absent and a decimal JDK may have leading zeroes.
	got, err = parseJavaVersionsRequest("for=a:b&jdk=021")
	if err != nil || got.JDK != 21 || len(got.With) != 0 {
		t.Fatalf("optional constraints = %+v, %v", got, err)
	}
}

func TestJavaVersionsInvalidRequestsDoNotQuery(t *testing.T) {
	// Every malformed request fails before invoking the lookup.
	queries := []string{
		"", "jdk=21", "for=&jdk=21", "for=a:b&jdk=", "for=a:b",
		"for=a:b&jdk=0", "for=a:b&jdk=-1", "for=a:b&jdk=21.0", "for=a:b&jdk=21,25",
		"for=a:b&jdk=+21", "for=a:b&jdk=%2B21", "for=a:b&jdk=9223372036854775808",
		"for=a:b&jdk=21&jdk=21", "for=a:b&for=a:b&jdk=21",
		"for=a:b&with=a:b@1&with=a:b@1&jdk=21", "for=a:b&jdk=21&unknown=x",
		"for=a:b&jdk=21&with=", "for=a:b&jdk=21&with=a:b",
		"for=a:b&jdk=21&with=a:b@", "for=a:b&jdk=21&with=a:b@1,",
		"for=a:b,&jdk=21", "for=,a:b&jdk=21", "for=a:b,,c:d&jdk=21",
		"for=a&jdk=21", "for=:b&jdk=21", "for=a:&jdk=21", "for=a:b:c&jdk=21",
		"for=a:b@1&jdk=21", "for=a%20:b&jdk=21", "for=%ZZ&jdk=21", "for=a:b;jdk=21",
	}
	for _, query := range queries {
		t.Run(query, func(t *testing.T) {
			called := false
			handler := javaVersionsHandler(javaVersionsLookupFunc(func(context.Context, techradar.Request) ([][]string, error) {
				called = true
				return nil, nil
			}))
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request.URL.RawQuery = query
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, request)
			if w.Code != http.StatusBadRequest || called {
				t.Fatalf("status = %d, queried = %v; want 400 without query", w.Code, called)
			}
		})
	}
}

func TestJavaVersionsResponses(t *testing.T) {
	// A failed lookup cannot become an empty or partial successful result.
	tests := []struct {
		name    string
		lookup  JavaVersionsLookup
		expired bool
		status  int
		body    string
	}{
		{"unconfigured", nil, false, 503, "Tech Radar is not configured\n"},
		{"no matches", javaVersionsLookupFunc(func(context.Context, techradar.Request) ([][]string, error) { return [][]string{}, nil }), false, 200, "{\"versions\":[]}\n"},
		{"observed set", javaVersionsLookupFunc(func(context.Context, techradar.Request) ([][]string, error) {
			return [][]string{{"a:b@1", "a:b@2"}}, nil
		}), false, 200, "{\"versions\":[[\"a:b@1\",\"a:b@2\"]]}\n"},
		{"upstream failure", javaVersionsLookupFunc(func(context.Context, techradar.Request) ([][]string, error) {
			return [][]string{{"a:b@1"}}, errors.New("secret upstream detail")
		}), false, 502, "Tech Radar lookup failed\n"},
		{"timeout with cancellation failure", javaVersionsLookupFunc(func(context.Context, techradar.Request) ([][]string, error) {
			return nil, errors.New("secret cancellation detail")
		}), true, 504, "Tech Radar lookup timed out\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/?for=a:b&jdk=21", nil)
			if tt.expired {
				ctx, cancel := context.WithDeadline(request.Context(), time.Now().Add(-time.Second))
				defer cancel()
				request = request.WithContext(ctx)
			}
			w := &javaVersionsRecorder{ResponseRecorder: httptest.NewRecorder()}
			javaVersionsHandler(tt.lookup).ServeHTTP(w, request)
			if w.Code != tt.status || w.Body.String() != tt.body {
				t.Fatalf("response = %d %q; want %d %q", w.Code, w.Body.String(), tt.status, tt.body)
			}
		})
	}
}

func TestJavaVersionsDeadlineIncludesDriverCancellation(t *testing.T) {
	// The response budget derives from the effective lookup deadline.
	request := httptest.NewRequest(http.MethodGet, "/?for=a:b&jdk=21", nil)
	w := &javaVersionsRecorder{ResponseRecorder: httptest.NewRecorder()}
	started := time.Now()
	lookup := javaVersionsLookupFunc(func(ctx context.Context, _ techradar.Request) ([][]string, error) {
		deadline, ok := ctx.Deadline()
		if !ok || deadline.Before(started.Add(javaVersionsLookupTimeout)) || deadline.After(time.Now().Add(javaVersionsLookupTimeout)) {
			t.Errorf("lookup deadline = %v", deadline)
		}
		if want := deadline.Add(2*trino.DefaultCancelQueryTimeout + 5*time.Second); !w.deadline.Equal(want) {
			t.Errorf("write deadline = %v, want %v", w.deadline, want)
		}
		return [][]string{}, nil
	})
	javaVersionsHandler(lookup).ServeHTTP(w, request)
}

func TestJavaVersionsRouteAuthorization(t *testing.T) {
	// Control-plane and cli-admin roles reach the route; worker access is denied.
	for _, role := range []auth.Role{auth.RoleControlPlane, auth.RoleCLIAdmin, auth.RoleWorker} {
		t.Run(string(role), func(t *testing.T) {
			srv := newTestServerWithRole(t, role)
			w := httptest.NewRecorder()
			srv.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/deployments/java/versions?for=a:b&jdk=21", nil))
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
