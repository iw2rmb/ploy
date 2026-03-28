package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

type panicInIsError struct{}

func (panicInIsError) Error() string { return "panic-in-is" }
func (panicInIsError) Is(error) bool { panic("boom from Is") }

type panicInErrorString struct{}

func (panicInErrorString) Error() string { panic("boom from Error") }

func TestClaimJob_ClaimErrorWithPanickingIs_Panics(t *testing.T) {
	t.Parallel()

	nodeID := domaintypes.NodeID(domaintypes.NewNodeKey())
	st := &jobStore{
		claimJobErr:   panicInIsError{},
	}
	st.getNode.val = store.Node{ID: nodeID}

	handler := claimJobHandler(st, &ConfigHolder{})
	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+nodeID.String()+"/claim", nil)
	req.SetPathValue("id", nodeID.String())
	rr := httptest.NewRecorder()
	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatalf("expected panic from error Is implementation")
		}
	}()

	handler.ServeHTTP(rr, req)
}

func TestClaimJob_ClaimErrorWithPanickingErrorString_DoesNotPanic(t *testing.T) {
	t.Parallel()

	nodeID := domaintypes.NodeID(domaintypes.NewNodeKey())
	st := &jobStore{
		claimJobErr:   panicInErrorString{},
	}
	st.getNode.val = store.Node{ID: nodeID}

	handler := claimJobHandler(st, &ConfigHolder{})
	rr := doRequest(t, handler, http.MethodPost, "/v1/nodes/"+nodeID.String()+"/claim", nil, "id", nodeID.String())

	assertStatus(t, rr, http.StatusInternalServerError)
	if !strings.Contains(rr.Body.String(), "panic while reading error string") {
		t.Fatalf("expected panic-safe fallback error text, got %q", rr.Body.String())
	}
}
