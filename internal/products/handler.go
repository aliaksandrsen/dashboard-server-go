package products

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/aliaksandrsen/dashboard-server-go/internal/domain"
	"github.com/aliaksandrsen/dashboard-server-go/internal/httputil"
	"github.com/aliaksandrsen/dashboard-server-go/internal/platform/cache"
	"github.com/go-chi/chi/v5"
)

const (
	productsCacheKey = "products"
	productsCacheTTL = 30 * time.Second
)

type Handler struct {
	service *Service
	cache   cache.Store
	log     *slog.Logger
}

func NewHandler(service *Service, cacheStore cache.Store, log *slog.Logger) *Handler {
	return &Handler{
		service: service,
		cache:   cacheStore,
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
	var req CreateProductRequest
	if err := httputil.DecodeStrictJSON(w, r, &req); err != nil {
		httputil.WriteError(w, r, h.log, err)
		return
	}
	if err := req.Validate(); err != nil {
		httputil.WriteError(w, r, h.log, err)
		return
	}

	product, err := h.service.Create(r.Context(), req)
	if err != nil {
		httputil.WriteError(w, r, h.log, err)
		return
	}

	h.invalidateCache(r.Context())
	httputil.WriteJSON(w, http.StatusCreated, product)
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

	query := ProductQuery{Skip: skip, Take: take, Search: search}
	if err := query.Validate(); err != nil {
		httputil.WriteError(w, r, h.log, err)
		return
	}

	if !query.HasFilters() && h.cache != nil {
		if raw, ok, err := h.cache.Get(r.Context(), productsCacheKey); err == nil && ok {
			var cached []domain.Product
			if err := json.Unmarshal(raw, &cached); err == nil {
				httputil.WriteJSON(w, http.StatusOK, cached)
				return
			}
		}
	}

	products, err := h.service.FindAll(r.Context(), query)
	if err != nil {
		httputil.WriteError(w, r, h.log, err)
		return
	}

	if !query.HasFilters() && h.cache != nil {
		if encoded, err := json.Marshal(products); err == nil {
			if err := h.cache.Set(r.Context(), productsCacheKey, encoded, productsCacheTTL); err != nil && h.log != nil {
				h.log.Warn("failed to populate products cache", "error", err)
			}
		}
	}

	httputil.WriteJSON(w, http.StatusOK, products)
}

func (h *Handler) FindOne(w http.ResponseWriter, r *http.Request) {
	id, err := httputil.ParsePathUUID(chi.URLParam(r, "id"), "id")
	if err != nil {
		httputil.WriteError(w, r, h.log, err)
		return
	}

	product, err := h.service.FindOne(r.Context(), id)
	if err != nil {
		httputil.WriteError(w, r, h.log, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, product)
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := httputil.ParsePathUUID(chi.URLParam(r, "id"), "id")
	if err != nil {
		httputil.WriteError(w, r, h.log, err)
		return
	}

	var req UpdateProductRequest
	if err := httputil.DecodeStrictJSON(w, r, &req); err != nil {
		httputil.WriteError(w, r, h.log, err)
		return
	}
	if err := req.Validate(); err != nil {
		httputil.WriteError(w, r, h.log, err)
		return
	}

	product, err := h.service.Update(r.Context(), id, req)
	if err != nil {
		httputil.WriteError(w, r, h.log, err)
		return
	}

	h.invalidateCache(r.Context())
	httputil.WriteJSON(w, http.StatusOK, product)
}

func (h *Handler) Remove(w http.ResponseWriter, r *http.Request) {
	id, err := httputil.ParsePathUUID(chi.URLParam(r, "id"), "id")
	if err != nil {
		httputil.WriteError(w, r, h.log, err)
		return
	}

	product, err := h.service.Remove(r.Context(), id)
	if err != nil {
		httputil.WriteError(w, r, h.log, err)
		return
	}

	h.invalidateCache(r.Context())
	httputil.WriteJSON(w, http.StatusOK, product)
}

func (h *Handler) invalidateCache(ctx context.Context) {
	if h.cache == nil {
		return
	}
	if err := h.cache.Del(ctx, productsCacheKey); err != nil && h.log != nil {
		h.log.Warn("failed to invalidate products cache", "error", err)
	}
}
