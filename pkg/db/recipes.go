package db

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Recipe struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Emoji       string    `json:"emoji"`
	Ingredients []string  `json:"ingredients"`
	Steps       []string  `json:"steps"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type RecipeInput struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Emoji       string   `json:"emoji"`
	Ingredients []string `json:"ingredients"`
	Steps       []string `json:"steps"`
}

type ShoppingListItem struct {
	ID             string  `json:"id"`
	Text           string  `json:"text"`
	Checked        bool    `json:"checked"`
	SourceRecipeID *string `json:"sourceRecipeId,omitempty"`
	SortOrder      int     `json:"sortOrder"`
}

type ShoppingList struct {
	ID        string             `json:"id"`
	Title     string             `json:"title"`
	Emoji     string             `json:"emoji"`
	Items     []ShoppingListItem `json:"items"`
	CreatedAt time.Time          `json:"createdAt"`
	UpdatedAt time.Time          `json:"updatedAt"`
}

type ShoppingListInput struct {
	Title          string   `json:"title"`
	Emoji          string   `json:"emoji"`
	Items          []string `json:"items"`
	SourceRecipeID *string  `json:"sourceRecipeId,omitempty"`
}

func ListRecipes(ctx context.Context, pool *pgxpool.Pool) ([]Recipe, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, title, description, emoji, ingredients, steps, created_at, updated_at
		FROM recipes
		ORDER BY updated_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Recipe
	for rows.Next() {
		r, err := scanRecipe(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if out == nil {
		out = []Recipe{}
	}
	return out, rows.Err()
}

func GetRecipe(ctx context.Context, pool *pgxpool.Pool, id string) (*Recipe, error) {
	row := pool.QueryRow(ctx, `
		SELECT id, title, description, emoji, ingredients, steps, created_at, updated_at
		FROM recipes
		WHERE id = $1
	`, id)
	r, err := scanRecipe(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func CreateRecipe(ctx context.Context, pool *pgxpool.Pool, in RecipeInput) (*Recipe, error) {
	row := pool.QueryRow(ctx, `
		INSERT INTO recipes (title, description, emoji, ingredients, steps)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, title, description, emoji, ingredients, steps, created_at, updated_at
	`, in.Title, in.Description, in.Emoji, in.Ingredients, in.Steps)
	r, err := scanRecipe(row)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func UpdateRecipe(ctx context.Context, pool *pgxpool.Pool, id string, in RecipeInput) (*Recipe, error) {
	row := pool.QueryRow(ctx, `
		UPDATE recipes
		SET title = $2,
			description = $3,
			emoji = $4,
			ingredients = $5,
			steps = $6,
			updated_at = now()
		WHERE id = $1
		RETURNING id, title, description, emoji, ingredients, steps, created_at, updated_at
	`, id, in.Title, in.Description, in.Emoji, in.Ingredients, in.Steps)
	r, err := scanRecipe(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func DeleteRecipe(ctx context.Context, pool *pgxpool.Pool, id string) (bool, error) {
	tag, err := pool.Exec(ctx, `DELETE FROM recipes WHERE id = $1`, id)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

type scannable interface {
	Scan(dest ...any) error
}

func scanRecipe(row scannable) (Recipe, error) {
	var r Recipe
	var ingredients []string
	var steps []string
	err := row.Scan(&r.ID, &r.Title, &r.Description, &r.Emoji, &ingredients, &steps, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return r, err
	}
	if ingredients == nil {
		ingredients = []string{}
	}
	if steps == nil {
		steps = []string{}
	}
	r.Ingredients = ingredients
	r.Steps = steps
	return r, nil
}

func ListShoppingLists(ctx context.Context, pool *pgxpool.Pool) ([]ShoppingList, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, title, emoji, created_at, updated_at
		FROM shopping_lists
		ORDER BY updated_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ShoppingList
	for rows.Next() {
		var list ShoppingList
		if err := rows.Scan(&list.ID, &list.Title, &list.Emoji, &list.CreatedAt, &list.UpdatedAt); err != nil {
			return nil, err
		}
		items, err := listItems(ctx, pool, list.ID)
		if err != nil {
			return nil, err
		}
		list.Items = items
		out = append(out, list)
	}
	if out == nil {
		out = []ShoppingList{}
	}
	return out, rows.Err()
}

func GetShoppingList(ctx context.Context, pool *pgxpool.Pool, id string) (*ShoppingList, error) {
	var list ShoppingList
	err := pool.QueryRow(ctx, `
		SELECT id, title, emoji, created_at, updated_at
		FROM shopping_lists
		WHERE id = $1
	`, id).Scan(&list.ID, &list.Title, &list.Emoji, &list.CreatedAt, &list.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	items, err := listItems(ctx, pool, id)
	if err != nil {
		return nil, err
	}
	list.Items = items
	return &list, nil
}

func CreateShoppingList(ctx context.Context, pool *pgxpool.Pool, in ShoppingListInput) (*ShoppingList, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	emoji := in.Emoji
	if emoji == "" {
		emoji = "🛒"
	}

	var list ShoppingList
	err = tx.QueryRow(ctx, `
		INSERT INTO shopping_lists (title, emoji)
		VALUES ($1, $2)
		RETURNING id, title, emoji, created_at, updated_at
	`, in.Title, emoji).Scan(&list.ID, &list.Title, &list.Emoji, &list.CreatedAt, &list.UpdatedAt)
	if err != nil {
		return nil, err
	}

	for i, text := range in.Items {
		_, err := tx.Exec(ctx, `
			INSERT INTO shopping_list_items (list_id, text, sort_order, source_recipe_id)
			VALUES ($1, $2, $3, $4)
		`, list.ID, text, i, in.SourceRecipeID)
		if err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return GetShoppingList(ctx, pool, list.ID)
}

func UpdateShoppingList(ctx context.Context, pool *pgxpool.Pool, id string, in ShoppingListInput) (*ShoppingList, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	emoji := in.Emoji
	if emoji == "" {
		emoji = "🛒"
	}

	tag, err := tx.Exec(ctx, `
		UPDATE shopping_lists
		SET title = $2, emoji = $3, updated_at = now()
		WHERE id = $1
	`, id, in.Title, emoji)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, nil
	}

	if _, err := tx.Exec(ctx, `DELETE FROM shopping_list_items WHERE list_id = $1`, id); err != nil {
		return nil, err
	}

	for i, text := range in.Items {
		_, err := tx.Exec(ctx, `
			INSERT INTO shopping_list_items (list_id, text, sort_order)
			VALUES ($1, $2, $3)
		`, id, text, i)
		if err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return GetShoppingList(ctx, pool, id)
}

func DeleteShoppingList(ctx context.Context, pool *pgxpool.Pool, id string) (bool, error) {
	tag, err := pool.Exec(ctx, `DELETE FROM shopping_lists WHERE id = $1`, id)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func ToggleShoppingListItem(ctx context.Context, pool *pgxpool.Pool, listID, itemID string) (*ShoppingList, error) {
	tag, err := pool.Exec(ctx, `
		UPDATE shopping_list_items
		SET checked = NOT checked
		WHERE id = $1 AND list_id = $2
	`, itemID, listID)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, nil
	}
	_, _ = pool.Exec(ctx, `UPDATE shopping_lists SET updated_at = now() WHERE id = $1`, listID)
	return GetShoppingList(ctx, pool, listID)
}

func AddShoppingListItem(ctx context.Context, pool *pgxpool.Pool, listID, text string) (*ShoppingList, error) {
	var maxOrder *int
	err := pool.QueryRow(ctx, `
		SELECT MAX(sort_order) FROM shopping_list_items WHERE list_id = $1
	`, listID).Scan(&maxOrder)
	if err != nil {
		return nil, err
	}
	next := 0
	if maxOrder != nil {
		next = *maxOrder + 1
	}

	tag, err := pool.Exec(ctx, `
		INSERT INTO shopping_list_items (list_id, text, sort_order)
		SELECT $1, $2, $3
		WHERE EXISTS (SELECT 1 FROM shopping_lists WHERE id = $1)
	`, listID, text, next)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, nil
	}
	_, _ = pool.Exec(ctx, `UPDATE shopping_lists SET updated_at = now() WHERE id = $1`, listID)
	return GetShoppingList(ctx, pool, listID)
}

func RemoveShoppingListItem(ctx context.Context, pool *pgxpool.Pool, listID, itemID string) (*ShoppingList, error) {
	tag, err := pool.Exec(ctx, `
		DELETE FROM shopping_list_items WHERE id = $1 AND list_id = $2
	`, itemID, listID)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, nil
	}
	_, _ = pool.Exec(ctx, `UPDATE shopping_lists SET updated_at = now() WHERE id = $1`, listID)
	return GetShoppingList(ctx, pool, listID)
}

func CreateShoppingListFromRecipe(ctx context.Context, pool *pgxpool.Pool, recipeID string) (*ShoppingList, error) {
	recipe, err := GetRecipe(ctx, pool, recipeID)
	if err != nil || recipe == nil {
		return nil, err
	}
	sourceID := recipe.ID
	emoji := recipe.Emoji
	if emoji == "" {
		emoji = "🛒"
	}
	return CreateShoppingList(ctx, pool, ShoppingListInput{
		Title:          "Shop: " + recipe.Title,
		Emoji:          emoji,
		Items:          recipe.Ingredients,
		SourceRecipeID: &sourceID,
	})
}

func AddRecipeToShoppingList(ctx context.Context, pool *pgxpool.Pool, listID, recipeID string) (*ShoppingList, error) {
	recipe, err := GetRecipe(ctx, pool, recipeID)
	if err != nil || recipe == nil {
		return nil, err
	}

	list, err := GetShoppingList(ctx, pool, listID)
	if err != nil || list == nil {
		return nil, err
	}

	var maxOrder *int
	err = pool.QueryRow(ctx, `
		SELECT MAX(sort_order) FROM shopping_list_items WHERE list_id = $1
	`, listID).Scan(&maxOrder)
	if err != nil {
		return nil, err
	}
	next := 0
	if maxOrder != nil {
		next = *maxOrder + 1
	}

	sourceID := recipe.ID
	for _, text := range recipe.Ingredients {
		_, err := pool.Exec(ctx, `
			INSERT INTO shopping_list_items (list_id, text, sort_order, source_recipe_id)
			VALUES ($1, $2, $3, $4)
		`, listID, text, next, sourceID)
		if err != nil {
			return nil, err
		}
		next++
	}

	_, _ = pool.Exec(ctx, `UPDATE shopping_lists SET updated_at = now() WHERE id = $1`, listID)
	return GetShoppingList(ctx, pool, listID)
}

func listItems(ctx context.Context, pool *pgxpool.Pool, listID string) ([]ShoppingListItem, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, text, checked, source_recipe_id, sort_order
		FROM shopping_list_items
		WHERE list_id = $1
		ORDER BY sort_order ASC, created_at ASC
	`, listID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []ShoppingListItem
	for rows.Next() {
		var item ShoppingListItem
		if err := rows.Scan(&item.ID, &item.Text, &item.Checked, &item.SourceRecipeID, &item.SortOrder); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if items == nil {
		items = []ShoppingListItem{}
	}
	return items, rows.Err()
}
