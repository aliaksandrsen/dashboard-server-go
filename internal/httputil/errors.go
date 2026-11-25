package httputil

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type ErrorResponseBody struct {
	StatusCode int    `json:"statusCode"`
	Error      string `json:"error"`
	Message    any    `json:"message"`
	Path       string `json:"path"`
	Timestamp  string `json:"timestamp"`
	RequestID  string `json:"requestId,omitempty"`
}

var statusNames = map[int]string{
	http.StatusBadRequest:          "Bad Request",
	http.StatusUnauthorized:        "Unauthorized",
	http.StatusForbidden:           "Forbidden",
	http.StatusNotFound:            "Not Found",
	http.StatusMethodNotAllowed:    "Method Not Allowed",
	http.StatusConflict:            "Conflict",
	http.StatusTooManyRequests:     "Too Many Requests",
	http.StatusInternalServerError: "Internal Server Error",
	http.StatusBadGateway:          "Bad Gateway",
	http.StatusServiceUnavailable:  "Service Unavailable",
}

type AppError struct {
	StatusCode int
	ErrLabel   string
	Message    any
	Cause      error
}

func (e *AppError) Error() string {
	switch msg := e.Message.(type) {
	case string:
		return msg
	default:
		return e.ErrLabel
	}
}

func (e *AppError) Unwrap() error {
	return e.Cause
}

func NewAppError(statusCode int, message any, cause error) *AppError {
	label, ok := statusNames[statusCode]
	if !ok {
		label = http.StatusText(statusCode)
		if label == "" {
			label = "Error"
		}
	}
	return &AppError{
		StatusCode: statusCode,
		ErrLabel:   label,
		Message:    message,
		Cause:      cause,
	}
}

func BadRequest(msg string) *AppError {
	return NewAppError(http.StatusBadRequest, msg, nil)
}

func BadRequestMessages(msgs []string) *AppError {
	return NewAppError(http.StatusBadRequest, msgs, nil)
}

func NotFound(msg string) *AppError {
	return NewAppError(http.StatusNotFound, msg, nil)
}

func Conflict(msg string) *AppError {
	return NewAppError(http.StatusConflict, msg, nil)
}

func TooManyRequests(msg string) *AppError {
	return NewAppError(http.StatusTooManyRequests, msg, nil)
}

func InternalServerError(msg string, cause error) *AppError {
	return NewAppError(http.StatusInternalServerError, msg, cause)
}

func BadGateway(msg string, cause error) *AppError {
	return NewAppError(http.StatusBadGateway, msg, cause)
}

func ServiceUnavailable(msg string, cause error) *AppError {
	return NewAppError(http.StatusServiceUnavailable, msg, cause)
}

// MapDBError maps PostgreSQL / pgx errors into HTTP AppErrors matching NestJS AllExceptionsFilter.
func MapDBError(err error, method string) *AppError {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505": // unique_violation (Prisma P2002)
			return NewAppError(http.StatusConflict, "A record with this unique value already exists.", err)
		case "23503": // foreign_key_violation (Prisma P2003)
			if method == http.MethodDelete {
				return NewAppError(http.StatusConflict, "Cannot delete resource because related records exist.", err)
			}
			return NewAppError(http.StatusBadRequest, "Referenced record does not exist.", err)
		case "22P02": // invalid_text_representation
			return NewAppError(http.StatusBadRequest, "Invalid database query parameters.", err)
		default:
			return NewAppError(http.StatusBadRequest, "Database request failed.", err)
		}
	}

	if errors.Is(err, pgx.ErrNoRows) {
		if method == http.MethodPost {
			return NewAppError(http.StatusBadRequest, "Referenced record does not exist.", err)
		}
		return NewAppError(http.StatusNotFound, "Requested record was not found.", err)
	}

	return NewAppError(http.StatusInternalServerError, "Internal server error", err)
}

func WriteJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(payload)
}

func WriteError(w http.ResponseWriter, r *http.Request, log *slog.Logger, err error) {
	var appErr *AppError
	if !errors.As(err, &appErr) {
		appErr = NewAppError(http.StatusInternalServerError, "Internal server error", err)
	}

	requestID := RequestIDFromRequest(r)
	path := ""
	method := "GET"
	if r != nil {
		if r.URL != nil {
			path = r.URL.RequestURI()
		}
		if r.Method != "" {
			method = r.Method
		}
	}

	if log != nil {
		reqIDLog := requestID
		if reqIDLog == "" {
			reqIDLog = "n/a"
		}
		if appErr.StatusCode >= 500 {
			log.Error("request failed",
				"method", method,
				"path", path,
				"status", appErr.StatusCode,
				"requestId", reqIDLog,
				"error", err,
			)
		} else {
			log.Warn("request rejected",
				"method", method,
				"path", path,
				"status", appErr.StatusCode,
				"errorLabel", appErr.ErrLabel,
				"requestId", reqIDLog,
			)
		}
	}

	body := ErrorResponseBody{
		StatusCode: appErr.StatusCode,
		Error:      appErr.ErrLabel,
		Message:    appErr.Message,
		Path:       path,
		Timestamp:  time.Now().UTC().Format(time.RFC3339Nano),
		RequestID:  requestID,
	}

	WriteJSON(w, appErr.StatusCode, body)
}
