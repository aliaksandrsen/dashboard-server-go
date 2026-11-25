package server

import (
	"log/slog"
	"net/http"

	"github.com/aliaksandrsen/dashboard-server-go/internal/health"
	"github.com/aliaksandrsen/dashboard-server-go/internal/httputil"
	"github.com/aliaksandrsen/dashboard-server-go/internal/httputil/middleware"
	"github.com/aliaksandrsen/dashboard-server-go/internal/jobs"
	"github.com/aliaksandrsen/dashboard-server-go/internal/orders"
	"github.com/aliaksandrsen/dashboard-server-go/internal/products"
	"github.com/aliaksandrsen/dashboard-server-go/internal/stripe"
	"github.com/aliaksandrsen/dashboard-server-go/internal/users"
	"github.com/go-chi/chi/v5"
)

type Dependencies struct {
	AllowedOrigins []string
	RateLimiter    *middleware.RateLimiter
	Logger         *slog.Logger
	Health         *health.Handler
	Users          *users.Handler
	Products       *products.Handler
	Orders         *orders.Handler
	Stripe         *stripe.Handler
	Jobs           *jobs.Handler
}

func NewRouter(deps Dependencies) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.Recover(deps.Logger))
	r.Use(middleware.SecurityHeaders)
	r.Use(middleware.CORS(deps.AllowedOrigins))

	r.NotFound(func(w http.ResponseWriter, req *http.Request) {
		httputil.WriteError(w, req, deps.Logger, httputil.NotFound("Cannot "+req.Method+" "+req.URL.Path))
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, req *http.Request) {
		httputil.WriteError(w, req, deps.Logger, httputil.NewAppError(http.StatusMethodNotAllowed, "Method Not Allowed", nil))
	})

	// Health routes are version-neutral and bypass rate limiting.
	r.Mount("/health", deps.Health.Routes())

	// Versioned API routes (/v1/*) enforce rate limiting.
	r.Route("/v1", func(v1 chi.Router) {
		if deps.RateLimiter != nil {
			v1.Use(deps.RateLimiter.Middleware)
		}
		v1.Mount("/users", deps.Users.Routes())
		v1.Mount("/products", deps.Products.Routes())
		v1.Mount("/orders", deps.Orders.Routes())
		v1.Mount("/stripe", deps.Stripe.Routes())
		v1.Mount("/jobs", deps.Jobs.Routes())
	})

	return r
}
