package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/robbi/recipe-app-backend/pkg/db"
)

type sendFriendRequestBody struct {
	Email string `json:"email"`
}

func (a *API) ListFriends(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	friends, err := db.ListFriends(r.Context(), a.DB, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list friends")
		return
	}
	writeJSON(w, http.StatusOK, friends)
}

func (a *API) ListFriendRequests(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	incoming, err := db.ListIncomingFriendRequests(r.Context(), a.DB, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list friend requests")
		return
	}
	outgoing, err := db.ListOutgoingFriendRequests(r.Context(), a.DB, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list friend requests")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"incoming": incoming,
		"outgoing": outgoing,
	})
}

func (a *API) SendFriendRequest(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.requireUser(w, r)
	if !ok {
		return
	}

	var body sendFriendRequestBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	email := db.NormalizeEmail(body.Email)
	if email == "" || !strings.Contains(email, "@") {
		writeError(w, http.StatusBadRequest, "a valid email is required")
		return
	}

	me, err := db.GetUserByID(r.Context(), a.DB, userID)
	if err != nil || me == nil {
		writeError(w, http.StatusInternalServerError, "failed to load user")
		return
	}

	target, err := db.GetUserByEmail(r.Context(), a.DB, email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to look up user")
		return
	}
	if target == nil {
		writeError(w, http.StatusNotFound, "no account found with that email")
		return
	}
	if target.ID == userID {
		writeError(w, http.StatusBadRequest, "you can't send a friend request to yourself")
		return
	}

	friendship, err := db.CreateFriendRequest(r.Context(), a.DB, userID, target.ID)
	if err != nil {
		switch {
		case errors.Is(err, db.ErrAlreadyFriends):
			writeError(w, http.StatusConflict, "you're already friends with this user")
		case errors.Is(err, db.ErrRequestPending):
			writeError(w, http.StatusConflict, "a friend request is already pending")
		case errors.Is(err, db.ErrCannotFriendSelf):
			writeError(w, http.StatusBadRequest, "you can't send a friend request to yourself")
		default:
			writeError(w, http.StatusInternalServerError, "failed to send friend request")
		}
		return
	}

	fromName := displayName(me)
	_, _ = db.CreateNotification(r.Context(), a.DB, target.ID, db.NotificationFriendRequest,
		"Friend request",
		fmt.Sprintf("%s sent you a friend request", fromName),
		map[string]any{
			"friendshipId":  friendship.ID,
			"fromUserId":    me.ID,
			"fromName":      me.Name,
			"fromEmail":     me.Email,
			"fromAvatarUrl": me.AvatarURL,
		},
	)

	friendship.OtherUser = db.PublicUserFromUser(target)
	writeJSON(w, http.StatusCreated, friendship)
}

func (a *API) AcceptFriendRequest(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")

	friendship, err := db.AcceptFriendRequest(r.Context(), a.DB, id, userID)
	if err != nil {
		if errors.Is(err, db.ErrFriendshipNotFound) {
			writeError(w, http.StatusNotFound, "friend request not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to accept friend request")
		return
	}

	_ = db.MarkFriendRequestNotificationsRead(r.Context(), a.DB, userID, friendship.ID)

	me, _ := db.GetUserByID(r.Context(), a.DB, userID)
	if me != nil {
		_, _ = db.CreateNotification(r.Context(), a.DB, friendship.RequesterID, db.NotificationFriendAccepted,
			"Friend request accepted",
			fmt.Sprintf("%s accepted your friend request", displayName(me)),
			map[string]any{
				"friendshipId":  friendship.ID,
				"fromUserId":    me.ID,
				"fromName":      me.Name,
				"fromEmail":     me.Email,
				"fromAvatarUrl": me.AvatarURL,
			},
		)
	}

	other, _ := db.GetUserByID(r.Context(), a.DB, friendship.RequesterID)
	friendship.OtherUser = db.PublicUserFromUser(other)
	writeJSON(w, http.StatusOK, friendship)
}

func (a *API) DeclineFriendRequest(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")

	if err := db.DeclineFriendRequest(r.Context(), a.DB, id, userID); err != nil {
		if errors.Is(err, db.ErrFriendshipNotFound) {
			writeError(w, http.StatusNotFound, "friend request not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to decline friend request")
		return
	}

	_ = db.MarkFriendRequestNotificationsRead(r.Context(), a.DB, userID, id)
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) RemoveFriend(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	friendID := chi.URLParam(r, "userId")
	if err := db.RemoveFriendship(r.Context(), a.DB, userID, friendID); err != nil {
		if errors.Is(err, db.ErrFriendshipNotFound) {
			writeError(w, http.StatusNotFound, "friendship not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to remove friend")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) GetOrCreateSharedShoppingList(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	friendID := chi.URLParam(r, "userId")
	list, created, err := db.GetOrCreateSharedShoppingList(r.Context(), a.DB, userID, friendID)
	if err != nil {
		switch {
		case errors.Is(err, db.ErrNotFriends):
			writeError(w, http.StatusForbidden, "you can only share a list with friends")
		case errors.Is(err, db.ErrCannotFriendSelf):
			writeError(w, http.StatusBadRequest, "you can't open a shared list with yourself")
		default:
			writeError(w, http.StatusInternalServerError, "failed to open shared shopping list")
		}
		return
	}
	if list == nil {
		writeError(w, http.StatusNotFound, "shared shopping list not found")
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, list)
}

func (a *API) GetOrCreateSharedPantry(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	friendID := chi.URLParam(r, "userId")
	pantry, created, err := db.GetOrCreateSharedPantry(r.Context(), a.DB, userID, friendID)
	if err != nil {
		switch {
		case errors.Is(err, db.ErrNotFriends):
			writeError(w, http.StatusForbidden, "you can only share a pantry with friends")
		case errors.Is(err, db.ErrCannotFriendSelf):
			writeError(w, http.StatusBadRequest, "you can't open a shared pantry with yourself")
		default:
			writeError(w, http.StatusInternalServerError, "failed to open shared pantry")
		}
		return
	}
	if pantry == nil {
		writeError(w, http.StatusNotFound, "shared pantry not found")
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, pantry)
}

func (a *API) GetSharedPantry(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	pantry, err := db.GetSharedPantry(r.Context(), a.DB, userID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load shared pantry")
		return
	}
	if pantry == nil {
		writeError(w, http.StatusNotFound, "shared pantry not found")
		return
	}
	writeJSON(w, http.StatusOK, pantry)
}

type setNicknameBody struct {
	Nickname string `json:"nickname"`
}

func (a *API) SetFriendNickname(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	friendID := chi.URLParam(r, "userId")
	var body setNicknameBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	friend, err := db.SetFriendNickname(r.Context(), a.DB, userID, friendID, body.Nickname)
	if err != nil {
		if errors.Is(err, db.ErrFriendshipNotFound) {
			writeError(w, http.StatusNotFound, "friend not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to save nickname")
		return
	}
	if friend == nil {
		writeError(w, http.StatusNotFound, "friend not found")
		return
	}

	writeJSON(w, http.StatusOK, friend)
}

func displayName(u *db.User) string {
	if u == nil {
		return "Someone"
	}
	if name := strings.TrimSpace(u.Name); name != "" {
		return name
	}
	return u.Email
}
