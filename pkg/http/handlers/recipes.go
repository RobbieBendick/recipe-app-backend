package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/robbi/recipe-app-backend/pkg/db"
)

func (a *API) ListRecipes(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	recipes, err := db.ListRecipes(r.Context(), a.DB, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list recipes")
		return
	}
	writeJSON(w, http.StatusOK, recipes)
}

func (a *API) GetRecipe(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	recipe, err := db.GetRecipe(r.Context(), a.DB, userID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load recipe")
		return
	}
	if recipe == nil {
		writeError(w, http.StatusNotFound, "recipe not found")
		return
	}
	writeJSON(w, http.StatusOK, recipe)
}

func (a *API) CreateRecipe(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	var in db.RecipeInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	in.Title = trimNonEmpty(in.Title)
	if in.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}
	if in.Ingredients == nil {
		in.Ingredients = []string{}
	}
	if in.Steps == nil {
		in.Steps = []string{}
	}

	recipe, err := db.CreateRecipe(r.Context(), a.DB, userID, in)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create recipe")
		return
	}
	writeJSON(w, http.StatusCreated, recipe)
}

func (a *API) UpdateRecipe(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	var in db.RecipeInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	in.Title = trimNonEmpty(in.Title)
	if in.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}
	if in.Ingredients == nil {
		in.Ingredients = []string{}
	}
	if in.Steps == nil {
		in.Steps = []string{}
	}

	recipe, err := db.UpdateRecipe(r.Context(), a.DB, userID, id, in)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update recipe")
		return
	}
	if recipe == nil {
		writeError(w, http.StatusNotFound, "recipe not found")
		return
	}
	writeJSON(w, http.StatusOK, recipe)
}

func (a *API) DeleteRecipe(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	okDel, err := db.DeleteRecipe(r.Context(), a.DB, userID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete recipe")
		return
	}
	if !okDel {
		writeError(w, http.StatusNotFound, "recipe not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) MakeShoppingListFromRecipe(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	list, err := db.CreateShoppingListFromRecipe(r.Context(), a.DB, userID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create shopping list")
		return
	}
	if list == nil {
		writeError(w, http.StatusNotFound, "recipe not found")
		return
	}
	writeJSON(w, http.StatusCreated, list)
}
