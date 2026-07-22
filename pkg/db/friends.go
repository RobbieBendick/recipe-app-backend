package db

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	FriendshipPending  = "pending"
	FriendshipAccepted = "accepted"
	FriendshipDeclined = "declined"
)

type PublicUser struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatarUrl"`
}

type Friendship struct {
	ID          string     `json:"id"`
	RequesterID string     `json:"requesterId"`
	AddresseeID string     `json:"addresseeId"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
	OtherUser   *PublicUser `json:"otherUser,omitempty"`
}

var (
	ErrFriendshipNotFound = errors.New("friendship not found")
	ErrAlreadyFriends     = errors.New("already friends")
	ErrRequestPending     = errors.New("friend request already pending")
	ErrCannotFriendSelf   = errors.New("cannot send a friend request to yourself")
)

func PublicUserFromUser(u *User) *PublicUser {
	if u == nil {
		return nil
	}
	return &PublicUser{
		ID:        u.ID,
		Email:     u.Email,
		Name:      u.Name,
		AvatarURL: u.AvatarURL,
	}
}

func GetFriendshipBetween(ctx context.Context, pool *pgxpool.Pool, userA, userB string) (*Friendship, error) {
	row := pool.QueryRow(ctx, `
		SELECT id, requester_id, addressee_id, status, created_at, updated_at
		FROM friendships
		WHERE (requester_id = $1 AND addressee_id = $2)
		   OR (requester_id = $2 AND addressee_id = $1)
		LIMIT 1
	`, userA, userB)
	return scanFriendship(row)
}

func GetFriendshipByID(ctx context.Context, pool *pgxpool.Pool, id string) (*Friendship, error) {
	row := pool.QueryRow(ctx, `
		SELECT id, requester_id, addressee_id, status, created_at, updated_at
		FROM friendships
		WHERE id = $1
	`, id)
	return scanFriendship(row)
}

func scanFriendship(row pgx.Row) (*Friendship, error) {
	var f Friendship
	err := row.Scan(&f.ID, &f.RequesterID, &f.AddresseeID, &f.Status, &f.CreatedAt, &f.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &f, nil
}

func CreateFriendRequest(ctx context.Context, pool *pgxpool.Pool, requesterID, addresseeID string) (*Friendship, error) {
	if requesterID == addresseeID {
		return nil, ErrCannotFriendSelf
	}

	existing, err := GetFriendshipBetween(ctx, pool, requesterID, addresseeID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		switch existing.Status {
		case FriendshipAccepted:
			return nil, ErrAlreadyFriends
		case FriendshipPending:
			return nil, ErrRequestPending
		case FriendshipDeclined:
			row := pool.QueryRow(ctx, `
				UPDATE friendships
				SET requester_id = $2,
					addressee_id = $3,
					status = $4,
					updated_at = now()
				WHERE id = $1
				RETURNING id, requester_id, addressee_id, status, created_at, updated_at
			`, existing.ID, requesterID, addresseeID, FriendshipPending)
			return scanFriendship(row)
		}
	}

	row := pool.QueryRow(ctx, `
		INSERT INTO friendships (requester_id, addressee_id, status)
		VALUES ($1, $2, $3)
		RETURNING id, requester_id, addressee_id, status, created_at, updated_at
	`, requesterID, addresseeID, FriendshipPending)
	return scanFriendship(row)
}

func AcceptFriendRequest(ctx context.Context, pool *pgxpool.Pool, friendshipID, addresseeID string) (*Friendship, error) {
	row := pool.QueryRow(ctx, `
		UPDATE friendships
		SET status = $3, updated_at = now()
		WHERE id = $1 AND addressee_id = $2 AND status = $4
		RETURNING id, requester_id, addressee_id, status, created_at, updated_at
	`, friendshipID, addresseeID, FriendshipAccepted, FriendshipPending)
	f, err := scanFriendship(row)
	if err != nil {
		return nil, err
	}
	if f == nil {
		return nil, ErrFriendshipNotFound
	}
	return f, nil
}

func DeclineFriendRequest(ctx context.Context, pool *pgxpool.Pool, friendshipID, addresseeID string) error {
	tag, err := pool.Exec(ctx, `
		UPDATE friendships
		SET status = $3, updated_at = now()
		WHERE id = $1 AND addressee_id = $2 AND status = $4
	`, friendshipID, addresseeID, FriendshipDeclined, FriendshipPending)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrFriendshipNotFound
	}
	return nil
}

func RemoveFriendship(ctx context.Context, pool *pgxpool.Pool, userID, friendUserID string) error {
	tag, err := pool.Exec(ctx, `
		DELETE FROM friendships
		WHERE status = $3
		  AND (
			(requester_id = $1 AND addressee_id = $2)
			OR (requester_id = $2 AND addressee_id = $1)
		  )
	`, userID, friendUserID, FriendshipAccepted)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrFriendshipNotFound
	}
	return nil
}

func ListFriends(ctx context.Context, pool *pgxpool.Pool, userID string) ([]PublicUser, error) {
	rows, err := pool.Query(ctx, `
		SELECT u.id, u.email, u.name, u.avatar_url
		FROM friendships f
		JOIN users u ON u.id = CASE
			WHEN f.requester_id = $1 THEN f.addressee_id
			ELSE f.requester_id
		END
		WHERE f.status = $2
		  AND (f.requester_id = $1 OR f.addressee_id = $1)
		ORDER BY lower(COALESCE(NULLIF(u.name, ''), u.email)) ASC
	`, userID, FriendshipAccepted)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]PublicUser, 0)
	for rows.Next() {
		var u PublicUser
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.AvatarURL); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func ListIncomingFriendRequests(ctx context.Context, pool *pgxpool.Pool, userID string) ([]Friendship, error) {
	rows, err := pool.Query(ctx, `
		SELECT f.id, f.requester_id, f.addressee_id, f.status, f.created_at, f.updated_at,
			u.id, u.email, u.name, u.avatar_url
		FROM friendships f
		JOIN users u ON u.id = f.requester_id
		WHERE f.addressee_id = $1 AND f.status = $2
		ORDER BY f.created_at DESC
	`, userID, FriendshipPending)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Friendship, 0)
	for rows.Next() {
		var f Friendship
		var other PublicUser
		if err := rows.Scan(
			&f.ID, &f.RequesterID, &f.AddresseeID, &f.Status, &f.CreatedAt, &f.UpdatedAt,
			&other.ID, &other.Email, &other.Name, &other.AvatarURL,
		); err != nil {
			return nil, err
		}
		f.OtherUser = &other
		out = append(out, f)
	}
	return out, rows.Err()
}

func ListOutgoingFriendRequests(ctx context.Context, pool *pgxpool.Pool, userID string) ([]Friendship, error) {
	rows, err := pool.Query(ctx, `
		SELECT f.id, f.requester_id, f.addressee_id, f.status, f.created_at, f.updated_at,
			u.id, u.email, u.name, u.avatar_url
		FROM friendships f
		JOIN users u ON u.id = f.addressee_id
		WHERE f.requester_id = $1 AND f.status = $2
		ORDER BY f.created_at DESC
	`, userID, FriendshipPending)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Friendship, 0)
	for rows.Next() {
		var f Friendship
		var other PublicUser
		if err := rows.Scan(
			&f.ID, &f.RequesterID, &f.AddresseeID, &f.Status, &f.CreatedAt, &f.UpdatedAt,
			&other.ID, &other.Email, &other.Name, &other.AvatarURL,
		); err != nil {
			return nil, err
		}
		f.OtherUser = &other
		out = append(out, f)
	}
	return out, rows.Err()
}

func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
