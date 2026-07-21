package handlers

import (
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/robbi/recipe-app-backend/pkg/auth"
	"github.com/robbi/recipe-app-backend/pkg/db"
)

type authResponse struct {
	Token string   `json:"token"`
	User  *db.User `json:"user"`
}

type registerBody struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

type loginBody struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type googleBody struct {
	IDToken string `json:"idToken"`
}

func (a *API) Register(w http.ResponseWriter, r *http.Request) {
	var body registerBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	email := strings.ToLower(trimNonEmpty(body.Email))
	password := body.Password
	name := trimNonEmpty(body.Name)
	if email == "" || !strings.Contains(email, "@") {
		writeError(w, http.StatusBadRequest, "valid email is required")
		return
	}
	if utf8.RuneCountInString(password) < 8 {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}
	if name == "" {
		name = strings.Split(email, "@")[0]
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	user, err := db.CreateUserWithPassword(r.Context(), a.DB, email, hash, name)
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "an account with that email already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create account")
		return
	}

	a.writeAuth(w, http.StatusCreated, user)
}

func (a *API) Login(w http.ResponseWriter, r *http.Request) {
	var body loginBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	email := strings.ToLower(trimNonEmpty(body.Email))
	if email == "" || body.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password are required")
		return
	}

	user, err := db.GetUserByEmail(r.Context(), a.DB, email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to login")
		return
	}
	if user == nil || user.PasswordHash == nil || !auth.CheckPassword(*user.PasswordHash, body.Password) {
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}

	a.writeAuth(w, http.StatusOK, user)
}

func (a *API) GoogleAuth(w http.ResponseWriter, r *http.Request) {
	var body googleBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	idToken := trimNonEmpty(body.IDToken)
	if idToken == "" {
		writeError(w, http.StatusBadRequest, "idToken is required")
		return
	}

	profile, err := a.Auth.VerifyGoogleIDToken(r.Context(), idToken)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid Google token")
		return
	}

	user, err := db.GetUserByGoogleSub(r.Context(), a.DB, profile.Sub)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to lookup account")
		return
	}
	if user == nil {
		user, err = db.GetUserByEmail(r.Context(), a.DB, profile.Email)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to lookup account")
			return
		}
		if user != nil {
			user, err = db.LinkGoogleSub(r.Context(), a.DB, user.ID, profile.Sub, profile.Name, profile.AvatarURL)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "failed to link Google account")
				return
			}
		} else {
			user, err = db.CreateUserWithGoogle(r.Context(), a.DB, profile.Email, profile.Sub, profile.Name, profile.AvatarURL)
			if err != nil {
				if isUniqueViolation(err) {
					writeError(w, http.StatusConflict, "an account with that email already exists")
					return
				}
				writeError(w, http.StatusInternalServerError, "failed to create account")
				return
			}
		}
	}

	a.writeAuth(w, http.StatusOK, user)
}

func (a *API) Me(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	user, err := db.GetUserByID(r.Context(), a.DB, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load profile")
		return
	}
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func (a *API) Logout(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) writeAuth(w http.ResponseWriter, status int, user *db.User) {
	token, err := a.Auth.SignToken(user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to issue token")
		return
	}
	writeJSON(w, status, authResponse{Token: token, User: user})
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	// pgx v5 wraps differently sometimes
	return strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "23505")
}
