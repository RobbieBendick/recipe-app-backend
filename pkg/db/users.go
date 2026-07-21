package db

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	PasswordHash *string   `json:"-"`
	GoogleSub    *string   `json:"-"`
	Name         string    `json:"name"`
	AvatarURL    string    `json:"avatarUrl"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

func GetUserByID(ctx context.Context, pool *pgxpool.Pool, id string) (*User, error) {
	row := pool.QueryRow(ctx, `
		SELECT id, email, password_hash, google_sub, name, avatar_url, created_at, updated_at
		FROM users
		WHERE id = $1
	`, id)
	return scanUser(row)
}

func GetUserByEmail(ctx context.Context, pool *pgxpool.Pool, email string) (*User, error) {
	row := pool.QueryRow(ctx, `
		SELECT id, email, password_hash, google_sub, name, avatar_url, created_at, updated_at
		FROM users
		WHERE lower(email) = lower($1)
	`, strings.TrimSpace(email))
	return scanUser(row)
}

func GetUserByGoogleSub(ctx context.Context, pool *pgxpool.Pool, sub string) (*User, error) {
	row := pool.QueryRow(ctx, `
		SELECT id, email, password_hash, google_sub, name, avatar_url, created_at, updated_at
		FROM users
		WHERE google_sub = $1
	`, sub)
	return scanUser(row)
}

func CreateUserWithPassword(ctx context.Context, pool *pgxpool.Pool, email, passwordHash, name string) (*User, error) {
	row := pool.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, name)
		VALUES (lower($1), $2, $3)
		RETURNING id, email, password_hash, google_sub, name, avatar_url, created_at, updated_at
	`, strings.TrimSpace(email), passwordHash, strings.TrimSpace(name))
	return scanUser(row)
}

func CreateUserWithGoogle(ctx context.Context, pool *pgxpool.Pool, email, googleSub, name, avatarURL string) (*User, error) {
	row := pool.QueryRow(ctx, `
		INSERT INTO users (email, google_sub, name, avatar_url)
		VALUES (lower($1), $2, $3, $4)
		RETURNING id, email, password_hash, google_sub, name, avatar_url, created_at, updated_at
	`, strings.TrimSpace(email), googleSub, strings.TrimSpace(name), strings.TrimSpace(avatarURL))
	return scanUser(row)
}

func LinkGoogleSub(ctx context.Context, pool *pgxpool.Pool, userID, googleSub, name, avatarURL string) (*User, error) {
	row := pool.QueryRow(ctx, `
		UPDATE users
		SET google_sub = $2,
			name = CASE WHEN $3 <> '' THEN $3 ELSE name END,
			avatar_url = CASE WHEN $4 <> '' THEN $4 ELSE avatar_url END,
			updated_at = now()
		WHERE id = $1
		RETURNING id, email, password_hash, google_sub, name, avatar_url, created_at, updated_at
	`, userID, googleSub, strings.TrimSpace(name), strings.TrimSpace(avatarURL))
	return scanUser(row)
}

func scanUser(row scannable) (*User, error) {
	var u User
	err := row.Scan(
		&u.ID,
		&u.Email,
		&u.PasswordHash,
		&u.GoogleSub,
		&u.Name,
		&u.AvatarURL,
		&u.CreatedAt,
		&u.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}
