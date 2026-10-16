package payment

import (
	"context"

	"bilibili-lite/internal/mall/biz"
)

// Store is a durable mock-provider ledger, separate from order transactions.
// A real provider adapter implements biz.PaymentGateway without using this store.
type Store interface {
	Charge(context.Context, string, int64) (biz.PaymentResult, error)
	Close(context.Context, string) error
	Lookup(context.Context, string) (biz.PaymentResult, error)
	Refund(context.Context, string) error
}

type Simulated struct {
	store Store
}

func NewSimulated(s Store) biz.PaymentGateway {
	return &Simulated{store: s}
}

func (s *Simulated) Pay(ctx context.Context, order biz.Order) (biz.PaymentResult, error) {
	return s.store.Charge(ctx, order.ID, order.PriceCents)
}

func (s *Simulated) Query(ctx context.Context, order biz.Order) (biz.PaymentResult, error) {
	return s.store.Lookup(ctx, order.ID)
}

func (s *Simulated) Refund(ctx context.Context, order biz.Order) error {
	return s.store.Refund(ctx, order.ID)
}

func (s *Simulated) Close(ctx context.Context, order biz.Order) error {
	return s.store.Close(ctx, order.ID)
}
