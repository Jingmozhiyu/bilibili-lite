package data

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"bilibili-lite/internal/mall/biz"
	"bilibili-lite/internal/mall/payment"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func testRepo(t *testing.T) (*mallRepo, *biz.MallUsecase) {
	t.Helper()
	dsn := os.Getenv("MALL_TEST_DATABASE_SOURCE")
	if dsn == "" {
		t.Skip("set MALL_TEST_DATABASE_SOURCE to run PostgreSQL integration tests")
	}
	db, e := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		t.Fatal(e)
	}
	schema := "test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if e = db.Exec("CREATE SCHEMA " + schema).Error; e != nil {
		t.Fatal(e)
	}
	scoped, e := gorm.Open(postgres.Open(dsn+" search_path="+schema), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		t.Fatal(e)
	}
	sql, _ := scoped.DB()
	sql.SetMaxOpenConns(12)
	t.Cleanup(func() {
		sql.Close()
		db.Exec("DROP SCHEMA " + schema + " CASCADE")
		raw, _ := db.DB()
		raw.Close()
	})
	if e = migrate(scoped); e != nil {
		t.Fatal(e)
	}
	d := &Data{db: scoped}
	r := &mallRepo{d: d}
	return r, biz.NewMallUsecase(r, payment.NewSimulated(NewSimulatedStore(d)), nil)
}

func product(t *testing.T, r *mallRepo, stock int64) *biz.Product {
	t.Helper()
	p, e := r.CreateProduct(context.Background(), biz.Product{
		Name:         "BW",
		SaleStartsAt: time.Now().Add(-time.Hour),
		TicketTypes: []biz.TicketType{{
			Name:       "normal",
			PriceCents: 12800,
			Available:  stock,
		}, {
			Name:       "vip",
			PriceCents: 58800,
			Available:  stock,
		}},
	})
	if e != nil {
		t.Fatal(e)
	}
	return p
}

func order(t *testing.T, r *mallRepo, p *biz.Product, b uint64) *biz.Order {
	t.Helper()
	o, e := r.CreateOrder(context.Background(), biz.OrderRequest{
		BuyerID:      b,
		ProductID:    p.ID,
		TicketTypeID: p.TicketTypes[0].ID,
		RequestID:    uuid.NewString(),
	})
	if e != nil {
		t.Fatal(e)
	}
	return o
}

func invariant(t *testing.T, r *mallRepo) {
	t.Helper()
	var n int64
	q := `SELECT count(*) FROM ticket_types t WHERE t.available<0 OR t.reserved<0 OR t.sold<0 OR t.withheld<0 OR
 t.available<>(SELECT coalesce(sum(available_delta),0) FROM inventory_events e WHERE e.ticket_type_id=t.id) OR
 t.reserved<>(SELECT coalesce(sum(reserved_delta),0) FROM inventory_events e WHERE e.ticket_type_id=t.id) OR
 t.sold<>(SELECT coalesce(sum(sold_delta),0) FROM inventory_events e WHERE e.ticket_type_id=t.id) OR
 t.withheld<>(SELECT coalesce(sum(withheld_delta),0) FROM inventory_events e WHERE e.ticket_type_id=t.id) OR
 t.reserved<>(SELECT count(*) FROM orders o WHERE o.ticket_type_id=t.id AND o.state='pending') OR
 t.sold<>(SELECT count(*) FROM orders o WHERE o.ticket_type_id=t.id AND o.state IN ('paid','refunding'))`
	if e := r.d.db.Raw(q).Scan(&n).Error; e != nil {
		t.Fatal(e)
	}
	if n != 0 {
		t.Fatalf("inventory invariant violated for %d ticket types", n)
	}
}

