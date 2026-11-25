package domain

import "time"

type User struct {
	ID      int    `json:"id"`
	Email   string `json:"email"`
	Name    string `json:"name"`
	Address string `json:"address"`
}

type UserWithOrders struct {
	User
	Orders []Order `json:"orders"`
}

type UserWithOrdersAndProducts struct {
	User
	Orders []OrderWithProducts `json:"orders"`
}

type Product struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Price           float64   `json:"price"`
	StripeProductID *string   `json:"stripeProductId"`
	StripePriceID   *string   `json:"stripePriceId"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type Order struct {
	ID              int       `json:"id"`
	Total           float64   `json:"total"`
	StripeInvoiceID *string   `json:"stripeInvoiceId"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
	UserID          *int      `json:"userId"`
}

type OrderWithProducts struct {
	Order
	Products []Product `json:"products"`
}

type OrderWithRelations struct {
	Order
	User     *User     `json:"user"`
	Products []Product `json:"products"`
}

type SyncResult struct {
	Processed int `json:"processed"`
	Created   int `json:"created"`
	Skipped   int `json:"skipped"`
	Failed    int `json:"failed"`
}
