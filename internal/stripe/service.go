package stripe

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"

	"github.com/aliaksandrsen/dashboard-server-go/internal/httputil"
	"github.com/aliaksandrsen/dashboard-server-go/internal/platform/origin"
	stripego "github.com/stripe/stripe-go/v82"
)

// Client abstracts Stripe API calls so the service can be tested cleanly without network access.
type Client interface {
	CreateProduct(ctx context.Context, params *stripego.ProductCreateParams) (*stripego.Product, error)
	UpdateProduct(ctx context.Context, id string, params *stripego.ProductUpdateParams) (*stripego.Product, error)
	CreatePrice(ctx context.Context, params *stripego.PriceCreateParams) (*stripego.Price, error)
	CreateCheckoutSession(ctx context.Context, params *stripego.CheckoutSessionCreateParams) (*stripego.CheckoutSession, error)
	ListInvoices(ctx context.Context, params *stripego.InvoiceListParams) ([]Invoice, error)
}

type sdkClient struct {
	client *stripego.Client
}

func NewSDKClient(secretKey string) Client {
	return &sdkClient{
		client: stripego.NewClient(secretKey),
	}
}

func (c *sdkClient) CreateProduct(ctx context.Context, params *stripego.ProductCreateParams) (*stripego.Product, error) {
	return c.client.V1Products.Create(ctx, params)
}

func (c *sdkClient) UpdateProduct(ctx context.Context, id string, params *stripego.ProductUpdateParams) (*stripego.Product, error) {
	return c.client.V1Products.Update(ctx, id, params)
}

func (c *sdkClient) CreatePrice(ctx context.Context, params *stripego.PriceCreateParams) (*stripego.Price, error) {
	return c.client.V1Prices.Create(ctx, params)
}

func (c *sdkClient) CreateCheckoutSession(ctx context.Context, params *stripego.CheckoutSessionCreateParams) (*stripego.CheckoutSession, error) {
	return c.client.V1CheckoutSessions.Create(ctx, params)
}

func (c *sdkClient) ListInvoices(ctx context.Context, params *stripego.InvoiceListParams) ([]Invoice, error) {
	invoices := make([]Invoice, 0)
	for inv, err := range c.client.V1Invoices.List(ctx, params) {
		if err != nil {
			return nil, err
		}
		invoices = append(invoices, Invoice{
			ID:            inv.ID,
			Total:         inv.Total,
			CustomerEmail: inv.CustomerEmail,
		})
	}
	return invoices, nil
}

// Manager defines the Stripe operations used by Products, Stripe HTTP handler, and Cron sync.
type Manager interface {
	CreateProduct(ctx context.Context, params CreateProductParams) (ProductResult, error)
	DeactivateProduct(ctx context.Context, stripeProductID string)
	CreatePayment(ctx context.Context, req CreatePaymentRequest, originHeader string) (CheckoutSessionResult, error)
	ListPaidInvoicesSince(ctx context.Context, sinceUnix int64) ([]Invoice, error)
}

type Service struct {
	client      Client
	frontendURL string
	log         *slog.Logger
}

func NewService(secretKey, nodeEnv, frontendURL string, log *slog.Logger) *Service {
	if secretKey == "" {
		if nodeEnv != "production" && log != nil {
			log.Warn("STRIPE_SECRET_KEY is not configured; Stripe API requests will fail unless mocked.")
		}
		secretKey = "sk_test_placeholder"
	}

	return &Service{
		client:      NewSDKClient(secretKey),
		frontendURL: frontendURL,
		log:         log,
	}
}

func NewServiceWithClient(client Client, frontendURL string, log *slog.Logger) *Service {
	return &Service{
		client:      client,
		frontendURL: frontendURL,
		log:         log,
	}
}

