package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/iw2rmb/ploy/internal/store"
)

func TestSBOMCompatHandler_ReturnsNullWhenNoStackEvidence(t *testing.T) {
	t.Parallel()

	st := &mockStore{
		hasSBOMEvidenceForStackResult: false,
	}
	handler := sbomCompatHandler(st)
	req := httptest.NewRequest(http.MethodGet, "/v1/sboms/compat?lang=java&release=17&tool=maven&libs=lib-a", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if body := rr.Body.String(); body != "null" {
		t.Fatalf("body = %q, want null", body)
	}
	if st.listSBOMCompatRowsCalled {
		t.Fatal("expected ListSBOMCompatRows not to be called without stack evidence")
	}
}

func TestSBOMCompatHandler_ReturnsMinAndFloorFilteredVersions(t *testing.T) {
	t.Parallel()

	st := &mockStore{
		hasSBOMEvidenceForStackResult: true,
		listSBOMCompatRowsResult: []store.ListSBOMCompatRowsRow{
			{Lib: "lib-a", Ver: "1.0.0"},
			{Lib: "lib-a", Ver: "1.2.0"},
			{Lib: "lib-a", Ver: "2.0.0"},
			{Lib: "lib-b", Ver: "3.5.1"},
		},
	}
	handler := sbomCompatHandler(st)
	req := httptest.NewRequest(http.MethodGet, "/v1/sboms/compat?lang=java&release=17&tool=maven&libs=lib-a:1.1.0,lib-b", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 body=%s", rr.Code, rr.Body.String())
	}

	var got map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got["lib-a"] != "1.2.0" {
		t.Fatalf("lib-a = %q, want 1.2.0", got["lib-a"])
	}
	if got["lib-b"] != "3.5.1" {
		t.Fatalf("lib-b = %q, want 3.5.1", got["lib-b"])
	}
}

func TestSBOMCompatHandler_UsesEcosystemAwareVersionOrdering(t *testing.T) {
	t.Parallel()

	st := &mockStore{
		hasSBOMEvidenceForStackResult: true,
		listSBOMCompatRowsResult: []store.ListSBOMCompatRowsRow{
			{Lib: "lib-a", Ver: "1.2.0"},
			{Lib: "lib-a", Ver: "1.10.0"},
			{Lib: "lib-a", Ver: "2.0.0"},
		},
	}
	handler := sbomCompatHandler(st)
	req := httptest.NewRequest(http.MethodGet, "/v1/sboms/compat?lang=java&release=17&tool=maven&libs=lib-a:1.3.0", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 body=%s", rr.Code, rr.Body.String())
	}
	var got map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got["lib-a"] != "1.10.0" {
		t.Fatalf("lib-a = %q, want 1.10.0", got["lib-a"])
	}
}

func TestSBOMCompatHandler_SupportsMavenCoordinateSelectors(t *testing.T) {
	t.Parallel()

	st := &mockStore{
		hasSBOMEvidenceForStackResult: true,
		listSBOMCompatRowsResult: []store.ListSBOMCompatRowsRow{
			{Lib: "org.slf4j:slf4j-api", Ver: "1.7.36"},
			{Lib: "org.slf4j:slf4j-api", Ver: "2.0.13"},
			{Lib: "ch.qos.logback:logback-classic", Ver: "1.5.6"},
		},
	}
	handler := sbomCompatHandler(st)
	req := httptest.NewRequest(http.MethodGet, "/v1/sboms/compat?lang=java&release=17&tool=maven&libs=org.slf4j:slf4j-api,ch.qos.logback:logback-classic:1.5.0", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 body=%s", rr.Code, rr.Body.String())
	}

	wantLibs := []string{"ch.qos.logback:logback-classic", "org.slf4j", "org.slf4j:slf4j-api"}
	if len(st.listSBOMCompatRowsParams.Libs) != len(wantLibs) {
		t.Fatalf("query libs len = %d, want %d (%v)", len(st.listSBOMCompatRowsParams.Libs), len(wantLibs), st.listSBOMCompatRowsParams.Libs)
	}
	for i := range wantLibs {
		if st.listSBOMCompatRowsParams.Libs[i] != wantLibs[i] {
			t.Fatalf("query libs[%d] = %q, want %q", i, st.listSBOMCompatRowsParams.Libs[i], wantLibs[i])
		}
	}

	var got map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got["org.slf4j:slf4j-api"] != "1.7.36" {
		t.Fatalf("org.slf4j:slf4j-api = %q, want 1.7.36", got["org.slf4j:slf4j-api"])
	}
	if got["ch.qos.logback:logback-classic"] != "1.5.6" {
		t.Fatalf("ch.qos.logback:logback-classic = %q, want 1.5.6", got["ch.qos.logback:logback-classic"])
	}
}

func TestSBOMCompatHandler_SupportsSingleColonLibraryNamesWithoutDot(t *testing.T) {
	t.Parallel()

	st := &mockStore{
		hasSBOMEvidenceForStackResult: true,
		listSBOMCompatRowsResult: []store.ListSBOMCompatRowsRow{
			{Lib: "svc:api", Ver: "1.0.0"},
			{Lib: "svc:api", Ver: "1.2.0"},
			{Lib: "svc", Ver: "0.9.0"},
		},
	}
	handler := sbomCompatHandler(st)
	req := httptest.NewRequest(http.MethodGet, "/v1/sboms/compat?lang=java&release=17&tool=maven&libs=svc:api", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 body=%s", rr.Code, rr.Body.String())
	}

	var got map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got["svc:api"] != "1.0.0" {
		t.Fatalf("svc:api = %q, want 1.0.0", got["svc:api"])
	}
}

func TestSBOMCompatHandler_SupportsFloorForColonLibraryNamesWithoutDot(t *testing.T) {
	t.Parallel()

	st := &mockStore{
		hasSBOMEvidenceForStackResult: true,
		listSBOMCompatRowsResult: []store.ListSBOMCompatRowsRow{
			{Lib: "svc:api", Ver: "1.0.0"},
			{Lib: "svc:api", Ver: "1.2.0"},
			{Lib: "svc", Ver: "0.9.0"},
		},
	}
	handler := sbomCompatHandler(st)
	req := httptest.NewRequest(http.MethodGet, "/v1/sboms/compat?lang=java&release=17&tool=maven&libs=svc:api:1.1.0", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 body=%s", rr.Code, rr.Body.String())
	}

	var got map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got["svc:api"] != "1.2.0" {
		t.Fatalf("svc:api = %q, want 1.2.0", got["svc:api"])
	}
}

func TestSBOMCompatHandler_PrefersExactSingleColonLibraryName(t *testing.T) {
	t.Parallel()

	st := &mockStore{
		hasSBOMEvidenceForStackResult: true,
		listSBOMCompatRowsResult: []store.ListSBOMCompatRowsRow{
			{Lib: "lib-a:1.1.0", Ver: "0.0.2"},
			{Lib: "lib-a", Ver: "1.2.0"},
		},
	}
	handler := sbomCompatHandler(st)
	req := httptest.NewRequest(http.MethodGet, "/v1/sboms/compat?lang=java&release=17&tool=maven&libs=lib-a:1.1.0", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 body=%s", rr.Code, rr.Body.String())
	}

	var got map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got["lib-a:1.1.0"] != "0.0.2" {
		t.Fatalf("lib-a:1.1.0 = %q, want 0.0.2", got["lib-a:1.1.0"])
	}
}

func TestSBOMCompatHandler_ValidatesInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		url  string
	}{
		{
			name: "missing libs",
			url:  "/v1/sboms/compat?lang=java&release=17&tool=maven",
		},
		{
			name: "invalid floor selector",
			url:  "/v1/sboms/compat?lang=java&release=17&tool=maven&libs=lib-a:",
		},
		{
			name: "duplicate selector with conflicting floor",
			url:  "/v1/sboms/compat?lang=java&release=17&tool=maven&libs=lib-a:1.0.0,lib-a:2.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			handler := sbomCompatHandler(&mockStore{})
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", rr.Code)
			}
		})
	}
}
