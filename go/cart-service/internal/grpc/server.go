package grpc

import (
	"context"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/kabradshaw1/portfolio/go/cart-service/internal/model"
	pb "github.com/kabradshaw1/portfolio/go/cart-service/pb/cart/v1"
)

type CartServicer interface {
	GetCartRaw(ctx context.Context, userID uuid.UUID) ([]model.CartItem, error)
	ClearCart(ctx context.Context, userID uuid.UUID) error
	ReserveItems(ctx context.Context, userID uuid.UUID) error
	ReleaseItems(ctx context.Context, userID uuid.UUID) error
}

type CartGRPCServer struct {
	pb.UnimplementedCartServiceServer
	svc CartServicer
}

func NewCartGRPCServer(svc CartServicer) *CartGRPCServer {
	return &CartGRPCServer{svc: svc}
}

func (s *CartGRPCServer) GetCart(ctx context.Context, req *pb.GetCartRequest) (*pb.GetCartResponse, error) {
	userID, err := uuid.Parse(req.UserId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid user ID: %v", err)
	}

	items, err := s.svc.GetCartRaw(ctx, userID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get cart: %v", err)
	}

	resp := &pb.GetCartResponse{}
	for i := range items {
		resp.Items = append(resp.Items, modelToProto(&items[i]))
	}
	return resp, nil
}

func (s *CartGRPCServer) ClearCart(ctx context.Context, req *pb.ClearCartRequest) (*pb.ClearCartResponse, error) {
	userID, err := uuid.Parse(req.UserId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid user ID: %v", err)
	}

	if err := s.svc.ClearCart(ctx, userID); err != nil {
		return nil, status.Errorf(codes.Internal, "clear cart: %v", err)
	}

	return &pb.ClearCartResponse{}, nil
}

func (s *CartGRPCServer) ReserveItems(ctx context.Context, req *pb.ReserveItemsRequest) (*pb.ReserveItemsResponse, error) {
	userID, err := uuid.Parse(req.UserId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid user ID: %v", err)
	}
	if err := s.svc.ReserveItems(ctx, userID); err != nil {
		return nil, status.Errorf(codes.Internal, "reserve items: %v", err)
	}
	return &pb.ReserveItemsResponse{}, nil
}

func (s *CartGRPCServer) ReleaseItems(ctx context.Context, req *pb.ReleaseItemsRequest) (*pb.ReleaseItemsResponse, error) {
	userID, err := uuid.Parse(req.UserId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid user ID: %v", err)
	}
	if err := s.svc.ReleaseItems(ctx, userID); err != nil {
		return nil, status.Errorf(codes.Internal, "release items: %v", err)
	}
	return &pb.ReleaseItemsResponse{}, nil
}

func modelToProto(item *model.CartItem) *pb.CartItem {
	return &pb.CartItem{
		Id:        item.ID.String(),
		UserId:    item.UserID.String(),
		ProductId: item.ProductID.String(),
		Quantity:  int32(item.Quantity),
		CreatedAt: timestamppb.New(item.CreatedAt),
	}
}
