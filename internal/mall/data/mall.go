package data

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"bilibili-lite/internal/mall/biz"

	kerrors "github.com/go-kratos/kratos/v3/errors"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type mallRepo struct {
	d *Data
}

// NewMallRepo uses the mall-owned clients for order and inventory persistence.
func NewMallRepo(d *Data) biz.MallRepo {
	return &mallRepo{d: d}
}

func (r *mallRepo) GetProduct(ctx context.Context, id string) (*biz.Product, error) {
	return getProduct(r.d.db.WithContext(ctx), id)
}

func (r *mallRepo) GetOrder(ctx context.Context, buyerID uint64, id string) (*biz.Order, error) {
	var order orderRow
	err := r.d.db.WithContext(ctx).First(&order, "id=? AND buyer_id=?", id, buyerID).Error
	if err != nil {
		return nil, storage(err)
	}
	return toBiz(order), nil
}

// PendingPayments rotates reconciliation order so repeated failures do not starve later orders.
func (r *mallRepo) PendingPayments(ctx context.Context, limit int) ([]biz.Order, error) {
	var rows []orderRow
	err := r.d.db.WithContext(ctx).Where("payment_state='processing'").Order("reconciled_at").Limit(limit).Find(&rows).Error
	out := make([]biz.Order, 0, len(rows))
	for _, order := range rows {
		out = append(out, *toBiz(order))
		if err == nil {
			err = r.d.db.WithContext(ctx).Exec("UPDATE orders SET reconciled_at=clock_timestamp() WHERE id=?", order.ID).Error
		}
	}
	return out, storage(err)
}

func (r *mallRepo) PendingRefunds(ctx context.Context, limit int) ([]biz.Order, error) {
	var rows []orderRow
	err := r.d.db.WithContext(ctx).Where("payment_state='refund_pending'").Order("reconciled_at").Limit(limit).Find(&rows).Error
	out := make([]biz.Order, 0, len(rows))
	for _, order := range rows {
		out = append(out, *toBiz(order))
		if err == nil {
			err = r.d.db.WithContext(ctx).Exec("UPDATE orders SET reconciled_at=clock_timestamp() WHERE id=?", order.ID).Error
		}
	}
	return out, storage(err)
}

func (r *mallRepo) CreateProduct(ctx context.Context, product biz.Product) (*biz.Product, error) {
	product.ID = uuid.NewString()
	err := r.d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&productRow{
			ID:           product.ID,
			Name:         product.Name,
			SaleStartsAt: product.SaleStartsAt,
			Description:  product.Description,
			Year:         product.Year,
		}).Error; err != nil {
			return err
		}
		for _, ticket := range product.TicketTypes {
			ticket.ID = uuid.NewString()
			if err := tx.Create(&ticketRow{
				ID:         ticket.ID,
				ProductID:  product.ID,
				Name:       ticket.Name,
				PriceCents: ticket.PriceCents,
				Available:  ticket.Available,
			}).Error; err != nil {
				return err
			}
			if err := event(tx, ticket.ID, "initial:"+ticket.ID, "initial", ticket.Available, 0, 0, 0, ""); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, storage(err)
	}
	return r.GetProduct(ctx, product.ID)
}

func (r *mallRepo) AdjustInventory(ctx context.Context, adjustment biz.InventoryAdjustment) (*biz.Product, error) {
	err := r.d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Serialize retries of the same adjustment, even when their arguments differ.
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", "adjust:"+adjustment.RequestID).Error; err != nil {
			return err
		}
		var previous struct {
			TicketTypeID   string
			ProductID      string
			Kind           string
			Reason         string
			AvailableDelta int64
		}
		previousResult := tx.Raw(`
SELECT e.ticket_type_id, t.product_id, e.kind, e.reason, e.available_delta
FROM inventory_events e
JOIN ticket_types t ON t.id=e.ticket_type_id
WHERE e.operation_id=?
`, "adjust:"+adjustment.RequestID).Scan(&previous)
		if previousResult.Error != nil {
			return previousResult.Error
		}
		kind := "adjust"
		if adjustment.ReleaseWithheld {
			kind = "release"
		}
		if previousResult.RowsAffected > 0 {
			if previous.ProductID != adjustment.ProductID ||
				previous.TicketTypeID != adjustment.TicketTypeID ||
				previous.Kind != kind ||
				previous.AvailableDelta != adjustment.Delta ||
				previous.Reason != adjustment.Reason {
				return biz.ErrIdempotency
			}
			return nil
		}
		var result *gorm.DB
		withheldDelta := int64(0)
		if adjustment.ReleaseWithheld {
			withheldDelta = -adjustment.Delta
			result = tx.Exec(`
UPDATE ticket_types
SET available=available+?, withheld=withheld-?
WHERE id=? AND product_id=? AND withheld>=?
`, adjustment.Delta, adjustment.Delta, adjustment.TicketTypeID, adjustment.ProductID, adjustment.Delta)
		} else {
			result = tx.Exec(`
UPDATE ticket_types
SET available=available+?
WHERE id=? AND product_id=? AND available+?>=0
`, adjustment.Delta, adjustment.TicketTypeID, adjustment.ProductID, adjustment.Delta)
		}
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return biz.ErrInvalid
		}
		return event(tx, adjustment.TicketTypeID, "adjust:"+adjustment.RequestID, kind, adjustment.Delta, 0, 0, withheldDelta, adjustment.Reason)
	})
	if err != nil {
		return nil, storage(err)
	}
	return r.GetProduct(ctx, adjustment.ProductID)
}

