package handlers

import (
	"net/http"
)

func (a *API) Health(w http.ResponseWriter, r *http.Request) {
	if err := a.DB.Ping(r.Context()); err != nil {
		writeError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
