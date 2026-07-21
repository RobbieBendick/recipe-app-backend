package db

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PantryItem struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Emoji     string    `json:"emoji"`
	Notes     string    `json:"notes"`
	InStock   bool      `json:"inStock"`
	Percent   int       `json:"percent"`
	SortOrder int       `json:"sortOrder"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type PantryItemInput struct {
	Name    string `json:"name"`
	Emoji   string `json:"emoji"`
	Notes   string `json:"notes"`
	InStock *bool  `json:"inStock,omitempty"`
	Percent *int   `json:"percent,omitempty"`
}

type PantryReplaceItem struct {
	ID      string `json:"id,omitempty"`
	Name    string `json:"name"`
	Emoji   string `json:"emoji"`
	Notes   string `json:"notes"`
	InStock bool   `json:"inStock"`
	Percent int    `json:"percent"`
}

var defaultPantry = []struct {
	Emoji string
	Name  string
}{
	{"🥚", "Eggs"},
	{"🧈", "Butter"},
	{"🥛", "Milk"},
	{"🍞", "Bread"},
	{"🫒", "Olive oil"},
	{"🧂", "Salt"},
	{"🌶️", "Black pepper"},
	{"🧄", "Garlic"},
	{"🧅", "Onion"},
	{"🍋", "Lemon"},
	{"🍚", "Rice"},
	{"🍝", "Pasta"},
	{"🧀", "Cheese"},
	{"🫙", "Yogurt"},
}

func clampPercent(value int) int {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}

// 0% => out, >0% => in stock. Toggling out forces 0%; toggling in from 0% becomes 50%.
func normalizeStock(percent int, inStockHint *bool) (int, bool) {
	p := clampPercent(percent)
	if inStockHint != nil {
		if !*inStockHint {
			return 0, false
		}
		if p == 0 {
			return 50, true
		}
		return p, true
	}
	return p, p > 0
}

func ListPantry(ctx context.Context, pool *pgxpool.Pool) ([]PantryItem, error) {
	items, err := queryPantry(ctx, pool)
	if err != nil {
		return nil, err
	}
	if len(items) > 0 {
		return items, nil
	}

	if err := seedDefaultPantry(ctx, pool); err != nil {
		return nil, err
	}
	return queryPantry(ctx, pool)
}

func queryPantry(ctx context.Context, pool *pgxpool.Pool) ([]PantryItem, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, name, emoji, notes, in_stock, percent, sort_order, created_at, updated_at
		FROM pantry_items
		ORDER BY sort_order ASC, name ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PantryItem
	for rows.Next() {
		item, err := scanPantry(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if out == nil {
		out = []PantryItem{}
	}
	return out, rows.Err()
}

func seedDefaultPantry(ctx context.Context, pool *pgxpool.Pool) error {
	for i, item := range defaultPantry {
		_, err := pool.Exec(ctx, `
			INSERT INTO pantry_items (name, emoji, notes, in_stock, percent, sort_order)
			VALUES ($1, $2, '', TRUE, 50, $3)
		`, item.Name, item.Emoji, i)
		if err != nil {
			return err
		}
	}
	return nil
}

func CreatePantryItem(ctx context.Context, pool *pgxpool.Pool, in PantryItemInput) (*PantryItem, error) {
	percent := 50
	if in.Percent != nil {
		percent = *in.Percent
	}
	percent, inStock := normalizeStock(percent, in.InStock)

	var maxOrder *int
	_ = pool.QueryRow(ctx, `SELECT MAX(sort_order) FROM pantry_items`).Scan(&maxOrder)
	next := 0
	if maxOrder != nil {
		next = *maxOrder + 1
	}

	row := pool.QueryRow(ctx, `
		INSERT INTO pantry_items (name, emoji, notes, in_stock, percent, sort_order)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, name, emoji, notes, in_stock, percent, sort_order, created_at, updated_at
	`, in.Name, in.Emoji, in.Notes, inStock, percent, next)
	item, err := scanPantry(row)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func UpdatePantryItem(ctx context.Context, pool *pgxpool.Pool, id string, in PantryItemInput) (*PantryItem, error) {
	existing, err := getPantryItem(ctx, pool, id)
	if err != nil || existing == nil {
		return nil, err
	}

	percent := existing.Percent
	if in.Percent != nil {
		percent = *in.Percent
	}
	percent, inStock := normalizeStock(percent, in.InStock)

	row := pool.QueryRow(ctx, `
		UPDATE pantry_items
		SET name = $2,
			emoji = $3,
			notes = $4,
			in_stock = $5,
			percent = $6,
			updated_at = now()
		WHERE id = $1
		RETURNING id, name, emoji, notes, in_stock, percent, sort_order, created_at, updated_at
	`, id, in.Name, in.Emoji, in.Notes, inStock, percent)
	item, err := scanPantry(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func TogglePantryStock(ctx context.Context, pool *pgxpool.Pool, id string) (*PantryItem, error) {
	row := pool.QueryRow(ctx, `
		UPDATE pantry_items
		SET
			percent = CASE WHEN percent > 0 THEN 0 ELSE 50 END,
			in_stock = CASE WHEN percent > 0 THEN FALSE ELSE TRUE END,
			updated_at = now()
		WHERE id = $1
		RETURNING id, name, emoji, notes, in_stock, percent, sort_order, created_at, updated_at
	`, id)
	item, err := scanPantry(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func DeletePantryItem(ctx context.Context, pool *pgxpool.Pool, id string) (bool, error) {
	tag, err := pool.Exec(ctx, `DELETE FROM pantry_items WHERE id = $1`, id)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// ReplacePantry rewrites the whole pantry from an editable list (easy bulk edit).
func ReplacePantry(ctx context.Context, pool *pgxpool.Pool, items []PantryReplaceItem) ([]PantryItem, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM pantry_items`); err != nil {
		return nil, err
	}

	for i, item := range items {
		name := item.Name
		if name == "" {
			continue
		}
		inStockHint := item.InStock
		percent, inStock := normalizeStock(item.Percent, &inStockHint)
		_, err := tx.Exec(ctx, `
			INSERT INTO pantry_items (name, emoji, notes, in_stock, percent, sort_order)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, name, item.Emoji, item.Notes, inStock, percent, i)
		if err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return queryPantry(ctx, pool)
}

func getPantryItem(ctx context.Context, pool *pgxpool.Pool, id string) (*PantryItem, error) {
	row := pool.QueryRow(ctx, `
		SELECT id, name, emoji, notes, in_stock, percent, sort_order, created_at, updated_at
		FROM pantry_items
		WHERE id = $1
	`, id)
	item, err := scanPantry(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func scanPantry(row scannable) (PantryItem, error) {
	var item PantryItem
	err := row.Scan(
		&item.ID,
		&item.Name,
		&item.Emoji,
		&item.Notes,
		&item.InStock,
		&item.Percent,
		&item.SortOrder,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	return item, err
}
