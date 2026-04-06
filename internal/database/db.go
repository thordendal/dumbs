package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DB wraps a pgxpool.Pool and exposes a minimal API used by the rest of dumbs.
type DB struct {
	pool *pgxpool.Pool
}

// Connect creates a connection pool for dsn, verifies it with a ping, and
// returns a ready-to-use DB. The caller must call Close when done.
func Connect(ctx context.Context, dsn string) (*DB, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}
	cfg.MaxConns = 20
	cfg.MinConns = 2
	cfg.MaxConnLifetime = 30 * time.Minute
	cfg.MaxConnIdleTime = 5 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return &DB{pool: pool}, nil
}

// Pool returns the underlying *pgxpool.Pool for use by chaos workers.
func (db *DB) Pool() *pgxpool.Pool {
	return db.pool
}

// InitSchema creates the events table if it does not already exist.
// Idempotent — safe to call on every startup.
func (db *DB) InitSchema(ctx context.Context) error {
	_, err := db.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS events (
			id         BIGSERIAL   PRIMARY KEY,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			payload    TEXT
		)
	`)
	if err != nil {
		return fmt.Errorf("init schema: %w", err)
	}
	return nil
}

// ResetSchema drops the events table and recreates it from scratch.
// Useful for cleaning up after database chaos without restarting the app.
func (db *DB) ResetSchema(ctx context.Context) error {
	_, err := db.pool.Exec(ctx, `DROP TABLE IF EXISTS events`)
	if err != nil {
		return fmt.Errorf("drop table: %w", err)
	}
	return db.InitSchema(ctx)
}

// Ping checks that the database is reachable. Used by the readiness probe.
func (db *DB) Ping(ctx context.Context) error {
	return db.pool.Ping(ctx)
}

// Close releases all pooled connections. Safe to call on a nil *DB.
func (db *DB) Close() {
	if db != nil {
		db.pool.Close()
	}
}
