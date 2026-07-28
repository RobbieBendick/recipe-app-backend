package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/robbi/recipe-app-backend/pkg/auth"
	"github.com/robbi/recipe-app-backend/pkg/kroger"
)

type API struct {
	DB           *pgxpool.Pool
	Auth         *auth.Service
	Kroger       *kroger.Client
	DefaultZip   string
	GeminiAPIKey string
	GeminiModel  string
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func decodeJSON(r *http.Request, dest any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(dest)
}

func trimNonEmpty(value string) string {
	return strings.TrimSpace(value)
}

func (a *API) requireUser(w http.ResponseWriter, r *http.Request) (string, bool) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return "", false
	}
	return userID, true
}
