//go:build e2e

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aliaksandrsen/dashboard-server-go/internal/config"
	"github.com/aliaksandrsen/dashboard-server-go/internal/domain"
	"github.com/aliaksandrsen/dashboard-server-go/internal/health"
	"github.com/aliaksandrsen/dashboard-server-go/internal/httputil"
	"github.com/aliaksandrsen/dashboard-server-go/internal/httputil/middleware"
	"github.com/aliaksandrsen/dashboard-server-go/internal/jobs"
	"github.com/aliaksandrsen/dashboard-server-go/internal/logger"
	"github.com/aliaksandrsen/dashboard-server-go/internal/orders"
	"github.com/aliaksandrsen/dashboard-server-go/internal/platform/cache"
	"github.com/aliaksandrsen/dashboard-server-go/internal/platform/origin"
	"github.com/aliaksandrsen/dashboard-server-go/internal/platform/postgres"
	"github.com/aliaksandrsen/dashboard-server-go/internal/products"
	"github.com/aliaksandrsen/dashboard-server-go/internal/repository"
	"github.com/aliaksandrsen/dashboard-server-go/internal/stripe"
	"github.com/aliaksandrsen/dashboard-server-go/internal/users"
	"github.com/joho/godotenv"
)

type e2eStripeMock struct{}

func (m *e2eStripeMock) CreateProduct(_ context.Context, params stripe.CreateProductParams) (stripe.ProductResult, error) {
	suffix := fmt.Sprintf("%d_%d", len(params.Name), time.Now().UnixNano())
	return stripe.ProductResult{
		StripeProductID: "prod_e2e_" + suffix,
		StripePriceID:   "price_e2e_" + suffix,
	}, nil
}

func (m *e2eStripeMock) DeactivateProduct(context.Context, string) {}

func (m *e2eStripeMock) CreatePayment(_ context.Context, req stripe.CreatePaymentRequest, originHeader string) (stripe.CheckoutSessionResult, error) {
	allowed := origin.GetAllowedOrigins("")
	if _, err := origin.ResolveAllowedOrigin(originHeader, allowed); err != nil {
		return stripe.CheckoutSessionResult{}, httputil.BadRequest(err.Error())
	}
	return stripe.CheckoutSessionResult{
		SessionID: "cs_e2e_123",
		URL:       "https://checkout.stripe.com/pay/cs_e2e_123",
	}, nil
}

func (m *e2eStripeMock) ListPaidInvoicesSince(context.Context, int64) ([]stripe.Invoice, error) {
	return []stripe.Invoice{}, nil
}

type e2eSyncMock struct{}

func (m *e2eSyncMock) CronSyncStripeOrders(context.Context, time.Time) (domain.SyncResult, error) {
	return domain.SyncResult{
		Processed: 2,
		Created:   1,
		Skipped:   1,
		Failed:    0,
	}, nil
}

