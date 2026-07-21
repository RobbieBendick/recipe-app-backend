package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/robbi/recipe-app-backend/pkg/db"
)

func (a *API) ListShoppingLists(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	lists, err := db.ListShoppingLists(r.Context(), a.DB, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list shopping lists")
		return
	}
	writeJSON(w, http.StatusOK, lists)
}

func (a *API) GetShoppingList(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	list, err := db.GetShoppingList(r.Context(), a.DB, userID, id)
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
	userID, ok := a.requireUser(w, r)
	if !ok {
		return
	}
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

	list, err := db.CreateShoppingList(r.Context(), a.DB, userID, in)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create shopping list")
		return
	}
	writeJSON(w, http.StatusCreated, list)
}

func (a *API) UpdateShoppingList(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.requireUser(w, r)
	if !ok {
		return
	}
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

	list, err := db.UpdateShoppingList(r.Context(), a.DB, userID, id, in)
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
	userID, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	okDel, err := db.DeleteShoppingList(r.Context(), a.DB, userID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete shopping list")
		return
	}
	if !okDel {
		writeError(w, http.StatusNotFound, "shopping list not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type addItemBody struct {
	Text string `json:"text"`
}

func (a *API) AddShoppingListItem(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.requireUser(w, r)
	if !ok {
		return
	}
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

	list, err := db.AddShoppingListItem(r.Context(), a.DB, userID, listID, body.Text)
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
	userID, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	listID := chi.URLParam(r, "id")
	itemID := chi.URLParam(r, "itemId")
	list, err := db.ToggleShoppingListItem(r.Context(), a.DB, userID, listID, itemID)
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
	userID, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	listID := chi.URLParam(r, "id")
	itemID := chi.URLParam(r, "itemId")
	list, err := db.RemoveShoppingListItem(r.Context(), a.DB, userID, listID, itemID)
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

func (a *API) AddRecipeToList(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.requireUser(w, r)
	if !ok {
		return
	}
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

	list, err := db.AddRecipeToShoppingList(r.Context(), a.DB, userID, listID, body.RecipeID)
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
