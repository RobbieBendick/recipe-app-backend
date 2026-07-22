package handlers

import (
	"net/http"
	"strings"

	"github.com/robbi/recipe-app-backend/pkg/db"
	"github.com/robbi/recipe-app-backend/pkg/estimate"
	"github.com/robbi/recipe-app-backend/pkg/kroger"
)

type costBody struct {
	Lines      []string               `json:"lines"`
	LocationID string                 `json:"locationId"`
	Zip        string                 `json:"zip"`
	Overrides  []estimate.LineOverride `json:"overrides"`
}

type storeBody struct {
	Zip string `json:"zip"`
}

type productsBody struct {
	Line       string `json:"line"`
	SearchTerm string `json:"searchTerm"`
	LocationID string `json:"locationId"`
	Zip        string `json:"zip"`
}

type estimateResponse struct {
	*estimate.Result
	Store *estimate.StoreInfo `json:"store,omitempty"`
}

func (a *API) EstimateStatus(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireUser(w, r); !ok {
		return
	}
	if a.Kroger == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"configured": false,
			"tokenOk":    false,
			"tokenError": "kroger client not initialized",
		})
		return
	}
	writeJSON(w, http.StatusOK, a.Kroger.Status(r.Context()))
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

	user, err := db.UpdateUserKrogerStore(
		r.Context(),
		a.DB,
		userID,
		zip,
		kroger.NormalizeLocationID(store.LocationID),
		storeDisplayName(store),
	)
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
	locationID = kroger.NormalizeLocationID(locationID)

	result, err := estimate.EstimateLines(r.Context(), a.Kroger, locationID, body.Lines, body.Overrides)
	if err != nil {
		writeError(w, http.StatusBadGateway, "failed to estimate cost: "+err.Error())
		return
	}
	if store == nil {
		store = &estimate.StoreInfo{LocationID: locationID}
		if user.KrogerLocationID == locationID {
			store.Name = user.KrogerStoreName
			store.ZipCode = user.KrogerZip
		}
	}
	store.LocationID = locationID

	writeJSON(w, http.StatusOK, estimateResponse{
		Result: result,
		Store:  store,
	})
}

func (a *API) EstimateProducts(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	if a.Kroger == nil || !a.Kroger.Configured() {
		writeError(w, http.StatusServiceUnavailable, "Kroger price estimates are not configured")
		return
	}

	var body productsBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	line := strings.TrimSpace(body.Line)
	if line == "" {
		writeError(w, http.StatusBadRequest, "line is required")
		return
	}

	user, err := db.GetUserByID(r.Context(), a.DB, userID)
	if err != nil || user == nil {
		writeError(w, http.StatusInternalServerError, "failed to load profile")
		return
	}

	locationID, _, errResp := a.resolveLocation(r, user, body.LocationID, body.Zip)
	if errResp != "" {
		if strings.HasPrefix(errResp, "404:") {
			writeError(w, http.StatusNotFound, strings.TrimPrefix(errResp, "404:"))
			return
		}
		if strings.HasPrefix(errResp, "502:") {
			writeError(w, http.StatusBadGateway, strings.TrimPrefix(errResp, "502:"))
			return
		}
		writeError(w, http.StatusBadRequest, errResp)
		return
	}

	result, err := estimate.ListProductOptions(r.Context(), a.Kroger, locationID, line, body.SearchTerm)
	if err != nil {
		writeError(w, http.StatusBadGateway, "failed to search products: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// resolveLocation returns a normalized locationId. errResp is empty on success.
func (a *API) resolveLocation(r *http.Request, user *db.User, locationID, zip string) (string, *estimate.StoreInfo, string) {
	locationID = strings.TrimSpace(locationID)
	zip = strings.TrimSpace(zip)
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
			s, err := estimate.NearestStore(r.Context(), a.Kroger, zip)
			if err != nil {
				return "", nil, "502:failed to look up Kroger store: " + err.Error()
			}
			if s == nil {
				return "", nil, "404:no Kroger store found near that ZIP"
			}
			store = s
			locationID = s.LocationID
		}
	}

	if locationID == "" {
		return "", nil, "set a ZIP code or locationId to estimate prices"
	}
	return kroger.NormalizeLocationID(locationID), store, ""
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
