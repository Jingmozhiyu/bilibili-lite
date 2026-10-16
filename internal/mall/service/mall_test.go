package service

import (
	"context"
	"testing"
	"time"

	v1 "bilibili-lite/api/mall/v1"
	"bilibili-lite/internal/mall/biz"
	"bilibili-lite/internal/platform/auth"

	"google.golang.org/protobuf/types/known/timestamppb"
)

type fakeUsecase struct {
	MallUsecase
	request biz.OrderRequest
}

func (f *fakeUsecase) CreateOrder(_ context.Context, r biz.OrderRequest) (*biz.Order, error) {
	f.request = r
	return &biz.Order{
		ID:        "order",
		BuyerID:   r.BuyerID,
		ExpiresAt: time.Now(),
	}, nil
}

func TestBoundaryAuthAndValidation(t *testing.T) {
	f := &fakeUsecase{}
	s := NewMallService(f)
	c := context.Background()
	if _, e := s.CreateOrder(c, &v1.CreateOrderRequest{
		ProductId:    "p",
		TicketTypeId: "t",
		RequestId:    "r",
	}); e != auth.ErrSessionInvalid {
		t.Fatal(e)
	}
	c = auth.WithUserID(c, 42)
	if _, e := s.CreateOrder(c, &v1.CreateOrderRequest{}); e != biz.ErrInvalid {
		t.Fatal(e)
	}
	if _, e := s.CreateProduct(c, &v1.CreateProductRequest{}); e != auth.ErrUserForbidden {
		t.Fatal(e)
	}
	if _, e := s.CreateOrder(c, &v1.CreateOrderRequest{
		ProductId:    "p",
		TicketTypeId: "t",
		RequestId:    "r",
	}); e != nil {
		t.Fatal(e)
	}
	if f.request.BuyerID != 42 {
		t.Fatal("buyer did not come from verified identity")
	}
	admin := auth.WithIdentity(context.Background(), auth.Identity{UserID: 1, IsAdmin: true})
	if _, e := s.CreateProduct(admin, &v1.CreateProductRequest{
		Name:         "test",
		SaleStartsAt: timestamppb.Now(),
		TicketTypes:  []*v1.TicketType{{Name: "bad", PriceCents: -1}},
	}); e != biz.ErrInvalid {
		t.Fatal(e)
	}
	if _, e := s.AdjustInventory(admin, &v1.AdjustInventoryRequest{
		ProductId:       "p",
		TicketTypeId:    "t",
		RequestId:       "r",
		Reason:          "test",
		Delta:           -1,
		ReleaseWithheld: true,
	}); e != biz.ErrInvalid {
		t.Fatal(e)
	}
}
