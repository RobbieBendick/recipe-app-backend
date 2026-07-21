package handler

import (
	"context"
	"log"
	"net/http"
	"sync"

	"github.com/robbi/recipe-app-backend/pkg/config"
	"github.com/robbi/recipe-app-backend/pkg/db"
	httpserver "github.com/robbi/recipe-app-backend/pkg/http"
)

var (
	initOnce sync.Once
	router   http.Handler
	initErr  error
)

// Handler is the Vercel serverless entrypoint.
func Handler(w http.ResponseWriter, r *http.Request) {
	initOnce.Do(func() {
		cfg := config.Load()
		ctx := context.Background()

		pool, err := db.NewPostgresPool(ctx, cfg.DatabaseURL, cfg.DatabasePoolMaxConns)
		if err != nil {
			initErr = err
			log.Printf("db connect failed: %v", err)
			return
		}

		if err := db.Migrate(ctx, pool); err != nil {
			initErr = err
			log.Printf("migrate failed: %v", err)
			return
		}

		router = httpserver.NewRouter(cfg, pool)
	})

	if initErr != nil {
		http.Error(w, "service unavailable: "+initErr.Error(), http.StatusServiceUnavailable)
		return
	}

	router.ServeHTTP(w, r)
}
