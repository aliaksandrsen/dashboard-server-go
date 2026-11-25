package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/aliaksandrsen/dashboard-server-go/internal/httputil"
)

func Recover(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					if log != nil {
						log.Error("panic recovered",
							"panic", rec,
							"stack", string(debug.Stack()),
							"requestId", httputil.RequestIDFromRequest(r),
						)
					}
					httputil.WriteError(w, r, log, httputil.InternalServerError(
						"Internal server error",
						fmt.Errorf("panic: %v", rec),
					))
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
