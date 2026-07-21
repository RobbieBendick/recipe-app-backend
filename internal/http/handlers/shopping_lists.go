package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/robbi/recipe-app-backend/internal/db"
)

func (a *API) ListShoppingLists(w http.ResponseWriter, r *http.Request) {
	lists, err := db.ListShoppingLists(r.Context(), a.DB)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list shopping lists")
		return
	}
	writeJSON(w, http.StatusOK, lists)
}

func (a *API) GetShoppingList(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	list, err := db.GetShoppingList(r.Context(), a.DB, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load shopping list")
		return
	}
	if list == nil {
		writeError(w, http.StatusNotFound, "shopping list not found")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (a *API) CreateShoppingList(w http.ResponseWriter, r *http.Request) {
	var in db.ShoppingListInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	in.Title = trimNonEmpty(in.Title)
	if in.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}
	if in.Items == nil {
		in.Items = []string{}
	}

	list, err := db.CreateShoppingList(r.Context(), a.DB, in)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create shopping list")
		return
	}
	writeJSON(w, http.StatusCreated, list)
}

func (a *API) UpdateShoppingList(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var in db.ShoppingListInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	in.Title = trimNonEmpty(in.Title)
	if in.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}
	if in.Items == nil {
		in.Items = []string{}
	}

	list, err := db.UpdateShoppingList(r.Context(), a.DB, id, in)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update shopping list")
		return
	}
	if list == nil {
		writeError(w, http.StatusNotFound, "shopping list not found")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (a *API) DeleteShoppingList(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ok, err := db.DeleteShoppingList(r.Context(), a.DB, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete shopping list")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "shopping list not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type addItemBody struct {
	Text string `json:"text"`
}

func (a *API) AddShoppingListItem(w http.ResponseWriter, r *http.Request) {
	listID := chi.URLParam(r, "id")
	var body addItemBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	body.Text = trimNonEmpty(body.Text)
	if body.Text == "" {
		writeError(w, http.StatusBadRequest, "text is required")
		return
	}

	list, err := db.AddShoppingListItem(r.Context(), a.DB, listID, body.Text)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to add item")
		return
	}
	if list == nil {
		writeError(w, http.StatusNotFound, "shopping list not found")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (a *API) ToggleShoppingListItem(w http.ResponseWriter, r *http.Request) {
	listID := chi.URLParam(r, "id")
	itemID := chi.URLParam(r, "itemId")
	list, err := db.ToggleShoppingListItem(r.Context(), a.DB, listID, itemID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to toggle item")
		return
	}
	if list == nil {
		writeError(w, http.StatusNotFound, "item not found")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (a *API) RemoveShoppingListItem(w http.ResponseWriter, r *http.Request) {
	listID := chi.URLParam(r, "id")
	itemID := chi.URLParam(r, "itemId")
	list, err := db.RemoveShoppingListItem(r.Context(), a.DB, listID, itemID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to remove item")
		return
	}
	if list == nil {
		writeError(w, http.StatusNotFound, "item not found")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

type addRecipeBody struct {
	RecipeID string `json:"recipeId"`
}

// AddRecipeToList appends a recipe's ingredients onto an existing shopping list
// (the "drag recipe onto list" workflow).
func (a *API) AddRecipeToList(w http.ResponseWriter, r *http.Request) {
	listID := chi.URLParam(r, "id")
	var body addRecipeBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	body.RecipeID = trimNonEmpty(body.RecipeID)
	if body.RecipeID == "" {
		writeError(w, http.StatusBadRequest, "recipeId is required")
		return
	}

	list, err := db.AddRecipeToShoppingList(r.Context(), a.DB, listID, body.RecipeID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to add recipe to list")
		return
	}
	if list == nil {
		writeError(w, http.StatusNotFound, "shopping list or recipe not found")
		return
	}
	writeJSON(w, http.StatusOK, list)
}