// CreateOrder reserves stock, occupies eligibility and inserts the order in one transaction.
func (r *mallRepo) CreateOrder(ctx context.Context, input biz.OrderRequest) (*biz.Order, error) {
	var out orderRow
	err := r.d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Serialize the same idempotency key even before an order row exists.
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", fmtRequest(input.BuyerID, input.RequestID)).Error; err != nil {
			return err
		}
		err := tx.First(&out, "buyer_id=? AND request_id=?", input.BuyerID, input.RequestID).Error
		if err == nil {
			if out.ProductID != input.ProductID || out.TicketTypeID != input.TicketTypeID {
				return biz.ErrIdempotency
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		// Use database time so sale eligibility does not depend on individual application clocks.
		var product productRow
		if err = tx.First(&product, "id=?", input.ProductID).Error; err != nil {
			return err
		}
		var now time.Time
		if err = tx.Raw("SELECT clock_timestamp()").Scan(&now).Error; err != nil {
			return err
		}
		if now.Before(product.SaleStartsAt) {
			return biz.ErrNotOnSale
		}
		if err = tx.Exec(`
INSERT INTO purchase_claims(product_id, buyer_id)
VALUES (?, ?)
ON CONFLICT DO NOTHING
`, input.ProductID, input.BuyerID).Error; err != nil {
			return err
		}

		// The account + product lock excludes concurrent purchases across different ticket tiers.
		var claim claimRow
		if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&claim, "product_id=? AND buyer_id=?", input.ProductID, input.BuyerID).Error; err != nil {
			return err
		}
		if claim.Permanent || claim.OrderID != nil {
			return biz.ErrLimit
		}
		var ticket ticketRow
		if err = tx.First(&ticket, "id=? AND product_id=?", input.TicketTypeID, input.ProductID).Error; err != nil {
			return err
		}

		// Conditional decrement is the final stock check; a precheck cannot prevent overselling.
		result := tx.Exec(`
UPDATE ticket_types
SET available=available-1, reserved=reserved+1
WHERE id=? AND available>0
`, ticket.ID)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return biz.ErrSoldOut
		}
		if err = tx.Raw("SELECT clock_timestamp()").Scan(&now).Error; err != nil {
			return err
		}

		// Snapshot price and names; later catalogue edits must not change an existing order.
		out = orderRow{
			ID:             uuid.NewString(),
			ProductID:      product.ID,
			TicketTypeID:   ticket.ID,
			BuyerID:        input.BuyerID,
			RequestID:      input.RequestID,
			State:          "pending",
			PriceCents:     ticket.PriceCents,
			ProductName:    product.Name,
			TicketTypeName: ticket.Name,
			ExpiresAt:      now.Add(biz.HoldDuration),
			PaymentState:   "unpaid",
		}
		if err = tx.Create(&out).Error; err != nil {
			return err
		}
		if err = tx.Exec("UPDATE purchase_claims SET order_id=? WHERE product_id=? AND buyer_id=?", out.ID, product.ID, input.BuyerID).Error; err != nil {
			return err
		}
		return event(tx, ticket.ID, "reserve:"+out.ID, "reserve", -1, 1, 0, 0, "")
	})
	if err != nil {
		return nil, storage(err)
	}
	return toBiz(out), nil
}

