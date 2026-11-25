package stripe

import (
	"strings"

	"github.com/aliaksandrsen/dashboard-server-go/internal/httputil"
)

type CreatePaymentRequest struct {
	PriceID  string `json:"priceId"`
	Quantity int    `json:"quantity"`
}

func (r *CreatePaymentRequest) Validate() error {
	var errs []string
	if strings.TrimSpace(r.PriceID) == "" {
		errs = append(errs, "priceId should not be empty")
	}
	if r.Quantity < 1 {
		errs = append(errs, "Quantity must be at least 1")
	}
	if len(errs) > 0 {
		return httputil.BadRequestMessages(errs)
	}
	return nil
}

type CreateProductParams struct {
	Name        string
	Price       float64
	Description *string
	Currency    string
}

type ProductResult struct {
	StripeProductID string `json:"stripeProductId"`
	StripePriceID   string `json:"stripePriceId"`
}

type CheckoutSessionResult struct {
	SessionID string `json:"sessionId"`
	URL       string `json:"url"`
}

type Invoice struct {
	ID            string
	Total         int64
	CustomerEmail string
}
