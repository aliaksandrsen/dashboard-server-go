package orders

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/aliaksandrsen/dashboard-server-go/internal/domain"
	"github.com/aliaksandrsen/dashboard-server-go/internal/httputil"
	"github.com/aliaksandrsen/dashboard-server-go/internal/repository"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type Repository interface {
	Create(ctx context.Context, total float64, userID *int, stripeInvoiceID *string, productIDs []string) (domain.OrderWithRelations, error)
	FindAll(ctx context.Context, skip, take, userID *int) ([]domain.OrderWithRelations, error)
	FindByID(ctx context.Context, id int) (domain.OrderWithRelations, error)
	Exists(ctx context.Context, id int) (bool, error)
	Update(ctx context.Context, id int, patch repository.OrderPatch) (domain.OrderWithRelations, error)
	Delete(ctx context.Context, id int) (domain.Order, error)
}

type Service struct {
	repo Repository
	log  *slog.Logger
}

func NewService(repo Repository, log *slog.Logger) *Service {
	return &Service{
		repo: repo,
		log:  log,
	}
}

func (s *Service) Create(ctx context.Context, req CreateOrderRequest) (domain.OrderWithRelations, error) {
	if s.log != nil {
		s.log.Info("Creating new order", "userId", req.UserID)
	}

	order, err := s.repo.Create(ctx, req.Total, req.UserID, req.StripeInvoiceID, req.ProductIDs)
	if err != nil {
		return domain.OrderWithRelations{}, httputil.MapDBError(err, http.MethodPost)
	}

	if s.log != nil {
		s.log.Info("Order created successfully", "id", order.ID)
	}
	return order, nil
}

func (s *Service) FindAll(ctx context.Context, query OrderQuery) ([]domain.OrderWithRelations, error) {
	if s.log != nil {
		s.log.Info("Fetching orders list")
	}

	orders, err := s.repo.FindAll(ctx, query.Skip, query.Take, query.UserID)
	if err != nil {
		return nil, httputil.MapDBError(err, http.MethodGet)
	}
	return orders, nil
}

func (s *Service) FindOne(ctx context.Context, id int) (domain.OrderWithRelations, error) {
	if s.log != nil {
		s.log.Info("Fetching order", "id", id)
	}

	order, err := s.repo.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			if s.log != nil {
				s.log.Warn("Order not found", "id", id)
			}
			return domain.OrderWithRelations{}, httputil.NotFound(fmt.Sprintf("Order with ID %d not found", id))
		}
		return domain.OrderWithRelations{}, httputil.MapDBError(err, http.MethodGet)
	}
	return order, nil
}

func (s *Service) Update(ctx context.Context, id int, req UpdateOrderRequest) (domain.OrderWithRelations, error) {
	if s.log != nil {
		s.log.Info("Updating order", "id", id)
	}

	exists, err := s.repo.Exists(ctx, id)
	if err != nil {
		return domain.OrderWithRelations{}, httputil.MapDBError(err, http.MethodPatch)
	}
	if !exists {
		if s.log != nil {
			s.log.Warn("Order not found", "id", id)
		}
		return domain.OrderWithRelations{}, httputil.NotFound(fmt.Sprintf("Order with ID %d not found", id))
	}

	order, err := s.repo.Update(ctx, id, repository.OrderPatch{
		Total:           req.Total,
		UserID:          req.UserID,
		StripeInvoiceID: req.StripeInvoiceID,
		ProductIDs:      req.ProductIDs,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" && pgErr.ConstraintName == "_OrderToProduct_B_fkey" {
			return domain.OrderWithRelations{}, httputil.BadRequest("Invalid productId provided. Referenced record does not exist.")
		}
		return domain.OrderWithRelations{}, httputil.MapDBError(err, http.MethodPatch)
	}

	if s.log != nil {
		s.log.Info("Order updated successfully", "id", id)
	}
	return order, nil
}

func (s *Service) Remove(ctx context.Context, id int) (domain.Order, error) {
	if s.log != nil {
		s.log.Info("Deleting order", "id", id)
	}

	order, err := s.repo.Delete(ctx, id)
	if err != nil {
		return domain.Order{}, httputil.MapDBError(err, http.MethodDelete)
	}

	if s.log != nil {
		s.log.Info("Order deleted successfully", "id", id)
	}
	return order, nil
}