func setupE2EServer(t *testing.T) (*httptest.Server, func()) {
	t.Helper()

	_ = godotenv.Load("../../.env")
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgresql://postgres:postgres@localhost:5432/dashboard?schema=public"
	}
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://localhost:6379"
	}

	cfg, err := config.LoadFromMap(map[string]string{
		"NODE_ENV":     "test",
		"DATABASE_URL": dbURL,
		"REDIS_URL":    redisURL,
	})
	if err != nil {
		t.Fatalf("config error: %v", err)
	}

	log, closeLog, err := logger.New("test", t.TempDir()+"/server.log", io.Discard)
	if err != nil {
		t.Fatalf("logger error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := postgres.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		_ = closeLog()
		t.Fatalf("postgres connection failed (ensure docker compose up -d & migrate): %v", err)
	}

	cacheStore, err := cache.NewRedisStore(cfg.EffectiveRedisURL())
	if err != nil {
		pool.Close()
		_ = closeLog()
		t.Fatalf("redis connection failed: %v", err)
	}

	// Ensure cache starts clean for products tests
	_ = cacheStore.Del(ctx, "products")

	usersRepo := repository.NewUsersRepository(pool)
	productsRepo := repository.NewProductsRepository(pool)
	ordersRepo := repository.NewOrdersRepository(pool)

	stripeMock := &e2eStripeMock{}
	rl := middleware.NewRateLimiter(time.Minute, 1000, log)

	router := NewRouter(Dependencies{
		AllowedOrigins: origin.GetAllowedOrigins(cfg.FrontendURL),
		RateLimiter:    rl,
		Logger:         log,
		Health:         health.NewHandler(pool, cacheStore, log),
		Users:          users.NewHandler(users.NewService(usersRepo, log), log),
		Products:       products.NewHandler(products.NewService(productsRepo, stripeMock, log), cacheStore, log),
		Orders:         orders.NewHandler(orders.NewService(ordersRepo, log), log),
		Stripe:         stripe.NewHandler(stripeMock, log),
		Jobs:           jobs.NewHandler(&e2eSyncMock{}, log),
	})

	ts := httptest.NewServer(router)
	cleanup := func() {
		ts.Close()
		rl.Close()
		_ = cacheStore.Close()
		pool.Close()
		_ = closeLog()
	}
	return ts, cleanup
}

func doJSON(t *testing.T, method, url string, payload any, headers map[string]string) (*http.Response, []byte) {
	t.Helper()
	var bodyReader io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		bodyReader = bytes.NewReader(raw)
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		t.Fatal(err)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp, data
}

func TestE2E_HealthEndpoints(t *testing.T) {
	ts, cleanup := setupE2EServer(t)
	defer cleanup()

	resp, body := doJSON(t, http.MethodGet, ts.URL+"/health/live", nil, map[string]string{
		"X-Request-Id": "e2e-health-trace",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on /health/live, got %d: %s", resp.StatusCode, body)
	}
	if resp.Header.Get("X-Request-Id") != "e2e-health-trace" {
		t.Fatalf("expected X-Request-Id echoed, got %q", resp.Header.Get("X-Request-Id"))
	}
	if resp.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("expected nosniff header")
	}
	if resp.Header.Get("X-Powered-By") != "" {
		t.Fatalf("expected no X-Powered-By")
	}
	if resp.Header.Get("X-RateLimit-Limit") != "" {
		t.Fatalf("expected no rate limit headers on health endpoints")
	}

	resp, body = doJSON(t, http.MethodGet, ts.URL+"/health", nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on /health, got %d: %s", resp.StatusCode, body)
	}
	var hc health.CheckResult
	if err := json.Unmarshal(body, &hc); err != nil {
		t.Fatal(err)
	}
	if hc.Status != "ok" || hc.Details["database"].Status != "up" || hc.Details["cache"].Status != "up" {
		t.Fatalf("unexpected health response: %+v", hc)
	}

	resp, body = doJSON(t, http.MethodGet, ts.URL+"/health/ready", nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on /health/ready, got %d: %s", resp.StatusCode, body)
	}
}

