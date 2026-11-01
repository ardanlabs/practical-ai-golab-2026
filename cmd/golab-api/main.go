package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
	"github.com/pressly/goose/v3"

	"github.com/ardanlabs/practical-ai-golab-2026/app/domain/userapp"
	"github.com/ardanlabs/practical-ai-golab-2026/app/sdk/api"
	"github.com/ardanlabs/practical-ai-golab-2026/business/domain/userbus"
	"github.com/ardanlabs/practical-ai-golab-2026/business/domain/userbus/stores/userdb"
)

// Config holds the configuration for the application.
type Config struct {
	DatabaseURL    string
	MigrationsPath string
	WebAddr        string
	ReadTimeout    time.Duration
	WriteTimeout   time.Duration
}

func loadConfig() Config {
	return Config{
		DatabaseURL:    cmp.Or(os.Getenv("DATABASE_URL"), "postgres://postgres:postgres@localhost:5432/golab?sslmode=disable"),
		MigrationsPath: cmp.Or(os.Getenv("MIGRATIONS_PATH"), "zarf/schemas/db"),
		WebAddr:        cmp.Or(os.Getenv("ADDR"), ":3000"),
		ReadTimeout:    5 * time.Second,
		WriteTimeout:   5 * time.Second,
	}
}

func main() {
	if err := run(); err != nil {
		slog.Error("startup", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := loadConfig()

	if err := migrate(ctx, cfg); err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}

	db, err := sqlx.ConnectContext(ctx, "pgx", cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connecting to postgres: %w", err)
	}
	defer db.Close()

	userStore := userdb.New(db)
	userService := userbus.New(userStore)
	userHandlers := userapp.NewHandlers(userService)

	mux := http.NewServeMux()
	userHandlers.Routes(mux)
	registerRoutes(mux)

	srv := http.Server{
		Addr:         cfg.WebAddr,
		Handler:      mux,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	}

	errs := make(chan error, 1)
	go func() {
		slog.Info("service started", "addr", cfg.WebAddr)
		errs <- srv.ListenAndServe()
	}()

	select {
	case err := <-errs:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("listening: %w", err)
		}
	case <-ctx.Done():
		slog.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutting down: %w", err)
	}

	slog.Info("service stopped")

	return nil
}

// registerRoutes wires the non-domain routes into the mux.
func registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("GET /hello/{user}", func(w http.ResponseWriter, r *http.Request) {
		msg := fmt.Sprintf("Hello, %s", r.PathValue("user"))
		if err := api.WriteText(w, http.StatusOK, msg); err != nil {
			slog.ErrorContext(r.Context(), "hello", "error", err)
		}
	})
}

// migrate applies any pending goose migrations against postgres.
func migrate(ctx context.Context, cfg Config) error {
	db, err := goose.OpenDBWithDriver("postgres", cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("opening goose connection: %w", err)
	}
	defer db.Close()

	goose.SetBaseFS(os.DirFS(cfg.MigrationsPath))

	if err := goose.RunContext(ctx, "up", db, "."); err != nil {
		return fmt.Errorf("applying migrations: %w", err)
	}

	slog.Info("migrations applied", "path", cfg.MigrationsPath)

	return nil
}
