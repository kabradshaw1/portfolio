package grpc

import (
	"context"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/kabradshaw1/portfolio/go/product-service/internal/model"
	pb "github.com/kabradshaw1/portfolio/go/product-service/internal/pb/product/v1"
)

type ProductServicer interface {
	List(ctx context.Context, params model.ProductListParams) ([]model.Product, int, error)
	GetByID(ctx context.Context, id uuid.UUID) (*model.Product, error)
	Categories(ctx context.Context) ([]string, error)
	InvalidateCache(ctx context.Context) error
}

type StockDecrementer interface {
	DecrementStock(ctx context.Context, productID uuid.UUID, qty int) error
}

type ProductGRPCServer struct {
	pb.UnimplementedProductServiceServer
	svc   ProductServicer
	stock StockDecrementer
}

func NewProductGRPCServer(svc ProductServicer, opts ...func(*ProductGRPCServer)) *ProductGRPCServer {
	s := &ProductGRPCServer{svc: svc}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func WithStockDecrementer(d StockDecrementer) func(*ProductGRPCServer) {
	return func(s *ProductGRPCServer) { s.stock = d }
}

func modelToProto(p *model.Product) *pb.Product {
	return &pb.Product{
		Id:          p.ID.String(),
		Name:        p.Name,
		Description: p.Description,
		Price:       int32(p.Price),
		Category:    p.Category,
		ImageUrl:    p.ImageURL,
		Stock:       int32(p.Stock),
		CreatedAt:   timestamppb.New(p.CreatedAt),
		UpdatedAt:   timestamppb.New(p.UpdatedAt),
	}
}

func (s *ProductGRPCServer) GetProduct(ctx context.Context, req *pb.GetProductRequest) (*pb.Product, error) {
	id, err := uuid.Parse(req.Id)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid product ID: %v", err)
	}

	product, err := s.svc.GetByID(ctx, id)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "product not found: %v", err)
	}

	return modelToProto(product), nil
}

func (s *ProductGRPCServer) GetProducts(ctx context.Context, req *pb.GetProductsRequest) (*pb.GetProductsResponse, error) {
	params := model.ProductListParams{
		Category: req.Category,
		Query:    req.Query,
		Sort:     req.Sort,
		Page:     int(req.Page),
		Limit:    int(req.Limit),
		Cursor:   req.Cursor,
	}

	if params.Limit <= 0 {
		params.Limit = 20
	}
	if params.Page <= 0 {
		params.Page = 1
	}

	products, total, err := s.svc.List(ctx, params)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list products: %v", err)
	}

	resp := &pb.GetProductsResponse{
		Total:   int32(total),
		Page:    int32(params.Page),
		Limit:   int32(params.Limit),
		HasMore: len(products) > int(params.Limit),
	}

	for i := range products {
		resp.Products = append(resp.Products, modelToProto(&products[i]))
	}

	return resp, nil
}

func (s *ProductGRPCServer) CheckAvailability(ctx context.Context, req *pb.CheckAvailabilityRequest) (*pb.CheckAvailabilityResponse, error) {
	id, err := uuid.Parse(req.ProductId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid product ID: %v", err)
	}

	product, err := s.svc.GetByID(ctx, id)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "product not found: %v", err)
	}

	return &pb.CheckAvailabilityResponse{
		Available:    product.Stock >= int(req.Quantity),
		CurrentStock: int32(product.Stock),
		Price:        int32(product.Price),
	}, nil
}

func (s *ProductGRPCServer) DecrementStock(ctx context.Context, req *pb.DecrementStockRequest) (*pb.DecrementStockResponse, error) {
	if s.stock == nil {
		return nil, status.Errorf(codes.Unimplemented, "stock decrement not configured")
	}

	id, err := uuid.Parse(req.ProductId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid product ID: %v", err)
	}

	if err := s.stock.DecrementStock(ctx, id, int(req.Quantity)); err != nil {
		return nil, status.Errorf(codes.FailedPrecondition, "decrement stock: %v", err)
	}

	product, err := s.svc.GetByID(ctx, id)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "fetch updated product: %v", err)
	}

	return &pb.DecrementStockResponse{
		RemainingStock: int32(product.Stock),
	}, nil
}

func (s *ProductGRPCServer) InvalidateCache(ctx context.Context, _ *pb.InvalidateCacheRequest) (*pb.InvalidateCacheResponse, error) {
	if err := s.svc.InvalidateCache(ctx); err != nil {
		return nil, status.Errorf(codes.Internal, "invalidate cache: %v", err)
	}
	return &pb.InvalidateCacheResponse{}, nil
}
