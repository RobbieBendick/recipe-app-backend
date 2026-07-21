package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/robbi/recipe-app-backend/pkg/config"
	"github.com/robbi/recipe-app-backend/pkg/db"
	httpserver "github.com/robbi/recipe-app-backend/pkg/http"
)

func main() {
	_ = godotenv.Load()

	cfg := config.Load()

	if cfg.Environment == "production" {
		if cfg.DatabaseURL == "" {
			log.Fatal(
				"No database URL found. Set DATABASE_URL or POSTGRES_URL_NON_POOLING " +
					"(Neon/Vercel env).",
			)
		}
	}

	ctx := context.Background()
	dbPool, err := db.NewPostgresPool(ctx, cfg.DatabaseURL, cfg.DatabasePoolMaxConns)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer dbPool.Close()

	if err := db.Migrate(ctx, dbPool); err != nil {
		log.Fatalf("failed to run migrations: %v", err)
	}

	writeTimeout := time.Duration(cfg.HTTPWriteTimeoutSec) * time.Second
	addr := "0.0.0.0:" + cfg.Port
	server := &http.Server{
		Addr:         addr,
		Handler:      httpserver.NewRouter(cfg, dbPool),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: writeTimeout,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("recipe-app-backend listening on %s (try: curl http://127.0.0.1:%s/health)", addr, cfg.Port)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("http server failed: %v", err)
		}
	}()

	shutdownCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	<-shutdownCtx.Done()
	log.Println("shutdown signal received")

	ctxTimeout, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctxTimeout); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
	}
}
