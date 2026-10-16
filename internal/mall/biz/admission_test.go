package biz

import (
	"context"
	"github.com/go-kratos/kratos/v3/errors"
	"testing"
	"time"
)

type admissionRepo struct {
	MallRepo
	preErr    error
	createErr error
	created   bool
	checked   bool
	prior     *Order
}

func (r *admissionRepo) Precheck(context.Context, OrderRequest) (*Order, error) {
	r.checked = true
	return r.prior, r.preErr
}

func (r *admissionRepo) CreateOrder(context.Context, OrderRequest) (*Order, error) {
	r.created = true
	return &Order{ID: "new"}, r.createErr
}

type guardFake struct {
	checkErr error
	full     bool
	released bool
	cooldown time.Duration
}

func (g *guardFake) Check(context.Context, uint64, string) error {
	return g.checkErr
}

func (g *guardFake) Cooldown(_ context.Context, _ uint64, _ string, d time.Duration) error {
	g.cooldown = d
	return nil
}

func (g *guardFake) Acquire() (func(), bool) {
	if g.full {
		return nil, false
	}
	return func() {
		g.released = true
	}, true
}

func TestProtectedPurchaseDecisions(t *testing.T) {
	for _, test := range []struct {
		name     string
		code     string
		pre      error
		inner    error
		full     bool
		cooldown time.Duration
	}{
		{
			"precheck soldout",
			"100009",
			ErrSoldOut,
			nil,
			false,
			0,
		},
		{
			"already purchased",
			"100019",
			ErrLimit,
			nil,
			false,
			0,
		},
		{
			"inner soldout",
			"219",
			nil,
			ErrSoldOut,
			false,
			2 * time.Second,
		},
		{
			"inner full",
			"221",
			nil,
			nil,
			true,
			5 * time.Second,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := &admissionRepo{preErr: test.pre, createErr: test.inner}
			g := &guardFake{full: test.full}
			u := NewMallUsecase(r, nil, g)
			_, e := u.CreateOrder(context.Background(), OrderRequest{})
			if errors.FromError(e).Metadata["business_code"] != test.code || g.cooldown != test.cooldown {
				t.Fatalf("%v cooldown %v", e, g.cooldown)
			}
			if test.inner != nil && !g.released {
				t.Fatal("slot leaked")
			}
			if (test.pre != nil || test.full) && r.created {
				t.Fatal("bypassed rejection")
			}
		})
	}
}

func TestCooldownStopsAllPrechecks(t *testing.T) {
	r := &admissionRepo{}
	g := &guardFake{checkErr: PurchaseError(429, "3", "cooldown", 1000)}
	u := NewMallUsecase(r, nil, g)
	u.CreateOrder(context.Background(), OrderRequest{})
	if r.checked || r.created {
		t.Fatal("cooldown reached database")
	}
}

func TestIdempotentRetryBypassesFinalInventoryCheck(t *testing.T) {
	r := &admissionRepo{prior: &Order{ID: "existing"}}
	g := &guardFake{full: true}
	o, e := NewMallUsecase(r, nil, g).CreateOrder(context.Background(), OrderRequest{})
	if e != nil || o.ID != "existing" || r.created {
		t.Fatalf("retry: %+v %v", o, e)
	}
}

func (g *guardFake) ObserveResult(string) {}
