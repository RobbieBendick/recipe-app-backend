package db

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	NotificationFriendRequest  = "friend_request"
	NotificationFriendAccepted = "friend_accepted"
)

type Notification struct {
	ID        string          `json:"id"`
	UserID    string          `json:"userId"`
	Type      string          `json:"type"`
	Title     string          `json:"title"`
	Body      string          `json:"body"`
	Data      json.RawMessage `json:"data"`
	ReadAt    *time.Time      `json:"readAt,omitempty"`
	CreatedAt time.Time       `json:"createdAt"`
}

func CreateNotification(ctx context.Context, pool *pgxpool.Pool, userID, notifType, title, body string, data any) (*Notification, error) {
	payload, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	if payload == nil {
		payload = []byte("{}")
	}

	row := pool.QueryRow(ctx, `
		INSERT INTO notifications (user_id, type, title, body, data)
		VALUES ($1, $2, $3, $4, $5::jsonb)
		RETURNING id, user_id, type, title, body, data, read_at, created_at
	`, userID, notifType, title, body, string(payload))
	return scanNotification(row)
}

func ListNotifications(ctx context.Context, pool *pgxpool.Pool, userID string, limit int) ([]Notification, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := pool.Query(ctx, `
		SELECT id, user_id, type, title, body, data, read_at, created_at
		FROM notifications
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Notification, 0)
	for rows.Next() {
		n, err := scanNotificationRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *n)
	}
	return out, rows.Err()
}

func CountUnreadNotifications(ctx context.Context, pool *pgxpool.Pool, userID string) (int, error) {
	var count int
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*)::int
		FROM notifications
		WHERE user_id = $1 AND read_at IS NULL
	`, userID).Scan(&count)
	return count, err
}

func MarkNotificationRead(ctx context.Context, pool *pgxpool.Pool, userID, notificationID string) (*Notification, error) {
	row := pool.QueryRow(ctx, `
		UPDATE notifications
		SET read_at = COALESCE(read_at, now())
		WHERE id = $1 AND user_id = $2
		RETURNING id, user_id, type, title, body, data, read_at, created_at
	`, notificationID, userID)
	n, err := scanNotification(row)
	if err != nil {
		return nil, err
	}
	if n == nil {
		return nil, errors.New("notification not found")
	}
	return n, nil
}

func MarkAllNotificationsRead(ctx context.Context, pool *pgxpool.Pool, userID string) error {
	_, err := pool.Exec(ctx, `
		UPDATE notifications
		SET read_at = now()
		WHERE user_id = $1 AND read_at IS NULL
	`, userID)
	return err
}

func MarkFriendRequestNotificationsRead(ctx context.Context, pool *pgxpool.Pool, userID, friendshipID string) error {
	_, err := pool.Exec(ctx, `
		UPDATE notifications
		SET read_at = COALESCE(read_at, now())
		WHERE user_id = $1
		  AND type = $2
		  AND read_at IS NULL
		  AND data->>'friendshipId' = $3
	`, userID, NotificationFriendRequest, friendshipID)
	return err
}

func scanNotification(row pgx.Row) (*Notification, error) {
	var n Notification
	err := row.Scan(&n.ID, &n.UserID, &n.Type, &n.Title, &n.Body, &n.Data, &n.ReadAt, &n.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if n.Data == nil {
		n.Data = json.RawMessage("{}")
	}
	return &n, nil
}

func scanNotificationRows(rows pgx.Rows) (*Notification, error) {
	var n Notification
	err := rows.Scan(&n.ID, &n.UserID, &n.Type, &n.Title, &n.Body, &n.Data, &n.ReadAt, &n.CreatedAt)
	if err != nil {
		return nil, err
	}
	if n.Data == nil {
		n.Data = json.RawMessage("{}")
	}
	return &n, nil
}
