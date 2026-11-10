package postgres

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/vincenttanjaya/sitetrack-backend/internal/domain"
	driver "gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Open(ctx context.Context, dsn string) (*gorm.DB, error) {
	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	return OpenPool(ctx, stdlib.OpenDB(*config))
}

// OpenPool configures a PostgreSQL pool and takes ownership of it.
func OpenPool(ctx context.Context, pool *sql.DB) (*gorm.DB, error) {
	db, err := gorm.Open(driver.New(driver.Config{Conn: pool}), &gorm.Config{
		DisableAutomaticPing: true,
		Logger:               logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		pool.Close()
		return nil, err
	}
	pool.SetMaxOpenConns(16)
	pool.SetMaxIdleConns(8)
	pool.SetConnMaxLifetime(30 * time.Minute)
	pool.SetConnMaxIdleTime(5 * time.Minute)
	if err := pool.PingContext(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return db, nil
}

func dbError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, sql.ErrNoRows) {
		return domain.ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return domain.Conflict("resource already exists")
		case "23503":
			return domain.Conflict("resource is referenced or no longer exists")
		case "22003":
			return domain.Conflict("numeric limit exceeded")
		}
	}
	return err
}

var likeEscaper = strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`)

func containsPattern(term string) string { return "%" + likeEscaper.Replace(term) + "%" }
