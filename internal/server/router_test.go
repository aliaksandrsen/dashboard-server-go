package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aliaksandrsen/dashboard-server-go/internal/domain"
	"github.com/aliaksandrsen/dashboard-server-go/internal/health"
	"github.com/aliaksandrsen/dashboard-server-go/internal/httputil"
	"github.com/aliaksandrsen/dashboard-server-go/internal/httputil/middleware"
	"github.com/aliaksandrsen/dashboard-server-go/internal/jobs"
	"github.com/aliaksandrsen/dashboard-server-go/internal/orders"
	"github.com/aliaksandrsen/dashboard-server-go/internal/products"
	"github.com/aliaksandrsen/dashboard-server-go/internal/stripe"
	"github.com/aliaksandrsen/dashboard-server-go/internal/users"
)

type stubDB struct{}

func (stubDB) Ping(context.Context) error { return nil }

type stubCache struct{}

func (stubCache) Get(context.Context, string) ([]byte, bool, error) { return []byte("ok"), true, nil }
func (stubCache) Set(context.Context, string, []byte, time.Duration) error {
	return nil
}
func (stubCache) Del(context.Context, string) error { return nil }
func (stubCache) Close() error                      { return nil }

type stubSync struct{}

func (stubSync) CronSyncStripeOrders(context.Context, time.Time) (domain.SyncResult, error) {
	return domain.SyncResult{}, nil
}

func TestRouter_HealthBypassesRateLimitAndUnifiedErrors(t *testing.T) {
	log := slog.Default()
	rl := middleware.NewRateLimiter(time.Minute, 5, log)
	defer rl.Close()

	router := NewRouter(Dependencies{
		AllowedOrigins: []string{"http://localhost:5173"},
		RateLimiter:    rl,
		Logger:         log,
		Health:         health.NewHandler(stubDB{}, stubCache{}, log),
		Users:          users.NewHandler(users.NewService(nil, log), log),
		Products:       products.NewHandler(products.NewService(nil, nil, log), stubCache{}, log),
		Orders:         orders.NewHandler(orders.NewService(nil, log), log),
		Stripe:         stripe.NewHandler(nil, log),
		Jobs:           jobs.NewHandler(stubSync{}, log),
	})

	// /health/live must include security headers and X-Request-Id, and omit rate limit headers
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	req.Header.Set("X-Request-Id", "router-trace")
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Header().Get("X-Request-Id") != "router-trace" {
		t.Fatalf("expected X-Request-Id=router-trace, got %q", rec.Header().Get("X-Request-Id"))
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("expected nosniff header")
	}
	if rec.Header().Get("X-RateLimit-Limit") != "" {
		t.Fatal("expected no rate limit header on /health/live")
	}

	// Unknown route returns unified 404 JSON
	rec404 := httptest.NewRecorder()
	router.ServeHTTP(rec404, httptest.NewRequest(http.MethodGet, "/v1/does-not-exist", nil))
	if rec404.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec404.Code)
	}
	var errBody httputil.ErrorResponseBody
	if err := json.Unmarshal(rec404.Body.Bytes(), &errBody); err != nil {
		t.Fatal(err)
	}
	if errBody.StatusCode != 404 || errBody.Error != "Not Found" || errBody.Path != "/v1/does-not-exist" || errBody.RequestID == "" {
		t.Fatalf("unexpected 404 body: %+v", errBody)
	}

	// Rate-limited /v1 endpoint sets X-RateLimit-Limit
	recJob := httptest.NewRecorder()
	router.ServeHTTP(recJob, httptest.NewRequest(http.MethodPost, "/v1/jobs/sync-stripe", nil))
	if recJob.Code != http.StatusOK || recJob.Header().Get("X-RateLimit-Limit") != "5" {
		t.Fatalf("expected 200 with X-RateLimit-Limit=5, got %d / %q", recJob.Code, recJob.Header().Get("X-RateLimit-Limit"))
	}
}
