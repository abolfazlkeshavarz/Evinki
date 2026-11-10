// Package db wraps pgx: pool, embedded migrations and small generic helpers
// that keep handlers short (All / One / Tx).
package db

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("not found")

type DB struct{ *pgxpool.Pool }

// Q is satisfied by both the pool and a transaction, so helpers work in either.
type Q interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func Connect(ctx context.Context, url string) (*DB, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = 20
	cfg.ConnConfig.RuntimeParams["timezone"] = "Asia/Tehran"
	cfg.MaxConnLifetime = time.Hour
	var pool *pgxpool.Pool
	// The database container may still be starting; retry for a little while.
	for i := 0; i < 30; i++ {
		pool, err = pgxpool.NewWithConfig(ctx, cfg)
		if err == nil {
			if err = pool.Ping(ctx); err == nil {
				return &DB{pool}, nil
			}
			pool.Close()
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return nil, fmt.Errorf("database not reachable: %w", err)
}

// All returns every row mapped onto T by column name (`db:"col"` tags).
func All[T any](ctx context.Context, q Q, sql string, args ...any) ([]T, error) {
	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, pgx.RowToStructByNameLax[T])
	if out == nil {
		out = []T{}
	}
	return out, err
}

func One[T any](ctx context.Context, q Q, sql string, args ...any) (T, error) {
	var zero T
	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return zero, err
	}
	v, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[T])
	if errors.Is(err, pgx.ErrNoRows) {
		return zero, ErrNotFound
	}
	return v, err
}

// Scalar reads a single value.
func Scalar[T any](ctx context.Context, q Q, sql string, args ...any) (T, error) {
	var v T
	err := q.QueryRow(ctx, sql, args...).Scan(&v)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, ErrNotFound
	}
	return v, err
}

// Tx runs fn in a transaction and rolls back on error or panic.
func (d *DB) Tx(ctx context.Context, fn func(tx pgx.Tx) error) (err error) {
	tx, err := d.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback(ctx)
			panic(p)
		}
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()
	if err = fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// IsUnique / IsFK / IsCheck classify Postgres errors for friendly API answers.
func pgCode(err error) string {
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		return pe.Code
	}
	return ""
}
func IsUnique(err error) bool { return pgCode(err) == "23505" }
func IsFK(err error) bool     { return pgCode(err) == "23503" }
func IsCheck(err error) bool  { return pgCode(err) == "23514" }

// ConstraintName returns the violated constraint, for messages.
func ConstraintName(err error) string {
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		return pe.ConstraintName
	}
	return ""
}

// ─────────── migrations ───────────

type Migration struct {
	Version string
	SQL     string
}

func Migrations(fsys fs.FS) ([]Migration, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, err
	}
	var out []Migration
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		b, err := fs.ReadFile(fsys, e.Name())
		if err != nil {
			return nil, err
		}
		out = append(out, Migration{Version: strings.TrimSuffix(e.Name(), ".sql"), SQL: string(b)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out, nil
}

// Migrate applies pending migrations, each in its own transaction, under an
// advisory lock so two containers starting together cannot race.
func (d *DB) Migrate(ctx context.Context, fsys fs.FS, log func(string)) error {
	conn, err := d.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	if _, err = conn.Exec(ctx, `SELECT pg_advisory_lock(727274)`); err != nil {
		return err
	}
	defer conn.Exec(context.Background(), `SELECT pg_advisory_unlock(727274)`)

	if _, err = conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`); err != nil {
		return err
	}
	applied := map[string]bool{}
	rows, err := conn.Query(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return err
		}
		applied[v] = true
	}
	rows.Close()

	ms, err := Migrations(fsys)
	if err != nil {
		return err
	}
	for _, m := range ms {
		if applied[m.Version] {
			continue
		}
		tx, err := conn.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, m.SQL); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("migration %s: %w", m.Version, err)
		}
		if _, err = tx.Exec(ctx, `INSERT INTO schema_migrations(version) VALUES ($1)`, m.Version); err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
		if err = tx.Commit(ctx); err != nil {
			return err
		}
		if log != nil {
			log("applied " + m.Version)
		}
	}
	return nil
}
