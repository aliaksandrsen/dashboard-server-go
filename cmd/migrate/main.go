package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

func main() {
	baseline := flag.Bool("baseline", false, "mark all migrations as applied without executing SQL")
	dir := flag.String("dir", "migrations", "path to SQL migrations directory")
	flag.Parse()

	_ = godotenv.Load()
	dbURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if dbURL == "" {
		fmt.Fprintln(os.Stderr, "DATABASE_URL is required")
		os.Exit(1)
	}

	cleanURL, err := sanitizeDatabaseURL(dbURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid DATABASE_URL: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, cleanURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to connect to database: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := runMigrations(ctx, pool, *dir, *baseline); err != nil {
		fmt.Fprintf(os.Stderr, "migration failed: %v\n", err)
		os.Exit(1)
	}
}

func sanitizeDatabaseURL(rawURL string) (string, error) {
	// Strip Prisma-specific ?schema= parameter that pgx rejects as an unknown runtime option.
	idx := strings.IndexByte(rawURL, '?')
	if idx == -1 {
		return rawURL, nil
	}
	base := rawURL[:idx]
	queryPart := rawURL[idx+1:]
	parts := strings.Split(queryPart, "&")
	kept := make([]string, 0, len(parts))
	var schemaVal string
	hasSearchPath := false
	for _, p := range parts {
		if p == "" {
			continue
		}
		kv := strings.SplitN(p, "=", 2)
		if kv[0] == "schema" {
			if len(kv) == 2 {
				schemaVal = kv[1]
			}
			continue
		}
		if kv[0] == "search_path" {
			hasSearchPath = true
		}
		kept = append(kept, p)
	}
	if schemaVal != "" && !hasSearchPath {
		kept = append(kept, "search_path="+schemaVal)
	}
	if len(kept) == 0 {
		return base, nil
	}
	return base + "?" + strings.Join(kept, "&"), nil
}

func runMigrations(ctx context.Context, pool *pgxpool.Pool, dir string, baseline bool) error {
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS "schema_migrations" (
			"filename" TEXT PRIMARY KEY,
			"applied_at" TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
	`)
	if err != nil {
		return fmt.Errorf("create schema_migrations table: %w", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read migrations directory %q: %w", dir, err)
	}

	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			files = append(files, entry.Name())
		}
	}
	sort.Strings(files)

	applied := make(map[string]bool)
	rows, err := pool.Query(ctx, `SELECT "filename" FROM "schema_migrations"`)
	if err != nil {
		return fmt.Errorf("query applied migrations: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return fmt.Errorf("scan applied migration: %w", err)
		}
		applied[name] = true
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate applied migrations: %w", err)
	}

	for _, name := range files {
		if applied[name] {
			continue
		}

		tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return fmt.Errorf("begin tx for %s: %w", name, err)
		}

		if !baseline {
			sqlBytes, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				_ = tx.Rollback(ctx)
				return fmt.Errorf("read %s: %w", name, err)
			}
			if _, err := tx.Exec(ctx, string(sqlBytes)); err != nil {
				_ = tx.Rollback(ctx)
				return fmt.Errorf("execute %s: %w", name, err)
			}
		}

		if _, err := tx.Exec(ctx, `INSERT INTO "schema_migrations" ("filename") VALUES ($1)`, name); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("record %s: %w", name, err)
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit %s: %w", name, err)
		}

		if baseline {
			fmt.Printf("Baselined %s\n", name)
		} else {
			fmt.Printf("Applied %s\n", name)
		}
	}

	return nil
}
