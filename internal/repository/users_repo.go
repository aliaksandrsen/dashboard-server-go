package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/aliaksandrsen/dashboard-server-go/internal/domain"
	"github.com/aliaksandrsen/dashboard-server-go/internal/httputil"
	"github.com/jackc/pgx/v5/pgxpool"
)

type UserPatch struct {
	Email   *string
	Name    *string
	Address *string
}

type UsersRepository struct {
	pool *pgxpool.Pool
}

func NewUsersRepository(pool *pgxpool.Pool) *UsersRepository {
	return &UsersRepository{pool: pool}
}

func (r *UsersRepository) Create(ctx context.Context, email, name, address string) (domain.User, error) {
	var user domain.User
	err := r.pool.QueryRow(ctx, `
		INSERT INTO "User" ("email", "name", "address")
		VALUES ($1, $2, $3)
		RETURNING "id", "email", "name", "address"
	`, email, name, address).Scan(&user.ID, &user.Email, &user.Name, &user.Address)
	return user, err
}

func (r *UsersRepository) FindAll(ctx context.Context, skip, take *int, search *string) ([]domain.UserWithOrders, error) {
	var qb strings.Builder
	args := make([]any, 0, 3)
	argIdx := 1

	qb.WriteString(`SELECT "id", "email", "name", "address" FROM "User"`)

	if search != nil && *search != "" {
		qb.WriteString(fmt.Sprintf(` WHERE ("name" ILIKE $%d OR "email" ILIKE $%d)`, argIdx, argIdx))
		args = append(args, "%"+*search+"%")
		argIdx++
	}

	qb.WriteString(` ORDER BY "id" ASC`)

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

	users := make([]domain.UserWithOrders, 0)
	userIDs := make([]int, 0)
	indexByID := make(map[int]int)

	for rows.Next() {
		var u domain.User
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.Address); err != nil {
			return nil, err
		}
		indexByID[u.ID] = len(users)
		userIDs = append(userIDs, u.ID)
		users = append(users, domain.UserWithOrders{
			User:   u,
			Orders: make([]domain.Order, 0),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(userIDs) == 0 {
		return users, nil
	}

	orderRows, err := r.pool.Query(ctx, `
		SELECT "id", "total", "stripeInvoiceId", "createdAt", "updatedAt", "userId"
		FROM "Order"
		WHERE "userId" = ANY($1)
		ORDER BY "id" ASC
	`, userIDs)
	if err != nil {
		return nil, err
	}
	defer orderRows.Close()

	for orderRows.Next() {
		var o domain.Order
		if err := orderRows.Scan(&o.ID, &o.Total, &o.StripeInvoiceID, &o.CreatedAt, &o.UpdatedAt, &o.UserID); err != nil {
			return nil, err
		}
		if o.UserID != nil {
			if idx, ok := indexByID[*o.UserID]; ok {
				users[idx].Orders = append(users[idx].Orders, o)
			}
		}
	}
	if err := orderRows.Err(); err != nil {
		return nil, err
	}

	return users, nil
}

func (r *UsersRepository) FindByID(ctx context.Context, id int) (domain.UserWithOrdersAndProducts, error) {
	var res domain.UserWithOrdersAndProducts
	res.Orders = make([]domain.OrderWithProducts, 0)

	err := r.pool.QueryRow(ctx, `
		SELECT "id", "email", "name", "address"
		FROM "User"
		WHERE "id" = $1
	`, id).Scan(&res.ID, &res.Email, &res.Name, &res.Address)
	if err != nil {
		return res, err
	}

	orderRows, err := r.pool.Query(ctx, `
		SELECT "id", "total", "stripeInvoiceId", "createdAt", "updatedAt", "userId"
		FROM "Order"
		WHERE "userId" = $1
		ORDER BY "id" ASC
	`, id)
	if err != nil {
		return res, err
	}
	defer orderRows.Close()

	orderIDs := make([]int, 0)
	orderIndexByID := make(map[int]int)
	for orderRows.Next() {
		var o domain.Order
		if err := orderRows.Scan(&o.ID, &o.Total, &o.StripeInvoiceID, &o.CreatedAt, &o.UpdatedAt, &o.UserID); err != nil {
			return res, err
		}
		orderIndexByID[o.ID] = len(res.Orders)
		orderIDs = append(orderIDs, o.ID)
		res.Orders = append(res.Orders, domain.OrderWithProducts{
			Order:    o,
			Products: make([]domain.Product, 0),
		})
	}
	if err := orderRows.Err(); err != nil {
		return res, err
	}

	if len(orderIDs) == 0 {
		return res, nil
	}

	prodRows, err := r.pool.Query(ctx, `
		SELECT otp."A", p."id", p."name", p."price", p."stripeProductId", p."stripePriceId", p."createdAt", p."updatedAt"
		FROM "_OrderToProduct" otp
		INNER JOIN "Product" p ON p."id" = otp."B"
		WHERE otp."A" = ANY($1)
		ORDER BY p."createdAt" DESC
	`, orderIDs)
	if err != nil {
		return res, err
	}
	defer prodRows.Close()

	for prodRows.Next() {
		var orderID int
		var p domain.Product
		if err := prodRows.Scan(
			&orderID,
			&p.ID,
			&p.Name,
			&p.Price,
			&p.StripeProductID,
			&p.StripePriceID,
			&p.CreatedAt,
			&p.UpdatedAt,
		); err != nil {
			return res, err
		}
		if idx, ok := orderIndexByID[orderID]; ok {
			res.Orders[idx].Products = append(res.Orders[idx].Products, p)
		}
	}
	if err := prodRows.Err(); err != nil {
		return res, err
	}

	return res, nil
}

func (r *UsersRepository) FindByEmail(ctx context.Context, email string) (domain.User, error) {
	var user domain.User
	err := r.pool.QueryRow(ctx, `
		SELECT "id", "email", "name", "address"
		FROM "User"
		WHERE "email" = $1
	`, email).Scan(&user.ID, &user.Email, &user.Name, &user.Address)
	return user, err
}

func (r *UsersRepository) Update(ctx context.Context, id int, patch UserPatch) (domain.User, error) {
	setClauses := make([]string, 0, 3)
	args := make([]any, 0, 4)
	argIdx := 1

	if patch.Email != nil {
		setClauses = append(setClauses, fmt.Sprintf(`"email" = $%d`, argIdx))
		args = append(args, *patch.Email)
		argIdx++
	}
	if patch.Name != nil {
		setClauses = append(setClauses, fmt.Sprintf(`"name" = $%d`, argIdx))
		args = append(args, *patch.Name)
		argIdx++
	}
	if patch.Address != nil {
		setClauses = append(setClauses, fmt.Sprintf(`"address" = $%d`, argIdx))
		args = append(args, *patch.Address)
		argIdx++
	}

	if len(setClauses) == 0 {
		var user domain.User
		err := r.pool.QueryRow(ctx, `
			SELECT "id", "email", "name", "address"
			FROM "User"
			WHERE "id" = $1
		`, id).Scan(&user.ID, &user.Email, &user.Name, &user.Address)
		return user, err
	}

	args = append(args, id)
	query := fmt.Sprintf(`
		UPDATE "User"
		SET %s
		WHERE "id" = $%d
		RETURNING "id", "email", "name", "address"
	`, strings.Join(setClauses, ", "), argIdx)

	var user domain.User
	err := r.pool.QueryRow(ctx, query, args...).Scan(&user.ID, &user.Email, &user.Name, &user.Address)
	return user, err
}

func (r *UsersRepository) Delete(ctx context.Context, id int) (domain.User, error) {
	var user domain.User
	err := r.pool.QueryRow(ctx, `
		DELETE FROM "User"
		WHERE "id" = $1
		RETURNING "id", "email", "name", "address"
	`, id).Scan(&user.ID, &user.Email, &user.Name, &user.Address)
	return user, err
}

// Keep httputil import referenced when Optional is used in sibling repositories.
var _ = httputil.RequestIDHeader
