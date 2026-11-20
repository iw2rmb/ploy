package tui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
	"time"

	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
)

// All persisted token usernames remain selectable, with duplicates and blank names removed.
func TestLoadUsersUsesTokenUsernames(t *testing.T) {
	alice, bob, blank := "alice", " bob ", " "
	revoked := time.Now()
	for _, status := range []int{http.StatusOK, http.StatusForbidden, http.StatusUnauthorized, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/tokens" {
					t.Errorf("path = %s", r.URL.Path)
				}
				w.WriteHeader(status)
				_ = json.NewEncoder(w).Encode(domainapi.APITokenListResponse{Tokens: []domainapi.APITokenListItem{
					{Username: &bob, RevokedAt: &revoked}, {Username: &alice}, {Username: &alice}, {Username: &blank}, {},
				}})
			}))
			defer server.Close()
			base, _ := url.Parse(server.URL)
			msg := loadUsersCmd(t.Context(), server.Client(), base)().(usersLoadedMsg)
			if msg.enabled != (status == http.StatusOK) || (msg.err != nil) != (status != http.StatusOK && status != http.StatusForbidden) {
				t.Fatalf("capability = %+v", msg)
			}
			if msg.enabled && !reflect.DeepEqual(msg.users, []string{"alice", "bob"}) {
				t.Fatalf("users = %v", msg.users)
			}
		})
	}
}
