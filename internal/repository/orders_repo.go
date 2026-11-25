package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aliaksandrsen/dashboard-server-go/internal/domain"
	"github.com/aliaksandrsen/dashboard-server-go/internal/httputil"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type OrderPatch struct {
	Total           *float64
	UserID          httputil.Optional[int]
	StripeInvoiceID httputil.Optional[string]
	ProductIDs      *[]string
}

type OrdersRepository struct {
	pool *pgxpool.Pool
}

func NewOrdersRepository(pool *pgxpool.Pool) *OrdersRepository {
	return &OrdersRepository{pool: pool}
}

func (r *OrdersRepository) Create(
	ctx context.Context,
	total float64,
	userID *int,
	stripeInvoiceID *string,
	productIDs []string,
) (domain.OrderWithRelations, error) {
	var res domain.OrderWithRelations
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return res, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	now := time.Now().UTC()
	var o domain.Order
	err = tx.QueryRow(ctx, `
		INSERT INTO "Order" ("total", "userId", "stripeInvoiceId", "createdAt", "updatedAt")
		VALUES ($1, $2, $3, $4, $5)
		RETURNING "id", "total", "stripeInvoiceId", "createdAt", "updatedAt", "userId"
	`, total, userID, stripeInvoiceID, now, now).Scan(
		&o.ID, &o.Total, &o.StripeInvoiceID, &o.CreatedAt, &o.UpdatedAt, &o.UserID,
	)
	if err != nil {
		return res, err
	}

	for _, productID := range productIDs {
		if _, err := tx.Exec(ctx, `
			INSERT INTO "_OrderToProduct" ("A", "B")
			VALUES ($1, $2)
			ON CONFLICT ("A", "B") DO NOTHING
		`, o.ID, productID); err != nil {
			return res, err
		}
	}

	res, err = r.loadRelationsTx(ctx, tx, o)
	if err != nil {
		return res, err
	}

	if err := tx.Commit(ctx); err != nil {
		return res, err
	}
	return res, nil
}

