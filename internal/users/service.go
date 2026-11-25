package users

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
)

type Repository interface {
	Create(ctx context.Context, email, name, address string) (domain.User, error)
	FindAll(ctx context.Context, skip, take *int, search *string) ([]domain.UserWithOrders, error)
	FindByID(ctx context.Context, id int) (domain.UserWithOrdersAndProducts, error)
	Update(ctx context.Context, id int, patch repository.UserPatch) (domain.User, error)
	Delete(ctx context.Context, id int) (domain.User, error)
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

func (s *Service) Create(ctx context.Context, req CreateUserRequest) (domain.User, error) {
	if s.log != nil {
		s.log.Info("Creating new user", "email", req.Email)
	}

	user, err := s.repo.Create(ctx, req.Email, req.Name, req.Address)
	if err != nil {
		return domain.User{}, httputil.MapDBError(err, http.MethodPost)
	}

	if s.log != nil {
		s.log.Info("User created successfully", "id", user.ID)
	}
	return user, nil
}

func (s *Service) FindAll(ctx context.Context, query UserQuery) ([]domain.UserWithOrders, error) {
	if s.log != nil {
		s.log.Info("Fetching users list")
	}

	users, err := s.repo.FindAll(ctx, query.Skip, query.Take, query.Search)
	if err != nil {
		return nil, httputil.MapDBError(err, http.MethodGet)
	}
	return users, nil
}

func (s *Service) FindOne(ctx context.Context, id int) (domain.UserWithOrdersAndProducts, error) {
	if s.log != nil {
		s.log.Info("Fetching user", "id", id)
	}

	user, err := s.repo.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			if s.log != nil {
				s.log.Warn("User not found", "id", id)
			}
			return domain.UserWithOrdersAndProducts{}, httputil.NotFound(fmt.Sprintf("User with ID %d not found", id))
		}
		return domain.UserWithOrdersAndProducts{}, httputil.MapDBError(err, http.MethodGet)
	}
	return user, nil
}

func (s *Service) Update(ctx context.Context, id int, req UpdateUserRequest) (domain.User, error) {
	if s.log != nil {
		s.log.Info("Updating user", "id", id)
	}

	user, err := s.repo.Update(ctx, id, repository.UserPatch{
		Email:   req.Email,
		Name:    req.Name,
		Address: req.Address,
	})
	if err != nil {
		return domain.User{}, httputil.MapDBError(err, http.MethodPatch)
	}

	if s.log != nil {
		s.log.Info("User updated successfully", "id", id)
	}
	return user, nil
}

func (s *Service) Remove(ctx context.Context, id int) (domain.User, error) {
	if s.log != nil {
		s.log.Info("Deleting user", "id", id)
	}

	user, err := s.repo.Delete(ctx, id)
	if err != nil {
		return domain.User{}, httputil.MapDBError(err, http.MethodDelete)
	}

	if s.log != nil {
		s.log.Info("User deleted successfully", "id", id)
	}
	return user, nil
}
