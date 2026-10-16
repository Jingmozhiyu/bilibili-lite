package data

import (
	"time"

	"bilibili-lite/internal/mall/biz"
)

type productRow struct {
	ID           string
	Name         string
	Description  string
	Year         int32
	SaleStartsAt time.Time
}

func (productRow) TableName() string {
	return "products"
}

type ticketRow struct {
	ID         string
	ProductID  string
	Name       string
	PriceCents int64
	Available  int64
	Reserved   int64
	Sold       int64
	Withheld   int64
}

func (ticketRow) TableName() string {
	return "ticket_types"
}

type orderRow struct {
	ID               string
	ProductID        string
	TicketTypeID     string
	RequestID        string
	State            string
	ProductName      string
	TicketTypeName   string
	PaymentState     string
	PaymentReference *string
	BuyerID          uint64
	PriceCents       int64
	ExpiresAt        time.Time
}

func (orderRow) TableName() string {
	return "orders"
}

// claimRow serializes one account's purchases for an event, regardless of ticket tier.
// Permanent survives voluntary refunds; OrderID can be cleared after an unpaid hold expires.
type claimRow struct {
	ProductID string
	BuyerID   uint64
	OrderID   *string
	Permanent bool
}

func (claimRow) TableName() string {
	return "purchase_claims"
}

func toBiz(order orderRow) *biz.Order {
	return &biz.Order{
		ID:             order.ID,
		ProductID:      order.ProductID,
		TicketTypeID:   order.TicketTypeID,
		BuyerID:        order.BuyerID,
		State:          order.State,
		PriceCents:     order.PriceCents,
		ProductName:    order.ProductName,
		TicketTypeName: order.TicketTypeName,
		ExpiresAt:      order.ExpiresAt,
		PaymentState:   order.PaymentState,
	}
}
