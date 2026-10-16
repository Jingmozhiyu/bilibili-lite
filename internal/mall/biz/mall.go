package biz

import (
	"context"
	stderrors "errors"
	"time"

	v1 "bilibili-lite/api/mall/v1"

	"github.com/go-kratos/kratos/v3/errors"
	"github.com/google/wire"
)

var ProviderSet = wire.NewSet(NewMallUsecase)
var (
	ErrInvalid     = errors.BadRequest(v1.ErrorReason_MALL_INVALID_ARGUMENT.String(), "invalid mall argument")
	ErrNotFound    = errors.NotFound(v1.ErrorReason_MALL_NOT_FOUND.String(), "resource not found")
	ErrLimit       = errors.Conflict(v1.ErrorReason_MALL_PURCHASE_LIMIT.String(), "account already has a purchase for this product")
	ErrSoldOut     = errors.Conflict(v1.ErrorReason_MALL_SOLD_OUT.String(), "ticket type sold out")
	ErrNotOnSale   = errors.Conflict(v1.ErrorReason_MALL_NOT_ON_SALE.String(), "sale has not started")
	ErrState       = errors.Conflict(v1.ErrorReason_MALL_ORDER_STATE.String(), "order state does not allow this operation")
	ErrIdempotency = errors.Conflict(v1.ErrorReason_MALL_IDEMPOTENCY_CONFLICT.String(), "request ID reused with different arguments")
	ErrStorage     = errors.InternalServer(v1.ErrorReason_MALL_STORAGE.String(), "mall storage unavailable")
	ErrPayment     = errors.ServiceUnavailable(v1.ErrorReason_MALL_PAYMENT_UNAVAILABLE.String(), "payment outcome unavailable; retry with the same order")
)

const HoldDuration = 10 * time.Minute

// TicketType holds independently priced inventory for one ticket tier.
type TicketType struct {
	ID         string
	ProductID  string
	Name       string
	PriceCents int64
	Available  int64
	Reserved   int64
	Sold       int64
	Withheld   int64
}

// Product represents one BW event; purchase eligibility is shared across its ticket tiers.
type Product struct {
	ID           string
	Name         string
	Description  string
	Year         int32
	SaleStartsAt time.Time
	TicketTypes  []TicketType
}

// Order snapshots the purchased ticket and tracks order and payment states separately.
// A closed order can still need a refund when a late payment succeeds.
type Order struct {
	ID             string
	ProductID      string
	TicketTypeID   string
	State          string
	ProductName    string
	TicketTypeName string
	PaymentState   string
	BuyerID        uint64
	PriceCents     int64
	ExpiresAt      time.Time
}

// OrderRequest identifies one purchase attempt. Retries must reuse RequestID.
type OrderRequest struct {
	BuyerID      uint64
	ProductID    string
	TicketTypeID string
	RequestID    string
}

// InventoryAdjustment records an operator change or releases previously refunded stock.
type InventoryAdjustment struct {
	ProductID       string
	TicketTypeID    string
	RequestID       string
	Reason          string
	Delta           int64
	ReleaseWithheld bool
}

type PaymentResult struct {
	Reference string
	Paid      bool
}

// MallRepo owns transactional persistence. Precheck is advisory; CreateOrder rechecks under locks.
type MallRepo interface {
	ListProducts(context.Context, string) ([]Product, error)
	GetPurchase(context.Context, uint64, string) (*Order, bool, error)
	Precheck(context.Context, OrderRequest) (*Order, error)
	CreateProduct(context.Context, Product) (*Product, error)
	GetProduct(context.Context, string) (*Product, error)
	AdjustInventory(context.Context, InventoryAdjustment) (*Product, error)
	CreateOrder(context.Context, OrderRequest) (*Order, error)
	GetOrder(context.Context, uint64, string) (*Order, error)
	BeginPayment(context.Context, uint64, string) (*Order, error)
	FinishUnpaid(context.Context, string) error
	PendingPayments(context.Context, int) ([]Order, error)
	ApplyPayment(context.Context, uint64, string, PaymentResult) (*Order, error)
	RequestRefund(context.Context, uint64, string) (*Order, error)
	CompleteRefund(context.Context, string) error
	ExpireOrders(context.Context, int) (int, error)
	PendingRefunds(context.Context, int) ([]Order, error)
}

// PaymentGateway implementations must deduplicate by order ID, including refunds.
// Close must fence future charges before returning; existing charges remain queryable.
// Refund returns nil only after confirmed completion, not merely provider acceptance.
type PaymentGateway interface {
	Close(context.Context, Order) error
	Query(context.Context, Order) (PaymentResult, error)
	Pay(context.Context, Order) (PaymentResult, error)
	Refund(context.Context, Order) error
}

// MallUsecase coordinates purchase protection, transactions and payment recovery.
type MallUsecase struct {
	repo     MallRepo
	payments PaymentGateway
	guard    PurchaseGuard
}

func NewMallUsecase(repo MallRepo, payments PaymentGateway, guard PurchaseGuard) *MallUsecase {
	return &MallUsecase{
		repo:     repo,
		payments: payments,
		guard:    guard,
	}
}

func (u *MallUsecase) ListProducts(ctx context.Context, q string) ([]Product, error) {
	return u.repo.ListProducts(ctx, q)
}

func (u *MallUsecase) GetPurchase(ctx context.Context, buyerID uint64, id string) (*Order, bool, error) {
	return u.repo.GetPurchase(ctx, buyerID, id)
}

func (u *MallUsecase) GetProduct(ctx context.Context, id string) (*Product, error) {
	return u.repo.GetProduct(ctx, id)
}

