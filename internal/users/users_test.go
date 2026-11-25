package users

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aliaksandrsen/dashboard-server-go/internal/domain"
	"github.com/aliaksandrsen/dashboard-server-go/internal/repository"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type mockUsersRepo struct {
	createFn  func(ctx context.Context, email, name, address string) (domain.User, error)
	findAllFn func(ctx context.Context, skip, take *int, search *string) ([]domain.UserWithOrders, error)
	findOneFn func(ctx context.Context, id int) (domain.UserWithOrdersAndProducts, error)
	updateFn  func(ctx context.Context, id int, patch repository.UserPatch) (domain.User, error)
	deleteFn  func(ctx context.Context, id int) (domain.User, error)
}

func (m *mockUsersRepo) Create(ctx context.Context, email, name, address string) (domain.User, error) {
	return m.createFn(ctx, email, name, address)
}
func (m *mockUsersRepo) FindAll(ctx context.Context, skip, take *int, search *string) ([]domain.UserWithOrders, error) {
	return m.findAllFn(ctx, skip, take, search)
}
func (m *mockUsersRepo) FindByID(ctx context.Context, id int) (domain.UserWithOrdersAndProducts, error) {
	return m.findOneFn(ctx, id)
}
func (m *mockUsersRepo) Update(ctx context.Context, id int, patch repository.UserPatch) (domain.User, error) {
	return m.updateFn(ctx, id, patch)
}
func (m *mockUsersRepo) Delete(ctx context.Context, id int) (domain.User, error) {
	return m.deleteFn(ctx, id)
}

func TestUsersHandler_ValidationAndCRUD(t *testing.T) {
	repo := &mockUsersRepo{
		createFn: func(_ context.Context, email, name, address string) (domain.User, error) {
			if email == "dup@example.com" {
				return domain.User{}, &pgconn.PgError{Code: "23505"}
			}
			return domain.User{ID: 1, Email: email, Name: name, Address: address}, nil
		},
		findAllFn: func(_ context.Context, skip, take *int, search *string) ([]domain.UserWithOrders, error) {
			return []domain.UserWithOrders{{User: domain.User{ID: 1, Email: "a@b.com", Name: "Alice", Address: "1 St"}, Orders: []domain.Order{}}}, nil
		},
		findOneFn: func(_ context.Context, id int) (domain.UserWithOrdersAndProducts, error) {
			if id == 999 {
				return domain.UserWithOrdersAndProducts{}, pgx.ErrNoRows
			}
			return domain.UserWithOrdersAndProducts{User: domain.User{ID: id, Email: "a@b.com", Name: "Alice", Address: "1 St"}, Orders: []domain.OrderWithProducts{}}, nil
		},
		updateFn: func(_ context.Context, id int, patch repository.UserPatch) (domain.User, error) {
			return domain.User{ID: id, Email: "a@b.com", Name: *patch.Name, Address: "1 St"}, nil
		},
		deleteFn: func(_ context.Context, id int) (domain.User, error) {
			return domain.User{ID: id, Email: "a@b.com", Name: "Alice", Address: "1 St"}, nil
		},
	}

	h := NewHandler(NewService(repo, slog.Default()), slog.Default())
	r := h.Routes()

	// Invalid email -> 400
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"email":"bad","name":"","address":""}`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}

	// Unknown field -> 400
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"email":"a@b.com","name":"A","address":"1","extra":1}`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for extra field, got %d", rec.Code)
	}

	// Valid create -> 201
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"email":"a@b.com","name":"Alice","address":"1 St"}`)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}

	// Duplicate email -> 409
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"email":"dup@example.com","name":"Dup","address":"1 St"}`)))
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", rec.Code)
	}

	// Excessive take=101 -> 400
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/?take=101", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for take=101, got %d", rec.Code)
	}

	// Not found -> 404
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/999", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}

	// Update -> 200
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPatch, "/1", strings.NewReader(`{"name":"Updated"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var updated domain.User
	if err := json.Unmarshal(rec.Body.Bytes(), &updated); err != nil || updated.Name != "Updated" {
		t.Fatalf("unexpected updated response: %s, err=%v", rec.Body.String(), err)
	}
}
