package handlers

import (
	"net/http"
	"testing"

	"github.com/iw2rmb/ploy/internal/server/config"
)

// TestConfigCAListReturnsAllEntries verifies that GET /v1/config/ca
// returns all hash+section pairs sorted by section then hash (canonical ordering).
func TestConfigCAListReturnsAllEntries(t *testing.T) {
	holder := NewConfigHolder(config.GitLabConfig{}, nil)
	holder.AddConfigCA("mig", "abcdef1234567")
	holder.AddConfigCA("pre_gate", "1234567890abc")
	holder.AddConfigCA("mig", "1111111111111")

	handler := listConfigCAHandler(holder)
	rr := doRequest(t, handler, http.MethodGet, "/v1/config/ca", nil)

	assertStatus(t, rr, http.StatusOK)

	resp := decodeBody[[]configCAListItem](t, rr)
	if len(resp) != 3 {
		t.Fatalf("got %d entries, want 3", len(resp))
	}

	// Sorted by section, then by hash within section (deterministic canonical ordering).
	if resp[0].Section != "mig" || resp[0].Hash != "1111111111111" {
		t.Errorf("entry[0] = %+v, want mig/1111111111111", resp[0])
	}
	if resp[1].Section != "mig" || resp[1].Hash != "abcdef1234567" {
		t.Errorf("entry[1] = %+v, want mig/abcdef1234567", resp[1])
	}
	if resp[2].Section != "pre_gate" || resp[2].Hash != "1234567890abc" {
		t.Errorf("entry[2] = %+v, want pre_gate/1234567890abc", resp[2])
	}
}

// TestConfigCAListBySectionFilters verifies section-scoped listing.
func TestConfigCAListBySectionFilters(t *testing.T) {
	holder := NewConfigHolder(config.GitLabConfig{}, nil)
	holder.AddConfigCA("mig", "abcdef1234567")
	holder.AddConfigCA("pre_gate", "1234567890abc")

	handler := listConfigCABySectionHandler(holder)
	rr := doRequest(t, handler, http.MethodGet, "/v1/config/ca/mig", nil, "section", "mig")

	assertStatus(t, rr, http.StatusOK)

	resp := decodeBody[[]configCAListItem](t, rr)
	if len(resp) != 1 {
		t.Fatalf("got %d entries, want 1", len(resp))
	}
	if resp[0].Hash != "abcdef1234567" {
		t.Errorf("hash = %q, want abcdef1234567", resp[0].Hash)
	}
}

// TestConfigCAListBySectionInvalidSection verifies 400 for invalid section.
func TestConfigCAListBySectionInvalidSection(t *testing.T) {
	holder := NewConfigHolder(config.GitLabConfig{}, nil)
	handler := listConfigCABySectionHandler(holder)
	rr := doRequest(t, handler, http.MethodGet, "/v1/config/ca/bogus", nil, "section", "bogus")
	assertStatus(t, rr, http.StatusBadRequest)
}

// TestConfigCAPutUpsertsEntry verifies PUT /v1/config/ca/{hash}
// persists to store and updates the holder.
func TestConfigCAPutUpsertsEntry(t *testing.T) {
	st := &configStore{}
	holder := NewConfigHolder(config.GitLabConfig{}, nil)

	handler := putConfigCAHandler(holder, st)
	reqBody := map[string]any{"section": "mig"}
	rr := doRequest(t, handler, http.MethodPut, "/v1/config/ca/abcdef1234567", reqBody, "hash", "abcdef1234567")

	assertStatus(t, rr, http.StatusOK)

	if !st.upsertConfigCA.called {
		t.Error("store.UpsertConfigCA was not called")
	}
	if st.upsertConfigCA.params.Hash != "abcdef1234567" {
		t.Errorf("store Hash = %q, want abcdef1234567", st.upsertConfigCA.params.Hash)
	}
	if st.upsertConfigCA.params.Section != "mig" {
		t.Errorf("store Section = %q, want mig", st.upsertConfigCA.params.Section)
	}

	// Verify holder was updated.
	hashes := holder.GetConfigCA("mig")
	if len(hashes) != 1 || hashes[0] != "abcdef1234567" {
		t.Errorf("holder CA = %v, want [abcdef1234567]", hashes)
	}
}

// TestConfigCAPut_ValidationErrors verifies that PUT /v1/config/ca/{hash}
// returns 400 for invalid inputs and does not call the store.
func TestConfigCAPut_ValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		hash    string
		section string
	}{
		{name: "invalid hash", hash: "INVALID", section: "mig"},
		{name: "invalid section", hash: "abcdef1234567", section: "bogus"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := &configStore{}
			holder := NewConfigHolder(config.GitLabConfig{}, nil)

			handler := putConfigCAHandler(holder, st)
			reqBody := map[string]any{"section": tt.section}
			rr := doRequest(t, handler, http.MethodPut, "/v1/config/ca/"+tt.hash, reqBody, "hash", tt.hash)

			assertStatus(t, rr, http.StatusBadRequest)
			if st.upsertConfigCA.called {
				t.Error("store should not be called for invalid input")
			}
		})
	}
}

