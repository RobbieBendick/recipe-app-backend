package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/robbi/recipe-app-backend/pkg/db"
)

type shareRecipeBody struct {
	FriendUserID string `json:"friendUserId"`
}

func (a *API) ShareRecipe(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	recipeID := chi.URLParam(r, "id")

	var body shareRecipeBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	friendID := strings.TrimSpace(body.FriendUserID)
	if friendID == "" {
		writeError(w, http.StatusBadRequest, "friendUserId is required")
		return
	}

	recipe, err := db.GetRecipe(r.Context(), a.DB, userID, recipeID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load recipe")
		return
	}
	if recipe == nil {
		writeError(w, http.StatusNotFound, "recipe not found")
		return
	}

	me, err := db.GetUserByID(r.Context(), a.DB, userID)
	if err != nil || me == nil {
		writeError(w, http.StatusInternalServerError, "failed to load user")
		return
	}

	share, err := db.CreateRecipeShare(r.Context(), a.DB, userID, friendID, recipe)
	if err != nil {
		switch {
		case errors.Is(err, db.ErrNotFriends):
			writeError(w, http.StatusForbidden, "you can only share recipes with friends")
		case errors.Is(err, db.ErrSharePending):
			writeError(w, http.StatusConflict, "you already sent this recipe to that friend")
		case errors.Is(err, db.ErrCannotShareSelf):
			writeError(w, http.StatusBadRequest, "you can't share a recipe with yourself")
		default:
			writeError(w, http.StatusInternalServerError, "failed to share recipe")
		}
		return
	}

	fromName := displayName(me)
	emoji := strings.TrimSpace(recipe.Emoji)
	title := "Recipe shared with you"
	bodyText := fmt.Sprintf("%s shared “%s” with you", fromName, recipe.Title)
	if emoji != "" {
		bodyText = fmt.Sprintf("%s shared %s %s with you", fromName, emoji, recipe.Title)
	}

	_, _ = db.CreateNotification(r.Context(), a.DB, friendID, db.NotificationRecipeShare,
		title,
		bodyText,
		map[string]any{
			"shareId":       share.ID,
			"recipeId":      recipe.ID,
			"recipeTitle":   recipe.Title,
			"recipeEmoji":   recipe.Emoji,
			"fromUserId":    me.ID,
			"fromName":      me.Name,
			"fromEmail":     me.Email,
			"fromAvatarUrl": me.AvatarURL,
		},
	)

	writeJSON(w, http.StatusCreated, share)
}

func (a *API) AcceptRecipeShare(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	shareID := chi.URLParam(r, "id")

	recipe, share, err := db.AcceptRecipeShare(r.Context(), a.DB, shareID, userID)
	if err != nil {
		if errors.Is(err, db.ErrRecipeShareNotFound) {
			writeError(w, http.StatusNotFound, "recipe share not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to accept recipe share")
		return
	}

	_ = db.DeleteRecipeShareNotifications(r.Context(), a.DB, userID, share.ID)

	me, _ := db.GetUserByID(r.Context(), a.DB, userID)
	if me != nil {
		_, _ = db.CreateNotification(r.Context(), a.DB, share.FromUserID, db.NotificationRecipeAccepted,
			"Recipe accepted",
			fmt.Sprintf("%s added “%s” to their recipes", displayName(me), share.RecipeTitle),
			map[string]any{
				"shareId":       share.ID,
				"recipeTitle":   share.RecipeTitle,
				"recipeEmoji":   share.RecipeEmoji,
				"fromUserId":    me.ID,
				"fromName":      me.Name,
				"fromEmail":     me.Email,
				"fromAvatarUrl": me.AvatarURL,
			},
		)
	}

	writeJSON(w, http.StatusOK, recipe)
}

func (a *API) DeclineRecipeShare(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	shareID := chi.URLParam(r, "id")

	if err := db.DeclineRecipeShare(r.Context(), a.DB, shareID, userID); err != nil {
		if errors.Is(err, db.ErrRecipeShareNotFound) {
			writeError(w, http.StatusNotFound, "recipe share not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to decline recipe share")
		return
	}

	_ = db.DeleteRecipeShareNotifications(r.Context(), a.DB, userID, shareID)
	w.WriteHeader(http.StatusNoContent)
}
