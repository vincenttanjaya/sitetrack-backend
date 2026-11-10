package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	httpapi "github.com/vincenttanjaya/sitetrack-backend/internal/adapters/http"
	"github.com/vincenttanjaya/sitetrack-backend/internal/adapters/postgres"
	"github.com/vincenttanjaya/sitetrack-backend/internal/application"
	"github.com/vincenttanjaya/sitetrack-backend/internal/platform"
	"github.com/vincenttanjaya/sitetrack-backend/migrations"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil {
		slog.Error("SiteTrack exited", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	cfg, err := platform.LoadConfig()
	if err != nil {
		return err
	}
	if cfg.EphemeralJWT {
		slog.Info("using a process-local JWT signing key")
	}

	startup, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	db, err := postgres.Open(startup, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect to PostgreSQL: %w", err)
	}
	pool, err := db.DB()
	if err != nil {
		return err
	}
	defer pool.Close()
	if _, err := pool.ExecContext(startup, "SELECT now() AT TIME ZONE $1::text", cfg.Location.String()); err != nil {
		return fmt.Errorf("validate BUSINESS_TIMEZONE in PostgreSQL: %w", err)
	}
	if err := migrations.Apply(startup, pool); err != nil {
		return err
	}
	cancel()

	auth := application.NewAuth(postgres.NewAuthRepository(db), cfg.JWTSecret, time.Now, cfg.AccessTTL, cfg.RefreshTTL)
	router := httpapi.NewRouter(auth, pool.PingContext)
	router.RegisterProjectRoutes(application.NewProjects(postgres.NewProjectRepository(db), time.Now, cfg.Location))
	router.RegisterTaskRoutes(application.NewTasks(postgres.NewTaskRepository(db), time.Now, cfg.Location))
	router.RegisterInventoryRoutes(application.NewInventory(postgres.NewInventoryRepository(db), cfg.Location))
	server := &http.Server{
		Addr: cfg.Addr, Handler: router,
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second,
		WriteTimeout: 15 * time.Second, IdleTimeout: time.Minute, MaxHeaderBytes: 32 << 10,
	}
	result := make(chan error, 1)
	go func() {
		slog.Info("SiteTrack listening", "addr", cfg.Addr)
		result <- server.ListenAndServe()
	}()
	select {
	case err := <-result:
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			_ = server.Close()
			return err
		}
		if err := <-result; !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
}