// TestConfigCADeleteRemovesEntry verifies DELETE /v1/config/ca/{hash}?section=
// removes from store and holder.
func TestConfigCADeleteRemovesEntry(t *testing.T) {
	st := &configStore{}
	holder := NewConfigHolder(config.GitLabConfig{}, nil)
	holder.AddConfigCA("mig", "abcdef1234567")

	handler := deleteConfigCAHandler(holder, st)
	rr := doRequest(t, handler, http.MethodDelete, "/v1/config/ca/abcdef1234567?section=mig", nil, "hash", "abcdef1234567")

	assertStatus(t, rr, http.StatusNoContent)

	if !st.deleteConfigCA.called {
		t.Error("store.DeleteConfigCA was not called")
	}
	if st.deleteConfigCA.params.Hash != "abcdef1234567" {
		t.Errorf("store Hash = %q, want abcdef1234567", st.deleteConfigCA.params.Hash)
	}
	if st.deleteConfigCA.params.Section != "mig" {
		t.Errorf("store Section = %q, want mig", st.deleteConfigCA.params.Section)
	}

	// Verify holder was updated.
	hashes := holder.GetConfigCA("mig")
	if len(hashes) != 0 {
		t.Errorf("holder CA = %v, want empty", hashes)
	}
}

// TestConfigCADelete_ValidationErrors verifies that DELETE /v1/config/ca/{hash}
// returns 400 for invalid inputs and does not call the store.
func TestConfigCADelete_ValidationErrors(t *testing.T) {
	tests := []struct {
		name string
		hash string
		url  string
	}{
		{name: "invalid hash", hash: "INVALID", url: "/v1/config/ca/INVALID?section=mig"},
		{name: "missing section", hash: "abcdef1234567", url: "/v1/config/ca/abcdef1234567"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := &configStore{}
			holder := NewConfigHolder(config.GitLabConfig{}, nil)

			handler := deleteConfigCAHandler(holder, st)
			rr := doRequest(t, handler, http.MethodDelete, tt.url, nil, "hash", tt.hash)

			assertStatus(t, rr, http.StatusBadRequest)
			if st.deleteConfigCA.called {
				t.Error("store should not be called for invalid input")
			}
		})
	}
}

// TestConfigCA_StoreErrors verifies that store failures return 500.
func TestConfigCA_StoreErrors(t *testing.T) {
	tests := []struct {
		name   string
		method string
		setup  func(*configStore)
		invoke func(*ConfigHolder, *configStore) (http.Handler, string, any, []string)
	}{
		{
			name:   "put store error",
			method: http.MethodPut,
			setup:  func(st *configStore) { st.upsertConfigCA.err = errMockDatabase },
			invoke: func(h *ConfigHolder, st *configStore) (http.Handler, string, any, []string) {
				return putConfigCAHandler(h, st), "/v1/config/ca/abcdef1234567",
					map[string]any{"section": "mig"}, []string{"hash", "abcdef1234567"}
			},
		},
		{
			name:   "delete store error",
			method: http.MethodDelete,
			setup: func(st *configStore) {
				st.deleteConfigCA.err = errMockDatabase
			},
			invoke: func(h *ConfigHolder, st *configStore) (http.Handler, string, any, []string) {
				h.AddConfigCA("mig", "abcdef1234567")
				return deleteConfigCAHandler(h, st), "/v1/config/ca/abcdef1234567?section=mig",
					nil, []string{"hash", "abcdef1234567"}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := &configStore{}
			tt.setup(st)
			holder := NewConfigHolder(config.GitLabConfig{}, nil)

			handler, path, body, params := tt.invoke(holder, st)
			rr := doRequest(t, handler, tt.method, path, body, params...)

			assertStatus(t, rr, http.StatusInternalServerError)
		})
	}
}

// TestConfigCAPutDeduplicates verifies that adding the same hash twice
// to the same section does not produce duplicates.
func TestConfigCAPutDeduplicates(t *testing.T) {
	st := &configStore{}
	holder := NewConfigHolder(config.GitLabConfig{}, nil)

	handler := putConfigCAHandler(holder, st)
	reqBody := map[string]any{"section": "mig"}

	rr := doRequest(t, handler, http.MethodPut, "/v1/config/ca/abcdef1234567", reqBody, "hash", "abcdef1234567")
	assertStatus(t, rr, http.StatusOK)

	rr = doRequest(t, handler, http.MethodPut, "/v1/config/ca/abcdef1234567", reqBody, "hash", "abcdef1234567")
	assertStatus(t, rr, http.StatusOK)

	hashes := holder.GetConfigCA("mig")
	if len(hashes) != 1 {
		t.Errorf("holder CA = %v, want exactly 1 entry", hashes)
	}
}

// TestConfigCAHydraOverlaySync verifies that CA changes are reflected
// in hydra overlays visible to claim mutators.
func TestConfigCAHydraOverlaySync(t *testing.T) {
	holder := NewConfigHolder(config.GitLabConfig{}, nil)
	holder.AddConfigCA("mig", "abcdef1234567")
	holder.AddConfigCA("mig", "1111111111111")

	overlays := holder.GetHydraOverlays()
	if overlays == nil {
		t.Fatal("hydra overlays should not be nil")
	}
	mig := overlays["mig"]
	if mig == nil {
		t.Fatal("mig overlay should not be nil")
	}
	if len(mig.CA) != 2 {
		t.Errorf("mig.CA = %v, want 2 entries", mig.CA)
	}
}
