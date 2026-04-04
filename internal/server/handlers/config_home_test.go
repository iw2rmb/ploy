package handlers

import (
	"net/http"
	"testing"

	"github.com/iw2rmb/ploy/internal/server/config"
)

// TestConfigHomeListReturnsAllEntries verifies that GET /v1/config/home
// returns all entries sorted by section then dst.
func TestConfigHomeListReturnsAllEntries(t *testing.T) {
	holder := NewConfigHolder(config.GitLabConfig{}, nil)
	holder.AddConfigHome("mig", ConfigHomeEntry{Entry: "abcdef1:.config/app", Dst: ".config/app", Section: "mig"})
	holder.AddConfigHome("pre_gate", ConfigHomeEntry{Entry: "1234567:.ssh", Dst: ".ssh", Section: "pre_gate"})
	holder.AddConfigHome("mig", ConfigHomeEntry{Entry: "bbbbbbb:.local/bin", Dst: ".local/bin", Section: "mig"})

	handler := listConfigHomeHandler(holder)
	rr := doRequest(t, handler, http.MethodGet, "/v1/config/home", nil)

	assertStatus(t, rr, http.StatusOK)

	resp := decodeBody[[]configHomeListItem](t, rr)
	if len(resp) != 3 {
		t.Fatalf("got %d entries, want 3", len(resp))
	}

	// Sorted by section, then by dst within section (deterministic canonical ordering).
	if resp[0].Section != "mig" || resp[0].Dst != ".config/app" {
		t.Errorf("entry[0] = %+v, want mig/.config/app", resp[0])
	}
	if resp[1].Section != "mig" || resp[1].Dst != ".local/bin" {
		t.Errorf("entry[1] = %+v, want mig/.local/bin", resp[1])
	}
	if resp[2].Section != "pre_gate" || resp[2].Dst != ".ssh" {
		t.Errorf("entry[2] = %+v, want pre_gate/.ssh", resp[2])
	}
}

// TestConfigHomeListBySectionFilters verifies section-scoped listing.
func TestConfigHomeListBySectionFilters(t *testing.T) {
	holder := NewConfigHolder(config.GitLabConfig{}, nil)
	holder.AddConfigHome("mig", ConfigHomeEntry{Entry: "abcdef1:.config/app", Dst: ".config/app", Section: "mig"})
	holder.AddConfigHome("pre_gate", ConfigHomeEntry{Entry: "1234567:.ssh", Dst: ".ssh", Section: "pre_gate"})

	handler := listConfigHomeBySectionHandler(holder)
	rr := doRequest(t, handler, http.MethodGet, "/v1/config/home/mig", nil, "section", "mig")

	assertStatus(t, rr, http.StatusOK)

	resp := decodeBody[[]configHomeListItem](t, rr)
	if len(resp) != 1 {
		t.Fatalf("got %d entries, want 1", len(resp))
	}
	if resp[0].Dst != ".config/app" {
		t.Errorf("dst = %q, want .config/app", resp[0].Dst)
	}
}

// TestConfigHomeListBySectionInvalidSection verifies 400 for invalid section.
func TestConfigHomeListBySectionInvalidSection(t *testing.T) {
	holder := NewConfigHolder(config.GitLabConfig{}, nil)
	handler := listConfigHomeBySectionHandler(holder)
	rr := doRequest(t, handler, http.MethodGet, "/v1/config/home/bogus", nil, "section", "bogus")
	assertStatus(t, rr, http.StatusBadRequest)
}

// TestConfigHomePutUpsertsEntry verifies PUT /v1/config/home
// persists to store and updates the holder.
func TestConfigHomePutUpsertsEntry(t *testing.T) {
	st := &configStore{}
	holder := NewConfigHolder(config.GitLabConfig{}, nil)

	handler := putConfigHomeHandler(holder, st)
	reqBody := map[string]any{"entry": "abcdef1:.config/app", "section": "mig"}
	rr := doRequest(t, handler, http.MethodPut, "/v1/config/home", reqBody)

	assertStatus(t, rr, http.StatusOK)

	if !st.upsertConfigHome.called {
		t.Error("store.UpsertConfigHome was not called")
	}
	if st.upsertConfigHome.params.Dst != ".config/app" {
		t.Errorf("store Dst = %q, want .config/app", st.upsertConfigHome.params.Dst)
	}
	if st.upsertConfigHome.params.Section != "mig" {
		t.Errorf("store Section = %q, want mig", st.upsertConfigHome.params.Section)
	}
	if st.upsertConfigHome.params.Entry != "abcdef1:.config/app" {
		t.Errorf("store Entry = %q, want abcdef1:.config/app", st.upsertConfigHome.params.Entry)
	}

	// Verify holder was updated.
	entries := holder.GetConfigHome("mig")
	if len(entries) != 1 || entries[0].Dst != ".config/app" {
		t.Errorf("holder Home = %v, want [{Dst:.config/app}]", entries)
	}
}