func TestE2E_UsersCRUDAndValidation(t *testing.T) {
	ts, cleanup := setupE2EServer(t)
	defer cleanup()

	email := fmt.Sprintf("e2e_user_%d@example.com", time.Now().UnixNano())

	resp, body := doJSON(t, http.MethodPost, ts.URL+"/v1/users", map[string]any{
		"email":   "not-an-email",
		"name":    "",
		"address": "",
	}, nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", resp.StatusCode, body)
	}
	if resp.Header.Get("X-RateLimit-Limit") == "" {
		t.Fatal("expected X-RateLimit-Limit header on /v1 endpoints")
	}

	// Reject unknown fields
	resp, _ = doJSON(t, http.MethodPost, ts.URL+"/v1/users", map[string]any{
		"email":      email,
		"name":       "E2E User",
		"address":    "123 Test Ave",
		"extraField": "forbidden",
	}, nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for extraField, got %d", resp.StatusCode)
	}

	// Reject take=101
	for _, path := range []string{"/v1/users?take=101", "/v1/products?take=101", "/v1/orders?take=101"} {
		resp, _ = doJSON(t, http.MethodGet, ts.URL+path, nil, nil)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400 for %s, got %d", path, resp.StatusCode)
		}
	}

	// Create user
	resp, body = doJSON(t, http.MethodPost, ts.URL+"/v1/users", map[string]any{
		"email":   email,
		"name":    "E2E User",
		"address": "123 Test Ave",
	}, nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, body)
	}
	var created domain.User
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatal(err)
	}

	// Duplicate email -> 409 Conflict
	resp, body = doJSON(t, http.MethodPost, ts.URL+"/v1/users", map[string]any{
		"email":   email,
		"name":    "Dup",
		"address": "456 Test Ave",
	}, nil)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", resp.StatusCode, body)
	}

	// Fetch, Update, Delete, then 404s
	resp, body = doJSON(t, http.MethodGet, fmt.Sprintf("%s/v1/users/%d", ts.URL, created.ID), nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on get, got %d: %s", resp.StatusCode, body)
	}

	resp, body = doJSON(t, http.MethodPatch, fmt.Sprintf("%s/v1/users/%d", ts.URL, created.ID), map[string]any{
		"name": "E2E User Updated",
	}, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on patch, got %d: %s", resp.StatusCode, body)
	}
	var updated domain.User
	_ = json.Unmarshal(body, &updated)
	if updated.Name != "E2E User Updated" {
		t.Fatalf("expected updated name, got %q", updated.Name)
	}

	resp, _ = doJSON(t, http.MethodDelete, fmt.Sprintf("%s/v1/users/%d", ts.URL, created.ID), nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on delete, got %d", resp.StatusCode)
	}

	resp, _ = doJSON(t, http.MethodGet, fmt.Sprintf("%s/v1/users/%d", ts.URL, created.ID), nil, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 after delete on GET, got %d", resp.StatusCode)
	}
	resp, _ = doJSON(t, http.MethodPatch, fmt.Sprintf("%s/v1/users/%d", ts.URL, created.ID), map[string]any{"name": "Ghost"}, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 after delete on PATCH, got %d", resp.StatusCode)
	}
	resp, _ = doJSON(t, http.MethodDelete, fmt.Sprintf("%s/v1/users/%d", ts.URL, created.ID), nil, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 after delete on DELETE, got %d", resp.StatusCode)
	}
}

func TestE2E_ProductsCRUDAndCacheIsolation(t *testing.T) {
	ts, cleanup := setupE2EServer(t)
	defer cleanup()

	uniqueName := fmt.Sprintf("E2E Unique Gadget %d", time.Now().UnixNano())
	resp, body := doJSON(t, http.MethodPost, ts.URL+"/v1/products", map[string]any{
		"name":        uniqueName,
		"price":       24.5,
		"description": "Automated E2E product",
	}, nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, body)
	}

	var product domain.Product
	if err := json.Unmarshal(body, &product); err != nil {
		t.Fatal(err)
	}
	if product.StripeProductID == nil || !strings.HasPrefix(*product.StripeProductID, "prod_e2e_") {
		t.Fatalf("expected stripeProductId, got %v", product.StripeProductID)
	}
	if product.StripePriceID == nil || !strings.HasPrefix(*product.StripePriceID, "price_e2e_") {
		t.Fatalf("expected stripePriceId, got %v", product.StripePriceID)
	}

	resp, body = doJSON(t, http.MethodGet, ts.URL+"/v1/products?search="+strings.ReplaceAll(uniqueName, " ", "+"), nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on search, got %d: %s", resp.StatusCode, body)
	}
	var searchList []domain.Product
	_ = json.Unmarshal(body, &searchList)
	if len(searchList) == 0 {
		t.Fatal("expected search results to include created product")
	}

	resp, body = doJSON(t, http.MethodGet, ts.URL+"/v1/products", nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on list, got %d: %s", resp.StatusCode, body)
	}
	var allList []domain.Product
	_ = json.Unmarshal(body, &allList)
	if len(allList) < len(searchList) {
		t.Fatalf("expected unfiltered list (%d) >= search list (%d)", len(allList), len(searchList))
	}

	// Clear stripe IDs with explicit null
	resp, body = doJSON(t, http.MethodPatch, ts.URL+"/v1/products/"+product.ID, map[string]any{
		"stripeProductId": nil,
		"stripePriceId":   nil,
	}, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on patch null, got %d: %s", resp.StatusCode, body)
	}
	var cleared domain.Product
	_ = json.Unmarshal(body, &cleared)
	if cleared.StripeProductID != nil || cleared.StripePriceID != nil {
		t.Fatalf("expected cleared stripe IDs to be nil, got %+v", cleared)
	}

	resp, _ = doJSON(t, http.MethodDelete, ts.URL+"/v1/products/"+product.ID, nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on delete product, got %d", resp.StatusCode)
	}
}

