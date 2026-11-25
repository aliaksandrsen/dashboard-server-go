package httputil

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestMapDBError(t *testing.T) {
	errUnique := MapDBError(&pgconn.PgError{Code: "23505"}, http.MethodPost)
	if errUnique.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409, got %d", errUnique.StatusCode)
	}

	errFKDelete := MapDBError(&pgconn.PgError{Code: "23503"}, http.MethodDelete)
	if errFKDelete.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 on DELETE FK error, got %d", errFKDelete.StatusCode)
	}

	errFKPost := MapDBError(&pgconn.PgError{Code: "23503"}, http.MethodPost)
	if errFKPost.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 on POST FK error, got %d", errFKPost.StatusCode)
	}

	errNoRowsPatch := MapDBError(pgx.ErrNoRows, http.MethodPatch)
	if errNoRowsPatch.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 on PATCH no rows, got %d", errNoRowsPatch.StatusCode)
	}

	errNoRowsPost := MapDBError(pgx.ErrNoRows, http.MethodPost)
	if errNoRowsPost.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 on POST no rows, got %d", errNoRowsPost.StatusCode)
	}

	errUnknown := MapDBError(errors.New("SELECT * FROM secret_table failed"), http.MethodGet)
	if errUnknown.StatusCode != http.StatusInternalServerError || errUnknown.Message != "Internal server error" {
		t.Fatalf("expected sanitized 500 error, got %+v", errUnknown)
	}
}

func TestWriteError_IncludesMetadata(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/users", nil)
	req = req.WithContext(ContextWithRequestID(req.Context(), "req-123"))
	rec := httptest.NewRecorder()

	WriteError(rec, req, slog.Default(), BadRequestMessages([]string{"email must be an email"}))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}

	var body ErrorResponseBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	if body.StatusCode != 400 || body.Error != "Bad Request" || body.Path != "/v1/users" || body.RequestID != "req-123" || body.Timestamp == "" {
		t.Fatalf("unexpected error response body: %+v", body)
	}
}

func TestDecodeStrictJSON_RejectsUnknownFields(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
	}
	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{"name":"ok","extra":"bad"}`))
	rec := httptest.NewRecorder()

	var dst payload
	err := DecodeStrictJSON(rec, req, &dst)
	if err == nil {
		t.Fatal("expected error for unknown field")
	}
	var appErr *AppError
	if !errors.As(err, &appErr) || appErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 AppError, got %v", err)
	}
}

func TestOptionalUnmarshal(t *testing.T) {
	type patch struct {
		UserID Optional[int] `json:"userId"`
	}

	var p1 patch
	if err := json.Unmarshal([]byte(`{}`), &p1); err != nil {
		t.Fatal(err)
	}
	if p1.UserID.Set || p1.UserID.Null {
		t.Fatalf("expected unset optional, got %+v", p1.UserID)
	}

	var p2 patch
	if err := json.Unmarshal([]byte(`{"userId":null}`), &p2); err != nil {
		t.Fatal(err)
	}
	if !p2.UserID.Set || !p2.UserID.Null {
		t.Fatalf("expected explicit null optional, got %+v", p2.UserID)
	}

	var p3 patch
	if err := json.Unmarshal([]byte(`{"userId":42}`), &p3); err != nil {
		t.Fatal(err)
	}
	if !p3.UserID.Set || p3.UserID.Null || p3.UserID.Value != 42 {
		t.Fatalf("expected value 42, got %+v", p3.UserID)
	}
}
