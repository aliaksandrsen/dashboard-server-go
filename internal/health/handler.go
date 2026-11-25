package health

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/aliaksandrsen/dashboard-server-go/internal/httputil"
	"github.com/aliaksandrsen/dashboard-server-go/internal/platform/cache"
	"github.com/go-chi/chi/v5"
)

const (
	healthPingKey = "health:ping"
	healthPingTTL = 5 * time.Second
)

type DBPinger interface {
	Ping(ctx context.Context) error
}

type IndicatorStatus struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

type CheckResult struct {
	Status    string                     `json:"status"`
	Info      map[string]IndicatorStatus `json:"info"`
	Error     map[string]IndicatorStatus `json:"error"`
	Details   map[string]IndicatorStatus `json:"details"`
	Path      string                     `json:"path,omitempty"`
	Timestamp string                     `json:"timestamp,omitempty"`
	RequestID string                     `json:"requestId,omitempty"`
}

type Handler struct {
	db    DBPinger
	cache cache.Store
	log   *slog.Logger
}

func NewHandler(db DBPinger, cacheStore cache.Store, log *slog.Logger) *Handler {
	return &Handler{
		db:    db,
		cache: cacheStore,
		log:   log,
	}
}

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.Check)
	r.Get("/live", h.Live)
	r.Get("/ready", h.Ready)
	return r
}

func (h *Handler) Check(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var (
		wg      sync.WaitGroup
		dbStat  IndicatorStatus
		cacheSt IndicatorStatus
	)

	wg.Add(2)
	go func() {
		defer wg.Done()
		if h.db == nil {
			dbStat = IndicatorStatus{Status: "down", Message: "Database client is not configured"}
			return
		}
		if err := h.db.Ping(ctx); err != nil {
			dbStat = IndicatorStatus{Status: "down", Message: err.Error()}
			return
		}
		dbStat = IndicatorStatus{Status: "up"}
	}()

	go func() {
		defer wg.Done()
		if h.cache == nil {
			cacheSt = IndicatorStatus{Status: "down", Message: "Cache client is not configured"}
			return
		}
		if err := h.cache.Set(ctx, healthPingKey, []byte("ok"), healthPingTTL); err != nil {
			cacheSt = IndicatorStatus{Status: "down", Message: err.Error()}
			return
		}
		val, found, err := h.cache.Get(ctx, healthPingKey)
		if err != nil {
			cacheSt = IndicatorStatus{Status: "down", Message: err.Error()}
			return
		}
		if !found || string(val) != "ok" {
			cacheSt = IndicatorStatus{Status: "down", Message: "Cache roundtrip check returned unexpected value"}
			return
		}
		cacheSt = IndicatorStatus{Status: "up"}
	}()

	wg.Wait()

	info := make(map[string]IndicatorStatus)
	errMap := make(map[string]IndicatorStatus)
	details := map[string]IndicatorStatus{
		"database": dbStat,
		"cache":    cacheSt,
	}

	for name, st := range details {
		if st.Status == "up" {
			info[name] = st
		} else {
			errMap[name] = st
		}
	}

	if len(errMap) > 0 {
		reqID := httputil.RequestIDFromRequest(r)
		reqIDLog := reqID
		if reqIDLog == "" {
			reqIDLog = "n/a"
		}
		if h.log != nil {
			h.log.Warn("health check failed",
				"method", r.Method,
				"path", r.URL.RequestURI(),
				"status", http.StatusServiceUnavailable,
				"requestId", reqIDLog,
			)
		}

		res := CheckResult{
			Status:    "error",
			Info:      info,
			Error:     errMap,
			Details:   details,
			Path:      r.URL.RequestURI(),
			Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
			RequestID: reqID,
		}
		httputil.WriteJSON(w, http.StatusServiceUnavailable, res)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, CheckResult{
		Status:  "ok",
		Info:    info,
		Error:   errMap,
		Details: details,
	})
}

func (h *Handler) Live(w http.ResponseWriter, r *http.Request) {
	httputil.WriteJSON(w, http.StatusOK, map[string]string{
		"status":    "ok",
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
	})
}

func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	h.Check(w, r)
}