func TestConcurrentInventoryAndCrossTypeLimit(t *testing.T) {
	r, _ := testRepo(t)
	c := context.Background()
	p := product(t, r, 10)
	g := new(errgroup.Group)
	g.SetLimit(20)
	var successes atomic.Int64
	for i := 0; i < 80; i++ {
		g.Go(func() error {
			_, e := r.CreateOrder(c, biz.OrderRequest{
				BuyerID:      1,
				ProductID:    p.ID,
				TicketTypeID: p.TicketTypes[i%2].ID,
				RequestID:    uuid.NewString(),
			})
			if e == nil {
				successes.Add(1)
				return nil
			}
			if e != biz.ErrLimit {
				return e
			}
			return nil
		})
	}
	if e := g.Wait(); e != nil {
		t.Fatal(e)
	}
	if successes.Load() != 1 {
		t.Fatalf("same account obtained %d orders", successes.Load())
	}
	p = product(t, r, 7)
	successes.Store(0)
	for i := 0; i < 80; i++ {
		g.Go(func() error {
			_, e := r.CreateOrder(c, biz.OrderRequest{
				BuyerID:      uint64(i + 2),
				ProductID:    p.ID,
				TicketTypeID: p.TicketTypes[0].ID,
				RequestID:    uuid.NewString(),
			})
			if e == nil {
				successes.Add(1)
				return nil
			}
			if e != biz.ErrSoldOut {
				return e
			}
			return nil
		})
	}
	if e := g.Wait(); e != nil {
		t.Fatal(e)
	}
	if successes.Load() != 7 {
		t.Fatalf("stock 7 sold %d", successes.Load())
	}
	invariant(t, r)
}

func TestIdempotencyExpiryRefund(t *testing.T) {
	r, u := testRepo(t)
	c := context.Background()
	p := product(t, r, 2)
	req := biz.OrderRequest{
		BuyerID:      1,
		ProductID:    p.ID,
		TicketTypeID: p.TicketTypes[0].ID,
		RequestID:    "retry",
	}
	a, e := r.CreateOrder(c, req)
	if e != nil {
		t.Fatal(e)
	}
	g := new(errgroup.Group)
	g.SetLimit(10)
	for i := 0; i < 20; i++ {
		g.Go(func() error {
			o, e := r.CreateOrder(c, req)
			if e != nil {
				return e
			}
			if o.ID != a.ID {
				return fmt.Errorf("duplicate order")
			}
			return nil
		})
	}
	if e = g.Wait(); e != nil {
		t.Fatal(e)
	}
	req.TicketTypeID = p.TicketTypes[1].ID
	if _, e = r.CreateOrder(c, req); e != biz.ErrIdempotency {
		t.Fatalf("changed retry: %v", e)
	}
	if _, e = r.GetOrder(c, 2, a.ID); e != biz.ErrNotFound {
		t.Fatalf("ownership: %v", e)
	}
	if e = r.d.db.Exec("UPDATE orders SET expires_at=clock_timestamp()-interval '1 second' WHERE id=?", a.ID).Error; e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 10; i++ {
		g.Go(func() error {
			_, e := r.ExpireOrders(c, 100)
			return e
		})
	}
	if e = g.Wait(); e != nil {
		t.Fatal(e)
	}
	req.RequestID = "new-after-expiry"
	b, e := r.CreateOrder(c, req)
	if e != nil {
		t.Fatal(e)
	}
	if b.ExpiresAt.Sub(time.Now()) < 9*time.Minute {
		t.Fatal("hold is not ten minutes")
	}
	if _, e = u.PayOrder(c, 1, b.ID); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 10; i++ {
		g.Go(func() error {
			_, e := u.RefundOrder(c, 1, b.ID)
			return e
		})
	}
	if e = g.Wait(); e != nil {
		t.Fatal(e)
	}
	req.RequestID = "after-refund"
	if _, e = r.CreateOrder(c, req); e != biz.ErrLimit {
		t.Fatalf("refund must permanently prevent rebuy: %v", e)
	}
	p, e = r.GetProduct(c, p.ID)
	if e != nil {
		t.Fatal(e)
	}
	var refunded biz.TicketType
	for _, v := range p.TicketTypes {
		if v.ID == b.TicketTypeID {
			refunded = v
		}
	}
	if refunded.Withheld != 1 || refunded.Available != 1 {
		t.Fatalf("refund unexpectedly returned inventory: %+v", refunded)
	}
	adj := biz.InventoryAdjustment{
		ProductID:       p.ID,
		TicketTypeID:    refunded.ID,
		RequestID:       "release",
		Delta:           1,
		ReleaseWithheld: true,
		Reason:          "second release",
	}
	if _, e = r.AdjustInventory(c, adj); e != nil {
		t.Fatal(e)
	}
	if _, e = r.AdjustInventory(c, adj); e != nil {
		t.Fatal(e)
	}
	adj.Delta = 2
	if _, e = r.AdjustInventory(c, adj); e != biz.ErrIdempotency {
		t.Fatalf("adjustment retry: %v", e)
	}
	invariant(t, r)
}

