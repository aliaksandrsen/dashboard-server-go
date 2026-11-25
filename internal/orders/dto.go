package orders

import (
	"github.com/aliaksandrsen/dashboard-server-go/internal/httputil"
)

type CreateOrderRequest struct {
	Total           float64  `json:"total"`
	UserID          *int     `json:"userId"`
	StripeInvoiceID *string  `json:"stripeInvoiceId"`
	ProductIDs      []string `json:"productIds"`
}

func (r *CreateOrderRequest) Validate() error {
	var errs []string
	if r.Total <= 0 {
		errs = append(errs, "total must be a positive number")
	}
	if r.UserID != nil && *r.UserID <= 0 {
		errs = append(errs, "userId must be a positive number")
	}
	if len(errs) > 0 {
		return httputil.BadRequestMessages(errs)
	}
	return nil
}

type UpdateOrderRequest struct {
	Total           *float64                  `json:"total"`
	UserID          httputil.Optional[int]    `json:"userId"`
	StripeInvoiceID httputil.Optional[string] `json:"stripeInvoiceId"`
	ProductIDs      *[]string                 `json:"productIds"`
}

func (r *UpdateOrderRequest) Validate() error {
	var errs []string
	if r.Total != nil && *r.Total <= 0 {
		errs = append(errs, "total must be a positive number")
	}
	if r.UserID.Set && !r.UserID.Null && r.UserID.Value <= 0 {
		errs = append(errs, "userId must be a positive number")
	}
	if len(errs) > 0 {
		return httputil.BadRequestMessages(errs)
	}
	return nil
}

type OrderQuery struct {
	Skip   *int
	Take   *int
	UserID *int
}

func (q *OrderQuery) Validate() error {
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
	if q.UserID != nil && *q.UserID <= 0 {
		errs = append(errs, "userId must be a positive number")
	}
	if len(errs) > 0 {
		return httputil.BadRequestMessages(errs)
	}
	return nil
}
