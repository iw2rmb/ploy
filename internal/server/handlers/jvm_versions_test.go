package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/trinodb/trino-go-client/trino"

	"github.com/iw2rmb/ploy/internal/domain/api"
	"github.com/iw2rmb/ploy/internal/domain/jvmversions"
	"github.com/iw2rmb/ploy/internal/server/auth"
)

type jvmVersionsLookupFunc func(context.Context, jvmversions.Request) ([]jvmversions.Observation, error)

func (f jvmVersionsLookupFunc) Lookup(ctx context.Context, request jvmversions.Request) ([]jvmversions.Observation, error) {
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
	want := jvmversions.Request{
		For: []jvmversions.Coordinate{{Group: "example", Artifact: "client"}, {Group: "example", Artifact: "starter"}},
		With: []jvmversions.Constraint{
			{Coordinate: jvmversions.Coordinate{Group: "baseline", Artifact: "client"}, Selector: jvmversions.Selector{Kind: jvmversions.Exact, Value: "1.0+build"}},
			{Coordinate: jvmversions.Coordinate{Group: "other", Artifact: "client"}, Selector: jvmversions.Selector{Kind: jvmversions.Exact, Value: "2.0"}},
		},
		JVM: 21,
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
		"for=a:b&with=c:d,c:d@1", "for=a:b&with=c:d,c:d@4.x.x",
		"for=a:b&with=c:d@4.x.x,c:d@4.0.x", "for=a:b&with=c:d@4.0.x,c:d@4.0.1",
		"for=a:b&with=c:d@", "for=a:b&with=c:d@4.x", "for=a:b&with=c:d@4.x.7",
		"for=a:b&with=c:d@x", "for=a:b&with=c:d@x.x.x", "for=a:b&with=c:d@4.0.x.extra",
		"for=a:b&with=c:d@4..x", "for=a:b&with=c:d@-4.x.x", "for=a:b&with=c:d@4.-1.x",
		"for=a:b&with=c:d@%2B4.x.x", "for=a:b&with=c:d@4.%2B1.x",
		"for=a:b&with=c:d@%D9%A4.x.x", "for=a:b&with=c:d@4.%D9%A1.x",
	}
	for _, query := range queries {
		t.Run(query, func(t *testing.T) {
			called := false
			handler := jvmVersionsHandler(techRadarJVMVersions, jvmVersionsLookupFunc(func(context.Context, jvmversions.Request) ([]jvmversions.Observation, error) {
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
		{"no matches", jvmVersionsLookupFunc(func(context.Context, jvmversions.Request) ([]jvmversions.Observation, error) {
			return []jvmversions.Observation{}, nil
		}), false, 200, "{\"versions\":[]}\n"},
		{"nil matches", jvmVersionsLookupFunc(func(context.Context, jvmversions.Request) ([]jvmversions.Observation, error) { return nil, nil }), false, 200, "{\"versions\":[]}\n"},
		{"nil observation fields", jvmVersionsLookupFunc(func(context.Context, jvmversions.Request) ([]jvmversions.Observation, error) {
			return []jvmversions.Observation{{}}, nil
		}), false, 200, "{\"versions\":[{\"found\":[],\"with\":[]}]}\n"},
		{"observed set", jvmVersionsLookupFunc(func(context.Context, jvmversions.Request) ([]jvmversions.Observation, error) {
			return []jvmversions.Observation{{Found: []string{"a:b@1", "a:b@2"}}}, nil
		}), false, 200, "{\"versions\":[{\"found\":[\"a:b@1\",\"a:b@2\"],\"with\":[]}]}\n"},
		{"upstream failure", jvmVersionsLookupFunc(func(context.Context, jvmversions.Request) ([]jvmversions.Observation, error) {
			return []jvmversions.Observation{{Found: []string{"a:b@1"}}}, errors.New("secret upstream detail")
		}), false, 502, "Tech Radar lookup failed\n"},
		{"timeout with cancellation failure", jvmVersionsLookupFunc(func(context.Context, jvmversions.Request) ([]jvmversions.Observation, error) {
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
			jvmVersionsHandler(techRadarJVMVersions, tt.lookup).ServeHTTP(w, request)
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
	lookup := jvmVersionsLookupFunc(func(ctx context.Context, _ jvmversions.Request) ([]jvmversions.Observation, error) {
		deadline, ok := ctx.Deadline()
		if !ok || deadline.Before(started.Add(jvmVersionsLookupTimeout)) || deadline.After(time.Now().Add(jvmVersionsLookupTimeout)) {
			t.Errorf("lookup deadline = %v", deadline)
		}
		if want := deadline.Add(2*trino.DefaultCancelQueryTimeout + 5*time.Second); !w.deadline.Equal(want) {
			t.Errorf("write deadline = %v, want %v", w.deadline, want)
		}
		return []jvmversions.Observation{}, nil
	})
	jvmVersionsHandler(techRadarJVMVersions, lookup).ServeHTTP(w, request)
}

func TestJVMVersionsRouteAuthorization(t *testing.T) {
	// Only the deployments route requires Tech Radar configuration.
	for _, role := range []auth.Role{auth.RoleControlPlane, auth.RoleCLIAdmin, auth.RoleWorker, auth.RoleJob} {
		t.Run(string(role), func(t *testing.T) {
			srv := newTestServerWithRole(t, role)
			for source, want := range map[string]int{"deployments": http.StatusServiceUnavailable, "sbom": http.StatusOK} {
				w := &jvmVersionsRecorder{ResponseRecorder: httptest.NewRecorder()}
				srv.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/"+source+"/jvm/21/versions?for=a:b", nil))
				if role == auth.RoleWorker {
					want = http.StatusForbidden
				}
				if w.Code != want {
					t.Errorf("%s status = %d, want %d", source, w.Code, want)
				}
				if strings.Contains(w.Body.String(), "password") {
					t.Fatal("response exposed credentials")
				}
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
	handler := jvmVersionsHandler(techRadarJVMVersions, jvmVersionsLookupFunc(func(context.Context, jvmversions.Request) ([]jvmversions.Observation, error) {
		called = true
		return nil, nil
	}))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/?for=a:b", nil))
	if w.Code != http.StatusBadRequest || called {
		t.Fatalf("status = %d, queried = %v; want 400 without query", w.Code, called)
	}
}

func TestJVMVersionsSelectorsReachLookup(t *testing.T) {
	for _, tt := range []struct {
		name, constraint string
		selector         jvmversions.Selector
	}{
		{"unversioned matches any version", "c:d", jvmversions.Selector{Kind: jvmversions.Any}},
		{"exact retains version", "c:d@4.0.7", jvmversions.Selector{Kind: jvmversions.Exact, Value: "4.0.7"}},
		{"major mask becomes dotted prefix", "c:d@4.x.x", jvmversions.Selector{Kind: jvmversions.Prefix, Value: "4."}},
		{"minor mask becomes dotted prefix", "c:d@4.0.x", jvmversions.Selector{Kind: jvmversions.Prefix, Value: "4.0."}},
		{"major spelling survives", "c:d@004.x.x", jvmversions.Selector{Kind: jvmversions.Prefix, Value: "004."}},
		{"minor spelling survives", "c:d@004.00.x", jvmversions.Selector{Kind: jvmversions.Prefix, Value: "004.00."}},
		{"punctuation stays literal", "c:d@4.%_+'", jvmversions.Selector{Kind: jvmversions.Exact, Value: "4.%_+'"}},
		{"wildcards stay literal", "c:d@4.*", jvmversions.Selector{Kind: jvmversions.Exact, Value: "4.*"}},
		{"range stays literal", "c:d@[4;5)", jvmversions.Selector{Kind: jvmversions.Exact, Value: "[4;5)"}},
		{"uppercase X stays literal", "c:d@4.X.X", jvmversions.Selector{Kind: jvmversions.Exact, Value: "4.X.X"}},
		{"embedded x stays literal", "c:d@4.0.x-RC1", jvmversions.Selector{Kind: jvmversions.Exact, Value: "4.0.x-RC1"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// Repeated identical selectors collapse before the lookup.
			query := url.Values{"for": {"a:b"}, "with": {tt.constraint + ", " + tt.constraint + " "}}.Encode()
			request := httptest.NewRequest(http.MethodGet, "/?"+query, nil)
			request.SetPathValue("jvm", "21")
			called := false
			lookup := jvmVersionsLookupFunc(func(_ context.Context, got jvmversions.Request) ([]jvmversions.Observation, error) {
				called = true
				want := jvmversions.Request{
					JVM: 21, For: []jvmversions.Coordinate{{Group: "a", Artifact: "b"}},
					With: []jvmversions.Constraint{{Coordinate: jvmversions.Coordinate{Group: "c", Artifact: "d"}, Selector: tt.selector}},
				}
				if !reflect.DeepEqual(got, want) {
					t.Errorf("request = %+v, want %+v", got, want)
				}
				return nil, nil
			})
			w := &jvmVersionsRecorder{ResponseRecorder: httptest.NewRecorder()}
			jvmVersionsHandler(techRadarJVMVersions, lookup).ServeHTTP(w, request)
			if w.Code != http.StatusOK || !called {
				t.Fatalf("status = %d, queried = %v; want 200 and lookup", w.Code, called)
			}
		})
	}
}

func TestJVMVersionsNormalizesCompleteObservations(t *testing.T) {
	// Equal found arrays remain distinct when their matching constraints differ.
	lookup := jvmVersionsLookupFunc(func(context.Context, jvmversions.Request) ([]jvmversions.Observation, error) {
		return []jvmversions.Observation{
			{Found: []string{"g:b@2", "g:a@1", "g:a@1"}, With: []string{"c:d@4.0.7", "c:d@4.0.6", "c:d@4.0.7"}},
			{Found: []string{"g:a@1", "g:b@2"}, With: []string{"c:d@4.0.7"}},
			{Found: []string{"g:b@2", "g:a@1"}, With: []string{"c:d@4.0.6", "c:d@4.0.7"}},
			{Found: []string{"g:a@0", "g:b@3"}, With: []string{"c:d@4.0.9"}},
			{Found: []string{"g:a@1", "g:b@1"}, With: []string{"c:d@4.0.8"}},
		}, nil
	})
	request := httptest.NewRequest(http.MethodGet, "/?for=g:a,g:b&with=c:d@4.0.x", nil)
	request.SetPathValue("jvm", "21")
	want := []jvmversions.Observation{
		{Found: []string{"g:a@0", "g:b@3"}, With: []string{"c:d@4.0.9"}},
		{Found: []string{"g:a@1", "g:b@1"}, With: []string{"c:d@4.0.8"}},
		{Found: []string{"g:a@1", "g:b@2"}, With: []string{"c:d@4.0.6", "c:d@4.0.7"}},
		{Found: []string{"g:a@1", "g:b@2"}, With: []string{"c:d@4.0.7"}},
	}
	for _, policy := range []jvmVersionsHTTPPolicy{techRadarJVMVersions, sbomJVMVersions} {
		t.Run(policy.source, func(t *testing.T) {
			w := &jvmVersionsRecorder{ResponseRecorder: httptest.NewRecorder()}
			jvmVersionsHandler(policy, lookup).ServeHTTP(w, request)
			var got api.JVMVersionsResponse
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || w.Code != http.StatusOK {
				t.Fatalf("response = %d %q, %v", w.Code, w.Body.String(), err)
			}
			if !reflect.DeepEqual(got.Versions, want) {
				t.Fatalf("versions = %+v, want %+v", got.Versions, want)
			}
		})
	}
}