// BeginPayment records recoverable intent and rechecks expiry while holding the order lock.
func (r *mallRepo) BeginPayment(ctx context.Context, buyerID uint64, id string) (*biz.Order, error) {
	var out *orderRow
	err := r.d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		order, err := lockOrder(tx, id, buyerID)
		if err != nil {
			return err
		}
		out = order
		if order.State == "paid" {
			return nil
		}
		var now time.Time
		if err = tx.Raw("SELECT clock_timestamp()").Scan(&now).Error; err != nil {
			return err
		}
		if order.State != "pending" || !now.Before(order.ExpiresAt) {
			return biz.ErrState
		}
		order.PaymentState = "processing"
		return tx.Save(order).Error
	})
	if err != nil {
		return nil, storage(err)
	}
	return toBiz(*out), nil
}

// ApplyPayment deduplicates provider results and resolves payment racing with expiry.
func (r *mallRepo) ApplyPayment(ctx context.Context, buyerID uint64, id string, payment biz.PaymentResult) (*biz.Order, error) {
	if !payment.Paid || payment.Reference == "" {
		return nil, biz.ErrPayment
	}
	var out *orderRow
	err := r.d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		order, err := lockOrder(tx, id, buyerID)
		if err != nil {
			return err
		}
		out = order
		if order.PaymentReference != nil {
			if *order.PaymentReference != payment.Reference {
				return biz.ErrIdempotency
			}
			return nil
		}
		var now time.Time
		if err = tx.Raw("SELECT clock_timestamp()").Scan(&now).Error; err != nil {
			return err
		}
		if order.State == "pending" && !now.Before(order.ExpiresAt) {
			if err = closeOrder(tx, order); err != nil {
				return err
			}
		}
		switch order.State {
		case "pending":
			if err = tx.Exec("UPDATE ticket_types SET reserved=reserved-1,sold=sold+1 WHERE id=?", order.TicketTypeID).Error; err != nil {
				return err
			}
			if err = event(tx, order.TicketTypeID, "pay:"+id, "pay", 0, -1, 1, 0, ""); err != nil {
				return err
			}
			if err = tx.Exec(`
UPDATE purchase_claims
SET permanent=true
WHERE product_id=? AND buyer_id=?
`, order.ProductID, order.BuyerID).Error; err != nil {
				return err
			}
			order.State = "paid"
			order.PaymentState = "paid"
		// Expired stock may already belong to another buyer; compensate instead of reopening the order.
		case "closed":
			order.PaymentState = "refund_pending"
		default:
			return biz.ErrState
		}
		order.PaymentReference = &payment.Reference
		return tx.Save(order).Error
	})
	if err != nil {
		return nil, storage(err)
	}
	return toBiz(*out), nil
}

func (r *mallRepo) FinishUnpaid(ctx context.Context, id string) error {
	return storage(r.d.db.WithContext(ctx).Exec(`
UPDATE orders
SET payment_state='unpaid'
WHERE id=? AND state='closed' AND payment_state='processing'
`, id).Error)
}

func (r *mallRepo) RequestRefund(ctx context.Context, buyerID uint64, id string) (*biz.Order, error) {
	var out *orderRow
	err := r.d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		order, err := lockOrder(tx, id, buyerID)
		if err != nil {
			return err
		}
		out = order
		if order.State == "refunded" || order.State == "refunding" {
			return nil
		}
		if order.State != "paid" {
			return biz.ErrState
		}
		order.State = "refunding"
		order.PaymentState = "refund_pending"
		return tx.Save(order).Error
	})
	if err != nil {
		return nil, storage(err)
	}
	return toBiz(*out), nil
}

// CompleteRefund moves voluntarily refunded tickets to withheld stock and retains purchase eligibility usage.
func (r *mallRepo) CompleteRefund(ctx context.Context, id string) error {
	return storage(r.d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		order, err := lockOrder(tx, id, 0)
		if err != nil {
			return err
		}
		if order.PaymentState == "refunded" {
			return nil
		}
		if order.PaymentState != "refund_pending" {
			return biz.ErrState
		}
		if order.State == "refunding" {
			if err = tx.Exec("UPDATE ticket_types SET sold=sold-1,withheld=withheld+1 WHERE id=?", order.TicketTypeID).Error; err != nil {
				return err
			}
			if err = event(tx, order.TicketTypeID, "refund:"+id, "refund", 0, 0, -1, 1, ""); err != nil {
				return err
			}
			order.State = "refunded"
		}
		order.PaymentState = "refunded"
		return tx.Save(order).Error
	}))
}

