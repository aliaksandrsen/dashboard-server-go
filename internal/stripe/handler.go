package stripe

import (
	"log/slog"
	"net/http"

	"github.com/aliaksandrsen/dashboard-server-go/internal/httputil"
	"github.com/go-chi/chi/v5"
)

type Handler struct {
	service Manager
	log     *slog.Logger
}

func NewHandler(service Manager, log *slog.Logger) *Handler {
	return &Handler{
		service: service,
		log:     log,
	}
}

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/payments", h.CreatePayment)
	return r
}

func (h *Handler) CreatePayment(w http.ResponseWriter, r *http.Request) {
	var req CreatePaymentRequest
	if err := httputil.DecodeStrictJSON(w, r, &req); err != nil {
		httputil.WriteError(w, r, h.log, err)
		return
	}
	if err := req.Validate(); err != nil {
		httputil.WriteError(w, r, h.log, err)
		return
	}

	res, err := h.service.CreatePayment(r.Context(), req, r.Header.Get("Origin"))
	if err != nil {
		httputil.WriteError(w, r, h.log, err)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, res)
}
