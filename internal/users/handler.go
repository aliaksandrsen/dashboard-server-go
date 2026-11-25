package users

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
	var req CreateUserRequest
	if err := httputil.DecodeStrictJSON(w, r, &req); err != nil {
		httputil.WriteError(w, r, h.log, err)
		return
	}
	if err := req.Validate(); err != nil {
		httputil.WriteError(w, r, h.log, err)
		return
	}

	user, err := h.service.Create(r.Context(), req)
	if err != nil {
		httputil.WriteError(w, r, h.log, err)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, user)
}

func (h *Handler) FindAll(w http.ResponseWriter, r *http.Request) {
	if err := httputil.ValidateAllowedQueryParams(r, "skip", "take", "search"); err != nil {
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
	search := httputil.ParseOptionalQueryString(r, "search")

	query := UserQuery{Skip: skip, Take: take, Search: search}
	if err := query.Validate(); err != nil {
		httputil.WriteError(w, r, h.log, err)
		return
	}

	users, err := h.service.FindAll(r.Context(), query)
	if err != nil {
		httputil.WriteError(w, r, h.log, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, users)
}

func (h *Handler) FindOne(w http.ResponseWriter, r *http.Request) {
	id, err := httputil.ParsePathInt(chi.URLParam(r, "id"), "id")
	if err != nil {
		httputil.WriteError(w, r, h.log, err)
		return
	}

	user, err := h.service.FindOne(r.Context(), id)
	if err != nil {
		httputil.WriteError(w, r, h.log, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, user)
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := httputil.ParsePathInt(chi.URLParam(r, "id"), "id")
	if err != nil {
		httputil.WriteError(w, r, h.log, err)
		return
	}

	var req UpdateUserRequest
	if err := httputil.DecodeStrictJSON(w, r, &req); err != nil {
		httputil.WriteError(w, r, h.log, err)
		return
	}
	if err := req.Validate(); err != nil {
		httputil.WriteError(w, r, h.log, err)
		return
	}

	user, err := h.service.Update(r.Context(), id, req)
	if err != nil {
		httputil.WriteError(w, r, h.log, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, user)
}

func (h *Handler) Remove(w http.ResponseWriter, r *http.Request) {
	id, err := httputil.ParsePathInt(chi.URLParam(r, "id"), "id")
	if err != nil {
		httputil.WriteError(w, r, h.log, err)
		return
	}

	user, err := h.service.Remove(r.Context(), id)
	if err != nil {
		httputil.WriteError(w, r, h.log, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, user)
}
