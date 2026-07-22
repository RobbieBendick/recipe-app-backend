package http

import (
	"log"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/robbi/recipe-app-backend/pkg/auth"
	"github.com/robbi/recipe-app-backend/pkg/config"
	"github.com/robbi/recipe-app-backend/pkg/http/handlers"
	"github.com/robbi/recipe-app-backend/pkg/kroger"
)

func NewRouter(cfg config.Config, pool *pgxpool.Pool) http.Handler {
	authService, err := auth.NewService(cfg.JWTSecret, cfg.GoogleClientID)
	if err != nil {
		log.Printf("auth config warning: %v (using empty JWT will reject protected routes)", err)
		// Fall back so the process can still boot health checks; protected routes need a real secret.
		authService, _ = auth.NewService(fallbackJWTSecret(cfg), cfg.GoogleClientID)
	}

	krogerClient := kroger.NewClient(cfg.KrogerClientID, cfg.KrogerClientSecret, cfg.KrogerAPIBaseURL)
	if !krogerClient.Configured() {
		log.Printf("kroger credentials missing: price estimates disabled until KROGER_CLIENT_ID/SECRET are set")
	}

	api := &handlers.API{
		DB:         pool,
		Auth:       authService,
		Kroger:     krogerClient,
		DefaultZip: cfg.KrogerDefaultZip,
	}
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
		r.Route("/auth", func(r chi.Router) {
			r.Post("/register", api.Register)
			r.Post("/login", api.Login)
			r.Post("/google", api.GoogleAuth)
			r.Post("/logout", api.Logout)
			r.With(authService.Middleware).Get("/me", api.Me)
		})

		r.Group(func(r chi.Router) {
			r.Use(authService.Middleware)

			r.Route("/recipes", func(r chi.Router) {
				r.Get("/", api.ListRecipes)
				r.Post("/", api.CreateRecipe)
				r.Route("/{id}", func(r chi.Router) {
					r.Get("/", api.GetRecipe)
					r.Put("/", api.UpdateRecipe)
					r.Delete("/", api.DeleteRecipe)
					r.Post("/shopping-list", api.MakeShoppingListFromRecipe)
					r.Post("/share", api.ShareRecipe)
				})
			})

			r.Route("/recipe-shares", func(r chi.Router) {
				r.Post("/{id}/accept", api.AcceptRecipeShare)
				r.Post("/{id}/decline", api.DeclineRecipeShare)
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

			r.Route("/estimates", func(r chi.Router) {
				r.Get("/status", api.EstimateStatus)
				r.Get("/store", api.GetEstimateStore)
				r.Put("/store", api.SaveEstimateStore)
				r.Post("/cost", api.EstimateCost)
				r.Post("/products", api.EstimateProducts)
			})

			r.Route("/friends", func(r chi.Router) {
				r.Get("/", api.ListFriends)
				r.Get("/requests", api.ListFriendRequests)
				r.Post("/requests", api.SendFriendRequest)
				r.Post("/requests/{id}/accept", api.AcceptFriendRequest)
				r.Post("/requests/{id}/decline", api.DeclineFriendRequest)
				r.Get("/{userId}/shared-list", api.GetOrCreateSharedShoppingList)
				r.Delete("/{userId}", api.RemoveFriend)
			})

			r.Route("/notifications", func(r chi.Router) {
				r.Get("/", api.ListNotifications)
				r.Get("/unread-count", api.UnreadNotificationCount)
				r.Post("/read-all", api.MarkAllNotificationsRead)
				r.Delete("/", api.ClearAllNotifications)
				r.Post("/{id}/read", api.MarkNotificationRead)
				r.Delete("/{id}", api.DeleteNotification)
			})
		})
	})

	return r
}

func fallbackJWTSecret(cfg config.Config) string {
	if strings.TrimSpace(cfg.JWTSecret) != "" {
		return cfg.JWTSecret
	}
	if cfg.Environment == "production" {
		return "missing-jwt-secret-reject-tokens"
	}
	return "dev-only-jwt-secret-change-me"
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
