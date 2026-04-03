package handlers

import (
	"net/http"
	"testing"

	"github.com/iw2rmb/ploy/internal/server/config"
)

// TestConfigCAListReturnsAllEntries verifies that GET /v1/config/ca
// returns all hash+section pairs sorted by section, insertion order within.
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

	// Sorted by section; within section, insertion order.
	if resp[0].Section != "mig" || resp[0].Hash != "abcdef1234567" {
		t.Errorf("entry[0] = %+v, want mig/abcdef1234567", resp[0])
	}
	if resp[1].Section != "mig" || resp[1].Hash != "1111111111111" {
		t.Errorf("entry[1] = %+v, want mig/1111111111111", resp[1])
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

// TestConfigCAPutInvalidHash verifies 400 for invalid hash format.
func TestConfigCAPutInvalidHash(t *testing.T) {
	st := &configStore{}
	holder := NewConfigHolder(config.GitLabConfig{}, nil)

	handler := putConfigCAHandler(holder, st)
	reqBody := map[string]any{"section": "mig"}
	rr := doRequest(t, handler, http.MethodPut, "/v1/config/ca/INVALID", reqBody, "hash", "INVALID")

	assertStatus(t, rr, http.StatusBadRequest)
	if st.upsertConfigCA.called {
		t.Error("store should not be called for invalid hash")
	}
}

// TestConfigCAPutInvalidSection verifies 400 for invalid section.
func TestConfigCAPutInvalidSection(t *testing.T) {
	st := &configStore{}
	holder := NewConfigHolder(config.GitLabConfig{}, nil)

	handler := putConfigCAHandler(holder, st)
	reqBody := map[string]any{"section": "bogus"}
	rr := doRequest(t, handler, http.MethodPut, "/v1/config/ca/abcdef1234567", reqBody, "hash", "abcdef1234567")

	assertStatus(t, rr, http.StatusBadRequest)
	if st.upsertConfigCA.called {
		t.Error("store should not be called for invalid section")
	}
}

// TestConfigCAPutStoreError verifies 500 on store failure.
func TestConfigCAPutStoreError(t *testing.T) {
	st := &configStore{}
	st.upsertConfigCA.err = errMockDatabase
	holder := NewConfigHolder(config.GitLabConfig{}, nil)

	handler := putConfigCAHandler(holder, st)
	reqBody := map[string]any{"section": "mig"}
	rr := doRequest(t, handler, http.MethodPut, "/v1/config/ca/abcdef1234567", reqBody, "hash", "abcdef1234567")

	assertStatus(t, rr, http.StatusInternalServerError)
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

// TestConfigCADeleteMissingSection verifies 400 when section is missing.
func TestConfigCADeleteMissingSection(t *testing.T) {
	st := &configStore{}
	holder := NewConfigHolder(config.GitLabConfig{}, nil)

	handler := deleteConfigCAHandler(holder, st)
	rr := doRequest(t, handler, http.MethodDelete, "/v1/config/ca/abcdef1234567", nil, "hash", "abcdef1234567")

	assertStatus(t, rr, http.StatusBadRequest)
	if st.deleteConfigCA.called {
		t.Error("store should not be called when section is missing")
	}
}

// TestConfigCADeleteStoreError verifies 500 on store failure.
func TestConfigCADeleteStoreError(t *testing.T) {
	st := &configStore{}
	st.deleteConfigCA.err = errMockDatabase
	holder := NewConfigHolder(config.GitLabConfig{}, nil)
	holder.AddConfigCA("mig", "abcdef1234567")

	handler := deleteConfigCAHandler(holder, st)
	rr := doRequest(t, handler, http.MethodDelete, "/v1/config/ca/abcdef1234567?section=mig", nil, "hash", "abcdef1234567")

	assertStatus(t, rr, http.StatusInternalServerError)
}

// TestConfigCADeleteInvalidHash verifies 400 for invalid hash format on delete.
func TestConfigCADeleteInvalidHash(t *testing.T) {
	st := &configStore{}
	holder := NewConfigHolder(config.GitLabConfig{}, nil)

	handler := deleteConfigCAHandler(holder, st)
	rr := doRequest(t, handler, http.MethodDelete, "/v1/config/ca/INVALID?section=mig", nil, "hash", "INVALID")

	assertStatus(t, rr, http.StatusBadRequest)
	if st.deleteConfigCA.called {
		t.Error("store should not be called for invalid hash")
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
