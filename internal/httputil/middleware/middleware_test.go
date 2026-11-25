package middleware

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRequestIDMiddleware(t *testing.T) {
	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Generates UUID when absent
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	handler.ServeHTTP(rec, req)
	if rec.Header().Get("X-Request-Id") == "" {
		t.Fatal("expected generated X-Request-Id")
	}

	// Preserves valid header
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req2.Header.Set("X-Request-Id", "  trace-123  ")
	handler.ServeHTTP(rec2, req2)
	if rec2.Header().Get("X-Request-Id") != "trace-123" {
		t.Fatalf("expected trace-123, got %q", rec2.Header().Get("X-Request-Id"))
	}

	// Replaces overly long header
	rec3 := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodGet, "/test", nil)
	tooLong := strings.Repeat("a", 129)
	req3.Header.Set("X-Request-Id", tooLong)
	handler.ServeHTTP(rec3, req3)
	if rec3.Header().Get("X-Request-Id") == tooLong {
		t.Fatal("expected overly long request ID to be replaced")
	}
}

func TestSecurityHeadersMiddleware(t *testing.T) {
	handler := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("expected nosniff header")
	}
	if rec.Header().Get("X-Powered-By") != "" {
		t.Fatal("expected no X-Powered-By header")
	}
}

func TestRateLimiterMiddleware(t *testing.T) {
	rl := NewRateLimiter(time.Minute, 2, slog.Default())
	defer rl.Close()

	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 1; i <= 2; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/v1/users", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d expected 200, got %d", i, rec.Code)
		}
		if rec.Header().Get("X-RateLimit-Limit") != "2" {
			t.Fatalf("expected X-RateLimit-Limit=2, got %q", rec.Header().Get("X-RateLimit-Limit"))
		}
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/users", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 on 3rd request, got %d", rec.Code)
	}
}

func TestCORSMiddleware(t *testing.T) {
	handler := CORS([]string{"http://localhost:5173"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/v1/users", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204 preflight, got %d", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "http://localhost:5173" {
		t.Fatalf("expected allowed origin header, got %q", rec.Header().Get("Access-Control-Allow-Origin"))
	}
}
