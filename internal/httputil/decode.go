package httputil

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

const MaxBodyBytes = 1 << 20 // 1 MiB

// Optional represents a tri-state JSON field (absent, explicit null, or typed value) for PATCH payloads.
type Optional[T any] struct {
	Set   bool
	Null  bool
	Value T
}

func (o *Optional[T]) UnmarshalJSON(data []byte) error {
	o.Set = true
	trimmed := bytes.TrimSpace(data)
	if bytes.Equal(trimmed, []byte("null")) {
		o.Null = true
		var zero T
		o.Value = zero
		return nil
	}
	o.Null = false
	return json.Unmarshal(trimmed, &o.Value)
}

// DecodeStrictJSON decodes a JSON request body while forbidding unknown fields
// (equivalent to NestJS ValidationPipe with whitelist + forbidNonWhitelisted).
func DecodeStrictJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)
	defer r.Body.Close()

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		var syntaxErr *json.SyntaxError
		var unmarshalTypeErr *json.UnmarshalTypeError
		var maxBytesErr *http.MaxBytesError

		switch {
		case errors.Is(err, io.EOF):
			return BadRequest("Request body must not be empty")
		case errors.As(err, &maxBytesErr):
			return BadRequest("Request body too large")
		case errors.As(err, &syntaxErr):
			return BadRequest(fmt.Sprintf("Malformed JSON at position %d", syntaxErr.Offset))
		case errors.As(err, &unmarshalTypeErr):
			return BadRequest(fmt.Sprintf("%s must be a valid %s", unmarshalTypeErr.Field, unmarshalTypeErr.Type.String()))
		case strings.HasPrefix(err.Error(), "json: unknown field "):
			field := strings.TrimPrefix(err.Error(), "json: unknown field ")
			return BadRequest(fmt.Sprintf("property %s should not exist", field))
		default:
			return BadRequest("Invalid JSON payload")
		}
	}

	var trailing any
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		return BadRequest("Request body must contain only a single JSON object")
	}

	return nil
}

// ValidateAllowedQueryParams ensures no unexpected query parameters are present.
func ValidateAllowedQueryParams(r *http.Request, allowed ...string) error {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, k := range allowed {
		allowedSet[k] = struct{}{}
	}
	for key := range r.URL.Query() {
		if _, ok := allowedSet[key]; !ok {
			return BadRequest(fmt.Sprintf("property %s should not exist", key))
		}
	}
	return nil
}

func ParseOptionalQueryInt(r *http.Request, name string) (*int, error) {
	q := r.URL.Query()
	if !q.Has(name) {
		return nil, nil
	}
	raw := strings.TrimSpace(q.Get(name))
	if raw == "" {
		return nil, nil
	}
	val, err := strconv.Atoi(raw)
	if err != nil {
		return nil, BadRequest(fmt.Sprintf("%s must be an integer number", name))
	}
	return &val, nil
}

func ParseOptionalQueryString(r *http.Request, name string) *string {
	q := r.URL.Query()
	if !q.Has(name) {
		return nil
	}
	val := q.Get(name)
	return &val
}

func ParsePathInt(raw string, name string) (int, error) {
	val, err := strconv.Atoi(raw)
	if err != nil {
		return 0, BadRequest(fmt.Sprintf("Validation failed (numeric string is expected for %s)", name))
	}
	return val, nil
}

func ParsePathUUID(raw string, name string) (string, error) {
	parsed, err := uuid.Parse(raw)
	if err != nil {
		return "", BadRequest(fmt.Sprintf("Validation failed (uuid is expected for %s)", name))
	}
	return parsed.String(), nil
}
