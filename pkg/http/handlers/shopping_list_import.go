package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/robbi/recipe-app-backend/pkg/recipeimport"
)

type importImageBody struct {
	Image    string `json:"image"`
	MimeType string `json:"mimeType"`
}

func (a *API) ImportShoppingListFromImage(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireUser(w, r); !ok {
		return
	}
	if strings.TrimSpace(a.GeminiAPIKey) == "" {
		writeError(w, http.StatusServiceUnavailable, "photo import isn't configured yet")
		return
	}

	var body importImageBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	extracted, err := recipeimport.FromImage(r.Context(), a.GeminiAPIKey, a.GeminiModel, body.MimeType, body.Image)
	if err != nil {
		switch {
		case errors.Is(err, recipeimport.ErrInvalidImage):
			writeError(w, http.StatusBadRequest, "that photo couldn't be read — try a clearer JPEG or PNG")
		case errors.Is(err, recipeimport.ErrNoListItems):
			writeError(w, http.StatusUnprocessableEntity, "couldn't find shopping items in that photo — try a closer shot of the list or groceries")
		case errors.Is(err, recipeimport.ErrAIFailed):
			writeError(w, http.StatusBadGateway, "AI couldn't read that photo — try another picture or add items manually")
		default:
			writeError(w, http.StatusBadGateway, "couldn't import from that photo")
		}
		return
	}

	writeJSON(w, http.StatusOK, extracted)
}
