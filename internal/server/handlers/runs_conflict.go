package handlers

import (
	"errors"
	"net/http"

	"github.com/iw2rmb/ploy/internal/store"
)

func writeActiveRepoRunConflict(w http.ResponseWriter, err error) bool {
	var conflict *store.ActiveRepoRunError
	if !errors.As(err, &conflict) {
		return false
	}
	writeHTTPError(w, http.StatusConflict, "%s", conflict)
	return true
}