func (u *MallUsecase) GetOrder(ctx context.Context, buyerID uint64, id string) (*Order, error) {
	return u.repo.GetOrder(ctx, buyerID, id)
}

func (u *MallUsecase) CreateProduct(ctx context.Context, product Product) (*Product, error) {
	return u.repo.CreateProduct(ctx, product)
}

func (u *MallUsecase) AdjustInventory(ctx context.Context, adjustment InventoryAdjustment) (*Product, error) {
	return u.repo.AdjustInventory(ctx, adjustment)
}

// CreateOrder filters repeated requests before entering the bounded transaction stage.
func (u *MallUsecase) CreateOrder(ctx context.Context, request OrderRequest) (out *Order, err error) {
	defer func() {
		if u.guard != nil {
			result := "success"
			if err != nil {
				result = "error"
				if code := errors.FromError(err).Metadata["business_code"]; code != "" {
					result = code
				}
			}
			u.guard.ObserveResult(result)
		}
	}()
	// Isolated transaction tests omit the guard; production wiring always injects it.
	if u.guard == nil {
		return u.repo.CreateOrder(ctx, request)
	}

	// Check cooldown before any eligibility query; rejected retries must not extend the window.
	if err := u.guard.Check(ctx, request.BuyerID, request.ProductID); err != nil {
		return nil, err
	}
	existing, err := u.repo.Precheck(ctx, request)
	if err != nil {
		if stderrors.Is(err, ErrSoldOut) {
			return nil, PurchaseError(409, "100009", "该票种暂时无票", 0)
		}
		if stderrors.Is(err, ErrLimit) || stderrors.Is(err, ErrInvalid) || stderrors.Is(err, ErrNotFound) {
			return nil, PurchaseError(409, "100019", "当前账号无法购买此门票", 0)
		}
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}

	// Do not queue work beyond this instance's transaction capacity.
	release, ok := u.guard.Acquire()
	if !ok {
		if err = u.guard.Cooldown(ctx, request.BuyerID, request.ProductID, 5*time.Second); err != nil {
			return nil, err
		}
		return nil, PurchaseError(429, "221", "购票人数较多，请稍后再试", 5000)
	}
	defer release()

	// The precheck is a snapshot: another buyer may have taken the last ticket since then.
	order, err := u.repo.CreateOrder(ctx, request)
	if stderrors.Is(err, ErrSoldOut) {
		if err = u.guard.Cooldown(ctx, request.BuyerID, request.ProductID, 2*time.Second); err != nil {
			return nil, err
		}
		return nil, PurchaseError(409, "219", "余票刚刚售罄，请稍后再试", 2000)
	}
	if stderrors.Is(err, ErrLimit) {
		return nil, PurchaseError(409, "100019", "当前账号已有订单或购票记录", 0)
	}
	return order, err
}

// PayOrder persists payment intent before contacting the provider so a crash remains recoverable.
func (u *MallUsecase) PayOrder(ctx context.Context, buyerID uint64, id string) (*Order, error) {
	order, err := u.repo.GetOrder(ctx, buyerID, id)
	if err != nil {
		return nil, err
	}
	if order.State == "paid" {
		return order, nil
	}
	if order.State != "pending" {
		return nil, ErrState
	}

	// This commit precedes the external charge. Maintain can query any unfinished attempt.
	order, err = u.repo.BeginPayment(ctx, buyerID, id)
	if err != nil {
		return nil, err
	}
	if order.State == "paid" {
		return order, nil
	}
	result, err := u.payments.Pay(ctx, *order)
	if err != nil {
		return nil, ErrPayment
	}
	return u.repo.ApplyPayment(ctx, buyerID, id, result)
}

// RefundOrder releases sold stock only after the provider confirms the refund.
func (u *MallUsecase) RefundOrder(ctx context.Context, buyerID uint64, id string) (*Order, error) {
	order, err := u.repo.RequestRefund(ctx, buyerID, id)
	if err != nil {
		return nil, err
	}
	if order.State == "refunded" {
		return order, nil
	}
	if err = u.payments.Refund(ctx, *order); err != nil {
		return nil, ErrPayment
	}
	if err = u.repo.CompleteRefund(ctx, id); err != nil {
		return nil, err
	}
	return u.repo.GetOrder(ctx, buyerID, id)
}

// Maintain closes expired holds and reconciles interrupted payments and refunds in bounded batches.
func (u *MallUsecase) Maintain(ctx context.Context) error {
	if _, err := u.repo.ExpireOrders(ctx, 100); err != nil {
		return err
	}
	pending, err := u.repo.PendingPayments(ctx, 100)
	if err != nil {
		return err
	}
	var failures []error
	for _, order := range pending {
		// Fence future charges before declaring a closed order unpaid. Existing charges remain queryable.
		if order.State == "closed" {
			if err = u.payments.Close(ctx, order); err != nil {
				failures = append(failures, ErrPayment)
				continue
			}
		}
		payment, err := u.payments.Query(ctx, order)
		if err != nil {
			failures = append(failures, ErrPayment)
			continue
		}
		if payment.Paid {
			_, err = u.repo.ApplyPayment(ctx, order.BuyerID, order.ID, payment)
		} else if order.State == "closed" {
			err = u.repo.FinishUnpaid(ctx, order.ID)
		}
		if err != nil {
			failures = append(failures, err)
		}
	}
	orders, err := u.repo.PendingRefunds(ctx, 100)
	if err != nil {
		return err
	}
	for _, order := range orders {
		if err = u.payments.Refund(ctx, order); err != nil {
			failures = append(failures, ErrPayment)
			continue
		}
		if err = u.repo.CompleteRefund(ctx, order.ID); err != nil {
			failures = append(failures, err)
		}
	}
	return stderrors.Join(failures...)
}
