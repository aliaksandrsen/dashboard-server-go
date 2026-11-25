package orders

import (
	"log/slog"
	"net/http"

	"github.com/aliaksandrsen/dashboard-server-go/internal/httputil"
	"github.com/go-chi/chi/v5"
)

type Handler struct {
	service *Service
	log     *slog.Logger
}

func NewHandler(service *Service, log *slog.Logger) *Handler {
	return &Handler{
		service: service,
		log:     log,
	}
}

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/", h.Create)
	r.Get("/", h.FindAll)
	r.Get("/{id}", h.FindOne)
	r.Patch("/{id}", h.Update)
	r.Delete("/{id}", h.Remove)
	return r
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateOrderRequest
	if err := httputil.DecodeStrictJSON(w, r, &req); err != nil {
		httputil.WriteError(w, r, h.log, err)
		return
	}
	if err := req.Validate(); err != nil {
		httputil.WriteError(w, r, h.log, err)
		return
	}

	order, err := h.service.Create(r.Context(), req)
	if err != nil {
		httputil.WriteError(w, r, h.log, err)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, order)
}

func (h *Handler) FindAll(w http.ResponseWriter, r *http.Request) {
	if err := httputil.ValidateAllowedQueryParams(r, "skip", "take", "userId"); err != nil {
		httputil.WriteError(w, r, h.log, err)
		return
	}

	skip, err := httputil.ParseOptionalQueryInt(r, "skip")
	if err != nil {
		httputil.WriteError(w, r, h.log, err)
		return
	}
	take, err := httputil.ParseOptionalQueryInt(r, "take")
	if err != nil {
		httputil.WriteError(w, r, h.log, err)
		return
	}
	userID, err := httputil.ParseOptionalQueryInt(r, "userId")
	if err != nil {
		httputil.WriteError(w, r, h.log, err)
		return
	}

	query := OrderQuery{Skip: skip, Take: take, UserID: userID}
	if err := query.Validate(); err != nil {
		httputil.WriteError(w, r, h.log, err)
		return
	}

	orders, err := h.service.FindAll(r.Context(), query)
	if err != nil {
		httputil.WriteError(w, r, h.log, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, orders)
}

func (h *Handler) FindOne(w http.ResponseWriter, r *http.Request) {
	id, err := httputil.ParsePathInt(chi.URLParam(r, "id"), "id")
	if err != nil {
		httputil.WriteError(w, r, h.log, err)
		return
	}

	order, err := h.service.FindOne(r.Context(), id)
	if err != nil {
		httputil.WriteError(w, r, h.log, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, order)
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := httputil.ParsePathInt(chi.URLParam(r, "id"), "id")
	if err != nil {
		httputil.WriteError(w, r, h.log, err)
		return
	}

	var req UpdateOrderRequest
	if err := httputil.DecodeStrictJSON(w, r, &req); err != nil {
		httputil.WriteError(w, r, h.log, err)
		return
	}
	if err := req.Validate(); err != nil {
		httputil.WriteError(w, r, h.log, err)
		return
	}

	order, err := h.service.Update(r.Context(), id, req)
	if err != nil {
		httputil.WriteError(w, r, h.log, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, order)
}

func (h *Handler) Remove(w http.ResponseWriter, r *http.Request) {
	id, err := httputil.ParsePathInt(chi.URLParam(r, "id"), "id")
	if err != nil {
		httputil.WriteError(w, r, h.log, err)
		return
	}

	order, err := h.service.Remove(r.Context(), id)
	if err != nil {
		httputil.WriteError(w, r, h.log, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, order)
}