func TestE2E_OrdersStripeAndJobs(t *testing.T) {
	ts, cleanup := setupE2EServer(t)
	defer cleanup()

	resp, body := doJSON(t, http.MethodPost, ts.URL+"/v1/users", map[string]any{
		"email":   fmt.Sprintf("order_owner_%d@example.com", time.Now().UnixNano()),
		"name":    "Order Owner",
		"address": "456 Order Rd",
	}, nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 user, got %d: %s", resp.StatusCode, body)
	}
	var user domain.User
	_ = json.Unmarshal(body, &user)

	resp, body = doJSON(t, http.MethodPost, ts.URL+"/v1/orders", map[string]any{
		"total":           120.5,
		"userId":          user.ID,
		"stripeInvoiceId": fmt.Sprintf("in_e2e_%d", time.Now().UnixNano()),
	}, nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 order, got %d: %s", resp.StatusCode, body)
	}
	var order domain.OrderWithRelations
	_ = json.Unmarshal(body, &order)

	// Invalid productId -> 400
	resp, _ = doJSON(t, http.MethodPatch, fmt.Sprintf("%s/v1/orders/%d", ts.URL, order.ID), map[string]any{
		"productIds": []string{"non-existent-product"},
	}, nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 on invalid productId, got %d", resp.StatusCode)
	}

	// Unassign userId & clear stripeInvoiceId with null
	resp, body = doJSON(t, http.MethodPatch, fmt.Sprintf("%s/v1/orders/%d", ts.URL, order.ID), map[string]any{
		"userId":          nil,
		"stripeInvoiceId": nil,
		"total":           150,
	}, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on patch order, got %d: %s", resp.StatusCode, body)
	}
	var updated domain.OrderWithRelations
	_ = json.Unmarshal(body, &updated)
	if updated.Total != 150 || updated.UserID != nil || updated.StripeInvoiceID != nil || updated.User != nil {
		t.Fatalf("unexpected updated order: %+v", updated)
	}

	// Cleanup order & user
	doJSON(t, http.MethodDelete, fmt.Sprintf("%s/v1/orders/%d", ts.URL, order.ID), nil, nil)
	doJSON(t, http.MethodDelete, fmt.Sprintf("%s/v1/users/%d", ts.URL, user.ID), nil, nil)

	// Stripe payments -> 201
	resp, body = doJSON(t, http.MethodPost, ts.URL+"/v1/stripe/payments", map[string]any{
		"priceId":  "price_123",
		"quantity": 2,
	}, map[string]string{
		"Origin": "http://localhost:5173",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 on stripe payment, got %d: %s", resp.StatusCode, body)
	}

	// Jobs sync-stripe -> 200
	resp, body = doJSON(t, http.MethodPost, ts.URL+"/v1/jobs/sync-stripe", nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on jobs sync, got %d: %s", resp.StatusCode, body)
	}
	var syncResp jobs.TriggerSyncResponse
	_ = json.Unmarshal(body, &syncResp)
	if syncResp.Result.Processed != 2 || syncResp.Result.Created != 1 {
		t.Fatalf("unexpected sync result: %+v", syncResp)
	}
}
