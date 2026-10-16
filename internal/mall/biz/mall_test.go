package biz

import (
	"context"
	"errors"
	"testing"
)

type repoFake struct {
	MallRepo
	order    Order
	beginErr error
	marked   bool
	complete bool
}

func (r *repoFake) GetOrder(context.Context, uint64, string) (*Order, error) {
	o := r.order
	return &o, nil
}

func (r *repoFake) BeginPayment(context.Context, uint64, string) (*Order, error) {
	if r.beginErr != nil {
		return nil, r.beginErr
	}
	r.marked = true
	o := r.order
	return &o, nil
}

func (r *repoFake) RequestRefund(context.Context, uint64, string) (*Order, error) {
	r.marked = true
	o := r.order
	o.State = "refunding"
	return &o, nil
}

func (r *repoFake) CompleteRefund(context.Context, string) error {
	r.complete = true
	return nil
}

type gatewayFake struct {
	PaymentGateway
	called bool
	err    error
}

func (g *gatewayFake) Pay(context.Context, Order) (PaymentResult, error) {
	g.called = true
	return PaymentResult{}, g.err
}

func (g *gatewayFake) Refund(context.Context, Order) error {
	g.called = true
	return g.err
}

func TestPaymentAttemptIsDurableBeforeExternalCall(t *testing.T) {
	r := &repoFake{order: Order{State: "pending"}, beginErr: ErrStorage}
	g := &gatewayFake{}
	u := NewMallUsecase(r, g, nil)
	if _, e := u.PayOrder(context.Background(), 1, "order"); e != ErrStorage || g.called {
		t.Fatalf("called provider without durable attempt: %v", e)
	}
	r.beginErr = nil
	g.err = errors.New("network timeout")
	if _, e := u.PayOrder(context.Background(), 1, "order"); e != ErrPayment || !r.marked {
		t.Fatalf("unknown outcome not recoverable: %v", e)
	}
}

func TestRefundFailureDoesNotReleaseInventory(t *testing.T) {
	r := &repoFake{order: Order{State: "paid"}}
	g := &gatewayFake{err: errors.New("provider unavailable")}
	u := NewMallUsecase(r, g, nil)
	if _, e := u.RefundOrder(context.Background(), 1, "order"); e != ErrPayment {
		t.Fatal(e)
	}
	if !r.marked || r.complete {
		t.Fatal("refund must remain pending until provider confirms")
	}
}
