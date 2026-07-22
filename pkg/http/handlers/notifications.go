package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/robbi/recipe-app-backend/pkg/db"
)

func (a *API) ListNotifications(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	items, err := db.ListNotifications(r.Context(), a.DB, userID, 50)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list notifications")
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (a *API) UnreadNotificationCount(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	count, err := db.CountUnreadNotifications(r.Context(), a.DB, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to count notifications")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"count": count})
}

func (a *API) MarkNotificationRead(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	n, err := db.MarkNotificationRead(r.Context(), a.DB, userID, id)
	if err != nil {
		writeError(w, http.StatusNotFound, "notification not found")
		return
	}
	writeJSON(w, http.StatusOK, n)
}

func (a *API) MarkAllNotificationsRead(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	if err := db.MarkAllNotificationsRead(r.Context(), a.DB, userID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to mark notifications read")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) DeleteNotification(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	if err := db.DeleteNotification(r.Context(), a.DB, userID, id); err != nil {
		writeError(w, http.StatusNotFound, "notification not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) ClearAllNotifications(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	if err := db.ClearAllNotifications(r.Context(), a.DB, userID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to clear notifications")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
