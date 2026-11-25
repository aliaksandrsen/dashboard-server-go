package health

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aliaksandrsen/dashboard-server-go/internal/httputil"
)

type mockDB struct {
	err error
}

func (m *mockDB) Ping(context.Context) error { return m.err }

type mockCache struct {
	data   map[string][]byte
	setErr error
	getErr error
}

func (m *mockCache) Get(_ context.Context, key string) ([]byte, bool, error) {
	if m.getErr != nil {
		return nil, false, m.getErr
	}
	v, ok := m.data[key]
	return v, ok, nil
}

func (m *mockCache) Set(_ context.Context, key string, val []byte, _ time.Duration) error {
	if m.setErr != nil {
		return m.setErr
	}
	if m.data == nil {
		m.data = make(map[string][]byte)
	}
	m.data[key] = val
	return nil
}

func (m *mockCache) Del(context.Context, string) error { return nil }
func (m *mockCache) Close() error                      { return nil }

func TestHealthCheck_OK(t *testing.T) {
	h := NewHandler(&mockDB{}, &mockCache{}, slog.Default())
	r := h.Routes()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var body CheckResult
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Status != "ok" || body.Details["database"].Status != "up" || body.Details["cache"].Status != "up" {
		t.Fatalf("unexpected health payload: %+v", body)
	}
}

func TestHealthCheck_Unhealthy503(t *testing.T) {
	h := NewHandler(&mockDB{}, &mockCache{setErr: errors.New("redis down")}, slog.Default())
	r := h.Routes()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	req = req.WithContext(httputil.ContextWithRequestID(req.Context(), "req-health"))
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}

	var body CheckResult
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Status != "error" || body.Error["cache"].Status != "down" || body.RequestID != "req-health" || body.Timestamp == "" {
		t.Fatalf("unexpected 503 payload: %+v", body)
	}
}

func TestHealthLive(t *testing.T) {
	h := NewHandler(&mockDB{}, &mockCache{}, slog.Default())
	r := h.Routes()

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/live", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}