func (s *Service) CreateProduct(ctx context.Context, params CreateProductParams) (ProductResult, error) {
	currency := params.Currency
	if currency == "" {
		currency = "usd"
	}

	if s.log != nil {
		s.log.Debug("Started product creation in Stripe", "name", params.Name, "price", params.Price)
	}

	productParams := &stripego.ProductCreateParams{
		Name: stripego.String(params.Name),
	}
	if params.Description != nil && *params.Description != "" {
		productParams.Description = stripego.String(*params.Description)
	}

	prodResp, err := s.client.CreateProduct(ctx, productParams)
	if err != nil {
		if s.log != nil {
			s.log.Error("Stripe product creation failed", "error", err)
		}
		return ProductResult{}, httputil.InternalServerError(fmt.Sprintf("Stripe service error: %s", err.Error()), err)
	}

	unitAmount := int64(math.Round(params.Price * 100))
	if s.log != nil {
		s.log.Info("Creating Stripe price", "productId", prodResp.ID, "unitAmount", unitAmount, "currency", currency)
	}

	priceResp, err := s.client.CreatePrice(ctx, &stripego.PriceCreateParams{
		Product:    stripego.String(prodResp.ID),
		UnitAmount: stripego.Int64(unitAmount),
		Currency:   stripego.String(currency),
	})
	if err != nil {
		if s.log != nil {
			s.log.Error("Stripe price creation failed", "error", err)
		}
		return ProductResult{}, httputil.InternalServerError(fmt.Sprintf("Stripe service error: %s", err.Error()), err)
	}

	return ProductResult{
		StripeProductID: prodResp.ID,
		StripePriceID:   priceResp.ID,
	}, nil
}

func (s *Service) DeactivateProduct(ctx context.Context, stripeProductID string) {
	if s.log != nil {
		s.log.Warn("Deactivating product in Stripe", "stripeProductId", stripeProductID)
	}

	_, err := s.client.UpdateProduct(ctx, stripeProductID, &stripego.ProductUpdateParams{
		Active: stripego.Bool(false),
	})
	if err != nil {
		if s.log != nil {
			s.log.Error("Failed to deactivate product in Stripe", "stripeProductId", stripeProductID, "error", err)
		}
		return
	}

	if s.log != nil {
		s.log.Info("Successfully deactivated product in Stripe", "stripeProductId", stripeProductID)
	}
}

func (s *Service) CreatePayment(ctx context.Context, req CreatePaymentRequest, originHeader string) (CheckoutSessionResult, error) {
	if s.log != nil {
		s.log.Info("Starting Stripe payment")
	}

	allowed := origin.GetAllowedOrigins(s.frontendURL)
	resolvedOrigin, err := origin.ResolveAllowedOrigin(originHeader, allowed)
	if err != nil {
		return CheckoutSessionResult{}, httputil.BadRequest(err.Error())
	}

	session, err := s.client.CreateCheckoutSession(ctx, &stripego.CheckoutSessionCreateParams{
		LineItems: []*stripego.CheckoutSessionCreateLineItemParams{
			{
				Price:    stripego.String(req.PriceID),
				Quantity: stripego.Int64(int64(req.Quantity)),
			},
		},
		Mode:       stripego.String(string(stripego.CheckoutSessionModePayment)),
		SuccessURL: stripego.String(resolvedOrigin + "/?success=true"),
		CancelURL:  stripego.String(resolvedOrigin + "/?canceled=true"),
	})
	if err != nil {
		var stripeErr *stripego.Error
		if errors.As(err, &stripeErr) {
			if stripeErr.Type == stripego.ErrorTypeInvalidRequest || stripeErr.Type == stripego.ErrorTypeCard {
				return CheckoutSessionResult{}, httputil.BadRequest(stripeErr.Msg)
			}
		}
		if s.log != nil {
			s.log.Error("Error creating Stripe payment", "error", err)
		}
		return CheckoutSessionResult{}, httputil.BadGateway("Failed to create Stripe payment session", err)
	}

	if session == nil || session.URL == "" {
		return CheckoutSessionResult{}, httputil.BadGateway("Failed to retrieve Stripe session URL", nil)
	}

	return CheckoutSessionResult{
		SessionID: session.ID,
		URL:       session.URL,
	}, nil
}

func (s *Service) ListPaidInvoicesSince(ctx context.Context, sinceUnix int64) ([]Invoice, error) {
	params := &stripego.InvoiceListParams{
		Status: stripego.String("paid"),
		CreatedRange: &stripego.RangeQueryParams{
			GreaterThanOrEqual: sinceUnix,
		},
	}
	params.Limit = stripego.Int64(100)

	invoices, err := s.client.ListInvoices(ctx, params)
	if err != nil {
		if s.log != nil {
			s.log.Error("Failed to fetch invoices from Stripe", "error", err)
		}
		return nil, err
	}
	return invoices, nil
}
