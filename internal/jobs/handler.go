package jobs

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/aliaksandrsen/dashboard-server-go/internal/domain"
	"github.com/aliaksandrsen/dashboard-server-go/internal/httputil"
	"github.com/go-chi/chi/v5"
)

type TriggerSyncResponse struct {
	Message string            `json:"message"`
	Result  domain.SyncResult `json:"result"`
}

type Handler struct {
	sync SyncRunner
	log  *slog.Logger
}

func NewHandler(sync SyncRunner, log *slog.Logger) *Handler {
	return &Handler{
		sync: sync,
		log:  log,
	}
}

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/sync-stripe", h.TriggerStripeSync)
	return r
}

func (h *Handler) TriggerStripeSync(w http.ResponseWriter, r *http.Request) {
	result, err := h.sync.CronSyncStripeOrders(r.Context(), time.Now().UTC())
	if err != nil {
		httputil.WriteError(w, r, h.log, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, TriggerSyncResponse{
		Message: "Stripe orders synchronization triggered successfully",
		Result:  result,
	})
}
