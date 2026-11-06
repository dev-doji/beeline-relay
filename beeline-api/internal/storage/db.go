// Package storage owns the PostgreSQL connection pool and schema migrations.
package storage

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/golang-migrate/migrate/v4"
	migratepgx "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/dev-doji/beeline-relay/beeline-api/migrations"
)

// Open connects to PostgreSQL and waits until the database answers.
//
// Each connection attempt dials IPv4 first and falls back to any address family. Some
// hosts (for example Render) have no IPv6 egress, and the database host publishes IPv6
// records, so a plain dial can hang instead of failing. Every step has a time limit, and
// failed attempts are logged (host only, never the password) and retried.
func Open(ctx context.Context, databaseURL string, log *slog.Logger) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database URL: %w", err)
	}
	// Return every timestamp in UTC regardless of server or session defaults.
	cfg.ConnConfig.RuntimeParams["timezone"] = "UTC"
	cfg.ConnConfig.ConnectTimeout = 15 * time.Second
	cfg.ConnConfig.DialFunc = dialPreferIPv4

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	host := cfg.ConnConfig.Host
	const attempts = 6
	for i := 1; ; i++ {
		pingCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		err = pool.Ping(pingCtx)
		cancel()
		if err == nil {
			if log != nil {
				log.Info("database connected", "host", host, "attempt", i)
			}
			return pool, nil
		}
		if log != nil {
			log.Warn("database not reachable yet", "host", host, "attempt", i, "error", err)
		}
		if i == attempts || ctx.Err() != nil {
			pool.Close()
			return nil, fmt.Errorf("ping database %s after %d attempts: %w", host, i, err)
		}
		select {
		case <-ctx.Done():
		case <-time.After(5 * time.Second):
		}
	}
}

func dialPreferIPv4(ctx context.Context, network, addr string) (net.Conn, error) {
	d := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	if network == "tcp" {
		if conn, err := d.DialContext(ctx, "tcp4", addr); err == nil {
			return conn, nil
		}
	}
	return d.DialContext(ctx, network, addr)
}

// Migrate applies all pending up migrations.
func Migrate(pool *pgxpool.Pool) error {
	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("load migrations: %w", err)
	}
	db := stdlib.OpenDBFromPool(pool)
	driver, err := migratepgx.WithInstance(db, &migratepgx.Config{})
	if err != nil {
		db.Close()
		return fmt.Errorf("migration driver: %w", err)
	}
	m, err := migrate.NewWithInstance("iofs", src, "pgx5", driver)
	if err != nil {
		driver.Close()
		return fmt.Errorf("migrator: %w", err)
	}
	// The driver holds a dedicated pool connection; Close returns it. Without this the
	// connection leaks and pool.Close blocks forever on shutdown.
	defer m.Close()
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}
