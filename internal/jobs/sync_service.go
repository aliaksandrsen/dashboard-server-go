package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/aliaksandrsen/dashboard-server-go/internal/domain"
	"github.com/aliaksandrsen/dashboard-server-go/internal/stripe"
	"github.com/jackc/pgx/v5"
	"github.com/robfig/cron/v3"
)

type OrdersStore interface {
	FindByStripeInvoiceID(ctx context.Context, stripeInvoiceID string) (*domain.Order, error)
	Create(ctx context.Context, total float64, userID *int, stripeInvoiceID *string, productIDs []string) (domain.OrderWithRelations, error)
}

type UsersStore interface {
	FindByEmail(ctx context.Context, email string) (domain.User, error)
}

type StripeInvoiceLister interface {
	ListPaidInvoicesSince(ctx context.Context, sinceUnix int64) ([]stripe.Invoice, error)
}

type SyncRunner interface {
	CronSyncStripeOrders(ctx context.Context, syncDate time.Time) (domain.SyncResult, error)
}

type SyncService struct {
	orders      OrdersStore
	users       UsersStore
	stripe      StripeInvoiceLister
	log         *slog.Logger
	cronExpr    string
	timezone    string
	scheduler   *cron.Cron
	mu          sync.Mutex
	entries     map[string]cron.EntryID
	syncTimeout time.Duration
}

func NewSyncService(
	orders OrdersStore,
	users UsersStore,
	stripeClient StripeInvoiceLister,
	cronExpr string,
	timezone string,
	log *slog.Logger,
) (*SyncService, error) {
	if cronExpr == "" {
		cronExpr = "5 00 * * *"
	}
	if timezone == "" {
		timezone = "America/New_York"
	}

	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, fmt.Errorf("load timezone %q: %w", timezone, err)
	}

	scheduler := cron.New(
		cron.WithLocation(loc),
		cron.WithParser(cron.NewParser(cron.Minute|cron.Hour|cron.Dom|cron.Month|cron.Dow)),
	)

	return &SyncService{
		orders:      orders,
		users:       users,
		stripe:      stripeClient,
		log:         log,
		cronExpr:    cronExpr,
		timezone:    timezone,
		scheduler:   scheduler,
		entries:     make(map[string]cron.EntryID),
		syncTimeout: 2 * time.Minute,
	}, nil
}

func (s *SyncService) Start() error {
	if err := s.AddCronJob("sync_stripe_orders", s.cronExpr, func(ctx context.Context) error {
		_, err := s.CronSyncStripeOrders(ctx, time.Now().UTC())
		return err
	}); err != nil {
		return err
	}
	s.scheduler.Start()
	return nil
}

func (s *SyncService) Stop(ctx context.Context) error {
	stopCtx := s.scheduler.Stop()
	select {
	case <-stopCtx.Done():
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *SyncService) AddCronJob(name, expr string, fn func(context.Context) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if oldID, exists := s.entries[name]; exists {
		s.scheduler.Remove(oldID)
		delete(s.entries, name)
	}

	id, err := s.scheduler.AddFunc(expr, func() {
		ctx, cancel := context.WithTimeout(context.Background(), s.syncTimeout)
		defer cancel()
		if err := fn(ctx); err != nil && s.log != nil {
			s.log.Error("Error executing cron job", "name", name, "error", err)
		}
	})
	if err != nil {
		return err
	}

	s.entries[name] = id
	if s.log != nil {
		s.log.Info("Cron job added", "name", name, "cron", expr, "timezone", s.timezone)
	}
	return nil
}

func (s *SyncService) DeleteCronJob(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if id, exists := s.entries[name]; exists {
		s.scheduler.Remove(id)
		delete(s.entries, name)
		if s.log != nil {
			s.log.Info("Cron job deleted", "name", name)
		}
	}
}

// CronSyncStripeOrders synchronizes paid Stripe invoices from the previous 24 hours into PostgreSQL Orders.
func (s *SyncService) CronSyncStripeOrders(ctx context.Context, syncDate time.Time) (domain.SyncResult, error) {
	if syncDate.IsZero() {
		syncDate = time.Now().UTC()
	}

	if s.log != nil {
		s.log.Debug("Stripe orders sync started", "startedAt", time.Now().UTC().Format(time.RFC3339Nano))
	}

	previousDate := syncDate.Add(-24 * time.Hour)
	previousTimestampSeconds := previousDate.Unix()

	invoices, err := s.stripe.ListPaidInvoicesSince(ctx, previousTimestampSeconds)
	if err != nil {
		if s.log != nil {
			s.log.Error("Failed to fetch invoices from Stripe", "error", err)
		}
		return domain.SyncResult{}, err
	}

	if len(invoices) == 0 {
		if s.log != nil {
			s.log.Debug("No new Stripe invoices", "since", previousDate.UTC().Format(time.RFC3339Nano))
		}
		return domain.SyncResult{}, nil
	}

	var res domain.SyncResult
	res.Processed = len(invoices)

	for _, inv := range invoices {
		existing, err := s.orders.FindByStripeInvoiceID(ctx, inv.ID)
		if err != nil {
			res.Failed++
			if s.log != nil {
				s.log.Error("Failed checking existing order for invoice", "invoiceId", inv.ID, "error", err)
			}
			continue
		}

		if existing != nil {
			res.Skipped++
			continue
		}

		total := float64(inv.Total) / 100.0

		var userID *int
		if inv.CustomerEmail != "" {
			user, err := s.users.FindByEmail(ctx, inv.CustomerEmail)
			if err == nil {
				uid := user.ID
				userID = &uid
			} else if !errors.Is(err, pgx.ErrNoRows) {
				res.Failed++
				if s.log != nil {
					s.log.Error("Failed looking up user by invoice email", "invoiceId", inv.ID, "email", inv.CustomerEmail, "error", err)
				}
				continue
			}
		}

		invoiceID := inv.ID
		if _, err := s.orders.Create(ctx, total, userID, &invoiceID, nil); err != nil {
			res.Failed++
			if s.log != nil {
				s.log.Error("Failed creating order for invoice", "invoiceId", inv.ID, "error", err)
			}
			continue
		}

		res.Created++
		if s.log != nil {
			s.log.Info("Created order record for Stripe invoice", "invoiceId", inv.ID)
		}
	}

	if s.log != nil {
		s.log.Debug("Stripe orders sync finished", "finishedAt", time.Now().UTC().Format(time.RFC3339Nano))
	}

	return res, nil
}
