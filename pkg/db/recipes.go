package db

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Recipe struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Description  string    `json:"description"`
	Emoji        string    `json:"emoji"`
	Ingredients  []string  `json:"ingredients"`
	Steps        []string  `json:"steps"`
	PrepMinutes  int       `json:"prepMinutes"`
	CookMinutes  int       `json:"cookMinutes"`
	Servings     int       `json:"servings"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type RecipeInput struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Emoji       string   `json:"emoji"`
	Ingredients []string `json:"ingredients"`
	Steps       []string `json:"steps"`
	PrepMinutes int      `json:"prepMinutes"`
	CookMinutes int      `json:"cookMinutes"`
	Servings    int      `json:"servings"`
}

type ShoppingListItem struct {
	ID             string  `json:"id"`
	Text           string  `json:"text"`
	Checked        bool    `json:"checked"`
	SourceRecipeID *string `json:"sourceRecipeId,omitempty"`
	SortOrder      int     `json:"sortOrder"`
}

type ShoppingList struct {
	ID           string             `json:"id"`
	Title        string             `json:"title"`
	Emoji        string             `json:"emoji"`
	Items        []ShoppingListItem `json:"items"`
	RecipeCounts map[string]int     `json:"recipeCounts"`
	SharedWith   *PublicUser        `json:"sharedWith,omitempty"`
	CreatedAt    time.Time          `json:"createdAt"`
	UpdatedAt    time.Time          `json:"updatedAt"`
}

type ShoppingListInput struct {
	Title          string         `json:"title"`
	Emoji          string         `json:"emoji"`
	Items          []string       `json:"items"`
	SourceRecipeID *string        `json:"sourceRecipeId,omitempty"`
	RecipeCounts   map[string]int `json:"recipeCounts,omitempty"`
}

func ListRecipes(ctx context.Context, pool *pgxpool.Pool, userID string) ([]Recipe, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, title, description, emoji, ingredients, steps,
			COALESCE(prep_minutes, 0), COALESCE(cook_minutes, 0), COALESCE(servings, 0),
			created_at, updated_at
		FROM recipes
		WHERE user_id = $1
		ORDER BY updated_at DESC
	`, userID)
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

