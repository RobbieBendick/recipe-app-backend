package db

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	RecipeSharePending  = "pending"
	RecipeShareAccepted = "accepted"
	RecipeShareDeclined = "declined"
)

var (
	ErrRecipeShareNotFound = errors.New("recipe share not found")
	ErrNotFriends          = errors.New("not friends")
	ErrSharePending        = errors.New("recipe share already pending")
	ErrCannotShareSelf     = errors.New("cannot share a recipe with yourself")
)

type RecipeShare struct {
	ID                string    `json:"id"`
	RecipeID          *string   `json:"recipeId,omitempty"`
	FromUserID        string    `json:"fromUserId"`
	ToUserID          string    `json:"toUserId"`
	Status            string    `json:"status"`
	RecipeTitle       string    `json:"recipeTitle"`
	RecipeDescription string    `json:"recipeDescription"`
	RecipeEmoji       string    `json:"recipeEmoji"`
	RecipeIngredients []string  `json:"recipeIngredients"`
	RecipeSteps       []string  `json:"recipeSteps"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

func CreateRecipeShare(ctx context.Context, pool *pgxpool.Pool, fromUserID, toUserID string, recipe *Recipe) (*RecipeShare, error) {
	if fromUserID == toUserID {
		return nil, ErrCannotShareSelf
	}
	ok, err := AreFriends(ctx, pool, fromUserID, toUserID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrNotFriends
	}

	existing, err := getPendingRecipeShare(ctx, pool, fromUserID, toUserID, recipe.ID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrSharePending
	}

	ingredients := recipe.Ingredients
	if ingredients == nil {
		ingredients = []string{}
	}
	steps := recipe.Steps
	if steps == nil {
		steps = []string{}
	}

	row := pool.QueryRow(ctx, `
		INSERT INTO recipe_shares (
			recipe_id, from_user_id, to_user_id, status,
			recipe_title, recipe_description, recipe_emoji, recipe_ingredients, recipe_steps
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, recipe_id, from_user_id, to_user_id, status,
			recipe_title, recipe_description, recipe_emoji, recipe_ingredients, recipe_steps,
			created_at, updated_at
	`, recipe.ID, fromUserID, toUserID, RecipeSharePending,
		recipe.Title, recipe.Description, recipe.Emoji, ingredients, steps)
	return scanRecipeShare(row)
}

func getPendingRecipeShare(ctx context.Context, pool *pgxpool.Pool, fromUserID, toUserID, recipeID string) (*RecipeShare, error) {
	row := pool.QueryRow(ctx, `
		SELECT id, recipe_id, from_user_id, to_user_id, status,
			recipe_title, recipe_description, recipe_emoji, recipe_ingredients, recipe_steps,
			created_at, updated_at
		FROM recipe_shares
		WHERE from_user_id = $1 AND to_user_id = $2 AND recipe_id = $3 AND status = $4
		LIMIT 1
	`, fromUserID, toUserID, recipeID, RecipeSharePending)
	return scanRecipeShare(row)
}

func GetRecipeShareByID(ctx context.Context, pool *pgxpool.Pool, id string) (*RecipeShare, error) {
	row := pool.QueryRow(ctx, `
		SELECT id, recipe_id, from_user_id, to_user_id, status,
			recipe_title, recipe_description, recipe_emoji, recipe_ingredients, recipe_steps,
			created_at, updated_at
		FROM recipe_shares
		WHERE id = $1
	`, id)
	return scanRecipeShare(row)
}

func AcceptRecipeShare(ctx context.Context, pool *pgxpool.Pool, shareID, toUserID string) (*Recipe, *RecipeShare, error) {
	share, err := GetRecipeShareByID(ctx, pool, shareID)
	if err != nil {
		return nil, nil, err
	}
	if share == nil || share.ToUserID != toUserID || share.Status != RecipeSharePending {
		return nil, nil, ErrRecipeShareNotFound
	}

	recipe, err := CreateRecipe(ctx, pool, toUserID, RecipeInput{
		Title:       share.RecipeTitle,
		Description: share.RecipeDescription,
		Emoji:       share.RecipeEmoji,
		Ingredients: share.RecipeIngredients,
		Steps:       share.RecipeSteps,
	})
	if err != nil {
		return nil, nil, err
	}

	row := pool.QueryRow(ctx, `
		UPDATE recipe_shares
		SET status = $3, updated_at = now()
		WHERE id = $1 AND to_user_id = $2 AND status = $4
		RETURNING id, recipe_id, from_user_id, to_user_id, status,
			recipe_title, recipe_description, recipe_emoji, recipe_ingredients, recipe_steps,
			created_at, updated_at
	`, shareID, toUserID, RecipeShareAccepted, RecipeSharePending)
	updated, err := scanRecipeShare(row)
	if err != nil {
		return nil, nil, err
	}
	if updated == nil {
		return nil, nil, ErrRecipeShareNotFound
	}
	return recipe, updated, nil
}

func DeclineRecipeShare(ctx context.Context, pool *pgxpool.Pool, shareID, toUserID string) error {
	tag, err := pool.Exec(ctx, `
		UPDATE recipe_shares
		SET status = $3, updated_at = now()
		WHERE id = $1 AND to_user_id = $2 AND status = $4
	`, shareID, toUserID, RecipeShareDeclined, RecipeSharePending)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrRecipeShareNotFound
	}
	return nil
}

func scanRecipeShare(row pgx.Row) (*RecipeShare, error) {
	var s RecipeShare
	err := row.Scan(
		&s.ID, &s.RecipeID, &s.FromUserID, &s.ToUserID, &s.Status,
		&s.RecipeTitle, &s.RecipeDescription, &s.RecipeEmoji, &s.RecipeIngredients, &s.RecipeSteps,
		&s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if s.RecipeIngredients == nil {
		s.RecipeIngredients = []string{}
	}
	if s.RecipeSteps == nil {
		s.RecipeSteps = []string{}
	}
	return &s, nil
}