// TestConfigHomePut_ValidationErrors verifies that PUT /v1/config/home
// returns 400 for invalid inputs and does not call the store.
func TestConfigHomePut_ValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		entry   string
		section string
	}{
		{name: "invalid entry", entry: "INVALID", section: "mig"},
		{name: "invalid section", entry: "abcdef1:.config/app", section: "bogus"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := &configStore{}
			holder := NewConfigHolder(config.GitLabConfig{}, nil)

			handler := putConfigHomeHandler(holder, st)
			reqBody := map[string]any{"entry": tt.entry, "section": tt.section}
			rr := doRequest(t, handler, http.MethodPut, "/v1/config/home", reqBody)

			assertStatus(t, rr, http.StatusBadRequest)
			if st.upsertConfigHome.called {
				t.Error("store should not be called for invalid input")
			}
		})
	}
}

// TestConfigHomeDeleteRemovesEntry verifies DELETE /v1/config/home?dst=&section=
// removes from store and holder.
func TestConfigHomeDeleteRemovesEntry(t *testing.T) {
	st := &configStore{}
	holder := NewConfigHolder(config.GitLabConfig{}, nil)
	holder.AddConfigHome("mig", ConfigHomeEntry{Entry: "abcdef1:.config/app", Dst: ".config/app", Section: "mig"})

	handler := deleteConfigHomeHandler(holder, st)
	rr := doRequest(t, handler, http.MethodDelete, "/v1/config/home?dst=.config/app&section=mig", nil)

	assertStatus(t, rr, http.StatusNoContent)

	if !st.deleteConfigHome.called {
		t.Error("store.DeleteConfigHome was not called")
	}
	if st.deleteConfigHome.params.Dst != ".config/app" {
		t.Errorf("store Dst = %q, want .config/app", st.deleteConfigHome.params.Dst)
	}
	if st.deleteConfigHome.params.Section != "mig" {
		t.Errorf("store Section = %q, want mig", st.deleteConfigHome.params.Section)
	}

	// Verify holder was updated.
	entries := holder.GetConfigHome("mig")
	if len(entries) != 0 {
		t.Errorf("holder Home = %v, want empty", entries)
	}
}

// TestConfigHomeDelete_ValidationErrors verifies that DELETE /v1/config/home
// returns 400 for invalid or missing inputs and does not call the store.
func TestConfigHomeDelete_ValidationErrors(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{name: "missing dst", url: "/v1/config/home?section=mig"},
		{name: "missing section", url: "/v1/config/home?dst=.config/app"},
		{name: "absolute path dst", url: "/v1/config/home?dst=/etc/passwd&section=mig"},
		{name: "path traversal dst", url: "/v1/config/home?dst=../escape&section=mig"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := &configStore{}
			holder := NewConfigHolder(config.GitLabConfig{}, nil)

			handler := deleteConfigHomeHandler(holder, st)
			rr := doRequest(t, handler, http.MethodDelete, tt.url, nil)

			assertStatus(t, rr, http.StatusBadRequest)
			if st.deleteConfigHome.called {
				t.Error("store should not be called for invalid input")
			}
		})
	}
}

