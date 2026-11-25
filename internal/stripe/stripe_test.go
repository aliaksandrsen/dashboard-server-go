package stripe

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aliaksandrsen/dashboard-server-go/internal/httputil"
	stripego "github.com/stripe/stripe-go/v82"
)

type mockStripeClient struct {
	createProductFn         func(ctx context.Context, params *stripego.ProductCreateParams) (*stripego.Product, error)
	updateProductFn         func(ctx context.Context, id string, params *stripego.ProductUpdateParams) (*stripego.Product, error)
	createPriceFn           func(ctx context.Context, params *stripego.PriceCreateParams) (*stripego.Price, error)
	createCheckoutSessionFn func(ctx context.Context, params *stripego.CheckoutSessionCreateParams) (*stripego.CheckoutSession, error)
	listInvoicesFn          func(ctx context.Context, params *stripego.InvoiceListParams) ([]Invoice, error)
}

func (m *mockStripeClient) CreateProduct(ctx context.Context, p *stripego.ProductCreateParams) (*stripego.Product, error) {
	return m.createProductFn(ctx, p)
}
func (m *mockStripeClient) UpdateProduct(ctx context.Context, id string, p *stripego.ProductUpdateParams) (*stripego.Product, error) {
	return m.updateProductFn(ctx, id, p)
}
func (m *mockStripeClient) CreatePrice(ctx context.Context, p *stripego.PriceCreateParams) (*stripego.Price, error) {
	return m.createPriceFn(ctx, p)
}
func (m *mockStripeClient) CreateCheckoutSession(ctx context.Context, p *stripego.CheckoutSessionCreateParams) (*stripego.CheckoutSession, error) {
	return m.createCheckoutSessionFn(ctx, p)
}
func (m *mockStripeClient) ListInvoices(ctx context.Context, p *stripego.InvoiceListParams) ([]Invoice, error) {
	return m.listInvoicesFn(ctx, p)
}

func TestStripeService_CreateProduct(t *testing.T) {
	var capturedUnitAmount int64
	client := &mockStripeClient{
		createProductFn: func(_ context.Context, p *stripego.ProductCreateParams) (*stripego.Product, error) {
			return &stripego.Product{ID: "prod_123"}, nil
		},
		createPriceFn: func(_ context.Context, p *stripego.PriceCreateParams) (*stripego.Price, error) {
			capturedUnitAmount = *p.UnitAmount
			return &stripego.Price{ID: "price_456"}, nil
		},
	}

	svc := NewServiceWithClient(client, "", slog.Default())
	res, err := svc.CreateProduct(context.Background(), CreateProductParams{
		Name:  "Keyboard",
		Price: 49.99,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedUnitAmount != 4999 {
		t.Fatalf("expected 4999 cents, got %d", capturedUnitAmount)
	}
	if res.StripeProductID != "prod_123" || res.StripePriceID != "price_456" {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestStripeService_CreatePayment_OriginAndErrors(t *testing.T) {
	client := &mockStripeClient{
		createCheckoutSessionFn: func(_ context.Context, p *stripego.CheckoutSessionCreateParams) (*stripego.CheckoutSession, error) {
			if *p.LineItems[0].Price == "price_bad" {
				return nil, &stripego.Error{Type: stripego.ErrorTypeInvalidRequest, Msg: "No such price"}
			}
			if *p.LineItems[0].Price == "price_no_url" {
				return &stripego.CheckoutSession{ID: "cs_empty", URL: ""}, nil
			}
			return &stripego.CheckoutSession{ID: "cs_123", URL: "https://checkout.stripe.com/pay/cs_123"}, nil
		},
	}

	svc := NewServiceWithClient(client, "http://localhost:5173", slog.Default())

	// Disallowed origin -> 400
	_, err := svc.CreatePayment(context.Background(), CreatePaymentRequest{PriceID: "price_1", Quantity: 1}, "https://evil.com")
	var appErr *httputil.AppError
	if !errors.As(err, &appErr) || appErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for disallowed origin, got %v", err)
	}

	// Stripe invalid request -> 400
	_, err = svc.CreatePayment(context.Background(), CreatePaymentRequest{PriceID: "price_bad", Quantity: 1}, "http://localhost:5173")
	if !errors.As(err, &appErr) || appErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid stripe price, got %v", err)
	}

	// Missing URL -> 502
	_, err = svc.CreatePayment(context.Background(), CreatePaymentRequest{PriceID: "price_no_url", Quantity: 1}, "http://localhost:5173")
	if !errors.As(err, &appErr) || appErr.StatusCode != http.StatusBadGateway {
		t.Fatalf("expected 502 for empty session URL, got %v", err)
	}

	// Handler happy path -> 201
	h := NewHandler(svc, slog.Default())
	r := h.Routes()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/payments", strings.NewReader(`{"priceId":"price_1","quantity":2}`))
	req.Header.Set("Origin", "http://localhost:5173")
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d (%s)", rec.Code, rec.Body.String())
	}
	var out CheckoutSessionResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.SessionID != "cs_123" {
		t.Fatalf("unexpected response: %s, err=%v", rec.Body.String(), err)
	}
}