func (r *OrdersRepository) FindAll(ctx context.Context, skip, take, userID *int) ([]domain.OrderWithRelations, error) {
	var qb strings.Builder
	args := make([]any, 0, 3)
	argIdx := 1

	qb.WriteString(`SELECT "id", "total", "stripeInvoiceId", "createdAt", "updatedAt", "userId" FROM "Order"`)

	if userID != nil {
		qb.WriteString(fmt.Sprintf(` WHERE "userId" = $%d`, argIdx))
		args = append(args, *userID)
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

	orders := make([]domain.OrderWithRelations, 0)
	orderIDs := make([]int, 0)
	orderIndexByID := make(map[int]int)
	userIDSet := make(map[int]struct{})

	for rows.Next() {
		var o domain.Order
		if err := rows.Scan(&o.ID, &o.Total, &o.StripeInvoiceID, &o.CreatedAt, &o.UpdatedAt, &o.UserID); err != nil {
			return nil, err
		}
		orderIndexByID[o.ID] = len(orders)
		orderIDs = append(orderIDs, o.ID)
		if o.UserID != nil {
			userIDSet[*o.UserID] = struct{}{}
		}
		orders = append(orders, domain.OrderWithRelations{
			Order:    o,
			Products: make([]domain.Product, 0),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(orders) == 0 {
		return orders, nil
	}

	if len(userIDSet) > 0 {
		userIDs := make([]int, 0, len(userIDSet))
		for uid := range userIDSet {
			userIDs = append(userIDs, uid)
		}

		userRows, err := r.pool.Query(ctx, `
			SELECT "id", "email", "name", "address"
			FROM "User"
			WHERE "id" = ANY($1)
		`, userIDs)
		if err != nil {
			return nil, err
		}
		defer userRows.Close()

		usersByID := make(map[int]domain.User, len(userIDs))
		for userRows.Next() {
			var u domain.User
			if err := userRows.Scan(&u.ID, &u.Email, &u.Name, &u.Address); err != nil {
				return nil, err
			}
			usersByID[u.ID] = u
		}
		if err := userRows.Err(); err != nil {
			return nil, err
		}

		for i := range orders {
			if orders[i].UserID != nil {
				if u, ok := usersByID[*orders[i].UserID]; ok {
					userCopy := u
					orders[i].User = &userCopy
				}
			}
		}
	}

	prodRows, err := r.pool.Query(ctx, `
		SELECT otp."A", p."id", p."name", p."price", p."stripeProductId", p."stripePriceId", p."createdAt", p."updatedAt"
		FROM "_OrderToProduct" otp
		INNER JOIN "Product" p ON p."id" = otp."B"
		WHERE otp."A" = ANY($1)
		ORDER BY p."createdAt" DESC
	`, orderIDs)
	if err != nil {
		return nil, err
	}
	defer prodRows.Close()

	for prodRows.Next() {
		var orderID int
		var p domain.Product
		if err := prodRows.Scan(
			&orderID, &p.ID, &p.Name, &p.Price, &p.StripeProductID, &p.StripePriceID, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if idx, ok := orderIndexByID[orderID]; ok {
			orders[idx].Products = append(orders[idx].Products, p)
		}
	}
	if err := prodRows.Err(); err != nil {
		return nil, err
	}

	return orders, nil
}

func (r *OrdersRepository) FindByID(ctx context.Context, id int) (domain.OrderWithRelations, error) {
	var res domain.OrderWithRelations
	var o domain.Order
	err := r.pool.QueryRow(ctx, `
		SELECT "id", "total", "stripeInvoiceId", "createdAt", "updatedAt", "userId"
		FROM "Order"
		WHERE "id" = $1
	`, id).Scan(&o.ID, &o.Total, &o.StripeInvoiceID, &o.CreatedAt, &o.UpdatedAt, &o.UserID)
	if err != nil {
		return res, err
	}

	return r.loadRelationsQueryer(ctx, r.pool, o)
}

func (r *OrdersRepository) FindByStripeInvoiceID(ctx context.Context, stripeInvoiceID string) (*domain.Order, error) {
	var o domain.Order
	err := r.pool.QueryRow(ctx, `
		SELECT "id", "total", "stripeInvoiceId", "createdAt", "updatedAt", "userId"
		FROM "Order"
		WHERE "stripeInvoiceId" = $1
	`, stripeInvoiceID).Scan(&o.ID, &o.Total, &o.StripeInvoiceID, &o.CreatedAt, &o.UpdatedAt, &o.UserID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

func (r *OrdersRepository) Exists(ctx context.Context, id int) (bool, error) {
	var found int
	err := r.pool.QueryRow(ctx, `SELECT "id" FROM "Order" WHERE "id" = $1`, id).Scan(&found)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (r *OrdersRepository) Update(ctx context.Context, id int, patch OrderPatch) (domain.OrderWithRelations, error) {
	var res domain.OrderWithRelations
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return res, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	setClauses := make([]string, 0, 4)
	args := make([]any, 0, 5)
	argIdx := 1

	if patch.Total != nil {
		setClauses = append(setClauses, fmt.Sprintf(`"total" = $%d`, argIdx))
		args = append(args, *patch.Total)
		argIdx++
	}
	if patch.UserID.Set {
		setClauses = append(setClauses, fmt.Sprintf(`"userId" = $%d`, argIdx))
		if patch.UserID.Null {
			args = append(args, nil)
		} else {
			args = append(args, patch.UserID.Value)
		}
		argIdx++
	}
	if patch.StripeInvoiceID.Set {
		setClauses = append(setClauses, fmt.Sprintf(`"stripeInvoiceId" = $%d`, argIdx))
		if patch.StripeInvoiceID.Null {
			args = append(args, nil)
		} else {
			args = append(args, patch.StripeInvoiceID.Value)
		}
		argIdx++
	}

	setClauses = append(setClauses, fmt.Sprintf(`"updatedAt" = $%d`, argIdx))
	args = append(args, time.Now().UTC())
	argIdx++

	args = append(args, id)
	query := fmt.Sprintf(`
		UPDATE "Order"
		SET %s
		WHERE "id" = $%d
		RETURNING "id", "total", "stripeInvoiceId", "createdAt", "updatedAt", "userId"
	`, strings.Join(setClauses, ", "), argIdx)

	var o domain.Order
	err = tx.QueryRow(ctx, query, args...).Scan(
		&o.ID, &o.Total, &o.StripeInvoiceID, &o.CreatedAt, &o.UpdatedAt, &o.UserID,
	)
	if err != nil {
		return res, err
	}

	if patch.ProductIDs != nil {
		if _, err := tx.Exec(ctx, `DELETE FROM "_OrderToProduct" WHERE "A" = $1`, id); err != nil {
			return res, err
		}
		for _, productID := range *patch.ProductIDs {
			if _, err := tx.Exec(ctx, `
				INSERT INTO "_OrderToProduct" ("A", "B")
				VALUES ($1, $2)
				ON CONFLICT ("A", "B") DO NOTHING
			`, id, productID); err != nil {
				return res, err
			}
		}
	}

	res, err = r.loadRelationsTx(ctx, tx, o)
	if err != nil {
		return res, err
	}

	if err := tx.Commit(ctx); err != nil {
		return res, err
	}
	return res, nil
}

func (r *OrdersRepository) Delete(ctx context.Context, id int) (domain.Order, error) {
	var o domain.Order
	err := r.pool.QueryRow(ctx, `
		DELETE FROM "Order"
		WHERE "id" = $1
		RETURNING "id", "total", "stripeInvoiceId", "createdAt", "updatedAt", "userId"
	`, id).Scan(&o.ID, &o.Total, &o.StripeInvoiceID, &o.CreatedAt, &o.UpdatedAt, &o.UserID)
	return o, err
}

type rowQueryer interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func (r *OrdersRepository) loadRelationsTx(ctx context.Context, tx pgx.Tx, o domain.Order) (domain.OrderWithRelations, error) {
	return r.loadRelationsQueryer(ctx, tx, o)
}

func (r *OrdersRepository) loadRelationsQueryer(ctx context.Context, q rowQueryer, o domain.Order) (domain.OrderWithRelations, error) {
	res := domain.OrderWithRelations{
		Order:    o,
		Products: make([]domain.Product, 0),
	}

	if o.UserID != nil {
		var u domain.User
		err := q.QueryRow(ctx, `
			SELECT "id", "email", "name", "address"
			FROM "User"
			WHERE "id" = $1
		`, *o.UserID).Scan(&u.ID, &u.Email, &u.Name, &u.Address)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return res, err
		}
		if err == nil {
			res.User = &u
		}
	}

	rows, err := q.Query(ctx, `
		SELECT p."id", p."name", p."price", p."stripeProductId", p."stripePriceId", p."createdAt", p."updatedAt"
		FROM "_OrderToProduct" otp
		INNER JOIN "Product" p ON p."id" = otp."B"
		WHERE otp."A" = $1
		ORDER BY p."createdAt" DESC
	`, o.ID)
	if err != nil {
		return res, err
	}
	defer rows.Close()

	for rows.Next() {
		var p domain.Product
		if err := rows.Scan(
			&p.ID, &p.Name, &p.Price, &p.StripeProductID, &p.StripePriceID, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return res, err
		}
		res.Products = append(res.Products, p)
	}
	if err := rows.Err(); err != nil {
		return res, err
	}

	return res, nil
}