// ExpireOrders reads a bounded candidate set, then rechecks each order under the same locks as payment.
func (r *mallRepo) ExpireOrders(ctx context.Context, limit int) (int, error) {
	var ids []string
	if err := r.d.db.WithContext(ctx).
		Model(&orderRow{}).
		Where("state='pending' AND expires_at<=clock_timestamp()").
		Order("expires_at").
		Limit(limit).
		Pluck("id", &ids).Error; err != nil {
		return 0, storage(err)
	}
	count := 0
	for _, id := range ids {
		err := r.d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			order, err := lockOrder(tx, id, 0)
			if err != nil {
				return err
			}
			var now time.Time
			if err = tx.Raw("SELECT clock_timestamp()").Scan(&now).Error; err != nil {
				return err
			}
			if order.State != "pending" || now.Before(order.ExpiresAt) {
				return nil
			}
			if err = closeOrder(tx, order); err != nil {
				return err
			}
			count++
			return nil
		})
		if err != nil {
			return count, storage(err)
		}
	}
	return count, nil
}

func getProduct(db *gorm.DB, id string) (*biz.Product, error) {
	var product productRow
	if err := db.First(&product, "id=?", id).Error; err != nil {
		return nil, storage(err)
	}
	var rows []ticketRow
	if err := db.Where("product_id=?", id).Order("id").Find(&rows).Error; err != nil {
		return nil, storage(err)
	}
	out := &biz.Product{
		ID:           product.ID,
		Name:         product.Name,
		SaleStartsAt: product.SaleStartsAt,
		Description:  product.Description,
		Year:         product.Year,
	}
	for _, ticket := range rows {
		out.TicketTypes = append(out.TicketTypes, biz.TicketType{
			ID:         ticket.ID,
			ProductID:  ticket.ProductID,
			Name:       ticket.Name,
			PriceCents: ticket.PriceCents,
			Available:  ticket.Available,
			Reserved:   ticket.Reserved,
			Sold:       ticket.Sold,
			Withheld:   ticket.Withheld,
		})
	}
	return out, nil
}

// Every order mutation locks claim -> order -> inventory, matching CreateOrder.
func lockOrder(tx *gorm.DB, id string, buyerID uint64) (*orderRow, error) {
	var order orderRow
	q := tx.Where("id=?", id)
	if buyerID != 0 {
		q = q.Where("buyer_id=?", buyerID)
	}
	if err := q.First(&order).Error; err != nil {
		return nil, err
	}
	var claim claimRow
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&claim, "product_id=? AND buyer_id=?", order.ProductID, order.BuyerID).Error; err != nil {
		return nil, err
	}
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&order, "id=?", id).Error; err != nil {
		return nil, err
	}
	return &order, nil
}

// closeOrder requires a locked pending order. Only its temporary claim may be cleared.
func closeOrder(tx *gorm.DB, order *orderRow) error {
	if err := tx.Exec(`
UPDATE ticket_types
SET reserved=reserved-1, available=available+1
WHERE id=?
`, order.TicketTypeID).Error; err != nil {
		return err
	}
	if err := tx.Exec(`
UPDATE purchase_claims
SET order_id=NULL
WHERE product_id=? AND buyer_id=? AND order_id=? AND NOT permanent
`, order.ProductID, order.BuyerID, order.ID).Error; err != nil {
		return err
	}
	order.State = "closed"
	if err := tx.Save(order).Error; err != nil {
		return err
	}
	return event(tx, order.TicketTypeID, "expire:"+order.ID, "expire", 1, -1, 0, 0, "")
}

// event writes inventory deltas in the same transaction as the corresponding stock change.
func event(tx *gorm.DB, ticketTypeID, operationID, kind string, availableDelta, reservedDelta, soldDelta, withheldDelta int64, reason string) error {
	return tx.Exec(`
INSERT INTO inventory_events(ticket_type_id, operation_id, kind, available_delta, reserved_delta, sold_delta, withheld_delta, reason)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
`, ticketTypeID, operationID, kind, availableDelta, reservedDelta, soldDelta, withheldDelta, reason).Error
}

func fmtRequest(buyerID uint64, id string) string {
	return fmt.Sprintf("order:%d:%s", buyerID, id)
}

func storage(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return biz.ErrNotFound
	}
	var ke *kerrors.Error
	if errors.As(err, &ke) {
		return err
	}
	slog.Error("mall database operation failed", "error", err)
	return biz.ErrStorage
}
