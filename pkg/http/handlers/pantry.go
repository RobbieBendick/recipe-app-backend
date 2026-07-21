package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/robbi/recipe-app-backend/pkg/db"
)

func (a *API) ListPantry(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	items, err := db.ListPantry(r.Context(), a.DB, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list pantry")
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (a *API) CreatePantryItem(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	var in db.PantryItemInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	in.Name = trimNonEmpty(in.Name)
	if in.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	item, err := db.CreatePantryItem(r.Context(), a.DB, userID, in)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create pantry item")
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (a *API) UpdatePantryItem(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	var in db.PantryItemInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	in.Name = trimNonEmpty(in.Name)
	if in.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	item, err := db.UpdatePantryItem(r.Context(), a.DB, userID, id, in)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update pantry item")
		return
	}
	if item == nil {
		writeError(w, http.StatusNotFound, "pantry item not found")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (a *API) TogglePantryStock(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	item, err := db.TogglePantryStock(r.Context(), a.DB, userID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to toggle pantry item")
		return
	}
	if item == nil {
		writeError(w, http.StatusNotFound, "pantry item not found")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (a *API) DeletePantryItem(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	okDel, err := db.DeletePantryItem(r.Context(), a.DB, userID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete pantry item")
		return
	}
	if !okDel {
		writeError(w, http.StatusNotFound, "pantry item not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) ReplacePantry(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	var items []db.PantryReplaceItem
	if err := decodeJSON(r, &items); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if items == nil {
		items = []db.PantryReplaceItem{}
	}

	out, err := db.ReplacePantry(r.Context(), a.DB, userID, items)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save pantry")
		return
	}
	writeJSON(w, http.StatusOK, out)
}
