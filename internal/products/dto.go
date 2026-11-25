package products

import (
	"strings"

	"github.com/aliaksandrsen/dashboard-server-go/internal/httputil"
)

type CreateProductRequest struct {
	Name            string  `json:"name"`
	Price           float64 `json:"price"`
	Description     *string `json:"description"`
	StripeProductID *string `json:"stripeProductId"`
	StripePriceID   *string `json:"stripePriceId"`
}

func (r *CreateProductRequest) Validate() error {
	var errs []string
	if strings.TrimSpace(r.Name) == "" {
		errs = append(errs, "name should not be empty")
	}
	if r.Price <= 0 {
		errs = append(errs, "price must be a positive number")
	}
	if len(errs) > 0 {
		return httputil.BadRequestMessages(errs)
	}
	return nil
}

type UpdateProductRequest struct {
	Name            *string                   `json:"name"`
	Price           *float64                  `json:"price"`
	Description     *string                   `json:"description"`
	StripeProductID httputil.Optional[string] `json:"stripeProductId"`
	StripePriceID   httputil.Optional[string] `json:"stripePriceId"`
}

func (r *UpdateProductRequest) Validate() error {
	var errs []string
	if r.Name != nil && strings.TrimSpace(*r.Name) == "" {
		errs = append(errs, "name should not be empty")
	}
	if r.Price != nil && *r.Price <= 0 {
		errs = append(errs, "price must be a positive number")
	}
	if len(errs) > 0 {
		return httputil.BadRequestMessages(errs)
	}
	return nil
}

type ProductQuery struct {
	Skip   *int
	Take   *int
	Search *string
}

func (q *ProductQuery) Validate() error {
	var errs []string
	if q.Skip != nil && *q.Skip < 0 {
		errs = append(errs, "skip must not be less than 0")
	}
	if q.Take != nil {
		if *q.Take < 1 {
			errs = append(errs, "take must be a positive number")
		} else if *q.Take > 100 {
			errs = append(errs, "take must not be greater than 100")
		}
	}
	if len(errs) > 0 {
		return httputil.BadRequestMessages(errs)
	}
	return nil
}

func (q *ProductQuery) HasFilters() bool {
	if q.Skip != nil || q.Take != nil {
		return true
	}
	return q.Search != nil && strings.TrimSpace(*q.Search) != ""
}
