package data

import (
	"context"
	"errors"
	"strings"
	"time"

	"bilibili-lite/internal/mall/biz"

	"gorm.io/gorm"
)

func (r *mallRepo) ListProducts(ctx context.Context, query string) ([]biz.Product, error) {
	if strings.EqualFold(strings.TrimSpace(query), "bw") {
		query = "bilibili world"
	}
	var rows []productRow
	// Small, bounded demo catalogue. No unbounded wildcard SQL or client-supplied order clause.
	q := r.d.db.WithContext(ctx).Order("year DESC, id").Limit(100)
	if query != "" {
		q = q.Where("strpos(lower(name),lower(?))>0", query)
	}
	if err := q.Find(&rows).Error; err != nil {
		return nil, storage(err)
	}
	out := make([]biz.Product, 0, len(rows))
	ids := make([]string, 0, len(rows))
	for _, product := range rows {
		ids = append(ids, product.ID)
		out = append(out, biz.Product{
			ID:           product.ID,
			Name:         product.Name,
			Description:  product.Description,
			Year:         product.Year,
			SaleStartsAt: product.SaleStartsAt,
		})
	}
	if len(ids) == 0 {
		return out, nil
	}
	var tickets []ticketRow
	if err := r.d.db.WithContext(ctx).Where("product_id IN ?", ids).Order("price_cents,id").Find(&tickets).Error; err != nil {
		return nil, storage(err)
	}
	positions := map[string]int{}
	for i, product := range out {
		positions[product.ID] = i
	}
	for _, ticket := range tickets {
		i := positions[ticket.ProductID]
		out[i].TicketTypes = append(out[i].TicketTypes, biz.TicketType{
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

// GetPurchase restores the current order and permanent purchase restriction when reopening the UI.
func (r *mallRepo) GetPurchase(ctx context.Context, buyerID uint64, id string) (*biz.Order, bool, error) {
	var claim claimRow
	err := r.d.db.WithContext(ctx).First(&claim, "product_id=? AND buyer_id=?", id, buyerID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, storage(err)
	}
	if claim.OrderID == nil {
		return nil, claim.Permanent, nil
	}
	order, err := r.GetOrder(ctx, buyerID, *claim.OrderID)
	return order, claim.Permanent, err
}

// Precheck avoids entering a write transaction for known invalid requests.
// Its snapshot takes no locks and grants no reservation; CreateOrder remains authoritative.
func (r *mallRepo) Precheck(ctx context.Context, input biz.OrderRequest) (*biz.Order, error) {
	// Return an earlier attempt before checking stock, including when that order has since closed.
	var prior orderRow
	err := r.d.db.WithContext(ctx).First(&prior, "buyer_id=? AND request_id=?", input.BuyerID, input.RequestID).Error
	if err == nil {
		if prior.ProductID != input.ProductID || prior.TicketTypeID != input.TicketTypeID {
			return nil, biz.ErrIdempotency
		}
		return toBiz(prior), nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, storage(err)
	}
	var snapshot struct {
		Available    int64
		SaleStartsAt time.Time
		Now          time.Time
		Permanent    bool
		OrderID      *string
	}
	result := r.d.db.WithContext(ctx).Raw(`
SELECT t.available,
       p.sale_starts_at,
       clock_timestamp() AS now,
       coalesce(c.permanent, false) AS permanent,
       c.order_id
FROM products p
JOIN ticket_types t ON t.product_id = p.id
LEFT JOIN purchase_claims c ON c.product_id = p.id AND c.buyer_id = ?
WHERE p.id = ? AND t.id = ?
`, input.BuyerID, input.ProductID, input.TicketTypeID).Scan(&snapshot)
	if result.Error != nil {
		return nil, storage(result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, biz.ErrInvalid
	}
	if snapshot.Permanent || snapshot.OrderID != nil {
		return nil, biz.ErrLimit
	}
	if snapshot.Now.Before(snapshot.SaleStartsAt) {
		return nil, biz.ErrNotOnSale
	}
	if snapshot.Available <= 0 {
		return nil, biz.ErrSoldOut
	}
	return nil, nil
}
