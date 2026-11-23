package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/domain/jvmversions"
	"github.com/iw2rmb/ploy/internal/store"
)

func TestSBOMJVMVersionsProjectsValidatedRequestAndNormalizesResults(t *testing.T) {
	st := &handlerStore{}
	st.listSBOMJVMVersions.val = []store.ListSBOMJVMVersionsRow{
		{Found: []string{"A:b@2", "A:b@1"}, With: []string{"c:d@1", "e:f@2.3.1", "g:h@4"}},
		{Found: []string{"A:b@1", "A:b@2"}, With: []string{"e:f@2.3.1", "g:h@4", "c:d@1"}},
	}
	r := httptest.NewRequest("GET", "/?for=A:b&with=c:d@1,e:f@2.3.x,g:h", nil)
	r.SetPathValue("jvm", "021")
	w := &jvmVersionsRecorder{ResponseRecorder: httptest.NewRecorder()}
	jvmVersionsHandler(sbomJVMVersions, sbomJVMVersionsLookup{st}).ServeHTTP(w, r)
	want := "{\"versions\":[{\"found\":[\"A:b@1\",\"A:b@2\"],\"with\":[\"c:d@1\",\"e:f@2.3.1\",\"g:h@4\"]}]}\n"
	if w.Code != 200 || w.Body.String() != want {
		t.Fatalf("response = %d %s", w.Code, w.Body)
	}
	params := st.listSBOMJVMVersions.params
	var entries []map[string]string
	if err := json.Unmarshal(params.Requested, &entries); err != nil {
		t.Fatal(err)
	}
	expected := []map[string]string{
		{"lib": "A:b", "kind": "any", "value": "", "membership": "for"},
		{"lib": "c:d", "kind": "exact", "value": "1", "membership": "with"},
		{"lib": "e:f", "kind": "prefix", "value": "2.3.", "membership": "with"},
		{"lib": "g:h", "kind": "any", "value": "", "membership": "with"},
	}
	if params.Jvm != "21" || !reflect.DeepEqual(entries, expected) {
		t.Fatalf("query JVM %q, entries %v", params.Jvm, entries)
	}
}

func TestSBOMJVMVersionsDatabaseFailureAndTimeout(t *testing.T) {
	for _, tt := range []struct {
		name    string
		expired bool
		status  int
		body    string
	}{
		{"database failure", false, 500, "Ploy SBOM lookup failed\n"},
		{"expired request", true, 504, "Ploy SBOM lookup timed out\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			st := &handlerStore{}
			st.listSBOMJVMVersions.val = []store.ListSBOMJVMVersionsRow{{Found: []string{"g:a@1"}}}
			st.listSBOMJVMVersions.err = errors.New("sensitive database error")
			r := httptest.NewRequest("GET", "/?for=g:a", nil)
			r.SetPathValue("jvm", "21")
			if tt.expired {
				ctx, cancel := context.WithDeadline(r.Context(), time.Now().Add(-time.Second))
				defer cancel()
				r = r.WithContext(ctx)
			}
			w := &jvmVersionsRecorder{ResponseRecorder: httptest.NewRecorder()}
			jvmVersionsHandler(sbomJVMVersions, sbomJVMVersionsLookup{st}).ServeHTTP(w, r)
			if w.Code != tt.status || w.Body.String() != tt.body {
				t.Fatalf("response = %d %q", w.Code, w.Body)
			}
		})
	}
}

func TestSBOMJVMVersionsWriteDeadlineAndRequestCancellation(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/?for=g:a", nil)
	r.SetPathValue("jvm", "21")
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	w := &jvmVersionsRecorder{ResponseRecorder: httptest.NewRecorder()}
	start := time.Now()
	lookup := jvmVersionsLookupFunc(func(ctx context.Context, _ jvmversions.Request) ([]jvmversions.Observation, error) {
		deadline, ok := ctx.Deadline()
		if !ok || deadline.Before(start.Add(60*time.Second)) || deadline.After(time.Now().Add(60*time.Second)) || !w.deadline.Equal(deadline.Add(5*time.Second)) {
			t.Fatalf("lookup deadline %v, write deadline %v", deadline, w.deadline)
		}
		cancel()
		if !errors.Is(ctx.Err(), context.Canceled) {
			t.Fatal("lookup context did not observe cancellation")
		}
		return nil, ctx.Err()
	})
	jvmVersionsHandler(sbomJVMVersions, lookup).ServeHTTP(w, r.WithContext(ctx))
	if w.Body.Len() != 0 {
		t.Fatalf("canceled request wrote %q", w.Body)
	}
}

func TestSBOMJVMVersionsUnrepresentablePostgresTextHasNoMatches(t *testing.T) {
	for _, query := range []string{
		"for=a:%00", "for=a:b,%ff:b", "for=a:b&with=c:%00",
		"for=a:b&with=%ff:d", "for=a:b&with=c:d@1%00", "for=a:b&with=c:d@%ff",
	} {
		t.Run(query, func(t *testing.T) {
			st := &handlerStore{}
			r := httptest.NewRequest(http.MethodGet, "/?"+query, nil)
			r.SetPathValue("jvm", "21")
			w := &jvmVersionsRecorder{ResponseRecorder: httptest.NewRecorder()}
			jvmVersionsHandler(sbomJVMVersions, sbomJVMVersionsLookup{st}).ServeHTTP(w, r)
			if w.Code != http.StatusOK || w.Body.String() != "{\"versions\":[]}\n" || st.listSBOMJVMVersions.called {
				t.Fatalf("response = %d %q, queried = %v", w.Code, w.Body, st.listSBOMJVMVersions.called)
			}
		})
	}
}
