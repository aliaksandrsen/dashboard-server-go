package main

import (
	"context"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"time"

	"github.com/aliaksandrsen/dashboard-server-go/internal/config"
	"github.com/aliaksandrsen/dashboard-server-go/internal/platform/postgres"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := postgres.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "database connection error: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	fmt.Println("Clearing old data and resetting IDs...")
	if _, err := pool.Exec(ctx, `TRUNCATE TABLE "User", "Order", "Product", "_OrderToProduct" RESTART IDENTITY CASCADE;`); err != nil {
		fmt.Fprintf(os.Stderr, "truncate error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Starting seeding (1 user, 5 products, 5 orders)...")

	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "begin tx error: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var userID int
	err = tx.QueryRow(ctx, `
		INSERT INTO "User" ("email", "name", "address")
		VALUES ($1, $2, $3)
		RETURNING "id"
	`, fmt.Sprintf("demo.%d@example.com", time.Now().Unix()), "Alex Rivera", "742 Evergreen Terrace").Scan(&userID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "seed user error: %v\n", err)
		os.Exit(1)
	}

	productNames := []string{
		"Ergonomic Mechanical Keyboard",
		"4K Ultrawide Monitor",
		"Noise Canceling Headphones",
		"Standing Desk Converter",
		"USB-C Thunderbolt Dock",
	}

	productIDs := make([]string, 0, len(productNames))
	now := time.Now().UTC()
	for _, name := range productNames {
		id := uuid.NewString()
		price := roundTwoDecimals(35 + rand.Float64()*(1055-35))
		ts := now.Add(-time.Duration(rand.IntN(30*24)) * time.Hour)

		if _, err := tx.Exec(ctx, `
			INSERT INTO "Product" ("id", "name", "price", "createdAt", "updatedAt")
			VALUES ($1, $2, $3, $4, $5)
		`, id, name, price, ts, ts); err != nil {
			fmt.Fprintf(os.Stderr, "seed product error: %v\n", err)
			os.Exit(1)
		}
		productIDs = append(productIDs, id)
	}

	for i := 0; i < 5; i++ {
		total := roundTwoDecimals(50 + rand.Float64()*(5000-50))
		ts := now.Add(-time.Duration(rand.IntN(14*24)) * time.Hour)

		var orderID int
		if err := tx.QueryRow(ctx, `
			INSERT INTO "Order" ("total", "userId", "createdAt", "updatedAt")
			VALUES ($1, $2, $3, $4)
			RETURNING "id"
		`, total, userID, ts, ts).Scan(&orderID); err != nil {
			fmt.Fprintf(os.Stderr, "seed order error: %v\n", err)
			os.Exit(1)
		}

		perm := rand.Perm(len(productIDs))
		count := 1 + rand.IntN(3)
		for j := 0; j < count; j++ {
			if _, err := tx.Exec(ctx, `
				INSERT INTO "_OrderToProduct" ("A", "B")
				VALUES ($1, $2)
				ON CONFLICT DO NOTHING
			`, orderID, productIDs[perm[j]]); err != nil {
				fmt.Fprintf(os.Stderr, "seed order-product link error: %v\n", err)
				os.Exit(1)
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "commit seed error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Seeding completed! Created 1 user, 5 products, and 5 orders.")
}

func roundTwoDecimals(v float64) float64 {
	return math.Round(v*100) / 100
}
