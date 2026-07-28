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

	extracted, err := recipeimport.FromURL(r.Context(), body.URL)
	if err != nil {
		switch {
		case errors.Is(err, recipeimport.ErrInvalidURL):
			writeError(w, http.StatusBadRequest, "enter a valid http(s) URL")
		case errors.Is(err, recipeimport.ErrBlockedHost):
			writeError(w, http.StatusBadRequest, "that URL is not allowed")
		case errors.Is(err, recipeimport.ErrNoRecipeData):
			writeError(w, http.StatusUnprocessableEntity, "couldn't find recipe details on that page — try another link or fill the form manually")
		case errors.Is(err, recipeimport.ErrFetchFailed):
			writeError(w, http.StatusBadGateway, "couldn't load that page — check the link and try again")
		default:
			writeError(w, http.StatusBadGateway, "couldn't import from that URL")
		}
		return
	}

	writeJSON(w, http.StatusOK, extracted)
}
