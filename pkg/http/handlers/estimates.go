package handlers

import (
	"net/http"
	"strings"

	"github.com/robbi/recipe-app-backend/pkg/db"
	"github.com/robbi/recipe-app-backend/pkg/estimate"
)

type costBody struct {
	Lines      []string `json:"lines"`
	LocationID string   `json:"locationId"`
	Zip        string   `json:"zip"`
}

type storeBody struct {
	Zip string `json:"zip"`
}

type estimateResponse struct {
	*estimate.Result
	Store *estimate.StoreInfo `json:"store,omitempty"`
}

func (a *API) GetEstimateStore(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	if a.Kroger == nil || !a.Kroger.Configured() {
		writeError(w, http.StatusServiceUnavailable, "Kroger price estimates are not configured")
		return
	}

	user, err := db.GetUserByID(r.Context(), a.DB, userID)
	if err != nil || user == nil {
		writeError(w, http.StatusInternalServerError, "failed to load profile")
		return
	}

	zip := strings.TrimSpace(r.URL.Query().Get("zip"))
	if zip == "" {
		zip = strings.TrimSpace(user.KrogerZip)
	}
	if zip == "" {
		zip = strings.TrimSpace(a.DefaultZip)
	}
	if zip == "" {
		writeJSON(w, http.StatusOK, map[string]any{
			"store": nil,
			"zip":   "",
		})
		return
	}

	// Prefer saved store when ZIP matches.
	if user.KrogerLocationID != "" && (user.KrogerZip == "" || user.KrogerZip == zip) {
		writeJSON(w, http.StatusOK, map[string]any{
			"store": estimate.StoreInfo{
				LocationID: user.KrogerLocationID,
				Name:       user.KrogerStoreName,
				ZipCode:    user.KrogerZip,
			},
			"zip": zip,
		})
		return
	}

	store, err := estimate.NearestStore(r.Context(), a.Kroger, zip)
	if err != nil {
		writeError(w, http.StatusBadGateway, "failed to look up Kroger store: "+err.Error())
		return
	}
	if store == nil {
		writeError(w, http.StatusNotFound, "no Kroger store found near that ZIP")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"store": store,
		"zip":   zip,
	})
}

func (a *API) SaveEstimateStore(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	if a.Kroger == nil || !a.Kroger.Configured() {
		writeError(w, http.StatusServiceUnavailable, "Kroger price estimates are not configured")
		return
	}

	var body storeBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	zip := strings.TrimSpace(body.Zip)
	if zip == "" {
		writeError(w, http.StatusBadRequest, "zip is required")
		return
	}

	store, err := estimate.NearestStore(r.Context(), a.Kroger, zip)
	if err != nil {
		writeError(w, http.StatusBadGateway, "failed to look up Kroger store: "+err.Error())
		return
	}
	if store == nil {
		writeError(w, http.StatusNotFound, "no Kroger store found near that ZIP")
		return
	}

	user, err := db.UpdateUserKrogerStore(r.Context(), a.DB, userID, zip, store.LocationID, storeDisplayName(store))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save store preference")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"store": store,
		"user":  user,
		"zip":   zip,
	})
}

func (a *API) EstimateCost(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	if a.Kroger == nil || !a.Kroger.Configured() {
		writeError(w, http.StatusServiceUnavailable, "Kroger price estimates are not configured")
		return
	}

	var body costBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if len(body.Lines) == 0 {
		writeError(w, http.StatusBadRequest, "lines are required")
		return
	}

	user, err := db.GetUserByID(r.Context(), a.DB, userID)
	if err != nil || user == nil {
		writeError(w, http.StatusInternalServerError, "failed to load profile")
		return
	}

	locationID := strings.TrimSpace(body.LocationID)
	zip := strings.TrimSpace(body.Zip)
	var store *estimate.StoreInfo

	if locationID == "" {
		if zip == "" {
			zip = strings.TrimSpace(user.KrogerZip)
		}
		if zip == "" {
			zip = strings.TrimSpace(a.DefaultZip)
		}
		if user.KrogerLocationID != "" && (zip == "" || zip == user.KrogerZip) {
			locationID = user.KrogerLocationID
			store = &estimate.StoreInfo{
				LocationID: user.KrogerLocationID,
				Name:       user.KrogerStoreName,
				ZipCode:    user.KrogerZip,
			}
		} else if zip != "" {
			store, err = estimate.NearestStore(r.Context(), a.Kroger, zip)
			if err != nil {
				writeError(w, http.StatusBadGateway, "failed to look up Kroger store: "+err.Error())
				return
			}
			if store == nil {
				writeError(w, http.StatusNotFound, "no Kroger store found near that ZIP")
				return
			}
			locationID = store.LocationID
		}
	}

	if locationID == "" {
		writeError(w, http.StatusBadRequest, "set a ZIP code or locationId to estimate prices")
		return
	}

	result, err := estimate.EstimateLines(r.Context(), a.Kroger, locationID, body.Lines)
	if err != nil {
		writeError(w, http.StatusBadGateway, "failed to estimate cost: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, estimateResponse{
		Result: result,
		Store:  store,
	})
}

func storeDisplayName(store *estimate.StoreInfo) string {
	if store == nil {
		return ""
	}
	name := strings.TrimSpace(store.Name)
	if name == "" {
		name = strings.TrimSpace(store.Chain)
	}
	parts := []string{}
	if name != "" {
		parts = append(parts, name)
	}
	loc := strings.TrimSpace(strings.TrimSpace(store.City) + ", " + strings.TrimSpace(store.State))
	loc = strings.Trim(loc, ", ")
	if loc != "" {
		parts = append(parts, loc)
	}
	return strings.Join(parts, " · ")
}
