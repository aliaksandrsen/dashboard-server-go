package orders

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aliaksandrsen/dashboard-server-go/internal/domain"
	"github.com/aliaksandrsen/dashboard-server-go/internal/repository"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type mockOrdersRepo struct {
	createFn  func(ctx context.Context, total float64, userID *int, stripeInvoiceID *string, productIDs []string) (domain.OrderWithRelations, error)
	findAllFn func(ctx context.Context, skip, take, userID *int) ([]domain.OrderWithRelations, error)
	findOneFn func(ctx context.Context, id int) (domain.OrderWithRelations, error)
	existsFn  func(ctx context.Context, id int) (bool, error)
	updateFn  func(ctx context.Context, id int, patch repository.OrderPatch) (domain.OrderWithRelations, error)
	deleteFn  func(ctx context.Context, id int) (domain.Order, error)
}

func (m *mockOrdersRepo) Create(ctx context.Context, t float64, u *int, s *string, p []string) (domain.OrderWithRelations, error) {
	return m.createFn(ctx, t, u, s, p)
}
func (m *mockOrdersRepo) FindAll(ctx context.Context, skip, take, userID *int) ([]domain.OrderWithRelations, error) {
	return m.findAllFn(ctx, skip, take, userID)
}
func (m *mockOrdersRepo) FindByID(ctx context.Context, id int) (domain.OrderWithRelations, error) {
	return m.findOneFn(ctx, id)
}
func (m *mockOrdersRepo) Exists(ctx context.Context, id int) (bool, error) {
	return m.existsFn(ctx, id)
}
func (m *mockOrdersRepo) Update(ctx context.Context, id int, patch repository.OrderPatch) (domain.OrderWithRelations, error) {
	return m.updateFn(ctx, id, patch)
}
func (m *mockOrdersRepo) Delete(ctx context.Context, id int) (domain.Order, error) {
	return m.deleteFn(ctx, id)
}

func TestOrdersHandler_UpdateNotFoundVsInvalidProductID(t *testing.T) {
	now := time.Now().UTC()
	repo := &mockOrdersRepo{
		existsFn: func(_ context.Context, id int) (bool, error) {
			return id != 999, nil
		},
		updateFn: func(_ context.Context, id int, patch repository.OrderPatch) (domain.OrderWithRelations, error) {
			if patch.ProductIDs != nil && len(*patch.ProductIDs) > 0 && (*patch.ProductIDs)[0] == "bad-prod" {
				return domain.OrderWithRelations{}, &pgconn.PgError{
					Code:           "23503",
					ConstraintName: "_OrderToProduct_B_fkey",
				}
			}
			return domain.OrderWithRelations{
				Order:    domain.Order{ID: id, Total: 150, CreatedAt: now, UpdatedAt: now},
				Products: []domain.Product{},
			}, nil
		},
		findOneFn: func(_ context.Context, id int) (domain.OrderWithRelations, error) {
			return domain.OrderWithRelations{}, pgx.ErrNoRows
		},
	}

	h := NewHandler(NewService(repo, slog.Default()), slog.Default())
	r := h.Routes()

	// Missing order -> 404
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPatch, "/999", strings.NewReader(`{"total":150}`)))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}

	// Invalid productId -> 400
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPatch, "/1", strings.NewReader(`{"productIds":["bad-prod"]}`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid productId, got %d", rec.Code)
	}

	// Nullify userId and stripeInvoiceId -> 200 with explicit null fields in JSON
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPatch, "/1", strings.NewReader(`{"userId":null,"stripeInvoiceId":null,"total":150}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}

	var rawMap map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &rawMap); err != nil {
		t.Fatal(err)
	}
	if _, hasUser := rawMap["user"]; !hasUser {
		t.Fatalf("expected explicit 'user': null key in response: %s", rec.Body.String())
	}
	if _, hasUserID := rawMap["userId"]; !hasUserID {
		t.Fatalf("expected explicit 'userId': null key in response: %s", rec.Body.String())
	}
}
