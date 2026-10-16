package data

import (
	"context"

	"bilibili-lite/internal/mall/biz"
	"bilibili-lite/internal/mall/payment"

	"github.com/google/uuid"
)

type simulatedStore struct {
	d *Data
}

func NewSimulatedStore(d *Data) payment.Store {
	return &simulatedStore{d: d}
}

// Charge commits independently from order transactions, allowing recovery tests to model a provider success.
func (s *simulatedStore) Charge(ctx context.Context, id string, amount int64) (biz.PaymentResult, error) {
	if err := s.d.db.WithContext(ctx).Exec(`
INSERT INTO simulated_payments(order_id, reference, amount)
VALUES (?, ?, ?)
ON CONFLICT DO NOTHING
`, id, uuid.NewString(), amount).Error; err != nil {
		return biz.PaymentResult{}, err
	}
	return s.Lookup(ctx, id)
}

func (s *simulatedStore) Lookup(ctx context.Context, id string) (biz.PaymentResult, error) {
	var p struct {
		Reference string
		Closed    bool
	}
	q := s.d.db.WithContext(ctx).Raw("SELECT reference,closed FROM simulated_payments WHERE order_id=?", id).Scan(&p)
	return biz.PaymentResult{Reference: p.Reference, Paid: q.RowsAffected == 1 && !p.Closed}, q.Error
}

func (s *simulatedStore) Refund(ctx context.Context, id string) error {
	q := s.d.db.WithContext(ctx).Exec(`
UPDATE simulated_payments
SET refunded=true
WHERE order_id=? AND NOT closed
`, id)
	if q.Error != nil {
		return q.Error
	}
	if q.RowsAffected != 1 {
		return biz.ErrPayment
	}
	return nil
}

// Close inserts a fence only when no charge exists. A delayed Charge cannot overwrite this row.
func (s *simulatedStore) Close(ctx context.Context, id string) error {
	return s.d.db.WithContext(ctx).Exec(`
INSERT INTO simulated_payments(order_id, reference, amount, closed)
VALUES (?, ?, 0, true)
ON CONFLICT DO NOTHING
`, id, uuid.NewString()).Error
}
