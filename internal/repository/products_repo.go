package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aliaksandrsen/dashboard-server-go/internal/domain"
	"github.com/aliaksandrsen/dashboard-server-go/internal/httputil"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ProductPatch struct {
	Name            *string
	Price           *float64
	StripeProductID httputil.Optional[string]
	StripePriceID   httputil.Optional[string]
}

type ProductsRepository struct {
	pool *pgxpool.Pool
}

func NewProductsRepository(pool *pgxpool.Pool) *ProductsRepository {
	return &ProductsRepository{pool: pool}
}

func (r *ProductsRepository) Create(
	ctx context.Context,
	name string,
	price float64,
	stripeProductID *string,
	stripePriceID *string,
) (domain.Product, error) {
	now := time.Now().UTC()
	id := uuid.NewString()

	var p domain.Product
	err := r.pool.QueryRow(ctx, `
		INSERT INTO "Product" ("id", "name", "price", "stripeProductId", "stripePriceId", "createdAt", "updatedAt")
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING "id", "name", "price", "stripeProductId", "stripePriceId", "createdAt", "updatedAt"
	`, id, name, price, stripeProductID, stripePriceID, now, now).Scan(
		&p.ID, &p.Name, &p.Price, &p.StripeProductID, &p.StripePriceID, &p.CreatedAt, &p.UpdatedAt,
	)
	return p, err
}

func (r *ProductsRepository) FindAll(ctx context.Context, skip, take *int, search *string) ([]domain.Product, error) {
	var qb strings.Builder
	args := make([]any, 0, 3)
	argIdx := 1

	qb.WriteString(`SELECT "id", "name", "price", "stripeProductId", "stripePriceId", "createdAt", "updatedAt" FROM "Product"`)

	if search != nil && *search != "" {
		qb.WriteString(fmt.Sprintf(` WHERE "name" ILIKE $%d`, argIdx))
		args = append(args, "%"+*search+"%")
		argIdx++
	}

	qb.WriteString(` ORDER BY "createdAt" DESC`)

	if take != nil {
		qb.WriteString(fmt.Sprintf(` LIMIT $%d`, argIdx))
		args = append(args, *take)
		argIdx++
	}
	if skip != nil {
		qb.WriteString(fmt.Sprintf(` OFFSET $%d`, argIdx))
		args = append(args, *skip)
		argIdx++
	}

	rows, err := r.pool.Query(ctx, qb.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	products := make([]domain.Product, 0)
	for rows.Next() {
		var p domain.Product
		if err := rows.Scan(
			&p.ID, &p.Name, &p.Price, &p.StripeProductID, &p.StripePriceID, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, err
		}
		products = append(products, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return products, nil
}

func (r *ProductsRepository) FindByID(ctx context.Context, id string) (domain.Product, error) {
	var p domain.Product
	err := r.pool.QueryRow(ctx, `
		SELECT "id", "name", "price", "stripeProductId", "stripePriceId", "createdAt", "updatedAt"
		FROM "Product"
		WHERE "id" = $1
	`, id).Scan(
		&p.ID, &p.Name, &p.Price, &p.StripeProductID, &p.StripePriceID, &p.CreatedAt, &p.UpdatedAt,
	)
	return p, err
}

func (r *ProductsRepository) Update(ctx context.Context, id string, patch ProductPatch) (domain.Product, error) {
	setClauses := make([]string, 0, 5)
	args := make([]any, 0, 6)
	argIdx := 1

	if patch.Name != nil {
		setClauses = append(setClauses, fmt.Sprintf(`"name" = $%d`, argIdx))
		args = append(args, *patch.Name)
		argIdx++
	}
	if patch.Price != nil {
		setClauses = append(setClauses, fmt.Sprintf(`"price" = $%d`, argIdx))
		args = append(args, *patch.Price)
		argIdx++
	}
	if patch.StripeProductID.Set {
		setClauses = append(setClauses, fmt.Sprintf(`"stripeProductId" = $%d`, argIdx))
		if patch.StripeProductID.Null {
			args = append(args, nil)
		} else {
			args = append(args, patch.StripeProductID.Value)
		}
		argIdx++
	}
	if patch.StripePriceID.Set {
		setClauses = append(setClauses, fmt.Sprintf(`"stripePriceId" = $%d`, argIdx))
		if patch.StripePriceID.Null {
			args = append(args, nil)
		} else {
			args = append(args, patch.StripePriceID.Value)
		}
		argIdx++
	}

	setClauses = append(setClauses, fmt.Sprintf(`"updatedAt" = $%d`, argIdx))
	args = append(args, time.Now().UTC())
	argIdx++

	args = append(args, id)
	query := fmt.Sprintf(`
		UPDATE "Product"
		SET %s
		WHERE "id" = $%d
		RETURNING "id", "name", "price", "stripeProductId", "stripePriceId", "createdAt", "updatedAt"
	`, strings.Join(setClauses, ", "), argIdx)

	var p domain.Product
	err := r.pool.QueryRow(ctx, query, args...).Scan(
		&p.ID, &p.Name, &p.Price, &p.StripeProductID, &p.StripePriceID, &p.CreatedAt, &p.UpdatedAt,
	)
	return p, err
}

func (r *ProductsRepository) Delete(ctx context.Context, id string) (domain.Product, error) {
	var p domain.Product
	err := r.pool.QueryRow(ctx, `
		DELETE FROM "Product"
		WHERE "id" = $1
		RETURNING "id", "name", "price", "stripeProductId", "stripePriceId", "createdAt", "updatedAt"
	`, id).Scan(
		&p.ID, &p.Name, &p.Price, &p.StripeProductID, &p.StripePriceID, &p.CreatedAt, &p.UpdatedAt,
	)
	return p, err
}
