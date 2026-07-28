package handlers

import (
	"errors"
	"net/http"

	"github.com/robbi/recipe-app-backend/pkg/recipeimport"
)

type importURLBody struct {
	URL string `json:"url"`
}

func (a *API) ImportRecipeFromURL(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireUser(w, r); !ok {
		return
	}

	var body importURLBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	extracted, err := recipeimport.FromURL(r.Context(), body.URL, recipeimport.Options{
		GeminiAPIKey: a.GeminiAPIKey,
		GeminiModel:  a.GeminiModel,
	})
	if err != nil {
		switch {
		case errors.Is(err, recipeimport.ErrInvalidURL):
			writeError(w, http.StatusBadRequest, "enter a valid http(s) URL")
		case errors.Is(err, recipeimport.ErrBlockedHost):
			writeError(w, http.StatusBadRequest, "that URL is not allowed")
		case errors.Is(err, recipeimport.ErrNoRecipeData):
			writeError(w, http.StatusUnprocessableEntity, "couldn't find recipe details — for Instagram/Facebook, make sure the post is public and the caption lists the recipe")
		case errors.Is(err, recipeimport.ErrFetchFailed):
			writeError(w, http.StatusBadGateway, "couldn't load that page — for Reels, the post may be private or blocked")
		case errors.Is(err, recipeimport.ErrAIFailed):
			writeError(w, http.StatusBadGateway, "AI couldn't extract that recipe — try another link or fill the form manually")
		default:
			writeError(w, http.StatusBadGateway, "couldn't import from that URL")
		}
		return
	}

	writeJSON(w, http.StatusOK, extracted)
}
