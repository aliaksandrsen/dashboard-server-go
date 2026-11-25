package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aliaksandrsen/dashboard-server-go/internal/domain"
	"github.com/aliaksandrsen/dashboard-server-go/internal/stripe"
	"github.com/jackc/pgx/v5"
)

type mockOrdersStore struct {
	existing map[string]domain.Order
	created  []struct {
		total     float64
		userID    *int
		invoiceID string
	}
	failOnID string
}

func (m *mockOrdersStore) FindByStripeInvoiceID(_ context.Context, id string) (*domain.Order, error) {
	if o, ok := m.existing[id]; ok {
		return &o, nil
	}
	return nil, nil
}

func (m *mockOrdersStore) Create(_ context.Context, total float64, userID *int, invoiceID *string, _ []string) (domain.OrderWithRelations, error) {
	if invoiceID != nil && *invoiceID == m.failOnID {
		return domain.OrderWithRelations{}, errors.New("insert failed")
	}
	id := ""
	if invoiceID != nil {
		id = *invoiceID
	}
	m.created = append(m.created, struct {
		total     float64
		userID    *int
		invoiceID string
	}{total: total, userID: userID, invoiceID: id})
	return domain.OrderWithRelations{}, nil
}

type mockUsersStore struct {
	byEmail map[string]domain.User
}

func (m *mockUsersStore) FindByEmail(_ context.Context, email string) (domain.User, error) {
	if u, ok := m.byEmail[email]; ok {
		return u, nil
	}
	return domain.User{}, pgx.ErrNoRows
}

type mockInvoiceLister struct {
	invoices []stripe.Invoice
	err      error
}

func (m *mockInvoiceLister) ListPaidInvoicesSince(context.Context, int64) ([]stripe.Invoice, error) {
	return m.invoices, m.err
}

func TestSyncService_CronSyncStripeOrders(t *testing.T) {
	ordersStore := &mockOrdersStore{
		existing: map[string]domain.Order{
			"in_existing": {ID: 1},
		},
		failOnID: "in_fail",
	}
	usersStore := &mockUsersStore{
		byEmail: map[string]domain.User{
			"alice@example.com": {ID: 7, Email: "alice@example.com"},
		},
	}
	lister := &mockInvoiceLister{
		invoices: []stripe.Invoice{
			{ID: "in_existing", Total: 1000, CustomerEmail: "alice@example.com"},
			{ID: "in_new_user", Total: 2550, CustomerEmail: "alice@example.com"},
			{ID: "in_fail", Total: 500, CustomerEmail: "nobody@example.com"},
		},
	}

	svc, err := NewSyncService(ordersStore, usersStore, lister, "5 00 * * *", "America/New_York", slog.Default())
	if err != nil {
		t.Fatal(err)
	}

	res, err := svc.CronSyncStripeOrders(context.Background(), time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}

	if res.Processed != 3 || res.Created != 1 || res.Skipped != 1 || res.Failed != 1 {
		t.Fatalf("unexpected sync result: %+v", res)
	}
	if len(ordersStore.created) != 1 {
		t.Fatalf("expected 1 created order, got %d", len(ordersStore.created))
	}
	c := ordersStore.created[0]
	if c.total != 25.5 || c.userID == nil || *c.userID != 7 || c.invoiceID != "in_new_user" {
		t.Fatalf("unexpected created order: %+v", c)
	}
}

func TestJobsHandler_TriggerStripeSync(t *testing.T) {
	ordersStore := &mockOrdersStore{}
	usersStore := &mockUsersStore{}
	lister := &mockInvoiceLister{invoices: []stripe.Invoice{}}

	svc, err := NewSyncService(ordersStore, usersStore, lister, "5 00 * * *", "America/New_York", slog.Default())
	if err != nil {
		t.Fatal(err)
	}

	h := NewHandler(svc, slog.Default())
	r := h.Routes()

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/sync-stripe", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp TriggerSyncResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Message != "Stripe orders synchronization triggered successfully" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}
