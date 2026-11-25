package postgres

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SanitizeDatabaseURL removes the Prisma-specific ?schema= query parameter
// (which pgx treats as an unknown PostgreSQL runtime parameter) and translates
// it to search_path if search_path is not already set.
func SanitizeDatabaseURL(rawURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", fmt.Errorf("parse database URL: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid database URL: missing scheme or host")
	}

	query := parsed.Query()
	if schema := query.Get("schema"); schema != "" {
		query.Del("schema")
		if query.Get("search_path") == "" {
			query.Set("search_path", schema)
		}
		parsed.RawQuery = query.Encode()
	}

	return parsed.String(), nil
}

// NewPool creates a pgxpool.Pool and verifies connectivity via Ping.
func NewPool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	cleanURL, err := SanitizeDatabaseURL(databaseURL)
	if err != nil {
		return nil, err
	}

	pool, err := pgxpool.New(ctx, cleanURL)
	if err != nil {
		return nil, fmt.Errorf("create pgx pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	return pool, nil
}
