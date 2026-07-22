package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type SharedPantry struct {
	ID         string      `json:"id"`
	SharedWith *PublicUser `json:"sharedWith,omitempty"`
	Items      []PantryItem `json:"items"`
}

func querySharedPantryItems(ctx context.Context, pool *pgxpool.Pool, pantryID string) ([]PantryItem, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, name, emoji, notes, in_stock, percent, unit, sort_order, created_at, updated_at
		FROM pantry_items
		WHERE shared_pantry_id = $1
		ORDER BY sort_order ASC, name ASC
	`, pantryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]PantryItem, 0)
	for rows.Next() {
		item, err := scanPantry(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func GetSharedPantry(ctx context.Context, pool *pgxpool.Pool, userID, pantryID string) (*SharedPantry, error) {
	ok, err := canAccessSharedPantry(ctx, pool, userID, pantryID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}

	row := pool.QueryRow(ctx, `
		SELECT
			u.id, u.email, u.name, u.avatar_url, COALESCE(n.nickname, '')
		FROM shared_pantries s
		JOIN users u ON u.id = CASE WHEN s.user_a = $2 THEN s.user_b ELSE s.user_a END
		LEFT JOIN friend_nicknames n
			ON n.user_id = $2 AND n.friend_user_id = u.id
		WHERE s.id = $1
	`, pantryID, userID)
	var other PublicUser
	if err := row.Scan(&other.ID, &other.Email, &other.Name, &other.AvatarURL, &other.Nickname); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	items, err := querySharedPantryItems(ctx, pool, pantryID)
	if err != nil {
		return nil, err
	}

	return &SharedPantry{
		ID:         pantryID,
		SharedWith: &other,
		Items:      items,
	}, nil
}

func GetOrCreateSharedPantry(ctx context.Context, pool *pgxpool.Pool, userID, friendUserID string) (*SharedPantry, bool, error) {
	if userID == friendUserID {
		return nil, false, ErrCannotFriendSelf
	}
	ok, err := AreFriends(ctx, pool, userID, friendUserID)
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return nil, false, ErrNotFriends
	}

	var pantryID string
	err = pool.QueryRow(ctx, `
		SELECT id FROM shared_pantries
		WHERE user_a = LEAST($1::uuid, $2::uuid)
		  AND user_b = GREATEST($1::uuid, $2::uuid)
	`, userID, friendUserID).Scan(&pantryID)
	if err == nil {
		pantry, err := GetSharedPantry(ctx, pool, userID, pantryID)
		return pantry, false, err
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, false, err
	}

	me, err := GetUserByID(ctx, pool, userID)
	if err != nil || me == nil {
		return nil, false, err
	}

	err = pool.QueryRow(ctx, `
		INSERT INTO shared_pantries (user_a, user_b)
		VALUES (LEAST($1::uuid, $2::uuid), GREATEST($1::uuid, $2::uuid))
		RETURNING id
	`, userID, friendUserID).Scan(&pantryID)
	if err != nil {
		return nil, false, err
	}

	fromName := me.Name
	if fromName == "" {
		fromName = me.Email
	}
	_, _ = CreateNotification(ctx, pool, friendUserID, NotificationSharedPantry,
		"Shared pantry",
		fmt.Sprintf("%s started a shared pantry with you", fromName),
		map[string]any{
			"pantryId":      pantryID,
			"fromUserId":    me.ID,
			"fromName":      me.Name,
			"fromEmail":     me.Email,
			"fromAvatarUrl": me.AvatarURL,
		},
	)

	pantry, err := GetSharedPantry(ctx, pool, userID, pantryID)
	return pantry, true, err
}

func DeleteSharedPantryForPair(ctx context.Context, pool *pgxpool.Pool, userA, userB string) error {
	_, err := pool.Exec(ctx, `
		DELETE FROM shared_pantries
		WHERE user_a = LEAST($1::uuid, $2::uuid)
		  AND user_b = GREATEST($1::uuid, $2::uuid)
	`, userA, userB)
	return err
}
