package service

import (
	"context"
	"math"
	"strings"

	v1 "bilibili-lite/api/mall/v1"
	"bilibili-lite/internal/mall/biz"
	"bilibili-lite/internal/platform/auth"

	"google.golang.org/protobuf/types/known/timestamppb"
)

type MallUsecase interface {
	ListProducts(context.Context, string) ([]biz.Product, error)
	GetPurchase(context.Context, uint64, string) (*biz.Order, bool, error)
	GetProduct(context.Context, string) (*biz.Product, error)
	CreateProduct(context.Context, biz.Product) (*biz.Product, error)
	AdjustInventory(context.Context, biz.InventoryAdjustment) (*biz.Product, error)
	GetOrder(context.Context, uint64, string) (*biz.Order, error)
	CreateOrder(context.Context, biz.OrderRequest) (*biz.Order, error)
	PayOrder(context.Context, uint64, string) (*biz.Order, error)
	RefundOrder(context.Context, uint64, string) (*biz.Order, error)
}

type MallService struct {
	v1.UnimplementedMallServiceServer
	uc MallUsecase
}

func NewMallService(u MallUsecase) *MallService {
	return &MallService{uc: u}
}

func (s *MallService) ListProducts(ctx context.Context, r *v1.ListProductsRequest) (*v1.ProductsSet, error) {
	if len(r.Query) > 100 {
		return nil, biz.ErrInvalid
	}
	products, err := s.uc.ListProducts(ctx, strings.TrimSpace(r.Query))
	if err != nil {
		return nil, err
	}
	out := &v1.ProductsSet{Products: make([]*v1.Product, 0, len(products))}
	for _, product := range products {
		out.Products = append(out.Products, &v1.Product{
			Id:           product.ID,
			Name:         product.Name,
			Description:  product.Description,
			Year:         product.Year,
			SaleStartsAt: timestamppb.New(product.SaleStartsAt),
			TicketTypes:  ticketDTOs(product.TicketTypes),
		})
	}
	return out, nil
}

func (s *MallService) GetPurchase(ctx context.Context, r *v1.GetProductRequest) (*v1.Purchase, error) {
	buyerID, err := buyer(ctx)
	if err != nil {
		return nil, err
	}
	if !validID(r.Id) {
		return nil, biz.ErrInvalid
	}
	order, blocked, err := s.uc.GetPurchase(ctx, buyerID, r.Id)
	if err != nil {
		return nil, err
	}
	out := &v1.Purchase{PurchaseBlocked: blocked}
	if order != nil {
		out.Order = &v1.Order{
			Id:             order.ID,
			ProductId:      order.ProductID,
			TicketTypeId:   order.TicketTypeID,
			State:          order.State,
			PriceCents:     order.PriceCents,
			ProductName:    order.ProductName,
			TicketTypeName: order.TicketTypeName,
			ExpiresAt:      timestamppb.New(order.ExpiresAt),
			PaymentState:   order.PaymentState,
		}
	}
	return out, nil
}

func (s *MallService) GetOrder(ctx context.Context, r *v1.GetOrderRequest) (*v1.Order, error) {
	buyerID, err := buyer(ctx)
	if err != nil {
		return nil, err
	}
	if !validID(r.Id) {
		return nil, biz.ErrInvalid
	}
	order, err := s.uc.GetOrder(ctx, buyerID, r.Id)
	if err != nil {
		return nil, err
	}
	return &v1.Order{
		Id:             order.ID,
		ProductId:      order.ProductID,
		TicketTypeId:   order.TicketTypeID,
		State:          order.State,
		PriceCents:     order.PriceCents,
		ProductName:    order.ProductName,
		TicketTypeName: order.TicketTypeName,
		ExpiresAt:      timestamppb.New(order.ExpiresAt),
		PaymentState:   order.PaymentState,
	}, nil
}

func (s *MallService) GetProduct(ctx context.Context, r *v1.GetProductRequest) (*v1.Product, error) {
	if !validID(r.Id) {
		return nil, biz.ErrInvalid
	}
	product, err := s.uc.GetProduct(ctx, r.Id)
	if err != nil {
		return nil, err
	}
	return &v1.Product{
		Id:           product.ID,
		Name:         product.Name,
		SaleStartsAt: timestamppb.New(product.SaleStartsAt),
		TicketTypes:  ticketDTOs(product.TicketTypes),
		Description:  product.Description,
		Year:         product.Year,
	}, nil
}

func (s *MallService) CreateProduct(ctx context.Context, r *v1.CreateProductRequest) (*v1.Product, error) {
	if _, err := auth.RequireAdmin(ctx); err != nil {
		return nil, err
	}
	if strings.TrimSpace(r.Name) == "" ||
		len(r.Name) > 200 ||
		r.SaleStartsAt == nil ||
		r.SaleStartsAt.CheckValid() != nil ||
		len(r.TicketTypes) == 0 ||
		len(r.TicketTypes) > 20 {
		return nil, biz.ErrInvalid
	}
	product := biz.Product{Name: strings.TrimSpace(r.Name), SaleStartsAt: r.SaleStartsAt.AsTime()}
	for _, ticket := range r.TicketTypes {
		if ticket == nil ||
			strings.TrimSpace(ticket.Name) == "" ||
			len(ticket.Name) > 200 ||
			ticket.PriceCents <= 0 ||
			ticket.PriceCents > 100000000 ||
			ticket.Available < 0 ||
			ticket.Available > 100000000 {
			return nil, biz.ErrInvalid
		}
		product.TicketTypes = append(product.TicketTypes, biz.TicketType{
			Name:       ticket.Name,
			PriceCents: ticket.PriceCents,
			Available:  ticket.Available,
		})
	}
	out, err := s.uc.CreateProduct(ctx, product)
	if err != nil {
		return nil, err
	}
	return &v1.Product{
		Id:           out.ID,
		Name:         out.Name,
		SaleStartsAt: timestamppb.New(out.SaleStartsAt),
		TicketTypes:  ticketDTOs(out.TicketTypes),
		Description:  out.Description,
		Year:         out.Year,
	}, nil
}

