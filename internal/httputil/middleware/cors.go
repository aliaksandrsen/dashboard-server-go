package middleware

import (
	"net/http"
	"slices"

	"github.com/aliaksandrsen/dashboard-server-go/internal/platform/origin"
)

func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	normalized := make([]string, 0, len(allowedOrigins))
	for _, o := range allowedOrigins {
		if norm := origin.NormalizeOrigin(o); norm != "" {
			normalized = append(normalized, norm)
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reqOrigin := origin.NormalizeOrigin(r.Header.Get("Origin"))
			w.Header().Add("Vary", "Origin")

			if reqOrigin != "" && slices.Contains(normalized, reqOrigin) {
				w.Header().Set("Access-Control-Allow-Origin", reqOrigin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, PUT, PATCH, POST, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-Id")
			}

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
