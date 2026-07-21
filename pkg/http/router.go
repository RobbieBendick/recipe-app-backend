package http

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/robbi/recipe-app-backend/pkg/config"
	"github.com/robbi/recipe-app-backend/pkg/http/handlers"
)

func NewRouter(cfg config.Config, pool *pgxpool.Pool) http.Handler {
	api := &handlers.API{DB: pool}
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   splitOrigins(cfg.AllowedOrigin),
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	r.Get("/health", api.Health)

	r.Route("/api", func(r chi.Router) {
		r.Route("/recipes", func(r chi.Router) {
			r.Get("/", api.ListRecipes)
			r.Post("/", api.CreateRecipe)
			r.Route("/{id}", func(r chi.Router) {
				r.Get("/", api.GetRecipe)
				r.Put("/", api.UpdateRecipe)
				r.Delete("/", api.DeleteRecipe)
				r.Post("/shopping-list", api.MakeShoppingListFromRecipe)
			})
		})

		r.Route("/shopping-lists", func(r chi.Router) {
			r.Get("/", api.ListShoppingLists)
			r.Post("/", api.CreateShoppingList)
			r.Route("/{id}", func(r chi.Router) {
				r.Get("/", api.GetShoppingList)
				r.Put("/", api.UpdateShoppingList)
				r.Delete("/", api.DeleteShoppingList)
				r.Post("/items", api.AddShoppingListItem)
				r.Post("/recipes", api.AddRecipeToList)
				r.Route("/items/{itemId}", func(r chi.Router) {
					r.Patch("/", api.ToggleShoppingListItem)
					r.Delete("/", api.RemoveShoppingListItem)
				})
			})
		})

		r.Route("/pantry", func(r chi.Router) {
			r.Get("/", api.ListPantry)
			r.Put("/", api.ReplacePantry)
			r.Post("/", api.CreatePantryItem)
			r.Route("/{id}", func(r chi.Router) {
				r.Put("/", api.UpdatePantryItem)
				r.Patch("/", api.TogglePantryStock)
				r.Delete("/", api.DeletePantryItem)
			})
		})
	})

	return r
}

func splitOrigins(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	if len(out) == 0 {
		return []string{"*"}
	}
	return out
}