// TestConfigHome_StoreErrors verifies that store failures return 500.
func TestConfigHome_StoreErrors(t *testing.T) {
	tests := []struct {
		name   string
		method string
		setup  func(*configStore)
		invoke func(*ConfigHolder, *configStore) (http.Handler, string, any)
	}{
		{
			name:   "put store error",
			method: http.MethodPut,
			setup:  func(st *configStore) { st.upsertConfigHome.err = errMockDatabase },
			invoke: func(h *ConfigHolder, st *configStore) (http.Handler, string, any) {
				return putConfigHomeHandler(h, st), "/v1/config/home",
					map[string]any{"entry": "abcdef1:.config/app", "section": "mig"}
			},
		},
		{
			name:   "delete store error",
			method: http.MethodDelete,
			setup:  func(st *configStore) { st.deleteConfigHome.err = errMockDatabase },
			invoke: func(h *ConfigHolder, st *configStore) (http.Handler, string, any) {
				h.AddConfigHome("mig", ConfigHomeEntry{Entry: "abcdef1:.config/app", Dst: ".config/app", Section: "mig"})
				return deleteConfigHomeHandler(h, st), "/v1/config/home?dst=.config/app&section=mig", nil
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := &configStore{}
			tt.setup(st)
			holder := NewConfigHolder(config.GitLabConfig{}, nil)

			handler, path, body := tt.invoke(holder, st)
			rr := doRequest(t, handler, tt.method, path, body)

			assertStatus(t, rr, http.StatusInternalServerError)
		})
	}
}

// TestConfigHomeDeleteNormalizesNonCanonicalDst verifies that deleting
// with a non-canonical destination (e.g. extra slashes) normalizes the dst
// to match the canonical form persisted by the put handler.
func TestConfigHomeDeleteNormalizesNonCanonicalDst(t *testing.T) {
	st := &configStore{}
	holder := NewConfigHolder(config.GitLabConfig{}, nil)
	holder.AddConfigHome("mig", ConfigHomeEntry{Entry: "abcdef1:.config/app", Dst: ".config/app", Section: "mig"})

	handler := deleteConfigHomeHandler(holder, st)

	// Use non-canonical dst ".config//app" which cleans to ".config/app".
	rr := doRequest(t, handler, http.MethodDelete, "/v1/config/home?dst=.config//app&section=mig", nil)

	assertStatus(t, rr, http.StatusNoContent)

	if !st.deleteConfigHome.called {
		t.Error("store.DeleteConfigHome was not called")
	}
	// Store must receive the normalized dst, not the raw query value.
	if st.deleteConfigHome.params.Dst != ".config/app" {
		t.Errorf("store Dst = %q, want .config/app (normalized)", st.deleteConfigHome.params.Dst)
	}

	// Verify holder was updated using the normalized dst.
	entries := holder.GetConfigHome("mig")
	if len(entries) != 0 {
		t.Errorf("holder Home = %v, want empty", entries)
	}
}

// TestConfigHomePutDeduplicates verifies that upserting the same dst twice
// in the same section does not produce duplicates.
func TestConfigHomePutDeduplicates(t *testing.T) {
	st := &configStore{}
	holder := NewConfigHolder(config.GitLabConfig{}, nil)

	handler := putConfigHomeHandler(holder, st)

	reqBody := map[string]any{"entry": "abcdef1:.config/app", "section": "mig"}
	rr := doRequest(t, handler, http.MethodPut, "/v1/config/home", reqBody)
	assertStatus(t, rr, http.StatusOK)

	// Upsert with different hash but same dst.
	reqBody2 := map[string]any{"entry": "bbbbbbb:.config/app", "section": "mig"}
	rr = doRequest(t, handler, http.MethodPut, "/v1/config/home", reqBody2)
	assertStatus(t, rr, http.StatusOK)

	entries := holder.GetConfigHome("mig")
	if len(entries) != 1 {
		t.Errorf("holder Home = %v, want exactly 1 entry", entries)
	}
	if entries[0].Entry != "bbbbbbb:.config/app" {
		t.Errorf("entry = %q, want bbbbbbb:.config/app", entries[0].Entry)
	}
}

// TestConfigHomeHydraOverlaySync verifies that Home changes are reflected
// in hydra overlays visible to claim mutators.
func TestConfigHomeHydraOverlaySync(t *testing.T) {
	holder := NewConfigHolder(config.GitLabConfig{}, nil)
	holder.AddConfigHome("mig", ConfigHomeEntry{Entry: "abcdef1:.config/app", Dst: ".config/app", Section: "mig"})
	holder.AddConfigHome("mig", ConfigHomeEntry{Entry: "bbbbbbb:.local/bin", Dst: ".local/bin", Section: "mig"})

	overlays := holder.GetHydraOverlays()
	if overlays == nil {
		t.Fatal("hydra overlays should not be nil")
	}
	mig := overlays["mig"]
	if mig == nil {
		t.Fatal("mig overlay should not be nil")
	}
	if len(mig.Home) != 2 {
		t.Errorf("mig.Home = %v, want 2 entries", mig.Home)
	}
}
