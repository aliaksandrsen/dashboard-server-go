package middleware

import "net/http"

// SecurityHeaders mirrors Helmet defaults and ensures X-Powered-By is omitted.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "SAMEORIGIN")
		h.Set("X-DNS-Prefetch-Control", "off")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Cross-Origin-Resource-Policy", "same-origin")
		h.Set("Origin-Agent-Cluster", "?1")
		h.Set("X-Download-Options", "noopen")
		h.Set("X-Permitted-Cross-Domain-Policies", "none")
		h.Del("X-Powered-By")

		next.ServeHTTP(w, r)
	})
}
