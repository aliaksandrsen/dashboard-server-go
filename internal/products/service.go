package products

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/aliaksandrsen/dashboard-server-go/internal/domain"
	"github.com/aliaksandrsen/dashboard-server-go/internal/httputil"
	"github.com/aliaksandrsen/dashboard-server-go/internal/repository"
	"github.com/aliaksandrsen/dashboard-server-go/internal/stripe"
	"github.com/jackc/pgx/v5"
)

type Repository interface {
	Create(ctx context.Context, name string, price float64, stripeProductID, stripePriceID *string) (domain.Product, error)
	FindAll(ctx context.Context, skip, take *int, search *string) ([]domain.Product, error)
	FindByID(ctx context.Context, id string) (domain.Product, error)
	Update(ctx context.Context, id string, patch repository.ProductPatch) (domain.Product, error)
	Delete(ctx context.Context, id string) (domain.Product, error)
}

type StripeClient interface {
	CreateProduct(ctx context.Context, params stripe.CreateProductParams) (stripe.ProductResult, error)
	DeactivateProduct(ctx context.Context, stripeProductID string)
}

type Service struct {
	repo   Repository
	stripe StripeClient
	log    *slog.Logger
}

func NewService(repo Repository, stripeClient StripeClient, log *slog.Logger) *Service {
	return &Service{
		repo:   repo,
		stripe: stripeClient,
		log:    log,
	}
}

// Create runs a two-phase pipeline:
// 1. Register product and price in Stripe (when stripeProductId is not provided).
// 2. Persist product in PostgreSQL, automatically deactivating the Stripe product on DB failure.
func (s *Service) Create(ctx context.Context, req CreateProductRequest) (domain.Product, error) {
	stripeProductID := req.StripeProductID
	stripePriceID := req.StripePriceID
	createdInStripe := false

	if stripeProductID == nil || *stripeProductID == "" {
		if s.log != nil {
			s.log.Info("Phase 1: Creating product in Stripe", "name", req.Name, "price", req.Price)
		}
		res, err := s.stripe.CreateProduct(ctx, stripe.CreateProductParams{
			Name:        req.Name,
			Price:       req.Price,
			Description: req.Description,
		})
		if err != nil {
			return domain.Product{}, err
		}
		stripeProductID = &res.StripeProductID
		stripePriceID = &res.StripePriceID
		createdInStripe = true
		if s.log != nil {
			s.log.Info("Phase 1 completed", "stripeProductId", res.StripeProductID, "stripePriceId", res.StripePriceID)
		}
	} else if s.log != nil {
		s.log.Info("Phase 1 skipped: Existing stripeProductId provided", "stripeProductId", *stripeProductID)
	}

	product, err := s.repo.Create(ctx, req.Name, req.Price, stripeProductID, stripePriceID)
	if err != nil {
		if s.log != nil {
			s.log.Error("Phase 2 failed: Database insertion failed; initiating rollback", "name", req.Name, "error", err)
		}
		if createdInStripe && stripeProductID != nil && *stripeProductID != "" {
			s.stripe.DeactivateProduct(ctx, *stripeProductID)
		}
		return domain.Product{}, httputil.MapDBError(err, http.MethodPost)
	}

	if s.log != nil {
		s.log.Info("Phase 2 completed: Product created", "id", product.ID)
	}
	return product, nil
}

func (s *Service) FindAll(ctx context.Context, query ProductQuery) ([]domain.Product, error) {
	if s.log != nil {
		s.log.Info("Fetching products list")
	}
	products, err := s.repo.FindAll(ctx, query.Skip, query.Take, query.Search)
	if err != nil {
		return nil, httputil.MapDBError(err, http.MethodGet)
	}
	return products, nil
}

func (s *Service) FindOne(ctx context.Context, id string) (domain.Product, error) {
	if s.log != nil {
		s.log.Info("Fetching product", "id", id)
	}
	product, err := s.repo.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			if s.log != nil {
				s.log.Warn("Product not found", "id", id)
			}
			return domain.Product{}, httputil.NotFound(fmt.Sprintf("Product with ID %s not found", id))
		}
		return domain.Product{}, httputil.MapDBError(err, http.MethodGet)
	}
	return product, nil
}

func (s *Service) Update(ctx context.Context, id string, req UpdateProductRequest) (domain.Product, error) {
	if s.log != nil {
		s.log.Info("Updating product", "id", id)
	}
	product, err := s.repo.Update(ctx, id, repository.ProductPatch{
		Name:            req.Name,
		Price:           req.Price,
		StripeProductID: req.StripeProductID,
		StripePriceID:   req.StripePriceID,
	})
	if err != nil {
		return domain.Product{}, httputil.MapDBError(err, http.MethodPatch)
	}
	if s.log != nil {
		s.log.Info("Product updated successfully", "id", id)
	}
	return product, nil
}

func (s *Service) Remove(ctx context.Context, id string) (domain.Product, error) {
	if s.log != nil {
		s.log.Info("Deleting product", "id", id)
	}
	product, err := s.repo.Delete(ctx, id)
	if err != nil {
		return domain.Product{}, httputil.MapDBError(err, http.MethodDelete)
	}
	if s.log != nil {
		s.log.Info("Product deleted successfully", "id", id)
	}
	return product, nil
}
