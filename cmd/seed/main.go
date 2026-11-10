package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/vincenttanjaya/sitetrack-backend/internal/adapters/postgres"
	"github.com/vincenttanjaya/sitetrack-backend/internal/domain"
	"github.com/vincenttanjaya/sitetrack-backend/internal/seed"
	"github.com/vincenttanjaya/sitetrack-backend/migrations"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "seed:", err)
		os.Exit(1)
	}
}

func run() error {
	preset := flag.String("preset", "small", "seed size: small or full")
	anchor := flag.String("anchor", time.Now().UTC().Format(time.DateOnly), "reference date, YYYY-MM-DD")
	reset := flag.Bool("reset", false, "replace all application data in the target database")
	flag.Parse()
	if flag.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return errors.New("DATABASE_URL is required")
	}
	date, err := domain.ParseDate(*anchor)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	connect, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	db, err := postgres.Open(connect, dsn)
	if err != nil {
		return err
	}
	pool, err := db.DB()
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := migrations.Apply(connect, pool); err != nil {
		return err
	}
	return seed.Populate(ctx, pool, seed.Options{Preset: *preset, Anchor: date, Reset: *reset, Progress: os.Stdout})
}
