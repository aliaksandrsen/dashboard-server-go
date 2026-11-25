package httputil

import (
	"context"
	"net/http"
)

const RequestIDHeader = "X-Request-Id"

type ctxKeyRequestID struct{}

func ContextWithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, ctxKeyRequestID{}, requestID)
}

func RequestIDFromContext(ctx context.Context) string {
	if val, ok := ctx.Value(ctxKeyRequestID{}).(string); ok {
		return val
	}
	return ""
}

func RequestIDFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	if id := RequestIDFromContext(r.Context()); id != "" {
		return id
	}
	return r.Header.Get(RequestIDHeader)
}