func GetRecipe(ctx context.Context, pool *pgxpool.Pool, userID, id string) (*Recipe, error) {
	row := pool.QueryRow(ctx, `
		SELECT id, title, description, emoji, ingredients, steps,
			COALESCE(prep_minutes, 0), COALESCE(cook_minutes, 0), COALESCE(servings, 0),
			created_at, updated_at
		FROM recipes
		WHERE id = $1 AND user_id = $2
	`, id, userID)
	r, err := scanRecipe(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func CreateRecipe(ctx context.Context, pool *pgxpool.Pool, userID string, in RecipeInput) (*Recipe, error) {
	row := pool.QueryRow(ctx, `
		INSERT INTO recipes (user_id, title, description, emoji, ingredients, steps, prep_minutes, cook_minutes, servings)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, title, description, emoji, ingredients, steps,
			COALESCE(prep_minutes, 0), COALESCE(cook_minutes, 0), COALESCE(servings, 0),
			created_at, updated_at
	`, userID, in.Title, in.Description, in.Emoji, in.Ingredients, in.Steps,
		clampNonNeg(in.PrepMinutes), clampNonNeg(in.CookMinutes), clampNonNeg(in.Servings))
	r, err := scanRecipe(row)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func UpdateRecipe(ctx context.Context, pool *pgxpool.Pool, userID, id string, in RecipeInput) (*Recipe, error) {
	row := pool.QueryRow(ctx, `
		UPDATE recipes
		SET title = $3,
			description = $4,
			emoji = $5,
			ingredients = $6,
			steps = $7,
			prep_minutes = $8,
			cook_minutes = $9,
			servings = $10,
			updated_at = now()
		WHERE id = $1 AND user_id = $2
		RETURNING id, title, description, emoji, ingredients, steps,
			COALESCE(prep_minutes, 0), COALESCE(cook_minutes, 0), COALESCE(servings, 0),
			created_at, updated_at
	`, id, userID, in.Title, in.Description, in.Emoji, in.Ingredients, in.Steps,
		clampNonNeg(in.PrepMinutes), clampNonNeg(in.CookMinutes), clampNonNeg(in.Servings))
	r, err := scanRecipe(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func clampNonNeg(n int) int {
	if n < 0 {
		return 0
	}
	if n > 9999 {
		return 9999
	}
	return n
}

func DeleteRecipe(ctx context.Context, pool *pgxpool.Pool, userID, id string) (bool, error) {
	tag, err := pool.Exec(ctx, `DELETE FROM recipes WHERE id = $1 AND user_id = $2`, id, userID)
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
	err := row.Scan(
		&r.ID, &r.Title, &r.Description, &r.Emoji, &ingredients, &steps,
		&r.PrepMinutes, &r.CookMinutes, &r.Servings,
		&r.CreatedAt, &r.UpdatedAt,
	)
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

func ListShoppingLists(ctx context.Context, pool *pgxpool.Pool, userID string) ([]ShoppingList, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, title, emoji, COALESCE(recipe_counts, '{}'::jsonb), created_at, updated_at
		FROM shopping_lists
		WHERE user_id = $1
		  AND NOT EXISTS (
			SELECT 1 FROM shopping_list_shares s WHERE s.list_id = shopping_lists.id
		  )
		ORDER BY updated_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ShoppingList
	for rows.Next() {
		var list ShoppingList
		if err := rows.Scan(&list.ID, &list.Title, &list.Emoji, &list.RecipeCounts, &list.CreatedAt, &list.UpdatedAt); err != nil {
			return nil, err
		}
		normalizeRecipeCounts(&list)
		items, err := listItems(ctx, pool, list.ID)
		if err != nil {
			return nil, err
		}
		list.Items = items
		out = append(out, list)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	shared, err := ListSharedShoppingLists(ctx, pool, userID)
	if err != nil {
		return nil, err
	}
	out = append(out, shared...)
	if out == nil {
		out = []ShoppingList{}
	}
	return out, nil
}

func GetShoppingList(ctx context.Context, pool *pgxpool.Pool, userID, id string) (*ShoppingList, error) {
	ok, err := canAccessShoppingList(ctx, pool, userID, id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}

	var list ShoppingList
	err = pool.QueryRow(ctx, `
		SELECT id, title, emoji, COALESCE(recipe_counts, '{}'::jsonb), created_at, updated_at
		FROM shopping_lists
		WHERE id = $1
	`, id).Scan(&list.ID, &list.Title, &list.Emoji, &list.RecipeCounts, &list.CreatedAt, &list.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	normalizeRecipeCounts(&list)
	items, err := listItems(ctx, pool, id)
	if err != nil {
		return nil, err
	}
	list.Items = items
	if err := attachSharedWith(ctx, pool, userID, &list); err != nil {
		return nil, err
	}
	return &list, nil
}

func CreateShoppingList(ctx context.Context, pool *pgxpool.Pool, userID string, in ShoppingListInput) (*ShoppingList, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	emoji := in.Emoji
	if emoji == "" {
		emoji = "🛒"
	}
	counts := cleanRecipeCounts(in.RecipeCounts)
	if in.SourceRecipeID != nil && *in.SourceRecipeID != "" {
		var decoded map[string]int
		_ = json.Unmarshal(counts, &decoded)
		if decoded == nil {
			decoded = map[string]int{}
		}
		if decoded[*in.SourceRecipeID] <= 0 {
			decoded[*in.SourceRecipeID] = 1
		}
		counts = cleanRecipeCounts(decoded)
	}

	var list ShoppingList
	err = tx.QueryRow(ctx, `
		INSERT INTO shopping_lists (user_id, title, emoji, recipe_counts)
		VALUES ($1, $2, $3, COALESCE($4::jsonb, '{}'::jsonb))
		RETURNING id, title, emoji, COALESCE(recipe_counts, '{}'::jsonb), created_at, updated_at
	`, userID, in.Title, emoji, counts).Scan(&list.ID, &list.Title, &list.Emoji, &list.RecipeCounts, &list.CreatedAt, &list.UpdatedAt)
	if err != nil {
		return nil, err
	}
	normalizeRecipeCounts(&list)

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
	return GetShoppingList(ctx, pool, userID, list.ID)
}

func UpdateShoppingListTitle(ctx context.Context, pool *pgxpool.Pool, userID, id, title string) (*ShoppingList, error) {
	ok, err := canAccessShoppingList(ctx, pool, userID, id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}

	title = strings.TrimSpace(title)
	if title == "" {
		return nil, ErrInvalidShoppingListTitle
	}

	tag, err := pool.Exec(ctx, `
		UPDATE shopping_lists
		SET title = $2, updated_at = now()
		WHERE id = $1
	`, id, title)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, nil
	}
	return GetShoppingList(ctx, pool, userID, id)
}

func UpdateShoppingList(ctx context.Context, pool *pgxpool.Pool, userID, id string, in ShoppingListInput) (*ShoppingList, error) {
	ok, err := canAccessShoppingList(ctx, pool, userID, id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	emoji := in.Emoji
	if emoji == "" {
		emoji = "🛒"
	}
	counts := cleanRecipeCounts(in.RecipeCounts)

	tag, err := tx.Exec(ctx, `
		UPDATE shopping_lists
		SET title = $2, emoji = $3, recipe_counts = COALESCE($4::jsonb, '{}'::jsonb), updated_at = now()
		WHERE id = $1
	`, id, in.Title, emoji, counts)
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
	return GetShoppingList(ctx, pool, userID, id)
}

func DeleteShoppingList(ctx context.Context, pool *pgxpool.Pool, userID, id string) (bool, error) {
	ok, err := canAccessShoppingList(ctx, pool, userID, id)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}

	var shared bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM shopping_list_shares WHERE list_id = $1)
	`, id).Scan(&shared); err != nil {
		return false, err
	}
	if shared {
		return false, ErrCannotDeleteSharedList
	}

	tag, err := pool.Exec(ctx, `DELETE FROM shopping_lists WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func canAccessShoppingList(ctx context.Context, pool *pgxpool.Pool, userID, listID string) (bool, error) {
	var exists bool
	err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM shopping_lists WHERE id = $1 AND user_id = $2
			UNION ALL
			SELECT 1 FROM shopping_list_shares
			WHERE list_id = $1 AND (user_a = $2 OR user_b = $2)
		)
	`, listID, userID).Scan(&exists)
	return exists, err
}

func ownsList(ctx context.Context, pool *pgxpool.Pool, userID, listID string) (bool, error) {
	return canAccessShoppingList(ctx, pool, userID, listID)
}

func touchShoppingList(ctx context.Context, pool *pgxpool.Pool, listID string) {
	_, _ = pool.Exec(ctx, `UPDATE shopping_lists SET updated_at = now() WHERE id = $1`, listID)
}

func ToggleShoppingListItem(ctx context.Context, pool *pgxpool.Pool, userID, listID, itemID string) (*ShoppingList, error) {
	ok, err := canAccessShoppingList(ctx, pool, userID, listID)
	if err != nil || !ok {
		return nil, err
	}
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
	touchShoppingList(ctx, pool, listID)
	return GetShoppingList(ctx, pool, userID, listID)
}

func AddShoppingListItem(ctx context.Context, pool *pgxpool.Pool, userID, listID, text string) (*ShoppingList, error) {
	ok, err := canAccessShoppingList(ctx, pool, userID, listID)
	if err != nil || !ok {
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

	_, err = pool.Exec(ctx, `
		INSERT INTO shopping_list_items (list_id, text, sort_order)
		VALUES ($1, $2, $3)
	`, listID, text, next)
	if err != nil {
		return nil, err
	}
	touchShoppingList(ctx, pool, listID)
	return GetShoppingList(ctx, pool, userID, listID)
}

func RemoveShoppingListItem(ctx context.Context, pool *pgxpool.Pool, userID, listID, itemID string) (*ShoppingList, error) {
	ok, err := canAccessShoppingList(ctx, pool, userID, listID)
	if err != nil || !ok {
		return nil, err
	}
	tag, err := pool.Exec(ctx, `
		DELETE FROM shopping_list_items WHERE id = $1 AND list_id = $2
	`, itemID, listID)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, nil
	}
	touchShoppingList(ctx, pool, listID)
	return GetShoppingList(ctx, pool, userID, listID)
}

func CreateShoppingListFromRecipe(ctx context.Context, pool *pgxpool.Pool, userID, recipeID string) (*ShoppingList, error) {
	recipe, err := GetRecipe(ctx, pool, userID, recipeID)
	if err != nil || recipe == nil {
		return nil, err
	}
	sourceID := recipe.ID
	emoji := recipe.Emoji
	if emoji == "" {
		emoji = "🛒"
	}
	return CreateShoppingList(ctx, pool, userID, ShoppingListInput{
		Title:          "Shop: " + recipe.Title,
		Emoji:          emoji,
		Items:          recipe.Ingredients,
		SourceRecipeID: &sourceID,
		RecipeCounts:   map[string]int{sourceID: 1},
	})
}

func AddRecipeToShoppingList(ctx context.Context, pool *pgxpool.Pool, userID, listID, recipeID string) (*ShoppingList, error) {
	recipe, err := GetRecipe(ctx, pool, userID, recipeID)
	if err != nil || recipe == nil {
		return nil, err
	}

	list, err := GetShoppingList(ctx, pool, userID, listID)
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

	counts := list.RecipeCounts
	if counts == nil {
		counts = map[string]int{}
	}
	counts[sourceID] = counts[sourceID] + 1
	_, err = pool.Exec(ctx, `
		UPDATE shopping_lists
		SET recipe_counts = COALESCE($2::jsonb, '{}'::jsonb), updated_at = now()
		WHERE id = $1
	`, listID, cleanRecipeCounts(counts))
	if err != nil {
		return nil, err
	}
	return GetShoppingList(ctx, pool, userID, listID)
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

func normalizeRecipeCounts(list *ShoppingList) {
	if list.RecipeCounts == nil {
		list.RecipeCounts = map[string]int{}
	}
}

func cleanRecipeCounts(counts map[string]int) []byte {
	cleaned := map[string]int{}
	for id, n := range counts {
		id = strings.TrimSpace(id)
		if id == "" || n <= 0 {
			continue
		}
		cleaned[id] = n
	}
	raw, err := json.Marshal(cleaned)
	if err != nil {
		return []byte("{}")
	}
	return raw
}
