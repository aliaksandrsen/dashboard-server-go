package middleware

import (
	"net/http"
	"strings"

	"github.com/aliaksandrsen/dashboard-server-go/internal/httputil"
	"github.com/google/uuid"
)

const maxRequestIDLength = 128

func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		incoming := strings.TrimSpace(r.Header.Get(httputil.RequestIDHeader))
		reqID := incoming
		if reqID == "" || len(reqID) > maxRequestIDLength {
			reqID = uuid.NewString()
		}

		r.Header.Set(httputil.RequestIDHeader, reqID)
		w.Header().Set(httputil.RequestIDHeader, reqID)
		ctx := httputil.ContextWithRequestID(r.Context(), reqID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