func TestPaymentRecoveryAndLatePayment(t *testing.T) {
	r, u := testRepo(t)
	c := context.Background()
	p := product(t, r, 2)
	o := order(t, r, p, 1)
	if _, e := r.BeginPayment(c, 1, o.ID); e != nil {
		t.Fatal(e)
	}
	gateway := payment.NewSimulated(NewSimulatedStore(r.d))
	if _, e := gateway.Pay(c, *o); e != nil {
		t.Fatal(e)
	}
	// Simulate process death after provider commit but before local confirmation.
	if e := u.Maintain(c); e != nil {
		t.Fatal(e)
	}
	got, e := r.GetOrder(c, 1, o.ID)
	if e != nil || got.State != "paid" {
		t.Fatalf("recovery: %+v %v", got, e)
	}
	late := order(t, r, p, 2)
	if _, e = r.BeginPayment(c, 2, late.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = gateway.Pay(c, *late); e != nil {
		t.Fatal(e)
	}
	if e = r.d.db.Exec("UPDATE orders SET expires_at=clock_timestamp()-interval '1 second' WHERE id=?", late.ID).Error; e != nil {
		t.Fatal(e)
	}
	if e = u.Maintain(c); e != nil {
		t.Fatal(e)
	}
	got, e = r.GetOrder(c, 2, late.ID)
	if e != nil || got.State != "closed" || got.PaymentState != "refunded" {
		t.Fatalf("late payment: %+v %v", got, e)
	}
	// An automatic refund of a late payment does not consume the purchase right.
	order(t, r, p, 2)
	invariant(t, r)
}

func TestPayExpiryRace(t *testing.T) {
	r, _ := testRepo(t)
	c := context.Background()
	p := product(t, r, 30)
	g := new(errgroup.Group)
	g.SetLimit(12)
	for i := 0; i < 30; i++ {
		o := order(t, r, p, uint64(i+1))
		if e := r.d.db.Exec("UPDATE orders SET expires_at=clock_timestamp() WHERE id=?", o.ID).Error; e != nil {
			t.Fatal(e)
		}
		g.Go(func() error {
			_, e := r.ApplyPayment(c, o.BuyerID, o.ID, biz.PaymentResult{Reference: "provider:" + o.ID, Paid: true})
			return e
		})
		g.Go(func() error {
			_, e := r.ExpireOrders(c, 100)
			return e
		})
	}
	if e := g.Wait(); e != nil {
		t.Fatal(e)
	}
	invariant(t, r)
}

func TestNotOnSaleAndSoldOutRollback(t *testing.T) {
	r, _ := testRepo(t)
	p := product(t, r, 0)
	c := context.Background()
	req := biz.OrderRequest{
		BuyerID:      1,
		ProductID:    p.ID,
		TicketTypeID: p.TicketTypes[0].ID,
		RequestID:    "soldout",
	}
	if _, e := r.CreateOrder(c, req); e != biz.ErrSoldOut {
		t.Fatal(e)
	}
	if e := r.d.db.Exec("UPDATE products SET sale_starts_at=clock_timestamp()+interval '1 hour' WHERE id=?", p.ID).Error; e != nil {
		t.Fatal(e)
	}
	if _, e := r.CreateOrder(c, req); e != biz.ErrNotOnSale {
		t.Fatal(e)
	}
	var n int64
	r.d.db.Model(&claimRow{}).Count(&n)
	if n != 0 {
		t.Fatal("failed order left purchase claim")
	}
	invariant(t, r)
}

func TestPaymentAttemptCrashBeforeCharge(t *testing.T) {
	r, u := testRepo(t)
	c := context.Background()
	p := product(t, r, 1)
	o := order(t, r, p, 1)
	if _, e := r.BeginPayment(c, 1, o.ID); e != nil {
		t.Fatal(e)
	}
	if e := r.d.db.Exec("UPDATE orders SET expires_at=clock_timestamp()-interval '1 second' WHERE id=?", o.ID).Error; e != nil {
		t.Fatal(e)
	}
	if e := u.Maintain(c); e != nil {
		t.Fatal(e)
	}
	got, e := r.GetOrder(c, 1, o.ID)
	if e != nil || got.State != "closed" || got.PaymentState != "unpaid" {
		t.Fatalf("orphan attempt: %+v %v", got, e)
	}
	// A delayed in-flight charge cannot debit a provider order that was closed.
	result, e := payment.NewSimulated(NewSimulatedStore(r.d)).Pay(c, *o)
	if e != nil {
		t.Fatal(e)
	}
	if result.Paid {
		t.Fatal("provider charged after close")
	}
	order(t, r, p, 1)
	invariant(t, r)
}

func TestPaymentWinsAndRefundRecovery(t *testing.T) {
	r, u := testRepo(t)
	c := context.Background()
	p := product(t, r, 1)
	o := order(t, r, p, 1)
	if _, e := u.PayOrder(c, 1, o.ID); e != nil {
		t.Fatal(e)
	}
	if _, e := u.PayOrder(c, 1, o.ID); e != nil {
		t.Fatal(e)
	}
	if e := r.d.db.Exec("UPDATE orders SET expires_at=clock_timestamp()-interval '1 second' WHERE id=?", o.ID).Error; e != nil {
		t.Fatal(e)
	}
	if _, e := r.ExpireOrders(c, 100); e != nil {
		t.Fatal(e)
	}
	got, e := r.GetOrder(c, 1, o.ID)
	if e != nil || got.State != "paid" {
		t.Fatalf("paid order expired: %+v %v", got, e)
	}
	if _, e = r.RequestRefund(c, 1, o.ID); e != nil {
		t.Fatal(e)
	}
	if e = payment.NewSimulated(NewSimulatedStore(r.d)).Refund(c, *o); e != nil {
		t.Fatal(e)
	}
	// Retry after provider refund committed and application exited.
	if e = u.Maintain(c); e != nil {
		t.Fatal(e)
	}
	got, e = r.GetOrder(c, 1, o.ID)
	if e != nil || got.State != "refunded" {
		t.Fatalf("refund recovery: %+v %v", got, e)
	}
	invariant(t, r)
}

func TestCatalogueSeedAndPrecheck(t *testing.T) {
	r, _ := testRepo(t)
	c := context.Background()
	if e := seedCatalogue(r.d.db); e != nil {
		t.Fatal(e)
	}
	all, err := r.ListProducts(c, "BW")
	if err != nil || len(all) != 4 {
		t.Fatalf("BW search: %d %v", len(all), err)
	}
	products, e := r.ListProducts(c, "2026")
	if e != nil || len(products) != 1 || products[0].Year != 2026 {
		t.Fatalf("catalogue: %+v %v", products, e)
	}
	p := &products[0]
	req := biz.OrderRequest{
		BuyerID:      1,
		ProductID:    p.ID,
		TicketTypeID: p.TicketTypes[0].ID,
		RequestID:    "catalogue",
	}
	if _, e = r.Precheck(c, req); e != nil {
		t.Fatal(e)
	}
	o, e := r.CreateOrder(c, req)
	if e != nil {
		t.Fatal(e)
	}
	if e = seedCatalogue(r.d.db); e != nil {
		t.Fatal(e)
	}
	p, e = r.GetProduct(c, p.ID)
	if e != nil {
		t.Fatal(e)
	}
	for _, tType := range p.TicketTypes {
		if tType.ID == req.TicketTypeID && tType.Available != 99 {
			t.Fatal("seed reset stock")
		}
	}
	existing, e := r.Precheck(c, req)
	if e != nil || existing.ID != o.ID {
		t.Fatalf("retry precheck: %+v %v", existing, e)
	}
	req.RequestID = "other"
	req.TicketTypeID = p.TicketTypes[1].ID
	if _, e = r.Precheck(c, req); e != biz.ErrLimit {
		t.Fatalf("cross-type precheck: %v", e)
	}
	purchase, blocked, e := r.GetPurchase(c, 1, p.ID)
	if e != nil || blocked || purchase.ID != o.ID {
		t.Fatalf("purchase recovery: %+v %v %v", purchase, blocked, e)
	}
	invariant(t, r)
}