func (s *MallService) AdjustInventory(ctx context.Context, r *v1.AdjustInventoryRequest) (*v1.Product, error) {
	if _, err := auth.RequireAdmin(ctx); err != nil {
		return nil, err
	}
	if !validID(r.ProductId) ||
		!validID(r.TicketTypeId) ||
		!validID(r.RequestId) ||
		strings.TrimSpace(r.Reason) == "" ||
		len(r.Reason) > 500 ||
		r.Delta == 0 ||
		r.Delta < -100000000 ||
		r.Delta > 100000000 ||
		(r.ReleaseWithheld && r.Delta < 0) {
		return nil, biz.ErrInvalid
	}
	product, err := s.uc.AdjustInventory(ctx, biz.InventoryAdjustment{
		ProductID:       r.ProductId,
		TicketTypeID:    r.TicketTypeId,
		RequestID:       r.RequestId,
		Delta:           r.Delta,
		ReleaseWithheld: r.ReleaseWithheld,
		Reason:          r.Reason,
	})
	if err != nil {
		return nil, err
	}
	return &v1.Product{
		Id:           product.ID,
		Name:         product.Name,
		SaleStartsAt: timestamppb.New(product.SaleStartsAt),
		TicketTypes:  ticketDTOs(product.TicketTypes),
		Description:  product.Description,
		Year:         product.Year,
	}, nil
}

// CreateOrder takes buyer identity from verified authentication, never from client input.
func (s *MallService) CreateOrder(ctx context.Context, request *v1.CreateOrderRequest) (*v1.Order, error) {
	buyerID, err := buyer(ctx)
	if err != nil {
		return nil, err
	}
	if !validID(request.ProductId) || !validID(request.TicketTypeId) || !validID(request.RequestId) {
		return nil, biz.ErrInvalid
	}
	order, err := s.uc.CreateOrder(ctx, biz.OrderRequest{
		BuyerID:      buyerID,
		ProductID:    request.ProductId,
		TicketTypeID: request.TicketTypeId,
		RequestID:    request.RequestId,
	})
	if err != nil {
		return nil, err
	}
	return &v1.Order{
		Id:             order.ID,
		ProductId:      order.ProductID,
		TicketTypeId:   order.TicketTypeID,
		State:          order.State,
		PriceCents:     order.PriceCents,
		ProductName:    order.ProductName,
		TicketTypeName: order.TicketTypeName,
		ExpiresAt:      timestamppb.New(order.ExpiresAt),
		PaymentState:   order.PaymentState,
	}, nil
}

func (s *MallService) PayOrder(ctx context.Context, r *v1.GetOrderRequest) (*v1.Order, error) {
	buyerID, err := buyer(ctx)
	if err != nil {
		return nil, err
	}
	if !validID(r.Id) {
		return nil, biz.ErrInvalid
	}
	order, err := s.uc.PayOrder(ctx, buyerID, r.Id)
	if err != nil {
		return nil, err
	}
	return &v1.Order{
		Id:             order.ID,
		ProductId:      order.ProductID,
		TicketTypeId:   order.TicketTypeID,
		State:          order.State,
		PriceCents:     order.PriceCents,
		ProductName:    order.ProductName,
		TicketTypeName: order.TicketTypeName,
		ExpiresAt:      timestamppb.New(order.ExpiresAt),
		PaymentState:   order.PaymentState,
	}, nil
}

func (s *MallService) RefundOrder(ctx context.Context, r *v1.GetOrderRequest) (*v1.Order, error) {
	buyerID, err := buyer(ctx)
	if err != nil {
		return nil, err
	}
	if !validID(r.Id) {
		return nil, biz.ErrInvalid
	}
	order, err := s.uc.RefundOrder(ctx, buyerID, r.Id)
	if err != nil {
		return nil, err
	}
	return &v1.Order{
		Id:             order.ID,
		ProductId:      order.ProductID,
		TicketTypeId:   order.TicketTypeID,
		State:          order.State,
		PriceCents:     order.PriceCents,
		ProductName:    order.ProductName,
		TicketTypeName: order.TicketTypeName,
		ExpiresAt:      timestamppb.New(order.ExpiresAt),
		PaymentState:   order.PaymentState,
	}, nil
}

func buyer(ctx context.Context) (uint64, error) {
	buyerID, err := auth.RequireUserID(ctx)
	if err != nil {
		return 0, err
	}
	if buyerID > math.MaxInt64 {
		return 0, biz.ErrInvalid
	}
	return buyerID, nil
}

func validID(s string) bool {
	return strings.TrimSpace(s) != "" && len(s) <= 128
}

func ticketDTOs(ts []biz.TicketType) []*v1.TicketType {
	out := make([]*v1.TicketType, 0, len(ts))
	for _, t := range ts {
		out = append(out, &v1.TicketType{
			Id:         t.ID,
			Name:       t.Name,
			PriceCents: t.PriceCents,
			Available:  t.Available,
			Reserved:   t.Reserved,
			Sold:       t.Sold,
			Withheld:   t.Withheld,
		})
	}
	return out
}
