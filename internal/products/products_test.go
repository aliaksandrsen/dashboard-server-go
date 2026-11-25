package products

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aliaksandrsen/dashboard-server-go/internal/domain"
	"github.com/aliaksandrsen/dashboard-server-go/internal/repository"
	"github.com/aliaksandrsen/dashboard-server-go/internal/stripe"
)

type mockProductsRepo struct {
	createFn  func(ctx context.Context, name string, price float64, spID, sPriceID *string) (domain.Product, error)
	findAllFn func(ctx context.Context, skip, take *int, search *string) ([]domain.Product, error)
	findOneFn func(ctx context.Context, id string) (domain.Product, error)
	updateFn  func(ctx context.Context, id string, patch repository.ProductPatch) (domain.Product, error)
	deleteFn  func(ctx context.Context, id string) (domain.Product, error)
}

func (m *mockProductsRepo) Create(ctx context.Context, n string, p float64, sp, spr *string) (domain.Product, error) {
	return m.createFn(ctx, n, p, sp, spr)
}
func (m *mockProductsRepo) FindAll(ctx context.Context, skip, take *int, search *string) ([]domain.Product, error) {
	return m.findAllFn(ctx, skip, take, search)
}
func (m *mockProductsRepo) FindByID(ctx context.Context, id string) (domain.Product, error) {
	return m.findOneFn(ctx, id)
}
func (m *mockProductsRepo) Update(ctx context.Context, id string, patch repository.ProductPatch) (domain.Product, error) {
	return m.updateFn(ctx, id, patch)
}
func (m *mockProductsRepo) Delete(ctx context.Context, id string) (domain.Product, error) {
	return m.deleteFn(ctx, id)
}

type mockStripeProducts struct {
	deactivatedID string
}

func (m *mockStripeProducts) CreateProduct(_ context.Context, _ stripe.CreateProductParams) (stripe.ProductResult, error) {
	return stripe.ProductResult{StripeProductID: "prod_1", StripePriceID: "price_1"}, nil
}
func (m *mockStripeProducts) DeactivateProduct(_ context.Context, id string) {
	m.deactivatedID = id
}

type mockCacheStore struct {
	data map[string][]byte
	gets int
	sets int
	dels int
}

func (m *mockCacheStore) Get(_ context.Context, key string) ([]byte, bool, error) {
	m.gets++
	v, ok := m.data[key]
	return v, ok, nil
}
func (m *mockCacheStore) Set(_ context.Context, key string, val []byte, _ time.Duration) error {
	m.sets++
	if m.data == nil {
		m.data = make(map[string][]byte)
	}
	m.data[key] = val
	return nil
}
func (m *mockCacheStore) Del(_ context.Context, key string) error {
	m.dels++
	delete(m.data, key)
	return nil
}
func (m *mockCacheStore) Close() error { return nil }

func TestProductsService_CreateRollbackOnDBFailure(t *testing.T) {
	repo := &mockProductsRepo{
		createFn: func(context.Context, string, float64, *string, *string) (domain.Product, error) {
			return domain.Product{}, errors.New("db failure")
		},
	}
	sc := &mockStripeProducts{}
	svc := NewService(repo, sc, slog.Default())

	_, err := svc.Create(context.Background(), CreateProductRequest{Name: "Mouse", Price: 25})
	if err == nil {
		t.Fatal("expected error")
	}
	if sc.deactivatedID != "prod_1" {
		t.Fatalf("expected rollback deactivation for prod_1, got %q", sc.deactivatedID)
	}
}

func TestProductsHandler_CachingAndInvalidation(t *testing.T) {
	sample := domain.Product{
		ID:        "b6f69931-eeb7-4e02-b2f7-e6fbe5aa32db",
		Name:      "Mouse",
		Price:     49.99,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	dbCalls := 0
	repo := &mockProductsRepo{
		createFn: func(_ context.Context, name string, price float64, sp, spr *string) (domain.Product, error) {
			return sample, nil
		},
		findAllFn: func(_ context.Context, skip, take *int, search *string) ([]domain.Product, error) {
			dbCalls++
			return []domain.Product{sample}, nil
		},
		updateFn: func(_ context.Context, id string, patch repository.ProductPatch) (domain.Product, error) {
			return sample, nil
		},
		deleteFn: func(_ context.Context, id string) (domain.Product, error) {
			return sample, nil
		},
	}

	c := &mockCacheStore{}
	h := NewHandler(NewService(repo, &mockStripeProducts{}, slog.Default()), c, slog.Default())
	r := h.Routes()

	// Filtered search must bypass cache completely
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/?search=Mouse", nil))
	if rec.Code != http.StatusOK || c.gets != 0 || c.sets != 0 {
		t.Fatalf("expected cache bypass on filtered search, gets=%d sets=%d", c.gets, c.sets)
	}

	// Unfiltered request populates cache
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK || c.sets != 1 || dbCalls != 2 {
		t.Fatalf("expected cache populate, sets=%d dbCalls=%d", c.sets, dbCalls)
	}

	// Second unfiltered request hits cache (dbCalls stays 2)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK || dbCalls != 2 {
		t.Fatalf("expected cache hit, dbCalls=%d", dbCalls)
	}

	// Invalid UUID -> 400
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/not-a-uuid", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid uuid, got %d", rec.Code)
	}

	// Create invalidates cache
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"Mouse","price":49.99}`)))
	if rec.Code != http.StatusCreated || c.dels != 1 {
		t.Fatalf("expected 201 and cache invalidation, code=%d dels=%d", rec.Code, c.dels)
	}
	var created domain.Product
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil || created.ID != sample.ID {
		t.Fatalf("unexpected response: %s", rec.Body.String())
	}
}
