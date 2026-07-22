package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func attachSharedWith(ctx context.Context, pool *pgxpool.Pool, viewerID string, list *ShoppingList) error {
	if list == nil {
		return nil
	}
	row := pool.QueryRow(ctx, `
		SELECT
			u.id, u.email, u.name, u.avatar_url, COALESCE(n.nickname, '')
		FROM shopping_list_shares s
		JOIN users u ON u.id = CASE WHEN s.user_a = $2 THEN s.user_b ELSE s.user_a END
		LEFT JOIN friend_nicknames n
			ON n.user_id = $2 AND n.friend_user_id = u.id
		WHERE s.list_id = $1
	`, list.ID, viewerID)
	var other PublicUser
	err := row.Scan(&other.ID, &other.Email, &other.Name, &other.AvatarURL, &other.Nickname)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	list.SharedWith = &other
	return nil
}

func GetOrCreateSharedShoppingList(ctx context.Context, pool *pgxpool.Pool, userID, friendUserID string) (*ShoppingList, bool, error) {
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

	var listID string
	err = pool.QueryRow(ctx, `
		SELECT list_id
		FROM shopping_list_shares
		WHERE user_a = LEAST($1::uuid, $2::uuid)
		  AND user_b = GREATEST($1::uuid, $2::uuid)
	`, userID, friendUserID).Scan(&listID)
	if err == nil {
		list, err := GetShoppingList(ctx, pool, userID, listID)
		return list, false, err
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, false, err
	}

	friend, err := GetUserByID(ctx, pool, friendUserID)
	if err != nil || friend == nil {
		return nil, false, err
	}
	me, err := GetUserByID(ctx, pool, userID)
	if err != nil || me == nil {
		return nil, false, err
	}

	friendPublic := PublicUserFromUser(friend)
	if nick, err := GetFriendNickname(ctx, pool, userID, friendUserID); err == nil {
		friendPublic.Nickname = nick
	}
	title := fmt.Sprintf("Shopping with %s", FriendDisplayName(friendPublic))

	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback(ctx)

	var createdID string
	err = tx.QueryRow(ctx, `
		INSERT INTO shopping_lists (user_id, title, emoji, recipe_counts)
		VALUES ($1, $2, '🛒', '{}'::jsonb)
		RETURNING id
	`, userID, title).Scan(&createdID)
	if err != nil {
		return nil, false, err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO shopping_list_shares (list_id, user_a, user_b)
		VALUES ($1, LEAST($2::uuid, $3::uuid), GREATEST($2::uuid, $3::uuid))
	`, createdID, userID, friendUserID)
	if err != nil {
		return nil, false, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, false, err
	}

	fromName := me.Name
	if fromName == "" {
		fromName = me.Email
	}
	_, _ = CreateNotification(ctx, pool, friendUserID, NotificationSharedShoppingList,
		"Shared shopping list",
		fmt.Sprintf("%s started a shared shopping list with you", fromName),
		map[string]any{
			"listId":        createdID,
			"fromUserId":    me.ID,
			"fromName":      me.Name,
			"fromEmail":     me.Email,
			"fromAvatarUrl": me.AvatarURL,
		},
	)

	list, err := GetShoppingList(ctx, pool, userID, createdID)
	return list, true, err
}

func DeleteSharedShoppingListForPair(ctx context.Context, pool *pgxpool.Pool, userA, userB string) error {
	_, err := pool.Exec(ctx, `
		DELETE FROM shopping_lists
		WHERE id IN (
			SELECT list_id
			FROM shopping_list_shares
			WHERE user_a = LEAST($1::uuid, $2::uuid)
			  AND user_b = GREATEST($1::uuid, $2::uuid)
		)
	`, userA, userB)
	return err
}
